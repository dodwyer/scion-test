# Proposal: Redis Kubernetes Operator

## Summary

Build a Kubernetes operator that manages Redis instances through the Kubernetes CRD/CR pattern. The operator will own the full lifecycle of Redis deployments: provisioning, configuration management, horizontal scaling, version upgrades, and teardown.

## Motivation

Running Redis on Kubernetes today requires manual management of StatefulSets, Services, ConfigMaps, and Secrets. Teams must implement their own logic for rolling restarts, scale operations, and health monitoring. This creates operational burden and drift between clusters.

A dedicated operator encapsulates this domain knowledge, provides a declarative API for Redis instances, and enforces consistent operational practices across all Redis deployments in the cluster.

## Goals

- Provide a `RedisInstance` Custom Resource Definition (CRD) under API group `redis.example.io/v1alpha1`
- Automate provisioning of StatefulSet, headless Service, ConfigMap, and optional Sentinel resources
- Support standalone and Sentinel topologies
- Implement safe rolling restarts triggered by config or version changes
- Support horizontal scaling via `spec.replicas` with scale-down draining
- Expose rich status via `status.phase`, `status.conditions`, `status.readyReplicas`, `status.masterEndpoint`, and `status.sentinelEndpoints`
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

Implement a Go-based Kubernetes operator using controller-runtime. The operator watches `RedisInstance` CR events and reconciles owned resources to match the desired state declared in the spec. A finalizer (`redis.example.io/cleanup`) ensures ordered teardown. Leader election enables controller HA with two or more replicas.

## Stakeholders

- Platform Engineering (owner)
- Application teams consuming managed Redis instances
- Security team (RBAC and credential management review)

## Open Questions

See `design.md` for the full list of open questions requiring stakeholder decisions before implementation begins.
