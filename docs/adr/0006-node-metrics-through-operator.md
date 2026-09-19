<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: MIT -->

# 6. Serve node metrics through the authenticated operator endpoint

- **Status**: proposed
- **Date**: 2026-09-19
- **Deciders**: Fathom maintainers, through review of #273's implementation

## Context and Problem Statement

Node agents expose certificate paths and expiry, and node-health measurements,
through an unauthenticated HTTP metrics endpoint. Their NetworkPolicies do not
protect non-enforcing CNIs or host-network health agents. The operator already
receives node reports through Kubernetes, validates their bindings and freshness,
and serves its own metrics using HTTPS, authentication, and authorization.

Adding delegated authentication to every dynamically created agent would require
cluster-scoped TokenReview/SubjectAccessReview grants and a TLS credential/trust
lifecycle. That adds a new permission and credential surface immediately after
the agent report-access permissions were narrowed in #255/#274.

## Considered Options

1. Add authenticated HTTPS to each agent and maintain per-agent scrape discovery,
   credentials, and auth-delegation permissions.
2. Remove agent metrics and project accepted reports through the existing operator
   endpoint.
3. Reduce labels and retain direct anonymous scraping behind NetworkPolicy.

## Decision Outcome

Choose **option 2**. Reuse the existing report validation and monitoring access
boundary. Agents continue scanning, publishing reports, and serving liveness;
their `/metrics` route is removed in both modes. No new cluster-wide grants,
credentials, dependencies, or CRD fields are needed.

Certificate expiry becomes one minimum known days-remaining value per namespace,
check, and node, with no certificate path label. Health measurements retain item
identity behind authorized operator access. Detailed certificate diagnostics
remain in Kubernetes reports.

The operator publishes only accepted, fresh, expected-node evidence. An incomplete
fleet may expose its accepted subset: one missing node should not hide another
node's measured failure. Metric detail is withdrawn and rebuilt per check during
reconciliation, following the existing DNS target metrics convention. This does
not change the last-known aggregate status or history policy in ADR-0005.

### Consequences

- Reuses the shipped authenticated ServiceMonitor and avoids new agent privileges.
- Custom agent scrapes and certificate queries grouped by path must migrate.
- Node-detail metrics now depend on controller reconciliation as well as agent
  publication. They are not a live scrape of the scanner. Existing check age and
  cadence metrics remain necessary to detect a stalled operator.
- A scrape during deletion/repopulation may briefly omit a check's detail series.
  Multi-replica queries must account for duplicate operator samples, as with other
  operator metrics.
- Both images must be upgraded and old agent pods terminated. Explicit old image
  overrides preserve the vulnerable endpoint; rollback reintroduces it.
- Existing listener flags, port allocation, liveness probes, and NetworkPolicies
  remain compatible. NetworkPolicy is defense in depth, never the inventory
  authorization boundary.

## Links

- [Issue #273](https://github.com/skaphos/fathom/issues/273)
- [Specification and plan](../../specs/010-node-agent-metrics-security/spec.md)
- [Metrics contract](../../specs/010-node-agent-metrics-security/contracts/metrics.md)
- [ADR-0005](0005-clusterhealth-staleness-semantics.md)
