<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Fathom Architecture

This document describes how Fathom is put together: the custom resources it
reconciles, how their statuses flow into one another, what each controller
owns, and the two extension surfaces (the in-process adapter contract and the
probe-pod model). It is the design reference the repository previously pointed
at as `DESIGN.md`.

Architecturally significant decisions are recorded as ADRs and linked from the
relevant sections rather than restated here:

- [ADR-0001 — In-process adapter contract](adr/0001-in-process-adapter-contract.md)
- [ADR-0002 — HealthReport as a first-class CRD](adr/0002-healthreport-as-first-class-crd.md)
- [ADR-0003 — Probe-pod model](adr/0003-probe-pod-model.md)
- [ADR-0004 — HealthCheck as a thin wrapper](adr/0004-healthcheck-as-wrapper.md)

## What Fathom Is

Fathom is a Kubernetes operator (API group `fathom.skaphos.io`) that validates
the health of platform add-ons — cert-manager, CoreDNS, External Secrets
Operator, and others reachable through an adapter. It reconciles five custom
resources, runs adapter-defined checks against the cluster, persists the
results as history, and rolls those results up into a single cluster-wide
verdict that dashboards, alerting, and deployment gates can consume.

The operator binary is built from `cmd/main.go`, which constructs a cobra root
command in [`internal/app`](../internal/app) and runs it. A second, much
smaller binary (`cmd/probe`) ships as the probe image and is launched as
short-lived pods (see [Probe-pod model](#probe-pod-model)).

## What Fathom Is Not

- It is **not** a metrics/time-series system. Long-term trend storage is out of
  scope; `HealthReport` history is bounded per check (see
  [ADR-0002](adr/0002-healthreport-as-first-class-crd.md)).
- `ClusterHealth` is **not** derived from `HealthReport` history. It is derived
  *only* from `HealthCheck.status` (the `AGENTS.md` invariant; see
  [Aggregation chain](#the-aggregation--status-mirror-chain)).
- Adapters are **not** out-of-process plugins, sidecars, or DaemonSets. They are
  compiled into the operator (see [ADR-0001](adr/0001-in-process-adapter-contract.md)).

## The CRD Model

All kinds live in `api/v1alpha1` and the group `fathom.skaphos.io`. The
generated, field-level reference is in [reference/api.md](reference/api.md); this
section is the conceptual overview.

| Kind | File | Role | Executes checks? |
| --- | --- | --- | --- |
| `AddonCheck` | `api/v1alpha1/addoncheck_types.go` | Declares a check against one add-on; selects an adapter via `spec.addonType`. | Yes (via its adapter) |
| `NodeCertificateCheck` | `api/v1alpha1/nodecertificatecheck_types.go` | Declares an on-disk certificate-expiry scan; the operator runs it via a node-agent DaemonSet. | Yes (via the node-agent) |
| `HealthCheck` | `api/v1alpha1/healthcheck_types.go` | Thin wrapper that mirrors a specialized check's status into a uniform shape. | No |
| `ClusterHealth` | `api/v1alpha1/clusterhealth_types.go` | Aggregates selected `HealthCheck` statuses into one worst-case result. | No |
| `HealthReport` | `api/v1alpha1/healthreport_types.go` | Immutable, first-class history record of one check run. | n/a |

`AddonCheck` and `NodeCertificateCheck` are the kinds that drive work
(`AddonCheck` in-process via an adapter; `NodeCertificateCheck` on each node via
the node-agent DaemonSet). `HealthCheck` and
`ClusterHealth` are projection/aggregation layers; `HealthReport` is the audit
trail.

### Result severity

Every result uses the `HealthReportResult` enum (`Pass`, `Warn`, `Fail`,
`Error`, `Skipped`, `Unknown`). Worst-case aggregation uses the ordering encoded
in `HealthReportResult.Severity()` (`api/v1alpha1/healthreport_types.go`):

```
Pass(1) < Skipped(2) < Warn(3) < Unknown(4) < Fail(5) < Error(6)
```

`Unknown` sits above `Warn` but below `Fail`: a known `Fail` is more actionable
than an `Unknown`, so when both appear in a set the aggregate reports `Fail`.
Empty/unrecognized values return severity `0`, meaning "exclude from worst-case
aggregation" (e.g. a `HealthCheck` not yet reconciled).

## The Aggregation / Status-Mirror Chain

Status flows in one direction:

```
        adapter run                 mirror                   aggregate
AddonCheck.status  ───────▶  HealthCheck.status  ───────▶  ClusterHealth.status
   (LastResult)              (Result, Summary,             (worst-case Result
                              SourceObservedAt)             over children)
        │
        └──creates──▶ HealthReport (immutable history; NOT read by the chain)
```

ASCII view of the reconcile flow:

```
                         +-----------------------------+
  user applies           |     AddonCheckReconciler    |
  AddonCheck ───────────▶ | - resolve adapter (registry)|
                         | - adapter.Run(ctx, Request) |
                         | - create HealthReport        |───▶ HealthReport (history,
                         | - prune to spec.historyLimit |      owner-ref'd to AddonCheck)
                         | - write AddonCheck.status     |
                         +--------------+--------------+
                                        │ status change
                          watch re-enqueues wrappers
                                        ▼
                         +-----------------------------+
                         |    HealthCheckReconciler    |
                         | - Get referenced AddonCheck |
                         | - mirror LastResult/etc into|
                         |   HealthCheck.status         |
                         +--------------+--------------+
                                        │ status change
                          watch re-enqueues aggregates
                                        ▼
                         +-----------------------------+
                         |   ClusterHealthReconciler   |
                         | - List HealthChecks by      |
                         |   spec.selector (same ns)   |
                         | - worst-case Result + summary|
                         |   of children                |
                         +-----------------------------+
```

The key invariant: `ClusterHealthReconciler` deliberately never imports or reads
`HealthReport`. Its only input is `HealthCheck.Status`, which the
`HealthCheckReconciler` maintains. This keeps `ClusterHealth`'s external contract
stable regardless of how history is stored (see
[ADR-0004](adr/0004-healthcheck-as-wrapper.md) and the comment block on
`ClusterHealthReconciler` in `internal/controller/clusterhealth_controller.go`).

## Reconcilers

All reconcilers live in `internal/controller` and are registered with the
manager in `internal/app/run.go` (`DefaultControllers`). Each writes only its
own resource's `status` subresource and uses a `DeepEqual` no-op guard so a
reconcile with no status change issues no API write. The operational
condition/reason tables are in
[reference/status-conditions.md](reference/status-conditions.md); this section
focuses on controller ownership and data flow.

### AddonCheckReconciler

`internal/controller/addoncheck_controller.go`

- **Owns / produces:** `AddonCheck.status`; creates `HealthReport` objects
  (owner-referenced to the `AddonCheck` via `controllerutil.SetControllerReference`).
- **Watches:** `AddonCheck` only (`For(&AddonCheck{})`).
- **Adapter dispatch:** resolves the adapter for `spec.addonType` from the
  in-process registry. If paused, missing, or lookup fails, it records a
  `Ready=False` condition (`Paused` / `MissingAdapter` / `AdapterLookupFailed`)
  and does not run.
- **Scoped client (least privilege):** the adapter's `Request.Client`
  impersonates the addon's own ServiceAccount (`fathom-addon-<addonType>`,
  resolved by the `fathom.skaphos.io/addon` label), so an adapter reads exactly
  and only what it declares — the operator ServiceAccount holds no addon reads,
  just `impersonate` on those ServiceAccounts. The client is direct/uncached
  because impersonation is an API-server feature the manager cache does not
  honor. A missing addon ServiceAccount surfaces as an adapter-level `Error`
  rather than a fall-back to broader access. See
  [Adapter RBAC](reference/rbac.md).
- **Run trigger:** an adapter run happens when the adapter is ready **and** the
  check is first observed (`status.lastRunTime == nil`), the observed generation
  changed, `spec.interval` has elapsed, or the `fathom.skaphos.io/run-now`
  annotation carries a fresh non-empty value.
- **Timeout:** bounded by `spec.timeout` if set and positive, else a built-in
  default of `30s` (`defaultAddonCheckTimeout`). The run executes under a
  `context.WithTimeout`.
- **History retention:** after creating a `HealthReport`, it prunes the oldest
  reports for this `AddonCheck` down to `spec.historyLimit` (default `10`,
  minimum `1`). Reports are located by the labels
  `fathom.skaphos.io/source-kind=AddonCheck` and
  `fathom.skaphos.io/source-name=<name>`. Prune failures are logged, not
  returned — the user-facing write already succeeded and the next reconcile
  retries.
- **Status written:** `observedGeneration`, the `Accepted` / `Paused` / `Ready`
  conditions, `lastRunTime`, `lastResult`, `lastReportName`.

The aggregate `HealthReport.spec.result` is the worst-case over the adapter's
per-check outcomes (`aggregateHealthReportResult`); a run that errors at the
adapter level is forced to `Error`.

### HealthCheckReconciler

`internal/controller/healthcheck_controller.go`

- **Owns / produces:** `HealthCheck.status` only. It creates nothing.
- **Watches:** `HealthCheck` (`For`) **and** `AddonCheck` (`Watches` with a
  map function `healthChecksForAddonCheck`), so a target's status change
  re-enqueues every `HealthCheck` that wraps it. A
  `ResourceVersionChangedPredicate` filters no-op events.
- **Behavior:** mirrors the referenced check's status into the uniform
  `HealthCheck.status` shape (`result`, `summary`, `sourceObservedAt`,
  `lastReportName`). The only supported `checkRef.kind` in this build is
  `AddonCheck`; any other kind yields `Ready=False / UnsupportedKind`. A missing
  target yields `Ready=False / TargetNotFound`. `checkRef.namespace` may point
  at an `AddonCheck` in another namespace; when it is omitted, the
  `HealthCheck` namespace is used.
- **Paused:** when `spec.paused`, mirroring is suspended and the last snapshot
  is preserved (`Ready=False / Paused`).

### ClusterHealthReconciler

`internal/controller/clusterhealth_controller.go`

- **Owns / produces:** `ClusterHealth.status` only.
- **Watches:** `ClusterHealth` (`For`) **and** `HealthCheck` (`Watches` with
  `clusterHealthsForHealthCheck`), so a member's status change re-enqueues every
  `ClusterHealth` whose selector matches it.
- **Selection:** `ClusterHealth` is cluster-scoped; it lists `HealthCheck`s
  matching `spec.selector` (nil/empty selector matches all) under namespace
  precedence: `spec.namespaces` allowlist (definitive when set) → else
  `spec.excludedNamespaces` denylist → else open (all namespaces).
- **Aggregation:** worst-case `Result` over children with a non-empty
  `Status.Result`; a deterministic `children` summary sorted by namespace,
  then name;
  `matchedCount`; and `observedAt` set to the latest child `SourceObservedAt`
  (input freshness, not wall-clock — wall-clock would defeat the no-op guard).

### NodeCertificateCheckReconciler

`internal/controller/nodecertificatecheck_controller.go`

- **Owns / produces:** the node-agent `DaemonSet`, a per-check `ServiceAccount`,
  `RoleBinding`, and `NetworkPolicy` (metrics-only ingress, API-server-only
  egress — see [Network policies](reference/network-policies.md)), and the
  `fathom-node-agent-role` `ClusterRole` (created at
  runtime so its name survives kustomize/OLM name prefixing); creates
  `HealthReport` objects and writes `NodeCertificateCheck.status`. All owned
  objects live in the check's namespace and are owner-referenced for cascading
  garbage collection.
- **Watches:** `NodeCertificateCheck` (`For`), the owned `DaemonSet` /
  `ServiceAccount` / `RoleBinding` / `NetworkPolicy` (`Owns`), and per-node
  report `ConfigMap`s by label (`Watches`), so a fresh node report triggers a
  roll-up.
- **Execution model:** unlike the in-process adapters, on-disk certificate
  scanning must run **on each node**. The reconciler provisions a hardened,
  read-only node-agent `DaemonSet` (`cmd/node-agent`, its own dedicated image —
  see below). Each agent scans the configured certificate paths over read-only
  `hostPath` mounts and publishes a per-node report `ConfigMap`
  (`<check>-<node>`, labelled `source-kind=NodeCertificateCheck`). The operator
  reads those ConfigMaps, rolls them into one `HealthReport` (one check per
  `(node, certificate)`, worst-case aggregate), and mirrors the result into
  `status` (`lastResult`, `lastReportName`, `reportingNodes`/`desiredNodes`).
- **Thresholds:** a certificate within `spec.criticalDays` (default `7`) of
  expiry — or already expired — is `Fail`; within `spec.warnDays` (default `30`)
  is `Warn`. Each agent also exports a `fathom_node_certificate_expiry_days`
  gauge for alerting.
- **Paused:** when `spec.paused`, the agent `DaemonSet` is removed and the last
  status snapshot is preserved (`Ready=False / Paused`).

### Requeue / interval handling

`AddonCheckReconciler` and `NodeCertificateCheckReconciler` both return
`RequeueAfter` based on their effective `spec.interval` (`5m` and `1h` defaults,
respectively). `AddonCheck` uses that cadence to re-run the adapter and refresh
`status.lastRunTime`; it creates a new `HealthReport` only for the first run or
an aggregate-result transition. `NodeCertificateCheck` uses the cadence to
refresh the rolled-up node-agent report, in addition to the ConfigMap-watch
events its agents generate.

`HealthCheckReconciler` and `ClusterHealthReconciler` are projection/
aggregation controllers with no timer; they are event-driven by spec edits and
cross-resource watches.

## The In-Process Adapter Contract

The adapter contract is defined in [`pkg/adapter`](../pkg/adapter) and is the
seam by which Fathom learns to check a new add-on. Adapters are compiled into
the operator; there is no out-of-process plugin boundary (see
[ADR-0001](adr/0001-in-process-adapter-contract.md)).

- **Interface (`pkg/adapter/adapter.go`):** an `Adapter` exposes `Name()`,
  `Version()`, `ContractVersion()`, `Capabilities()`, and
  `Run(ctx, Request) (Result, error)`. Implementations must be safe for
  concurrent use. The returned `error` is reserved for adapter-level failures;
  per-check problems are reported as `CheckResult` entries with `OutcomeFail` or
  `OutcomeError`.
- **Request:** carries a least-privilege controller-runtime `Client`, a logger,
  the driving `TargetRef`, the parsed per-family `Policy`, the run `Timeout`,
  and `ProbeImage` (the operator-level default probe image, added in contract
  `0.2.0`).
- **Result / CheckResult:** a `Result` is the aggregate of zero or more
  `CheckResult`s. Each `CheckResult` carries a `Family`, an `Outcome`, the
  observed `TargetRef`, a `Summary`, a string-keyed `Details` map, and timing.
- **Families:** adapter-defined check groupings keyed in `Request.Policy`. They
  are scoped to the declaring adapter; Fathom imposes no global namespace.

### Registry and version handshake

The in-process registry (`internal/adapter/registry/registry.go`) indexes
adapters by add-on type. `BuildAdapterRegistry` in `internal/app/run.go`
registers every built-in adapter at startup via `builtInAdapters()`.

At registration, `Registry.Register` calls
`adapter.EnsureCompatible(a.ContractVersion())` (`pkg/adapter/version.go`). The
host contract version is the constant `adapter.ContractVersion` (currently
`1.0.0`). Compatibility rules:

- `>= 1.0.0`: same major version, and the adapter's minor must not exceed the
  host's (a newer-minor adapter may rely on contract surface the host lacks).
- `0.x.y` (pre-stable): same major **and** same minor are required — a minor
  bump is treated as breaking.

An adapter that reports an incompatible contract version is rejected at
registration, so the operator fails fast at startup rather than at reconcile
time. Registration also rejects nil adapters and adapters advertising no add-on
types, and treats a fully-overlapping re-registration as an idempotent no-op.

### Built-in adapters

Built-in adapters live under `internal/adapter/*`. Each declares one add-on type
and a set of families:

| Adapter (`addonType`) | Package | Families |
| --- | --- | --- |
| `cert-manager` | `internal/adapter/certmanager` | `system_health`, `issuer_health`, `certificate_health` |
| `coredns` | `internal/adapter/coredns` | `system_health`, `dns_resolution` |
| `kube-state-metrics` | `internal/adapter/kubestatemetrics` | `system_health`, `metrics_endpoint` |
| `external-secrets` | `internal/adapter/declarative` (`externalsecrets.go`) | `system_health`, `secret_sync` |
| `cilium` | `internal/adapter/declarative` (`cilium.go`) | `control_plane_health`, `agent_health`, `crd_health` |
| `external-dns` | `internal/adapter/declarative` (`externaldns.go`) | `system_health`, `crd_health` |
| `metrics-server` | `internal/adapter/declarative` (`metricsserver.go`) | `system_health`, `api_availability` |
| `envoy-gateway` | `internal/adapter/declarative` (`envoygateway.go`) | `system_health`, `crd_health`, `gateway_status` |
| `istio` | `internal/adapter/declarative` (`istio.go`) | `system_health`, `ztunnel_health`, `istio_cni_health`, `crd_health` |
| `keda` | `internal/adapter/declarative` (`keda.go`) | `system_health`, `crd_health`, `scaling_health` |
| `vpa` | `internal/adapter/declarative` (`vpa.go`) | `system_health`, `crd_health`, `recommendation_health` |
| `descheduler` | `internal/adapter/declarative` (`descheduler.go`) | `system_health`, `policy_validity`, `last_run` |
| `kured` | `internal/adapter/declarative` (`kured.go`) | `system_health`, `reboot_state` |
| `argocd` | `internal/adapter/declarative` (`argocd.go`) | `system_health`, `sync_health` |
| `azure-workload-identity` | `internal/adapter/declarative` (`azureworkloadidentity.go`) | `system_health`, `webhook_wiring`, `projection_sanity` |

`internal/adapter/crdutil` is a shared helper used by the CRD-aware adapters to
confirm an add-on's CRDs are installed and served (this is why `NewScheme`
registers `apiextensions/v1`; see [Runtime shape](#runtime-shape)). Per-family
threshold keys are documented inline in each adapter and demonstrated in the
top-level `README.md` examples.

The `cilium` adapter differs from the others in how it treats a missing add-on:
when Cilium is not installed at all (the `cilium-operator` Deployment, the
`cilium` agent DaemonSet, and the core Cilium CRDs are all absent) it reports
`Skipped` (which rolls up green) rather than `Fail`, so a `cilium` AddonCheck
stays quiet on clusters that may or may not run Cilium. A workload that exists
but is unhealthy still reports `Fail`.

## Probe-Pod Model

Active in-cluster network checks (today: CoreDNS `dns_resolution` and
kube-state-metrics `metrics_endpoint`) do not run
inside the operator pod. Instead an adapter launches a single-shot, hardened
**probe pod** per check, in the workload's namespace, so the resolver topology
matches real workloads rather than the operator (see
[ADR-0003](adr/0003-probe-pod-model.md)).

- **Manifest builder (`internal/probe/pod.go`):** `Pod(Request)` builds the pod.
  The probe binary supports modes `dns`, `tcp-connect`, `tcp-listen`, and
  `http-get` (a bounded metrics scrape that can assert expected Prometheus
  metric families). The
  hardening profile is fixed: `AutomountServiceAccountToken=false`,
  `RunAsNonRoot` (UID `65532`), `ReadOnlyRootFilesystem`, drop **ALL**
  capabilities, no privilege escalation, `seccompProfile=RuntimeDefault`,
  `RestartPolicy=Never`, small CPU/memory requests and limits, and an
  `ActiveDeadlineSeconds` of `timeout + 5s`. Optional pod anti-affinity supports
  placing client/server probes on different nodes.
- **Launcher (`internal/probe/launcher.go`):** `Launcher.Run` creates the pod,
  polls it (default `500ms`) until it reaches a terminal phase, parses the JSON
  `Result` the probe wrote to its termination message, and always deletes the
  pod afterward (best-effort, even on context cancellation). It tolerates up to
  3 consecutive transient `NotFound` errors and promotes a previously-observed
  terminal result if the pod vanishes after completing (SKA-429).
- **Orphan sweeper (`internal/probe/sweeper.go`):** the launcher's delete is an
  in-process defer, so a probe pod orphans if the operator dies between pod
  create and cleanup — and the kubelet never garbage-collects terminated pods.
  `Sweeper` runs on the elected leader (once at startup, then hourly) and
  deletes probe pods — identified by the `fathom.skaphos.io/managed-by=fathom`
  and `fathom.skaphos.io/probe` labels — that are in a terminal phase and whose
  containers finished more than a 5-minute grace period ago. The grace is
  measured from container termination rather than pod creation: a probe with a
  long `spec.timeout` is already older than the grace period the moment it
  turns terminal, and sweeping on creation age could delete it out from under a
  launcher still polling for its result. Labels alone are not sufficient: because
  the operator holds cluster-wide pod `delete`, a pod must also match the
  immutable structural shape `Pod()` produces (`restartPolicy: Never`,
  `automountServiceAccountToken: false`, a single container named `probe`)
  before it is reaped. That prevents an actor who can patch labels — but not
  delete — from stamping the probe labels onto a victim pod and borrowing the
  operator's permission. It lists through the live API reader, never the
  manager cache, so no cluster-wide Pod informer is opened.
- **Probe binary (`cmd/probe/main.go`):** a tiny static binary that runs the
  requested mode, writes a JSON `{outcome, summary, details}` to
  `/dev/termination-log` (and stdout), and exits. It ships as the probe image
  built from `Dockerfile.probe` on `scratch`.

The probe image precedence chain (resolved per check by each probe-launching
adapter's `resolveProbeImage`) is: per-`AddonCheck` `probeImage` threshold → operator-level
`--probe-image` (`adapter.Request.ProbeImage`) → adapter-hardcoded fallback
(`ghcr.io/skaphos/fathom-probe:v0.4.0`). The operator default is
`DefaultProbeImage` in `internal/app/options.go`.

## Runtime Shape

The manager is constructed in `internal/app/run.go`:

- **Scheme (`NewScheme`):** registers client-go's scheme, the Fathom
  `v1alpha1` types, and `apiextensions/v1`. The last is required because the
  cert-manager and external-secrets adapters `Get` `CustomResourceDefinition`
  objects to verify an add-on's CRDs are installed; envtest auto-registers it
  but real clusters do not (SKA-422).
- **Manager options (`BuildManagerOptions`):** translates `Options` into
  `ctrl.Options` plus any cert watchers. Performs no cluster I/O, so it is
  unit-testable.
- **Metrics:** served by controller-runtime's metrics server. When
  `metrics.secure` is true (the default), the authn/authz filter
  (`filters.WithAuthenticationAndAuthorization`) is installed so scrapes require
  a valid token with RBAC. `Options.Validate` refuses to serve plaintext metrics
  on a cluster-routable port unless `metrics.allow_insecure` is set (SKA-287).
- **Tracing:** optional OpenTelemetry spans around each reconcile and adapter
  run, exported via OTLP/gRPC (SKA-293). Off by default — `tracing.Init`
  installs a no-op provider when `tracing.enabled` is false, so the hot paths
  carry ~zero overhead. When enabled, a parent-based ratio sampler and a batch
  exporter are wired, and the provider is flushed with a bounded timeout on
  shutdown. See [reference/configuration.md](reference/configuration.md#tracing).
- **HTTP/2:** disabled by default to mitigate CVE-2023-44487 / CVE-2023-39325;
  re-enable with `--enable-http2`.
- **Leader election:** on by default (SKA-303); disable with
  `--leader-elect=false` for single-process local runs. The lease name is
  `leader_election_id` (default `2d3dbc4f.skaphos.io`).
- **Readiness:** `/readyz` is gated on informer cache sync (`readyzCheck`) so a
  not-yet-synced replica is not routed traffic during a rolling update;
  `/healthz` is a plain liveness ping.

The full configuration surface (flags, env vars, config file, defaults) is in
[reference/configuration.md](reference/configuration.md). The internal package
layout is in [code-map.md](code-map.md).

## Known Limitations

- **Single supported wrapper target.** `HealthCheck` only mirrors `AddonCheck`
  in this build; other `checkRef.kind` values are rejected with
  `Ready=False / UnsupportedKind`.
- **Cluster-wide wrapper selection.** `ClusterHealth` is cluster-scoped and
  selects `HealthCheck`s under the allowlist / denylist / open namespace filter
  (`spec.namespaces`, `spec.excludedNamespaces`). A selected `HealthCheck` can
  also mirror an explicit `checkRef.namespace`, so tenancy depends on who is
  allowed to create wrappers, the aggregate's namespace filter, and the
  operator's RBAC to read the referenced target.
