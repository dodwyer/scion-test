# Spec: Redis CR Example Files

**File**: `examples/` (YAML example files)
**Change type**: ADDED

---

## ADDED Requirements

### Requirement: Valid Redis CR example

A file `examples/valid-redis-cr.yaml` MUST exist containing a Redis CR with all required fields set correctly.

#### Scenario: Valid example contains all required fields

**Given** the file `examples/valid-redis-cr.yaml` is read  
**When** the validator is run against it  
**Then** the validator MUST exit with code `0` and report no errors

#### Scenario: Valid example apiVersion field

**Given** the file `examples/valid-redis-cr.yaml`  
**Then** it MUST contain a non-empty `apiVersion` field

#### Scenario: Valid example kind field

**Given** the file `examples/valid-redis-cr.yaml`  
**Then** it MUST contain `kind: Redis`

#### Scenario: Valid example metadata.name field

**Given** the file `examples/valid-redis-cr.yaml`  
**Then** it MUST contain a non-empty `metadata.name` field

#### Scenario: Valid example spec.replicas field

**Given** the file `examples/valid-redis-cr.yaml`  
**Then** it MUST contain `spec.replicas` set to a positive integer

#### Scenario: Valid example spec.storage.size field

**Given** the file `examples/valid-redis-cr.yaml`  
**Then** it MUST contain `spec.storage.size` set to a valid Kubernetes storage quantity

---

### Requirement: Invalid example — missing metadata.name

A file `examples/invalid-missing-name.yaml` MUST exist containing a Redis CR that omits `metadata.name`.

#### Scenario: Validator reports missing name

**Given** the file `examples/invalid-missing-name.yaml` is read  
**When** the validator is run against it  
**Then** the validator MUST exit with a non-zero code  
**And** report an error referencing `metadata.name`

---

### Requirement: Invalid example — missing spec.replicas

A file `examples/invalid-missing-replicas.yaml` MUST exist containing a Redis CR that omits `spec.replicas`.

#### Scenario: Validator reports missing replicas

**Given** the file `examples/invalid-missing-replicas.yaml` is read  
**When** the validator is run against it  
**Then** the validator MUST exit with a non-zero code  
**And** report an error referencing `spec.replicas`

---

### Requirement: Invalid example — missing spec.storage.size

A file `examples/invalid-missing-storage.yaml` MUST exist containing a Redis CR that omits `spec.storage.size`.

#### Scenario: Validator reports missing storage size

**Given** the file `examples/invalid-missing-storage.yaml` is read  
**When** the validator is run against it  
**Then** the validator MUST exit with a non-zero code  
**And** report an error referencing `spec.storage.size`

---

### Requirement: Invalid example — wrong kind

A file `examples/invalid-wrong-kind.yaml` MUST exist containing a CR where `kind` is not `"Redis"`.

#### Scenario: Validator reports wrong kind

**Given** the file `examples/invalid-wrong-kind.yaml` is read  
**When** the validator is run against it  
**Then** the validator MUST exit with a non-zero code  
**And** report an error indicating `kind` must equal `"Redis"`
