# Design: Redis Kubernetes Operator

## Overview

The Redis Kubernetes Operator is a Go-based controller built with controller-runtime. It manages the lifecycle of `RedisInstance` custom resources and reconciles them to a set of owned Kubernetes primitives.

## API Design

### Custom Resource: RedisInstance

**API group/version:** `redis.example.io/v1alpha1`  
**Kind:** `RedisInstance`

#### Spec Fields

| Field | Type | Description |
|-------|------|-------------|
| `replicas` | `int32` | Number of Redis replicas |
| `redisVersion` | `string` | Redis container image tag or digest (e.g., `"redis:7.2"`); any registry is allowed |
| `storage` | `PersistentVolumeClaimTemplate` | PVC template for data volumes |
| `resources` | `ResourceRequirements` | CPU and memory requests/limits |
| `config` | `map[string]string` | redis.conf key/value overrides |
| `auth.passwordSecretRef` | `SecretKeySelector` | Optional reference to Secret holding the Redis password; if omitted, Redis runs without auth |
| `auth.tls` | `TLSConfig` | Optional TLS certificate configuration |
| `topology` | `string` | `standalone` or `sentinel` |

#### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `string` | `Pending`, `Running`, `Degraded`, or `Failed` |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes condition array |
| `observedGeneration` | `int64` | Latest `.metadata.generation` that has been reconciled and reflected in status |
| `readyReplicas` | `int32` | Count of ready replicas |
| `masterEndpoint` | `string` | DNS name of the current master |
| `sentinelEndpoints` | `[]string` | DNS names of Sentinel instances (sentinel topology only) |

#### Topology and Replication Model

- V1 supports only `standalone` and `sentinel` topologies. Redis Cluster is explicitly out of scope.
- `spec.replicas` defines the total number of Redis data pods managed by the operator.
- The operator maintains exactly one writable primary and `spec.replicas - 1` read replicas.
- For `topology: standalone`, the primary is the ordinal `0` pod and every higher ordinal pod is configured to replicate from that primary.
- For `topology: sentinel`, Sentinel is responsible for primary discovery and failover. Redis data pods still run in a single StatefulSet, and replicas follow the primary advertised by the Sentinel quorum.
- `status.masterEndpoint` reports the currently writable primary endpoint for both supported topologies.
- `spec.storage` is mandatory in v1 and must be backed by a PVC template. Ephemeral storage modes such as `emptyDir` are out of scope.
- Authentication is optional in v1. When `auth.passwordSecretRef` is unset, the operator configures Redis without password auth.
- V1 does not maintain a Redis-version allowlist. Users provide a pullable Redis image reference, and the operator treats that reference as the desired runtime image.

## Controller Architecture

### Reconcile Loop

The controller watches `RedisInstance` CR events (create, update, delete) and runs the following reconcile steps in order:

1. Fetch the `RedisInstance` CR; return if not found (deleted without finalizer).
2. Add finalizer `redis.example.io/cleanup` if absent.
3. If deletion timestamp is set, run ordered teardown and remove finalizer.
4. Inspect owned resources. If `status.observedGeneration == metadata.generation` and all required owned resources exist, skip resource mutation and proceed only with status/event reconciliation as needed. If any required owned resource is missing or drifted, continue with full reconcile even when the generation is current.
5. Reconcile ConfigMap from `spec.config`.
6. Reconcile headless Service.
7. Reconcile StatefulSet (image, replicas, volume mounts, resource limits).
8. If `spec.topology == sentinel`, reconcile Sentinel StatefulSet and Service.
9. Update `status` subresource (`phase`, `conditions`, `observedGeneration`, `readyReplicas`, endpoints).
10. Emit Kubernetes Events on phase transitions.

All steps are idempotent: safe to re-run on any event.

### Owned Resources

| Resource | Name pattern | Notes |
|----------|-------------|-------|
| ConfigMap | `<cr-name>-config` | Rendered redis.conf |
| Service (headless) | `<cr-name>` | DNS for StatefulSet pods |
| StatefulSet | `<cr-name>` | Redis data nodes |
| StatefulSet | `<cr-name>-sentinel` | Sentinel nodes (sentinel topology) |
| Service | `<cr-name>-sentinel` | Sentinel access endpoint |

### Scaling

- **Scale-up**: Increase `spec.replicas`; StatefulSet controller adds pods.
- **Scale-down**:
  1. Update the Redis StatefulSet `.spec.replicas` to the desired lower count.
  2. Wait for the highest-ordinal pods above the new replica count to terminate successfully.
  3. Reconcile the headless Service selector so it continues to match only the remaining operator-managed pods.
  4. During CR deletion, rely on the finalizer to delete PVCs that belonged to removed pods before removing the finalizer.

Scale-down in normal reconciliation does not delete PVCs for retained StatefulSet ordinals; PVC cleanup is only guaranteed during finalizer-driven teardown.

### Rolling Restart

Triggered when `spec.redisVersion` or `spec.config` changes. The StatefulSet update strategy is `RollingUpdate` with a configurable `maxUnavailable`. The controller annotates the StatefulSet pod template to force a rolling restart.

### Finalizer / Teardown

On deletion, the finalizer ensures:
1. Sentinel instances are stopped (if applicable).
2. Redis replicas are shut down in reverse ordinal order.
3. Owned PVCs are deleted by default before finalizer removal.
4. Finalizer is removed, allowing the CR to be garbage collected.

### Leader Election

The controller deployment runs with 2+ replicas. Only the elected leader runs the reconcile loop. Lease-based leader election via `leases.coordination.k8s.io`.

## RBAC

The operator ClusterRole follows least-privilege:

- `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` on: StatefulSets, Services, ConfigMaps, Pods
- `get`, `list`, `watch` on: Secrets, PersistentVolumeClaims
- `get`, `list`, `watch`, `update`, `patch` on: `redisinstances` (and `/status` subresource)
- `create`, `patch` on: Events
- `get`, `create`, `update`, `patch`, `delete` on: Leases (leader election)

Credentials (passwords, TLS certs) are referenced by Secret name and key; they are never injected as environment variable literals.

## Observability

- Kubernetes Events emitted on phase transitions (e.g., `Pending → Running`, `Running → Degraded`).
- `status.conditions` array follows standard Kubernetes condition conventions (`Ready`, `Reconciling`, `Degraded`).
- `status.observedGeneration` is updated whenever the operator has inspected the latest desired spec and reconciled or confirmed owned resources for that generation.
- Controller exposes Prometheus metrics via the controller-runtime metrics endpoint (default port 8080).

## Security Considerations

- Auth credentials sourced from Kubernetes Secrets via `secretKeyRef`; never stored in the CR spec.
- TLS termination configurable via `spec.auth.tls`; certificates referenced from Secrets.
- Pod security: non-root user, read-only root filesystem, dropped capabilities.
- Network policy: restrict inter-pod Redis traffic to operator-managed labels.
- V1 does not inject a Redis exporter sidecar; metrics beyond controller-runtime defaults are out of scope.

---

## Implementation Constraints

- Minimum supported Kubernetes version is 1.26.
- The CRD is namespace-scoped and reconciles namespaced resources only.
- Backup and restore remain out of scope for v1.
- No image registry restrictions are imposed by the operator beyond standard Kubernetes image pull behavior.
