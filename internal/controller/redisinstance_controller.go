package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"path"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

const (
	redisFinalizer                  = "redis.example.io/cleanup"
	readyConditionType              = "Ready"
	reconcilingConditionType        = "Reconciling"
	degradedConditionType           = "Degraded"
	configChecksumAnnotation        = "redis.example.io/config-checksum"
	versionChecksumAnnotation       = "redis.example.io/version-checksum"
	authChecksumAnnotation          = "redis.example.io/auth-checksum"
	requeueShort                    = 5 * time.Second
	redisPort                 int32 = 6379
	sentinelPort              int32 = 26379
)

// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=get;create;update;patch;delete

type RedisInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var instance redisv1alpha1.RedisInstance
	if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &instance)
	}

	if !controllerutil.ContainsFinalizer(&instance, redisFinalizer) {
		original := instance.DeepCopy()
		controllerutil.AddFinalizer(&instance, redisFinalizer)
		if err := r.Patch(ctx, &instance, client.MergeFrom(original)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.resolveSecretReferences(ctx, &instance); err != nil {
		return r.failInstance(ctx, &instance, "AuthSecretMissing", err.Error())
	}

	needsMutation, err := r.requiresMutation(ctx, &instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	if needsMutation {
		if err := r.reconcileConfigMap(ctx, &instance); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcileHeadlessService(ctx, &instance); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcileRedisStatefulSet(ctx, &instance); err != nil {
			return ctrl.Result{}, err
		}
		if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
			if err := r.reconcileSentinelService(ctx, &instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.reconcileSentinelStatefulSet(ctx, &instance); err != nil {
				return ctrl.Result{}, err
			}
		} else if _, err := r.deleteSentinelResources(ctx, &instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	if scalingDown, err := r.isScaleDownInProgress(ctx, &instance); err != nil {
		return ctrl.Result{}, err
	} else if scalingDown {
		if err := r.updateStatus(ctx, &instance, "Pending"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueShort}, nil
	}

	if err := r.updateDynamicStatus(ctx, &instance); err != nil {
		logger.Error(err, "updating status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RedisInstanceReconciler) reconcileDelete(ctx context.Context, instance *redisv1alpha1.RedisInstance) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(instance, redisFinalizer) {
		return ctrl.Result{}, nil
	}

	if pending, err := r.deleteSentinelResources(ctx, instance); err != nil {
		return ctrl.Result{}, err
	} else if pending {
		return ctrl.Result{RequeueAfter: requeueShort}, nil
	}
	if pending, err := r.deleteRedisResources(ctx, instance); err != nil {
		return ctrl.Result{}, err
	} else if pending {
		return ctrl.Result{RequeueAfter: requeueShort}, nil
	}
	if err := r.deleteOwnedPVCs(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	original := instance.DeepCopy()
	controllerutil.RemoveFinalizer(instance, redisFinalizer)
	if err := r.Patch(ctx, instance, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RedisInstanceReconciler) requiresMutation(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	if instance.Status.ObservedGeneration != instance.Generation {
		return true, nil
	}

	desiredConfigMap := r.desiredConfigMap(instance)
	var actualConfigMap corev1.ConfigMap
	if err := r.Get(ctx, client.ObjectKeyFromObject(desiredConfigMap), &actualConfigMap); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	if !maps.Equal(actualConfigMap.Data, desiredConfigMap.Data) {
		return true, nil
	}

	desiredService := r.desiredHeadlessService(instance)
	var actualService corev1.Service
	if err := r.Get(ctx, client.ObjectKeyFromObject(desiredService), &actualService); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	if !maps.Equal(actualService.Spec.Selector, desiredService.Spec.Selector) || actualService.Spec.ClusterIP != corev1.ClusterIPNone {
		return true, nil
	}
	if !reflect.DeepEqual(actualService.Spec.Ports, desiredService.Spec.Ports) {
		return true, nil
	}

	desiredStatefulSet := r.desiredRedisStatefulSet(instance)
	var actualStatefulSet appsv1.StatefulSet
	if err := r.Get(ctx, client.ObjectKeyFromObject(desiredStatefulSet), &actualStatefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	if !redisStatefulSetMatches(&actualStatefulSet, desiredStatefulSet) {
		return true, nil
	}

	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		desiredSentinelService := r.desiredSentinelService(instance)
		var actualSentinelService corev1.Service
		if err := r.Get(ctx, client.ObjectKeyFromObject(desiredSentinelService), &actualSentinelService); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		if !sentinelServiceMatches(&actualSentinelService, desiredSentinelService) {
			return true, nil
		}

		desiredSentinelStatefulSet := r.desiredSentinelStatefulSet(instance)
		var actualSentinelStatefulSet appsv1.StatefulSet
		if err := r.Get(ctx, client.ObjectKeyFromObject(desiredSentinelStatefulSet), &actualSentinelStatefulSet); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		if !sentinelStatefulSetMatches(&actualSentinelStatefulSet, desiredSentinelStatefulSet) {
			return true, nil
		}
	}

	return false, nil
}

func (r *RedisInstanceReconciler) reconcileConfigMap(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	desired := r.desiredConfigMap(instance)
	var existing corev1.ConfigMap
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	original := existing.DeepCopy()
	existing.Labels = desired.Labels
	existing.Data = desired.Data
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

func (r *RedisInstanceReconciler) reconcileHeadlessService(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	desired := r.desiredHeadlessService(instance)
	var existing corev1.Service
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	original := existing.DeepCopy()
	existing.Labels = desired.Labels
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.ClusterIP = corev1.ClusterIPNone
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

func (r *RedisInstanceReconciler) reconcileRedisStatefulSet(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	desired := r.desiredRedisStatefulSet(instance)
	var existing appsv1.StatefulSet
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	original := existing.DeepCopy()
	existing.Labels = desired.Labels
	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.ServiceName = desired.Spec.ServiceName
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.UpdateStrategy = desired.Spec.UpdateStrategy
	existing.Spec.Template = desired.Spec.Template
	existing.Spec.VolumeClaimTemplates = desired.Spec.VolumeClaimTemplates
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

func (r *RedisInstanceReconciler) reconcileSentinelService(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	desired := r.desiredSentinelService(instance)
	var existing corev1.Service
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	original := existing.DeepCopy()
	existing.Labels = desired.Labels
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

func (r *RedisInstanceReconciler) reconcileSentinelStatefulSet(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	desired := r.desiredSentinelStatefulSet(instance)
	var existing appsv1.StatefulSet
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	original := existing.DeepCopy()
	existing.Labels = desired.Labels
	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.ServiceName = desired.Spec.ServiceName
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Template = desired.Spec.Template
	return r.Patch(ctx, &existing, client.MergeFrom(original))
}

func (r *RedisInstanceReconciler) deleteSentinelResources(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	pending, err := r.deletePodsReverseOrdinal(ctx, instance.Namespace, r.sentinelLabels(instance), instance.Name+"-sentinel")
	if err != nil {
		return false, err
	}
	if pending {
		return true, nil
	}

	service := r.desiredSentinelService(instance)
	statefulSet := r.desiredSentinelStatefulSet(instance)
	if err := client.IgnoreNotFound(r.Delete(ctx, service)); err != nil {
		return false, err
	}
	if err := client.IgnoreNotFound(r.Delete(ctx, statefulSet)); err != nil {
		return false, err
	}
	return false, nil
}

func (r *RedisInstanceReconciler) deleteRedisResources(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	pending, err := r.deletePodsReverseOrdinal(ctx, instance.Namespace, r.redisPodLabels(instance), instance.Name)
	if err != nil {
		return false, err
	}
	if pending {
		return true, nil
	}

	service := r.desiredHeadlessService(instance)
	configMap := r.desiredConfigMap(instance)
	statefulSet := r.desiredRedisStatefulSet(instance)
	if err := client.IgnoreNotFound(r.Delete(ctx, statefulSet)); err != nil {
		return false, err
	}
	if err := client.IgnoreNotFound(r.Delete(ctx, service)); err != nil {
		return false, err
	}
	if err := client.IgnoreNotFound(r.Delete(ctx, configMap)); err != nil {
		return false, err
	}
	return false, nil
}

func (r *RedisInstanceReconciler) deleteOwnedPVCs(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	var pvcList corev1.PersistentVolumeClaimList
	if err := r.List(ctx, &pvcList, client.InNamespace(instance.Namespace)); err != nil {
		return err
	}
	deletePolicy := metav1.DeletePropagationForeground
	for i := range pvcList.Items {
		pvc := pvcList.Items[i]
		if pvc.Labels["app.kubernetes.io/instance"] != instance.Name {
			continue
		}
		if pvc.Name != fmt.Sprintf("data-%s-0", instance.Name) && !strings.HasPrefix(pvc.Name, "data-"+instance.Name+"-") {
			continue
		}
		if err := client.IgnoreNotFound(r.Delete(ctx, &pvc, &client.DeleteOptions{PropagationPolicy: &deletePolicy})); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisInstanceReconciler) resolveSecretReferences(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	if instance.Spec.Auth == nil {
		return nil
	}

	if err := r.resolveSecretKeyRef(ctx, instance.Namespace, instance.Spec.Auth.PasswordSecretRef, "password"); err != nil {
		return err
	}
	if instance.Spec.Auth.TLS == nil {
		return nil
	}
	if err := r.resolveSecretKeyRef(ctx, instance.Namespace, instance.Spec.Auth.TLS.CertSecretRef, "tls cert"); err != nil {
		return err
	}
	if err := r.resolveSecretKeyRef(ctx, instance.Namespace, instance.Spec.Auth.TLS.KeySecretRef, "tls key"); err != nil {
		return err
	}
	if err := r.resolveSecretKeyRef(ctx, instance.Namespace, instance.Spec.Auth.TLS.CASecretRef, "tls ca"); err != nil {
		return err
	}
	return nil
}

func (r *RedisInstanceReconciler) resolveSecretKeyRef(ctx context.Context, namespace string, ref *corev1.SecretKeySelector, description string) error {
	if ref == nil {
		return nil
	}

	var secret corev1.Secret
	key := types.NamespacedName{Name: ref.Name, Namespace: namespace}
	if err := r.Get(ctx, key, &secret); err != nil {
		return fmt.Errorf("%s secret %s/%s: %w", description, namespace, ref.Name, err)
	}
	if _, ok := secret.Data[ref.Key]; !ok {
		return fmt.Errorf("%s secret %s/%s missing key %q", description, namespace, ref.Name, ref.Key)
	}
	return nil
}

func (r *RedisInstanceReconciler) failInstance(ctx context.Context, instance *redisv1alpha1.RedisInstance, reason, message string) (ctrl.Result, error) {
	r.Recorder.Event(instance, corev1.EventTypeWarning, reason, message)
	if err := r.updateStatusWithConditions(ctx, instance, "Failed", 0, nil); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RedisInstanceReconciler) updateDynamicStatus(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	var sts appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, &sts); err != nil {
		return err
	}

	ready := sts.Status.ReadyReplicas
	phase := "Pending"
	if ready == instance.Spec.Replicas {
		phase = "Running"
	} else if ready > 0 {
		phase = "Degraded"
	}

	var sentinelEndpoints []string
	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		for i := int32(0); i < instance.Spec.Replicas; i++ {
			sentinelEndpoints = append(sentinelEndpoints, fmt.Sprintf("%s-sentinel-%d.%s-sentinel.%s.svc.cluster.local:%d", instance.Name, i, instance.Name, instance.Namespace, sentinelPort))
		}
	}

	return r.updateStatusWithConditions(ctx, instance, phase, ready, sentinelEndpoints)
}

func (r *RedisInstanceReconciler) updateStatus(ctx context.Context, instance *redisv1alpha1.RedisInstance, phase string) error {
	return r.updateStatusWithConditions(ctx, instance, phase, instance.Status.ReadyReplicas, instance.Status.SentinelEndpoints)
}

func (r *RedisInstanceReconciler) updateStatusWithConditions(ctx context.Context, instance *redisv1alpha1.RedisInstance, phase string, readyReplicas int32, sentinelEndpoints []string) error {
	previousPhase := instance.Status.Phase
	original := instance.DeepCopy()

	instance.Status.Phase = phase
	instance.Status.ReadyReplicas = readyReplicas
	instance.Status.ObservedGeneration = instance.Generation
	instance.Status.MasterEndpoint = fmt.Sprintf("%s-0.%s.%s.svc.cluster.local:%d", instance.Name, instance.Name, instance.Namespace, redisPort)
	instance.Status.SentinelEndpoints = sentinelEndpoints
	instance.Status.Conditions = buildConditions(instance, phase, readyReplicas)

	if err := r.Status().Patch(ctx, instance, client.MergeFrom(original)); err != nil {
		return err
	}

	if phase != previousPhase {
		switch phase {
		case "Running":
			r.Recorder.Event(instance, corev1.EventTypeNormal, "InstanceRunning", "Redis instance is running")
		case "Degraded":
			r.Recorder.Event(instance, corev1.EventTypeWarning, "InstanceDegraded", "Redis instance is degraded")
		}
	}
	return nil
}

func buildConditions(instance *redisv1alpha1.RedisInstance, phase string, readyReplicas int32) []metav1.Condition {
	now := metav1.Now()
	conditions := []metav1.Condition{
		{
			Type:               readyConditionType,
			Status:             boolToConditionStatus(phase == "Running"),
			Reason:             phase,
			Message:            fmt.Sprintf("%d/%d replicas ready", readyReplicas, instance.Spec.Replicas),
			LastTransitionTime: now,
			ObservedGeneration: instance.Generation,
		},
		{
			Type:               reconcilingConditionType,
			Status:             boolToConditionStatus(phase == "Pending"),
			Reason:             phase,
			Message:            "Reconcile completed",
			LastTransitionTime: now,
			ObservedGeneration: instance.Generation,
		},
		{
			Type:               degradedConditionType,
			Status:             boolToConditionStatus(phase == "Degraded" || phase == "Failed"),
			Reason:             phase,
			Message:            fmt.Sprintf("phase=%s", phase),
			LastTransitionTime: now,
			ObservedGeneration: instance.Generation,
		},
	}
	return conditions
}

func boolToConditionStatus(v bool) metav1.ConditionStatus {
	if v {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func (r *RedisInstanceReconciler) isScaleDownInProgress(ctx context.Context, instance *redisv1alpha1.RedisInstance) (bool, error) {
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace(instance.Namespace), client.MatchingLabels(r.redisPodLabels(instance))); err != nil {
		return false, err
	}

	desired := instance.Spec.Replicas
	for i := range podList.Items {
		pod := podList.Items[i]
		ordinal, ok := podOrdinal(pod.Name, instance.Name)
		if ok && ordinal >= desired && pod.DeletionTimestamp == nil {
			return true, nil
		}
	}
	return false, nil
}

func podOrdinal(name, prefix string) (int32, bool) {
	if !strings.HasPrefix(name, prefix+"-") {
		return 0, false
	}
	var ordinal int32
	if _, err := fmt.Sscanf(strings.TrimPrefix(name, prefix+"-"), "%d", &ordinal); err != nil {
		return 0, false
	}
	return ordinal, true
}

func (r *RedisInstanceReconciler) desiredConfigMap(instance *redisv1alpha1.RedisInstance) *corev1.ConfigMap {
	configData := renderRedisConfig(instance.Spec.Config)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-config",
			Namespace: instance.Namespace,
			Labels:    r.baseLabels(instance),
		},
		Data: map[string]string{
			"redis.conf":  configData,
			"start.sh":    r.redisStartScript(instance),
			"sentinel.sh": r.sentinelStartScript(instance),
		},
	}
	_ = controllerutil.SetControllerReference(instance, cm, r.Scheme)
	return cm
}

func renderRedisConfig(config map[string]string) string {
	if len(config) == 0 {
		return ""
	}
	keys := slices.Collect(maps.Keys(config))
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString(" ")
		builder.WriteString(config[key])
		builder.WriteString("\n")
	}
	return builder.String()
}

func (r *RedisInstanceReconciler) desiredHeadlessService(instance *redisv1alpha1.RedisInstance) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    r.baseLabels(instance),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  r.redisPodLabels(instance),
			Ports: []corev1.ServicePort{{
				Name:       "redis",
				Port:       redisPort,
				TargetPort: intstr.FromInt32(redisPort),
			}},
		},
	}
	_ = controllerutil.SetControllerReference(instance, svc, r.Scheme)
	return svc
}

func (r *RedisInstanceReconciler) desiredRedisStatefulSet(instance *redisv1alpha1.RedisInstance) *appsv1.StatefulSet {
	replicas := instance.Spec.Replicas
	podLabels := r.redisPodLabels(instance)
	templateAnnotations := map[string]string{
		configChecksumAnnotation:  checksum(renderRedisConfig(instance.Spec.Config)),
		versionChecksumAnnotation: checksum(instance.Spec.RedisVersion),
		authChecksumAnnotation:    checksum(authSpecChecksum(instance.Spec.Auth)),
	}

	runAsUser := int64(1001)
	runAsGroup := int64(1001)
	fsGroup := int64(1001)
	readOnly := true
	allowEscalation := false

	env := []corev1.EnvVar{
		{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
		{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
		{Name: "SERVICE_NAME", Value: instance.Name},
		{Name: "REDIS_TOPOLOGY", Value: instance.Spec.Topology},
	}
	volumes := []corev1.Volume{{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: instance.Name + "-config"},
				DefaultMode:          ptr(int32(0755)),
			},
		},
	}}
	volumeMounts := []corev1.VolumeMount{
		{Name: "config", MountPath: "/config"},
		{Name: "data", MountPath: "/data"},
	}
	volumes, volumeMounts = appendAuthVolumes(instance, volumes, volumeMounts)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    r.baseLabels(instance),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: instance.Name,
			Selector: &metav1.LabelSelector{
				MatchLabels: podLabels,
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: templateAnnotations,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr(true),
						RunAsUser:    &runAsUser,
						RunAsGroup:   &runAsGroup,
						FSGroup:      &fsGroup,
					},
					Containers: []corev1.Container{{
						Name:            "redis",
						Image:           instance.Spec.RedisVersion,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/bin/sh", "/config/start.sh"},
						Env:             env,
						Ports: []corev1.ContainerPort{{
							Name:          "redis",
							ContainerPort: redisPort,
						}},
						Resources: instance.Spec.Resources,
						SecurityContext: &corev1.SecurityContext{
							ReadOnlyRootFilesystem:   &readOnly,
							AllowPrivilegeEscalation: &allowEscalation,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: volumeMounts,
					}},
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "data",
					Labels: r.baseLabels(instance),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      instance.Spec.Storage.AccessModes,
					Resources:        instance.Spec.Storage.Resources,
					StorageClassName: instance.Spec.Storage.StorageClassName,
				},
			}},
		},
	}
	_ = controllerutil.SetControllerReference(instance, sts, r.Scheme)
	return sts
}

