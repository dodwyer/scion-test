package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

const (
	sentinelPort = int32(26379)
)

// reconcileSentinel creates or updates the Sentinel StatefulSet and Service.
func (r *RedisInstanceReconciler) reconcileSentinel(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	if err := r.reconcileSentinelService(ctx, ri); err != nil {
		return fmt.Errorf("sentinel service: %w", err)
	}
	return r.reconcileSentinelStatefulSet(ctx, ri)
}

func (r *RedisInstanceReconciler) reconcileSentinelService(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ri.Name + "-sentinel",
			Namespace: ri.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = resourceLabels(ri.Name, componentSentinel)
		if svc.Spec.ClusterIP == "" {
			svc.Spec.ClusterIP = "None"
		}
		svc.Spec.Selector = podLabels(ri.Name, componentSentinel)
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "sentinel",
				Port:       26379,
				TargetPort: intstr.FromInt32(26379),
				Protocol:   corev1.ProtocolTCP,
			},
		}
		return controllerutil.SetControllerReference(ri, svc, r.Scheme)
	})
	return err
}

func (r *RedisInstanceReconciler) reconcileSentinelStatefulSet(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ri.Name + "-sentinel",
			Namespace: ri.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		sts.Labels = resourceLabels(ri.Name, componentSentinel)
		replicas := sentinelReplicas
		sts.Spec.Replicas = &replicas
		sts.Spec.ServiceName = ri.Name + "-sentinel"
		sts.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: podLabels(ri.Name, componentSentinel),
		}
		sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		}
		sts.Spec.Template = buildSentinelPodTemplate(ri)
		return controllerutil.SetControllerReference(ri, sts, r.Scheme)
	})
	return err
}

// buildSentinelPodTemplate builds the PodTemplateSpec for Sentinel pods.
func buildSentinelPodTemplate(ri *redisv1alpha1.RedisInstance) corev1.PodTemplateSpec {
	labels := podLabels(ri.Name, componentSentinel)
	initEnv := buildSentinelInitEnv(ri)

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
			Annotations: map[string]string{
				configHashAnnotation: computeConfigHash(ri),
			},
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr(true),
				RunAsUser:    ptr(redisUser),
				RunAsGroup:   ptr(redisUser),
				FSGroup:      ptr(redisUser),
			},
			InitContainers: []corev1.Container{
				{
					Name:  "init-sentinel",
					Image: ri.Spec.RedisVersion,
					Command: []string{"/bin/sh", "-c", `
cat > /data/sentinel.conf << EOFCONF
sentinel monitor mymaster ${REDIS_PRIMARY_HOST} 6379 2
sentinel down-after-milliseconds mymaster 5000
sentinel failover-timeout mymaster 10000
sentinel parallel-syncs mymaster 1
EOFCONF
if [ -n "${REDIS_PASSWORD}" ]; then
    echo "sentinel auth-pass mymaster ${REDIS_PASSWORD}" >> /data/sentinel.conf
fi
`},
					Env: initEnv,
					VolumeMounts: []corev1.VolumeMount{
						{Name: dataVolumeName, MountPath: "/data"},
					},
					SecurityContext: containerSecurityContext(),
				},
			},
			Containers: []corev1.Container{
				{
					Name:    componentSentinel,
					Image:   ri.Spec.RedisVersion,
					Command: []string{"redis-sentinel", "/data/sentinel.conf"},
					Ports: []corev1.ContainerPort{
						{Name: "sentinel", ContainerPort: sentinelPort, Protocol: corev1.ProtocolTCP},
					},
					Resources: ri.Spec.Resources,
					VolumeMounts: []corev1.VolumeMount{
						{Name: dataVolumeName, MountPath: "/data"},
						{Name: tmpVolumeName, MountPath: "/tmp"},
					},
					SecurityContext: containerSecurityContext(),
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"redis-cli", "-p", "26379", "ping"},
							},
						},
						InitialDelaySeconds: 30,
						PeriodSeconds:       10,
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"redis-cli", "-p", "26379", "ping"},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       5,
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					// Sentinel rewrites its own config file; use emptyDir so it can write.
					Name:         dataVolumeName,
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
				{
					Name:         tmpVolumeName,
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
			},
		},
	}
}

// buildSentinelInitEnv builds environment variables for the Sentinel init container.
func buildSentinelInitEnv(ri *redisv1alpha1.RedisInstance) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "REDIS_PRIMARY_HOST", Value: primaryDNS(ri.Name, ri.Namespace)},
	}
	if ri.Spec.Auth != nil && ri.Spec.Auth.PasswordSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: ri.Spec.Auth.PasswordSecretRef,
			},
		})
	}
	return env
}

// deleteSentinelResources removes the Sentinel StatefulSet and Service during teardown.
func (r *RedisInstanceReconciler) deleteSentinelResources(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, namespacedName(ri.Namespace, ri.Name+"-sentinel"), sts); err == nil {
		if err := r.Delete(ctx, sts); err != nil {
			return fmt.Errorf("delete sentinel statefulset: %w", err)
		}
	}
	svc := &corev1.Service{}
	if err := r.Get(ctx, namespacedName(ri.Namespace, ri.Name+"-sentinel"), svc); err == nil {
		if err := r.Delete(ctx, svc); err != nil {
			return fmt.Errorf("delete sentinel service: %w", err)
		}
	}
	return nil
}
