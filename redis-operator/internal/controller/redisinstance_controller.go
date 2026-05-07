package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

const (
	FinalizerName = "redis.example.io/cleanup"

	conditionTypeReady       = "Ready"
	conditionTypeReconciling = "Reconciling"
	conditionTypeDegraded    = "Degraded"

	eventReasonRunning  = "InstanceRunning"
	eventReasonDegraded = "InstanceDegraded"
	eventReasonFailed   = "InstanceFailed"

	labelManagedBy   = "app.kubernetes.io/managed-by"
	labelName        = "app.kubernetes.io/name"
	labelComponent   = "app.kubernetes.io/component"
	labelInstance    = "app.kubernetes.io/instance"
	labelOperatorVal = "redis-operator"
	labelRedisComp   = "redis"
	labelSentinelCo  = "sentinel"

	annotationRestartHash = "redis.example.io/restart-hash"

	sentinelReplicas     = 3
	sentinelPort         = 26379
	redisPort            = 6379
	metricsPort          = 8080
)

// RedisInstanceReconciler reconciles RedisInstance objects.
type RedisInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redis.example.io,resources=redisinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update;patch;delete

func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &redisv1alpha1.RedisInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add finalizer on first reconcile.
	if !controllerutil.ContainsFinalizer(instance, FinalizerName) {
		controllerutil.AddFinalizer(instance, FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Handle deletion.
	if !instance.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, instance)
	}

	// Resolve auth secret if provided.
	if err := r.validateAuthSecret(ctx, instance); err != nil {
		return ctrl.Result{}, r.setFailedPhase(ctx, instance, err.Error())
	}

	// Gate: skip mutation only when generation is current AND all owned resources exist.
	needsMutation := instance.Status.ObservedGeneration != instance.Generation ||
		!r.allOwnedResourcesExist(ctx, instance)

	if needsMutation {
		if err := r.reconcileConfigMap(ctx, instance); err != nil {
			logger.Error(err, "failed to reconcile ConfigMap")
			return ctrl.Result{}, err
		}
		if err := r.reconcileService(ctx, instance); err != nil {
			logger.Error(err, "failed to reconcile Service")
			return ctrl.Result{}, err
		}
		if err := r.reconcileStatefulSet(ctx, instance); err != nil {
			logger.Error(err, "failed to reconcile StatefulSet")
			return ctrl.Result{}, err
		}
		if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
			if err := r.reconcileSentinelResources(ctx, instance); err != nil {
				logger.Error(err, "failed to reconcile Sentinel resources")
				return ctrl.Result{}, err
			}
		}
	}

	if err := r.updateStatus(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleDeletion performs ordered teardown and removes the finalizer.
func (r *RedisInstanceReconciler) handleDeletion(ctx context.Context, instance *redisv1alpha1.RedisInstance) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("handling deletion of RedisInstance", "name", instance.Name)

	// 1. Delete Sentinel StatefulSet and Service first (if applicable).
	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		sentinelSS := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: instance.Name + "-sentinel", Namespace: instance.Namespace}, sentinelSS); err == nil {
			if err := r.Delete(ctx, sentinelSS); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			}
		}
		sentinelSvc := &corev1.Service{}
		if err := r.Get(ctx, types.NamespacedName{Name: instance.Name + "-sentinel", Namespace: instance.Namespace}, sentinelSvc); err == nil {
			if err := r.Delete(ctx, sentinelSvc); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// 2. Delete the Redis StatefulSet (in reverse ordinal, StatefulSet controller handles this).
	redisSS := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, redisSS); err == nil {
		if err := r.Delete(ctx, redisSS); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		// Requeue until the StatefulSet is gone.
		if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, redisSS); err == nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	// 3. Delete owned PVCs.
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcList, client.InNamespace(instance.Namespace),
		client.MatchingLabels(redisLabels(instance.Name))); err != nil {
		return ctrl.Result{}, err
	}
	for i := range pvcList.Items {
		if err := r.Delete(ctx, &pvcList.Items[i]); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. Remove finalizer.
	controllerutil.RemoveFinalizer(instance, FinalizerName)
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("successfully completed ordered teardown", "name", instance.Name)
	return ctrl.Result{}, nil
}

