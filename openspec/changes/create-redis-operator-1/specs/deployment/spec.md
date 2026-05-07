# Spec: Operator Deployment

## Scope

This spec defines how the Redis Operator itself is packaged, installed, and configured on a Kubernetes cluster. It covers the Helm chart, Kustomize overlay, operator Deployment, RBAC, and single-namespace vs cluster-wide modes. There is no pre-existing deployment; all requirements are additive.

**Relationship between Helm chart and Kustomize overlay**: kubebuilder scaffolding generates a `config/` directory containing raw Kustomize-compatible YAML (CRD, RBAC, manager Deployment). The Helm chart at `charts/redis-operator/` wraps those manifests with parameterisation (image tags, namespace, RBAC toggles) for end-user installation. Both paths deploy the same operator but serve different audiences: Kustomize for GitOps pipelines, Helm for interactive installation.

---

## ADDED Requirements

### Requirement: Helm Chart Structure

The operator MUST be distributable as a Helm chart located at `charts/redis-operator/` in the repository.

Required files:

| File | Purpose |
|------|---------|
| `Chart.yaml` | Chart metadata (name, version, appVersion) |
| `values.yaml` | Default configuration values |
| `templates/deployment.yaml` | Operator Deployment |
| `templates/serviceaccount.yaml` | ServiceAccount |
| `templates/clusterrole.yaml` | ClusterRole (cluster-wide mode) |
| `templates/clusterrolebinding.yaml` | ClusterRoleBinding (cluster-wide mode) |
| `templates/role.yaml` | Role (single-namespace mode) |
| `templates/rolebinding.yaml` | RoleBinding (single-namespace mode) |
| `templates/crd.yaml` | CRD manifest (or embedded via `crds/` directory) |
| `NOTES.txt` | Post-install usage instructions |

#### Scenario: helm lint passes

Given the chart source at `charts/redis-operator/`,
when `helm lint charts/redis-operator/` is executed,
then the command MUST exit 0 with no errors or warnings.

#### Scenario: helm template renders valid YAML

Given default `values.yaml`,
when `helm template my-release charts/redis-operator/` is executed,
then all rendered manifests MUST be valid Kubernetes YAML parseable by `kubectl apply --dry-run=client`.

---

### Requirement: Helm Chart Values

`values.yaml` MUST expose the following configuration knobs:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `operator.image.repository` | string | `ghcr.io/example/redis-operator` | Operator container image |
| `operator.image.tag` | string | `""` (uses Chart appVersion) | Image tag override |
| `operator.image.pullPolicy` | string | `IfNotPresent` | Pull policy |
| `operator.replicas` | int | `1` | Number of operator Deployment replicas |
| `operator.resources` | object | `{}` | CPU/memory requests and limits |
| `rbac.create` | bool | `true` | Whether to create RBAC resources |
| `rbac.clusterScoped` | bool | `true` | Cluster-wide vs single-namespace mode |
| `serviceAccount.create` | bool | `true` | Whether to create a ServiceAccount |
| `serviceAccount.name` | string | `""` | Override ServiceAccount name |
| `watchNamespace` | string | `""` | Namespace to watch (empty = all namespaces) |

#### Scenario: Disable RBAC creation

Given `rbac.create: false` in user values,
when `helm template` is run,
then no ClusterRole, ClusterRoleBinding, Role, or RoleBinding manifests MUST be rendered.

#### Scenario: Single-namespace mode

Given `rbac.clusterScoped: false` and `watchNamespace: "my-app"`,
when `helm template` is run,
then the rendered manifests MUST include a Role and RoleBinding scoped to `my-app`, and MUST NOT include ClusterRole or ClusterRoleBinding.

---

### Requirement: Operator Deployment

The operator MUST run as a Kubernetes `Deployment` with the following properties:

