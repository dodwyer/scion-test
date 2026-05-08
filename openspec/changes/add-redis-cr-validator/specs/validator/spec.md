# Spec: Redis CR Validator Tool

**File**: `validator/` (source and test files)
**Change type**: ADDED

---

## ADDED Requirements

### Requirement: CLI accepts a YAML file argument

The validator MUST accept a single positional argument: the path to a Redis CR YAML file.

#### Scenario: Valid file path provided

**Given** the validator is invoked with a path to a readable YAML file  
**When** the file is parsed successfully  
**Then** validation proceeds and the tool exits with code `0` if all checks pass

#### Scenario: No argument provided

**Given** the validator is invoked with no arguments  
**When** the CLI parses arguments  
**Then** it MUST print a usage message to stderr and exit with a non-zero code

#### Scenario: File path does not exist

**Given** the validator is invoked with a path to a non-existent file  
**When** the tool attempts to open the file  
**Then** it MUST print an error message to stderr and exit with a non-zero code

---

### Requirement: apiVersion field validation

The Redis CR MUST contain a non-empty `apiVersion` field at the document root.

#### Scenario: apiVersion is present

**Given** a Redis CR YAML with `apiVersion: redis.example.com/v1`  
**When** the validator runs  
**Then** the `apiVersion` check MUST pass with no error

#### Scenario: apiVersion is absent

**Given** a Redis CR YAML that omits the `apiVersion` field  
**When** the validator runs  
**Then** it MUST report an error indicating `apiVersion` is required  
**And** exit with a non-zero code

---

### Requirement: kind field validation

The Redis CR MUST contain `kind: Redis` at the document root.

#### Scenario: kind is Redis

**Given** a Redis CR YAML with `kind: Redis`  
**When** the validator runs  
**Then** the `kind` check MUST pass with no error

#### Scenario: kind is absent

**Given** a Redis CR YAML that omits the `kind` field  
**When** the validator runs  
**Then** it MUST report an error indicating `kind` is required  
**And** exit with a non-zero code

#### Scenario: kind is a different value

**Given** a Redis CR YAML with `kind: Foo`  
**When** the validator runs  
**Then** it MUST report an error indicating `kind` must equal `"Redis"`  
**And** exit with a non-zero code

---

### Requirement: metadata.name field validation

The Redis CR MUST contain a non-empty `metadata.name` field.

#### Scenario: metadata.name is present

**Given** a Redis CR YAML with `metadata.name: my-redis`  
**When** the validator runs  
**Then** the `metadata.name` check MUST pass with no error

#### Scenario: metadata.name is absent

**Given** a Redis CR YAML that omits `metadata.name`  
**When** the validator runs  
**Then** it MUST report an error indicating `metadata.name` is required  
**And** exit with a non-zero code

---

### Requirement: spec.replicas field validation

The Redis CR MUST contain `spec.replicas` set to a positive integer (≥ 1).

#### Scenario: spec.replicas is a positive integer

**Given** a Redis CR YAML with `spec.replicas: 3`  
**When** the validator runs  
**Then** the `spec.replicas` check MUST pass with no error

#### Scenario: spec.replicas is absent

**Given** a Redis CR YAML that omits `spec.replicas`  
**When** the validator runs  
**Then** it MUST report an error indicating `spec.replicas` is required  
**And** exit with a non-zero code

#### Scenario: spec.replicas is zero

**Given** a Redis CR YAML with `spec.replicas: 0`  
**When** the validator runs  
**Then** it MUST report an error indicating `spec.replicas` must be a positive integer  
**And** exit with a non-zero code

#### Scenario: spec.replicas is negative

**Given** a Redis CR YAML with `spec.replicas: -1`  
**When** the validator runs  
**Then** it MUST report an error indicating `spec.replicas` must be a positive integer  
**And** exit with a non-zero code

---

### Requirement: spec.storage.size field validation

The Redis CR MUST contain `spec.storage.size` set to a valid Kubernetes storage quantity string (e.g. `1Gi`, `500Mi`, `2Ti`).

#### Scenario: spec.storage.size is a valid quantity

**Given** a Redis CR YAML with `spec.storage.size: 10Gi`  
**When** the validator runs  
**Then** the `spec.storage.size` check MUST pass with no error

#### Scenario: spec.storage.size is absent

**Given** a Redis CR YAML that omits `spec.storage.size`  
**When** the validator runs  
**Then** it MUST report an error indicating `spec.storage.size` is required  
**And** exit with a non-zero code

#### Scenario: spec.storage.size has an invalid format

**Given** a Redis CR YAML with `spec.storage.size: big`  
**When** the validator runs  
**Then** it MUST report an error indicating `spec.storage.size` must be a valid Kubernetes storage quantity  
**And** exit with a non-zero code

---

### Requirement: All errors reported together

When multiple fields fail validation, the tool MUST report all errors before exiting, not stop at the first failure.

#### Scenario: Multiple fields are invalid

**Given** a Redis CR YAML missing both `metadata.name` and `spec.replicas`  
**When** the validator runs  
**Then** it MUST report errors for both missing fields in the same run  
**And** exit with a non-zero code

---

### Requirement: Unit test coverage

A unit test file MUST exist alongside the validator source and cover every validation rule.

#### Scenario: Tests pass for valid CR

**Given** a complete, correct Redis CR passed to the validation logic  
**When** the test suite runs  
**Then** all tests for the valid case MUST pass

#### Scenario: Tests pass for each invalid case

**Given** one test per missing or invalid field  
**When** the test suite runs  
**Then** each test MUST assert the correct error is returned for that field
