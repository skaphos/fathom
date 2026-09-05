<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Feature Specification: DNSCheck Completion

**Feature Branch**: `feature/267-dnscheck-completion`

**Created**: 2026-08-23

**Status**: Draft

**Input**: Complete DNSCheck as an end-to-end health signal by making it a
supported `HealthCheck` target, carrying its verdict into `ClusterHealth`, and
closing the remaining real-cluster and documentation gaps.

## Overview

DNSCheck already has a resource contract, reconciler, probe execution, and
real-cluster coverage. The remaining gap is composition: `HealthCheck` claims
to reference several specialized checks but only mirrors `AddonCheck` today.
As a result, a DNS verdict cannot participate in the aggregate cluster verdict.

This feature completes that integration for every specialized check kind that
actually exists in the current API: `AddonCheck`, `DNSCheck`, and
`NodeCertificateCheck`. It also removes references to unimplemented kinds,
defines API-version behavior, verifies the complete DNS-to-cluster path on a
real cluster, and adds focused authoring guidance.

## Decisions and Tradeoffs

| Decision | Choice | Rationale |
|---|---|---|
| Supported target kinds | Advertise and support `AddonCheck`, `DNSCheck`, and `NodeCertificateCheck` only | A public contract must describe behavior that exists. `NodeHealthCheck` and `ReachabilityCheck` are not current resources and must not appear as supported targets. |
| Target API version | Treat an empty value as the current Fathom API version; accept the exact current version; reject unsupported nonempty values | Silently ignoring the field can bind a reference to the wrong contract. Keeping it bounded but not admission-enumerated avoids blocking a future version migration. |
| Extensibility boundary | Route each target kind through an explicit kind-specific handler contract | Adding another specialized check should not require growing one monolithic reconciliation branch. The implementation location is deferred to planning. |
| End-to-end scope | Add integration coverage for DNSCheck through HealthCheck and ClusterHealth while retaining existing DNS resolver scenarios | Existing tests already establish DNS execution behavior. Completion needs proof of composition, not duplicated resolver tests. |

Doing nothing leaves the generic aggregation API misleading and forces users to
observe DNS separately from cluster health. The scope decision should be
revisited if another specialized check kind lands before implementation; in
that case, its inclusion must be evaluated against the same complete-contract
and test requirements rather than added to documentation alone.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - DNS contributes to cluster health (Priority: P1)

As a cluster operator, I can reference a DNSCheck from a HealthCheck so that
DNS availability contributes to the same ClusterHealth verdict used by my
other health signals.

**Why this priority**: Without this path, DNSCheck is observable but not
composable, which is the principal remaining functional gap.

**Independent Test**: On a real cluster, create a DNSCheck, a HealthCheck that
references it, and a ClusterHealth that selects the HealthCheck. Change the DNS
outcome and verify that both downstream verdicts and evidence converge.

**Acceptance Scenarios**:

1. **Given** a passing DNSCheck and a HealthCheck that references it, **When** reconciliation completes, **Then** the HealthCheck reports Pass with DNS-derived evidence and the selecting ClusterHealth reports Pass.
2. **Given** that same resource chain, **When** the DNSCheck changes to Fail, **Then** the HealthCheck reports Fail with current DNS-derived evidence and the selecting ClusterHealth reports Fail.
3. **Given** a DNSCheck whose verdict has not changed, **When** it is reconciled again, **Then** the integration does not create duplicate change-history records.

---

### User Story 2 - Every advertised target kind works (Priority: P1)

As an API user, I can trust that every kind named by the HealthCheck target
contract is accepted and mirrored consistently.

**Why this priority**: A contract that advertises unsupported kinds produces
valid-looking resources that can never become useful.

**Independent Test**: Reference each advertised kind from a HealthCheck and
verify the same projection semantics for verdict, observation time, interval,
report identity, and bounded summary.

**Acceptance Scenarios**:

1. **Given** an AddonCheck, DNSCheck, or NodeCertificateCheck in the referenced namespace, **When** a HealthCheck targets it with the current API version, **Then** the source status is projected into the HealthCheck status.
2. **Given** the HealthCheck API documentation, **When** a user selects a target kind, **Then** every listed kind corresponds to a resource and supported projection path in the release.
3. **Given** existing AddonCheck-backed HealthChecks, **When** this feature is deployed, **Then** their behavior remains unchanged.

