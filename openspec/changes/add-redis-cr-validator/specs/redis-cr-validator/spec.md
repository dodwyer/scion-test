# Spec: Redis CR Validator

**File**: `cmd/redis-cr-validator/main.go`, `internal/validator/validator.go`
**Change type**: ADDED

---

## ADDED Requirements

### Requirement: CLI Accepts Input

The validator MUST accept a Redis CR YAML file path as a positional argument. It MUST also support reading from stdin when invoked with `-` as the argument or when no argument is provided and stdin is not a terminal.

#### Scenario: File path argument

**Given** the binary is invoked as `redis-cr-validator path/to/redis-cr.yaml`  
**When** the file exists and is readable  
**Then** the tool MUST parse and validate the file contents

#### Scenario: Stdin input

**Given** the binary is invoked as `cat redis-cr.yaml | redis-cr-validator -`  
**When** stdin contains a YAML document  
**Then** the tool MUST parse and validate the stdin contents

#### Scenario: File not found

**Given** the binary is invoked with a path that does not exist  
**Then** the tool MUST print an error to stderr and exit with code `2`

---

### Requirement: apiVersion Validation

The `apiVersion` field MUST equal the exact string `redis.example.com/v1alpha1`.

#### Scenario: Correct apiVersion

**Given** a Redis CR YAML where `apiVersion` is `redis.example.com/v1alpha1`  
**When** the validator runs  
**Then** no error for `apiVersion` is emitted

#### Scenario: Wrong apiVersion

**Given** a Redis CR YAML where `apiVersion` is any value other than `redis.example.com/v1alpha1`  
**When** the validator runs  
**Then** the validator MUST emit `ERROR: apiVersion: must equal "redis.example.com/v1alpha1"`

#### Scenario: Missing apiVersion

**Given** a Redis CR YAML with no `apiVersion` key  
**When** the validator runs  
**Then** the validator MUST emit an error for `apiVersion` referencing the missing or empty value

---

### Requirement: kind Validation

The `kind` field MUST equal the exact string `Redis` (case-sensitive).

#### Scenario: Correct kind

**Given** a Redis CR YAML where `kind` is `Redis`  
**When** the validator runs  
**Then** no error for `kind` is emitted

#### Scenario: Wrong kind (wrong case)

**Given** a Redis CR YAML where `kind` is `redis` or `REDIS`  
**When** the validator runs  
**Then** the validator MUST emit `ERROR: kind: must equal "Redis"`

#### Scenario: Missing kind

**Given** a Redis CR YAML with no `kind` key  
**When** the validator runs  
**Then** the validator MUST emit an error for `kind` referencing the missing or empty value

---

### Requirement: metadata.name Validation

The `metadata.name` field MUST be present, non-empty, and conform to DNS subdomain format as defined by RFC 1123: lowercase alphanumeric characters or hyphens, no leading or trailing hyphens, maximum 253 characters.

#### Scenario: Valid name

**Given** a Redis CR YAML where `metadata.name` is `my-redis-cluster`  
**When** the validator runs  
**Then** no error for `metadata.name` is emitted

#### Scenario: Empty name

**Given** a Redis CR YAML where `metadata.name` is an empty string  
**When** the validator runs  
**Then** the validator MUST emit `ERROR: metadata.name: must not be empty`

#### Scenario: Name with uppercase letters

**Given** a Redis CR YAML where `metadata.name` is `MyRedis`  
**When** the validator runs  
**Then** the validator MUST emit an error for `metadata.name` citing the DNS subdomain format violation

#### Scenario: Name with leading hyphen

**Given** a Redis CR YAML where `metadata.name` is `-my-redis`  
**When** the validator runs  
**Then** the validator MUST emit an error for `metadata.name` citing the DNS subdomain format violation

#### Scenario: Name exceeds 253 characters

**Given** a Redis CR YAML where `metadata.name` is a string of 254 or more characters  
**When** the validator runs  
**Then** the validator MUST emit an error for `metadata.name` citing the maximum length violation

---

### Requirement: spec.replicas Validation

The `spec.replicas` field MUST be a positive integer with a value of at least `1`. Zero, negative integers, floats, and non-numeric strings are all invalid.

#### Scenario: Valid replicas

**Given** a Redis CR YAML where `spec.replicas` is `3`  
**When** the validator runs  
**Then** no error for `spec.replicas` is emitted

#### Scenario: Zero replicas

**Given** a Redis CR YAML where `spec.replicas` is `0`  
**When** the validator runs  
**Then** the validator MUST emit `ERROR: spec.replicas: must be a positive integer (got 0)`

#### Scenario: Negative replicas

