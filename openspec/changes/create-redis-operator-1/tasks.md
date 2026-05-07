# Tasks: create-redis-operator-1

## CRD and API

- [ ] Define RedisInstance CRD schema with validation (OpenAPI v3 schema, CEL rules for replicas/topology constraints)
- [ ] Generate CRD manifest and register in API server

## Reconciler Implementation

- [ ] Implement reconciler for StatefulSet management (create, patch on spec change, rolling update)
- [ ] Implement reconciler for Service management (headless and ClusterIP services)
- [ ] Implement reconciler for ConfigMap management (render redis.conf from CR spec)
- [ ] Implement finalizer logic for cleanup (ordered teardown on CR deletion)
- [ ] Implement status condition updates (Ready, Degraded, observedGeneration, readyReplicas)

## HA / Sentinel

- [ ] Implement Sentinel HA mode reconciliation (multi-replica StatefulSet with Sentinel sidecar or co-located process)

## Testing

- [ ] Write unit tests for reconciler (mock client, table-driven cases for create/update/delete/scale)
- [ ] Write e2e tests for CR lifecycle (install CRD, create CR, assert pods ready, scale, delete)

## Deployment Manifests

- [ ] Write RBAC manifests (ClusterRole, ClusterRoleBinding, ServiceAccount for operator)
- [ ] Write operator Deployment manifest (image, leader-election flags, resource limits)

## Documentation

- [ ] Document CRD fields and usage (README with quickstart, CR example, field reference)
