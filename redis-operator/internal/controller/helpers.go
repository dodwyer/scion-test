package controller

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/types"
)

// namespacedName is a convenience constructor for types.NamespacedName.
func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

// podLabels returns the standard label set for Redis or Sentinel pods.
func podLabels(instanceName, component string) map[string]string {
	return map[string]string{
		labelInstanceName: instanceName,
		labelManagedBy:    operatorName,
		labelComponent:    component,
	}
}

// resourceLabels returns the standard label set for owned non-pod resources.
// Uses the same structure as podLabels for consistency.
func resourceLabels(instanceName, component string) map[string]string {
	return podLabels(instanceName, component)
}

// ordinalFromPodName extracts the StatefulSet ordinal from a pod name.
// Pod names follow the pattern "<stsName>-<ordinal>".
func ordinalFromPodName(podName, stsName string) int32 {
	suffix := strings.TrimPrefix(podName, stsName+"-")
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return -1
	}
	return int32(n)
}

// primaryDNS returns the stable DNS name for Redis pod-0 (the initial primary).
func primaryDNS(instanceName, namespace string) string {
	return fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", instanceName, instanceName, namespace)
}

// sentinelPodDNS returns the stable DNS name for a Sentinel pod.
func sentinelPodDNS(instanceName, namespace string, ordinal int32) string {
	return fmt.Sprintf("%s-sentinel-%d.%s-sentinel.%s.svc.cluster.local",
		instanceName, ordinal, instanceName, namespace)
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }
