package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

// updateStatus reads the current StatefulSet state and updates status on the CR.
func (r *RedisInstanceReconciler) updateStatus(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	// Work on a fresh copy to avoid conflicts with the object we reconciled.
	patch := client.MergeFrom(ri.DeepCopy())

	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, namespacedName(ri.Namespace, ri.Name), sts); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		ri.Status.Phase = redisv1alpha1.PhasePending
		ri.Status.ReadyReplicas = 0
	} else {
		ri.Status.ReadyReplicas = sts.Status.ReadyReplicas
	}

	// Determine phase.
	previousPhase := ri.Status.Phase
	ri.Status.Phase = computePhase(ri.Status.ReadyReplicas, ri.Spec.Replicas)

	// Stable fields.
	ri.Status.ObservedGeneration = ri.Generation
	ri.Status.MasterEndpoint = primaryDNS(ri.Name, ri.Namespace)

	// Sentinel endpoints.
	if ri.Spec.Topology == redisv1alpha1.TopologySentinel {
		endpoints := make([]string, 0, sentinelReplicas)
		for i := int32(0); i < sentinelReplicas; i++ {
			endpoints = append(endpoints, sentinelPodDNS(ri.Name, ri.Namespace, i))
		}
		ri.Status.SentinelEndpoints = endpoints
	} else {
		ri.Status.SentinelEndpoints = nil
	}

	// Conditions.
	setCondition(ri, metav1.Condition{
		Type:               redisv1alpha1.ConditionTypeReady,
		Status:             boolToConditionStatus(ri.Status.Phase == redisv1alpha1.PhaseRunning),
		Reason:             ri.Status.Phase,
		Message:            fmt.Sprintf("Redis instance phase is %s", ri.Status.Phase),
		ObservedGeneration: ri.Generation,
	})
	setCondition(ri, metav1.Condition{
		Type:   redisv1alpha1.ConditionTypeReconciling,
		Status: boolToConditionStatus(ri.Status.Phase == redisv1alpha1.PhasePending),
		Reason: ri.Status.Phase,
		Message: fmt.Sprintf("Redis instance phase is %s", ri.Status.Phase),
		ObservedGeneration: ri.Generation,
	})
	setCondition(ri, metav1.Condition{
		Type:   redisv1alpha1.ConditionTypeDegraded,
		Status: boolToConditionStatus(ri.Status.Phase == redisv1alpha1.PhaseDegraded || ri.Status.Phase == redisv1alpha1.PhaseFailed),
		Reason: ri.Status.Phase,
		Message: fmt.Sprintf("Redis instance phase is %s", ri.Status.Phase),
		ObservedGeneration: ri.Generation,
	})

	// Emit events on phase transitions.
	if previousPhase != "" && previousPhase != ri.Status.Phase {
		r.emitPhaseEvent(ri, previousPhase, ri.Status.Phase)
	}

	return r.Status().Patch(ctx, ri, patch)
}

func computePhase(ready, desired int32) string {
	switch {
	case ready == 0:
		return redisv1alpha1.PhasePending
	case ready == desired:
		return redisv1alpha1.PhaseRunning
	default:
		return redisv1alpha1.PhaseDegraded
	}
}

func boolToConditionStatus(v bool) metav1.ConditionStatus {
	if v {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

// setCondition upserts a condition on the RedisInstance, preserving LastTransitionTime
// when the Status value has not changed.
func setCondition(ri *redisv1alpha1.RedisInstance, cond metav1.Condition) {
	now := metav1.Now()
	for i, existing := range ri.Status.Conditions {
		if existing.Type == cond.Type {
			if existing.Status == cond.Status {
				// Status unchanged — keep original transition time.
				cond.LastTransitionTime = existing.LastTransitionTime
			} else {
				cond.LastTransitionTime = now
			}
			ri.Status.Conditions[i] = cond
			return
		}
	}
	cond.LastTransitionTime = now
	ri.Status.Conditions = append(ri.Status.Conditions, cond)
}

// emitPhaseEvent emits a Kubernetes Event describing the phase transition.
func (r *RedisInstanceReconciler) emitPhaseEvent(ri *redisv1alpha1.RedisInstance, from, to string) {
	msg := fmt.Sprintf("Phase transitioned from %s to %s", from, to)
	switch to {
	case redisv1alpha1.PhaseRunning:
		r.Recorder.Event(ri, corev1.EventTypeNormal, "InstanceRunning", msg)
	case redisv1alpha1.PhaseDegraded:
		r.Recorder.Event(ri, corev1.EventTypeWarning, "InstanceDegraded", msg)
	case redisv1alpha1.PhaseFailed:
		r.Recorder.Event(ri, corev1.EventTypeWarning, "InstanceFailed", msg)
	default:
		r.Recorder.Event(ri, corev1.EventTypeNormal, "InstancePending", msg)
	}
}
