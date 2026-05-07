# Design: Redis Kubernetes Operator

## Architecture

The operator is a Go binary built on the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) library. It runs as a single Deployment inside the cluster and uses leader-election for high availability of the operator process itself.

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

### Framework Choice (Open Question)

Candidate frameworks: **Operator SDK** (higher-level scaffolding, Helm/Ansible support, OLM integration) vs **Kubebuilder** (closer to controller-runtime primitives, lighter dependency footprint). Decision to be made before implementation begins. Both produce compatible controller-runtime-based binaries.

## CRD: RedisInstance

**Group/Version/Kind**: `redis.example.io/v1alpha1 / RedisInstance`

### Spec Fields

| Field | Type | Description |
|---|---|---|
| `image` | string | Redis container image (e.g. `redis:7.2`). |
| `replicas` | int32 | Number of Redis replicas. Must be 1 (standalone) or ≥ 3 odd number (sentinel). |
| `resources` | ResourceRequirements | CPU/memory requests and limits for the Redis container. |
| `storage` | StorageSpec | PVC size and storageClassName for each replica's data volume. |
| `auth.secretRef` | LocalObjectReference | Name of a Secret in the same namespace containing `redis-password`. |
| `topology` | enum: `standalone` \| `sentinel` | Deployment topology. Defaults to `standalone`. |

### Status Fields

| Field | Type | Description |
|---|---|---|
| `conditions` | []metav1.Condition | Standard Kubernetes conditions. Types: `Ready`, `Degraded`. |
| `observedGeneration` | int64 | Last `.metadata.generation` processed by the reconciler. |
| `readyReplicas` | int32 | Number of StatefulSet replicas currently ready. |

## Managed Resources

| Resource | Purpose |
|---|---|
| `StatefulSet` | Runs Redis pods with stable network identity and ordered scaling. |
| Headless `Service` | Provides stable DNS entries (`pod.svc.ns.svc.cluster.local`) for each pod. |
| ClusterIP `Service` | Stable virtual IP for client connections. Points to primary replica. |
| `ConfigMap` | Holds `redis.conf` rendered from CR spec fields. |
| PVCs (via `volumeClaimTemplates`) | Per-replica persistent data volumes; lifecycle owned by StatefulSet. |

All managed resources carry `ownerReference` pointing to the `RedisInstance` CR and the label set `app.kubernetes.io/managed-by: redis-operator`.

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
8. Read StatefulSet status → compute Ready/Degraded conditions
        │
        ▼
9. Patch CR .status with conditions, readyReplicas, observedGeneration
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

## Open Questions

1. **Framework**: Operator SDK vs Kubebuilder — to be resolved before scaffolding.
2. **Redis version support floor**: Minimum Redis version to target (e.g. 6.x vs 7.x) affects config syntax.
3. **Auth model**: Secret injection as env var (`REDIS_PASSWORD`) vs volume-mounted file; affects hot-reload behavior.
4. **Sentinel topology details**: Whether to co-locate Sentinel processes in the same pod or run separate Sentinel StatefulSet.
