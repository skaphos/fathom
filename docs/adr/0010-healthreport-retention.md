<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# ADR 0010: HealthReports outlive their check for a bounded window

- **Status:** Accepted
- **Date:** 2026-10-20
- **Deciders:** Shawn Stratton
- **Tracking:** skaphos/fathom#262, #149; spec `specs/014-api-v1-promotion/`
- **Amends:** ADR 0002 (HealthReport as a first-class CRD), whose cleanup
  rationale ("garbage-collected with the source AddonCheck via owner
  references") this decision replaces. ADR 0002 otherwise stands.

## Context

With `spec.paused` removed (#262, ADR 0009), deleting a check is the only way
to stop it. Reports were dependents of their check through a controller
owner reference, so a deliberate stop erased the history an operator most
wants during an incident: "was this passing before?". Pause had preserved
that history as a side effect; delete must preserve it on purpose.

Three constraints shaped the design:

1. Without a finalizer the operator cannot intercept a delete, and a
   finalizer would block every check deletion whenever the operator is down,
   add six finalizer paths and an uninstall hazard.
2. The `v1` `HealthReport` schema is frozen from 0.7.0. Retention
   bookkeeping must not live in it, or a later change to retention would be
   an incompatible change to a stable kind.
3. A namespace that creates and deletes checks in a loop (a CI pipeline)
   must not accumulate history without bound; the window bounds age, not
   count.

## Decision

1. **No owner reference.** The `v1` operator writes reports with the three
   source labels (`fathom.skaphos.io/source-kind`, `source-name` and the new
   `source-uid`) and nothing else tying them to the check. The per-check
   `spec.historyLimit` prune selects on all three, so a check recreated under
   the same name neither counts nor prunes its predecessor's reports.
2. **A namespace-keyed retention sweep.** A reconciler triggered by the
   deletion of any report-writing check kind, by the creation of a report
   (which also replays every namespace at startup), by an hourly requeue and
   by a requeue timed to the earliest retained report's expiry
   lists the namespace's reports, treats a report as orphaned when no check
   with its kind, name and UID exists, stamps
   `fathom.skaphos.io/source-deleted-at` the first time (never rewriting
   it), deletes orphans whose stamp is older than the window, and then keeps
   every kind under a per-namespace cap, oldest first, live and orphaned
   alike, sparing the report each live check's status names. Each pass
   performs at most fifty writes and continues a second later when it hits
   that budget; a write the API server refuses is reported and retried
   without blocking the rest of the pass.
3. **Configuration, not schema.** `retention.report_window` (default 30
   days; zero prunes orphans on the first pass) and
   `retention.report_cap_per_kind` (default 500) are operator options under
   the normal flag, environment, config-file precedence. No `v1` kind gains
   a field.
4. **Labels and one annotation carry the bookkeeping.** This is a deliberate,
   recorded deviation from the constitution's rule that health semantics
   never live in annotations. The boundary is explicit: the verdict, the
   evidence, the observation time and every check result stay in `spec` and
   `status`; the labels and the annotation say only which check instance a
   report belonged to and when the operator first found it gone, which is
   lifecycle provenance about a deleted owner, not a health signal.
5. **Observable pruning.** Two metrics (`fathom_healthreports_retained` by
   namespace and kind; `fathom_healthreports_pruned_total` by namespace, kind
   and reason) and one Event per namespace per pass that deleted anything,
   recorded regarding the last pruned report so it lives in the affected
   namespace (an Event whose subject is cluster-scoped must live in
   `default`; an Event may reference an object that no longer exists, as Pod
   events do).

## Consequences

- `kubectl delete` of a check leaves its reports behind, listable with
  `fathomctl reports <kind>/<name>` or by label, marked as retained with the
  deletion time. After the window they are gone; nothing restores them.
- The operator role gains `patch` on `healthreports` for the stamp. It is
  the only new verb; the justification is in `docs/reference/operator-rbac.md`.
- `fathomctl uninstall` keeps custom resources, so retained reports survive
  it; `--purge-crds` removes them with the CRD.
- A namespace deleted outright takes its reports with it; the first sweep
  that finds the namespace empty drops its gauge series.

## Alternatives considered

- **A finalizer on every check kind** that strips owner references at
  delete time: blocks deletion while the operator is down, six more
  finalizer paths, an uninstall hazard, and the cap would still need a sweep.
- **Explicit `status` fields on `HealthReport`** for the check UID and the
  deletion time: freezes the retention design into the stable schema.
- **Keeping owner references and asking deleters to use orphan
  propagation**: propagation is the deleter's choice, not the operator's.
- **No count cap, window only**: unbounded growth under check churn.
