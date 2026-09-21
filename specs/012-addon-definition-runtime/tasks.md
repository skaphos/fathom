<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Tasks: Runtime Addon Definitions

**Regenerated**: 2026-09-20 after three accepted clarifications. Execution status is tracked below; checked tasks have evidence in execution.md.

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/runtime.md](contracts/runtime.md).

Tests are mandatory under FR-016 and repository policy. Write regression tests before implementation, demonstrate relevant failures, then make them pass. New file paths below are planned. Every unchecked item is unimplemented; this document does not authorize external grant installation.

## Phase 1: Setup

Baseline and execution ledger; no runtime changes.

- [X] T001 Read the accepted RFC/ADR and contracts/decision-supplement.md; initialize requirement, event-row, numeric-row and payload-field test/result tracking in specs/012-addon-definition-runtime/execution.md; record source baseline, accepted clarification precedence, toolchain and #256 release dependency.
- [X] T002 Inventory existing registry, evaluator, impersonation and evidence tests; record exact reusable seams and baseline test results in specs/012-addon-definition-runtime/execution.md.

## Phase 2: Foundation

Shared contracts required by every story; no executable runtime path.

- [X] T003 Define shared bounded validation constants and pure API/rendering package boundary in pkg/addondefinition/limits.go; encode every input cap from contracts/runtime.md; api/v1alpha1 must not import pkg/addondefinition, and internal/cli must not import internal adapter/app/controller packages; add schema-literal parity tests to avoid constant drift.
- [X] T004 Add runtimeLoading.enabled=false to Options and bindings() in internal/app/options.go and precedence/default tests in internal/app/options_test.go; when election is disabled refuse runtime activation with LeaderElectionRequired diagnostics while preserving manager startup and built-ins; do not activate runtime wiring yet.
- [X] T005 Create at-limit/over-limit fixture builders for every numeric contract row in internal/adapter/runtime/testutil/fixtures.go and map each to planned enforcement/tests in specs/012-addon-definition-runtime/execution.md.
- [X] T006 Verify the pinned controller-runtime v0.25.1 manager election/lifecycle interfaces and client-go epoch behavior; record supported unique-holder, cancellation and termination hooks in specs/012-addon-definition-runtime/research.md before runtime wiring; do not weaken contracts/leadership.md for an incompatible API.

## Phase 3: US1 — Author installable coverage (P1)

Independent test: valid definition and disabled binding admit; invalid contracts reject; deterministic renderer performs no writes. This is the first useful increment, not runtime release.

