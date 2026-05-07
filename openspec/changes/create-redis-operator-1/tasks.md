# Tasks: Redis Kubernetes Operator Implementation

## Phase 1 — Project Scaffolding

- [ ] Initialise Go module (`go mod init`)
- [ ] Scaffold project with kubebuilder (`kubebuilder init --domain redis.example.com`)
- [ ] Create `Redis` API type (`kubebuilder create api --group redis --version v1alpha1 --kind Redis`)
- [ ] Define `RedisSpec` struct with all fields from design.md
- [ ] Define `RedisStatus` struct with `Phase`, `ReadyReplicas`, `Conditions`, `ObservedGeneration`
- [ ] Run `make generate` to produce DeepCopy methods
- [ ] Run `make manifests` to generate CRD YAML from kubebuilder markers
- [ ] Add validation markers (`+kubebuilder:validation:*`) to spec fields
- [ ] Commit scaffolding and generated CRD manifests

## Phase 2 — Controller Core

- [ ] Implement `Reconcile()` entrypoint in `internal/controller/redis_controller.go`
- [ ] Implement finalizer registration and removal logic
- [ ] Implement `reconcileConfigMap()` — build `redis.conf` from spec defaults and `spec.config` overrides
- [ ] Implement `reconcileStatefulSet()` — create/update StatefulSet with correct pod template
- [ ] Implement `reconcileHeadlessService()` — create/update headless Service
- [ ] Implement `reconcileClientService()` — create/update client-facing Service
- [ ] Implement `reconcilePVCs()` — add VolumeClaimTemplates when `persistence.enabled`
- [ ] Implement `updateStatus()` — derive phase and conditions from StatefulSet status; patch CR
- [ ] Add owner references to all owned resources
- [ ] Add requeue logic for transient errors and not-yet-ready StatefulSets
- [ ] Configure RBAC markers on the controller for all required permissions
- [ ] Run `make manifests` to regenerate RBAC ClusterRole YAML

## Phase 3 — Master-Replica Support

- [ ] Add init container or startup script logic to inject `replicaof` directive for pods with ordinal > 0
- [ ] Render replica configuration into ConfigMap conditionally when `spec.replicas > 1`
- [ ] Verify pod-0 is always designated master during reconcile
- [ ] Update status phase to `Degraded` when expected replicas are not all ready

## Phase 4 — Health Checks

- [ ] Add liveness probe (`redis-cli ping`, initial delay 15s, period 20s) to pod template
- [ ] Add readiness probe (`redis-cli ping`, initial delay 5s, period 10s) to pod template
- [ ] Validate probes work against a live Redis pod in a local cluster

## Phase 5 — Helm Chart

- [ ] Create `charts/redis-operator/` directory structure
- [ ] Write `Chart.yaml` with name, version, and appVersion
- [ ] Write `values.yaml` with default operator image, replicas, and RBAC flags
- [ ] Template `Deployment` manifest for the operator
- [ ] Template `ServiceAccount`, `ClusterRole`, `ClusterRoleBinding`
- [ ] Template `CustomResourceDefinition` (embed or reference generated CRD YAML)
- [ ] Add `NOTES.txt` with post-install usage instructions
- [ ] Verify `helm lint` passes with no errors
- [ ] Verify `helm template` renders valid Kubernetes YAML

## Phase 6 — Kustomize Overlay

- [ ] Create `config/default/kustomization.yaml` (kubebuilder default layout)
- [ ] Ensure `config/crd/`, `config/rbac/`, `config/manager/` directories are complete
- [ ] Add namespace overlay example for single-namespace deployment mode
- [ ] Verify `kubectl kustomize config/default` renders without errors

## Phase 7 — Testing

- [ ] Set up `envtest` in `internal/controller/suite_test.go`
- [ ] Write table-driven reconcile test: standalone Redis CR (replicas=1, persistence=false)
- [ ] Write reconcile test: replica Redis CR (replicas=3)
- [ ] Write reconcile test: persistence enabled CR
- [ ] Write reconcile test: CR deletion triggers finalizer cleanup
- [ ] Write reconcile test: invalid spec fields are rejected by webhook or validation
- [ ] Verify `make test` passes with race detector enabled (`-race`)
- [ ] (Optional) Add kind-based integration test in `test/e2e/`

## Phase 8 — CI / Release

- [ ] Add GitHub Actions workflow: lint, test, build image, helm lint
- [ ] Tag image with semantic version on release
- [ ] Publish Helm chart to OCI registry or GitHub Pages chart repository
- [ ] Write `CHANGELOG.md` entry for v0.1.0
