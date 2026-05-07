# Tasks: Redis Kubernetes Operator

## Phase 1 — Foundation

- [ ] Set up Go module and project scaffold using controller-runtime and kubebuilder markers
- [ ] Define `RedisInstanceSpec` Go type fields and validation markers
- [ ] Define `RedisInstanceStatus` Go type fields including `observedGeneration`
- [ ] Generate CRD YAML manifests from Go types using controller-gen
- [ ] Register the CRD scheme with the controller manager
- [ ] Write unit tests covering required CRD fields
- [ ] Write unit tests covering topology enum validation
- [ ] Write unit tests covering storage-required validation

## Phase 2 — Core Controller

- [ ] Implement `RedisInstanceReconciler` with controller-runtime `Reconcile` method
- [ ] Add finalizer handling (`redis.example.io/cleanup`) for ordered teardown
- [ ] Implement reconcile gating that skips mutation only when `observedGeneration == generation` and owned resources already exist
- [ ] Reconcile ConfigMap name and ownership metadata
- [ ] Render `redis.conf` contents from `spec.config`
- [ ] Reconcile headless Service for StatefulSet DNS
- [ ] Reconcile StatefulSet image version
- [ ] Reconcile StatefulSet replica count
- [ ] Reconcile StatefulSet volume mounts from the PVC template
- [ ] Reconcile StatefulSet resource requests and limits
- [ ] Update `status.phase`
- [ ] Update `status.conditions`
- [ ] Update `status.observedGeneration`
- [ ] Update `status.readyReplicas`
- [ ] Update `status.masterEndpoint`
- [ ] Emit Kubernetes Events on phase transitions

## Phase 3 — Topology and Scaling

- [ ] Reconcile Sentinel StatefulSet when `spec.topology == sentinel`
- [ ] Reconcile Sentinel Service when `spec.topology == sentinel`
- [ ] Skip Sentinel resources when `spec.topology == standalone`
- [ ] Implement scale-up by increasing StatefulSet replicas
- [ ] Implement scale-down by reducing StatefulSet replicas to the desired count
- [ ] Wait for higher-ordinal pods to terminate before completing scale-down reconciliation
- [ ] Preserve the headless Service selector for remaining Redis pods after scale-down
- [ ] Implement rolling restart trigger on `spec.redisVersion` changes
- [ ] Implement rolling restart trigger on `spec.config` changes
- [ ] Populate `status.sentinelEndpoints` for sentinel topology

## Phase 4 — Security and RBAC

- [ ] Define least-privilege `ClusterRole` manifest
- [ ] Define `ClusterRoleBinding` manifest
- [ ] Implement Secret reference resolution for `spec.auth.passwordSecretRef`
- [ ] Allow reconcile to proceed without auth when `spec.auth.passwordSecretRef` is unset
- [ ] Implement optional TLS configuration via `spec.auth.tls` Secret reference
- [ ] Configure pod security context to run as non-root
- [ ] Configure pod security context with a read-only root filesystem
- [ ] Configure pod security context with dropped Linux capabilities
- [ ] Write RBAC integration tests verifying controller cannot escalate privileges

## Phase 5 — HA Controller

- [ ] Configure leader election in controller manager (`leases.coordination.k8s.io`)
- [ ] Set controller deployment replicas to 2 in operator manifests
- [ ] Verify leader failover does not interrupt reconcile loop beyond one lease TTL

## Phase 6 — Observability

- [ ] Verify controller-runtime Prometheus metrics endpoint is exposed on port 8080
- [ ] Document `status.conditions` semantics for `Ready`
- [ ] Document `status.conditions` semantics for `Reconciling`
- [ ] Document `status.conditions` semantics for `Degraded`
- [ ] Add reconcile duration metrics instrumentation
- [ ] Add reconcile error-rate metrics instrumentation

## Phase 7 — Testing and Validation

- [ ] Write envtest coverage for create reconciliation
- [ ] Write envtest coverage for update reconciliation
- [ ] Write envtest coverage for delete reconciliation
- [ ] Write integration tests for scale-up
- [ ] Write integration tests for scale-down
- [ ] Write integration tests for rolling restart on version change
- [ ] Write integration tests for rolling restart on config change
- [ ] Write integration tests for finalizer teardown ordering
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
- [ ] Update this tasks.md to mark all completed items
