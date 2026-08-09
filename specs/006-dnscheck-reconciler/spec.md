# Feature Specification: DNSCheck Reconciler

**Feature Branch**: `feature/266-dnscheck-reconciler`

**Created**: 2026-08-09

**Status**: Draft

**Input**: User description: "GitHub issue skaphos/fathom#266 — feat(controller): DNSCheckReconciler driving the probe dns mode. Part of epic #205 (DNSCheck), Track B2. Depends on #265 (merged as #294). Reconcile honoring `spec.interval` and `spec.timeout`, launch probe pods via `internal/probe.Launcher`, fan out `spec.targets` × `spec.resolvers`, fold per-target results with the shared `WorstResult` fold, persist `HealthReport` only on result change, emit the #240 metrics and Events, and keep probe pods owned and reaped by the #220 sweeper."

## Overview

Feature 005 defined the `DNSCheck` **contract** — what an operator may declare and
what the resource reports. It deliberately stopped short of the component that
executes it. This feature is that component: the controller that evaluates a
`DNSCheck` on its cadence, folds the results into a verdict, publishes them, and
keeps the workloads it creates accountable.

This is a **thin specification**. Every behavioral requirement that feature 005
already stated and explicitly deferred to this one is **inherited by reference**
below, not restated. Restating merged, reviewed language would invite drift
between two copies of the same obligation. Only requirements this feature
genuinely introduces are written out.

## Clarifications

### Session 2026-08-09

- Q: Fan-out topology and concurrency bound (FR-103) — one workload per pair, or
  one per vantage point serving many targets? → A: One workload per pair, with a
  configurable cap on how many are in flight per check. The probe's single-result
  contract shipped in #294 is unchanged.
- Q: Run-bound distribution (FR-104) — is the declared bound per pair or for the
  whole run? → A: The whole run. A pair's own bound is the lesser of the budget
  remaining and a per-pair ceiling. This is the only reading under which the
  shipped `timeout <= interval` admission rule constrains anything, and it makes
  a run structurally incapable of outlasting its own cadence.
- Q: Cadence anchoring (FR-107) — is the next run scheduled a cadence after the
  previous one started, or after it finished? → A: After it started, with a
  floor on the gap. The next run is due one cadence from the previous run's
  start; if that moment has already passed, a minimum gap applies instead. This
  diverges deliberately from `AddonCheck`, which anchors on completion.
- Q: Outcome for pairs a truncated run never reached (FR-106) → A: Unknown. It
  participates in the fold above Warn and below Fail, so incomplete evidence
  degrades the verdict without outranking a genuine failure among the pairs that
  did run. Skipped would report green on 8-of-48 evidence; Error would mask a
  real failure. Truncation is additionally signalled at the check level, since
  the per-pair outcome alone does not tell an operator their bound was too small.

## Inherited Requirements

Feature 005's *Out of Scope* section is a hand-off list. It names, by identifier,
the requirements that only the controller can deliver. Those requirements are
**normative here** exactly as written in
`specs/005-dnscheck-resource-contract/spec.md` — read them there.

| Inherited | The obligation this feature takes on |
|---|---|
| **FR-021** | The one-line summary must name assertion polarity, so a negative-assertion failure is not misread as a resolution outage. |
| **FR-031** | Resolution happens in the check's **own** namespace, never the operator's. |
| **FR-037** | The broader pod-placement permission that FR-031 requires, granted deliberately, shipping **with this component**, and carrying a written justification. |
| **FR-032** | The check participates in the existing check-level result and last-run gauges under its own kind. |
| **FR-033** | A per-target gauge, labelled by check, namespace, subject, record kind, vantage point, and outcome. |
| **FR-034** | That gauge's series count stays bounded by the schema caps alone. |
| **FR-036** | Per-target results are **rebuilt** from the current specification each run, never accumulated; a pair the spec no longer declares loses both its result and its series. |
| **SC-008** | Zero DNS queries originate outside the declaring check's namespace. |
| **SC-009** | A single check's per-target series stay capped at 288 (16 targets × 3 vantage points × 6 outcomes). |

Three further requirements from feature 005 are already satisfied by the probe
binary but constrain this controller, which must not undermine them:

- **FR-035** — the unit of evaluation, of result identity, and of metric series
  is the **(target, vantage point) pair**. The controller performs the fan-out.
- **FR-014** — "the resolver could not be reached" must never be folded together
  with "the resolver answered that the name does not exist".
