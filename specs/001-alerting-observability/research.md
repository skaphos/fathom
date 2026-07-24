# Phase 0 Research: Alerting-Grade Observability

No NEEDS CLARIFICATION markers survived `/speckit-clarify`; this phase records
the design decisions that turn the clarified spec into an implementable
contract, each with rationale and rejected alternatives.

## R1 — Result gauge encoding: one-hot across all six states

**Decision**: `fathom_check_result{kind,name,namespace,result}` emits one
series per result state per check — value `1` for the current state, `0` for
the other five. All six series exist whenever the check exists.

**Rationale**: alert rules become trivial and absence-proof
(`fathom_check_result{result="Fail"} == 1`); a scrape always distinguishes
"check is not failing" (`0`) from "check unknown to Fathom" (no series).
Matches SC-005's budget of N × (states + 1) exactly, and matches the
kube-state-metrics `stateSet` convention operators already know.

**Alternatives considered**: (a) single series with a numeric result mapping
(1=Pass…6=Error) — compact but forces magic-number rules and breaks when the
vocabulary grows; (b) only the current state's series present — smaller but
alerting on `absent()` of a labeled series is fragile PromQL. Both rejected.

## R2 — Staleness metric: reconcile-completion wall time, one per check

**Decision**: `fathom_check_last_run_timestamp_seconds{kind,name,namespace}`
is set to wall-clock time when the reconciler finishes writing an evaluated
result to status — uniformly for all four kinds, not from per-kind status
timestamps (`lastRunTime`, `observedAt`, `sourceObservedAt`).

**Rationale**: the metric answers "when did the operator last evaluate this
check", which is the staleness question; per-kind status timestamps have
divergent semantics (e.g. HealthCheck's `sourceObservedAt` is the *source's*
observation time) and would make one staleness rule mean four things.

**Alternatives considered**: mirroring each kind's status timestamp —
rejected for the semantic divergence above.

