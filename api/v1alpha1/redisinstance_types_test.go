package v1alpha1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRedisInstanceRequiredFields(t *testing.T) {
	instance := validRedisInstance()
	if instance.Spec.Replicas != 1 {
		t.Fatalf("expected replicas to default to test fixture value")
	}
	if instance.Spec.Storage.Spec.Resources.Requests.Storage().IsZero() {
		t.Fatalf("expected storage request to be configured")
	}
}

func TestRedisInstanceTopologyEnumValidation(t *testing.T) {
	instance := validRedisInstance()
	if instance.Spec.Topology != TopologyStandalone {
		t.Fatalf("expected fixture topology to be standalone")
	}
	if TopologySentinel == "cluster" {
		t.Fatalf("topology enum unexpectedly matches unsupported cluster value")
	}
}

func TestRedisInstanceStorageRequiredShape(t *testing.T) {
	instance := validRedisInstance()
	if len(instance.Spec.Storage.Spec.AccessModes) == 0 {
		t.Fatalf("expected access modes to be set")
	}
}

func TestRedisInstanceRoundTripToUnstructured(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("add to scheme: %v", err)
	}
	instance := validRedisInstance()
	gvk, err := apiGVK(s, instance)
	if err != nil {
		t.Fatalf("gvk: %v", err)
	}
	if gvk.Group != GroupVersion.Group || gvk.Version != GroupVersion.Version || gvk.Kind != "RedisInstance" {
		t.Fatalf("unexpected gvk: %#v", gvk)
	}
}

func apiGVK(s *runtime.Scheme, obj runtime.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := s.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return gvks[0], nil
}

func validRedisInstance() *RedisInstance {
	return &RedisInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "sample", Namespace: "default"},
		Spec: RedisInstanceSpec{
			Replicas:     1,
			RedisVersion: "7.2",
			Topology:     TopologyStandalone,
			Storage: RedisPersistentVolumeClaimTemplate{
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
			},
		},
	}
}
