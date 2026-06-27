<!--
SPDX-FileCopyrightText: 2026 Skaphos
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
| `helmfile.yaml` | Installs cert-manager + external-secrets via their official charts. CoreDNS is preinstalled by kind and is not managed here. |

The AddonCheck samples used by this stack live in `config/samples/`:

- `fathom_v1alpha1_addoncheck_coredns.yaml` — exercises CoreDNS `system_health` + `dns_resolution`.
- `fathom_v1alpha1_addoncheck.yaml` — exercises cert-manager `system_health`, `issuer_health`, `certificate_health`.
- `fathom_v1alpha1_addoncheck_external_secrets.yaml` — exercises external-secrets `system_health` + `secret_sync`.

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

# 2. Install cert-manager + external-secrets via helmfile.
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
