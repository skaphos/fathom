<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Concepts

This page is the mental model a platform team needs to use Fathom well. It is
deliberately lighter than the [architecture reference](../architecture.md),
which covers reconcilers, watches, and the adapter contract in full. Read this
to understand *what the resources are for*; read the architecture doc when you
need to know *how the operator implements them*.

## What Fathom does

Fathom is a Kubernetes operator (API group `fathom.skaphos.io`) that
continuously validates the **integrity of your platform** — the add-ons and
node-level material your workloads depend on — and rolls the results into a
single, machine-readable verdict.

It answers questions like:

- Is cert-manager healthy, and are my `Certificate`s actually issued and not
  about to expire?
- Is CoreDNS resolving names *from where my workloads run*, not just "is the
  Deployment up"?
- Are External Secrets syncing, or have they gone stale?
- Is my CNI (Cilium) control plane and per-node agent healthy?
- Are the on-disk certificates on my nodes (kubelet, etcd, API server) close to
  expiring?

Fathom is declarative: you describe the checks you want as custom resources,
and the operator keeps their status current. There is nothing to poll and no
external database to run.

### What Fathom is not

- **Not a metrics or time-series system.** History is bounded per check; Fathom
  is not where you store long-term trends. Pair it with Prometheus for that
  (see [Monitoring](monitoring.md)).
- **Not an installer.** Fathom validates add-ons; it does not deploy them.
- **Not an agent mesh.** Add-on checks run in-process inside the operator. Only
  two narrow cases run elsewhere: short-lived **probe pods** for active network
  checks, and the **node-agent DaemonSet** for on-disk certificate scanning.

## The resource kinds

Fathom defines a small set of custom resources. Two of them *drive work*; the
rest are projection and history layers.

| Kind | What you use it for | Drives work? |
| --- | --- | --- |
| **`AddonCheck`** | Declare a check against one add-on (cert-manager, CoreDNS, Cilium, Argo CD, and twelve more). Selects an adapter via `spec.addonType`. | Yes — via its adapter |
| **`NodeCertificateCheck`** *(newer kind — see note below)* | Declare an on-disk certificate-expiry scan across your nodes. | Yes — via the node-agent DaemonSet |
| **`HealthCheck`** | Wrap one check and mirror its status into a uniform shape so it can be aggregated. | No |
| **`ClusterHealth`** | Aggregate many `HealthCheck`s into one worst-case verdict. | No |
| **`HealthReport`** | Immutable, per-run history record. Created for you; you read it, you don't write it. | n/a |

A good way to hold it in your head:

- **`AddonCheck`** (and **`NodeCertificateCheck`**, on builds that include it)
  are the *sensors*.
- **`HealthCheck`** is an *adapter plug* that gives every sensor a uniform
  shape.
- **`ClusterHealth`** is the *dashboard light* — one verdict.
- **`HealthReport`** is the *flight recorder* — what happened, when.

> `NodeCertificateCheck` ships with the node-certificate feature; if your
> cluster's CRDs don't list it yet, see
> [Node certificate checks → Availability](node-certificate-checks.md#availability).

## Check families

An `AddonCheck` doesn't run one monolithic check — it enables a set of **check
families**, each a focused group of assertions, under `spec.policy`. For
cert-manager the families are `system_health`, `issuer_health`, and
`certificate_health`; for CoreDNS they are `system_health` and
`dns_resolution`; and so on.

Each family:

- is enabled or disabled independently (`policy.<family>.enabled`),
- takes string-keyed **thresholds** that tune names, counts, and limits
  (`policy.<family>.thresholds`), and
- contributes its outcome to the check's overall result.

Families are scoped to their adapter — `system_health` for cert-manager and
`system_health` for CoreDNS are different checks that happen to share a name.
The per-adapter family catalog is in [Add-on checks](addon-checks.md).

## Results and severity

Every outcome — per target, per family, per check, and the cluster roll-up —
uses one result vocabulary:

| Result | Meaning |
| --- | --- |
| `Pass` | Healthy. |
| `Skipped` | Not applicable here (e.g. a path/add-on that isn't present). Rolls up **green**. |
| `Warn` | Degraded but not down (e.g. a certificate inside its warn window, restarts above threshold). |
| `Unknown` | Could not determine (e.g. a check not yet reconciled). |
| `Fail` | A definite problem (e.g. a workload down, a certificate expired). |
| `Error` | The check itself could not run (e.g. the adapter errored). |

Aggregation is always **worst-case**, using this ordering:

```
Pass < Skipped < Warn < Unknown < Fail < Error
```

Two consequences worth internalizing:

- **`Skipped` is safe.** A check that doesn't apply on a given node or cluster
  reports `Skipped`, and `Skipped` rolls up green. You can declare a superset of
  checks across a fleet without lighting up red on the clusters a given check
  doesn't apply to.
- **`Unknown` sits below `Fail`.** A *known* failure is more actionable than an
  unknown, so when both appear in a set the aggregate reports `Fail`.

## How status flows

Status moves in one direction, from sensor to verdict:

```
        runs                       mirror                      aggregate
AddonCheck.status  ───────────▶  HealthCheck.status  ───────────▶  ClusterHealth.status
   (lastResult)                  (uniform shape)                  (worst-case over
        │                                                          its HealthChecks)
        └── creates ──▶ HealthReport (history; never read by the roll-up)
```

The roll-up is event-driven: when an `AddonCheck`'s status changes, the
`HealthCheck` wrapping it re-mirrors, and the `ClusterHealth` aggregating that
`HealthCheck` re-aggregates. You don't trigger any of this manually.

`NodeCertificateCheck` is slightly different: it reports its own status and
history directly (it is not wrapped by `HealthCheck` in this build), and it
*does* re-run on a timer. See its [guide](node-certificate-checks.md).

## Where work actually runs

Almost everything runs **in-process** inside the operator pod — the operator
talks to the Kubernetes API to inspect Deployments, CRDs, `Certificate`s,
`ExternalSecret`s, and the like. Two deliberate exceptions push work out of the
operator pod, because checking from the operator's vantage point would give the
wrong answer:

- **Probe pods** — active network checks (today, the CoreDNS and
  node-local-dns `dns_resolution` families and kube-state-metrics'
  `metrics_endpoint`) launch a single-shot, hardened pod *in the workload's
  namespace*, so DNS is resolved — and metrics are scraped — with the same
  topology a real workload would see.
- **The node-agent DaemonSet** — on-disk certificate scanning must happen *on
  each node*, so `NodeCertificateCheck` provisions a hardened, read-only agent
  per node.

Both are managed for you; you configure them through the check's spec and
operator-level image flags, never by hand.

## Next steps

- **[Getting started](getting-started.md)** — install and create your first
  check end-to-end.
- **[Add-on checks](addon-checks.md)** — the adapter catalog and threshold
  reference.
- **[Node certificate checks](node-certificate-checks.md)** — the on-disk
  certificate scanner.
- **[Architecture](../architecture.md)** — the implementation-level reference.
