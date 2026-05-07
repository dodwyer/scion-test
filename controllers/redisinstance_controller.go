package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"k8s.io/client-go/tools/record"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

const (
	redisFinalizer          = "redis.example.io/cleanup"
	redisDataVolumeName     = "redis-data"
	redisConfigVolumeName   = "redis-config"
	redisTmpVolumeName      = "redis-tmp"
	redisAuthVolumeName     = "redis-auth"
	redisTLSVolumeName      = "redis-tls"
	redisConfigMountPath    = "/etc/redis"
	redisAuthMountPath      = "/etc/redis-auth"
	redisTLSMountPath       = "/etc/redis-tls"
	statefulSetRestartAnno  = "redis.example.io/restart-trigger"
	redisServerPort   int32 = 6379
	sentinelPort      int32 = 26379
)

// RedisInstanceReconciler reconciles a RedisInstance object.
type RedisInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &redisv1alpha1.RedisInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	previousPhase := instance.Status.Phase

	if instance.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(instance, redisFinalizer) {
			controllerutil.AddFinalizer(instance, redisFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		return r.reconcileDeletion(ctx, instance)
	}

	if phase, reason, err := r.validateReferencedSecrets(ctx, instance); err != nil {
		base := instance.DeepCopy()
		if statusErr := r.patchStatus(ctx, base, instance, phase, reason, previousPhase); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	skipMutation, err := r.shouldSkipMutation(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !skipMutation {
		if err := r.reconcileResources(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	scaleDownPending, err := r.scaleDownPending(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.refreshStatus(ctx, instance, previousPhase, scaleDownPending); err != nil {
		return ctrl.Result{}, err
	}

	if scaleDownPending {
		logger.Info("waiting for higher-ordinal pods to terminate", "name", instance.Name)
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *RedisInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1alpha1.RedisInstance{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}

func (r *RedisInstanceReconciler) reconcileResources(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	resources := []client.Object{
		r.desiredConfigMap(instance),
		r.desiredHeadlessService(instance),
		r.desiredRedisStatefulSet(instance),
	}
	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		resources = append(resources, r.desiredSentinelService(instance), r.desiredSentinelStatefulSet(instance))
	}

	for _, obj := range resources {
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
			if err := controllerutil.SetControllerReference(instance, obj, r.Scheme); err != nil {
				return err
			}
			return r.mutateDesiredObject(instance, obj)
		}); err != nil {
			return err
		}
	}

	if instance.Spec.Topology == redisv1alpha1.TopologyStandalone {
		if err := r.deleteIfExists(ctx, &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: sentinelStatefulSetName(instance), Namespace: instance.Namespace}}); err != nil {
			return err
		}
		if err := r.deleteIfExists(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: sentinelServiceName(instance), Namespace: instance.Namespace}}); err != nil {
			return err
		}
	}

	return nil
}

