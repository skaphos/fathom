# Phase 1 Data Model: Alerting-Grade Observability

No CRD schema changes. The model consists of two derived series families, an
event schema, and the per-kind mapping that drives both.

## Check identity

Every series and event is keyed by the check's identity:

| Field | Values | Notes |
|---|---|---|
| `kind` | `AddonCheck`, `HealthCheck`, `ClusterHealth`, `NodeCertificateCheck` | CRD Kind, TitleCase |
| `name` | metadata.name | |
| `namespace` | metadata.namespace; `""` for `ClusterHealth` | ClusterHealth is the only cluster-scoped kind |

## Result vocabulary

Reused unchanged from `api/v1alpha1` (`HealthReportResult`): `Pass`, `Warn`,
`Fail`, `Error`, `Skipped`, `Unknown` — TitleCase, severity-ordered by the
existing `Severity()` ladder (Pass < Skipped < Warn < Unknown < Fail <
Error). The metric helper iterates this canonical list so the one-hot set
always covers exactly the CRD vocabulary (FR-002, SC-004).

## Series family: current result

- **Name**: `fathom_check_result`
- **Type**: gauge (one-hot state set)
- **Labels**: `kind`, `name`, `namespace`, `result`
- **Invariant**: for an existing check, exactly six series exist (one per
  result state); exactly one has value `1`.
- **Value source**: the result just written to status (`status.lastResult`
  for AddonCheck/NodeCertificateCheck, `status.result` for
  HealthCheck/ClusterHealth). Before the first evaluated result:
  `Unknown = 1` (sentinel at discovery).

## Series family: last run

- **Name**: `fathom_check_last_run_timestamp_seconds`
- **Type**: gauge (unix seconds)
- **Labels**: `kind`, `name`, `namespace`
- **Value source**: wall-clock time at reconcile status-write of an
  evaluated result (uniform across kinds; per-kind status timestamps are
  NOT mirrored — see research R2). `0` until the first evaluation completes
  (sentinel for "never ran").

## Series lifecycle (state transitions)

| Trigger | Transition |
|---|---|
| Check first observed (reconcile, no evaluated status) | six result series appear (`Unknown=1`), last-run appears at `0` |
| Evaluation completes | one-hot set rewritten; last-run set to now |
| Check deleted (reconcile hits IsNotFound) | all seven series removed (`DeletePartialMatch` on both vecs) |
| Operator restart | registry empty; startup reconciles repopulate every existing check; checks deleted while down never reappear |
| Delete + recreate same name | series continue under the same identity — values reflect the new lineage only (status of the new object) |

## Event schema

Standard `corev1.Event` via `record.EventRecorder`; involved object is the
check resource itself.

| Field | Contract |
|---|---|
| `type` | `Normal` for transitions landing below Warn severity; `Warning` for transitions landing at/above Warn and for all operational failures |
| `reason` | stable, CamelCase, machine-usable (table below) |
| `message` | human-readable; transition messages name old and new result; failure messages carry the underlying error |
| aggregation | client-go EventCorrelator (built-in): identical events aggregate with incremented `count` — bounds sustained-failure volume (SC-006) |

### Reason vocabulary

| Reason | Type | Emitted by | When |
|---|---|---|---|
| `ResultChanged` | Normal/Warning | all four reconcilers | evaluated result differs from status' previous result; first result counts as `Unknown → X`; previous result read from status, never memory |
| `AdapterRunFailed` | Warning | AddonCheck | adapter `Run()` returned an error (existing `runErr` site) |
| `ProbeLaunchFailed` | Warning | AddonCheck | `runErr` matches the typed probe `LaunchError` (research R5) |
| `RBACProvisioningFailed` | Warning | NodeCertificateCheck | node-agent ClusterRole/ServiceAccount/RoleBinding ensure failure |
| `AdmissionPolicyProvisioningFailed` | Warning | NodeCertificateCheck | report-authenticity policy ensure failure |
| `ReconcileError` | Warning | all four reconcilers | reconcile returns a terminal error after the resource was fetched |

## Relationships

- Series and events are **derived, write-only outputs** of the reconcile
  path; nothing reads them back. Status remains the single source of truth
  (FR-011); `ClusterHealth` derivation is untouched.
- `HealthReport` remains the durable history; events are the short-window
  `kubectl describe` view; metrics are the alerting view.
