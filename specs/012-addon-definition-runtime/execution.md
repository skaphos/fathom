<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Implementation execution evidence

The entries below preserve the chronological implementation record, including
failed trials and initially pending planning tables. The current one-to-one
requirement, numeric and lifecycle evidence map is
[qualification.md](qualification.md); initial pending rows are not the current
qualification status. Release dependencies and proposed review boundaries are
recorded in [delivery.md](delivery.md).

## Setup — 2026-09-20

- Baseline: main `3e7eafdea7a704b1cd8dd2b4935257b5f284220e`.
- Working branch: `docs/280-addon-definition-implementation`; feature planning
  artifacts were untracked before implementation. No user code changes present.
- Requirements checklist: 16/16 checked; read-only gate passed, no marker edits.
- No extension hooks configured. Existing Git/Docker/Helm ignore files cover
  relevant generated binaries, test output, local editor state and credentials.
- Go: go1.27.1 darwin/arm64. gopls, kind, docker, helm and helmfile are installed.
  Daemon/cluster availability is not yet verified.
- Design authority: accepted RFC 0001/ADR 0007 plus the three explicit user
  clarifications in spec.md and contracts/decision-supplement.md.
- #256 remains a separate release dependency; no resolution or release readiness
  is claimed. All runtime implementation tasks remain open until verified.

## Baseline verification

Initial package tests could not access the sandboxed Go build cache. Retried with
approved cache/network access. Registry, declarative, impersonation, CLI and app
package suites reported PASS. The app TestMain may skip envtest-dependent cases
when assets cannot start; this baseline is not full envtest/e2e qualification.
Command: `go test ./internal/adapter/registry/... ./internal/adapter/declarative/...
./internal/adapter/impersonation/... ./internal/cli/... ./internal/app/...`.
Reusable suites: registry concurrency/atomic registration, declarative evaluator
and version tests, impersonation factory tests, CLI fake-client command tests,
app option precedence and injected manager tests. Controller envtest and
`test/e2e/impersonation_test.go` remain later integration seams.

## Requirement coverage ledger

| Requirement | Planned implementation/test tasks | Evidence |
| --- | --- | --- |
| FR-001 | T007–T019,T023 | Pending |
| FR-002 | T011–T012,T024–T025,T038 | Pending |
| FR-003 | T024–T029,T039 | Pending |
| FR-004 | T020–T022 | Pending |
| FR-005 | T034,T050 | Pending |
| FR-006 | T036–T038 | Pending |
| FR-007 | T037,T039–T040 | Pending |
| FR-008 | T033,T044–T046 | Pending |
| FR-009 | T045–T046 | Pending |
| FR-010 | T004,T006,T012,T041–T043 | Pending |
| FR-011 | T003,T005,T017–T018,T026–T035 | Pending |
| FR-012 | T030–T032,T035,T056 | Pending |
| FR-013 | T004,T047,T051,T056 | Pending |
| FR-014 | T019,T036,T053–T054 | Pending |
| FR-015 | T052–T055 | Pending |
| FR-016 | T001–T002,T035,T049–T050,T055–T057 | Pending |

## Numeric boundaries

| Contract row | Test/result evidence |
| --- | --- |
| Definition | Pending |
| Strings/maps/lists | Pending |
| Names and identifiers | Pending |
| Binding and target scope | Pending |
| Version expressions | Pending |
| Selectors and field paths | Pending |
| YAML and annotations | Pending |
| API response | Pending |
| Lists and work | Pending |
| Target object traversal | Pending |
| Time | Pending |
| Results | Pending |
| Compiled snapshot cache | Pending |
| Scheduling | Pending |
| Recovery/retry | Pending |
| Internal request retry | Pending |

## Lifecycle cases

| Contract row | Test/result evidence |
| --- | --- |
| Missing definition | Pending |
| Valid definition added | Pending |
| Edited to valid revision | Pending |
| Invalid edit stored | Pending |
| Unknown kind submitted | Pending |
| Definition deleted | Pending |
| Same name recreated | Pending |
| Binding disabled/deleted or SA replaced | Pending |
| RBAC denies an API read | Pending |
| Revocation during run | Pending |
| Restart or partial informer sync | Pending |
| Binding/grants recover | Pending |
| Edit during evaluation | Pending |
| Two valid runtime names claim identity | Pending |
| New built-in collides | Pending |
| Deadline, size, parser or read budget exhausted | Pending |
| Evidence ages out | Pending |

## Payload field coverage

- Envelope, family and check: pending row-by-row admission/default/conversion tests.
- Common target and conversion rules: pending row-by-row admission/default/conversion tests.
- Workload → WorkloadCheck: pending row-by-row admission/default/conversion tests.
- CRD → CRDCheck: pending row-by-row admission/default/conversion tests.
- Condition → ConditionCheck: pending row-by-row admission/default/conversion tests.
- Field → FieldCheck: pending row-by-row admission/default/conversion tests.
- Webhook → WebhookCheck: pending row-by-row admission/default/conversion tests.
- CronJob → CronJobCheck: pending row-by-row admission/default/conversion tests.
- ConfigMap → ConfigMapCheck: pending row-by-row admission/default/conversion tests.
- AnnotationStaleness → AnnotationStalenessCheck: pending row-by-row admission/default/conversion tests.
- PodProjection → PodProjectionCheck: pending row-by-row admission/default/conversion tests.
- Declared reads and validation split: pending row-by-row admission/default/conversion tests.

## Foundation progress

- T004: `TestRuntimeLoadingConfiguration` and
  `TestRuntimeLoadingAdmissionRequiresElection` first failed to compile because
  the option and eligibility method did not exist. After implementation,
  `go test ./internal/app -run TestRuntimeLoading` passed. Configuration precedence
  is flag → FATHOM_RUNTIMELOADING_ENABLED → runtimeLoading.enabled → false.
  Election/namespace failures produce static eligibility reasons without making
  Options.Validate reject built-in startup. No runtime wiring is enabled yet.
- T006: verified pinned controller-runtime v0.25.1 manager/election sources;
  findings recorded in research.md. Lifecycle integration remains T041.
- T003 dependency correction: constants can precede US1, but schema-literal parity
  requires the generated US1 schemas. Keep T003 open until T023 verifies parity;
  this does not authorize executing runtime work without the bounds.

## Schema and compiler checkpoint

- T003: shared constants cover input, runtime, and leadership bounds. The external
  API test `TestDefinitionSchemaLimitParity` compares generated literals with
  authoring constants without introducing an API→authoring package dependency.
- T005: `internal/adapter/runtime/testutil/fixtures.go` enumerates numeric
  dimensions, boundary pairs and enforcement task IDs, plus byte/list/map/JSON/
  YAML/deadline fixture builders. This is fixture availability, not evidence of
  runtime enforcement; the runtime numeric ledger above remains pending.
- T010–T016: typed definition/binding/status contracts and all nine payloads are
  present, with generated CRDs/deepcopy. Definitions and bindings intentionally
  stay outside the `fathom` category because they are catalog/authorization
  objects, not health-intent checks. PROJECT and CRD kustomization include both.
- `go -C tools tool task test:api` PASS using pinned Kubernetes 1.37 envtest.
  Includes schema installation/CEL cost, `TestAddonDefinitionAdmission`,
  `TestAddonDefinitionBindingAdmission`, `TestDefinitionSchemaLimitParity`,
  `TestDefinitionMaximumCheckAdmissionCost`, and existing API regressions.
  The maximum-count fixture admits 512 Workload checks. This does not yet prove
  the full worst-byte-size/all-payload cost matrix required by T007–T009.
- Semantic validator tests pass for all nine payload kinds, malformed payload
  constraints, reserved keys, namespace scopes, and read-declaration expansion.
  T017–T019 remain open for full numeric/field/override coverage.
- Ordered compiler and immutable snapshot regression passes alongside existing
  declarative tests. No controller registers runtime adapters yet.
- Podman daemon confirmed running (arm64, rootful connection). Created isolated
  Kind cluster `fathom-addondefinition` with `KIND_EXPERIMENTAL_PROVIDER=podman`
  and the pinned `kindest/node:v1.37.0` fixture. This is cluster availability,
  not full-stack e2e success.

## Adversarial review and integration progress

The checkpoint review is in review.md. Reproduced and fixed comparator whitespace
and explicit-empty-outcome bugs; tightened generated selector and binding-name
schemas and corrected an admission test that could reject for an unrelated reason.
`go test ./pkg/addondefinition ./internal/adapter/declarative -count=1` passed
following the whitespace fix. Focused race tests for both packages and app passed.

Pinned lint reported 0 issues; Helm CRDs and API reference were regenerated.
REUSE lint passed (667/667 files at that checkpoint), and git diff --check passed.
Graphify update completed after source edits (AST-only); later edits require a
final refresh. Full CI and full Podman/Kind e2e are still running as of this entry.
The first e2e reuse attempt exposed Kind's Podman 6 label-template incompatibility;
name-scoped `kind get nodes --name` was verified and adopted for cluster reuse.

Renderer tasks T020–T022 await the user decision recorded as review R1. Do not
invent resource plurals or convert all mixed-scope requestedReads into cluster-wide
grants. Phase-dependent runtime tasks remain unimplemented, not “passed by default”.

## Verification outcome — 2026-09-20

- Full `go -C tools tool task ci`: PASS (version/alert/CRD compatibility checks,
  lint, envtest-backed unit suites, staticcheck, govulncheck and both builds).
  govulncheck found no called vulnerable symbols. Existing sanctioned CRD
  compatibility entries were reported; no new incompatible change was found.
- Latest `go -C tools tool task test:api helm:sync docs:api-ref`: PASS, including
  all nine payload admission/default fixtures, status limits, field pruning,
  and the corrected family-cap test. Distribution/API docs regenerated.
- Fixture parser/binding tests: PASS, 60.9% testutil package coverage. The first
  full-CI profile predates these tests; a final full test/coverage refresh is
  running. No threshold has been lowered.
- Full Podman e2e attempt: FAIL during addon installation, before Fathom image
  deployment or Ginkgo execution. Cilium, istio-base, node-local-dns,
  kube-state-metrics and external-dns installed. cert-manager, Argo CD,
  metrics-server, Envoy Gateway, external-secrets and azure-workload-identity
  failed after roughly 11 minutes with API/watch/TLS connection failures.
  Bounded kubectl reads independently failed. VM kernel logs recorded a manager
  process killed by a pod memory cgroup; the exact addon could not be mapped
  because the API was unavailable. The VM had 3,608 MiB RAM and no swap, but this
  evidence alone does not establish VM capacity as the sole cause.
- The e2e task's deferred cleanup removed `fathom-addondefinition` and its node.
  An attempted targeted cancellation found Helmfile already exited and sent no
  signals. Podman VM settings were not modified.
- Mandatory pre/post implementation extension file remains absent; no hooks to
  dispatch. The failed qualification gate remains open; no release/PR readiness
  or full implementation completion is claimed.

## Final checkpoint checks and development-cluster validation

- Refreshed `go -C tools tool task test`: PASS for all non-e2e packages, including
  final status/default/unknown-field fixtures. `scripts/check-coverage.sh
  coverage.out`: PASS for every package at unchanged thresholds. Authoring
  package 77.81%, declarative 87.56%, runtime fixture helpers 60.87%.
- Final `reuse lint`: PASS, 670/670 files; `git diff --check`: PASS.
- Final AST-only `graphify update .`: PASS. Generated graph artifacts and its
  backup are intentionally not hand-edited or reviewed as source.
- User additionally authorized validation against an existing development
  cluster. Used its explicit kubeconfig/context for read-only inventory and
  server-side dry runs; private context/endpoint identifiers are omitted here.
  Server Kubernetes **v1.36.2**, linux/amd64. Existing Fathom cert-manager,
  Cilium and external-secrets checks reported Pass. Healthy addon deployments
  included cert-manager, Cilium, Envoy Gateway, external-dns, external-secrets,
  Istio and metrics-server.
