# Contract: Check Events

Kubernetes Events recorded on check resources, visible via
`kubectl describe <kind> <name>` and `kubectl get events`. Reasons are a
stable contract; messages are human-readable and may be reworded.

## Reasons

| Reason | Type | Object kinds | Message shape |
|---|---|---|---|
| `ResultChanged` | Normal (new severity < Warn) / Warning (≥ Warn) | all four | `check result changed from <old> to <new>` (first evaluation: `<old>` = `Unknown`) |
| `AdapterRunFailed` | Warning | AddonCheck | `adapter run failed for <addonType>: <error>` |
| `ProbeLaunchFailed` | Warning | AddonCheck | `probe pod launch failed: <error>` |
| `RBACProvisioningFailed` | Warning | NodeCertificateCheck | `node-agent RBAC provisioning failed: <error>` |
| `AdmissionPolicyProvisioningFailed` | Warning | NodeCertificateCheck | `report-authenticity policy provisioning failed: <error>` |
| `ReconcileError` | Warning | all four | `reconcile failed: <error>` |

## Semantics

- **Transitions only**: a no-change evaluation emits nothing. The previous
  result is read from the resource's status before it is overwritten — never
  from process memory — so an operator restart cannot fire a false
  `Unknown → X` transition for a check whose status already holds a result.
- **First result**: a check's first completed evaluation emits
  `ResultChanged` from `Unknown` (spec clarification Q3).
- **Bounded volume**: identical repeated events aggregate via client-go's
  event correlator (count increments, single Event object) — a check failing
  identically for 24h yields a bounded trail (SC-006).
- **Non-blocking**: recording is fire-and-forget; an unreachable event sink
  never fails or delays a reconcile (FR-012).
- **Event source**: `fathom-<kind>-controller` (e.g.
  `fathom-addoncheck-controller`) via the manager's shared broadcaster.

## RBAC

One new grant, entering via `+kubebuilder:rbac` markers and materializing in
`config/rbac/`: core `events` with `create;patch`.
