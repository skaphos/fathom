<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Node Health Checks

A `NodeHealthCheck` asserts node-local health signals on every selected node —
filesystem headroom, the node's own conditions, kubelet health, and
container-runtime liveness — and reports one verdict per node plus a single
folded verdict for the check. It reuses the hardened **node-agent DaemonSet**
that `NodeCertificateCheck` established, running it in health mode, and the
operator grades the node conditions itself.

Use this guide to author the check and understand what each check type costs.
For every field and validation rule, see the generated
[API reference](../reference/api.md).

> ## Availability
>
> `NodeHealthCheck` ships from v0.6.0. If
> `kubectl get crd | grep nodehealthchecks` returns nothing, your installed
> build predates it — upgrade the operator (and apply the new CRD; Helm does not
> upgrade CRDs automatically) before using this guide.

## What it does

For each node it targets, the agent:

1. measures free bytes and inodes on the filesystem behind each configured
   **path** (over a read-only `hostPath` mount),
2. optionally probes the kubelet's localhost `/healthz` and dials the CRI
   socket, and
3. publishes a per-node result that the operator reads.

The operator merges each node's report with the node's **status conditions**
(read from the Node object — the agent never carries a Node grant), rolls every
node into a single `HealthReport` (one entry per `(node, check)`, worst-case
aggregate), and mirrors the aggregate plus a per-node result list into the
check's `status`. Each agent also exports per-check gauges (see
[Monitoring](monitoring.md#node-health-metrics)).

## A minimal check

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: NodeHealthCheck
metadata:
  name: node-health
  namespace: fathom-system
spec:
  interval: 5m
  timeout: 30s
  historyLimit: 10
  checks:
    - type: DiskHeadroom
      path: /var/lib/kubelet
      warnPercentFree: 20
      criticalPercentFree: 10
    - type: InodeHeadroom
      path: /var/lib/kubelet
    - type: NodeCondition
```

```sh
kubectl apply -f node-health.yaml
kubectl -n fathom-system get nodehealthcheck node-health
```

```
NAME          RESULT   REPORTING   DESIRED   LAST RUN   AGE
node-health   Pass     3           3         42s        2m
```

- **RESULT** — worst-case across every node in scope.
- **REPORTING / DESIRED** — how many node-agents have published a fresh result
  vs. how many nodes the DaemonSet targets.
- `status.summary` says how many nodes passed and, when some did not, names
  the worst node and check: `2 of 3 node(s) passed; worst: node-b DiskHeadroom
  /var/lib/kubelet: 8.2% of bytes free (at or below criticalPercentFree 10)`.
- `status.nodeResults` lists every node's verdict and message (capped at 100
  entries; the verdict is folded across every node before the cap applies).

`fathomctl describe nhc node-health` renders the same, with a per-node table.

## The check types

`spec.checks` is a list of typed items. Which fields are legal depends on the
type, and the API server rejects a field on a type that does not use it, so a
misapplied threshold is a write-time error rather than a silently ignored one.

| Type | Measures | Fields | Verdict |
| --- | --- | --- | --- |
| `DiskHeadroom` | Percentage of free **bytes** on the filesystem holding `path` (`statfs`; blocks available to an unprivileged caller, so ext4's root reservation is not counted). | `path` (required), `warnPercentFree`, `criticalPercentFree` | `<= criticalPercentFree` → Fail, `<= warnPercentFree` → Warn, else Pass. A path absent on a node is Skipped. |
| `InodeHeadroom` | Percentage of free **inodes** on the filesystem holding `path`. | same as `DiskHeadroom` | same |
| `NodeCondition` | The node's `status.conditions`, graded by the operator. `Ready` must be `True`; every other listed condition must be `False` — the healthy value for every pressure and unavailability condition Kubernetes and the cloud providers define. | `conditions` (defaults to `Ready`, `MemoryPressure`, `DiskPressure`, `PIDPressure`) | A condition with its expected value → Pass; otherwise Fail, with the condition's reason and message in the summary. A condition the node does not report is Skipped. |
| `KubeletHealthz` | `GET http://127.0.0.1:10248/healthz` from the node. | none | 2xx → Pass; any other response, or an unreachable kubelet, → Fail. |
| `ContainerRuntime` | A connection to the CRI socket (dial + close; the agent speaks no CRI). | `socketPath` (defaults to `/run/containerd/containerd.sock`) | Accepts a connection → Pass; refused or missing → Fail. |

Thresholds default to **20 / 10** when unset. Zero is legal (`warnPercentFree:
0` means "never warn"), which is why the defaults are applied at runtime rather
than by the schema — a schema default on one type's field would break the
type rules for every other item.

Items must be unique by `(type, path)`, and the list is capped at 16.

### Which path to measure

On most nodes `/var/lib/kubelet` sits on the root filesystem, so a headroom
check there doubles as a root-filesystem check. Nodes with a dedicated
container or log volume should add `/var/lib/containerd` (or `/var/lib/docker`)
and `/var/log`. The host root `/` is never allowed: the operator mounts the
measured path read-only into the agent, and mounting `/` is the opposite of
least privilege.

Paths are restricted to an operator-approved allowlist — `/var/lib/kubelet`,
`/var/lib/containerd`, `/var/lib/docker`, `/var/lib/etcd`, `/var/log`,
`/var/lib/rancher`, `/etc/kubernetes`, `/run/containerd` — mirrored in the CRD
schema and re-checked by the operator.

## What each check costs

The five types are **not equal in what they cost the agent**. The operator
grants each privilege only when an item of that type is present, so a spec
with only headroom checks keeps exactly the hardened profile the certificate
agent runs with: non-root (uid 65532), every capability dropped, read-only root
filesystem, no host network, read-only `hostPath` mounts for the measured
paths only.

| Type | Privilege | Why |
| --- | --- | --- |
| `DiskHeadroom`, `InodeHeadroom` | none beyond a read-only `hostPath` of the measured directory | `statfs` needs a path on the filesystem, nothing more. |
| `NodeCondition` | a cluster-scoped `get` on **nodes** for the **operator** (not the agent) | The conditions live only on the Node object. The operator reads one node at a time, by name, only for the nodes agent pods landed on — never a `list` or `watch` — so it starts no Node informer and can enumerate nothing through this grant. See [Operator RBAC](../reference/operator-rbac.md). |
| `KubeletHealthz` | `hostNetwork: true` on the agent pod | The kubelet's health endpoint binds to `127.0.0.1`. **A host-network pod is not isolated by the per-check NetworkPolicy**, and its metrics port binds on the node itself (a per-check port in 30000–32767, derived from the check's name), so the agent's plaintext gauges are reachable from the node's network. |
| `ContainerRuntime` | the CRI socket mounted (`hostPath` type `Socket`) and the agent running **as root** (`runAsUser: 0`), still with every capability dropped and a read-only root filesystem | The socket is root-owned on every mainstream runtime. The agent only dials and closes — it carries no CRI client — but a compromised agent process would hold the socket. |

