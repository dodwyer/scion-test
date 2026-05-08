# Design: Redis Custom Resource Validator

## Approach

Implement a single-binary CLI tool (`redis-cr-validator`) that reads a Redis CR YAML file, parses it, and validates the presence and correctness of required fields. The tool reports all validation errors at once (rather than failing on the first), prints human-readable messages to stderr, and exits with a non-zero status code when any validation fails.

Validation rules:

| Field | Rule |
|-------|------|
| `apiVersion` | Must be present (non-empty string) |
| `kind` | Must be present and equal to `"Redis"` |
| `metadata.name` | Must be present (non-empty string) |
| `spec.replicas` | Must be present and a positive integer (≥ 1) |
| `spec.storage.size` | Must be present and match a Kubernetes storage quantity pattern (e.g. `1Gi`, `500Mi`, `2Ti`) |

## Files Affected

| File | Change |
|------|--------|
| `validator/main.go` (or `validator/validator.py`) | New — CLI entry point and validation logic |
| `validator/validator_test.go` (or `validator/test_validator.py`) | New — unit tests for each validation rule |
| `examples/valid-redis-cr.yaml` | New — example Redis CR with all required fields set correctly |
| `examples/invalid-missing-name.yaml` | New — example CR missing `metadata.name` |
| `examples/invalid-missing-replicas.yaml` | New — example CR missing `spec.replicas` |
| `examples/invalid-missing-storage.yaml` | New — example CR missing `spec.storage.size` |
| `examples/invalid-wrong-kind.yaml` | New — example CR with `kind` not equal to `"Redis"` |
| `README.md` | New — installation, usage, and example commands |

## Implementation Steps

1. Create the `validator/` directory with the CLI source file.
2. Implement YAML parsing to a generic map structure (avoid coupling to a generated CRD type).
3. Implement each field validator as a discrete function returning an error or nil.
4. Implement the Kubernetes storage quantity regex: `^[0-9]+(Ki|Mi|Gi|Ti|Pi|Ei|m|k|M|G|T|P|E)?$`.
5. Collect all errors and print them; exit non-zero if any errors were found.
6. Create unit tests covering: valid CR, each missing-field case, wrong `kind`, invalid storage size, non-positive replicas.
7. Create the `examples/` directory and write YAML files for valid and each invalid scenario.
8. Write `README.md` with installation steps, CLI usage, and example commands.

## Risks

- The storage quantity regex may not cover all edge cases in the full Kubernetes resource.Quantity spec. Document the subset supported and link to upstream spec.
- Tool language choice (Go vs Python) is left to the implementer; this spec is language-agnostic.
