<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Network policies

Fathom runs three kinds of workload: the operator Deployment, ephemeral probe
pods, and the node-agent DaemonSet. Each has a deliberate — and different —
NetworkPolicy posture, described here (#153).

> **Enforcement requires a NetworkPolicy-capable CNI** (Calico, Cilium,
> Antrea, …). On clusters whose CNI ignores NetworkPolicy (e.g. plain kindnet,
> flannel without a policy engine) these objects are inert: nothing breaks,
> and nothing is protected.

## Operator (static, opt-in)

`config/network-policy/allow-metrics-traffic.yaml` restricts ingress to the
operator's metrics port (8443) to pods in namespaces labeled
`metrics: enabled`. It ships commented out in
`config/default/kustomization.yaml` (uncomment `- ../network-policy` to
enable).

It is opt-in rather than default because the operator's metrics endpoint is
already **TLS-served and authenticated**: every scrape is authorized in-process
via TokenReview/SubjectAccessReview (`--metrics-secure`, the default), so the
NetworkPolicy is defense-in-depth, not the primary control.

## Node-agent DaemonSet (runtime-managed, always on)

The node-agent's metrics endpoint is different: it serves **plaintext,
unauthenticated** Prometheus gauges (`fathom_node_certificate_expiry_days`) on
container port 8080 of a pod on every selected node. Without a policy, any pod
in the cluster can enumerate the node cert inventory — paths and days-to-expiry
for control-plane certificates. Adding TokenReview auth to the agent would
force API-server-facing credentials into a per-node workload that today needs
none for serving, so the guard is a NetworkPolicy instead.

Because the DaemonSet itself is created at runtime in the check's namespace,
the policy cannot ship statically. The `NodeCertificateCheckReconciler` creates
an owner-referenced NetworkPolicy (`<check>-node-agent`) alongside each
DaemonSet:

- **Pod selector**: exactly the agent pods (the DaemonSet's own selector) —
  nothing else in the namespace is isolated by it.
- **Ingress**: only TCP 8080 (metrics), and only from namespaces labeled
  `metrics: enabled` — the same label contract as the operator policy. **Label
  your monitoring namespace** (`kubectl label namespace <ns> metrics=enabled`)
  or, on an enforcing CNI, scrapes of the agent will be dropped.
- **Egress**: only TCP 443 and 6443 — the API server, the one thing the agent
  talks to (it publishes its report ConfigMap; it needs no DNS because
  `KUBERNETES_SERVICE_HOST` is an IP). Both ports are allowed because service
  traffic to the `kubernetes` ClusterIP (443) is policed post-DNAT against the
  endpoint port (6443) by most CNIs. If your API server listens on a
  nonstandard port (e.g. some distributions use 16443), add your own
  NetworkPolicy allowing agent egress to it — policies are additive.
- **Lifecycle**: garbage-collected with the check; like the agent's RBAC it is
  left in place while the check is paused.

## Probe pods (deliberately no Fathom-shipped policy)

Probe pods (ADR-0003) exist to answer one question: *does this network path
work from a workload's point of view?* They are launched into the **target
addon's namespace** precisely so they are subject to the same NetworkPolicies
as the workloads there. A Fathom-shipped allowance would corrupt that signal:
NetworkPolicies are additive, so granting a probe its own DNS or HTTP egress
would let the probe pass in a namespace whose real workloads are
egress-denied — a false healthy verdict about a broken data path.

The probe's exposure is bounded by construction instead:

- no ServiceAccount token is mounted, so it holds no API credentials;
- it opens no listening socket (except the explicit `tcp-listen` mode), so
  there is nothing to ingress to;
- it runs non-root, read-only-rootfs, all capabilities dropped, RuntimeDefault
  seccomp, with a hard `activeDeadlineSeconds`, and is deleted on completion
  (a leader-elected sweeper reaps orphans after an operator crash);
- its behavior is fully auditable from the pod's args (`-mode`, `-target`,
  `-port`).

Cluster admins who accept the false-negative tradeoff can still constrain
probes themselves: every probe pod carries
`fathom.skaphos.io/managed-by: fathom` and a `fathom.skaphos.io/probe` label to
select on.

## The `metrics: enabled` label contract

One label gates all Fathom metrics ingress rules:

```sh
kubectl label namespace monitoring metrics=enabled
```

Apply it to the namespace running your Prometheus/agent scrapers. It is
honored by the opt-in operator policy and by every runtime node-agent policy.
