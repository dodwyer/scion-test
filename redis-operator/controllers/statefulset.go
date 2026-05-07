package controllers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

func labelsForInstance(instance *redisv1alpha1.RedisInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "redis",
		"app.kubernetes.io/instance":   instance.Name,
		"app.kubernetes.io/managed-by": "redis-operator",
	}
}

func configHashAnnotation(config map[string]string) string {
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%s\n", k, config[k])
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func (r *RedisInstanceReconciler) buildStatefulSet(instance *redisv1alpha1.RedisInstance) *appsv1.StatefulSet {
	labels := labelsForInstance(instance)
	replicas := instance.Spec.Replicas

	// Security context
	nonRoot := true
	readOnlyFS := true
	runAsUser := int64(999)
	allowPrivEsc := false

	podSecCtx := &corev1.PodSecurityContext{
		RunAsNonRoot: &nonRoot,
		RunAsUser:    &runAsUser,
	}

	containerSecCtx := &corev1.SecurityContext{
		AllowPrivilegeEscalation: &allowPrivEsc,
		ReadOnlyRootFilesystem:   &readOnlyFS,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}

	// Command: detect pod ordinal and configure replication
	primaryHost := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local",
		instance.Name, instance.Name, instance.Namespace)

	command := fmt.Sprintf(`
set -e
cp /etc/redis/redis.conf /tmp/redis-running.conf
ORDINAL="${HOSTNAME##*-}"
if [ "$ORDINAL" != "0" ]; then
  echo "replicaof %s 6379" >> /tmp/redis-running.conf
fi
if [ -n "$REDIS_PASSWORD" ]; then
  echo "requirepass $REDIS_PASSWORD" >> /tmp/redis-running.conf
  if [ "$ORDINAL" != "0" ]; then
    echo "masterauth $REDIS_PASSWORD" >> /tmp/redis-running.conf
  fi
fi
exec redis-server /tmp/redis-running.conf
`, primaryHost)

	env := []corev1.EnvVar{}
	volumes := []corev1.Volume{
		{
			Name: "redis-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName(instance),
					},
				},
			},
		},
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{Name: "redis-config", MountPath: "/etc/redis"},
		{Name: "data", MountPath: "/data"},
		{Name: "tmp", MountPath: "/tmp"},
	}

	// Auth via secret
	if instance.Spec.Auth != nil && instance.Spec.Auth.PasswordSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: instance.Spec.Auth.PasswordSecretRef,
			},
		})
	}

	// PVC template
	storageClassName := instance.Spec.Storage.StorageClassName
	pvcTemplate := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "data",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      instance.Spec.Storage.AccessModes,
			StorageClassName: storageClassName,
			Resources:        instance.Spec.Storage.Resources,
		},
	}

	updateStrategy := appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			ServiceName:         instance.Name,
			UpdateStrategy:      updateStrategy,
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"redis.example.io/config-hash":   configHashAnnotation(instance.Spec.Config),
						"redis.example.io/redis-version": instance.Spec.RedisVersion,
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: podSecCtx,
					Containers: []corev1.Container{
						{
							Name:            "redis",
							Image:           instance.Spec.RedisVersion,
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{command},
							Env:             env,
							VolumeMounts:    volumeMounts,
							Resources:       instance.Spec.Resources,
							SecurityContext: containerSecCtx,
						},
					},
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{pvcTemplate},
		},
	}

	return sts
}

func (r *RedisInstanceReconciler) reconcileStatefulSet(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	desired := r.buildStatefulSet(instance)
	if err := ctrl.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update mutable fields
	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
	existing.Spec.Template.Annotations = desired.Spec.Template.Annotations
	return r.Update(ctx, existing)
}
