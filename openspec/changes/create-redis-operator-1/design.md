# Design: create-redis-operator-1

## Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                  │
│                                                      │
│  ┌────────────────┐       Watch/Reconcile            │
│  │  Operator Pod  │◄──────────────────────────────┐  │
│  │  (controller-  │                               │  │
│  │   runtime)     │                               │  │
│  └───────┬────────┘                               │  │
│          │ Owns / manages                         │  │
│          ▼                                        │  │
│  ┌───────────────────────────────────────────┐   │  │
│  │  Redis CR (redis.example.com/v1alpha1)    │───┘  │
│  └───────────────────────────────────────────┘      │
│          │ Reconciles into                           │
│          ▼                                           │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────┐  │
│  │  StatefulSet │  │  ConfigMap   │  │  Secret   │  │
│  │  (Redis pods)│  │  (redis.conf)│  │  (auth)   │  │
│  └──────────────┘  └──────────────┘  └───────────┘  │
│  ┌──────────────┐  ┌──────────────┐                  │
│  │  ClusterIP   │  │  Headless    │                  │
│  │  Service     │  │  Service     │                  │
│  └──────────────┘  └──────────────┘                  │
└─────────────────────────────────────────────────────┘
```

The operator runs as a single `Deployment` in the `redis-system` namespace (with leader election enabling multiple replicas for HA). It watches `Redis` custom resources and drives the owned Kubernetes resources toward the desired state on every reconcile. The manager can be configured at deploy time to watch all namespaces or a single namespace via controller-runtime manager options, so the Deployment namespace and the watch scope are distinct concerns.

## Framework Choices

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go | idiomatic controller ecosystem |
| SDK | kubebuilder v3+ / controller-runtime v0.15+ | code generation, CRD scaffolding |
| CRD validation | OpenAPI v3 schema (embedded in CRD) | native Kubernetes validation |
| Leader election | controller-runtime built-in (Lease resource) | zero extra dependencies |
| Metrics | `controller-runtime` default Prometheus registry | standard operator metrics + custom |
| Logging | `logr` + `zap` sink | structured JSON logs |

## CRD Schema

### `spec` Fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `version` | string | Yes | — | Redis image tag (e.g., `"7.2"`) |
| `replicas` | integer | No | `1` | Number of Redis pods (standalone baseline) |
| `resources.requests.cpu` | string | No | `"100m"` | CPU request for each Redis pod |
| `resources.requests.memory` | string | No | `"128Mi"` | Memory request for each Redis pod |
| `resources.limits.cpu` | string | No | `"500m"` | CPU limit for each Redis pod |
| `resources.limits.memory` | string | No | `"512Mi"` | Memory limit for each Redis pod |
| `persistence.enabled` | bool | No | `false` | Mount a PVC for Redis data |
| `persistence.storageClassName` | string | No | `""` (cluster default) | StorageClass for PVC |
| `persistence.size` | string | No | `"1Gi"` | PVC storage request |
| `auth.enabled` | bool | No | `false` | Require `requirepass` in redis.conf |
| `auth.secretName` | string | No | `""` | Existing Secret holding key `password` |
| `config` | map[string]string | No | `{}` | Arbitrary redis.conf key/value overrides |

### `status` Fields

| Field | Type | Description |
|---|---|---|
| `phase` | string | `Pending` \| `Running` \| `Degraded` \| `Terminating` |
| `readyReplicas` | integer | Number of ready pods reported by StatefulSet |
| `conditions` | []metav1.Condition | Standard Kubernetes conditions (see below) |

### Conditions

| Type | Meaning |
|---|---|
| `Available` | All desired replicas are ready |
| `Reconciling` | A reconcile loop is actively in progress |
| `Degraded` | One or more replicas are not ready |

## Owned Resources Table

| Kind | Name pattern | Purpose |
|---|---|---|
| `StatefulSet` | `<cr-name>` | Runs Redis pods with stable network identity |
| `Service` (ClusterIP) | `<cr-name>` | Stable virtual IP for clients |
| `Service` (Headless) | `<cr-name>-headless` | Per-pod DNS (`<pod>.<cr-name>-headless.<ns>.svc`) |
| `ConfigMap` | `<cr-name>-config` | Rendered `redis.conf` from spec fields |
| `Secret` | `<cr-name>-auth` | Created only when `auth.enabled && !auth.secretName` |
| `PVC` | managed via StatefulSet `volumeClaimTemplates` | Persisted data directory (optional) |

All owned resources carry `ownerReferences` pointing to the `Redis` CR so that garbage collection removes them when the CR is deleted.

## Reconciler Loop Pseudocode

```
func Reconcile(ctx, req):
    redis = fetch Redis CR (req.NamespacedName)
    if not found: return OK  // deleted; owner refs handle cleanup

    if redis.DeletionTimestamp != nil:
        setPhase(Terminating)
        return OK

    setPhase(Pending) if not yet Running

    // 1. ConfigMap
    desired = buildConfigMap(redis)
    createOrUpdate(ConfigMap, desired)

    // 2. Auth Secret (only when auth.enabled and no external secret)
    if redis.spec.auth.enabled and redis.spec.auth.secretName == "":
        desired = buildAuthSecret(redis)
        createOrUpdate(Secret, desired)

    // 3. StatefulSet
    desired = buildStatefulSet(redis)  // references ConfigMap volume, Secret env
    createOrUpdate(StatefulSet, desired)

    // 4. ClusterIP Service
    desired = buildService(redis, ClusterIP)
    createOrUpdate(Service, desired)

    // 5. Headless Service
    desired = buildHeadlessService(redis)
    createOrUpdate(Service, desired)

    // 6. Observe current state
    sts = fetch StatefulSet
    readyReplicas = sts.Status.ReadyReplicas
    updateStatus(redis, readyReplicas)

    if readyReplicas == redis.spec.replicas:
        setPhase(Running); setCondition(Available, True)
    else:
        setPhase(Degraded); setCondition(Available, False)

    emitEvent(redis, reason, message)
    return OK (requeue after 30s if Degraded)