func (r *RedisInstanceReconciler) reconcileDeletion(ctx context.Context, instance *redisv1alpha1.RedisInstance) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(instance, redisFinalizer) {
		return ctrl.Result{}, nil
	}

	if err := r.teardownSentinel(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	done, err := r.teardownRedisPods(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !done {
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	done, err = r.teardownPVCs(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !done {
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	controllerutil.RemoveFinalizer(instance, redisFinalizer)
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RedisInstanceReconciler) teardownSentinel(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	if err := r.deleteIfExists(ctx, &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: sentinelStatefulSetName(instance), Namespace: instance.Namespace}}); err != nil {
		return err
	}
	if err := r.deleteIfExists(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: sentinelServiceName(instance), Namespace: instance.Namespace}}); err != nil {
		return err
	}

	sentinelPods := &corev1.PodList{}
	if err := r.List(ctx, sentinelPods, client.InNamespace(instance.Namespace), client.MatchingLabels(sentinelPodLabels(instance))); err != nil {
		return err
	}
	for _, pod := range sentinelPods.Items {
		if pod.DeletionTimestamp == nil {
			if err := r.Delete(ctx, &pod, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func (r *RedisInstanceReconciler) teardownRedisPods(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(instance.Namespace), client.MatchingLabels(redisPodLabels(instance))); err != nil {
		return false, err
	}
	if len(pods.Items) == 0 {
		return true, nil
	}

	sort.Slice(pods.Items, func(i, j int) bool {
		return podOrdinal(pods.Items[i].Name) > podOrdinal(pods.Items[j].Name)
	})

	for _, pod := range pods.Items {
		if pod.DeletionTimestamp == nil {
			if err := r.Delete(ctx, &pod, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
		}
	}

	return false, nil
}

func (r *RedisInstanceReconciler) teardownPVCs(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcs, client.InNamespace(instance.Namespace), client.MatchingLabels(commonLabels(instance))); err != nil {
		return false, err
	}
	if len(pvcs.Items) == 0 {
		return true, nil
	}

	sort.Slice(pvcs.Items, func(i, j int) bool {
		return podOrdinal(pvcs.Items[i].Name) > podOrdinal(pvcs.Items[j].Name)
	})

	for _, pvc := range pvcs.Items {
		if pvc.DeletionTimestamp == nil {
			if err := r.Delete(ctx, &pvc, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
		}
	}

	return false, nil
}

func (r *RedisInstanceReconciler) refreshStatus(ctx context.Context, instance *redisv1alpha1.RedisInstance, previousPhase string, scaleDownPending bool) error {
	base := instance.DeepCopy()
	phase := "Pending"
	reason := "WaitingForReadyReplicas"

	redisStatefulSet := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, redisStatefulSet); err != nil {
		return err
	}

	readyReplicas := redisStatefulSet.Status.ReadyReplicas
	masterEndpoint := redisMasterEndpoint(instance)
	sentinelEndpoints := []string{}
	sentinelReady := int32(0)

	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		sentinelEndpoints = sentinelHostnames(instance)
		sentinelStatefulSet := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: sentinelStatefulSetName(instance), Namespace: instance.Namespace}, sentinelStatefulSet); err != nil {
			return err
		}
		sentinelReady = sentinelStatefulSet.Status.ReadyReplicas
	}

	if readyReplicas == instance.Spec.Replicas && (!isSentinel(instance) || sentinelReady == instance.Spec.Replicas) && !scaleDownPending {
		phase = "Running"
		reason = "Ready"
	} else if readyReplicas > 0 || scaleDownPending {
		phase = "Degraded"
		reason = "ReplicasNotReady"
	}

	instance.Status.ReadyReplicas = readyReplicas
	instance.Status.MasterEndpoint = masterEndpoint
	instance.Status.SentinelEndpoints = sentinelEndpoints
	return r.patchStatus(ctx, base, instance, phase, reason, previousPhase)
}

func (r *RedisInstanceReconciler) patchStatus(ctx context.Context, base, instance *redisv1alpha1.RedisInstance, phase, reason, previousPhase string) error {
	instance.Status.Phase = phase
	instance.Status.ObservedGeneration = instance.Generation
	setStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus(phase == "Running"),
		Reason:             reason,
		ObservedGeneration: instance.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            fmt.Sprintf("Redis instance phase is %s", phase),
	})
	setStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Reconciling",
		Status:             conditionStatus(phase != "Running"),
		Reason:             reason,
		ObservedGeneration: instance.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            "Controller is reconciling the desired state",
	})
	setStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Degraded",
		Status:             conditionStatus(phase == "Degraded" || phase == "Failed"),
		Reason:             reason,
		ObservedGeneration: instance.Generation,
		LastTransitionTime: metav1.Now(),
		Message:            fmt.Sprintf("Redis instance phase is %s", phase),
	})

	if err := r.Status().Patch(ctx, instance, client.MergeFrom(base)); err != nil {
		return err
	}

	if previousPhase != phase {
		switch phase {
		case "Running":
			r.Recorder.Event(instance, corev1.EventTypeNormal, "InstanceRunning", "Redis instance is running")
		case "Degraded":
			r.Recorder.Event(instance, corev1.EventTypeWarning, "InstanceDegraded", "Redis instance is degraded")
		case "Failed":
			r.Recorder.Event(instance, corev1.EventTypeWarning, "InstanceFailed", "Redis instance reconciliation failed")
		default:
			r.Recorder.Event(instance, corev1.EventTypeNormal, "InstancePending", "Redis instance is pending")
		}
	}

	return nil
}