- Both generated CRDs passed `kubectl apply --server-side --dry-run=server
  --field-manager=fathom-definition-validation -f <definition-crd>
  -f <binding-crd>` on that real API server. Neither CRD was installed beforehand;
  dry-run persisted no changes. This verifies schema/CEL compatibility on 1.36.2,
  not new runtime execution or publication behavior. It does not replace the
  failed full Kind qualification or complete T050/T055.

Implementation remains at the authoring checkpoint with 13 of 59 tasks checked.
Renderer clarification R1 is pending. No commits, pull requests, deployment of
this implementation, or runtime release were created.


### Offline renderer clarification — option A

The user selected offline rendering with safely determined grants and explicit
manual-completion diagnostics for unresolved custom-resource grants. No live
discovery, guessed pluralization, or scope broadening is authorized by this
decision. R1 is resolved as a design choice; renderer implementation and tests
remain required before T020 is complete.


### T020 — offline staged renderer

Implemented pure `PlanGrants` and `Render` in pkg/addondefinition. Typed evaluator
reads retain exact namespaces and named get restrictions. Webhook verification
uses discovery.k8s.io EndpointSlice lists, matching the evaluator. Arbitrary GVKs
and requestedReads produce manual-completion diagnostics; they cannot widen grants.
Name overrides preserve default-only grants and require explicit review.
The caller supplies the operator SA and bundled inventory; same-SA delegation and
built-in collisions are rejected. Output strips live metadata, stages a dedicated
SA and scoped impersonation Role, and ends with an inadmissible disabled binding
containing empty UIDs. CLI inventory wiring is still T021/T022.

Validation: grant regression tests first failed because PlanGrants did not exist;
`go test ./pkg/addondefinition` then passed with grant and serialized manifest
tests. Tests verify mixed-scope isolation, custom-resource diagnostics, deterministic
nonmutating output, actual EndpointSlice helper grants, incomplete UID staging,
identity mistakes, and no broad impersonation permission. T020 complete; 14/59 tasks
checked. Runtime activation and full implementation remain incomplete.

Renderer checkpoint checks: `go -C tools tool task lint` passed (0 issues);
`go test -race ./pkg/addondefinition` passed; `git diff --check` passed.
Graphify AST update completed. No cluster resources or Git commits were created.


### Authoring CLI and generated inventory — T021/T022

Added read-only `definition render`, `bind`, `collisions`, and `drain` commands.
Render does not load kubeconfig; strict bounded file input rejects unknown fields
and multiple YAML documents. Bind reads live definition/SA UIDs, checks exact
reviewed spec equality (including admission defaults), and emits a disabled
binding with the bounded union of primary/helper scopes. It performs no writes.
Collision preflight uses bundled inventory and binary version/build, rejects
missing metadata/inventory, and distinguishes collision (1) from unverifiable (2).
Drain uses independent uncached reads, progressing renewal of the same Lease
epoch, current generation and acknowledgement conditions, at most 16 Lease reads
plus one binding read, 5-second requests, and a 15-second outer deadline.

Pinned `gen:runtime-definitions` generates inventory from app.BuiltInAdapters and
nine validated non-built-in sample definitions. verify-generated includes both.
Build/distribution tooling stamps the source revision alongside the version.
The CLI imports no operator/app/adapter/controller implementation packages.

Focused package tests passed for CLI, pure authoring, and rbacgen. Regression
cases include offline/no-kubeconfig rendering; strict/multidocument rejection;
live UID/spec mismatch/no-write binding; collision result codes and provenance;
denied/stalled/changed/final-changed leadership, stale generations, active runs,
old epochs, missing conditions, and bounded reads; inventory and sample drift.
The synthetic Lease clock is in 2040 to ensure CLI wall-clock offset is irrelevant.
T021/T022 checked: 16/59 total. Real manager drain integration remains US3.

Remote checkpoint fc9ec3a was pushed to docs/280-addon-definition-implementation.
SSH agent signing failed; the existing GitHub CLI login successfully pushed via
HTTPS without changing Git configuration. Subsequent implementation is pending
its own validated checkpoint push.

Adversarial checkpoint R7: a wire-round-trip regression failed because metav1.Time
truncated microseconds from Lease acquisition time. Binding leaderEpoch now uses
metav1.MicroTime, matching coordination/v1 Lease. Race-enabled CLI, authoring and
generator suites pass after the fix; regenerated API artifacts are under validation.

T023 complete: pinned generate/manifests/helm:sync/docs:api-ref tasks passed,
and test:api passed against envtest 1.37 (API and authoring packages).
Registration, conversion, rendering and artifact evidence are recorded above.
17/59 tasks are checked; remaining US1 exhaustive contract matrices still precede
runtime authority/lifecycle implementation.

Authoring checkpoint final gates: lint (0 issues), fathomctl-build,
verify-generated, test:api, focused race suites, REUSE (697/697), and diff checks
all passed. The built binary rendered the generated webhook example successfully
with a nonexistent kubeconfig, confirming offline operation. No cluster writes
were performed. Full runtime qualification and final adversarial review remain
open; this checkpoint must not be interpreted as runtime release readiness.


### US1 contract matrix completion

T007–T009 and T017–T019 are complete. Admission suites now cover every payload's
required fields, enums, declared scalar/list/map bounds, all published defaults,
immutable envelope identity, union mismatches, maximum binding/status CEL cost,
and same-UID SA-name retargeting. Rejection tests assert the corresponding API
error category so another constraint cannot silently mask a missing bound.
The compiler suite covers non-default mappings across all nine payloads, threshold
consumption and resolved override validation, inherited Optional behavior,
version-reference ambiguity, order, and owned snapshots. Pure scope validation
rejects missing primary/helper permissions without dropping targets; transport
must still enforce the discovered scope independently in US2.

`TestCanonicalDefinitionByteBoundary` admits exactly 262144 canonical spec bytes
and rejects 262145 for DefinitionTooLarge. Additional tests cover structural caps,
UTF-8 bytes versus runes, comparator/alternative/range limits, selector term/value
limits, and closed resource/discovery grammar. Existing fixture tests cover bounds
that cannot exceed the cap within the typed schema (such as binding spec size).
Focused authoring/declarative tests and pinned test:api pass. Scope intersection
was first demonstrated failing because ValidateScope did not exist, then passed
with explicit all-target validation. US1 complete; 23/59 total tasks checked.

### US2 authority and transport primitives (partial)

Added an isolated runtime factory with live definition/binding/ServiceAccount UID
validation, manager/builtin/shared identity rejection, canonical final-hop
impersonation headers, and a fresh delegated discovery mapper. Tests inject
inherited and wrapper-added admin headers and mutate the guard callback's copy;
neither can change the captured reader identity. Forbidden discovery never falls
back to manager reads. CompileRuntimeScoped checks all effective primary/helper
scopes after policy resolution, allowing an authorized override while rejecting
out-of-scope defaults and overrides before reads.

The new shared Budget and Guard bound read-only routes, actual namespaces and
cluster scope, discovered GVK scope, request/run deadlines, process-wide QPS,
request retries, decoded success/error bodies (including gzip), cumulative bytes,
page/object counts, JSON nodes/depth and visits. Continuation at the object cap
fails rather than publishing a truncated list. Tests cover all nine payloads'
discovery expectations, Pod/EndpointSlice helpers and cancellation while blocked
reading a response body. The retry regression first accepted a fourth request;
it now rejects that request before network I/O.

Focused race tests passed for runtime, impersonation, declarative and authoring
packages. These primitives are not an activated runtime: evaluator pagination,
parser budgets, control-plane fence accounting, scheduling, lifecycle publication
and cluster qualification remain open. T024–T029 remain unchecked until their
complete integrated acceptance criteria pass; 23/59 tasks remain checked.

Added WalkPages for one-page-at-a-time consumption through the guarded reader.
Its tests verify continuation propagation, repeated-token rejection, restart
allowance shared across all lists in one run, and discarding earlier derived
evidence before restarting an expired snapshot. These tests first failed because
the walker was absent, then passed with the implementation. The walker still
needs integration into declarative collection/helper paths; it does not by itself
make existing evaluators pagination-complete. Pinned lint reports zero issues for
the authority/transport checkpoint; subsequent pagination lint is recorded below.

Checkpoint validation including pagination: pinned fmt/lint passed (0 issues),
focused race suites passed, REUSE passed (710/710 files), git diff --check passed,
and graphify update completed. Runtime activation and full cluster/adversarial
qualification remain pending; no real-cluster resources were changed here.

### US2 evaluator pagination and parser integration

Every declarative collection read now uses a common page-consumer seam. Runtime
consumers invoke the shared-budget walker; compiled-in adapters retain their
existing client behavior. Field, Condition, AnnotationStaleness, workload Pod,
PodProjection and webhook EndpointSlice evaluators score one page at a time.
They retain derived results/counts only, restore the collection's pre-run state
on snapshot restart, and refuse partial completion. PodProjection retains at
most five offending names while counting all matches. Tests cover every runtime
collection/helper path and final-page contributions to verdicts.

`TestRuntimeFieldConsumesFinalPageAndRestartsTransactionally` initially failed
because the integration seam was missing. `TestRuntimeMissingOptionalAPIIsCompletedSkipped`
then exposed a walker regression: mapper absence canceled the whole budget.
Expected API absence now returns to the evaluator for grading, while operational
failures still abort. Both tests pass.

T028 complete: a narrow ExecutionBudget interface carries cancellation, traversal
charges and pagination into runtime evaluators without importing runtime into
declarative. The runtime-only client reserves a full bounded object traversal
before evaluator/version-helper inspection and charges the reservation scan as
well. YAML checks bound bytes before parsing, then reject excess nodes/depth and
aliases before expansion; syntax errors retain the declared invalid outcome.
Annotation values are bounded before timestamp/JSON parsing. Scoped runtime
execution refuses a missing shared budget before I/O. The compiler's unscoped
conversion API remains available for isolated authoring tests.

`TestRuntimeConfigMapParserBoundaries` first accepted each over-limit/alias case;
it now passes exact/over byte, node and depth boundaries plus alias rejection.
`TestRuntimeAnnotationByteBoundary` covers 1024/1025 bytes.
`TestRuntimeEvaluatorAndVersionHelperShareVisitBudget` verifies exhausted work
prevents publication from both paths. All-disabled checks intentionally skip
version detection and perform no reads. The adapter-wide race suite passes,
including shipped adapters using the shared evaluators. Total checked: 24/59.

Runtime activation, authority/final-fence accounting, result serialization caps,
scheduling, lifecycle wiring and full Kind qualification remain outstanding.
The previous Kind bootstrap failure is still not a successful e2e qualification;
this checkpoint does not claim release readiness or completed adversarial review.

Final evaluator checkpoint checks passed: pinned fmt/lint (0 issues), adapter-wide
race suite, REUSE (715/715), diff whitespace validation and graphify update.
No cluster resources were changed during this checkpoint.

### US2 result gating and supervised execution

T030 complete. The shared result gate validates accumulated runtime evidence
before another declared evaluator executes and again at the final result boundary:
1000 entries, 256 KiB of JSON-serialized adapter.Result, 1024 UTF-8 message bytes,
and 32 detail entries. It bounds raw string allocation before JSON serialization,
counts escaping in the exact serialized limit, and rejects malformed/unfinished
outcomes. Completed Pass/Warn/Fail/Skipped remain evidence; a failed attempt
returns no partial evidence. Sealing copies result slices and detail maps so the
publication candidate does not alias evaluator-owned state. A separate fixed
failure summary remains available even when evidence capacity is exhausted.
Authority/revision publication precedence remains the controller work in T040.

