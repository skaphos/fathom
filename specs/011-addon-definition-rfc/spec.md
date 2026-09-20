<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Feature Specification: Complete AddonDefinition Design RFC

**Feature Branch**: `docs/addon-definition-rfc`

**Created**: 2026-09-19

**Status**: Draft

**Input**: Complete and validate the proposed AddonDefinition CRD design in
[#278](https://github.com/skaphos/fathom/issues/278) before implementation.

## Context and Scope

Fathom is a [standalone cluster-integrity primitive](../../../skaphos-resources/tools/fathom/FACTS.md)
whose published state is consumed one-way by other systems. The
[ecosystem review](../../../skaphos-resources/tools/ECOSYSTEM.md#fathom--cluster-platform-integrity-health-gate)
supports building this capability while learning from existing evaluator models. This work completes
the decisions in the [proposed RFC](../../docs/rfc/0001-addondefinition-crd.md)
without treating its current recommendations as accepted decisions.

The deliverable is an approved and merged RFC, with recorded approval and an
actual update to the existing [#280 implementation epic](https://github.com/skaphos/fathom/issues/280).
It defines the first-release contract, observable outcomes, tradeoffs, and
handoff. It does not implement a CRD, change runtime behavior, grant
privileges, or create a second implementation epic.

## Clarifications

### Session 2026-09-19

- Q: Should this feature be complete when the RFC is ready for review, or only after it is approved and merged? → A: Finish only after RFC approval, merge, and updates to #280.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Review a Complete Safety Boundary (Priority: P1)

As an RFC reviewer, I can determine exactly which identity evaluates a runtime
definition, which reads it can perform, and how every RBAC alternative affects
privilege, so I can accept or reject the design without relying on assumptions.

**Why this priority**: A user-authored definition must never become an indirect
way to use the operator or another identity's grants.

**Independent Test**: Review the RFC's RBAC comparison and trace an arbitrary
definition author through identity selection, impersonation, discovery, core
resource reads, and error handling in every supported execution mode.

**Acceptance Scenarios**:

1. **Given** an author can create a definition but cannot administer RBAC,
   **When** the author names an identity that already has broad grants, **Then**
   the RFC states why operator impersonation alone does not authorize that use
   and identifies the explicit administrator-controlled authorization step.
2. **Given** evaluation runs without configured per-addon clients or outside
   the cluster, **When** a fallback client could carry operator or local
   kubeconfig privileges, **Then** the RFC chooses and justifies a fail-closed
   or equivalently bounded behavior rather than silently inheriting them.
3. **Given** an evaluator performs discovery or reads core resources in
   addition to addon resources, **When** its effective access is reviewed,
   **Then** every read path is assigned to the declared evaluation identity.

---

### User Story 2 - Predict Definition Lifecycle Outcomes (Priority: P1)

As an operator, I can predict the visible status, evidence, and active revision
when a definition is missing, added, edited, deleted, recreated, revoked, or
encountered during restart and concurrent evaluation.

**Why this priority**: A dynamic definition is trustworthy only when changes
cannot mix revisions, retain authority after revocation, or present stale
evidence as fresh success.

**Independent Test**: Apply the lifecycle table in the RFC to each required
event and determine the next evaluation behavior, status condition, verdict,
evidence freshness, and recovery path without guessing.

**Acceptance Scenarios**:

1. **Given** revision A is evaluating while revision B becomes active, **When**
   the evaluation completes, **Then** the RFC specifies one coherent revision
   for its inputs, authorization, verdict, and evidence.
2. **Given** the active definition is deleted or its authorization is revoked,
   **When** the next check is due, **Then** last-known evidence remains
   inspectable with explicit freshness/loss state and is not reported as a new
   successful evaluation.
3. **Given** a definition is deleted and recreated with the same name, **When**
   Fathom restarts or reconciles it, **Then** the RFC distinguishes the new
   identity/revision from the deleted object and defines deterministic recovery.

---

### User Story 3 - Implement from an Unambiguous Contract (Priority: P2)

As an implementation owner, I can refine the existing epic into bounded work
because schema, versioning, identity, precedence, lifecycle, and failure
isolation decisions each include alternatives, rationale, consequences, and
acceptance examples.

**Why this priority**: Implementation should follow reviewed decisions rather
than resolve architectural questions inside code changes.

**Independent Test**: Map every accepted RFC decision and deferred item to
[#280](https://github.com/skaphos/fathom/issues/280) without inventing missing
behavior or creating a duplicate epic.

**Acceptance Scenarios**:

1. **Given** two runtime definitions claim one addon identity, or a later
   Fathom release adds a built-in definition with that identity, **When**
   precedence is evaluated, **Then** the RFC gives one deterministic result and
   a visible operator outcome for both collisions.
2. **Given** a definition contains an unknown evaluator kind or reaches a size,
   expression, read, or time bound, **When** it is admitted or evaluated,
   **Then** the RFC identifies the boundary and the affected definition/check
   fails without blocking unrelated work.
3. **Given** [#256](https://github.com/skaphos/fathom/issues/256) reserves
   `warnRatio` and `failRatio`, **When** contracts are versioned, **Then** the
   RFC either resolves the coordination or records a bounded deferral with an
   owner, dependency, and condition for resolution.
4. **Given** the RFC is ready for review, **When** approval, merge, or the
   #280 epic update is still missing, **Then** the feature remains incomplete;
   completion requires recorded approval, a merged RFC, and an updated #280
   epic that reflects the accepted scope and deferrals.

### Edge Cases

- A runtime identity duplicates another definition through normalization,
  aliases, or a delete/recreate race.
- An upgrade introduces a built-in identity already used by a runtime
  definition.
- Authorization is removed after evaluation begins but before evidence is
  published.
- Restart observes a definition and its authorization resources at different
  points in reconciliation.
- Discovery succeeds under one identity while the subsequent resource read
  would use another.
- A definition reaches multiple bounds simultaneously or repeatedly fails,
  while unrelated definitions continue to evaluate.
- Previously successful evidence outlives its definition or declared freshness
  window.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The RFC MUST keep proposed choices distinguishable from accepted
  decisions until reviewers approve them.
- **FR-002**: The RFC MUST decide all six #278 topics: schema, RBAC, contract
  versioning, identity/precedence, runtime lifecycle, and bounded failure
  isolation. Each decision MUST state alternatives considered, rationale,
  consequences, and at least one observable acceptance example.
- **FR-003**: The schema decision MUST define the supported first-release
  vocabulary, required fields and relationships, validation behavior for
  unknown evaluator kinds, and finite bounds for strings, collections,
  expressions, and nested work.
- **FR-004**: The RBAC decision MUST compare pregranted permission sets,
  separate administrator grants, bounded operator-generated grants, and a
  trusted-publisher gate. It MUST state why the selected option does or does not
  add operator privileges and what explicit administrator action establishes
  authority.
- **FR-005**: The trust analysis MUST ensure a definition author cannot borrow
  another identity's grants solely because the operator can impersonate that
  identity. It MUST cover addon reads, discovery, core-resource reads, absent
  per-addon clients, and out-of-cluster/local-kubeconfig execution.
- **FR-006**: Declared permissions MUST be treated according to an explicit
  decision as grants, requests, diagnostics, or documentation; the RFC MUST
  never imply that applying a definition automatically creates privileges
  unless that option is explicitly selected and its escalation risk accepted.
- **FR-007**: The versioning decision MUST distinguish schema compatibility
  from the existing adapter contract and MUST resolve or explicitly defer
  coordination with #256, including the reserved `warnRatio` and `failRatio`
  keys.
- **FR-008**: The identity and precedence decision MUST define canonical
  identity, duplicate runtime identity handling, runtime-versus-built-in
  precedence, and the upgrade case where a new built-in collides with an
  existing runtime definition.
- **FR-009**: The lifecycle decision MUST define observable behavior for
  missing, added, edited, deleted, recreated, and invalid definitions; process
  restart; authorization revocation; recovery; and an edit or revocation while
  evaluation is in flight.
- **FR-010**: Every evaluation and published result MUST correspond to one
  coherent definition revision and authorization context. The RFC MUST define
  how that revision is identified and how superseded in-flight results are
  handled.
- **FR-011**: Last-known verdicts and evidence MUST remain inspectable when
  inputs are unavailable, with age, source revision, and freshness state made
  clear. Stale evidence MUST NOT be represented as a current successful run.
- **FR-012**: Failure isolation MUST bound admitted size, evaluation work,
  elapsed time, retry behavior, and blast radius so one definition or check
  cannot wedge reconciliation or prevent unrelated checks from progressing.
- **FR-013**: The RFC MUST decide its existing open questions: cluster-scoped
  versus namespace-scoped definitions, rendered RBAC assistance versus
  documentation-only guidance, and whether generated authoring samples are in
  first-release scope.
- **FR-014**: First-release scope and non-goals MUST explicitly address
  replacement of built-in definitions, new evaluator kinds, migration of the
  built-in pack, automatic privilege creation, publisher trust, authoring aids,
  and cross-namespace or out-of-cluster behavior.
- **FR-015**: The RFC MUST state whether it is a compatible extension of the
  accepted [ADR 0001](../../docs/adr/0001-in-process-adapter-contract.md) or
  requires a new superseding ADR. The accepted ADR MUST remain immutable.
- **FR-016**: After RFC approval and merge, the existing #280 epic MUST be
  updated to reference the merged RFC and cover accepted work and explicit
  deferrals; no duplicate implementation epic or implementation claim is part
  of this specification.

### Key Entities *(include if feature involves data)*

- **Addon definition**: A declared addon identity, supported versions,
  evaluator families, checks, requested reads, and schema revision.
- **Evaluation identity**: The administrator-authorized subject and complete
  set of paths under which a definition may discover and read resources.
- **Definition revision**: An identity for the exact definition used by one
  evaluation, independent of changes to its authorization.
- **Evaluation evidence**: The verdict inputs, observations, source revision,
  authorization context, completion time, freshness, and failure reason exposed
  to operators.
- **Built-in definition**: A definition shipped with Fathom whose identity may
  collide with a runtime definition across installation or upgrade.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Reviewers can locate exactly six principal decision records, one
  per #278 topic, and each contains alternatives, rationale, consequences, and
  an observable acceptance example.
- **SC-002**: For 100% of the lifecycle and collision scenarios named in
  FR-008 through FR-011, a reviewer can derive a single documented active
  revision, authorization outcome, visible state, and recovery behavior without
  unresolved alternatives.
- **SC-003**: The security review traces 100% of evaluation read paths to an
  explicitly authorized identity and finds no path that silently uses operator
  or local-kubeconfig privileges.
- **SC-004**: Every collection, string, expression, evaluation, and retry class
  admitted by the first-release design has a stated finite bound or an explicit
  reason it cannot amplify work.
- **SC-005**: After approval is recorded and the RFC is merged, the existing
  #280 epic is updated to reference the merged RFC and map accepted decisions
  and deferrals to one scoped refinement, with #256 and ADR 0001 each receiving
  an explicit disposition and no duplicate epic.

## Assumptions

- Fathom remains independently deployable; downstream consumers read its
  published health state and do not become dependencies of definition loading.
- `ClusterHealth` remains derived only from `HealthCheck.status`; runtime
  definitions and report history do not become direct inputs to that contract.
- The existing evaluator vocabulary is the baseline to assess, while the RFC
  must verify rather than assume that every evaluator obeys the selected trust
  boundary and finite work limits.
- The current RFC is evidence and a proposed design, not authority to implement
  its recommendations before review.
- Git-managed declarations and explicit administrator grants remain the durable
  desired-state boundary.
- This specification judges documentation completeness and reviewability. It
  requires documented approval, merge, and handoff to #280, and makes no claim
  that runtime behavior, tests, manifests, or user documentation have shipped.