func (r *RedisInstanceReconciler) shouldSkipMutation(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	if instance.Status.ObservedGeneration != instance.Generation {
		return false, nil
	}

	checks := []func(context.Context, *redisv1alpha1.RedisInstance) (bool, error){
		r.configMapMatches,
		r.serviceMatches,
		r.redisStatefulSetMatches,
	}
	if isSentinel(instance) {
		checks = append(checks, r.sentinelServiceMatches, r.sentinelStatefulSetMatches)
	} else {
		checks = append(checks, r.sentinelResourcesAbsent)
	}

	for _, check := range checks {
		ok, err := check(ctx, instance)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	return true, nil
}

func (r *RedisInstanceReconciler) scaleDownPending(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(instance.Namespace), client.MatchingLabels(redisPodLabels(instance))); err != nil {
		return false, err
	}
	for _, pod := range pods.Items {
		if int32(podOrdinal(pod.Name)) >= instance.Spec.Replicas {
			return true, nil
		}
	}
	return false, nil
}

func (r *RedisInstanceReconciler) validateReferencedSecrets(ctx context.Context, instance *redisv1alpha1.RedisInstance) (string, string, error) {
	if instance.Spec.Auth == nil {
		return "", "", nil
	}
	if ref := instance.Spec.Auth.PasswordSecretRef; ref != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: instance.Namespace}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				return "Failed", "MissingPasswordSecret", nil
			}
			return "", "", err
		}
		if _, ok := secret.Data[ref.Key]; !ok {
			return "Failed", "MissingPasswordSecretKey", nil
		}
	}
	if tls := instance.Spec.Auth.TLS; tls != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: tls.SecretName, Namespace: instance.Namespace}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				return "Failed", "MissingTLSSecret", nil
			}
			return "", "", err
		}
		for _, key := range []string{tls.CertKey, tls.KeyKey, tls.CAKey} {
			if _, ok := secret.Data[key]; !ok {
				return "Failed", "MissingTLSSecretKey", nil
			}
		}
	}
	return "", "", nil
}

func (r *RedisInstanceReconciler) desiredConfigMap(instance *redisv1alpha1.RedisInstance) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName(instance),
			Namespace: instance.Namespace,
		},
	}
}

func (r *RedisInstanceReconciler) desiredHeadlessService(instance *redisv1alpha1.RedisInstance) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}
}

func (r *RedisInstanceReconciler) desiredRedisStatefulSet(instance *redisv1alpha1.RedisInstance) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}
}

func (r *RedisInstanceReconciler) desiredSentinelStatefulSet(instance *redisv1alpha1.RedisInstance) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sentinelStatefulSetName(instance),
			Namespace: instance.Namespace,
		},
	}
}

func (r *RedisInstanceReconciler) desiredSentinelService(instance *redisv1alpha1.RedisInstance) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sentinelServiceName(instance),
			Namespace: instance.Namespace,
		},
	}
}

func (r *RedisInstanceReconciler) mutateDesiredObject(instance *redisv1alpha1.RedisInstance, obj client.Object) error {
	switch typed := obj.(type) {
	case *corev1.ConfigMap:
		typed.Labels = commonLabels(instance)
		typed.Data = map[string]string{"redis.conf": renderRedisConfig(instance)}
	case *corev1.Service:
		typed.Labels = commonLabels(instance)
		if typed.Name == sentinelServiceName(instance) {
			typed.Spec.ClusterIP = corev1.ClusterIPNone
			typed.Spec.Selector = sentinelPodLabels(instance)
			typed.Spec.Ports = []corev1.ServicePort{{
				Name:       "sentinel",
				Port:       sentinelPort,
				TargetPort: intstr.FromInt32(sentinelPort),
			}}
		} else {
			typed.Spec.ClusterIP = corev1.ClusterIPNone
			typed.Spec.PublishNotReadyAddresses = true
			typed.Spec.Selector = redisPodLabels(instance)
			typed.Spec.Ports = []corev1.ServicePort{{
				Name:       "redis",
				Port:       redisServerPort,
				TargetPort: intstr.FromInt32(redisServerPort),
			}}
		}
	case *appsv1.StatefulSet:
		if typed.Name == sentinelStatefulSetName(instance) {
			typed.Labels = commonLabels(instance)
			typed.Spec = desiredSentinelStatefulSetSpec(instance)
		} else {
			typed.Labels = commonLabels(instance)
			typed.Spec = desiredRedisStatefulSetSpec(instance)
		}
	}
	return nil
}

