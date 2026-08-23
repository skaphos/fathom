# Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth

**Feature Branch**: `feature/277-clusterhealth-staleness`

**Created**: 2026-08-23

**Status**: Draft

**Input**: GitHub issue [#277](https://github.com/skaphos/fathom/issues/277) — "ClusterHealth ObservedAt takes the newest child, masking a stuck child promoting Fail"

## Problem

`ClusterHealth` is the headline "one verdict for the cluster" resource. Its
freshness signal is currently the **newest** child's observation time, while its
verdict is the **worst** of all children. Those two folds disagree, and the
disagreement is silent.

For an aggregate with one live child and one frozen child:

- the frozen child's stale `Fail` still propagates into the aggregate verdict;
- but the aggregate reports itself perfectly fresh, because a live sibling
  supplies the newest timestamp.

The documented mechanism for catching "the last recorded result can no longer be
trusted" is a staleness alert. At the aggregate layer that mechanism is defeated
by a single healthy sibling — precisely where operators are most likely to be
watching.

### Three sources currently disagree

Verification on `main` (commit `76843af`) found the aggregate's freshness field
described three different ways:

| Source | Claim |
| --- | --- |
| `internal/controller/clusterhealth_controller.go` | Sets the field to the **newest** child's `SourceObservedAt` |
| `api/v1alpha1/clusterhealth_types.go` (godoc, flows into the generated API reference) | "ObservedAt is when the **aggregator last refreshed** this status" |
| `docs/guides/monitoring.md` | "a `ClusterHealth` [carries] the **freshest of its children** — so a stale source reads as a stale wrapper" |

The code matches neither its own godoc nor the operator-facing guide, and the
guide's stated guarantee ("a stale source reads as a stale wrapper") is the exact
property that does not hold.

### A naive fix is wrong

Taking the **oldest** child instead would fix the masking but break a different
case: check cadences differ by more than 10× (`defaultAddonCheckInterval` 5m vs
`defaultNodeCertInterval` 1h). A slow-but-healthy hourly child would drag every
aggregate containing it into permanent staleness. Any correct answer must
evaluate each child against **its own expected cadence**, not against a single
global minimum or maximum.

### The cadence gap is already causing a second, live problem

No check kind exposes its effective interval anywhere in status or metrics.
Consequently the shipped sample alert rule in `docs/guides/monitoring.md` and
`config/components/prometheus-rule` hardcodes a single threshold:

```
expr: time() - fathom_check_last_run_timestamp_seconds > 900
# comment: "900s suits the default 5m AddonCheck interval (3x interval); tune to yours."
```

Against a 1h `NodeCertificateCheck` that rule fires continuously as a false
positive. Adopters cannot express "overdue relative to this check's own cadence"
because the cadence is not published. This is the same root gap as the aggregate
masking, seen from the metrics side.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A frozen child cannot hide behind a healthy sibling (Priority: P1)

A platform operator watches a single `ClusterHealth` as the cluster's headline
verdict. One contributing check wedges — a stuck reconcile, a target held in
last-good preservation on transient lookup errors (#248), or a target in a
terminal state. Its recorded verdict freezes at `Fail`. Another contributing
check keeps running normally.

Today the aggregate promotes `Fail` indefinitely while reporting itself fully
fresh, so the operator's staleness alert never fires and nothing contradicts the
frozen verdict — they cannot tell "this is genuinely failing right now" from
"this froze at Fail an hour ago and nobody knows".

After this change the aggregate publishes a freshness signal that reflects the
**stalest** contributing evidence, so the operator can distinguish a live failure
from a frozen one.

**Why this priority**: This is the reported defect, it affects the most prominent
resource Fathom exposes, and it silently disarms the one alert documented to
catch untrustworthy results.

**Independent Test**: Create a `ClusterHealth` selecting two `HealthCheck`
children; hold one child's observation time frozen while the other advances;
assert the aggregate's freshness signal reflects the frozen child. Delivers the
core value on its own.

**Acceptance Scenarios**:

1. **Given** an aggregate with one live child and one child frozen at `Fail`,
   **When** the aggregate is reconciled, **Then** it MUST NOT simultaneously
   report `Fail` and full freshness.
2. **Given** the same aggregate, **When** an operator inspects its status,
   **Then** the stalest contributing evidence MUST be discoverable without
   reading each child individually.
3. **Given** an aggregate whose children are all current, **When** it is
   reconciled, **Then** it MUST report as fresh and MUST NOT raise any staleness
   signal.

---

### User Story 2 - A healthy slow check does not poison its aggregate (Priority: P1)

An operator's `ClusterHealth` selects both a 5-minute addon check and a
1-hour node-certificate check. Both are healthy and both are running exactly on
their intended cadence.

The aggregate must treat both as current. A check observed 20 minutes ago is
perfectly healthy at an hourly cadence and badly overdue at a 5-minute cadence;
the same absolute age means opposite things.

**Why this priority**: Equal in priority to User Story 1 — a fix that satisfies
only US1 (for example, "use the oldest child") ships a permanent false positive
and would be worse than the current behavior for any mixed-cadence aggregate.
The two stories together define correctness; neither alone does.

**Independent Test**: Build an aggregate mixing a short-interval and a
long-interval child, both observed within their own cadence; assert the aggregate
reports fresh and raises no staleness signal.

**Acceptance Scenarios**:

1. **Given** an aggregate containing a healthy child on a long interval and a
   healthy child on a short interval, **When** it is reconciled, **Then** it MUST
   report as fresh.
2. **Given** a child whose age exceeds its own expected cadence by the overdue
   allowance, **When** the aggregate is reconciled, **Then** that child MUST be
   counted as overdue regardless of how short or long its interval is.
3. **Given** two children of different cadences that are the same absolute age,
   **When** only one exceeds its own cadence allowance, **Then** only that one
   MUST be counted as overdue.

---

### User Story 3 - Staleness can be alerted on without guessing a threshold (Priority: P2)

An operator installs Fathom's shipped alerting rules across a fleet running
checks at several different cadences. They want a single staleness rule that is
correct for every check without hand-tuning a threshold per check kind.

Today the shipped rule hardcodes 900 seconds and its own comment concedes it only
suits the default 5-minute cadence, so it false-positives on every hourly check.

**Why this priority**: P2 because US1 and US2 restore correctness of the resource
contract, which is the reported defect; this story makes the fix usable from the
metrics surface, where most operators actually consume it. It is the difference
between "the status is now correct" and "operators can act on it".

**Independent Test**: With checks at two different cadences, a single
cadence-relative alerting expression correctly identifies only the genuinely
overdue check.

**Acceptance Scenarios**:

1. **Given** checks running at different cadences, **When** an operator writes a
   staleness rule, **Then** they MUST be able to express "overdue relative to
   this check's own cadence" without embedding a per-kind constant.
2. **Given** a check running normally at any cadence, **When** the shipped
   staleness rule is evaluated, **Then** it MUST NOT fire.

---

### Edge Cases

- **A child has never run.** A selected child with no observation time yet must
  be treated as the strongest possible staleness signal, not silently skipped —
  consistent with the existing "coerce empty verdict to Unknown" rule and with
  the metrics' 0 "never ran" sentinel.
- **No children match the selector.** The existing `NoMatches` path must be
  unaffected; an aggregate matching nothing must not report itself stale on top
  of already reporting not-ready.
- **Every child is frozen.** The aggregate must report the stalest evidence, not
  fall back to "fresh" through an empty-set default.
- **Clock skew / future timestamps.** A child reporting an observation time in the
  future must not be able to make an aggregate look fresh indefinitely.
- **A child's cadence changes.** Overdue evaluation must follow the child's
  current effective cadence, not one captured when the aggregate was first
  reconciled.
- **A large aggregate.** Per-child evaluation must stay bounded and must not turn
  reconciliation into unbounded work.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The aggregate MUST publish a freshness signal derived from the
  **stalest** contributing child, so that a frozen child cannot be masked by a
  live sibling.
- **FR-002**: Staleness for each contributing child MUST be evaluated relative to
  **that child's own expected cadence**, never against a single global threshold
  shared across cadences.
- **FR-003**: The aggregate MUST expose how many contributing children are
  currently overdue, so an operator can act without inspecting each child.
- **FR-004**: A contributing child that has never produced an observation MUST be
  treated as overdue.
- **FR-005**: An observation time in the future MUST NOT cause a child to be
  treated as fresher than the present moment.
- **FR-006**: Each check's effective cadence MUST be discoverable by consumers so
  that staleness can be expressed relative to cadence rather than as a
  hardcoded constant.
- **FR-007**: The existing aggregate verdict fold (worst-of, with empty coerced to
  `Unknown`) MUST remain unchanged by this feature.
- **FR-008**: The aggregate MUST continue to derive exclusively from
  `HealthCheck.status` and MUST NOT read `HealthReport` history.
- **FR-009**: Any field whose documented meaning changes MUST have its godoc, the
  generated API reference, and `docs/guides/monitoring.md` brought into agreement
  with the implemented behavior, resolving the current three-way contradiction.
- **FR-010**: The shipped sample alerting rules MUST be updated so that a single
  staleness rule is correct across differing cadences.
- **FR-011**: Existing consumers of the aggregate's current freshness field MUST
  have a defined migration outcome — either the field's meaning is preserved and
  new signal is added alongside it, or the change is called out as a breaking
  contract change with a stated rationale.
- **FR-012**: The overdue allowance (how far past its cadence a check may drift
  before counting as overdue) MUST be consistent across check kinds and stated in
  operator-facing documentation.
- **FR-013**: [NEEDS CLARIFICATION: Must a stale aggregate also change its
  *verdict* (for example, degrading to `Unknown` when the evidence behind a
  `Fail` is no longer trustworthy), or is a separate freshness signal alone
  sufficient while the verdict continues to reflect worst-of recorded results?]
- **FR-014**: [NEEDS CLARIFICATION: Does cadence-aware staleness apply only to
  the `ClusterHealth` aggregate, or to every check kind's own status as well
  (making each individual check self-describing about being overdue)?]

### Key Entities

- **Aggregate health record**: the cluster-level roll-up. Carries a verdict, a
  freshness signal, a per-child summary, and — new in this feature — an explicit
  count of overdue contributors.
- **Contributing child summary**: the per-child entry the aggregate already
  publishes (identity, verdict, summary, observation time). This feature adds the
  ability to tell whether that child is overdue *for its own cadence*.
- **Effective cadence**: the interval a given check is actually expected to run
  at, after defaults and any per-resource override are applied. Currently
  internal; this feature requires it to be discoverable by consumers.
- **Overdue allowance**: the agreed multiple of a check's cadence beyond which it
  is considered overdue rather than merely late.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In an aggregate containing a frozen contributor and a healthy
  contributor, an operator can determine that the evidence is stale in a single
  look at the aggregate, without opening any child resource.
- **SC-002**: An aggregate mixing checks whose cadences differ by more than 10×,
  all running on schedule, reports zero overdue contributors and raises no
  staleness signal — no false positives from cadence mixing.
- **SC-003**: A single staleness alerting expression is correct for every shipped
  check kind, with no per-kind threshold constants remaining in the shipped rules.
- **SC-004**: 100% of the statements describing the aggregate's freshness field
  agree with its implemented behavior across the CRD godoc, the generated API
  reference, and the monitoring guide.
- **SC-005**: A regression test reproduces the reported failure — two
  contributors, one live and one frozen at `Fail` — and fails against the
  pre-change behavior.
- **SC-006**: Reconciliation cost for an aggregate remains proportional to its
  number of contributors, with no additional per-child API reads introduced.

## Assumptions

- **Overdue allowance defaults to 3× a check's cadence.** This matches the factor
  the shipped alert rule already uses (900s against a 5m interval) and the
  reasoning documented beside it, so it preserves existing behavior for the
  default cadence. To be confirmed at plan time.
- **Effective cadence is already computable from existing state.** Interval
  defaults per kind exist in the controllers (`defaultAddonCheckInterval`,
  `defaultNodeCertInterval`); this feature surfaces them rather than introducing
  new scheduling behavior.
- **"Stalest evidence" is expressed as a timestamp**, consistent with how
  freshness is represented elsewhere, rather than as a duration that would go
  stale in stored status.
- **This feature does not change when checks run.** It changes only what is
  reported about how current their results are.
- **A new status field is acceptable and additive**, as long as the existing
  derivation constraint (aggregate reads only `HealthCheck.status`) holds.

## Dependencies and Constraints

- **Constitution — `ClusterHealth` contract stability**: the aggregate is derived
  only from `HealthCheck.status`, never from `HealthReport` history. Changing that
  derivation is a breaking API change and is out of scope here.
- **Constitution — bounded, idempotent reconciliation**: per-child staleness
  evaluation must not introduce unbounded work in the reconcile loop.
- **CRD versioning is a one-way door.** Any status field added by this feature is
  a schema change that MUST land **before** [#149](https://github.com/skaphos/fathom/issues/149)
  promotes CRDs from `v1alpha1` to `v1`. After that freeze, in-place breaking
  changes to a GA version are forbidden by the CRD API versioning standard. This
  is the single most important scheduling constraint on this work.
- **e2e is required.** This touches `internal/controller/*`, which per `AGENTS.md`
  mandates an e2e run before the PR is considered ready.
- **Generated artifacts are drift-gated.** `docs/reference/api.md` is generated;
  any godoc change must be regenerated via the documented task, never hand-edited.
- **Related work**:
  - [#279](https://github.com/skaphos/fathom/issues/279) (bounded opt-in CR label
    projection onto check gauges) shares the metrics surface FR-006 touches;
    sequencing should be considered to avoid conflicting gauge changes.
  - [#262](https://github.com/skaphos/fathom/issues/262) (remove `spec.paused`)
    eliminates one cause of frozen children but not the class, so it neither
    blocks nor substitutes for this work.
  - [#248](https://github.com/skaphos/fathom/issues/248) (last-good preservation
    on transient target lookup errors) is a deliberate, ongoing source of frozen
    children — this feature must not treat it as a bug to suppress.

## Out of Scope

- Changing which checks run, or their scheduling/cadence behavior.
- Changing the worst-of verdict fold itself (see FR-007).
- Auto-remediating or auto-clearing frozen children.
- Deriving any part of the aggregate from `HealthReport` history.
- Promoting CRDs to `v1` (tracked separately in #149).