// validateAuthSecret checks the referenced password Secret exists; returns an error if it does not.
func (r *RedisInstanceReconciler) validateAuthSecret(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	if instance.Spec.Auth == nil || instance.Spec.Auth.PasswordSecretRef == nil {
		return nil
	}
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: instance.Spec.Auth.PasswordSecretRef.Name, Namespace: instance.Namespace}
	if err := r.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("auth Secret %q not found", key.Name)
		}
		return err
	}
	return nil
}

// allOwnedResourcesExist returns true when all required owned resources are present.
func (r *RedisInstanceReconciler) allOwnedResourcesExist(ctx context.Context, instance *redisv1alpha1.RedisInstance) bool {
	if !r.resourceExists(ctx, instance.Namespace, instance.Name+"-config", &corev1.ConfigMap{}) {
		return false
	}
	if !r.resourceExists(ctx, instance.Namespace, instance.Name, &corev1.Service{}) {
		return false
	}
	if !r.resourceExists(ctx, instance.Namespace, instance.Name, &appsv1.StatefulSet{}) {
		return false
	}
	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		if !r.resourceExists(ctx, instance.Namespace, instance.Name+"-sentinel", &appsv1.StatefulSet{}) {
			return false
		}
		if !r.resourceExists(ctx, instance.Namespace, instance.Name+"-sentinel", &corev1.Service{}) {
			return false
		}
	}
	return true
}

func (r *RedisInstanceReconciler) resourceExists(ctx context.Context, ns, name string, obj client.Object) bool {
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
	return err == nil
}

// reconcileConfigMap creates or updates the redis.conf ConfigMap.
func (r *RedisInstanceReconciler) reconcileConfigMap(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-config",
			Namespace: instance.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = commonLabels(instance.Name, labelRedisComp)
		cm.Data = map[string]string{
			"redis.conf": renderRedisConf(instance),
		}
		return controllerutil.SetControllerReference(instance, cm, r.Scheme)
	})
	return err
}

// renderRedisConf builds a redis.conf from spec.config entries.
func renderRedisConf(instance *redisv1alpha1.RedisInstance) string {
	var sb strings.Builder
	// Stable ordering for deterministic hash.
	keys := make([]string, 0, len(instance.Spec.Config))
	for k := range instance.Spec.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s %s\n", k, instance.Spec.Config[k])
	}
	return sb.String()
}

// reconcileService creates or updates the headless Service.
func (r *RedisInstanceReconciler) reconcileService(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = commonLabels(instance.Name, labelRedisComp)
		svc.Spec.ClusterIP = "None"
		svc.Spec.Selector = redisLabels(instance.Name)
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "redis", Port: redisPort, Protocol: corev1.ProtocolTCP},
		}
		return controllerutil.SetControllerReference(instance, svc, r.Scheme)
	})
	return err
}

// reconcileStatefulSet creates or updates the Redis StatefulSet.
func (r *RedisInstanceReconciler) reconcileStatefulSet(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	restartHash := computeRestartHash(instance)

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ss, func() error {
		ss.Labels = commonLabels(instance.Name, labelRedisComp)
		replicas := instance.Spec.Replicas
		ss.Spec.Replicas = &replicas
		ss.Spec.ServiceName = instance.Name
		ss.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: redisLabels(instance.Name),
		}
		ss.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		}

		podTemplate := buildRedisPodTemplate(instance, restartHash)
		ss.Spec.Template = podTemplate

		ss.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			buildPVC(instance),
		}

		return controllerutil.SetControllerReference(instance, ss, r.Scheme)
	})
	return err
}

