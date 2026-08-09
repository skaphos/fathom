# Implementation Plan: DNSCheck Reconciler

**Branch**: `feature/266-dnscheck-reconciler` | **Date**: 2026-08-09 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-dnscheck-reconciler/spec.md`

## Summary

Feature 005 shipped the `DNSCheck` contract and the probe's DNS capability. This
feature is the controller that drives them: it evaluates each declared
(target, vantage point) pair in the check's **own** namespace, folds the results
with the project's shared verdict fold, publishes status, metrics, events, and
history, and keeps every pod it creates accountable.

The approach reuses more than it builds. `observeCheck` already delivers the
check-level gauges and the whole events contract for any kind. The
persist-on-change rollup, deterministic report naming, cadence clamping, and the
probe launcher all exist. What is genuinely new is the fan-out layer — bounded
concurrency over pairs under a single run deadline — plus a per-target gauge, an
owner reference the probe package cannot currently express, an uncached client
so pod polling does not open a cluster-wide watch, and the widened pods grant
with the document that has to defend it.

Phase 0 turned up three places where issue #266's own framing is wrong about the
current code. They are recorded in [research.md](./research.md) as D1, D2 and D9,
and they change the work: probe pods have **no** owner reference today, the
manager client **cannot** safely read pods, and the operator **cannot create a
pod at all**.

## Technical Context

**Language/Version**: Go 1.26.5 (`go.mod` is the source of truth)

**Primary Dependencies**: `sigs.k8s.io/controller-runtime` v0.24.1, `k8s.io/api`
v0.36.3, `golang.org/x/sync/errgroup` for bounded fan-out, `prometheus/client_golang`
via `internal/metrics`

**Storage**: Kubernetes API only — `DNSCheck` status as current state,
`HealthReport` objects as history. No external store.

**Testing**: Ginkgo v2 + Gomega under envtest for the reconcile loop; stdlib
`testing` with table-driven cases for the pure helpers (fan-out planning, budget
derivation, cadence arithmetic, truncation folding); Kind-based e2e for the parts
only a real cluster exercises. envtest pinned at Kubernetes 1.36, in lockstep
with the `k8s.io/*` modules.

**Target Platform**: Linux; the operator runs as a Deployment, probe pods run as
short-lived pods in arbitrary tenant namespaces.

**Project Type**: Kubernetes operator (kubebuilder v4 layout)

**Performance Goals**: A run completes within its declared bound at any pair
count the schema admits, or truncates visibly. Effective cadence equals the
declared cadence whenever the run leaves slack (SC-108). Per-target series stay
at or below 288 per check (SC-009).

**Constraints**: No unbounded work inside `Reconcile` (constitution). No
cluster-wide Pod informer — a cached Pod read reintroduces the #164 memory
blow-up. Pods grant must not exceed what pod placement requires; no `watch`.
The `DNSCheck` schema is frozen — everything here fits the shipped fields.

**Scale/Scope**: Up to 16 targets × 3 vantage points = 48 pairs per check.
Cluster-wide concurrent probe pods bounded by the per-check cap × the
controller's reconcile concurrency, derivable from configuration alone.

## Constitution Check

*GATE: evaluated before Phase 0 and re-evaluated after Phase 1.*

| Principle / Constraint | Assessment |
|---|---|
| **I. Explicit State Over Implicit Behavior** | PASS — every outcome is a declared field on `DNSCheck.status`; truncation surfaces as a condition, not a log line. |
| **III. Deterministic, Reconstructible Operation** | PASS — report names are content-derived via `deterministicHealthReportName`; the result set is rebuilt from spec each run, so state cannot drift. |
| **IV. Kubernetes-Native, Never Obscured** | PASS — CRD status, conditions, events, owner references, and pods. No side channel. |
| **VI. Explainable Reconciliation, Evidence-Grade Audit** | PASS — per-pair results carry message, answers, and latency (FR-110); the verdict always has its reason. |
| **VII. Read-Only Degradation Over Blindness** | PASS — a truncated or failing run degrades the verdict to `Unknown` and keeps the last-known per-pair evidence visible rather than blanking status. |
| **IX. Technical Precision, Honest Scope** | PASS — FR-104b requires documenting that a maximum-size check does **not** complete at the default bound, rather than leaving it to be discovered. |
| **Bounded, idempotent reconciliation** | PASS — one run per pass, bounded by a single derived deadline; re-running a pass with unchanged spec produces identical status. |
| **Minimal RBAC** | **ATTENTION** — requires widening pods cluster-wide from `list;delete` to add `create;get`. Recorded in Complexity Tracking; carries a written justification per FR-115. |
| **Configuration model** | PASS — the concurrency cap enters through the `bindings()` table, keeping flag / env / config-file / default in sync. Needs a new integer binding (D10). |
| **`ClusterHealth` contract stability** | PASS — untouched; mirroring `DNSCheck` into the aggregate is explicitly out of scope. |

**Post-Phase-1 re-evaluation**: unchanged. The design added no new violation. The
single RBAC item was known from feature 005's FR-037 and is inherited
deliberately, not introduced here.

## Project Structure

### Documentation (this feature)

```text
specs/006-dnscheck-reconciler/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── reconcile-loop.md
│   └── metrics-and-events.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
api/v1alpha1/
└── dnscheck_types.go              # unchanged — schema is frozen

internal/controller/
├── dnscheck_controller.go         # NEW — Reconcile, fan-out, rollup, status
├── dnscheck_plan.go               # NEW — pure helpers: pair expansion, budget,
│                                  #       cadence arithmetic, truncation fold
├── cadence.go                     # reused — clampCadence, cadenceClampMessages
├── observe.go                     # reused — observeCheck (gauges + events)
└── healthreport_idempotency.go    # reused — deterministic naming, create-or-reuse

internal/probe/
├── pod.go                         # CHANGED — Request gains owner references
└── launcher.go                    # unchanged
└── sweeper.go                     # unchanged — still the orphan backstop

internal/metrics/
└── metrics.go                     # CHANGED — per-target gauge + its delete helper

internal/app/
├── run.go                         # CHANGED — register reconciler, uncached
│                                  #           pod client
└── options.go                     # CHANGED — integer binding, concurrency cap

config/rbac/
└── role.yaml                      # regenerated — pods create;get added

docs/
├── rbac-justification.md          # NEW (or extended) — defends the pods grant
└── reference/                     # sizing guidance for FR-104b

test/e2e/
└── dnscheck_test.go               # NEW — foreign-namespace placement, ownership
```

**Structure Decision**: Standard kubebuilder v4 layout, already established in
this repository. The reconciler follows the file-per-kind convention in
`internal/controller/`, with a companion `_plan.go` holding the pure functions so
fan-out planning, budget derivation, and cadence arithmetic are table-testable
without envtest — mirroring how `decideNodeCertRollup` is kept pure and separately
tested.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Cluster-wide `pods: create;get` | FR-031 requires resolution to run in the check's own namespace, and a `DNSCheck` may exist in any namespace, so the set is unknown when RBAC is rendered. The operator currently holds `pods: [list, delete]` and cannot create a pod at all. | **Namespace-scoped Roles**: would require the operator to create a Role and RoleBinding in every namespace a check appears in — a strictly broader grant (`roles`/`rolebindings` write, cluster-wide) to achieve a narrower one. **Impersonating a per-namespace ServiceAccount** (the `AddonCheck`/SKA-58 pattern): the operator would still need to create that ServiceAccount in a namespace it does not own, requiring the same class of cluster-wide write. **Running probes from the operator's namespace**: rejected by FR-031 on security grounds — it would let a check author borrow the operator namespace's egress posture. Per FR-037 this cost is not introduced by this feature: the planned reachability checks require the same grant with neither endpoint in the operator's namespace. |
| Uncached client dedicated to probe pods | The manager's cached client would open an unfiltered cluster-wide Pod informer on first read (D2), reintroducing the #164 memory blow-up. | **`mgr.GetAPIReader()`**: read-only, and `Launcher` needs one `client.Client` that can Create and Delete. **Adding `Pod` to `scopedCacheOptions()`**: still opens a cluster-wide Pod watch; the label scoping cannot be expressed in RBAC, so it buys nothing over an uncached client. |
