// Package v1alpha1 contains API Schema definitions for the redis.example.io v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=redis.example.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion = schema.GroupVersion{Group: "redis.example.io", Version: "v1alpha1"}

	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)
