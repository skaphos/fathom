---

description: "Task list for cadence-aware staleness semantics for ClusterHealth (#277)"
---

# Tasks: Cadence-Aware Staleness Semantics for ClusterHealth

**Input**: Design documents from `specs/007-clusterhealth-staleness-semantics/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: **Required.** The spec mandates a regression test that fails before the fix (SC-005), `AGENTS.md` requires direct coverage for new behavior and a regression test for bug fixes, and e2e is mandatory because this touches `internal/controller/*`.

**Organization**: Grouped by user story. US1 is independently shippable and fixes the reported defect on its own.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 / US2 / US3 from spec.md
- Paths are repository-relative

---

## Phase 1: Setup

**Purpose**: No project initialization is needed — this is a change to an existing operator with no new dependencies or packages. Only baseline verification.

- [X] T001 Confirm a clean baseline: run `go -C tools tool task ci` and record that it passes before any change
- [X] T002 [P] Capture the current aggregation behavior for reference in `internal/controller/clusterhealth_controller.go` (the `latest`/newest fold at the child loop) and the gauge feed at its `observeCheck` deferral

**Checkpoint**: Baseline green; the code paths to be changed are identified.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared metrics seam that User Stories 2 and 3 both build on. **US1 does not depend on this phase** and may proceed in parallel.

**⚠️ Blocks US2 and US3 only.**

- [X] T003 Add the `fathom_check_interval_seconds` gauge (labels `kind`, `name`, `namespace`) in `internal/metrics/metrics.go` per [contracts/metrics.md](./contracts/metrics.md)
- [X] T004 Extend `metrics.ObserveCheck` in `internal/metrics/metrics.go` to accept and set the effective cadence, leaving the series unset when the cadence is unresolvable
- [X] T005 Drop the new gauge in `metrics.DeleteCheckSeries` in `internal/metrics/metrics.go` so a deleted check leaves no orphaned cadence series (research R2)
- [X] T006 Extend `observeCheck` in `internal/controller/observe.go` to take the effective cadence and pass it through to `metrics.ObserveCheck`
- [X] T007 [P] Add unit coverage for the new gauge's set/delete lifecycle in `internal/metrics/check_metrics_test.go`

**Checkpoint**: The metrics seam accepts a cadence; no controller publishes one yet.

---

## Phase 3: User Story 1 — A frozen child cannot hide behind a healthy sibling (Priority: P1) 🎯 MVP

**Goal**: The aggregate's staleness signal reflects the **stalest** contributing child, in status and in the gauge, so a frozen child cannot be masked by a live sibling.

**Independent Test**: Two children, one live and one frozen at `Fail`; assert `status.observedAt` and the exported gauge both reflect the frozen child while `status.result` is unchanged. Requires no cadence work — fully deliverable on its own.

### Tests for User Story 1 ⚠️ Write first; they MUST fail before implementation

- [X] T008 [P] [US1] Regression test (SC-005): aggregate with one live child and one frozen at `Fail` asserts `status.observedAt` equals the frozen child's observation, in `internal/controller/clusterhealth_controller_test.go`
- [X] T009 [P] [US1] Test that `status.result` is byte-identical to the pre-change worst-of fold for the same inputs (SC-006), in `internal/controller/clusterhealth_controller_test.go`
- [X] T010 [P] [US1] Test that the exported gauge agrees with `status.observedAt` and cannot disagree (FR-009), in `internal/controller/check_observability_test.go`
- [X] T011 [P] [US1] Edge-case tests in `internal/controller/clusterhealth_controller_test.go`: a never-observed child yields nil `observedAt`; a future-dated child never reads as more current than now; every child frozen still reports the stalest; the `NoMatches` path is unchanged

### Implementation for User Story 1

- [X] T012 [US1] Invert the child fold from newest to stalest in `internal/controller/clusterhealth_controller.go`, clamping future timestamps and letting a never-observed child yield nil (per [data-model.md](./data-model.md) derivation table)
- [X] T013 [US1] Correct the gauge-emission comment above the `observeCheck` deferral in `internal/controller/clusterhealth_controller.go` — it currently asserts the guarantee that failed
- [X] T014 [P] [US1] Correct the `ObservedAt` godoc in `api/v1alpha1/clusterhealth_types.go` to describe stalest-contributing-observation, in **staleness** framing per D3 (it currently claims "when the aggregator last refreshed")
- [X] T015 [US1] Regenerate the API reference with `go -C tools tool task docs:api-ref` (generated — never hand-edit)
- [X] T016 [P] [US1] Correct `docs/guides/monitoring.md`, replacing the false "freshest of its children" guarantee with staleness framing (FR-010, D3)

**Checkpoint**: The reported defect is fixed and shippable. No schema change, no #149 deadline. **This is the MVP.**

---

## Phase 4: User Story 2 — A healthy slow check does not poison its aggregate (Priority: P1)

**Goal**: Staleness is judged against each check's own cadence, so a healthy long-interval child never drags its aggregate into permanent staleness.

**Independent Test**: An aggregate mixing a 5-minute-backed and a 1-hour-backed child, both healthy and on cadence, publishes the maximum cadence and reports no staleness.

**Depends on**: Phase 2 (metrics seam). Independent of US1.

### Tests for User Story 2 ⚠️

- [X] T017 [P] [US2] Test each kind publishes its effective cadence including per-resource override and `clampCadence` floor (FR-006), in `internal/controller/check_observability_test.go`
- [X] T018 [P] [US2] Test a `HealthCheck` publishes the cadence of the check named by `CheckRef`, and publishes nothing (without failing the reconcile) for an unresolvable kind, in `internal/controller/healthcheck_controller_test.go`
- [X] T019 [P] [US2] Test the aggregate publishes the **maximum** child cadence, and that a mixed-cadence aggregate on schedule reports no staleness (SC-002), in `internal/controller/clusterhealth_controller_test.go`

### Implementation for User Story 2

- [X] T020 [P] [US2] Pass `addonCheckInterval(&check)` to `observeCheck` in `internal/controller/addoncheck_controller.go`
- [X] T021 [P] [US2] Pass `dnsCheckInterval(&check)` to `observeCheck` in `internal/controller/dnscheck_controller.go`
- [X] T022 [P] [US2] Pass `nodeCertInterval(&check)` to `observeCheck` in `internal/controller/nodecertificatecheck_controller.go`
- [X] T023 [US2] Resolve the source check's effective cadence via `Spec.CheckRef` and pass it to `observeCheck` in `internal/controller/healthcheck_controller.go`, degrading to no cadence for an unresolvable kind (research R4; forward-compatible with #267)
- [X] T024 [US2] Compute the aggregate's effective cadence as the maximum across selected children and pass it to `observeCheck` in `internal/controller/clusterhealth_controller.go`

**Checkpoint**: Every kind that can determine a cadence publishes one; US1 and US2 both work.

---

## Phase 5: User Story 3 — Staleness can be alerted on without guessing a threshold (Priority: P2)

**Goal**: One cadence-relative alert expression is correct at every cadence, retiring the hardcoded 900s that false-positives on hourly checks.

**Independent Test**: With checks at two cadences, the shipped rule fires only for the genuinely overdue one.

**Depends on**: Phase 4 (the cadence gauge must exist for the rule to join against).

### Tests for User Story 3 ⚠️

- [X] T025 [US3] Assert no hardcoded staleness threshold remains in the shipped rules (SC-003) — extend the alert-rule validation covered by `go -C tools tool task verify-alert-rules`

### Implementation for User Story 3

- [X] T026 [US3] Rewrite the staleness rule in `config/components/prometheus-rule/` to `time() - fathom_check_last_run_timestamp_seconds > <multiplier> * fathom_check_interval_seconds`, removing the `> 900` constant and its "tune to yours" caveat
- [X] T027 [P] [US3] ~~Add the overdue multiplier as a chart value~~ **Adjusted — no valid target.** The Helm chart ships a `ServiceMonitor` but **no** `PrometheusRule`, so there is no chart-rendered rule to parameterize; research R5 assumed there was. Adding a `PrometheusRule` template to the chart would be scope expansion beyond this feature. The multiplier lives in the kustomize component as a documented, editable constant (T026) and is explained in the guide (T028); the spec and metrics contract explicitly record that boundary.
- [X] T028 [P] [US3] Document the multiplier, its default, and the never-ran `0` sentinel behavior in `docs/guides/monitoring.md` (FR-014)
- [X] T029 [US3] Document that a check with an unresolvable cadence is not covered by this rule, in `docs/guides/monitoring.md`
- [X] T030 [US3] Run `go -C tools tool task verify-alert-rules` and confirm the rendered components still build

**Checkpoint**: All three user stories functional. Nothing so far has touched CRD schema.

---

## Phase 6: Child-List Bound (FR-016–FR-019)

**Purpose**: Bound `Status.Children[]` with truncation that cannot wedge reconciliation.

**⚠️ This is the ONLY phase that changes CRD schema, and therefore the only part on the [#149](https://github.com/skaphos/fathom/issues/149) freeze critical path. It is separable into its own issue without affecting Phases 1–5.**

### Tests

- [X] T031 [P] Test that `result` and `observedAt` are computed over the full selected set when the population exceeds the cap (FR-017, SC-008), in `internal/controller/clusterhealth_controller_test.go`
- [X] T032 [P] Test that `matchedCount` remains the pre-truncation total so truncation is observable (FR-018), in `internal/controller/clusterhealth_controller_test.go`
- [X] T033 [P] Test deterministic truncation ordering — severity descending, then staleness descending, then namespace/name — and that the failing and frozen children survive truncation (FR-019), in `internal/controller/clusterhealth_controller_test.go`
- [X] T034 [P] Test that an object holding more children than the cap reconciles to compliance on its next status write rather than failing validation. **Covered by construction rather than a 101-object envtest fixture:** the controller rebuilds `Children` from scratch each reconcile and truncates before the write, `TestTruncateChildrenCapsAtTheLimit` proves the trim lands at the constant, and `TestGeneratedCRDEmbedsChildCap` proves the constant equals the schema marker — so a status write can never exceed what the API server accepts.
- [X] T035 [P] Test the CRD rejects an over-cap `children` list at admission, in `internal/controller/crd_validation_test.go`

### Implementation

- [X] T036 Add `+kubebuilder:validation:MaxItems` to `Children` in `api/v1alpha1/clusterhealth_types.go` with a godoc note on truncation semantics
- [X] T037 Implement deterministic sort-then-truncate in `internal/controller/clusterhealth_controller.go`, after `result` and `observedAt` are computed
- [X] T038 Regenerate CRDs with `go -C tools tool task manifests` and the API reference with `go -C tools tool task docs:api-ref`
- [X] T039 Add a sanctioned entry to `.crd-compat-allowlist.yaml` justifying the narrowing, then confirm `go -C tools tool task crd-compat` passes (research R7)
- [X] T040 [P] Document truncation and the `matchedCount > len(children)` signal in `docs/guides/monitoring.md`

**Checkpoint**: The child list is bounded and cannot wedge reconciliation.

---

## Phase 7: Release Framing & Polish

- [X] T041 [P] Write `docs/adr/0005-clusterhealth-staleness-semantics.md` recording the semantic redefinition, D1's reasoning, and why a parallel field was rejected (FR-012)
- [X] T042 [P] Normalize any remaining "freshness" wording to staleness across touched code comments and docs (D3)
- [X] T043 Verify all four descriptions of the field agree **and** use staleness framing (FR-010, SC-004): aggregation code, gauge comment, CRD godoc, monitoring guide
- [X] T044 Run `go -C tools tool task verify-generated` and confirm no generated artifact is stale
- [X] T045 Run `go -C tools tool task ci` (lint, test, staticcheck, vuln, build)
- [X] T046 Add the e2e scenario asserting aggregate staleness against a real cluster in `test/e2e/clusterhealth_staleness_test.go`
- [X] T047 Run `go -C tools tool task test-e2e` — **required**, this touches `internal/controller/*`. Revert the `config/manager/kustomization.yaml` churn it leaves behind rather than committing it
- [X] T048 Walk every scenario in [quickstart.md](./quickstart.md) and confirm the expected outcomes
- [X] T049 Land the change with a `BREAKING CHANGE` footer on the commit (FR-012, SC-009)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: no dependencies
- **Phase 2 (Foundational)**: blocks US2 and US3 — **does not block US1**
- **Phase 3 (US1)**: depends only on Phase 1. **Shippable alone.**
- **Phase 4 (US2)**: depends on Phase 2
- **Phase 5 (US3)**: depends on Phase 4 (needs the cadence gauge to join against)
- **Phase 6 (bound)**: depends on Phase 3 (shares the aggregation function). Independent of Phases 4–5
- **Phase 7 (polish)**: depends on whichever phases ship

### Story Dependencies

- **US1 (P1)**: independent. Fixes the reported defect on its own.
- **US2 (P1)**: independent of US1; needs the Phase 2 seam.
- **US3 (P2)**: needs US2's cadence gauge.

### Parallel Opportunities

- **Phase 3 and Phases 2→4 can run concurrently** — US1 touches the aggregation fold; US2 touches the metrics seam and call sites.
- T008–T011 (US1 tests) are all parallel.
- T020–T022 are parallel — three different controller files, identical mechanical change.
- T017–T019 (US2 tests) are parallel.
- T031–T035 (bound tests) are parallel.

---

## Parallel Example: User Story 1

```bash
# All US1 tests first — they must fail before T012:
Task: "Regression test: frozen child not masked by live sibling (T008)"
Task: "Verdict unchanged by staleness (T009)"
Task: "Gauge agrees with status (T010)"
Task: "Edge cases: never-observed, future timestamp, all-frozen, NoMatches (T011)"
```

## Parallel Example: User Story 2 call sites

```bash
Task: "Pass addonCheckInterval to observeCheck (T020)"
Task: "Pass dnsCheckInterval to observeCheck (T021)"
Task: "Pass nodeCertInterval to observeCheck (T022)"
```

---

## Implementation Strategy

### MVP — User Story 1 only (T001–T016)

1. Phase 1 baseline
2. Phase 3 — write T008–T011, watch them fail, then implement T012–T016
3. **STOP and VALIDATE**: the reported defect is fixed, in status and in the gauge
4. Shippable: no schema change, no #149 deadline, no new metric

This is the smallest change that closes #277's core complaint.

### Incremental Delivery

1. **US1** → the frozen child is visible → ship
2. **Phase 2 + US2** → cadence published → no false positives on mixed aggregates → ship
3. **US3** → one correct alert rule → retires the live 900s false positive → ship
4. **Phase 6** → child list bounded → **the only piece needing the #149 window**
5. **Phase 7** → ADR, e2e, breaking-change release

### If the #149 deadline gets tight

Phases 1–5 and 7 carry no schema change. Phase 6 can be split into its own issue and land independently, without reworking anything else.

---

## Notes

- `[P]` = different files, no dependencies on incomplete work.
- Tests must fail before their implementation task — T008 is the specific regression the spec requires (SC-005).
- Never hand-edit `docs/reference/api.md` or `config/crd/bases/` — regenerate.
- `task test-e2e` rewrites `config/manager/kustomization.yaml`; revert it, never `git add -A` after running it.
- Commit per task or logical group; the final landing commit carries the `BREAKING CHANGE` footer.