**Given** a Redis CR YAML where `spec.replicas` is `-1`  
**When** the validator runs  
**Then** the validator MUST emit an error for `spec.replicas` citing the positive integer requirement

#### Scenario: Non-numeric replicas

**Given** a Redis CR YAML where `spec.replicas` is `"three"`  
**When** the validator runs  
**Then** the validator MUST emit an error for `spec.replicas` citing the integer type requirement

---

### Requirement: spec.storage.size Validation

The `spec.storage.size` field MUST be a valid Kubernetes resource quantity string representing a value greater than zero. Accepted formats include suffixes such as `Gi`, `Mi`, `G`, `M`, `Ki`. Plain integers without a unit suffix are invalid.

#### Scenario: Valid size with binary suffix

**Given** a Redis CR YAML where `spec.storage.size` is `1Gi`  
**When** the validator runs  
**Then** no error for `spec.storage.size` is emitted

#### Scenario: Valid size with decimal suffix

**Given** a Redis CR YAML where `spec.storage.size` is `500Mi`  
**When** the validator runs  
**Then** no error for `spec.storage.size` is emitted

#### Scenario: Plain integer without unit

**Given** a Redis CR YAML where `spec.storage.size` is `1024`  
**When** the validator runs  
**Then** the validator MUST emit an error for `spec.storage.size` citing that a unit suffix is required

#### Scenario: Zero quantity

**Given** a Redis CR YAML where `spec.storage.size` is `0Gi`  
**When** the validator runs  
**Then** the validator MUST emit an error for `spec.storage.size` citing that the value must be greater than zero

#### Scenario: Missing size

**Given** a Redis CR YAML with no `spec.storage.size` key  
**When** the validator runs  
**Then** the validator MUST emit an error for `spec.storage.size` referencing the missing or empty value

---

### Requirement: All-Errors Reporting

The validator MUST report **all** validation errors in a single run. It MUST NOT stop after the first error encountered (no fail-fast behavior).

#### Scenario: Multiple invalid fields

**Given** a Redis CR YAML with three invalid fields  
**When** the validator runs  
**Then** the validator MUST emit exactly three `ERROR:` lines to stderr, one per invalid field

#### Scenario: Error line format

**Given** any validation error  
**Then** each error line MUST match the format `ERROR: <field.path>: <human-readable reason>`

---

### Requirement: Exit Codes

The validator MUST exit with a well-defined code that reflects the outcome category.

#### Scenario: Valid CR

**Given** a Redis CR YAML that passes all validation rules  
**When** the validator runs  
**Then** it MUST exit with code `0` and produce no output to stderr

#### Scenario: Validation errors present

**Given** a Redis CR YAML with one or more validation errors  
**When** the validator runs  
**Then** it MUST exit with code `1`

#### Scenario: Parse or I/O error

**Given** a file path that does not exist, or a YAML document that cannot be parsed into the expected structure  
**When** the validator runs  
**Then** it MUST exit with code `2`

---

### Requirement: Example Fixtures

The repository MUST include at least one valid and one invalid example Redis CR YAML file to aid users and serve as test references.

#### Scenario: Valid example passes validation

**Given** the file `examples/valid-redis-cr.yaml` is passed to the validator  
**When** the validator runs  
**Then** it MUST exit with code `0`

#### Scenario: Invalid example fails validation

**Given** the file `examples/invalid-redis-cr.yaml` is passed to the validator  
**When** the validator runs  
**Then** it MUST exit with code `1` and emit at least one `ERROR:` line

---

### Requirement: Unit Test Coverage

The validator package MUST include unit tests that cover the valid case and each of the five invalid-field scenarios individually.

#### Scenario: Valid CR test

**Given** a unit test constructing a fully valid `RedisCR` struct  
**When** `Validate` is called  
**Then** the returned error slice MUST be empty

#### Scenario: Each invalid-field test

**Given** a unit test constructing a `RedisCR` with exactly one invalid field  
**When** `Validate` is called  
**Then** the returned error slice MUST contain exactly one entry referencing the correct field path

---

### Requirement: README Usage Section

`README.md` MUST contain a **Usage** section describing how to build and run the validator, including an example invocation and sample output.

#### Scenario: Usage section present

**Given** the file `README.md` is read  
**Then** it MUST contain a section headed `## Usage` (or equivalent) for `redis-cr-validator`

#### Scenario: Example invocation shown

**Given** the Usage section  
**Then** it MUST show at least one shell command demonstrating how to invoke the validator against a YAML file

#### Scenario: Example output shown

**Given** the Usage section  
**Then** it MUST show example output illustrating both the success (exit 0) and failure (exit 1) cases
