# Spec: Redis Controller Reconciliation Logic

## Scope

This spec defines the behaviour of the `RedisReconciler` — the controller that watches `Redis` CRs and reconciles desired state in the cluster. There is no pre-existing controller; all requirements are additive.

---

## ADDED Requirements

### Requirement: Controller Entrypoint

The `Reconcile(ctx, req)` function MUST be the single entry point for all reconciliation triggered by changes to `Redis` CRs or owned resources (StatefulSet, Service, ConfigMap).

#### Scenario: CR not found is a no-op

Given the reconcile loop is triggered for a `Redis` resource that no longer exists (deleted without finalizer),
when `Reconcile` fetches the CR and receives `NotFound`,
then the function MUST return `(ctrl.Result{}, nil)` without further action.

#### Scenario: Fetch error causes requeue

Given the API server returns a transient error when fetching the CR,
when `Reconcile` receives the error,
then the function MUST return `(ctrl.Result{RequeueAfter: 10s}, err)`.

---

### Requirement: Finalizer Lifecycle

The controller MUST manage a finalizer `redis.example.com/finalizer` on every `Redis` CR to ensure owned resources are cleaned up before the CR is garbage-collected.

#### Scenario: Finalizer added on first reconcile

Given a newly created `Redis` CR without the finalizer,
when `Reconcile` runs for the first time,
then the controller MUST add `redis.example.com/finalizer` to `.metadata.finalizers` and update the CR before proceeding.

#### Scenario: Deletion triggers cleanup

Given a `Redis` CR with the finalizer and `DeletionTimestamp` set,
when `Reconcile` detects the deletion,
then the controller MUST delete any resources not covered by owner-reference garbage collection, remove the finalizer, and update the CR.

#### Scenario: Cleanup failure blocks deletion

Given deletion cleanup returns an error (e.g. API server unavailable),
when the controller encounters the error,
then it MUST NOT remove the finalizer and MUST requeue with `RequeueAfter: 10s`.

---

### Requirement: ConfigMap Reconciliation

The controller MUST create and keep up to date a ConfigMap named `<cr-name>-config` in the same namespace containing the full `redis.conf` content.

#### Scenario: ConfigMap created on first reconcile

Given a `Redis` CR with `spec.config: {}`,
when the controller reconciles,
then a ConfigMap named `<cr-name>-config` MUST exist with a `redis.conf` key containing at least the default directives (`bind 0.0.0.0`, `protected-mode no`, `port 6379`).

#### Scenario: Config overrides are merged

Given a `Redis` CR with `spec.config: {save: ""}` (disabling persistence snapshots),
when the controller reconciles,
then the ConfigMap's `redis.conf` MUST include `save ""` and the default directives.

#### Scenario: ConfigMap updated when spec.config changes

Given an existing ConfigMap and a CR whose `spec.config` changes,
when the controller reconciles,
then the ConfigMap data MUST be patched to reflect the new config without recreating the ConfigMap.

#### Scenario: Existing unowned ConfigMap is skipped

Given a ConfigMap named `<cr-name>-config` already exists in the namespace without an owner reference to the `Redis` CR,
when the controller reconciles,
then the controller MUST leave the ConfigMap unchanged, log a warning Event indicating the resource is unowned, and continue reconciling the remaining resources without returning an error.

---

### Requirement: StatefulSet Reconciliation

The controller MUST create and keep up to date a StatefulSet named `<cr-name>` that runs the Redis container(s).

#### Scenario: StatefulSet created with correct replica count

Given a `Redis` CR with `spec.replicas: 3`,
when the controller reconciles,
then the StatefulSet MUST have `spec.replicas: 3`.

#### Scenario: StatefulSet image updated

Given a `Redis` CR that changes `spec.version` from `"7.0"` to `"7.2"`,
when the controller reconciles,
then the StatefulSet's container image MUST be updated to `redis:7.2` and a rolling update triggered.

#### Scenario: StatefulSet owns ConfigMap mount

Given any `Redis` CR,
when the controller reconciles,
then each pod in the StatefulSet MUST mount the `<cr-name>-config` ConfigMap at `/usr/local/etc/redis/redis.conf` as a file.

#### Scenario: Owner reference prevents orphan resources

Given a StatefulSet owned by a `Redis` CR,
when the CR is deleted (after finalizer is removed),
then Kubernetes garbage collection MUST delete the StatefulSet automatically via the owner reference.

#### Scenario: Existing unowned StatefulSet is skipped

Given a StatefulSet named `<cr-name>` already exists in the namespace without an owner reference to the `Redis` CR,
when the controller reconciles,
then the controller MUST leave the StatefulSet unchanged, log a warning Event indicating the resource is unowned, and continue reconciling the remaining resources without returning an error.

---

### Requirement: Service Reconciliation

The controller MUST maintain two Services:

1. **Headless Service** (`<cr-name>-headless`, `clusterIP: None`) — for stable per-pod DNS used by replication config.
2. **Client Service** (`<cr-name>`, type = `spec.serviceType`) — for application access to Redis on port 6379.

