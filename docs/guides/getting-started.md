<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Getting Started

This guide takes a platform team from an empty cluster to a single
cluster-wide health verdict in about fifteen minutes. You will install the
Fathom operator, declare a check against an existing add-on (cert-manager),
and roll its result up into one `ClusterHealth` object your dashboards and
gates can read.

If you want the conceptual picture first — what each resource is for and how
status flows between them — read [Concepts](concepts.md). You can also follow
this guide top-to-bottom and pick up the model as you go.

## Prerequisites

- A Kubernetes cluster you can reach with `kubectl`. Fathom is tested against
  the Kubernetes version pinned in the repo's e2e fixtures (currently v1.36);
  recent releases are expected to work.
- `kubectl` configured for that cluster's context.
- [Helm](https://helm.sh/) v3.8+ (the operator ships as an OCI chart, which
  requires OCI support — standard in v3.8 and later).
- At least one add-on Fathom can check. This guide uses
  [cert-manager](https://cert-manager.io/); the same shape applies to CoreDNS,
  External Secrets Operator, and Cilium (see [Add-on checks](addon-checks.md)).
- Permission to install CRDs and a cluster-scoped operator (typically
  cluster-admin for the install step).

> If you do not already run cert-manager, install it first (its own
> documentation covers this) or substitute one of the other built-in adapters
> from [Add-on checks](addon-checks.md). Fathom validates add-ons; it does not
> install them.

## 1. Install the operator

Fathom publishes an OCI-only Helm chart to GHCR. It installs the operator
Deployment, its CRDs, and the RBAC the operator needs:

```sh
helm install fathom oci://ghcr.io/skaphos/charts/fathom-operator \
  --version X.Y.Z \
  -n fathom-system --create-namespace
```

Replace `X.Y.Z` with a released chart version (plain semver, no leading `v`).
See the chart's GHCR page or the project releases for the current version.

A few things worth knowing up front:

- **CRDs install once.** Helm installs CRDs from the chart's `crds/` directory
  on first install only; it never upgrades or removes them. Before a breaking
  `helm upgrade`, apply the new CRDs with `kubectl` yourself.
- **The probe image is not a separate Deployment.** Some checks launch
  short-lived probe pods on demand; you do not run or scale them. Point at a
  specific build with `--set probeImage.tag=vX.Y.Z` if you mirror images
  privately.
- **Metrics are HTTPS by default** on `:8443` behind controller-runtime's
  authn/authz filter. See [Monitoring](monitoring.md) for scraping.

Confirm the operator is running:

```sh
kubectl -n fathom-system get deploy,pod
kubectl get crd | grep fathom.skaphos.io
```

You should see the controller-manager Deployment `Available` and the
`fathom.skaphos.io` CRDs registered.

## 2. Declare your first check

An `AddonCheck` declares *what to check* and *how strict to be*. It selects a
built-in adapter via `spec.addonType` and enables one or more **check
families** under `spec.policy`. Save this as `cert-manager-check.yaml`:

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: cert-manager-system-health
  namespace: fathom-system
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
        restartWarnCount: "3"
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

Apply it:

```sh
kubectl apply -f cert-manager-check.yaml
```

`AddonCheck` is the only resource that *drives work*: the operator resolves the
`cert-manager` adapter, runs the enabled families against your cluster, writes
the outcome to `status`, and records the run as an immutable `HealthReport`.

> **Set expectations on cadence.** In the current build `spec.interval` is
> accepted but **not yet honored** for `AddonCheck` — a run is triggered when
> the check is first created and whenever you change its spec, not on a timer.
> To force a fresh run, edit the spec (any generation-changing edit re-runs the
> adapter). Periodic requeue is a tracked limitation; see
> [Add-on checks → Run cadence](addon-checks.md#run-cadence-and-the-interval-caveat).
> `NodeCertificateCheck` — a feature-gated kind not present in every build (see
> its [Availability note](node-certificate-checks.md#availability)) — *does*
> honor `interval`.

## 3. Read the result

Check the rolled-up result on the `AddonCheck` itself:

```sh
kubectl -n fathom-system get addoncheck cert-manager-system-health
kubectl -n fathom-system describe addoncheck cert-manager-system-health
```

`status.lastResult` is the worst-case outcome across the enabled families,
using Fathom's severity ordering:

```
Pass < Skipped < Warn < Unknown < Fail < Error
```

So a single failing certificate makes the whole check `Fail`; a missing,
not-applicable path reports `Skipped` (which rolls up green). The `Accepted`,
`Ready`, and (if set) `Paused` conditions explain *why* a check is in its
current state — for example `Ready=False / MissingAdapter` if `addonType` is
misspelled.

Every run is also persisted as a `HealthReport` for history and audit:

```sh
kubectl -n fathom-system get healthreport \
  -l fathom.skaphos.io/source-kind=AddonCheck \
  -l fathom.skaphos.io/source-name=cert-manager-system-health
```

The newest report holds the per-family, per-target detail; older reports are
pruned to `spec.historyLimit` (default 10).

## 4. Roll checks up into one verdict

Most platform teams do not want to watch individual checks — they want one
green/red signal for "is the platform healthy." Fathom provides that in two
thin layers:

- **`HealthCheck`** wraps one check and mirrors its status into a uniform
  shape. It is the join point for aggregation.
- **`ClusterHealth`** aggregates the `HealthCheck`s in its namespace into a
  single worst-case `Result`.

Add both (save as `rollup.yaml`):

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: cert-manager
  namespace: fathom-system
spec:
  description: "Mirror the cert-manager AddonCheck for ClusterHealth aggregation."
  checkRef:
    apiVersion: fathom.skaphos.io/v1alpha1
    kind: AddonCheck
    name: cert-manager-system-health
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: ClusterHealth
metadata:
  name: platform
  namespace: fathom-system
spec:
  description: "Worst-case Result across every HealthCheck in this namespace."
  # An omitted/empty selector matches all HealthChecks in the same namespace.
```

```sh
kubectl apply -f rollup.yaml
kubectl -n fathom-system get clusterhealth platform
```

`ClusterHealth.status.result` is now the single verdict over every wrapped
check in the namespace. As you add more `AddonCheck`s and wrap each in a
`HealthCheck`, they all fold into this one object automatically.

> `ClusterHealth` aggregates `HealthCheck` status **only** — never raw
> `HealthReport` history — so its external contract stays stable regardless of
> how Fathom stores history. Aggregation is same-namespace in this build.

## What you have now

```
AddonCheck ──runs──▶ status + HealthReport (history)
     │
  HealthCheck ──mirrors──▶ uniform status
     │
  ClusterHealth ──aggregates──▶ one cluster-wide verdict
```

## Next steps

- **[Concepts](concepts.md)** — the full mental model: the resource kinds,
  what drives work vs. aggregates, and result severity.
- **[Add-on checks](addon-checks.md)** — every built-in adapter
  (cert-manager, CoreDNS, External Secrets, Cilium), their families, and the
  threshold knobs.
- **[Node certificate checks](node-certificate-checks.md)** — *(feature-gated;
  see its Availability note)* scan on-disk X.509 certificates on every node and
  alert before they expire.
- **[Monitoring & alerting](monitoring.md)** — scrape Fathom's metrics, wire
  tracing, and alert on results.
- **[Configuration reference](../reference/configuration.md)** — every operator
  flag, env var, and config-file key.
