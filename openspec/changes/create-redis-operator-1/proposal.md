# Proposal: Redis Kubernetes Operator

## Summary

Build a Kubernetes operator that manages Redis instances through the Kubernetes CRD/CR pattern. The operator will own the full lifecycle of Redis deployments: provisioning, configuration management, horizontal scaling, version upgrades, and teardown.

## Motivation

Running Redis on Kubernetes today requires manual management of StatefulSets, Services, ConfigMaps, and Secrets. Teams must implement their own logic for rolling restarts, scale operations, and health monitoring. This creates operational burden and drift between clusters.

A dedicated operator encapsulates this domain knowledge, provides a declarative API for Redis instances, and enforces consistent operational practices across all Redis deployments in the cluster.

## Goals

- Provide a `RedisInstance` Custom Resource Definition (CRD) under API group `redis.example.io/v1alpha1`
- Automate provisioning of StatefulSet, headless Service, ConfigMap, and optional Sentinel resources
- Support standalone and Sentinel topologies in v1; exclude Redis Cluster
- Implement safe rolling restarts triggered by config or version changes
- Support horizontal scaling via `spec.replicas`, where one writable primary is maintained and remaining replicas are read replicas
- Require PVC-backed persistent storage in v1
- Expose rich status via `status.phase`, `status.conditions`, `status.observedGeneration`, `status.readyReplicas`, `status.masterEndpoint`, and `status.sentinelEndpoints`
- Run the controller itself in HA mode using leader election

## Non-Goals

- Redis Enterprise or commercial Redis features
- Redis Modules (RediSearch, RedisJSON, etc.)
- Multi-cluster or cross-region federation
- Backup and restore (deferred pending stakeholder decision)
- Service mesh integration
- GUI or web dashboard
- Cloud-provider-specific integrations (e.g., AWS ElastiCache passthrough)

## Proposed Solution

Implement a Go-based Kubernetes operator using controller-runtime. The operator watches `RedisInstance` CR events and reconciles owned resources to match the desired state declared in the spec. V1 supports any Redis image registry as long as the referenced image is pullable by the cluster. Authentication is optional through a password Secret reference; if omitted, Redis runs without auth in v1. A finalizer (`redis.example.io/cleanup`) ensures ordered teardown and deletes owned PVCs by default. Leader election enables controller HA with two or more replicas on Kubernetes 1.26+.

## Stakeholders

- Platform Engineering (owner)
- Application teams consuming managed Redis instances
- Security team (RBAC and credential management review)

## Scope Decisions

- V1 supports standalone and Sentinel only; Redis Cluster is out of scope.
- `spec.replicas` means one writable primary plus `N-1` read replicas.
- PVC-backed persistent storage is mandatory in v1; `emptyDir` is not supported.
- Authentication is optional through `spec.auth.passwordSecretRef`; if omitted, Redis runs without auth.
- Finalizer cleanup deletes owned PVCs by default during teardown.
- Minimum supported Kubernetes version is 1.26.
- No Redis exporter sidecar is included in v1.
- Any image registry is allowed for the Redis container image.
