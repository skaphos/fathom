<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# 2. HealthReport as a first-class custom resource

- **Status**: accepted
- **Date**: 2026-05-10
- **Deciders**: Shawn Stratton

## Context and Problem Statement

Each `AddonCheck` adapter run produces a structured `Result` containing one
or more per-target check outcomes. Two consumers need access to that data:

- A "current state" consumer (operators, dashboards, `kubectl get`) needs
  the most recent outcome per check.
- A "history" consumer (audit, post-incident review, longitudinal trend
  analysis) needs prior runs, with payloads, attached to a specific
  `AddonCheck`.

Status conditions on `AddonCheck` itself can serve the first consumer but
not the second — they overwrite on each reconcile. We need a persistence
shape that handles both without introducing an external time-series store
as a v0.1 dependency.

## Decision Drivers

- v0.1 must work in clusters that have no Prometheus, no Loki, no
  external storage of any kind. Health visibility cannot depend on
  external infra.
- The consumers above are different audiences (live SREs vs. post-incident
  reviewers); coupling them produces a worse contract for both.
- AGENTS.md states a load-bearing invariant: `ClusterHealth` is derived
  from `HealthCheck.status` only, never from `HealthReport` history. The
  decision must not silently violate this.
- etcd is finite. Anything we persist must have a retention story, even if
  the retention controller is deferred (SKA-288).

## Considered Options

- **A. Status conditions on the source check only.** No history; consumers
  that want trends must scrape externally.
- **B. HealthReport as a first-class CRD owned by the source check.** Each
  run creates a new `HealthReport` resource with `ownerReferences`
  pointing back to the `AddonCheck`. The source check's status carries a
  `LastReportName` pointer.
- **C. External time-series store (Prometheus, OpenTelemetry).** Adapter
  Results emitted as metrics or OTel spans; HealthReports never touch
  etcd.
- **D. Kubernetes Event objects.** Each adapter Result becomes one or more
  Events.

## Decision Outcome

Chosen option: **B. HealthReport as a first-class CRD owned by the source
check**, because it gives both consumers what they need with no external
dependency, supports per-tenant RBAC out of the box (Roles on
`healthreports`), and propagates cleanup automatically via owner
references.

The aggregate `Result` lives on the `AddonCheck.status` (current state for
ClusterHealth aggregation per ADR-0004). The full per-check payload lives
on the new `HealthReport` resource. The two contracts are deliberately
separate: ClusterHealth must never read history.

### Consequences

- **Positive**: works in air-gapped and storage-free clusters; queryable
  via standard `kubectl`; per-namespace RBAC isolates teams' history;
  garbage-collected with the source `AddonCheck`. The current-state
  invariant for ClusterHealth is structural, not advisory — the wrong
  CRD literally has the wrong shape for trend queries.
- **Negative**: every adapter run writes to etcd. Without a retention
  policy (SKA-288) a long-lived AddonCheck on a tight interval will
  unboundedly grow `HealthReport` count. Etcd is not designed for
  high-cardinality history; users wanting weeks of trend data should
  still scrape to Prometheus or similar.
- **Neutral**: the schema embeds adapter identity (`adapterName`,
  `adapterVersion`, `contractVersion`) so historical reports remain
  interpretable across adapter upgrades.

## Pros and Cons of the Options

### A. Status conditions on the source check only

- Good, because zero new schema; no etcd cost beyond what already exists.
- Bad, because no history. Post-incident review of "when did this start
  failing" is impossible without an external scraper.

### B. HealthReport as a first-class CRD

- Good, because both current state and history are first-class and
  observable with cluster-native tools.
- Good, because owner references give us cleanup-on-AddonCheck-delete for
  free.
- Bad, because etcd consumption grows with run cadence and history depth
  until a retention policy lands.

### C. External time-series store

- Good, because purpose-built for high-cardinality history.
- Bad, because it makes Prometheus (or equivalent) a hard dependency for
  v0.1, which we explicitly do not want.

### D. Kubernetes Events

- Good, because Events are designed for "something happened" and have
  built-in TTL.
- Bad, because Event payload is a single message string. Embedding the
  per-check structured payload would either bloat Event messages or lose
  the structure.

## Links

- `api/v1alpha1/healthreport_types.go` — `HealthReport`, `HealthReportSpec`, `HealthReportCheck`
- `internal/controller/addoncheck_controller.go:188` — `r.Create(ctx, report)`
  (HealthReport creation per run)
- `AGENTS.md` — ClusterHealth invariant
- ADR-0004 — HealthCheck-as-wrapper (relies on ClusterHealth invariant)
- Commit: `5b509db` (persist adapter health reports)
- Related Linear: SKA-288 (retention path), SKA-46
