# Spec: Redis Operator

**Change**: create-redis-operator-1
**API Group**: redis.example.io/v1alpha1
**Kind**: RedisInstance

---

## ADDED Requirements

### Requirement: CRD Schema

The `RedisInstance` CustomResourceDefinition MUST be installable into a Kubernetes cluster and MUST enforce structural validation on all CR instances.

The CRD schema MUST define the following fields and behaviors:
- `spec.image`: required string. It identifies the Redis image and MUST reference Redis `7.2+`.
- `spec.replicas`: integer with default `1`. Valid values are `1` through `6`.
- `spec.resources`: optional Kubernetes `ResourceRequirements`.
- `spec.storage.size`: string with default `1Gi`.
- `spec.storage.storageClassName`: optional string.
- `spec.auth.secretName`: optional string naming a Secret in the same namespace.
- `spec.auth.passwordKey`: string with default `redis-password`.
- `spec.topology`: enum `standalone|sentinel` with default `standalone`.
- `spec.version`: string with default `7.2`. Minimum supported Redis version is `7.2`.

The CRD validation rules MUST enforce:
- `spec.topology=standalone` requires `spec.replicas=1`.
- `spec.topology=sentinel` requires an odd replica count greater than or equal to `3`; given the global `1-6` range, valid sentinel replica counts are `3` and `5`.
- If `spec.auth.secretName` is provided, the referenced Secret is expected to contain the key named by `spec.auth.passwordKey`, which defaults to `redis-password`.

#### Scenario: CRD installs cleanly

**Given** a Kubernetes cluster (v1.25+) with no prior `redis.example.io` CRD installed
**When** the operator's CRD manifest is applied with `kubectl apply`
**Then** the CRD resource appears in `kubectl get crds` with `ESTABLISHED: True`
**And** no admission webhook errors are returned

#### Scenario: CR field validation rejects invalid replicas

**Given** the CRD is installed
**When** a user creates a `RedisInstance` CR with `spec.topology: sentinel` and `spec.replicas: 2`
**Then** the API server rejects the request with a 422 Unprocessable Entity status
**And** the error message references the replicas constraint (sentinel requires an odd number ≥ 3)

#### Scenario: CRD defaults are applied to omitted optional fields

**Given** the CRD is installed
**When** a user creates a `RedisInstance` CR with only `spec.image` set to a Redis `7.2+` image
**Then** the stored resource defaults `spec.replicas` to `1`
**And** defaults `spec.storage.size` to `1Gi`
**And** defaults `spec.auth.passwordKey` to `redis-password`
**And** defaults `spec.topology` to `standalone`
**And** defaults `spec.version` to `7.2`

#### Scenario: CRD rejects unsupported Redis versions

**Given** the CRD is installed
**When** a user creates a `RedisInstance` CR with `spec.version: "7.0"`
**Then** the API server rejects the request
**And** the validation error states that the minimum supported Redis version is `7.2`

#### Scenario: CR status fields are initialized on creation

**Given** the CRD is installed and the operator is running
**When** a valid `RedisInstance` CR is created
**Then** the operator patches `.status.conditions` with an initial `Ready: False` condition
**And** `.status.observedGeneration` is set to the CR's `.metadata.generation`
**And** `.status.readyReplicas` is set to `0`

---

### Requirement: Reconciliation Loop

The operator reconciler MUST react to all CR lifecycle events and converge owned resources toward the desired state declared in the CR spec.

#### Scenario: Reconciler registers the finalizer before managing resources

**Given** a valid `RedisInstance` CR exists without `redis.example.io/cleanup` in `.metadata.finalizers`
**When** the reconciler processes the CR and `.metadata.deletionTimestamp` is not set
**Then** the reconciler adds `redis.example.io/cleanup` to `.metadata.finalizers`
**And** subsequent reconciliation proceeds with managed resource creation and updates

#### Scenario: Reconciler runs on CR create

**Given** the operator is running and the CRD is installed
**When** a new `RedisInstance` CR is created
**Then** the reconciler is invoked within 5 seconds
**And** all owned resources (StatefulSet, headless Service, ClusterIP Service, ConfigMap) are created in the same namespace
**And** each owned resource carries an `ownerReference` pointing to the `RedisInstance` CR

#### Scenario: Reconciler runs on CR update

