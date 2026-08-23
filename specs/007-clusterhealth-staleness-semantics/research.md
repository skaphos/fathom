# Phase 0 Research: Cadence-Aware Staleness Semantics for ClusterHealth

**Feature**: 007-clusterhealth-staleness-semantics | **Date**: 2026-08-23
**Spec**: [spec.md](./spec.md)

All findings verified against `main` at `74f524f`.

---

## Summary of what changed as a result of research

Two spec statements did not survive contact with the code. Both are corrected
below and reflected in [plan.md](./plan.md); the spec should be amended to match.

| # | Spec statement | Finding |
| --- | --- | --- |
| C1 | D2: `HealthCheck` is out of scope because "it mirrors an external source whose cadence Fathom does not know" | **False.** `HealthCheck.Spec.CheckRef` references a Fathom-native check whose cadence Fathom owns. Excluding it does not merely leave a gap — it makes the aggregate's cadence-awareness impossible, breaking User Story 2. |
| C2 | FR-015: the overdue allowance is configured through the operator's cobra/viper options | **Wrong home.** Per D1 the operator never evaluates staleness, so an operator flag would be dead configuration. The multiplier's only consumer is the alert rule. |

---

## R1 — Effective cadence already exists as three parallel resolvers

Each self-scheduling kind has one function of identical shape, all routing
through the shared `clampCadence` backstop:

| Kind | Resolver | Default |
| --- | --- | --- |
| `AddonCheck` | `addonCheckInterval` (`addoncheck_controller.go:554`) | 5m |
| `DNSCheck` | `dnsCheckInterval` (`dnscheck_plan.go:162`) | 1m |
| `NodeCertificateCheck` | `nodeCertInterval` (`nodecertificatecheck_helpers.go:114`) | 1h |

**Decision**: reuse these as-is; publish their return value. No new cadence
logic, and the published value automatically reflects both per-resource override
and runtime clamping, satisfying FR-006's "cadence actually in force".

**Alternatives considered**: a single generic resolver over an interface. Rejected —
it would refactor three working controllers for no behavioral gain, and each
resolver's default is genuinely kind-specific (1m vs 5m vs 1h).

---

## R2 — `observeCheck` is a single seam; series deletion is a matching obligation

`internal/controller/observe.go:60` is the one place gauges are written; all five
reconcilers `defer` exactly one call to it. `metrics.ObserveCheck` sets the result
one-hot and the last-run timestamp.

**Decision**: extend `observeCheck` / `metrics.ObserveCheck` with the effective
cadence, so all kinds are covered by one change at five call sites.

**Obligation found**: `metrics.DeleteCheckSeries` (`metrics.go:188`) drops
`CheckResult` and `CheckLastRunTimestamp` when a check disappears. A new gauge
**must** be dropped there too, or a deleted check leaves an orphaned cadence
series asserting a cadence for a resource that no longer exists.

---

## R3 — The stalest timestamp needs no timer, which is what makes D1 implementable

The aggregate never requeues on a timer (every return in
`clusterhealth_controller.go` is a bare `ctrl.Result{}`). It re-enqueues via a
watch on `HealthCheck` with `ResourceVersionChangedPredicate`, so any child status
change re-runs the aggregation.

The concern this raises — "if every child freezes, nothing re-reconciles" — does
**not** apply to a stalest *timestamp*, because the timestamp is a fact about when
evidence was last seen. Once written it never needs updating; the alerting engine
computes `time() - timestamp` continuously.

**This is the empirical confirmation of the D1 corollary.** A stored *judgment*
(an overdue count, a stale boolean) would need a timer to stay true, and would be
silently wrong precisely when everything is frozen — the exact scenario the
feature exists to detect. A stored *timestamp* needs nothing.

**Decision**: no requeue changes. The constitution's bounded-reconciliation
constraint is satisfied trivially.

---

## R4 — C1: the aggregate cannot be cadence-aware without `HealthCheck`

`ClusterHealth` selects `HealthCheck`s. `HealthCheck` has no `spec.interval` — but
it is **not** an opaque external mirror. `HealthCheck.Spec.CheckRef` is a
`CheckTargetRef`, documented as referencing:

> "a specialized check resource (AddonCheck, DNSCheck, NodeHealthCheck,
> NodeCertificateCheck, ReachabilityCheck) whose status a HealthCheck mirrors and
> surfaces for ClusterHealth aggregation"

Every one of those is a Fathom CR whose cadence Fathom owns. So the cadence chain
is:

```
ClusterHealth  →  HealthCheck  →  source check (owns spec.interval)
```

