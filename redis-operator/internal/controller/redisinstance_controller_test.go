package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

// newScheme returns a Scheme with all required types registered.
func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := redisv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// minimalRedisInstance returns a minimal valid RedisInstance for testing.
func minimalRedisInstance(name, namespace string, topology redisv1alpha1.RedisTopology) *redisv1alpha1.RedisInstance {
	return &redisv1alpha1.RedisInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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

// newReconciler returns a reconciler wired to a fake client containing the given objects.
func newReconciler(t *testing.T, objs ...client.Object) *RedisInstanceReconciler {
	t.Helper()
	s := newScheme(t)
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).WithStatusSubresource(&redisv1alpha1.RedisInstance{}).Build()
	return &RedisInstanceReconciler{
		Client:   fc,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(100),
	}
}

// reconcile is a helper that calls Reconcile and fails the test on unexpected errors.
func reconcile(t *testing.T, r *RedisInstanceReconciler, name, namespace string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	return res
}

// --- Tests ---

func TestReconcile_AddsFinalizer(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	r := newReconciler(t, ri)

	// First reconcile should add the finalizer and return (no error).
	reconcile(t, r, "myredis", "default")

	got := &redisv1alpha1.RedisInstance{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis"), got); err != nil {
		t.Fatal(err)
	}
	if len(got.Finalizers) == 0 || got.Finalizers[0] != redisv1alpha1.FinalizerName {
		t.Errorf("expected finalizer %q, got %v", redisv1alpha1.FinalizerName, got.Finalizers)
	}
}

func TestReconcile_CreatesConfigMap(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	// Second reconcile (after finalizer is added).
	reconcile(t, r, "myredis", "default")

	cm := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis-config"), cm); err != nil {
		t.Fatalf("ConfigMap not found: %v", err)
	}
	if _, ok := cm.Data["redis.conf"]; !ok {
		t.Error("ConfigMap missing redis.conf key")
	}
}

func TestReconcile_CreatesHeadlessService(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	reconcile(t, r, "myredis", "default")

	svc := &corev1.Service{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis"), svc); err != nil {
		t.Fatalf("Service not found: %v", err)
	}
	if svc.Spec.ClusterIP != "None" {
		t.Errorf("expected headless service (ClusterIP=None), got %q", svc.Spec.ClusterIP)
	}
}

func TestReconcile_CreatesStatefulSet(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	reconcile(t, r, "myredis", "default")

	sts := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis"), sts); err != nil {
		t.Fatalf("StatefulSet not found: %v", err)
	}
	if *sts.Spec.Replicas != 1 {
		t.Errorf("expected 1 replica, got %d", *sts.Spec.Replicas)
	}
	if sts.Spec.Template.Spec.Containers[0].Image != "redis:7.2" {
		t.Errorf("unexpected container image: %s", sts.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestReconcile_Standalone_NoSentinelResources(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	reconcile(t, r, "myredis", "default")

	sentinelSts := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis-sentinel"), sentinelSts); err == nil {
		t.Error("expected no sentinel StatefulSet for standalone topology, but found one")
	}
	sentinelSvc := &corev1.Service{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis-sentinel"), sentinelSvc); err == nil {
		t.Error("expected no sentinel Service for standalone topology, but found one")
	}
}

func TestReconcile_Sentinel_CreatesSentinelResources(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologySentinel)
	ri.Spec.Replicas = 3
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	reconcile(t, r, "myredis", "default")

	sentinelSts := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis-sentinel"), sentinelSts); err != nil {
		t.Fatalf("sentinel StatefulSet not found: %v", err)
	}
	if *sentinelSts.Spec.Replicas != sentinelReplicas {
		t.Errorf("expected %d sentinel replicas, got %d", sentinelReplicas, *sentinelSts.Spec.Replicas)
	}

	sentinelSvc := &corev1.Service{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis-sentinel"), sentinelSvc); err != nil {
		t.Fatalf("sentinel Service not found: %v", err)
	}
}

func TestReconcile_ConfigMapContent(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	ri.Spec.Config = map[string]string{
		"maxmemory":        "512mb",
		"maxmemory-policy": "allkeys-lru",
	}
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	reconcile(t, r, "myredis", "default")

	cm := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis-config"), cm); err != nil {
		t.Fatal(err)
	}
	conf := cm.Data["redis.conf"]
	for _, want := range []string{"maxmemory 512mb", "maxmemory-policy allkeys-lru"} {
		if !contains(conf, want) {
			t.Errorf("redis.conf missing %q; got:\n%s", want, conf)
		}
	}
}

func TestReconcile_StatefulSetSecurityContext(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	reconcile(t, r, "myredis", "default")

	sts := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis"), sts); err != nil {
		t.Fatal(err)
	}

	pod := sts.Spec.Template.Spec
	if pod.SecurityContext == nil || !*pod.SecurityContext.RunAsNonRoot {
		t.Error("expected RunAsNonRoot=true in pod security context")
	}
	if len(pod.Containers) == 0 {
		t.Fatal("no containers")
	}
	csc := pod.Containers[0].SecurityContext
	if csc == nil || csc.ReadOnlyRootFilesystem == nil || !*csc.ReadOnlyRootFilesystem {
		t.Error("expected ReadOnlyRootFilesystem=true in container security context")
	}
	if csc == nil || csc.AllowPrivilegeEscalation == nil || *csc.AllowPrivilegeEscalation {
		t.Error("expected AllowPrivilegeEscalation=false in container security context")
	}
	if csc == nil || csc.Capabilities == nil || len(csc.Capabilities.Drop) == 0 {
		t.Error("expected capabilities to be dropped")
	}
}