**Given** a `RedisInstance` CR exists with `spec.image: redis:7.0`
**When** the CR is patched to `spec.image: redis:7.2`
**Then** the reconciler is invoked
**And** the owned StatefulSet's pod template image is updated to `redis:7.2`
**And** Kubernetes initiates a rolling update of the StatefulSet pods

#### Scenario: Reconciler handles CR delete via finalizer

**Given** a `RedisInstance` CR exists with finalizer `redis.example.io/cleanup`
**When** `kubectl delete` is issued for the CR
**Then** the API server sets `.metadata.deletionTimestamp` and does not remove the CR
**And** the reconciler detects the non-zero `deletionTimestamp` and runs finalizer cleanup
**And** after all owned resources are deleted, the finalizer is removed from the CR
**And** the CR is garbage-collected by the API server

---

### Requirement: StatefulSet Management

The operator MUST create and maintain a StatefulSet that matches the replica count, image, and resource configuration declared in the CR spec.

#### Scenario: StatefulSet creation

**Given** no StatefulSet exists for the `RedisInstance`
**When** the reconciler runs for a new CR with `spec.replicas: 1` and `spec.image: redis:7.2`
**Then** a StatefulSet is created with `spec.replicas: 1`
**And** the pod template's container image is `redis:7.2`
**And** `volumeClaimTemplates` include one PVC per replica sized per `spec.storage`
**And** the pod template includes environment variable injection from `secretKeyRef` when `spec.auth.secretName` is set
**And** the default Secret key consumed is `redis-password` unless `spec.auth.passwordKey` overrides it

#### Scenario: Scaling up replicas

**Given** a `RedisInstance` CR with `spec.replicas: 1` and a running StatefulSet
**When** the CR is updated to `spec.replicas: 3`
**Then** the reconciler patches the StatefulSet to `spec.replicas: 3`
**And** Kubernetes provisions two additional pods and PVCs
**And** `.status.readyReplicas` increases to `3` once all pods are ready

#### Scenario: Rolling update on image change

**Given** a `RedisInstance` CR with `spec.replicas: 3` and all pods running `redis:7.0`
**When** the CR is updated to `spec.image: redis:7.2`
**Then** the StatefulSet pod template is updated
**And** Kubernetes performs a rolling update, restarting one pod at a time
**And** no more than one pod is unavailable at any point during the rollout

---

### Requirement: ConfigMap Rendering

The operator MUST render a ConfigMap for Redis runtime configuration from the `RedisInstance` spec and keep it synchronized with supported CR field changes.

#### Scenario: Redis ConfigMap is rendered from CR fields

**Given** a `RedisInstance` CR with `spec.version`, `spec.storage`, and `spec.topology` set
**When** the reconciler runs
**Then** it creates or patches a ConfigMap in the same namespace
**And** the ConfigMap contains rendered Redis configuration derived from those fields
**And** the StatefulSet mounts or references that ConfigMap so pod configuration matches the latest desired state

#### Scenario: Sentinel ConfigMap content is rendered for sentinel topology

**Given** a `RedisInstance` CR with `spec.topology: sentinel` and `spec.replicas: 3`
**When** the reconciler runs
**Then** the rendered configuration includes Sentinel-specific settings for quorum monitoring
**And** each pod's sidecar container named `sentinel` consumes the rendered Sentinel configuration

---

### Requirement: Secret Authentication Consumption

The operator MUST consume Redis authentication credentials through pod environment variable injection using `secretKeyRef`.

#### Scenario: Secret-based auth is injected through environment variables

**Given** a `RedisInstance` CR with `spec.auth.secretName: redis-auth`
**And** the `redis-auth` Secret exists in the same namespace with key `redis-password`
**When** the reconciler renders the StatefulSet
**Then** the Redis container pod spec includes environment variable injection via `secretKeyRef`
**And** the referenced Secret name is `redis-auth`
**And** the referenced Secret key is `redis-password`

#### Scenario: Custom password key overrides the default secret key name

**Given** a `RedisInstance` CR with `spec.auth.secretName: redis-auth` and `spec.auth.passwordKey: operator-password`
**And** the `redis-auth` Secret exists in the same namespace with key `operator-password`
**When** the reconciler renders the StatefulSet
**Then** the Redis container uses `secretKeyRef` to read key `operator-password`
**And** the operator does not require the default key name for that CR instance

---

### Requirement: Service Management

