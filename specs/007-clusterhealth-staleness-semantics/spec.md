# Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth

**Feature Branch**: `feature/277-clusterhealth-staleness`

**Created**: 2026-08-23

**Status**: Draft — scope decisions resolved, ready for `/speckit-plan`

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

### The metric is wrong too, not just the status

`clusterhealth_controller.go` feeds `Status.ObservedAt` straight into the
`fathom_check_last_run_timestamp_seconds` gauge, under a comment asserting the
very guarantee that does not hold:

> "ObservedAt is the latest input freshness across children … so the staleness
> gauge follows the evidence chain: stale children — or a selector matching
> nothing — read as a stale roll-up."

So the defect reaches the surface operators actually alert on. Both surfaces are
fed from one field, which means a single change to how that field is derived
repairs the status and the gauge together.

### Four sources currently disagree

Verified on `main` (commit `74f524f`):

| Source | Claim |
| --- | --- |
| `internal/controller/clusterhealth_controller.go` (aggregation) | Sets the field to the **newest** child's `SourceObservedAt` |
| `internal/controller/clusterhealth_controller.go` (gauge comment) | "stale children … read as a stale roll-up" |
| `api/v1alpha1/clusterhealth_types.go` godoc (flows into the generated API reference) | "ObservedAt is when the **aggregator last refreshed** this status" |
| `docs/guides/monitoring.md` | "a `ClusterHealth` [carries] the **freshest of its children** — so a stale source reads as a stale wrapper" |

Only the first describes what the code does. The other three all promise, in
different words, that a stale child produces a stale aggregate.

### A naive fix is wrong

Taking the **oldest** child unconditionally would fix the masking but break a
different case: check cadences differ by more than 10×
(`defaultAddonCheckInterval` 5m vs `defaultNodeCertInterval` 1h). A
slow-but-healthy hourly child would drag every aggregate containing it into
permanent staleness. Freshness must be judged against each check's **own**
cadence, not a single global threshold.

### The cadence gap is already causing a second, live problem

No check kind publishes its effective interval. Consequently the shipped alert
rule hardcodes one threshold:

```
expr: time() - fathom_check_last_run_timestamp_seconds > 900
# comment: "900s suits the default 5m AddonCheck interval (3x interval); tune to yours."
```

Against a 1-hour `NodeCertificateCheck` that rule fires continuously as a false
positive. Adopters cannot express "overdue relative to this check's own cadence"
because the cadence is not published. Same root gap, seen from the metrics side.

## Scope Decisions

Two questions were resolved before planning; they are recorded here because they
bound everything below.

### D1 — Staleness is a signal, never a verdict change

The aggregate's `Status.Result` is **not** modified by this feature. Staleness is
reported alongside the verdict, never folded into it. Rationale:

- **Degrading a stale `Fail` would silence alerts.** The severity ladder is
  `Pass(1) < Skipped(2) < Warn(3) < Unknown(4) < Fail(5) < Error(6)`, so
  `Unknown` is *less* severe than `Fail`. Downgrading a stale `Fail` to `Unknown`
  would stop `fathom_check_result{result=~"Fail|Error"} == 1` from firing,
  converting "failing and now untrustworthy" into "not failing". A frozen `Fail`
  is still the best evidence available; nothing has contradicted it.
- **It could not work without new machinery.** The `ClusterHealth` reconciler
  never requeues on a timer — every return is a bare `ctrl.Result{}`, so it is
  purely event-driven. A frozen child stops producing events, so the exact
  condition that should trigger a degrade is the condition guaranteeing no
  reconcile runs. A time-dependent verdict would require periodic requeue on
  every aggregate.
- **It would make `Result` a function of wall-clock time.** Today it is a pure
  fold over children's `status`. A time-dependent value stored in status is
  correct only at the instant it is written.
- **It is policy, not signal.** How stale is too stale, and what to do about it,
  belongs to the adopter.

**Corollary**: for the same reason, status MUST NOT carry a computed staleness
*judgment* (for example an "overdue children" count). Such a value decays between
reconciles exactly as a time-dependent verdict would. Status carries evidence
that stays true once written; the continuous "is it overdue *now*" evaluation
belongs to the alerting engine, which is the only component that re-evaluates
"now".

### D2 — Cadence is published for self-scheduling kinds; the aggregate is fixed at its derivation

Check kinds partition by whether they own a cadence:

