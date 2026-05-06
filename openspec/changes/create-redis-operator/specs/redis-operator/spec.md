# Spec: Redis Kubernetes Operator

**Version**: 0.1.0-draft  
**API Group**: `redis.example.io`  
**Kind**: `Redis`  
**Scope**: Namespaced  

---

## 1. CRD Definition

### 1.1 TypeMeta

```yaml
apiVersion: redis.example.io/v1alpha1
kind: Redis
```

### 1.2 ObjectMeta

Standard Kubernetes metadata. The operator reads:
- `metadata.name` — used as prefix for all owned object names.
- `metadata.namespace` — all owned objects are co-located in the same namespace.
- `metadata.generation` — tracked in `status.observedGeneration` to detect spec drift.

Operator-managed annotations on the CR:
- `redis.example.io/config-hash` — SHA-256 of serialized `spec.config`; change triggers rolling restart.

### 1.3 Spec Fields

#### 1.3.1 `spec.mode` (required)

| Value | Description |
|---|---|
| `Standalone` | Single Redis instance; no replication. |
| `Sentinel` | Redis replication with Sentinel high-availability. |
| `Cluster` | Redis Cluster (hash-slot sharding). |

Default: `Standalone`.  
Immutable after creation.

#### 1.3.2 `spec.replicas` (required)

Number of Redis data nodes. Constraints enforced via CEL:
- `Standalone`: must be `1`.
- `Sentinel`: must be ≥ 3 and odd.
- `Cluster`: must be ≥ 6 and even (pairs of primary + replica per shard).

#### 1.3.3 `spec.version` (required)

Semantic version string matching `^\d+\.\d+\.\d+$` (e.g., `"7.2.4"`). Used as the image tag. Changing this field triggers a rolling upgrade.

#### 1.3.4 `spec.image`

| Field | Type | Default | Description |
|---|---|---|---|
| `repository` | string | `docker.io/library/redis` | OCI image repository |
| `pullPolicy` | enum | `IfNotPresent` | `IfNotPresent`, `Always`, `Never` |
| `pullSecrets` | []string | — | Names of `imagePullSecrets` in the same namespace |

#### 1.3.5 `spec.resources`

Standard `corev1.ResourceRequirements`. Both `requests` and `limits` should be set for predictable QoS. If unset, pods run in `BestEffort` class (not recommended for production).

#### 1.3.6 `spec.storage`

| Field | Type | Default | Description |
|---|---|---|---|
| `size` | Quantity | — | PVC size (e.g., `"10Gi"`). Required unless `ephemeral: true`. |
| `storageClassName` | string | cluster default | StorageClass for PVC provisioning. Immutable after creation. |
| `ephemeral` | bool | `false` | Use `emptyDir` instead of PVC. Not for production. |
| `deletePVCOnDelete` | bool | `false` | Delete PVCs when the CR is deleted. Dangerous — opt-in only. |

#### 1.3.7 `spec.config`

Arbitrary map of `string → string` matching `redis.conf` directive names to values (e.g., `maxmemory: "512mb"`, `maxmemory-policy: allkeys-lru`). The operator generates a `redis.conf` from this map, prefixed with operator-managed directives (bind, port, TLS settings). Operator directives take precedence over user-supplied values for safety-critical settings.

#### 1.3.8 `spec.auth`

| Field | Type | Description |
|---|---|---|
| `secretName` | string | Name of a `Secret` in the same namespace containing key `redis-password`. |

When set, the operator sets `requirepass` and `masterauth` in the generated `redis.conf`. The operator also uses the password for its own health-check connections.

#### 1.3.9 `spec.tls`

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable TLS on the Redis port. |
| `secretName` | string | — | `Secret` containing `tls.crt`, `tls.key`, and optionally `ca.crt`. |

When TLS is enabled, the operator configures `tls-port`, `tls-cert-file`, `tls-key-file`, and (if `ca.crt` present) `tls-ca-cert-file`. Plaintext port is disabled. A rolling restart is triggered when the Secret content changes.

#### 1.3.10 `spec.sentinel` (Sentinel mode only)

| Field | Type | Default | Description |
|---|---|---|---|
| `quorum` | int | `(replicas/2)+1` | Number of Sentinels that must agree to trigger failover. |
| `downAfterMilliseconds` | int | `30000` | Time before a master is considered down. |
| `failoverTimeout` | int | `180000` | Timeout for a single failover attempt (ms). |

#### 1.3.11 `spec.cluster` (Cluster mode only)

| Field | Type | Default | Description |
|---|---|---|---|
| `nodesPerShard` | int | `2` | Number of nodes per shard (primary + N-1 replicas). Minimum 2. |

