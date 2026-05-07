# Tasks: Redis Operator

## CRD and API

- [ ] Define `Redis` CRD schema with kubebuilder markers
- [ ] Generate CRD YAML manifest via `controller-gen`
- [ ] Define `RedisSpec` struct with all specified fields
- [ ] Define `RedisStatus` struct with phase, readyReplicas, conditions
- [ ] Add validation markers (required fields, enum values, min/max)
- [ ] Register `Redis` kind with scheme

## Controller

- [ ] Scaffold controller with kubebuilder
- [ ] Implement ConfigMap sub-reconciler (generate redis.conf)
- [ ] Implement Secret sub-reconciler (auth password)
- [ ] Implement StatefulSet sub-reconciler (create/update)
- [ ] Implement ClusterIP Service sub-reconciler
- [ ] Implement Headless Service sub-reconciler
- [ ] Implement PVC sub-reconciler (if persistence enabled)
- [ ] Implement status update logic (phase, readyReplicas, conditions)
- [ ] Set ownerReferences on all owned resources
- [ ] Handle reconcile errors with requeue
- [ ] Add Reconciling/Available/Degraded condition management

## RBAC

- [ ] Define ClusterRole with required resource permissions
- [ ] Define ClusterRoleBinding for operator ServiceAccount
- [ ] Define ServiceAccount for operator Deployment
- [ ] Add kubebuilder RBAC markers to controller

## Deployment

- [ ] Write operator Deployment manifest
- [ ] Configure leader election via manager options
- [ ] Define resource requests/limits for operator container
- [ ] Write Namespace manifest for `redis-system`
- [ ] Write Kustomize base or Helm chart skeleton
- [ ] Write operator Dockerfile (distroless base image, compiled Go binary)

## Observability

- [ ] Confirm Prometheus metrics endpoint enabled (`:8080/metrics`)
- [ ] Add structured logging at key reconcile steps
- [ ] Emit Kubernetes Events on create/update/delete transitions

## Testing

- [ ] Write unit tests for ConfigMap generation logic
- [ ] Write unit tests for StatefulSet spec generation
- [ ] Write integration tests (envtest) for full reconcile lifecycle
- [ ] Test create Redis CR → resources appear
- [ ] Test update Redis CR version → StatefulSet image updated
- [ ] Test delete Redis CR → owned resources removed
- [ ] Test persistence enabled → PVC created
- [ ] Test auth enabled → Secret created and mounted

## Documentation

- [ ] Write README with quickstart and CR reference
- [ ] Document all spec fields with descriptions and defaults
- [ ] Provide example `Redis` CR manifests (minimal, full)
