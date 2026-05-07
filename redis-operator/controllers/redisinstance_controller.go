package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

// RedisInstanceReconciler reconciles RedisInstance objects.
type RedisInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update;patch;delete

// Reconcile reconciles a RedisInstance resource.
func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &redisv1alpha1.RedisInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle finalizer
	if instance.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(instance, redisv1alpha1.FinalizerName) {
			controllerutil.AddFinalizer(instance, redisv1alpha1.FinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		// Object is being deleted
		if controllerutil.ContainsFinalizer(instance, redisv1alpha1.FinalizerName) {
			if err := r.teardown(ctx, instance); err != nil {
				logger.Error(err, "teardown failed")
				return ctrl.Result{RequeueAfter: 10 * time.Second}, err
			}
			controllerutil.RemoveFinalizer(instance, redisv1alpha1.FinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Check auth secret if specified
	if instance.Spec.Auth != nil && instance.Spec.Auth.PasswordSecretRef != nil {
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      instance.Spec.Auth.PasswordSecretRef.Name,
		}
		if err := r.Get(ctx, secretKey, secret); err != nil {
			if errors.IsNotFound(err) {
				r.Recorder.Event(instance, corev1.EventTypeWarning, "MissingSecret",
					fmt.Sprintf("referenced Secret %s not found", instance.Spec.Auth.PasswordSecretRef.Name))
				if updateErr := r.updatePhase(ctx, instance, redisv1alpha1.PhaseFailed); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			return ctrl.Result{}, err
		}
	}

	// Generation gating: skip mutation only when generation is current AND all resources exist
	if instance.Status.ObservedGeneration == instance.Generation {
		allExist, err := r.allOwnedResourcesExist(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}
		if allExist {
			// Only refresh status
			return r.reconcileStatus(ctx, instance)
		}
	}

	// Reconcile ConfigMap
	if err := r.reconcileConfigMap(ctx, instance); err != nil {
		logger.Error(err, "failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Reconcile headless Service
	if err := r.reconcileService(ctx, instance); err != nil {
		logger.Error(err, "failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Reconcile StatefulSet
	if err := r.reconcileStatefulSet(ctx, instance); err != nil {
		logger.Error(err, "failed to reconcile StatefulSet")
		return ctrl.Result{}, err
	}

	// Reconcile Sentinel resources if topology is sentinel
	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		if err := r.reconcileSentinel(ctx, instance); err != nil {
			logger.Error(err, "failed to reconcile Sentinel")
			return ctrl.Result{}, err
		}
	}

	// Update status
	return r.reconcileStatus(ctx, instance)
}

// allOwnedResourcesExist checks if all required owned resources exist.
func (r *RedisInstanceReconciler) allOwnedResourcesExist(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: configMapName(instance)}, cm); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, svc); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, sts); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		sentinelSts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: sentinelStatefulSetName(instance)}, sentinelSts); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
	}

	return true, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1alpha1.RedisInstance{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
