package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RedisTopology specifies the Redis deployment topology.
// +kubebuilder:validation:Enum=standalone;sentinel
type RedisTopology string

const (
	TopologyStandalone RedisTopology = "standalone"
	TopologySentinel   RedisTopology = "sentinel"
)

// Phase constants for status.phase.
const (
	PhasePending  = "Pending"
	PhaseRunning  = "Running"
	PhaseDegraded = "Degraded"
	PhaseFailed   = "Failed"
)

// FinalizerName is the finalizer added to every RedisInstance CR.
const FinalizerName = "redis.example.io/cleanup"

// Condition type constants following Kubernetes conventions.
const (
	ConditionTypeReady       = "Ready"
	ConditionTypeReconciling = "Reconciling"
	ConditionTypeDegraded    = "Degraded"
)

// RedisInstanceSpec defines the desired state of RedisInstance.
type RedisInstanceSpec struct {
	// Replicas is the total number of Redis data pods (one writable primary + N-1 read replicas).
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas"`

	// RedisVersion is the Redis container image tag or digest (e.g. "redis:7.2").
	RedisVersion string `json:"redisVersion"`

	// Storage is the PVC template used for each Redis data pod.
	Storage corev1.PersistentVolumeClaimTemplate `json:"storage"`

	// Resources specifies CPU/memory requests and limits for Redis containers.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Config holds redis.conf key/value overrides applied to every Redis pod.
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// Auth holds optional authentication settings.
	// +optional
	Auth *RedisAuth `json:"auth,omitempty"`

	// Topology defines the Redis deployment topology.
	// +kubebuilder:validation:Enum=standalone;sentinel
	Topology RedisTopology `json:"topology"`
}

// RedisAuth defines optional authentication configuration.
type RedisAuth struct {
	// PasswordSecretRef references a Kubernetes Secret that holds the Redis password.
	// If unset, Redis runs without authentication.
	// +optional
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`

	// TLS holds optional TLS certificate configuration.
	// +optional
	TLS *RedisTLSConfig `json:"tls,omitempty"`
}

// RedisTLSConfig references a Secret containing TLS certificates.
type RedisTLSConfig struct {
	// SecretRef names the Secret holding tls.crt and tls.key.
	SecretRef corev1.SecretReference `json:"secretRef"`
}

// RedisInstanceStatus defines the observed state of RedisInstance.
type RedisInstanceStatus struct {
	// Phase is one of Pending, Running, Degraded, or Failed.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions holds standard Kubernetes condition objects.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the metadata.generation that was last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyReplicas is the count of Redis pods in the Ready state.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// MasterEndpoint is the DNS name of the current writable primary.
	// +optional
	MasterEndpoint string `json:"masterEndpoint,omitempty"`

	// SentinelEndpoints are DNS names of Sentinel pods (sentinel topology only).
	// +optional
	SentinelEndpoints []string `json:"sentinelEndpoints,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ri
// +kubebuilder:printcolumn:name="Topology",type=string,JSONPath=`.spec.topology`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
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
