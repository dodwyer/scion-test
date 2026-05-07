package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TopologyType defines the Redis deployment topology.
// +kubebuilder:validation:Enum=standalone;sentinel
type TopologyType string

const (
	TopologyStandalone TopologyType = "standalone"
	TopologySentinel   TopologyType = "sentinel"
)

// PhaseType represents the current phase of a RedisInstance.
type PhaseType string

const (
	PhasePending  PhaseType = "Pending"
	PhaseRunning  PhaseType = "Running"
	PhaseDegraded PhaseType = "Degraded"
	PhaseFailed   PhaseType = "Failed"
)

// StorageSpec defines the PVC template for Redis data volumes.
type StorageSpec struct {
	// VolumeClaimTemplate is the PVC template for Redis data storage.
	// +kubebuilder:validation:Required
	VolumeClaimTemplate corev1.PersistentVolumeClaimTemplate `json:"volumeClaimTemplate"`
}

// TLSConfig references a Secret containing TLS certificate material.
type TLSConfig struct {
	// SecretRef is the name of the Secret containing the TLS certificate and key.
	// +kubebuilder:validation:Required
	SecretRef corev1.SecretReference `json:"secretRef"`
}

// AuthSpec configures optional Redis authentication.
type AuthSpec struct {
	// PasswordSecretRef references a Secret key holding the Redis password.
	// When omitted, Redis runs without password authentication.
	// +optional
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
	// TLS configures optional TLS for Redis connections.
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
}

// RedisInstanceSpec defines the desired state of RedisInstance.
type RedisInstanceSpec struct {
	// Replicas is the total number of Redis pods (one primary + N-1 read replicas).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas"`

	// RedisVersion is the Redis container image reference (e.g. "redis:7.2").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	RedisVersion string `json:"redisVersion"`

	// Storage defines the PVC template for Redis data volumes. Required in v1.
	// +kubebuilder:validation:Required
	Storage StorageSpec `json:"storage"`

	// Resources sets CPU and memory requests/limits for Redis containers.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Config contains redis.conf key/value overrides.
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// Auth configures optional password and TLS authentication.
	// +optional
	Auth *AuthSpec `json:"auth,omitempty"`

	// Topology selects the Redis deployment topology.
	// Allowed values: standalone, sentinel. Redis Cluster is out of scope for v1.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=standalone;sentinel
	Topology TopologyType `json:"topology"`
}

// RedisInstanceStatus defines the observed state of RedisInstance.
type RedisInstanceStatus struct {
	// Phase summarises the current lifecycle phase of the instance.
	// +optional
	Phase PhaseType `json:"phase,omitempty"`

	// Conditions contains detailed condition information for the instance.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent metadata.generation seen by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyReplicas is the count of Redis pods that are currently ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// MasterEndpoint is the DNS name of the current writable primary pod.
	// +optional
	MasterEndpoint string `json:"masterEndpoint,omitempty"`

	// SentinelEndpoints lists the DNS names of Sentinel pods (sentinel topology only).
	// +optional
	SentinelEndpoints []string `json:"sentinelEndpoints,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ri
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Topology",type=string,JSONPath=`.spec.topology`
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
