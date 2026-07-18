<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# 3. Probe-pod model for active in-cluster network checks

- **Status**: accepted
- **Date**: 2026-05-10
- **Deciders**: Shawn Stratton

## Context and Problem Statement

Some adapter check families need active in-cluster network behavior, not
API state inspection. Examples:

- CoreDNS `dns_resolution`: prove `kubernetes.default.svc.cluster.local`
  resolves end-to-end from inside the cluster.
- Future DNSCheck (SKA-47), ReachabilityCheck (SKA-50), and per-NetworkPolicy
  smoke tests need TCP-connect or TCP-listen probes.

The operator pod cannot perform these reliably from its own process: its
NetworkPolicy posture, DNS configuration, and node placement do not match
the workloads being checked. A check that "works from the operator pod"
does not prove the workload's connectivity.

We need a way to launch ephemeral, hardened, in-cluster probes whose
network topology mirrors the workload being checked, then collect a
structured result.

## Decision Drivers

- The signal must be representative — same NetworkPolicy domain, same DNS
  resolver topology, same node-locality constraints as the workload being
  checked. Otherwise the check produces false negatives (passes from the
  operator pod, fails from the actual workload).
- The operator's RBAC and network privileges should not expand to "talk
  to everything in the cluster." That makes the operator a juicier target
  and erodes the security posture of clusters that adopt it.
- v0.1 should not require a DaemonSet. Operators of small clusters and
  edge clusters resist anything that adds per-node footprint.
- Probe code must be auditable. A user reading the probe Pod's command
  args should be able to tell exactly what the check is doing.

## Considered Options

- **A. In-process net code from the operator pod.** `net.Resolver.LookupHost`
  inside `Adapter.Run`.
- **B. Sidecar container in the operator Deployment.** Long-lived probe
  process colocated with the manager.
- **C. DaemonSet probe agent on every node.** One probe Pod per node,
  always running; the operator dispatches RPCs to it.
- **D. Single-shot, hardened probe Pod per check, launched by the
  operator.** Pod runs the probe binary, writes JSON to
  `/dev/termination-log`, exits. Operator parses and deletes.
- **E. CronJob-based probes.** A `CronJob` per check; results extracted
  from completed Pods.

## Decision Outcome

Chosen option: **D. Single-shot, hardened probe Pod per check, launched
by the operator**, because it is the only option that preserves
representativeness (probe runs in the same NetworkPolicy/DNS topology as
the workload) without requiring a DaemonSet or expanding the operator's
privileges.

The probe binary lives at `cmd/probe/`; the Pod spec lives at
`internal/probe/pod.go`. Hardened defaults are baked into `Pod()`:
non-root (uid 65532), drop ALL capabilities, read-only rootfs,
RuntimeDefault seccomp, no service-account token automount,
`ActiveDeadlineSeconds = timeout + 5s`, optional pod anti-affinity for
spread. Result extraction relies on
`TerminationMessagePolicy=FallbackToLogsOnError` so the launcher can read
JSON from the termination message and fall back to container logs if the
Pod was killed before writing.

The launcher (lifecycle, RBAC, namespace contract, cleanup) is tracked
separately in SKA-307. Until it lands, `Pod()` is the manifest builder
and adapters cannot yet launch probes.

### Consequences

- **Positive**: probe runs from inside the target namespace, subject to
  the same NetworkPolicies and DNS as the workload. Operator RBAC stays
  narrow (no need to add cluster-wide network privileges). Each probe is
  auditable: read the Pod's args. No DaemonSet, no per-node footprint
  when no probes are pending.
- **Negative**: Pod-create and image-pull latency on every probe (mitigated
  by image pull policy and by the probe image being small/distroless).
  The operator becomes responsible for Pod lifecycle: leaks must be
  prevented even on cancellation. A separate `fathom-probe` image must
  be built, signed, and published in lockstep with the operator image —
  release pipeline cost not yet sized.
- **Neutral**: probes that need to bypass kubelet-injected DNSPolicy
  (e.g., upstream-resolver checks) will need an extension to
  `probe.Request` to set `Pod.Spec.DNSPolicy`/`DNSConfig`. Tracked
  alongside SKA-47 (DNSCheck).

## Pros and Cons of the Options

### A. In-process net code from the operator pod

- Good, because zero deployment surface; the operator is already running.
- Bad, because the operator's NetworkPolicy and DNS topology are wrong;
  the check answers a different question than the user is asking.

### B. Sidecar container in the operator Deployment

- Good, because the sidecar shares the operator's lifecycle and is easy
  to upgrade.
- Bad, because the sidecar still runs in the operator's namespace; same
  topology problem as A.

### C. DaemonSet probe agent on every node

- Good, because nodes are the right network position for many checks
  (host-level reachability, node-local DNS).
- Bad, because every cluster pays per-node Pod overhead even for checks
  that don't need node placement. v0.1 doesn't have a check that requires
  per-node placement.
- Bad, because operating a DaemonSet has a much higher install/upgrade
  surface; agents drift.

### D. Single-shot probe Pod per check

- Good, because topology is right and lifecycle is bounded.
- Good, because the probe image is the entire trust boundary — operators
  can audit it independently of Fathom.
- Bad, because per-probe Pod creation is not free; expensive at very
  high cadence (mitigated by adapter intervals being on the order of
  minutes).

### E. CronJob-based probes

- Good, because lifecycle is owned by Kubernetes; we get retry/backoff
  for free.
- Bad, because the operator can't trigger ad-hoc runs without writing
  CronJob spec edits, and result extraction across CronJob-owned Job-
  owned Pods is awkward.

## Links

- `cmd/probe/main.go` — probe binary (dns, tcp-connect, tcp-listen modes)
- `internal/probe/pod.go` — `Pod(Request)` hardened manifest builder,
  `ParseResult`
- `Dockerfile.probe` — distroless probe image
- ADR-0001 — adapter contract (probe is launched on behalf of an adapter)
- Commits: `62e559f` (probe pod foundation)
- Related Linear: SKA-306 (foundation, done), SKA-307 (launcher),
  SKA-308 (CoreDNS resolution check), SKA-47 (DNSCheck)
