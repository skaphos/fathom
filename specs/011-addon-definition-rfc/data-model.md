<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Conceptual Data Model: AddonDefinition RFC

This model defines the information the revised [RFC 0001](../../docs/rfc/0001-addondefinition-crd.md) must describe and the
records needed to review it. It is a documented design only: these names and
fields are conceptual, not a deployable Kubernetes API, storage schema, or
claim about current or future runtime behavior. Runtime choices remain proposed
until the RFC is approved.

## Design Entities

### Addon definition

A declarative proposal for evaluating one canonical addon identity.

| Field | Meaning |
| --- | --- |
| `objectIdentity` | Object name, scope, and immutable object UID. |
| `schemaRevision` | Version of the definition vocabulary, distinct from adapter and addon versions. |
| `addonIdentity` | Exact canonical identity claimed by this definition; no implicit aliases. |
| `supportedVersions` | Addon release ranges the definition proposes to evaluate. |
| `evaluators` | Supported typed evaluator families and their bounded check inputs. |
| `requestedReads` | Resource, discovery, and core-resource capabilities needed by evaluation. |
| `evaluationIdentityRef` | Reference to the administrator-controlled identity binding. |
| `freshnessPolicy` | Rule by which prior evidence becomes stale or unknown. |

Validation to settle in the RFC includes required fields, recognized evaluator
kinds, version syntax, cross-field rules, and finite limits for strings,
collections, nesting, result volume, work, elapsed time, and retries. The first
release should use the existing typed evaluator vocabulary and should not add a
new user expression language. Numeric limits and enforcement points belong in
the revised RFC and must not be invented by this planning artifact.

### Definition revision

An immutable identifier for the exact definition input used by an evaluation.

| Field | Meaning |
| --- | --- |
| `objectUID` | Distinguishes deletion and recreation under the same name. |
| `generation` | Distinguishes edits to the same object. |
| `schemaRevision` | Identifies the vocabulary used to interpret the snapshot. |
| `contentDigest` | Optional stable digest for audit and comparison, if the RFC selects it. |

A revision relates to exactly one definition snapshot. One definition object
can yield many revisions. A running evaluation retains one revision throughout;
publication must compare it with the active revision and visibly discard or
supersede an obsolete result according to the accepted RFC.

### Evaluation identity

The subject whose authority covers every discovery and object-read path used by
one runtime definition.

| Field | Meaning |
| --- | --- |
| `subjectRef` | Exact ServiceAccount or other supported subject identity. |
| `definitionBinding` | Administrator-controlled authorization for this addon identity to use the subject. |
| `requestedCapabilities` | Definition-declared reads used as requests and diagnostics. |
| `effectiveGrantContext` | Conceptual effective authorization from additive grants that apply to the subject. |
| `authorizationRevision` | Observable identity/binding context used for publication and diagnostics. |

Requested capabilities are not a privilege ceiling: Kubernetes grants combine
additively. `effectiveGrantContext` does not require serializing every RBAC
binding or claiming an atomic grant snapshot. The RFC must decide whether the
minimum requested set or the full effective grant union drives diagnostics,
and must keep those diagnostics independent of metrics configuration.
Permission to impersonate a subject does not authorize a definition to select
it. Missing binding, identity-scoped discovery, or scoped client must fail
closed, including manager and local kubeconfig paths; FR-005 permits no implicit
discovery exception.

### Evaluation evidence

The inspectable account of an attempted evaluation and its freshness.

| Field | Meaning |
| --- | --- |
| `addonIdentity` | Canonical identity evaluated. |
| `definitionRevision` | Exact source snapshot. |
| `authorizationContext` | Identity and binding context observed for the run. |
| `observations` | Bounded evaluator inputs and results sufficient to explain the verdict. |
| `verdict` | Documented healthy, warning, failure, error, or unknown outcome. |
| `observedAt` | Completion time associated with the preserved verdict and observations. |
| `latestAttemptAt` | Time of a later attempt, including one that failed before producing replacement evidence. |
| `freshness` | Current, stale, superseded, or unavailable state with age. |
| `failureReason` | Bounded, operator-readable denial, limit, timeout, conflict, or input error. |

Evidence belongs to one definition revision and one authorization context.
Last-known evidence retains its original `observedAt` after deletion,
invalidation, revocation, or missing input. A failed attempt may update
`latestAttemptAt` and failure state without presenting the preserved verdict as
a newly successful evaluation.

### Built-in definition

A typed definition shipped with Fathom. It has a canonical addon identity and
a release provenance. Built-ins remain available in the proposed first release;
runtime replacement is a separate decision. The RFC must choose a deterministic,
visible outcome for runtime/runtime conflicts and for a new built-in colliding
with an existing runtime identity during upgrade. Local schema validation or
CEL alone cannot enforce this global conflict rule; the RFC must name the actual
arbitration and upgrade enforcement points.

## Governance Entities

### RFC decision record

Exactly six principal records cover schema, RBAC, versions, identity,
lifecycle, and bounds. Each contains status (`Proposed` or `Accepted`), question,
selected proposal, relevant alternatives, rationale, negative consequences,
observable acceptance example, source evidence, and deferred work. Across the
RFC, alternatives include at least two real choices and the status quo.
No runtime choice becomes accepted before recorded approval of the final RFC
revision.

### Review record

| Field | Meaning |
| --- | --- |
| `revision` | Commit or PR revision actually reviewed. |
| `status` | Draft, In Review, Accepted, Rejected, or Withdrawn. |
| `owner` | `@mfacenet`, the default code owner responsible for review routing. |
| `decider` | Shawn Stratton, the existing RFC's designated decider. |
| `reviewPeriod` | Actual start/end dates and forum. |
| `findings` | Resolved objections, required changes, and explicit deferrals. |
| `decisionEvidence` | Attributable acceptance/rejection/withdrawal of the final revision. |

Allowed transitions are `Draft -> In Review`, `In Review -> Draft` for
substantive revision, and `In Review -> Accepted|Rejected|Withdrawn` after a
recorded decision. Withdrawal may also occur from Draft. Acceptance freezes the
document; substantive changes require a new or superseding RFC and renewed
approval of its final revision. A merge or named participant alone is not
approval evidence.

### Handoff record

The completion record links the merged accepted RFC, its recorded approval,
the resulting accepted ADR, and a read-back of the actual #280 update. It also
records #256's explicit disposition, accepted implementation scope, deferrals,
owners and resolution conditions. ADR 0001 remains immutable; the new ADR states
whether it narrowly supersedes startup-only loading or otherwise relates to it.

The feature is incomplete unless all required links resolve, approval covers
the merged final revision, #280 was actually updated and read back, and #256 has
an explicit disposition. Rejection, withdrawal, a draft ADR, an unmerged RFC,
or a proposed issue comment does not satisfy completion.

## Relationships and Invariants

1. One canonical addon identity has at most one active definition after global arbitration.
2. One evaluation uses exactly one definition revision and one authorization context.
3. Discovery, addon reads, and core-resource reads all use the declared evaluation identity.
4. Authorization changes independently of definition generation; revocation fences publication without claiming atomic rollback of prior reads.
5. Failures and limit exhaustion affect the responsible definition/check while unrelated work progresses.
6. `ClusterHealth` remains derived only from `HealthCheck.status`; evidence history is not a direct input.
7. RFC acceptance authorizes planning and ADR recording, not deployment or claimed runtime behavior.
