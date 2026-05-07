# Tasks: create-redis-operator-1

## CRD and API

- [ ] Define `spec.image` as a required string field
- [ ] Define `spec.replicas` with default `1` and validation range `1-6`
- [ ] Define optional `spec.resources` as Kubernetes `ResourceRequirements`
- [ ] Define `spec.storage.size` with default `1Gi`
- [ ] Define optional `spec.storage.storageClassName`
- [ ] Define optional `spec.auth.secretName`
- [ ] Define `spec.auth.passwordKey` with default `redis-password`
- [ ] Define `spec.topology` enum values `standalone` and `sentinel` with default `standalone`
- [ ] Define `spec.version` with default `7.2` and minimum supported Redis version `7.2+`
- [ ] Add validation rules for `standalone` requiring `replicas=1`
- [ ] Add validation rules for `sentinel` requiring odd replicas greater than or equal to `3`
- [ ] Generate the CRD manifest from the API types
- [ ] Register the CRD in the target cluster or installation bundle

## Reconciler Implementation

- [ ] Register finalizer `redis.example.io/cleanup` on non-deleting `RedisInstance` resources
- [ ] Render `redis.conf` into a ConfigMap from CR fields
- [ ] Render Sentinel configuration into a ConfigMap when `spec.topology=sentinel`
- [ ] Create the StatefulSet when it does not exist
- [ ] Patch the StatefulSet when image, replica count, storage, or resource settings change
- [ ] Inject Redis authentication through environment variables using `secretKeyRef`
- [ ] Use Secret key `redis-password` by default and honor `spec.auth.passwordKey` override
- [ ] Create the headless Service for stable pod DNS
- [ ] Create the client-facing ClusterIP Service
- [ ] Update the ClusterIP Service selector to the current primary in sentinel mode
- [ ] Delete the ClusterIP Service during finalizer cleanup
- [ ] Delete the headless Service during finalizer cleanup
- [ ] Delete the ConfigMap during finalizer cleanup
- [ ] Delete the StatefulSet during finalizer cleanup
- [ ] Remove the finalizer after owned resources are cleaned up
- [ ] Patch status `observedGeneration` after successful reconciliation
- [ ] Patch status `readyReplicas` from StatefulSet readiness
- [ ] Set condition `Ready=True` only when all desired replicas are healthy
- [ ] Set condition `Degraded=True` when one or more replicas are unavailable
- [ ] Populate standard condition `reason` and `message` fields

## HA / Sentinel

- [ ] Add a co-located sidecar container named `sentinel` to each pod in sentinel mode
- [ ] Configure Sentinel sidecars to monitor the active primary
- [ ] Watch Sentinel state changes relevant to primary failover
- [ ] Repoint the client-facing Service after primary failover

## Testing

- [ ] Write unit tests for CRD validation rules
- [ ] Write unit tests for ConfigMap rendering
- [ ] Write unit tests for Secret-to-env-var pod rendering
- [ ] Write unit tests for StatefulSet reconciliation on create
- [ ] Write unit tests for StatefulSet reconciliation on update
- [ ] Write unit tests for Service reconciliation
- [ ] Write unit tests for finalizer registration and cleanup ordering
- [ ] Write unit tests for status condition transitions
- [ ] Write e2e tests for CR creation and initial readiness
- [ ] Write e2e tests for scaling replicas
- [ ] Write e2e tests for CR deletion and finalizer cleanup
- [ ] Write e2e tests for sentinel failover and Service selector updates

## Deployment Manifests

- [ ] Write the operator `ServiceAccount` manifest
- [ ] Write the operator `ClusterRole` manifest
- [ ] Write the operator `ClusterRoleBinding` manifest
- [ ] Write the operator Deployment manifest
- [ ] Set leader-election flags in the operator Deployment
- [ ] Set operator resource requests and limits in the Deployment

## Documentation

- [ ] Document CRD fields and defaults
- [ ] Document standalone deployment usage
- [ ] Document sentinel deployment usage
- [ ] Document Secret requirements for authentication
- [ ] Document finalizer and PVC retention behavior