| Kind | Has `spec.interval` | Freshness derives from |
| --- | --- | --- |
| `AddonCheck` | yes | its own cadence |
| `DNSCheck` | yes | its own cadence |
| `NodeCertificateCheck` | yes | its own cadence |
| `HealthCheck` | no | a mirrored external source |
| `ClusterHealth` | no | its aggregated children |

Cadence-awareness is only meaningful for the three kinds that own a cadence.
Therefore:

- The three self-scheduling kinds publish their **effective interval** so
  staleness can be expressed relative to cadence.
- The aggregate is fixed by correcting how its freshness is **derived** from its
  children, not by giving it a cadence it does not have.
- No check kind gains a staleness field in its own status (see D1 corollary).

`HealthCheck` is explicitly out of scope: it mirrors an external source whose
cadence Fathom does not know, so no meaningful cadence can be published for it.

**Consequence for CRD schema**: this feature is expected to add **no new CRD
fields**, which takes it off the critical path of
[#149](https://github.com/skaphos/fathom/issues/149) (the `v1alpha1` → `v1`
freeze). Metrics are not CRD schema and are not frozen by that promotion.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A frozen child cannot hide behind a healthy sibling (Priority: P1)

A platform operator watches a single `ClusterHealth` as the cluster's headline
verdict. One contributing check wedges — a stuck reconcile, a target held in
last-good preservation on transient lookup errors (#248), or a target in a
terminal state. Its recorded verdict freezes at `Fail`. Another contributing
check keeps running normally.

Today the aggregate promotes `Fail` indefinitely while reporting itself fully
fresh — in status *and* in the gauge — so the operator's staleness alert never
fires and nothing contradicts the frozen verdict. They cannot tell "this is
genuinely failing right now" from "this froze at `Fail` an hour ago and nobody
knows".

After this change the aggregate's freshness reflects the **stalest** contributing
evidence, so the operator can distinguish a live failure from a frozen one.

**Why this priority**: This is the reported defect, it affects the most prominent
resource Fathom exposes, and it silently disarms the one alert documented to
catch untrustworthy results.

**Independent Test**: Create a `ClusterHealth` selecting two `HealthCheck`
children; hold one child's observation time frozen while the other advances;
assert both the aggregate's status freshness and its exported gauge reflect the
frozen child. Delivers the core value on its own.

**Acceptance Scenarios**:

1. **Given** an aggregate with one live child and one child frozen at `Fail`,
   **When** the aggregate is reconciled, **Then** its reported freshness MUST
   reflect the frozen child, not the live one.
2. **Given** the same aggregate, **When** its exported staleness gauge is read,
   **Then** the gauge MUST agree with the status rather than reporting the
   aggregate as fresh.
3. **Given** an aggregate whose children are all current, **When** it is
   reconciled, **Then** it MUST report as fresh.
4. **Given** an aggregate containing a frozen child, **When** it is reconciled,
   **Then** its `Status.Result` MUST be exactly what the unchanged worst-of fold
   produces — staleness MUST NOT alter the verdict.

---

### User Story 2 - A healthy slow check does not poison its aggregate (Priority: P1)

An operator's `ClusterHealth` selects both a 5-minute addon check and a 1-hour
node-certificate check. Both are healthy and running exactly on their intended
cadence. The aggregate must treat both as current: a check observed 20 minutes
ago is healthy at an hourly cadence and badly overdue at a 5-minute one.

**Why this priority**: Equal to User Story 1. A fix satisfying only US1 (for
example, "use the oldest child" unconditionally) would ship a permanent false
positive on every mixed-cadence aggregate — worse than today's behavior. The two
stories together define correctness; neither alone does.

**Independent Test**: Build an aggregate mixing a short-interval and a
long-interval child, both observed within their own cadence, and assert no
staleness is reported.

**Acceptance Scenarios**:

1. **Given** an aggregate containing a healthy child on a long interval and a
   healthy child on a short interval, **When** it is reconciled, **Then** it MUST
   NOT be reported as stale.
2. **Given** two children of different cadences that are the same absolute age,
   **When** only one exceeds its own cadence allowance, **Then** only that one
   MUST contribute a staleness signal.

---

### User Story 3 - Staleness can be alerted on without guessing a threshold (Priority: P2)

An operator installs Fathom's shipped alerting rules across a fleet running
checks at several cadences. They want one staleness rule that is correct for
every check without hand-tuning a threshold per kind. Today the shipped rule
hardcodes 900 seconds and its own comment concedes it only suits the default
5-minute cadence, so it false-positives on every hourly check.

**Why this priority**: P2 because US1 and US2 restore correctness of the resource
contract, which is the reported defect. This story makes the fix usable from the
metrics surface, where most operators consume it, and retires a live false
positive.

**Independent Test**: With checks at two different cadences, a single
cadence-relative alerting expression identifies only the genuinely overdue check.

**Acceptance Scenarios**:

1. **Given** checks running at different cadences, **When** an operator writes a
   staleness rule, **Then** they MUST be able to express "overdue relative to
   this check's own cadence" without embedding a per-kind constant.
2. **Given** a check running normally at any cadence, **When** the shipped
   staleness rule is evaluated, **Then** it MUST NOT fire.

---

### Edge Cases

- **A child has never run.** A selected child with no observation time yet MUST
  be treated as the strongest staleness signal, not silently skipped —
  consistent with the existing "coerce empty verdict to `Unknown`" rule and with
  the gauge's 0 "never ran" sentinel.
- **No children match the selector.** The existing `NoMatches` path must be
  unaffected; an aggregate matching nothing must not report itself stale on top
  of already reporting not-ready. Today that path yields a nil observation time,
  which the gauge already renders as the 0 sentinel — that behavior is preserved.
- **Every child is frozen.** The aggregate reports the stalest evidence; it must
  not fall back to "fresh" through an empty-set default.
- **Clock skew / future timestamps.** A child reporting an observation time in
  the future must not make an aggregate look fresh indefinitely.
- **A child's cadence changes.** Staleness must follow the child's current
  effective cadence, including a runtime-clamped one (`clampCadence`), not a
  value captured earlier.
- **A large aggregate.** Per-child evaluation must stay bounded and introduce no
  additional per-child API reads.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The aggregate's freshness MUST be derived from the **stalest**
  contributing child, so a frozen child cannot be masked by a live sibling.
- **FR-002**: Staleness MUST be judged against each check's **own** effective
  cadence, never a single global threshold shared across cadences.
- **FR-003**: Status MUST carry only non-decaying evidence — a timestamp of the
  stalest contributing observation. Status MUST NOT carry a computed staleness
  judgment (such as an overdue count or a boolean) that would be correct only at
  the moment it was written. *(See D1 corollary.)*
- **FR-004**: A contributing child that has never produced an observation MUST
  be treated as the strongest staleness signal.
- **FR-005**: An observation time in the future MUST NOT make a child appear
  fresher than the present moment.
- **FR-006**: Each self-scheduling check kind (`AddonCheck`, `DNSCheck`,
  `NodeCertificateCheck`) MUST publish its effective cadence so consumers can
  express staleness relative to it. The published value MUST reflect the cadence
  actually in force, including runtime clamping.
- **FR-007**: The aggregate verdict fold (worst-of, with empty coerced to
  `Unknown`) MUST be unchanged by this feature.
- **FR-008**: The aggregate MUST continue to derive exclusively from
  `HealthCheck.status` and MUST NOT read `HealthReport` history.
- **FR-009**: The exported staleness gauge for the aggregate MUST agree with the
  aggregate's status freshness — the two MUST NOT be able to disagree.
- **FR-010**: All four descriptions of the aggregate's freshness field (the
  aggregation code, the gauge-emission comment, the CRD godoc, and
  `docs/guides/monitoring.md`) MUST be brought into agreement with the
  implemented behavior.
