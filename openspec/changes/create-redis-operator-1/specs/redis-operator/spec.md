# Spec: Redis Operator

**Change**: create-redis-operator-1
**API Group**: redis.example.io/v1alpha1
**Kind**: RedisInstance

---

## ADDED Requirements

### Requirement: CRD Schema

The `RedisInstance` CustomResourceDefinition MUST be installable into a Kubernetes cluster and MUST enforce structural validation on all CR instances.

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

#### Scenario: CR status fields are initialized on creation

**Given** the CRD is installed and the operator is running
**When** a valid `RedisInstance` CR is created
**Then** the operator patches `.status.conditions` with an initial `Ready: False` condition
**And** `.status.observedGeneration` is set to the CR's `.metadata.generation`
**And** `.status.readyReplicas` is set to `0`

---

### Requirement: Reconciliation Loop

The operator reconciler MUST react to all CR lifecycle events and converge owned resources toward the desired state declared in the CR spec.

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

#### Scenario: Ready condition is set when all replicas are healthy

**Given** a `RedisInstance` CR with `spec.replicas: 3`
**When** all 3 StatefulSet pods report `Ready: True`
**Then** the operator patches `.status.conditions` with a condition `type: Ready, status: True`
**And** `.status.readyReplicas` equals `3`
**And** no `Degraded` condition with `status: True` is present

#### Scenario: Degraded condition is set when replicas are unavailable

**Given** a `RedisInstance` CR with `spec.replicas: 3`
**When** one or more pods become `Ready: False` (e.g. crash loop or OOMKill)
**Then** the operator patches `.status.conditions` with a condition `type: Degraded, status: True`
**And** the condition `message` field contains a human-readable description of the failure
**And** `.status.readyReplicas` reflects the actual count of ready pods

---

### Requirement: Sentinel HA Mode

When `spec.topology` is set to `sentinel`, the operator MUST configure the Redis deployment for high-availability with automatic primary failover.

#### Scenario: Sentinel topology deploys multi-replica StatefulSet with failover

**Given** a `RedisInstance` CR with `spec.topology: sentinel` and `spec.replicas: 3`
**When** the reconciler runs
**Then** a StatefulSet with 3 replicas is created
**And** each pod includes a Redis Sentinel process (co-located container or sidecar)
**And** Sentinel processes are configured to monitor the primary and trigger failover if the primary is unreachable for more than the configured threshold
**And** after a primary failure and failover, the ClusterIP Service is updated (or Sentinel DNS is used) so clients can reach the new primary
