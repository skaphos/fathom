<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Feature Specification: Runtime Addon Definitions

**Feature Branch**: `docs/280-addon-definition-implementation`
**Created**: 2026-09-20
**Status**: Draft implementation specification; design accepted, implementation unshipped
**Input**: Create the implementation spec and task breakdown for [#280](https://github.com/skaphos/fathom/issues/280).

The accepted [RFC 0001](../../docs/rfc/0001-addondefinition-crd.md) and
[ADR 0007](../../docs/adr/0007-runtime-addon-definition-loading.md) govern this
feature. This specification translates them into implementation acceptance, not
new architecture. All RFC numeric limits and lifecycle scenarios are normative.
The explicit answers below refine that baseline; their narrow effect on the RFC's
completed-evidence wording is recorded in the
[decision supplement](contracts/decision-supplement.md).

## Clarifications

### Session 2026-09-20

- Q: Should runtime addon checks require leader election, even when running a single operator instance? → A: Require leader election for runtime loading; if disabled, refuse runtime activation while leaving built-in behavior unchanged.

- Q: When a run completes with every check marked Skipped, should its current result replace an earlier Pass? → A: Replace current evidence with fresh Skipped evidence and explicit “no checks evaluated” coverage; retain earlier historical evidence and create a report only when the verdict changes.

- Q: How should administrators check for addon-name collisions before upgrading to a different Fathom version? → A: Use the target release’s fathomctl, display its version, and compare its bundled built-in inventory against live definitions; a separately supplied inventory file is not part of this interface.

- Q: How should offline rendering handle grants whose custom-resource plural or scope requires discovery? → A: Keep rendering offline. Emit only safely determined grants and explicit diagnostics requiring manual completion for custom-resource grants; never guess plurals or broaden scopes.

## User Scenarios & Testing

### User Story 1 - Author installable coverage (Priority: P1)

An addon author describes supported checks and a platform administrator prepares
reviewable installation manifests without rebuilding the operator.

**Why this priority**: Removes the custom-image requirement while making the
new resource contract independently testable before execution is enabled.

**Independent Test**: Admit a valid definition and disabled binding; render
installation manifests twice with identical inputs and compare output. No
runtime execution is required for this first increment.

**Acceptance Scenarios**:

1. **Given** a definition using any of the nine supported check kinds, **when**
   submitted, **then** valid typed payloads are accepted in declared order and
   unknown kinds, mismatched payloads, duplicate identities and excess sizes fail.
2. **Given** invalid version-source references or unsupported semantics, **when**
   activation is attempted, **then** the definition remains ineligible with a reason.
3. **Given** definition and dedicated identity manifests, **when** the administrator
   renders installation and subsequently resolves live UIDs, **then** output is
   reviewable and deterministic; the tool never applies resources or grants.
4. **Given** missing or incomplete declared reads, **when** diagnostics are shown,
   **then** they explain the limitation without claiming effective permissions.

### User Story 2 - Delegate bounded execution (Priority: P1)

An administrator enables coverage only under an explicitly delegated identity
and target scope; unrelated workloads keep making progress on hostile input.

**Why this priority**: Runtime execution creates a new authority boundary.

**Independent Test**: Run one enabled definition alongside a healthy peer in a
real cluster; deny discovery, change scope, exceed each budget and inject a
recoverable panic. Verify isolation, attribution and continued peer progress.

**Acceptance Scenarios**:

1. **Given** an unbound definition or borrowed built-in, manager or shared identity,
   **when** execution is requested, **then** no evaluation runs with that authority.
2. **Given** a matching enabled binding, **when** checks and helper reads run,
   **then** every read and discovery request uses its dedicated identity and scope.
3. **Given** an author edits the same definition UID to another already granted
   resource, **when** that target is in scope, **then** delegated execution is allowed;
   denied permissions or out-of-scope targets still prevent completed evidence.
4. **Given** missing identity infrastructure or local namespace configuration,
   **when** evaluated, **then** execution fails closed without local/admin fallback.
5. **Given** oversized responses, parser attacks, repeated errors, churn or panics,
   **when** evaluated, **then** bounded failure replaces no prior health evidence,
   releases work slots and allows another eligible definition a turn.

### User Story 3 - Preserve evidence across change (Priority: P1)

An operator sees which revision produced a verdict, whether it remains fresh,
and whether authorization has drained during changes, recovery and upgrades.

**Why this priority**: Old success must never be presented as new coverage.

**Independent Test**: Pause execution, edit/delete/recreate definition, binding,
identity and check, then release it. Verify publication rejection, original
observation retention, restart reconstruction and eventual recovery.

**Acceptance Scenarios**:

1. **Given** prior evidence, **when** input becomes invalid, missing, denied or
   superseded, **then** its time, revision and authority remain unchanged and
   freshness explains unavailability; no-evidence objects show Unknown.
2. **Given** a newer valid revision with the same verdict, **when** it completes,
   **then** current evidence updates but no transition-only historical report is added.
3. **Given** disabled authority, **when** all work drains, **then** acknowledgement
   identifies the current leader and observed generation with zero active runs;
   status-only writes do not invalidate runs and old-leader acknowledgements expire.
   With leader election disabled, runtime activation is refused even for a single
   operator instance; built-in behavior remains unchanged.
4. **Given** an upgrade introduces a built-in collision, **when** dispatch occurs,
   **then** both conflicting adapters are blocked until explicit resolution;
   unrelated built-ins continue and preflight inventory exposes the collision.
   Preflight uses the target release’s fathomctl, displays its version, and compares
   its bundled built-in inventory against live definitions.
5. **Given** a missing definition or restored grants, **when** state recovers,
   **then** watches and bounded retries recover checks without restarting Fathom.
6. **Given** retained Pass evidence ages beyond its freshness window, **when**
   inspected through HealthCheck and ClusterHealth, **then** it is not fresh success.
7. **Given** prior Pass evidence, **when** a valid run completes with all checks
   Skipped, **then** current evidence becomes Skipped with the new observation time
   and explicit “no checks evaluated” coverage. The verdict change creates a report;
   existing history remains unchanged. Repeated Skipped results update current
   evidence without creating additional reports.

### User Story 4 - Install and operate safely (Priority: P2)

An adopter follows authoring, upgrade and rollback guidance with generated
examples and verified real-cluster behavior.

**Why this priority**: A feature is incomplete without a repeatable installation
and explicit compatibility disposition.

**Independent Test**: Follow the documented two-stage install, collision preflight,
drain, rollback and re-enable procedures on the supported cluster version.

**Acceptance Scenarios**:

1. **Given** the default installation, **when** unbound definitions exist, **then**
   runtime loading remains disabled and compiled coverage is unchanged.
2. **Given** a qualified build, **when** an administrator opts in and installs
   matching bindings/grants, **then** new coverage works without rebuilding Fathom.
3. **Given** rollback, **when** loading is disabled and bindings revoked, **then**
   history remains inspectable with unavailable freshness; CRDs are not deleted.
4. **Given** unresolved ratio compatibility or a known input-triggerable fatal
   process failure, **when** release is assessed, **then** runtime release is blocked.

### Edge Cases

The complete RFC section 5 event table and section 6 boundary table are required
acceptance cases, including unknown kinds rejected before storage, legacy invalid
objects, partial informer sync, skipped upgrade preflight, status-only binding
writes, stale leader drain claims, exhausted pagination, policy-override bypass,
unchanged-verdict revision changes and final validation failure.

## Requirements

### Functional Requirements

- **FR-001**: Authors MUST express ordered, typed coverage with canonical identity,
  declared versions and validated references; unsupported input cannot activate (US1).
- **FR-002**: Administrators MUST independently authorize exact definition and
  dedicated identity incarnations, with explicit target scope and disabled default (US2).
- **FR-003**: Every evaluation read, discovery request and helper operation MUST
  honor delegated identity, read-only capability and scope without fallback (US2).
- **FR-004**: Authoring tools MUST produce deterministic, review-only installation
  and UID-binding output and expose collision inventory (US1, US4).
  Upgrade collision preflight MUST use the target release’s fathomctl and its
  bundled built-in inventory, display that version, and compare against live
  definitions. A separately supplied inventory file is outside this interface.
- **FR-005**: Diagnostics MUST reflect actual requests without speculative access,
  effective-union claims or a metrics dependency (US2).
- **FR-006**: Definition activation/removal MUST be attributable to immutable
  revisions; canonical collisions MUST block ambiguous dispatch (US3).
- **FR-007**: Completion MUST revalidate definition, check and authority context;
  mismatches or failed validation MUST reject publication (US3).
- **FR-008**: Input loss and failed attempts MUST retain original evidence,
  separately record attempts, age freshness and recover through bounded retries (US3).
  A valid completed all-Skipped run MUST replace current evidence with Skipped,
  its new observation time and explicit “no checks evaluated” coverage. Freshness
  MUST describe observation recency without implying healthy coverage; Skipped
  MUST NOT be presented as Pass. This is completed evidence, not a failed attempt.
- **FR-009**: History MUST remain transition-only and retain its original source;
  ClusterHealth MUST continue consuming only HealthCheck.status (US3).
- **FR-010**: Disabled bindings MUST expose current-leader, matching-generation
  drain acknowledgement; missing/stale acknowledgements MUST NOT imply drained (US3).
  Runtime loading MUST require leader election even with one operator instance.
  When leader election is disabled, runtime activation MUST be refused without
  changing built-in behavior.
- **FR-011**: All input, parse, transport, work, time, result, cache, retry and
  scheduling caps in RFC section 6 MUST apply, including policy overrides (US2).
- **FR-012**: Recoverable compile/evaluation panics MUST remain isolated; failures
  MUST NOT produce partial healthy verdicts or starve peers (US2).
- **FR-013**: Built-ins MUST remain compiled; runtime loading MUST be disabled by
  default until security and full real-cluster gates pass (US4).
- **FR-014**: Schema, semantics, adapter release, addon release and Go contract
  compatibility MUST remain distinct; #256 MUST be resolved before release (US4).
- **FR-015**: Installation, migration and rollback MUST preserve inspectable data,
  expose limits and state the observation-based revocation boundary (US4).
- **FR-016**: Each requirement, RFC event and numeric boundary MUST have direct
  tests; real admission/RBAC/evaluation MUST be exercised before release (all stories).

### Key Entities

- **AddonDefinition**: Cluster-wide identity, versioned typed ordered coverage and
  observed acceptance/diagnostics; independent from authorization.
- **AddonDefinitionBinding**: Administrator-owned delegation to exact definition
  and ServiceAccount UIDs, enabled state, target scope and drain acknowledgement.
- **Evaluation evidence**: Completed verdict, original observations/time, source
  revision and authority; separate latest attempt and freshness. Completed
  all-Skipped runs are evidence with explicit absence of evaluated health coverage.
- **Runtime snapshot**: Immutable eligible definition plus captured authority.
- **Installation output**: Reviewable manifests, staged UID references and collision inventory.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A documented installation yields working new coverage with zero
  custom operator builds and zero resources applied by the authoring tool.
- **SC-002**: Every unauthorized-identity and out-of-scope scenario denies execution
  without fallback; every same-UID delegated-read scenario matches the RFC.
- **SC-003**: Every lifecycle scenario retains attributable evidence and rejects
  superseded completion; unchanged verdicts create zero revision-only reports.
- **SC-004**: Every numeric boundary has at-limit and over-limit evidence; a healthy
  peer continues during failure/churn and no partial healthy result is published.
- **SC-005**: All real-cluster acceptance scenarios pass and rollback preserves
  all prior history; #256 disposition is linked before runtime release.

## Assumptions

- Accepted RFC/ADR decisions are settled; changes require explicit decision review.
- Administrators control bindings and grants; same-UID edits intentionally inherit
  delegated reads. Immediate atomic revocation is not promised.
- #256 is a separate implementation/release dependency; #149 does not control the
  new resources' independent alpha track.
- New evaluators, publisher signing, builtin replacement, generated live grants,
  cross-namespace identities and process isolation remain deferred.
- This delivery creates planning artifacts only; unchecked tasks are unimplemented.