The check reports which of these are in effect on its own object: the
`AgentPrivileged` condition is `False/Hardened` for a headroom-only spec and
`True/HostNetwork`, `True/RunAsRoot`, or `True/HostNetworkAndRoot` otherwise,
with a message naming the socket and host port. A namespace enforcing the
`restricted` Pod Security Standard rejects host-network and root pods: the
DaemonSet is created but its pods are not admitted, which surfaces as
`AgentReady=False` and `CoverageComplete=False`.

Opt into the two privileged types deliberately, in namespaces you control:

```yaml
spec:
  checks:
    - type: DiskHeadroom
      path: /var/lib/kubelet
    - type: KubeletHealthz        # hostNetwork
    - type: ContainerRuntime      # root + socket
      socketPath: /run/crio/crio.sock
```

## Node scope and control-plane nodes

`spec.nodeSelector` and `spec.tolerations` scope the DaemonSet exactly as they
would on any other one. Control-plane nodes are an explicit opt-in:

```yaml
spec:
  includeControlPlaneNodes: true
```

adds tolerations for the standard control-plane and legacy master taints. It
defaults to false — with the two privileged types available, landing the agent
on a control-plane node is a decision the author makes, not a default.

## Cadence and freshness

`spec.interval` (default `5m`, floor `10s`) is how often the operator refreshes
the rolled-up `HealthReport` and the check's liveness. The **agent** re-evaluates
at `min(interval, 5m)`, and a report counts as fresh for that agent cadence
plus `spec.timeout` — never for the full interval. Headroom, kubelet, and
runtime liveness change on the order of minutes, so a long interval must not
accept a measurement that old: a `24h` interval still detects a disk filling
up within minutes (the fold transitions immediately, and a new `HealthReport`
is written), and simply refreshes on its own cadence when nothing changed.

