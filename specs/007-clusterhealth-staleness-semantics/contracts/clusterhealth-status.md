# Contract: `ClusterHealth.Status`

**Feature**: 007-clusterhealth-staleness-semantics

`ClusterHealth` is the headline external contract. This documents exactly what
changes for a consumer.

## `status.observedAt` — meaning inverted

| | Before | After |
| --- | --- | --- |
| Value | Newest contributing observation | **Stalest** contributing observation |
| Type | `*metav1.Time` | unchanged |
| Optionality | optional | unchanged |
| Guarantee | *(none held — three of four descriptions promised the new behavior)* | A stale contributor produces a stale aggregate |

**This is a breaking behavioral change with no schema signal.** The field keeps
its name, type, and optionality, so the CRD compatibility gate cannot detect it.
It ships with a `BREAKING CHANGE` marker and ADR-0005 as compensating controls.

**Who is affected**: a consumer reading `observedAt` as "when did anything last
happen". After this change it answers "how far back does my least-current
evidence go" — which is what the godoc, the gauge comment, and the monitoring
guide all already claimed it answered.

### New guarantees

1. An aggregate folding a frozen contributor's verdict cannot report zero
   staleness.
2. `nil` means at least one selected contributor has never been observed.
3. The value never exceeds the present moment, regardless of contributor clock
   skew.

## `status.children[]` — bounded and ordered

| Aspect | Contract |
| --- | --- |
| Maximum size | Capped (previously unbounded) |
| Ordering | Deterministic: severity descending, then staleness descending, then namespace/name |
| Truncation | Only when the selected set exceeds the cap. The retained entries are the worst and stalest — never an arbitrary slice |
| Entry shape | Unchanged |

**Consumer-visible consequence**: `children` may no longer enumerate every
selected check. `matchedCount` remains the **full** count, so
`matchedCount > len(children)` is the signal that truncation occurred.

A consumer that treated `children` as exhaustive must switch to `matchedCount`
for totals. Consumers that read it for display or for locating failures are
unaffected — those entries sort first by construction.

## `status.result` — explicitly unchanged

The worst-of fold (with empty coerced to `Unknown`) is byte-identical for every
input. Staleness never alters the verdict.

This is deliberate: `Unknown` ranks **below** `Fail` in the severity ladder, so
degrading a stale `Fail` would *silence* the shipped
`result=~"Fail|Error"` alert — converting "failing and now untrustworthy" into
"not failing".

## `status.matchedCount` — contract strengthened

Was the number of selected checks. Remains exactly that, and is now explicitly
guaranteed to be the **pre-truncation** total. It is the mechanism by which
truncation is observable, which is why no new field is required.

## Unchanged

- Derivation exclusively from `HealthCheck.status`; `HealthReport` history is
  never read.
- `conditions[]` — no staleness condition is added. A stored "is stale" condition
  would be a judgment that decays between reconciles, which the aggregate's
  event-driven (non-timer) reconcile cannot keep true.
- The `NoMatches` path.
