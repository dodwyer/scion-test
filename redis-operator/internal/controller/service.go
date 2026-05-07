package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

// reconcileService creates or updates the headless Service for Redis StatefulSet DNS.
func (r *RedisInstanceReconciler) reconcileService(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ri.Name,
			Namespace: ri.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = resourceLabels(ri.Name, componentRedis)
		// Headless service: clusterIP must be None.
		// Preserve existing ClusterIP on update (k8s rejects changing it).
		if svc.Spec.ClusterIP == "" {
			svc.Spec.ClusterIP = "None"
		}
		svc.Spec.Selector = podLabels(ri.Name, componentRedis)
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "redis",
				Port:       6379,
				TargetPort: intstr.FromInt32(6379),
				Protocol:   corev1.ProtocolTCP,
			},
		}
		return controllerutil.SetControllerReference(ri, svc, r.Scheme)
	})
	return err
}
