<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: HealthReport retention and the per-kind cap

## Ownership

From this release the operator writes HealthReports with no owner reference.
Deleting a check does not delete its reports. The relationship is carried by
the three `fathom.skaphos.io/source-*` labels ([api-v1.md](api-v1.md)).

## While the check lives

`spec.historyLimit` (default 10) is enforced by the check's own reconciler
after each report it writes, counting only reports whose three source labels
match the check (kind, name, **UID**). Reports left by a previous check of
the same name are neither counted nor pruned by it.

## After the check is deleted

| Step | Behaviour |
|---|---|
| orphan detection | a report is orphaned when no check with its source kind, name and UID exists |
| first observation | the operator annotates `fathom.skaphos.io/source-deleted-at` with the current time; the annotation is never rewritten |
| expiry | when `now − source-deleted-at > retention.report_window`, the report is deleted with reason `window` |
| window `0` | orphaned reports are deleted on the first pass that sees them |

Orphan detection runs on delete events of any check kind in the namespace
and on a periodic sweep, so a check deleted while the operator was down is
noticed on the next sweep; its `source-deleted-at` is then the sweep time.
A pass that leaves stamped reports behind schedules the next sweep for the
earliest expiry (at most an hour away), so the window is honoured promptly
rather than at the mercy of unrelated report traffic in the namespace.

## The per-kind cap

Independently of the window, each namespace keeps at most
`retention.report_cap_per_kind` (default 500) reports per source kind,
counting live and orphaned reports alike. When exceeded, the oldest reports
of that kind are deleted with reason `cap`. "Oldest" orders by
`spec.observedAt`, then creation time, then name.

## Bounded work

Each pass attempts at most 50 writes (stamps and deletes, refused attempts
included) and requeues after one second if work remains. A report the API server refuses to write is
reported and retried with backoff without blocking the rest of the pass. The
cap never removes the report a live check's `status.lastReportName` names,
so a namespace with more live checks of one kind than the cap stays above
it by that many. A pass never blocks a check's own reconcile.

## Observability

| Signal | Shape |
|---|---|
| `fathom_healthreports_retained` | gauge, labels `namespace`, `source_kind`: orphaned reports currently held |
| `fathom_healthreports_pruned_total` | counter, labels `namespace`, `source_kind`, `reason` (`window`, `cap`) |
| Event | one per namespace per pass that deleted anything; regarding the most recently pruned report (it no longer exists; Events may reference deleted objects), so the Event is in the affected namespace; reason `ReportsPruned`; action `Retain`; message `pruned N HealthReports (window: a, cap: b)` |

No Event is emitted per report.

## `fathomctl reports`

Lists a check's reports by kind and name as today, so a deleted check's
history remains listable. Orphaned reports show `retained (check deleted
<time>)` in the listing, and the full-report view prints the three source
labels and the deleted-at annotation.

## Uninstall

`fathomctl uninstall` keeps custom resources, so retained reports survive it.
`--purge-crds` removes them with the CRD, as it does every Fathom object.

## Configuration

See data-model.md. Both options follow the flag, env var, config file,
default precedence of the operator.