```

## RBAC Permissions

The operator's `ClusterRole` requires the following rules:

| API Group | Resources | Verbs |
|---|---|---|
| `redis.example.com` | `redis` | `get`, `list`, `watch`, `update`, `patch` |
| `redis.example.com` | `redis/status` | `get`, `update`, `patch` |
| `redis.example.com` | `redis/finalizers` | `update` |
| `apps` | `statefulsets` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` |
| `""` (core) | `services` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` |
| `""` (core) | `configmaps` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` |
| `""` (core) | `secrets` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` |
| `""` (core) | `persistentvolumeclaims` | `get`, `list`, `watch` |
| `""` (core) | `events` | `create`, `patch` |
| `coordination.k8s.io` | `leases` | `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` |

A dedicated `ServiceAccount` is bound to this `ClusterRole` via a `ClusterRoleBinding`.

## Observability

### Prometheus Metrics

The controller exposes metrics on `:8080/metrics` (default controller-runtime endpoint):

- Standard controller-runtime metrics: `controller_runtime_reconcile_total`, `controller_runtime_reconcile_errors_total`, `controller_runtime_reconcile_time_seconds`
- Custom gauge: `redis_operator_ready_replicas{namespace, name}` — tracks `status.readyReplicas`
- Custom counter: `redis_operator_phase_transitions_total{namespace, name, from_phase, to_phase}`

### Structured Logs

All log entries use `logr`/`zap` JSON format with fields: `namespace`, `name`, `reconcileID`, `phase`, `error` (when applicable).

### Kubernetes Events

The controller emits `Normal` and `Warning` events on the `Redis` object:

| Reason | Type | When |
|---|---|---|
| `Reconciling` | Normal | Reconcile loop starts |
| `Provisioned` | Normal | Owned resource created or updated |
| `StatusUpdated` | Normal | `status.phase` changed |
| `ReconcileError` | Warning | Any error during reconciliation |

## Deployment Approach

The operator is expected to ship with both a **Helm chart** and a **Kustomize base**. Which packaging option is considered the primary deployment mechanism remains an open pre-implementation decision.

- **Helm chart** — installs CRD, RBAC, Deployment, and ServiceAccount; supports `values.yaml` overrides for image, replicas, resource limits, and namespace watch mode.
- **Kustomize base** — raw manifests under `config/` (kubebuilder layout) suitable for `kubectl apply -k`.

In both packaging modes, the operator `Deployment` itself runs in the `redis-system` namespace. Watch scope is configured separately through manager options so the same Deployment can operate cluster-wide or be restricted to a single namespace.

CRD installation is handled by Helm's `crds/` directory (auto-applied before other resources).

## Update Strategy

- StatefulSet uses `RollingUpdate` strategy with `partition: 0` by default.
- Controller image updates trigger a Deployment rolling update (one pod at a time).
- CRD schema updates must remain backward-compatible within `v1alpha1`; breaking changes require a new API version.
