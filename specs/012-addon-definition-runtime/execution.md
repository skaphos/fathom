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
