# Spec: README Development Notes

**File**: `README.md`
**Change type**: MODIFIED

---

## MODIFIED Requirements

### Requirement: Development Notes Section Present

`README.md` MUST contain a `## Development Notes` section below the existing title.

#### Scenario: Section heading exists

**Given** the file `README.md` is read  
**Then** it MUST contain the line `## Development Notes`

#### Scenario: Clone instructions present

**Given** the `## Development Notes` section  
**Then** it MUST include a `git clone` command referencing the repository URL

#### Scenario: Local inspection instructions present

**Given** the `## Development Notes` section  
**Then** it MUST describe how a developer can inspect or browse the project locally

#### Scenario: No code or runtime added

**Given** this change  
**Then** no source code, dependency files, Kubernetes manifests, or generated files are introduced or modified
