<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Add-on Checks

An `AddonCheck` validates one platform add-on. It picks a built-in **adapter**
with `spec.addonType` and enables one or more **check families** under
`spec.policy`. This guide is the working reference for the four built-in
adapters, their families, and the threshold knobs you'll actually set.

New to the model? Start with [Concepts](concepts.md) and
[Getting started](getting-started.md).

## The shape of an AddonCheck

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: <name>
  namespace: <where you keep your checks, e.g. fathom-system>
spec:
  addonType: <cert-manager | coredns | external-secrets | cilium>
  interval: 5m          # accepted; see "Run cadence" below
  timeout: 30s          # per-run bound; defaults to 30s
  historyLimit: 10      # HealthReports kept per check (min 1)
  policy:
    <family>:
      enabled: true
      namespaces:        # optional; where the add-on's workloads live
        - kube-system
      thresholds:        # string-keyed knobs, family-specific
        someKey: "someValue"
```

Key rules that apply to every adapter:

- **Enable only the families you want.** A disabled or omitted family
  contributes nothing.
- **Thresholds are strings.** Even numeric knobs (`restartWarnCount: "3"`,
  `warnDays: "30"`) are quoted strings — that is the threshold map's type.
- **The overall result is worst-case** across the enabled families, using the
  severity ordering `Pass < Skipped < Warn < Unknown < Fail < Error`.
- **Set the name thresholds for renamed installs.** Distributions like RKE2 or
  k3s rename the standard workloads; the thresholds let you point the check at
  the real object names.

## Run cadence and the `interval` caveat

In the current build, `spec.interval` is **accepted but not yet honored** for
`AddonCheck`. The operator runs an adapter when:

1. the check is first created (`status.lastRunTime` is empty), or
2. the check's spec changes (its generation increments).

There is no periodic timer yet. In practice this means:

- To re-run a check on demand, edit its spec (any generation-changing edit
  re-runs the adapter). A no-op annotation bump works.
- Until periodic requeue lands, treat `interval` as advisory and drive
  re-runs from your GitOps/apply cadence if you need them.

This is a known limitation tracked in the
[architecture reference](../architecture.md#known-limitations).
`NodeCertificateCheck` — a newer kind not present in older builds (see
its [guide](node-certificate-checks.md#availability)) — is the exception that
*does* honor `interval`.

## Adapter catalog

| `addonType` | Families | What it validates |
| --- | --- | --- |
| [`cert-manager`](#cert-manager) | `system_health`, `issuer_health`, `certificate_health` | cert-manager workloads, issuers, and certificate expiry/issuance |
| [`coredns`](#coredns) | `system_health`, `dns_resolution` | CoreDNS workloads + real DNS resolution from a workload's vantage point |
| [`external-secrets`](#external-secrets) | `system_health`, `secret_sync` | External Secrets Operator workloads + ExternalSecret sync state |
| [`cilium`](#cilium) | `control_plane_health`, `agent_health`, `crd_health` | Cilium operator, per-node agent, and CRDs |

### cert-manager

```yaml
spec:
  addonType: cert-manager
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

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | Core cert-manager Deployments and their pods, required cert-manager CRDs, and (optionally) the webhook Service plus the admission webhook configuration. | `controllerName`, `webhookName`, `cainjectorName`, `webhookServiceName`, `webhookConfigName`, `restartWarnCount`, `webhookProbe` (`"true"` to also probe the webhook) |
| `issuer_health` | `Issuer` and `ClusterIssuer` readiness. | `kinds` (comma-separated, e.g. `"Issuer,ClusterIssuer"`) |
| `certificate_health` | `Certificate` readiness, renewal timing, expiry, issuer references, and secret linkage. | `warnDays` (Warn window), `failDays` (Fail window) |

### CoreDNS

```yaml
spec:
  addonType: coredns
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        deploymentName: "coredns"
        serviceName: "kube-dns"
        restartWarnCount: "3"
        autoscalerName: ""        # set if you run a CoreDNS autoscaler
    dns_resolution:
      enabled: true
      thresholds:
        targets: "kubernetes.default.svc.cluster.local"
        probeImage: "ghcr.io/skaphos/fathom-probe:v0.0.2"
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | CoreDNS Deployment and pods, the `kube-dns` Service and its EndpointSlices, and (optionally) a node-count autoscaler Deployment. | `deploymentName`, `serviceName`, `autoscalerName` (empty disables the autoscaler check), `restartWarnCount` |
| `dns_resolution` | Launches a short-lived **probe pod** per target *in the AddonCheck's namespace* and records each target's outcome plus resolver latency — so DNS is resolved with workload topology, not the operator's. | `targets` (comma-separated names), `probeImage` (per-check override) |

`dns_resolution` is the one add-on family that runs out-of-process. See
[Probe image](#probe-image) for how the image is resolved, and
[the probe-pod model](../architecture.md#probe-pod-model) for the why.

### External Secrets

```yaml
spec:
  addonType: external-secrets
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

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | External Secrets Operator controller Deployments and pods, plus the required ESO CRDs. | `restartWarnCount` |
| `secret_sync` | `ExternalSecret` readiness, stale-refresh state, failure reasons, store references, and target-secret linkage. | `staleMinutes` (how long since last successful refresh before a synced secret is considered stale) |

