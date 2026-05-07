package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TopologyStandalone = "standalone"
	TopologySentinel   = "sentinel"
)

type RedisAuthSpec struct {
	PasswordSecretRef *corev1.SecretKeySelector `json:"passwordSecretRef,omitempty"`
	TLS               *RedisTLSSpec             `json:"tls,omitempty"`
}

type RedisTLSSpec struct {
	CertSecretRef *corev1.SecretKeySelector `json:"certSecretRef,omitempty"`
	KeySecretRef  *corev1.SecretKeySelector `json:"keySecretRef,omitempty"`
	CASecretRef   *corev1.SecretKeySelector `json:"caSecretRef,omitempty"`
}

type RedisStorageSpec struct {
	// +kubebuilder:validation:MinItems=1
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes"`
	Resources   corev1.VolumeResourceRequirements   `json:"resources"`
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// RedisInstanceSpec defines the desired state of RedisInstance.
type RedisInstanceSpec struct {
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas"`
	// +kubebuilder:validation:MinLength=1
	RedisVersion string           `json:"redisVersion"`
	Storage      RedisStorageSpec `json:"storage"`
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	Config map[string]string `json:"config,omitempty"`
	// +optional
	Auth *RedisAuthSpec `json:"auth,omitempty"`
	// +kubebuilder:validation:Enum=standalone;sentinel
	Topology string `json:"topology"`
}

// RedisInstanceStatus defines the observed state of RedisInstance.
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

func (in *RedisAuthSpec) DeepCopyInto(out *RedisAuthSpec) {
	*out = *in
	if in.PasswordSecretRef != nil {
		out.PasswordSecretRef = in.PasswordSecretRef.DeepCopy()
	}
	if in.TLS != nil {
		out.TLS = new(RedisTLSSpec)
		in.TLS.DeepCopyInto(out.TLS)
	}
}

func (in *RedisTLSSpec) DeepCopyInto(out *RedisTLSSpec) {
	*out = *in
	if in.CertSecretRef != nil {
		out.CertSecretRef = in.CertSecretRef.DeepCopy()
	}
	if in.KeySecretRef != nil {
		out.KeySecretRef = in.KeySecretRef.DeepCopy()
	}
	if in.CASecretRef != nil {
		out.CASecretRef = in.CASecretRef.DeepCopy()
	}
}

func (in *RedisStorageSpec) DeepCopyInto(out *RedisStorageSpec) {
	*out = *in
	if in.AccessModes != nil {
		out.AccessModes = make([]corev1.PersistentVolumeAccessMode, len(in.AccessModes))
		copy(out.AccessModes, in.AccessModes)
	}
	in.Resources.DeepCopyInto(&out.Resources)
	if in.StorageClassName != nil {
		out.StorageClassName = new(string)
		*out.StorageClassName = *in.StorageClassName
	}
}

func (in *RedisInstanceSpec) DeepCopyInto(out *RedisInstanceSpec) {
	*out = *in
	in.Storage.DeepCopyInto(&out.Storage)
	in.Resources.DeepCopyInto(&out.Resources)
	if in.Config != nil {
		out.Config = make(map[string]string, len(in.Config))
		for key, value := range in.Config {
			out.Config[key] = value
		}
	}
	if in.Auth != nil {
		out.Auth = new(RedisAuthSpec)
		in.Auth.DeepCopyInto(out.Auth)
	}
}

func (in *RedisInstanceStatus) DeepCopyInto(out *RedisInstanceStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
	if in.SentinelEndpoints != nil {
		out.SentinelEndpoints = make([]string, len(in.SentinelEndpoints))
		copy(out.SentinelEndpoints, in.SentinelEndpoints)
	}
}

func init() {
	SchemeBuilder.Register(&RedisInstance{}, &RedisInstanceList{})
}