Boundary tests cover exact/over entries, message bytes, detail count and serialized
bytes (262144/262145), UTF-8 versus rune count, JSON escaping, owned snapshots,
first-error preservation and deadline precedence. The integration test first
showed an overlong ConfigMap message accepted with the next declared check still
executing; it now fails for ResultLimitExceeded after one read and returns no
partial evidence. The annotation-input boundary test now uses valid JSON timestamp
padding to isolate the input cap from the independent error-message output cap.

T031 complete at the execution-component level. Execute runs compilation and
adapter evaluation synchronously in the invoking worker, recovers panics in that
same goroutine, cancels child contexts and releases the caller's held slot exactly
once. It does not launch an evaluator goroutine or release its slot just because
a deadline fired. The compiler receives a one-second child deadline; expired
compilation cannot start evaluation. Successful execution leaves the shared outer
budget available for final fencing. The caller must create that budget with the
check timeout before pre-validation; full fence accounting remains T029/T039.

Runner tests cover compilation/evaluation panic, normal compilation/evaluation
errors, missing compiled output, result overflow, delayed cooperative cancellation,
compile timeout, first failure, slot release, a subsequent healthy run, and a
concurrent healthy peer. Adapter-wide race tests pass. The result/runner APIs are
not production wiring or a completed scheduler. T032/T033 still own admission,
fairness, cache/backoff and actual shared slots. Total checked: 26/59.

Final result/runner checkpoint checks passed: pinned fmt/lint (0 issues),
adapter-wide race suite plus a final focused race rerun, REUSE (720/720),
git diff --check and graphify update. Runtime remains default-off; final
adversarial review, full cluster qualification and release gates are still open.

### US2 scheduling, worker isolation and revision cache

T032/T033 complete at the component level. A manager-owned Scheduler admits at
most four active runtime runs, one per definition name and one per namespaced
check name. Stable name keys prevent UID/revision recreation or retargeting from
escaping held slots or backoff. It coalesces queued work and notification wakes,
rotates ready definitions round-robin, and resolves live revisions only after
admission. A fixed four-worker Run loop waits on notifications/deadlines, joins
all handlers on cancellation, and retains slots until handlers return. Unexpected
handler panics cancel children, release admission and retry without stopping peers.
The scheduler is separate from existing built-in controller workers; manager
leadership/startup wiring is still T047.

Retries follow 5/10/20/40/60 seconds with bounded positive jitter. The final delay
is clamped to the absolute 60-second MaxRetryBackoff ceiling (so capped retries
may receive zero added jitter); missing inputs poll after exactly 60 seconds.
Informer churn cannot advance due times. Deleted failed checks retain temporary
backoff state until its due time. Unchanged failure reasons are deduplicated;
changed failure events have a five-second per-check cooldown. A regression first
showed completion/recreation resetting that cooldown; separate expiring event
timestamps now preserve it without retaining idle queue records indefinitely.

Cache retains at most 128 idle LRU revisions plus four active reservations.
Definition UID/generation, schema/semantics version, operator build and adapter
version all participate in its key. Active snapshots cannot be evicted, releases
are idempotent, and compilation occurs outside the mutex within a one-second
child context. Panics/errors/cancellation release reservations and do not cache
failed construction. Concurrent equivalent constructions reuse the already
published immutable entry.

Named scheduler/cache/pool tests cover at/over concurrent bounds, definition
fairness, duplicate completions, deduplicated wakeups, retarget/delete/recreate
churn, exact retry and missing-input delays, jitter ceiling, event cooldown,
LRU refresh/eviction, active pins, all revision identity fields, cancellation,
compile panic cleanup and concurrent off-lock construction. A two-transport test
proves the 10 QPS/burst-20 bucket is shared: two independent burst buckets would
incorrectly allow all 40 requests immediately. Adapter-wide race tests pass.
Total checked: 28/59; production activation and cluster qualification remain open.

Final scheduler/cache checkpoint checks passed: pinned fmt/lint (0 issues),
adapter-wide race tests plus a final runtime race rerun, REUSE (726/726),
git diff --check and graphify update. Every worker handler also receives an
outer 30-second deadline covering setup/execution/final-validation phases; the
check-specific shared budget still needs to be created before pre-validation.
No cluster resources changed. Runtime activation and final adversarial/cluster
qualification remain pending.

### US2 actual-request permission diagnostics

T034 is complete at the component level. Each delegated Guard records only
charged network attempts, independently of metrics, and produces bounded
PermissionsVerified status data. Before requests it reports Unknown/NotEvaluated;
a 403 reports False/AccessDenied even when its body is missing or oversized.
Transport failures, incomplete requests and non-success responses report
Unknown/AccessCheckUnavailable; later successes cannot erase that uncertainty.
Successful responses confirm only the requests actually made. Local scope
rejections do not become API permission observations.

The diagnostic also compares requestedReads locally with typed default reads,
helper reads, group/version discovery and actual observed requests. Missing
resource names, verbs, discovery declarations, helper list access and effective
name overrides are reported without changing request authorization. Arbitrary
GVK mappings and override names remain explicitly unresolved rather than guessing
plurals or claiming full coverage. Declarations never authorize new grants,
and diagnostics make no speculative requests or SubjectAccessReviews.

TestPermissionsUseOnlyActualRequests, TestPermissionDeclarationsAreAdvisory,
TestPermissionFailuresSurviveLaterSuccess,
TestPermissionsObserveOverridesAndConcurrentRequests,
TestPermissionsDoNotCountLocallyDeniedRequests,
TestPermissionDeclarationHelpersAndUnknownMappings and
TestPermissionDiscoverySuccessDoesNotClaimTargetAccess cover these cases,
including concurrent snapshots and bounded messages without raw transport errors.
The initial tests failed before the Diagnostics API existed, then passed.
Controller status/event publication remains part of the lifecycle integration.
Total checked: 29/59.

### US2 shared control-plane and evaluator budgets

T029 is complete at the component level. NewRuntimeControlReader constructs a
fresh uncached reader for each run, with a fixed mapper restricted to definitions,
bindings, service accounts, AddonChecks and the manager Lease. ControlGuard pins
the definition/check/Lease names and operator namespace; only the binding
inventory may be listed. Target resources, discovery, writes, watches, subresources
and unrelated metadata are rejected. This reader retains manager authentication
while stripping inherited impersonation and transport wrappers. Delegated clients
continue to use only the canonical bound service-account identity.

Both transports use the same Budget and process-wide rate limiter. Metadata reads
therefore share requests, decoded bytes, objects, visits, retries and remaining
deadline with delegated discovery and evaluator reads. Manager observations never
enter PermissionsVerified. The fixed mapper avoids unbudgeted discovery on the
control path. NewRuntimeControlReader exposes only client.Reader methods.

TestRuntimeControlReaderSharesBudgetWithoutSharingIdentity exercises actual HTTP
authority reads, delegated root/resource discovery and ConfigMap reads, then
AddonCheck/Lease fence reads. It verifies identity headers, no target fallback,
no manager discovery and no extra request after the shared request cap.
TestControlGuardRestrictsManagerReads exercises the exact metadata allowlist.
TestControlAndDelegatedRequestsShareBudget,
TestControlAndDelegatedRequestsShareResponseBytes and
TestControlGuardSharesRemainingDeadline verify combined request/byte/deadline
limits. Existing runner, request-retry and pagination tests cover the one-second
compile child, two retries and one shared pagination restart.

Controller pre/final revision comparison and publication wiring remain T039.
This checkpoint supplies and tests their bounded reader, not an implemented
publication fence. Total checked: 30/59. Runtime remains default-off.

Final diagnostics/control-reader checkpoint checks passed: pinned fmt/lint
(0 issues), adapter-wide race tests, REUSE (732/732), git diff --check and
graphify update. No cluster resources changed. Full real-cluster qualification,
controller wiring and final adversarial review remain pending.

### US2 completion — authority scope, transport boundaries and hostile input (T024–T027, T035) — 2026-09-21

T024–T027 target files already existed when this session began; the work was
gap closure against the contract rows, not greenfield. A twelve-assessor audit
(two opposing lenses per task plus numeric-row, boundary-discipline, test-theater
and hostile-input sweeps) was adversarially reconciled into 32 work items; ten
assessor claims were discarded as unreachable or wrong and are recorded in that
reconciliation rather than implemented.

