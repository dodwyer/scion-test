# Spec: Redis Operator

## ADDED Requirements

### Requirement: CRD Registration

The system must define a `Redis` Custom Resource Definition in the `redis.example.com` API group at version `v1alpha1`, scoped to namespaces.

#### Scenario: User creates a Redis CR

Given a Kubernetes cluster with the operator installed,
When a user applies a `Redis` CR manifest,
Then the API server accepts it and the operator begins reconciliation.

#### Scenario: CRD schema validation rejects invalid spec

Given a `Redis` CR with an invalid field value (e.g., negative replicas),
When the user applies it,
Then the API server rejects it with a validation error before reaching the operator.

---

### Requirement: StatefulSet Lifecycle Management

The operator must create, update, and own a `StatefulSet` matching the `Redis` CR spec. The StatefulSet uses the Redis image at the version specified in `.spec.version`.

#### Scenario: Create triggers StatefulSet provisioning

Given a new `Redis` CR is created,
When the reconciler runs,
Then a `StatefulSet` is created in the same namespace with ownerReference pointing to the CR.

#### Scenario: Version update triggers rolling update

Given an existing `Redis` CR with `spec.version: "7.0"`,
When the user updates it to `spec.version: "7.2"`,
Then the operator updates the StatefulSet image tag and a rolling update begins.

#### Scenario: CR deletion cascades to StatefulSet

Given a `Redis` CR and its owned StatefulSet exist,
When the CR is deleted,
Then Kubernetes garbage collection removes the StatefulSet via ownerReference.

---

### Requirement: Service Provisioning

The operator must create and own two Services for each `Redis` CR: a ClusterIP Service for client access and a headless Service for StatefulSet DNS.

#### Scenario: ClusterIP service enables client connections

Given a running `Redis` CR,
When a pod in the same namespace connects to `<redis-name>:<port>`,
Then the connection reaches the Redis pod.

#### Scenario: Headless service provides stable DNS

Given a running `Redis` CR with name `my-redis`,
When a client resolves `my-redis-headless`,
Then it returns the pod IP(s) directly.

---

### Requirement: ConfigMap-based Redis Configuration

The operator must generate and own a `ConfigMap` containing a `redis.conf` derived from the CR spec. The StatefulSet mounts this ConfigMap.

#### Scenario: Default config is applied

Given a `Redis` CR with no `.spec.config` overrides,
When the operator reconciles,
Then the ConfigMap contains a minimal valid `redis.conf`.

#### Scenario: User config overrides are applied

Given a `Redis` CR with `.spec.config: {maxmemory: "256mb"}`,
When the operator reconciles,
Then the ConfigMap `redis.conf` includes `maxmemory 256mb`.

---

### Requirement: Optional Persistence via PVC

When `.spec.persistence.enabled` is `true`, the operator must create a `PersistentVolumeClaim` and configure the StatefulSet to mount it at `/data`.

#### Scenario: Persistence disabled uses emptyDir

Given a `Redis` CR with `spec.persistence.enabled: false` (or omitted),
When the StatefulSet is created,
Then the data volume is an `emptyDir`.

#### Scenario: Persistence enabled creates PVC

Given a `Redis` CR with `spec.persistence.enabled: true` and `spec.persistence.size: "5Gi"`,
When the operator reconciles,
Then a `PersistentVolumeClaim` of size `5Gi` is created and mounted at `/data`.

---

### Requirement: Optional Authentication via Secret

When `.spec.auth.enabled` is `true`, the operator must ensure a Secret containing the Redis password exists, and configure the StatefulSet to pass it via `--requirepass`.

#### Scenario: External secret referenced

Given a `Redis` CR with `spec.auth.secretName: "my-redis-password"`,
When the operator reconciles,
Then the StatefulSet reads the password from the named Secret without creating a new one.

#### Scenario: Auth disabled means no password

Given a `Redis` CR with `spec.auth.enabled: false`,
When the StatefulSet is created,
Then no `--requirepass` argument is passed to Redis.

---

### Requirement: Status Reporting

The operator must keep the `Redis` CR status up to date after each reconcile, reflecting the current phase, ready replica count, and condition set.

#### Scenario: Available condition set when ready

Given a `Redis` CR whose StatefulSet has all replicas ready,
When the reconciler runs,
Then the CR status has condition `Available=True` and `phase=Running`.

#### Scenario: Degraded condition set on error

Given a reconcile error occurs (e.g., StatefulSet create fails),
When the reconciler handles the error,
Then the CR status has condition `Degraded=True` with the error message, and the request is requeued.

---

### Requirement: RBAC Minimal Permissions

The operator must declare the minimal RBAC permissions required to manage owned resource types. No cluster-admin or wildcard permissions are permitted.

#### Scenario: Operator can manage owned resource types

Given the operator's ServiceAccount has the defined ClusterRole bound,
When the reconciler creates a StatefulSet, Service, ConfigMap, Secret, or PVC,
Then the API server authorizes the request.

#### Scenario: Operator cannot access unrelated resources

Given the operator's ServiceAccount,
When it attempts to access a resource not in its ClusterRole (e.g., Deployments in other namespaces),
Then the API server denies the request.

---

### Requirement: Idempotent Reconciliation

The reconciler must produce the same cluster state regardless of how many times it is invoked for the same CR spec.

#### Scenario: Repeated reconcile does not create duplicate resources

Given a `Redis` CR that has already been fully reconciled,
When the reconciler runs again with no spec change,
Then no resources are created, updated, or deleted.

---

### Requirement: Observability

The operator must expose Prometheus-compatible metrics and emit structured logs and Kubernetes Events.

#### Scenario: Prometheus metrics are accessible

Given the operator is running,
When a Prometheus scraper queries `:8080/metrics`,
Then it receives reconcile duration histograms and error counters.

#### Scenario: Event emitted on successful creation

Given a new `Redis` CR is created and reconciled successfully,
When the user runs `kubectl describe redis <name>`,
Then they see a `Normal` event with reason `Created` or `Reconciled`.
