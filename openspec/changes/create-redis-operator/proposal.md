# Proposal: Redis Kubernetes Operator

## Summary

Build a production-grade Redis Kubernetes operator that manages the full lifecycle of Redis instances via custom resources. The operator follows the standard Kubernetes CRD/CR pattern and integrates with native Kubernetes primitives (StatefulSets, Services, ConfigMaps, Secrets, PodDisruptionBudgets) to deliver reliable, self-healing Redis deployments.

## Problem Statement

Running Redis on Kubernetes today requires operators to manually manage StatefulSets, Services, ConfigMaps, and health checks. There is no declarative API that encodes Redis-specific operational knowledge (rolling upgrades, failover sequencing, replication topology, configuration hot-reload) into the cluster. As a result, teams repeatedly build fragile, bespoke automation or accept operational toil.

## Proposed Solution

A Kubernetes operator (controller-runtime, Go) that:

1. Defines a `Redis` CRD in the `redis.example.io/v1alpha1` API group.
2. Watches `Redis` custom resources and reconciles cluster state to match the declared spec.
3. Supports standalone, Sentinel, and Cluster modes through a single resource kind.
4. Exposes rich `.status` fields and Kubernetes-standard conditions so platform tooling can observe and act on Redis health.

## Goals

- Declarative management of Redis topology, configuration, and resources.
- Zero-downtime rolling upgrades gated on readiness probes.
- Automatic failover coordination in Sentinel mode.
- Fine-grained RBAC so the operator runs with least-privilege.
- Validation via CEL admission rules on the CRD.
- Conformance with the Kubernetes Operator pattern (level 3+).

## Non-Goals

- Cross-cluster federation or multi-region replication.
- Backup/restore automation (deferred to a separate change).
- Managed-cloud Redis wrappers (AWS ElastiCache, GCP Memorystore).
- A Helm chart or OLM bundle (packaging is a follow-on change).

## Success Criteria

- A `Redis` CR transitions from `Pending` to `Ready` within two minutes on a standard cluster.
- Rolling upgrades complete without dropped connections on a correctly configured client.
- The operator passes the upstream `operator-sdk scorecard` at level 3.
- Unit test coverage ≥ 80 %; integration tests cover all major reconciliation paths.

## Stakeholders

| Role | Party |
|---|---|
| Author | Platform Engineering |
| Reviewer | SRE Lead, Security Team |
| Consumer | Application teams running Redis workloads |
