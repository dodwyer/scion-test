package validator

import (
	"strings"
	"testing"
)

const validYAML = `apiVersion: redis.example.com/v1alpha1
kind: Redis
metadata:
  name: my-redis-cluster
spec:
  replicas: 3
  storage:
    size: 1Gi
`

func TestParseAndValidateValidCR(t *testing.T) {
	errs := validateYAML(t, validYAML)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestParseAndValidateInvalidAPIVersion(t *testing.T) {
	yaml := strings.Replace(validYAML, "redis.example.com/v1alpha1", "redis.example.com/v1", 1)
	assertSingleFieldError(t, yaml, "apiVersion")
}

func TestParseAndValidateInvalidKind(t *testing.T) {
	yaml := strings.Replace(validYAML, "kind: Redis", "kind: redis", 1)
	assertSingleFieldError(t, yaml, "kind")
}

func TestParseAndValidateInvalidMetadataName(t *testing.T) {
	yaml := strings.Replace(validYAML, "name: my-redis-cluster", "name: MyRedis", 1)
	assertSingleFieldError(t, yaml, "metadata.name")
}

func TestParseAndValidateInvalidReplicas(t *testing.T) {
	yaml := strings.Replace(validYAML, "replicas: 3", "replicas: 0", 1)
	assertSingleFieldError(t, yaml, "spec.replicas")
}

func TestParseAndValidateInvalidStorageSize(t *testing.T) {
	yaml := strings.Replace(validYAML, "size: 1Gi", "size: 1024", 1)
	assertSingleFieldError(t, yaml, "spec.storage.size")
}

func TestParseAndValidateMultipleErrors(t *testing.T) {
	yaml := `apiVersion: redis.example.com/v1
kind: redis
metadata:
  name: my.redis
spec:
  replicas: "three"
  storage:
    size: 0Gi
`

	errs := validateYAML(t, yaml)
	if len(errs) != 5 {
		t.Fatalf("expected 5 errors, got %d: %v", len(errs), errs)
	}
	wantFields := []string{"apiVersion", "kind", "metadata.name", "spec.replicas", "spec.storage.size"}
	for i, want := range wantFields {
		if errs[i].Field != want {
			t.Fatalf("error %d field = %q, want %q; all errors: %v", i, errs[i].Field, want, errs)
		}
	}
}

func TestParseAndValidateScalarTypeErrorIsValidationError(t *testing.T) {
	yaml := strings.Replace(validYAML, "replicas: 3", `replicas: "three"`, 1)
	errs := validateYAML(t, yaml)
	if len(errs) != 1 {
		t.Fatalf("expected one validation error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "spec.replicas" || !strings.Contains(errs[0].Reason, "integer") {
		t.Fatalf("expected integer validation error for spec.replicas, got %v", errs[0])
	}
}

func assertSingleFieldError(t *testing.T, yaml string, field string) {
	t.Helper()
	errs := validateYAML(t, yaml)
	if len(errs) != 1 {
		t.Fatalf("expected one error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != field {
		t.Fatalf("error field = %q, want %q", errs[0].Field, field)
	}
}

func validateYAML(t *testing.T, yaml string) []ValidationError {
	t.Helper()
	errs, err := ParseAndValidate([]byte(yaml))
	if err != nil {
		t.Fatalf("expected validation result, got parse error: %v", err)
	}
	return errs
}
