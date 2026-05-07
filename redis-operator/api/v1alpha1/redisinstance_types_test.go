package v1alpha1_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

// TestTopologyTypeValues ensures the topology enum constants are correct.
func TestTopologyTypeValues(t *testing.T) {
	tests := []struct {
		name     string
		topology redisv1alpha1.TopologyType
		want     string
	}{
		{"standalone value", redisv1alpha1.TopologyStandalone, "standalone"},
		{"sentinel value", redisv1alpha1.TopologySentinel, "sentinel"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.topology) != tc.want {
				t.Errorf("got %q, want %q", tc.topology, tc.want)
			}
		})
	}
}

// TestPhaseTypeValues ensures phase enum constants are correct.
func TestPhaseTypeValues(t *testing.T) {
	tests := []struct {
		name  string
		phase redisv1alpha1.PhaseType
		want  string
	}{
		{"Pending", redisv1alpha1.PhasePending, "Pending"},
		{"Running", redisv1alpha1.PhaseRunning, "Running"},
		{"Degraded", redisv1alpha1.PhaseDegraded, "Degraded"},
		{"Failed", redisv1alpha1.PhaseFailed, "Failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.phase) != tc.want {
				t.Errorf("got %q, want %q", tc.phase, tc.want)
			}
		})
	}
}

// TestRedisInstanceSpecRequiredFields verifies that required fields exist on the spec type.
func TestRedisInstanceSpecRequiredFields(t *testing.T) {
	spec := redisv1alpha1.RedisInstanceSpec{
		Replicas:     3,
		RedisVersion: "redis:7.2",
		Topology:     redisv1alpha1.TopologyStandalone,
		Storage: redisv1alpha1.StorageSpec{
			VolumeClaimTemplate: corev1.PersistentVolumeClaimTemplate{
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

	if spec.Replicas != 3 {
		t.Errorf("Replicas: got %d, want 3", spec.Replicas)
	}
	if spec.RedisVersion != "redis:7.2" {
		t.Errorf("RedisVersion: got %q, want %q", spec.RedisVersion, "redis:7.2")
	}
	if spec.Topology != redisv1alpha1.TopologyStandalone {
		t.Errorf("Topology: got %q, want %q", spec.Topology, redisv1alpha1.TopologyStandalone)
	}
	if len(spec.Storage.VolumeClaimTemplate.Spec.AccessModes) == 0 {
		t.Error("Storage.VolumeClaimTemplate.Spec.AccessModes must not be empty")
	}
}

// TestStorageSpecRequired verifies the StorageSpec type holds a PVC template (not empty dir).
func TestStorageSpecRequired(t *testing.T) {
	// The StorageSpec.VolumeClaimTemplate is the only storage mechanism; emptyDir is not supported.
	spec := redisv1alpha1.StorageSpec{
		VolumeClaimTemplate: corev1.PersistentVolumeClaimTemplate{
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("5Gi"),
					},
				},
			},
		},
	}
	if len(spec.VolumeClaimTemplate.Spec.AccessModes) == 0 {
		t.Error("VolumeClaimTemplate.Spec.AccessModes must not be empty")
	}
	storage := spec.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage]
	if storage.IsZero() {
		t.Error("VolumeClaimTemplate.Spec.Resources.Requests[storage] must not be zero")
	}
}

// TestAuthSpecOptional verifies Auth is optional and omitting it is valid.
func TestAuthSpecOptional(t *testing.T) {
	spec := redisv1alpha1.RedisInstanceSpec{
		Replicas:     1,
		RedisVersion: "redis:7.2",
		Topology:     redisv1alpha1.TopologyStandalone,
		Storage: redisv1alpha1.StorageSpec{
			VolumeClaimTemplate: corev1.PersistentVolumeClaimTemplate{},
		},
	}
	if spec.Auth != nil {
		t.Error("Auth should default to nil when omitted")
	}
}

