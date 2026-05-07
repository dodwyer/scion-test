# Tasks: create-redis-operator-1

## CRD / API

- [ ] Scaffold kubebuilder project (`kubebuilder init --domain redis.example.com`)
- [ ] Generate `Redis` API type (`kubebuilder create api --group redis --version v1alpha1 --kind Redis`)
- [ ] Define `RedisSpec` struct with all fields: `version`, `replicas`, `resources`, `persistence`, `auth`, `config`
- [ ] Define `RedisStatus` struct with `phase`, `readyReplicas`, `conditions`
- [ ] Add OpenAPI v3 validation markers (`+kubebuilder:validation:*`) to all spec fields
- [ ] Add CRD printer columns for `phase`, `readyReplicas`, `version`, `age`
- [ ] Add defaulting markers (`+kubebuilder:default`) for optional fields with defaults
- [ ] Run `make generate manifests` and commit generated CRD YAML to `config/crd/bases/`
- [ ] Write unit tests for API type defaulting and validation

## Controller

- [ ] Implement `ConfigMap` builder: render `redis.conf` from `spec.config` and auth settings
- [ ] Implement `Secret` builder: generate or reference auth secret
- [ ] Implement `StatefulSet` builder: pods, volume mounts, image tag from `spec.version`
- [ ] Implement ClusterIP `Service` builder
- [ ] Implement Headless `Service` builder
- [ ] Implement `createOrUpdate` helper using `controller-runtime` `controllerutil.CreateOrUpdate`
- [ ] Set `ownerReferences` on all owned resources via `controllerutil.SetControllerReference`
- [ ] Implement status update logic: compute `phase` and `readyReplicas` from StatefulSet status
- [ ] Implement `conditions` helpers: `Available`, `Reconciling`, `Degraded`
- [ ] Implement event emission (`recorder.Eventf`) for key lifecycle transitions
- [ ] Handle `DeletionTimestamp` and set `Terminating` phase
- [ ] Configure requeue interval (30 s) when `phase == Degraded`
- [ ] Enable leader election in `main.go`
- [ ] Wire Prometheus metrics: `redis_operator_ready_replicas`, `redis_operator_phase_transitions_total`
- [ ] Write unit tests for each builder function (ConfigMap, StatefulSet, Services, Secret)
- [ ] Write integration/e2e tests using `envtest` against a real CRD + controller

## RBAC

- [ ] Define `ClusterRole` with minimal permissions (all resource/verb pairs from design)
- [ ] Define `ClusterRoleBinding` binding operator `ServiceAccount` to `ClusterRole`
- [ ] Define dedicated `ServiceAccount` in operator namespace
- [ ] Add `+kubebuilder:rbac` markers in controller source and regenerate RBAC manifests
- [ ] Review generated RBAC for least-privilege compliance

## Deployment

- [ ] Write multi-stage `Dockerfile` (build stage: `golang`, runtime stage: distroless or UBI-minimal)
- [ ] Add `Makefile` targets: `docker-build`, `docker-push`, `deploy`, `undeploy`
- [ ] Create Kustomize base under `config/default/` (kubebuilder layout)
- [ ] Create Helm chart skeleton under `charts/redis-operator/` with `Chart.yaml`, `values.yaml`, templates for Deployment, CRD, RBAC, ServiceAccount
- [ ] Document image registry and tag conventions in `values.yaml`
- [ ] Set up GitHub Actions (or equivalent) CI pipeline: lint, test, build, push image, package Helm chart
- [ ] Add `PodDisruptionBudget` for operator HA

## Observability

- [ ] Expose `:8080/metrics` endpoint via controller-runtime default metrics server
- [ ] Register custom `redis_operator_ready_replicas` gauge
- [ ] Register custom `redis_operator_phase_transitions_total` counter
- [ ] Add `ServiceMonitor` (or `PodMonitor`) manifest for Prometheus Operator scraping
- [ ] Document log fields and log levels in operator README
- [ ] Validate event reasons match Kubernetes naming conventions

## Testing

- [ ] Unit tests: API builders achieve >80% coverage
- [ ] Integration tests: `envtest` suite covering happy path create/update/delete lifecycle
- [ ] Integration tests: persistence enabled/disabled paths
- [ ] Integration tests: auth enabled with external secret and with auto-generated secret
- [ ] Integration tests: config override merging
- [ ] Integration tests: status and conditions updated correctly after StatefulSet readiness change
- [ ] E2e smoke test script against a real cluster (kind or similar)

## Documentation

- [ ] Write operator `README.md`: overview, prerequisites, quick start, CRD field reference
- [ ] Document all `spec` fields with types, defaults, and examples
- [ ] Document status fields and condition meanings
- [ ] Add troubleshooting guide (common failure modes and remediation)
- [ ] Add `CONTRIBUTING.md` with dev setup, `make` targets, and test instructions
- [ ] Update this spec (`openspec/`) once any open questions are resolved
