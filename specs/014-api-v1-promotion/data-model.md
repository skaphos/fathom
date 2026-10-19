<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Data Model: Stable `v1` API for the Established Check Kinds

**Feature**: [spec.md](spec.md) | **Research**: [research.md](research.md)

## Group versions after this feature

| Group version | Kinds | Served | Storage | Maturity |
|---|---|---|---|---|
| `fathom.skaphos.io/v1` | AddonCheck, HealthCheck, ClusterHealth, HealthReport, NodeCertificateCheck, DNSCheck, NodeHealthCheck | yes | yes | stable |
| `fathom.skaphos.io/v1alpha1` | AddonDefinition, AddonDefinitionBinding | yes | yes | alpha |

No CRD serves two versions. No conversion strategy other than `None` exists.

## `v1` kinds: deltas from the alpha schema

Everything not listed is carried forward byte-for-byte in meaning
(research R7, FR-003).

| Kind | Change | Rule |
|---|---|---|
| all seven | `spec` required | `+kubebuilder:validation:Required`; `json:"spec"` |
| AddonCheck | `spec.paused` removed | unknown field at admission |
| AddonCheck | `status.conditions` is a map list | `listType=map`, `listMapKey=type`, merge patch strategy |
| AddonCheck | printer columns | Result `.status.lastResult`, Addon `.spec.addonType`, Last Run `.status.lastRunTime`, Age |
| HealthCheck | `spec.paused` removed | unknown field at admission |
| HealthCheck | `spec.checkRef.apiVersion` enumerated | `Enum=fathom.skaphos.io/v1`; empty defaults to it |
| HealthReport | printer columns | Result `.spec.result`, Source `.spec.sourceRef.kind`, Name `.spec.sourceRef.name`, Observed `.spec.observedAt`, Age |
| HealthReport | `spec.sourceRef.apiVersion` value | always `fathom.skaphos.io/v1` for reports the `v1` operator writes |
| NodeCertificateCheck | `spec.paused` removed | unknown field at admission |
| ClusterHealth, DNSCheck, NodeHealthCheck | none | version only |

Removed status surface: the `Paused` condition type on AddonCheck,
HealthCheck and NodeCertificateCheck; the `AgentReady=False/Paused` reason on
NodeCertificateCheck.

## Shared type relocation

`DefinitionLeaderEpoch` is defined in `api/v1` and referenced from
`v1.AddonCheckEvidenceAuthority.LeaderEpoch` and
`v1alpha1.AddonDefinitionBindingStatus.LeaderEpoch`. Fields unchanged; the
alpha CRD schema is unchanged (research R2).

## HealthReport identity and retention metadata

A report is identified by its source labels. The `v1` operator writes three;
the third is new.

| Label | Value | Set when |
|---|---|---|
| `fathom.skaphos.io/source-kind` | check kind | creation (existing) |
| `fathom.skaphos.io/source-name` | check name | creation (existing) |
| `fathom.skaphos.io/source-uid` | check `metadata.uid` | creation (new) |

| Annotation | Value | Set when |
|---|---|---|
| `fathom.skaphos.io/source-deleted-at` | RFC 3339 time | first retention pass that finds no check with the report's kind, name and UID |

Reports written by the `v1` operator carry **no owner reference**. Reports
created by the alpha operator do not survive the upgrade (the CRD is
replaced), so no mixed population exists.

### Report lifecycle

```text
created ──(check alive)──▶ owned: counted against spec.historyLimit of the
   │                        check whose UID matches; oldest beyond the limit
   │                        are deleted by the check's reconciler
   │
   └─(check deleted, or recreated with a new UID)──▶ orphaned:
         first pass stamps source-deleted-at
         pass finds now − source-deleted-at > window ──▶ deleted (reason window)
   any state: namespace holds > cap reports of this kind ──▶ oldest deleted (reason cap)
```

Ordering for "oldest" is `spec.observedAt`, then `metadata.creationTimestamp`,
then name, matching the existing per-check prune tie-break.

## Retention configuration

| Option | Flag | Env | Config key | Default | Bounds |
|---|---|---|---|---|---|
| report window | `--retention-report-window` | `FATHOM_RETENTION_REPORT_WINDOW` | `retention.report_window` | `720h` | `>= 0`; `0` disables retention |
| report cap per kind | `--retention-report-cap-per-kind` | `FATHOM_RETENTION_REPORT_CAP_PER_KIND` | `retention.report_cap_per_kind` | `500` | `>= 1` |

## Retention reconciler

- **Key**: namespace name.
- **Triggers**: delete events on AddonCheck, DNSCheck, NodeCertificateCheck,
  NodeHealthCheck, HealthCheck, ClusterHealth (cluster-scoped: no reports,
  excluded); a periodic resync (fixed internal interval, 1h).
- **Per pass**: list reports in the namespace; group by the three labels;
  resolve each group's check by kind and name and compare UID; stamp or
  evaluate `source-deleted-at`; enforce the per-kind cap; delete at most a
  fixed per-pass budget and requeue immediately if any work remains.
- **Outputs**: metrics `fathom_healthreports_retained{namespace,source_kind}`
  (gauge) and `fathom_healthreports_pruned_total{namespace,source_kind,reason}`
  (counter, reason `window` | `cap`); one Event per pass that deleted anything,
  on the Namespace, reason `ReportsPruned`, action `Retain`.
- **RBAC**: no new verbs (healthreports already `create;get;list;watch;delete`;
  events already `create;patch`).

## Upgrade-path and compat-gate entries

`deploy/helm/fathom-operator/upgrade-path.yaml` gains one release entry:

| Field | Value |
|---|---|
| `version` | `0.7.0` |
| `stop` | `true` |
| `sanctioned[]` | seven entries, one per promoted CRD, `path: "*"`, reason "served version replaced: v1alpha1 removed, v1 added; alpha resources are deleted with the CRD", issue skaphos/fathom#149 |

`.crd-compat-allowlist.yaml` gains the same seven `*` entries, matched by the
new `servedVersionRemoval` check (research R4).

## CLI CRD plan actions

| Action | Chosen when | Needs |
|---|---|---|
| `add` | CRD absent | nothing |
| `unchanged` | specs equal after defaulting | nothing |
| `update` | specs differ, every live stored version still served by target | sanction + flag for incompatible property findings (existing) |
| `replace` (new) | target drops a version in live `status.storedVersions` | `*` sanction and `--accept-incompatible-crds`; prints the custom-resource inventory first |

## Export verb output

One YAML document per object, `---` separated, in kind order ClusterHealth,
AddonCheck, DNSCheck, NodeCertificateCheck, NodeHealthCheck, HealthCheck
(targets before the wrappers that reference them), then namespace, then name.
Each document is the alpha object with `apiVersion: fathom.skaphos.io/v1`,
`spec.paused` removed, `spec.checkRef.apiVersion` set to `fathom.skaphos.io/v1`
when the field was present, no `status`, and metadata reduced to `name`,
`namespace`, `labels`, `annotations`.
