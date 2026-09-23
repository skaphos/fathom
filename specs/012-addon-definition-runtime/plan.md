<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Implementation Plan: Runtime Addon Definitions

**Branch**: `feature/280-runtime-addon-definitions` | **Date**: 2026-09-20
**Spec**: [spec.md](spec.md) | **Issue**: [#280](https://github.com/skaphos/fathom/issues/280)

## Summary

Implement accepted typed runtime loading in reviewable increments behind a disabled
configuration option. Schema/rendering lands first; scoped bounded execution and
lifecycle publication must all be complete before runtime activation is qualified.
This file records the original planning baseline. Implementation and qualification
have progressed from that baseline; current outcomes are recorded in
[execution.md](execution.md), [qualification.md](qualification.md), and
[operations-qualification.md](operations-qualification.md). The 2026-09-20
clarifications and 2026-09-22 test-contract supplement require leader election,
count completed Skipped as current evidence, use the target release CLI for
collision preflight, and separate deterministic boundary/panic tests from
real-cluster authority and lifecycle tests. See the [decision supplement](contracts/decision-supplement.md)
for the narrow RFC refinement.

## Delivery shape (2026-09-22)

The approved delivery is one feature PR organized into four review sections that
follow Milestones 1–4 above. Existing checkpoint commits remain review landmarks,
not separate PR heads that each require independent CI. The #256 compatibility
work is a separate prerequisite PR; its implementation and tests are maintained
in `/tmp/fathom-ratio-contract-256`, with the PR link to be added when published.
It is not claimed merged or resolved here.

## Technical Context

Go 1.27.1, Kubernetes modules v0.37.0 (go.mod is authoritative), controller-runtime,
existing declarative engine, cobra/viper, Masterminds SemVer and existing YAML stack.
Kubernetes CRDs/status are storage; immutable snapshots are bounded process state.
Target: supported Kubernetes Linux deployments and existing cross-platform fathomctl.
Tests: stdlib unit tests, Ginkgo/Gomega envtest and full kind e2e. Constraints and
performance goals are the exact caps in [contracts/runtime.md](contracts/runtime.md),
not throughput claims. No new service, expression runtime or process protocol.

## Constitution Check

Pre-design and post-design: PASS, with no requested exceptions.

- I/IV: definitions, bindings, status, conditions and history are Kubernetes resources.
- II/III: staged UID rendering is review-only, intended manifests round-trip through
  Git; pinned generation and immutable owned snapshots preserve determinism.
- V: no dependency on another Skaphos control plane; existing typed engine is adopted.
- VI/VII: source-attributed evidence survives input loss; attempts/freshness remain explicit.
- VIII: namespace/cluster target scope is declared and checked on every read.
- IX: limits and observation-based revocation are explicit. The original plan did
  not claim tests had run; current named results are linked from the execution and
  qualification records.
- Fathom constraints: minimal generated RBAC, bounded work, normal config precedence,
  and ClusterHealth consuming only HealthCheck.status remain intact.
- Existing accepted ADR 0007 resolves adoption/architecture; no upstream decision is
  reopened and no constitution amendment is needed.

## Project Structure

Feature artifacts are this directory's spec, research, data model, contracts,
quickstart, tasks and requirements checklist. Planned source changes:

- `api/v1alpha1/`: two new kinds plus evidence/provenance/freshness fields.
- `pkg/addondefinition/`: pure API-based rendering and generated inventory data.
- `internal/adapter/declarative/`: runtime conversion and ordered execution.
- `internal/adapter/impersonation/`, `internal/adapter/runtime/`: scoped bounded
  clients, counters, worker admission, compilation cache and lifecycle coordination.
- `internal/adapter/registry/`: immutable ownership and collision barriers.
- `internal/controller/`: definition/binding reconciliation, publication and mirrors.
- `internal/app/`, `internal/cli/`: default-off wiring and read-only authoring commands.
- `config/`, `deploy/helm/`, `docs/`, `test/e2e/`: generated distribution and validation.

The paths below are the original implementation map; completed and pending work
and named evidence are tracked in the execution and qualification records.

## Goal

An administrator installs typed, attributable coverage without a custom image,
under explicit authority, bounded work and inspectable lifecycle state.

## Acceptance Criteria

All FR-001–FR-016, all six accepted decisions, every lifecycle row and every
numeric row have named tests and recorded outcomes. Deterministic component tests
prove numeric at/over boundaries and injected recoverable panics; full kind
verifies actual admission/RBAC, delegated execution, lifecycle, drain, rollback
and hostile-input isolation. #256 is resolved before release.

## Assumptions and Unknowns

The RFC plus the recorded user clarification supplement govern; runtime loading remains off by default. Numeric
limits require measurement during implementation; implementation failure is not
permission to weaken them. No open user decision blocks planning. The concrete wire mappings, leadership
verification and CLI contracts are in [payloads](contracts/payloads.md),
[leadership](contracts/leadership.md) and [runtime](contracts/runtime.md).

## Original planning baseline

The planning baseline was a startup-only registry with shallow shared engine
definitions, grouped evaluator order, manager RESTMapper/local-client fallback,
and failed attempts refreshing LastRunTime. Existing transition-only reports and
CLI import restrictions remain preservation requirements. See [research.md](research.md)
for the original evidence and rejected alternatives; see [qualification.md](qualification.md)
for current implementation and qualification state.

## Target State

Two alpha resources, independent delegated identities, ordered immutable snapshots,
bounded scoped execution, publication fencing, retained evidence and read-only tools.

## Plan

### Milestone 1 — Schema and authoring (M; foundation and US1)

Define input/status contracts, admission and compiler validation, pure rendering
and staged UID lookup. Generate CRDs/docs through tasks. Verify every union branch,
reference/default/immutability rule, worst-size CEL admission and deterministic
no-write output. Risk: admitting unusable input; detect with semantic invalid cases.
Rollback: revert disabled code; retain installed CRDs/data if already used.

### Milestone 2 — Authority and bounded execution (M; US2)

Build dedicated discovery/read clients, scope intersections, shared counters,
cooperative compile/decode limits, supervised panics and fair worker admission.
Verify every cap at/over boundary, injected panics, all helper identities and
peer progress in deterministic component tests. This milestone is a component
checkpoint only: US2's spec acceptance remains open until production wiring and
real-cluster authority, hostile-input and lifecycle tests in milestones 3–4 pass.
Risk: fallback privilege or hidden unbounded reads; detect with denied discovery,
local/metrics-off and malicious fixture tests. Rollback: disable loader and drain.

### Milestone 3 — Lifecycle and evidence (M; US3)

Add owned snapshots, collision barriers, synchronized startup, dependency indexes,
pre/final uncached fences, mandatory election, independently verifiable Lease epoch,
cancellation/drain and attempt/evidence separation including completed Skipped.
Verify the complete event matrix with race-controlled runs, leader change and
unchanged-verdict revisions. Risk: stale publication or misleading freshness;
detect by barriers and immutable timestamp/context assertions. Rollback: disable,
record unavailable state and preserve history before binary downgrade.

### Milestone 4 — Qualification and delivery (M; US4)

Wire default-off option and packaging, document installation/preflight/rollback,
run full kind and CI (including final US2 acceptance), use target-version CLI
preflight, and record #256 disposition. Real kind evidence covers actual
RBAC/admission, authority, lifecycle, drain, rollback and hostile-input isolation;
component evidence covers exact boundaries and injected panics. Risk: fake clients
hide real RBAC/admission differences; only real-cluster evidence satisfies those
runtime gates.
Rollback: documented disable/drain/revoke sequence; do not delete stored APIs.

## Irreversible Steps

None in this planning change or disabled implementation delivery. Removing stored
schema/semantics versions is excluded; it requires inventory, backup and a separate
migration decision. Runtime reads already authorized cannot be rolled back.

## Rollback Plan

Disable admission to new runtime work, revoke bindings and drain under the current
leader; expose unavailable freshness, preserve/export evidence and leave CRDs.
Downgrade only after users stop relying on runtime checks, since old binaries do
not maintain their freshness. Re-enable only after direct revalidation.

## Done When

Implementation records named evidence for every contract row; normal CI/generated/
compatibility/licensing gates and full kind pass; #256 is resolved. Planning is
complete when the artifacts cross-reference and all tasks are dependency ordered.

## Complexity Tracking

No constitution violations. A separate runtime pool and binding resource are
required by accepted decisions, not speculative abstractions.

## Synthesis and Decisions

Socratic level: standard, because architecture is accepted and this is reversible
planning. Position: stage API/rendering before execution and lifecycle, keep all
runtime opt-in until qualification. Strongest objection: schema-first can produce
an unusable contract. Load-bearing assumption: it remains disabled while the
compiler/runtime are completed. Verify worst-size admission and conversion before
calling the schema increment ready. Verdict: retain sequence with explicit gates.

| Fork | Options | Criterion | Choice | Reversibility |
| --- | --- | --- | --- | --- |
| Delivery size | One epic PR / staged increments | Reviewable security boundaries | Four milestones, no premature activation | Cheap before activation |
| CLI sharing | Duplicate inventory / allowed pure package | Import boundaries and drift | Pure package plus generated inventory | Internal refactor |
| Design | Reopen authority / implement accepted contract | Existing approval | Preserve RFC/ADR | New decision needed for changes |

Depth audit: PASS for anchored seams, concrete mechanisms, existing-engine reuse,
real alternatives, status-quo custom-image cost, honest operational complexity,
falsifiable admission/runtime gates and visible decisions. No unresolved user-decision flags. Verify pinned manager lifecycle integration and
every payload mapping with executable tests before implementation qualification.

## Refined implementation boundaries

- `contracts/payloads.md` supplies the field-level input contract required before
  admission/conversion implementation, including check-specific scope and helper reads.
- `contracts/leadership.md` defines a Lease epoch, live renewal observation, monotonic
  takeover grace, mandatory-election behavior and independent CLI verification.
- API packages must not import pkg/addondefinition: use API-local schema literals
  and test their parity against validation constants to avoid a dependency cycle.
- Define the narrow budget/cancellation interface beside declarative EvalContext;
  runtime implements it. Declarative must not import internal/adapter/runtime,
  because runtime compilation/runner needs to import declarative. This prevents
  a package cycle while keeping budget implementation out of the public Go contract.
- A single bounded runtime context charges control-plane fences and evaluator reads
  while preserving separate identities. Final validation is not outside the deadline.
- Binding acceptance/status contracts are defined in US1; controller integration is
  US3. CLI contract tests use fake reads before live leader integration exists.
- US2 harness completion does not satisfy its real-cluster independent test. US4
  explicitly closes that acceptance after US3 wiring; no phase marks US2 fully done early.
- Test layers have distinct obligations: component tests use deterministic injected
  faults for exact numeric boundaries, impossible-under-normal-admission collision
  fixtures, recoverable panics, slot release and peer progress; Docker/kind tests
  use real permissions, execution, lifecycle, drain, rollback and hostile inputs.
  The shipped operator gains no fault-injection controls.
- Constitution recheck after refinements: PASS. Minimal namespaced Lease read rights,
  unchanged built-in execution and attributed Skipped history preserve the principles.
