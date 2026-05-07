package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

func (r *RedisInstanceReconciler) reconcileStatus(ctx context.Context, instance *redisv1alpha1.RedisInstance) (ctrl.Result, error) {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, sts); err != nil {
		return ctrl.Result{}, err
	}

	readyReplicas := sts.Status.ReadyReplicas
	desiredReplicas := instance.Spec.Replicas

	var newPhase redisv1alpha1.RedisPhase
	switch {
	case readyReplicas == 0:
		newPhase = redisv1alpha1.PhasePending
	case readyReplicas == desiredReplicas:
		newPhase = redisv1alpha1.PhaseRunning
	default:
		newPhase = redisv1alpha1.PhaseDegraded
	}

	oldPhase := instance.Status.Phase
	instance.Status.Phase = newPhase
	instance.Status.ReadyReplicas = readyReplicas
	instance.Status.ObservedGeneration = instance.Generation
	instance.Status.MasterEndpoint = fmt.Sprintf("%s-0.%s.%s.svc.cluster.local:6379",
		instance.Name, instance.Name, instance.Namespace)

	// Update sentinel endpoints
	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		sentinelReplicas := int32(3)
		endpoints := make([]string, 0, sentinelReplicas)
		for i := int32(0); i < sentinelReplicas; i++ {
			endpoints = append(endpoints,
				fmt.Sprintf("%s-sentinel-%d.%s-sentinel.%s.svc.cluster.local:26379",
					instance.Name, i, instance.Name, instance.Namespace))
		}
		instance.Status.SentinelEndpoints = endpoints
	} else {
		instance.Status.SentinelEndpoints = nil
	}

	// Update conditions
	r.setCondition(instance, "Ready", newPhase == redisv1alpha1.PhaseRunning)
	r.setCondition(instance, "Degraded", newPhase == redisv1alpha1.PhaseDegraded)

	// Emit events on phase transitions
	if oldPhase != newPhase && oldPhase != "" {
		switch newPhase {
		case redisv1alpha1.PhaseRunning:
			r.Recorder.Event(instance, corev1.EventTypeNormal, "InstanceRunning",
				fmt.Sprintf("RedisInstance %s is now Running (%d/%d replicas ready)", instance.Name, readyReplicas, desiredReplicas))
		case redisv1alpha1.PhaseDegraded:
			r.Recorder.Event(instance, corev1.EventTypeWarning, "InstanceDegraded",
				fmt.Sprintf("RedisInstance %s is Degraded (%d/%d replicas ready)", instance.Name, readyReplicas, desiredReplicas))
		case redisv1alpha1.PhaseFailed:
			r.Recorder.Event(instance, corev1.EventTypeWarning, "InstanceFailed",
				fmt.Sprintf("RedisInstance %s has Failed", instance.Name))
		}
	}

	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RedisInstanceReconciler) updatePhase(ctx context.Context, instance *redisv1alpha1.RedisInstance, phase redisv1alpha1.RedisPhase) error {
	instance.Status.Phase = phase
	return r.Status().Update(ctx, instance)
}

func (r *RedisInstanceReconciler) setCondition(instance *redisv1alpha1.RedisInstance, condType string, status bool) {
	condStatus := metav1.ConditionFalse
	if status {
		condStatus = metav1.ConditionTrue
	}
	cond := metav1.Condition{
		Type:               condType,
		Status:             condStatus,
		ObservedGeneration: instance.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             condType,
	}
	meta.SetStatusCondition(&instance.Status.Conditions, cond)
}
