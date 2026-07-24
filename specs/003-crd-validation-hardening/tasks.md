# Tasks: Pre-1.0 CRD Validation Hardening

**Input**: Design documents from `specs/003-crd-validation-hardening/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Included — FR-012 and the constitution require direct test coverage
for every new validation rule (envtest admission matrices, clamp unit tests,
gate fixture tests).

**Organization**: Grouped by user story. The three stories are fully
independent — any one can ship alone as an increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no dependency on an incomplete task)
- **[US1/US2/US3]**: maps to the spec's user stories

## Phase 1: Setup

**Purpose**: Confirm a clean baseline so schema diffs and test failures are attributable to this feature.

- [X] T001 Verify clean baseline: run `go -C tools tool task manifests` then `go -C tools tool task test` at branch tip and confirm green with no generated drift (`git status` clean under `config/crd/bases/` and `docs/reference/`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: None required — the three stories share no code and touch disjoint files. The floor constants (US1) are used only by US1's CEL and clamp.

*(no tasks — proceed directly to any user story)*

**Checkpoint**: Baseline verified — US1, US2, US3 can start in parallel.

---

## Phase 3: User Story 1 - Reject dangerously short check cadences at admission (Priority: P1) 🎯 MVP

**Goal**: `interval < 10s` or `timeout < 1s` rejected at apply time on both
kinds; stored sub-floor objects clamped at runtime with a Warning Event +
`Accepted=True/SpecClamped` condition. Contracts:
[crd-validation.md](contracts/crd-validation.md) (floors section),
[clamp-signal.md](contracts/clamp-signal.md).

**Independent Test**: quickstart.md §2 (apply below/at/above-floor manifests →
rejection/acceptance) and §4 (clamp signal on a legacy object).

### Implementation for User Story 1

- [X] T002 [P] [US1] Add exported floor constants `MinCheckInterval = 10 * time.Second` and `MinCheckTimeout = time.Second` with intent comments in new file `api/v1alpha1/validation.go` (SPDX header per `hack/boilerplate.go.txt`)
- [X] T003 [US1] Replace the `> duration('0s')` XValidation rules with `>= duration('10s')` (interval) and `>= duration('1s')` (timeout) on `AddonCheckSpec` in `api/v1alpha1/addoncheck_types.go`, keeping the `timeout <= interval` rule; update the field doc comments to state the floors (depends on T002 for the documented constant names)
- [X] T004 [P] [US1] Same floor-rule replacement and doc-comment update on `NodeCertificateCheckSpec` in `api/v1alpha1/nodecertificatecheck_types.go`
- [X] T005 [US1] Regenerate: `go -C tools tool task manifests` and `go -C tools tool task docs:api-ref`; commit regenerated `config/crd/bases/*.yaml` and `docs/reference/api.md` (depends on T003, T004)
- [X] T006 [P] [US1] Schema↔constant drift test in `api/v1alpha1/validation_test.go` (external `v1alpha1_test` package): parse the generated `config/crd/bases/fathom.skaphos.io_addonchecks.yaml` and `..._nodecertificatechecks.yaml` and assert the CEL rule strings embed the durations from `MinCheckInterval`/`MinCheckTimeout` (depends on T005)
- [X] T007 [P] [US1] envtest admission matrix for AddonCheck floors in `internal/controller/addoncheck_admission_test.go`: table-driven create/update cases per the contract (1ms/9s/10s/unset interval; 500ms/999ms/1s timeout; `timeout: 5s, interval: 5m` accepted; cross-field rule still enforced; rejection messages name field + minimum) (depends on T005)
- [X] T008 [P] [US1] envtest admission matrix for NodeCertificateCheck floors in `internal/controller/nodecertificatecheck_admission_test.go`, same boundary structure plus proof existing rules (paths allowlist, warnDays ≥ criticalDays) still hold (depends on T005)
- [X] T009 [US1] Clamp in AddonCheck cadence helpers `addonCheckInterval`/`addonCheckTimeout` in `internal/controller/addoncheck_controller.go`: raise set-but-sub-floor values to `v1alpha1.MinCheckInterval`/`MinCheckTimeout`; reconciler emits Warning Event reason `CadenceClamped` and sets `Accepted=True` reason `SpecClamped` with the contract's message format (field, configured, effective); `InvalidPolicy` still outranks `SpecClamped`; unit tests for helpers + reconciler-level Event/condition assertions in `internal/controller/addoncheck_controller_test.go`
- [X] T010 [P] [US1] Same clamp + Event/condition in `internal/controller/nodecertificatecheck_controller.go` (`nodeCertInterval`/`nodeCertTimeout`), ensuring the clamped interval feeds the node-agent DaemonSet `--interval` argument; tests in `internal/controller/nodecertificatecheck_controller_test.go`
- [X] T011 [P] [US1] Samples-regression envtest spec in `internal/controller/samples_admission_test.go`: decode and create every `config/samples/fathom_v1alpha1_*.yaml`, assert all admit unchanged (FR-008; also guards US2 automatically once its rules land) (depends on T005)
- [X] T012 [P] [US1] e2e admission smoke spec (core tier, no addon needed) in `test/e2e/crd_validation_test.go`: apply a sub-floor AddonCheck against the kind cluster, assert API-server rejection message (depends on T005)

**Checkpoint**: US1 fully functional — floors enforced at admission, clamp observable, samples still admit.

---

## Phase 4: User Story 2 - Catch invalid policy configuration at admission (Priority: P2)

**Goal**: Structurally invalid `AddonCheck.spec.policy` (bad namespace names,
oversized lists/maps, malformed family keys, non-numeric numeric thresholds,
broken selectors) rejected at apply time; reconcile-time validation remains
the semantic backstop. Contract:
[crd-validation.md](contracts/crd-validation.md) (policy section);
rules: [data-model.md](data-model.md).

**Independent Test**: quickstart.md §3 (apply each invalid-policy manifest →
rejection; underscore family keys and all shipped samples still admit).

### Implementation for User Story 2

- [X] T013 [US2] Policy map bounds + key format in `api/v1alpha1/addoncheck_types.go`: `MaxProperties=32` on `Policy` with CEL key rule `self.all(k, k.matches('^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$'))` (message naming the key format); on `AddonCheckFamilyPolicy.Namespaces` add `MaxItems=64`, `items:MaxLength=63`, `items:Pattern` DNS-1123 label; on `Thresholds` add `MaxProperties=16` + CEL key-shape rule; update doc comments
- [X] T014 [US2] Numeric threshold value CEL on `AddonCheckFamilyPolicy.Thresholds` in `api/v1alpha1/addoncheck_types.go`: `warnDays`/`failDays` → `^[0-9]{1,4}$`, `warnRatio`/`failRatio` → decimal-in-[0,1] pattern; unknown keys pass; doc comment lists the admission-validated keys (same file as T013 — sequential)
- [X] T015 [US2] Label-selector structural CEL on `AddonCheckFamilyPolicy.LabelSelector` in `api/v1alpha1/addoncheck_types.go`: operator ∈ {In,NotIn,Exists,DoesNotExist}; In/NotIn need non-empty values; Exists/DoesNotExist need empty values. If controller-gen or the envtest API server rejects the rule on cost, drop ONLY this rule, note the fallback in the field comment, and record it for T017's doc split (R6 fallback; same file — sequential)
- [X] T016 [US2] Regenerate: `go -C tools tool task manifests` and `go -C tools tool task docs:api-ref`; commit regenerated CRDs + API reference (depends on T013–T015)
- [X] T017 [US2] envtest policy admission matrix in `internal/controller/addoncheck_policy_admission_test.go` per the contract table: 33-family reject / underscore family accept / `Certificates`+`-bad`+64-char keys reject / 65 namespaces reject / `Prod_NS` reject / `kube-system` accept / 17 thresholds reject / `warnDays: banana` reject / `warnDays: "30"` accept / `failRatio: "1.5"` reject / `warnRatio: "0.9"` accept / unknown key accept / selector operator cases (or reconcile-time assertions if T015 fell back) / empty policy accept; plus one reconcile-time case proving unknown-family still lands as `Accepted=False/InvalidPolicy` (depends on T016)
- [X] T018 [P] [US2] Verify e2e fixtures with policies still admit: run the samples-regression spec from T011 plus `kubectl apply --dry-run=server` sweep of `test/e2e/fixtures/` manifests containing `policy:`; fix any fixture that was exercising now-rejected values or flag it as a deliberate negative fixture (depends on T016)
- [X] T019 [P] [US2] Document the admission/reconcile enforcement split (FR-007): README validation section + doc comments already updated in T013–T015; state exactly which checks are admission-time vs `Accepted`-condition-time in `README.md` (depends on T017 outcome for the selector-rule split)

**Checkpoint**: US1 + US2 both enforced at admission; all samples/fixtures green.

---

## Phase 5: User Story 3 - Block accidental incompatible schema changes in CI (Priority: P3)

**Goal**: Every PR diffs `config/crd/bases` against the latest release tag via
pinned crdify; unsanctioned incompatibilities fail CI naming CRD/field/change;
the committed allowlist is the auditable override. Contract:
[schema-compat-gate.md](contracts/schema-compat-gate.md).

**Independent Test**: quickstart.md §5 (gate green on this branch with
`SANCTIONED` findings; deleting a field → exit 1; fixture matrix test passes).

### Implementation for User Story 3

- [X] T020 [P] [US3] Pin `sigs.k8s.io/crdify` v0.6.0 as a tool directive in `tools/go.mod` (`go -C tools get -tool sigs.k8s.io/crdify@v0.6.0`); verify `go -C tools tool crdify --help` runs
- [X] T021 [P] [US3] Gate script `scripts/check-crd-compat.sh` implementing the contract algorithm: baseline = highest `v*` semver tag (exit 0 + notice when none); per-CRD `git show <tag>:<path>` → crdify old vs new; new CRDs skipped with notice; findings matched against allowlist → `SANCTIONED` (exit 0, reason+issue shown) or `INCOMPATIBLE` (exit 1, CRD + property path + change + allowlist pointer); unmatched allowlist entries → `STALE` warning; SPDX header; executable bit
- [X] T022 [P] [US3] Seeded allowlist `.crd-compat-allowlist.yaml` at repo root: entries for this feature's tightenings (AddonCheck + NodeCertificateCheck interval/timeout floors; AddonCheck policy bounds/patterns), each with `crd`, `path`, `reason`, `issue: …/issues/152`; header comment documenting format + pruning rule per data-model.md (depends on T021 for exact crdify path strings — write together)
- [X] T023 [US3] Fixture matrix test `scripts/check-crd-compat_test.sh`: synthetic old/new CRD pairs + temp allowlist exercising the contract's pass/fail matrix (no change / added optional field / new CRD / removed field / tightened validation / allowlisted / stale entry); runnable locally without tags (depends on T021)
- [X] T024 [US3] Taskfile task `crd-compat` wrapping the script (pattern-match existing script-wrapping tasks) in `Taskfile.yml`; add task to the command list in `AGENTS.md` (depends on T021)
- [X] T025 [US3] CI job `crd-compat` in `.github/workflows/ci.yml`: checkout with `fetch-depth: 0` (tags needed), setup-go, run `go -C tools tool task crd-compat` and `scripts/check-crd-compat_test.sh`; pin action SHAs matching existing jobs (depends on T024)
- [X] T026 [P] [US3] Update `docs/reference/api-versioning.md` "Enforcement and tooling": replace the "Recommended:" bullet with a description of the implemented gate, the allowlist override, and the pruning lifecycle (depends on T021–T022 for accurate description)

**Checkpoint**: Gate green on this branch with this feature's own changes visibly `SANCTIONED`.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation closure, full verification, PR readiness.

- [X] T027 [P] `README.md` user-facing validation notes: cadence floors (10s/1s), clamp behavior for pre-existing objects, policy structural limits table (complements T019's enforcement-split section)
- [X] T028 [P] Confirm coverage: `go -C tools tool task test` + `./scripts/check-coverage.sh coverage.out` — new packages/files meet the per-package minimum; ratchet only upward
- [X] T029 Full local CI: `go -C tools tool task ci` (lint, test, staticcheck, vuln, build) green
- [X] T030 Full e2e: `go -C tools tool task test-e2e` (mandatory — `api/v1alpha1/*_types.go` changed; core tier includes T012's admission smoke); record outcome in PR test plan
- [X] T031 Walk `specs/003-crd-validation-hardening/quickstart.md` §§1–5 end-to-end; fix any drift between docs and behavior; mark PR ready for review with the exact checks run

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none — start immediately
- **Foundational (Phase 2)**: empty — no blocker between stories
- **US1 / US2 / US3 (Phases 3–5)**: each depends only on T001; mutually independent
  - US1 and US2 both edit `api/v1alpha1/addoncheck_types.go` and regenerate CRDs — within a single working copy run them sequentially (US1 first) or rebase; conceptually independent
  - US3 touches no Go API/controller files; fully parallel with US1/US2. Note: until a US1/US2 schema change exists on the branch, the gate simply reports "compatible" — the seeded allowlist entries (T022) show as `STALE` warnings until the corresponding tightenings land, which the fixture test (T023) covers deterministically either way
- **Polish (Phase 6)**: after all three stories

### Within Each User Story

- US1: T002 → T003/T004 → T005 → {T006, T007, T008, T011, T012} ∥; T009/T010 need T002 only (clamp is schema-independent) but their reconciler tests read generated CRDs → after T005
- US2: T013 → T014 → T015 (same file) → T016 → {T017, T018} ∥ → T019
- US3: {T020, T021} ∥ → T022 → {T023, T024, T026} ∥ → T025

### Parallel Opportunities

- After T005: five US1 test tasks (T006–T008, T011, T012) in parallel
- After T016: T017 and T018 in parallel
- US3 start (T020, T021) in parallel with all of US1/US2
- Polish: T027 and T028 in parallel

## Parallel Example: User Story 1

```bash
# After T005 (regenerated CRDs committed), launch together:
Task: "Schema↔constant drift test in api/v1alpha1/validation_test.go"
Task: "envtest floor matrix in internal/controller/addoncheck_admission_test.go"
Task: "envtest floor matrix in internal/controller/nodecertificatecheck_admission_test.go"
Task: "samples-regression spec in internal/controller/samples_admission_test.go"
Task: "e2e admission smoke in test/e2e/crd_validation_test.go"
```

## Implementation Strategy

### MVP First (User Story 1 Only)

1. T001 baseline → Phase 3 complete (T002–T012)
2. **STOP and VALIDATE**: quickstart §§1–2 + envtest green — floors enforced, clamp observable
3. This alone closes the issue's cluster-safety bullet and is shippable

### Incremental Delivery

1. US1 → validate → shippable (hot-loop protection)
2. US2 → validate → shippable (policy black hole closed)
3. US3 → validate → shippable (compensating control for 1.0)
4. Polish → full ci + e2e → PR ready

### Single-Branch Note

This feature ships as one PR (branch `feature/152-crd-validation-hardening`,
draft PR #244); checkpoints are commit boundaries, not separate PRs. The
issue's "1.0 requires" framing wants all three slices before the schema is
declared stable.
