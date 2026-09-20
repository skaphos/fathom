<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Tasks: Complete AddonDefinition Design RFC

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[data-model.md](data-model.md), and [review contract](contracts/rfc-review.md).

This is documentation and review work. Tasks do not authorize runtime changes
or accept proposed architecture. Approval, merge, an accepted ADR, and the actual
existing-epic update are required for feature completion.

## Phase 1: Setup

- [X] T001 Confirm the branch baseline against research commit `92c5d8bc44df72596703e92652be9df20cfeb333` and record any changed evidence in specs/011-addon-definition-rfc/research.md.
- [X] T002 Read the checklist gate without modifying it, then run the prerequisite and document checks in specs/011-addon-definition-rfc/quickstart.md with `SPECIFY_FEATURE_DIRECTORY=specs/011-addon-definition-rfc`; record missing dependencies and check outcomes in specs/011-addon-definition-rfc/execution.md.

## Phase 2: Foundational evidence

Complete before drafting any story. No new architecture is accepted here.

- [X] T003 Refresh source evidence for client fallback, discovery identity, actual operator grants, registry mutation, evidence freshness, evaluator buckets, and ratio compatibility in specs/011-addon-definition-rfc/research.md; verify external claims against the linked Kubernetes primary sources.
- [X] T004 Establish six Proposed decision records and distinguish current behavior from recommendations in docs/rfc/0001-addondefinition-crd.md; preserve ADR 0001 and document review ownership, eventual acceptance evidence, and the status quo alternative.

## Phase 3: US1 — Review a Complete Safety Boundary (P1)

**Goal**: A reviewer can trace a definition author's authority through every
evaluation read path and compare the four grant models.

**Independent validation**: Walk the first six authorization scenarios in the
review contract, including disabled metrics and local kubeconfig execution.

- [X] T005 [US1] Replace the impersonation-only safety claim with an explicit administrator-controlled definition-to-identity binding proposal in docs/rfc/0001-addondefinition-crd.md; specify author/admin permissions, binding ownership, identity recreation, and revocation.
- [X] T006 [US1] Compare pregrants, separate administrator grants, bounded operator grants, and publisher admission in docs/rfc/0001-addondefinition-crd.md, recording privilege delta, installation cost, strongest objections, and an observable acceptance example.
- [X] T007 [US1] Trace discovery, addon and core reads through the declared identity in docs/rfc/0001-addondefinition-crd.md; enforce the conceptual constraint “Missing binding, identity-scoped discovery, or scoped client must fail closed” for in-cluster and local execution.
- [X] T008 [US1] Decide requested-permission semantics and minimum-request versus effective-union diagnostics in docs/rfc/0001-addondefinition-crd.md; explain additive grants, access-review limitations, denied/error/unknown outcomes, and permissions independent of metrics configuration.
- [X] T009 [US1] Record scenario evidence for FR-004–006 in specs/011-addon-definition-rfc/contracts/rfc-review.md, including attempted borrowing of an already allowlisted identity and all fallback paths; unresolved consequential choices remain explicitly proposed for the decider.

## Phase 4: US2 — Predict Definition Lifecycle Outcomes (P1)

**Goal**: Every lifecycle event has one derivable active revision, authorization
outcome, evidence state, and recovery path.

**Independent validation**: Walk all lifecycle, collision, restart, in-flight,
and stale-evidence scenarios in the review contract without inventing behavior.

- [X] T010 [US2] Define canonical identity and global arbitration in docs/rfc/0001-addondefinition-crd.md: “Exact canonical identity claimed by this definition; no implicit aliases.” Compare name-based uniqueness, controller arbitration, and stateful admission; specify runtime duplicates, built-in collisions, upgrade behavior, enforcement points, and visible outcomes.
- [X] T011 [US2] Specify immutable UID/generation snapshots, separately changing authorization context, owner-aware removal, and publication fencing in docs/rfc/0001-addondefinition-crd.md; preserve “One evaluation uses exactly one definition revision and one authorization context” without promising an atomic RBAC snapshot or rollback of completed reads.
- [X] T012 [US2] Add the missing/add/edit/delete/recreate/invalid/restart/revoke/recover and in-flight outcome matrix to docs/rfc/0001-addondefinition-crd.md, including active revision, condition/reason, verdict, timestamps, and bounded retry/watch recovery for each row.
- [X] T013 [US2] Specify evidence age and freshness policy in docs/rfc/0001-addondefinition-crd.md: “Last-known evidence retains its original `observedAt` after deletion, invalidation, revocation, or missing input.” Distinguish `latestAttemptAt`, superseded outcomes, and current success; keep ClusterHealth derived only from HealthCheck.status.
- [X] T014 [US2] Record FR-008–011 scenario evidence in specs/011-addon-definition-rfc/contracts/rfc-review.md, including same-name/new-UID recreation, mismatched startup observations, and simultaneous revision/authorization changes.

## Phase 5: US3 — Implement from an Unambiguous Contract (P2)

**Goal**: The six proposals become a reviewed contract and an attributable
handoff to the existing implementation epic.

**Independent validation**: Map FR-001–016 to the final six decisions and verify
approval, merged RFC/ADR content, and the actual #280 update by readback.

