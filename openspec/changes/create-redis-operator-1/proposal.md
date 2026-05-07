# Proposal: create-redis-operator-1

## Change Name

`create-redis-operator-1`

## Goal

Introduce a Kubernetes Operator that manages the full lifecycle of single-instance Redis deployments via a custom `Redis` Custom Resource Definition (CRD) at `redis.example.com/v1alpha1`.

## Motivation

Operating Redis on Kubernetes manually requires maintaining StatefulSets, Services, ConfigMaps, Secrets, and PersistentVolumeClaims in a consistent and coordinated way. A Kubernetes Operator encodes this operational knowledge into a reconciliation loop, enabling declarative Redis management with automatic day-2 operations such as configuration updates, rolling restarts, and status reporting.

## Scope

### In Scope (v1alpha1)

- `Redis` CRD at group `redis.example.com`, version `v1alpha1`, scope `Namespaced`
- Controller built with kubebuilder v3+ / controller-runtime v0.15+, written in Go
- Owned resources reconciled by the controller:
  - `StatefulSet` — Redis pod management
  - ClusterIP `Service` — stable client endpoint
  - Headless `Service` — pod DNS identity
  - `ConfigMap` — rendered `redis.conf`
  - `Secret` — optional auth token
  - `PersistentVolumeClaim` — optional persistence (bound via StatefulSet volumeClaimTemplates)
- Status reporting: `phase`, `readyReplicas`, `conditions`
- RBAC: `ClusterRole` + `ClusterRoleBinding` + dedicated `ServiceAccount`
- Observability: Prometheus metrics on `:8080/metrics`, structured logs, Kubernetes Events
- Leader election for HA operator deployments

### Out of Scope

- Redis Sentinel topology
- Redis Cluster (sharded) topology
- Backup and restore
- TLS termination at the Redis protocol layer
- Multi-cluster federation

## Assumptions

1. Target clusters run Kubernetes 1.24 or later.
2. CRD validation is enforced via OpenAPI v3 schemas embedded in the CRD manifest.
3. The operator image will be built from a standard multi-stage Dockerfile (distroless or UBI-minimal base).
4. Default resource requests/limits for Redis pods will be defined in the CRD defaulting webhook or controller defaults until explicitly set by the user.
5. A single operator deployment manages `Redis` resources cluster-wide (all namespaces) unless scoped differently at deploy time.

## Open Questions

1. **Namespace watch scope** — Should the operator watch all namespaces (cluster-wide) or only the namespace it is deployed in? Cluster-wide is the typical default but requires `ClusterRole`; namespace-scoped reduces blast radius.
2. **Container registry** — Where will the operator image be published? This affects the default `image` reference in Helm/Kustomize and any image pull secret requirements.
3. **Primary deployment mechanism** — Should the operator ship as a Helm chart, a Kustomize base, or both? This influences how CRD installation and upgrades are handled.
4. **Redis Sentinel/Cluster in v1alpha1** — Should the `spec` include reserved (but unimplemented) topology fields now to avoid a breaking schema change later, or keep the schema strictly minimal?
5. **Default resource requests/limits** — What CPU and memory values should the controller use when `spec.resources` is omitted? (e.g., `100m`/`128Mi` requests, `500m`/`512Mi` limits)
