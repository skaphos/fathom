<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# DNS Checks

A `DNSCheck` tests whether names resolve—or deliberately do not resolve—from
the network vantage point that matters. Fathom runs each lookup in a hardened,
short-lived probe Pod in the check's namespace, records per-target evidence,
and can project the verdict through `HealthCheck` into `ClusterHealth`.

Use this guide to author the complete signal chain. For every field and
validation rule, see the generated [API reference](../reference/api.md).

## Check cluster DNS and aggregate the result

This manifest verifies both a positive and a required-absence expectation,
then includes their combined verdict in cluster health:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: platform-health
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: cluster-dns
  namespace: platform-health
spec:
  interval: 1m
  timeout: 30s
  historyLimit: 10
  targets:
    - name: kubernetes.default.svc.cluster.local
      recordType: Host
    - name: decommissioned-name.invalid.
      recordType: A
      absent: true
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: cluster-dns
  namespace: platform-health
  labels:
    fathom.skaphos.io/scope: platform
spec:
  checkRef:
    apiVersion: fathom.skaphos.io/v1alpha1
    kind: DNSCheck
    name: cluster-dns
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: ClusterHealth
metadata:
  name: platform
spec:
  namespaces:
    - platform-health
  selector:
    matchLabels:
      fathom.skaphos.io/scope: platform
```

Save it as `dns-health.yaml`, then apply and inspect it:

```sh
kubectl apply -f dns-health.yaml
kubectl -n platform-health get dnscheck cluster-dns
kubectl -n platform-health get healthcheck cluster-dns
kubectl get clusterhealth platform
```

The omitted target `resolver` selects every declared resolver. Because this
example declares none, Fathom uses the implicit resolver named `cluster`, which
is the check namespace's normal cluster DNS configuration. `Host` accepts an
IPv4 or IPv6 answer. The `.invalid.` name is reserved for non-resolution, so
`absent: true` is a deterministic positive assertion that the name stays gone.

`checkRef.apiVersion` may be omitted to select the current
`fathom.skaphos.io/v1alpha1` version. A nonempty value is matched exactly.
`checkRef.namespace` may be omitted when the source and wrapper share a
namespace, as above.

## Choose a resolver

Resolvers are vantage points, not fallback servers. A target without a
`resolver` runs against every resolver in `spec.resolvers`; a target naming one
runs only there.

### Cluster resolver

Use the implicit cluster resolver for the same DNS path ordinary Pods in the
namespace use. Declare it explicitly only when a check also has other vantage
points and one target must select cluster DNS:

```yaml
spec:
  targets:
    - name: kubernetes.default.svc.cluster.local
      recordType: Host
      resolver: cluster
```

The name `cluster` is reserved and must not appear in `spec.resolvers`.

### Explicit resolver

Use `from: Explicit` to query an upstream by IP address. An explicit resolver
requires `address`; it accepts an IP with an optional port, never a hostname.
Targets must use fully qualified names because the probe has no cluster search
domains on this path.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: public-dns
  namespace: platform-health
spec:
  interval: 5m
  timeout: 30s
  resolvers:
    - name: public
      from: Explicit
      address: 1.1.1.1:53
  targets:
    - name: example.com.
      recordType: A
      resolver: public
```

Before using a public address, confirm the namespace's NetworkPolicy and the
cluster's egress controls allow UDP/TCP port 53. For an internal upstream,
replace the example address with that resolver's reachable IP.

`from: Node` uses the node resolver configuration visible to the probe. It is
useful when cluster DNS and node-level resolution must be compared as distinct
signals.

## Express expectations

- Omit `expectedAnswers` when any nonempty answer is acceptable.
- Set `expectedAnswers` when specific answers must be present. Matching is
  containment: extra round-robin answers do not fail the check.
- Set `absent: true` when the name must not resolve. Do not combine it with
  `expectedAnswers`.
- Choose `Host`, `A`, `AAAA`, `CNAME`, `SRV`, or `PTR`. `Host` is the default
  and accepts either address family; `PTR` requires an IP address as `name`.