#### 1.3.12 `spec.updateStrategy`

| Field | Type | Default | Description |
|---|---|---|---|
| `type` | enum | `RollingUpdate` | `RollingUpdate` or `OnDelete`. |
| `rollingUpdate.maxUnavailable` | intOrPercent | `1` | Max pods that can be unavailable during update. |

Maps directly to the StatefulSet `updateStrategy`.

#### 1.3.13 `spec.podTemplate`

Partial `PodTemplateSpec` allowing the user to set:
- `metadata.labels` / `metadata.annotations` (merged with operator labels).
- `spec.affinity`, `spec.tolerations`, `spec.nodeSelector`.
- `spec.securityContext` (merged with operator defaults; operator defaults take precedence for security-critical fields).
- `spec.topologySpreadConstraints`.

#### 1.3.14 `spec.podDisruptionBudget`

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Create a PDB for the StatefulSet. |
| `minAvailable` | intOrPercent | Computed | Defaults: Standalone=1, Sentinel=quorum, Cluster=(shards). |

#### 1.3.15 `spec.serviceAnnotations`

Map of annotations applied to all managed Services (e.g., cloud load-balancer annotations).

#### 1.3.16 `spec.priorityClassName`

Name of a `PriorityClass` to assign to Redis pods.

---

## 2. Status Fields

### 2.1 `status.phase`

| Value | Description |
|---|---|
| `Pending` | CR accepted; no owned objects created yet. |
| `Initializing` | Owned objects created; pods not yet ready. |
| `Running` | All `spec.replicas` pods are Ready. |
| `Degraded` | Fewer than `spec.replicas` pods are Ready; cluster is still partially serving. |
| `Failed` | Unrecoverable error; manual intervention required. |
| `Terminating` | CR is being deleted; cleanup in progress. |

### 2.2 `status.readyReplicas`

Count of pods in `Ready` state at the time of the last reconcile.

### 2.3 `status.currentReplicas`

Count of pods currently running (including non-Ready).

### 2.4 `status.observedGeneration`

The `metadata.generation` processed in the last successful reconcile.

### 2.5 `status.masterRef`

```
masterRef:
  name: <podName>    # e.g. "my-redis-0"
  podIP: <ip>
```

Populated for Standalone and Sentinel modes. Updated on failover.

### 2.6 `status.sentinelEndpoint`

`<sentinelServiceName>:<port>` — only populated in Sentinel mode.

### 2.7 `status.clusterID`

Cluster UUID reported by `CLUSTER INFO` — only populated in Cluster mode.

### 2.8 `status.version`

Image tag currently running on all pods. May differ from `spec.version` during a rolling upgrade.

### 2.9 `status.conditions`

Each condition conforms to the Kubernetes metav1.Condition type:

| Condition Type | True Meaning | False Meaning |
|---|---|---|
| `Available` | ≥1 replica Ready | No replicas Ready |
| `Progressing` | Rollout/scale/config change in flight | No active changes |
| `Degraded` | Fewer replicas than desired | Replica count matches desired |
| `ConfigSynced` | Running config matches spec.config | Config update pending or failed |

---

## 3. Owned Object Specifications

### 3.1 StatefulSet

- Name: `<cr-name>`
- `spec.serviceName`: `<cr-name>-headless`
- `spec.replicas`: `spec.replicas`
- Pod labels include `redis.example.io/name: <cr-name>` and `redis.example.io/role: data` (or `sentinel`).
- Container name: `redis`
- Container command: `redis-server /etc/redis/redis.conf`
- Liveness probe: `redis-cli -a $REDIS_PASSWORD PING` (TCP check if TLS only)
- Readiness probe: `redis-cli -a $REDIS_PASSWORD PING`; for replicas, also checks `role` is `slave` or `master`.
- Volume mounts: `/etc/redis` (ConfigMap), `/data` (PVC or emptyDir), `/etc/redis-tls` (TLS Secret, if TLS enabled).

### 3.2 Headless Service

- Name: `<cr-name>-headless`
- `clusterIP: None`
- Port: `6379` (or TLS port `6380`)
- Selector: `redis.example.io/name: <cr-name>`

### 3.3 Primary Service

- Name: `<cr-name>`
- `type: ClusterIP`
- Port: `6379`
- Selector: `redis.example.io/name: <cr-name>`, `redis.example.io/role: primary`
- The operator updates the selector's role label dynamically on failover (Sentinel mode).

### 3.4 Replica Service (Sentinel & Cluster modes)

- Name: `<cr-name>-replica`
- `type: ClusterIP`
- Selector: `redis.example.io/name: <cr-name>`, `redis.example.io/role: replica`