---

### User Story 3 - Invalid references fail explicitly and safely (Priority: P2)

As an operator troubleshooting configuration, I receive a clear bounded status
when a target cannot be resolved instead of observing stale or misleading data.

**Why this priority**: Explicit failure behavior prevents an old healthy
snapshot from masking a broken reference while preserving useful state during
temporary control-plane errors.

**Independent Test**: Exercise an unsupported kind, unsupported API version,
missing target, and transient lookup failure, then inspect HealthCheck status.

**Acceptance Scenarios**:

1. **Given** a nonempty unsupported API version or unsupported kind, **When** the HealthCheck reconciles, **Then** its target snapshot is cleared and its status explains the supported contract without unbounded detail.
2. **Given** a target that does not exist, **When** the HealthCheck reconciles, **Then** its target snapshot is cleared so stale health is not retained.
3. **Given** a transient target lookup failure, **When** the HealthCheck reconciles, **Then** its last known target snapshot is preserved and the controller retries according to existing error behavior.

---

### User Story 4 - DNSCheck is straightforward to author and diagnose (Priority: P2)

As a Fathom user, I have one focused guide showing how to author DNS checks,
choose resolver and expectation behavior, connect them to aggregate health, and
interpret common failures.

**Why this priority**: The API is already capable, but its behavior is spread
across reference material and examples rather than an operational workflow.

**Independent Test**: Follow the guide on a cluster using both the cluster
resolver and an explicit resolver, then trace the result through HealthCheck
and ClusterHealth without consulting source code.

**Acceptance Scenarios**:

1. **Given** a new user, **When** they follow the guide, **Then** they can create a DNSCheck and connect it to aggregate health using complete manifests.
2. **Given** a failing DNSCheck, **When** the user consults the guide, **Then** they can distinguish resolution failure, expectation mismatch, timeout, and explicit-resolver reachability symptoms.

### Edge Cases

- An empty target namespace resolves in the HealthCheck namespace; an explicit
  namespace is honored.
- An empty target API version defaults to `fathom.skaphos.io/v1alpha1`; any
  nonempty value must be matched exactly.
- A target may exist before it has published status; the HealthCheck remains
  bounded and does not invent a verdict.
- A target may be deleted after a healthy snapshot was projected; deletion
  clears that snapshot.
- A target status update unrelated to the referenced object must not enqueue or
  mutate the HealthCheck.
- DNS evidence can contain verbose resolver errors; summaries remain bounded by
  the existing HealthCheck status contract.
- The referenced source can request an interval that differs from the wrapper;
  the effective source interval is projected consistently with existing
  AddonCheck behavior.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The HealthCheck target contract MUST advertise exactly the specialized check kinds supported by the current release: AddonCheck, DNSCheck, and NodeCertificateCheck.
