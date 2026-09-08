<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Status and Conditions Reference

This page explains the runtime status contract for Fathom's CRDs: which fields
are authoritative, what each condition means, and what an operator should check
next. The generated [API reference](api.md) is still the field-level schema
source; this page is the operational interpretation of those fields.

Implementation anchors:

- API types: `api/v1alpha1/*_types.go`
- Reconcilers: `internal/controller/*_controller.go`
- Generated CRDs: `config/crd/bases/fathom.skaphos.io_*.yaml`

## How to Read Conditions

All Fathom status conditions follow Kubernetes `metav1.Condition` conventions.

| Field | How to use it |
| --- | --- |
| `type` | Stable category such as `Accepted`, `Paused`, `Ready`, or `AgentReady`. |
| `status` | `True` means that condition currently applies; `False` means it currently does not. |
| `reason` | Machine-readable cause within the condition type. Alerting and automation should key on this rather than free-form `message`. |
| `message` | Human-readable context. Validation failures and API errors appear here. |
| `observedGeneration` | The `.metadata.generation` the controller evaluated when it wrote the condition. If it is lower than the object's current generation, the status is stale for the current spec. |
| `lastTransitionTime` | When the condition last changed status/reason/message. Use it for "stuck" diagnostics, not as the source check's observation time. |

Quick freshness check:

```sh
kubectl -n fathom-system get addoncheck cert-manager-system-health \
  -o jsonpath='{.metadata.generation}{" observed="}{.status.observedGeneration}{"\n"}'
```

If `observedGeneration` lags, the controller has not reconciled the latest spec
yet or failed before persisting status. Check the controller logs and reconcile
metrics.

## Result Fields

Fathom uses one result vocabulary everywhere:

```
Pass < Skipped < Warn < Unknown < Fail < Error
```

`Skipped` is non-fatal and rolls up green. Empty results are not included in
worst-case aggregation.

| Resource | Current verdict field | Freshness field | Detail/history field |
| --- | --- | --- | --- |
| `AddonCheck` | `status.lastResult` | `status.lastRunTime` | `status.lastReportName` |
| `HealthCheck` | `status.result` | `status.sourceObservedAt` | `status.lastReportName` |
| `ClusterHealth` | `status.result` | `status.observedAt` | `status.children` |
| `NodeCertificateCheck` | `status.lastResult` | `status.lastRunTime` | `status.lastReportName` |
| `HealthReport` | `spec.result` | `spec.observedAt` | `spec.checks` |

`ClusterHealth.status.observedAt` is the newest input observation time from its
selected `HealthCheck`s, not wall-clock time. It intentionally does not read
`HealthReport` history.

## On-demand runs

Every executable kind (`AddonCheck`, `DNSCheck`, `NodeCertificateCheck`)
honours the same on-demand trigger. Write a fresh, non-empty value to the
`fathom.skaphos.io/run-now` annotation and the controller runs the check
regardless of `spec.interval`; when that run completes it records the value in
`status.lastRunTrigger`. The rules are identical across kinds:

- A value that differs from `status.lastRunTrigger` forces a run. The same
  value never fires twice, so write a timestamp or nonce each time, not a
  constant (`fathomctl run` writes a UTC timestamp plus a random suffix).
- The consumed value is written in the same status update as the run's
  verdict, so `status.lastRunTrigger == <value>` means "that run finished".
  `fathomctl run --wait` and automation should match on it rather than on
  `lastRunTime` moving.
- A run with no annotation, or with the already-consumed value, never clears
  `status.lastRunTrigger`; re-applying a spent value does nothing, and
  removing the annotation after consumption is not itself a change (for
  `NodeCertificateCheck` it does not roll the agents).
- A value longer than 253 characters, the bound on `status.lastRunTrigger`,
  is ignored rather than recorded: recording it would make every status
  update fail validation. No supported writer produces one.
- A paused check does not consume the trigger. It stays pending until the
  check is unpaused, and `run --wait` would only time out.
- `NodeCertificateCheck` completes the trigger differently from the other
  two: the operator stamps the value onto the node-agent DaemonSet's pod
  template, which restarts every agent, each agent scans on start and
  reports the value it started with, and the operator records the value only
  once every desired node's fresh report carries it. Until then the previous
  verdict and `lastRunTrigger` are retained. Budget one agent pod restart per
  node for a forced scan.
