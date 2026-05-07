## ADDED Requirements

### Requirement: CRD Registration

The `Redis` Custom Resource Definition MUST be registered with the Kubernetes API server at group `redis.example.com`, version `v1alpha1`, kind `Redis`, with `Namespaced` scope.

#### Scenario: CRD is installed and accepted by the API server

Given the CRD manifest is applied to a Kubernetes 1.24+ cluster,
When the API server processes the manifest,
Then `kubectl get crds redis.redis.example.com` returns the CRD with `ESTABLISHED` condition `True`,
And `kubectl api-resources --api-group=redis.example.com` lists the `redis` resource.

#### Scenario: Creating a Redis CR with only required fields succeeds

Given the CRD is installed,
When a user applies a `Redis` CR with only `spec.version` set (e.g., `"7.2"`),
Then the API server accepts the object without validation errors,
And the controller begins reconciliation.

#### Scenario: Creating a Redis CR without the required version field is rejected

Given the CRD is installed,
When a user applies a `Redis` CR omitting `spec.version`,
Then the API server returns a validation error indicating `spec.version` is required,
And no CR is persisted.

#### Scenario: Unknown fields in spec are rejected

Given the CRD has `x-kubernetes-preserve-unknown-fields: false`,
When a user applies a `Redis` CR with an unrecognized spec field,
Then the API server rejects the request with a field validation error.

---

### Requirement: StatefulSet Lifecycle Management

The controller MUST create, update, and reflect the deletion of a `StatefulSet` that runs Redis pods matching the desired state declared in the `Redis` CR spec.

#### Scenario: StatefulSet is created on first reconcile

Given a new `Redis` CR is created,
When the controller reconciles it for the first time,
Then a `StatefulSet` named `<cr-name>` exists in the same namespace,
And the StatefulSet's `spec.replicas` matches `redis.spec.replicas` (default `1`),
And the container image is `redis:<spec.version>`.

#### Scenario: StatefulSet is updated when spec changes

Given an existing `Redis` CR with `spec.version: "7.0"`,
When the user updates `spec.version` to `"7.2"`,
Then the controller updates the StatefulSet container image to `redis:7.2`,
And the StatefulSet performs a rolling update.

#### Scenario: StatefulSet replica count follows spec.replicas

Given a `Redis` CR with `spec.replicas: 3`,
When the controller reconciles,
Then the StatefulSet `spec.replicas` is set to `3`.

#### Scenario: Owned StatefulSet is garbage-collected on CR deletion

Given a `Redis` CR and its owned `StatefulSet` exist,
When the user deletes the `Redis` CR,
Then Kubernetes garbage-collects the `StatefulSet` via owner reference cascade,
And the StatefulSet is eventually removed.

#### Scenario: Resource requests and limits are applied to Redis pods

Given a `Redis` CR with `spec.resources` populated,
When the controller reconciles,
Then the StatefulSet pod template container has matching `resources.requests` and `resources.limits`.

---

### Requirement: Service Provisioning

The controller MUST create and maintain a ClusterIP `Service` and a Headless `Service` for each `Redis` CR, providing stable network endpoints for clients and pod DNS identity respectively.

#### Scenario: ClusterIP Service is created

Given a new `Redis` CR,
When the controller reconciles,
Then a `Service` of type `ClusterIP` named `<cr-name>` exists in the same namespace,
And the Service selector targets pods managed by the StatefulSet,
And port `6379` is exposed.

#### Scenario: Headless Service is created

Given a new `Redis` CR,
When the controller reconciles,
Then a `Service` named `<cr-name>-headless` with `spec.clusterIP: None` exists in the same namespace,
And individual pods are addressable as `<pod-name>.<cr-name>-headless.<namespace>.svc.cluster.local`.

#### Scenario: Services are updated if spec changes require it

Given an existing `Redis` CR,
When the CR spec changes in a way that affects service selectors or ports,
Then the controller updates the affected Service(s) to reflect the new desired state.

#### Scenario: Services are garbage-collected on CR deletion

Given a `Redis` CR and its owned Services exist,
When the user deletes the `Redis` CR,
Then both Services are garbage-collected via owner reference cascade.

---

### Requirement: ConfigMap-based Redis Configuration

The controller MUST render a `ConfigMap` containing a valid `redis.conf` file assembled from the CR's `spec.config` overrides and derived settings (e.g., `requirepass` when auth is enabled).

#### Scenario: ConfigMap is created with default configuration

Given a `Redis` CR with no `spec.config` overrides,
When the controller reconciles,
Then a `ConfigMap` named `<cr-name>-config` exists in the same namespace,
And it contains a `redis.conf` key with at minimum a parseable Redis configuration.

#### Scenario: spec.config overrides appear in redis.conf

Given a `Redis` CR with `spec.config: {maxmemory: "256mb", maxmemory-policy: "allkeys-lru"}`,
When the controller reconciles,
Then the `redis.conf` in the ConfigMap contains `maxmemory 256mb` and `maxmemory-policy allkeys-lru`.