The operator MUST create and maintain both a headless Service and a ClusterIP Service for each `RedisInstance`.

#### Scenario: Headless Service provides stable DNS

**Given** a `RedisInstance` CR is reconciled successfully
**Then** a headless Service (`.spec.clusterIP: None`) exists in the same namespace
**And** each StatefulSet pod is reachable at `<pod-name>.<svc-name>.<namespace>.svc.cluster.local`

#### Scenario: ClusterIP Service provides stable client endpoint

**Given** a `RedisInstance` CR is reconciled successfully
**Then** a ClusterIP Service exists in the same namespace
**And** the service selector targets pods with the label `app.kubernetes.io/instance: <cr-name>`
**And** clients can connect to Redis via the ClusterIP Service on port 6379

#### Scenario: ClusterIP Service follows the current primary in sentinel mode

**Given** a `RedisInstance` CR with `spec.topology: sentinel`
**And** a Sentinel failover changes the current primary from one pod to another
**When** the operator's Sentinel watcher observes the new primary
**Then** the operator updates the ClusterIP Service selector
**And** the client-facing Service points to the current primary pod after reconciliation

---

### Requirement: Finalizer Cleanup

The operator MUST use a finalizer to ensure all owned resources are deleted before the `RedisInstance` CR is removed, preventing orphaned resources.

#### Scenario: All owned resources are deleted on CR deletion

**Given** a `RedisInstance` CR with finalizer `redis.example.io/cleanup`
**And** owned resources: StatefulSet, headless Service, ClusterIP Service, ConfigMap
**When** the CR deletion is triggered
**Then** the reconciler deletes the ClusterIP Service
**And** deletes the headless Service
**And** deletes the ConfigMap
**And** deletes the StatefulSet (triggering pod termination)
**And** removes the `redis.example.io/cleanup` finalizer from the CR
**And** PVCs created via `volumeClaimTemplates` are NOT deleted (data preservation)

---

### Requirement: Status Conditions

The operator MUST reflect the current health of the Redis cluster in the CR `.status.conditions` field using standard Kubernetes condition types.

The status contract is:
- `Ready=True` means all desired replicas are healthy.
- `Degraded=True` means one or more desired replicas are unavailable.
- `Ready=True` and `Degraded=True` MAY coexist during partial degradation reporting windows.
- Each condition entry MUST use standard Kubernetes `reason` and `message` fields.

#### Scenario: Ready condition is set when all replicas are healthy

**Given** a `RedisInstance` CR with `spec.replicas: 3`
**When** all 3 StatefulSet pods report `Ready: True`
**Then** the operator patches `.status.conditions` with a condition `type: Ready, status: True`
**And** `.status.readyReplicas` equals `3`
**And** the `Ready` condition includes non-empty `reason` and `message` fields

#### Scenario: Degraded condition is set when replicas are unavailable

**Given** a `RedisInstance` CR with `spec.replicas: 3`
**When** one or more pods become `Ready: False` (e.g. crash loop or OOMKill)
**Then** the operator patches `.status.conditions` with a condition `type: Degraded, status: True`
**And** the condition `message` field contains a human-readable description of the failure
**And** the condition `reason` field contains a machine-meaningful summary of the failure class
**And** `.status.readyReplicas` reflects the actual count of ready pods

#### Scenario: Ready and Degraded can coexist during partial degradation

**Given** a `RedisInstance` CR was previously fully healthy
**And** the deployment remains available for client traffic
**When** one replica in a multi-replica deployment becomes unavailable
**Then** the operator MAY report `type: Ready, status: True`
**And** it also reports `type: Degraded, status: True`
**And** both conditions include standard `reason` and `message` fields describing the current state

---

### Requirement: Sentinel HA Mode

When `spec.topology` is set to `sentinel`, the operator MUST configure the Redis deployment for high-availability with automatic primary failover.

#### Scenario: Sentinel topology deploys multi-replica StatefulSet with failover

**Given** a `RedisInstance` CR with `spec.topology: sentinel` and `spec.replicas: 3`
**When** the reconciler runs
**Then** a StatefulSet with 3 replicas is created
**And** each pod includes a co-located sidecar container named `sentinel`
**And** Sentinel processes are configured to monitor the primary and trigger failover if the primary is unreachable for more than the configured threshold
**And** after a primary failure and failover, the operator's Sentinel watcher updates the ClusterIP Service selector so clients can reach the new primary
