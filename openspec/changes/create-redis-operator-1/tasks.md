# Tasks: Redis Kubernetes Operator

## Phase 1 — Foundation

- [x] Set up Go module and project scaffold using controller-runtime and kubebuilder markers
- [x] Define `RedisInstanceSpec` Go type fields and validation markers
- [x] Define `RedisInstanceStatus` Go type fields including `observedGeneration`
- [x] Generate CRD YAML manifests from Go types using controller-gen
- [x] Register the CRD scheme with the controller manager
- [x] Write unit tests covering required CRD fields
- [x] Write unit tests covering topology enum validation
- [x] Write unit tests covering storage-required validation

## Phase 2 — Core Controller

- [x] Implement `RedisInstanceReconciler` with controller-runtime `Reconcile` method
- [x] Add finalizer handling (`redis.example.io/cleanup`) for ordered teardown
- [x] Implement reconcile gating that skips mutation only when `observedGeneration == generation` and owned resources already exist
- [x] Reconcile ConfigMap name and ownership metadata
- [x] Render `redis.conf` contents from `spec.config`
- [x] Reconcile headless Service for StatefulSet DNS
- [x] Reconcile StatefulSet image version
- [x] Reconcile StatefulSet replica count
- [x] Reconcile StatefulSet volume mounts from the PVC template
- [x] Reconcile StatefulSet resource requests and limits
- [x] Update `status.phase`
- [x] Update `status.conditions`
- [x] Update `status.observedGeneration`
- [x] Update `status.readyReplicas`
- [x] Update `status.masterEndpoint`
- [x] Emit Kubernetes Events on phase transitions

## Phase 3 — Topology and Scaling

- [x] Reconcile Sentinel StatefulSet when `spec.topology == sentinel`
- [x] Reconcile Sentinel Service when `spec.topology == sentinel`
- [x] Skip Sentinel resources when `spec.topology == standalone`
- [x] Implement scale-up by increasing StatefulSet replicas
- [x] Implement scale-down by reducing StatefulSet replicas to the desired count
- [ ] Wait for higher-ordinal pods to terminate before completing scale-down reconciliation
- [x] Preserve the headless Service selector for remaining Redis pods after scale-down
- [x] Implement rolling restart trigger on `spec.redisVersion` changes
- [x] Implement rolling restart trigger on `spec.config` changes
- [x] Populate `status.sentinelEndpoints` for sentinel topology

## Phase 4 — Security and RBAC

- [x] Define least-privilege `ClusterRole` manifest
- [x] Define `ClusterRoleBinding` manifest
- [x] Implement Secret reference resolution for `spec.auth.passwordSecretRef`
- [x] Allow reconcile to proceed without auth when `spec.auth.passwordSecretRef` is unset
- [x] Implement optional TLS configuration via `spec.auth.tls` Secret reference
- [x] Configure pod security context to run as non-root
- [x] Configure pod security context with a read-only root filesystem
- [x] Configure pod security context with dropped Linux capabilities
- [ ] Write RBAC integration tests verifying controller cannot escalate privileges

## Phase 5 — HA Controller

- [x] Configure leader election in controller manager (`leases.coordination.k8s.io`)
- [x] Set controller deployment replicas to 2 in operator manifests
- [ ] Verify leader failover does not interrupt reconcile loop beyond one lease TTL

## Phase 6 — Observability

- [x] Verify controller-runtime Prometheus metrics endpoint is exposed on port 8080
- [x] Document `status.conditions` semantics for `Ready`
- [x] Document `status.conditions` semantics for `Reconciling`
- [x] Document `status.conditions` semantics for `Degraded`
- [ ] Add reconcile duration metrics instrumentation
- [ ] Add reconcile error-rate metrics instrumentation

## Phase 7 — Testing and Validation

- [x] Write envtest coverage for create reconciliation
- [x] Write envtest coverage for update reconciliation
- [x] Write envtest coverage for delete reconciliation
- [x] Write integration tests for scale-up
- [x] Write integration tests for scale-down
- [x] Write integration tests for rolling restart on version change
- [x] Write integration tests for rolling restart on config change
- [x] Write integration tests for finalizer teardown ordering
- [ ] Run end-to-end tests against a real Kubernetes cluster (kind or equivalent)
- [ ] Validate CRD against Kubernetes API server with strict validation enabled

## Phase 8 — Documentation and Release

- [ ] Write operator installation guide for CRD installation
- [ ] Write operator installation guide for RBAC setup
- [ ] Write operator installation guide for controller deployment
- [ ] Write `RedisInstance` API reference for spec fields
- [ ] Write `RedisInstance` API reference for status fields
- [ ] Write runbook for scale operations
- [ ] Write runbook for upgrade operations
- [ ] Write runbook for teardown operations
- [ ] Tag v0.1.0 release and publish operator container image
- [x] Update this tasks.md to mark all completed items
