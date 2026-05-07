package controller

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

func TestCRDRejectsInvalidTopology(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "invalid-topology"}}
	if err := k8sClient.Create(context.Background(), namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	instance := validRedisInstance(namespace.Name, "invalid-topology")
	instance.Spec.Topology = "cluster"
	if err := k8sClient.Create(context.Background(), instance); err == nil {
		t.Fatal("expected invalid topology to be rejected")
	}
}

func TestCRDRejectsMissingStorage(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "missing-storage"}}
	if err := k8sClient.Create(context.Background(), namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	instance := validRedisInstance(namespace.Name, "missing-storage")
	instance.Spec.Storage = redisv1alpha1.RedisStorageSpec{}
	if err := k8sClient.Create(context.Background(), instance); err == nil {
		t.Fatal("expected missing storage details to be rejected")
	}
}

func TestReconcileCreatesOwnedResourcesAndStatus(t *testing.T) {
	reconciler := newTestReconciler(t)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "create-flow"}}
	if err := k8sClient.Create(context.Background(), namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	instance := validRedisInstance(namespace.Name, "example")
	if err := k8sClient.Create(context.Background(), instance); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	runReconcile(t, reconciler, namespace.Name, instance.Name)
	runReconcile(t, reconciler, namespace.Name, instance.Name)

	var current redisv1alpha1.RedisInstance
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: instance.Name, Namespace: namespace.Name}, &current); err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if !containsString(current.Finalizers, redisFinalizer) {
		t.Fatalf("expected finalizer %q", redisFinalizer)
	}
	if current.Status.Phase != "Pending" {
		t.Fatalf("expected Pending phase, got %q", current.Status.Phase)
	}
	if current.Status.ObservedGeneration != current.Generation {
		t.Fatalf("expected observed generation %d, got %d", current.Generation, current.Status.ObservedGeneration)
	}
	if current.Status.MasterEndpoint == "" {
		t.Fatal("expected master endpoint")
	}

	var cm corev1.ConfigMap
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "example-config", Namespace: namespace.Name}, &cm); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if got := cm.Data["redis.conf"]; got != "maxmemory 256mb\n" {
		t.Fatalf("unexpected redis.conf: %q", got)
	}

	var svc corev1.Service
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "example", Namespace: namespace.Name}, &svc); err != nil {
		t.Fatalf("get service: %v", err)
	}
	if svc.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Fatalf("expected headless service, got clusterIP=%q", svc.Spec.ClusterIP)
	}

	var sts appsv1.StatefulSet
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "example", Namespace: namespace.Name}, &sts); err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if *sts.Spec.Replicas != 1 {
		t.Fatalf("expected 1 replica, got %d", *sts.Spec.Replicas)
	}
	if sts.Spec.Template.Spec.Containers[0].Image != "redis:7.2" {
		t.Fatalf("unexpected image: %s", sts.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestReconcileUpdatesStatusToRunningAndDegraded(t *testing.T) {
	reconciler := newTestReconciler(t)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "status-flow"}}
	if err := k8sClient.Create(context.Background(), namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	instance := validRedisInstance(namespace.Name, "status-demo")
	instance.Spec.Replicas = 3
	if err := k8sClient.Create(context.Background(), instance); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	runReconcile(t, reconciler, namespace.Name, instance.Name)
	runReconcile(t, reconciler, namespace.Name, instance.Name)

	var sts appsv1.StatefulSet
	key := types.NamespacedName{Name: "status-demo", Namespace: namespace.Name}
	if err := k8sClient.Get(context.Background(), key, &sts); err != nil {
		t.Fatalf("get statefulset: %v", err)
	}

	sts.Status.ReadyReplicas = 2
	sts.Status.Replicas = 3
	if err := k8sClient.Status().Update(context.Background(), &sts); err != nil {
		t.Fatalf("update sts status: %v", err)
	}
	runReconcile(t, reconciler, namespace.Name, instance.Name)

	var current redisv1alpha1.RedisInstance
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "status-demo", Namespace: namespace.Name}, &current); err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if current.Status.Phase != "Degraded" || current.Status.ReadyReplicas != 2 {
		t.Fatalf("expected degraded/2, got %s/%d", current.Status.Phase, current.Status.ReadyReplicas)
	}

	if err := k8sClient.Get(context.Background(), key, &sts); err != nil {
		t.Fatalf("get statefulset again: %v", err)
	}
	sts.Status.ReadyReplicas = 3
	sts.Status.Replicas = 3
	if err := k8sClient.Status().Update(context.Background(), &sts); err != nil {
		t.Fatalf("update sts status: %v", err)
	}
	runReconcile(t, reconciler, namespace.Name, instance.Name)

	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "status-demo", Namespace: namespace.Name}, &current); err != nil {
		t.Fatalf("get instance again: %v", err)
	}
	if current.Status.Phase != "Running" || current.Status.ReadyReplicas != 3 {
		t.Fatalf("expected running/3, got %s/%d", current.Status.Phase, current.Status.ReadyReplicas)
	}
}

