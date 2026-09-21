<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Implementation execution evidence

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
