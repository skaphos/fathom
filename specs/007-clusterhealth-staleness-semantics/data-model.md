# Phase 1 Data Model: Cadence-Aware Staleness Semantics

**Feature**: 007-clusterhealth-staleness-semantics | **Date**: 2026-08-23

No new entities and no new fields. One field changes meaning, one list gains a
bound, and one derived quantity becomes externally visible as a metric.

---

## Entity: `ClusterHealth.Status`

| Field | Change | Notes |
| --- | --- | --- |
| `result` | **unchanged** | Worst-of fold with empty coerced to `Unknown`. FR-007 forbids staleness from touching it. |
| `matchedCount` | **contract strengthened** | Must remain the count of the **full** selected set, even when `children` is truncated. This is what makes truncation observable (FR-018) without a new field. |
| `children[]` | **bounded + ordered** | Gains a maximum size and a deterministic truncation rule. Entry shape unchanged. |
| `observedAt` | **meaning inverted** | Was the newest contributing observation; becomes the **stalest**. Same name, type, optionality — which is why no schema gate can detect the change. |
| `conditions[]` | **unchanged** | No staleness condition is added (D1: staleness is not a verdict, and a condition would be a decaying judgment). |

### `observedAt` derivation

| Input | Result |
| --- | --- |
| No children selected | `nil` (existing `NoMatches` path, unchanged) |
| All children have an observation | The **oldest** of them |
| Any child has **no** observation | `nil` — a never-observed child dominates (FR-004) |
| A child reports a **future** observation | Clamped to now for comparison; the raw value is preserved in that child's summary entry (FR-005) |

`nil` is already rendered by the gauge as the `0` "never ran" sentinel, which the
shipped rule documents as firing the staleness alert without `absent()`. FR-004
therefore needs no new mechanism.

### `children[]` truncation

Ordering, applied only when the selected set exceeds the cap:

1. **Severity descending** — worst verdicts first, using the existing
   `Severity()` ranking (`Error(6) > Fail(5) > Unknown(4) > Warn(3) > Skipped(2) > Pass(1)`).
2. **Staleness descending** — within equal severity, oldest observation first;
   a never-observed child sorts as maximally stale.
3. **Namespace then name** — for a stable, deterministic result.

Rationale: an arbitrary alphabetical truncation could hide the single failing or
frozen child, which is the one entry the operator needs (FR-019).

**Invariant**: `result` and `observedAt` are computed over the **full** selected
set *before* truncation, so the cap can never change the verdict or the staleness
signal (FR-017).

**Validation note**: the cap narrows a previously unbounded list. The CRD
compatibility gate reports this as incompatible by construction; it is sanctioned
via `.crd-compat-allowlist.yaml` rather than worked around.

---

## Entity: `HealthCheck` (read-only in this feature)

No field changes. `Spec.CheckRef` becomes load-bearing: it is how a `HealthCheck`
resolves the effective cadence of the check it wraps, which is the only path by
which an aggregate can be cadence-aware (R4).

| Aspect | Behavior |
| --- | --- |
| `CheckRef.Kind` resolvable | Publish the referenced check's effective cadence for this `HealthCheck` |
| `CheckRef.Kind` not resolvable | Publish **no** cadence. Never fail the reconcile — #267 expands the supported kinds over time |

---

## Derived quantity: effective cadence

Not stored in any CR. Computed per reconcile and exported as a metric.

| Kind | Source | Default |
| --- | --- | --- |
| `AddonCheck` | `addonCheckInterval()` | 5m |
| `DNSCheck` | `dnsCheckInterval()` | 1m |
| `NodeCertificateCheck` | `nodeCertInterval()` | 1h |
| `HealthCheck` | effective cadence of the check named by `CheckRef` | none if unresolvable |
| `ClusterHealth` | **maximum** across selected children | none if no child resolves |

All three existing resolvers already apply per-resource override and the
`clampCadence` floor, so the published value is the cadence actually in force
(FR-006).

**Why maximum for the aggregate**: an aggregate can only be as current as its
slowest legitimate contributor. Minimum would make every mixed-cadence aggregate
permanently overdue — the User Story 2 failure this feature exists to avoid.

---

## State transitions

None. This feature adds no lifecycle, no phases, and no new conditions. The
aggregate's observable state remains (`result`, `observedAt`, `children`,
`matchedCount`, `conditions`); only how two of them are computed changes.