func desiredRedisStatefulSetSpec(instance *redisv1alpha1.RedisInstance) appsv1.StatefulSetSpec {
	labels := redisPodLabels(instance)
	replicas := instance.Spec.Replicas

	annotations := map[string]string{
		statefulSetRestartAnno: restartTrigger(instance),
	}

	volumes := []corev1.Volume{
		{
			Name: redisConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName(instance)},
				},
			},
		},
		{
			Name:         redisTmpVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{Name: redisConfigVolumeName, MountPath: redisConfigMountPath, ReadOnly: true},
		{Name: redisDataVolumeName, MountPath: "/data"},
		{Name: redisTmpVolumeName, MountPath: "/tmp"},
	}

	command := []string{"/bin/sh", "-c", redisStartupScript(instance)}
	envVars := []corev1.EnvVar{}

	if ref := passwordSecretRef(instance); ref != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: ref,
			},
		})
	}

	if tls := tlsConfig(instance); tls != nil {
		volumes = append(volumes, corev1.Volume{
			Name: redisTLSVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: tls.SecretName},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: redisTLSVolumeName, MountPath: redisTLSMountPath, ReadOnly: true})
	}

	container := corev1.Container{
		Name:            "redis",
		Image:           redisImage(instance.Spec.RedisVersion),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         command,
		Env:             envVars,
		Ports: []corev1.ContainerPort{{
			Name:          "redis",
			ContainerPort: redisServerPort,
		}},
		Resources:    instance.Spec.Resources,
		VolumeMounts: volumeMounts,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(true),
			RunAsNonRoot:             ptr.To(true),
			RunAsUser:                ptr.To[int64](1001),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		},
	}

	return appsv1.StatefulSetSpec{
		ServiceName: instance.Name,
		Replicas:    &replicas,
		Selector:    &metav1.LabelSelector{MatchLabels: labels},
		UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		},
		PodManagementPolicy: appsv1.OrderedReadyPodManagement,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: ptr.To(true),
					RunAsUser:    ptr.To[int64](1001),
					RunAsGroup:   ptr.To[int64](1001),
					FSGroup:      ptr.To[int64](1001),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				Containers:                    []corev1.Container{container},
				Volumes:                       volumes,
				TerminationGracePeriodSeconds: ptr.To[int64](30),
			},
		},
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{persistentVolumeClaimTemplate(instance)},
	}
}

func desiredSentinelStatefulSetSpec(instance *redisv1alpha1.RedisInstance) appsv1.StatefulSetSpec {
	labels := sentinelPodLabels(instance)
	replicas := instance.Spec.Replicas
	command := []string{"/bin/sh", "-c", sentinelStartupScript(instance)}

	return appsv1.StatefulSetSpec{
		ServiceName: sentinelServiceName(instance),
		Replicas:    &replicas,
		Selector:    &metav1.LabelSelector{MatchLabels: labels},
		UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
				Annotations: map[string]string{
					statefulSetRestartAnno: restartTrigger(instance),
				},
			},
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: ptr.To(true),
					RunAsUser:    ptr.To[int64](1001),
					RunAsGroup:   ptr.To[int64](1001),
					FSGroup:      ptr.To[int64](1001),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				Containers: []corev1.Container{{
					Name:            "sentinel",
					Image:           redisImage(instance.Spec.RedisVersion),
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         command,
					Ports: []corev1.ContainerPort{{
						Name:          "sentinel",
						ContainerPort: sentinelPort,
					}},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						ReadOnlyRootFilesystem:   ptr.To(true),
						RunAsNonRoot:             ptr.To(true),
						RunAsUser:                ptr.To[int64](1001),
						Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
					},
					VolumeMounts: []corev1.VolumeMount{{Name: redisTmpVolumeName, MountPath: "/tmp"}},
				}},
				Volumes: []corev1.Volume{{
					Name:         redisTmpVolumeName,
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				}},
			},
		},
	}
}

func persistentVolumeClaimTemplate(instance *redisv1alpha1.RedisInstance) corev1.PersistentVolumeClaim {
	template := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        redisDataVolumeName,
			Labels:      commonLabels(instance),
			Annotations: maps.Clone(instance.Spec.Storage.Annotations),
		},
		Spec: *instance.Spec.Storage.Spec.DeepCopy(),
	}
	template.Labels = mergeStringMaps(template.Labels, instance.Spec.Storage.Labels)
	return template
}

