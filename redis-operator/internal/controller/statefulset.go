package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

const (
	configHashAnnotation = "redis.example.io/config-hash"
	dataVolumeName       = "data"
	configVolumeName     = "config"
	tmpVolumeName        = "tmp"
	redisPort            = int32(6379)
	redisUser            = int64(999)
)

// reconcileStatefulSet creates or updates the Redis StatefulSet.
func (r *RedisInstanceReconciler) reconcileStatefulSet(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ri.Name,
			Namespace: ri.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		sts.Labels = resourceLabels(ri.Name, componentRedis)
		sts.Spec.Replicas = ptr(ri.Spec.Replicas)
		sts.Spec.ServiceName = ri.Name
		sts.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: podLabels(ri.Name, componentRedis),
		}
		sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		}
		sts.Spec.Template = buildRedisPodTemplate(ri)
		// VolumeClaimTemplates are immutable after creation; only set on creation.
		if len(sts.Spec.VolumeClaimTemplates) == 0 {
			sts.Spec.VolumeClaimTemplates = redisPVCTemplates(ri)
		}
		return controllerutil.SetControllerReference(ri, sts, r.Scheme)
	})
	return err
}

// buildRedisPodTemplate builds the PodTemplateSpec for Redis data pods.
func buildRedisPodTemplate(ri *redisv1alpha1.RedisInstance) corev1.PodTemplateSpec {
	labels := podLabels(ri.Name, componentRedis)
	initEnv := buildInitEnv(ri)

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
			Annotations: map[string]string{
				// Changes when spec.config or spec.redisVersion changes → triggers rolling restart.
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
					Name:  "init-redis",
					Image: ri.Spec.RedisVersion,
					Command: []string{"/bin/sh", "-c", `
ORDINAL=$(echo "${HOSTNAME}" | awk -F- '{print $NF}')
cp /base-config/redis.conf /data/redis.conf
if [ "${TOPOLOGY}" = "standalone" ] && [ "${ORDINAL}" != "0" ]; then
    printf "\nreplicaof %s 6379\n" "${REDIS_PRIMARY_HOST}" >> /data/redis.conf
fi
if [ -n "${REDIS_PASSWORD}" ]; then
    printf "\nrequirepass %s\nmasterauth %s\n" "${REDIS_PASSWORD}" "${REDIS_PASSWORD}" >> /data/redis.conf
fi
`},
					Env: initEnv,
					VolumeMounts: []corev1.VolumeMount{
						{Name: dataVolumeName, MountPath: "/data"},
						{Name: configVolumeName, MountPath: "/base-config"},
					},
					SecurityContext: containerSecurityContext(),
				},
			},
			Containers: []corev1.Container{
				{
					Name:    componentRedis,
					Image:   ri.Spec.RedisVersion,
					Command: []string{"redis-server", "/data/redis.conf"},
					Ports: []corev1.ContainerPort{
						{Name: "redis", ContainerPort: redisPort, Protocol: corev1.ProtocolTCP},
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
								Command: []string{"redis-cli", "ping"},
							},
						},
						InitialDelaySeconds: 30,
						PeriodSeconds:       10,
						TimeoutSeconds:      5,
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"redis-cli", "ping"},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       5,
						TimeoutSeconds:      3,
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: configVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: ri.Name + "-config"},
						},
					},
				},
				{
					Name:         tmpVolumeName,
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
			},
		},
	}
}

// buildInitEnv builds environment variables for the Redis init container.
func buildInitEnv(ri *redisv1alpha1.RedisInstance) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "TOPOLOGY", Value: string(ri.Spec.Topology)},
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

// containerSecurityContext returns the least-privilege security context for all containers.
func containerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		ReadOnlyRootFilesystem:   ptr(true),
		AllowPrivilegeEscalation: ptr(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

// redisPVCTemplates builds the volumeClaimTemplates from the CR storage spec.
func redisPVCTemplates(ri *redisv1alpha1.RedisInstance) []corev1.PersistentVolumeClaim {
	pvcMeta := ri.Spec.Storage.ObjectMeta.DeepCopy()
	if pvcMeta == nil {
		pvcMeta = &metav1.ObjectMeta{}
	}
	pvcMeta.Name = dataVolumeName
	return []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: *pvcMeta,
			Spec:       ri.Spec.Storage.Spec,
		},
	}
}

// computeConfigHash returns a short SHA256 hash of spec.config + spec.redisVersion.
// Used as a pod template annotation to trigger rolling restarts on config/version changes.
func computeConfigHash(ri *redisv1alpha1.RedisInstance) string {
	h := sha256.New()
	keys := make([]string, 0, len(ri.Spec.Config))
	for k := range ri.Spec.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%s\n", k, ri.Spec.Config[k])
	}
	fmt.Fprintf(h, "version=%s\n", ri.Spec.RedisVersion)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