- Derived kinds (`HealthCheck`, `ClusterHealth`) ignore the annotation. To
  re-check them, trigger their sources; `fathomctl run` does that for you.

```sh
kubectl -n fathom-system annotate dnscheck cluster-dns \
  fathom.skaphos.io/run-now="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
kubectl -n fathom-system get dnscheck cluster-dns \
  -o jsonpath='{.status.lastRunTrigger}{"\n"}'
```

## AddonCheck

`AddonCheck` is a sensor. It resolves the adapter named by `spec.addonType`,
validates the policy for that adapter, runs on create/generation change,
`spec.interval`, or a fresh `fathom.skaphos.io/run-now` annotation, and writes a
`HealthReport` on the first run or when the aggregate result changes.

Status fields to start with:

- `status.lastResult` - worst-case result from the most recent adapter run.
- `status.lastRunTime` - when the most recent adapter result was observed.
- `status.lastReportName` - latest retained `HealthReport` for the current
  result transition.
- `status.absent` - count of checks that reported an absent target marker.
- `status.detectedVersion` - add-on version detected by adapters that support
  version detection.
- `status.lastRunTrigger` - last consumed non-empty `run-now` annotation value
  (see [On-demand runs](#on-demand-runs)).

| Condition | Status / reason | Meaning | Operator action |
| --- | --- | --- | --- |
| `Accepted` | `True / SpecAccepted` | The controller accepted the spec/policy it could validate. This does not prove the add-on is healthy. | Continue to `Ready` and `lastResult`. |
| `Accepted` | `False / InvalidPolicy` | A policy family name or label selector is invalid for the selected adapter. No adapter run occurs. | Fix `spec.policy`; the message lists deterministic validation errors. |
| `Paused` | `False / RunEnabled` | The check is eligible to run. | None. |
| `Paused` | `True / Paused` | `spec.paused=true`; adapter execution is disabled and the previous status snapshot is preserved. | Unset `spec.paused` to resume. |
| `Ready` | `True / RunCompleted` | The adapter ran and status reflects the run. | Inspect `lastResult` and the referenced `HealthReport`. |
| `Ready` | `True / AdapterResolved` | The adapter and policy are valid, but this reconcile did not need to execute a run. | Check `lastRunTime` against `spec.interval` if freshness is in question. |
| `Ready` | `False / InvalidPolicy` | Policy validation failed. Also see `Accepted=False`. | Fix `spec.policy`. |
| `Ready` | `False / MissingAdapter` | No registry is configured or `spec.addonType` does not match a built-in adapter. | Check `spec.addonType`; valid values are listed in [Add-on checks](../guides/addon-checks.md). |
| `Ready` | `False / AdapterLookupFailed` | Adapter lookup failed for a reason other than "not found". | Inspect operator logs; this points to startup/registry wiring. |
| `Ready` | `False / AdapterRunFailed` | The adapter could not complete the run. The check result is `Error`. | Read the condition message and operator logs; verify adapter RBAC in [rbac.md](rbac.md). |
| `Ready` | `False / Paused` | The check is paused. | Unset `spec.paused`. |

Force an immediate run:

```sh
kubectl -n fathom-system annotate addoncheck cert-manager-system-health \
  fathom.skaphos.io/run-now="$(date -Iseconds)" --overwrite
```

## HealthCheck

`HealthCheck` is a projection layer. It mirrors one `AddonCheck`, `DNSCheck`,
or `NodeCertificateCheck` into a uniform status shape for `ClusterHealth`.
`spec.checkRef` is immutable — retargeting a wrapper would silently repoint its
mirrored status snapshot at a different check, so replace the wrapper instead.

Status fields to start with:

- `status.result` - mirrored specialized-check `status.lastResult`.
- `status.summary` - derived from the wrapped check's `Ready` condition.
- `status.sourceObservedAt` - mirrored specialized-check `status.lastRunTime`.
- `status.lastReportName` - mirrored specialized-check
  `status.lastReportName`.

| Condition | Status / reason | Meaning | Operator action |
| --- | --- | --- | --- |
| `Accepted` | `True / SpecAccepted` | The wrapper spec was accepted for reconciliation. | Continue to `Ready`. |
| `Paused` | `False / RunEnabled` | Mirroring is enabled. | None. |
| `Paused` | `True / Paused` | `spec.paused=true`; mirroring is suspended and the previous mirrored snapshot is preserved. | Unset `spec.paused` to resume. |
| `Ready` | `True / TargetMirrored` | The referenced specialized check was read and mirrored. | Check `status.result` and `sourceObservedAt`. |
| `Ready` | `False / UnsupportedAPIVersion` | A nonempty `spec.checkRef.apiVersion` is not the current `fathom.skaphos.io/v1alpha1` contract. Mirrored fields are cleared without reading a target. | Replace the immutable wrapper with the current API version, or omit `apiVersion` to use the current default. |
| `Ready` | `False / UnsupportedKind` | `spec.checkRef.kind` is not `AddonCheck`, `DNSCheck`, or `NodeCertificateCheck`. Mirrored fields are cleared. | Replace the immutable wrapper with one of the supported kinds. |
| `Ready` | `False / TargetNotFound` | The referenced target does not exist in the wrapper namespace, or in explicit `checkRef.namespace`. Mirrored fields are cleared. | Create the target, or replace the wrapper if the immutable reference is wrong. |
| `Ready` | `False / TargetLookupFailed` | Reading a supported target failed with a transient API error. The last readable mirrored snapshot is preserved and the controller returns the error for retry. | Check controller logs, API-server availability, and RBAC; do not treat the retained snapshot as new evidence. |
| `Ready` | `False / Paused` | The wrapper is paused. | Unset `spec.paused`. |

Namespace contract: `ClusterHealth` is cluster-scoped and selects `HealthCheck`
wrappers under the allowlist / denylist / open filter (`spec.namespaces`,
`spec.excludedNamespaces`). A wrapper may mirror a supported specialized check
in another namespace with `spec.checkRef.namespace`, so control who can create
wrappers and how the aggregate filters namespaces.

## ClusterHealth

`ClusterHealth` aggregates `HealthCheck.status` under the allowlist /
denylist / open namespace filter (`spec.namespaces` is definitive when set;
otherwise `spec.excludedNamespaces`; otherwise all namespaces). It never reads
`AddonCheck` directly and never reads `HealthReport` history.

Status fields to start with:

- `status.result` - worst-case result across selected children with non-empty
  results.
- `status.matchedCount` - number of selected `HealthCheck` objects.
- `status.children` - deterministic summary of selected children, sorted by
  namespace, then name.
- `status.observedAt` - newest `sourceObservedAt` among selected children.

| Condition | Status / reason | Meaning | Operator action |
| --- | --- | --- | --- |
| `Accepted` | `True / SpecAccepted` | The aggregate spec was accepted for reconciliation. | Continue to `Ready`. |
| `Ready` | `True / Aggregated` | Selected `HealthCheck`s were listed and aggregated. | Read `result`, `matchedCount`, and `children`. |
| `Ready` | `False / InvalidSelector` | `spec.selector` could not be parsed. Aggregate fields are cleared. | Fix the label selector. |
| `Ready` | `False / ListFailed` | The controller could not list selected `HealthCheck`s. Aggregate fields are cleared. | Check controller RBAC and API-server errors in logs. |

An empty or omitted selector matches all `HealthCheck`s in the `ClusterHealth`
namespace. If `matchedCount=0`, the result is empty because no child results are
available to aggregate.

## NodeCertificateCheck

`NodeCertificateCheck` manages a node-agent DaemonSet and rolls fresh per-node
reports into a `HealthReport`. A `HealthCheck` can project that status into
`ClusterHealth`.

Status fields to start with:

- `status.lastResult` - worst-case result across complete, fresh node reports.
- `status.lastRunTime` - `HealthReport.spec.observedAt` from the latest roll-up.
- `status.lastReportName` - latest retained node-certificate `HealthReport`.
- `status.desiredNodes` - DaemonSet desired scheduled count.
- `status.reportingNodes` - count of fresh node reports consumed in the latest
  reconcile.
- `status.lastRunTrigger` - last `run-now` value every desired node reported
  back after the forced scan (see [On-demand runs](#on-demand-runs)).

Freshness and coverage rules:

- A report is fresh when `report.observedAt` is no older than
  `spec.interval + spec.timeout` and not implausibly in the future.
- Reports are keyed by node; duplicate reports for a node collapse to the newest
  observed report.
- The operator rolls up only once the DaemonSet has fully converged to its
  current spec (`status.observedGeneration == metadata.generation`, every desired
  pod updated and ready) and every node in scope has a fresh report, so a
  roll-up is never computed from stale-template pods that are still rolling out.
- **Coverage is per-node-identity, not a count.** The nodes in scope are the
  nodes the agent pods are scheduled on; coverage is complete only when each of
  those node names has a fresh report. A count comparison would let a departed
  node's still-fresh report stand in for a newly joined node's missing one, so
  the roll-up could claim full coverage while a live node had never been scanned.
- Coverage still tolerates transient node-count churn: a report from a node that
  was removed or deselected survives (it is owner-referenced by the check) until
  it ages out. Surplus reports (`reportingNodes > desiredNodes`) are simply not
  consulted, so they neither complete nor block coverage.
- Incomplete coverage **freezes** the roll-up: `lastResult`, `lastRunTime`, and
  `lastReportName` keep their last known values, and `lastRunTime` never moves
  backward. A gap in reporting — a rollout, a node joining, an agent restart — is
  not evidence that the fleet became healthy or unhealthy, and clearing the
  verdict churned every mirroring `HealthCheck` and `ClusterHealth` through
  `Unknown`. The `CoverageComplete` condition carries the gap, so a frozen
  verdict is always distinguishable from a fresh one.
- A provisioning failure (RBAC, admission policy, NetworkPolicy, or DaemonSet)
  is persisted as `Ready=False` with the matching reason. It does not clear the
  verdict either — provisioning failing says nothing about what the last
  complete scan found.
- A report ConfigMap is adopted only after its decoded payload belongs to the
  current check (`report.checkName == metadata.name`), so mislabeled reports are
  ignored and not garbage-collected by the wrong check.
- A pending `run-now` value no longer needs a rule of its own: freezing is now
  the behavior for every incomplete window, so a forced scan cannot flap the
  mirroring `HealthCheck` or `ClusterHealth` either. Ordinary roll-ups continue
  from whatever fresh reports exist. The value is recorded in `lastRunTrigger`
  only when every node in scope has a fresh report carrying that value — the
  same per-node-identity rule as an ordinary roll-up, so a departed node cannot
  complete a trigger on a live node's behalf. Reports with an empty or different
  value never count toward it, and `lastRunTime` is refreshed on completion even
  when the aggregate did not change.

| Condition | Status / reason | Meaning | Operator action |
| --- | --- | --- | --- |
| `Accepted` | `True / SpecAccepted` | The spec was accepted. Structural invalid specs are normally rejected by the API server from CRD validation before reconciliation. | Continue to `AgentReady` and `Ready`. |
| `Paused` | `False / RunEnabled` | The node-agent is eligible to run. | None. |
| `Paused` | `True / Paused` | `spec.paused=true`; the operator deletes the agent DaemonSet and preserves the last status snapshot. | Unset `spec.paused` to recreate the DaemonSet. |
| `AgentReady` | `True / RolledOut` | The DaemonSet has fully converged: the current generation is observed and every desired pod is updated and ready. | Continue to `Ready`. |
| `AgentReady` | `False / RollingOut` | The DaemonSet exists but has not fully converged: the current generation is not yet observed, or not every desired pod is updated and ready. | Inspect DaemonSet pods, scheduling, image pulls, and tolerations. |
| `AgentReady` | `False / NoMatchingNodes` | The DaemonSet selects zero nodes. | Check `spec.nodeSelector` and cluster labels. |
| `AgentReady` | `False / Paused` | The node-agent is intentionally stopped. | Unset `spec.paused` to resume. |
| `Ready` | `True / Reporting` | Complete, fresh reports were rolled up into a `HealthReport`. | Read `lastResult` and the referenced `HealthReport`. |
| `Ready` | `False / NoMatchingNodes` | No nodes match the DaemonSet. | Fix `spec.nodeSelector`, tolerations, or node labels. |
| `Ready` | `False / AwaitingReports` | No fresh reports have been consumed yet. | Check node-agent pods and ConfigMaps. |
| `Ready` | `False / PartialReports` | Some, but not all, nodes in scope have fresh reports. The previous verdict is frozen, not cleared. | Find missing/stale node-agent pods or report ConfigMaps; `CoverageComplete` names the missing nodes. |
| `Ready` | `False / AgentRollingOut` | Every node in scope has reported, but the DaemonSet has not fully converged (a pod is not ready or an update is still rolling). A surplus of reports from node churn is tolerated here, not flagged. | Wait for rollout or inspect pod scheduling/image pulls. |
| `CoverageComplete` | `True / AllNodesReporting` | Every node in scope published a fresh scan result, so `lastResult` reflects a complete scan. | None. |
| `CoverageComplete` | `False / PartialReports` | At least one node in scope has no fresh report. The message names the missing nodes (up to five). `lastResult` is the frozen previous verdict. | Inspect the named nodes' agent pods and report ConfigMaps. |
| `CoverageComplete` | `False / AgentRollingOut` | The DaemonSet has not fully converged, so no roll-up was computed. `lastResult` is frozen. | Wait for rollout or inspect pod scheduling/image pulls. |
| `CoverageComplete` | `False / NoMatchingNodes` | The DaemonSet selects zero nodes; there is nothing to scan. | Check `spec.nodeSelector` and cluster labels. |
| `ReportsAuthentic` | `True / AllReportsBound` | Every collected report is bound to the node it claims. | None. |
| `ReportsAuthentic` | `False / ForgedReportRejected` | One or more reports failed a binding only a writer passing off another node's report can fail — a payload disagreeing with the node-name annotation, or a report at a non-canonical ConfigMap name. The message names the ConfigMaps (up to five) and the reason. The rejected reports are excluded from the aggregate. | Investigate: some principal with ConfigMap write in the namespace is attempting to steer a node's verdict. Check who holds `configmaps` write there, and confirm the report-authenticity `ValidatingAdmissionPolicy` is enforced. |
| `Ready` | `False / RBACProvisioningFailed` | Runtime ClusterRole/ServiceAccount/RoleBinding provisioning failed. | Check operator RBAC and admission failures. |
| `Ready` | `False / DaemonSetProvisioningFailed` | Creating/updating the node-agent DaemonSet failed. | Check admission policies, security policies, and image settings. |
| `Ready` | `False / AdmissionPolicyProvisioningFailed` | Creating/updating the report-authenticity `ValidatingAdmissionPolicy` or its binding failed. | Check operator RBAC on `admissionregistration.k8s.io` and cluster API support. |
| `Ready` | `False / NetworkPolicyProvisioningFailed` | Creating/updating the per-check node-agent `NetworkPolicy` failed. | Check operator RBAC on `networking.k8s.io` and admission policies. |
| `Ready` | `False / Paused` | The check is paused. | Unset `spec.paused`. |

Useful report inspection:

```sh
kubectl -n fathom-system get configmap \
  -l 'fathom.skaphos.io/managed-by=fathom,fathom.skaphos.io/source-name=node-certificates'

kubectl -n fathom-system get healthreport \
  -l 'fathom.skaphos.io/source-kind=NodeCertificateCheck,fathom.skaphos.io/source-name=node-certificates'
```

## DNSCheck

`DNSCheck` asserts that names resolve — or deliberately do not — from one or
more vantage points. Resolution runs in a probe Pod inside **the check's own
namespace**, so a check author's reach is exactly their existing reach and the
namespace's own NetworkPolicy governs the query.

The unit of everything here is the **(target, vantage point) pair**. A target
naming a resolver produces one pair; a target naming none is evaluated against
every declared vantage point, or against the implicit one named `cluster` when
the check declares none. The schema caps this at 16 targets × 3 vantage points,
so a check implies at most 48 pairs and that number is derivable from its spec
before it ever runs.

Status fields to start with:

- `status.lastResult` — the folded verdict across every pair, using the same
  vocabulary and precedence as every other kind.
- `status.summary` — one line. When a failure comes from a negative assertion it
  says so explicitly, so a deliberate "this name must be gone" failure is not
  triaged as a DNS outage.
- `status.targetResults` — one entry per pair, keyed by name, record type, and
  resolver, carrying the message, the answers returned, and latency as evidence.
- `status.observedTargets` — how many pairs the last run covered.
- `status.lastRunTime` / `status.lastReportName` — when it last ran, and the
  `HealthReport` capturing the current verdict.
- `status.lastRunTrigger` — last consumed `run-now` value (see
  [On-demand runs](#on-demand-runs)). A `DNSCheck` evaluates on every
  reconcile, so the annotation write itself causes the run; this field is what
  makes it observable.

Rules worth knowing:

- **Results are rebuilt, never accumulated.** A pair the spec no longer declares
  disappears from `targetResults` and its metric series is withdrawn on the next
  run — it does not freeze at its last verdict.
- **A history record is written only when the verdict changes.** `lastRunTime`
  still advances every run, so staleness alerting stays honest.
- **A resolution failure is a `Fail`, not an `Error`.** Error is reserved for
  faults that are not the resolver's answer, which keeps a real DNS outage from
  masking unrelated failures in the aggregate.
- **An unreachable resolver never satisfies `absent: true`.** That is a network
  fault, not evidence a name was retired, and it reports `Error`.
- **The next run is scheduled one cadence from when the previous run *started***,
  not from when it finished, so the effective cadence equals the declared one.
  A minimum gap applies if a run consumes its whole cadence.
- **`spec.timeout` bounds the whole run, not each pair.** A check with many pairs
  and a small bound truncates: see `Complete` below.

There is no `Paused` condition and no `spec.paused` — stopping a `DNSCheck`
means deleting it.

| Condition | Status / reason | Meaning | Operator action |
| --- | --- | --- | --- |
| `Accepted` | `True / SpecAccepted` | The spec was accepted. Structural invalid specs are rejected by CRD validation before reconciliation. | Continue to `Ready`. |
| `Accepted` | `True / SpecClamped` | A stored `interval`/`timeout` below the schema floors is running clamped up. | Raise the values in the spec to match what is actually running. |
| `Ready` | `True / EvaluationSucceeded` | Fathom could evaluate the check. This says nothing about what it found — a check whose every name legitimately fails to resolve is `Ready=True` with a `Fail` verdict. | None. |
| `Ready` | `False / ProbeExecutionFailed` | One or more pairs could not be *performed*: quota, admission, image pull, or an unschedulable node. The message names how many. | Check probe Pod events in the check's namespace; confirm the namespace admits the hardened probe Pod and can pull the probe image. |
| `Complete` | `True / AllPairsEvaluated` | Every pair the spec implies was reached. | None. |
| `Complete` | `False / RunTruncated` | The run bound elapsed before every pair was reached. Unreached pairs report `Unknown`, which degrades the verdict without outranking a real `Fail`. The message names the count. | Raise `spec.timeout` (and `spec.interval`, which must not be lower) or declare fewer targets. See [DNSCheck fan-out](configuration.md#dnscheck-fan-out) for sizing. |

```bash
kubectl -n <namespace> get dnscheck <name> -o yaml

# Which pair failed, without reading controller logs:
kubectl -n <namespace> get dnscheck <name> \
  -o jsonpath='{range .status.targetResults[*]}{.name}{" "}{.recordType}{" "}{.resolver}{" -> "}{.result}{"\n"}{end}'

kubectl -n <namespace> get healthreport \
  -l 'fathom.skaphos.io/source-kind=DNSCheck,fathom.skaphos.io/source-name=<name>'
```

## HealthReport

`HealthReport` is an immutable history object created by the controllers; the
CRD schema rejects any `spec` update. It has no meaningful status conditions
today; read `spec`.

Important labels:

| Label | Meaning |
| --- | --- |
| `fathom.skaphos.io/source-kind` | Source kind, such as `AddonCheck` or `NodeCertificateCheck`. |
| `fathom.skaphos.io/source-name` | Source object name. |

Important spec fields:

- `spec.sourceRef` - source object reference.
- `spec.result` - aggregate result for this report.
- `spec.checks` - per-family/per-target details.
- `spec.observedAt` - source observation time.
- `spec.detectedVersion` - add-on version when the adapter reports one.

Controllers use deterministic names for replay-safe report creation around
status conflicts. If a deterministic name already exists, it is reused only when
the existing `HealthReport` has the same source reference, expected source
labels, and result.
