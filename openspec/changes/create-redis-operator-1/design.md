# Design: Redis Kubernetes Operator

## Overview

The Redis Operator is a Kubernetes controller written in **Go** using the **kubebuilder** scaffolding framework (built on `controller-runtime`). It watches `Redis` Custom Resources and reconciles the cluster state to match the declared spec by managing StatefulSets, headless Services, ConfigMaps, and (optionally) PersistentVolumeClaims.

## Toolchain Decision

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Language | Go | De-facto standard for Kubernetes operators; strong ecosystem |
| Framework | kubebuilder v4 | Code-generation for CRD manifests, RBAC markers, and webhook scaffolding |
| Dependency | controller-runtime v0.17+ | Used internally by kubebuilder; exposes Reconciler interface |
| Build | ko or Docker | `ko` for fast image builds without a Dockerfile; Docker as fallback |
| Packaging | Helm chart (primary) + Kustomize overlay (secondary) | kubebuilder generates the base `config/` Kustomize layout; the Helm chart wraps those manifests for parameterised installs, while Kustomize remains suitable for GitOps pipelines |

## CRD Schema

### Group / Version / Kind

```
group:   redis.example.com
version: v1alpha1
kind:    Redis
```

### Spec Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | string | `"7.2"` | Redis image tag (e.g. `"6.2"`, `"7.2"`) |
| `replicas` | int32 | `1` | Number of Redis pods (1 = standalone, >1 = master-replica) |
| `resources.requests.cpu` | resource.Quantity | `"100m"` | CPU request per pod |
| `resources.requests.memory` | resource.Quantity | `"128Mi"` | Memory request per pod |
| `resources.limits.cpu` | resource.Quantity | — | CPU limit per pod |
| `resources.limits.memory` | resource.Quantity | — | Memory limit per pod |
| `persistence.enabled` | bool | `false` | Enable PVC-backed storage |
| `persistence.storageClassName` | string | `""` | StorageClass for PVCs |
| `persistence.size` | resource.Quantity | `"1Gi"` | PVC storage request |
| `config` | map[string]string | `{}` | Extra redis.conf key-value overrides |
| `image.repository` | string | `"redis"` | Container image repository |
| `image.pullPolicy` | PullPolicy | `IfNotPresent` | Image pull policy |
| `serviceType` | ServiceType | `ClusterIP` | Kubernetes Service type |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | `Pending` / `Running` / `Degraded` / `Terminating` |
| `readyReplicas` | int32 | Number of pods in Ready state |
| `conditions[]` | []Condition | Standard metav1.Condition array |
| `observedGeneration` | int64 | Generation of the spec last reconciled |

Standard condition types: `Available`, `Progressing`, `Degraded`.

## Controller Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Redis Controller                   │
│                                                     │
│  Watch: Redis CR  ──► Reconcile()                   │
│                          │                          │
│               ┌──────────▼──────────────┐           │
│               │  fetch Redis CR          │           │
│               │  handle finalizer        │           │
│               │  reconcileConfigMap()    │           │
│               │  reconcileStatefulSet()  │           │
│               │  reconcileService()      │           │
│               │  reconcilePVCs()         │           │
│               │  updateStatus()          │           │
│               └─────────────────────────┘           │
└─────────────────────────────────────────────────────┘
```

### Reconcile Loop Steps

1. **Fetch CR** — retrieve the `Redis` object; return if not found (deleted).
2. **Finalizer handling** — add `redis.example.com/finalizer` on creation; on deletion run cleanup (delete owned resources not covered by owner references) and remove finalizer.
3. **ConfigMap** — create or update a ConfigMap containing `redis.conf` built from `spec.config` plus sensible defaults.
4. **StatefulSet** — create or update a StatefulSet with `spec.replicas` pods mounting the ConfigMap and (if enabled) PVCs. Uses owner references for garbage collection.
5. **Service** — create or update a headless Service (`clusterIP: None`) for stable DNS within the StatefulSet, plus an optional client-facing Service of `spec.serviceType`.
6. **PVCs** — when `persistence.enabled`, ensure a VolumeClaimTemplate exists on the StatefulSet.
7. **Status update** — recompute `phase`, `readyReplicas`, and `conditions` from the StatefulSet's status; patch the CR status subresource.

### Requeue Strategy

| Situation | Action |
|-----------|--------|
| Transient API error | `ctrl.Result{RequeueAfter: 10s}` |
| StatefulSet not yet ready | `ctrl.Result{RequeueAfter: 30s}` |
| Reconcile complete | `ctrl.Result{}` (no requeue; watches re-trigger) |

## RBAC

The controller's ServiceAccount requires the following permissions:

```yaml
# Core resources
- apiGroups: [""]
  resources: [configmaps, services, persistentvolumeclaims, pods]
  verbs: [get, list, watch, create, update, patch, delete]

# Apps
- apiGroups: [apps]
  resources: [statefulsets]
  verbs: [get, list, watch, create, update, patch, delete]

# CRD group
- apiGroups: [redis.example.com]
  resources: [redis, redis/status, redis/finalizers]
  verbs: [get, list, watch, create, update, patch, delete]

# Events
- apiGroups: [""]
  resources: [events]
  verbs: [create, patch]
```

## Deployment Topology

The repository follows the standard kubebuilder-generated `config/` layout for CRDs, RBAC, and manager manifests. The Helm chart at `charts/redis-operator/` wraps those same resources into a parameterised installation path rather than introducing a separate deployment model.

### Single-Namespace Mode

The operator Deployment is installed in a target namespace and only watches that namespace. A `Role` + `RoleBinding` replaces `ClusterRole` + `ClusterRoleBinding`.

### Cluster-Wide Mode

The operator Deployment watches all namespaces. A `ClusterRole` + `ClusterRoleBinding` is required. This is the default Helm chart mode.

## Owned Resource Naming Convention

For a `Redis` CR named `my-redis` in namespace `default`:

| Resource | Name |
|----------|------|
| StatefulSet | `my-redis` |
| Headless Service | `my-redis-headless` |
| Client Service | `my-redis` |
| ConfigMap | `my-redis-config` |
| PVC (per pod) | `data-my-redis-<ordinal>` |

## Master-Replica Topology (replicas > 1)

When `spec.replicas > 1`:
- Pod `my-redis-0` is designated master.
- Pods `my-redis-1..N` are replicas, configured with `replicaof my-redis-0.my-redis-headless.<ns>.svc.cluster.local 6379`.
- This is injected via an init container or a startup script rendered into the ConfigMap.
- No automatic failover (Sentinel) is provided in v1.

## Health Checks

- **Liveness probe**: `redis-cli ping` via exec probe, initial delay 15s, period 20s.
- **Readiness probe**: `redis-cli ping` via exec probe, initial delay 5s, period 10s.

## Testing Strategy

- **Unit tests**: controller logic tested with `envtest` (in-process Kubernetes API server). Each reconcile scenario is a table-driven test.
- **Integration tests** (optional, CI): kind cluster spun up, operator deployed, end-to-end CR lifecycle verified.
