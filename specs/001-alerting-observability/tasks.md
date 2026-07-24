# Tasks: Alerting-Grade Observability — Current-Result and Staleness Metrics, Kubernetes Events

**Input**: Design documents from `specs/001-alerting-observability/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/metrics.md, contracts/events.md, quickstart.md

**Tests**: included — the constitution requires new behavior to ship with direct test coverage, so test tasks are not optional here.

**Organization**: grouped by user story; US1 (result gauge) is the MVP slice.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no dependency on an incomplete task)
- **[Story]**: US1 = alert on current result, US2 = staleness + shipped rules, US3 = Kubernetes Events

## Phase 1: Setup

**Purpose**: confirm a clean baseline on the feature branch — no scaffolding is needed (existing single-module operator repo).

- [X] T001 Verify baseline gates on the branch (`go -C tools tool task fmt`, `lint`, `test`) so later failures are attributable to this feature

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the metrics contract surface both metric stories build on (research R7).

**⚠️ CRITICAL**: US1 and US2 both call these helpers; complete before story phases.

- [X] T002 Add `CheckResult` and `CheckLastRunTimestamp` GaugeVecs (names/labels per contracts/metrics.md) and register them in `init()` in internal/metrics/metrics.go
- [X] T003 Add helpers in internal/metrics/metrics.go: `SetCheckResult(kind, name, namespace, result)` writing the full one-hot set over the canonical `api/v1alpha1` result vocabulary (drive the list off the exported constants so a new state cannot silently miss the metric — FR-002/SC-004), `SetCheckObserved(kind, name, namespace, t)` (t=0 sentinel allowed), and `DeleteCheckSeries(kind, name, namespace)` using `DeletePartialMatch` on both vecs
- [X] T004 Unit tests in internal/metrics/metrics_test.go (existing Reset → Gather → dto.MetricFamily pattern): one-hot invariant (exactly one `1`, six series), sentinel writes (Unknown/0), `DeleteCheckSeries` removes all seven series, vocabulary-completeness against the `api/v1alpha1` constants

**Checkpoint**: metrics contract exists and is unit-tested — story phases can begin.

---

## Phase 3: User Story 1 - Alert on a check's current result (Priority: P1) 🎯 MVP

**Goal**: every check kind exports the one-hot `fathom_check_result` series from first observation through deletion, so operators can alert on failing checks with stock PromQL.

**Independent Test**: create a check against a healthy target, scrape `/metrics`, see `result="Pass"` at 1; break the target, see the series flip to `Fail`; delete the check, see the series vanish — with US2/US3 untouched (spec US1 acceptance scenarios 1–4).

### Implementation for User Story 1

- [X] T005 [P] [US1] AddonCheck: in internal/controller/addoncheck_controller.go call `metrics.SetCheckResult` where `runAddonCheck` writes `Status.LastResult` (~line 390), emit the Unknown sentinel on reconciles where status has no evaluated result yet (discovery), and call `metrics.DeleteCheckSeries` in the `IsNotFound` early-return (~line 152)
- [X] T006 [P] [US1] HealthCheck: same pattern in internal/controller/healthcheck_controller.go around `mirrorTarget`'s writes to `Status.Result` (~lines 179/194) and the `IsNotFound` return (~line 84)
- [X] T007 [P] [US1] ClusterHealth: same pattern in internal/controller/clusterhealth_controller.go around `aggregate`'s `Status.Result` writes (~lines 204/248/261) and `IsNotFound` (~line 83), passing `namespace=""` (cluster-scoped — data-model.md)
- [X] T008 [P] [US1] NodeCertificateCheck: same pattern in internal/controller/nodecertificatecheck_controller.go around `rollup`'s `Status.LastResult` writes (~lines 744/821) and `IsNotFound` (~line 161)
- [X] T009 [US1] Controller test coverage in internal/controller/addoncheck_controller_test.go, healthcheck_controller_test.go, clusterhealth_controller_test.go, nodecertificatecheck_controller_test.go: after a reconcile the gauge reports the status result (assert via a fresh registry gather); discovery emits Unknown=1; deletion reconcile removes the series; delete+recreate under the same name reports only the new object's result (spec edge case)
- [X] T010 [US1] e2e assertion in test/e2e/: scrape the operator metrics endpoint on the kind cluster and assert the six-series one-hot shape for a core-tier check, and absence after deleting it (quickstart §2–3)

**Checkpoint**: US1 fully functional — MVP shippable.

---

## Phase 4: User Story 2 - Detect stale checks and a wedged operator (Priority: P2)

**Goal**: `fathom_check_last_run_timestamp_seconds` advances on every evaluation (0 until the first), and the two canonical alert rules ship in docs and as the opt-in PrometheusRule component (FR-009/FR-009a).

**Independent Test**: record the last-run series for a check, pause/stop evaluation, watch the timestamp stop advancing while wall time passes; `kustomize build` renders the rule component (spec US2 acceptance scenarios; quickstart §5).

### Implementation for User Story 2

- [X] T011 [US2] Stamp `metrics.SetCheckObserved(..., now)` at the same evaluated-status-write sites touched in T005–T008 (all four controllers in internal/controller/), with the 0 sentinel on discovery — wall-clock at reconcile completion, NOT per-kind status timestamps (research R2)
- [X] T012 [US2] Test coverage in internal/controller/*_test.go and internal/metrics/metrics_test.go: last-run is 0 before the first evaluation, advances after one, and does not advance on a reconcile that performs no evaluation
- [X] T013 [P] [US2] Create the opt-in component config/components/prometheus-rule/kustomization.yaml + config/components/prometheus-rule/prometheusrule.yaml carrying `FathomCheckFailing` and `FathomCheckStale` (exprs per contracts/metrics.md)
- [X] T014 [P] [US2] Add the commented opt-in `components:` entry for `../components/prometheus-rule` in config/default/kustomization.yaml, mirroring the existing `../components/prometheus` wiring (~line 56)
- [X] T015 [P] [US2] Add a lint-gate task step in Taskfile.yml that `kustomize build`s config/components/prometheus-rule (via the pinned tool wrappers) and keeps `config/default` building with the component commented out (research R6)
- [X] T016 [P] [US2] Document both gauges (names, labels, one-hot semantics, Unknown/0 sentinels, deletion lifecycle) under §2 "Operator metrics" in docs/guides/monitoring.md
- [X] T017 [US2] Rewrite §4's "Add-on check results — read the status, not a metric" subsection in docs/guides/monitoring.md: show the shipped `FathomCheckFailing`/`FathomCheckStale` rules, point at the opt-in component, and note rules are build-validated but not promtool-tested (Principle IX honesty)

**Checkpoint**: US1 + US2 independently functional; alerting story complete.

---

## Phase 5: User Story 3 - See check history in kubectl describe (Priority: P3)

**Goal**: first EventRecorder wiring — transition events (first result = Unknown → X, previous result from status) and Warning events for adapter-run, probe-launch, RBAC-provisioning, and reconcile failures, with bounded volume.

**Independent Test**: create a check, force a transition, see `ResultChanged` in `kubectl describe`; induce an unlaunchable probe, see `ProbeLaunchFailed`; repeated identical failures aggregate (spec US3 acceptance scenarios; quickstart §4).

### Implementation for User Story 3

- [X] T018 [US3] Add a typed launch error in internal/probe/launcher.go wrapping the pod-build (~line 89) and pod-create (~line 92) failure paths so callers can `errors.As` it (research R5), with a unit test in internal/probe covering the wrap and the non-launch paths staying untyped
- [X] T019 [US3] Thread `mgr.GetEventRecorderFor("fathom-<kind>-controller")` into each reconciler: add a `Recorder record.EventRecorder` field to all four reconciler structs and populate it in `DefaultControllers` in internal/app/run.go (~lines 215–245); keep the `Setupper`/fake-controllers test path compiling
- [X] T020 [P] [US3] AddonCheck events in internal/controller/addoncheck_controller.go: `ResultChanged` at the existing `resultChanged` detection (~line 362) with Normal/Warning per `Severity()` and old result read from status; at the `runErr` site (~lines 394–414) emit `ProbeLaunchFailed` when `errors.As` matches the typed launch error, else `AdapterRunFailed` (messages per contracts/events.md)
- [X] T021 [P] [US3] HealthCheck + ClusterHealth events in internal/controller/healthcheck_controller.go and internal/controller/clusterhealth_controller.go: `ResultChanged` on status-result change (compare before overwrite), `ReconcileError` on terminal reconcile errors after fetch
- [X] T022 [P] [US3] NodeCertificateCheck events in internal/controller/nodecertificatecheck_controller.go: `ResultChanged` at `rollup`, `RBACProvisioningFailed` at the ensure* failure sites (~lines 193–207), `AdmissionPolicyProvisioningFailed` (~lines 198–201), `ReconcileError`
- [X] T023 [US3] Add the `+kubebuilder:rbac:groups="",resources=events,verbs=create;patch` marker (one marker block, alongside the existing markers in internal/controller/addoncheck_controller.go ~line 122) and regenerate manifests (`go -C tools tool task lint` / manifests task) so config/rbac/role.yaml picks it up; confirm docs/reference/rbac.md regen if it covers the manager role
- [X] T024 [US3] Event test coverage in internal/controller/*_test.go using `record.FakeRecorder`: first result emits `Unknown → X`; no-change evaluation emits nothing; previous-result-from-status means a fresh reconciler (simulated restart) with populated status emits no false transition; each failure reason fires at its site with reason strings per contracts/events.md
- [X] T025 [US3] e2e assertions in test/e2e/: `kubectl describe`/Event list shows the first-result `ResultChanged` for a core-tier check and a Warning event for an induced failure; verify no per-reconcile event spam across several intervals (quickstart §4)

**Checkpoint**: all three stories independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T026 [P] Note the new metrics + events surface in README.md (user-visible behavior) and add the prometheus-rule component to the config/ layout notes in AGENTS.md if structure docs list components
- [X] T027 Run the full gate suite: `go -C tools tool task ci` and `scripts/check-coverage.sh` (coverage must not regress; ratchet only upward)
- [ ] T028 Execute quickstart.md end-to-end on a kind cluster: full `go -C tools tool task test-e2e` (shared surfaces touched — internal/app, all controllers, internal/probe — so the scoped addon run is NOT sufficient per AGENTS.md)
  - Deferred to the PR's `kind e2e` CI workflow (full stack — shared surfaces
    trigger every shard), per the session convention of not running kind
    locally; the new `Check observability` core-tier specs run there.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (P1)** → **Foundational (P2)** → story phases
- **US1 (Phase 3)**: needs T002–T004 only
- **US2 (Phase 4)**: T011/T012 need US1's controller touchpoints (same status-write sites, same files); T013–T017 need nothing beyond Foundational and can run any time after it
- **US3 (Phase 5)**: independent of US1/US2 logic, but T020–T022 edit the same controller files — schedule after US1/US2 controller edits to avoid conflicts (or coordinate)
- **Polish (Phase 6)**: after all desired stories

### Within stories

- T005–T008 are [P] (one controller file each); T009 after all four; T010 after T009
- T013/T014/T015 are [P] with T011/T012 and with each other (config/ + Taskfile vs controllers); T016 [P]; T017 after T016 (same file)
- T018/T019 before T020–T022; T020–T022 are [P] (different files); T023 after T020 (same file); T024 after T020–T022; T025 last

### Parallel opportunities

- Phase 3: T005, T006, T007, T008 concurrently (four files)
- Phase 4: {T011} ∥ {T013, T014, T015} ∥ {T016}
- Phase 5: T020, T021, T022 concurrently after T018/T019

## Parallel Example: User Story 1

```text
# After T004, launch the four controller wirings together:
Task: "T005 AddonCheck gauge wiring in internal/controller/addoncheck_controller.go"
Task: "T006 HealthCheck gauge wiring in internal/controller/healthcheck_controller.go"
Task: "T007 ClusterHealth gauge wiring in internal/controller/clusterhealth_controller.go"
Task: "T008 NodeCertificateCheck gauge wiring in internal/controller/nodecertificatecheck_controller.go"
```

## Implementation Strategy

**MVP first**: T001–T010 (Setup + Foundational + US1) is a shippable increment — operators can alert on current results. Stop, validate via quickstart §2–3, then continue.

**Incremental delivery**: US2 completes the alerting story (staleness + shipped rules); US3 adds the describe-history story. Each checkpoint leaves the tree green (all gates pass) and each story's acceptance scenarios verifiable on their own.

**Single-PR note**: all three stories are expected to land on this feature branch/PR (#240 lineage); checkpoints are for validation, not separate PRs, unless the user asks to split.