- [X] T007 [US1] Add admission tests in api/v1alpha1/addondefinition_types_test.go for all nine kinds, exactly-one payload, unique names, canonical equality/immutability, unknown kinds, version-source relationships and all input limits; assert failures before implementation.
- [X] T008 [P] [US1] Add binding admission/default/immutability/status tests in api/v1alpha1/addondefinitionbinding_types_test.go, including real CRD creation and worst-size CEL cost admission cases.
- [X] T009 [US1] Add table-driven contract tests for every field/default/constraint/mapping in contracts/payloads.md to api/v1alpha1/addondefinition_payloads_test.go and internal/adapter/declarative/runtime_payloads_test.go; include scope intersection, singleton multi-namespace rejection, helper scope, reserved thresholds, policy overrides and strict/pruned unknown-field behavior.
- [X] T010 [US1] Implement api/v1alpha1/addondefinition_types.go from contracts/payloads.md envelope/family/check tables: cluster scope; lowercase DNS-label addonType=name, immutable, 1–63 ASCII; required SemVer adapterVersion≤256 bytes and integer semanticsVersion=1; optional=false; nonempty ordered unique families/checks and exactly-one typed payload; family.defaultEnabled=false; APIService maps to Condition.
- [X] T011 [US1] Implement api/v1alpha1/addondefinitionbinding_types.go: name=definitionRef.name; immutable name/UID reference objects; nonempty UIDs ≤128 bytes; SA names ≤253 characters; enabled=false; ≤32 exact unique namespaces each ≤63 characters; allowClusterScoped=false; require namespace or cluster scope; spec ≤16 KiB.
- [X] T012 [US1] Implement status subresources in api/v1alpha1/addondefinition_types.go and addondefinitionbinding_types.go; binding observedGeneration≥0, activeRuns 0–4, leaderIdentity≤253 bytes, ≤8 map-list conditions with type≤64/reason≤128/message≤1,024 bytes and Accepted/Ready/Drained semantics; add leaderEpoch with leaseUID nonempty≤128 bytes, holderIdentity nonempty≤253 bytes, acquireTime timestamp and leaseTransitions nonnegative int32; status never grants authority.
- [X] T013 [US1] Implement Workload, CRD and Condition payloads in api/v1alpha1/addondefinition_payloads.go from contracts/payloads.md: Workload kind enum and required name, checkPods=false, restart int32≥0; CRD names1–32/versions1–8; Condition named-vs-list union, True/False/Unknown predicate, paired versionCRD/versions and defaults Fail/Fail; common target and inherited absence rules apply.
- [X] T014 [US1] Implement Field, Webhook and CronJob payloads in api/v1alpha1/addondefinition_payloads.go from contracts/payloads.md: Field path1–16/segments≤128 bytes, required expectedValue, outcome map≤32, Warn defaults; Webhook closed kind enum, paired service name/namespace and verifyEndpoints=false; CronJob duration≥0 default0s, staleOutcome=Warn; preserve all named mapping rows.
- [X] T015 [US1] Implement ConfigMap, AnnotationStaleness and PodProjection payloads in api/v1alpha1/addondefinition_payloads.go from contracts/payloads.md: ConfigMap key≤253 and version list≤8 with Warn/Fail defaults; Annotation named/list exclusivity, annotation qualified key, literal JSON key≤128, required maxAge>0; PodProjection nonempty selector≤32, required volume DNS label≤63, optional envVar≤253 and Fail default.
- [X] T016 [US1] Implement common explicit target, threshold and requestedReads structures in api/v1alpha1/addondefinition_payloads.go: target scope Namespaced|Cluster, namespace set≤32 each≤63, no all-namespace runtime reads; required one namespace for singletons; threshold keys≤63 with reserved-key rejection, durations≤256 bytes fitting int64 nanoseconds, read declarations≤32 closed get/list resource or get-only exact discovery rules per contracts/payloads.md.
- [X] T017 [US1] Add bounded input/semantic validation in pkg/addondefinition/validation.go and internal/adapter/declarative/runtime_compile.go: spec≤256 KiB, ≤16 families, ≤32 checks/family, ≤512 checks; strings≤1,024 bytes/code points, maps/lists≤32, depth≤8; identifiers≤63 ASCII and resource/group/path segments≤253 bytes; exact tighter exceptions in contracts/runtime.md.
- [X] T018 [US1] Implement range/selector/path/requestedReads validation in pkg/addondefinition/validation.go: ranges≤256 bytes/16 comparators/8 alternatives, API versions≤8, selectors≤32 terms/32 values per term/256-byte values, literal paths≤16 segments/128 bytes each; get/list exact identifiers and discovery URLs only; reject wildcards, writes and arbitrary URLs.
- [X] T019 [US1] Add mixed-kind order, deep-copy, supported-version ambiguity and unsupported-semantics tests in internal/adapter/declarative/runtime_compile_test.go; implement immutable conversion and explicit ordered evaluator sequence in internal/adapter/declarative/runtime_compile.go and evaluator.go while preserving built-in bucket order.
- [X] T020 [P] [US1] Add deterministic offline rendering/no-write tests in pkg/addondefinition/render_test.go, including non-built-in identities, incomplete requestedReads and binding UID staging; implement pure rendering in pkg/addondefinition/render.go.
- [X] T021 [US1] Implement read-only definition render/bind/collisions/drain command tests in internal/cli/definition_test.go and commands in internal/cli/definition.go per contracts/runtime.md; register in internal/cli/root.go. Use an injectable inventory provider until generated inventory lands in the next task; require target-release binary inventory, display version/build, reject unknown metadata or external inventory input; test collision/read error exit codes and no writes. Drain uses a fake read seam here; real leadership integration is US3.
- [X] T022 [US1] Add pinned inventory/sample generation task in Taskfile.yml and generator in internal/adapter/rbacgen/runtime_samples.go; emit stable inventory into pkg/addondefinition/inventory_generated.go and all-nine-kind non-built-in examples under config/samples/addondefinition/; include version/build in CLI output, validate generation drift and obey CLI import restrictions.
- [X] T023 [US1] Register resources in api/v1alpha1/groupversion_info.go and PROJECT; use generate/manifests/helm:sync/docs:api-ref tasks to produce deepcopy, CRDs, distribution and docs/reference/api.md; record admission/conversion/render verification in specs/012-addon-definition-runtime/execution.md.

## Phase 4: US2 — Delegate bounded execution (P1)