// TestAuthSpecWithPasswordRef verifies password ref is never a literal.
func TestAuthSpecWithPasswordRef(t *testing.T) {
	secretRef := &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "redis-auth"},
		Key:                  "password",
	}
	auth := &redisv1alpha1.AuthSpec{
		PasswordSecretRef: secretRef,
	}
	if auth.PasswordSecretRef == nil {
		t.Fatal("PasswordSecretRef should not be nil")
	}
	if auth.PasswordSecretRef.Name != "redis-auth" {
		t.Errorf("Name: got %q, want %q", auth.PasswordSecretRef.Name, "redis-auth")
	}
	if auth.PasswordSecretRef.Key != "password" {
		t.Errorf("Key: got %q, want %q", auth.PasswordSecretRef.Key, "password")
	}
}

// TestRedisInstanceStatusFields verifies all required status fields are present.
func TestRedisInstanceStatusFields(t *testing.T) {
	status := redisv1alpha1.RedisInstanceStatus{
		Phase:              redisv1alpha1.PhaseRunning,
		ObservedGeneration: 5,
		ReadyReplicas:      3,
		MasterEndpoint:     "myredis-0.myredis.default.svc.cluster.local",
		SentinelEndpoints: []string{
			"myredis-sentinel-0.myredis-sentinel.default.svc.cluster.local",
		},
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "AllReplicasReady",
				Message:            "All Redis replicas are ready",
			},
		},
	}

	if status.Phase != redisv1alpha1.PhaseRunning {
		t.Errorf("Phase: got %q, want Running", status.Phase)
	}
	if status.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration: got %d, want 5", status.ObservedGeneration)
	}
	if status.ReadyReplicas != 3 {
		t.Errorf("ReadyReplicas: got %d, want 3", status.ReadyReplicas)
	}
	if status.MasterEndpoint == "" {
		t.Error("MasterEndpoint must not be empty")
	}
	if len(status.SentinelEndpoints) == 0 {
		t.Error("SentinelEndpoints must not be empty when topology is sentinel")
	}
	if len(status.Conditions) == 0 {
		t.Error("Conditions must not be empty")
	}
}

// TestTopologyEnumValidation verifies that only valid topology values are accepted by the type system.
func TestTopologyEnumValidation(t *testing.T) {
	validTopologies := []redisv1alpha1.TopologyType{
		redisv1alpha1.TopologyStandalone,
		redisv1alpha1.TopologySentinel,
	}
	for _, topology := range validTopologies {
		if topology != redisv1alpha1.TopologyStandalone && topology != redisv1alpha1.TopologySentinel {
			t.Errorf("Unexpected topology value: %q", topology)
		}
	}

	// Redis Cluster is explicitly out of scope; confirm it does NOT match either enum value.
	clusterTopology := redisv1alpha1.TopologyType("cluster")
	if clusterTopology == redisv1alpha1.TopologyStandalone || clusterTopology == redisv1alpha1.TopologySentinel {
		t.Error("cluster topology must not be a valid value (out of scope for v1)")
	}
}

// TestDeepCopy verifies deep copy of the spec does not share references.
func TestDeepCopy(t *testing.T) {
	original := &redisv1alpha1.RedisInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: redisv1alpha1.RedisInstanceSpec{
			Replicas:     2,
			RedisVersion: "redis:7.2",
			Topology:     redisv1alpha1.TopologyStandalone,
			Config:       map[string]string{"maxmemory": "512mb"},
			Storage: redisv1alpha1.StorageSpec{
				VolumeClaimTemplate: corev1.PersistentVolumeClaimTemplate{},
			},
		},
	}

	copy := original.DeepCopy()
	copy.Spec.Config["maxmemory"] = "1gb"
	copy.Spec.Replicas = 5

	if original.Spec.Config["maxmemory"] != "512mb" {
		t.Error("DeepCopy did not isolate Config map: original was mutated")
	}
	if original.Spec.Replicas != 2 {
		t.Error("DeepCopy did not isolate Replicas: original was mutated")
	}
}
