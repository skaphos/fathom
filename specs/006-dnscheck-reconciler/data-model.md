# Phase 1 Data Model: DNSCheck Reconciler

**Feature**: 006-dnscheck-reconciler · **Date**: 2026-08-09

The persisted schema is **frozen** — `api/v1alpha1/dnscheck_types.go` shipped in
#294 and this feature adds no field. What follows is therefore two things: the
in-memory entities the reconciler works with, and the mapping from those onto
the fields that already exist.

---

## 1. In-memory entities

### Pair

The unit of evaluation, of result identity, and of metric series (FR-035).

| Field | Source | Notes |
|---|---|---|
| `Name` | `spec.targets[].name` | the subject looked up |
| `RecordType` | `spec.targets[].recordType` | `Host`\|`A`\|`AAAA`\|`CNAME`\|`SRV`\|`PTR` |
| `Resolver` | see expansion below | vantage-point name; `cluster` for the implicit one |
| `ExpectedAnswers` | `spec.targets[].expectedAnswers` | containment assertion |
| `Absent` | `spec.targets[].absent` | negative assertion |
| `Source` | resolved from `spec.resolvers[]` | `Cluster` \| `Node` \| `Explicit` |
| `Nameservers` | resolved from `spec.resolvers[]` | only when `Source == Explicit` |

**Identity** is `(Name, RecordType, Resolver)` — matching the `listMapKey` triple
already declared on `status.targetResults`, so an in-memory pair and a persisted
result share one key and cannot disagree.

**Expansion rule** (FR-035): a target naming a resolver produces exactly one
pair against it. A target naming none produces one pair per declared vantage
point, or a single `cluster` pair when the check declares none at all (FR-007,
FR-038).

**Cardinality**: ≤ 16 targets × ≤ 3 vantage points = **48 pairs**, derivable
from the spec without running anything.

### RunPlan

The full pair set for one evaluation, built from the *current* spec at the start
of each run. Never carried across runs — that is what makes FR-036 structural
rather than a cleanup step.

| Field | Notes |
|---|---|
| `Pairs` | expanded per above; ordered deterministically for reproducible logs |
| `Deadline` | one `context` deadline for the whole run (FR-104) |
| `PerPairCeiling` | upper bound on any single pair's probe timeout |
| `Concurrency` | in-flight cap (FR-103a) |
| `StartedAt` | the anchor for the next run's schedule (FR-107) |

### PairOutcome

One pair's result. **Seeded as `Unknown` for every pair before the run starts**
and overwritten as results land — so whatever is still `Unknown` at the deadline
is exactly the set the run did not reach (FR-106), with no separate bookkeeping.

| Field | Notes |
|---|---|
| `Pair` | identity |
| `Result` | `Pass`\|`Warn`\|`Fail`\|`Error`\|`Skipped`\|`Unknown` |
| `Message` | what was asked, what came back (≤ 512 chars persisted) |
| `Answers` | records returned, ≤ 16 |
| `LatencyMillis` | evidence only; never a pass/fail criterion in v1alpha1 |

**Outcome sourcing**, and the distinction FR-105 turns on:

| Situation | Result | Why |
|---|---|---|
| Probe answered, assertion satisfied | `Pass` | the check's finding |
| Probe answered, assertion violated | `Fail` | the check's finding (FR-025) — including resolution failure under a positive assertion |
| Pod could not be placed or run (`probe.LaunchError`) | `Error` | Fathom-side fault, not a resolver answer |
| Pod ran but wrote no usable result | `Error` | as `Launcher` already reports it |
| Deadline reached before the pair started | `Unknown` | never evaluated (FR-106) |

### RunResult

| Field | Notes |
|---|---|
| `Outcomes` | one `PairOutcome` per planned pair, always the full set |
| `Verdict` | `WorstResult(outcomes, coerceEmptyToUnknown=true)` (FR-108) |
| `Unreached` | count still `Unknown` at the deadline (FR-106a) |
| `Summary` | one line; names polarity on a negative-assertion failure (FR-021) |

---

## 2. Mapping onto the frozen schema