Development checkpoint: explicitly constructed runtime harness uses only delegated reads, rejects out-of-scope requests, enforces all limits and preserves a healthy peer. Production wiring remains off. This does not complete US2 acceptance: its real-cluster independent test remains open until US3 wiring and US4 qualification.

- [ ] T024 [US2] Add runtime authority tests in internal/adapter/impersonation/runtime_test.go for UID recreation, shared/manager/builtin SA rejection, absent binding/factory/namespace, inherited headers, same-UID retargeting, denied discovery and metrics-off operation.
- [ ] T025 [US2] Implement runtime authority validation and dedicated client construction in internal/adapter/impersonation/runtime.go; intersect explicit target scope, clear inherited impersonation, use canonical SA identity and identity-keyed discovery; never use manager mapper/client or local credentials for evaluation.
- [ ] T026 [US2] Add transport boundary tests in internal/adapter/runtime/transport_test.go for decompressed success/error/discovery responses, retries, continued pages, helper reads, cancellation and policy overrides; implement read-only scope enforcement and counters in internal/adapter/runtime/transport.go.
- [ ] T027 [US2] Enforce ≤2 MiB decoded response/request, ≤16 MiB/run, ≤100 objects/page, ≤1,000 objects/run, ≤100 requests/run including helpers/discovery/retries, ≤100,000 visits/run, ≤32,768 JSON nodes and depth≤64/object in internal/adapter/runtime/budget.go and transport.go; continuation at cap is failure, not truncated success.
- [ ] T028 [US2] Add parser/work tests and enforce ConfigMap value≤64 KiB, ≤4,096 YAML nodes/depth≤16/no aliases and annotations≤1,024 bytes in internal/adapter/declarative/configmap.go and annotation.go; define a narrow budget/cancellation interface in internal/adapter/declarative/evaluator.go and thread it through runtime evaluator/version-helper paths; runtime implements it, declarative never imports runtime.
- [ ] T029 [US2] Enforce run deadline min(check timeout,30s), request deadline min(5s,remaining), cooperative compilation≤1s, ≤2 retries/request and ≤1 pagination restart/run in internal/adapter/runtime/budget.go and internal/adapter/declarative/runtime_compile.go; pre/final manager identity fences share run counters/deadline with evaluator traffic but never share credentials; charge every retry.
- [ ] T030 [US2] Add result/precedence tests in internal/adapter/runtime/result_test.go; enforce ≤1,000 entries/256 KiB serialized evidence/messages≤1,024 bytes/details≤32, reserved failure summary, deterministic first failure and deadline precedence in internal/adapter/runtime/result.go; no partial healthy ratio rollup.
- [ ] T031 [US2] Add compile/evaluate panic, slot-release and healthy-peer regression tests in internal/adapter/runtime/runner_test.go; implement supervised recovery/cancellation in internal/adapter/runtime/runner.go with no evaluator goroutine escaping supervision.
- [ ] T032 [US2] Add concurrency/cache/backoff tests in internal/adapter/runtime/scheduler_test.go; implement ≤4 runtime runs/process, ≤1/definition/check, round-robin definition admission, 10 requests/s burst20 shared limiter, deduplicated wake keys and separate built-in workers in internal/adapter/runtime/scheduler.go.
- [ ] T033 [US2] Implement ≤128 idle cached revisions plus ≤4 active snapshots with idle LRU in internal/adapter/runtime/cache.go; retry 5/10/20/40/60s with 0–20% jitter, missing-input poll60s, one queued wake/check, churn-resistant slots/backoff and event deduplication in internal/adapter/runtime/scheduler.go.
- [ ] T034 [US2] Implement actual-request PermissionsVerified diagnostics in internal/adapter/runtime/diagnostics.go: Unknown/NotEvaluated before reads, False/AccessDenied on403, Unknown/AccessCheckUnavailable for transport failures; explain omitted/incomplete requestedReads without speculative requests, SAR permission or effective-union claims.
- [ ] T035 [US2] Run numeric-row, payload-override, race/fairness and hostile-input component tests; record actual names/results in specs/012-addon-definition-runtime/execution.md; mark US2 harness checkpoint only, leaving real-cluster acceptance pending until US4. Fix known input-triggerable process-wide failure before qualification.

## Phase 5: US3 — Preserve evidence across change (P1)

Independent test: controlled lifecycle races reject obsolete completion, retain original evidence, recover and acknowledge drain only under current leader/generation.

