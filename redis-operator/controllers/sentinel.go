package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

func sentinelStatefulSetName(instance *redisv1alpha1.RedisInstance) string {
	return fmt.Sprintf("%s-sentinel", instance.Name)
}

func labelsForSentinel(instance *redisv1alpha1.RedisInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "redis-sentinel",
		"app.kubernetes.io/instance":   instance.Name,
		"app.kubernetes.io/managed-by": "redis-operator",
	}
}

func (r *RedisInstanceReconciler) reconcileSentinel(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	if err := r.reconcileSentinelStatefulSet(ctx, instance); err != nil {
		return err
	}
	return r.reconcileSentinelService(ctx, instance)
}

func (r *RedisInstanceReconciler) reconcileSentinelStatefulSet(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	labels := labelsForSentinel(instance)
	sentinelReplicas := int32(3)
	primaryHost := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local",
		instance.Name, instance.Name, instance.Namespace)

	sentinelConf := fmt.Sprintf(`sentinel monitor mymaster %s 6379 2
sentinel down-after-milliseconds mymaster 5000
sentinel failover-timeout mymaster 60000
sentinel parallel-syncs mymaster 1
`, primaryHost)

	command := fmt.Sprintf(`set -e
mkdir -p /tmp/sentinel
cat > /tmp/sentinel/sentinel.conf << 'SENTINELEOF'
%s
SENTINELEOF
exec redis-sentinel /tmp/sentinel/sentinel.conf
`, sentinelConf)

	nonRoot := true
	runAsUser := int64(999)
	allowPrivEsc := false
	readOnlyFS := true

	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sentinelStatefulSetName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &sentinelReplicas,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			ServiceName: sentinelStatefulSetName(instance),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &nonRoot,
						RunAsUser:    &runAsUser,
					},
					Containers: []corev1.Container{
						{
							Name:    "sentinel",
							Image:   instance.Spec.RedisVersion,
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{command},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivEsc,
								ReadOnlyRootFilesystem:   &readOnlyFS,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "tmp", MountPath: "/tmp"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: sentinelStatefulSetName(instance)}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
	return r.Update(ctx, existing)
}

func (r *RedisInstanceReconciler) reconcileSentinelService(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	labels := labelsForSentinel(instance)
	svcName := sentinelStatefulSetName(instance)
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{Name: "sentinel", Port: 26379},
			},
		},
	}
	if err := ctrl.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: svcName}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec.Selector = labels
	return r.Update(ctx, existing)
}
