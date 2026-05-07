package v1alpha1

import (
	"os"
	"path/filepath"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestCRDSchemaRequiresStorageAndCoreSpecFields(t *testing.T) {
	crd := loadRedisInstanceCRD(t)
	specSchema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]

	required := make(map[string]struct{}, len(specSchema.Required))
	for _, field := range specSchema.Required {
		required[field] = struct{}{}
	}

	for _, field := range []string{"replicas", "redisVersion", "storage", "topology"} {
		if _, ok := required[field]; !ok {
			t.Fatalf("expected spec.%s to be required", field)
		}
	}
}

func TestCRDSchemaRestrictsTopologyEnum(t *testing.T) {
	crd := loadRedisInstanceCRD(t)
	topologySchema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["topology"]

	if len(topologySchema.Enum) != 2 {
		t.Fatalf("expected 2 topology enum values, got %d", len(topologySchema.Enum))
	}

	values := map[string]struct{}{}
	for _, enumValue := range topologySchema.Enum {
		values[string(enumValue.Raw)] = struct{}{}
	}

	for _, expected := range []string{`"standalone"`, `"sentinel"`} {
		if _, ok := values[expected]; !ok {
			t.Fatalf("expected topology enum %s", expected)
		}
	}
}

func TestCRDStatusIncludesObservedGeneration(t *testing.T) {
	crd := loadRedisInstanceCRD(t)
	statusProps := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties

	field, ok := statusProps["observedGeneration"]
	if !ok {
		t.Fatal("expected status.observedGeneration in schema")
	}
	if field.Type != "integer" {
		t.Fatalf("expected observedGeneration to be integer, got %q", field.Type)
	}
}

func loadRedisInstanceCRD(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "bases", "redis.example.io_redisinstances.yaml"))
	if err != nil {
		t.Fatalf("read CRD: %v", err)
	}

	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(contents, &crd); err != nil {
		t.Fatalf("unmarshal CRD: %v", err)
	}

	if len(crd.Spec.Versions) == 0 {
		t.Fatal("expected CRD versions")
	}

	return crd
}
