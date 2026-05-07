package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

const (
	labelInstanceName = "app.kubernetes.io/name"
	labelManagedBy    = "app.kubernetes.io/managed-by"
	labelComponent    = "app.kubernetes.io/component"
	operatorName      = "redis-operator"
	componentRedis    = "redis"
	componentSentinel = "sentinel"

	sentinelReplicas = int32(3)
)

// RedisInstanceReconciler reconciles RedisInstance objects.
type RedisInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update;patch;delete

// Reconcile reconciles a RedisInstance.
func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ri := &redisv1alpha1.RedisInstance{}
	if err := r.Get(ctx, req.NamespacedName, ri); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Step 2: add finalizer if absent.
	if !controllerutil.ContainsFinalizer(ri, redisv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ri, redisv1alpha1.FinalizerName)
		return ctrl.Result{}, r.Update(ctx, ri)
	}

	// Step 3: handle deletion.
	if !ri.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, ri)
	}

	// Step 4: generation gate — skip resource mutation only when generation matches
	// AND all required owned resources already exist.
	skipMutation := ri.Status.ObservedGeneration == ri.Generation
	if skipMutation {
		allExist, err := r.allResourcesExist(ctx, ri)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !allExist {
			skipMutation = false
		}
	}

	if !skipMutation {
		// Steps 5–8: reconcile owned resources.
		if err := r.reconcileConfigMap(ctx, ri); err != nil {
			logger.Error(err, "failed to reconcile ConfigMap")
			return ctrl.Result{}, err
		}
		if err := r.reconcileService(ctx, ri); err != nil {
			logger.Error(err, "failed to reconcile Service")
			return ctrl.Result{}, err
		}
		if err := r.reconcileStatefulSet(ctx, ri); err != nil {
			logger.Error(err, "failed to reconcile StatefulSet")
			return ctrl.Result{}, err
		}
		if ri.Spec.Topology == redisv1alpha1.TopologySentinel {
			if err := r.reconcileSentinel(ctx, ri); err != nil {
				logger.Error(err, "failed to reconcile Sentinel")
				return ctrl.Result{}, err
			}
		}
	}

	// Check whether a scale-down is still in progress.
	scalingDown, err := r.isScalingDown(ctx, ri)
	if err != nil {
		return ctrl.Result{}, err
	}
	if scalingDown {
		if err := r.updateStatus(ctx, ri); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Step 9: update status.
	return ctrl.Result{}, r.updateStatus(ctx, ri)
}

// allResourcesExist returns true when every owned resource required by ri exists.
func (r *RedisInstanceReconciler) allResourcesExist(ctx context.Context, ri *redisv1alpha1.RedisInstance) (bool, error) {
	type namespacedName = client.ObjectKey
	names := []namespacedName{
		{Namespace: ri.Namespace, Name: ri.Name + "-config"},
		{Namespace: ri.Namespace, Name: ri.Name},
		{Namespace: ri.Namespace, Name: ri.Name},
	}
	objFactories := []func() client.Object{
		func() client.Object { return &corev1.ConfigMap{} },
		func() client.Object { return &corev1.Service{} },
		func() client.Object { return &appsv1.StatefulSet{} },
	}
	if ri.Spec.Topology == redisv1alpha1.TopologySentinel {
		names = append(names,
			namespacedName{Namespace: ri.Namespace, Name: ri.Name + "-sentinel"},
			namespacedName{Namespace: ri.Namespace, Name: ri.Name + "-sentinel"},
		)
		objFactories = append(objFactories,
			func() client.Object { return &appsv1.StatefulSet{} },
			func() client.Object { return &corev1.Service{} },
		)
	}
	for i, nn := range names {
		obj := objFactories[i]()
		if err := r.Get(ctx, nn, obj); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}

// isScalingDown returns true when there are still Redis pods above the desired replica count.
func (r *RedisInstanceReconciler) isScalingDown(ctx context.Context, ri *redisv1alpha1.RedisInstance) (bool, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(ri.Namespace),
		client.MatchingLabels(podLabels(ri.Name, componentRedis)),
	); err != nil {
		return false, err
	}
	for _, pod := range podList.Items {
		if ordinalFromPodName(pod.Name, ri.Name) >= ri.Spec.Replicas {
			return true, nil
		}
	}
	return false, nil
}

// SetupWithManager registers the controller with the manager.
func (r *RedisInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1alpha1.RedisInstance{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
