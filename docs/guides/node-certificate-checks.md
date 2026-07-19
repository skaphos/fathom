<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
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

- A **directory** is scanned recursively, to a bounded depth, for `*.crt`,
  `*.pem`, and `*.cert` files.
- A **certificate file** (`.crt` / `.pem` / `.cert`) is read directly.
- A **kubeconfig** (`.conf` / `.kubeconfig`) is parsed and its embedded
  client/CA certificates are extracted.
- A path the non-root agent cannot read is `Skipped`, never `Fail` or `Error`.
- A path that **doesn't exist on a node is omitted from that node's report.**
  That's why listing a superset across distributions is safe — each node only
  reports on the paths it actually has. For explicitly configured directories,
  Kubernetes may create an empty hostPath on the node; narrow `paths` on
  immutable-OS distributions where that side effect matters.

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

**Paths are restricted to an operator-approved allowlist.** Because the agent
mounts host directories, an unconstrained `spec.paths` would let anyone who can
create a `NodeCertificateCheck` turn the privileged agent into a confused deputy
that reads arbitrary host files. Every entry must therefore be a traversal-free
absolute path (no `..`, never the host root `/`) under one of these prefixes:

| Allowed prefix | Covers |
| --- | --- |
| `/etc/kubernetes` | kubeadm PKI (`pki/`) and the embedded-cert kubeconfigs (`*.conf`) |
| `/var/lib/kubelet` | kubelet client/serving certificates |
| `/etc/etcd` | standalone etcd PKI |
| `/var/lib/etcd` | etcd data-dir PKI on some distributions |
| `/var/lib/rancher` | k3s / RKE2 server TLS material |

A path outside the allowlist is **rejected at admission**. The whole default
scan set already lives under these prefixes, so leaving `paths` empty always
validates.

## Thresholds

| Field | Default | Meaning |
| --- | --- | --- |
| `warnDays` | `30` | A certificate within this many days of expiry is `Warn`. Must be ≥ `criticalDays`. |
| `criticalDays` | `7` | A certificate within this many days of expiry is `Fail`. An already-expired certificate is always `Fail`, regardless of this value. |

## Choosing which nodes run the agent

```yaml
spec:
  includeControlPlaneNodes: true   # opt in to scanning control-plane certs
  nodeSelector:
    node-role.kubernetes.io/control-plane: ""
  tolerations:
    - key: dedicated
      operator: Equal
      value: certs
      effect: NoSchedule
```

- **`nodeSelector`** restricts the DaemonSet to matching nodes. Empty targets
  every schedulable node.
- **`includeControlPlaneNodes`** (default `false`) opts the agent into scheduling
  on **control-plane nodes** by adding tolerations for the standard
  `node-role.kubernetes.io/control-plane` and legacy `.../master` taints. The
  kubeadm apiserver, etcd, and front-proxy certificates live on control-plane
  nodes, so **set this to `true` to scan them**.
- **`tolerations`** let the agent schedule onto nodes with *other* taints. They
  are applied verbatim (empty means none) and are independent of
  `includeControlPlaneNodes`.

> **Behavior change (v0.5.0).** Earlier builds tolerated control-plane taints by
> default, so a `NodeCertificateCheck` silently placed the privileged agent on
> control-plane nodes. Scheduling there — and mounting control-plane host paths —
> is now an explicit, auditable opt-in via `includeControlPlaneNodes: true`. If
> you relied on the old default to scan kubeadm control-plane certificates, add
> that field to your spec.

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
  and reporting. See
  [Status and conditions](../reference/status-conditions.md#nodecertificatecheck)
  for every `AgentReady` / `Ready` reason and the next action.

The detail (which certificate on which node, and its days-to-expiry) lives in
the `HealthReport`:

```sh
kubectl -n fathom-system get healthreport \
  -l 'fathom.skaphos.io/source-kind=NodeCertificateCheck,fathom.skaphos.io/source-name=node-certificates'
```

> `NodeCertificateCheck` reports its own status and history directly. In this
> build it is **not** wrapped by `HealthCheck`/`ClusterHealth`, so read it
> directly (or via its Prometheus gauge) rather than expecting it in a
> `ClusterHealth` roll-up.

## The node-agent image

The node-agent is a **dedicated, purpose-built image** — never the operator or
probe image. The operator passes it to the DaemonSet via `--node-agent-image`
(default `ghcr.io/skaphos/fathom-node-agent:v0.4.0`).

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
- **Reports are bound to the writing node.** Because RBAC cannot scope a
  `create`/`update` to a single object name, the shared node-agent
  ServiceAccount can technically write any report ConfigMap in the namespace. To
  stop one compromised node from forging or suppressing another node's verdict,
  each report carries a `fathom.skaphos.io/node-name` annotation, and the
  operator provisions a cluster-scoped **`ValidatingAdmissionPolicy`**
  (`fathom-node-report-authenticity`) that requires this annotation to equal the
  writing agent's ServiceAccount-token node claim
  (`authentication.kubernetes.io/node-name`). A node-agent can therefore only
  publish a report attributed to *its own* node. The operator additionally
  re-checks the annotation against the report payload at collection time, so a
  mismatched report is dropped even on a cluster where the policy is not
  enforced. Requires ServiceAccount-token node info (GA in Kubernetes 1.33) and
  the `ValidatingAdmissionPolicy` feature (GA 1.30).
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
