# Proposal: Redis Kubernetes Operator

## Summary

Introduce a Redis Operator for Kubernetes that enables declarative management of Redis instances using the standard Custom Resource Definition (CRD) / Custom Resource (CR) pattern. Operators of Redis instances will define their desired state in a `Redis` CR; the operator's controller loop will reconcile that state continuously.

## Motivation

Running Redis on Kubernetes today requires manual management of StatefulSets, headless Services, ConfigMaps, and PersistentVolumeClaims. There is no first-class Kubernetes API for Redis, so teams either write ad-hoc Helm templates or rely on third-party charts that cannot adapt to custom operational requirements.

A purpose-built operator solves this by:

1. **Extending the Kubernetes API** — `Redis` becomes a native resource `kubectl` users can create, inspect, and delete like any other object.
2. **Encoding operational knowledge** — rolling upgrades, readiness gating, and status reporting are baked into the controller rather than left to human operators.
3. **Standardising configuration** — a validated CRD schema prevents misconfigurations that a raw Helm `values.yaml` cannot catch.

## Goals

- Provide a `Redis` CRD supporting standalone and basic master-replica topologies.
- Implement a controller (reconcile loop) that owns StatefulSets, Services, ConfigMaps, and optionally PersistentVolumeClaims.
- Expose a `.status` subresource reflecting `Ready` / `Degraded` health.
- Ship operator deployment manifests (Helm chart preferred, Kustomize overlay as alternative).
- Support Redis 6.x and 7.x container images.

## Non-Goals (v1)

- Redis Sentinel or Redis Cluster (advanced HA / sharded topology beyond basic master-replica).
- Backup and restore workflows.
- TLS certificate provisioning.
- Cross-namespace Redis instances.
- Monitoring / alerting integrations (Prometheus ServiceMonitor may be added in a follow-on).

## Open Questions

The following questions remain unresolved and must be decided before implementation begins. The design doc records each option; the chosen answer should be recorded in `design.md` before coding starts.

| # | Question | Options |
|---|----------|---------|
| 1 | Operator toolchain | kubebuilder scaffolding, controller-runtime directly, operator-sdk |
| 2 | Persistence requirement | PVC-backed persistence required in v1, or optional |
| 3 | Deployment packaging | Helm chart, Kustomize, or raw manifests |
| 4 | Testing strategy | envtest unit tests, full integration tests, or both |
| 5 | Redis version support | 6.x only, 7.x only, or both |

## Stakeholders

- Platform / infrastructure teams consuming the operator.
- Application teams deploying Redis-backed services.
- Security / compliance teams reviewing RBAC and network exposure.
