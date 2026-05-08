package validator

import (
	"strings"
	"testing"
)

func validCR() RedisCR {
	return RedisCR{
		APIVersion: "redis.example.com/v1alpha1",
		Kind:       "Redis",
		Metadata:   Metadata{Name: "my-redis-cluster"},
		Spec:       Spec{Replicas: 3, Storage: Storage{Size: "1Gi"}},
	}
}

// --- Validate() tests (typed struct) ---

func TestValidate_Valid(t *testing.T) {
	errs := Validate(validCR())
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidate_InvalidAPIVersion(t *testing.T) {
	cr := validCR()
	cr.APIVersion = "apps/v1"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "apiVersion" {
		t.Errorf("expected one apiVersion error, got %v", errs)
	}
}

func TestValidate_MissingAPIVersion(t *testing.T) {
	cr := validCR()
	cr.APIVersion = ""
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "apiVersion" {
		t.Errorf("expected one apiVersion error for empty value, got %v", errs)
	}
}

func TestValidate_InvalidKind(t *testing.T) {
	cr := validCR()
	cr.Kind = "redis"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "kind" {
		t.Errorf("expected one kind error, got %v", errs)
	}
}

func TestValidate_KindWrongCase(t *testing.T) {
	cr := validCR()
	cr.Kind = "REDIS"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "kind" {
		t.Errorf("expected one kind error for REDIS, got %v", errs)
	}
}

func TestValidate_MissingKind(t *testing.T) {
	cr := validCR()
	cr.Kind = ""
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "kind" {
		t.Errorf("expected one kind error for empty value, got %v", errs)
	}
}

func TestValidate_EmptyName(t *testing.T) {
	cr := validCR()
	cr.Metadata.Name = ""
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "metadata.name" {
		t.Errorf("expected one metadata.name error, got %v", errs)
	}
	if !strings.Contains(errs[0].Message, "empty") {
		t.Errorf("expected 'empty' in message, got %q", errs[0].Message)
	}
}

func TestValidate_NameWithUppercase(t *testing.T) {
	cr := validCR()
	cr.Metadata.Name = "MyRedis"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "metadata.name" {
		t.Errorf("expected one metadata.name error for uppercase, got %v", errs)
	}
}

func TestValidate_NameWithLeadingHyphen(t *testing.T) {
	cr := validCR()
	cr.Metadata.Name = "-my-redis"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "metadata.name" {
		t.Errorf("expected one metadata.name error for leading hyphen, got %v", errs)
	}
}

func TestValidate_NameWithDot(t *testing.T) {
	cr := validCR()
	cr.Metadata.Name = "my.redis"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "metadata.name" {
		t.Errorf("expected one metadata.name error for dot, got %v", errs)
	}
	if !strings.Contains(errs[0].Message, "dot") {
		t.Errorf("expected 'dot' in message, got %q", errs[0].Message)
	}
}

func TestValidate_NameTooLong(t *testing.T) {
	cr := validCR()
	cr.Metadata.Name = strings.Repeat("a", 254)
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "metadata.name" {
		t.Errorf("expected one metadata.name error for length, got %v", errs)
	}
}

func TestValidate_NameMaxLength(t *testing.T) {
	cr := validCR()
	cr.Metadata.Name = strings.Repeat("a", 253)
	errs := Validate(cr)
	if len(errs) != 0 {
		t.Errorf("expected no errors for 253-char name, got %v", errs)
	}
}

func TestValidate_ZeroReplicas(t *testing.T) {
	cr := validCR()
	cr.Spec.Replicas = 0
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "spec.replicas" {
		t.Errorf("expected one spec.replicas error, got %v", errs)
	}
	if !strings.Contains(errs[0].Message, "0") {
		t.Errorf("expected '0' in message, got %q", errs[0].Message)
	}
}

func TestValidate_NegativeReplicas(t *testing.T) {
	cr := validCR()
	cr.Spec.Replicas = -1
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "spec.replicas" {
		t.Errorf("expected one spec.replicas error, got %v", errs)
	}
}

func TestValidate_EmptyStorageSize(t *testing.T) {
	cr := validCR()
	cr.Spec.Storage.Size = ""
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "spec.storage.size" {
		t.Errorf("expected one spec.storage.size error, got %v", errs)
	}
}

func TestValidate_StorageSizePlainInteger(t *testing.T) {
	cr := validCR()
	cr.Spec.Storage.Size = "1024"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "spec.storage.size" {
		t.Errorf("expected one spec.storage.size error for plain integer, got %v", errs)
	}
	if !strings.Contains(errs[0].Message, "unit") {
		t.Errorf("expected 'unit' in message, got %q", errs[0].Message)
	}
}

func TestValidate_StorageSizeZero(t *testing.T) {
	cr := validCR()
	cr.Spec.Storage.Size = "0Gi"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "spec.storage.size" {
		t.Errorf("expected one spec.storage.size error for zero value, got %v", errs)
	}
	if !strings.Contains(errs[0].Message, "greater than zero") {
		t.Errorf("expected 'greater than zero' in message, got %q", errs[0].Message)
	}
}

func TestValidate_StorageSizeInvalid(t *testing.T) {
	cr := validCR()
	cr.Spec.Storage.Size = "notasize"
	errs := Validate(cr)
	if len(errs) != 1 || errs[0].Field != "spec.storage.size" {
		t.Errorf("expected one spec.storage.size error for invalid quantity, got %v", errs)
	}
}

func TestValidate_StorageSizeBinarySuffix(t *testing.T) {
	cr := validCR()
	cr.Spec.Storage.Size = "500Mi"
	errs := Validate(cr)
	if len(errs) != 0 {
		t.Errorf("expected no errors for 500Mi, got %v", errs)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cr := RedisCR{
		APIVersion: "wrong/version",
		Kind:       "redis",
		Metadata:   Metadata{Name: ""},
		Spec:       Spec{Replicas: 0, Storage: Storage{Size: "1024"}},
	}
	errs := Validate(cr)
	if len(errs) != 5 {
		t.Errorf("expected 5 errors, got %d: %v", len(errs), errs)
	}
}

// --- ParseAndValidate() tests (two-stage parsing) ---

func TestParseAndValidate_Valid(t *testing.T) {
	yaml := `
apiVersion: redis.example.com/v1alpha1
kind: Redis
metadata:
  name: my-redis-cluster
spec:
  replicas: 3
  storage:
    size: 1Gi
`
	errs, err := ParseAndValidate([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestParseAndValidate_NonNumericReplicas(t *testing.T) {
	yaml := `
apiVersion: redis.example.com/v1alpha1
kind: Redis
metadata:
  name: my-redis
spec:
  replicas: "three"
  storage:
    size: 1Gi
`
	errs, err := ParseAndValidate([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(errs) != 1 || errs[0].Field != "spec.replicas" {
		t.Errorf("expected one spec.replicas error, got %v", errs)
	}
}

func TestParseAndValidate_ScalarTypeErrorWithSemanticError(t *testing.T) {
	yaml := `
apiVersion: redis.example.com/v1alpha1
kind: redis
metadata:
  name: my-redis
spec:
  replicas: "three"
  storage:
    size: 1Gi
`
	errs, err := ParseAndValidate([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	fields := make(map[string]bool)
	for _, e := range errs {
		fields[e.Field] = true
	}
	if !fields["spec.replicas"] {
		t.Errorf("expected spec.replicas error, got %v", errs)
	}
	if !fields["kind"] {
		t.Errorf("expected kind error, got %v", errs)
	}
}

func TestParseAndValidate_MalformedYAML(t *testing.T) {
	bad := []byte(":\t:\tbad yaml{{{")
	_, err := ParseAndValidate(bad)
	if err == nil {
		t.Error("expected parse error for malformed YAML, got nil")
	}
}

func TestParseAndValidate_ErrorFormat(t *testing.T) {
	yaml := `
apiVersion: wrong
kind: Redis
metadata:
  name: my-redis
spec:
  replicas: 1
  storage:
    size: 1Gi
`
	errs, err := ParseAndValidate([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	want := `ERROR: apiVersion: must equal "redis.example.com/v1alpha1"`
	if errs[0].Error() != want {
		t.Errorf("error format mismatch\ngot:  %q\nwant: %q", errs[0].Error(), want)
	}
}