Source changes. `ClientFor` now calls `definitions.ValidateScope` before
`buildGuard` and fails closed with `ScopeDenied` (T025's "intersect explicit
target scope"): the intersection previously existed only downstream in
`declarative` and in the transport guard, so a caller ignoring
`Binding.Spec.TargetScope` still received a working impersonating client.
`NewRuntimeFactory` now rejects any reader that is not a `RuntimeControlReader`,
making the uncached-control-plane requirement enforceable rather than comment-only.
`runtimeAdapter.Run` demands the shared budget for scoped *and* unscoped adapters
before any read; previously an adapter from the exported `CompileRuntime` ran with
`ec.runtime = true` and no budget, silently skipping the per-object node/depth caps
and the evidence cap. `scheduler.go` pins `MaxRunsPerCheck` and
`MaxQueuedWakesPerCheck` with compile-time array assertions so neither can be
retuned without the structural change a value above one would require.
`transport.go` records why node/depth inspection is scoped to 2xx bodies: a
non-2xx body never becomes a target object, so charging it would let a hostile
error body exhaust a run's visits and mask the real failure, while
`ChargeResponse` still caps it unconditionally.

Numeric-row enforcement. `TestNumericRowsAreEnforced` drives `testutil.Boundaries()`,
which until now had no callers at all. It went from 9 enforced / 56 skipped to
31 enforced / 34 pinned, with skip hatches only for rows whose enforcer lives in
another package, each naming that enforcer.

Tests added or materially changed (36 functions):
`TestNumericRowsAreEnforced`, `TestBudgetRequestContextClampsToRemaining`,
`TestBudgetDeadlineClamps` (rewritten), `TestGuardRequiresExplicitScopeAndBudget`,
`TestTransportRejectsUnauthorizedRoutesBeforeIO` (extended to 8 write-method and
watch subtests), `TestTransportBoundsCallerSuppliedListOptions`,
`TestTransportChargesRetriesAndDiscoveryToRunBudget`,
`TestTransportDeliversDecompressedBodies`, `TestTransportForwardsContinuationBelowCap`,
`TestTransportHelperReadsShareIdentityAndScope`,
`TestTransportPassesThroughBoundedErrorBodies`, `TestTransportRejectsUndecodableResponses`,
`TestTransportStopsAfterFirstFailure`, `TestTransportTreatsNamespacelessListAsClusterScoped`,
`TestWalkPagesClampsCallerPageLimit`, `TestWalkPagesRefusesContinuationAtRunObjectCap`,
`TestWalkPagesRejectsUnusablePreconditionsBeforeIO`, `TestWalkPagesStopsAtTheRequestCap`,
`TestWalkPagesStopsOversizedPagesBeforeConsuming`,
`TestWalkPagesStopsWhenTraversalBudgetIsSpent`, `TestSchedulerPerCheckRunAndWakeCaps`,
`TestCacheRejectsIncompleteRevisionKey`, `TestEvidenceStringBudgetRejectsBeforeMarshal`,
`TestDiscoveryExpectationsRejectContradictoryScope`,
`TestRunnerLeavesNoUnsupervisedEvaluatorGoroutines`,
`TestControlGuardClampsRequestToMaxRequestDuration`,
`TestRuntimeSurvivesHostileDefinitionAndResponses`,
`TestRuntimeAcceptsResponsesExactlyAtTheirBounds`,
`TestRuntimeHealthyPeerCompletesAfterHostileNeighbour`,
`TestRuntimeWalkStopsAtRunObjectCapThroughRealClient`,
`TestRuntimeClientRefusesOutOfScopeDefinition`,
`TestRuntimeClientRefusesUnsatisfiableTargetScope`,
`TestRuntimeClientConstructionFailsClosed`, `TestRuntimeClientRefusesRedirects`,
`TestRuntimeDiagnosticsSurviveWithMetricsOff`, `TestRuntimeDiscoveryIsKeyedPerIdentity`,
`TestRuntimeAuthorityInventoryStaysInsideRunBudget`,
`TestRuntimeAuthorityRejectsIdentityConfusion` (per-case reason assertions added),
`TestRuntimeExecutionRequiresSharedBudget` (version-source cases added).

Mutation testing. A passing test is not evidence, so every restored guard was
verified by `go test -overlay` against a mutated copy, never by editing the tree.
Twenty mutations across the three packages were applied and all are now caught,
including: dropping the 30s run-duration clamp; off-by-one on `charge()`; doubling
the per-request response cap; deleting the transport entry short-circuit; admitting
non-GET methods; dropping the forced `Accept` header; dropping the depth half of
the node/depth check; returning truncated success at the continuation cap; deleting
the per-check active-run guard; removing the `ValidateScope` call; forcing the
service-account UID comparison true; making the mandatory-budget check permissive;
deleting the 5s per-request clamp; removing the watch rejection, the namespace
allowlist, the global concurrency ceiling, the impersonation-header strip and the
page-limit clamp.

Two mutations initially survived and both were test defects, not production
defects. The rewritten `TestBudgetDeadlineClamps` had swapped a grossly-over-cap
input for cap+1ns while widening tolerance to 250ms, so deleting the run-duration
clamp left the suite green — the contract's `min(timeout, 30s)` row was unguarded;
it now reads remaining time at read time and accepts only `(want-50ms, want]`.
`TestRuntimeExecutionRequiresSharedBudget` nilled `VersionSource`, deleting the one
pre-step read (`Engine.detectAndGateVersion`) that escapes when `Run` does not fail
closed, so its error assertion was satisfied by a redundant downstream guard;
version-source cases were added and the mutation now fails them with
"unbudgeted run performed 1 reads" while the original cases still pass — the
distinction that proves the new cases carry the guard.

No input-triggerable process-wide failure was found. Every hostile body driven
through the assembled compiler → supervisor → guard → client path (oversize node
count, oversize depth, oversized page, oversized decompressed body, continuation at
the object cap, 403, non-object and items-less bodies) produced the correct named
bounded failure with no evidence and a released slot; no panic escaped, no
goroutine leaked, and a healthy peer definition still completed. Goroutine-delta
assertions were added for the panicking-compiler, panicking-evaluator,
cancelled-evaluator and budget-exhausted cases; before this session the repository
had no `NumGoroutine`/`goleak` usage at all.

Two contract observations, not defects. A single maximal 32,768-node object costs
roughly 98,300 of the 100,000 `MaxObjectVisits` because the guard charges one visit
per node during inspection and the evaluator charges two during pre-traversal, so a
run can afford exactly one maximum-size object; that is
`contracts/runtime.md`'s shared visit counter working as written, with under 2%
headroom. A maximal dotted field path (2,063 bytes) makes a FieldCheck "field is not
set" summary exceed `MaxMessageBytes`, so such a definition can only fail with
`ResultLimitExceeded` once it scores an object — bounded and correct, an authoring
foot-gun rather than a runtime bound.

Gate outcomes: `go build ./...` clean; `go vet ./...` clean; `gofmt -l` empty;
pinned `task lint` 0 issues; `-race` clean on runtime, declarative and impersonation;
`-count=3` clean on the runtime package (goroutine baselines are not flaky);
full `go test` green across `./internal/adapter/...`, `./pkg/...` and `./internal/cli/...`.
Pinned `task staticcheck` reports one pre-existing ST1000 on the generated
`pkg/addondefinition/inventory_generated.go` (unchanged since the previous commit;
its package comment is emitted by the T022 generator, so the fix belongs to that
generator and to T057's gate run, not to a hand edit of generated output).

`runtime.Execute` still has no production caller — the component test is now the
only thing driving the real path, and the production caller remains owed by
T039/T040. US2's real-cluster acceptance stays open until US4 (T050/T055) as the
task text requires; this is the harness checkpoint only. Total checked: 35/59.
Runtime remains default-off.

### US3 phase A — registry snapshots, leadership session, CLI drain (T036, T041, T043) — 2026-09-21

Three tasks in independent packages, built in parallel, then adversarially
reviewed and mutation-verified. T043 repeated the US2 pattern: its production
file `internal/cli/definition_drain.go` was byte-identical to HEAD, so the task
was entirely test coverage, not new code.

T036 — `internal/adapter/registry`. An owner-keyed copy-on-write runtime
snapshot layer now sits beside the untouched built-in map. `SetRuntime` validates
the full (definition UID, generation, schema, semantics) revision plus
operator-build/adapterVersion provenance, reads adapter capabilities and builds
the replacement off-lock, then publishes with one atomic store. `RemoveRuntime`
removes only a matching UID (lifecycle row "Definition deleted | Remove matching
UID only"). `Resolve` raises dispatch barriers for built-in collisions in BOTH
directions, for two runtime owners claiming one identity (fail closed, no
arrival-order winner) and for un-admitted dispatch. Decided and documented: an
EQUAL generation republishes, because a republished generation is a recompile of
the same definition revision under a new schema, semantics or operator build, and
refusing it would pin dispatch to the superseded compilation; only a strictly
older generation is stale. Built-in registration, lookup and capability semantics
are unchanged and still proven by the pre-existing suite.

T041 — `internal/app/runtime_leadership.go` (new). Configured namespaced Lease,
process-unique holder, terminate on loss with no reacquisition, per-binding
revoke/restore and active-run accounting, and a takeover grace measured on
monotonic elapsed time that ignores wall-clock jumps.

T043 — `internal/cli`. All four epoch fields (leaseUID, holderIdentity,
acquireTime, leaseTransitions) are now individually pinned, per leadership.md's
"Epoch equality uses all four fields ... resourceVersion is excluded because
normal renewal changes it"; the resourceVersion carve-out keeps its own test.
Deadline, poll pacing, the 16-read ceiling with a reserved final read, and the
no-writes property are all asserted.

52 test functions across the three packages. Names are listed in the git history
for this commit; the load-bearing ones are called out below by what they guard.

Adversarial review rejected all three on first pass. Reviewers applied mutations
to the production code and found 23 behaviours that NO test caught, including:
the empty- and duplicate-addon-type rejections (no coverage at all); "ordered by
definition UID" and collision claimant order (both unpinned, so a reversed sort
was invisible); `DispatchBarrier.Is` returning true for everything, making a
barrier indistinguishable from ErrNotFound; `Revoke` cancelling only the first
in-flight run of a binding; `ActiveRuns` summing across all bindings instead of
per binding, which is what drain correctness rests on; `Epoch()` handing out
aliased internal state; and deleting either the acquireTime or the
leaseTransitions comparison from the CLI's epoch equality.

Two findings were structural rather than per-test. First, a prerequisite that
failed OPEN: `waitForCacheSync` defaulted to `func(context.Context) bool { return
true }`, so a T047 that forgot to wire `mgr.GetCache().WaitForCacheSync` would
silently satisfy "the runtime admission runnable starts only after election and
cache sync" with a no-op — undetectable, because the default WAS the mutation.
The gate now has no usable zero value: `NewRuntimeLeadership` leaves it nil,
`WithCacheSync` is the only way to supply it, `Start` returns
`ErrCacheSyncUnwired`, and every Admit/AcknowledgeDrain/EpochValid refuses with
the same error.

Second, a duplicated admission gate. The registry had grown
Open/CloseRuntimeDispatch while the leadership session independently implemented
a complete gate of its own, with neither file referencing the other. Resolved by
decision: the REGISTRY keeps the barrier, because a barrier is only worth
anything at the enforcement point where dispatch happens, and the leadership
session is the single decider and only intended driver. `OpenRuntimeDispatch`
now requires the driving session's holder identity and returns
`ErrAdmissionDriverRequired` with the gate left closed when it is absent, so a
mis-wiring fails loudly instead of admitting undriven dispatch; the recorded
driver is surfaced in the revocation the registry reports, and pinned by
TestAdmittedDispatchIsAttributableToItsDriver. `CloseRuntimeDispatch` needs no
driver because barring dispatch is the safe direction and must never be refused.

The coupling itself — a leadership session actually driving the registry gate —
is deliberately still absent, because that wiring is T047. Independent
verification states the position precisely: one enforcement point, one decider,
zero code connecting them. The disagreement window is real but unreachable in
production today, since runtime leadership has no production caller at all. T047
must close it, and the fail-loud driver requirement is what makes a wrong wiring
visible when it does.

Mutation verification: 57 mutations applied via `go test -overlay` against scratch
copies, repository never written. 55 caught. The 23 originally-surviving ones are
all now caught, and 2 of the verifier's 33 new ones survived and were then closed
by hand (the registry's driver attribution, and `stopGracefully` skipping the work
drain — the latter verified by observing "stopGracefully returned after 1.917us,
less than the 250ms drain bound"). One reported survivor proved to be a no-op
mutation rather than a gap: the CLI's election-ID test reads
`internal/app/options.go` from disk with `go/parser`, which an overlay cannot
affect, so the verifier rsynced the repository to a scratch path, applied the edit
on disk and confirmed the kill there. That test now binds the CLI default to both
the operator source and `docs/reference/configuration.md` without importing
`internal/app`, which the AGENTS.md boundary rule forbids.

Gate outcomes: `go build ./...`, `go vet ./...`, `gofmt -l` all clean; pinned
`task lint` 0 issues; `-race` clean on registry and app; full suite green for
registry, app and cli. The pre-existing `staticcheck` ST1000 on generated
`pkg/addondefinition/inventory_generated.go` and the 0%-coverage
`internal/adapter/rbacgen/runtimecmd` package remain outstanding from T021/T022
and belong to T057's gate run.

Remaining US3 work is the controller chain: T037-T040, T042, T044-T046, then
T047 wiring, T048 RBAC and T049 gates. Runtime remains default-off and
`runtime.Execute` still has no production caller. Total checked: 38/59.

### US3 phase B1 — lifecycle fixtures, definition/binding reconcilers, drain (T037, T038, T042) — 2026-09-21

The first reconcilers for this feature. Written test-first: the lifecycle-matrix
fixture file was authored against absent reconcilers and watched fail to build,
then the reconcilers were implemented against it.

T037/T038 — `addondefinition_controller.go` and `addondefinitionbinding_controller.go`
(both new). Semantic validation, dedicated-SA UID and name uniqueness with manager
and built-in exclusions, invalid-snapshot removal, owner-aware delete by UID,
three field indexes backing every dependency lookup, and a deterministic barrier
seam that holds a reconcile at a chosen point with no sleeps. 16 of the 17
lifecycle-matrix rows have fixtures; the rows that remain belong to T039/T040
(publication fences, the AddonCheck-side UnknownAddonType condition) and T044
(evidence preservation and the ages-out row, whose status fields do not exist yet).

T042 — drain acknowledgement per contracts/leadership.md: enabled=false observed
through an uncached read, work cancelled and awaited, activeRuns=0, both Lease and
binding re-read uncached before publishing, and read failure or epoch mismatch
treated as unverifiable rather than drained.

Adversarial review rejected both on first pass and found THREE REAL BUGS, not
merely missing tests. Each was reproduced by the reviewer with an overlay probe
and then independently re-confirmed by exercise after the fix:

1. Deleting a binding never cancelled this session's in-flight work. Both deletion
   paths returned before touching the leadership session, so a run already
   executing under the deleted binding's dedicated identity ran to completion —
   against the matrix row "Binding disabled/deleted or SA replaced | ... Cancel
   active work". Both the hard-delete and finalizer-held shapes are now covered
   and are independently load-bearing: removing either cancellation fails exactly
   one arm.
2. An inventory-only built-in collision published a DISPATCHABLE snapshot while
   reporting Ready=False/BuiltinCollision. The registry only bars dispatch for
   built-ins the process actually registered, so a definition whose identity the
   shipped inventory claims was published with no barrier while its status claimed
   suppression. It was contained in production only because run.go registers every
   built-in unconditionally — the safety rested entirely on a registry/inventory
   agreement nothing asserted. The reconciler now fails closed before compiling,
   withdraws its own and every other claimant's snapshot, and the tests assert the
   OUTCOME the row names: a reported collision must resolve to no dispatchable
   candidate.
3. A disabled binding whose stored spec fails ValidateBinding was published as
   Drained=True/DrainAcknowledged beside Ready=False/InvalidDefinition, while
   contracts/leadership.md requires AuthorizationRevoked — so the operator claimed
   drained while its own independent CLI verifier returned exit 1, not drained.
   The verifier confirmed the fix by re-implementing the CLI's exit-0 conjunction
   independently of internal/cli and asserting operator and CLI now agree.

Also corrected: contested identities were compiled and published without a
binding, spending the bounded compile budget on a revision that cannot activate,
against "Activate only after valid binding and compilation".

Mutation verification: 28 mutations re-applied independently; all 9 originally
surviving ones are caught. Three of the verifier's own new mutations survived and
were closed by hand — the activeRuns schema clamp, the 401/Unauthorized arm of the
drain path's denied-read reporting (the Forbidden arm was proven but a 401 still
reported AuthorizationRevoked instead of AccessDenied), and the definition
reconciler's mandatory-OperatorBuild construction guard.

RBAC. The new markers changed the generated ClusterRole, and the delta is exactly
minimal: `addondefinitions` and `addondefinitionbindings` merged into the existing
fathom.skaphos.io get/list/watch rule, and their `/status` into the existing
status rule. No new rule block, no leases, no SubjectAccessReview, no grant write.
`config/rbac/role.yaml`, the Helm `manager-rules.yaml` distribution and the
`docs/reference/operator-rbac.md` justification rows are regenerated and updated
together, as the repository's lockstep doc guard requires.

The absences are this feature's security argument, so they are now asserted rather
than reviewed: `internal/controller/runtime_rbac_guard_test.go` fails if the
operator ever gains a write verb on either runtime kind (an operator that could
write a binding spec could enable a disabled binding or retarget it at another
identity — i.e. authorize itself), a cluster-wide Lease rule, an access-review
grant, or a ClusterRole/ClusterRoleBinding or bind/escalate grant. It is paired
with a positive test proving the two kinds ARE readable, so the absences cannot
be satisfied vacuously by granting nothing.

That work covers most of T048 (markers, namespaced election-Role Lease access,
role.yaml and Helm generation, and the proofs for binding-spec writes, grant
writes, cluster-wide Lease reads and SAR grants). T048 stays OPEN: its
"addon standing reads" clause is not yet asserted, and the Helm/values surface it
shares with T051 is untouched.

Gate outcomes: `go build ./...`, `go vet ./...`, `gofmt -l` clean; pinned
`task lint` 0 issues; full suite green across `./internal/...` and `./pkg/...`
including the envtest-backed controller package; `-race` green on
internal/controller. envtest assets are 1.37.0, matching ENVTEST_K8S_VERSION.

Not run: `task test-e2e`. Neither reconciler is reachable in a cluster yet —
SetupWithManager has no caller until T047 wires the manager — so an e2e run could
only re-prove unchanged built-in behaviour. AGENTS.md requires it for
internal/controller changes before the PR is ready, and it is recorded here as
owed rather than skipped. kind is on PATH; docker is aliased to podman in this
environment.

Runtime remains default-off and `runtime.Execute` still has no production caller.
Remaining US3: T039/T040 (publication fences and per-check CAS), T044-T046
(evidence preservation, Skipped coverage, HealthCheck mirror), then T047 wiring,
T048 completion and T049 gates. Total checked: 41/59.

### US3 phase B2 — publication fences, evidence preservation, Skipped coverage (T039, T040, T044, T045, T046) — 2026-09-21

This phase gives `runtime.Execute` its first production caller. `internal/controller/addoncheck_runtime.go`
(AddonCheckRuntimeRunner) resolves through the registry's dispatch gate, takes a
leadership admission slot, builds ONE execution.Budget for the whole run, fences
pre-run and post-run through an uncached control reader, and publishes under a
per-check serialized compare-and-swap. 63 test functions in its suite.

T039/T040. The pre-run fence captures ten facts: definition UID, generation and
revision; binding UID and SPEC generation; SA UID; check UID, generation and a
policy digest; and the leadership epoch. Both fences read only through the
control reader; the manager's cached client is used for exactly one thing, the
status compare-and-swap. The same Budget counters and deadline cover the fence
reads and the delegated evaluator traffic even though the identities are
separate. Publication precedence is modelled as an explicit RANK ORDER rather
than four branches, and is tested as an order: cases are built where two failures
apply simultaneously and the earlier must win, plus the one adjacent pair the
behavioural table cannot construct (invalid input vs execution failure) is pinned
directly over synthetic candidates. A status-only binding write does not
invalidate authority, because the fence compares metadata.generation and never
resourceVersion.

T044. A genuinely new evidence model on AddonCheckStatus: COMPLETED evidence (the
last run that finished, with its ORIGINAL observedAt, revision and context) is
separate from the LATEST ATTEMPT. A failed attempt updates the attempt fields and
never touches the completed evidence or its timestamp. Freshness is derived:
Current within two effective intervals plus the timeout, Stale beyond, Unavailable
when the inputs are no longer eligible. A completed all-Skipped run REPLACES
evidence and ADVANCES its observation, which is the one case that behaves unlike
an Error attempt. The change is additive; `crd-compat` reports OK.

T045/T046. Transition-only reporting with attribution, the five named transitions
(Pass->Skipped, Skipped->Skipped, mixed-Skipped, zero-enabled-check,
unchanged-verdict-with-changed-revision), and the HealthCheck readiness/freshness
mirror.

Adversarial review rejected two of the three engineers and found ONE REAL BUG.

The lost-transition hole. A verdict change reached history only if
createOrReuseHealthReport succeeded on the SAME in-process attempt that published
the evidence. The runtime path publishes evidence first and records the report
second, so a transient API error, a lost leadership or a restart lost the
transition PERMANENTLY: the next run's previous evidence already carries the new
verdict, the verdicts match, and no report is ever written. The built-in path is
immune because it is ordered report-first and carries an explicit backfill clause;
the runtime path had neither. It also left lastReportName naming a report whose
result contradicted the status, which the T046 mirror then republished. Fixed by
deciding the transition against what history actually holds. Confirmed by
exercise, not by reading the diff:

    STEP1 run with a failing HealthReport create: reportName="" storedReports=0
    STEP2 next run (SAME verdict): reportName="custom-addon-check-d43c..." storedReports=1
    STEP3 genuine no-change run: reportName="" storedReports=1
    STEP4 second genuine no-change run: reportName="" storedReports=1

so the transition backfills without turning every poll into a report, and the two
halves of the property are separable: reverting the backfill fails STEP2 while the
no-change test still passes.

Three findings were test gaps that mattered more than they looked. The freshness
ORDER (eligibility before age) was unproven: moving the age test ahead of the
eligibility switch left all 259 specs green while converting every lifecycle row
that names freshness=Unavailable for retained evidence into Stale — and
aged+ineligible is the STEADY STATE of a prolonged revocation, since evidence
stops being refreshed precisely while the binding is revoked. It is now pinned by
four rows that vary both axes, with a control row proving the rows differ only in
eligibility. The contract-verbatim message "no checks evaluated" was compared only
against its own constant, so mutating the constant left the package green; the
literal is now asserted directly and end-to-end. And `RuntimeRunClients.Control`
was typed `client.Reader`, which the manager's cached client satisfies — the
uncached property held only because nothing was wired yet. It is now the concrete
`impersonation.RuntimeControlReader`, so the T047 wiring cannot hand the fences a
cached client at compile time.

Mutation verification: 25 mutations re-applied independently, 23 caught. The two
survivors were closed by hand — the deterministic report-name key component (near
equivalent; its sibling mutation on observedAt was already caught, so the
uniqueness property is guarded) and the mis-wiring fail-closed guard in execute(),
whose mutation produces exactly the nil dereference the guard exists to prevent.

A previous self-report claimed "58 applied, 58 killed"; an independent harness
found 3 survivors among 19 reproduced. The figure is corrected here rather than
carried forward.

Regression evidence, checked rather than assumed. `addoncheck_controller.go` is
purely additive (228 inserted lines, zero removed). `clusterhealth_controller.go`
is untouched, and its ClusterHealth contract is re-verified by a focused spec that
reconciles through a HealthReport-blind client and asserts zero report reads while
the result still mirrors HealthCheck.status. T046's named regression — a RETAINED
Pass must not present as a fresh success — is live rather than vacuous: a mutation
dropping the mirror's freshness reason kills it.

Gate outcomes: `go build ./...`, `go vet ./...`, `gofmt -l` clean; pinned
`task lint` 0 issues; full suite green across `./api/...`, `./internal/...`,
`./pkg/...` and `./cmd/...`; `-race` green on internal/controller;
`check-crd-compat: OK`.

Not run: `task test-e2e`. The runtime runner and the transition backfill still have
no production caller — T047 owns the manager wiring — so there is nothing new for a
cluster to exercise yet. The obligation transfers to whoever lands T047 and is
recorded here as owed, not skipped.

Runtime remains default-off. Remaining US3: T047 wiring, T048 completion, T049
gates. Total checked: 46/59.

### US3 — manager wiring (T047) — 2026-09-21

The keystone. Everything built in US2 and US3 phases A and B was unreachable in a
cluster because nothing was wired; T047 connects it, default-off.

`internal/app/runtime_wiring.go` (new) owns the process-wide runtime singletons —
ONE leadership session, ONE execution.Scheduler pool — and three leader-elected
runnables: the session, a dispatch gate, and a worker pool. The gate waits on
Ready() (elected AND caches synced AND the >=30s monotonic takeover grace),
adopts the live Lease through the manager's uncached APIReader, and is the ONLY
caller of `registry.OpenRuntimeDispatch(holderIdentity)`, closing it on every exit
path via a deferred CloseRuntimeDispatch. `Run` builds the manager's
leader-election resource lock with the session's own holder identity in the
configured namespace and election ID — without that the elected holder and the
runtime decider would be two identities, no epoch would ever be adopted, and
runtime would stay permanently closed. The AddonCheck reconciler gained
Runtime + RuntimeQueue: a runtime-backed identity is enqueued onto the shared
pool instead of running inline, and the pool handler is the production caller of
AddonCheckRuntimeRunner.Run plus the T044/T045 transition and backfill path.

This closes the gap phase A deliberately left open and recorded as T047's job.
The phase A verifier's words were: "one enforcement point, one decider, zero code
connecting them." There is now code connecting them, and it is proven by identity
rather than by type (see below).

Two real defects were found and fixed along the way.

First, an identity-confusion bug found by the implementing engineer's own test:
`registry.Lookup` delegates to `Resolve`, so once dispatch is admitted the
BUILT-IN path would have picked up runtime adapters and run a compiled definition
under the per-addon ServiceAccount convention instead of its binding's dedicated
identity. The built-in path now resolves built-ins only, fail-closed.

Second, a production defect found while making the startup diagnostic observable:
`Run` derived its logger from the global `ctrl.Log`, and controller-runtime's
global delegating logger can only ever be FULFILLED ONCE per process. If anything
installed a logger before Run — another Run call, a library, an embedding binary —
every startup diagnostic Run emits would be silently swallowed, including
"runtime addon loading unavailable", which is the only place an administrator
learns why runtime loading is inactive. contracts/leadership.md requires that
diagnostic at startup, explicitly "without relying on a leader-gated controller
that will never start". Run now derives setupLog from the zap logger it just
built. Behaviour in the shipped binary is unchanged; the guarantee is now real.

Adversarial review ran three lenses. Default-off reviewed SOUND and was
re-proven independently: with runtimeLoading.enabled=false the built-in Setupper
list is element-wise identical to DefaultControllers, Runtime and RuntimeQueue are
nil, zero runnables are registered, no AddonDefinition index exists, and the
leader-election lock is not installed. Two further default-off mutations were
applied by the verifier and both were killed.

Integration and gate-ordering both returned needs-rework with the same finding,
and it is the most important review result of this feature: THE WIRING WAS
ASSERTED BY GO TYPE, NEVER BY IDENTITY. Eleven mutations survived in which every
wire could be crossed while the whole suite stayed green — the gate opening a
private registry (so the operator logs "runtime dispatch admitted" while Resolve
still returns a closed-admission barrier, exactly the silent disagreement T047
exists to close); the gate driven by a second session nobody starts; the pool
draining a foreign scheduler, so work is enqueued where no worker looks; the pool
handler a no-op, so no evaluation is ever reached; the Lease read through the
manager's CACHED client, which starts a cluster-wide Lease informer that
contracts/leadership.md forbids verbatim and that the operator holds no RBAC for;
the lock built with a divergent identity, making runtime permanently inert; `Run`
passing nil, making runtime loading dead code in the shipped binary; the
cache-sync gate replaced by a constant-true closure, the exact no-op the code's
own comment says must be impossible; and the startup diagnostic deleted.

For a wiring task, "the right things are connected to each other" IS the
deliverable, and that was the one thing with no coverage. It is now proven in the
strong form, against the objects `attach` actually registered rather than
hand-built copies: field-by-field identity pinning (gate.session == pool.session
== wiring.session, gate.registry == the shared registry that is also the
reconciler's Adapters and the runner's Registry, gate.reader == mgr.GetAPIReader()
with an explicit != mgr.GetClient() check, pool.scheduler == wiring.scheduler ==
addonCheck.RuntimeQueue, and pool.handle's function pointer == RunRuntimeWork),
plus an end-to-end test on a real started envtest manager with a real Lease that
observes admission through the shared registry's Resolve and execution through the
reconciler's own RuntimeQueue, then re-barring after shutdown.

Mutation verification: all eleven reviewed mutations are caught, confirmed by an
independent harness that re-applied every one rather than trusting the report. The
verifier then invented twelve of its own, of which five survived — all on seams
ADJACENT to the ones the reviewers named, which is a fair characterisation of a
rework that pinned what it was asked to pin and stopped. Four were then closed
here and mutation-proven: a second never-started session planted in the runner's
Session and in the binding reconciler's Leadership (the same defect shape as the
gate's), the runner's ProbeImage silently dropped, and the leader-election lock
naming `<election-id>-election` — which slipped past a `strings.Contains` check on
the lock description and is now an exact comparison against the session's LeaseRef.

One residual is accepted rather than closed: swapping the two adjacent string
arguments `w.opts.Namespace` and `w.managerServiceAccount` in the single
`impersonation.NewRuntimeFactory` call compiles and passes the factory's non-empty
check. It is recorded rather than fixed because the two values come from
structurally different sources (configuration versus the manager's own token), it
is one call site, and a swap fails closed loudly at runtime — authority resolution
cannot find a binding in a namespace named after a ServiceAccount. Closing it
properly means giving NewRuntimeFactory a struct argument in another package,
which is disproportionate to the risk; it is noted here so a future change to that
signature can take it.

Gate outcomes: `go build ./...`, `go vet ./...`, `gofmt -l` clean; pinned
`task lint` 0 issues; full suite green across `./api/...`, `./internal/...`,
`./pkg/...` and `./cmd/...`; `-race` green on internal/app and internal/controller;
`check-crd-compat: OK`; the runtime RBAC guard still holds (no new markers).

Two things carried forward, both pre-existing or environmental, neither introduced
here. `go test ./internal/app/ -count=3` fails on the untouched
TestRun_HappyPath_DefaultControllers because of controller-runtime's process-global
controller-name registry. And `task test-e2e` was NOT run: docker on this machine
is aliased to an unavailable podman, and the user has said they will run e2e on a
different machine. AGENTS.md requires it for internal/app/run.go and
internal/controller changes, so the obligation now covers everything from US2
onward and is the first thing to run on that machine.

With T047 landed the feature is reachable in a cluster for the first time. Runtime
remains default-off. Total checked: 47/59.

### Linux/Docker handoff and US3 completion (T048–T049) — 2026-09-22

The requirements checklist remains 16/16 checked. The prerequisite helper selected
the old RFC feature until invoked with `SPECIFY_FEATURE_DIRECTORY` pointing to
`specs/012-addon-definition-runtime`; subsequent feature context now selects 012.
Docker Engine 29.8.0 is available on this Linux/amd64 host, alongside Go 1.27.1,
kind, Helm, helmfile and kubectl. A fresh Docker-backed kind cluster uses the
pinned Kubernetes 1.37.0 node image. The full default-off e2e baseline is in
progress; its outcome is not claimed here and it cannot qualify runtime loading.

T048: the definition/binding read/watch/status markers and generated grants were
already present. Strengthened `internal/controller/runtime_rbac_guard_test.go`
to reject wildcard escalation and access-review variants, verify the existing
namespaced election Role and its RoleBinding, and compare Helm manager rules to
the generated ClusterRole. No production permission was added. The focused
`go test ./internal/controller -run 'Test(RuntimeDefinitions|RuntimeLeaseAccess|HelmManagerRules)' -count=1`
passed. Helm rendering also confirmed namespaced Lease get and a matching binding
to the operator ServiceAccount.

T049: with
`KUBEBUILDER_ASSETS=/home/sstratton/.local/share/kubebuilder-envtest/k8s/1.37.0-linux-amd64`,
`go test -race ./internal/controller -count=1` passed (47.683s) and
`go test -race ./internal/app -count=1` passed (13.685s), outside the sandbox that
prevents envtest opening local sockets. The earlier 4.772s sandbox app result is
not full integration evidence: app TestMain can silently skip envtest-backed
tests. A verbose asset-backed rerun explicitly confirmed RUN/PASS, with no skips,
for `TestRuntimeWiringAttachesEverySeam` and
`TestAttachedGateAndPoolDriveDispatchAndExecution`.

The controller race run covers the named lifecycle tests in
`addondefinition_lifecycle_test.go`: missing, added, edited, legacy-invalid,
deleted/deleting and recreated definitions; binding revocation and recovery;
denied control-plane reads; observed active-run revocation; unsynchronized cache;
edit-before-publication; duplicate ownership and builtin collisions; compilation
budget failure and dedicated identity exclusions. Publication/evidence tests in
`addoncheck_runtime_test.go` additionally cover final fences, authority/supersession
precedence, retained observation times, freshness and Skipped report transitions.
These are component/envtest results, not real-cluster qualification. Revocation
remains observation-based: a read already authorized cannot be undone, and a
suspended old process is not excluded by the takeover grace alone.

Pinned `task helm:sync` (including manifests) and `task docs:api-ref` passed with
no generated distribution/API reference drift. `git diff --check` passed.
`graphify update .` ran; its generated artifacts are not reviewed as source.
US3 is complete at 49/59 tasks. US4 and release qualification remain open.

Live GitHub verification on 2026-09-22 confirms
[#256](https://github.com/skaphos/fathom/issues/256) is open with no comments;
`pkg/adapter/version.go` still declares ContractVersion 1.0.0. T054 remains open:
the separate decision, older-adapter regression and migration/rejection evidence
have not been supplied, so runtime release remains blocked.

### US4 packaging, operations and Docker baseline — 2026-09-22

The user approved the test-layer clarification now recorded in `spec.md` and
`contracts/decision-supplement.md`: deterministic component tests prove exact
numeric boundaries, recoverable injected panics and collision states normal
admission prevents; Docker/kind proves real permissions, delegated execution,
lifecycle, drain, rollback and hostile-input isolation. No runtime limit or
authority guarantee is relaxed and no production fault-injection hook is added.

T051: Helm `runtimeLoading.enabled` defaults to false and is schema-validated as
a boolean. True emits `--runtime-loading-enabled`; false emits no runtime flag,
preserving existing environment/config-file opt-in. `leaderElect=false` remains
renderable so built-ins start while runtime activation is refused. The new
`scripts/helm_runtime_loading_test.go` checks default/off, explicit opt-in,
custom namespace and Lease, metrics-off, disabled election, config/env
precedence, downward-API namespace wiring and rejection of nonboolean input.
`go test ./scripts -run TestHelmRuntimeLoading -count=1` passed. Helm lint passed
with defaults and runtime enabled; custom namespace/Lease/metrics-off rendering
also passed.

T052–T053: `docs/guides/addon-definitions.md`, README, RELEASE and the handwritten
`docs/reference/operator-rbac.md` document reviewed two-stage UID installation,
an actual AddonCheck, exact reader/impersonation grants, declared order/caps,
target-release collision preflight, Skipped evidence and independently observed
Lease drain. Rollback explicitly disables bindings and verifies drain before
revoking grants and disabling the loader; older binaries may leave freshness
fields frozen. #256 and #149 remain separate. CLI flags were checked against
the implementation; local links, SPDX and `git diff --check` passed. The guide
does not claim the pending runtime installation/rollback qualification passed.

The Linux handoff's full default-off `go -C tools tool task test-e2e` passed:
95/95 Ginkgo specs, zero failures/pending/skipped, 470.031s for Ginkgo and
472.139s for the test package. It built the operator/probe/node-agent images,
installed the entire addon stack on Docker/kind Kubernetes 1.37.0, and removed
the cluster. Log: `/tmp/fathom-feature012-e2e-baseline.log` (local, not durable).
This closes the owed existing-behavior baseline, not T050/T055 runtime acceptance.
The enlarged suite now has a bounded 30-minute Go package timeout because its
serial election/restart scenarios add to an existing eight-minute baseline;
individual behavioral assertion deadlines remain unchanged.

Initial pinned CI passed compatibility, lint and unit/envtest suites but failed
staticcheck ST1000 on the generated inventory's package comment. The separate
coverage gate also found `internal/adapter/rbacgen/runtimecmd` at 0%. Fixed the
generator source and regenerated through `task gen:runtime-definitions`; added
a command `run(root)` seam tested for all-nine-kind generation and both output
write failures. Fresh package tests passed with runtimecmd coverage 62.5%,
rbacgen 92.3% and addondefinition 85.7%; pinned staticcheck then passed. No
coverage threshold or exemption changed. Full CI is being rerun.

Additional completed checks: pinned `task vuln build` passed (zero reachable
vulnerabilities; one imported but uncalled advisory reported); `go test -race
./internal/adapter/registry/... ./internal/adapter/runtime/...` passed; REUSE lint
passed on 754 files. Final results after all edits are recorded separately below.
Current task count: 52/59. Runtime cluster qualification and release remain open.

### First opt-in Docker trial — integration finding

The focused core-stack runtime suite produced attributed Pass evidence and a
transition report, then passed same-UID granted retargeting and out-of-scope
rejection. It failed the ServiceAccount recreation scenario: the execution
runner correctly reported `BindingMismatch`, but the binding's `Ready=True /
BindingAuthorized` condition remained unchanged after the reader UID changed.
The definition and binding controllers watch each other but neither watches
ServiceAccounts. Their existing identity indexes were used during reconciliation,
not to trigger it. T060 records the missing dependency watch and regression.
This is a real-cluster failure, not a passed qualification gate. The test's
cleanup restores the original manager arguments and removes test authority.
Log: `/tmp/fathom-feature012-runtime-e2e.log` (local).

Before this finding, the corrected full pinned `task ci` completed successfully,
including CRD compatibility, lint, unit/envtest, staticcheck, vulnerability scan
and builds. The separate per-package coverage gate passed. Those results precede
the T060 fix; its source changes require renewed verification.

### Dependency-watch fix and renewed verification

T060 adds indexed ServiceAccount watches to both definition controllers and a
peer-binding watch for identity sharing. Peer status-only updates are filtered;
old/new authority references are both mapped. Authorized bindings also recheck
every 60 seconds so a transient mapper failure cannot strand readiness. Existing
ServiceAccount read/watch permissions suffice; no RBAC grant was added.

The new started-manager envtest failed before the fix with `Ready=True` after
ServiceAccount deletion/recreation. After the fix it proves binding and definition
invalidation, replacement binding recovery, shared-reader conflict and recovery
after deleting the peer binding. Immutable references are reauthorized by
deleting/recreating the binding, not by changing its UID reference in place.
The asset-backed controller race suite passed in 44.174s. The new pinned `task ci`
run and separate package coverage gate passed after the production fix. Log:
`/tmp/fathom-feature012-ci-watchfix.log` (local). Docker rerun remains pending.

The durable [qualification map](qualification.md) records the requirement,
numeric and lifecycle audit plus focused security review. Open evidence gaps
remain explicit; test names alone do not complete T050/T055/T056. The separate
#256 compatibility decision remains a release dependency.

### Second opt-in Docker trial — history finding

The rebuilt watch-fix operator passed initial delegated execution but the next
same-UID retarget produced two Pass reports instead of one. The captured reports
combined newer check observations with older completion timestamps/definition
attribution. T061 tracks the transition/publication regression; the live suite
remains failed, and later ordered scenarios did not run. Log:
`/tmp/fathom-feature012-runtime-e2e-watchfix.log` (local). No history assertion was
weakened to accept the duplicate.

Pinned `verify-generated` reran all generators and changed none of 145 hashed
generated artifacts. Its final Git-diff gate returned nonzero solely because the
intentional generated inventory package-comment fix is still uncommitted. This
is not recorded as a passing gate. REUSE lint passed all 756 files at this point.

T061's regression forces the manager cache to lag behind publication. The fix
returns the exact API-accepted status and preceding evidence from the runner to
history creation, avoiding a cached reread. Report keys use the durable history
predecessor and verdict transition, so a failed report-pointer status write can
reuse the original immutable report even after a same-verdict generation change.
The second status write retains resource-version conflict protection. Repeated
pointer failures across opposite verdict flaps can coalesce intermediate history;
the implementation does not claim a durable ordering that was never recorded.
Stale-cache, report-pointer conflict and repeated-flap regressions pass, and the
full controller envtest/race run passed in 43.979s (261 Ginkgo specs).

Additional component qualification passed: exact 262,144-byte stored/defaulted
definition admission followed by compiler validation; representative 253/254-byte
resource and discovery grammars; non-ASCII identifiers; selector values at the
effective Kubernetes 63/64-character boundary. The maximum-length typed binding
fixture measures 4,115 bytes even with maximally escaped UIDs; false booleans are
omitted, so true values maximize its serialization. The 16-KiB bound cannot be
reached by the closed typed schema. Both affected package suites passed, including
asset-backed API envtest. See [qualification.md](qualification.md) for exact names
and unreachable-boundary explanations.

The first CI attempt after T061 stopped on lint while the new API fixture was
still being authored (unused test helpers and formatting). It is not a passing
final CI result. A compiled trial of the existing runtime scenarios is running
with only that unfinished fixture excluded through a Go overlay; the full final
suite must include it. No release coverage is credited to an excluded fixture.

The third opt-in trial has now passed the named ServiceAccount recreation,
shared-reader conflict/recovery and definition recreation scenarios, closing
T060's live regression. It also passed unchanged-verdict revision history,
Pass-to-Skipped and repeated-Skipped history checks, closing T061's live regression.
T050/T055 remain open until the complete final suite (including the new API
fixture) and required lifecycle checks finish; these individual results do not
declare that gate passed. Trial log:
`/tmp/fathom-feature012-runtime-e2e-historyfix.log` (local).

That third trial completed: 9/9 selected scenarios passed in 646.053s; 95
unselected specs were skipped by the focus filter. It includes real revocation,
identity recreation/borrowing, invalid stored revisions, history, drain, grant
revocation, rollback, election-disabled refusal and re-enable. It predates the
new API fixture and the strengthened startup-collision/metrics-off read cases.

The new API fixture reaches a test-host HTTPS server through an APIService,
Service and EndpointSlice with a trusted service-DNS certificate. Its first
focused trial proved delegated discovery 403 with no manager fallback, actual
read timeouts, compiled-peer progress, and stale original observation time through
HealthCheck and ClusterHealth. It failed a too-strict lasting attempt-reason
assertion after an observed in-flight revocation: the binding correctly reported
AuthorizationRevoked and Drained, evidence/report history stayed unchanged, and a
later no-snapshot attempt reported UnknownAddonType. A failed-attempt status CAS
can also legitimately lose to a concurrent status write. The live assertions are
being corrected to check durable binding revocation/drain and unchanged evidence,
while component tests retain exact single-run failure-precedence assertions. No
production change is justified by this trial. Log:
`/tmp/fathom-feature012-runtime-api-fixture.log` (local).

Pinned CI then passed against the frozen controller and boundary-test sources:
`/tmp/fathom-feature012-ci-frozen.log`. Two intervening CI attempts stopped on
test-fixture lint or an intermediate unused variable while that fixture was
being edited; those are superseded by this passing run. Final test-only assertion
edits still require focused lint/compile and the full real-cluster suite.

### Prior-binary downgrade qualification remains open

Read-only inspection of tag `v0.5.1` found that its `AddonCheckStatus` lacks the
new completed-evidence, freshness and latest-attempt fields, while its controller
uses a full status update. An older binary can therefore remove those fields;
it does not merely leave them frozen. Transition-only HealthReports cannot
reconstruct a newer same-verdict current observation. The operations guide and
RELEASE instructions now require exporting full AddonCheck objects/status outside
the cluster along with reports and definition/binding data before downgrade.

The current kind scenario verifies this build's default-off rollback and
re-enable. It does not run v0.5.1 or prove a prior-binary downgrade/restore.
T055 remains open for that release qualification, alongside the separate T054/#256
compatibility decision. No claim that all earlier gates or runtime release are
complete is supported by the flag-toggle scenario.

### Final component and generated-artifact checks

The final API-fixture trial passed all three selected scenarios in 442.597s
(444.649s package time), with 102 other specs excluded by the focus filter. Command:
`E2E_ADDONS=core go test ./test/e2e/ -timeout=20m -v -ginkgo.v
-ginkgo.focus='runtime AddonDefinition on a real cluster (keeps definitions|uses only|fences delegated)'`.
Log: `/tmp/fathom-feature012-runtime-api-fixture-final.log`. Final pinned lint
passed with zero issues after the test assertions settled; coverage passed without
changing thresholds. The complete all-addon `go -C tools tool task test-e2e` is
running separately and is not inferred from those focused results.

The working-tree `verify-generated` failure described above was resolved as a
verification limitation without staging or committing the working branch. A
separate temporary repository snapshot included every tracked and nonignored new
file, normalized only `config/manager/kustomization.yaml` back to the branch's
committed image configuration (the live e2e deploy task changes it), and committed
that snapshot with the configured author and a DCO trailer. Running the unmodified
`go -C tools tool task verify-generated` there passed, and `git status --short`
was empty afterward. This proves the proposed artifacts reproduce from the
proposed generator inputs; it is not a claim that the uncommitted working-tree
drift gate itself passed. No commit was created on the working branch. Log:
`/tmp/fathom-feature012-verify-generated-snapshot.log`.

Together with the passing frozen-source CI (including CRD compatibility),
controller/app/runtime/registry race suites, final lint and coverage results above,
this completes T057's checks. `reuse lint` passed all 753 files in the final
repository snapshot. `graphify update .` refreshed the code graph after source
changes: 7,173 nodes, 19,481 edges and 441 communities. It used AST extraction and
does not claim semantic re-extraction of documentation. AGENTS.md and
docs/architecture.md describe the final package boundaries; the pinned generators
reproduced samples, distributions and API reference documentation, completing
T058. Logs: `/tmp/fathom-feature012-reuse-final.log` and
`/tmp/fathom-feature012-graphify-final.log`.

### Full Docker trial: stored collision during startup

The full 105-spec suite passed the first nine runtime scenarios but failed the
strengthened same-binary startup-collision assertion in
`test/e2e/addondefinition_test.go:939`. A stored `coredns` definition and its
binding had already blocked the compiled adapter. After the manager restart at
16:16:04 UTC, the compiled check advanced `lastRunTime` to 16:16:33 UTC before
returning to Ready=False/BuiltinCollision. Eventual rejection does not satisfy
the startup contract: both colliding candidates must remain blocked. The
assertion is retained and T062 tracks the production fix and regression. The
previous passing component checks qualify the pre-T062 source only; affected
checks and the complete Docker suite must run again after that fix. Log:
`/tmp/fathom-feature012-e2e-full-final.log`.

That complete trial finished with **104 passed, 1 failed, 0 skipped** in
1,228.631s (1,230.706s package time). The startup collision was its only failing
spec. The final ordered runtime spec stopped at that failure, so its later
strengthened metrics-off assertions were not reached in this trial. The task
removed its temporary cluster afterward. This is a failed qualification run,
not a passing full-suite result.

### Startup correction and local-mode qualification

T062 now reserves registered built-in identities before manager startup and reads
only those exact AddonDefinition names through bounded uncached GETs. A found
identity remains provisional until normal reconciliation and direct observation
agree on UID and generation; an unknown read holds only that identity. NotFound
releases nonconflicting identities immediately. A bounded retry loop recovers
failed reads and deletions. The registry remembers a completed newer revision
until the next direct observation confirms it, avoiding a permanent hold when
reconciliation precedes that observation. Default-off wiring adds no inventory
work. Ordinary post-read mutations retain the documented non-atomic limitation.

The manager/envtest regression for a stored collision before first reconciliation
passed. Registry tests cover unknown reads, stale UIDs/generations, a newer
completed revision preceding its direct read, and no resurrection of released
claims; controller tests cover terminal invalid/missing-binding recovery. The
new-revision recovery test was observed failing before its correction and passing
afterward. The earlier startup regression's failure under pre-T062 wiring was
inferred from the source, not executed as an old-code overlay. Independent review
found the recovery race, which was corrected and re-reviewed. Final affected
race/envtest suites passed: registry 1.066s, app 13.102s, controller 46.055s. Log:
`/tmp/fathom-feature012-startup-collision-race-postreview.log`.

An initial CI/e2e restart was intentionally interrupted for that review correction
and provides no completed result. Pinned CI subsequently passed against T062
(`/tmp/fathom-feature012-ci-startupfix-final.log`). Its companion full Docker run
built the fixed image but was stopped before spec execution when the separate
host-local trial identified T063: despite `--namespace=fathom-system`, ordinary
manager construction lacked `LeaderElectionNamespace` and could not elect outside
a pod. Runtime correctly logged AuthorizationUnavailable for the missing
projected identity, but built-ins could not start. The explicit namespace must
reach the ordinary election options while empty namespace retains existing
autodetection. The final full suite must include this correction too.

T063's explicit-namespace manager-options regression failed before the mapping
was added and passed afterward; the empty-namespace case retained its previous
fallback. Full app race/envtest passed in 13.021s. Independent review confirmed
alignment with the runtime's existing explicitly namespaced Lease lock. Both
T062 and T063 are frozen pending their live acceptance, with no new RBAC.

Final pinned CI passed against all four integration corrections:
`/tmp/fathom-feature012-ci-qualified-final.log`. This includes lint, unit/envtest,
vet, staticcheck, vulnerability analysis, CRD compatibility and builds. The
coverage gate passed unchanged
(`/tmp/fathom-feature012-coverage-qualified-final.log`). The unmodified pinned
`verify-generated` gate passed again in the isolated committed snapshot with
the final source inputs; its Git status remained empty
(`/tmp/fathom-feature012-verify-generated-qualified-final.log`). REUSE lint
passed, and the final AST graph update produced 7,193 nodes, 19,554 edges and
441 communities. Logs: `/tmp/fathom-feature012-reuse-qualified-final.log` and
`/tmp/fathom-feature012-graphify-qualified-final.log`. These complete T057/T058
again after the additional fixes. Full Docker acceptance is tracked separately
in `/tmp/fathom-feature012-e2e-qualified-final.log` and remains running.

### Actual prior-binary operations qualification

The isolated `fathom-downgrade-012` Docker/kind trial completed using the exact
`v0.5.1` source and its unmodified Dockerfile (pinned Go 1.26.5 and distroless
base), current CRDs, and the core addon tier. The full environment, image IDs,
commands, observation times and scope limits are retained in
[operations-qualification.md](operations-qualification.md). Raw local scripts,
logs and JSON backups remain under `/tmp/fathom-downgrade-012/`; they are not
committed artifacts.

The trial verified reviewed UID installation, delegated Pass, a same-verdict
revision with unchanged report history, matching-generation drain and independent
CLI verification, grant revocation and full status export. The older binary
actually removed new evidence fields while its compiled CoreDNS checks continued.
After all old pods stopped, the current default-off binary accepted a guarded
status-subresource restore (UID, generation and resourceVersion tests). Readback
preserved exact original evidence, time, revision and authority. Reviewed
re-enable produced a fresh Pass without an artificial same-verdict report.

A separate cross-version step stored a valid CoreDNS definition under v0.5.1,
observed target CLI collision exit 1, then stopped the old pod and started the
T062-fixed image. The target blocked the compiled candidate before execution:
Ready=False/BuiltinCollision and unchanged lastRunTime through admission and the
observation window. Explicit collision deletion restored Pass. Both binaries
already include compiled CoreDNS; this proves cross-version stored-name
collision handling, while new-builtin registration itself remains component
evidence. It is not a released-chart upgrade or the separate #256 ratio decision.

Finally, the T063-fixed host binary ran with the isolated kubeconfig, explicit
operator namespace, leader election and runtime opt-in, without a projected
ServiceAccount token. It logged AuthorizationUnavailable, acquired the configured
namespaced Lease, advanced the compiled CoreDNS peer, and left runtime evidence
unchanged. This closes T063's live regression. The host process stopped and the
isolated deployment recovered before the temporary cluster was deleted. Only the
main full-suite cluster remains. T055's prior-binary gap is now closed; its
combined full-suite gate still awaits the ongoing final run. #256 is unaffected.

### Final e2e attempt and isolated rerun

The final attempt recorded in `/tmp/fathom-feature012-e2e-qualified-final.log`
passed the startup-collision, drain, grant-revocation, old-leader
drain-acknowledgement rejection,
no-election and new metrics-off epoch/drain scenarios before unrelated concurrent
work changed the shared kubeconfig current context from `kind-fathom-e2e` to
`kind-kind`. Subsequent `kubectl` calls therefore reached an unrelated cluster
without the AddonCheck CRDs, producing a cascading 29 failures, 11 passes and 65
skips in the 40/105-spec run (894.737s, 896.790s package time). This is kubeconfig
contamination, not a production failure; the original `fathom-e2e` task later
removed its temporary cluster. Read-only inspection of the other context found no
Fathom test namespaces or grants and made no changes; the failed test calls were
limited to the contaminated context.

The test-only portability correction makes the host-gateway helper read
`E2E_KIND_CLUSTER` (default `fathom-e2e`), and `Taskfile.yml` passes the resolved
cluster name to the e2e process. `go test ./test/e2e -run '^$'` compiled cleanly;
pinned lint passed with zero issues (`/tmp/fathom-feature012-lint-isolated-final.log`).
The same production tree as CI is being rerun with the isolated cluster and a
name-only copy of the pinned Kind fixture:

```sh
KUBECONFIG=/tmp/fathom-feature012-final.kubeconfig \
E2E_KIND_CLUSTER=fathom-feature012-final \
go -C tools tool task test-e2e \
  E2E_KIND_CLUSTER=fathom-feature012-final \
  E2E_KIND_CONFIG=/tmp/fathom-feature012-final-kind.yaml \
  > /tmp/fathom-feature012-e2e-isolated-final.log 2>&1
```

This isolated rerun was then completed successfully. The final AST graph audit recorded
7,196 nodes, 19,560 edges and 422 communities
(`/tmp/fathom-feature012-graphify-isolated-final.log`). Final SpecKit consistency
analysis maps all 21 requirements and 63 tasks with no remaining live gaps; the
baseline wording correction is included. Preserve the
failed contaminated attempt above as historical evidence.

### Isolated final qualification result

The isolated pinned command completed successfully from the same production tree
as CI. It ran 105/105 specs in 1422.543s (Go package time 1424.597s): **105
passed, 0 failed, 0 pending, 0 skipped**. All addons were included and all ten
runtime scenarios passed, including startup collision/restart and metrics-off
actual Pass/403, Pass preservation, recovery and no duplicate report. The
`fathom-feature012-final` cluster was deleted by the task. This supersedes the
earlier 104-pass startup-collision failure and the kubeconfig-contaminated
40/105 attempt; those remain historical records above.

The final one-to-one audit maps 16/16 functional requirements, 16/16 numeric
rows, 17/17 lifecycle rows and 5/5 success criteria across component and live
evidence. The runtime portions pass; FR-014 and SC-005 remain conditional on the
separate #256 disposition, which remains open and still blocks release. T050,
T055, T056 and T062 are complete;
T061 and T063 are complete from the earlier corrected regressions. Only T054
(#256) and T059 (PR/release preparation) remain open. The final cleanup REUSE
lint passed with 754/754 files and zero issues
(`/tmp/fathom-feature012-reuse-isolated-final.log`); intended working-tree
changes remain uncommitted.

| Success criterion | Evidence mapping | Result |
| --- | --- | --- |
| SC-001 render/install | T020–T023, T050, T055; qualification map | Passed |
| SC-002 delegated authority | T024–T029, T050; qualification map | Passed |
| SC-003 lifecycle/history | T036–T049, T050, T060–T062; qualification map | Passed |
| SC-004 bounds/panics/hostile input | T026–T035, T050, T056; qualification and operations maps | Passed |
| SC-005 live acceptance/rollback | T050, T055; qualification and operations maps | Runtime evidence passed; #256/T054 remains open |

### Delivery decision and separate #256 compatibility proof

On 2026-09-22 the approved publication shape became one feature PR organized
around four review sections/milestones, plus a separate #256 compatibility PR.
The feature result above remains the pre-compatibility 1.0-source 105/105
qualification; the combined feature-plus-compatibility 1.1 tree has not been
live-qualified. The separate compatibility work is [PR #351](https://github.com/skaphos/fathom/pull/351),
based on `main` at signed commit `a7e3a3e927f01de614e97677cb3e1d62aa86e932`.
T054 remains open until that PR is merged and integration is verified; T059
remains open because the feature PR has not yet been published.

The isolated #256 proof in `/tmp/fathom-ratio-contract-256` used repository
`isolated-fix/256-ratio-contract` at `3e7eafd`, ContractVersion 1.1. It rejects
older 1.0 policy on key presence before Run even when 1.0 is advertised or the
policy is disabled, while ordinary 1.0 private policies continue to work. The
audit rename/rebuild migration was exercised. A scoped independent review found
no actionable issue. The final older-controller overlay against 1.1 failed the
expected 1/2 compatibility checks in 6.650s; the fixed compatibility path passed
in 6.821s. Pinned CI, coverage, generated-output checks and REUSE passed, and the
separate all-addon 95/95 Kind run passed in 477.256s (479.346s package time,
zero failures/skips); its cluster was removed.

The source patch applies cleanly to the feature branch in a dry run but has not
been applied here. The compatibility proof is separate evidence, not a merged or
resolved #256 decision; see [PR #351](https://github.com/skaphos/fathom/pull/351).
The feature's 105/105 result,
operations record and component/live test split remain the current feature
evidence.

### Runtime policy validation review — 2026-09-22

Qualification commit `04f88ca` and signed merge commit `1cf52bb` (bringing in
compatibility commit `a7e3a3e` from [#351](https://github.com/skaphos/fathom/pull/351))
were recorded for the feature review. Before the current policy correction,
combined CI, coverage, generated-output checks, REUSE (754 files) and race tests
passed. A full Kind attempt was interrupted during addon setup, before any spec
ran, after an independent review found the runtime-policy validation bypass. This
is neither a failed test nor a passed full suite; the owned
`fathom-feature012-contract11` cluster was removed. Raw logs remain under
`/tmp/fathom-feature012-contract11*.log`.

The reproducer admitted policy 150, but runtime bypassed its validation gate and
the aggregate ignored the parse error. The correction is underway: resolve and
fresh-fence the policy, invoke shared validation before execution, publish
`Accepted=False/InvalidPolicy`, preserve prior evidence and recover with a valid
policy. Component and live verification are still required; no verification
claim is made. T054 and T059 remain open, and this finding is tracked as T064.

### ContractVersion 1.1 integration status — 2026-09-22 (historical in-progress snapshot)

Signed T064 fix `e08f9af` is implemented on the local feature branch. Final CI
passed (`/tmp/fathom-feature012-contract11-final-ci.log`), as did coverage,
generated-output verification and REUSE for 754 files
(`/tmp/fathom-feature012-contract11-final-{coverage,generated,reuse}.log`); the
AST graph update passed with 7,216 nodes, 19,624 edges and 447 communities.
The full controller suite passed in 39.487s. The first controller race attempt
had one DNSCheck `lastRunTime` timing failure at line 588, with no race report;
the unchanged-retry rerun passed in 45.453s. Exact policy evidence is in
`/tmp/fathom-feature012-runtime-policy-validation.md`.

The dedicated ContractVersion 1.1 Kind run is **RUNNING** with kubeconfig
`/tmp/fathom-feature012-contract11.kubeconfig`, cluster/config
`fathom-feature012-contract11`, and log
`/tmp/fathom-feature012-contract11-final-e2e.log`. Do not treat it as passed
yet. The local branch contains the #351 compatibility merge, but upstream #351
is still open and not draft. T054's decision/regression/migration verification
may complete after the combined run passes; the upstream merge remains the
release gate. T059 remains open because no feature PR has been published. T064
remains open until the fresh-fence/shared-validation correction and both
component/live policy scenarios are verified.

### Combined ContractVersion 1.1 qualification result — 2026-09-22

The signed T064 source `e08f9af` and local #351 merge were verified by the
dedicated command below; the `fathom-feature012-contract11` cluster was deleted
by the task after completion:

```sh
KUBECONFIG=/tmp/fathom-feature012-contract11.kubeconfig \
E2E_KIND_CLUSTER=fathom-feature012-contract11 \
go -C tools tool task test-e2e \
  E2E_KIND_CLUSTER=fathom-feature012-contract11 \
  E2E_KIND_CONFIG=/tmp/fathom-feature012-contract11-kind.yaml \
  > /tmp/fathom-feature012-contract11-final-e2e.log 2>&1
```

The combined ContractVersion 1.1 run exited 0 with 105/105 specs passed in
1410.503s (Go package time 1412.630s), zero failures, pending or skips. The
full controller suite passed in 39.487s; the first race attempt had one
DNSCheck `lastRunTime` timing failure without race detection, and the corrected
unchanged-retry race rerun passed in 45.453s. T054 is verified from the separate
#351 decision/regression/migration evidence and the combined run; the upstream
#351 merge remains the release gate. T064 is complete. T059 remains open until
the feature PR is published. No merged upstream release or chart qualification
claim is made.
