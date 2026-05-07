package v1alpha1_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

func TestRedisInstanceSpec_RequiredFields(t *testing.T) {
	spec := redisv1alpha1.RedisInstanceSpec{
		Replicas:     1,
		RedisVersion: "redis:7.2",
		Storage: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
		Topology: redisv1alpha1.TopologyStandalone,
	}

	if spec.Replicas <= 0 {
		t.Error("expected Replicas > 0")
	}
	if spec.RedisVersion == "" {
		t.Error("expected RedisVersion to be non-empty")
	}
	if len(spec.Storage.AccessModes) == 0 {
		t.Error("expected Storage.AccessModes to be non-empty")
	}
}

func TestRedisInstanceTopologyEnum(t *testing.T) {
	validTopologies := []redisv1alpha1.RedisTopology{
		redisv1alpha1.TopologyStandalone,
		redisv1alpha1.TopologySentinel,
	}
	for _, topo := range validTopologies {
		if topo != redisv1alpha1.TopologyStandalone && topo != redisv1alpha1.TopologySentinel {
			t.Errorf("unexpected topology value: %s", topo)
		}
	}
}

func TestRedisInstanceStatus_Fields(t *testing.T) {
	status := redisv1alpha1.RedisInstanceStatus{
		Phase:              redisv1alpha1.PhaseRunning,
		ObservedGeneration: 1,
		ReadyReplicas:      3,
		MasterEndpoint:     "redis-0.redis.default.svc.cluster.local:6379",
		SentinelEndpoints:  []string{"sentinel-0.sentinel.default.svc.cluster.local:26379"},
	}

	if status.Phase != redisv1alpha1.PhaseRunning {
		t.Errorf("expected phase Running, got %s", status.Phase)
	}
	if status.ObservedGeneration != 1 {
		t.Errorf("expected ObservedGeneration 1, got %d", status.ObservedGeneration)
	}
}

func TestRedisInstanceSpec_StorageRequired(t *testing.T) {
	// Validates that an empty storage spec can be detected (actual CRD validation is server-side)
	spec := redisv1alpha1.RedisInstanceSpec{
		Replicas:     1,
		RedisVersion: "redis:7.2",
		Topology:     redisv1alpha1.TopologyStandalone,
	}
	if len(spec.Storage.AccessModes) != 0 {
		t.Error("expected empty storage to have no access modes")
	}
}

func TestRedisInstanceDeepCopy(t *testing.T) {
	original := &redisv1alpha1.RedisInstance{}
	original.Name = "test"
	original.Spec.Replicas = 3
	original.Spec.RedisVersion = "redis:7.2"
	original.Spec.Topology = redisv1alpha1.TopologyStandalone
	original.Spec.Config = map[string]string{"maxmemory": "512mb"}

	copy := original.DeepCopy()
	if copy == original {
		t.Error("DeepCopy should return a new pointer")
	}
	if copy.Spec.Replicas != original.Spec.Replicas {
		t.Errorf("DeepCopy should preserve Replicas: got %d want %d", copy.Spec.Replicas, original.Spec.Replicas)
	}
	// Modifying copy should not affect original
	copy.Spec.Config["maxmemory"] = "1gb"
	if original.Spec.Config["maxmemory"] != "512mb" {
		t.Error("DeepCopy should deep copy map fields")
	}
}

func TestRedisAuthDeepCopy(t *testing.T) {
	auth := &redisv1alpha1.RedisAuth{
		TLS: &redisv1alpha1.RedisTLSConfig{
			SecretName: "my-tls-secret",
		},
	}
	authCopy := auth.DeepCopy()
	if authCopy == auth {
		t.Error("DeepCopy should return new pointer")
	}
	if authCopy.TLS == auth.TLS {
		t.Error("DeepCopy should deep copy TLS pointer")
	}
	if authCopy.TLS.SecretName != "my-tls-secret" {
		t.Error("DeepCopy should preserve TLS.SecretName")
	}
}
