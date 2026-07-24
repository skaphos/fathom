# Tasks: Quorum/Ratio Semantics for Managed-Resource Rollups

**Input**: Design documents from `specs/002-quorum-ratio-rollups/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ratio-rollup.md

**Tests**: Included — the constitution mandates direct test coverage for new
behavior, and SC-002 explicitly requires regression proof.

**Organization**: Grouped by user story from spec.md (US1 isolated failures,
US2 graduated escalation, US3 backwards compatibility).

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

No setup tasks — existing repo, no new dependencies, no scaffolding.

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: The ratio engine and the policy-validation exemption every story
depends on. Without the validation change, setting `failRatio` on an adapter
that implements `ThresholdAdvertiser` would be rejected as an unknown key.

- [x] T001 [P] Implement ratio engine in `pkg/adapter/ratio.go`: reserved key consts (`ThresholdKeyWarnRatio = "warnRatio"`, `ThresholdKeyFailRatio = "failRatio"`), fixed-point `RatioThresholds` with `Configured()`, `ParseRatioThresholds(map[string]string)` (grammar per contracts/ratio-rollup.md: decimal, optional `%`, range 0–100), and `FamilyRatioVerdict(checks []CheckResult, family Family, rt RatioThresholds)` returning verdict + population/unhealthy/degraded counts per data-model.md derivation (Error short-circuit, Skipped excluded, strict-exceed, omitted-level worst-of fallback, empty population → Pass)
- [x] T002 [P] Table-driven tests in `pkg/adapter/ratio_test.go` (external `adapter_test` package per repo convention): parse formats (`"5"`, `"5%"`, `"2.5"`), parse rejections (`"banana"`, `"150"`, `"-1"`, `"5%%"`), boundary equality does not escalate, `"0"` reproduces worst-of, omitted warn/fail fallbacks, Error short-circuit, Skipped exclusion, empty population, Warn-only fleets never Fail through ratios
- [x] T003 Reserved-key validation in `internal/controller/addoncheck_controller.go`: exempt `warnRatio`/`failRatio` in `unknownThresholdKeys` regardless of `ThresholdAdvertiser`, and extend `validateAddonCheckPolicy` to parse reserved values via `adapter.ParseRatioThresholds`, appending a problem naming family + key on failure (surfaces as `Accepted=False/InvalidPolicy`)
- [x] T004 Validation unit tests in `internal/controller/addoncheck_controller_test.go`: reserved keys accepted on advertising and non-advertising adapters, `failRatio: "banana"` and `"150"` produce `Accepted=False` with family+key in the message, valid values leave `Accepted=True`

**Checkpoint**: `go -C tools tool task test` green; ratio engine fully
covered; no behavior change yet (aggregation untouched).

## Phase 3: User Story 1 — Isolated failures stop redding the fleet verdict (P1) 🎯 MVP

**Goal**: With `failRatio` configured, one broken CR in a large family no
longer turns the family/overall verdict Fail, while its individual Fail entry
stays in the HealthReport.

**Independent test**: Unit-level — feed a fake adapter result with 1 Fail in
200 checks plus `failRatio: "5"` policy through report generation; family and
overall results are not Fail; the broken resource's entry is unchanged.

- [x] T005 [US1] Make aggregation family-aware in `internal/controller/addoncheck_controller.go`: pass the AddonCheck policy into `healthReportForAddonCheck`/`aggregateHealthReportResult`; group checks by family; families where `RatioThresholds.Configured()` contribute their `FamilyRatioVerdict` to the fold, all other families contribute raw outcomes exactly as today; preserve the empty-checks → Skipped and runErr → Error behaviors
- [x] T006 [US1] Emit synthetic rollup entries in `internal/controller/addoncheck_controller.go`: for each ratio-evaluated family append a `HealthReportCheck` (family, verdict, TargetRef = the AddonCheck, human summary, details `rollup: "ratio"`, `population`, `unhealthy`, `degraded`, plus `warnRatio`/`failRatio` verbatim when set) per contracts/ratio-rollup.md
- [x] T007 [US1] Aggregation unit tests in `internal/controller/addoncheck_controller_test.go`: 1-of-200 Fail under `failRatio: "5"` → report result Pass and rollup entry with `population: "200"`, `unhealthy: "1"`; 15-of-200 (7.5%) → Fail; per-resource check entries byte-identical to the no-policy case; `status.lastResult` mirrors the ratio-adjusted result

**Checkpoint**: US1 acceptance scenarios 1–3 pass at unit level — MVP
demonstrable.

## Phase 4: User Story 2 — Graduated escalation between Warn and Fail (P2)

**Goal**: Two-threshold escalation Pass → Warn → Fail with explainable
counts.

**Independent test**: Drive degraded fractions 1% / 5% / 12% through
`warnRatio: "2"` + `failRatio: "10"`; verdicts Pass / Warn / Fail; rollup
entry shows counts and both thresholds.

- [x] T008 [US2] Escalation tests in `internal/controller/addoncheck_controller_test.go`: US2 acceptance table (1% → Pass, 5% → Warn, 12% → Fail), Warn+Fail both count into degraded, equality-at-threshold does not escalate, Error in family → family Error regardless of ratios
- [x] T009 [US2] Verify and finalize rollup summary formatting in `internal/controller/addoncheck_controller.go` so an operator can read verdict provenance from `kubectl get healthreport -o yaml` alone (counts, percentages, thresholds, verdict; asserted in the T008 tests)

**Checkpoint**: US2 acceptance scenarios pass; FR-010 explainability
verified.

## Phase 5: User Story 3 — Existing checks keep exact worst-of behavior (P3)

**Goal**: Zero behavior change without thresholds; no implicit defaults.

**Independent test**: Existing adapter/controller suites pass unmodified;
no-threshold runs produce reports with no rollup entries.

- [x] T010 [US3] Regression tests in `internal/controller/addoncheck_controller_test.go`: no-threshold aggregation results identical across the outcome matrix (single Fail → Fail, Warn → Warn, empty → Skipped, runErr → Error), no `rollup` entries emitted, and existing suite expectations (`go -C tools tool task test`) unchanged — confirming no default thresholds exist anywhere
- [x] T011 [P] [US3] Contract-stability assertion: grep-level check plus a unit test proving `ClusterHealth` aggregation inputs (`HealthCheck.status`) are untouched by this change (no edits under `internal/controller/clusterhealth*` / `healthcheck*`); document the verification in the PR test plan

**Checkpoint**: SC-002 satisfied — all stories independently verified.

## Phase 6: Polish & cross-cutting

- [x] T012 [P] Document the ratio thresholds in `docs/guides/addon-checks.md`: reserved keys, value grammar, strict-exceed semantics, rollup report entry, worked example from contracts/ratio-rollup.md; state plainly that per-resource results are unchanged and ratios only affect rollups (constitution IX)
- [x] T013 [P] Note reserved engine-level keys in `docs/authoring-adapters.md`: adapters never validate or consume `warnRatio`/`failRatio`; `ThresholdAdvertiser` sets need not include them
- [x] T014 Full local gate: `go -C tools tool task fmt` + `lint` + `vet` + `staticcheck` + `test`; coverage ratchet holds for `pkg/adapter` and `internal/controller`
- [ ] T015 (BLOCKED locally: Docker daemon not running, helmfile missing — recorded in PR test plan per AGENTS.md) Full e2e: `go -C tools tool task test-e2e` (mandatory — touches `internal/controller/*` and `pkg/adapter/*`); execute quickstart.md scenarios 2–5 against the kind cluster where practical; record outcomes in the PR test plan

## Dependencies

- Phase 2 blocks everything: T005–T009 need T001/T003; tests T002/T004 can land with their implementation tasks.
- US1 (T005–T007) blocks US2 (T008–T009) — same aggregation code paths.
- US3 (T010–T011) depends only on Phase 2 + T005 landing (needs the final aggregation shape to regress against).
- Polish (T012–T015) last; T012/T013 can start any time after Phase 2 stabilizes the contract.

## Parallel execution examples

- T001 ∥ T002 (same PR, different files; T002 red until T001 lands — fine within a branch).
- T003+T004 ∥ T001+T002 (different files; T003 imports T001's symbols, so sequence T001 → T003 within one worker, or stub first).
- After T007: T008 ∥ T010 ∥ T012 ∥ T013.

## Implementation strategy

MVP = Phase 2 + Phase 3 (US1): the ratio engine, validation, family-aware
aggregation, and rollup entries — enough to demonstrate the headline behavior
end-to-end. US2 is test-heavy verification of the same engine; US3 is
regression armor; Polish closes docs and the mandatory e2e gate. Everything
lands as one focused PR on `feature/159-quorum-ratio-rollups` per repo
convention.