### Cilium

```yaml
spec:
  addonType: cilium
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

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `control_plane_health` | The `cilium-operator` Deployment and its pods. | `operatorDeploymentName`, `restartWarnCount` |
| `agent_health` | The `cilium` agent DaemonSet and its pods. | `agentDaemonSetName`, `restartWarnCount` |
| `crd_health` | The core Cilium CRDs are established and serve a supported version (`v2` / `v2alpha1`). | — |

**Cilium treats "not installed" differently.** The cert-manager and External
Secrets adapters `Fail` when a required component is missing. The Cilium adapter
reports `Skipped` (which rolls up green) when Cilium is absent entirely — the
operator Deployment, agent DaemonSet, and CRDs are all missing — so a `cilium`
`AddonCheck` stays quiet on clusters that may or may not run Cilium. A workload
that *exists but is unhealthy* still reports `Fail`. This makes a single Cilium
check safe to ship to a mixed fleet.

Either way, **`status.absent`** records how many checks in the most recent run found
their target not installed — the required-absent Fails and the optional-absent Skips
alike. It makes "not installed" queryable and distinct from "unhealthy" (a `Fail`
whose target exists) and "disabled" (a `Skipped` family), so a dashboard can tell an
absent Cilium apart from a broken one without parsing per-check details (SKA-526).

## Probe image

The CoreDNS `dns_resolution` family launches probe pods. The image is resolved
per check with this precedence (highest wins):

```
per-AddonCheck probeImage threshold  >  operator --probe-image  >  adapter fallback
```

So you can:

- set `policy.dns_resolution.thresholds.probeImage` on a single check, or
- set the operator-wide default once with `--probe-image` (Helm:
  `--set probeImage.tag=...`), which is what most teams running a private GHCR
  mirror do.

See [Configuration → Probe image default](../reference/configuration.md#probe-image-default).

## Rolling checks up into ClusterHealth

`AddonCheck.status` is the per-check truth. To get one verdict across many
checks, wrap each in a `HealthCheck` and let a `ClusterHealth` aggregate them:

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: cert-manager
  namespace: fathom-system
spec:
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
spec: {}   # empty selector matches all HealthChecks in this namespace
```

- One `HealthCheck` per `AddonCheck` you want in the roll-up.
- A `ClusterHealth` with an empty selector folds in **every** `HealthCheck` in
  its namespace; use `spec.selector` (label selector) to scope it.
- Aggregation is **same-namespace** in this build. Keep the checks you want
  rolled up together in one namespace.

> `HealthCheck` can only wrap `AddonCheck` (`checkRef.kind: AddonCheck`). The
> newer `NodeCertificateCheck` kind, where present, reports its own
> status directly and is not aggregated through `HealthCheck`/`ClusterHealth`.

## Troubleshooting

Read the conditions: `kubectl describe addoncheck <name>` shows `Accepted`,
`Ready`, and (when paused) `Paused`. `Accepted=False` means the spec/policy was
rejected and no adapter run happens until it is corrected — the exact problems
are in the `Accepted` condition message. Common `Ready=False` reasons:

| Condition reason | Meaning | Fix |
| --- | --- | --- |
| `MissingAdapter` | `spec.addonType` doesn't match a built-in adapter. | Check the spelling; valid values are `cert-manager`, `coredns`, `external-secrets`, `cilium`. |
| `AdapterLookupFailed` | The registry could not resolve the adapter. | Inspect operator logs; usually a startup/registration issue. |
| `Paused` | `spec.paused` is set. | The last status snapshot is preserved; unset `paused` to resume. |
| `InvalidPolicy` | A `spec.policy` key names a family the adapter doesn't advertise, or a family carries an invalid `labelSelector`. Also sets `Accepted=False`. | Use a family the adapter exposes and a valid selector; the `Accepted` message lists each problem. Editing the spec re-runs the check. |

On the wrapper side, `kubectl describe healthcheck <name>`:

| Condition reason | Meaning |
| --- | --- |
| `TargetNotFound` | `checkRef` points at an `AddonCheck` that doesn't exist (same namespace). |
| `UnsupportedKind` | `checkRef.kind` is not `AddonCheck`. |
| `Paused` | `spec.paused` is set; mirroring is suspended. |

If a check is stuck on a stale result, remember the
[run-cadence caveat](#run-cadence-and-the-interval-caveat): edit the spec to
force a fresh run.

## Reference

- [Configuration reference](../reference/configuration.md) — operator flags and
  env vars.
- [API reference](../reference/api.md) — generated, field-level CRD reference.
- [Architecture](../architecture.md) — adapter contract and reconciler detail.
