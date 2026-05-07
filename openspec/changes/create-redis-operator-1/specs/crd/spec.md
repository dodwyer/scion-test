# Spec: Redis CRD / CR

## Scope

This spec defines the `Redis` Custom Resource Definition (CRD) introduced by the Redis Operator. It covers the API group, version, validation schema, status subresource, and printer columns. There is no pre-existing CRD; all requirements are additive.

---

## ADDED Requirements

### Requirement: CRD Registration

The operator MUST register a CRD with the following identity on the cluster before any `Redis` CR can be created.

| Field | Value |
|-------|-------|
| `group` | `redis.example.com` |
| `version` | `v1alpha1` |
| `kind` | `Redis` |
| `plural` | `redis` |
| `singular` | `redis` |
| `scope` | `Namespaced` |

#### Scenario: CRD absent at install time

Given the operator Helm chart is installed on a cluster that does not have the `Redis` CRD,
when the chart's `pre-install` hook (or CRD Job) runs,
then the CRD must be created and transition to `Established=True` before the operator Deployment starts.

#### Scenario: CRD already present on upgrade

Given the CRD exists from a previous install,
when the chart is upgraded,
then the CRD MUST be patched (not deleted-and-recreated) to preserve existing CR instances.

---

### Requirement: Spec Schema Validation

The CRD MUST declare an OpenAPI v3 validation schema. The API server MUST reject CR creation or updates that violate the schema.

#### Scenario: Valid standalone CR is accepted

Given a `Redis` CR with `spec.replicas: 1` and no persistence fields,
when a user runs `kubectl apply`,
then the API server returns HTTP 200/201 and the CR is stored.

#### Scenario: Invalid replicas value is rejected

Given a `Redis` CR with `spec.replicas: 0` (minimum is 1),
when a user runs `kubectl apply`,
then the API server returns HTTP 422 with a validation error referencing `spec.replicas`.

#### Scenario: Unknown spec field is rejected

Given a `Redis` CR with an unrecognised field `spec.unknownField: "foo"` and the CRD has `x-kubernetes-preserve-unknown-fields: false`,
when a user runs `kubectl apply`,
then the API server returns HTTP 422 indicating the unknown field.

---

### Requirement: Spec Fields

The `spec` section MUST accept the following fields:

#### `spec.version`

- Type: `string`
- Required: no (default `"7.2"`)
- Validation: must match pattern `^\d+\.\d+(\.\d+)?$`
- Description: Redis image tag to use.

#### `spec.replicas`

- Type: `integer` (int32)
- Required: no (default `1`)
- Validation: minimum `1`, maximum `10`
- Description: Number of Redis pods. `1` = standalone; `>1` = master-replica.

#### `spec.resources`

- Type: object (corev1.ResourceRequirements)
- Required: no
- Description: CPU and memory requests/limits applied to every Redis container.

#### `spec.persistence`

- Type: object
- Required: no
- Fields:
  - `enabled` (bool, default `false`)
  - `storageClassName` (string, optional)
  - `size` (resource.Quantity, default `"1Gi"`)

#### `spec.config`

- Type: `object` with `additionalProperties: string`
- Required: no (default empty)
- Description: Key-value pairs merged into `redis.conf`. Keys must be valid `redis.conf` directives.

#### `spec.image`

- Type: object
- Required: no
- Fields:
  - `repository` (string, default `"redis"`)
  - `pullPolicy` (string, enum `Always|IfNotPresent|Never`, default `"IfNotPresent"`)

#### `spec.serviceType`

- Type: string
- Required: no (default `"ClusterIP"`)
- Validation: enum `ClusterIP|NodePort|LoadBalancer`

#### Scenario: Persistence enabled CR creates PVC template

Given a `Redis` CR with `spec.persistence.enabled: true` and `spec.persistence.size: "5Gi"`,
when the controller reconciles,
then the StatefulSet's `volumeClaimTemplates` MUST include a PVC with `requests.storage: 5Gi`.

#### Scenario: Config override is applied

Given a `Redis` CR with `spec.config: {maxmemory: "256mb", maxmemory-policy: "allkeys-lru"}`,
when the controller reconciles,
then the rendered `redis.conf` ConfigMap MUST contain both directives.

---

### Requirement: Status Subresource

The CRD MUST declare `subresources.status: {}` so that status updates are performed via the `/status` subresource endpoint and do not affect the spec generation counter.

#### `status.phase`

- Type: string
- Values: `Pending` | `Running` | `Degraded` | `Terminating`

#### `status.readyReplicas`

- Type: integer (int32)
- Description: Count of pods currently in Ready state.

#### `status.conditions`

- Type: array of `metav1.Condition`
- Condition types: `Available`, `Progressing`, `Degraded`

#### `status.observedGeneration`

- Type: integer (int64)
- Description: `.metadata.generation` of the spec that was last reconciled.

#### Scenario: Newly created CR shows Pending phase

Given a `Redis` CR is created,
when the controller has not yet created the StatefulSet,
then `status.phase` MUST be `Pending` and `status.readyReplicas` MUST be `0`.

#### Scenario: All pods ready sets Running phase

Given a `Redis` CR with `spec.replicas: 2` and both pods are Ready,
when the controller reconciles,
then `status.phase` MUST be `Running` and `status.readyReplicas` MUST be `2`.

#### Scenario: Partially ready sets Degraded phase

Given a `Redis` CR with `spec.replicas: 2` and only one pod is Ready,
when the controller reconciles,
then `status.phase` MUST be `Degraded` and the `Degraded` condition MUST be `True`.

---

### Requirement: Printer Columns

The CRD MUST declare `additionalPrinterColumns` so that `kubectl get redis` shows useful information without `-o json`.

| Column | JSONPath | Description |
|--------|----------|-------------|
| `Phase` | `.status.phase` | Current lifecycle phase |
| `Ready` | `.status.readyReplicas` | Ready pod count |
| `Version` | `.spec.version` | Redis version |
| `Age` | `.metadata.creationTimestamp` | Age |

#### Scenario: kubectl get redis output

Given one or more `Redis` CRs exist,
when a user runs `kubectl get redis -n <namespace>`,
then the output table MUST include columns Phase, Ready, Version, and Age.

---

### Requirement: Short Names

The CRD MUST register `rd` as a short name for `redis` resources.

#### Scenario: Short name resolves

Given the CRD is installed,
when a user runs `kubectl get rd`,
then the command returns the same result as `kubectl get redis`.
