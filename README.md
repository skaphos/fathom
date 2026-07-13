# fathom
Kubernetes platform integrity validation operator and CLI.

## Agent Instructions: Set Up and Use Fathom on a Cluster

Automation and AI agents should use this workflow when asked to install or
operate Fathom. Prefer the released Helm chart, delivered through the cluster's
existing GitOps workflow when it has one. Use the repository `deploy` task only
when the user explicitly asks to test the current checkout.

### 1. Confirm the target and release

- Get an explicit target cluster and an exact chart version approved by the
  user or already pinned in its authorized GitOps source. Do not infer a
  production target or install an unpinned `latest` release.
- If live access is authorized, pass the context to every command instead of
  changing the caller's current context. Repeat the workflow separately for
  each approved cluster.
- Fathom validates existing add-ons; it does not install them.

```sh
export KUBE_CONTEXT="your-approved-context"
export FATHOM_VERSION="X.Y.Z"  # released chart version, without a leading v

# Run only when the user has authorized live access.
kubectl --context "${KUBE_CONTEXT}" cluster-info
helm version --short
```

Stop and ask for direction if the context is not the approved target.

### 2. Discover add-ons and the cluster delivery model

Before choosing `AddonCheck`s or changing the cluster, ask the user:

- Are the cluster add-ons defined and reconciled through GitOps?
- If so, which GitOps repositories and paths contain them, and may the agent
  read those repositories?
- Should Fathom and its health-check resources also be delivered through
  GitOps? If so, which repository, path, and review workflow should be used?

When GitOps repositories are available, treat them as the desired-state source
of truth. Scan the relevant Argo CD Applications, Flux HelmReleases and
Kustomizations, Helm values, Helmfile definitions, or plain manifests to
identify supported add-ons and their configured namespaces, release names,
workload names, Services, modes, and versions. Use those values to configure
Fathom thresholds instead of assuming upstream defaults. Do not read Secret
payloads, change the repositories, or apply resources directly unless the user
has authorized that action.

Desired state may differ from live state. Ask before comparing it with the
cluster. If add-ons are not managed through GitOps, repository access is
incomplete, or the user requests live verification, ask permission to run
read-only discovery. Start with Helm releases:

```sh
helm list --all-namespaces --kube-context "${KUBE_CONTEXT}"
kubectl --context "${KUBE_CONTEXT}" get crd
```

Inspect only the workloads and configuration needed to resolve ambiguous
add-on names or namespaces. Summarize the discovered add-ons and proposed check
families for user confirmation before creating health checks.

### 3. Install and verify the operator

Follow the delivery model confirmed above. For GitOps-managed clusters, add a
pinned Fathom chart release to the authorized repository and let its controller
reconcile it; do not run a parallel `helm upgrade` against the cluster. When a
direct installation is explicitly authorized, use this idempotent, pinned Helm
command. `--atomic` removes a failed release, while Helm intentionally leaves
installed CRDs in place.

```sh
kubectl --context "${KUBE_CONTEXT}" auth can-i create customresourcedefinitions.apiextensions.k8s.io
kubectl --context "${KUBE_CONTEXT}" auth can-i create clusterroles.rbac.authorization.k8s.io

helm upgrade --install fathom oci://ghcr.io/skaphos/charts/fathom-operator \
  --version "${FATHOM_VERSION}" \
  --namespace fathom-system \
  --create-namespace \
  --kube-context "${KUBE_CONTEXT}" \
  --atomic --wait --timeout 5m
```

Direct installation requires both permission checks to return `yes`; otherwise
stop and ask the user to use an appropriately authorized identity or GitOps
workflow.

After either delivery path reports success, verify the live rollout:

```sh
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  wait --for=condition=Available deployment \
  --selector app.kubernetes.io/instance=fathom --timeout=2m
kubectl --context "${KUBE_CONTEXT}" get crd \
  addonchecks.fathom.skaphos.io \
  healthchecks.fathom.skaphos.io \
  clusterhealths.fathom.skaphos.io \
  healthreports.fathom.skaphos.io \
  nodecertificatechecks.fathom.skaphos.io
```

