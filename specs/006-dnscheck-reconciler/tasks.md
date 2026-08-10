# Tasks: DNSCheck Reconciler

**Feature**: 006-dnscheck-reconciler · **Branch**: `feature/266-dnscheck-reconciler` · **Date**: 2026-08-09

**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Revision**: incorporates the `/speckit-analyze` findings of 2026-08-09 — see [Analysis remediation](#analysis-remediation).

## Format: `[ID] [P?] [Story] Description`

- **[P]** — parallelisable: different file, no dependency on an incomplete task
- **[US#]** — the user story the task serves; setup, foundational, and polish tasks carry no story label

## Path Conventions

Kubebuilder v4 layout, paths relative to the repository root. Tests live beside
their source as `*_test.go`; e2e lives in `test/e2e/`.

**Tests are required.** AGENTS.md: new behavior ships with direct test coverage,
and `internal/controller/*` plus `internal/probe/*` changes mandate an e2e run.

---

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 Add an integer binding variant (`isInt` / `intDef`) to the `binding` struct and its two switch statements in `internal/app/options.go` — the table supports string, bool, and float only, and FR-103a needs a configurable integer
- [X] T002 Add `DNSCheckOptions{MaxConcurrentProbes}` to `Options` with a default, a `bindings()` row, and a `Validate` rule rejecting values below 1, in `internal/app/options.go` — **scope reduced, see note below**
- [X] T003 [P] Document the option and the pair-count/`spec.timeout` sizing relationship in `docs/reference/configuration.md` (new *DNSCheck Fan-out* section)
- [X] T004 [P] Cover the integer binding's full precedence chain and both `Validate` rejections in `internal/app/options_test.go`

> **Scope note on T002/T003.** The planned `DNSCheckPerPairTimeoutCeiling` and
> `DNSCheckMinRunGap` were **not** added as configuration. Only FR-103a requires
> a configurable value; FR-104's per-pair ceiling and FR-107a's minimum gap are
> stated as properties, not knobs. The per-pair bound is now *derived* — each
> pair takes the remaining budget divided by the batches still to run — so it
> needs no constant at all, and the minimum gap is a documented package
> constant. Two fewer knobs, every requirement still met. FR-107c is satisfied
> because a constant is as derivable from documentation as a flag.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Blocks every user story. Nothing below Phase 2 can start until this completes.**

- [X] T005 [P] Add an `OwnerReferences []metav1.OwnerReference` field to `Request` and stamp it onto the built pod in `internal/probe/pod.go` (research D1 — the probe package sets no owner reference today)
- [X] T006 [P] Test that owner references are stamped when set and that an unset field leaves the manifest byte-identical for existing callers, in `internal/probe/pod_test.go`
- [X] T007 [P] Add the `DNSCheckTargetResult` gauge (`fathom_dnscheck_target_result`), an `ObserveDNSTarget` setter, and a `DeleteDNSCheckTargetSeries` partial-match withdrawal in `internal/metrics/metrics.go`
- [X] T008 [P] Test the gauge's one-hot invariant, partial-match withdrawal by `{namespace,check}`, and that the label set yields at most 288 series per check, in `internal/metrics/check_metrics_test.go`
- [X] T009 Create the pure planning helpers in `internal/controller/dnscheck_plan.go`: pair expansion from spec, run-budget derivation, per-pair bound as `min(remaining, ceiling)`, start-anchored requeue with floor, truncation fold, and the polarity-aware summary builder
- [X] T010 Table-driven tests for every helper in `internal/controller/dnscheck_plan_test.go`, covering the quickstart Level 1 rows — including 8 `Pass` + 40 `Unknown` folding to `Unknown` and 1 `Fail` + 47 `Unknown` folding to `Fail`
- [X] T011 Create the `DNSCheckReconciler` struct, its `+kubebuilder:rbac` markers (pods `create;get;list;delete` — **no** `watch`), and `SetupWithManager` in `internal/controller/dnscheck_controller.go`
- [X] T012 Build a dedicated uncached client for probe pods and register the reconciler in `DefaultControllers` in `internal/app/run.go` (research D2 — a cached Pod read opens a cluster-wide informer)
- [X] T013 Regenerate RBAC via `go -C tools tool task manifests` and confirm `config/rbac/role.yaml` gained `create` and `get` on pods and no `watch`
- [X] T014 Rewrite the justification rows in `docs/reference/operator-rbac.md` and confirm `TestOperatorClusterRoleRulesAreJustifiedInDoc` passes — **four rows, not one; see note below**. Also re-synced `deploy/helm/fathom-operator/files/manager-rules.yaml`, which is derived from `config/rbac/role.yaml` and gated.

> **T013 and T014 are one atomic change — do not commit between them.**
> `operator_rbac_doc_test.go` enforces lockstep between the generated ClusterRole
> and the justification table, so the tree is red from the moment T013 lands
> until T014 completes. The existing row states verbatim *"no
> `get`/`watch`/`create`/`update` — … pod creation authority stays with the
> per-addon ServiceAccounts"*, which this feature reverses. T014 is a rewrite of
> an argument that currently says the opposite, not an append.
>
> The rewritten row must carry all six points listed in
> [contracts/metrics-and-events.md](./contracts/metrics-and-events.md#the-justification-document),
> and must keep the page's existing claim about addon probe pods intact — that
> path is unchanged and still runs under per-addon impersonation.

**Checkpoint**: the reconciler is registered and reachable, the probe can own its pods, the metric exists, RBAC is granted and defended, and all arithmetic is tested — but nothing evaluates yet.

---

## Phase 3: User Story 1 - A declared check produces a verdict on its cadence (Priority: P1) 🎯 MVP

**Goal**: A `DNSCheck` applied to the cluster begins evaluating on its cadence and reports a folded verdict.

**Independent test**: Apply a check naming one resolvable and one unresolvable subject; watch `status.lastResult` populate and refresh on the declared cadence with no further input.

### Tests for User Story 1

- [X] T015 [P] [US1] envtest: applying a check populates `lastResult`, `observedTargets`, `observedGeneration`, and `lastRunTime` after one cadence, in `internal/controller/dnscheck_controller_test.go`
- [X] T016 [P] [US1] envtest: one resolvable plus one unresolvable subject under positive assertions folds to `Fail`, not `Error`, in `internal/controller/dnscheck_controller_test.go`
- [X] T017 [P] [US1] envtest: a fake launcher returning an *unreachable-resolver* outcome for a pair asserting `absent: true` must **not** fold to `Pass`, in `internal/controller/dnscheck_controller_test.go` — FR-014's sharpest case, and the trap feature 005 called out by name
- [X] T018 [P] [US1] envtest: editing the spec is reflected on the next evaluation and `observedGeneration` advances, in `internal/controller/dnscheck_controller_test.go`
- [X] T019 [P] [US1] envtest: a check declaring several vantage points evaluates every (target, vantage point) pair exactly once, in `internal/controller/dnscheck_controller_test.go`
- [X] T020 [P] [US1] envtest: a fake launcher instrumented with an in-flight counter never exceeds the configured concurrency cap, in `internal/controller/dnscheck_controller_test.go` — without this a silently ignored cap passes every other test
- [X] T021 [P] [US1] envtest: a `probe.LaunchError` on one pair marks only that pair `Error` while the rest still evaluate, in `internal/controller/dnscheck_controller_test.go`

### Implementation for User Story 1

- [X] T022 [US1] Implement the `Reconcile` prologue in `internal/controller/dnscheck_controller.go`: trace span, deferred `RecordReconcile`, `Get` with the `NotFound` withdrawal path, status snapshot, deferred `observeCheck`, `observedGeneration`, and the `Accepted` condition with `cadenceClampMessages`
- [X] T023 [US1] Plan the run in `internal/controller/dnscheck_controller.go`: expand pairs from the current spec, seed every pair `Unknown`, and derive the single run deadline
- [X] T024 [US1] Implement bounded fan-out with `errgroup.SetLimit` in `internal/controller/dnscheck_controller.go`, launching one owner-referenced probe pod per pair with a per-pair bound of `min(remaining, ceiling)`
- [X] T025 [US1] Map probe results to pair outcomes in `internal/controller/dnscheck_controller.go`, keeping the FR-105 split — `LaunchError` and unusable results are `Error`; a resolver's answer is `Pass`/`Fail`; an unreachable resolver never satisfies a negative assertion (FR-014)
- [X] T026 [US1] Fold with `WorstResult(outcomes, true)` and build the summary in `internal/controller/dnscheck_controller.go`, naming polarity on a negative-assertion failure
- [X] T027 [US1] Implement `finish` in `internal/controller/dnscheck_controller.go`: write status only when it differs, and requeue at `max(minGap, interval − elapsed)` measured from run start

**Checkpoint**: US1 is independently shippable — checks run and report verdicts.

> **T032 and T034 landed here, not in Phase 4.** US1's own specs assert on
> `status.targetResults` (T016, T019, T021) and on the `Complete` condition
> (T015), so populating them was a prerequisite for Phase 3 rather than a
> follow-on. Phase 4 keeps the per-target *gauge* (T033) and the tests that
> exercise removal and truncation, which remain genuinely separate work.

---

## Phase 4: User Story 2 - An operator sees which name failed (Priority: P2)

**Goal**: The specific failing subject and vantage point are identifiable from the resource and metrics alone.

**Independent test**: Apply a multi-target check, break exactly one target, confirm both `targetResults` and the per-target gauge single it out.

### Tests for User Story 2

- [X] T028 [P] [US2] envtest: one failing target among several is named in `targetResults` with its evidence and carries a gauge series, in `internal/controller/dnscheck_controller_test.go`
- [X] T029 [P] [US2] envtest: removing a target from the spec drops its `targetResults` entry and withdraws its metric series on the next run, in `internal/controller/dnscheck_controller_test.go`
- [X] T030 [P] [US2] envtest: a negative-assertion failure produces a summary that names the polarity, in `internal/controller/dnscheck_controller_test.go`
- [X] T031 [P] [US2] envtest: a run truncated by its bound leaves unreached pairs `Unknown` and sets `Complete=False` naming the count, in `internal/controller/dnscheck_controller_test.go`

### Implementation for User Story 2

- [X] T032 [US2] Populate `status.targetResults` by full replacement — never merge — with message, answers, and latency per pair, in `internal/controller/dnscheck_controller.go`
- [X] T033 [US2] Emit the per-target gauge by delete-then-set each run in `internal/controller/dnscheck_controller.go`, so a dropped pair's series disappears without removal detection
- [X] T034 [US2] Set the `Complete` condition to `False` with the unreached count when any pair is still `Unknown` at the deadline, in `internal/controller/dnscheck_controller.go`

---

## Phase 5: User Story 3 - Result history is recorded without noise (Priority: P3)

**Goal**: Verdict changes become history and events; uneventful runs do not.

**Independent test**: Let a stable check run repeatedly and confirm history does not grow; break it and confirm exactly one new record.

### Tests for User Story 3

- [X] T035 [P] [US3] Table-driven test of the rollup decision function (persist / refresh-liveness / noop) in `internal/controller/dnscheck_plan_test.go`
- [X] T036 [P] [US3] envtest: a stable verdict across several cadences writes exactly one `HealthReport` while `lastRunTime` still advances, in `internal/controller/dnscheck_controller_test.go`
- [X] T037 [P] [US3] envtest: a verdict change writes exactly one new `HealthReport` and emits one `ResultChanged` event, in `internal/controller/dnscheck_controller_test.go`

### Implementation for User Story 3

- [X] T038 [US3] Add `dnsCheckShouldPersistReport` to `internal/controller/dnscheck_plan.go` — **binary, not the three-way `decideNodeCertRollup` shape**: that kind is watch-driven and needs a liveness throttle, whereas a DNSCheck reconciles on its own cadence, so the interval *is* the throttle and `lastRunTime` advances every run
- [X] T039 [US3] Persist the `HealthReport` in `internal/controller/dnscheck_controller.go` using `useDeterministicHealthReportName`, `SetControllerReference`, and `createOrReuseHealthReport`, then prune older reports as the other kinds do

---

## Phase 6: User Story 4 - Evaluation workloads never outlive their check (Priority: P3)

**Goal**: Every pod the controller creates is accountable and reclaimed, even after a mid-run crash.

**Independent test**: Kill the controller during a run; confirm no workload survives the sweeper's reclamation.

### Tests for User Story 4

- [X] T040 [P] [US4] envtest: every probe pod built for a pair carries an owner reference to its `DNSCheck`, in `internal/controller/dnscheck_controller_test.go`
- [X] T041 [P] [US4] envtest: deleting a check withdraws both the check-level and per-target metric series, in `internal/controller/dnscheck_controller_test.go`
- [ ] T042 [P] [US4] e2e: deleting a check mid-run removes its pods by owner-reference cascade, in `test/e2e/dnscheck_test.go`
- [X] T043 [P] [US4] e2e: killing the operator mid-run leaves an orphan that **both** reclamation paths can collect, in `test/e2e/dnscheck_test.go` — **reframed, see note below**

> **T043 reframed: asserts *reclaimable*, not *reclaimed*.** The orphan sweep
> only deletes pods that are terminal **and** older than its 5-minute minimum
> age, and it runs at startup then **hourly** — `defaultOrphanMinAge` and
> `defaultResweepInterval` in `internal/probe/sweeper.go`, neither overridden in
> `run.go`. Waiting out a genuine sweep would take over an hour of wall clock,
> which is not a viable e2e spec. The sweep already has direct coverage in
> `internal/probe/sweeper_test.go`.
>
> What is specific to DNSCheck — and what the spec now asserts — is that the pod
> left behind carries everything *both* reclamation paths need: the two labels
> the sweeper selects on, and an ownerReference. The ownerReference is the more
> important half here, because it collects the orphan the moment the check is
> deleted, with no sweep involved at all. Adapter probes have no such path.
>
> Worth noting separately: an orphan whose check *survives* can persist for up
> to ~65 minutes. That is pre-existing #220 behaviour, not something this feature
> introduces, but it is a candidate for making the sweeper's interval
> configurable.

### Implementation for User Story 4

- [X] T044 [US4] Set the owner reference on every probe request in `internal/controller/dnscheck_controller.go` — legal only because FR-031 co-locates pod and check in one namespace
- [X] T045 [US4] Withdraw both series sets on the `NotFound` path in `internal/controller/dnscheck_controller.go`

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T046 [P] e2e: a check in a namespace the operator does not own resolves `kubernetes.default.svc.cluster.local`, and the probe pod runs in the **check's** namespace, in `test/e2e/dnscheck_test.go` — this is SC-008 and the single most important e2e row
- [X] T047 [P] e2e: an `.invalid` target under a positive assertion yields `Fail`, and the same target with `absent: true` yields `Pass`, in `test/e2e/dnscheck_test.go`
- [X] T048 [P] e2e: a target with an explicit resolver pointed at an unroutable address and `absent: true` yields `Fail` or `Error` — never `Pass`, in `test/e2e/dnscheck_test.go` — the real-network counterpart to T017, proving an unreachable resolver cannot masquerade as a satisfied negative assertion (FR-014)
- [X] T049 [P] e2e: a check in a `restricted` PodSecurity namespace either admits the probe pod or surfaces an actionable per-pair `Error`, in `test/e2e/dnscheck_test.go`
- [X] T050 [P] e2e: a many-pair check with a deliberately short bound truncates without overrunning the bound, in `test/e2e/dnscheck_test.go`
- [X] T051 Measure the real per-pair cost during an e2e run and publish the pair-count-to-run-bound sizing guidance in `docs/reference/`, stating plainly that a maximum-size check does not complete at the default bound (FR-104b, SC-107) — the 1–3s figure used in planning is an estimate and must not ship as documentation
- [X] T052 [P] Add a `DNSCheck` section to `docs/reference/status-conditions.md` covering `Accepted`, `Ready`, and the new `Complete` condition — the page has a section per kind and DNSCheck is absent entirely (a gap inherited from #294). No guard test protects this page, so the omission is silent
- [X] T053 [P] Document `fathom_dnscheck_target_result`, its six labels, and its 288-series-per-check ceiling in `docs/guides/monitoring.md`, alongside the existing `fathom_check_result` guidance — operators need the ceiling for cardinality planning (FR-034)
- [X] T054 [P] Update `README.md` and `docs/architecture.md` for the new controller and its configuration surface
- [X] T055 Regenerate the CRD API reference via `go -C tools tool task docs:api-ref` and sync the Helm chart CRDs via `go -C tools tool task helm:sync`
- [X] T056 Run the full gate set: `go -C tools tool task ci`, plus `verify-generated`, `crd-compat` (expected to be a no-op — the schema is frozen), and `reuse lint`
- [ ] T057 Run `go -C tools tool task test-e2e` against Kind and record the outcome in the PR test plan
- [X] T058 Run `graphify update .` and include the refreshed graph in the PR

---

## Analysis remediation

`/speckit-analyze` on 2026-08-09 raised one CRITICAL and two HIGH findings. All
are resolved here; the file was renumbered rather than sub-lettered because no
task had been started.

| Finding | Severity | Resolution |
|---|---|---|
| RBAC doc guard test fails between the RBAC change and its justification | CRITICAL | The doc rewrite moved from Phase 7 into Phase 2 as **T014**, retargeted from an invented `docs/rbac-justification.md` to the canonical `docs/reference/operator-rbac.md`, and marked atomic with T013 |
| FR-014's unreachable-resolver case untested | HIGH | **T017** (envtest, fold semantics) and **T048** (e2e, real unroutable address) |
| `status-conditions.md` has no DNSCheck section | HIGH | **T052** |
| `monitoring.md` lacks the per-target gauge | MEDIUM | **T053** |
| FR-107b has no implementing task | LOW | No task needed — per-key workqueue serialisation guarantees it structurally; recorded in [contracts/reconcile-loop.md](./contracts/reconcile-loop.md) |
| FR-102 lacks a direct test | LOW | Absorbed into T015 |
| FR-107c thinly covered | LOW | Accepted — T003 documents the derivation |

---

> **T051 measurement, recorded here because the numbers matter.** Measured on a
> single-node kind cluster (Kubernetes 1.36, probe image preloaded, concurrency
> 4), resolving in-cluster names: 4 pairs / 1 batch took ~6.2s (twice), 12 pairs
> / 3 batches took 15.5s and 18.8s. That is ~5.5s per batch plus ~0.7s of fixed
> overhead — cost scales with **batches, not pairs**, confirming Pod startup
> dominates rather than the query. At the 48-pair schema maximum that is ~72s,
> so the default 10s bound truncates, exactly as FR-104b requires be documented.
> `docs/reference/configuration.md` carries the table and states plainly that
> 6s/batch is a floor, since a registry pull would be slower than a preloaded
> image.

## Dependencies & Execution Order

### Phase Dependencies

```text
Phase 1 (Setup)
    └─▶ Phase 2 (Foundational)  ◀── blocks everything; T013+T014 atomic
            ├─▶ Phase 3 (US1, P1) ── MVP, independently shippable
            │       ├─▶ Phase 4 (US2, P2)
            │       ├─▶ Phase 5 (US3, P3)
            │       └─▶ Phase 6 (US4, P3)
            └─▶ Phase 7 (Polish)
```

### User Story Dependencies

- **US1** depends only on Phase 2. It is a complete, shippable increment.
- **US2**, **US3**, and **US4** each depend on US1's fan-out and fold, but **not
  on each other** — they touch different concerns (status detail, history,
  lifecycle) and can proceed in parallel once US1 lands.

### Within Each User Story

Tests precede implementation. Within `dnscheck_controller.go`, tasks are
sequential — they edit one file — even where they are conceptually independent.

### Parallel Opportunities

- **Phase 2**: T005–T008 touch `internal/probe` and `internal/metrics`
  independently and can run alongside T009–T010. T013–T014 are strictly serial
  and strictly paired.
- **Phase 3**: T015–T021 are test tasks in one file; write them together, but
  note they are `[P]` by intent rather than by file separation.
- **Phase 7**: T046–T050 (e2e), T052–T054 (docs) are independent of one another.

### Critical Path

`T001 → T002 → T009 → T011 → T012 → T013 → T014 → T022 → T023 → T024 → T025 → T026 → T027`

Everything else hangs off it.

---

## Implementation Strategy

### MVP First

Phases 1–3 alone produce a working feature: checks evaluate on their cadence and
report a verdict. Stopping there would ship something honest and useful, though
without per-target detail it makes every failure an investigation.

### Incremental Delivery

1. **Phases 1–2** — plumbing plus the RBAC grant and its justification. No
   behaviour change, safely mergeable alone.
2. **Phase 3** — the MVP.
3. **Phase 4** — makes failures actionable. The highest-value follow-on.
4. **Phases 5–6** — history and lifecycle safety. US4 must not be deferred past
   the PR: re-introducing the orphan class #220 removed would be a regression.
5. **Phase 7** — the three documentation tasks (T051, T052, T053) are
   **release blockers**, not polish. Two of the three are unprotected by any
   guard test, which is exactly why they are called out here.

### Suggested PR Scope

One PR for the whole feature, matching how #265 shipped as #294. Phases 1–2
could split out if the diff proves unwieldy, but the RBAC change and the
controller that needs it should not land separately — a grant with no consumer
is harder to review, not easier.

---

## Notes

- **58 tasks**: 4 setup, 10 foundational, 13 for US1, 7 for US2, 5 for US3, 6 for
  US4, 13 polish.
- **The schema is frozen.** No task edits `api/v1alpha1/dnscheck_types.go`. If
  one appears to need to, that is a finding against feature 005, not a change to
  make here.
- **`status.lastRunTrigger` is deliberately untouched** — #264 owns the run-now
  trigger and is being taken next.
- **Three documentation pages have no guard test** — `status-conditions.md`,
  `monitoring.md`, and the sizing guidance. `operator-rbac.md` does have one, and
  it is the reason T014 cannot wait for Phase 7.
- **Never `git add -A` after `test-e2e`** — it rewrites
  `config/manager/kustomization.yaml`.
- Use an isolated `GOLANGCI_LINT_CACHE` in this worktree; the shared cache leaks
  false lint failures across parallel worktrees.
