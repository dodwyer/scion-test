# Design: Redis Operator

## Architecture Overview

The Redis operator follows the Kubernetes controller pattern: a single controller watches `Redis` custom resources and reconciles the cluster state to match the declared spec.

```
User → Redis CR → Operator Controller → Owned Resources
                      ↓
              StatefulSet, Services,
              ConfigMap, Secret, PVC
```

## CRD Schema

**Group:** `redis.example.com`
**Kind:** `Redis`
**Version:** `v1alpha1`
**Scope:** Namespaced

### Spec Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `version` | string | yes | Redis image version (e.g. `7.2`) |
| `replicas` | int32 | no | Number of replicas (default: 1) |
| `resources` | ResourceRequirements | no | CPU/memory requests and limits |
| `persistence.enabled` | bool | no | Enable PVC-backed storage (default: false) |
| `persistence.storageClassName` | string | no | StorageClass name for PVC |
| `persistence.size` | string | no | PVC size (default: `1Gi`) |
| `auth.enabled` | bool | no | Enable password authentication (default: false) |
| `auth.secretName` | string | no | Name of existing Secret containing `redis-password` key |
| `config` | map[string]string | no | Additional redis.conf key-value overrides |

### Status Fields

| Field | Type | Description |
|---|---|---|
| `phase` | string | `Pending`, `Running`, `Degraded`, `Terminating` |
| `readyReplicas` | int32 | Number of ready replicas |
| `conditions` | []Condition | Standard Kubernetes condition list |

### Status Conditions

| Type | Meaning |
|---|---|
| `Available` | Redis instance is ready to serve traffic |
| `Reconciling` | Controller is actively reconciling |
| `Degraded` | Instance is unhealthy or reconciliation failed |

## Owned Resources

All resources are owned by the `Redis` CR via `ownerReferences` and are garbage-collected on CR deletion.

| Resource | Name Pattern | Purpose |
|---|---|---|
| `StatefulSet` | `<name>` | Redis pod(s) |
| `Service` (ClusterIP) | `<name>` | Client access endpoint |
| `Service` (Headless) | `<name>-headless` | Pod DNS for StatefulSet |
| `ConfigMap` | `<name>-config` | Generated `redis.conf` |
| `Secret` | `<name>-auth` | Auth password (if auth enabled and no secretName provided) |
| `PersistentVolumeClaim` | `data-<name>-0` | Data volume (if persistence enabled) |

## Reconciler Design

### Reconcile Loop

```
1. Fetch Redis CR; handle not-found (object deleted, exit)
2. Set Reconciling condition
3. Reconcile ConfigMap (create or update)
4. Reconcile Secret (if auth.enabled and no external secret)
5. Reconcile StatefulSet (create or update spec)
6. Reconcile Services (ClusterIP + Headless)
7. Reconcile PVC (if persistence.enabled)
8. Update status: readyReplicas, phase, conditions
9. Return result (requeue on error)
```

### Idempotency

All sub-reconcilers use `CreateOrUpdate` with a merge function. Multiple reconcile calls produce the same outcome.

### Deletion

Kubernetes owner references handle cascading deletion. No finalizer needed for basic resources. A finalizer may be added in future for pre-delete hooks (e.g. graceful shutdown).

## RBAC

The operator requires a `ClusterRole` (or `Role` for namespace-scoped) with these permissions:

- `redis.example.com/redises`: get, list, watch, create, update, patch, delete
- `redis.example.com/redises/status`: get, update, patch
- `redis.example.com/redises/finalizers`: update
- `apps/statefulsets`: get, list, watch, create, update, patch, delete
- `core/services`: get, list, watch, create, update, patch, delete
- `core/configmaps`: get, list, watch, create, update, patch, delete
- `core/secrets`: get, list, watch, create, update, patch, delete
- `core/persistentvolumeclaims`: get, list, watch, create, update, patch, delete
- `core/events`: create, patch

## Observability

- **Metrics**: Expose standard controller-runtime metrics (reconcile duration, error count, queue depth) via Prometheus endpoint on `:8080/metrics`
- **Logs**: Structured JSON logging via `logr` / `zap`
- **Events**: Kubernetes Events on the `Redis` CR for notable state transitions

## Framework

- Language: Go
- Framework: kubebuilder v3+ / controller-runtime v0.15+
- Manager: single `ctrl.Manager` with leader election enabled for HA deployments
- Generated code: CRD manifests via `controller-gen`

## Deployment

- Operator runs as a `Deployment` with 1 replica (leader election allows scaling to 2)
- Namespace: `redis-system` (recommended)
- Image: published to container registry

## Update Strategy

Redis StatefulSet uses `RollingUpdate` strategy. Version upgrades update the StatefulSet image; controller-runtime handles the rollout.
