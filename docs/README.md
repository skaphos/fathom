<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Fathom Documentation

Fathom is a Kubernetes operator (API group `fathom.skaphos.io`) that validates
the integrity of platform add-ons — cert-manager, CoreDNS, External Secrets
Operator, and others reachable through an adapter. It reconciles `AddonCheck`
resources, runs adapter-defined checks against the cluster, records each run as
an immutable `HealthReport`, mirrors per-check status through `HealthCheck`, and
rolls everything up into a single cluster-wide verdict on `ClusterHealth`.

## Guides — for platform teams

Task-oriented guides for installing and using Fathom live in
[guides/](guides/README.md):

| Guide | What it covers |
| --- | --- |
| [Getting started](guides/getting-started.md) | Install the operator and go from an empty cluster to one cluster-wide verdict in ~15 minutes. |
| [Concepts](guides/concepts.md) | The platform-team mental model: the resource kinds, what drives work vs. aggregates, and result severity. |
| [Add-on checks](guides/addon-checks.md) | Configure `AddonCheck`s for the eight built-in adapters — families, thresholds, roll-up, troubleshooting. |
| [Node certificate checks](guides/node-certificate-checks.md) | Scan on-disk X.509 certificates on every node and catch expiry before an outage. *(Newer kind — included in recent builds; not in older ones.)* |
| [Monitoring & alerting](guides/monitoring.md) | Consume results via `kubectl`, Prometheus metrics, and tracing; wire alerts and gates. |

## Reference & internals

| Document | What it covers |
| --- | --- |
| [architecture.md](architecture.md) | The CRD model, the AddonCheck → HealthCheck → ClusterHealth aggregation chain, what each reconciler owns and watches, the adapter contract, the probe-pod model, and the runtime shape. |
| [code-map.md](code-map.md) | A module-by-module tour of the source tree for new contributors. |
| [authoring-adapters.md](authoring-adapters.md) | How to add a new add-on adapter — the declarative `AddonDefinition` path, the Go escape hatch, RBAC declaration, versioning, and a ship-it checklist. |
| [reference/configuration.md](reference/configuration.md) | Every operator option (flag / env var / config file / default) and the precedence rules. |
| [reference/api.md](reference/api.md) | Generated CRD API reference for the `fathom.skaphos.io/v1alpha1` kinds. |
| [reference/api-versioning.md](reference/api-versioning.md) | CRD API versioning policy — the maturity ladder (alpha/beta/GA), compatible vs. breaking schema changes, storage/served versions, and deprecation windows. Mirror of the org standard. |

## Architecture Decision Records

The architecturally significant decisions are recorded as ADRs:

- [ADR-0001 — In-process adapter contract](adr/0001-in-process-adapter-contract.md)
- [ADR-0002 — HealthReport as a first-class CRD](adr/0002-healthreport-as-first-class-crd.md)
- [ADR-0003 — Probe-pod model for active in-cluster checks](adr/0003-probe-pod-model.md)
- [ADR-0004 — HealthCheck as a thin wrapper](adr/0004-healthcheck-as-wrapper.md)

## Design & planning

Living planning docs for in-flight epics (draft; not stable references):

- [design/addon-adapters-implementation-plan.md](design/addon-adapters-implementation-plan.md)
  — implementation plan for the Addon Adapters epic (Waves 2–4): a declarative-first
  adapter engine with a Go escape hatch, periodic + on-demand execution, explicit
  absence semantics, per-addon least-privilege clients, version detection, and the
  reframed enabler/sequencing plan.

## Other repository docs

- [`../README.md`](../README.md) — install via Helm and per-adapter `AddonCheck`
  examples.
- [`../AGENTS.md`](../AGENTS.md) — contributor and AI-agent briefing (build,
  test, guardrails). `CLAUDE.md` is a symlink to it.

> The CRD API reference ([reference/api.md](reference/api.md)) is generated.
> Regenerate it with `go -C tools tool task docs:api-ref`; do not hand-edit it.
