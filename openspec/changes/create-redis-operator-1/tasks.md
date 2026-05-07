# Tasks: Redis Kubernetes Operator

## Phase 1 — Foundation

- [ ] Resolve open questions OQ-1 through OQ-8 with stakeholders and update design.md
- [ ] Set up Go module and project scaffold using controller-runtime and kubebuilder markers
- [ ] Define `RedisInstance` CRD Go types (spec and status structs)
- [ ] Generate CRD YAML manifests from Go types using controller-gen
- [ ] Register the CRD scheme with the controller manager
- [ ] Write unit tests for CRD type validation (required fields, enum constraints)

## Phase 2 — Core Controller

- [ ] Implement `RedisInstanceReconciler` with controller-runtime `Reconcile` method
- [ ] Add finalizer handling (`redis.example.io/cleanup`) for ordered teardown
- [ ] Implement generation-based reconcile gating (`observedGeneration == generation` check)
- [ ] Reconcile ConfigMap from `spec.config` redis.conf overrides
- [ ] Reconcile headless Service for StatefulSet DNS
- [ ] Reconcile StatefulSet (image version, replicas, volume mounts, resource limits)
- [ ] Implement status subresource updates (phase, conditions, readyReplicas, masterEndpoint)
- [ ] Emit Kubernetes Events on phase transitions

## Phase 3 — Topology and Scaling

- [ ] Implement Sentinel topology: reconcile Sentinel StatefulSet and Service when `spec.topology == sentinel`
- [ ] Implement scale-up path: increase `spec.replicas` and reconcile StatefulSet
- [ ] Implement scale-down path: cordon target replicas, drain replication lag, then reduce replicas
- [ ] Implement rolling restart trigger on `spec.redisVersion` or `spec.config` changes
- [ ] Add `sentinelEndpoints` population in status for sentinel topology

## Phase 4 — Security and RBAC

- [ ] Define least-privilege `ClusterRole` and `ClusterRoleBinding` manifests
- [ ] Implement Secret reference resolution for `spec.auth.passwordSecretRef`
- [ ] Implement optional TLS configuration via `spec.auth.tls` Secret reference
- [ ] Configure pod security context: non-root user, read-only root filesystem, dropped capabilities
- [ ] Write RBAC integration tests verifying controller cannot escalate privileges

## Phase 5 — HA Controller

- [ ] Configure leader election in controller manager (`leases.coordination.k8s.io`)
- [ ] Set controller deployment replicas to 2 in operator manifests
- [ ] Verify leader failover does not interrupt reconcile loop beyond one lease TTL

## Phase 6 — Observability

- [ ] Verify controller-runtime Prometheus metrics endpoint is exposed on port 8080
- [ ] Document `status.conditions` semantics: `Ready`, `Reconciling`, `Degraded`
- [ ] Add reconcile duration and error-rate metrics instrumentation

## Phase 7 — Testing and Validation

- [ ] Write integration tests using envtest for full reconcile loop (create, update, delete)
- [ ] Write integration tests for scale-up and scale-down paths
- [ ] Write integration tests for rolling restart triggered by version and config change
- [ ] Write integration tests for finalizer teardown ordering
- [ ] Run end-to-end tests against a real Kubernetes cluster (kind or equivalent)
- [ ] Validate CRD against Kubernetes API server with strict validation enabled

## Phase 8 — Documentation and Release

- [ ] Write operator installation guide (CRD install, RBAC, deployment)
- [ ] Write `RedisInstance` API reference documentation
- [ ] Write runbook for common operational tasks (scale, upgrade, teardown)
- [ ] Tag v0.1.0 release and publish operator container image
- [ ] Update this tasks.md to mark all completed items
