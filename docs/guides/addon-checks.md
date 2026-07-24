<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Add-on Checks

An `AddonCheck` validates one platform add-on. It picks a built-in **adapter**
with `spec.addonType` and enables one or more **check families** under
`spec.policy`. This guide is the working reference for the eight built-in
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
  addonType: <cert-manager | coredns | node-local-dns | external-secrets | cilium | external-dns | metrics-server | envoy-gateway | istio | keda | vpa | descheduler | kured | argocd>
  interval: 5m          # periodic adapter run cadence; defaults to 5m
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

## Run cadence

The operator runs an adapter when:

1. the check is first created (`status.lastRunTime` is empty),
2. the check's spec changes (its generation increments),
3. `spec.interval` has elapsed since the last run, or
4. the `fathom.skaphos.io/run-now` annotation changes to a fresh non-empty
   value.

`spec.interval` defaults to `5m` when omitted. A ready check is requeued after
the interval so results keep tracking live cluster state. To force an out-of-band
run, write a new run-now annotation value each time, such as an RFC3339
timestamp or nonce:

```sh
kubectl -n fathom-system annotate addoncheck cert-manager \
  fathom.skaphos.io/run-now="$(date -Iseconds)" --overwrite
```

Periodic runs update `status.lastRunTime`. `HealthReport` history is still a
transition log: the controller creates a new report on the first run or when the
aggregate result changes, and reuses status refreshes for same-result periodic
runs.

## Adapter catalog

