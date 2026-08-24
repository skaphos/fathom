<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Tasks: DNSCheck Completion

**Input**: Design documents from `specs/008-dnscheck-completion/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/healthcheck-target-projection.md`, `quickstart.md`

**Tests**: Required by the feature specification and repository constitution.
Write each listed behavior test first and confirm it fails for the intended
reason before implementing that behavior.

**Organization**: Shared target normalization is foundational. Subsequent
phases map directly to the four independently testable user stories.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel with adjacent marked tasks because it touches a
  different file and has no dependency on their unfinished work.
- **[Story]**: Maps the task to a user story in `spec.md`.

## Phase 1: Setup and Baseline

**Purpose**: Establish the existing AddonCheck wrapper and generated-contract
baseline before refactoring shared behavior.

- [X] T001 Run the existing HealthCheck envtest and generated-artifact baseline with `go -C tools tool task test` and `go -C tools tool task lint`, recording any pre-existing failure that affects `internal/controller/healthcheck_controller_test.go` or `api/v1alpha1/healthcheck_types.go` in `specs/008-dnscheck-completion/quickstart.md`

---

## Phase 2: Foundational Target Infrastructure

**Purpose**: Introduce the normalized target boundary while preserving the
existing AddonCheck contract. This phase blocks every user story.

**⚠️ CRITICAL**: No user-story implementation begins until the AddonCheck path
passes through the shared infrastructure without behavior changes.

- [X] T002 Add failing AddonCheck compatibility tests for normalized identity, snapshot replacement, empty API-version defaulting, cadence, summary truncation, and semantic no-op reconciliation in `internal/controller/healthcheck_target_handlers_test.go`
- [X] T003 Add API-version defaulting, normalized handler lookup, private target identity, normalized snapshot, handler descriptor/registry, and typed AddonCheck handler in `internal/controller/healthcheck_controller.go`
- [X] T004 Add failing shared watch-mapper tests for exact AddonCheck kind, effective namespace, and name matching in `internal/controller/healthcheck_watch_test.go`
- [X] T005 Refactor `SetupWithManager` and AddonCheck event mapping onto the handler registry and shared exact-reference mapper in `internal/controller/healthcheck_controller.go`
- [X] T006 Run the focused AddonCheck compatibility and watch tests through `go -C tools tool task test`, fixing only regressions attributable to `internal/controller/healthcheck_controller.go`, `internal/controller/healthcheck_target_handlers_test.go`, and `internal/controller/healthcheck_watch_test.go`

**Checkpoint**: Existing AddonCheck wrappers behave identically through the
new handler and watch infrastructure.

---

## Phase 3: User Story 1 — DNS contributes to cluster health (Priority: P1) 🎯 MVP

**Goal**: A DNSCheck verdict and evidence flow through HealthCheck into a
selecting ClusterHealth on a real cluster.

**Independent Test**: Create a DNSCheck, referencing HealthCheck, and selecting
ClusterHealth; observe Pass, change the DNS expectation to Fail, observe both
downstream transitions, and confirm unchanged DNS reconciliation does not add
history.

### Tests for User Story 1

- [X] T007 [P] [US1] Add failing DNSCheck projection tests covering empty and explicit-current API versions, default and explicit namespaces, result, observed time, report name, bounded Ready summary, and effective DNS cadence in `internal/controller/healthcheck_target_handlers_test.go`
- [X] T008 [P] [US1] Add failing exact DNSCheck source-event enqueue tests in `internal/controller/healthcheck_watch_test.go`
- [X] T009 [P] [US1] Add the failing real-cluster Pass-to-Fail DNSCheck → HealthCheck → ClusterHealth scenario, asserting HealthCheck result, bounded summary, source observation time, report name, matching ClusterHealth child result/summary/observed time, and unchanged-verdict HealthReport count in `test/e2e/dnscheck_aggregation_test.go`

### Implementation for User Story 1

- [X] T010 [US1] Implement the typed DNSCheck target handler using `dnsCheckInterval` and register its typed watch in `internal/controller/healthcheck_controller.go`
- [X] T011 [US1] Run the focused unit/envtest coverage for `internal/controller/healthcheck_target_handlers_test.go` and `internal/controller/healthcheck_watch_test.go` with `go -C tools tool task test`
- [X] T012 [US1] Run the core-tier e2e slice containing `test/e2e/dnscheck_aggregation_test.go` with `go -C tools tool task test-e2e E2E_ADDONS=core`, preserving the existing DNS resolution, restricted-policy, and lifecycle scenarios under `test/e2e/`

**Checkpoint**: DNSCheck is independently composable through ClusterHealth and
the real-cluster transition is proven.

---

## Phase 4: User Story 2 — Every advertised target kind works (Priority: P1)

**Goal**: The public HealthCheck contract advertises and projects exactly the
three specialized resources that exist.

**Independent Test**: Reference AddonCheck, DNSCheck, and
NodeCertificateCheck with the current API version and verify identical
normalized status semantics for all three; generated reference text names no
phantom kind.

### Tests for User Story 2

