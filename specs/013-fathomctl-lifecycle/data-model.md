<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Data Model: fathomctl Lifecycle Verbs

No new CRD fields. The entities below are what the CLI reads, computes and
writes; field names are the ones the contracts and JSON output use.

## Install

What `fathomctl` finds in the cluster (research R6).

| Field | Type | Notes |
|---|---|---|
| `managedBy` | enum `helm` \| `kustomize` \| `olm` \| `unmanaged` \| `none` | Only `helm` is operable; others are refused by install/upgrade/uninstall (FR-006). `none` = no operator found. |
| `release` | string | Helm release name (`helm` only). |
| `namespace` | string | Release (or manager) namespace. |
| `revision` | int | Helm release revision. |
| `releaseStatus` | string | Helm status: `deployed`, `failed`, `pending-install`, `pending-upgrade`, … |
| `installedBy` | `fathomctl` \| `helm` | From the release label `fathom.skaphos.io/installed-by`; absent means `helm`. |
| `version` | semver | The release's `appVersion` (equals the chart version, lockstep). |
| `images` | map component → reference | `operator`, `probe`, `node-agent`; a reference is `repo@sha256:…` or `repo:tag`. |
| `pinning` | `digest` \| `tag` | `tag` only for development builds (R3). |
| `ready` | bool + reason | Manager Deployment `Available`. |

**Rules**: at most one Fathom release per cluster; more than one is reported by
`status` and refused by the other verbs.

## CRD set

| Field | Type | Notes |
|---|---|---|
| `name` | string | e.g. `addonchecks.fathom.skaphos.io`; nine today. |
| `scope` | `Namespaced` \| `Cluster` | `clusterhealths` and `addondefinitions` are cluster-scoped. |
| `live` | CRD or absent | From the cluster. |
| `target` | CRD | From the embedded chart's `crds/`. |
| `recordedVersion` | semver or absent | Annotation `fathom.skaphos.io/version`, stamped on every CRD `install`/`upgrade` applies (FR-015); absent on CRDs applied by `helm` or `kubectl`. |
| `state` | `current` \| `outdated` \| `missing` \| `newer` \| `unknown` | `status` compares `recordedVersion` with the operator's version; `newer` means a later release applied it. Without a recorded version: `current` only when the CLI's version equals the operator's and the live spec equals the embedded one, else `unknown` (FR-040). |

## Upgrade path declaration (`upgrade-path.yaml`)

Shipped in the chart; append-only (research R5; schema in
[contracts/upgrade-path.md](contracts/upgrade-path.md)).

| Field | Type | Notes |
|---|---|---|
| `floor` | semver | Oldest version the recorded data covers (`0.5.0`); an install below it is refused `below-floor` (FR-027). |
| `releases[].version` | semver | One entry per release that sanctions a change or is a stop. |
| `releases[].stop` | bool | `true`: an upgrade must not skip this version. |
| `releases[].sanctioned[]` | list of `{crd, path, reason, issue}` | Incompatible CRD changes this release deliberately makes; `path` uses crdify's notation, `*` for the whole CRD. |

**Validation**: versions strictly increasing; entries never removed or edited
once released (gate); every `SANCTIONED` crd-compat finding for the upcoming
release has a matching `sanctioned` entry (gate).

## Upgrade plan

Computed from Install + CRD set + declaration for A (installed) → B (CLI).

| Field | Type | Notes |
|---|---|---|
| `from`, `to` | semver | `to` is always the CLI's version. |
| `path` | `direct` or ordered list of stop versions | Stops S with A < S < B. |
| `crds[]` | `{name, action, findings[]}` | `action`: `add` \| `update` \| `unchanged` \| `refuse`. |
| `crds[].findings[]` | `{check, path, message, sanctionedBy?}` | crdify findings; `sanctionedBy` names the release + issue when matched. |
| `refusals[]` | `{reason, detail, next}` | `reason`: `below-floor`, `stop-skipped`, `crd-downgrade`, `dev-version`, `scope-change`, `unsanctioned-incompatible`, `acknowledgement-required`, `downgrade`, `not-installed`, `not-helm`, `multiple-installs`, `release-pending`, `missing-permissions`. `next` is the exact next command. |
| `operator` | `{fromImages, toImages}` | Digest or tag references. |
| `values` | map | Effective values after reuse + overrides (secrets are not printed). |

**State transitions** (upgrade):
`planned` → (`--dry-run`: stop) → `values-validated` → `crds-applied` →
`rolling-out` → `available` (exit 0) | `timed-out` → (`--rollback-on-failure`
and no incompatible change: `rolled-back`) → exit 1. CRDs never transition back.

## Operator-created objects

Removed by every `uninstall` (research R7); never written by the CLI.

| Kind | Scope | Selector | Owner |
|---|---|---|---|
| DaemonSet, ServiceAccount, NetworkPolicy, Role, RoleBinding | check namespace | `fathom.skaphos.io/managed-by=fathom` | the check |
| ConfigMap (node reports) | check namespace | `fathom.skaphos.io/managed-by=fathom` | — |
| Pod (probes) | check namespace or `kube-system` | `fathom.skaphos.io/managed-by=fathom` | check, or none for addon probes |
| ValidatingAdmissionPolicy + Binding | cluster | name `fathom-node-report-authenticity` | none |
| Lease | operator namespace | name `2d3dbc4f.skaphos.io` | none |

## Purge export

| Field | Notes |
|---|---|
| file | One multi-document YAML, written atomically before any deletion. |
| order | `AddonDefinition`, `AddonDefinitionBinding`, `AddonCheck`, `DNSCheck`, `NodeCertificateCheck`, `NodeHealthCheck`, `HealthCheck`, `ClusterHealth`, `HealthReport`. |
| stripped | `metadata.uid`, `resourceVersion`, `managedFields`, `creationTimestamp`, `generation`, every `ownerReferences` entry (all owners are deleted by the purge; a stale owner UID would make the garbage collector delete the restored object — `HealthReport`s are owned by their checks); `status`. |