#### Scenario: Headless Service always created

Given any `Redis` CR,
when the controller reconciles,
then a headless Service named `<cr-name>-headless` with `clusterIP: None` and selector matching the StatefulSet pods MUST exist.

#### Scenario: Client Service type reflects spec

Given a `Redis` CR with `spec.serviceType: LoadBalancer`,
when the controller reconciles,
then the client Service MUST have `type: LoadBalancer`.

#### Scenario: Service selector matches StatefulSet pods

Given a StatefulSet with label `app.kubernetes.io/instance: <cr-name>`,
when the controller reconciles,
then both Services MUST select pods using that label.

#### Scenario: Existing unowned Service is skipped

Given either expected Service already exists in the namespace without an owner reference to the `Redis` CR,
when the controller reconciles,
then the controller MUST leave that Service unchanged, log a warning Event indicating the resource is unowned, and continue reconciling the remaining resources without returning an error.

---

### Requirement: PVC Reconciliation

When `spec.persistence.enabled` is `true`, the StatefulSet MUST include a `volumeClaimTemplate` and each pod MUST mount the resulting PVC at `/data`.

#### Scenario: PVC template added when persistence enabled

Given a `Redis` CR with `spec.persistence.enabled: true` and `spec.persistence.size: "10Gi"`,
when the controller reconciles,
then the StatefulSet's `spec.volumeClaimTemplates` MUST contain one template requesting `10Gi` storage.

#### Scenario: No PVC when persistence disabled

Given a `Redis` CR with `spec.persistence.enabled: false`,
when the controller reconciles,
then the StatefulSet MUST NOT include any `volumeClaimTemplates` and MUST use an `emptyDir` volume for `/data`.

#### Scenario: StorageClass propagated to PVC template

Given a `Redis` CR with `spec.persistence.storageClassName: "fast-ssd"`,
when the controller reconciles,
then the PVC template MUST specify `storageClassName: fast-ssd`.

---

### Requirement: Master-Replica Configuration

When `spec.replicas > 1`, the controller MUST configure pods with ordinal index ≥ 1 as replicas pointing to pod-0.

#### Scenario: Replica pod receives replicaof directive

Given a `Redis` CR with `spec.replicas: 2`,
when the controller reconciles,
then the ConfigMap (or pod init script) MUST inject `replicaof <cr-name>-0.<cr-name>-headless.<namespace>.svc.cluster.local 6379` for pods with ordinal ≥ 1.

#### Scenario: Pod-0 is never configured as replica

Given any replica count,
when the controller reconciles,
then pod-0 MUST NOT have a `replicaof` directive in its effective `redis.conf`.

---

### Requirement: Status Update

After each reconcile pass, the controller MUST patch the CR's `.status` subresource to reflect the current observed state.

#### Scenario: Phase transitions correctly

| observed StatefulSet state | expected `status.phase` |
|---------------------------|------------------------|
| StatefulSet not yet created | `Pending` |
| `readyReplicas < spec.replicas` | `Degraded` |
| `readyReplicas == spec.replicas` | `Running` |
| CR has `DeletionTimestamp` | `Terminating` |

#### Scenario: observedGeneration is updated

Given a `Redis` CR at `.metadata.generation: 5`,
when the controller completes a full reconcile pass without error,
then `status.observedGeneration` MUST be set to `5`.

#### Scenario: Available condition set True when Running

Given `status.phase` transitions to `Running`,
when the status patch is applied,
then `conditions[type=Available].status` MUST be `"True"` and `conditions[type=Degraded].status` MUST be `"False"`.

---

### Requirement: Requeue Strategy

The controller MUST use the following requeue strategy to avoid busy-loops while ensuring eventual convergence.

| Situation | Requeue behaviour |
|-----------|------------------|
| Transient API error | `RequeueAfter: 10s` |
| StatefulSet not yet ready | `RequeueAfter: 30s` |
| Reconcile fully converged | No explicit requeue (watches re-trigger on resource changes) |
| Unrecoverable error (e.g. invalid spec) | Emit Event; no requeue until spec changes |

#### Scenario: No tight requeue loop when converged

Given a `Redis` CR that is fully reconciled (`status.phase: Running`),
when no resources change,
then the controller MUST NOT requeue more frequently than once per 30 seconds (driven only by watch events or the 30s requeue for not-yet-ready check).

---

### Requirement: Event Recording

The controller MUST emit Kubernetes Events for significant lifecycle changes.

#### Scenario: Event on successful creation

Given a `Redis` CR is fully reconciled for the first time,
when `status.phase` transitions to `Running`,
then a Normal Event with reason `Created` MUST be recorded on the `Redis` CR.

#### Scenario: Event on reconcile error

Given the controller encounters a non-transient error,
when the error is returned,
then a Warning Event with reason `ReconcileError` MUST be recorded on the `Redis` CR.
