# Proposal: Add Redis CR Validator CLI

## Summary

Introduce a standalone **Redis CR Validator** CLI tool (written in Go) that validates a Redis Custom Resource YAML file against required-field rules, producing structured error output and a non-zero exit code on failure.

## Problem

Kubernetes operators and platform teams that manage Redis Custom Resources (CRs) today have no lightweight, offline way to verify that a CR YAML is well-formed before applying it to a cluster. Invalid CRs—missing required fields, malformed names, or incorrectly typed values—are only caught at admission time, causing slow feedback loops and obscure error messages in CI pipelines.

## Solution

Build a small Go CLI (`redis-cr-validator`) that:

- Accepts a Redis CR YAML file path as an argument (or reads from stdin)
- Validates five required fields: `apiVersion`, `kind`, `metadata.name`, `spec.replicas`, and `spec.storage.size`
- Reports **all** validation errors (not fail-fast) to stderr, one per line, each prefixed with the offending field path
- Returns exit code `0` (valid), `1` (validation errors), or `2` (parse/IO error)

The tool targets offline/pre-apply use—suitable for CI pipelines and local development—and is **not** an admission webhook.

## Scope

### In Scope

- Go CLI binary (`redis-cr-validator`)
- Validation logic for all five required fields (apiVersion, kind, metadata.name, spec.replicas, spec.storage.size)
- Structured stderr error output with field paths; non-zero exit codes
- Unit tests covering the valid case and each invalid-field scenario
- Valid and invalid example YAML fixtures
- README usage section

### Out of Scope

- Kubernetes admission webhook integration
- CRD schema generation or server-side validation
- Validation of fields beyond the five specified
- Helm chart or Kustomize packaging
- Multi-cluster or remote cluster support

## Why Now

The project is adding Redis operator support. Having a validator in place before the first CR is applied to production reduces the risk of misconfigured resources and enables CI enforcement from day one. The scope is small enough to deliver quickly while providing immediate value.
