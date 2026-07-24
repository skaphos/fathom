# Contract: Runtime Clamp Signal

How the operator surfaces a stored check whose `interval`/`timeout` predates
the admission floors (or bypassed them). Consumers: `kubectl describe`,
condition-watching tooling, the e2e/envtest assertions.

## Behavior

- Effective interval = `max(spec.interval, MinCheckInterval)` when set;
  effective timeout = `max(spec.timeout, MinCheckTimeout)` when set; unset
  fields keep their existing defaults (unchanged). Every consumer of cadence —
  requeue period, run deadline, node-agent `--interval` argument — reads the
  clamped value; there is no unclamped path.
- The check **keeps running**. Clamping never sets `Accepted=False`, never
  pauses, never errors the reconcile.

## Event

| Attribute | Value |
|---|---|
| Type | `Warning` |
| Reason | `CadenceClamped` |
| Object | the AddonCheck / NodeCertificateCheck |
| Message | `spec.<field> <configured> is below the minimum <floor>; using <effective>` (one Event per clamped field) |

## Condition

| Attribute | Value |
|---|---|
| Type | `Accepted` |
| Status | `True` |
| Reason | `SpecClamped` |
| Message | same format as the Event; when multiple fields clamp, messages joined `; ` |
| ObservedGeneration | current generation |

Transitions: `SpecClamped → SpecAccepted` on the first spec update (any
update must pass the new admission floors). If reconcile-time policy problems
coexist (AddonCheck), `Accepted=False/InvalidPolicy` wins — an invalid policy
outranks a clamp notice.

## Idempotence

Re-reconciling an unchanged clamped object must not spam: the condition write
is a no-op when unchanged (apimachinery `SetStatusCondition` semantics), and
the Event is emitted through the existing recorder plumbing whose
dedup/aggregation applies. Tests assert one condition state, not N events.
