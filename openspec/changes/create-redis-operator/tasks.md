# Tasks: Redis Kubernetes Operator

Tasks are sequenced by dependency. Each task produces a reviewable artifact or a passing test gate.

---

## Implementation Checklist

- [ ] Scaffold the Go operator project with controller-runtime/operator-sdk and verify `make generate`, `make manifests`, and `go test ./...`.
- [ ] Define the namespaced `Redis` API, status conditions, CRD validation, defaulting webhook, and validating webhook for immutable and unsafe fields.
- [ ] Implement the core reconciler for ConfigMap, Services, StatefulSet, PodDisruptionBudget, finalizer, events, and status updates.
- [ ] Add Sentinel high-availability reconciliation, failover detection, master status updates, and Sentinel endpoint reporting.
- [ ] Implement config, auth, TLS, upgrade, and scale workflows with unit, integration, and kind end-to-end coverage.
- [ ] Harden RBAC, pod security context, metrics, structured logging, and release/install manifests.

---

## Phase 1 — Scaffolding & API

| ID | Task | Acceptance Criteria |
|---|---|---|
| T1.1 | Initialize operator project with `operator-sdk init` (Go, controller-runtime) | Project compiles; `make generate` and `make manifests` succeed |
| T1.2 | Define `Redis` CRD types (`RedisSpec`, `RedisStatus`, conditions) | `make generate` produces deepcopy; CRD YAML validates against Kubernetes schema |
| T1.3 | Add CEL validation rules to CRD markers | `make manifests` embeds CEL rules; admission rejects invalid specs in unit tests |
| T1.4 | Implement defaulting MutatingWebhook | Defaults applied correctly; webhook unit tests pass |
| T1.5 | Implement ValidatingWebhook for immutability and downscale guards | Webhook rejects violating updates in unit tests |
| T1.6 | Register CRD, webhooks, and operator RBAC ClusterRole in kustomize manifests | `kubectl apply -k config/default` installs without errors on a fresh cluster |

---

## Phase 2 — Core Reconciler (Standalone Mode)

| ID | Task | Acceptance Criteria |
|---|---|---|
| T2.1 | Scaffold `RedisReconciler` with controller-runtime; wire leader election | Operator starts, logs readiness, leader election Lease created |
| T2.2 | Reconcile ConfigMap from `spec.config` (hash-based) | ConfigMap created/updated on spec change; annotation updated |
| T2.3 | Reconcile headless Service and primary Service | Services created with correct selectors and ports |
| T2.4 | Reconcile StatefulSet for Standalone mode (single replica) | StatefulSet created; pod reaches Running/Ready |
| T2.5 | Reconcile PodDisruptionBudget | PDB created; disruption budget respected in unit test |
| T2.6 | Update `.status` (phase, readyReplicas, conditions) | Status transitions Pending → Running observable in integration test |
| T2.7 | Emit Kubernetes Events for lifecycle transitions | Events appear under `kubectl describe redis <name>` |
| T2.8 | Handle finalizer and deletion cleanup | CR deletion removes finalizer; owned objects GC'd; PVC retained by default |

---

## Phase 3 — Sentinel Mode

| ID | Task | Acceptance Criteria |
|---|---|---|
| T3.1 | Extend StatefulSet reconciliation for Sentinel topology (N+3 pods) | StatefulSet with N data pods + 3 Sentinel pods created |
| T3.2 | Generate Sentinel configuration and inject via ConfigMap | Sentinel config references correct master; Sentinel pods start |
| T3.3 | Post-readiness: run `SENTINEL MONITOR` and configure quorum | `SENTINEL masters` reports monitored master in integration test |
| T3.4 | Expose `status.sentinelEndpoint` | Field populated after Sentinel is healthy |
| T3.5 | Test automatic failover: kill primary pod; verify Sentinel promotes replica | Primary changes; status.masterRef updated; `Available` stays True |

---

## Phase 4 — Cluster Mode

| ID | Task | Acceptance Criteria |
|---|---|---|
| T4.1 | Extend StatefulSet reconciliation for Cluster topology (≥6 pods, N shards × 2) | StatefulSet with correct replica count created |
| T4.2 | Post-readiness: run `CLUSTER MEET` to form cluster | `CLUSTER INFO` shows `cluster_state:ok` in integration test |
| T4.3 | Slot allocation: run `CLUSTER REBALANCE` | Slots evenly distributed; no migrating slots after rebalance |
| T4.4 | Expose `status.clusterID` | Field populated after cluster is healthy |
| T4.5 | Scale-down slot migration: migrate slots before removing node | Data integrity preserved during scale-down integration test |
| T4.6 | Reconcile read-only replica Service | Replica service selects non-primary pods correctly |

---

## Phase 5 — Config & Upgrade Flows

| ID | Task | Acceptance Criteria |
|---|---|---|
| T5.1 | In-place `CONFIG SET` for hot-reloadable directives | Supported directives applied without restart; `ConfigSynced` condition True |
| T5.2 | Rolling upgrade: update `spec.version`; observe ordered pod restarts | No connections dropped under test load; all pods updated to new version |
| T5.3 | `OnDelete` update strategy: pods not restarted until manually deleted | StatefulSet patch does not trigger automatic restart; manual delete triggers update |

---

## Phase 6 — Security & Hardening

| ID | Task | Acceptance Criteria |
|---|---|---|
| T6.1 | Auth: mount password Secret; configure `requirepass` in redis.conf | Unauthenticated connections rejected; operator uses password for health checks |
| T6.2 | TLS: mount cert Secret; configure TLS in redis.conf | TLS connections succeed; unencrypted connections rejected when TLS enabled |
| T6.3 | Apply pod security context (non-root, read-only FS, seccomp) | `kubectl auth can-i --as system:serviceaccount:...` confirms no excess permissions |
| T6.4 | Restrict operator ClusterRole to minimum required verbs | RBAC audit log shows no excess permission usage |

---

## Phase 7 — Observability & Testing

| ID | Task | Acceptance Criteria |
|---|---|---|
| T7.1 | Expose Prometheus metrics endpoint | Metrics scraped by in-cluster Prometheus; reconcile counters increment |
| T7.2 | Structured logging with configurable level | Log level flag changes verbosity; no sensitive data logged (password, keys) |
| T7.3 | Unit tests ≥ 80 % coverage | `go test ./... -cover` reports ≥ 80 % |
| T7.4 | Integration tests: Standalone, Sentinel, Cluster lifecycle | Tests pass in CI against real `envtest` or kind cluster |
| T7.5 | `operator-sdk scorecard` at level 3 | Scorecard reports all tests passing |
| T7.6 | Chaos test: random pod kills under Sentinel mode | Cluster recovers within SLO; no data loss for fsync-enabled config |

---

## Phase 8 — Documentation & Release Prep

| ID | Task | Acceptance Criteria |
|---|---|---|
| T8.1 | API reference (generated from CRD markers) | `make docs` produces accurate field-level reference |
| T8.2 | Runbook: when to use Standalone vs Sentinel vs Cluster | Reviewed and merged |
| T8.3 | Operator upgrade guide (from v1alpha1 to future v1beta1) | Conversion webhook stub in place; guide documents field renames |
| T8.4 | Kustomize install bundle for `config/default` | Single `kubectl apply -k` installs all components cleanly |