- **FR-011**: The shipped sample alerting rules MUST be updated so a single
  staleness rule is correct across differing cadences, removing the hardcoded
  per-kind constant.
- **FR-012**: Redefining the aggregate's existing freshness field MUST be called
  out for operators — it changes the value consumers read, even though it aligns
  the field with the guarantee already documented for it.
- **FR-013**: No check kind may gain a staleness field in its own status.
- **FR-014**: The overdue allowance (how far past its cadence a check may drift
  before counting as overdue) MUST be consistent across kinds and stated in
  operator-facing documentation.

### Key Entities

- **Aggregate health record**: the cluster-level roll-up. Carries a verdict, a
  freshness timestamp, and a per-child summary. This feature changes how the
  freshness timestamp is derived; it adds no fields.
- **Contributing child summary**: the per-child entry the aggregate already
  publishes (identity, verdict, summary, observation time) — already sufficient
  raw evidence, retained unchanged.
- **Effective cadence**: the interval a check actually runs at, after defaults
  and runtime clamping. Currently internal; this feature publishes it for
  self-scheduling kinds.
- **Overdue allowance**: the agreed multiple of a check's cadence beyond which it
  is considered overdue rather than merely late.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In an aggregate containing a frozen contributor and a healthy
  contributor, an operator can determine the evidence is stale from the
  aggregate alone, without opening any child resource.
