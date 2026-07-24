# Implementation Plan: Quorum/Ratio Semantics for Managed-Resource Rollups

**Branch**: `feature/159-quorum-ratio-rollups` | **Date**: 2026-07-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/002-quorum-ratio-rollups/spec.md`

## Summary

Add opt-in, per-family ratio thresholds (`warnRatio`, `failRatio`) to the
existing `AddonCheck.spec.policy.<family>.thresholds` surface, and make the
controller's HealthReport aggregation family-aware: families with ratio
thresholds roll up by population fractions (strict-exceed) instead of
worst-of, while everything else stays bit-for-bit identical. Ratio evaluation
is implemented once — pure helpers in `pkg/adapter` consumed by the
AddonCheck controller's aggregation — so every adapter (hand-written and
declarative) gets it uniformly with zero per-adapter changes. Ratio-evaluated
families emit a synthetic rollup entry in the HealthReport carrying the
counts and thresholds that produced the verdict.

## Technical Context

**Language/Version**: Go 1.26.5 (per `go.mod`)

**Primary Dependencies**: controller-runtime, kubebuilder v4 scaffolding; no new dependencies

**Storage**: Kubernetes API (CRDs: `AddonCheck`, `HealthReport`); no CRD schema changes — the feature rides the existing `thresholds map[string]string`

**Testing**: stdlib `testing` (table-driven) for `pkg/adapter` and controller unit seams; envtest for controller behavior; Ginkgo e2e against kind (full stack — shared-surface change per AGENTS.md)

**Target Platform**: Linux Kubernetes operator (same as existing manager binary)

**Project Type**: Kubernetes operator (existing single-module repo)

**Performance Goals**: aggregation stays O(checks) per reconcile; no extra API calls, no new watches

**Constraints**: bounded/idempotent reconcile preserved; `ClusterHealth` external contract untouched; contract additions to `pkg/adapter` must be additive under the 1.x adapter contract

**Scale/Scope**: ~2 new files in `pkg/adapter`, targeted edits in `internal/controller/addoncheck_controller.go`, docs updates; no RBAC, no new CRD fields, no adapter edits

## Constitution Check

*GATE: evaluated against `.specify/memory/constitution.md` (Fathom v1.0.0).*

| Gate | Verdict | Notes |
|------|---------|-------|
| I. Explicit state over implicit behavior | PASS | Ratio intent is declared in `spec.policy.<family>.thresholds`; no annotations or out-of-band config. |
| II. Git as desired-state boundary | PASS | Pure CRD-spec-driven behavior; GitOps-compatible. |
| III. Deterministic, reconstructible operation | PASS | Verdict is a pure function of check outcomes + declared thresholds; strict-exceed boundaries are exact (no float ambiguity at `0`). |
| IV. Kubernetes-native, never obscured | PASS | Surfaced via `Accepted` condition, `status.lastResult`, and HealthReport entries readable with `kubectl`. |
| V. Compose, don't trap | PASS | Additive `pkg/adapter` helpers; adapter contract stays 1.x-compatible; adapters need no changes. |
| VI. Explainable reconciliation | PASS | FR-010 rollup entry records population/unhealthy/degraded counts + thresholds in the persisted report. |
| VII. Read-only degradation over blindness | PASS | `Error` outcomes short-circuit ratios — adapter blindness is never averaged away (FR-004). |
| IX. Technical precision, honest scope | PASS | Docs will state ratio applies to family/overall rollups only; per-resource results unchanged. |
| ClusterHealth contract stability | PASS | Untouched (FR-011); change is upstream of `HealthCheck.status` mirroring and does not alter its derivation. |
| Bounded, idempotent reconciliation | PASS | Same single aggregation pass; no unbounded work. |
| Minimal RBAC | PASS | No new permissions. |
| Configuration model | PASS | No new operator options. |

Post-Phase-1 re-check: PASS (no violations introduced by the design; no Complexity Tracking entries needed).

## Project Structure

### Documentation (this feature)

```text
specs/002-quorum-ratio-rollups/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── ratio-rollup.md  # Threshold keys + HealthReport rollup entry contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (complete)
└── tasks.md             # Phase 2 output (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
pkg/adapter/
├── adapter.go           # existing contract (FamilyOutcome, ThresholdAdvertiser)
├── ratio.go             # NEW: reserved keys, RatioThresholds, ParseRatioThresholds, FamilyRatioVerdict
└── ratio_test.go        # NEW: external _test package, table-driven

internal/controller/
├── addoncheck_controller.go       # family-aware aggregation; reserved-key validation;
│                                  # synthetic rollup report entries
└── addoncheck_controller_test.go  # unit coverage for the new aggregation paths

docs/
├── guides/addon-checks.md         # user-facing threshold documentation
└── authoring-adapters.md          # note: reserved keys never reach adapter-private validation

test/e2e/                          # full-stack run required (controller + pkg/adapter change);
                                   # focused scenario per tasks.md if a core-tier addon fits
```

**Structure Decision**: Ratio parsing/evaluation lives in `pkg/adapter`
(`ratio.go`) because it is contract-adjacent — the keys ride the contract's
`FamilyPolicy.Thresholds` and the evaluator consumes `[]CheckResult` — and a
pure, exported helper keeps the controller thin and independently testable.
The only behavioral integration point is `internal/controller/addoncheck_controller.go`,
where per-run aggregation and policy validation already live.

## Complexity Tracking

No constitution violations; table intentionally empty.
