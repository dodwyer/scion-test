package validator

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ValidationError represents a single field validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("ERROR: %s: %s", e.Field, e.Message)
}

// RedisCR is the typed representation of a Redis Custom Resource.
type RedisCR struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata holds CR metadata fields.
type Metadata struct {
	Name string `yaml:"name"`
}

// Spec holds spec fields.
type Spec struct {
	Replicas int     `yaml:"replicas"`
	Storage  Storage `yaml:"storage"`
}

// Storage holds storage configuration.
type Storage struct {
	Size string `yaml:"size"`
}

// rawCR is the flexible intermediate representation used for two-stage parsing.
type rawCR struct {
	APIVersion interface{} `yaml:"apiVersion"`
	Kind       interface{} `yaml:"kind"`
	Metadata   struct {
		Name interface{} `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Replicas interface{} `yaml:"replicas"`
		Storage  struct {
			Size interface{} `yaml:"size"`
		} `yaml:"storage"`
	} `yaml:"spec"`
}

var nameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]*[a-z0-9])?$`)

// Validate validates a typed RedisCR and returns all validation errors.
func Validate(cr RedisCR) []ValidationError {
	var errs []ValidationError
	errs = append(errs, validateAPIVersion(cr.APIVersion)...)
	errs = append(errs, validateKind(cr.Kind)...)
	errs = append(errs, validateName(cr.Metadata.Name)...)
	errs = append(errs, validateReplicas(cr.Spec.Replicas)...)
	errs = append(errs, validateStorageSize(cr.Spec.Storage.Size)...)
	return errs
}

// ParseAndValidate decodes YAML using two-stage parsing and validates all fields.
// Type errors for known scalar fields are reported as field-path validation errors
// (exit 1), not parse errors (exit 2). Returns (nil, error) only on malformed YAML
// or I/O failure.
func ParseAndValidate(data []byte) ([]ValidationError, error) {
	var raw rawCR
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var errs []ValidationError
	var cr RedisCR

	// Extract string-typed fields; non-string scalars are coerced for semantic validation.
	cr.APIVersion = extractString(raw.APIVersion)
	cr.Kind = extractString(raw.Kind)
	cr.Metadata.Name = extractString(raw.Metadata.Name)
	cr.Spec.Storage.Size = extractString(raw.Spec.Storage.Size)

	// Extract spec.replicas with explicit type checking so "three" → validation error.
	skipReplicas := false
	switch v := raw.Spec.Replicas.(type) {
	case int:
		cr.Spec.Replicas = v
	case nil:
		// zero value; validateReplicas will catch it
	default:
		errs = append(errs, ValidationError{
			Field:   "spec.replicas",
			Message: fmt.Sprintf("must be an integer (got %v)", v),
		})
		skipReplicas = true
	}

	// Semantic validation.
	errs = append(errs, validateAPIVersion(cr.APIVersion)...)
	errs = append(errs, validateKind(cr.Kind)...)
	errs = append(errs, validateName(cr.Metadata.Name)...)
	if !skipReplicas {
		errs = append(errs, validateReplicas(cr.Spec.Replicas)...)
	}
	errs = append(errs, validateStorageSize(cr.Spec.Storage.Size)...)

	return errs, nil
}

func extractString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func validateAPIVersion(v string) []ValidationError {
	if v != "redis.example.com/v1alpha1" {
		return []ValidationError{{
			Field:   "apiVersion",
			Message: `must equal "redis.example.com/v1alpha1"`,
		}}
	}
	return nil
}

func validateKind(v string) []ValidationError {
	if v != "Redis" {
		return []ValidationError{{
			Field:   "kind",
			Message: `must equal "Redis"`,
		}}
	}
	return nil
}

func validateName(name string) []ValidationError {
	if name == "" {
		return []ValidationError{{Field: "metadata.name", Message: "must not be empty"}}
	}
	if len(name) > 253 {
		return []ValidationError{{
			Field:   "metadata.name",
			Message: fmt.Sprintf("must be 253 characters or fewer (got %d)", len(name)),
		}}
	}
	if strings.Contains(name, ".") {
		return []ValidationError{{
			Field:   "metadata.name",
			Message: "must not contain dots",
		}}
	}
	if !nameRegex.MatchString(name) {
		return []ValidationError{{
			Field:   "metadata.name",
			Message: "must consist of lowercase alphanumeric characters and hyphens only, with no leading or trailing hyphens",
		}}
	}
	return nil
}

func validateReplicas(n int) []ValidationError {
	if n < 1 {
		return []ValidationError{{
			Field:   "spec.replicas",
			Message: fmt.Sprintf("must be a positive integer (got %d)", n),
		}}
	}
	return nil
}

func validateStorageSize(size string) []ValidationError {
	if size == "" {
		return []ValidationError{{Field: "spec.storage.size", Message: "must not be empty"}}
	}
	if isPlainInteger(size) {
		return []ValidationError{{
			Field:   "spec.storage.size",
			Message: "must include a unit suffix (e.g. Gi, Mi, G, M)",
		}}
	}
	q, err := resource.ParseQuantity(size)
	if err != nil {
		return []ValidationError{{
			Field:   "spec.storage.size",
			Message: fmt.Sprintf("must be a valid Kubernetes resource quantity (e.g. 1Gi, 500Mi): %v", err),
		}}
	}
	if q.IsZero() {
		return []ValidationError{{
			Field:   "spec.storage.size",
			Message: "must be greater than zero",
		}}
	}
	return nil
}

// isPlainInteger reports whether s is a non-empty string of decimal digits only.
func isPlainInteger(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
