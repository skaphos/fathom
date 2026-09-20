<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# 7. Load typed addon definitions under explicit administrator authority

- **Status**: accepted
- **Date**: 2026-09-20
- **Deciders**: Shawn Stratton
- **Partially supersedes**: [ADR 0001](0001-in-process-adapter-contract.md), startup-only registration
- **Decision source**: [RFC 0001](../rfc/0001-addondefinition-crd.md), approved revision `80d1dd71033cd7cfa70a6680152d100118f8211a`

## Context and Problem Statement

The declarative engine already expresses adapters as typed data, but the registry
loads them only at startup from compiled code. Adding coverage requires a custom
operator image. Existing impersonation grants do not authorize a definition author
to select an arbitrary allowlisted ServiceAccount, and runtime mutation introduces
revision, revocation and stale-evidence concerns absent from static registration.

## Decision Drivers

- Install additional coverage without rebuilding the operator.
- Keep authority administrator-controlled and reads attributable to one identity.
- Preserve inspectable evidence without presenting old success as a fresh run.
- Bound work and retain deterministic behavior during restart and concurrent edits.

## Considered Options

1. Keep compiled definitions and startup-only registration.
2. Load typed cluster definitions with separate administrator bindings and grants.
3. Load runtime definitions with shared pregrants or operator-generated grants.
4. Run out-of-process adapter binaries with a separate execution protocol.

## Decision Outcome

Choose option 2 under the six accepted decisions in RFC 0001. Cluster-scoped
alpha definitions use the existing evaluator vocabulary; an administrator-owned
UID-bound resource delegates a dedicated ServiceAccount and explicit target scope.
All evaluation reads and discovery use that identity without manager fallback.
Immutable snapshots, publication revalidation, collision barriers and finite
budgets govern runtime dispatch. Built-in adapters remain compiled in.

This narrowly supersedes ADR 0001's startup-only registration choice. Its
in-process Go contract and internal registry boundary remain. ADR 0001 itself
stays unchanged. Acceptance records architecture, not shipped implementation;
implementation and real-cluster validation remain in #280, with the #256
compatibility disposition required before release.

### Consequences

- **Positive**: adopters gain runtime coverage without maintaining an operator
  fork, with explicit authority and attributable evaluation evidence.
- **Negative**: a second alpha resource, UID staging, dedicated identities,
  scoped discovery, lifecycle logic and bounded scheduling add operational and
  implementation cost. Large checks may need narrowing to fit the initial caps.
- **Negative**: publication fencing is observation-based, not an atomic Kubernetes
  authorization transaction. Immediate linearizable revocation is not promised.
- **Neutral**: schema, evaluator semantics, adapter releases, addon releases and
  the Go contract have distinct compatibility meanings.

## Pros and Cons of the Options

Static registration avoids the added API and trust boundary but preserves custom
builds. Shared pregrants simplify installation at the cost of shared authority;
operator grants enlarge the operator's privilege boundary. Separate bindings cost
more installation work but make delegation explicit. Out-of-process execution
improves fault isolation but adds protocol, deployment and startup overhead beyond
the first-release scope. The RFC records the detailed alternatives and limits.

## Links

- [RFC review PR #349](https://github.com/skaphos/fathom/pull/349)
- [Implementation epic #280](https://github.com/skaphos/fathom/issues/280)
- [Ratio compatibility #256](https://github.com/skaphos/fathom/issues/256)
