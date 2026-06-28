<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Node Certificate Checks

A `NodeCertificateCheck` scans on-disk X.509 certificates on every selected
node and reports their time-to-expiry — so you find out a kubelet, etcd, or API
server certificate is about to expire *before* it takes the cluster down,
instead of during the outage.

This is the one check that has to run **on each node**: the certificates it
cares about live on node filesystems, not in the Kubernetes API. Fathom handles
that for you with a hardened, read-only **node-agent DaemonSet** that the
operator manages.

> ## Availability
>
> `NodeCertificateCheck` ships with the node-certificate feature. If
> `kubectl get crd | grep nodecertificatechecks` returns nothing, your
> installed build predates it — upgrade the operator (and apply the new CRD;
> Helm does not upgrade CRDs automatically) before using this guide.

## What it does

For each node it targets, the agent:

1. scans the configured certificate **paths** over read-only `hostPath` mounts,
2. computes each certificate's days-to-expiry and classifies it against your
   thresholds, and
3. publishes a per-node result that the operator reads.

The operator then rolls all the per-node results into a single `HealthReport`
(one entry per `(node, certificate)`, worst-case aggregate) and mirrors the
aggregate into the check's `status`. Each agent also exports a Prometheus gauge,
`fathom_node_certificate_expiry_days`, for alerting (see
[Monitoring](monitoring.md)).

## A minimal check

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: NodeCertificateCheck
metadata:
  name: node-certificates
  namespace: fathom-system
spec:
  warnDays: 30
  criticalDays: 7
  interval: 1h
  timeout: 30s
  historyLimit: 10
  # paths omitted -> use the built-in default set (see "What gets scanned")
```

```sh
kubectl apply -f node-certificates.yaml
kubectl -n fathom-system get nodecertificatecheck node-certificates
```

The printed columns tell you the verdict and coverage at a glance:

```
NAME                RESULT   REPORTING   DESIRED   AGE
node-certificates   Pass     3           3         2m
```

- **RESULT** — worst-case across every reporting node.
- **REPORTING / DESIRED** — how many node-agents have published a result vs.
  how many nodes the DaemonSet targets. A gap usually means an agent hasn't
  scheduled or reported yet.

## What gets scanned

When `spec.paths` is empty, the agent uses a **distribution-agnostic default
set** covering the common control-plane and node certificate locations across
kubeadm, k3s, RKE2, standalone etcd, and the kubelet:

```
/etc/kubernetes/pki              # apiserver, kubelet-client, front-proxy, ca, etcd/
/etc/kubernetes/admin.conf       # kubeconfigs with embedded client certs
/etc/kubernetes/controller-manager.conf
/etc/kubernetes/scheduler.conf
/etc/kubernetes/super-admin.conf
/etc/kubernetes/kubelet.conf
/var/lib/kubelet/pki             # kubelet client/serving certs
/etc/etcd/pki                    # standalone etcd
/var/lib/rancher/k3s/server/tls  # k3s server TLS
/var/lib/rancher/rke2/server/tls # RKE2 server TLS
```

How paths are interpreted:

- A **directory** is scanned **non-recursively** for `*.crt`, `*.pem`, and
  `*.cert` files.
- A **certificate file** (`.crt` / `.pem` / `.cert`) is read directly.
- A **kubeconfig** (`.conf` / `.kubeconfig`) is parsed and its embedded
  client/CA certificates are extracted.
- A path that **doesn't exist on a node is `Skipped`, never `Fail`.** That's
  why listing a superset across distributions is safe — each node only reports
  on the paths it actually has.

To pin an explicit set instead of the defaults:

```yaml
spec:
  paths:
    - /etc/kubernetes/pki
    - /etc/kubernetes/admin.conf
    - /var/lib/kubelet/pki
