package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TopologyStandalone = "standalone"
	TopologySentinel   = "sentinel"
)

type RedisPersistentVolumeClaimTemplate struct {
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	Spec     corev1.PersistentVolumeClaimSpec `json:"spec"`
}

type RedisAuthSpec struct {
	// +optional
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
}

type TLSConfig struct {
	SecretName string `json:"secretName"`
	CertKey    string `json:"certKey"`
	KeyKey     string `json:"keyKey"`
	CAKey      string `json:"caKey"`
}

type RedisInstanceSpec struct {
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas"`
	RedisVersion string `json:"redisVersion"`
	Storage RedisPersistentVolumeClaimTemplate `json:"storage"`
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	Config map[string]string `json:"config,omitempty"`
	// +optional
	Auth *RedisAuthSpec `json:"auth,omitempty"`
	// +kubebuilder:validation:Enum=standalone;sentinel
	Topology string `json:"topology"`
}

type RedisInstanceStatus struct {
	// +optional
	Phase string `json:"phase,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// +optional
	MasterEndpoint string `json:"masterEndpoint,omitempty"`
	// +optional
	SentinelEndpoints []string `json:"sentinelEndpoints,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=redis
type RedisInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisInstanceSpec   `json:"spec,omitempty"`
	Status RedisInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type RedisInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisInstance{}, &RedisInstanceList{})
}