#### Scenario: ConfigMap is updated when spec.config changes

Given an existing `Redis` CR and its ConfigMap,
When `spec.config` is modified,
Then the controller updates the ConfigMap data within the next reconcile cycle.

#### Scenario: StatefulSet mounts the ConfigMap

Given the ConfigMap is created,
When the controller creates or updates the StatefulSet,
Then the StatefulSet pod template mounts the ConfigMap as a volume at a well-known path (e.g., `/etc/redis/`),
And the Redis process is started with `redis-server /etc/redis/redis.conf`.

---

### Requirement: Optional Persistence via PVC

When `spec.persistence.enabled` is `true`, the controller MUST configure the `StatefulSet` with `volumeClaimTemplates` so each pod receives a dedicated `PersistentVolumeClaim` for Redis data.

#### Scenario: PVC is provisioned when persistence is enabled

Given a `Redis` CR with `spec.persistence.enabled: true` and `spec.persistence.size: "5Gi"`,
When the controller reconciles,
Then the `StatefulSet` includes a `volumeClaimTemplate` requesting `5Gi` of storage,
And each pod mounts the PVC at the Redis data directory (e.g., `/data`).

#### Scenario: StorageClass is applied when specified

Given `spec.persistence.storageClassName: "fast-ssd"`,
When the controller reconciles,
Then the `volumeClaimTemplate` specifies `storageClassName: fast-ssd`.

#### Scenario: No PVC is created when persistence is disabled

Given a `Redis` CR with `spec.persistence.enabled: false` (or omitted),
When the controller reconciles,
Then the `StatefulSet` has no `volumeClaimTemplates`,
And Redis data is stored in an `emptyDir` volume or not persisted.

#### Scenario: Enabling persistence after initial creation updates StatefulSet

Given an existing `Redis` CR with persistence disabled,
When `spec.persistence.enabled` is changed to `true`,
Then the controller updates the StatefulSet to include the volumeClaimTemplate on the next reconcile.

---

### Requirement: Optional Authentication via Secret

When `spec.auth.enabled` is `true`, the controller MUST ensure a Redis password is set via `requirepass` in `redis.conf`, sourced either from a user-provided Secret or an auto-generated one.

#### Scenario: Auth secret is auto-generated when no secretName is provided

Given `spec.auth.enabled: true` and `spec.auth.secretName` is empty,
When the controller reconciles,
Then a `Secret` named `<cr-name>-auth` is created in the same namespace,
And it contains a randomly generated password under the key `password`,
And `requirepass <password>` is present in the rendered `redis.conf`.

#### Scenario: External secret is used when secretName is provided

Given `spec.auth.enabled: true` and `spec.auth.secretName: "my-redis-secret"`,
When the controller reconciles,
Then the controller reads `my-redis-secret` from the same namespace,
And injects the `password` key value into the rendered `redis.conf` as `requirepass`,
And does NOT create an additional Secret.

#### Scenario: Auth is disabled when spec.auth.enabled is false

Given `spec.auth.enabled: false` (or omitted),
When the controller reconciles,
Then `requirepass` is absent from `redis.conf`,
And no auth Secret is created or managed by the controller.

#### Scenario: Rotating the password in an external secret is reflected

Given `spec.auth.secretName: "my-redis-secret"` and the Secret's `password` is updated externally,
When the controller reconciles (triggered by re-queue or Secret watch),
Then the ConfigMap is updated with the new `requirepass` value.

---

### Requirement: Status Reporting

The controller MUST update the `Redis` CR's `status` subresource after every reconcile to accurately reflect the current observed state of the Redis deployment.

#### Scenario: Status phase transitions to Running when all replicas are ready

Given a `Redis` CR with `spec.replicas: 1`,
When the owned StatefulSet reports `readyReplicas: 1`,
Then `status.phase` is set to `Running`,
And `status.readyReplicas` is `1`,
And condition `Available` has status `True`.

#### Scenario: Status phase transitions to Degraded when replicas are not ready

Given a `Redis` CR with `spec.replicas: 3`,
When the owned StatefulSet reports `readyReplicas: 1`,
Then `status.phase` is set to `Degraded`,
And condition `Available` has status `False`,
And condition `Degraded` has status `True`.

#### Scenario: Status phase is Pending immediately after CR creation

Given a new `Redis` CR is created,
When the controller starts its first reconcile before the StatefulSet pods are ready,
Then `status.phase` is `Pending`.

#### Scenario: Status phase is Terminating while CR deletion is in progress

Given a `Redis` CR with a non-nil `deletionTimestamp`,
When the controller reconciles,
Then `status.phase` is set to `Terminating`.

#### Scenario: status.readyReplicas reflects StatefulSet observed state

