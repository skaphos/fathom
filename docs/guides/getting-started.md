<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
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
  [cert-manager](https://cert-manager.io/); the same shape applies to the
  other fifteen built-in adapters (see [Add-on checks](addon-checks.md)).
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
  `helm upgrade`, apply the new CRDs with `kubectl` yourself. The exception is
  a CRD whose **scope** changed (e.g. `clusterhealths` became cluster-scoped in
  chart 0.2.13): the API server rejects an in-place scope change, so delete the
  old CRD first (`kubectl delete crd clusterhealths.fathom.skaphos.io` — this
  removes its objects), apply the new one, and recreate your resources.
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

> **Set expectations on cadence.** `spec.interval` drives periodic
> `AddonCheck` re-runs and defaults to `5m`. To force an immediate run outside
> that cadence, set a fresh `fathom.skaphos.io/run-now` annotation value; see
> [Add-on checks → Run cadence](addon-checks.md#run-cadence).

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
misspelled. The complete condition reason table is in
[Status and conditions](../reference/status-conditions.md).

Every run is also persisted as a `HealthReport` for history and audit:

```sh
kubectl -n fathom-system get healthreport \
  -l 'fathom.skaphos.io/source-kind=AddonCheck,fathom.skaphos.io/source-name=cert-manager-system-health'
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
spec:
  description: "Worst-case Result across every HealthCheck in the cluster."
  # ClusterHealth is cluster-scoped. An omitted/empty selector matches all
  # HealthChecks; narrow namespaces with:
  #   namespaces: [...]            # allowlist (definitive when set)
  #   excludedNamespaces: [...]    # denylist (only when namespaces is empty)
```

```sh
kubectl apply -f rollup.yaml
kubectl get clusterhealth platform
```

`ClusterHealth.status.result` is now the single verdict over every wrapped
check in scope. As you add more `AddonCheck`s and wrap each in a
`HealthCheck`, they all fold into this one object automatically.

> `ClusterHealth` aggregates `HealthCheck` status **only** — never raw
> `HealthReport` history — so its external contract stays stable regardless of
> how Fathom stores history. It selects `HealthCheck` wrappers under the
> allowlist / denylist / open namespace filter; a selected wrapper may mirror
> an explicit cross-namespace `checkRef.namespace`.

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
- **[Add-on checks](addon-checks.md)** — all sixteen built-in adapters,
  their families, and the threshold knobs.
- **[Node certificate checks](node-certificate-checks.md)** — *(newer kind;
  see its Availability note)* scan on-disk X.509 certificates on every node and
  alert before they expire.
- **[Monitoring & alerting](monitoring.md)** — scrape Fathom's metrics, wire
  tracing, and alert on results.
- **[Configuration reference](../reference/configuration.md)** — every operator
  flag, env var, and config-file key.
