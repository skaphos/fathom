# Contract: Metrics Surface

**Feature**: 007-clusterhealth-staleness-semantics

The metrics surface is a public contract — adopters write alert rules against it.

## New gauge

```
fathom_check_interval_seconds{kind, name, namespace}
```

| Property | Value |
| --- | --- |
| Type | Gauge |
| Unit | Seconds (Prometheus base unit, matching the existing `*_seconds` gauges) |
| Labels | `kind`, `name`, `namespace` — **identical** to `fathom_check_last_run_timestamp_seconds`, so the two join without relabeling |
| Meaning | The cadence the check is currently expected to run at, after per-resource override and floor clamping |
| Absent when | The cadence cannot be resolved (e.g. a `HealthCheck` whose `CheckRef` kind is not yet supported). Absent rather than zero, so a join yields no result instead of a wrong one |

**Lifecycle**: written wherever the existing check gauges are written, and
removed wherever they are removed. A deleted check must not leave an orphaned
cadence series.

## Unchanged

`fathom_check_result` and `fathom_check_last_run_timestamp_seconds` keep their
names, labels, and semantics. `fathom_check_last_run_timestamp_seconds` for a
`ClusterHealth` now carries the **stalest** contributing observation rather than
the newest — a value change, not a contract change, and the same inversion
described in [clusterhealth-status.md](./clusterhealth-status.md).

The `0` "never evaluated" sentinel is preserved and now also covers an aggregate
with a never-observed child.

## Alerting contract

The shipped staleness rule becomes cadence-relative:

```
time() - fathom_check_last_run_timestamp_seconds
  > <multiplier> * fathom_check_interval_seconds
```

| Aspect | Contract |
| --- | --- |
| Multiplier | Documented editable constant in the opt-in kustomize rule, default **3**. Not operator runtime config (R5). The Helm chart does not ship a `PrometheusRule`, so no chart value applies yet |
| Correctness | One expression is correct for every cadence — no per-kind constants remain |
| Never-ran | Still caught: the `0` sentinel makes the left side enormous |
| Unresolvable cadence | The join drops the series; such a check is not alerted on by this rule, and this is documented rather than silently assumed |

**Removed**: the hardcoded `> 900` threshold and its "tune to yours" caveat,
which false-positives on every check slower than a 5-minute cadence.

## Compatibility

| Change | Breaking? |
| --- | --- |
| New gauge added | No — additive |
| `ClusterHealth` last-run value inverted to stalest | **Yes, behaviorally.** Same series, different value. Covered by the breaking-change marker and ADR |
| Shipped rule expression rewritten | No for adopters using their own rules; adopters using the shipped rule get the fix |