| `addonType` | Families | What it validates |
| --- | --- | --- |
| [`cert-manager`](#cert-manager) | `system_health`, `issuer_health`, `certificate_health` | cert-manager workloads, issuers, and certificate expiry/issuance |
| [`coredns`](#coredns) | `system_health`, `dns_resolution` | CoreDNS workloads + real DNS resolution from a workload's vantage point |
| [`node-local-dns`](#nodelocal-dnscache-node-local-dns) | `system_health`, `dns_resolution` | The NodeLocal DNSCache DaemonSet on every schedulable node (per-node gap detection) + resolution through the node-local cache itself |
| [`external-secrets`](#external-secrets) | `system_health`, `secret_sync` | External Secrets Operator workloads + ExternalSecret sync state |
| [`cilium`](#cilium) | `control_plane_health`, `agent_health`, `crd_health` | Cilium operator, per-node agent, and CRDs |
| [`external-dns`](#external-dns) | `system_health`, `crd_health` | external-dns controller workloads + the opt-in DNSEndpoint CRD |
| [`metrics-server`](#metrics-server) | `system_health`, `api_availability` | metrics-server workloads + the aggregated resource-metrics APIService |
| [`envoy-gateway`](#envoy-gateway) | `system_health`, `crd_health`, `gateway_status` | Envoy Gateway controller, Gateway API CRDs, and Gateway conditions |
| [`istio`](#istio) | `system_health`, `ztunnel_health`, `istio_cni_health`, `crd_health` | istiod + its admission webhooks, the ambient data plane, and core mesh CRDs |
| [`keda`](#keda) | `system_health`, `crd_health`, `scaling_health` | KEDA operator/metrics-apiserver/webhook workloads, CRDs, and ScaledObject Ready/Paused state |
| [`vpa`](#vpa) | `system_health`, `crd_health`, `recommendation_health` | VPA recommender/updater/admission workloads, CRDs, and whether VPAs are producing recommendations |
| [`descheduler`](#descheduler) | `system_health`, `policy_validity`, `last_run` | descheduler Deployment or CronJob, DeschedulerPolicy well-formedness, and last-run recency |
| [`kured`](#kured) | `system_health`, `reboot_state` | kured DaemonSet, and whether the reboot lock is wedged or a node has waited too long for a reboot |
| [`argocd`](#argocd) | `system_health`, `sync_health` | Argo CD control-plane workloads and CRDs, and every Application's sync/health state |

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
        probeImage: "ghcr.io/skaphos/fathom-probe:v0.4.0"
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | CoreDNS Deployment and pods, the `kube-dns` Service and its EndpointSlices, and (optionally) a node-count autoscaler Deployment. | `deploymentName`, `serviceName`, `autoscalerName` (empty disables the autoscaler check), `restartWarnCount` |
| `dns_resolution` | Launches a short-lived **probe pod** per target *in the AddonCheck's namespace* and records each target's outcome plus resolver latency — so DNS is resolved with workload topology, not the operator's. | `targets` (comma-separated names), `probeImage` (per-check override) |

`dns_resolution` is the one add-on family that runs out-of-process. See
[Probe image](#probe-image) for how the image is resolved, and
[the probe-pod model](../architecture.md#probe-pod-model) for the why.

### NodeLocal DNSCache (node-local-dns)

```yaml
spec:
  addonType: node-local-dns
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        daemonSetName: "node-local-dns"
        restartWarnCount: "3"
    dns_resolution:
      enabled: true
      thresholds:
        targets: "kubernetes.default.svc.cluster.local"
        listenAddress: "169.254.20.10"
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The node-local-dns DaemonSet and its pods, plus **per-node gap detection**: every schedulable (uncordoned) node must host a ready cache pod, and the check names the nodes that lack one (`missingNodes`, bounded to 10 names) instead of reporting a bare count mismatch. | `daemonSetName`, `restartWarnCount` |
| `dns_resolution` | Launches a short-lived **probe pod** per target in the AddonCheck's namespace with its resolver pinned to the cache's listen address (`dnsPolicy: None`), so the query is answered by the node-local cache — not by whatever the cluster's default resolver path happens to be. | `targets` (comma-separated **fully qualified** names — no cluster search domains apply under the pinned resolver), `listenAddress` (defaults to the upstream convention `169.254.20.10`), `probeImage`, `probeNamespace` |

This closes the blind spot cluster-level CoreDNS checks cannot see: once
kubelet points pod `resolv.conf` at the node-local cache, a node whose cache
pod is down loses DNS for every workload on that node even while CoreDNS
itself is fully healthy. For the same reason a missing DaemonSet scores
`Fail` (with the absent marker), not `Skipped`. See
[Probe image](#probe-image) for how the probe image is resolved.

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

### external-dns

```yaml
spec:
  addonType: external-dns
  policy:
    system_health:
      enabled: true
      namespaces:
        - external-dns
      thresholds:
        deploymentName: "external-dns"
        restartWarnCount: "3"
    crd_health:
      enabled: true
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The external-dns controller Deployment and its pods. | `deploymentName` (the Deployment follows the Helm release fullname), `restartWarnCount` |
| `crd_health` | The `dnsendpoints.externaldns.k8s.io` CRD is established and serves `v1alpha1`. | — |

The controller Deployment is required (absent → `Fail`), but the DNSEndpoint
CRD is **optional**: the CRD source is an opt-in external-dns feature, so an
install without it reports `Skipped` with the `absent` detail instead of
failing. DNS record reconciliation outcomes are not checked — `DNSEndpoint`
status carries no conditions, so per-record health is only observable through
external-dns's own metrics and logs.

### metrics-server

```yaml
spec:
  addonType: metrics-server
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        deploymentName: "metrics-server"
        restartWarnCount: "3"
    api_availability:
      enabled: true
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The metrics-server Deployment and its pods. | `deploymentName` (the Deployment follows the Helm release fullname), `restartWarnCount` |
| `api_availability` | The aggregated `v1beta1.metrics.k8s.io` APIService exists and reports `Available=True`. | — |

`api_availability` is the check that matters: a TLS-misconfigured or
unreachable metrics-server keeps its pods `Ready` while the APIService goes
`Unavailable` — taking `kubectl top` and HPA scaling with it. Both the
Deployment and the APIService are required; a missing APIService object is a
`Fail` with the `absent` detail.

### Envoy Gateway

```yaml
spec:
  addonType: envoy-gateway
  policy:
    system_health:
      enabled: true
      namespaces:
        - envoy-gateway-system
      thresholds:
        deploymentName: "envoy-gateway"
        restartWarnCount: "3"
    crd_health:
      enabled: true
    gateway_status:
      enabled: true
      namespaces:
        - edge
        - apps
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The `envoy-gateway` controller Deployment and its pods. | `deploymentName`, `restartWarnCount` |
| `crd_health` | The Gateway API core CRDs (`GatewayClass`, `Gateway`, `HTTPRoute`; `v1`/`v1beta1`) and Envoy Gateway's `EnvoyProxy` CRD (`v1alpha1`) are established. | — |
| `gateway_status` | Every `Gateway` in the policy namespaces reports `Accepted=True` and `Programmed=True`. | — |

Set `gateway_status.namespaces` to wherever your `Gateway` objects live — they
are usually declared outside `envoy-gateway-system` (the default when no
namespaces are given). A cluster with no Gateways yet reports `Skipped`.
Not covered (yet): the dynamically-named per-Gateway proxy Deployments
(`envoy-<ns>-<gateway>-<hash>`) — observe them indirectly through `Programmed`
— and `HTTPRoute` conditions, which live per-parent under `status.parents`
rather than in `status.conditions`.

### istio

```yaml
spec:
  addonType: istio
  policy:
    system_health:
      enabled: true
      namespaces:
        - istio-system
      thresholds:
        restartWarnCount: "3"
        # A revisioned control plane renames the Deployment (istiod-<rev>):
        # deploymentName: "istiod-1-30"
    ztunnel_health:
      enabled: true
      namespaces:
        - istio-system
    istio_cni_health:
      enabled: true
      namespaces:
        - istio-system
    crd_health:
      enabled: true
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The `istiod` Deployment and its pods, plus istiod's admission wiring: the `istio-sidecar-injector` `MutatingWebhookConfiguration` and the `istio-validator-istio-system` `ValidatingWebhookConfiguration` must exist, carry a populated `caBundle`, and point at the `istiod` Service in the family's namespace. | `deploymentName`, `injectorWebhookName`, `validatorWebhookName`, `restartWarnCount` |
| `ztunnel_health` | The `ztunnel` DaemonSet (ambient L4 node proxy) and its pods. **Optional**: a sidecar-mode mesh reports `Skipped` with the absent marker. | `daemonSetName`, `restartWarnCount` |
| `istio_cni_health` | The `istio-cni-node` DaemonSet and its pods. **Optional**: required for ambient, opt-in for sidecar mode. | `daemonSetName`, `restartWarnCount` |
| `crd_health` | The core `networking.istio.io` (`v1`/`v1beta1`/`v1alpha3`) and `security.istio.io` (`v1`/`v1beta1`) CRDs are established. | — |

The webhook checks are the ones that distinguish "istiod pods Ready" from
"injection and validation actually admit": an unpopulated `caBundle` means
istiod has not patched (or cannot patch) the bundle, so sidecar injection
silently stops. Names assume the default revision in `istio-system`; a
revisioned or relocated control plane is reachable entirely through policy —
`namespaces` redirects the workload and the expected backing service, and
`deploymentName` / `injectorWebhookName` / `validatorWebhookName` override
the renamed objects (`istiod-<rev>`, `istio-sidecar-injector-<rev>`,
`istio-validator-<rev>-<ns>`). Version detection follows the same overrides:
it resolves through the `system_health` workload check rather than a fixed
address, so a revisioned or relocated control plane still reports its
detected version. Not covered (yet):
`mesh_status` — proxy-sync / config-distribution anomalies are observable
only through istiod's XDS and metrics endpoints, not the Kubernetes API,
and `PeerAuthentication` carries no status conditions to score.

Either way, **`status.absent`** records how many checks in the most recent run found
their target not installed — the required-absent Fails and the optional-absent Skips
alike. It makes "not installed" queryable and distinct from "unhealthy" (a `Fail`
whose target exists) and "disabled" (a `Skipped` family), so a dashboard can tell an
absent Cilium apart from a broken one without parsing per-check details (SKA-526).

### keda

```yaml
spec:
  addonType: keda
  policy:
    system_health:
      enabled: true
      namespaces:
        - keda
      thresholds:
        webhookConfigurationName: "keda-admission"
        restartWarnCount: "3"
    crd_health:
      enabled: true
    scaling_health:
      enabled: true
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The `keda-operator`, `keda-operator-metrics-apiserver`, and `keda-admission-webhooks` Deployments and their pods, plus the `keda-admission` ValidatingWebhookConfiguration is present and its `caBundle` is populated. | `restartWarnCount`, `webhookConfigurationName` |
| `crd_health` | The `ScaledObject`, `ScaledJob`, `TriggerAuthentication`, and `ClusterTriggerAuthentication` CRDs are Established and serve a supported version. | — |
| `scaling_health` | Every `ScaledObject` (cluster-wide) reports `Ready=True`; a `Ready=False` object is a `Fail` (it is not autoscaling its workload), and a `Paused=True` object is surfaced as a `Warn`. | — |

KEDA is **Optional**: on a cluster that does not run KEDA every target is
`Skipped` with the `absent` detail rather than a `Fail`, and an empty cluster
yields a `Skipped` `scaling_health`.

### vpa

```yaml
spec:
  addonType: vpa
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        restartWarnCount: "3"
    crd_health:
      enabled: true
    recommendation_health:
      enabled: true
      thresholds:
        webhookConfigurationName: "vpa-webhook-config"
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The `vpa-recommender`, `vpa-updater`, and `vpa-admission-controller` Deployments and their pods. | `restartWarnCount` |
| `crd_health` | The `VerticalPodAutoscaler` and `VerticalPodAutoscalerCheckpoint` CRDs are Established and serve a supported version. | — |
| `recommendation_health` | Every `VerticalPodAutoscaler` (cluster-wide) reports `RecommendationProvided=True`, and the `vpa-webhook-config` MutatingWebhookConfiguration is present and wired. A VPA not yet producing a recommendation is a `Warn`, not a `Fail`. | `webhookConfigurationName` |

The recommender is blind without metrics-server; pair this adapter with a
`metrics-server` AddonCheck. VPA is **Optional**: absent targets are `Skipped`
with the `absent` detail.

### descheduler

```yaml
spec:
  addonType: descheduler
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        deploymentName: "descheduler"
        cronJobName: "descheduler"
        restartWarnCount: "3"
    policy_validity:
      enabled: true
      thresholds:
        configMapName: "descheduler"
    last_run:
      enabled: true
      thresholds:
        successMaxAge: "24h"
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The descheduler Deployment (loop mode) or CronJob (scheduled mode) is healthy. A real install is one mode or the other, so the mode not in use is `Skipped`. | `deploymentName`, `cronJobName`, `restartWarnCount` |
| `policy_validity` | The DeschedulerPolicy ConfigMap holds a `policy.yaml` that parses as YAML and declares a recognized descheduler apiVersion. Catches the silent no-op where an unparseable policy means nothing is ever descheduled. | `configMapName` |
| `last_run` | The CronJob's last scheduled run completed successfully and is recent. `Skipped` on a Deployment-mode install. | `cronJobName`, `successMaxAge` (a Go duration) |

The policy check is shape-level (valid YAML + recognized apiVersion); it does
not validate individual strategy/plugin names against the running descheduler
release. descheduler is **Optional**: absent targets are `Skipped` with the
`absent` detail.

### kured

```yaml
spec:
  addonType: kured
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        daemonSetName: "kured"
        restartWarnCount: "3"
    reboot_state:
      enabled: true
      thresholds:
        lockMaxAge: "1h"
        rebootPendingMaxAge: "24h"
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The `kured` DaemonSet and its pods are Ready. | `daemonSetName`, `restartWarnCount` |
| `reboot_state` | The reboot lock (the `weave.works/kured-node-lock` annotation on the DaemonSet) has not been held past `lockMaxAge` — a wedged lock means reboots have stopped progressing. With `--annotate-nodes` enabled, a node carrying `weave.works/kured-most-recent-reboot-needed` past `rebootPendingMaxAge` has waited too long. | `lockMaxAge`, `rebootPendingMaxAge` (Go durations) |

An idle cluster with no lock held and no node awaiting a reboot yields a quiet
`reboot_state` (the lock check passes; the node scan is `Skipped`). Node-level
reboot detection requires kured's `--annotate-nodes`; without it the node scan
is `Skipped` — the honest signal that the data is not published, not a false
all-clear. kured is **Optional**: absent targets are `Skipped` with the
`absent` detail.

### argocd

```yaml
spec:
  addonType: argocd
  policy:
    system_health:
      enabled: true
      namespaces:
        - argocd
      thresholds:
        restartWarnCount: "3"
    sync_health:
      enabled: true
```

| Family | Checks | Key thresholds |
| --- | --- | --- |
| `system_health` | The `argocd-application-controller` StatefulSet plus the `argocd-repo-server`, `argocd-server`, and `argocd-redis` Deployments and their pods, and the `Application`, `ApplicationSet`, and `AppProject` CRDs are Established and serve `v1alpha1`. | `applicationControllerName`, `repoServerName`, `serverName`, `redisName`, `restartWarnCount` |
| `sync_health` | Every `Application` (all namespaces unless `policy.sync_health.namespaces` narrows the scan) reports `status.sync.status: Synced` and `status.health.status: Healthy`. `Degraded` and `Missing` are `Fail`s; `OutOfSync`, `Progressing`, `Suspended`, and `Unknown` are surfaced as `Warn`s. | — |

The adapter is **strictly read-only**: it lists Applications through the
Kubernetes API and never annotates, syncs, or refreshes anything, so it can
never trigger reconciliation work. A cluster with Argo CD installed but no
Applications yields a quiet `Skipped` `sync_health`. Names assume a default
install in the `argocd` namespace; a renamed install is reachable through
`policy.namespaces` and the per-component name thresholds. Not covered: the
HA variant's `redis-ha` topology (point `redisName` at
`argocd-redis-ha-haproxy` to cover its proxy Deployment) and the optional
dex/applicationset/notifications controllers.

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
spec: {}   # empty spec matches all HealthChecks in all namespaces
```

- One `HealthCheck` per `AddonCheck` you want in the roll-up.
- `ClusterHealth` is cluster-scoped. An empty spec folds in **every**
  `HealthCheck` in the cluster; use `spec.selector` (label selector) and/or
  the namespace filter to scope it:
  - `spec.namespaces` — allowlist (definitive when set)
  - else `spec.excludedNamespaces` — denylist
  - else open (all namespaces)
- Any `HealthCheck` the scope covers contributes to the aggregate, and a
  wrapper may explicitly set `checkRef.namespace` to mirror an `AddonCheck` in
  another namespace — so use RBAC on `HealthCheck` creation plus the aggregate
  namespace filter to control which teams can surface status into a
  cluster-wide aggregate.

> `HealthCheck` can only wrap `AddonCheck` (`checkRef.kind: AddonCheck`). The
> newer `NodeCertificateCheck` kind, where present, reports its own
> status directly and is not aggregated through `HealthCheck`/`ClusterHealth`.

> **Upgrading from a release where `ClusterHealth` was namespaced:** Kubernetes
> cannot change a CRD's scope in place. Delete the old CRD first — this removes
> existing `ClusterHealth` objects — then install the new manifests and
> recreate your aggregates as cluster-scoped objects:
> `kubectl delete crd clusterhealths.fathom.skaphos.io`.

## Troubleshooting

Read the conditions: `kubectl describe addoncheck <name>` shows `Accepted`,
`Ready`, and (when paused) `Paused`. `Accepted=False` means the spec/policy was
rejected and no adapter run happens until it is corrected — the exact problems
are in the `Accepted` condition message. The full status contract is in
[Status and conditions](../reference/status-conditions.md). Common
`Ready=False` reasons:

| Condition reason | Meaning | Fix |
| --- | --- | --- |
| `MissingAdapter` | `spec.addonType` doesn't match a built-in adapter. | Check the spelling; valid values are `cert-manager`, `coredns`, `node-local-dns`, `external-secrets`, `cilium`, `external-dns`, `metrics-server`, `envoy-gateway`, `istio`, `keda`, `vpa`, `descheduler`, `kured`. |
| `AdapterLookupFailed` | The registry could not resolve the adapter. | Inspect operator logs; usually a startup/registration issue. |
| `Paused` | `spec.paused` is set. | The last status snapshot is preserved; unset `paused` to resume. |
| `InvalidPolicy` | A `spec.policy` key names a family the adapter doesn't advertise, or a family carries an invalid `labelSelector`. Also sets `Accepted=False`. | Use a family the adapter exposes and a valid selector; the `Accepted` message lists each problem. Editing the spec re-runs the check. |

On the wrapper side, `kubectl describe healthcheck <name>`:

| Condition reason | Meaning |
| --- | --- |
| `TargetNotFound` | `checkRef` points at an `AddonCheck` that doesn't exist in the wrapper namespace, or in the explicit `checkRef.namespace` when one is set. |
| `UnsupportedKind` | `checkRef.kind` is not `AddonCheck`. |
| `Paused` | `spec.paused` is set; mirroring is suspended. |

If a check is stuck on a stale result, compare `status.lastRunTime` with
`spec.interval` and the controller logs. To force an immediate run, set a fresh
`fathom.skaphos.io/run-now` annotation value.

## Reference

- [Configuration reference](../reference/configuration.md) — operator flags and
  env vars.
- [API reference](../reference/api.md) — generated, field-level CRD reference.
- [Architecture](../architecture.md) — adapter contract and reconciler detail.