// buildRedisPodTemplate constructs the pod template for Redis StatefulSet pods.
func buildRedisPodTemplate(instance *redisv1alpha1.RedisInstance, restartHash string) corev1.PodTemplateSpec {
	labels := redisLabels(instance.Name)

	runAsNonRoot := true
	readOnlyRootFilesystem := true
	allowPrivilegeEscalation := false

	volumeMounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/data"},
		{Name: "config", MountPath: "/etc/redis"},
	}
	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: instance.Name + "-config"},
				},
			},
		},
	}

	envVars := []corev1.EnvVar{}

	if instance.Spec.Auth != nil && instance.Spec.Auth.PasswordSecretRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: instance.Spec.Auth.PasswordSecretRef,
			},
		})
	}

	if instance.Spec.Auth != nil && instance.Spec.Auth.TLS != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "tls",
			MountPath: "/etc/redis/tls",
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instance.Spec.Auth.TLS.SecretRef.Name,
				},
			},
		})
	}

	// tmp dir needed since root filesystem is read-only.
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "tmp",
		MountPath: "/tmp",
	})
	volumes = append(volumes, corev1.Volume{
		Name: "tmp",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	annotations := map[string]string{
		annotationRestartHash: restartHash,
	}

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &runAsNonRoot,
			},
			Containers: []corev1.Container{
				{
					Name:  "redis",
					Image: instance.Spec.RedisVersion,
					Command: []string{
						"redis-server",
						"/etc/redis/redis.conf",
					},
					Env: envVars,
					Ports: []corev1.ContainerPort{
						{Name: "redis", ContainerPort: redisPort, Protocol: corev1.ProtocolTCP},
					},
					Resources: instance.Spec.Resources,
					VolumeMounts: volumeMounts,
					SecurityContext: &corev1.SecurityContext{
						ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"redis-cli", "ping"},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       10,
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"redis-cli", "ping"},
							},
						},
						InitialDelaySeconds: 15,
						PeriodSeconds:       20,
					},
				},
			},
			Volumes: volumes,
		},
	}
}

// buildPVC constructs the PVC from the spec's storage template.
// StatefulSet.Spec.VolumeClaimTemplates expects []corev1.PersistentVolumeClaim.
func buildPVC(instance *redisv1alpha1.RedisInstance) corev1.PersistentVolumeClaim {
	tmpl := instance.Spec.Storage.VolumeClaimTemplate.DeepCopy()
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: tmpl.ObjectMeta,
		Spec:       tmpl.Spec,
	}
	pvc.Name = "data"
	if pvc.Labels == nil {
		pvc.Labels = redisLabels(instance.Name)
	}
	return pvc
}

// reconcileSentinelResources creates or updates the Sentinel StatefulSet and Service.
func (r *RedisInstanceReconciler) reconcileSentinelResources(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	if err := r.reconcileSentinelService(ctx, instance); err != nil {
		return err
	}
	return r.reconcileSentinelStatefulSet(ctx, instance)
}

func (r *RedisInstanceReconciler) reconcileSentinelService(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-sentinel",
			Namespace: instance.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = commonLabels(instance.Name, labelSentinelCo)
		svc.Spec.ClusterIP = "None"
		svc.Spec.Selector = sentinelLabels(instance.Name)
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "sentinel", Port: sentinelPort, Protocol: corev1.ProtocolTCP},
		}
		return controllerutil.SetControllerReference(instance, svc, r.Scheme)
	})
	return err
}

func (r *RedisInstanceReconciler) reconcileSentinelStatefulSet(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	replicas := int32(sentinelReplicas)

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-sentinel",
			Namespace: instance.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ss, func() error {
		ss.Labels = commonLabels(instance.Name, labelSentinelCo)
		ss.Spec.Replicas = &replicas
		ss.Spec.ServiceName = instance.Name + "-sentinel"
		ss.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: sentinelLabels(instance.Name),
		}
		ss.Spec.Template = buildSentinelPodTemplate(instance)
		return controllerutil.SetControllerReference(instance, ss, r.Scheme)
	})
	return err
}

func buildSentinelPodTemplate(instance *redisv1alpha1.RedisInstance) corev1.PodTemplateSpec {
	labels := sentinelLabels(instance.Name)
	runAsNonRoot := true
	readOnlyRootFilesystem := true
	allowPrivilegeEscalation := false

	// Sentinel config references the primary pod by DNS.
	masterDNS := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", instance.Name, instance.Name, instance.Namespace)
	sentinelConf := fmt.Sprintf(
		"sentinel monitor mymaster %s %d 2\nsentinel down-after-milliseconds mymaster 5000\nsentinel failover-timeout mymaster 60000\nsentinel parallel-syncs mymaster 1\n",
		masterDNS, redisPort,
	)

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &runAsNonRoot,
			},
			InitContainers: []corev1.Container{
				{
					Name:    "sentinel-config",
					Image:   instance.Spec.RedisVersion,
					Command: []string{"sh", "-c", fmt.Sprintf("echo '%s' > /etc/sentinel/sentinel.conf", sentinelConf)},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "sentinel-config", MountPath: "/etc/sentinel"},
					},
					SecurityContext: &corev1.SecurityContext{
						ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "sentinel",
					Image:   instance.Spec.RedisVersion,
					Command: []string{"redis-sentinel", "/etc/sentinel/sentinel.conf"},
					Ports: []corev1.ContainerPort{
						{Name: "sentinel", ContainerPort: sentinelPort, Protocol: corev1.ProtocolTCP},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "sentinel-config", MountPath: "/etc/sentinel"},
						{Name: "tmp", MountPath: "/tmp"},
					},
					SecurityContext: &corev1.SecurityContext{
						ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"redis-cli", "-p", "26379", "ping"},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       10,
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "sentinel-config",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "tmp",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}
}