func (r *RedisInstanceReconciler) desiredSentinelService(instance *redisv1alpha1.RedisInstance) *corev1.Service {
	labels := r.sentinelLabels(instance)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-sentinel",
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labels,
			Ports: []corev1.ServicePort{{
				Name:       "sentinel",
				Port:       sentinelPort,
				TargetPort: intstr.FromInt32(sentinelPort),
			}},
		},
	}
	_ = controllerutil.SetControllerReference(instance, svc, r.Scheme)
	return svc
}

func (r *RedisInstanceReconciler) desiredSentinelStatefulSet(instance *redisv1alpha1.RedisInstance) *appsv1.StatefulSet {
	replicas := instance.Spec.Replicas
	labels := r.sentinelLabels(instance)
	readOnly := true
	allowEscalation := false
	runAsUser := int64(1001)
	runAsGroup := int64(1001)
	fsGroup := int64(1001)
	volumes := []corev1.Volume{{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: instance.Name + "-config"},
				DefaultMode:          ptr(int32(0755)),
			},
		},
	}}
	volumeMounts := []corev1.VolumeMount{{Name: "config", MountPath: "/config"}}
	volumes, volumeMounts = appendAuthVolumes(instance, volumes, volumeMounts)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-sentinel",
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: instance.Name + "-sentinel",
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr(true),
						RunAsUser:    &runAsUser,
						RunAsGroup:   &runAsGroup,
						FSGroup:      &fsGroup,
					},
					Containers: []corev1.Container{{
						Name:            "sentinel",
						Image:           instance.Spec.RedisVersion,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/bin/sh", "/config/sentinel.sh"},
						Ports: []corev1.ContainerPort{{
							Name:          "sentinel",
							ContainerPort: sentinelPort,
						}},
						SecurityContext: &corev1.SecurityContext{
							ReadOnlyRootFilesystem:   &readOnly,
							AllowPrivilegeEscalation: &allowEscalation,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: volumeMounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}
	_ = controllerutil.SetControllerReference(instance, sts, r.Scheme)
	return sts
}

func (r *RedisInstanceReconciler) redisStartScript(instance *redisv1alpha1.RedisInstance) string {
	passwordConfig := ""
	if instance.Spec.Auth != nil && instance.Spec.Auth.PasswordSecretRef != nil {
		passwordConfig = `
if [ -f /auth/password ]; then
  REDIS_PASSWORD="$(cat /auth/password)"
  echo "requirepass ${REDIS_PASSWORD}" >> /tmp/redis.conf
  echo "masterauth ${REDIS_PASSWORD}" >> /tmp/redis.conf
fi`
	}

	tlsConfig := ""
	if instance.Spec.Auth != nil && instance.Spec.Auth.TLS != nil {
		tlsConfig = fmt.Sprintf(`
cat >> /tmp/redis.conf <<EOF
port 0
tls-port %d
tls-cert-file /tls/cert/tls.crt
tls-key-file /tls/key/tls.key
tls-ca-cert-file /tls/ca/ca.crt
tls-replication yes
EOF`, redisPort)
	}

	return fmt.Sprintf(`#!/bin/sh
set -eu
cp /config/redis.conf /tmp/redis.conf
ordinal="${POD_NAME##*-}"
if [ "${ordinal}" != "0" ]; then
  echo "replicaof %s-0.%s.${POD_NAMESPACE}.svc.cluster.local %d" >> /tmp/redis.conf
fi
%s
%s
exec redis-server /tmp/redis.conf --appendonly yes --dir /data
`, instance.Name, instance.Name, redisPort, passwordConfig, tlsConfig)
}

func (r *RedisInstanceReconciler) sentinelStartScript(instance *redisv1alpha1.RedisInstance) string {
	authPassConfig := ""
	if instance.Spec.Auth != nil && instance.Spec.Auth.PasswordSecretRef != nil {
		authPassConfig = `
EOF
if [ -f /auth/password ]; then
  REDIS_PASSWORD="$(cat /auth/password)"
  echo "sentinel auth-pass mymaster ${REDIS_PASSWORD}" >> /tmp/sentinel.conf
fi
cat >>/tmp/sentinel.conf <<EOF`
	}

	tlsConfig := ""
	if instance.Spec.Auth != nil && instance.Spec.Auth.TLS != nil {
		tlsConfig = fmt.Sprintf(`
tls-port %d
port 0
tls-cert-file /tls/cert/tls.crt
tls-key-file /tls/key/tls.key
tls-ca-cert-file /tls/ca/ca.crt`, sentinelPort)
	}

	return fmt.Sprintf(`#!/bin/sh
set -eu
cat >/tmp/sentinel.conf <<EOF
port %d
sentinel monitor mymaster %s-0.%s.${POD_NAMESPACE}.svc.cluster.local %d 2
sentinel resolve-hostnames yes
%s
%s
EOF
exec redis-sentinel /tmp/sentinel.conf
`, sentinelPort, instance.Name, instance.Name, redisPort, authPassConfig, tlsConfig)
}

func (r *RedisInstanceReconciler) baseLabels(instance *redisv1alpha1.RedisInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "redis-operator",
		"app.kubernetes.io/component":  "redis",
		"app.kubernetes.io/managed-by": "redis-operator",
		"app.kubernetes.io/instance":   instance.Name,
	}
}

