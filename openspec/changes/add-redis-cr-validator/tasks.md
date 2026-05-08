# Tasks: Add Redis CR Validator

## Implementation

- [x] Scaffold Go module and directory structure (`cmd/redis-cr-validator/`, `internal/validator/`, `examples/`)
- [x] Define `RedisCR` struct with YAML tags in `internal/validator/validator.go`
- [x] Implement `Validate(cr RedisCR) []ValidationError` collecting all errors before returning
- [x] Implement `apiVersion` exact-match validation helper
- [x] Implement `kind` exact-match validation helper
- [x] Implement `metadata.name` presence and DNS subdomain format validation helper
- [x] Implement `spec.replicas` positive integer (≥ 1) validation helper
- [x] Implement `spec.storage.size` Kubernetes resource quantity (> 0) validation helper
- [x] Write `cmd/redis-cr-validator/main.go` with file/stdin argument parsing and exit-code logic
- [x] Create `examples/valid-redis-cr.yaml`
- [x] Create `examples/invalid-redis-cr.yaml` with multiple deliberate errors
- [x] Write unit tests in `internal/validator/validator_test.go` covering valid CR and each invalid-field scenario
- [x] Append **Usage** section to `README.md`

## Review

- [ ] Confirm all five field validation rules match the spec
- [ ] Confirm exit codes 0 / 1 / 2 are returned correctly in each scenario
- [ ] Confirm error messages include field paths in `ERROR: <field.path>: <reason>` format
- [ ] Confirm unit tests cover the valid case and every invalid-field scenario individually
- [ ] Confirm example YAMLs are syntactically valid and illustrate the documented rules
- [ ] Confirm README usage section shows install, invocation, and example output
- [ ] Confirm no admission webhook, CRD schema, or runtime cluster code was introduced
