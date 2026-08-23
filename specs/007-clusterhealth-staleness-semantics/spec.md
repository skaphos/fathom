# Feature Specification: Cadence-Aware Staleness Semantics for ClusterHealth

**Feature Branch**: `feature/277-clusterhealth-staleness`

**Created**: 2026-08-23

**Status**: Draft — scope decisions resolved, ready for `/speckit-plan`

**Input**: GitHub issue [#277](https://github.com/skaphos/fathom/issues/277) — "ClusterHealth ObservedAt takes the newest child, masking a stuck child promoting Fail"

## Problem

`ClusterHealth` is the headline "one verdict for the cluster" resource. Its
staleness signal is currently the **newest** child's observation time, while its
verdict is the **worst** of all children. Those two folds disagree, and the
disagreement is silent.

For an aggregate with one live child and one frozen child:

- the frozen child's stale `Fail` still propagates into the aggregate verdict;
- but the aggregate reports zero staleness, because a live sibling
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
permanent staleness. Staleness must be judged against each check's **own**
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

Three decisions were made before planning; they are recorded here because they
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

| Kind | Has `spec.interval` | Staleness judged against |
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
- The aggregate is fixed by correcting how its staleness signal is **derived** from its
  children, not by giving it a cadence it does not have.
- No check kind gains a staleness field in its own status (see D1 corollary).

`HealthCheck` is explicitly out of scope: it mirrors an external source whose
cadence Fathom does not know, so no meaningful cadence can be published for it.

**Consequence for CRD schema**: this feature adds **no new CRD fields**. It does,
however, add a size constraint to an existing status list (FR-016, resolved in
Clarifications), which *is* a schema change. That places the work **on** the
critical path of [#149](https://github.com/skaphos/fathom/issues/149) (the
`v1alpha1` → `v1` freeze): the constraint must land before the freeze, because
tightening validation on a GA version afterwards is forbidden by the CRD
versioning standard.

Metrics are not CRD schema and remain unfrozen by that promotion, so the cadence
half of this work (FR-006) carries no freeze deadline.

### D3 — "Staleness" is the canonical term; "freshness" is not used

The signal is named and described in terms of **staleness** throughout: the
canonical noun is the **staleness signal**, and the value backing it is the
**stalest contributing observation**.

Rationale: health is a risk surface. The operator's question is never "how fresh
is this?" — it is "how stale is this, and can I still trust it?" Framing the
signal positively inverts the thing being watched for and reads as reassurance
where a warning belongs. It is also the framing every consumer already uses: the
alert is `FathomCheckStale`, and the guarantee the docs promise is that "a stale
source reads as a stale wrapper".

This applies to the shipped artifacts as well as this document — the CRD godoc,
the gauge-emission comment, and the monitoring guide adopt the same term when
FR-010 brings them into agreement. The word "freshness" is never used as a term
in this spec — it survives only inside verbatim quotations of the current,
incorrect text, and in this decision where the word itself is the subject.

## Clarifications

### Session 2026-08-23

- Q: Should the overdue allowance be a fixed multiplier, or operator-tunable? → A: Configurable via the existing cobra/viper options, defaulting to 3× the check's cadence.
- Q: How should the redefinition of the existing staleness field be classified and communicated? → A: As a breaking change (`BREAKING CHANGE` footer) plus an ADR recording the semantic redefinition.
- Q: Should the aggregate's child-summary list be bounded, or is its unbounded size out of scope? → A: Bound it with `MaxItems`, paired with defined overflow behavior so the cap cannot wedge reconciliation.
- Q: Is "freshness" or "staleness" the canonical term for the signal? → A: Staleness. Health is a risk surface — the operator asks how stale the signal is, not how fresh (D3).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A frozen child cannot hide behind a healthy sibling (Priority: P1)

A platform operator watches a single `ClusterHealth` as the cluster's headline
verdict. One contributing check wedges — a stuck reconcile, a target held in
last-good preservation on transient lookup errors (#248), or a target in a
terminal state. Its recorded verdict freezes at `Fail`. Another contributing
check keeps running normally.

Today the aggregate promotes `Fail` indefinitely while reporting itself fully
showing zero staleness — in status *and* in the gauge — so the operator's staleness alert never
fires and nothing contradicts the frozen verdict. They cannot tell "this is
genuinely failing right now" from "this froze at `Fail` an hour ago and nobody
knows".

After this change the aggregate's staleness signal reflects the **stalest** contributing
evidence, so the operator can distinguish a live failure from a frozen one.

**Why this priority**: This is the reported defect, it affects the most prominent
resource Fathom exposes, and it silently disarms the one alert documented to
catch untrustworthy results.

**Independent Test**: Create a `ClusterHealth` selecting two `HealthCheck`
children; hold one child's observation time frozen while the other advances;
assert both the aggregate's status staleness signal and its exported gauge reflect the
frozen child. Delivers the core value on its own.

**Acceptance Scenarios**:

1. **Given** an aggregate with one live child and one child frozen at `Fail`,
   **When** the aggregate is reconciled, **Then** its reported staleness signal MUST
   reflect the frozen child, not the live one.
2. **Given** the same aggregate, **When** its exported staleness gauge is read,
   **Then** the gauge MUST agree with the status rather than reporting the
   aggregate as current.
3. **Given** an aggregate whose children are all current, **When** it is
   reconciled, **Then** it MUST report no staleness.
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
  not fall back to "no staleness" through an empty-set default.
- **Clock skew / future timestamps.** A child reporting an observation time in
  the future must not let an aggregate understate its staleness indefinitely.
- **A child's cadence changes.** Staleness must follow the child's current
  effective cadence, including a runtime-clamped one (`clampCadence`), not a
  value captured earlier.
- **A large aggregate.** Per-child evaluation must stay bounded and introduce no
  additional per-child API reads.
- **An aggregate exceeding the child-list cap.** The verdict and staleness signal
  must still be computed from every selected child; only the published per-child
  detail is truncated, and the truncation must be visible (FR-017, FR-018).
- **An aggregate stored before the cap existed.** A pre-existing object holding
  more children than the new maximum must keep reconciling rather than failing
  validation on its next status write.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The aggregate's staleness signal MUST be derived from the **stalest**
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
  less stale than the present moment.
- **FR-006**: Each self-scheduling check kind (`AddonCheck`, `DNSCheck`,
  `NodeCertificateCheck`) MUST publish its effective cadence so consumers can
  express staleness relative to it. The published value MUST reflect the cadence
  actually in force, including runtime clamping.
- **FR-007**: The aggregate verdict fold (worst-of, with empty coerced to
  `Unknown`) MUST be unchanged by this feature.
- **FR-008**: The aggregate MUST continue to derive exclusively from
  `HealthCheck.status` and MUST NOT read `HealthReport` history.
- **FR-009**: The exported staleness gauge for the aggregate MUST agree with the
  aggregate's status staleness signal — the two MUST NOT be able to disagree.
- **FR-010**: All four descriptions of the aggregate's staleness field (the
  aggregation code, the gauge-emission comment, the CRD godoc, and
  `docs/guides/monitoring.md`) MUST be brought into agreement with the
  implemented behavior, and MUST describe it in **staleness** terms per D3 —
  agreeing on the wrong framing would satisfy the letter of this requirement and
  miss its point.
- **FR-011**: The shipped sample alerting rules MUST be updated so a single
  staleness rule is correct across differing cadences, removing the hardcoded
  per-kind constant.
- **FR-012**: Redefining the aggregate's existing staleness field MUST be
  released as a **breaking change** (a `BREAKING CHANGE` footer on the landing
  commit), and MUST be recorded in an **ADR** capturing the semantic
  redefinition and why the alternative — adding a parallel field — was rejected.
  Rationale: the field keeps its name and type, so the schema-compatibility gate
  cannot detect the change; the release signal and the ADR are the only warnings
  a consumer receives.
- **FR-013**: No check kind may gain a staleness field in its own status.
- **FR-014**: The overdue allowance (how far past its cadence a check may drift
  before counting as overdue) MUST be operator-configurable cluster-wide,
  defaulting to **3× the check's cadence**, and MUST be stated in
  operator-facing documentation. It MUST apply consistently across kinds — it is
  a single cluster-wide tolerance, not a per-kind or per-resource value.
- **FR-015**: The overdue allowance MUST be configured through the existing
  configuration model, so the flag, `FATHOM_*` environment variable, config-file
  key, and default stay in sync. It MUST NOT introduce a CRD field.
- **FR-016**: The aggregate's child-summary list MUST carry an explicit maximum
  size, so a selector matching an unbounded population cannot grow the stored
  object without limit.
- **FR-017**: Exceeding that maximum MUST NOT fail the status write or wedge
  reconciliation. When the selected population exceeds the cap, the aggregate
  MUST still publish a verdict and a staleness signal derived from **all**
  selected children, and MUST truncate only the per-child detail list.
- **FR-018**: When the per-child list is truncated, the aggregate MUST make the
  truncation observable — the full selected count MUST remain readable, so an
  operator can tell "50 children, all shown" from "500 children, 50 shown".
- **FR-019**: Truncation MUST be deterministic and MUST prioritize the children
  an operator needs most — those contributing the worst verdict and the stalest
  evidence — rather than truncating on an arbitrary boundary that could hide the
  failing child.

### Key Entities

- **Aggregate health record**: the cluster-level roll-up. Carries a verdict, a
  staleness timestamp, and a per-child summary. This feature changes how the
  staleness timestamp is derived; it adds no fields.
- **Contributing child summary**: the per-child entry the aggregate already
  publishes (identity, verdict, summary, observation time). Its content is
  unchanged; the list gains a maximum size and a defined truncation rule.
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
- **SC-004**: All four statements describing the aggregate's staleness signal agree with
  its implemented behavior.
- **SC-005**: A regression test reproduces the reported failure — two
  contributors, one live and one frozen at `Fail` — and fails against the
  pre-change behavior.
- **SC-006**: The aggregate's verdict is byte-identical to today's for every
  input, confirming staleness never altered it.
- **SC-007**: Reconciliation cost for an aggregate stays proportional to its
  number of contributors, with no additional per-child API reads.
- **SC-008**: An aggregate whose selector matches a population larger than the
  child-list cap still reports a correct verdict and a correct staleness signal,
  and its status write succeeds — the cap never wedges reconciliation.
- **SC-009**: An operator upgrading across this change can discover the altered
  meaning of the staleness field from the release notes alone, without reading
  the diff — verified by the presence of the breaking-change marker and the ADR.

## Assumptions

- **Overdue allowance defaults to 3× a check's cadence** and is operator-tunable
  (resolved in Clarifications). The default matches the factor the shipped rule
  already uses (900s against a 5m interval) and the reasoning documented beside
  it, so existing behavior at the default cadence is preserved. Making it tunable
  rather than fixed follows the same principle as D1: how much drift is
  acceptable is adopter policy, not something Fathom should decide for them.
- **Effective cadence is already computable.** Per-kind interval defaults and the
  `clampCadence` backstop exist in `internal/controller`; this feature surfaces
  the resulting value rather than introducing new scheduling behavior.
- **Staleness is expressed as a timestamp**, consistent with how it is
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
- **Constitution — configuration model**: the overdue allowance (FR-015) enters
  through the established precedence chain (flag → `FATHOM_*` env var → config
  file → default) by extending the options table, so all four stay in sync.
- **On the #149 critical path.** The child-list size constraint (FR-016) is a
  schema change and must land before the `v1alpha1` → `v1` freeze; tightening
  validation on a GA version afterwards is forbidden. This is the single most
  important scheduling constraint on the work.
- **The size constraint narrows an existing field, so the CRD compatibility gate
  will flag it.** Adding a maximum to a previously unbounded list shrinks the set
  of accepted objects, which the gate reports as an incompatible change. It
  therefore needs a sanctioned entry in the compatibility allowlist carrying the
  justification, rather than being worked around.
- **Objects already exceeding the cap must keep reconciling.** Aggregates stored
  before the constraint existed may hold more children than the new maximum;
  FR-017 exists so those objects continue to publish a verdict and a staleness signal
  rather than failing validation on their next status write.
- **e2e is required.** This touches `internal/controller/*`, which per
  `AGENTS.md` mandates an e2e run before the PR is ready.
- **Generated artifacts are drift-gated.** `docs/reference/api.md` is generated;
  godoc changes must be regenerated via the documented task, never hand-edited.
- **No automated gate covers this change.** The CRD schema-compatibility check
  diffs schemas; the redefined field keeps its name, type, and optionality, so
  the check passes regardless. The breaking-change marker and the ADR (FR-012)
  are deliberate compensating controls, not ceremony.
- **Release mechanics**: the repository bumps the minor version for breaking
  changes while below 1.0, so the `BREAKING CHANGE` footer lands this as a minor
  release rather than forcing a major — the same milestone this work targets.
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
