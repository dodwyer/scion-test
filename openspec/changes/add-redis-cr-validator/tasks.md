# Tasks: Redis Custom Resource Validator

## Implementation

- [ ] Create `validator/` directory and CLI entry point source file
- [ ] Implement YAML file loading and parsing to a generic map
- [ ] Implement `apiVersion` presence validator
- [ ] Implement `kind` presence and value (`"Redis"`) validator
- [ ] Implement `metadata.name` presence validator
- [ ] Implement `spec.replicas` presence and positive-integer validator
- [ ] Implement `spec.storage.size` presence and Kubernetes quantity format validator
- [ ] Collect all validation errors and report them together before exiting
- [ ] Return exit code `0` on success, non-zero on any validation failure
- [ ] Create unit test file covering the valid CR case
- [ ] Create unit test for missing `apiVersion`
- [ ] Create unit test for missing `kind`
- [ ] Create unit test for `kind` not equal to `"Redis"`
- [ ] Create unit test for missing `metadata.name`
- [ ] Create unit test for missing `spec.replicas`
- [ ] Create unit test for `spec.replicas` set to zero or negative
- [ ] Create unit test for missing `spec.storage.size`
- [ ] Create unit test for `spec.storage.size` with invalid format
- [ ] Create `examples/valid-redis-cr.yaml` with all required fields correct
- [ ] Create `examples/invalid-missing-name.yaml` omitting `metadata.name`
- [ ] Create `examples/invalid-missing-replicas.yaml` omitting `spec.replicas`
- [ ] Create `examples/invalid-missing-storage.yaml` omitting `spec.storage.size`
- [ ] Create `examples/invalid-wrong-kind.yaml` with `kind: Foo` instead of `kind: Redis`
- [ ] Write `README.md` with installation, usage, and example commands

## Review

- [ ] Confirm all required field validators are covered by at least one test
- [ ] Confirm each example YAML file matches its described invalid scenario
- [ ] Confirm README commands match the actual CLI interface
- [ ] Confirm exit codes are documented in README
- [ ] Confirm no code is merged without passing unit tests
