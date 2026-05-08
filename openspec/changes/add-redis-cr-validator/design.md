# Design: Redis CR Validator

## Approach

Implement a Go CLI binary that parses a Redis CR YAML file using the standard `gopkg.in/yaml.v3` library (or equivalent). The validator uses two-stage parsing: first decode into a flexible intermediate representation to inspect known field types and collect scalar type errors as validation errors, then validate field values and optionally convert into typed structs for implementation convenience. All errors are collected before reporting so that a single run surfaces every problem. Output goes to stderr; the binary exits with a code that reflects the outcome category.

### Field Validation Rules

| Field | Rule |
|-------|------|
| `apiVersion` | Must equal `redis.example.com/v1alpha1` (exact string match) |
| `kind` | Must equal `Redis` (case-sensitive exact match) |
| `metadata.name` | Must be present and non-empty; must conform to Kubernetes simple name format: lowercase alphanumeric and hyphens only, no dots, no leading/trailing hyphens, max 253 characters |
| `spec.replicas` | Must be a positive integer ≥ 1; 0, negatives, floats, and non-numeric strings are invalid validation errors |
| `spec.storage.size` | Must be a valid Kubernetes resource quantity string > 0 (e.g. `1Gi`, `500Mi`, `10G`); plain integers without a unit are invalid |

### Error Output Format

```
ERROR: <field.path>: <human-readable reason>
```

Example:

```
ERROR: metadata.name: must not be empty
ERROR: spec.replicas: must be a positive integer (got 0)
```

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | YAML is valid; all fields pass |
| `1` | One or more validation errors |
| `2` | Parse or I/O error (file not found, syntactically malformed YAML) |

## Files Affected

| File | Change |
|------|--------|
| `cmd/redis-cr-validator/main.go` | New — CLI entry point, argument parsing, exit-code logic |
| `internal/validator/validator.go` | New — field validation functions |
| `internal/validator/validator_test.go` | New — unit tests for valid case and each invalid-field scenario |
| `examples/valid-redis-cr.yaml` | New — example valid Redis CR YAML |
| `examples/invalid-redis-cr.yaml` | New — example invalid Redis CR YAML with multiple errors |
| `README.md` | Modified — add usage section for the validator |

## Implementation Steps

1. Scaffold the Go module (`go.mod`) if not already present.
2. Define a flexible parsed representation for initial YAML decoding and, if useful, a `RedisCR` struct with YAML tags for correctly typed values.
3. Implement validation so type errors for known scalar fields and semantic errors are collected together; it returns all errors and never stops early.
4. Implement each field rule as a private helper; add the Kubernetes resource quantity parser (use `k8s.io/apimachinery/pkg/api/resource` or a minimal inline parser).
5. Write `cmd/redis-cr-validator/main.go`: parse args, read file or stdin, decode YAML into the flexible representation, validate, print errors to stderr, exit with appropriate code.
6. Add unit tests in `internal/validator/validator_test.go` covering: valid CR, each of the five fields individually invalid, and multiple simultaneous errors.
7. Create `examples/valid-redis-cr.yaml` and `examples/invalid-redis-cr.yaml`.
8. Append a **Usage** section to `README.md` showing install, basic invocation, and example output.

## Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Kubernetes resource quantity parsing complexity | Medium | Use `k8s.io/apimachinery` quantity package directly rather than reimplementing |
| Name regex edge cases | Low | Use Kubernetes simple name semantics with a focused regex; cover uppercase, leading/trailing hyphen, dots, and max length in tests |
| stdin piping in CI environments | Low | Test with `echo "..." \| redis-cr-validator -` in example docs |
