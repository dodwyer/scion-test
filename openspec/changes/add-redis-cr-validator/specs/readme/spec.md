# Spec: Redis CR Validator README

**File**: `README.md`
**Change type**: ADDED

---

## ADDED Requirements

### Requirement: README exists at project root

A `README.md` MUST exist that documents the Redis CR validator tool.

#### Scenario: README file is present

**Given** the repository root  
**Then** a file `README.md` MUST be present

---

### Requirement: Installation instructions

The README MUST contain an installation section explaining how to obtain or build the validator binary.

#### Scenario: Installation section exists

**Given** `README.md` is read  
**Then** it MUST contain a section heading for installation (e.g. `## Installation` or `## Getting Started`)

#### Scenario: Build or install command is shown

**Given** the installation section  
**Then** it MUST include at least one concrete command the user can run to install or build the tool  
(e.g. `go install`, `pip install`, `make build`, or a pre-built binary download command)

---

### Requirement: Basic usage instructions

The README MUST explain how to invoke the validator with a file argument.

#### Scenario: Usage section exists

**Given** `README.md` is read  
**Then** it MUST contain a section heading for usage (e.g. `## Usage`)

#### Scenario: Validate a file command shown

**Given** the usage section  
**Then** it MUST show a command demonstrating how to validate a Redis CR YAML file  
(e.g. `redis-cr-validator examples/valid-redis-cr.yaml`)

---

### Requirement: Exit code documentation

The README MUST document the meaning of exit codes returned by the tool.

#### Scenario: Exit code 0 documented

**Given** the README  
**Then** it MUST state that exit code `0` means validation passed

#### Scenario: Non-zero exit code documented

**Given** the README  
**Then** it MUST state that a non-zero exit code means one or more validation errors were found

---

### Requirement: Example commands using provided example files

The README MUST include example commands that reference the files in `examples/`.

#### Scenario: Valid example command shown

**Given** the README  
**Then** it MUST include a command that runs the validator against `examples/valid-redis-cr.yaml`

#### Scenario: Invalid example command shown

**Given** the README  
**Then** it MUST include at least one command that runs the validator against an invalid example file  
**And** show or describe the expected error output

---

### Requirement: No implementation code in README

The README MUST NOT contain implementation source code beyond short illustrative snippets in code blocks.

#### Scenario: README is documentation only

**Given** the README  
**Then** it MUST NOT duplicate or embed the full validator source code  
**And** MUST NOT reference internal implementation details that could become stale
