# scion-test

## Development Notes

This repository is a Scion test project.

### Inspecting the project locally

Clone the repository:

```bash
git clone https://github.com/dodwyer/scion-test.git
cd scion-test
```

Browse the file tree to explore the project structure:

```bash
ls -la
```

### Running

There is no build system or runtime in this project. To inspect or experiment locally, open the files directly in your editor of choice.

## Usage

### redis-cr-validator

A CLI tool that validates a Redis Custom Resource YAML file against required-field rules and reports all errors before exiting.

#### Build

```bash
go build -o redis-cr-validator ./cmd/redis-cr-validator/
```

#### Invoke

Validate a file by path:

```bash
./redis-cr-validator examples/valid-redis-cr.yaml
echo $?
# 0
```

Read from stdin with `-`:

```bash
cat examples/invalid-redis-cr.yaml | ./redis-cr-validator -
```

#### Exit codes

| Code | Meaning |
|------|---------|
| `0`  | YAML is valid; all fields pass |
| `1`  | One or more validation errors |
| `2`  | Parse or I/O error (file not found, malformed YAML) |

#### Example output

**Success** (`examples/valid-redis-cr.yaml`, exit 0):

```
(no output)
```

**Failure** (`examples/invalid-redis-cr.yaml`, exit 1):

```
ERROR: apiVersion: must equal "redis.example.com/v1alpha1"
ERROR: kind: must equal "Redis"
ERROR: metadata.name: must not be empty
ERROR: spec.replicas: must be a positive integer (got 0)
ERROR: spec.storage.size: must include a unit suffix (e.g. Gi, Mi, G, M)
```
