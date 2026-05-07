package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

func (r *RedisInstanceReconciler) reconcileService(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	labels := labelsForInstance(instance)
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Name: "redis",
					Port: 6379,
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update selector to match current labels
	existing.Spec.Selector = labels
	return r.Update(ctx, existing)
}