- [X] T015 [US3] Define the first-release typed vocabulary, including APIService reuse, required fields, scope, cross-field validation, unknown-kind behavior, and admission/reconciliation split in docs/rfc/0001-addondefinition-crd.md; propose no new user expression language and settle CLI RBAC rendering and generated samples against documentation-only alternatives.
- [X] T016 [US3] Specify numeric bounds with rationale and enforcement points for every string, collection, nesting/work class, list page/object/response, result size, elapsed time, retry/backoff, and concurrency class in docs/rfc/0001-addondefinition-crd.md; inventory existing runtime expression-bearing inputs separately from admission CEL, stating numeric input/cost limits and enforcement or an explicit non-applicability/no-amplification rationale for each class (FR-003/SC-004); excluding a new expression language does not discharge this inventory. Define deterministic multi-limit failure and fair progress for unrelated checks.
- [X] T017 [US3] Separate schema, evaluator semantics, addon version and Go adapter compatibility in docs/rfc/0001-addondefinition-crd.md; resolve or explicitly defer #256 with owner, dependency and resolution condition, including previously user-owned ratio-key semantics rather than merely version acceptance.
- [X] T018 [US3] Complete alternatives, costs, observable examples and deferrals for all six decisions, plus first-release non-goals, rollout, reversibility, prior art, and the proposed relationship to immutable ADR 0001 in docs/rfc/0001-addondefinition-crd.md.
- [X] T019 [US3] Map all FR-001–016 and every scenario to exact RFC sections in specs/011-addon-definition-rfc/contracts/rfc-review.md; record unresolved findings and evidence in specs/011-addon-definition-rfc/execution.md before opening review.
- [X] T020 [US3] Validate the review draft via specs/011-addon-definition-rfc/quickstart.md, verify skaphos Git identity and DCO before any commit, then open or update the branch's documentation PR; record its URL, reviewed revision, actual review dates, decider and required perspectives in docs/rfc/0001-addondefinition-crd.md and attach the PR to the task.
- [X] T021 [US3] Incorporate review findings in docs/rfc/0001-addondefinition-crd.md and obtain Shawn Stratton's explicit decision on the exact final proposal; record attributable acceptance evidence and reopen review for substantive revisions. Silence, authorship and merge are not acceptance.
- [X] T022 [US3] After acceptance, choose the next unused number under docs/adr/ and create the linked accepted decision record, stating whether startup-only loading is narrowly superseded; link it from docs/rfc/0001-addondefinition-crd.md without rewriting docs/adr/0001-in-process-adapter-contract.md.
- [X] T023 [US3] Complete required PR checks and merge through the repository workflow after recorded approval; read back the merged docs/rfc/0001-addondefinition-crd.md and new docs/adr/ record, and store revision-specific acceptance/merge evidence in specs/011-addon-definition-rfc/execution.md.
- [X] T024 [US3] Update existing GitHub epic #280 with merged RFC/ADR links, accepted scope, explicit deferrals, #256 disposition and required runtime e2e; read back the actual update, record evidence in specs/011-addon-definition-rfc/execution.md for the T026 follow-up PR; defer closing #278 until T027. Do not create a duplicate epic.

## Phase 6: Polish and completion verification

- [X] T025 Re-run whitespace, relative-link, template-marker and REUSE checks from specs/011-addon-definition-rfc/quickstart.md after substantive edits; record exact outcomes in specs/011-addon-definition-rfc/execution.md without implying runtime validation.
- [ ] T026 Publish the post-merge evidence and completed task statuses in specs/011-addon-definition-rfc/execution.md, contracts/rfc-review.md and tasks.md through a follow-up documentation PR from current main; verify identity and DCO before commits, run T025 checks, attach the PR, and merge after required checks/review. Preserve accepted RFC/ADR content. Record this follow-up PR's own check/merge evidence in its PR record, not another repository edit.
- [ ] T027 Read back the merged evidence from T026 and verify all completion links and requirement coverage in specs/011-addon-definition-rfc/contracts/rfc-review.md and execution.md; record this final task's completion in the follow-up PR record and then update or close #278. Make no further repository edits. Report any missing approval, merge or handoff evidence; clean up only merged, clean worktrees whose commits are reachable from upstream main.

## Dependencies and execution order

T001 → T002 → T003 → T004 → US1 (T005–T009) → US2 (T010–T014)
→ US3 draft (T015–T020) → decision (T021) → ADR (T022) → merge
(T023) → handoff (T024) → document checks (T025) → evidence PR (T026)
→ read-only repository verification and issue closure (T027).

Run T025's document checks before each publication as well as at completion.
US2 depends on US1's authority model. US3 incorporates both earlier stories;
their scenario walkthroughs remain independently reviewable.

## Parallel opportunities

No mutation tasks carry `[P]`: most update the same RFC or depend on prior
decisions. Independent read-only checks can be batched without shared edits:

- US1: inspect impersonation/client paths alongside generated RBAC and metrics grants.
- US2: inspect registry lifecycle alongside controller status/report timestamp behavior.
- US3: check relative links alongside whitespace/SPDX and issue/version evidence.

These are execution options, not an instruction to spawn agents.

## Implementation strategy

The first reviewable increment is US1's explicit safety boundary. Validate its
scenarios before building the lifecycle proposal on it. Add US2, then complete
schema/version/bounds and first-release scope in US3. Keep all six decisions
Proposed until the final revision receives explicit approval. The MVP draft is
not feature completion; accepted and merged RFC/ADR plus verified #280 handoff
and the merged evidence follow-up PR are required. CRD implementation and real-cluster tests remain #280 work.