- [ ] T036 [US3] Add owner-aware replacement/removal, immutable snapshot and collision tests in internal/adapter/registry/runtime_test.go; implement off-lock construction and atomic runtime snapshot replacement with (definition UID, generation, schema, semantics) revision plus operator-build/adapterVersion provenance, and dispatch barriers in internal/adapter/registry/registry.go without changing built-in registration semantics.
- [ ] T037 [US3] Add every RFC lifecycle-row fixture in internal/controller/addondefinition_lifecycle_test.go, including missing/invalid input, deletes/recreation, startup partial sync, grant recovery, concurrent edits, collisions and aging; use controllable evaluation/fence barriers.
- [ ] T038 [US3] Implement definition and binding reconcilers in internal/controller/addondefinition_controller.go and addondefinitionbinding_controller.go: semantic validation, dedicated-SA UID uniqueness, manager/builtin exclusions, invalid snapshot removal, owner-aware delete and indexed dependency requeues.
- [ ] T039 [US3] Implement pre-run/final uncached APIReader validation in internal/controller/addoncheck_runtime.go, capturing definition UID/generation/revision, binding UID/spec generation, SA UID, check UID/generation/policy and current leadership epoch; share outer budgets across manager fences and isolated evaluator requests; status-only binding writes do not invalidate authority; failed validation prevents publication.
- [ ] T040 [US3] Implement per-check publication serialization/CAS and authority-before-superseded-before-invalid-before-execution failure precedence in internal/controller/addoncheck_runtime.go; cancel observed revocations and requeue current inputs without claiming atomic revocation.
- [ ] T041 [US3] Add manager leadership-session tests in internal/app/runtime_leadership_test.go and implement integration in internal/app/runtime_leadership.go: configured namespaced Lease, process-unique holder, terminate on loss/no reacquisition, cancel admission/work, invalidate persisted epochs and wait ≥30s monotonic takeover grace before runtime admission/drain acknowledgement; preserve observation-based limitation.
- [ ] T042 [US3] Add drain/leader-change/status-only-write tests in internal/controller/addondefinitionbinding_controller_test.go and implement drain in addondefinitionbinding_controller.go per contracts/leadership.md: enabled=false, awaited cancellation, zero active runs, direct binding/Lease reads, matching generation/epoch and Ready=False/AuthorizationRevoked; reject old/missing epochs and incomplete conditions.
- [ ] T043 [US3] Implement independent CLI drain verification in internal/cli/definition_drain.go and tests in internal/cli/definition_drain_test.go: direct configured Lease/binding reads, progressing renewTime with stable epoch, ≤15s deadline/≤16 Lease reads plus one binding read/≤1 poll per second/per-request≤5s; reserve final Lease read, reject all incomplete/stale/denied evidence, allow renewal RV changes and display observed epoch.
- [ ] T044 [US3] Add completed Pass/Warn/Fail/Skipped evidence, original observedAt/revision/context, separate latestAttemptAt/outcome and freshness to api/v1alpha1/addoncheck_types.go; implement preservation on failed attempts, Unknown-without-evidence and Current age≤two effective intervals+timeout in internal/controller/addoncheck_runtime.go; all-Skipped replaces current evidence with new time and NoChecksEvaluated coverage, message “no checks evaluated”; Ready means completed eligibility, never Pass.
- [ ] T045 [US3] Add Pass→Skipped, Skipped→Skipped, mixed-Skipped, zero-enabled-check and unchanged-verdict revision tests in internal/controller/addoncheck_runtime_test.go; extend api/v1alpha1/healthreport_types.go attribution and transition-only reporting in internal/controller/addoncheck_controller.go; preserve existing aggregate behavior and historical provenance, never renew old evidence on failed attempts.
- [ ] T046 [US3] Extend api/v1alpha1/healthcheck_types.go and internal/controller/healthcheck_controller.go to mirror readiness/freshness; add regression tests in internal/controller/healthcheck_controller_test.go proving retained Pass cannot become fresh success and ClusterHealth still uses only HealthCheck.status.
- [ ] T047 [US3] Wire shared registry/runtime pool, manager APIReader fences, informer sync, explicit operator namespace and required leader election into internal/app/run.go and internal/controller/addoncheck_controller.go behind default-off configuration; register all watches/indexes, never run runtime admission before election/sync/grace, preserve unrelated built-ins.
- [ ] T048 [US3] Add minimal definition/binding read/watch/status RBAC markers in internal/controller/addondefinition_controller.go and addondefinitionbinding_controller.go; use existing namespaced election Role Lease access, generate config/rbac/role.yaml and Helm distribution, prove no binding-spec writes, new grant writes, cluster-wide Lease reads, addon standing reads or SAR grants.
- [ ] T049 [US3] Run lifecycle race/envtest suites and regenerate manifests/API docs; record per-event evidence, observed revocation limitations and all changed outputs in specs/012-addon-definition-runtime/execution.md.