Given the StatefulSet `status.readyReplicas` changes,
When the controller reconciles,
Then `redis.status.readyReplicas` matches the StatefulSet value.

---

### Requirement: RBAC Minimal Permissions

The operator's `ServiceAccount` MUST be bound to RBAC roles that grant only the permissions required for reconciliation, following the principle of least privilege.

#### Scenario: ClusterRole grants access to Redis CRD resources

Given the operator is installed,
Then the `ClusterRole` includes rules for `get`, `list`, `watch`, `update`, `patch` on `redis.example.com/redis`,
And rules for `update`, `patch` on `redis.example.com/redis/status`,
And rules for `update` on `redis.example.com/redis/finalizers`.

#### Scenario: ClusterRole grants access to owned Kubernetes resources

Given the operator is installed,
Then the `ClusterRole` includes rules for `create`, `get`, `list`, `watch`, `update`, `patch`, `delete` on `apps/statefulsets`,
And equivalent rules on core `services`, `configmaps`, and `secrets`,
And `create`, `patch` on core `events`.

#### Scenario: RBAC matches the selected watch scope

Given the operator is installed in cluster-scoped mode watching all namespaces,
Then its `ServiceAccount` is bound to a `ClusterRole` and `ClusterRoleBinding` that permit cross-namespace `list` and `watch` on `Redis` resources and owned objects,
And the rules include only the cluster-scoped permissions required for that mode.

Given the operator is installed in namespace-scoped mode watching a single namespace,
Then its `ServiceAccount` is bound only to namespace-limited RBAC for that namespace,
And it does not grant cross-namespace `list` or `watch` permissions that are unnecessary in namespace-scoped mode.

#### Scenario: ClusterRole does not grant cluster-admin or wildcard verbs

Given the operator `ClusterRole`,
When it is inspected,
Then no rule uses `*` for verbs, resources, or API groups.

#### Scenario: Operator ServiceAccount is isolated

Given the operator's `ServiceAccount` exists in the operator namespace,
When it is inspected,
Then it is not the `default` ServiceAccount,
And it is bound only to the operator RBAC roles required for the selected watch scope and no broader roles.

---

### Requirement: Idempotent Reconciliation

The controller MUST produce the same outcome regardless of how many times the reconcile loop runs for a given desired state, with no unintended side effects on repeated reconciles.

#### Scenario: Re-running reconcile on an unchanged CR causes no mutations

Given a `Redis` CR and all owned resources in their desired state,
When the controller reconciles again without any spec change,
Then no `create`, `update`, or `patch` calls are issued to the Kubernetes API for any owned resource,
And `status` is not unnecessarily patched.

#### Scenario: Reconcile after an external modification restores desired state

Given an owned `ConfigMap` is manually edited to remove a required field,
When the controller reconciles,
Then the ConfigMap is updated to match the desired state derived from the CR spec.

#### Scenario: Reconcile handles missing owned resources gracefully

Given an owned `Service` was manually deleted,
When the controller reconciles,
Then the Service is recreated,
And no error is propagated beyond logging and an event.

#### Scenario: Concurrent reconcile calls do not produce duplicate resources

Given two reconcile requests arrive simultaneously for the same CR,
When both run through `createOrUpdate`,
Then exactly one copy of each owned resource exists after both complete,
And no duplicate-resource errors are returned.

---

### Requirement: Observability

The operator MUST expose Prometheus metrics, structured logs, and Kubernetes Events sufficient to diagnose the health and behavior of the operator and managed Redis instances.

#### Scenario: Prometheus metrics endpoint is reachable

Given the operator pod is running,
When an HTTP GET is made to `:8080/metrics`,
Then the response is HTTP 200 with `Content-Type: text/plain`,
And it contains controller-runtime standard metrics (e.g., `controller_runtime_reconcile_total`).

#### Scenario: Custom ready-replicas gauge is reported

Given a `Redis` CR with `status.readyReplicas: 2`,
When `:8080/metrics` is scraped,
Then the response contains `redis_operator_ready_replicas{namespace="<ns>",name="<name>"} 2`.

#### Scenario: Reconcile errors are counted in metrics

Given the controller encounters an API error during reconciliation,
When `:8080/metrics` is scraped,
Then `controller_runtime_reconcile_errors_total` has incremented for the `redis` controller.

#### Scenario: Structured log entry is emitted on each reconcile

Given the controller processes a reconcile request,
Then a JSON log line is written to stdout containing fields `namespace`, `name`, and `reconcileID`.

#### Scenario: Kubernetes Event is emitted on phase transition

Given the `Redis` CR transitions from `Pending` to `Running`,
When `kubectl describe redis <name>` is run,
Then a `Normal` event with reason `StatusUpdated` appears in the Events section.

#### Scenario: Warning event is emitted on reconciliation error

Given the controller encounters an error during reconciliation,
Then a `Warning` event with reason `ReconcileError` and a descriptive message is emitted on the `Redis` CR.
