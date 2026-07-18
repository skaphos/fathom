<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Fathom User Guides

Task-oriented guides for **platform teams** running Fathom: how to install it,
declare checks, and consume the results. For the design-level reference (CRD
model, reconcilers, adapter contract) see [../architecture.md](../architecture.md);
for the generated, field-level CRD reference see
[../reference/api.md](../reference/api.md).

## Start here

| Guide | Read it to… |
| --- | --- |
| [Getting started](getting-started.md) | Go from an empty cluster to one cluster-wide verdict in ~15 minutes. |
| [Concepts](concepts.md) | Build the mental model: the resource kinds, what drives work vs. aggregates, and result severity. |

## How to use Fathom

| Guide | Read it to… |
| --- | --- |
| [Add-on checks](addon-checks.md) | Configure `AddonCheck`s for the eight built-in adapters — families, thresholds, roll-up, troubleshooting. |
| [Node certificate checks](node-certificate-checks.md) | Scan on-disk X.509 certificates on every node and catch expiry before it causes an outage. |
| [Monitoring & alerting](monitoring.md) | Consume results via `kubectl`, Prometheus metrics, and tracing; wire alerts and deployment gates. |

## Automating Fathom

| Guide | Read it to… |
| --- | --- |
| [Agent operations](agent-operations.md) | Point an AI agent or automation at a cluster to install and operate Fathom safely — the prescriptive, approval-gated runbook. |

## Conventions used in these guides

- Examples place checks in the `fathom-system` namespace (where the operator is
  installed). Any namespace works; `ClusterHealth` is cluster-scoped and
  selects `HealthCheck` wrappers across all namespaces (narrow with
  `spec.namespaces` and/or `spec.selector`).
- Results everywhere use the severity ordering
  `Pass < Skipped < Warn < Unknown < Fail < Error`; aggregation is worst-case.
- Threshold values in `spec.policy.<family>.thresholds` are **strings**, even
  when numeric (`"3"`, `"30"`).
- When a resource is not doing what you expect, start with
  [Status and conditions](../reference/status-conditions.md): it maps each
  `Accepted`, `Paused`, `Ready`, and `AgentReady` reason to the next operator
  action.