- **FR-025** — a resolution failure under a positive assertion is a **failing
  verdict**, not an operator-side error.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A declared check produces a verdict on its cadence (Priority: P1)

An operator applies a `DNSCheck` naming one or more DNS subjects. Without any
further action, the check begins evaluating on its declared cadence, and its
status carries a current verdict drawn from the same vocabulary as every other
Fathom check.

**Why this priority**: This is the entire point of the resource. Feature 005
shipped a schema that nothing executes; until this story lands, a `DNSCheck` is
inert. It is the MVP by itself.

**Independent Test**: Apply a check naming a subject known to resolve and a
second known not to. Observe the status verdict change from empty to a folded
result, and observe it refresh on the declared cadence without further input.

**Acceptance Scenarios**:

1. **Given** a check naming one resolvable subject, **When** its first cadence
   elapses, **Then** the status reports a passing verdict and the time it was
   evaluated.
2. **Given** a check naming one resolvable and one unresolvable subject under
   positive assertions, **When** it is evaluated, **Then** the folded verdict is
   failing — not erroring — and the summary distinguishes the two.
3. **Given** a check whose specification is edited, **When** the edit is applied,
   **Then** the next evaluation reflects the new specification and the status
   records the generation it observed.
4. **Given** a check that declares several vantage points, **When** it is
   evaluated, **Then** every (target, vantage point) pair the specification
   implies is evaluated exactly once.

---

### User Story 2 - An operator sees which name failed, not just that the check failed (Priority: P2)

When a check covering many subjects fails, the operator can tell from the
resource and from metrics alone which specific subject and vantage point failed,
without reading controller logs or re-running anything by hand.

**Why this priority**: A folded verdict alone turns every failure into an
investigation. This is what makes the check actionable, but story 1 is still
viable without it.

**Independent Test**: Apply a check with several targets, break exactly one, and
confirm both the per-target results and the per-target gauge identify that one.

**Acceptance Scenarios**:

1. **Given** a check with several targets where one fails, **When** it is
   evaluated, **Then** the per-target results name the failing pair and its
   evidence, and the per-target gauge carries a series for it.
2. **Given** a check whose failing target is removed from the specification,
   **When** the next evaluation runs, **Then** that pair's result is gone from
   the status and its metric series is withdrawn (FR-036).
3. **Given** a check that fails a negative assertion, **When** the summary is
   read, **Then** the polarity is explicit (FR-021).

---

### User Story 3 - Result history is recorded without noise (Priority: P3)

The check's verdict changes are recorded as history and surfaced as cluster
events, so an operator can see when a name started failing — without a new
history record on every uneventful run.

**Why this priority**: Matches doctrine established across the other kinds.
Valuable, but a check that reports current state is already useful.

**Independent Test**: Let a stable check run repeatedly and confirm history does
not grow; then break it and confirm exactly one new record appears.

**Acceptance Scenarios**:

1. **Given** a check whose verdict is unchanged across runs, **When** several
   cadences elapse, **Then** no new history record is written.
2. **Given** a check whose verdict changes, **When** the changed run completes,
   **Then** exactly one history record is written and an event is emitted.

---

### User Story 4 - Evaluation workloads never outlive their check (Priority: P3)

Every workload the controller creates to perform resolution is accountable to
the check that caused it, and is reclaimed even if the controller dies mid-run.