- **replicas**: configurable via `operator.replicas` (default 1).
- **Image**: `operator.image.repository:operator.image.tag`.
- **ServiceAccountName**: the created or pre-existing ServiceAccount.
- **Security context**: `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`.
- **Resource requests/limits**: set via `operator.resources`.
- **Leader election**: enabled via `--leader-elect` flag so multiple replicas do not conflict.

#### Scenario: Operator pod runs as non-root

Given the chart is installed with default values,
when the operator pod starts,
then the pod security context MUST set `runAsNonRoot: true` and the container MUST run as a non-zero UID.

#### Scenario: Leader election prevents split-brain

Given `operator.replicas: 2`,
when both operator pods are running,
then only one pod MUST be the active leader (holding the leader election lock); the other MUST be in standby.

#### Scenario: Operator restarts do not orphan Redis resources

Given a running `Redis` CR and the operator pod is restarted,
when the new pod starts and reconciles,
then all existing Redis StatefulSets, Services, and ConfigMaps MUST remain intact and the CR status MUST be re-synced.

---

### Requirement: RBAC

Refer to `design.md` for the full permission list. The Helm chart MUST render all required RBAC resources and bind them to the operator ServiceAccount.

#### Scenario: ClusterRole contains all required verbs

Given the chart is installed in cluster-wide mode,
when `kubectl get clusterrole redis-operator -o yaml` is inspected,
then the ClusterRole MUST include all verbs listed in `design.md` for each resource group.

#### Scenario: Minimal permissions in single-namespace mode

Given `rbac.clusterScoped: false`,
when the chart is installed,
then the Role MUST be scoped to `watchNamespace` and MUST NOT grant cluster-level permissions.

---

### Requirement: Kustomize Overlay

In addition to the Helm chart, the repository MUST include a Kustomize-compatible layout under `config/` (generated by kubebuilder) as an alternative deployment path.

Required directories:

| Path | Contents |
|------|----------|
| `config/crd/` | CRD manifests |
| `config/rbac/` | RBAC manifests |
| `config/manager/` | Operator Deployment and ServiceAccount |
| `config/default/` | Root kustomization composing the above |

#### Scenario: kustomize build succeeds

Given the `config/default/kustomization.yaml` is complete,
when `kubectl kustomize config/default` is run,
then the output MUST be valid Kubernetes YAML with no errors.

#### Scenario: Namespace overlay works

Given a user creates `config/overlays/my-namespace/kustomization.yaml` that sets `namespace: my-namespace`,
when `kubectl kustomize config/overlays/my-namespace` is run,
then all rendered resources MUST have `metadata.namespace: my-namespace`.

---

### Requirement: CRD Installation Order

CRDs MUST be installed on the cluster before the operator Deployment starts, ensuring the operator can register its watches at startup.

#### Scenario: Helm pre-install hook installs CRDs first

Given the CRD is placed in the Helm `crds/` directory (or uses a `pre-install` hook Job),
when `helm install` is run on a fresh cluster,
then CRD establishment MUST complete before the operator Deployment is created.

#### Scenario: Operator fails fast if CRD missing

Given the CRD is not installed (e.g. manual Kustomize deployment without CRD step),
when the operator starts and cannot register its watcher,
then the operator MUST log a clear error and exit with a non-zero exit code rather than silently looping.

---

### Requirement: Health and Readiness Probes for the Operator Pod

The operator Deployment MUST include HTTP health probes served by `controller-runtime`'s built-in health endpoint.

| Probe | Path | Port | Initial Delay | Period |
|-------|------|------|---------------|--------|
| Liveness | `/healthz` | `8081` | 15s | 20s |
| Readiness | `/readyz` | `8081` | 5s | 10s |

#### Scenario: Operator pod becomes Ready

Given the operator container starts and the controller manager initialises,
when the readiness probe polls `/readyz`,
then the probe MUST return HTTP 200 once all controllers and webhooks are registered.

#### Scenario: Failed health check restarts pod

Given the operator health check endpoint returns HTTP 500 (e.g. leader lock lost),
when the liveness probe fails three consecutive times,
then Kubernetes MUST restart the operator container.
