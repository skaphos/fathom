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

## Contents

| Document | What it covers |
| --- | --- |
| [architecture.md](architecture.md) | The CRD model, the AddonCheck → HealthCheck → ClusterHealth aggregation chain, what each reconciler owns and watches, the adapter contract, the probe-pod model, and the runtime shape. |
| [code-map.md](code-map.md) | A module-by-module tour of the source tree for new contributors. |
| [reference/configuration.md](reference/configuration.md) | Every operator option (flag / env var / config file / default) and the precedence rules. |
| [reference/api.md](reference/api.md) | Generated CRD API reference for all four `fathom.skaphos.io/v1alpha1` kinds. |

## Architecture Decision Records

The architecturally significant decisions are recorded as ADRs:

- [ADR-0001 — In-process adapter contract](adr/0001-in-process-adapter-contract.md)
- [ADR-0002 — HealthReport as a first-class CRD](adr/0002-healthreport-as-first-class-crd.md)
- [ADR-0003 — Probe-pod model for active in-cluster checks](adr/0003-probe-pod-model.md)
- [ADR-0004 — HealthCheck as a thin wrapper](adr/0004-healthcheck-as-wrapper.md)

## Other repository docs

- [`../README.md`](../README.md) — install via Helm and per-adapter `AddonCheck`
  examples.
- [`../AGENTS.md`](../AGENTS.md) — contributor and AI-agent briefing (build,
  test, guardrails). `CLAUDE.md` is a symlink to it.

> The CRD API reference ([reference/api.md](reference/api.md)) is generated.
> Regenerate it with `go -C tools tool task docs:api-ref`; do not hand-edit it.
