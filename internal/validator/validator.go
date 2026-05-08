package validator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	expectedAPIVersion = "redis.example.com/v1alpha1"
	expectedKind       = "Redis"
)

var simpleNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// RedisCR is the typed Redis custom resource representation used by validation.
type RedisCR struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   Metadata  `yaml:"metadata"`
	Spec       RedisSpec `yaml:"spec"`
	invalid    map[string]bool
}

type Metadata struct {
	Name string `yaml:"name"`
}

type RedisSpec struct {
	Replicas int          `yaml:"replicas"`
	Storage  RedisStorage `yaml:"storage"`
}

type RedisStorage struct {
	Size string `yaml:"size"`
}

// ValidationError describes a single field-level validation failure.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

// Parse decodes YAML into a RedisCR while preserving known scalar type errors
// as validation state for the later validation pass.
func Parse(data []byte) (RedisCR, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return RedisCR{}, err
	}

	cr := RedisCR{
		invalid: map[string]bool{},
	}
	if len(doc.Content) == 0 {
		return cr, nil
	}

	root := doc.Content[0]
	cr.APIVersion = stringField(root, "apiVersion", "apiVersion", cr.invalid)
	cr.Kind = stringField(root, "kind", "kind", cr.invalid)

	if metadata := mappingField(root, "metadata"); metadata != nil {
		cr.Metadata.Name = stringField(metadata, "name", "metadata.name", cr.invalid)
	}
	if spec := mappingField(root, "spec"); spec != nil {
		cr.Spec.Replicas = intField(spec, "replicas", "spec.replicas", cr.invalid)
		if storage := mappingField(spec, "storage"); storage != nil {
			cr.Spec.Storage.Size = scalarField(storage, "size", "spec.storage.size", cr.invalid)
		}
	}

	return cr, nil
}

// ParseAndValidate parses YAML and returns all validation errors for syntactically
// valid YAML. YAML syntax errors are returned separately.
func ParseAndValidate(data []byte) ([]ValidationError, error) {
	cr, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return Validate(cr), nil
}

// Validate collects all field validation errors for a RedisCR.
func Validate(cr RedisCR) []ValidationError {
	var errs []ValidationError
	errs = validateAPIVersion(cr, errs)
	errs = validateKind(cr, errs)
	errs = validateMetadataName(cr, errs)
	errs = validateReplicas(cr, errs)
	errs = validateStorageSize(cr, errs)
	return errs
}

func typeErrorReason(field string) string {
	switch field {
	case "spec.replicas":
		return "must be an integer"
	default:
		return "must be a string"
	}
}

func validateAPIVersion(cr RedisCR, errs []ValidationError) []ValidationError {
	if cr.invalid["apiVersion"] {
		return append(errs, ValidationError{Field: "apiVersion", Reason: typeErrorReason("apiVersion")})
	}
	if cr.APIVersion == "" {
		return append(errs, ValidationError{Field: "apiVersion", Reason: "must not be empty"})
	}
	if cr.APIVersion != expectedAPIVersion {
		return append(errs, ValidationError{Field: "apiVersion", Reason: `must equal "redis.example.com/v1alpha1"`})
	}
	return errs
}

func validateKind(cr RedisCR, errs []ValidationError) []ValidationError {
	if cr.invalid["kind"] {
		return append(errs, ValidationError{Field: "kind", Reason: typeErrorReason("kind")})
	}
	if cr.Kind == "" {
		return append(errs, ValidationError{Field: "kind", Reason: "must not be empty"})
	}
	if cr.Kind != expectedKind {
		return append(errs, ValidationError{Field: "kind", Reason: `must equal "Redis"`})
	}
	return errs
}

func validateMetadataName(cr RedisCR, errs []ValidationError) []ValidationError {
	if cr.invalid["metadata.name"] {
		return append(errs, ValidationError{Field: "metadata.name", Reason: typeErrorReason("metadata.name")})
	}
	name := cr.Metadata.Name
	if name == "" {
		return append(errs, ValidationError{Field: "metadata.name", Reason: "must not be empty"})
	}
	if len(name) > 253 {
		return append(errs, ValidationError{Field: "metadata.name", Reason: "must be no more than 253 characters"})
	}
	if strings.Contains(name, ".") {
		return append(errs, ValidationError{Field: "metadata.name", Reason: "must use lowercase alphanumeric characters and hyphens only; dots are not allowed"})
	}
	if !simpleNamePattern.MatchString(name) {
		return append(errs, ValidationError{Field: "metadata.name", Reason: "must use lowercase alphanumeric characters and hyphens only, with no leading or trailing hyphen"})
	}
	return errs
}

func validateReplicas(cr RedisCR, errs []ValidationError) []ValidationError {
	if cr.invalid["spec.replicas"] {
		return append(errs, ValidationError{Field: "spec.replicas", Reason: typeErrorReason("spec.replicas")})
	}
	if cr.Spec.Replicas < 1 {
		return append(errs, ValidationError{Field: "spec.replicas", Reason: fmt.Sprintf("must be a positive integer (got %d)", cr.Spec.Replicas)})
	}
	return errs
}

func validateStorageSize(cr RedisCR, errs []ValidationError) []ValidationError {
	if cr.invalid["spec.storage.size"] {
		return append(errs, ValidationError{Field: "spec.storage.size", Reason: typeErrorReason("spec.storage.size")})
	}
	size := cr.Spec.Storage.Size
	if size == "" {
		return append(errs, ValidationError{Field: "spec.storage.size", Reason: "must not be empty"})
	}
	if !hasUnitSuffix(size) {
		return append(errs, ValidationError{Field: "spec.storage.size", Reason: "must include a unit suffix"})
	}
	q, err := resource.ParseQuantity(size)
	if err != nil {
		return append(errs, ValidationError{Field: "spec.storage.size", Reason: "must be a valid Kubernetes resource quantity"})
	}
	if q.Sign() <= 0 {
		return append(errs, ValidationError{Field: "spec.storage.size", Reason: "must be greater than zero"})
	}
	return errs
}

func hasUnitSuffix(value string) bool {
	for i := len(value) - 1; i >= 0; i-- {
		if value[i] >= 'A' && value[i] <= 'Z' || value[i] >= 'a' && value[i] <= 'z' {
			return true
		}
		if value[i] != ' ' && value[i] != '\t' {
			return false
		}
	}
	return false
}

func stringField(root *yaml.Node, key, path string, invalid map[string]bool) string {
	node := mappingField(root, key)
	if node == nil {
		return ""
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		invalid[path] = true
		return ""
	}
	return node.Value
}

func scalarField(root *yaml.Node, key, path string, invalid map[string]bool) string {
	node := mappingField(root, key)
	if node == nil {
		return ""
	}
	if node.Kind != yaml.ScalarNode {
		invalid[path] = true
		return ""
	}
	return node.Value
}

func intField(root *yaml.Node, key, path string, invalid map[string]bool) int {
	node := mappingField(root, key)
	if node == nil {
		return 0
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
		invalid[path] = true
		return 0
	}
	value, err := strconv.Atoi(node.Value)
	if err != nil {
		invalid[path] = true
		return 0
	}
	return value
}

func mappingField(root *yaml.Node, key string) *yaml.Node {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}
