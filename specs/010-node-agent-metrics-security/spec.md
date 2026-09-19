<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Feature Specification: Node-agent metrics security

**Feature Branch**: `fix/node-agent-metrics-security`
**Created**: 2026-09-19
**Status**: Implemented and validated
**Input**: Implement [SEC-2 / #273](https://github.com/skaphos/fathom/issues/273), with regression coverage and a specification for the monitoring contract change.

## User Scenarios & Testing

### User Story 1 — Protect node inventory (Priority: P1)

As a cluster administrator, I need node inventory to be inaccessible to workloads without monitoring permission, even when network policies are not enforced.

**Independent Test**: An anonymous client cannot obtain node metrics from either agent mode or the supported monitoring endpoint; an authorized monitoring client can obtain useful signals.

**Acceptance Scenarios**:

1. **Given** certificate and health agents, including a host-network health agent, **when** an anonymous workload requests metrics, **then** no inventory is returned.
2. **Given** an authenticated client without metrics permission, **when** it requests the supported monitoring endpoint, **then** access is denied.
3. **Given** authorized monitoring, **when** it reads node metrics, **then** certificate paths, subjects, and issuers are absent from labels.
4. **Given** the security change, **when** an agent is running, **then** report publication and liveness remain functional without new cluster-wide agent permissions.

### User Story 2 — Keep monitoring useful (Priority: P1)

As a monitoring operator, I need the shipped scrape configuration to expose certificate-expiry and node-health signals with an explicit migration path for existing queries.

**Independent Test**: Authorized scrapes contain the earliest certificate expiry per check and node and the existing node-health measurements. Shipped check-alert rules continue to evaluate correctly.

**Acceptance Scenarios**:

1. **Given** multiple certificates on one node, **when** monitoring reads expiry, **then** the earliest known expiry is represented without revealing certificate paths.
2. **Given** checks with the same name in different namespaces or multiple checks on one node, **when** metrics are read, **then** their values remain distinct.
3. **Given** expired, removed, rejected, or out-of-scope reports, or a paused/deleted check, **when** the controller processes that state, **then** those reports no longer contribute node-detail metrics.
4. **Given** an upgrade, **when** an administrator follows the migration instructions, **then** the supported scrape configuration and updated example alerts work without direct agent scraping.

### Edge Cases

- A partial fleet report window must expose only accepted, currently in-scope evidence; it must not manufacture healthy measurements for missing nodes.
- A certificate scan without any known expiry emits no expiry value, rather than a healthy sentinel.
- Removing an item, node, or check must not erase another check's metrics or leave obsolete series after reconciliation.
- Operator restart rebuilds metrics from current reports; replicas follow the existing operator metrics/leader behavior.
- Existing agent image overrides need an explicit upgrade requirement; old binaries cannot be made safe solely by new operator code.

## Requirements

### Functional Requirements

- **FR-001**: Supported deployments MUST deny anonymous access to node metrics independently of network-policy enforcement, in both agent modes.
- **FR-002**: The supported monitoring endpoint MUST authenticate and authorize readers using the existing operator monitoring access contract.
- **FR-003**: Certificate metric labels MUST NOT contain certificate paths, subjects, or issuers. Expiry MUST represent the earliest known expiry per check and node.
- **FR-004**: Node-health measurement semantics MUST remain available, identified by namespace, check, node, and the existing item dimensions.
- **FR-005**: Metrics MUST use the controller's accepted, fresh, in-scope report set and be withdrawn on reconciliation of pause, deletion, invalid state, or report rejection/removal. Last-known CR status and history MUST remain intact.
- **FR-006**: Agents MUST continue publishing reports and answering liveness probes without additional cluster-wide permissions or new credential provisioning.
- **FR-007**: Shipped scrape configuration and check-alert rules MUST continue working. Documentation MUST explain the scrape and label migration, report cadence, stale evidence, replica behavior, and mixed-version rollout limits.
- **FR-008**: The fix MUST include a failing-before regression for the disclosure and real-cluster coverage for denial, authorized access, both agent modes, and continuing report/liveness operation.

### Key Entities

- **Node report**: existing certificate or health evidence associated with one check and node; accepted by the existing controller validation path.
- **Node metric series**: a bounded projection of accepted report evidence, scoped by namespace, check, and node.
- **Monitoring reader**: an authenticated principal with permission to read operator metrics.

## Success Criteria

### Measurable Outcomes

- **SC-001**: All tested anonymous and unauthorized node-metrics requests disclose zero inventory values.
- **SC-002**: Authorized monitoring observes the correct minimum expiry and health measurements for two independently scoped checks, with zero certificate-path labels.
- **SC-003**: Lifecycle regression tests withdraw obsolete node series while preserving unrelated checks and stored health history.
- **SC-004**: The complete Kind suite passes, including real node-agent publication and security tests; existing alert-rule tests pass.

## Assumptions

- Scope is #273; arbitrary label projection (#279), monitoring redesign (#276), and report-validation changes are separate work.
- Existing authenticated operator metrics are the supported deployment baseline; an administrator's explicit insecure-metrics override remains an opt-out.
- Upstream [Fathom facts](../../../skaphos-resources/tools/fathom/FACTS.md) establish standalone operation and published health/metrics. The [ecosystem assessment](../../../skaphos-resources/tools/ECOSYSTEM.md#fathom--cluster-platform-integrity-health-gate) supports reuse of existing monitoring infrastructure. Repository code is authoritative for current binary names and implementations.