func TestReconcileCreatesSentinelResourcesAndStatus(t *testing.T) {
	reconciler := newTestReconciler(t)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "sentinel-flow"}}
	if err := k8sClient.Create(context.Background(), namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	instance := validRedisInstance(namespace.Name, "sentinel-demo")
	instance.Spec.Topology = redisv1alpha1.TopologySentinel
	instance.Spec.Replicas = 2
	if err := k8sClient.Create(context.Background(), instance); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	runReconcile(t, reconciler, namespace.Name, instance.Name)
	runReconcile(t, reconciler, namespace.Name, instance.Name)

	var svc corev1.Service
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "sentinel-demo-sentinel", Namespace: namespace.Name}, &svc); err != nil {
		t.Fatalf("get sentinel service: %v", err)
	}
	var sts appsv1.StatefulSet
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "sentinel-demo-sentinel", Namespace: namespace.Name}, &sts); err != nil {
		t.Fatalf("get sentinel statefulset: %v", err)
	}

	redisKey := types.NamespacedName{Name: "sentinel-demo", Namespace: namespace.Name}
	if err := k8sClient.Get(context.Background(), redisKey, &appsv1.StatefulSet{}); err != nil {
		t.Fatalf("get redis statefulset: %v", err)
	}
	var redisSts appsv1.StatefulSet
	if err := k8sClient.Get(context.Background(), redisKey, &redisSts); err != nil {
		t.Fatalf("get redis statefulset: %v", err)
	}
	redisSts.Status.ReadyReplicas = 2
	redisSts.Status.Replicas = 2
	if err := k8sClient.Status().Update(context.Background(), &redisSts); err != nil {
		t.Fatalf("update redis sts status: %v", err)
	}
	runReconcile(t, reconciler, namespace.Name, instance.Name)

	var current redisv1alpha1.RedisInstance
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "sentinel-demo", Namespace: namespace.Name}, &current); err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if len(current.Status.SentinelEndpoints) != 2 {
		t.Fatalf("expected 2 sentinel endpoints, got %d", len(current.Status.SentinelEndpoints))
	}
}

func TestFinalizerDeletesOwnedResourcesAndPVCs(t *testing.T) {
	reconciler := newTestReconciler(t)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "delete-flow"}}
	if err := k8sClient.Create(context.Background(), namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	instance := validRedisInstance(namespace.Name, "delete-demo")
	if err := k8sClient.Create(context.Background(), instance); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	runReconcile(t, reconciler, namespace.Name, instance.Name)
	runReconcile(t, reconciler, namespace.Name, instance.Name)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-delete-demo-0",
			Namespace: namespace.Name,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "redis-operator",
				"app.kubernetes.io/component":  "redis",
				"app.kubernetes.io/managed-by": "redis-operator",
				"app.kubernetes.io/instance":   "delete-demo",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	if err := k8sClient.Create(context.Background(), pvc); err != nil {
		t.Fatalf("create pvc: %v", err)
	}

	var current redisv1alpha1.RedisInstance
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "delete-demo", Namespace: namespace.Name}, &current); err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if err := k8sClient.Delete(context.Background(), &current); err != nil {
		t.Fatalf("delete instance: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	runReconcile(t, reconciler, namespace.Name, instance.Name)

	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "delete-demo", Namespace: namespace.Name}, &current); err == nil && containsString(current.Finalizers, redisFinalizer) {
		t.Fatal("expected finalizer removed")
	}

	assertEventuallyDeletedOrTerminatingPVC(t, types.NamespacedName{Name: "data-delete-demo-0", Namespace: namespace.Name})
	assertEventuallyNotFound(t, &corev1.ConfigMap{}, types.NamespacedName{Name: "delete-demo-config", Namespace: namespace.Name})
	assertEventuallyNotFound(t, &corev1.Service{}, types.NamespacedName{Name: "delete-demo", Namespace: namespace.Name})
	assertEventuallyNotFound(t, &appsv1.StatefulSet{}, types.NamespacedName{Name: "delete-demo", Namespace: namespace.Name})
}

func validRedisInstance(namespace, name string) *redisv1alpha1.RedisInstance {
	return &redisv1alpha1.RedisInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: redisv1alpha1.GroupVersion.String(),
			Kind:       "RedisInstance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: redisv1alpha1.RedisInstanceSpec{
			Replicas:     1,
			RedisVersion: "redis:7.2",
			Topology:     redisv1alpha1.TopologyStandalone,
			Config: map[string]string{
				"maxmemory": "256mb",
			},
			Storage: redisv1alpha1.RedisStorageSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func assertEventuallyNotFound(t *testing.T, obj client.Object, key types.NamespacedName) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(context.Background(), key, obj); apierrors.IsNotFound(err) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := k8sClient.Get(context.Background(), key, obj); !apierrors.IsNotFound(err) {
		t.Fatalf("expected %T %s to be not found, got %v", obj, key.String(), err)
	}
}

func assertEventuallyDeletedOrTerminatingPVC(t *testing.T, key types.NamespacedName) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var pvc corev1.PersistentVolumeClaim
		err := k8sClient.Get(context.Background(), key, &pvc)
		if apierrors.IsNotFound(err) {
			return
		}
		if err == nil && pvc.DeletionTimestamp != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	var pvc corev1.PersistentVolumeClaim
	if err := k8sClient.Get(context.Background(), key, &pvc); err != nil {
		t.Fatalf("expected pvc %s to be deleted or terminating, got %v", key.String(), err)
	}
	if pvc.DeletionTimestamp == nil {
		t.Fatalf("expected pvc %s to be terminating", key.String())
	}
}