## Phase 6: US4 — Install and operate safely (P2)

Independent test: clean install through opt-in with leader election, target-version CLI collision preflight, independently verified drain, rollback and re-enable works in full kind with preserved history. This phase also completes US2 real-cluster acceptance.

- [ ] T050 [US4] Add full real-cluster tests in test/e2e/addondefinition_test.go and fixtures under test/e2e/fixtures/ for admission/RBAC/discovery, same-UID delegation, identity recreation/borrowing, local/metrics-off denial, required election, renewal/epoch/takeover drain, all-Skipped history/freshness, every lifecycle row, budgets/panics/fairness and target-version upgrade collisions; explicitly complete US2 real-cluster acceptance.
- [ ] T051 [US4] Expose default-off configuration through deploy/helm/fathom-operator/values.yaml and configuration templates; add tests for required election, configured operator namespace/Lease name and disabled-runtime compatibility without changing built-ins.
- [ ] T052 [P] [US4] Document two-stage Git-reviewed UID installation, minimal grants, diagnostics, exact caps and declared order in docs/guides/addon-definitions.md; update README.md and docs/reference/operator-rbac.md, checking actual reference filenames before editing.
- [ ] T053 [US4] Document target-version CLI inventory, clarification supplement, same-UID delegation, non-atomic revocation, Lease read permission and independent drain verification, Skipped coverage, disable/revoke/preserve rollback and re-enable validation in RELEASE.md and docs/guides/addon-definitions.md; keep #149 separate.
- [ ] T054 [US4] Record #256 separate PR decision, older-adapter warnRatio/failRatio regression and migration/rejection behavior in specs/012-addon-definition-runtime/execution.md; leave this task open and block runtime release until verified, without implementing or closing #256 here.
- [ ] T055 [US4] Run the complete quickstart installation/upgrade/rollback and full go -C tools tool task test-e2e; record exact environment, named scenarios and results in specs/012-addon-definition-runtime/execution.md; absent required tools are an explicit blocker.

## Phase 7: Polish and cross-cutting qualification

No epic completion or release without every prior gate.

- [ ] T056 Complete one-to-one requirement/lifecycle/numeric-row coverage audit in specs/012-addon-definition-runtime/execution.md; document security review and absence of known definition-triggerable fatal faults before enabling/releasing runtime loading.
- [ ] T057 Run pinned verify-generated, crd-compat, ci, race checks and reuse lint; update specs/012-addon-definition-runtime/execution.md with exact outcomes; do not lower coverage thresholds or silently bypass compatibility findings.
- [ ] T058 Update AGENTS.md and docs/architecture.md for final package boundaries, update generated samples/docs through tasks and run graphify update . after source changes; record results in specs/012-addon-definition-runtime/execution.md.
- [ ] T059 Prepare focused PRs following plan.md milestones with exact validation evidence, verify repository-specific author/committer/DCO identity before commits, and record PR/release dependency links in specs/012-addon-definition-runtime/execution.md.

## Dependencies and parallel opportunities

Setup → foundation → US1 → US2 → US3 → US4 → qualification. US1 is the first
independently useful schema/rendering increment; it is not a runtime MVP.
US2 component work can validate with a harness before US3 production wiring; its story acceptance remains incomplete until US4 real-cluster verification. US3 is independently
validated by lifecycle fixtures after shared execution exists. US4 qualifies the
combined system. #256 blocks release, not schema/rendering development.

Within each phase use listed order unless marked [P]. Parallel examples: US1
binding tests alongside definition tests, then rendering tests after validation
contracts settle; US2 transport tests and scheduler test design can be separated
after common budget interfaces settle, but do not edit shared runtime files in
parallel; US3 registry tests and lifecycle fixture design can proceed independently
before wiring; US4 authoring documentation can proceed alongside kind fixtures.
[P] is not authorization to skip prerequisites or split ownership of the same file.

## Implementation strategy

Deliver plan milestones as focused PRs. Keep runtime disabled while partial work
lands; do not substitute US1 for epic completion. Tests should exercise observable
contracts rather than mirror implementation. Update the execution ledger with
named evidence after each phase and preserve historical results when superseded.
Do not edit accepted RFC/ADR decisions or mark implementation tasks done from
planning. A failed qualification gate keeps runtime release blocked.