```yaml
spec:
  targets:
    - name: api.internal.example.com.
      recordType: A
      expectedAnswers:
        - 10.20.30.40
    - name: retired.internal.example.com.
      recordType: A
      absent: true
```

An unreachable resolver never satisfies `absent: true`. Fathom reports a
network/execution fault rather than treating lack of a reply as proof that the
name is absent.

## Read the evidence chain

Start at the source:

```sh
kubectl -n platform-health get dnscheck cluster-dns -o yaml
kubectl -n platform-health get dnscheck cluster-dns \
  -o jsonpath='{range .status.targetResults[*]}{.name}{" "}{.recordType}{" via "}{.resolver}{" -> "}{.result}{": "}{.message}{" answers="}{.answers}{"\n"}{end}'
```

`status.lastResult` is the worst pair verdict. `status.summary` is the concise
run summary; `status.targetResults` retains each pair's result, message,
answers, and latency. `status.lastRunTime` advances on every completed run.
`status.lastReportName` changes only when the verdict changes because
`HealthReport` history is transition-based.

Then compare the projection and aggregate:

```sh
kubectl -n platform-health get healthcheck cluster-dns \
  -o jsonpath='{.status.result}{" observed="}{.status.sourceObservedAt}{" report="}{.status.lastReportName}{"\n"}{.status.summary}{"\n"}'
kubectl get clusterhealth platform \
  -o jsonpath='{.status.result}{" observed="}{.status.observedAt}{"\n"}{range .status.children[*]}{.namespace}{"/"}{.name}{" -> "}{.result}{": "}{.summary}{"\n"}{end}'
```

The `HealthCheck` copies the DNS verdict, Ready-condition summary, source
observation time, report name, and effective cadence. `ClusterHealth` consumes
only `HealthCheck.status`; it never reads `DNSCheck` or `HealthReport` directly.

## Troubleshoot common failures

### The lookup returned no answer

Inspect the pair's `message`, `answers`, and `resolver`. A positive expectation
that receives NXDOMAIN/no records is `Fail`; that is a DNS finding, not an
operator error. Confirm spelling, qualification, record type, and which
resolver the target selected.

### Answers exist but the expectation fails

Compare `expectedAnswers` with `status.targetResults[*].answers`. For `A` and
`AAAA`, confirm the expected address family. For rotating records, list only
answers that must always be present; returned supersets are allowed.

### The run timed out or is incomplete

Read the `Complete` condition. `False / RunTruncated` means `spec.timeout`
expired before every `(target, resolver)` pair ran; unreached pairs become
`Unknown`. Increase `timeout` (it cannot exceed `interval`), reduce fan-out, or
name one resolver per target. See
[DNSCheck fan-out](../reference/configuration.md#dnscheck-fan-out).

### An explicit resolver is unreachable

`Ready=False / ProbeExecutionFailed` or an `Error` pair points to execution or
network reachability rather than an authoritative negative answer. Check probe
Pod events in the DNSCheck namespace, NetworkPolicy, egress policy, the IP and
port, and whether both UDP and TCP DNS are allowed.

```sh
kubectl -n platform-health get events --sort-by=.lastTimestamp
kubectl -n platform-health get pods -l fathom.skaphos.io/managed-by=fathom
```

### The wrapper does not mirror the source

Inspect its Ready reason:

- `UnsupportedAPIVersion`: replace the immutable wrapper with the current API
  version or omit `checkRef.apiVersion`.
- `UnsupportedKind`: use `AddonCheck`, `DNSCheck`, or
  `NodeCertificateCheck`.
- `TargetNotFound`: create the exact target or replace the wrapper with the
  correct immutable name/namespace.
- `TargetLookupFailed`: the last readable snapshot is preserved while the
  controller retries; check API-server availability and operator RBAC.

See [Status and conditions](../reference/status-conditions.md) for the complete
reason contract.