// updateStatus reads current resource state and updates the RedisInstance status.
func (r *RedisInstanceReconciler) updateStatus(ctx context.Context, instance *redisv1alpha1.RedisInstance) error {
	// Snapshot status before mutation so we can detect phase transitions.
	previousPhase := instance.Status.Phase

	ss := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, ss); err != nil {
		if apierrors.IsNotFound(err) {
			instance.Status.Phase = redisv1alpha1.PhasePending
			instance.Status.ReadyReplicas = 0
		} else {
			return err
		}
	} else {
		readyReplicas := ss.Status.ReadyReplicas
		desired := instance.Spec.Replicas
		instance.Status.ReadyReplicas = readyReplicas

		switch {
		case readyReplicas == 0:
			instance.Status.Phase = redisv1alpha1.PhasePending
		case readyReplicas == desired:
			instance.Status.Phase = redisv1alpha1.PhaseRunning
		default:
			instance.Status.Phase = redisv1alpha1.PhaseDegraded
		}

		// Primary is ordinal 0 pod DNS.
		instance.Status.MasterEndpoint = fmt.Sprintf(
			"%s-0.%s.%s.svc.cluster.local",
			instance.Name, instance.Name, instance.Namespace,
		)
	}

	// Sentinel endpoints.
	if instance.Spec.Topology == redisv1alpha1.TopologySentinel {
		endpoints := make([]string, sentinelReplicas)
		for i := 0; i < sentinelReplicas; i++ {
			endpoints[i] = fmt.Sprintf(
				"%s-sentinel-%d.%s-sentinel.%s.svc.cluster.local",
				instance.Name, i, instance.Name, instance.Namespace,
			)
		}
		instance.Status.SentinelEndpoints = endpoints
	} else {
		instance.Status.SentinelEndpoints = nil
	}

	// Conditions.
	r.setConditions(instance)

	// ObservedGeneration tracks the last generation we processed.
	instance.Status.ObservedGeneration = instance.Generation

	// Persist status.
	if err := r.Status().Update(ctx, instance); err != nil {
		return err
	}

	// Emit events on phase transitions.
	r.emitPhaseEvents(instance, previousPhase)

	return nil
}

// setConditions sets the standard Kubernetes conditions on the instance.
func (r *RedisInstanceReconciler) setConditions(instance *redisv1alpha1.RedisInstance) {
	now := metav1.Now()

	setCondition := func(condType string, status metav1.ConditionStatus, reason, msg string) {
		for i := range instance.Status.Conditions {
			if instance.Status.Conditions[i].Type == condType {
				if instance.Status.Conditions[i].Status != status {
					instance.Status.Conditions[i].LastTransitionTime = now
				}
				instance.Status.Conditions[i].Status = status
				instance.Status.Conditions[i].Reason = reason
				instance.Status.Conditions[i].Message = msg
				instance.Status.Conditions[i].ObservedGeneration = instance.Generation
				return
			}
		}
		instance.Status.Conditions = append(instance.Status.Conditions, metav1.Condition{
			Type:               condType,
			Status:             status,
			LastTransitionTime: now,
			Reason:             reason,
			Message:            msg,
			ObservedGeneration: instance.Generation,
		})
	}

	switch instance.Status.Phase {
	case redisv1alpha1.PhaseRunning:
		setCondition(conditionTypeReady, metav1.ConditionTrue, "AllReplicasReady", "All Redis replicas are ready")
		setCondition(conditionTypeDegraded, metav1.ConditionFalse, "NotDegraded", "Instance is healthy")
		setCondition(conditionTypeReconciling, metav1.ConditionFalse, "ReconcileComplete", "Reconciliation is complete")
	case redisv1alpha1.PhaseDegraded:
		setCondition(conditionTypeReady, metav1.ConditionFalse, "SomeReplicasNotReady", fmt.Sprintf("Only %d of %d replicas ready", instance.Status.ReadyReplicas, instance.Spec.Replicas))
		setCondition(conditionTypeDegraded, metav1.ConditionTrue, "SomeReplicasNotReady", "One or more replicas are not ready")
		setCondition(conditionTypeReconciling, metav1.ConditionFalse, "ReconcileComplete", "Reconciliation is complete")
	case redisv1alpha1.PhaseFailed:
		setCondition(conditionTypeReady, metav1.ConditionFalse, "InstanceFailed", "Instance has failed")
		setCondition(conditionTypeDegraded, metav1.ConditionTrue, "InstanceFailed", "Instance has failed")
		setCondition(conditionTypeReconciling, metav1.ConditionFalse, "ReconcileComplete", "Reconciliation is complete")
	default: // Pending
		setCondition(conditionTypeReady, metav1.ConditionFalse, "Pending", "Redis pods are not yet ready")
		setCondition(conditionTypeDegraded, metav1.ConditionFalse, "Pending", "Instance is being provisioned")
		setCondition(conditionTypeReconciling, metav1.ConditionTrue, "Reconciling", "Instance is being reconciled")
	}
}

