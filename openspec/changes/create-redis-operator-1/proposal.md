# Proposal: Redis Operator for Kubernetes

## Change

`create-redis-operator-1`

## Goal

Build a Kubernetes operator that manages Redis instances using the standard CRD/CR pattern. Users declare a `Redis` custom resource; the operator reconciles cluster state to match.

## Motivation

Deploying and managing Redis on Kubernetes requires manual creation of StatefulSets, Services, ConfigMaps, Secrets, and PersistentVolumeClaims. An operator encapsulates this complexity, provides lifecycle management, and exposes a simple declarative API.

## Scope

**In scope:**
- `Redis` Custom Resource Definition (CRD) schema (`redis.example.com/v1alpha1`)
- Controller reconciler: create, update, delete lifecycle
- Owned resource management: StatefulSet, Services, ConfigMap, Secret, PVC
- Status conditions and observability
- RBAC specification
- Optional persistence and optional authentication

**Out of scope:**
- Redis Sentinel or Cluster topology (future scope)
- Backup and restore
- Multi-cluster federation
- Redis configuration tuning beyond basic parameters

## Assumptions

- Kubernetes 1.24+
- kubebuilder / controller-runtime framework (Go)
- Initial CRD version: `v1alpha1`
- Standalone Redis (single instance) as baseline
- Namespace-scoped operator