```

Up to 64 paths are allowed. The operator mounts only the **minimal set of host
directories** needed to read your paths (collapsing descendants and refusing to
mount the host root), so a narrower `paths` list means a smaller hostPath
surface on the DaemonSet.

## Thresholds

| Field | Default | Meaning |
| --- | --- | --- |
| `warnDays` | `30` | A certificate within this many days of expiry is `Warn`. Must be ≥ `criticalDays`. |
| `criticalDays` | `7` | A certificate within this many days of expiry is `Fail`. An already-expired certificate is always `Fail`, regardless of this value. |

## Choosing which nodes run the agent

```yaml
spec:
  nodeSelector:
    node-role.kubernetes.io/control-plane: ""
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule
```

- **`nodeSelector`** restricts the DaemonSet to matching nodes. Empty targets
  every node.
- **`tolerations`** let the agent schedule onto tainted nodes. When you omit
  `tolerations` entirely, Fathom applies a **default toleration set** so the
  agent also lands on control-plane nodes — which is exactly where the kubeadm
  certificates live. Set an explicit **empty list** (`tolerations: []`) to apply
  *no* tolerations.

For most clusters the control-plane certificates are the ones that cause
outages, so the default behavior (cover every node, including control-plane) is
the right starting point.

## Cadence, pausing, and history

- **`interval`** (default `1h`) — unlike `AddonCheck`, `NodeCertificateCheck`
  **honors** `interval`: each agent re-scans and the operator refreshes the
  rolled-up report on this cadence, in addition to reacting to fresh per-node
  reports.
- **`timeout`** (default `30s`) — bounds a single scan-and-publish pass.
- **`paused`** — when set, the operator **removes the agent DaemonSet** and
  preserves the last status snapshot. Unset it to resume.
- **`historyLimit`** (default `10`, min `1`) — `HealthReport`s retained for this
  check; older ones are pruned.

## Reading results

The check's `status` carries the summary:

```sh
kubectl -n fathom-system describe nodecertificatecheck node-certificates
```

- `lastResult` — aggregate result across all reporting nodes.
- `reportingNodes` / `desiredNodes` — coverage.
- `lastReportName` — the `HealthReport` for the most recent roll-up.
- `conditions` — whether the spec was accepted and the DaemonSet is rolled out
  and reporting.

The detail (which certificate on which node, and its days-to-expiry) lives in
the `HealthReport`:

```sh
kubectl -n fathom-system get healthreport \
  -l fathom.skaphos.io/source-kind=NodeCertificateCheck \
  -l fathom.skaphos.io/source-name=node-certificates
```

> `NodeCertificateCheck` reports its own status and history directly. In this
> build it is **not** wrapped by `HealthCheck`/`ClusterHealth`, so read it
> directly (or via its Prometheus gauge) rather than expecting it in a
> `ClusterHealth` roll-up.

## The node-agent image

The node-agent is a **dedicated, purpose-built image** — never the operator or
probe image. The operator passes it to the DaemonSet via `--node-agent-image`
(default `ghcr.io/skaphos/fathom-node-agent:v0.0.2`).

- **Helm:** set `nodeAgent.image.repository` / `nodeAgent.image.tag` (tag
  defaults to the chart's appVersion). Teams mirroring images privately set
  this once.
- The agent is **not** templated as a standalone workload in the chart — the
  controller creates the DaemonSet, a per-check `ServiceAccount` and
  `RoleBinding`, and the `fathom-node-agent-role` `ClusterRole` at runtime, all
  owner-referenced for cascading cleanup.

## Security posture

The agent is built for least privilege:

- **Read-only host access.** Certificate directories are mounted read-only, and
  only the minimal set of directories needed for your `paths` is mounted.
- **Writes exactly one object** — its own per-node report ConfigMap. It needs no
  read access to the `NodeCertificateCheck` API; all scan configuration is
  passed in by the operator.
- **Hardened and dedicated.** It runs from its own image and serves only a
  Prometheus metrics endpoint and a health check.

When you `delete` a `NodeCertificateCheck`, its DaemonSet, ServiceAccount,
RoleBinding, and reports are garbage-collected via their owner references.

## Reference

- [Monitoring & alerting](monitoring.md) — the `fathom_node_certificate_expiry_days`
  gauge and example alert rules.
- [Configuration reference](../reference/configuration.md) — operator flags.
- [API reference](../reference/api.md) — generated, field-level CRD reference.
- [Architecture → NodeCertificateCheckReconciler](../architecture.md) — the
  reconciler and DaemonSet lifecycle in depth.