- **FR-002**: The HealthCheck target contract MUST NOT advertise NodeHealthCheck, ReachabilityCheck, or any other resource kind that does not exist and cannot be projected.
- **FR-003**: A HealthCheck MUST be able to reference and project status from an AddonCheck, DNSCheck, or NodeCertificateCheck.
- **FR-004**: An empty target API version MUST resolve to `fathom.skaphos.io/v1alpha1` for backward compatibility.
- **FR-005**: A nonempty target API version MUST be honored and MUST match the exact supported version before the target is read.
- **FR-006**: Unsupported nonempty API versions MUST produce a clear bounded status and MUST NOT be silently treated as the current version.
- **FR-007**: The target API-version field MUST remain length-bounded and MUST NOT be admission-enumerated to the current version, preserving a migration path for future API versions.
- **FR-008**: Existing target-reference immutability rules MUST remain unchanged.
- **FR-009**: Projection from every supported kind MUST populate the existing HealthCheck target snapshot with the source verdict, observed time, effective interval, report identity when available, and a bounded human-readable summary.
- **FR-010**: Projection MUST NOT add kind-specific status fields to HealthCheck or ClusterHealth.
- **FR-011**: A missing or deleted target MUST clear the prior target snapshot so stale health cannot contribute to aggregation.
- **FR-012**: A transient target lookup failure MUST preserve the last known target snapshot and follow the existing controller retry behavior.
- **FR-013**: An unsupported kind or API version MUST clear the prior target snapshot and identify the supported contract in bounded status output.
- **FR-014**: An omitted target namespace MUST default to the HealthCheck namespace, and an explicit namespace MUST be honored.
- **FR-015**: Source status updates and deletion events MUST enqueue only HealthChecks whose effective group, version, kind, namespace, and name match the changed source.
- **FR-016**: Existing AddonCheck projection and watch behavior MUST remain backward compatible.
- **FR-017**: ClusterHealth MUST continue deriving its external contract only from HealthCheck status and MUST NOT read DNSCheck or HealthReport objects directly.
- **FR-018**: DNS verdict and bounded evidence changes MUST propagate through HealthCheck to any selecting ClusterHealth.
- **FR-019**: Existing change-only HealthReport behavior MUST be preserved across the DNS-to-aggregate path.
- **FR-020**: Real-cluster tests MUST prove Pass and Fail propagation from DNSCheck through HealthCheck to ClusterHealth.
- **FR-021**: Existing DNS resolution, absence expectation, explicit resolver, truncation, restricted-policy, and lifecycle e2e coverage MUST remain part of the core DNS test tier.
- **FR-022**: A dedicated DNSCheck guide MUST document authoring, resolver selection, positive and absence expectations, aggregation wiring, and troubleshooting.
- **FR-023**: Existing overview and architecture documentation MUST link to or accurately reflect the completed aggregation path.
- **FR-024**: This feature MUST NOT require new cluster-wide permissions; controllers MUST use the permissions already required for the three existing specialized resources.

### Key Entities

- **CheckTargetRef**: Immutable reference from a HealthCheck to a specialized check, identified by API version, kind, namespace, and name.
- **Specialized check**: An AddonCheck, DNSCheck, or NodeCertificateCheck that owns execution-specific configuration and publishes a verdict plus evidence.
- **HealthCheck target snapshot**: The normalized, bounded projection of a specialized check's latest status used by aggregation.
- **ClusterHealth**: Aggregate health view derived exclusively from selected HealthCheck statuses.
- **HealthReport**: Change-history record whose existing deduplication semantics remain unchanged.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In real-cluster tests, a DNSCheck Pass and a subsequent Fail each propagate through the referenced HealthCheck to the selecting ClusterHealth within the existing reconciliation timeout.
- **SC-002**: Automated tests exercise successful status projection for 100% of the target kinds advertised by HealthCheck.
- **SC-003**: Automated tests verify missing targets, unsupported kinds, unsupported API versions, defaulted namespaces, explicit namespaces, and transient lookup failures without retaining stale state incorrectly.
- **SC-004**: Existing AddonCheck-focused unit and e2e tests continue to pass without changed user-visible behavior.
- **SC-005**: The generated API reference and authored documentation name no unsupported HealthCheck target kinds.
- **SC-006**: A user can copy complete manifests from the DNSCheck guide to create a DNS signal and include it in ClusterHealth without consulting implementation source.

## Assumptions and Dependencies

- DNSCheck resource reconciliation and probe execution delivered by specifications
  005 and 006 are the baseline and are not redesigned here.
- AddonCheck projection defines the existing normalized HealthCheck status
  semantics that the additional kinds must preserve.
- NodeCertificateCheck remains an existing v1alpha1 specialized resource with a
  controller-published verdict that can be normalized into the same snapshot.
- ClusterHealth selection and verdict aggregation remain unchanged.
- The current Fathom API group and version are
  `fathom.skaphos.io/v1alpha1`.

## Out of Scope

- Implementing NodeHealthCheck, ReachabilityCheck, or a user-extensible target
  plugin mechanism.
- Redesigning DNS probe execution, resolver selection, retry policy, or DNS
  result schemas already covered by specifications 005 and 006.
- Adding direct DNSCheck selection or DNS-specific fields to ClusterHealth.
- Changing HealthReport retention or deduplication semantics.
- Introducing a new API version solely to complete this integration.

## References

- GitHub epic #205 and completion issues #267 and #268
- ADR-0004: unified signal framework
- `specs/005-dnscheck-resource-contract/`
- `specs/006-dnscheck-reconciler/`