// emitPhaseEvents emits Kubernetes Events for phase transitions.
func (r *RedisInstanceReconciler) emitPhaseEvents(instance *redisv1alpha1.RedisInstance, previousPhase redisv1alpha1.PhaseType) {
	if instance.Status.Phase == previousPhase {
		return
	}
	switch instance.Status.Phase {
	case redisv1alpha1.PhaseRunning:
		r.Recorder.Event(instance, corev1.EventTypeNormal, eventReasonRunning,
			fmt.Sprintf("RedisInstance transitioned to Running (%d/%d replicas ready)", instance.Status.ReadyReplicas, instance.Spec.Replicas))
	case redisv1alpha1.PhaseDegraded:
		r.Recorder.Event(instance, corev1.EventTypeWarning, eventReasonDegraded,
			fmt.Sprintf("RedisInstance transitioned to Degraded (%d/%d replicas ready)", instance.Status.ReadyReplicas, instance.Spec.Replicas))
	case redisv1alpha1.PhaseFailed:
		r.Recorder.Event(instance, corev1.EventTypeWarning, eventReasonFailed, "RedisInstance has failed")
	}
}

// setFailedPhase updates the instance status to Failed and emits an event.
func (r *RedisInstanceReconciler) setFailedPhase(ctx context.Context, instance *redisv1alpha1.RedisInstance, msg string) error {
	previousPhase := instance.Status.Phase
	instance.Status.Phase = redisv1alpha1.PhaseFailed
	instance.Status.ObservedGeneration = instance.Generation
	r.setConditions(instance)
	if err := r.Status().Update(ctx, instance); err != nil {
		return err
	}
	r.emitPhaseEvents(instance, previousPhase)
	r.Recorder.Event(instance, corev1.EventTypeWarning, eventReasonFailed, msg)
	return nil
}

// computeRestartHash returns a short hash of version + config for rolling restart tracking.
func computeRestartHash(instance *redisv1alpha1.RedisInstance) string {
	h := sha256.New()
	fmt.Fprintf(h, "version=%s", instance.Spec.RedisVersion)
	keys := make([]string, 0, len(instance.Spec.Config))
	for k := range instance.Spec.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, ",%s=%s", k, instance.Spec.Config[k])
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// redisLabels returns the labels applied to Redis pods and used as selectors.
func redisLabels(name string) map[string]string {
	return map[string]string{
		labelName:      "redis",
		labelInstance:  name,
		labelComponent: labelRedisComp,
		labelManagedBy: labelOperatorVal,
	}
}

// sentinelLabels returns labels for Sentinel pods.
func sentinelLabels(name string) map[string]string {
	return map[string]string{
		labelName:      "redis",
		labelInstance:  name,
		labelComponent: labelSentinelCo,
		labelManagedBy: labelOperatorVal,
	}
}

// commonLabels merges instance and component labels for a resource.
func commonLabels(name, component string) map[string]string {
	return map[string]string{
		labelName:      "redis",
		labelInstance:  name,
		labelComponent: component,
		labelManagedBy: labelOperatorVal,
	}
}

// SetupWithManager registers the controller with the Manager.
func (r *RedisInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1alpha1.RedisInstance{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