`spec.timeout` (default `30s`, floor `1s`, at most `interval`) bounds one agent
pass, including the kubelet and socket probes.

## Coverage, freezing, and what the conditions mean

The operator rolls up only when **every node in scope** has a fresh report —
per node identity, not by count, so a node that left cannot stand in for one
that joined. An incomplete window (a rollout, a node joining, an agent restart,
a report ageing out) **freezes** `lastResult`, `lastReportName`,
`lastRunTime`, and `nodeResults` at their last complete values rather than
clearing them; the `CoverageComplete` condition carries the gap and names the
missing nodes. See [Status conditions](../reference/status-conditions.md#nodehealthcheck)
for the full table.

Report authenticity is enforced the same way as for `NodeCertificateCheck`:
the shared report-authenticity `ValidatingAdmissionPolicy` binds each report to
the writing agent's node identity, and the `ReportsAuthentic` condition (plus a
Warning event) surfaces any report that failed its bindings.

## Projecting into cluster health

Wrap the check in a `HealthCheck` and select it from a `ClusterHealth` exactly
as for any other executable kind:

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: node-health
  namespace: fathom-system
  labels:
    tier: platform
spec:
  checkRef:
    kind: NodeHealthCheck
    name: node-health
```

The `HealthCheck` mirrors `status.summary`, so the cluster-level view names
the worst node without opening the `HealthReport`.

## On-demand runs

`fathomctl run nhc node-health --wait` (or the `fathom.skaphos.io/run-now`
annotation) restarts one agent pod per node and records the token in
`status.lastRunTrigger` once every node in scope has reported with it. Budget
one pod restart per node; see
[On-demand runs](../reference/status-conditions.md#on-demand-runs).

## Troubleshooting

```sh
# Agent pods, per node
kubectl -n fathom-system get pods -l fathom.skaphos.io/source-kind=NodeHealthCheck,fathom.skaphos.io/source-name=node-health -o wide

# Per-node report ConfigMaps
kubectl -n fathom-system get configmap -l fathom.skaphos.io/managed-by=fathom,fathom.skaphos.io/source-kind=NodeHealthCheck,fathom.skaphos.io/source-name=node-health

# History
fathomctl reports nhc node-health
```

- **`AgentReady=False` and no agent pods** on a privileged spec: check the
  namespace's Pod Security labels — `restricted` rejects host-network and root
  pods. Run the check in a namespace that allows them, or drop the privileged
  types.
- **`ContainerRuntime` pods stuck `ContainerCreating`**: the socket is mounted
  with `hostPath` type `Socket`, so a wrong `socketPath` prevents the pod from
  starting at all (rather than reporting a misleading verdict). Set
  `socketPath` for your runtime — `/run/crio/crio.sock` for CRI-O.
- **`NodeCondition` items grade `Error: cannot read node`**: the operator's
  `nodes` `get` grant was removed (a restricted install). Restore it or drop
  the item; the rest of the check is unaffected.
- **Two host-network checks on one node, second agent in `CrashLoopBackOff`**:
  a metrics-port hash collision. Rename one check.