- [X] T013 [P] [US2] Add failing API marker/contract tests for the 317-character `apiVersion` bound, preserved checkRef immutability, and absence of a one-version enum in `api/v1alpha1/validation_test.go`
- [X] T014 [P] [US2] Add failing NodeCertificateCheck projection tests for result, observed time, report name, Ready summary, defaulted/clamped cadence, and an empty source status in `internal/controller/healthcheck_target_handlers_test.go`
- [X] T015 [P] [US2] Add failing exact NodeCertificateCheck source-event enqueue tests in `internal/controller/healthcheck_watch_test.go`

### Implementation for User Story 2

- [X] T016 [US2] Correct `CheckTargetRef` supported-kind documentation and add `MaxLength=317` to `APIVersion` without adding an enum in `api/v1alpha1/healthcheck_types.go`
- [X] T017 [US2] Implement the typed NodeCertificateCheck target handler using `nodeCertInterval` and register its typed watch in `internal/controller/healthcheck_controller.go`
- [X] T018 [US2] Add controller-local read-only RBAC markers for DNSCheck and NodeCertificateCheck in `internal/controller/healthcheck_controller.go`
- [X] T019 [US2] Regenerate CRDs, RBAC, deepcopy output, and the API reference with `go -C tools tool task manifests`, `go -C tools tool task generate`, and `go -C tools tool task docs:api-ref`, reviewing `config/crd/bases/fathom.skaphos.io_healthchecks.yaml`, `config/rbac/role.yaml`, `api/v1alpha1/zz_generated.deepcopy.go`, and `docs/reference/api.md` for exact contract and no net permission expansion
- [X] T020 [US2] Run the three-kind projection/watch and API validation coverage with `go -C tools tool task test` against `api/v1alpha1/validation_test.go`, `internal/controller/healthcheck_target_handlers_test.go`, and `internal/controller/healthcheck_watch_test.go`

**Checkpoint**: Every advertised kind has one typed handler, watch, and direct
test; generated contracts no longer claim nonexistent kinds.

---

## Phase 5: User Story 3 — Invalid references fail explicitly and safely (Priority: P2)

**Goal**: Invalid or missing references clear stale health, while transient API
failures preserve the last readable snapshot and retry.

**Independent Test**: Reconcile unsupported API version, unsupported kind,
NotFound, and injected transient read errors, then verify distinct Ready
reasons, snapshot action, bounded messages, retry behavior, and exact watch
matching.

### Tests for User Story 3

- [X] T021 [P] [US3] Add failing table-driven reconciliation tests for `UnsupportedAPIVersion`, `UnsupportedKind`, `TargetNotFound`, and `TargetLookupFailed` across applicable target handlers in `internal/controller/healthcheck_target_handlers_test.go`
- [X] T022 [P] [US3] Add failing watch tests proving API-version, kind, namespace, and name mismatches do not enqueue and source deletion does enqueue exact references in `internal/controller/healthcheck_watch_test.go`

### Implementation for User Story 3

- [X] T023 [US3] Implement unsupported API-version/kind rejection and shared terminal-versus-transient snapshot behavior with bounded Ready messages in `internal/controller/healthcheck_controller.go`
- [X] T024 [US3] Update status-reason documentation for unsupported API version/kind, missing targets, and transient lookup failures in `docs/reference/status-conditions.md`
- [X] T025 [US3] Run the invalid-reference and watch-isolation coverage with `go -C tools tool task test`, verifying `internal/controller/healthcheck_target_handlers_test.go` and `internal/controller/healthcheck_watch_test.go` pass without weakening existing tests in `internal/controller/healthcheck_controller_test.go`

**Checkpoint**: Every reference failure has deterministic, observable, and
safe snapshot semantics.

---

## Phase 6: User Story 4 — DNSCheck is straightforward to author and diagnose (Priority: P2)

**Goal**: Users can author DNS checks, connect them to aggregate health, and
troubleshoot common failures from one focused guide.

**Independent Test**: Copy the guide's cluster-resolver and explicit-resolver
examples, exercise positive and required-absence expectations, and trace status
through HealthCheck and ClusterHealth without consulting source code.

### Documentation for User Story 4

- [X] T026 [US4] Write copyable DNSCheck, HealthCheck, and ClusterHealth manifests plus resolver selection, positive/absence expectations, evidence interpretation, and troubleshooting in `docs/guides/dns-checks.md`
- [X] T027 [P] [US4] Add DNSCheck guide navigation and concise use-case wording in `docs/guides/README.md`
- [X] T028 [P] [US4] Update the wrapper/aggregation flow and supported-kind description, linking the DNS guide, in `docs/architecture.md`
- [X] T029 [P] [US4] Replace AddonCheck-only aggregation wording with the specialized-check → HealthCheck → ClusterHealth contract and add the DNS guide to user navigation in `README.md`
- [X] T030 [US4] Validate every documented field against generated `docs/reference/api.md` and every troubleshooting reason against `docs/reference/status-conditions.md`, correcting only `docs/guides/dns-checks.md` when examples drift