func (r *RedisInstanceReconciler) redisPodLabels(instance *redisv1alpha1.RedisInstance) map[string]string {
	labels := r.baseLabels(instance)
	labels["app.kubernetes.io/component"] = "redis"
	return labels
}

func (r *RedisInstanceReconciler) sentinelLabels(instance *redisv1alpha1.RedisInstance) map[string]string {
	labels := r.baseLabels(instance)
	labels["app.kubernetes.io/component"] = "sentinel"
	return labels
}

func checksum(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func authSpecChecksum(auth *redisv1alpha1.RedisAuthSpec) string {
	if auth == nil {
		return ""
	}

	var parts []string
	if auth.PasswordSecretRef != nil {
		parts = append(parts, fmt.Sprintf("password:%s/%s", auth.PasswordSecretRef.Name, auth.PasswordSecretRef.Key))
	}
	if auth.TLS != nil {
		parts = append(parts, secretRefChecksum("tls-cert", auth.TLS.CertSecretRef))
		parts = append(parts, secretRefChecksum("tls-key", auth.TLS.KeySecretRef))
		parts = append(parts, secretRefChecksum("tls-ca", auth.TLS.CASecretRef))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func secretRefChecksum(prefix string, ref *corev1.SecretKeySelector) string {
	if ref == nil {
		return prefix + ":"
	}
	return fmt.Sprintf("%s:%s/%s", prefix, ref.Name, ref.Key)
}

func appendAuthVolumes(instance *redisv1alpha1.RedisInstance, volumes []corev1.Volume, mounts []corev1.VolumeMount) ([]corev1.Volume, []corev1.VolumeMount) {
	if instance.Spec.Auth == nil {
		return volumes, mounts
	}

	if ref := instance.Spec.Auth.PasswordSecretRef; ref != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "auth",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ref.Name,
					Items: []corev1.KeyToPath{{
						Key:  ref.Key,
						Path: "password",
					}},
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "auth", MountPath: "/auth", ReadOnly: true})
	}

	if tls := instance.Spec.Auth.TLS; tls != nil {
		if tls.CertSecretRef != nil {
			volumes = append(volumes, secretVolume("tls-cert", tls.CertSecretRef, "tls.crt"))
			mounts = append(mounts, corev1.VolumeMount{Name: "tls-cert", MountPath: path.Dir("/tls/cert/tls.crt"), ReadOnly: true})
		}
		if tls.KeySecretRef != nil {
			volumes = append(volumes, secretVolume("tls-key", tls.KeySecretRef, "tls.key"))
			mounts = append(mounts, corev1.VolumeMount{Name: "tls-key", MountPath: path.Dir("/tls/key/tls.key"), ReadOnly: true})
		}
		if tls.CASecretRef != nil {
			volumes = append(volumes, secretVolume("tls-ca", tls.CASecretRef, "ca.crt"))
			mounts = append(mounts, corev1.VolumeMount{Name: "tls-ca", MountPath: path.Dir("/tls/ca/ca.crt"), ReadOnly: true})
		}
	}

	return volumes, mounts
}

