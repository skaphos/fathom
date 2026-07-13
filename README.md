# Fathom

**Kubernetes platform-integrity validation operator and CLI.**

Fathom is a Kubernetes operator (`fathom.skaphos.io`) that continuously
validates the integrity of your platform — the add-ons and node-level material
your workloads depend on — and rolls the results into a single, machine-readable
cluster-wide verdict.

It answers questions like *"is cert-manager actually healthy, and are its
certificates going to expire on me?"* or *"is CoreDNS resolving, and are any
node certificates about to lapse?"* — and gives you one object your dashboards,
alerts, and deployment gates can read to know whether the platform is sound.

Fathom **validates** existing add-ons; it does not install or manage them.

## What it checks

- **Add-on health** — built-in adapters for cert-manager, CoreDNS, External
  Secrets Operator, Cilium, external-dns, metrics-server, Envoy Gateway, istio,
  and more. Each adapter runs targeted check families (deployment/pod health,
  DNS resolution, certificate expiry, secret sync, CRD readiness, …).
- **Node certificates** — a hardened node-agent DaemonSet scans on-disk X.509
  certificates on every node and warns before an expiring cert can take the
  cluster down.
- **One cluster-wide verdict** — per-check results roll up through
  `HealthCheck` into a cluster-scoped `ClusterHealth` object, with immutable
  `HealthReport` run history for auditing and trend analysis.

## Quick start

Fathom ships an OCI-only Helm chart published to GHCR. Install the operator,
its CRDs, and RBAC with:

```sh
helm install fathom oci://ghcr.io/skaphos/charts/fathom-operator \
  --version X.Y.Z \
  -n fathom-system --create-namespace
```

Replace `X.Y.Z` with a released chart version (plain semver, no leading `v`).

> **New here?** The [Getting started guide](docs/guides/getting-started.md)
> walks a platform team from an empty cluster to one cluster-wide verdict in
> about fifteen minutes, with full explanation at each step. Start there.

A minimal `AddonCheck` that watches cert-manager's core system health:

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
    issuer_health:
      enabled: true
    certificate_health:
      enabled: true
      thresholds:
        warnDays: "30"
        failDays: "7"
```

To turn a check into a cluster-wide signal, wrap it in a `HealthCheck` and
select it from a `ClusterHealth`. See
[Add-on checks](docs/guides/addon-checks.md) for every adapter, its check
families, and thresholds, and [Concepts](docs/guides/concepts.md) for how the
`AddonCheck → HealthCheck → ClusterHealth` chain fits together.

## Documentation

Full documentation lives in [`docs/`](docs/README.md).

**Platform teams — start with the guides:**

- [Getting started](docs/guides/getting-started.md) — install the operator and
  reach one cluster-wide verdict in ~15 minutes.
- [Concepts](docs/guides/concepts.md) — the mental model for using Fathom.
- [Add-on checks](docs/guides/addon-checks.md) — configure checks for
  cert-manager, CoreDNS, External Secrets, Cilium, external-dns, metrics-server,
  Envoy Gateway, and istio.
- [Node certificate checks](docs/guides/node-certificate-checks.md) — scan
  on-disk certificates on every node.
- [Monitoring & alerting](docs/guides/monitoring.md) — metrics, tracing,
  alerts, and deployment gates.

**Reference and internals:**

- [Architecture](docs/architecture.md) — CRD model, the
  AddonCheck → HealthCheck → ClusterHealth aggregation chain, reconcilers,
  adapter contract, probe-pod model.
- [API reference](docs/reference/api.md) — generated CRD reference for
  `fathom.skaphos.io/v1alpha1`.
- [Status and conditions](docs/reference/status-conditions.md) — operational
  meaning of status fields and condition reasons.
- [Configuration reference](docs/reference/configuration.md) — every flag, env
  var, and config-file key.
- [RBAC reference](docs/reference/rbac.md) — generated least-privilege adapter
  permission matrix.
- [Code map](docs/code-map.md) — internal package tour for contributors.
- [Architecture Decision Records](docs/adr/) — ADR-0001 … ADR-0004.

**Automating Fathom?** If you are pointing an AI agent or automation at a
cluster to install or operate Fathom, see the prescriptive
[agent operations runbook](docs/guides/agent-operations.md).

## Installation details

The chart installs the five `fathom.skaphos.io` CRDs from its native `crds/`
directory. Helm installs CRDs on first install only and never upgrades or
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

## Node certificate checks

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
cluster-wide, via `--node-agent-image` / `FATHOM_NODE_AGENT_IMAGE`. See the
[Node certificate checks guide](docs/guides/node-certificate-checks.md) for the
full walkthrough.

## Probe pods

Fathom has a shared lightweight probe-pod path under `internal/probe` plus a
tiny Go probe binary in `cmd/probe`. The probe image is built from
`Dockerfile.probe` and runs on `scratch` as a static binary with no shell or
package manager. Supported probe modes:

- `dns` — resolve a DNS name from inside the probe Pod
- `tcp-connect` — attempt a TCP connection to a target/port
- `tcp-listen` — run a TCP listener for peer connectivity tests

The shared pod builder applies a default hardening profile (no service account
token, non-root UID, read-only root filesystem, all capabilities dropped, no
privilege escalation, runtime-default seccomp, small CPU/memory requests) and
supports pod anti-affinity so network checks can place client/server probe Pods
on different nodes.

## Contributing & development

Contributor and AI-agent build/test/coding guardrails live in
[`AGENTS.md`](AGENTS.md) (symlinked as `CLAUDE.md`). The contributor workflow —
branch naming, DCO sign-off, and PR expectations — is in
[`CONTRIBUTING.md`](CONTRIBUTING.md). New to the source tree? Start with the
[code map](docs/code-map.md).

## License

Fathom is licensed under the terms in [`LICENSE`](LICENSE); the repository is
[REUSE](https://reuse.software/)-compliant (see [`LICENSES/`](LICENSES/)).
