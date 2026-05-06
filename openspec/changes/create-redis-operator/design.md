# Design: Redis Kubernetes Operator

## Overview

The operator is a single Go binary (built with `controller-runtime`) that runs inside the cluster as a `Deployment`. It owns the `Redis` CRD and reconciles custom resources to a set of native Kubernetes objects. All interactions with the Kubernetes API server go through the Hub; no out-of-cluster access is required at runtime.

---

## API Design

### Group / Version / Kind

| Field | Value |
|---|---|
| Group | `redis.example.io` |
| Version | `v1alpha1` (stable API to graduate to `v1beta1` post-hardening) |
| Kind | `Redis` |
| Scope | Namespaced |

### RedisSpec (`.spec`)

```
spec:
  mode: Standalone | Sentinel | Cluster   # required; default Standalone
  replicas: <int>                          # Standalone: 1; Sentinel: ≥3 (odd); Cluster: ≥6
  version: "<semver>"                      # Redis image tag, e.g. "7.2.4"
  image:
    repository: <string>                   # defaults to docker.io/library/redis
    pullPolicy: IfNotPresent | Always | Never
    pullSecrets: [<secretName>]
  resources:
    requests/limits: cpu, memory           # standard ResourceRequirements
  storage:
    size: <quantity>                       # e.g. "10Gi"; required unless ephemeral
    storageClassName: <string>             # optional; uses cluster default if absent
    ephemeral: <bool>                      # use emptyDir instead of PVC (testing only)
  config:
    <key>: <value>                         # arbitrary redis.conf directives
  auth:
    secretName: <string>                   # Secret with key "redis-password"
  tls:
    enabled: <bool>
    secretName: <string>                   # Secret with TLS cert/key
  sentinel:
    quorum: <int>                          # only valid when mode=Sentinel
    downAfterMilliseconds: <int>
    failoverTimeout: <int>
  cluster:
    nodesPerShard: <int>                   # only valid when mode=Cluster; min 2
  updateStrategy:
    type: RollingUpdate | OnDelete
    rollingUpdate:
      maxUnavailable: <intOrPercent>       # default 1
  podTemplate:
    metadata:
      labels/annotations: {}
    spec:
      affinity, tolerations, nodeSelector, securityContext, ...
  serviceAnnotations: {}
  priorityClassName: <string>
  podDisruptionBudget:
    enabled: <bool>                        # default true
    minAvailable: <intOrPercent>
```

### RedisStatus (`.status`)

```
status:
  phase: Pending | Initializing | Running | Degraded | Failed | Terminating
  readyReplicas: <int>
  currentReplicas: <int>
  observedGeneration: <int>
  masterRef:
    name: <podName>
    podIP: <ip>
  sentinelEndpoint: <service>:<port>       # Sentinel mode only
  clusterID: <string>                      # Cluster mode only
  conditions:
    - type: Available
    - type: Progressing
    - type: Degraded
    - type: ConfigSynced
  version: <string>                        # currently running image tag
```

### Status Conditions

| Type | Meaning |
|---|---|
| `Available` | ≥1 replica is ready and serving |
| `Progressing` | A rollout, scale, or config change is in progress |
| `Degraded` | Fewer replicas than `spec.replicas` are ready |
| `ConfigSynced` | The running redis.conf matches the desired config |

Each condition carries `status` (True/False/Unknown), `reason` (CamelCase token), `message` (human text), and `lastTransitionTime`.

---

## Owned Kubernetes Objects

For each `Redis` CR the operator creates and manages:

| Object | Purpose |
|---|---|
| `StatefulSet` | Pods with stable network identity and ordered rollout |
| `Service` (headless) | DNS-based pod discovery (`<pod>.<svc>.<ns>.svc`) |
| `Service` (read/write) | ClusterIP service for the primary/master |
| `Service` (read-only) | ClusterIP service for replicas (Sentinel/Cluster modes) |
| `ConfigMap` | Generated `redis.conf` from `spec.config` |
| `Secret` (managed) | Operator-managed ACL file when auth is enabled |
| `PodDisruptionBudget` | Maintains quorum during voluntary disruptions |
| `ServiceAccount` | Pod identity (no cluster-level permissions needed for pods) |

All objects carry an `ownerReference` back to the `Redis` CR so that garbage collection on CR deletion is automatic.

---

## Reconciliation Design

### Reconcile Loop

```
Reconcile(request):
  1. Fetch Redis CR; if not found, return (already deleted).
  2. Set status.phase = Initializing on first-seen generation.
  3. Validate spec (supplement CEL with runtime checks where needed).
  4. Reconcile ConfigMap (hash-based change detection).
  5. Reconcile StatefulSet (create or patch; respect updateStrategy).
  6. Reconcile Services (headless, primary, replica).
  7. Reconcile PodDisruptionBudget.
  8. Wait for StatefulSet readiness (requeue with backoff if not ready).
  9. Run mode-specific post-readiness steps:
       Standalone: promote first pod to primary.
       Sentinel:   configure Sentinel quorum, trigger SENTINEL MONITOR.
       Cluster:    run CLUSTER MEET / CLUSTER REBALANCE as needed.
  10. Update status.readyReplicas, status.masterRef, conditions.
  11. Set status.phase = Running if all replicas ready; else Degraded.
  12. Return (no requeue unless explicit error or external change).
```