func secretVolume(name string, ref *corev1.SecretKeySelector, fileName string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: ref.Name,
				Items: []corev1.KeyToPath{{
					Key:  ref.Key,
					Path: fileName,
				}},
			},
		},
	}
}

func ptr[T any](value T) *T {
	return &value
}

func redisStatefulSetMatches(actual, desired *appsv1.StatefulSet) bool {
	return *actual.Spec.Replicas == *desired.Spec.Replicas &&
		actual.Spec.ServiceName == desired.Spec.ServiceName &&
		apiequality.Semantic.DeepEqual(actual.Spec.Selector, desired.Spec.Selector) &&
		apiequality.Semantic.DeepEqual(actual.Spec.Template, desired.Spec.Template) &&
		apiequality.Semantic.DeepEqual(actual.Spec.VolumeClaimTemplates, desired.Spec.VolumeClaimTemplates)
}

func sentinelStatefulSetMatches(actual, desired *appsv1.StatefulSet) bool {
	return *actual.Spec.Replicas == *desired.Spec.Replicas &&
		actual.Spec.ServiceName == desired.Spec.ServiceName &&
		apiequality.Semantic.DeepEqual(actual.Spec.Selector, desired.Spec.Selector) &&
		apiequality.Semantic.DeepEqual(actual.Spec.Template, desired.Spec.Template)
}

func sentinelServiceMatches(actual *corev1.Service, desired *corev1.Service) bool {
	return actual.Spec.ClusterIP == desired.Spec.ClusterIP &&
		maps.Equal(actual.Spec.Selector, desired.Spec.Selector) &&
		reflect.DeepEqual(actual.Spec.Ports, desired.Spec.Ports)
}

func (r *RedisInstanceReconciler) deletePodsReverseOrdinal(ctx context.Context, namespace string, labels map[string]string, prefix string) (bool, error) {
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return false, err
	}

	sort.Slice(podList.Items, func(i, j int) bool {
		left, _ := podOrdinal(podList.Items[i].Name, prefix)
		right, _ := podOrdinal(podList.Items[j].Name, prefix)
		return left > right
	})

	pending := false
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp != nil {
			pending = true
			continue
		}
		if err := client.IgnoreNotFound(r.Delete(ctx, pod)); err != nil {
			return false, err
		}
		pending = true
	}

	return pending, nil
}

func (r *RedisInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1alpha1.RedisInstance{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
