# Proposal: Redis Kubernetes Operator

## Problem Statement

Managing Redis on Kubernetes today requires manual, error-prone orchestration of multiple resources: StatefulSets for pod lifecycle, headless Services for stable DNS, ClusterIP Services for client access, ConfigMaps for Redis configuration, and Secrets for authentication. Operators must hand-craft YAML for each deployment, manually coordinate rolling updates, implement custom teardown logic to avoid data loss, and maintain consistency across all owned resources. There is no standard abstraction that encapsulates Redis cluster lifecycle as a single, declarative unit.

## Proposed Solution

Build a Kubernetes operator using the controller-runtime framework that introduces a `RedisInstance` Custom Resource Definition (CRD) under the API group `redis.example.io/v1alpha1`. Users declare their Redis topology in a single CR; the operator continuously reconciles the cluster to match the declared spec.

The operator will:
- Watch `RedisInstance` CR create, update, and delete events.
- Reconcile owned StatefulSets, headless Services, ClusterIP Services, ConfigMaps, and Secrets to match the declared spec.
- Reflect cluster health in CR `.status` conditions (`Ready`, `Degraded`).
- Use a finalizer (`redis.example.io/cleanup`) to ensure safe, ordered teardown of owned resources before the CR is deleted.

## Scope

### In scope (v1)
- **Single-instance (standalone) mode**: one Redis pod with a single PVC.
- **Sentinel HA mode**: multi-replica deployment with Redis Sentinel processes providing automatic failover.
- Replica scaling via CR `.spec.replicas` changes.
- Rolling updates when `.spec.image` or `.spec.resources` change.
- Auth integration via a referenced Kubernetes Secret.

### Out of scope (v1 non-goals)
- **Sharded Redis Cluster topology** (redis-cluster with slot-based sharding).
- **Backup and restore** operations or scheduled snapshots.
- **Cross-namespace replication** of Redis data.
- **Multi-region federation** or geo-distributed deployments.

## Success Criteria

1. The `RedisInstance` CRD installs cleanly into a Kubernetes cluster (v1.25+) with no errors.
2. Creating a `RedisInstance` CR results in a running, reachable Redis instance within the cluster.
3. Updating `.spec.replicas` triggers a safe scaling operation reflected in the StatefulSet and CR status.
4. Deleting a `RedisInstance` CR triggers the finalizer, which removes all owned resources before the CR is removed.
5. The CR `.status.conditions` accurately reflects `Ready` and `Degraded` states as the cluster health changes.
