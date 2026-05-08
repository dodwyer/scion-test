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

Use the instructions below to build and run the included Redis CR validator. To inspect or experiment with other project files, open them directly in your editor of choice.

## Usage

Build the Redis CR validator:

```bash
go build -o bin/redis-cr-validator ./cmd/redis-cr-validator
```

Validate a Redis custom resource YAML file:

```bash
bin/redis-cr-validator examples/valid-redis-cr.yaml
echo $?
```

Successful validation prints no output and exits with code `0`.

Validate from stdin:

```bash
cat examples/valid-redis-cr.yaml | bin/redis-cr-validator -
```

Invalid resources print one error per invalid field to stderr and exit with code `1`:

```bash
bin/redis-cr-validator examples/invalid-redis-cr.yaml
```

Example failure output:

```text
ERROR: apiVersion: must equal "redis.example.com/v1alpha1"
ERROR: kind: must equal "Redis"
ERROR: metadata.name: must use lowercase alphanumeric characters and hyphens only; dots are not allowed
ERROR: spec.replicas: must be an integer
ERROR: spec.storage.size: must be greater than zero
```
