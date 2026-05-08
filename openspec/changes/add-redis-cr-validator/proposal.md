# Proposal: Redis Custom Resource Validator

## Summary

Add a Kubernetes-focused command-line tool that validates Redis Custom Resource (CR) YAML files, checking all required fields for correctness before the CR is applied to a cluster.

## Problem

Teams deploying Redis via a custom Kubernetes operator often discover field errors only after applying the CR — at which point the operator may silently accept the resource or produce a confusing error. There is no lightweight pre-apply check that confirms required fields are present and well-formed.

## Solution

Implement a `redis-cr-validator` CLI tool that:

- Accepts a Redis CR YAML file path as input
- Validates the following required fields:
  - `apiVersion` — must be present
  - `kind` — must be present and equal to `"Redis"`
  - `metadata.name` — must be present
  - `spec.replicas` — must be present and a positive integer
  - `spec.storage.size` — must be present and a valid Kubernetes storage quantity (e.g. `1Gi`)
- Exits with code `0` on success and a non-zero code with clear error messages on failure
- Ships with valid and invalid example CR YAML files and a README describing installation and usage

## Scope

- **In scope**: validator tool source, unit tests, example CR YAML files, README usage documentation
- **Out of scope**: webhook admission controllers, CRD schema enforcement, operator changes, Helm charts

## Why Now

The project is adopting a Redis operator and needs a simple, portable validation step that can run in CI before manifests are applied, reducing broken-deploy cycles and reviewer burden.
