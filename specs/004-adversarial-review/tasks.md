# Tasks: Adversarial Codebase Review for the v0.5.0 Release Gate

**Input**: Design documents from `/specs/004-adversarial-review/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/findings-report.md, quickstart.md

**Tests**: Not applicable as separate test tasks — this feature is a review process. Regression tests are embedded in every fix disposition (FR-006), and quickstart.md is the end-to-end validation, executed in the final phase.

**Organization**: Tasks are grouped by user story. US1 = the adversarial review producing ranked confirmed findings; US2 = disposition of confirmed findings; US3 = coverage statement.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

## Path Conventions

Review deliverables live under `specs/004-adversarial-review/review/`. Fixes land in the existing source tree (`api/`, `internal/`, `pkg/`, `config/`, `.github/workflows/`, …) via separate focused PRs, one per finding.

---

## Phase 1: Setup (Gate Preconditions)

**Purpose**: Verify the gate may start and pin the review subject.

- [X] T001 Verify the v0.5.0 milestone precondition (`gh issue list --milestone v0.5.0 --state open` shows only #217) and record the anchor commit (`git rev-parse origin/main`); note both in a new `specs/004-adversarial-review/review/README.md` alongside the review start date (research R4)
- [X] T002 Scaffold the three deliverable files with their contract headers per `specs/004-adversarial-review/contracts/findings-report.md`: `specs/004-adversarial-review/review/findings.md`, `specs/004-adversarial-review/review/refuted.md`, `specs/004-adversarial-review/review/coverage.md` (anchor commit + five perspectives filled in)

---

## Phase 2: Foundational (Baseline Evidence)

**Purpose**: Tool-assisted evidence that seeds every perspective (research R5). BLOCKS the perspective passes because their candidate lists must incorporate tool output.

- [ ] T003 [P] Run `go -C tools tool task ci` at the anchor commit; record pass/fail and any warnings as evidence seeds in `specs/004-adversarial-review/review/README.md`
- [X] T004 [P] Run `go -C tools tool task crd-compat` at the anchor commit; record the result and any allowlist entries consulted in `specs/004-adversarial-review/review/README.md`

**Checkpoint**: Anchor recorded, deliverables scaffolded, baseline gates recorded — perspective passes can start in parallel.

---

## Phase 3: User Story 1 — Multi-Perspective Adversarial Review Produces a Ranked Findings List (Priority: P1) 🎯 MVP

**Goal**: Five independent perspective passes over the whole repository at the anchor commit, adversarial refutation of every candidate, and the ranked confirmed-findings deliverable.

**Independent Test**: `review/findings.md` exists with every entry carrying severity, file:line, failure scenario, and a refutation record; `review/refuted.md` lists every discarded candidate with its reason; each perspective's result (findings or explicit "no confirmed findings") is recorded.

### Perspective passes (independent — each writes only its own candidate file; do not read sibling candidate files, per research R2)

- [ ] T005 [P] [US1] Security perspective pass over `internal/nodecert/`, `internal/probe/`, `cmd/probe/`, `cmd/node-agent/`, `internal/app/` (serving/TLS/leader-election) and CR-spec input handling; write candidates (ID `SEC-n`, location, suspected severity, failure scenario) to `specs/004-adversarial-review/review/candidates-security.md`
- [ ] T006 [P] [US1] Correctness/reconcile-time perspective pass over `internal/controller/`, `internal/adapter/`, `internal/app/run.go` (timeouts, requeue, idempotency, bounded work, status semantics); write candidates (ID `COR-n`) to `specs/004-adversarial-review/review/candidates-correctness.md`
- [ ] T007 [P] [US1] API/CRD contract perspective pass over `api/v1alpha1/`, `pkg/adapter/`, CRD generation inputs, and the `ClusterHealth`-derived-only-from-`HealthCheck.status` rule; write candidates (ID `API-n`) to `specs/004-adversarial-review/review/candidates-api-contract.md`
- [ ] T008 [P] [US1] RBAC least-privilege perspective pass over `config/rbac/`, `+kubebuilder:rbac` markers in `internal/controller/`, node-agent DaemonSet privileges, and NetworkPolicy; write candidates (ID `RBAC-n`) to `specs/004-adversarial-review/review/candidates-rbac.md`
- [ ] T009 [P] [US1] Supply-chain/CI perspective pass over `.github/workflows/`, `Taskfile.yml`, `tools/` pinning, `scripts/`, `Dockerfile*`, and the release/bundle pipeline; write candidates (ID `SCM-n`) to `specs/004-adversarial-review/review/candidates-supply-chain.md`

### Consolidation and refutation

- [ ] T010 [US1] Merge cross-perspective duplicates across the five `specs/004-adversarial-review/review/candidates-*.md` files (first finder's ID survives; merged IDs noted) producing the unified candidate list appended to `specs/004-adversarial-review/review/README.md`
- [ ] T011 [US1] Adversarial refutation pass: for every candidate, re-read the actual code at the anchor commit and attempt to refute (unreachable path, existing guard, test coverage, misread semantics); mark each `confirmed`, `refuted`, or `contested` (research R2); write refuted candidates with reasons to `specs/004-adversarial-review/review/refuted.md`
- [ ] T012 [US1] Assemble the ranked deliverable `specs/004-adversarial-review/review/findings.md` per contract §1: confirmed findings sorted critical → high → medium → low with final severities per rubric R3, refutation records, contested markers, and `_No confirmed findings._` placeholders for empty severity sections; every disposition field initialized to `open`; delete the working `candidates-*.md` files or move their content under `review/README.md`

**Checkpoint**: Deliverable 1 complete — the ranked findings list is publishable even if no fixes follow (MVP).

---

## Phase 4: User Story 2 — Confirmed Critical/High Findings Are Fixed or Explicitly Deferred (Priority: P2)

**Goal**: Every confirmed critical/high finding gets exactly one disposition — a merged fix PR with a regression test, or a follow-up issue with written deferral rationale.

**Independent Test**: Cross-check `review/findings.md`: zero critical/high entries whose disposition is not `fixed (PR #NN)` or `deferred (issue #NN)`; each referenced PR is merged with CI green (and a linked e2e run where required); each referenced issue contains a "Deferral rationale" section.

- [ ] T013 [US2] Triage every confirmed critical/high finding in `specs/004-adversarial-review/review/findings.md` into fix-in-milestone vs defer, applying FR-007 (any fix requiring a CRD-schema or `ClusterHealth`-contract break is deferred); record the decision per finding in the Disposition field (`fix-in-progress` / `deferred-pending-issue`)
- [ ] T014 [US2] For each finding triaged fix-in-milestone: implement on a dedicated `fix/<finding-id>-<slug>` branch — regression test first (fails at the anchor behavior), then the fix in the affected source path, `go -C tools tool task ci` green, `go -C tools tool task crd-compat` green, conventional-commit + DCO, focused PR referencing #217; repeat until all fix-in-progress findings have open PRs (research R6)
- [ ] T015 [US2] For each fix PR touching AGENTS.md e2e-mandatory surfaces (`internal/app/run.go`, `internal/adapter/*/adapter.go`, `internal/controller/*`, `pkg/adapter/*`, `api/v1alpha1/*_types.go`, `internal/probe/*`, `internal/nodecert/*`, `cmd/node-agent/*`, `test/e2e/fixtures/*`): confirm the `kind e2e` CI workflow ran green on the PR and link the run in the PR description (research R7 — local e2e unavailable)
- [ ] T016 [P] [US2] For each finding triaged defer: open a follow-up GitHub issue per contract §4 (title `<severity>: <finding title> (adversarial review #217, <ID>)`, labels, location, failure scenario, refutation record, Deferral rationale section) and link it from #217
- [ ] T017 [US2] Shepherd all fix PRs to merge; after each merge, update the finding's Disposition in `specs/004-adversarial-review/review/findings.md` to `fixed (PR #NN)` with evidence line (regression test, `task ci`, e2e run link when required); set deferred findings to `deferred (issue #NN)`
- [ ] T018 [US2] Add triage notes for every confirmed medium/low finding in `specs/004-adversarial-review/review/findings.md` (`triaged — <note>`: fixed opportunistically with PR ref, or recorded for future work), per FR-008
- [ ] T019 [US2] Handle post-anchor drift (research R4): list all commits on `main` after the anchor (`git log <anchor>..origin/main --oneline`) — for each, either re-examine the touched surface under the owning perspective (new candidates loop back through T011–T012) or record it for the coverage statement's delta table

**Checkpoint**: Every critical/high finding dispositioned; `review/findings.md` shows no `open` critical/high entries.

---

## Phase 5: User Story 3 — Coverage Statement Eliminates Silent Gaps (Priority: P3)

**Goal**: A written statement classifying every repository area as reviewed (with perspectives) or intentionally excluded (with reason).

**Independent Test**: An independent reader can classify any repo file as "reviewed" or "explicitly excluded" using `review/coverage.md` alone; perspectives with zero confirmed findings appear with the explicit result.

- [ ] T020 [US3] Fill `specs/004-adversarial-review/review/coverage.md` per contract §3: Reviewed table (area → perspectives applied → result, including explicit "no confirmed findings"), Intentionally excluded table (generated artifacts: `zz_generated*.go`, `config/crd/bases`, OLM bundle metadata, `docs/reference/api.md`, `graphify-out/` — generation-input review only; plus any other exclusions with reasons), and the Post-anchor deltas table from T019
- [ ] T021 [US3] Verify SC-004 completeness: enumerate top-level paths (`ls` at repo root plus `.github/`) and confirm each appears exactly once across the Reviewed/Excluded tables in `specs/004-adversarial-review/review/coverage.md`; fix omissions

**Checkpoint**: All three deliverables complete.

---

## Phase 6: Polish & Gate Closure

**Purpose**: End-to-end validation, publication, and closing #217 last on the milestone (FR-010).

- [ ] T022 Run the full validation in `specs/004-adversarial-review/quickstart.md` steps 1–5 (milestone precondition, deliverable contract conformance, disposition completeness, fix quality gates including final `go -C tools tool task ci` + `crd-compat`, refutation record) and fix any failures
- [ ] T023 Merge the review-deliverables PR (branch `feature/004-adversarial-review`, PR #251) so `specs/004-adversarial-review/review/` is reachable from `main`
- [ ] T024 Post the closing comment on issue #217 per contract §5 (links to merged deliverables, disposition table for all critical/high findings, perspective completion summary, CI-green statement), then close #217 and verify via `gh issue list --milestone v0.5.0 --state open` (empty) that it closed last (quickstart step 6)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately
- **Foundational (Phase 2)**: needs T001 (anchor) — BLOCKS perspective passes
- **US1 (Phase 3)**: needs Phase 2; T005–T009 in parallel → T010 → T011 → T012
- **US2 (Phase 4)**: needs T012 (confirmed findings list); T013 → {T014, T016 in parallel} → T015 → T017; T018 anytime after T012; T019 after T014/T017 (drift includes this gate's own merges)
- **US3 (Phase 5)**: T020 needs T005–T012 results and T019's delta list; T021 after T020
- **Polish (Phase 6)**: T022 needs all of US1–US3; T023 after T022; T024 after T023 and after every fix PR from T017 is merged

### User Story Dependencies

- **US1 (P1)**: independent once Phase 2 completes — the MVP
- **US2 (P2)**: consumes US1's confirmed findings; its dispositions are independent of US3
- **US3 (P3)**: summarizes US1's scope + US2's drift handling; does not depend on dispositions completing (but final numbers land after US2)

### Parallel Opportunities

- T003 ∥ T004 (independent tool gates)
- T005 ∥ T006 ∥ T007 ∥ T008 ∥ T009 — the core fan-out: five perspectives, five separate candidate files, no shared state (independence is required, not just allowed)
- Within US2: individual fix branches from T014 are mutually independent (different findings, different files — verify no two fixes touch the same file before parallelizing); T016 deferral issues ∥ fix work
- T018 ∥ T014–T017 (medium/low triage notes touch a different section of findings.md — serialize the file writes)

## Parallel Example: User Story 1

```text
# After T003/T004, launch all five perspective passes together:
Task: "Security perspective pass → review/candidates-security.md"
Task: "Correctness perspective pass → review/candidates-correctness.md"
Task: "API/CRD contract perspective pass → review/candidates-api-contract.md"
Task: "RBAC perspective pass → review/candidates-rbac.md"
Task: "Supply-chain/CI perspective pass → review/candidates-supply-chain.md"
# Then serialize: merge duplicates (T010) → refute (T011) → assemble findings.md (T012)
```

## Implementation Strategy

### MVP First (US1 only)

1. Phase 1 + Phase 2 (anchor, scaffolds, baseline gates)
2. Phase 3 completely (five passes → refutation → ranked findings)
3. **STOP and VALIDATE**: findings.md + refuted.md satisfy contract §§1–2 — this alone is an honest release-risk picture
4. Proceed to dispositions

### Incremental Delivery

1. US1 → publishable findings list (MVP)
2. US2 → each fix PR merges independently; the gate tightens with every disposition
3. US3 → coverage statement finalized once drift is known
4. Phase 6 → quickstart validation, merge deliverables, close #217 last

### Sizing Note

T014 is deliberately a loop task: the number of fix PRs equals the number of critical/high findings triaged fix-in-milestone, unknowable until T013. Each iteration is a self-contained branch/test/fix/PR cycle following the identical recipe, so no per-finding task IDs are pre-allocated.

---

## Notes

- Perspective independence (T005–T009) is a correctness requirement of the method (FR-002) — run them without cross-reading candidate files
- Every fix commit: conventional commit type (`fix:`/`feat:` as appropriate), `Signed-off-by`, one logical change
- Never lower coverage thresholds to make a fix PR pass; never hand-edit generated files — re-run the pinned tasks
- Contract-breaking fixes are always deferred (FR-007), no exceptions inside this milestone
