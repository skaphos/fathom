<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Fathom End-to-End Fixtures

This directory holds the local stack scaffolding for running Fathom against a
real kind cluster with real addons installed. It is the manual bootstrap that
the Go e2e suite under `test/e2e/` will eventually drive automatically.

## Layout

| File | Purpose |
| ---- | ------- |
| `kind-cluster.yaml` | Single-node kind cluster, `kindest/node` pinned by digest to `v1.36.1` (matches `ENVTEST_K8S_VERSION` 1.36 in `Taskfile.yml`; kind only publishes specific patch tags per release, so use a real one). |
| `helmfile.yaml` | The tiered addon stack (see below): Cilium (the cluster CNI) + cert-manager + external-secrets always; external-dns, metrics-server, Envoy Gateway, istio (sidecar mode: base + istiod), Argo CD, NodeLocal DNSCache, and the azure-workload-identity webhook, and kube-state-metrics as per-addon opt-ins — via their official charts, except NodeLocal DNSCache, which uses the community deliveryhero chart (upstream ships raw manifests only). CoreDNS is preinstalled by kind and is not managed here. |

## Tiered Stack & Scoped Runs

The stack is tiered so per-adapter e2e stays fast as the adapter count grows
([skaphos/fathom#178]): a single monolithic cluster installing every addon
would make every adapter PR pay the full-stack cost.

[skaphos/fathom#178]: https://github.com/skaphos/fathom/issues/178

- **Core tier** (helmfile label `tier: core`, Ginkgo label `core`): Cilium,
  cert-manager, external-secrets — plus CoreDNS, which kind preinstalls. This
  tier is installed on *every* sync (Cilium is the CNI; nothing schedules
  without it) and its specs include the operator-infrastructure suites
  (Manager, RBAC impersonation, NodeCertificateCheck, refresh-on-change).
- **Opt-in addons** (helmfile label `addon: <name>`, no `tier: core`):
  external-dns, metrics-server, envoy-gateway, istio, argocd,
  node-local-dns, azure-workload-identity, kube-state-metrics. Each installs
  only when selected,
  layered on the core tier. The azure-workload-identity webhook needs no
  Azure cloud access: the chart only wants a tenant ID string, so it runs
  self-contained in kind.

The `E2E_ADDONS` variable (environment variable or Task var) selects a slice
of the stack. It scopes **both** what helmfile installs and which specs the
Ginkgo suite runs (addon names double as spec labels):

| `E2E_ADDONS` | Installs | Runs |
| ------------ | -------- | ---- |
| unset / `all` | everything | every spec |
| `core` | core tier only | core-labeled specs (operator infra + core-tier addons) |
| `istio` | core tier + istio charts | istio specs only |
| `cert-manager` | core tier (cert-manager is in it) | cert-manager specs only |
| `core,istio,external-dns` | core tier + istio + external-dns | the union of their specs |

Examples:

```sh
# Full historical behaviour (default).
go -C tools tool task test-e2e

# An adapter PR validating just its own addon.
go -C tools tool task test-e2e E2E_ADDONS=istio

# Same thing against an already-running cluster.
E2E_ADDONS=istio go test ./test/e2e/ -v -ginkgo.v
```

Unknown addon names are a hard error in the Go suite — a typo fails loudly
instead of filtering the run down to zero specs and reporting success. An
explicit `-ginkgo.label-filter` flag overrides the spec filter derived from
`E2E_ADDONS` (but not the install set).

In CI, `.github/workflows/e2e.yml` shards the suite across parallel kind
clusters: a `plan` job path-filters the PR diff via `scripts/e2e-shards.sh`,
so an adapter-scoped PR runs only its own shard plus `core`, docs-only PRs
run nothing, and shared-surface changes (or pushes to `main`) run the full
matrix. The branch-protection check is the aggregate `kind e2e` job.

### Adding an addon to the stack

For the full adapter workflow see `docs/authoring-adapters.md`; the e2e-stack
steps are:

1. Add the chart release(s) to `helmfile.yaml` with an `addon: <name>` label
   (every release of a multi-chart addon shares the label). Only genuinely
   foundational addons get `tier: core` — the core tier is meant to stay
   small.
2. Label the addon's Ginkgo spec `Label("<name>")` (core-tier addons:
   `Label(utils.CoreLabel, "<name>")`).
3. For an opt-in addon, add `<name>` to `optInAddons` in
   `test/utils/utils.go` and to `OPT_IN_SHARDS` in `scripts/e2e-shards.sh`,
   plus the addon's path patterns to `shard_for_file` there. Drift guards
   (`test/utils/utils_test.go`, `scripts/e2e_shards_gate_test.go`) fail if
   the lists disagree with the helmfile labels or each other.
4. Run the scoped suite locally: `task test-e2e E2E_ADDONS=<name>`.

The AddonCheck samples used by this stack live in `config/samples/`:

- `fathom_v1alpha1_addoncheck_coredns.yaml` — exercises CoreDNS `system_health` + `dns_resolution`.
- `fathom_v1alpha1_addoncheck.yaml` — exercises cert-manager `system_health`, `issuer_health`, `certificate_health`.
- `fathom_v1alpha1_addoncheck_external_secrets.yaml` — exercises external-secrets `system_health` + `secret_sync`.
- `fathom_v1alpha1_addoncheck_external_dns.yaml` — exercises external-dns `system_health` + `crd_health` (the chart ships the DNSEndpoint CRD in `crds/`, so it is Established in this stack).
- `fathom_v1alpha1_addoncheck_metrics_server.yaml` — exercises metrics-server `system_health` + `api_availability` (the aggregated `v1beta1.metrics.k8s.io` APIService).
- `fathom_v1alpha1_addoncheck_kube_state_metrics.yaml` — exercises kube-state-metrics `system_health` + `metrics_endpoint` (probe-pod scrapes of the main 8080 `/metrics` port and — because the fixture enables `selfMonitor` — the 8081 self-telemetry port).
- `fathom_v1alpha1_addoncheck_envoy_gateway.yaml` — exercises envoy-gateway `system_health` + `crd_health` + the `gateway_status` empty-cluster Skipped contract (no Gateway objects are declared).
- `fathom_v1alpha1_addoncheck_istio.yaml` — exercises istio `system_health` (istiod + both webhook configurations) + `crd_health`, plus the `ztunnel_health`/`istio_cni_health` Optional-absence Skipped contract (the stack installs sidecar mode only).
- `fathom_v1alpha1_addoncheck_argocd.yaml` — exercises argocd `system_health` (the application-controller StatefulSet, the repo-server/server/redis Deployments, and the Application/ApplicationSet/AppProject CRDs) + the `sync_health` empty-cluster Skipped contract (no Application objects are declared).
- `fathom_v1alpha1_addoncheck_node_local_dns.yaml` — exercises node-local-dns `system_health` (DaemonSet + per-node coverage) + `dns_resolution` through the cache's `169.254.20.10` listen address (the helmfile pins the chart's `config.localDns` to the upstream convention).
- `fathom_v1alpha1_addoncheck_azure_workload_identity.yaml` — exercises azure-workload-identity `system_health` + `webhook_wiring` (mutating configuration wired, backing service endpoints ready), plus the `projection_sanity` empty-cluster Skipped contract (no pod in the stack opts in via `azure.workload.identity/use=true`).

## Prerequisites

Install the following on `PATH`:

- `docker` (or Podman with the docker shim)
- `kind` (≥ v0.32 — earlier releases don't ship the `kindest/node:v1.36.1` image)
- `kubectl`
- `helm` (v3)
- [`helmfile`](https://github.com/helmfile/helmfile/releases) (v0.x)

## Quick Start

The Taskfile wraps the whole flow. From repo root:

```sh
# 1. Create the kind cluster.
go -C tools tool task e2e:cluster:up

# 2. Install the addon stack via helmfile (E2E_ADDONS=... for a slice of it).
go -C tools tool task e2e:cluster:addons

# 3. Build the operator image, load it into kind, and deploy Fathom.
go -C tools tool task e2e:cluster:fathom

# 4. Apply the AddonCheck samples and watch them reconcile.
go -C tools tool task e2e:cluster:samples

# 5. Tear down when done.
go -C tools tool task e2e:cluster:down
```

`task e2e:up` chains steps 1–4 in order.

## Inspecting AddonCheck Results

Once the samples are applied, Fathom's controllers reconcile each AddonCheck
on its `spec.interval`. To observe:

```sh
# Status conditions live on the AddonCheck.
kubectl get addonchecks -o wide

# Per-family results live in HealthReport history.
kubectl get healthreports -o wide

# Drill into one report.
kubectl describe healthreport <name>
```

For the CoreDNS `dns_resolution` family, Fathom launches short-lived probe
pods in the AddonCheck's namespace. To watch the probe lifecycle:

```sh
kubectl get pods -l app.kubernetes.io/managed-by=fathom -A --watch
```

## Troubleshooting

- **`ImagePullBackOff` on probe pods (CoreDNS `dns_resolution`)**: until
  SKA-311 publishes `ghcr.io/skaphos/fathom-probe:vX.Y.Z` to GHCR, the
  probe image referenced in `config/samples/fathom_v1alpha1_addoncheck_coredns.yaml`
  does not exist remotely. Two workarounds for local testing:

    1. Build the probe image locally with the tag the operator expects and
       load it into kind:

       ```sh
       go -C tools tool task probe-docker-build PROBE_IMG=ghcr.io/skaphos/fathom-probe:v0.1.0
       kind load docker-image ghcr.io/skaphos/fathom-probe:v0.1.0 --name fathom-e2e
       ```

       The default kubelet pull policy for tagged images is `IfNotPresent`,
       so the loaded image satisfies the operator's compiled-in default.

    2. Or disable the `dns_resolution` family on the AddonCheck (set
       `policy.dns_resolution.enabled: false`) and only run `system_health`
       until the probe image is published.

  `e2e:cluster:fathom` builds and loads `fathom-probe:e2e` by default;
  override the `E2E_PROBE_IMG` Task var if you want the loaded tag to match
  the operator's compiled-in default.
- **cert-manager webhook timeouts on first install**: helmfile uses
  `wait: true` and a 10-minute timeout. On a slow machine you may need to
  bump the timeout in `helmfile.yaml`.
- **External-secrets CRDs missing**: ensure the helmfile install completed
  (`helm list -A | grep external-secrets`). The chart installs CRDs by default
  but a partial install can leave them out.
- **Kind cluster won't start**: `kind delete cluster --name fathom-e2e` then
  retry. Docker daemon issues are the usual cause.

## What's NOT Wired Up Yet

This is the manual scaffolding. Go-based assertions that verify Fathom's
AddonCheck status matches the cluster reality are tracked as separate Linear
tickets (CoreDNS, cert-manager, external-secrets each get their own). The
existing `test/e2e/e2e_test.go` still calls `make` targets that no longer
exist in this repo — migrating it to `task` is also a separate ticket.