**Why this priority**: An orphan class was a real defect this project already
fixed once (#220). Re-introducing it under a new kind would be a regression, so
this must land with the feature rather than after it — but it is a safety
property, not the user-facing value.

**Independent Test**: Kill the controller during a run and confirm no workload
survives beyond the existing sweeper's reclamation.

**Acceptance Scenarios**:

1. **Given** a run in flight, **When** the controller restarts, **Then** no
   evaluation workload is left behind unreclaimed.
2. **Given** a check is deleted, **When** deletion completes, **Then** its
   workloads, its results, and its metric series are all withdrawn.

### Edge Cases

- A check declares more pairs than can be evaluated within its own run bound —
  the run truncates and the unreached pairs report Unknown (FR-106).
- A run consumes so much of its cadence that the next is already due — a minimum
  gap applies rather than continuous running (FR-107a). Two runs of one check
  overlapping is prevented outright (FR-104a, FR-107b).
- A pair's evaluation workload cannot be placed at all — quota, admission
  rejection, image pull failure, or unschedulable node. The remaining pairs of
  the run still proceed (FR-103b).
- Every pair fails for the same reason (a genuine resolver outage) versus one
  pair failing on its own merits.
- A check declares a vantage point that no longer resolves to a reachable
  address.
- The check's namespace disappears mid-run.
- Two checks in the same namespace declare the identical subject and vantage
  point.

## Requirements *(mandatory)*

### Numbering

This feature's own requirements are numbered from **FR-101** and **SC-101** so
they can never be confused with the feature 005 identifiers inherited above.

### Functional Requirements

**Execution**

- **FR-101**: The controller MUST evaluate a check on its declared cadence, and
  MUST NOT evaluate it more frequently than that cadence except when the
  specification itself changes.
- **FR-102**: A single pass of the reconcile loop MUST perform at most one
  evaluation of a check and MUST return control within a bounded time. Work
  whose duration is not bounded by the specification MUST NOT be performed
  inside the loop.
- **FR-103**: Every (target, vantage point) pair the specification implies
  (FR-035) MUST be evaluated on each run by its **own** evaluation workload, so
  a workload serves exactly one pair. The existing single-query evaluation
  mechanism is used as it stands; this feature does not widen it to carry
  several queries at once.
- **FR-103a**: The number of evaluation workloads in flight for a single check
  at any moment MUST be bounded by a configurable cap. The cluster-wide ceiling
  on concurrent evaluation workloads is then that cap multiplied by the
  controller's own reconcile concurrency, and MUST be derivable from
  configuration alone without observing a running system.
- **FR-103b**: A pair whose workload cannot be placed MUST NOT prevent the
  remaining pairs of the same run from being evaluated. Fault isolation between
  pairs is a property of the one-workload-per-pair choice and MUST be preserved
  by any future batching.
- **FR-104**: The declared run bound MUST bound the **run as a whole**, not each
  pair independently. Every pair is evaluated from that one budget, and a pair's
  own bound MUST be the lesser of the budget remaining and a per-pair ceiling —
  so no pair may consume budget that later pairs need, and no single unresponsive
  pair may consume the whole run.
- **FR-104a**: A run MUST NOT be able to outlast the cadence that scheduled it.
  This follows from FR-104 together with the contract's existing rule that the
  run bound may not exceed the cadence, and MUST be preserved rather than
  re-derived.
- **FR-104b**: The relationship between pair count, the per-pair cost of
  evaluation, and the run bound required to reach every pair MUST be documented,
  so an operator can size the bound before applying a check rather than
  discovering the shortfall from truncated runs. A check at the schema's maximum
  pair count is **not** expected to complete at the schema's default bound.
- **FR-105**: A pair whose evaluation could not be **performed** MUST be
  reported distinguishably from a pair whose resolver **answered**. The former
  is a fault on Fathom's side; the latter is the check's finding (FR-014,
  FR-025).
- **FR-106**: When a run cannot reach every pair within the run bound, each pair
  it did not reach MUST be reported as **Unknown** — a participating outcome that
  ranks above Warn and below Fail. Incomplete evidence therefore degrades the
  check's verdict, while a genuine failure among the pairs that *were* reached
  still wins the fold. The unreached pairs MUST NOT be reported as though a
  resolver had answered, and MUST NOT be reported as informational, which would
  let a mostly-unevaluated run report as passing.
- **FR-106a**: A truncated run MUST additionally be signalled at the check level,
  distinctly from the verdict itself, so an operator learns that the run bound
  was too small rather than only that the verdict degraded. The signal MUST name
  how many pairs went unreached.
- **FR-107**: The next run MUST be scheduled one cadence from the point the
  previous run **started**, not from the point it finished, so that the
  effective cadence equals the declared one whenever the run leaves any slack.
- **FR-107a**: When a run consumes so much of its cadence that the next run is
  already due, a minimum gap MUST separate them. A check that cannot keep up
  MUST degrade to a bounded rate, never to continuous back-to-back running.
- **FR-107b**: Two runs of the same check MUST NOT be in flight simultaneously.
- **FR-107c**: The effective cadence MUST be derivable by an operator from the
  declared cadence and the minimum gap alone, without observing timestamps.

**Status**

- **FR-108**: The check's single verdict MUST be produced by the project's
  shared outcome fold, with no DNSCheck-specific precedence rules, so this kind
  agrees with every other kind on how outcomes combine.
- **FR-109**: The status MUST report when the check was last evaluated, the
  specification generation observed, and how many pairs that specification
  implies.
- **FR-110**: Per-target results MUST carry enough evidence to explain the
  pair's outcome without re-running the check.

**History and signalling**

- **FR-111**: A history record MUST be written only when the check's verdict
  changes, matching the doctrine already established for the other kinds.
- **FR-112**: Verdict changes MUST be surfaced as cluster events using the
  existing convention, so consumers need no DNSCheck-specific handling.

**Lifecycle**

- **FR-113**: Every workload the controller creates MUST be attributable to the
  check that caused it and MUST be reclaimable by the project's existing
  reclamation sweep. A controller restart mid-run MUST NOT leave a workload that
  nothing will reclaim.
- **FR-114**: Deleting a check MUST result in its workloads, its reported
  results, and its metric series all being withdrawn.

**Permission**

- **FR-115**: The broader pod-placement permission required by FR-031 ships with
  this component (FR-037) and MUST be accompanied by an in-repository document
  defending why it is needed and why nothing narrower suffices. The permission
  MUST NOT be widened beyond what FR-031 requires.

### Key Entities

- **Check**: the declared `DNSCheck` — its subjects, vantage points, cadence,
  and run bound. Defined by feature 005; unchanged here.
- **Pair**: one (target, vantage point) combination. The unit of evaluation, of
  result identity, and of metric series (FR-035).
- **Run**: one evaluation of every pair a check implies, producing one verdict.
- **Evaluation workload**: the short-lived workload that performs a single
  pair's query in the check's namespace and reports one outcome.

## Success Criteria *(mandatory)*

Feature 005's **SC-008** and **SC-009** are inherited and verified here.

### Measurable Outcomes

- **SC-101**: A run never exceeds its declared run bound, at every pair count
  the schema permits — including the maximum of 48. Where the bound is too small
  to reach every pair, the run truncates visibly rather than overrunning.
- **SC-107**: An operator can compute, from documentation alone, the run bound a
  check of a given pair count requires in order to reach every pair — before
  applying it, without observing a truncated run.
- **SC-108**: For a check whose runs leave any slack in their cadence, the
  observed interval between consecutive run starts equals the declared cadence,
  and does not drift by the duration of the runs themselves.
- **SC-102**: No evaluation workload outlives its check beyond the existing
  reclamation sweep's period, including when the controller is killed mid-run.
  The steady-state count of unreclaimed workloads is zero.
- **SC-103**: A check whose verdict is stable across repeated runs produces
  exactly one history record, not one per run.
- **SC-104**: An operator can name the specific failing subject and vantage
  point from the resource and its metrics alone, without reading controller
  logs.
- **SC-105**: Removing a target from a specification clears both its reported
  result and its metric series within one subsequent run.
- **SC-106**: The permission granted for FR-031 is accompanied by a
  justification document, and the granted verbs do not exceed what pod
  placement in a foreign namespace requires.

## Assumptions

- The `DNSCheck` contract from feature 005 is fixed. This feature does not
  change the schema; if execution proves the contract wrong, that is a separate
  change against feature 005.
- The probe binary's DNS mode, its record kinds, its expected-answer matching,
  its negative assertions, and its vantage points are complete as shipped in
  #294. This feature drives them; it does not extend them. The one-workload-per-pair
  decision recorded under Clarifications keeps that boundary intact.
- The existing workload launcher, pod builder, and reclamation sweep are reused
  as they stand.
- The shared outcome fold, the check-level metrics, and the event conventions
  already exist and are reused rather than re-derived.
- The operator's existing per-namespace behaviour for the other check kinds is
  the model for this one wherever this specification is silent.

## Out of Scope

- **The run-now trigger.** Issue #266 anticipated it, but #264 — which extends
  the trigger beyond `AddonCheck` to every kind — is still open and is being
  taken next. This feature ships cadence-driven evaluation only, and inherits
  the trigger when #264 lands.
- **Any pause or suspend surface.** Feature 005's FR-019 forbids it, and
  `DNSCheck` never had the field, so #262's removal work does not touch this
  kind.
- **Mirroring a `DNSCheck` into the cluster-wide aggregate.** Feature 005 placed
  this out of scope and nothing here changes that.
- **The operator-facing guide.** Documentation of the resource for end users
  travels separately.
- **Correcting `AddonCheck`'s cadence anchoring.** `AddonCheck` schedules its
  next run a full cadence after the previous one *finished*, so its effective
  cadence drifts by the run duration. FR-107 deliberately does not copy that.
  Whether `AddonCheck` should be brought into line is a real question, but it
  is a change to a shipped kind's observable timing and belongs in its own
  issue — not folded into this one.
- **Extending the record kinds, DNSSEC validation, latency thresholds, and
  exact-set answer assertions** — all out of scope in feature 005 and still out
  of scope here.
