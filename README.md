# fathom
Kubernetes platform integrity validation operator and CLI.

## Install via Helm

Fathom ships an OCI-only Helm chart published to GHCR. Install the operator,
its CRDs, and RBAC with:

```sh
helm install fathom oci://ghcr.io/skaphos/charts/fathom-operator \
  --version X.Y.Z \
  -n fathom-system --create-namespace
```

Replace `X.Y.Z` with a released chart version (plain semver, no leading `v`).
The chart installs the four `fathom.skaphos.io` CRDs from its native `crds/`
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
- [Add-on checks](docs/guides/addon-checks.md) — configure checks for cert-manager, CoreDNS, External Secrets, Cilium, and external-dns.
- [Node certificate checks](docs/guides/node-certificate-checks.md) — scan on-disk certificates on every node *(newer kind; requires a build that includes `NodeCertificateCheck`)*.
- [Monitoring & alerting](docs/guides/monitoring.md) — metrics, tracing, alerts, and deployment gates.

Reference and internals:

- [Architecture](docs/architecture.md) — CRD model, the AddonCheck → HealthCheck → ClusterHealth aggregation chain, reconcilers, adapter contract, probe-pod model.
- [API reference](docs/reference/api.md) — generated CRD reference for `fathom.skaphos.io/v1alpha1`.
- [Configuration reference](docs/reference/configuration.md) — every flag, env var, and config-file key.
- [Code map](docs/code-map.md) — internal package tour for contributors.
- [Architecture Decision Records](docs/adr/) — ADR-0001 … ADR-0004.
