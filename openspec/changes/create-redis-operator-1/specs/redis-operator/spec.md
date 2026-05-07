# Spec: Redis Operator

**Component:** redis-operator  
**API group:** `redis.example.io/v1alpha1`  
**Kind:** `RedisInstance`  
**Status:** Draft

---

## ADDED Requirements

### Requirement: CRD Schema

The operator MUST define a `RedisInstance` Custom Resource Definition under API group `redis.example.io/v1alpha1` with the schema described in this section.

#### Scenario: Minimal standalone instance creation

Given a `RedisInstance` CR with `spec.replicas: 1`, `spec.redisVersion: "7.2"`, `spec.storage` set with a valid PVC template, and `spec.topology: standalone`,  
When the operator processes the CR,  
Then the operator creates a StatefulSet with one pod, a headless Service, and a ConfigMap, and sets `status.phase` to `Running` once the pod is ready.

#### Scenario: Invalid topology value rejected

Given a `RedisInstance` CR with `spec.topology` set to an unsupported value (e.g., `"cluster"`),  
When the CR is submitted to the Kubernetes API server,  
Then the API server rejects it with a validation error before the operator receives it.

#### Scenario: Missing required storage field rejected

Given a `RedisInstance` CR with no `spec.storage` field,  
When the CR is submitted to the Kubernetes API server,  
Then the API server rejects it with a validation error indicating `spec.storage` is required.

---

### Requirement: Reconcile Loop Idempotency

The reconcile loop MUST be idempotent: re-running it on any event MUST produce the same owned-resource state without side effects.

#### Scenario: Repeated reconcile produces no drift

Given a `RedisInstance` CR that has already been fully reconciled (status.phase is `Running`),  
When the operator receives a no-op watch event and runs reconcile again,  
Then no owned resources are modified and `status.observedGeneration` equals `metadata.generation`.

#### Scenario: Generation gate skips reconcile when already current

Given a `RedisInstance` CR where `status.observedGeneration` equals `metadata.generation`,  
When the operator enters the reconcile function,  
Then the operator returns early without modifying any owned resource.

---

### Requirement: StatefulSet Ownership and Management

The operator MUST create and own a StatefulSet for each `RedisInstance`, keeping it in sync with the CR spec at all times.

#### Scenario: StatefulSet created on CR creation

Given a new `RedisInstance` CR,  
When the operator reconciles it for the first time,  
Then a StatefulSet named `<cr-name>` is created in the same namespace, owned by the CR via `ownerReferences`.

#### Scenario: StatefulSet updated on spec change

Given a `RedisInstance` CR whose `spec.redisVersion` is updated from `"7.0"` to `"7.2"`,  
When the operator reconciles the updated CR,  
Then the StatefulSet pod template image tag is updated to `7.2` and a rolling restart begins.

#### Scenario: Externally modified StatefulSet is corrected

Given a StatefulSet owned by a `RedisInstance` that is modified out-of-band (e.g., replicas manually changed),  
When the operator next reconciles the CR,  
Then the StatefulSet is restored to match `spec.replicas`.

---

### Requirement: Headless Service Management

The operator MUST create and manage a headless Service to provide stable DNS for StatefulSet pods.

#### Scenario: Headless Service created alongside StatefulSet

Given a new `RedisInstance` CR,  
When the operator reconciles it,  
Then a Service named `<cr-name>` with `clusterIP: None` is created in the same namespace.

#### Scenario: Headless Service selector matches StatefulSet pods

Given the headless Service created by the operator,  
When inspecting its label selector,  
Then the selector matches the pod labels set by the operator-managed StatefulSet.

---

### Requirement: ConfigMap-Driven Configuration

The operator MUST render a ConfigMap from `spec.config` and mount it into Redis pods as the redis.conf file.

#### Scenario: ConfigMap created from spec.config

Given a `RedisInstance` CR with `spec.config` containing `{"maxmemory": "512mb", "maxmemory-policy": "allkeys-lru"}`,  
When the operator reconciles,  
Then a ConfigMap named `<cr-name>-config` is created containing a redis.conf with those key-value pairs.

#### Scenario: ConfigMap update triggers rolling restart

Given a `RedisInstance` CR whose `spec.config` is updated,  
When the operator reconciles,  
Then the ConfigMap is updated and the StatefulSet pod template annotation is changed to trigger a rolling restart.

---

### Requirement: Sentinel Topology Support

When `spec.topology` is `sentinel`, the operator MUST deploy a Sentinel StatefulSet and Service in addition to the primary Redis StatefulSet.

#### Scenario: Sentinel resources created for sentinel topology

Given a `RedisInstance` CR with `spec.topology: sentinel` and `spec.replicas: 3`,  
When the operator reconciles,  
Then a StatefulSet named `<cr-name>-sentinel` and a Service named `<cr-name>-sentinel` are created.

#### Scenario: Sentinel endpoints populated in status

Given a `RedisInstance` CR with `spec.topology: sentinel` that has been reconciled to `Running`,  
When inspecting `status.sentinelEndpoints`,  
Then the list contains the DNS addresses of all Sentinel pods.

#### Scenario: Sentinel resources absent for standalone topology

Given a `RedisInstance` CR with `spec.topology: standalone`,  
When the operator reconciles,  
Then no Sentinel StatefulSet or Service is created.

---

