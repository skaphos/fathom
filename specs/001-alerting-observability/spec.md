# Feature Specification: Alerting-Grade Observability — Current-Result and Staleness Metrics, Kubernetes Events

**Feature Branch**: `feature/154-alerting-observability`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Alerting-grade observability for Fathom (skaphos/fathom#154): export a current-result gauge (fathom_check_result{kind,name,namespace,result}) and a last-run-timestamp/staleness metric for every health check, and emit Kubernetes Events on check-result transitions, adapter-run failures, RBAC-provisioning failures, and probe-launch failures so operators can alert on stale or failing checks and see history in kubectl describe."

## Problem Statement

Fathom evaluates cluster health, but two gaps make it hard to *operate* Fathom
itself (skaphos/fathom#154, adversarial review 2026-07-12):

1. **No metric answers "what is this check's result right now?" or "when did it
   last run?"** Today the monitoring endpoint exposes only run counters,
   durations, and registration state. The authoritative result lives solely in
   each resource's status. A wedged operator, a paused check, or a check whose
   selector no longer matches anything leaves a stale `Pass` in status with no
   metric-side signal an alert can fire on — the monitoring guide documents
   this gap and tells users to build their own workaround.
2. **No Kubernetes Events.** Check-result transitions, adapter-run failures,
   RBAC-provisioning failures, and probe-launch failures leave no Event trail,
   so `kubectl describe` on a check shows current conditions but no history of
   what changed and why.

The 1.0 bar for a health operator is that operators can alert on its verdicts
and its liveness using their existing monitoring stack, and can debug it with
standard Kubernetes tooling.

## Clarifications

### Session 2026-07-23

- Q: When a check exists but has never completed an evaluation, what do the
  metrics report? → A: Series appear at discovery with sentinel values:
  current-result reports Unknown, last-run reports 0, so one staleness rule
  catches never-run and stopped-running checks alike (no `absent()`-based
  rules needed).
- Q: How are the ready-to-use alert rules delivered? → A: Both documentation
  examples in the monitoring guide and a shipped, opt-in installable
  PrometheusRule manifest maintained and lint-checked in-repo. Installing it
  is optional so the prometheus-operator CRD never becomes a hard dependency.
- Q: Does a check's very first evaluation result produce an event? → A: Yes —
  the first completed evaluation records a transition event from Unknown to
  the result, so `kubectl describe` is informative from the start. After an
  operator restart the previous result comes from resource status, so no
  false Unknown-transition event fires.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Alert on a check's current result (Priority: P1)

A platform operator points their existing Prometheus-compatible monitoring
stack at Fathom and writes an alert rule on the check-result metric — e.g.
"page when any check in namespace `platform` reports `Fail` for 10 minutes".
When cert-manager breaks, the alert fires; when the check recovers, the metric
returns to the passing state and the alert resolves. No custom
status-to-metric bridge is required.

**Why this priority**: this is the core ask — a health operator whose verdicts
cannot be alerted on forces every user to build the bridge themselves. It is
the single most valuable slice and stands alone.

**Independent Test**: create a check against a healthy target, scrape the
metrics endpoint, and observe the result series reporting the passing state;
break the target, wait for the next run, and observe the series flip to
failing — with no other part of this feature implemented.

**Acceptance Scenarios**:

1. **Given** a check whose latest evaluation is `Pass`, **When** the metrics
   endpoint is scraped, **Then** a current-result series for that check
   (identifying its kind, name, namespace, and result) reports it as the
   current result.
2. **Given** a check whose target then degrades, **When** the next evaluation
   completes with `Fail`, **Then** the very next scrape reflects `Fail` for
   that check and no longer reports `Pass` as current.
3. **Given** every result state a check can report (including error and
   skipped states), **When** each occurs, **Then** each is distinguishable in
   the metric so alert rules can treat them differently.
4. **Given** a check resource that is deleted, **When** the metrics endpoint
   is scraped afterwards, **Then** no series for the deleted check remains.

---

### User Story 2 - Detect stale checks and a wedged operator (Priority: P2)

The same operator alerts on staleness: "warn when a check has not completed an
evaluation for longer than 3× its configured interval". When the operator pod
wedges, when a check is paused, or when a check's target selector matches
nothing, the last-run timestamp stops advancing and the staleness alert fires —
even though the last recorded result is still `Pass`.

**Why this priority**: a stale `Pass` is the most dangerous failure mode a
health operator has — it reports health while asserting nothing. Second only
to being able to see results at all.

**Independent Test**: create a check, record its last-run metric, pause the
check (or stop the operator), and observe the timestamp stop advancing while
wall-clock time passes.

**Acceptance Scenarios**:

1. **Given** a check that completes an evaluation, **When** the metrics
   endpoint is scraped, **Then** a series for that check reports when its
   most recent evaluation completed.
2. **Given** a check that stops being evaluated (paused, wedged operator, or
   otherwise), **When** time passes, **Then** the last-run series stops
   advancing, so a rule comparing it against current time can fire.
3. **Given** a running operator and its documentation, **When** an operator
   follows the monitoring guide, **Then** it contains ready-to-use example
   alert rules for both "check currently failing" and "check stale".

---

### User Story 3 - See check history in kubectl describe (Priority: P3)

A cluster administrator debugging an unhealthy addon runs `kubectl describe`
on the check resource. The Events section shows when the result last changed
(and from what to what), plus any recent operational failures — an adapter run
that errored, a probe pod that could not be launched, per-check RBAC that
could not be provisioned — each with a reason. They diagnose the issue without
reading operator logs.

**Why this priority**: valuable for debugging workflows and expected of a
Kubernetes-native operator, but alerting (Stories 1–2) is the gap that blocks
production adoption.

**Independent Test**: create a check, force a result transition, and verify
`kubectl describe` on that resource shows a transition event; then induce an
operational failure (e.g. an unlaunchable probe) and verify a warning event
appears.

**Acceptance Scenarios**:

1. **Given** a check whose result changes between evaluations, **When** the
   transition happens, **Then** an event is recorded on that resource naming
   the old and new result, and is visible via `kubectl describe`.
2. **Given** a check whose result is unchanged across evaluations, **When**
   evaluations repeat, **Then** no per-evaluation event spam is produced —
   events record transitions and failures, not routine reconciles.
3. **Given** an adapter run failure, a probe-launch failure, or an
   RBAC-provisioning failure, **When** it occurs, **Then** a warning event
   with a machine-usable reason and a human-readable message is recorded on
   the affected check resource.
4. **Given** a reconcile error on any of Fathom's check kinds, **When** it
   occurs, **Then** it is observable as an event on the resource, not only as
   an operator log line.

---

### Edge Cases

- A deleted check MUST NOT leave orphaned metric series behind (stale series
  would keep alerts asserting on a check that no longer exists).
- A renamed/recreated check (delete + create with the same name) MUST report
  only its new lineage — no double series.
- Result transitions through an operator restart: after restart, the first
  evaluation re-establishes current-result and last-run series; a transition
  event MUST NOT falsely fire just because in-memory state was lost, and the
  authoritative previous result is the one recorded in the resource status.
- Metric cardinality MUST stay bounded by the number of check resources and
  the fixed result-state set — no unbounded label values (messages, targets,
  addon versions) on these series.
- Event volume MUST stay bounded under sustained failure: a check that fails
  the same way every interval produces a bounded event trail (Kubernetes
  event aggregation/refresh, not one fresh event per run).
- Cluster-scoped check kinds have no namespace; the metric and event contract
  MUST handle them consistently (empty namespace label, events on the
  cluster-scoped resource itself).
- The read-only degradation principle applies: when checks cannot execute,
  the last-known result series and status remain readable; inability to run
  is surfaced via staleness and failure events, never by blanking data.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST expose, on its existing monitoring endpoint, a
  current-result metric for every check resource it reconciles (all check
  kinds: addon checks, health checks, cluster health aggregates, and node
  certificate checks), identifying kind, name, namespace, and result.
- **FR-002**: The current-result metric MUST distinguish every result state a
  check can report (pass, warn, fail, error, skipped, unknown — the exact set
  is the existing result vocabulary) such that an alert rule can select any
  subset.
- **FR-003**: The system MUST expose a last-run metric per check reporting
  when the most recent evaluation completed, suitable for staleness rules of
  the form "now minus last run exceeds N".
- **FR-003a**: Both series MUST exist from the moment a check resource is
  first observed, before any evaluation completes: current-result reports the
  unknown state and last-run reports 0 (the sentinel for "never ran"), so a
  never-evaluated check is caught by the same staleness rule as one that
  stopped being evaluated.
- **FR-004**: Metric series for a check MUST be removed when the check
  resource is deleted.
- **FR-005**: The system MUST record a Kubernetes event on a check resource
  when its result transitions (old result → new result), including both
  values. A check's first completed evaluation counts as a transition from
  the unknown state, so a fresh check records an event for its initial
  result; the previous result is always taken from the resource status, never
  from in-memory state alone.
- **FR-006**: The system MUST record warning events on the affected check
  resource for operational failures: adapter-run failures, probe-launch
  failures, per-check RBAC-provisioning failures, and reconcile errors.
- **FR-007**: Events MUST NOT be emitted for routine no-change evaluations;
  sustained identical failures MUST produce a bounded event trail rather than
  one event per run.
- **FR-008**: Each event MUST carry a stable, machine-usable reason and a
  human-readable message stating what happened and why (consistent with the
  constitution's explainability principle — "failed" without a reason is a
  defect).
- **FR-009**: The monitoring documentation MUST document the new metrics and
  ship ready-to-use example alert rules for "check currently failing" and
  "check stale", replacing the current documented workaround.
- **FR-009a**: The repository MUST also ship those rules as an opt-in
  installable alert-rule manifest (prometheus-operator `PrometheusRule`),
  maintained and lint-checked in-repo. Installing it is optional: deployments
  without the prometheus-operator CRDs are unaffected, and the operator
  itself takes no dependency on it.
- **FR-010**: Metric label sets MUST be bounded: labels identify the check
  (kind, name, namespace) and the result state only — no free-text or
  per-run-varying label values.
- **FR-011**: The metrics and events MUST be additive observability outputs:
  they are derived from the same evaluations that populate resource status and
  MUST NOT alter the existing status contracts (including the ClusterHealth
  external contract) or check execution behavior.
- **FR-012**: Emitting metrics or events MUST NOT block or fail a
  reconciliation: an unavailable event sink degrades to status-only operation,
  never to a failed check run.

### Key Entities

- **Check resource**: any of the resources Fathom reconciles that carry a
  health verdict (addon check, health check, cluster health, node certificate
  check). Identified by kind, name, and (where namespaced) namespace.
- **Current-result series**: one metric time series per check per result
  state, marking which result is current. Lifecycle is tied to the check
  resource.
- **Last-run series**: one metric time series per check carrying the
  completion time of its most recent evaluation.
- **Check event**: a Kubernetes event attached to a check resource recording
  either a result transition (normal) or an operational failure (warning),
  with reason and message.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator using a standard Prometheus-compatible stack can
  alert on any check reporting a failing result using only shipped metrics
  and the documented example rules — zero custom bridging configuration —
  and the alert reflects a result change by the next scrape after the
  evaluation completes.
- **SC-002**: A wedged operator, a paused check, or a never-matching check is
  detectable from metrics alone within one staleness window (the documented
  rule's threshold, e.g. 3× the check interval) — today it is undetectable
  without log inspection.
- **SC-003**: For any check resource, `kubectl describe` shows the most
  recent result transition and any operational failure from the retained
  event window, with a reason for each — no operator-log access required.
- **SC-004**: 100% of result-state values representable in check status are
  representable in the current-result metric (verified per kind).
- **SC-005**: On a cluster with N check resources, the number of series
  contributed by this feature is bounded by N × (result states + 1), and
  deleting a check removes its series by the next scrape.
- **SC-006**: A check failing identically every interval for 24 hours
  produces a bounded number of events (aggregated/refreshed), not
  ~24h/interval distinct events.

## Assumptions

- First-class metrics are the chosen path: the issue offers "at minimum a
  tested kube-state-metrics custom-resource-state snippet + shipped
  PrometheusRule" as a fallback; this spec targets the primary option
  (operator-exported metrics) because the 1.0 wording requires it and it
  removes a deployment-time dependency for users. Alert rules ship both as
  documented examples in the monitoring guide (FR-009) and as an opt-in
  installable rule manifest (FR-009a).
- The metric naming follows the issue's proposal
  (`fathom_check_result{kind,name,namespace,result}` and
  `fathom_check_last_run_timestamp_seconds`) as the user-facing contract;
  final names are confirmed at planning against existing metric conventions.
- The existing result vocabulary (Pass / Warn / Fail / Error / Skipped /
  Unknown as defined by the CRDs today) is reused unchanged; this feature adds
  no new result states.
- Events use the standard Kubernetes event mechanism (visible in
  `kubectl describe` and `kubectl get events`), with the cluster's default
  event retention; long-term history remains the job of `HealthReport`.
- The node-agent's own metrics endpoint is out of scope: node certificate
  check results are covered via their check resource in the operator, not by
  changing the agent.
- Scrape configuration (ServiceMonitor/PodMonitor or equivalent) is the
  user's responsibility, as it is for the existing metrics; this feature adds
  series to the endpoint users already scrape.