### 3.5 ConfigMap

- Name: `<cr-name>-config`
- Data key: `redis.conf`
- Content: operator-generated `redis.conf` merging `spec.config` with operator-managed directives.
- Operator-managed directives (always set, not overridable): `bind`, `port`/`tls-port`, `dir /data`, `logfile ""`, `cluster-enabled` (Cluster mode), `sentinel` stanza (Sentinel mode).
- Annotation: `redis.example.io/config-hash: <sha256>`

### 3.6 PodDisruptionBudget

- Name: `<cr-name>`
- `spec.minAvailable`: see `spec.podDisruptionBudget.minAvailable`.
- Selector: `redis.example.io/name: <cr-name>`

---

## 4. Reconciliation Invariants

The following invariants must hold after every successful reconcile:

1. **Ownership**: Every owned object has an `ownerReference` pointing to the `Redis` CR with `controller: true`.
2. **Label consistency**: All pods carry `redis.example.io/name` and `redis.example.io/role` labels matching the Service selectors.
3. **Config hash**: The ConfigMap annotation and the StatefulSet pod template annotation carry identical config hashes.
4. **Status accuracy**: `status.readyReplicas` equals the number of pods where all containers are Ready.
5. **Phase consistency**: `status.phase == Running` if and only if `status.readyReplicas == spec.replicas` and `ConfigSynced == True`.
6. **PDB validity**: The PDB `minAvailable` never exceeds `spec.replicas` (would make the PDB block all voluntary disruptions).

---

## 5. Error Handling & Requeue Policy

| Scenario | Action |
|---|---|
| Transient API error (conflict, timeout) | Requeue with exponential backoff (max 5 min) |
| Pod not ready after StatefulSet update | Requeue after 15 s; emit Warning event after 5 consecutive requeues |
| Cluster mode: CLUSTER INFO not ok | Requeue after 30 s; set `Degraded=True` after 10 min |
| Unrecoverable error (invalid state) | Set `phase=Failed`; emit event; do not requeue |
| Webhook validation failure | Reject at admission; operator never sees the CR |

---

## 6. Upgrade & Migration

### 6.1 CRD Version Migration (v1alpha1 → v1beta1)

- A conversion webhook (Hub-based) translates between versions.
- Field renames are documented in the conversion webhook's `conversion_notes.md`.
- Both versions are served simultaneously during the migration window.
- `storage: true` moves to `v1beta1` once all CRs are migrated.

### 6.2 Operator Binary Upgrade

- The operator deployment uses `RollingUpdate` strategy with `maxUnavailable: 0`.
- Leader election ensures a single active reconciler during the rollover.
- The new operator version must be able to read CRs written by the previous version (forward-compatible status writes).

---

## 7. Verification Strategy

### 7.1 Unit Tests

- All reconcile functions tested with `controller-runtime`'s `envtest` fake client.
- CEL rules tested via the `cel-go` library directly and via webhook unit tests.
- Coverage target: ≥ 80 % of lines.

### 7.2 Integration Tests

- Spin up a real `envtest` API server (no full cluster required).
- Test matrix: {Standalone, Sentinel, Cluster} × {create, update-config, upgrade, scale-up, scale-down, delete}.
- Readiness assertions poll `status.conditions` with a 5-minute timeout.

### 7.3 End-to-End Tests (kind cluster)

- Deploy the operator from the Kustomize bundle.
- Create a `Redis` CR; assert `status.phase == Running`.
- Upgrade `spec.version`; assert all pods updated and `Available=True` throughout.
- Kill the primary pod; assert `status.masterRef` updates (Sentinel mode).
- `operator-sdk scorecard` suite at capability level 3.

### 7.4 Security Verification

- Run `trivy image` against the operator image; no critical CVEs allowed.
- `kube-bench` or `kubescape` scan against the operator's RBAC manifests.
- Confirm no RBAC wildcard verbs or resources in the ClusterRole.

---

## 8. Open Questions

| # | Question | Impact |
|---|---|---|
| Q1 | Should the operator support single-namespace mode via `--namespace` flag? | Deployment model; affects ClusterRole vs Role |
| Q2 | Should `RedisACL` be a separate CRD or a sub-field of `Redis`? | API surface; deferred to follow-on change |
| Q3 | Live TLS rotation without rolling restart — feasible via `CONFIG SET tls-*`? | Availability during cert renewal; needs Redis version testing |
| Q4 | Should PVCs be named with a well-known template so they survive CR recreation? | Data durability; StatefulSet volumeClaimTemplate naming convention |
| Q5 | Operator image base: distroless vs UBI minimal? | Security posture vs debuggability |