func renderRedisConfig(instance *redisv1alpha1.RedisInstance) string {
	keys := make([]string, 0, len(instance.Spec.Config))
	for key := range instance.Spec.Config {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := []string{
		"bind 0.0.0.0",
		"protected-mode no",
		"dir /data",
	}
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s %s", key, instance.Spec.Config[key]))
	}
	return strings.Join(lines, "\n") + "\n"
}

func redisStartupScript(instance *redisv1alpha1.RedisInstance) string {
	args := []string{
		"cp /etc/redis/redis.conf /tmp/redis.conf",
		"ordinal=${HOSTNAME##*-}",
		fmt.Sprintf("master_host=%q", fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", instance.Name, instance.Name, instance.Namespace)),
	}

	if ref := passwordSecretRef(instance); ref != nil {
		args = append(args,
			"echo \"requirepass ${REDIS_PASSWORD}\" >> /tmp/redis.conf",
			"echo \"masterauth ${REDIS_PASSWORD}\" >> /tmp/redis.conf",
		)
	}

	if tls := tlsConfig(instance); tls != nil {
		args = append(args,
			"echo \"port 0\" >> /tmp/redis.conf",
			fmt.Sprintf("echo \"tls-port %d\" >> /tmp/redis.conf", redisServerPort),
			fmt.Sprintf("echo \"tls-cert-file %s/%s\" >> /tmp/redis.conf", redisTLSMountPath, tls.CertKey),
			fmt.Sprintf("echo \"tls-key-file %s/%s\" >> /tmp/redis.conf", redisTLSMountPath, tls.KeyKey),
			fmt.Sprintf("echo \"tls-ca-cert-file %s/%s\" >> /tmp/redis.conf", redisTLSMountPath, tls.CAKey),
		)
	}

	args = append(args,
		"if [ \"$ordinal\" != \"0\" ]; then",
		"  echo \"replicaof ${master_host} 6379\" >> /tmp/redis.conf",
		"fi",
		"exec redis-server /tmp/redis.conf",
	)

	return strings.Join(args, "\n")
}

func sentinelStartupScript(instance *redisv1alpha1.RedisInstance) string {
	return strings.Join([]string{
		fmt.Sprintf("cat <<'EOF' >/tmp/sentinel.conf\nport %d\nsentinel monitor mymaster %s 6379 2\nsentinel down-after-milliseconds mymaster 5000\nsentinel failover-timeout mymaster 10000\nsentinel parallel-syncs mymaster 1\nEOF", sentinelPort, redisMasterEndpoint(instance)),
		"exec redis-server /tmp/sentinel.conf --sentinel",
	}, "\n")
}

func restartTrigger(instance *redisv1alpha1.RedisInstance) string {
	hash := sha256.Sum256([]byte(instance.Spec.RedisVersion + "\n" + renderRedisConfig(instance)))
	return hex.EncodeToString(hash[:8])
}

func redisImage(version string) string {
	if strings.Contains(version, "/") || strings.Contains(version, "@sha256:") || strings.Contains(version, ":") {
		return version
	}
	return "redis:" + version
}

func configMapName(instance *redisv1alpha1.RedisInstance) string {
	return instance.Name + "-config"
}

func sentinelStatefulSetName(instance *redisv1alpha1.RedisInstance) string {
	return instance.Name + "-sentinel"
}

func sentinelServiceName(instance *redisv1alpha1.RedisInstance) string {
	return instance.Name + "-sentinel"
}

func commonLabels(instance *redisv1alpha1.RedisInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "redis-operator",
		"app.kubernetes.io/component":  "redis",
		"app.kubernetes.io/managed-by": "redis-operator",
		"redis.example.io/instance":    instance.Name,
	}
}

func redisPodLabels(instance *redisv1alpha1.RedisInstance) map[string]string {
	labels := mergeStringMaps(commonLabels(instance), map[string]string{
		"app.kubernetes.io/instance": instance.Name,
		"redis.example.io/role":      "data",
	})
	return labels
}