### Requirement: Horizontal Scaling

The operator MUST support horizontal scaling by changing `spec.replicas`.

#### Scenario: Scale-up adds replicas

Given a `RedisInstance` CR currently at `spec.replicas: 1`,  
When `spec.replicas` is updated to `3`,  
Then the operator updates the StatefulSet to 3 replicas and `status.readyReplicas` reaches 3.

#### Scenario: Scale-down cordons before termination

Given a `RedisInstance` CR at `spec.replicas: 3`,  
When `spec.replicas` is updated to `1`,  
Then the operator cordons the highest-ordinal replicas (removes them from service), verifies replication drain, and then reduces the StatefulSet replica count to 1.

---

### Requirement: Ordered Teardown via Finalizer

The operator MUST add a finalizer (`redis.example.io/cleanup`) to every `RedisInstance` CR and perform ordered teardown before allowing deletion.

#### Scenario: Finalizer added on creation

Given a new `RedisInstance` CR,  
When the operator processes it for the first time,  
Then `redis.example.io/cleanup` appears in `metadata.finalizers`.

#### Scenario: Teardown executes before CR deletion

Given a `RedisInstance` CR with the finalizer set and a deletion timestamp present,  
When the operator reconciles,  
Then the operator tears down Sentinel instances (if applicable) before Redis pods, then removes the finalizer.

#### Scenario: CR deleted after finalizer removal

Given a `RedisInstance` CR whose finalizer has been removed during teardown,  
When the Kubernetes garbage collector runs,  
Then the CR is deleted and owned resources are removed via ownerReference cascading.

---

### Requirement: Status Subresource Updates

The operator MUST update the `status` subresource after each reconcile pass to reflect current state.

#### Scenario: Phase set to Pending on creation

Given a newly created `RedisInstance` CR whose StatefulSet pods are not yet ready,  
When the operator completes its first reconcile pass,  
Then `status.phase` is `Pending`.

#### Scenario: Phase set to Running when all replicas are ready

Given a `RedisInstance` CR where all StatefulSet pods reach the Ready condition,  
When the operator updates status,  
Then `status.phase` is `Running` and `status.readyReplicas` equals `spec.replicas`.

#### Scenario: Phase set to Degraded when some replicas are not ready

Given a `RedisInstance` CR with `spec.replicas: 3` where one pod is not ready,  
When the operator updates status,  
Then `status.phase` is `Degraded` and `status.readyReplicas` is `2`.

#### Scenario: masterEndpoint populated when master is elected

Given a `RedisInstance` CR that is fully reconciled,  
When the master pod is identified,  
Then `status.masterEndpoint` contains the DNS name of the master pod.

---

### Requirement: Kubernetes Events on State Transitions

The operator MUST emit Kubernetes Events when `status.phase` transitions between values.

#### Scenario: Event emitted on transition to Running

Given a `RedisInstance` CR transitioning from `Pending` to `Running`,  
When the operator updates the status,  
Then a `Normal` Kubernetes Event with reason `InstanceRunning` is emitted on the CR.

#### Scenario: Event emitted on transition to Degraded

Given a `RedisInstance` CR transitioning from `Running` to `Degraded`,  
When the operator detects a pod failure,  
Then a `Warning` Kubernetes Event with reason `InstanceDegraded` is emitted on the CR.

---

### Requirement: Authentication via Kubernetes Secrets

When `spec.auth.passwordSecretRef` is set, the operator MUST configure Redis with the referenced password and MUST NOT expose credentials as environment variable literals.

#### Scenario: Password Secret mounted into Redis pods

Given a `RedisInstance` CR with `spec.auth.passwordSecretRef` pointing to Secret `redis-auth` key `password`,  
When the operator reconciles,  
Then the StatefulSet pod template mounts the Secret as a volume or environment variable using `secretKeyRef`, not a literal value.

#### Scenario: Missing referenced Secret sets phase to Failed

Given a `RedisInstance` CR with `spec.auth.passwordSecretRef` referencing a Secret that does not exist,  
When the operator reconciles,  
Then `status.phase` is set to `Failed` and a `Warning` Event is emitted describing the missing Secret.

---

### Requirement: Controller High Availability via Leader Election

The operator controller MUST support running with multiple replicas and MUST use leader election to ensure only one replica runs the reconcile loop at a time.

#### Scenario: Only elected leader reconciles

Given the operator deployed with 2 replicas,  
When both replicas are running,  
Then only the leader processes reconcile events; the standby replica does not modify any resources.

#### Scenario: Standby takes over after leader failure

Given the operator deployed with 2 replicas and the leader pod is terminated,  
When the Kubernetes Lease expires,  
Then the standby replica acquires the lease and resumes reconcile processing within one lease TTL.

---

### Requirement: Least-Privilege RBAC

The operator MUST run under a ClusterRole that grants only the permissions required for its reconcile loop and no more.

#### Scenario: Operator ClusterRole does not grant wildcard permissions

Given the operator's ClusterRole manifest,  
When inspecting the rules,  
Then no rule contains `"*"` for resources or verbs.

#### Scenario: Operator cannot read arbitrary Secrets

Given the operator's ClusterRole,  
When inspecting Secret permissions,  
Then the role grants only `get`, `list`, and `watch` on Secrets (no `create`, `delete`, or `update`), scoped to the operator's namespace.
