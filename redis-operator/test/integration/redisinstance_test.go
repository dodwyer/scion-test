package integration

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

const (
	pollInterval = 100 * time.Millisecond
	pollTimeout  = 30 * time.Second
)

func newTestRI(name, ns string, topology redisv1alpha1.RedisTopology) *redisv1alpha1.RedisInstance {
	return &redisv1alpha1.RedisInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: redisv1alpha1.RedisInstanceSpec{
			Replicas:     1,
			RedisVersion: "redis:7.2",
			Topology:     topology,
			Storage: corev1.PersistentVolumeClaimTemplate{
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
}

func waitForObject(t *testing.T, obj client.Object, nn types.NamespacedName) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(context.Background(), pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, nn, obj); err != nil {
			return false, client.IgnoreNotFound(err)
		}
		return true, nil
	}); err != nil {
		t.Fatalf("timed out waiting for %T %s: %v", obj, nn, err)
	}
}

func TestCreate_StandaloneReconciliation(t *testing.T) {
	ctx := context.Background()
	ns := "default"
	name := "integ-standalone"

	ri := newTestRI(name, ns, redisv1alpha1.TopologyStandalone)
	if err := k8sClient.Create(ctx, ri); err != nil {
		t.Fatalf("create RedisInstance: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ri)
	})

	// Finalizer should be added.
	got := &redisv1alpha1.RedisInstance{}
	waitForObject(t, got, types.NamespacedName{Namespace: ns, Name: name})
	if err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, got); err != nil {
			return false, err
		}
		for _, f := range got.Finalizers {
			if f == redisv1alpha1.FinalizerName {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("timed out waiting for finalizer: %v", err)
	}

	// ConfigMap must be created.
	cm := &corev1.ConfigMap{}
	waitForObject(t, cm, types.NamespacedName{Namespace: ns, Name: name + "-config"})

	// Headless Service must be created.
	svc := &corev1.Service{}
	waitForObject(t, svc, types.NamespacedName{Namespace: ns, Name: name})
	if svc.Spec.ClusterIP != "None" {
		t.Errorf("expected headless service, got ClusterIP=%q", svc.Spec.ClusterIP)
	}

	// StatefulSet must be created.
	sts := &appsv1.StatefulSet{}
	waitForObject(t, sts, types.NamespacedName{Namespace: ns, Name: name})
	if *sts.Spec.Replicas != 1 {
		t.Errorf("expected 1 replica, got %d", *sts.Spec.Replicas)
	}

	// No Sentinel resources.
	sentinelSts := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name + "-sentinel"}, sentinelSts); err == nil {
		t.Error("unexpected sentinel StatefulSet for standalone topology")
	}
}

func TestCreate_SentinelReconciliation(t *testing.T) {
	ctx := context.Background()
	ns := "default"
	name := "integ-sentinel"

	ri := newTestRI(name, ns, redisv1alpha1.TopologySentinel)
	ri.Spec.Replicas = 3
	if err := k8sClient.Create(ctx, ri); err != nil {
		t.Fatalf("create RedisInstance: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ri)
	})

	// Sentinel StatefulSet must be created.
	sentinelSts := &appsv1.StatefulSet{}
	waitForObject(t, sentinelSts, types.NamespacedName{Namespace: ns, Name: name + "-sentinel"})

	// Sentinel Service must be created.
	sentinelSvc := &corev1.Service{}
	waitForObject(t, sentinelSvc, types.NamespacedName{Namespace: ns, Name: name + "-sentinel"})
}

func TestUpdate_ConfigChangeTriggersAnnotation(t *testing.T) {
	ctx := context.Background()
	ns := "default"
	name := "integ-config-update"

	ri := newTestRI(name, ns, redisv1alpha1.TopologyStandalone)
	ri.Spec.Config = map[string]string{"maxmemory": "256mb"}
	if err := k8sClient.Create(ctx, ri); err != nil {
		t.Fatalf("create RedisInstance: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ri)
	})

	// Wait for initial StatefulSet.
	sts := &appsv1.StatefulSet{}
	waitForObject(t, sts, types.NamespacedName{Namespace: ns, Name: name})

	hash1 := sts.Spec.Template.Annotations["redis.example.io/config-hash"]

	// Update config.
	updated := &redisv1alpha1.RedisInstance{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, updated); err != nil {
		t.Fatal(err)
	}
	updated.Spec.Config = map[string]string{"maxmemory": "512mb"}
	if err := k8sClient.Update(ctx, updated); err != nil {
		t.Fatalf("update RedisInstance: %v", err)
	}

	// Wait for annotation to change.
	if err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		sts2 := &appsv1.StatefulSet{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sts2); err != nil {
			return false, err
		}
		return sts2.Spec.Template.Annotations["redis.example.io/config-hash"] != hash1, nil
	}); err != nil {
		t.Fatalf("timed out waiting for config-hash annotation to change: %v", err)
	}
}

func TestDelete_FinalizerPreventsDeletion(t *testing.T) {
	ctx := context.Background()
	ns := "default"
	name := "integ-delete"

	ri := newTestRI(name, ns, redisv1alpha1.TopologyStandalone)
	if err := k8sClient.Create(ctx, ri); err != nil {
		t.Fatalf("create RedisInstance: %v", err)
	}

	// Wait for finalizer.
	got := &redisv1alpha1.RedisInstance{}
	if err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, got); err != nil {
			return false, err
		}
		for _, f := range got.Finalizers {
			if f == redisv1alpha1.FinalizerName {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("timed out waiting for finalizer: %v", err)
	}

	// Delete the CR — it should not disappear immediately (finalizer in place).
	if err := k8sClient.Delete(ctx, got); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Give the controller a moment to start teardown.
	time.Sleep(200 * time.Millisecond)

	// The CR should eventually disappear once teardown completes
	// (in envtest there are no real pods to wait for).
	if err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		check := &redisv1alpha1.RedisInstance{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, check); err != nil {
			return client.IgnoreNotFound(err) == nil, nil
		}
		return false, nil
	}); err != nil {
		t.Fatalf("timed out waiting for CR deletion: %v", err)
	}
}

func TestScaleUp_UpdatesStatefulSetReplicas(t *testing.T) {
	ctx := context.Background()
	ns := "default"
	name := "integ-scaleup"

	ri := newTestRI(name, ns, redisv1alpha1.TopologyStandalone)
	ri.Spec.Replicas = 1
	if err := k8sClient.Create(ctx, ri); err != nil {
		t.Fatalf("create RedisInstance: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ri)
	})

	// Wait for initial StatefulSet.
	sts := &appsv1.StatefulSet{}
	waitForObject(t, sts, types.NamespacedName{Namespace: ns, Name: name})

	// Scale up.
	updated := &redisv1alpha1.RedisInstance{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, updated); err != nil {
		t.Fatal(err)
	}
	updated.Spec.Replicas = 3
	if err := k8sClient.Update(ctx, updated); err != nil {
		t.Fatalf("update replicas: %v", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, func(ctx context.Context) (bool, error) {
		sts2 := &appsv1.StatefulSet{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sts2); err != nil {
			return false, err
		}
		return *sts2.Spec.Replicas == 3, nil
	}); err != nil {
		t.Fatalf("timed out waiting for scale-up: %v", err)
	}
}
