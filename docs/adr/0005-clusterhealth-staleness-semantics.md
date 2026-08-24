<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# 5. ClusterHealth staleness is the stalest child, and is a signal not a verdict

- **Status**: accepted
- **Date**: 2026-08-23
- **Deciders**: Shawn Stratton

## Context and Problem Statement

`ClusterHealth.Status.ObservedAt` took the **newest** contributing child's
observation, while `Status.Result` took the **worst** verdict across the same
children. The two folds disagreed, and the disagreement was silent.

For an aggregate with one live child and one frozen child:

- the frozen child's stale `Fail` still propagated into the aggregate verdict;
- but the aggregate reported itself perfectly current, because a live sibling
  supplied the newest timestamp.

The documented mechanism for catching "the last recorded result can no longer be
trusted" is a staleness alert. At the aggregate layer — the headline "one verdict
for the cluster" resource — that mechanism was defeated by a single healthy
sibling. The same field feeds `fathom_check_last_run_timestamp_seconds`, so the
defect also reached the surface operators actually alert on.

Four descriptions of the field disagreed with each other. Only the first
described what the code did:

| Source | Claim |
| --- | --- |
| aggregation code | the **newest** child's observation |
| gauge-emission comment | "stale children … read as a stale roll-up" |
| CRD godoc | "when the **aggregator last refreshed** this status" |
| `docs/guides/monitoring.md` | "the **freshest** of its children — so a stale source reads as a stale wrapper" |

Three of the four already promised the behaviour that did not hold.

## Decision

**1. `ObservedAt` is the stalest contributing observation.** A child that has
never been evaluated outranks every timestamp and yields `nil`, which the gauge
already renders as its `0` never-ran sentinel. A child clock running fast is
clamped to the present, so it cannot make an aggregate look indefinitely current.

**2. Staleness never changes a verdict.** `Status.Result` remains the worst-of
fold, untouched.

**3. Status carries evidence, not judgement.** No stored "is overdue" count or
boolean. The continuous "is it overdue *now*" evaluation belongs to the alerting
engine.

**4. Cadence is published as a metric** (`fathom_check_interval_seconds`) so
staleness can be expressed relative to how often a check is meant to run.

## Rationale

### Why not degrade a stale `Fail` to `Unknown`

It would **silence** the alarm rather than raise one. The severity ladder is
`Pass(1) < Skipped(2) < Warn(3) < Unknown(4) < Fail(5) < Error(6)`, so `Unknown`
is *less* severe than `Fail`. Degrading would stop the shipped
`fathom_check_result{result=~"Fail|Error"} == 1` rule from firing, converting
"failing, and now untrustworthy" into "not failing". A frozen `Fail` is still the
best evidence available; nothing has contradicted it.

It also could not work. The aggregate never requeues on a timer — every return is
a bare `ctrl.Result{}` — and re-enqueues only through its `HealthCheck` watch. A
frozen child produces no events, so the exact condition that should trigger the
degrade is the condition guaranteeing no reconcile runs.

Finally, it is policy. How stale is too stale, and what to do about it, belongs
to the adopter.

### Why status holds a timestamp rather than a judgement

The same argument that rules out a time-dependent verdict rules out a stored
staleness judgement: both are correct only at the instant they are written, and
both go wrong precisely when every child is frozen and nothing re-enqueues the
aggregate. A timestamp is a fact about when evidence was last seen — it never
needs recomputing, which is why this change requires no timer requeue at all.

### Why not the oldest child, unconditionally

Cadences differ by more than 10× (5m `AddonCheck` vs 1h `NodeCertificateCheck`).
A healthy hourly child would drag every aggregate containing it into permanent
staleness. Staleness has to be judged against each check's own cadence, which is
why the cadence is published. The aggregate's own cadence is the **slowest** of
its children: a roll-up can only be as current as its least frequently refreshed
contributor.

## Consequences

### This is a breaking behavioural change with no schema signal

The field keeps its name, type, and optionality, so the CRD compatibility gate
diffs schemas and sees nothing. **No automated check can detect a semantic
redefinition.** The `BREAKING CHANGE` marker on the landing commit and this ADR
are the compensating controls, deliberately chosen because nothing else would
warn a consumer.

A consumer reading `ObservedAt` as "when did anything last happen" is affected.
After this change it answers "how far back does my least-current evidence go" —
which is what three of the four existing descriptions already claimed.

### Alternative rejected: add a parallel field

Keeping `ObservedAt` as the newest and adding a separate stalest field avoids the
break. Rejected because:

- it adds CRD surface immediately before the `v1alpha1` → `v1` freeze (#149);
- it leaves two fields where one is correct, and the wrong one keeps feeding the
  staleness gauge;
- the documented guarantee was always stalest-wins, so the redefinition moves the
  field toward what consumers were told, not away from it.

### Accepted tradeoff: detection latency in mixed aggregates

With a slowest-child cadence, a frozen *fast* child inside an aggregate that also
holds a slow child is detected on the slow child's timescale. This trades
detection latency for the elimination of false positives, and is strictly better
than the previous behaviour where such a child was never detected at all.
Per-check alerts still fire on the fast child's own cadence; the aggregate is a
roll-up, not the only signal.

### Terminology

The signal is named in terms of **staleness** throughout. Health is a risk
surface: the operator's question is "how stale is this, can I still trust it?",
never "how fresh is this?". The positive framing inverts the thing being watched
for and reads as reassurance where a warning belongs.

## References

- Issue [#277](https://github.com/skaphos/fathom/issues/277)
- Specification: `specs/007-clusterhealth-staleness-semantics/`
- [ADR-0004](0004-healthcheck-as-wrapper.md) — why aggregates read `HealthCheck`
  rather than the specialized kinds, which is why the wrapper must surface its
  source's cadence