| Runtime | Persisted field | Notes |
|---|---|---|
| `RunResult.Verdict` | `status.lastResult` | shared vocabulary, no DNSCheck-specific values |
| `RunResult.Summary` | `status.summary` | ≤ 1024 chars |
| `RunResult.Outcomes` | `status.targetResults` | ≤ 48, `listType=map` on the identity triple |
| `RunPlan.StartedAt` | `status.lastRunTime` | also the freshness input to `observeCheck` |
| `len(RunPlan.Pairs)` | `status.observedTargets` | pair count, derivable from spec |
| `check.Generation` | `status.observedGeneration` | set before any work |
| persisted report name | `status.lastReportName` | from `createOrReuseHealthReport` |
| — | `status.lastRunTrigger` | **left untouched**: #264's run-now trigger is out of scope |

### Conditions

| Type | Meaning |
|---|---|
| `Accepted` | spec accepted; reason `SpecClamped` when a stored sub-floor cadence was raised at runtime (reuses `cadenceClampMessages`) |
| `Ready` | the controller can evaluate this check; `False` with a reason on launch/RBAC failure |
| `Complete` | **the truncation signal (FR-106a)** — `False` when `Unreached > 0`, message naming the count |

`Complete` is a new condition *type*, not a schema change: `status.conditions` is
a standard `[]metav1.Condition`.

---

## 3. Metric series

### Existing, reused unchanged (FR-032)

`fathom_check_result{kind,name,namespace,result}` and
`fathom_check_last_run_timestamp_seconds{kind,name,namespace}` — both written by
`observeCheck` with `kind="DNSCheck"`, and removed by `DeleteCheckSeries` when a
reconcile observes the check is gone.

### New (FR-033)

`fathom_dnscheck_target_result{namespace,check,name,record_type,resolver,result}`

One-hot per pair, mirroring the check-level gauge one level down.

**Ceiling**: 16 × 3 × 6 = **288 per check** — exactly SC-009, derivable from the
schema caps alone (FR-034). Raising any cap is a cardinality change, not a limits
change.

**Rebuild, not diff** (FR-036, D7): each run deletes the check's series by
partial match on `{namespace, check}`, then sets the current pairs. A pair the
spec no longer declares is simply never re-set, so its series disappears with no
removal-detection logic — and the behaviour survives an operator restart, which
a diff against remembered state would not.

---

## 4. Lifecycle and ownership

| Object | Lifetime | Reclaimed by |
|---|---|---|
| Probe pod | one pair, one run | `Launcher`'s delete defer; **owner reference** to the `DNSCheck` (D1) for the deletion cascade; `Sweeper` for the crash-orphan case |
| `HealthReport` | until pruned | written only on verdict change (FR-111), pruned as the other kinds do |
| Per-target series | one run | delete-then-set each run; all withdrawn on check deletion (FR-114) |

The owner reference is legal here **only** because FR-031 puts the pod in the
check's own namespace — a namespaced owner and its object must share a namespace,
which is why `AddonCheck` cannot do the same (see research D1).

---

## 5. State transitions

```text
                    ┌─────────────────────────────────────┐
                    │ no status (never evaluated)         │
                    │ lastResult "" → reads as Unknown    │
                    └───────────────┬─────────────────────┘
                                    │ first reconcile
                                    ▼
   ┌────────────────────────────────────────────────────────────┐
   │ plan: expand pairs from current spec, seed all Unknown     │
   └───────────────┬────────────────────────────────────────────┘
                   │ fan out, bounded by Concurrency, one deadline
                   ▼
   ┌────────────────────────────────────────────────────────────┐
   │ fold: WorstResult over every planned pair                  │
   │  · all reached      → verdict from real outcomes           │
   │  · some unreached   → those stay Unknown, Complete=False   │
   └───────────────┬────────────────────────────────────────────┘
                   │
     verdict changed?├── yes ──▶ write HealthReport, set lastReportName
                   │
                   └── no  ──▶ refresh lastRunTime only (throttled)
                   │
                   ▼
   ┌────────────────────────────────────────────────────────────┐
   │ requeue: max(minGap, interval − elapsed)  ← anchored to     │
   │          RunPlan.StartedAt, not to completion (FR-107)      │
   └────────────────────────────────────────────────────────────┘
```

Deletion short-circuits the whole thing: a `NotFound` on the initial `Get` calls
`DeleteCheckSeries` plus the new per-target withdrawal and returns, so a deleted
check stops asserting anything (FR-114).
