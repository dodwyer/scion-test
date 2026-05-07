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
| `redisVersion` | `string` | Redis container image tag (e.g., `"7.2"`) |
| `storage` | `PersistentVolumeClaimTemplate` | PVC template for data volumes |
| `resources` | `ResourceRequirements` | CPU and memory requests/limits |
| `config` | `map[string]string` | redis.conf key/value overrides |
| `auth.passwordSecretRef` | `SecretKeySelector` | Reference to Secret holding the Redis password |
| `auth.tls` | `TLSConfig` | Optional TLS certificate configuration |
| `topology` | `string` | `standalone` or `sentinel` |

#### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `string` | `Pending`, `Running`, `Degraded`, or `Failed` |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes condition array |
| `readyReplicas` | `int32` | Count of ready replicas |
| `masterEndpoint` | `string` | DNS name of the current master |
| `sentinelEndpoints` | `[]string` | DNS names of Sentinel instances (sentinel topology only) |

## Controller Architecture

### Reconcile Loop

The controller watches `RedisInstance` CR events (create, update, delete) and runs the following reconcile steps in order:

1. Fetch the `RedisInstance` CR; return if not found (deleted without finalizer).
2. Add finalizer `redis.example.io/cleanup` if absent.
3. If deletion timestamp is set, run ordered teardown and remove finalizer.
4. Gate on `observedGeneration == generation`; skip reconcile if already current.
5. Reconcile ConfigMap from `spec.config`.
6. Reconcile headless Service.
7. Reconcile StatefulSet (image, replicas, volume mounts, resource limits).
8. If `spec.topology == sentinel`, reconcile Sentinel StatefulSet and Service.
9. Update `status` subresource (phase, conditions, readyReplicas, endpoints).
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
- **Scale-down**: Cordon target replicas (remove from service endpoints), drain replication lag to zero, then reduce `spec.replicas`.

### Rolling Restart

Triggered when `spec.redisVersion` or `spec.config` changes. The StatefulSet update strategy is `RollingUpdate` with a configurable `maxUnavailable`. The controller annotates the StatefulSet pod template to force a rolling restart.

### Finalizer / Teardown

On deletion, the finalizer ensures:
1. Sentinel instances are stopped (if applicable).
2. Redis replicas are shut down in reverse ordinal order.
3. PVCs are retained or deleted per a retention policy (TBD — see open questions).
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
- Controller exposes Prometheus metrics via the controller-runtime metrics endpoint (default port 8080).

## Security Considerations

- Auth credentials sourced from Kubernetes Secrets via `secretKeyRef`; never stored in the CR spec.
- TLS termination configurable via `spec.auth.tls`; certificates referenced from Secrets.
- Pod security: non-root user, read-only root filesystem, dropped capabilities.
- Network policy: restrict inter-pod Redis traffic to operator-managed labels.

---

## Open Questions

The following questions must be resolved by stakeholders before implementation begins. Answers will update this design document.

**OQ-1 — Cluster Topology Scope**  
Must Redis Cluster mode be supported in v1, or is standalone + Sentinel sufficient? Redis Cluster requires sharding logic and a significantly different reconcile path.  
*Default assumption*: standalone + Sentinel only for v1.

**OQ-2 — Supported Redis Versions**  
Which Redis major versions must be supported: 6.x, 7.x, or both? The operator must validate `spec.redisVersion` against a supported list.  
*Default assumption*: 7.x; 6.x support deferred.

**OQ-3 — Storage Optionality**  
Is PVC-backed storage mandatory, or should the operator support `emptyDir` for dev/test scenarios? Allowing `emptyDir` requires a storage mode discriminator in the spec.  
*Default assumption*: PVC mandatory; `emptyDir` added as a follow-on.

**OQ-4 — Namespace Scope**  
Should the `RedisInstance` CRD be namespace-scoped (operator watches one or all namespaces) or cluster-scoped? Namespace-scoped is simpler; cluster-scoped complicates RBAC.  
*Default assumption*: namespace-scoped.

**OQ-5 — Authentication Policy**  
Is authentication (password) mandatory on every `RedisInstance`, or optional? Mandating auth simplifies security posture but breaks unauthenticated dev workflows.  
*Default assumption*: optional; security policy enforced at admission webhook level (future work).

**OQ-6 — Backup and Restore**  
Should v1 include backup/restore support (e.g., scheduled RDB snapshots to object storage)? This significantly expands scope.  
*Default assumption*: out of scope for v1; re-evaluate in v2.

**OQ-7 — Redis Exporter Sidecar**  
Should the operator automatically inject a `redis_exporter` sidecar for Prometheus metrics scraping, or leave that to the user?  
*Default assumption*: optional, controlled by `spec.monitoring.enabled` flag (not implemented in v1 unless OQ resolved in favor).

**OQ-8 — Kubernetes Version Floor**  
What is the minimum supported Kubernetes version? This determines which API versions and features are available (e.g., Server-Side Apply requires 1.22+, StatefulSet minReadySeconds requires 1.25+).  
*Default assumption*: 1.24.