D2 excluded `HealthCheck` on the grounds that its source cadence is unknowable.
That is factually wrong, and the consequence is structural: **with `HealthCheck`
excluded, the aggregate has no path to any cadence at all**, so User Story 2 ("a
healthy slow check does not poison its aggregate") cannot be satisfied.

**Decision**: `HealthCheck` surfaces its source's effective cadence. The aggregate's
own effective cadence is then the **maximum** across its children — an aggregate
can only be as current as its slowest legitimate contributor.

**Alternatives considered**:

- *Minimum child cadence*: wrong direction — it would make every mixed aggregate
  permanently overdue, reintroducing the US2 failure.
- *Leave the aggregate with no cadence and have adopters hand-tune per aggregate*:
  rejected; it recreates the hardcoded-threshold problem this feature exists to
  remove.

**Known tradeoff, accepted**: with a max-cadence rule, a frozen fast child inside
an aggregate that also holds a slow child is detected on the slow child's
timescale (e.g. 3h rather than 15m). This trades detection latency for the
elimination of false positives. It is strictly better than today, where such a
child is never detected at all. Per-check alerts still fire on the fast child's
own timescale — the aggregate is a roll-up, not the only signal.

**Dependency noted**: [#267](https://github.com/skaphos/fathom/issues/267)
generalizes `CheckTargetRef` beyond `AddonCheck`. The cadence lookup must handle
whichever kinds `CheckRef` supports and degrade gracefully (no cadence published)
for a kind it cannot resolve, rather than failing the reconcile.

---

## R5 — C2: the overdue multiplier does not belong in operator config

FR-015 places the overdue allowance in the operator's cobra/viper options. But
D1 establishes that the operator never evaluates staleness — the alerting engine
does. An operator flag would therefore be read by nothing.

The multiplier's only consumer is the alert expression:

```
time() - fathom_check_last_run_timestamp_seconds
  > <multiplier> * fathom_check_interval_seconds
```

**Decision**: the multiplier is a **packaging** value, not runtime config. It lives
where the shipped rules are rendered — the Helm chart value and the kustomize
`prometheus-rule` component — defaulting to 3. Adopters override it there, or
write their own rule.

**Consequence**: FR-015 as written should be struck. No new operator flag, no
`Options` change, no `bindings()` row. This *reduces* scope.

**Alternatives considered**:

- *Operator flag that renders nothing*: rejected as dead configuration.
- *Have the operator emit a per-check "overdue" gauge using the multiplier*:
  rejected — that is exactly the decaying computed judgment D1's corollary
  forbids, and it would require the timer requeue R3 avoids.

---

## R6 — Staleness derivation and the never-observed case

**Decision**: `Status.ObservedAt` becomes the **oldest** non-nil child
`SourceObservedAt`. A selected child with **no** observation dominates outright,
yielding a nil aggregate `ObservedAt`.

That nil already renders as the gauge's `0` sentinel, which the existing shipped
rule documents as making "a check that never executed fire this alert too — no
`absent()` gymnastics needed". So FR-004 ("never-observed is the strongest
staleness signal") requires no new mechanism — it falls out of the existing
convention.

**Clock skew (FR-005)**: a child timestamp in the future is clamped to the
present for comparison purposes, so it can never make an aggregate look more
current than now. The stored child summary keeps the raw value; only the
aggregate's derivation clamps.

---

## R7 — Bounding `Children[]` is an incompatible narrowing

`Children` carries no `MaxItems` today, while `Namespaces` and
`ExcludedNamespaces` both cap at 50.

**Decision**: cap `Children` and truncate deterministically, ordered by severity
then staleness, so the retained entries are the ones an operator needs (FR-019).
`MatchedCount` already exists and is set from the full selected set, so FR-018's
"full count remains readable" needs no new field — it needs a guarantee that
`MatchedCount` continues to reflect the pre-truncation total.

**Verdict and staleness are computed before truncation** (FR-017), so neither is
affected by the cap.

**Gate impact**: adding a maximum to a previously unbounded list shrinks the
accepted set, which the CRD compatibility gate reports as an incompatible change.
It needs a sanctioned entry in `.crd-compat-allowlist.yaml` carrying the
justification — the repo's existing mechanism for exactly this.

**Migration**: objects stored before the cap may hold more children than the new
maximum. Because the controller rebuilds `Children` from scratch each reconcile
and now truncates, the next status write brings such an object into compliance
rather than failing validation.

**Open risk flagged to the user**: this is the only part of the feature that
touches CRD schema, and it is what places the work on the #149 freeze critical
path. It remains separable into its own issue if that tradeoff is unattractive.

---

## R8 — Metric shape

**Decision**: one new gauge, matching the existing label set exactly so it joins
cleanly in PromQL:

```
fathom_check_interval_seconds{kind, name, namespace}
```

Seconds, matching Prometheus base-unit convention and the existing
`..._seconds` gauges. Published only for a check whose cadence can be resolved;
absent otherwise, so a join simply yields no result rather than a wrong one.

**Alternatives considered**: encoding cadence as a label on the existing last-run
gauge. Rejected — cadence changes would churn series identity, and label
cardinality would grow with every distinct interval value.