### Config Change Flow

1. Operator computes `sha256(spec.config)` and stores it as an annotation on the ConfigMap.
2. If the hash changes, the ConfigMap is updated and the StatefulSet's pod template annotation is patched to trigger a rolling restart.
3. Config directives that support `CONFIG SET` are applied in-place via a Redis CLIENT command before the rolling restart completes (`ConfigSynced` condition gates this).

### Upgrade Flow

1. Operator detects `spec.version` differs from the running image tag.
2. Sets `Progressing=True`.
3. Updates the StatefulSet image; controller-runtime's StatefulSet rolling update (ordered) takes over.
4. Readiness probe gates each pod restart. `maxUnavailable` controls pace.
5. On completion, `Progressing=False`, `Available=True`.

### Scale-Up Flow

1. `spec.replicas` is increased.
2. StatefulSet `.spec.replicas` is patched.
3. New pods join as replicas; operator runs `REPLICAOF` via exec probe.
4. For Cluster mode, operator runs `CLUSTER REBALANCE` after all pods are ready.

### Scale-Down Flow

1. `spec.replicas` is decreased.
2. Operator verifies PDB is not violated.
3. For Cluster mode, operator migrates slots off the pod being removed (`CLUSTER SETSLOT … MIGRATE`), then removes the node from the cluster.
4. StatefulSet `.spec.replicas` is patched.

---

## Validation

### CRD CEL Rules (admission-time)

- `spec.mode == "Standalone" ? spec.replicas == 1 : true`
- `spec.mode == "Sentinel" ? spec.replicas >= 3 && spec.replicas % 2 == 1 : true`
- `spec.mode == "Cluster" ? spec.replicas >= 6 && spec.replicas % 2 == 0 : true`
- `spec.sentinel != null ? spec.mode == "Sentinel" : true`
- `spec.cluster != null ? spec.mode == "Cluster" : true`
- `spec.storage.ephemeral == true ? spec.storage.size == null : true`
- `spec.version` matches regex `^\d+\.\d+\.\d+$`

### Webhook Validation (defaulting + complex invariants)

A `MutatingAdmissionWebhook` sets defaults (replicas, pullPolicy, PDB settings) so specs are always complete before the reconciler sees them.

A `ValidatingAdmissionWebhook` enforces:
- Immutable fields: `spec.mode`, `spec.storage.storageClassName` (after initial creation).
- Downscale guard: replicas may not decrease below quorum in Sentinel mode.

---

## RBAC & Runtime Expectations

### Operator ClusterRole (minimum)

```
- apiGroups: ["redis.example.io"]
  resources: ["redis", "redis/status", "redis/finalizers"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["configmaps", "services", "serviceaccounts", "secrets", "pods", "pods/exec", "events"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["apps"]
  resources: ["statefulsets"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

### Pod ServiceAccount

Redis pods run under a dedicated `ServiceAccount` with no additional permissions (not cluster-scoped).

### Security Context

- `runAsNonRoot: true`, `runAsUser: 999` (redis uid)
- `readOnlyRootFilesystem: true` with a writable `emptyDir` for `/data` (unless PVC)
- `allowPrivilegeEscalation: false`
- `seccompProfile: RuntimeDefault`

---

## Observability

- **Metrics**: The operator exposes Prometheus metrics at `:8080/metrics` (controller-runtime defaults plus custom gauges for `redis_operator_reconcile_total`, `redis_operator_reconcile_errors_total`, `redis_operator_ready_replicas`).
- **Events**: Kubernetes Events are emitted for key lifecycle transitions (Created, Updated, Degraded, FailoverTriggered).
- **Structured Logs**: `zap` logger; log level configurable via operator flag `--log-level`.

---

## Leader Election

The operator deployment runs with `--leader-elect=true` (controller-runtime leader election via `Lease` object) to ensure a single active reconciler even when multiple replicas are deployed.

---

## Finalizer Strategy

The CR carries finalizer `redis.example.io/cleanup`. On deletion:
1. Operator removes the cluster from Sentinel monitors (Sentinel mode).
2. Removes nodes from the Redis Cluster (Cluster mode).
3. Removes finalizer; Kubernetes garbage-collects owned objects via ownerReference.

PVCs are **not** deleted by default (data safety). A `spec.storage.deletePVCOnDelete: bool` field controls opt-in deletion.

---

## Unresolved Questions

1. **Multi-namespace watch**: Should the operator watch all namespaces (cluster-scoped) or be constrained to a single namespace per deployment? Default is cluster-scoped; single-namespace mode is possible via `--namespace` flag.
2. **ACL complexity**: Redis 6+ ACL file support is sketched but full ACL CR management may warrant a separate `RedisACL` kind.
3. **Sentinel vs. Redis Cluster trade-offs**: Documentation for consumers on when to choose each mode is out of scope for this spec but should accompany the release.
4. **TLS rotation**: Live TLS certificate rotation without restart is not specified; operator currently triggers a rolling restart on cert Secret change.
