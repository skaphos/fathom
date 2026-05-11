<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# 4. HealthCheck as a thin wrapper over a specialized check

- **Status**: accepted
- **Date**: 2026-05-11
- **Deciders**: Shawn Stratton

## Context and Problem Statement

`HealthCheck` and `ClusterHealth` were kubebuilder placeholders since
project init (`Foo string`). AGENTS.md states:

> Keep the `ClusterHealth` external contract stable. It is derived only
> from `HealthCheck.status` — never from `HealthReport` history.

But the project has multiple specialized check kinds, each with its own
schema:

- `AddonCheck` — shipped in v0.1 with three working adapters.
- `DNSCheck`, `NodeHealthCheck`, `NodeCertificateCheck`, `ReachabilityCheck`
  — planned in SKA-47, SKA-48, SKA-49, SKA-50.

`ClusterHealth` cannot aggregate over each specialized schema directly
without coupling the aggregator to every specialized type. We need a
single, uniform thing the aggregator reads. The placeholder `HealthCheck`
is the natural slot, but its actual role had never been decided.

## Decision Drivers

- The AGENTS.md invariant is load-bearing for ADR-0002 (HealthReport vs.
  status separation). Anything that gives `ClusterHealth` history-
  shaped data forecloses that.
- v0.1 already shipped `AddonCheck` as a real CRD with a reconciler and
  three adapters. A choice that requires reversing it is out of scope.
- The body-implementation tickets (SKA-309 HealthCheck reconciler,
  SKA-310 ClusterHealth reconciler) need a concrete schema before they
  can start.
- Adding a wrapper resource per check is a UX cost. The decision should
  weigh that against alternatives that fold the wrapper away.

## Considered Options

- **A. HealthCheck as a thin wrapper over a specialized check.** Spec
  carries `CheckRef` (kind+name+optional namespace) pointing at one
  specialized check. The HealthCheckReconciler watches the referenced
  check and mirrors its status into a uniform shape (Result, Summary,
  SourceObservedAt, LastReportName, Conditions). ClusterHealth aggregates
  over `HealthCheck.Status.Result` only.
- **B. Discriminator-typed unified CRD.** Fold `AddonCheck`, `DNSCheck`,
  etc. into `HealthCheck.spec.type = addon|dns|node|...` with type-
  specific sub-specs. Specialized kinds become deprecated.
- **C. Delete HealthCheck and ClusterHealth.** Make the specialized kinds
  the user-facing types directly; rework the AGENTS.md invariant or
  drop the cluster-wide aggregator.

## Decision Outcome

Chosen option: **A. HealthCheck as a thin wrapper over a specialized
check**, because it is the smallest schema that satisfies the AGENTS.md
invariant without invalidating shipped CRDs (`AddonCheck`) or planned
ones (DNSCheck/NodeHealthCheck/etc.).

Spec carries:
- `CheckRef` — required, points at the specialized check
- `Description` — optional, human-readable purpose
- `Paused` — optional, suspends mirroring while preserving last snapshot

Status carries the uniform shape: `Result` (HealthReportResult enum),
`Summary`, `SourceObservedAt`, `LastReportName`, `Conditions`,
`ObservedGeneration`. `ClusterHealth.spec.Selector` selects HealthChecks;
`ClusterHealth.status` rolls up the worst-case `Result`,
`MatchedCount`, and per-child summaries derived strictly from
HealthCheck.Status.

### Consequences

- **Positive**: `ClusterHealth` has one schema to read, regardless of how
  many specialized check kinds exist. Adding a new specialized check
  (e.g., NodeCertificateCheck) does not touch the aggregator. Existing
  CRDs are preserved; SKA-46 work is not invalidated. The AGENTS.md
  invariant is structurally enforced — ClusterHealth's reconciler will
  watch HealthCheck only, never HealthReport.
- **Negative**: users who want a check to surface in `ClusterHealth` must
  create two resources: one specialized (e.g. AddonCheck) and one
  HealthCheck wrapper. UX cost is real and we accept it for v0.1. A
  future operator-side default ("auto-create a HealthCheck wrapper for
  every AddonCheck") could mitigate this without a schema change.
- **Negative**: discriminator-typed unified CRD (Option B) is foreclosed
  without superseding this ADR. If we later decide one CRD is the right
  user-facing shape, deprecating the wrapper requires a migration.
- **Neutral**: the wrapper's `Status.Result` reuses `HealthReportResult`
  as the shared result enum, so per-check, per-report, and per-aggregate
  outcomes share vocabulary.

## Pros and Cons of the Options

### A. HealthCheck as a thin wrapper

- Good, because aggregation over a uniform Status is the simplest
  contract for `ClusterHealth` to honor.
- Good, because all existing and planned specialized CRDs survive
  unchanged.
- Bad, because it adds one resource per surfaced check.

### B. Discriminator-typed unified CRD

- Good, because users have one CRD to learn; no wrapper.
- Good, because the aggregator can read `HealthCheck.spec.type` for
  filtering as well as `Status.Result` for rolling up.
- Bad, because `AddonCheck` shipped in v0.1 with adapters — folding it in
  requires deprecation, migration, and adapter-RBAC rework. Not
  containable in SKA-289.
- Bad, because adding a new specialized type later means adding a new
  variant to the discriminator and updating CRD validation across the
  board.

### C. Delete HealthCheck and ClusterHealth

- Good, because the simplest possible schema (no wrapper, no aggregator).
- Bad, because `ClusterHealth` is the only existing cluster-wide rollup
  surface. Dropping it requires a replacement story, and the AGENTS.md
  invariant explicitly anchors on it.
- Bad, because it forecloses the cluster-wide rollup pattern that
  downstream consumers (dashboards, gating PRs against health, alerting)
  will eventually want.

## Links

- `api/v1alpha1/healthcheck_types.go` — `HealthCheckSpec`, `CheckTargetRef`,
  `HealthCheckStatus`
- `api/v1alpha1/clusterhealth_types.go` — `ClusterHealthSpec`,
  `ClusterHealthStatus`, `ClusterHealthChildSummary`
- `AGENTS.md` — ClusterHealth invariant
- ADR-0002 — HealthReport-as-CRD (the invariant this preserves)
- Commit: `86e8132` (real schemas)
- Related Linear: SKA-289 (this commit), SKA-309 (HealthCheckReconciler
  body), SKA-310 (ClusterHealthReconciler body), SKA-46 (AddonCheck),
  SKA-47/48/49/50 (other specialized check kinds)
