package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

func (r *RedisInstanceReconciler) teardown(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	// 1. Delete Sentinel resources first (if sentinel topology)
	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		sentinelSts := &appsv1.StatefulSet{}
		err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: sentinelStatefulSetName(instance)}, sentinelSts)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("getting sentinel statefulset: %w", err)
		}
		if err == nil {
			if delErr := r.Delete(ctx, sentinelSts); delErr != nil && !errors.IsNotFound(delErr) {
				return fmt.Errorf("deleting sentinel statefulset: %w", delErr)
			}
		}
	}

	// 2. Delete Redis StatefulSet (reverse ordinal ordering is handled by K8s OrderedReadyPodManagement)
	redisSts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, redisSts)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("getting redis statefulset: %w", err)
	}
	if err == nil {
		if delErr := r.Delete(ctx, redisSts); delErr != nil && !errors.IsNotFound(delErr) {
			return fmt.Errorf("deleting redis statefulset: %w", delErr)
		}
	}

	// 3. Delete owned PVCs
	pvcList := &corev1.PersistentVolumeClaimList{}
	labelSelector := labels.SelectorFromSet(labelsForInstance(instance))
	if err := r.List(ctx, pvcList, &client.ListOptions{
		Namespace:     instance.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return fmt.Errorf("listing PVCs: %w", err)
	}
	for i := range pvcList.Items {
		if delErr := r.Delete(ctx, &pvcList.Items[i]); delErr != nil && !errors.IsNotFound(delErr) {
			return fmt.Errorf("deleting PVC %s: %w", pvcList.Items[i].Name, delErr)
		}
	}

	return nil
}
