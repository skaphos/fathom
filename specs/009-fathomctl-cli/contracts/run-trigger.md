<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: on-demand run trigger (operator side)

The trigger is the existing annotation contract, generalised to every
executable kind. It is part of the public API surface and is documented in
`docs/reference/fathomctl.md`.

## Annotation

`fathom.skaphos.io/run-now: <value>` on an `AddonCheck`, `DNSCheck`, or
`NodeCertificateCheck`. Any writer may set it; `fathomctl run` writes a
timestamp-plus-suffix value. The annotation is never removed by the
operator.

## Consumption

For every executable kind:

1. A reconcile in which the annotation is non-empty and differs from
   `status.lastRunTrigger` MUST perform a run regardless of interval.
2. On completing that run, the reconciler MUST set `status.lastRunTrigger`
   to the annotation value in the same status update that carries the run's
   verdict.
3. A reconcile with no annotation, or with an annotation equal to
   `status.lastRunTrigger`, MUST NOT run on account of the trigger and MUST
   NOT clear `status.lastRunTrigger`.
4. A paused check MUST NOT consume the trigger; it stays pending until the
   check is unpaused or deleted. The same holds for any check the operator
   cannot run (invalid policy, missing adapter, no matching nodes): the
   operator leaves the token pending and states the reason on `Ready`, and
   a waiter reports that reason rather than a timeout.
5. A value longer than the `status.lastRunTrigger` bound (253) MUST be
   ignored, never recorded.
6. Removing a consumed annotation MUST NOT cause a run or, for
   `NodeCertificateCheck`, a rollout; the template falls back to the consumed
   token.

## Kind-specific behaviour

| Kind | "Perform a run" means | Completion signal |
|---|---|---|
| AddonCheck | run the adapter now (existing) | verdict written |
| DNSCheck | evaluate every (target, resolver) pair now (already every reconcile) | verdict written |
| NodeCertificateCheck | stamp the token into the node-agent DaemonSet pod template (annotation `fathom.skaphos.io/run-now` and env `FATHOM_RUN_TRIGGER`), which rolls the agents; each agent scans on start and reports `trigger == token` | every desired node's fresh report carries the token; the verdict from those reports is written with `lastRunTrigger` |

While a NodeCertificateCheck rollout is in flight the previous verdict and
`lastRunTrigger` are retained; the `RolledOut` condition reports progress.
A report with an empty or different `trigger` never satisfies completion.

## Derived kinds

`HealthCheck` and `ClusterHealth` do not consume the annotation; the
operator ignores it on them. Propagation to their sources is performed by
the client and documented alongside the CLI.

## Status field

`status.lastRunTrigger` (string, optional, MaxLength 253) exists on all
three executable kinds. It is informational for readers and the completion
signal for waiters.

## Test obligations

- Unit (envtest or fake): for each kind, a new value forces a run; the same
  value does not re-fire; a run without an annotation preserves the stored
  value; a paused check leaves the value unconsumed.
- Node-agent: `FATHOM_RUN_TRIGGER` lands in the report; an unset variable
  yields an empty field.
- e2e: a real triggered run per kind with `lastRunTrigger` observed equal to
  the written value.