func TestReconcile_RollingRestartAnnotation(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	ri.Spec.Config = map[string]string{"maxmemory": "256mb"}
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	reconcile(t, r, "myredis", "default")

	sts := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis"), sts); err != nil {
		t.Fatal(err)
	}
	hash1 := sts.Spec.Template.Annotations[configHashAnnotation]
	if hash1 == "" {
		t.Fatal("expected config-hash annotation to be set")
	}

	// Update spec.config. The fake client does not auto-increment generation, so we
	// trigger drift detection by deleting the StatefulSet; allResourcesExist returns
	// false and the reconciler performs a full reconcile with the new config.
	if err := r.Delete(context.Background(), sts); err != nil {
		t.Fatal(err)
	}
	ri2 := &redisv1alpha1.RedisInstance{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis"), ri2); err != nil {
		t.Fatal(err)
	}
	ri2.Spec.Config = map[string]string{"maxmemory": "512mb"}
	if err := r.Update(context.Background(), ri2); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "myredis", "default")

	sts2 := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis"), sts2); err != nil {
		t.Fatal(err)
	}
	hash2 := sts2.Spec.Template.Annotations[configHashAnnotation]
	if hash1 == hash2 {
		t.Error("expected config-hash annotation to change when spec.config changes")
	}
}

func TestReconcile_StatefulSetReplicaCount(t *testing.T) {
	ri := minimalRedisInstance("myredis", "default", redisv1alpha1.TopologyStandalone)
	ri.Spec.Replicas = 3
	r := newReconciler(t, ri)

	reconcile(t, r, "myredis", "default")
	reconcile(t, r, "myredis", "default")

	sts := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(), namespacedName("default", "myredis"), sts); err != nil {
		t.Fatal(err)
	}
	if *sts.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %d", *sts.Spec.Replicas)
	}
}

func TestReconcile_NotFound_ReturnsNoError(t *testing.T) {
	r := newReconciler(t) // empty fake client
	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
	})
	if err != nil {
		t.Errorf("expected no error for not-found CR, got %v", err)
	}
	if res.Requeue {
		t.Error("expected no requeue for not-found CR")
	}
}

func TestRenderRedisConf_Empty(t *testing.T) {
	got := renderRedisConf(nil)
	if got == "" {
		t.Error("renderRedisConf returned empty string for nil config")
	}
}

func TestRenderRedisConf_KeyValuePairs(t *testing.T) {
	config := map[string]string{
		"maxmemory":        "512mb",
		"maxmemory-policy": "allkeys-lru",
	}
	got := renderRedisConf(config)
	for _, want := range []string{"maxmemory 512mb", "maxmemory-policy allkeys-lru"} {
		if !contains(got, want) {
			t.Errorf("renderRedisConf output missing %q; got:\n%s", want, got)
		}
	}
}

func TestComputeConfigHash_Deterministic(t *testing.T) {
	ri := minimalRedisInstance("x", "y", redisv1alpha1.TopologyStandalone)
	ri.Spec.Config = map[string]string{"a": "1", "b": "2"}
	h1 := computeConfigHash(ri)
	h2 := computeConfigHash(ri)
	if h1 != h2 {
		t.Error("computeConfigHash is not deterministic")
	}
}

func TestComputeConfigHash_ChangesOnVersionUpdate(t *testing.T) {
	ri := minimalRedisInstance("x", "y", redisv1alpha1.TopologyStandalone)
	ri.Spec.Config = map[string]string{"a": "1"}
	h1 := computeConfigHash(ri)
	ri.Spec.RedisVersion = "redis:7.0"
	h2 := computeConfigHash(ri)
	if h1 == h2 {
		t.Error("expected config hash to change when redisVersion changes")
	}
}

func TestComputePhase(t *testing.T) {
	tests := []struct {
		ready, desired int32
		want           string
	}{
		{0, 3, redisv1alpha1.PhasePending},
		{3, 3, redisv1alpha1.PhaseRunning},
		{2, 3, redisv1alpha1.PhaseDegraded},
		{1, 1, redisv1alpha1.PhaseRunning},
	}
	for _, tc := range tests {
		got := computePhase(tc.ready, tc.desired)
		if got != tc.want {
			t.Errorf("computePhase(%d, %d) = %q, want %q", tc.ready, tc.desired, got, tc.want)
		}
	}
}

func TestOrdinalFromPodName(t *testing.T) {
	tests := []struct {
		podName  string
		stsName  string
		expected int32
	}{
		{"myredis-0", "myredis", 0},
		{"myredis-2", "myredis", 2},
		{"myredis-sentinel-1", "myredis-sentinel", 1},
	}
	for _, tc := range tests {
		got := ordinalFromPodName(tc.podName, tc.stsName)
		if got != tc.expected {
			t.Errorf("ordinalFromPodName(%q, %q) = %d, want %d", tc.podName, tc.stsName, got, tc.expected)
		}
	}
}

func TestPrimaryDNS(t *testing.T) {
	got := primaryDNS("myredis", "staging")
	want := "myredis-0.myredis.staging.svc.cluster.local"
	if got != want {
		t.Errorf("primaryDNS = %q, want %q", got, want)
	}
}

// --- helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexStr(s, substr) >= 0
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