For upgrades, read the release notes first. Helm installs CRDs only on the
initial install and does not upgrade them automatically; follow the CRD upgrade
notes in [Getting started](docs/guides/getting-started.md#1-install-the-operator).

### 4. Create a first cluster health signal

Use the confirmed GitOps or live-discovery results to create checks only for
add-ons that exist, with thresholds matching their actual object names and
namespaces. For GitOps-managed clusters, put these resources in the authorized
repository and let the GitOps controller apply them. For an explicitly
authorized direct workflow, the following baseline checks CoreDNS, mirrors its
status through a `HealthCheck`, and limits the cluster-wide aggregate to
wrappers carrying the `platform` label.

```sh
kubectl --context "${KUBE_CONTEXT}" apply -f - <<'EOF'
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: coredns-system-health
  namespace: fathom-system
spec:
  addonType: coredns
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        deploymentName: "coredns"
        serviceName: "kube-dns"
        restartWarnCount: "3"
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: coredns
  namespace: fathom-system
  labels:
    fathom.skaphos.io/aggregate: platform
spec:
  description: "Mirror CoreDNS health for cluster aggregation."
  checkRef:
    apiVersion: fathom.skaphos.io/v1alpha1
    kind: AddonCheck
    name: coredns-system-health
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: ClusterHealth
metadata:
  name: platform
spec:
  description: "Worst-case platform health across selected checks."
  namespaces:
    - fathom-system
  selector:
    matchLabels:
      fathom.skaphos.io/aggregate: platform
EOF

kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  wait --for=condition=Ready addoncheck/coredns-system-health --timeout=2m
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  wait --for=condition=Ready healthcheck/coredns --timeout=2m
kubectl --context "${KUBE_CONTEXT}" \
  wait --for=condition=Ready clusterhealth/platform --timeout=2m
```

If the distribution renames CoreDNS resources, adjust the thresholds before
applying the example. For other supported add-ons and check families, use the
manifests in [`config/samples/`](config/samples/) and the
[Add-on checks guide](docs/guides/addon-checks.md).

### 5. Read, refresh, and troubleshoot results

```sh
# Current check, normalized status, and cluster-wide verdict.
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  get addoncheck,healthcheck
kubectl --context "${KUBE_CONTEXT}" get clusterhealth platform

# Immutable run history and per-target details.
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  get healthreport \
  --selector 'fathom.skaphos.io/source-kind=AddonCheck,fathom.skaphos.io/source-name=coredns-system-health' \
  --sort-by=.metadata.creationTimestamp

# Force one immediate run by changing the one-shot annotation value.
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  annotate addoncheck coredns-system-health \
  fathom.skaphos.io/run-now="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite

# Inspect conditions and operator logs when a result is unexpected.
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  describe addoncheck coredns-system-health
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  logs --selector app.kubernetes.io/instance=fathom \
  --container manager --tail=100
```

Treat `Pass` and `Skipped` as green outcomes, investigate `Warn` and `Unknown`,
and treat `Fail` and `Error` as unhealthy. Read `status.conditions` and the
newest `HealthReport` before changing a check or the cluster. `ClusterHealth`
is cluster-scoped and derives its verdict only from selected `HealthCheck`
status; it never aggregates `HealthReport` history directly.

### 6. Remove only what was authorized

Remove GitOps-managed resources from their source repository and allow the
controller to prune them; do not fight reconciliation with direct deletes or a
parallel Helm uninstall. For a direct installation, delete agent-created
examples before uninstalling the release. Do not delete Fathom CRDs unless the
user explicitly authorizes deleting all Fathom custom resources and their
report history from the cluster.

```sh
kubectl --context "${KUBE_CONTEXT}" delete clusterhealth platform
kubectl --context "${KUBE_CONTEXT}" --namespace fathom-system \
  delete healthcheck/coredns addoncheck/coredns-system-health
helm uninstall fathom --namespace fathom-system --kube-context "${KUBE_CONTEXT}"
```

## Install via Helm

Fathom ships an OCI-only Helm chart published to GHCR. Install the operator,
its CRDs, and RBAC with:

```sh
helm install fathom oci://ghcr.io/skaphos/charts/fathom-operator \
  --version X.Y.Z \
  -n fathom-system --create-namespace
```

Replace `X.Y.Z` with a released chart version (plain semver, no leading `v`).
The chart installs the five `fathom.skaphos.io` CRDs from its native `crds/`
directory — Helm installs CRDs on first install only and never upgrades or
removes them, so apply new CRDs with `kubectl` before a breaking `helm upgrade`.

The probe is not a separate Deployment; the operator launches it as short-lived
pods. Point at a specific probe build with `--set probeImage.tag=vX.Y.Z`
(defaults to the chart's appVersion). Metrics are served over HTTPS on `:8443`
using controller-runtime's built-in authn/authz filter; see
`deploy/helm/fathom-operator/values.yaml` for the full value reference.

The chart sources its CRDs and the manager ClusterRole rules from `config/`
(kustomize stays the source of truth). Regenerate the derived bits with
`go -C tools tool task helm:sync`.

Each addon adapter runs under its **own least-privilege ServiceAccount**: the
operator holds no addon read permissions and instead impersonates a per-addon
ServiceAccount for each check, so an adapter reads exactly what it declares. The
per-addon ServiceAccounts, read-only ClusterRoles, and the operator's scoped
`impersonate` Role are generated from the adapters (`task gen:addon-rbac`); the
full permission matrix is [`docs/reference/rbac.md`](docs/reference/rbac.md).

## AddonCheck Example

The built-in cert-manager adapter supports `system_health`, `issuer_health`, and
`certificate_health` families. `system_health` checks the core cert-manager
deployments, their matching pods, required cert-manager CRDs, and optionally the
webhook Service plus admission webhook configuration. `issuer_health` checks
`Issuer` and `ClusterIssuer` readiness. `certificate_health` checks Certificate
readiness, renewal timing, expiry thresholds, issuer references, and secret
linkage. Set the cert-manager name thresholds for distributions that rename the
controller, webhook, cainjector, Service, or webhook configuration objects.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: cert-manager-system-health
spec:
  addonType: cert-manager
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      thresholds:
        controllerName: "cert-manager"
        webhookName: "cert-manager-webhook"
        cainjectorName: "cert-manager-cainjector"
        webhookServiceName: "cert-manager-webhook"
        webhookConfigName: "cert-manager-webhook"
        restartWarnCount: "3"
        webhookProbe: "true"
    issuer_health:
      enabled: true
      thresholds:
        kinds: "Issuer,ClusterIssuer"
    certificate_health:
      enabled: true
      thresholds:
        warnDays: "30"
        failDays: "7"
```

The built-in CoreDNS adapter supports `system_health` and `dns_resolution`
families. `system_health` checks the CoreDNS deployment, matching pods,
`kube-dns` Service, EndpointSlices, and optionally a node-count autoscaler
deployment. Set `deploymentName`, `serviceName`, and `autoscalerName` for
distributions such as RKE2 that rename CoreDNS objects. `dns_resolution`
launches a short-lived probe Pod per target in the AddonCheck's namespace
(per ADR-0003, so the resolver topology matches workloads rather than the
operator pod) and records the per-target outcome plus resolver latency.
Override `probeImage` if the default image tag is not pullable from your
cluster.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: coredns-health
spec:
  addonType: coredns
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        deploymentName: "coredns"
        serviceName: "kube-dns"
        restartWarnCount: "3"
        autoscalerName: ""
    dns_resolution:
      enabled: true
      thresholds:
        targets: "kubernetes.default.svc.cluster.local"
        probeImage: "ghcr.io/skaphos/fathom-probe:v0.0.2"
```

The built-in External Secrets Operator adapter supports `system_health` and
`secret_sync` families. `system_health` checks controller deployments, pods, and
required ESO CRDs. `secret_sync` checks `ExternalSecret` readiness, stale refresh
state, failure reasons, store references, and target secret linkage.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: external-secrets-health
spec:
  addonType: external-secrets
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      thresholds:
        restartWarnCount: "3"
    secret_sync:
      enabled: true
      thresholds:
        staleMinutes: "60"
```

The built-in Cilium (CNI baseline) adapter supports `control_plane_health`,
`agent_health`, and `crd_health` families. `control_plane_health` checks the
`cilium-operator` Deployment and its pods. `agent_health` checks the `cilium`
agent DaemonSet and its pods. `crd_health` checks that the core Cilium
CustomResourceDefinitions are established and serve a supported version (`v2` /
`v2alpha1`). Unlike the cert-manager and External Secrets adapters, which `Fail`
when a required component is missing, the Cilium adapter reports `Skipped` (which
rolls up green) when Cilium is not installed at all — the operator Deployment,
agent DaemonSet, and CRDs are all absent — so a `cilium` AddonCheck stays quiet
on clusters that may or may not run Cilium. A workload that exists but is
unhealthy still reports `Fail`. Set `operatorDeploymentName` and
`agentDaemonSetName` for non-standard installs.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: cilium-health
spec:
  addonType: cilium
  interval: 5m
  timeout: 30s
  policy:
    control_plane_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        operatorDeploymentName: "cilium-operator"
        restartWarnCount: "3"
    agent_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        agentDaemonSetName: "cilium"
        restartWarnCount: "3"
    crd_health:
      enabled: true
```

## Probe Pods

Fathom has a shared lightweight probe-pod path under `internal/probe` plus a
tiny Go probe binary in `cmd/probe`. The probe image is built from
`Dockerfile.probe` and runs on `scratch` as a static binary with no shell or
package manager.

Supported probe modes are currently:

- `dns`: resolve a DNS name from inside the probe Pod
- `tcp-connect`: attempt a TCP connection to a target/port
- `tcp-listen`: run a TCP listener for peer connectivity tests

Build helpers:

```sh
go -C tools tool task probe-build
go -C tools tool task probe-docker-build PROBE_IMG=example.com/fathom-probe:latest
```

The shared pod builder applies the default hardening profile used for probe
workloads: no service account token, non-root UID, read-only root filesystem,
all capabilities dropped, no privilege escalation, runtime-default seccomp, and
small CPU/memory requests. It also supports pod anti-affinity so future network
checks can place client/server probe Pods on different nodes.

## Node Certificate Checks

A `NodeCertificateCheck` continuously scans **on-disk X.509 certificates on each
node** and reports time-to-expiry before an expiring certificate can take the
cluster down. The operator runs the scan via a dedicated, hardened node-agent
DaemonSet (`cmd/node-agent`, built from `Dockerfile.node-agent` — its own image,
distinct from the operator and probe images). Each agent reads the configured
paths over **read-only `hostPath` mounts**, exports a
`fathom_node_certificate_expiry_days` gauge, and publishes a per-node result
that the operator rolls up into a `HealthReport`.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: NodeCertificateCheck
metadata:
  name: node-certificates
spec:
  # Omit paths to use the built-in, distribution-agnostic default set
  # (kubeadm/k3s/RKE2/etcd/kubelet locations); paths absent on a node are
  # skipped, never failed. Or pin explicit files/directories:
  paths:
    - /etc/kubernetes/pki
    - /etc/kubernetes/admin.conf
    - /var/lib/kubelet/pki
  warnDays: 30      # <= this many days to expiry -> Warn
  criticalDays: 7   # <= this many days (or expired) -> Fail
  interval: 1h
```

The agent defaults to non-root (uid 65532); it reads world-readable certificate
files (e.g. kubeadm's `*.crt`, mode `0644`) and silently skips root-only
material it cannot read (e.g. `etcd` keys). Configure the DaemonSet image once,
cluster-wide, via `--node-agent-image` / `FATHOM_NODE_AGENT_IMAGE`. Build it
with:

```sh
go -C tools tool task node-agent-docker-build NODE_AGENT_IMG=example.com/fathom-node-agent:latest
```

## Documentation

Full documentation lives in [`docs/`](docs/README.md).

**New here? Platform teams should start with the guides:**

- [Getting started](docs/guides/getting-started.md) — install the operator and reach one cluster-wide verdict in ~15 minutes.
- [Concepts](docs/guides/concepts.md) — the mental model for using Fathom.
- [Add-on checks](docs/guides/addon-checks.md) — configure checks for cert-manager, CoreDNS, External Secrets, Cilium, external-dns, metrics-server, Envoy Gateway, and istio.
- [Node certificate checks](docs/guides/node-certificate-checks.md) — scan on-disk certificates on every node *(newer kind; requires a build that includes `NodeCertificateCheck`)*.
- [Monitoring & alerting](docs/guides/monitoring.md) — metrics, tracing, alerts, and deployment gates.

Reference and internals:

- [Architecture](docs/architecture.md) — CRD model, the AddonCheck → HealthCheck → ClusterHealth aggregation chain, reconcilers, adapter contract, probe-pod model.
- [API reference](docs/reference/api.md) — generated CRD reference for `fathom.skaphos.io/v1alpha1`.
- [Status and conditions](docs/reference/status-conditions.md) — operational meaning of status fields and condition reasons.
- [Configuration reference](docs/reference/configuration.md) — every flag, env var, and config-file key.
- [RBAC reference](docs/reference/rbac.md) — generated least-privilege adapter permission matrix.
- [Code map](docs/code-map.md) — internal package tour for contributors.
- [Architecture Decision Records](docs/adr/) — ADR-0001 … ADR-0004.
