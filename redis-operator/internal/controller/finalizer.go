package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

// handleDeletion performs ordered teardown and removes the finalizer.
//
// Order:
//  1. Stop Sentinel StatefulSet (if applicable).
//  2. Delete the Redis StatefulSet and wait for pods to terminate.
//  3. Delete owned PVCs.
//  4. Remove the finalizer.
func (r *RedisInstanceReconciler) handleDeletion(ctx context.Context, ri *redisv1alpha1.RedisInstance) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(ri, redisv1alpha1.FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Step 1: stop Sentinel resources first.
	if ri.Spec.Topology == redisv1alpha1.TopologySentinel {
		if err := r.deleteSentinelResources(ctx, ri); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Step 2: delete the Redis StatefulSet.
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, namespacedName(ri.Namespace, ri.Name), sts); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	} else {
		if err := r.Delete(ctx, sts); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Wait until all Redis pods have terminated.
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(ri.Namespace),
		client.MatchingLabels(podLabels(ri.Name, componentRedis)),
	); err != nil {
		return ctrl.Result{}, err
	}
	if len(podList.Items) > 0 {
		// Pods still terminating — requeue and check again.
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Step 3: delete owned PVCs.
	if err := r.deleteOwnedPVCs(ctx, ri); err != nil {
		return ctrl.Result{}, err
	}

	// Step 4: remove the finalizer so the CR can be garbage collected.
	controllerutil.RemoveFinalizer(ri, redisv1alpha1.FinalizerName)
	return ctrl.Result{}, r.Update(ctx, ri)
}

// deleteOwnedPVCs deletes all PVCs created by the Redis StatefulSet volumeClaimTemplates.
// It uses a label selector to discover PVCs rather than iterating up to spec.replicas,
// so previously scaled-down replicas (with ordinals above the current spec) are also cleaned up.
func (r *RedisInstanceReconciler) deleteOwnedPVCs(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcList,
		client.InNamespace(ri.Namespace),
		client.MatchingLabels(resourceLabels(ri.Name, componentRedis)),
	); err != nil {
		return fmt.Errorf("list owned pvcs: %w", err)
	}
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("delete pvc %s: %w", pvc.Name, err)
		}
	}
	return nil
}