**Implementation amendment (2026-07-23)**: the executing-kinds half stands
(AddonCheck/NodeCertificateCheck stamp their own evaluation time, which their
`lastRunTime` already records at the same instant), but for the *wrapper*
kinds a pure watch-driven mirror has no cadence of its own: stamping "now" at
mirror time and nothing else would freeze only when the operator freezes,
missing the stale-source case. The shipped semantics are therefore "freshness
of the evidence backing the current result": HealthCheck mirrors
`sourceObservedAt` (its target's run time) and ClusterHealth `observedAt`
(freshest child). A stale source now reads as a stale wrapper — strictly more
useful for the staleness alert, uniform in meaning, and it also gives correct
re-establishment after an operator restart. Contracts and docs state this.

## R3 — Series lifecycle: sentinel at discovery, delete on IsNotFound

**Decision**: on the first reconcile of a check with no evaluated status,
emit the full one-hot set with `Unknown=1` and last-run `0` (clarification
Q1). On the reconcile that observes deletion (the existing
`apierrors.IsNotFound` early-return in all four controllers), call
`DeletePartialMatch` on both gauge vecs for that {kind,name,namespace}
before returning. No finalizers are added.

**Rationale**: controller-runtime triggers a reconcile for every existing
object at informer sync and for every delete event, so discovery and cleanup
both ride existing reconciles. A process restart resets the in-memory
registry, and startup reconciles repopulate every *existing* check — so a
delete missed while the operator was down cannot leave a stale series.
Finalizers would add deletion latency and RBAC surface for a problem the
registry lifecycle already solves.

**Alternatives considered**: finalizer-based cleanup — rejected (adds a
mutation path and deletion coupling purely for metrics hygiene); periodic
resync sweep comparing registry to cluster — rejected (unbounded background
work, violates the bounded-reconciliation constraint).

## R4 — Events: manager EventRecorder, per-controller sources

**Decision**: thread `mgr.GetEventRecorderFor("fathom-<kind>-controller")`
into each reconciler as a `Recorder record.EventRecorder` struct field set in
`DefaultControllers`. Emit:
- **Normal** `ResultChanged` on transitions where the new result's severity
  is below Warn; **Warning** `ResultChanged` otherwise (severity per the
  existing `HealthReportResult.Severity()` ladder).
- **Warning** operational-failure events at the existing error sites:
  `AdapterRunFailed` (AddonCheck `runErr` handling), `ProbeLaunchFailed`
  (see R5), `RBACProvisioningFailed` / `AdmissionPolicyProvisioningFailed`
  (NodeCertificateCheck ensure* failures), `ReconcileError` (terminal
  reconcile errors on any kind).

The first evaluated result is a transition from `Unknown` (clarification
Q3); the previous result is always read from the resource status
(`status.lastResult` / `status.result`), never from process memory, so
restarts cannot fire false transitions.

**Rationale**: the manager's recorder rides the shared broadcaster with
client-go's built-in EventCorrelator, which aggregates identical events and
increments counts — that alone satisfies FR-007/SC-006's bounded-volume
requirement without custom throttling. Emitting only at status-write and
error sites keeps reconciles idempotent-quiet (no event on a no-change run).

**Alternatives considered**: a hand-rolled events client — rejected, loses
aggregation and correlation; events.k8s.io/v1 direct API — the recorder
abstraction already writes the modern API where available.

## R5 — Distinguishing probe-launch failures from generic adapter failures

**Decision**: introduce a typed error in `internal/probe` (e.g.
`launcher.Run` wraps pod-build and pod-create failures in a `LaunchError`
that supports `errors.As`). The AddonCheck reconciler inspects `runErr`: a
matched `LaunchError` emits `ProbeLaunchFailed`, anything else emits
`AdapterRunFailed`.

**Rationale**: probe launch failures (image pull, RBAC, quota) are
infrastructure faults with different remediation than an addon actually
failing its checks; FR-006 names them as a distinct category. String
matching on error text is not a contract; a typed error is.

**Alternatives considered**: dedicated event emission inside
`internal/probe` — rejected, the launcher has no reference to the check
resource the event must attach to; separate reason per probe mode —
rejected as cardinality without operational value.

## R6 — PrometheusRule delivery and lint gate

**Decision**: new opt-in kustomize component `config/components/prometheus-rule/`
(kustomization + `prometheusrule.yaml` with `FathomCheckFailing` and
`FathomCheckStale` rules), wired exactly like the existing
`config/components/prometheus` ServiceMonitor component: a commented
`components:` entry in `config/default/kustomization.yaml` (clarification
Q2 — shipped *and* opt-in, so the prometheus-operator CRD never becomes an
install dependency). The lint gate is a task step that `kustomize build`s the
component (catches YAML/kustomize breakage in CI); full `promtool`-style rule
unit tests are **not** added — pinning promtool into `tools/` for two rules
is not worth the toolchain weight, and the honesty principle (IX) requires
saying so: the monitoring guide notes rules are build-validated, not
promtool-tested.

**Alternatives considered**: rules inside the Helm chart only — rejected,
kustomize users get nothing; promtool in `tools/` — deferred until the rule
set grows.

## R7 — Where the gauge code lives

**Decision**: extend `internal/metrics` with the two `GaugeVec`s plus three
helpers — `SetCheckResult(kind, name, namespace, result)` writing the full
one-hot set, `SetCheckObserved(kind, name, namespace, t)` for last-run
(`t=0` sentinel case included), and `DeleteCheckSeries(kind, name,
namespace)` wrapping `DeletePartialMatch` on both vecs. Controllers call the
helpers; the result-state list lives next to the vocabulary it mirrors
(driven off the `api/v1alpha1` constants so a new result state cannot
silently miss the metric — FR-002/SC-004).

**Rationale**: single registry and init point already exist there; a
dedicated helper keeps the one-hot invariant (exactly one `1`) in one tested
place instead of four reconcilers.

**Alternatives considered**: per-controller inline gauge writes — rejected,
four copies of the invariant; new `internal/observability` package —
rejected, adds a layer for no seam.