- **SC-002**: An aggregate mixing checks whose cadences differ by more than 10×,
  all running on schedule, reports no staleness — zero false positives from
  cadence mixing.
- **SC-003**: A single staleness alerting expression is correct for every
  self-scheduling check kind, with no per-kind threshold constants left in the
  shipped rules.
- **SC-004**: All four statements describing the aggregate's freshness agree with
  its implemented behavior.
- **SC-005**: A regression test reproduces the reported failure — two
  contributors, one live and one frozen at `Fail` — and fails against the
  pre-change behavior.
- **SC-006**: The aggregate's verdict is byte-identical to today's for every
  input, confirming staleness never altered it.
- **SC-007**: Reconciliation cost for an aggregate stays proportional to its
  number of contributors, with no additional per-child API reads.
- **SC-008**: The feature adds zero new CRD fields, so it imposes no schema
  obligation on the `v1alpha1` → `v1` promotion.

## Assumptions

- **Overdue allowance defaults to 3× a check's cadence.** This matches the factor
  the shipped rule already uses (900s against a 5m interval) and the reasoning
  documented beside it, preserving existing behavior at the default cadence. To
  be confirmed at plan time.
- **Effective cadence is already computable.** Per-kind interval defaults and the
  `clampCadence` backstop exist in `internal/controller`; this feature surfaces
  the resulting value rather than introducing new scheduling behavior.
- **Freshness is expressed as a timestamp**, consistent with how it is
  represented today, rather than as a duration that would decay in stored status.
- **This feature does not change when checks run** — only what is reported about
  how current their results are.
- **`observeCheck` is the single seam for gauge emission**, so publishing cadence
  is a change at one helper plus its call sites rather than per-controller work.
- **No consumer depends on the current "newest child" reading.** Three of the
  four existing descriptions already promise stalest-wins behavior, so the
  redefinition moves the field toward what consumers were told, not away from it.

## Dependencies and Constraints

- **Constitution — `ClusterHealth` contract stability**: the aggregate derives
  only from `HealthCheck.status`, never from `HealthReport` history. Unchanged.
- **Constitution — bounded, idempotent reconciliation**: per-child staleness
  evaluation must not introduce unbounded work, and this feature must not add
  timer-based requeue to the aggregate (see D1).
- **Not on the #149 critical path.** Because the feature is expected to add no
  CRD fields, it does not have to land before the `v1alpha1` → `v1` freeze. If
  planning discovers a new field is unavoidable, that assumption inverts and the
  work becomes freeze-blocking — this is the single most important thing to
  re-check at plan time.
- **e2e is required.** This touches `internal/controller/*`, which per
  `AGENTS.md` mandates an e2e run before the PR is ready.
- **Generated artifacts are drift-gated.** `docs/reference/api.md` is generated;
  godoc changes must be regenerated via the documented task, never hand-edited.
- **Alert rules are build-validated.** Changes to the shipped rules must keep
  `task verify-alert-rules` passing.
- **Related work**:
  - [#279](https://github.com/skaphos/fathom/issues/279) (bounded opt-in CR label
    projection onto check gauges) touches the same gauge surface as FR-006;
    sequence to avoid conflicting changes.
  - [#262](https://github.com/skaphos/fathom/issues/262) (remove `spec.paused`)
    removes one cause of frozen children but not the class — neither blocks nor
    substitutes for this work.
  - [#248](https://github.com/skaphos/fathom/issues/248) (last-good preservation
    on transient lookup errors) is a deliberate, ongoing source of frozen
    children; this feature must surface it, not suppress it.

## Out of Scope

- Changing the aggregate verdict, or any verdict, in response to staleness (D1).
- Adding staleness fields to individual check kinds' status (D2, FR-013).
- Publishing a cadence for `HealthCheck`, whose mirrored source cadence is
  unknown to Fathom (D2).
- Changing when checks run, or their scheduling behavior.
- Adding timer-based requeue to the `ClusterHealth` reconciler.
- Auto-remediating or auto-clearing frozen children.
- Promoting CRDs to `v1` (tracked separately in #149).
