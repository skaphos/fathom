<!--
SPDX-FileCopyrightText: 2026 Skaphos
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
- `status.lastRunTrigger` - last consumed non-empty `run-now` annotation value.

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

`HealthCheck` is a projection layer. It mirrors one `AddonCheck` into a uniform
status shape for `ClusterHealth`. `spec.checkRef` is immutable — retargeting a
wrapper would silently repoint its mirrored status snapshot at a different
check, so replace the wrapper instead.

Status fields to start with:

- `status.result` - mirrored `AddonCheck.status.lastResult`.
- `status.summary` - derived from the wrapped check's `Ready` condition.
- `status.sourceObservedAt` - mirrored `AddonCheck.status.lastRunTime`.
- `status.lastReportName` - mirrored `AddonCheck.status.lastReportName`.

| Condition | Status / reason | Meaning | Operator action |
| --- | --- | --- | --- |
| `Accepted` | `True / SpecAccepted` | The wrapper spec was accepted for reconciliation. | Continue to `Ready`. |
| `Paused` | `False / RunEnabled` | Mirroring is enabled. | None. |
| `Paused` | `True / Paused` | `spec.paused=true`; mirroring is suspended and the previous mirrored snapshot is preserved. | Unset `spec.paused` to resume. |
| `Ready` | `True / TargetMirrored` | The referenced `AddonCheck` was read and mirrored. | Check `status.result` and `sourceObservedAt`. |
| `Ready` | `False / UnsupportedKind` | `spec.checkRef.kind` is not supported. This build supports `AddonCheck` only. Mirrored fields are cleared. | Replace the wrapper with one whose `checkRef.kind` is `AddonCheck` (`checkRef` is immutable). |
| `Ready` | `False / TargetNotFound` | The referenced `AddonCheck` does not exist in the wrapper namespace, or in explicit `checkRef.namespace`. Mirrored fields are cleared. | Create the target, or replace the wrapper if the ref is wrong (`checkRef` is immutable). |
| `Ready` | `False / TargetLookupFailed` | Reading the target failed for another API error. Mirrored fields are cleared. | Check controller logs and RBAC. |
| `Ready` | `False / Paused` | The wrapper is paused. | Unset `spec.paused`. |

Namespace contract: `ClusterHealth` is cluster-scoped and selects `HealthCheck`
wrappers across all namespaces (narrow with `spec.namespaces`). A wrapper may
mirror an `AddonCheck` in another namespace with `spec.checkRef.namespace`, so
control who can create wrappers anywhere in the cluster.

## ClusterHealth

`ClusterHealth` aggregates `HealthCheck.status` across all namespaces
(optionally narrowed by `spec.namespaces`). It never reads `AddonCheck`
directly and never reads `HealthReport` history.

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
reports into a `HealthReport`. It is not wrapped by `HealthCheck` in this build.

Status fields to start with:

- `status.lastResult` - worst-case result across complete, fresh node reports.
- `status.lastRunTime` - `HealthReport.spec.observedAt` from the latest roll-up.
- `status.lastReportName` - latest retained node-certificate `HealthReport`.
- `status.desiredNodes` - DaemonSet desired scheduled count.
- `status.reportingNodes` - count of fresh node reports consumed in the latest
  reconcile.

Freshness and coverage rules:

- A report is fresh when `report.observedAt` is no older than
  `spec.interval + spec.timeout` and not implausibly in the future.
- Reports are keyed by node; duplicate reports for a node collapse to the newest
  observed report.
- The operator rolls up only when fresh reports are complete for the DaemonSet's
  desired node count. Partial/mismatched coverage clears `lastResult`,
  `lastRunTime`, and `lastReportName`.
- A report ConfigMap is adopted only after its decoded payload belongs to the
  current check (`report.checkName == metadata.name`), so mislabeled reports are
  ignored and not garbage-collected by the wrong check.

| Condition | Status / reason | Meaning | Operator action |
| --- | --- | --- | --- |
| `Accepted` | `True / SpecAccepted` | The spec was accepted. Structural invalid specs are normally rejected by the API server from CRD validation before reconciliation. | Continue to `AgentReady` and `Ready`. |
| `Paused` | `False / RunEnabled` | The node-agent is eligible to run. | None. |
| `Paused` | `True / Paused` | `spec.paused=true`; the operator deletes the agent DaemonSet and preserves the last status snapshot. | Unset `spec.paused` to recreate the DaemonSet. |
| `AgentReady` | `True / RolledOut` | The DaemonSet reports all desired nodes ready. | Continue to `Ready`. |
| `AgentReady` | `False / RollingOut` | The DaemonSet exists but not all selected pods are ready. | Inspect DaemonSet pods, scheduling, image pulls, and tolerations. |
| `AgentReady` | `False / NoMatchingNodes` | The DaemonSet selects zero nodes. | Check `spec.nodeSelector` and cluster labels. |
| `AgentReady` | `False / Paused` | The node-agent is intentionally stopped. | Unset `spec.paused` to resume. |
| `Ready` | `True / Reporting` | Complete, fresh reports were rolled up into a `HealthReport`. | Read `lastResult` and the referenced `HealthReport`. |
| `Ready` | `False / NoMatchingNodes` | No nodes match the DaemonSet. | Fix `spec.nodeSelector`, tolerations, or node labels. |
| `Ready` | `False / AwaitingReports` | No fresh reports have been consumed yet. | Check node-agent pods and ConfigMaps. |
| `Ready` | `False / PartialReports` | Some, but not all, selected nodes have fresh reports. | Find missing/stale node-agent pods or report ConfigMaps. |
| `Ready` | `False / ReportMismatch` | Fresh report count exceeds desired node count. | Look for stale/mislabeled report ConfigMaps or node identity churn. |
| `Ready` | `False / AgentRollingOut` | Reports exist, but the DaemonSet is still rolling out. | Wait for rollout or inspect pod scheduling/image pulls. |
| `Ready` | `False / RBACProvisioningFailed` | Runtime ClusterRole/ServiceAccount/RoleBinding provisioning failed. | Check operator RBAC and admission failures. |
| `Ready` | `False / DaemonSetProvisioningFailed` | Creating/updating the node-agent DaemonSet failed. | Check admission policies, security policies, and image settings. |
| `Ready` | `False / Paused` | The check is paused. | Unset `spec.paused`. |

Useful report inspection:

```sh
kubectl -n fathom-system get configmap \
  -l 'fathom.skaphos.io/managed-by=fathom,fathom.skaphos.io/source-name=node-certificates'

kubectl -n fathom-system get healthreport \
  -l 'fathom.skaphos.io/source-kind=NodeCertificateCheck,fathom.skaphos.io/source-name=node-certificates'
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
