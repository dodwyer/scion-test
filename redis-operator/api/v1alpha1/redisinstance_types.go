package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RedisTopology defines the topology type for a Redis instance.
// +kubebuilder:validation:Enum=standalone;sentinel
type RedisTopology string

const (
	TopologyStandalone RedisTopology = "standalone"
	TopologySentinel   RedisTopology = "sentinel"
)

// RedisPhase defines the phase of a Redis instance.
type RedisPhase string

const (
	PhasePending  RedisPhase = "Pending"
	PhaseRunning  RedisPhase = "Running"
	PhaseDegraded RedisPhase = "Degraded"
	PhaseFailed   RedisPhase = "Failed"
)

// FinalizerName is the finalizer added to every RedisInstance.
const FinalizerName = "redis.example.io/cleanup"

// RedisTLSConfig holds TLS certificate configuration.
type RedisTLSConfig struct {
	// SecretName is the name of the Secret containing TLS certificates.
	SecretName string `json:"secretName"`
}

// RedisAuth configures Redis authentication.
type RedisAuth struct {
	// PasswordSecretRef references a Secret key containing the Redis password.
	// +optional
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
	// TLS configures optional TLS for Redis connections.
	// +optional
	TLS *RedisTLSConfig `json:"tls,omitempty"`
}

// RedisInstanceSpec defines the desired state of RedisInstance.
type RedisInstanceSpec struct {
	// Replicas is the total number of Redis pods (1 primary + N-1 replicas).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas"`

	// RedisVersion is the Redis container image reference (e.g. "redis:7.2").
	// +kubebuilder:validation:Required
	RedisVersion string `json:"redisVersion"`

	// Storage defines the PVC template for Redis data volumes.
	// +kubebuilder:validation:Required
	Storage corev1.PersistentVolumeClaimSpec `json:"storage"`

	// Resources defines CPU and memory requests/limits for Redis pods.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Config is a map of redis.conf key-value overrides.
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// Auth configures Redis authentication.
	// +optional
	Auth *RedisAuth `json:"auth,omitempty"`

	// Topology defines the Redis topology: standalone or sentinel.
	// +kubebuilder:validation:Enum=standalone;sentinel
	// +kubebuilder:default=standalone
	Topology RedisTopology `json:"topology"`
}

// RedisInstanceStatus defines the observed state of RedisInstance.
type RedisInstanceStatus struct {
	// Phase is the current lifecycle phase of the RedisInstance.
	// +optional
	Phase RedisPhase `json:"phase,omitempty"`

	// Conditions contains the latest available observations.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the latest metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyReplicas is the count of ready Redis replicas.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// MasterEndpoint is the DNS name of the current writable primary.
	// +optional
	MasterEndpoint string `json:"masterEndpoint,omitempty"`

	// SentinelEndpoints contains DNS addresses of Sentinel instances.
	// +optional
	SentinelEndpoints []string `json:"sentinelEndpoints,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ri
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RedisInstance is the Schema for the redisinstances API.
type RedisInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisInstanceSpec   `json:"spec,omitempty"`
	Status RedisInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RedisInstanceList contains a list of RedisInstance.
type RedisInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisInstance{}, &RedisInstanceList{})
}