func sentinelPodLabels(instance *redisv1alpha1.RedisInstance) map[string]string {
	return mergeStringMaps(commonLabels(instance), map[string]string{
		"app.kubernetes.io/instance": instance.Name + "-sentinel",
		"redis.example.io/role":      "sentinel",
	})
}

func mergeStringMaps(left, right map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		out[key] = value
	}
	return out
}

func redisMasterEndpoint(instance *redisv1alpha1.RedisInstance) string {
	return fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", instance.Name, instance.Name, instance.Namespace)
}

func sentinelHostnames(instance *redisv1alpha1.RedisInstance) []string {
	out := make([]string, 0, instance.Spec.Replicas)
	for i := int32(0); i < instance.Spec.Replicas; i++ {
		out = append(out, fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", sentinelStatefulSetName(instance), i, sentinelServiceName(instance), instance.Namespace))
	}
	return out
}

func conditionStatus(ok bool) metav1.ConditionStatus {
	if ok {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func setStatusCondition(conditions *[]metav1.Condition, condition metav1.Condition) {
	existing := *conditions
	for i := range existing {
		if existing[i].Type == condition.Type {
			if existing[i].Status == condition.Status && existing[i].Reason == condition.Reason && existing[i].Message == condition.Message {
				condition.LastTransitionTime = existing[i].LastTransitionTime
			}
			existing[i] = condition
			*conditions = existing
			return
		}
	}
	*conditions = append(existing, condition)
}

func podOrdinal(name string) int {
	lastDash := strings.LastIndex(name, "-")
	if lastDash == -1 || lastDash == len(name)-1 {
		return -1
	}
	value, err := strconv.Atoi(name[lastDash+1:])
	if err != nil {
		return -1
	}
	return value
}

func passwordSecretRef(instance *redisv1alpha1.RedisInstance) *corev1.SecretKeySelector {
	if instance.Spec.Auth == nil {
		return nil
	}
	return instance.Spec.Auth.PasswordSecretRef
}

func tlsConfig(instance *redisv1alpha1.RedisInstance) *redisv1alpha1.TLSConfig {
	if instance.Spec.Auth == nil {
		return nil
	}
	return instance.Spec.Auth.TLS
}

func isSentinel(instance *redisv1alpha1.RedisInstance) bool {
	return instance.Spec.Topology == redisv1alpha1.TopologySentinel
}

func (r *RedisInstanceReconciler) configMapMatches(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	current := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: configMapName(instance), Namespace: instance.Namespace}, current); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return current.Data["redis.conf"] == renderRedisConfig(instance), nil
}

func (r *RedisInstanceReconciler) serviceMatches(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	current := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, current); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return current.Spec.ClusterIP == corev1.ClusterIPNone && maps.Equal(current.Spec.Selector, redisPodLabels(instance)), nil
}

func (r *RedisInstanceReconciler) redisStatefulSetMatches(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	current := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, current); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	desired := desiredRedisStatefulSetSpec(instance)
	return *current.Spec.Replicas == *desired.Replicas &&
		current.Spec.Template.Spec.Containers[0].Image == desired.Template.Spec.Containers[0].Image &&
		current.Spec.Template.Annotations[statefulSetRestartAnno] == desired.Template.Annotations[statefulSetRestartAnno], nil
}

func (r *RedisInstanceReconciler) sentinelStatefulSetMatches(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	current := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: sentinelStatefulSetName(instance), Namespace: instance.Namespace}, current); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return current.Spec.Replicas != nil && *current.Spec.Replicas == instance.Spec.Replicas, nil
}

func (r *RedisInstanceReconciler) sentinelServiceMatches(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	current := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: sentinelServiceName(instance), Namespace: instance.Namespace}, current); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return current.Spec.ClusterIP == corev1.ClusterIPNone && maps.Equal(current.Spec.Selector, sentinelPodLabels(instance)), nil
}

func (r *RedisInstanceReconciler) sentinelResourcesAbsent(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	for _, obj := range []client.Object{
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: sentinelStatefulSetName(instance), Namespace: instance.Namespace}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: sentinelServiceName(instance), Namespace: instance.Namespace}},
	} {
		err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err == nil {
			return false, nil
		}
		if !apierrors.IsNotFound(err) {
			return false, err
		}
	}
	return true, nil
}

func (r *RedisInstanceReconciler) deleteIfExists(ctx context.Context, obj client.Object) error {
	if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