**Checkpoint**: The guide independently supports authoring and diagnosis of the
complete DNS aggregation chain.

---

## Phase 7: Polish and Cross-Cutting Verification

**Purpose**: Regenerate all contracts, run the full quality/e2e gates, and
leave the knowledge graph consistent with the implementation.

- [x] T031 Run formatting and deterministic generation with `go -C tools tool task fmt`, `go -C tools tool task docs:api-ref`, and `go -C tools tool task lint`, then verify no second-run drift in `api/v1alpha1/zz_generated.deepcopy.go`, `config/crd/bases/`, `config/rbac/`, and `docs/reference/api.md`
- [x] T032 Run `go -C tools tool task test`, `go -C tools tool task vet`, `go -C tools tool task staticcheck`, and `go -C tools tool task vuln`, resolving feature regressions in `api/v1alpha1/`, `internal/controller/`, and `test/e2e/`
- [ ] T033 Run the mandatory full-stack `go -C tools tool task test-e2e` suite for shared controller/watch changes, recording any unavailable Kind/Docker/Helm/Helmfile prerequisite in `specs/008-dnscheck-completion/quickstart.md`
- [x] T034 Run `reuse --no-multiprocessing lint`, `git diff --check`, and `graphify update .`, then review updated `graphify-out/` artifacts for only relationships caused by `api/v1alpha1/healthcheck_types.go`, `internal/controller/healthcheck_controller.go`, tests, and documentation

---

## Dependencies and Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependency.
- **Foundation (Phase 2)**: Depends on T001 and blocks every user story.
- **US1 (Phase 3)**: Depends on Phase 2; this is the MVP and proves the missing
  DNS composition path.
- **US2 (Phase 4)**: Depends on Phase 2. It can be developed alongside US1
  after the shared handler boundary stabilizes, but integration should follow
  US1 so both touchpoints in `healthcheck_controller.go` are serialized.
- **US3 (Phase 5)**: Depends on the complete three-handler registry from US2.
- **US4 (Phase 6)**: Depends on the final public/reason contracts from US2 and
  US3; draft authoring can begin after Phase 2.
- **Polish (Phase 7)**: Depends on all selected user stories.

### User Story Dependency Graph

```text
Setup → Foundation → US1 (DNS aggregation MVP)
                   └→ US2 (all real kinds) → US3 (failure safety)
                                          └→ US4 (guide, after US3 reasons)

US1 + US2 + US3 + US4 → Polish/full e2e
```

### Within Each User Story

- Add behavior tests first and confirm the intended failure.
- Implement typed projection before relying on its watch in a real manager.
- Run focused validation at the story checkpoint.
- Regenerate artifacts only through pinned task wrappers.
- Do not edit generated CRDs, deepcopy output, RBAC, or API reference by hand.

## Parallel Opportunities

- T007, T008, and T009 can run in parallel after Phase 2 because they create or
  modify different test files.
- T013, T014, and T015 can run in parallel after Phase 2.
- T021 and T022 can run in parallel after US2.
- T027, T028, and T029 can run in parallel after the DNS guide outline is stable.
- US1 test authoring and US2 API test authoring can overlap, but implementation
  edits to `internal/controller/healthcheck_controller.go` must be serialized.

## Parallel Example: User Story 1

```text
Task: "Add DNS projection tests in internal/controller/healthcheck_target_handlers_test.go"
Task: "Add DNS watch tests in internal/controller/healthcheck_watch_test.go"
Task: "Add DNS aggregation e2e in test/e2e/dnscheck_aggregation_test.go"
```

## Parallel Example: User Story 2

```text
Task: "Add CheckTargetRef validation tests in api/v1alpha1/validation_test.go"
Task: "Add NodeCertificateCheck projection tests in internal/controller/healthcheck_target_handlers_test.go"
Task: "Add NodeCertificateCheck watch tests in internal/controller/healthcheck_watch_test.go"
```

## Implementation Strategy

### MVP First

1. Complete Setup and Foundation.
2. Complete US1 tests, DNS handler/watch, and scoped real-cluster validation.
3. Stop and demonstrate DNSCheck Pass/Fail propagation through ClusterHealth.

This MVP closes the user-visible DNS composition gap while preserving the
existing AddonCheck path. It does not yet claim the full advertised-kind and
invalid-reference cleanup required for release completion.

### Incremental Delivery

1. Foundation: private handler/mapper boundary with AddonCheck parity.
2. US1: DNS composition and real-cluster proof.
3. US2: NodeCertificateCheck plus accurate generated API contract.
4. US3: complete negative-path safety and reason documentation.
5. US4: authoring/troubleshooting workflow.
6. Polish: deterministic generation, full CI-quality checks, full e2e, REUSE,
   and graph refresh.

## Notes

- Every task includes the concrete repository path or task-owned artifact.
- `[P]` tasks are safe only under the dependency assumptions above.
- All commits created during implementation must be DCO-signed and
  cryptographically signed.
- If local e2e prerequisites are missing, document that fact; do not mark T033
  complete as though the suite passed.
