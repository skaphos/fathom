# Implementation Plan: Alerting-Grade Observability — Current-Result and Staleness Metrics, Kubernetes Events

**Branch**: `feature/154-alerting-observability` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-alerting-observability/spec.md`

## Summary

Add first-class alerting signals to the operator: a one-hot
`fathom_check_result{kind,name,namespace,result}` gauge and a
`fathom_check_last_run_timestamp_seconds{kind,name,namespace}` gauge for all
four check kinds, series existing from first observation (Unknown / 0
sentinels) and deleted with the resource; wire the first
`record.EventRecorder` into all four reconcilers to emit transition events
(first result counts, previous result read from status) and warning events for
adapter-run, probe-launch, RBAC-provisioning, and reconcile failures; ship the
two documented alert rules both in the monitoring guide and as an opt-in
kustomize component carrying a `PrometheusRule` manifest. No CRD schema
changes, no new config options, no change to check execution or the
ClusterHealth contract.

## Technical Context

**Language/Version**: Go (version per `go.mod`), kubebuilder v4 layout

**Primary Dependencies**: controller-runtime (manager, `GetEventRecorderFor`),
`prometheus/client_golang` (existing `internal/metrics` registry),
`k8s.io/client-go/tools/record` (indirect dep already in the module graph).
No new modules.

**Storage**: none — metrics are process-local series in the
controller-runtime registry; events go to the cluster's Event API with
default retention. Durable history remains `HealthReport` (unchanged).

**Testing**: plain unit tests for `internal/metrics` (existing
Reset → Gather → assert `dto.MetricFamily` pattern in `metrics_test.go`);
`record.FakeRecorder` for reconciler event assertions in envtest suites;
e2e assertions via `kubectl get events` / metrics scrape on the kind cluster.

**Target Platform**: the operator binary (Linux container); node-agent is out
of scope per spec Assumptions.

**Project Type**: Kubernetes operator (existing single-module layout)

**Performance Goals**: O(result-state-count) gauge writes per reconcile
(6 label-set writes + 1 timestamp write) — negligible against existing
reconcile work. Event emission is fire-and-forget through the recorder's
buffered broadcaster.

**Constraints**: emission must never fail or block a reconcile (FR-012);
series cardinality bounded at N checks × 7 series (FR-010/SC-005); event
volume bounded by the recorder's built-in aggregation (FR-007/SC-006); no new
`Options`/flags — metrics and events are always-on operator behavior.

**Scale/Scope**: clusters run tens-to-hundreds of check resources; at 500
checks this adds ≤ 3,500 series — well within a normal scrape budget.

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` v1.0.0 — pre-Phase-0 and re-checked post-Phase-1.*

| Principle / constraint | Verdict | Notes |
|---|---|---|
| I. Explicit state over implicit behavior | PASS | Metrics/events are derived views of declared status; no health semantics move out of CRDs. |
| II. Git durable desired-state boundary | PASS | PrometheusRule ships as an in-repo kustomize component; nothing configured out-of-band. |
| III. Deterministic, reconstructible | PASS | Series are a pure function of observed resources + status; manifests rendered by pinned tasks. |
| IV. Kubernetes-native, never obscured | PASS | This feature *closes* the IV gap: "surfaced through CRD status and Kubernetes events" becomes true — first EventRecorder in the codebase. |
| V. Compose, don't trap | PASS | prometheus-operator CRD is opt-in only (component stays commented out by default); operator takes no dependency on it. |
| VI. Explainable, evidence-grade | PASS | Every event carries a stable reason + message (FR-008); "failed" without reason remains a defect. |
| VII. Read-only degradation | PASS | Last-known series persist when checks can't run; staleness + warning events signal inability, never blank data (FR-012 degrades to status-only). |
| VIII. Topology is deployment state | N/A | No topology modeling touched. |
| IX. Technical precision, honest scope | PASS | Monitoring guide's "no result metric" caveat is replaced with actual behavior; promtool-grade rule testing explicitly documented as not provided (see research R6). |
| ClusterHealth contract stability | PASS | Status derivation untouched (FR-011); metrics read status, never feed it. |
| Bounded, idempotent reconciliation | PASS | Fixed small per-reconcile work; emission is non-blocking; no retries added. |
| Minimal RBAC | PASS | Exactly one new grant: core `events` `create;patch` via `+kubebuilder:rbac` markers → `config/rbac/`. |
| Configuration model | PASS | No new options; nothing enters `options.go`. |

**Post-Phase-1 re-check**: design introduces no violations — no Complexity
Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/001-alerting-observability/
├── spec.md              # Feature specification (done)
├── plan.md              # This file
├── research.md          # Phase 0: design decisions R1–R7
├── data-model.md        # Phase 1: series/event schema + per-kind mapping
├── quickstart.md        # Phase 1: end-to-end validation guide
├── contracts/
│   ├── metrics.md       # Metric names, labels, semantics, lifecycle
│   └── events.md        # Event reasons, types, message formats
└── tasks.md             # Phase 2 (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
internal/metrics/
├── metrics.go                    # + CheckResult, CheckLastRunTimestamp gauges;
│                                 #   SetCheckResult / SetCheckObserved /
│                                 #   DeleteCheckSeries helpers
└── metrics_test.go               # + one-hot, sentinel, delete coverage

internal/controller/
├── addoncheck_controller.go      # Recorder field; sentinel-at-discovery; set
│                                 #   gauges at status write; transition event at
│                                 #   existing resultChanged; AdapterRunFailed /
│                                 #   ProbeLaunchFailed events; delete series on
│                                 #   IsNotFound; +kubebuilder:rbac events marker
├── healthcheck_controller.go     # Same pattern around mirrorTarget
├── clusterhealth_controller.go   # Same pattern around aggregate (namespace="")
├── nodecertificatecheck_controller.go  # Same pattern around rollup; RBAC /
│                                 #   admission-policy provisioning-failure events
└── *_test.go                     # FakeRecorder + gauge assertions per kind

internal/probe/
└── launcher.go                   # Typed launch error (errors.As target) so the
                                  #   reconciler can distinguish ProbeLaunchFailed

internal/app/
└── run.go                        # DefaultControllers: thread
                                  #   mgr.GetEventRecorderFor(...) into each
                                  #   reconciler struct

config/
├── rbac/role.yaml                # regenerated: events create;patch
├── components/prometheus-rule/   # NEW opt-in component
│   ├── kustomization.yaml
│   └── prometheusrule.yaml       # FathomCheckFailing / FathomCheckStale
└── default/kustomization.yaml    # commented opt-in entry, mirroring
                                  #   ../components/prometheus

docs/guides/monitoring.md         # §2: document both gauges; §4: replace
                                  #   "read the status, not a metric" with real
                                  #   rules + PrometheusRule component pointer
test/e2e/                         # events + metrics assertions on kind cluster
Taskfile.yml                      # kustomize-build lint gate for the component
```

**Structure Decision**: extend the existing packages in place — the metrics
contract stays in `internal/metrics` (single registry, single init), event
emission lives inside each reconciler at its existing status-write /
error-handling sites (no new "observability" package; keeps cognitive load
low per the engineering guardrails), and manager wiring stays in
`DefaultControllers` (`internal/app/run.go`), which is already the injection
seam unit tests mock through `Setupper`.

## Complexity Tracking

No constitution violations — table intentionally empty.
