# Design: Redis Kubernetes Operator

## Architecture

The operator is a Go binary scaffolded with [Kubebuilder](https://book.kubebuilder.io/) and built on the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) library. It runs as a single Deployment inside the cluster and uses leader-election for high availability of the operator process itself.

```
┌─────────────────────────────────────────────────────┐
│  Operator Pod                                        │
│  ┌───────────────────┐   ┌────────────────────────┐ │
│  │  Manager           │   │  RedisInstance         │ │
│  │  (controller-      │──▶│  Reconciler            │ │
│  │   runtime)         │   │                        │ │
│  └───────────────────┘   └──────────┬─────────────┘ │
└─────────────────────────────────────┼───────────────┘
                                      │ reconciles
              ┌───────────────────────┼───────────────┐
              ▼               ▼       ▼       ▼       ▼
         StatefulSet    headless  ClusterIP ConfigMap Secret
                        Service   Service
```

### Framework Choice

The implementation uses **Kubebuilder**. It provides the required CRD/controller scaffolding while keeping the operator close to controller-runtime primitives and avoiding additional framework layers that are unnecessary for this operator.

## CRD: RedisInstance

**Group/Version/Kind**: `redis.example.io/v1alpha1 / RedisInstance`

### Spec Fields

| Field | Type | Description |
|---|---|---|
| `image` | string, required | Redis container image. Must reference Redis `7.2+`. |
| `replicas` | int32 | Number of Redis replicas. Default `1`. Valid range `1-6`. `standalone` requires `1`; `sentinel` requires an odd value `3` or `5`. |
| `resources` | ResourceRequirements | Optional CPU/memory requests and limits for the Redis container. |
| `storage.size` | string | Requested PVC size. Defaults to `1Gi`. |
| `storage.storageClassName` | string | Optional storage class name for replica PVCs. |
| `auth.secretName` | string | Optional Secret name in the same namespace. |
| `auth.passwordKey` | string | Secret data key containing the password. Defaults to `redis-password`. |
| `topology` | enum: `standalone` \| `sentinel` | Deployment topology. Defaults to `standalone`. |
| `version` | string | Redis major/minor version. Defaults to `7.2`. Minimum supported value is `7.2`. |

The effective validation rules are:
- `spec.image` is required and must point to a Redis 7.2+ image.
- `spec.replicas` defaults to `1` and must be between `1` and `6`.
- `spec.topology=standalone` requires `spec.replicas=1`.
- `spec.topology=sentinel` requires `spec.replicas` to be odd and at least `3`, which makes the valid values `3` and `5`.
- `spec.auth.passwordKey` defaults to `redis-password`, and if `spec.auth.secretName` is set the referenced Secret must contain a `redis-password` data entry unless the CR explicitly overrides `passwordKey`.

### Status Fields

| Field | Type | Description |
|---|---|---|
| `conditions` | []metav1.Condition | Standard Kubernetes conditions. Types: `Ready`, `Degraded`, each with standard `reason` and `message` fields. |
| `observedGeneration` | int64 | Last `.metadata.generation` processed by the reconciler. |
| `readyReplicas` | int32 | Number of StatefulSet replicas currently ready. |

Condition semantics are:
- `Ready=True` means all desired replicas are healthy and ready.
- `Degraded=True` means one or more desired replicas are unavailable.
- `Ready=True` and `Degraded=True` may coexist during partial degradation reporting windows while the operator reflects both overall service availability and loss of redundancy.

## Managed Resources

| Resource | Purpose |
|---|---|
| `StatefulSet` | Runs Redis pods with stable network identity and ordered scaling. |
| Headless `Service` | Provides stable DNS entries (`pod.svc.ns.svc.cluster.local`) for each pod. |
| ClusterIP `Service` | Stable virtual IP for client connections. In sentinel mode, its selector is updated to point to the current primary. |
| `ConfigMap` | Holds rendered `redis.conf` and Sentinel configuration derived from CR spec fields and topology. |
| PVCs (via `volumeClaimTemplates`) | Per-replica persistent data volumes; lifecycle owned by StatefulSet. |

All managed resources carry `ownerReference` pointing to the `RedisInstance` CR and the label set `app.kubernetes.io/managed-by: redis-operator`.

For authentication, the operator consumes credentials by setting environment variables in the pod spec using `secretKeyRef`. The Redis container receives its password from a Secret key named `redis-password` by default, with optional override through `spec.auth.passwordKey`.

## Reconciliation Loop

```
Event (CR create/update/delete)
        │
        ▼
1. Fetch RedisInstance CR
        │ (not found → already deleted, return)
        ▼
2. Check for deletion timestamp → run finalizer cleanup if set
        │
        ▼
3. Ensure finalizer is registered on CR
        │
        ▼
4. Render desired ConfigMap → apply (create or patch)
        │
        ▼
5. Render desired StatefulSet → apply (create or patch)
        │
        ▼
6. Render desired headless Service → apply
        │
        ▼
7. Render desired ClusterIP Service → apply
        │
        ▼
8. In sentinel topology, watch Sentinel-reported primary and update ClusterIP Service selector to target the current primary pod
        │
        ▼
9. Read StatefulSet status → compute Ready/Degraded conditions
        │
        ▼
10. Patch CR .status with conditions, readyReplicas, observedGeneration
        │
        ▼
Return (requeue on error or until stable)
```

## Finalizer

Finalizer name: `redis.example.io/cleanup`

On detection of a non-zero `deletionTimestamp` on the CR:
1. Delete the ClusterIP Service.
2. Delete the headless Service.
3. Delete the ConfigMap.
4. Delete the StatefulSet (pods terminated by Kubernetes).
5. Remove the finalizer from the CR, allowing the API server to complete deletion.

PVCs created via `volumeClaimTemplates` are **not** deleted automatically to prevent accidental data loss; cluster operators can delete them manually or via a separate retention policy.

## Sentinel Topology

When `spec.topology=sentinel`, each StatefulSet pod runs:
- a primary Redis container
- a co-located Sentinel sidecar container named `sentinel`

The Sentinel sidecar monitors the local Redis process and participates in quorum with peer sidecars. The operator watches Sentinel failover state and updates the client-facing ClusterIP Service selector so that it always targets the current primary pod after failover.

## Version Support

The minimum supported Redis version is **7.2**. The default CR value is `spec.version: "7.2"`, and `spec.image` must use a Redis 7.2+ image compatible with the rendered configuration.
