<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Implementation Plan: Stable `v1` API for the Established Check Kinds

**Branch**: `feature/149-api-v1-promotion` | **Date**: 2026-10-18 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/014-api-v1-promotion/spec.md`

## Summary

Author `api/v1` as the single served and stored version of the seven check
kinds (`AddonCheck`, `HealthCheck`, `ClusterHealth`, `HealthReport`,
`NodeCertificateCheck`, `DNSCheck`, `NodeHealthCheck`), carrying every alpha
hardening forward and folding in the remaining review findings: required
`spec`, map-list conditions and printer columns on `AddonCheck`, printer
columns on `HealthReport`, an enumerated `checkRef.apiVersion`, and the
removal of `spec.paused`. `v1alpha1` keeps only the two runtime definition
kinds. Because the API server refuses to drop a stored version in place, the
CLI's CRD plan gains a `replace` action that inventories the alpha resources,
requires the sanctioned `*` entry plus `--accept-incompatible-crds`, then
deletes and recreates each promoted CRD; a new read-only `fathomctl export`
renders the alpha checks as `v1` manifests beforehand. Deleting a check no
longer deletes its reports: `v1` reports carry a source-UID label instead of
an owner reference, a namespace-keyed retention reconciler stamps orphans
with a deleted-at annotation, prunes them after a 30-day window, enforces a
per-namespace per-kind cap of 500, and reports through two metrics and one
Event per pass. The compat gate learns to flag served-version removal so the
swap is a sanctioned finding and `v1` has an automated backstop. Two ADRs
record the single-version policy and the retention change.

## Technical Context

**Language/Version**: Go 1.27.2 (`go.mod`)

**Primary Dependencies**: existing only: controller-runtime v0.25.x,
apimachinery/client-go v0.37.x, controller-gen v0.22.0 (pinned in `tools/`),
crd-ref-docs, crdify v0.6.0 (CLI and gate), Helm v4 library (CLI), cobra,
viper. No new modules (research R1-R16).

**Storage**: CRDs at `fathom.skaphos.io/v1` (seven) and `v1alpha1` (two);
HealthReport retention state as labels and one annotation on the report; two
operator options in the existing config model. No new kinds or fields for
retention.

**Testing**: envtest (Ginkgo) for controllers including the retention
reconciler; stdlib `testing` for `api/v1` CEL constants, `internal/app`
options, `internal/cli` (fake client, Helm memory driver, CRD plan with
`storedVersions` fixtures) and the gate scripts; `samples_admission_test.go`
and `crd_kustomization_test.go` over the renamed samples and CRD list; e2e:
core shard on the `v1` samples plus `crdvalidation` assertions, and a new
`upgrade` scenario in the `lifecycle` shard that installs the published
0.6.0 release.

**Target Platform**: the operator, probe and node-agent images (schema only,
no image content change) and the six `fathomctl` archives.

**Project Type**: single Go module, kubebuilder v4 operator plus CLI.

**Performance Goals**: a retention pass over a namespace at the cap (500
reports per kind) completes in one bounded reconcile with a fixed delete
budget; no change to check reconcile latency; `fathomctl export` on a cluster
with a few hundred checks finishes within the global request timeout.

**Constraints**: `v1` frozen from 0.7.0; no conversion webhook; no new RBAC
verbs; `internal/cli` import boundary unchanged; `ClusterHealth` derivation
unchanged; upgrade-path file append-only; every generated artifact produced
by the pinned tasks; DCO on every commit; `feat!:` on the breaking commit.

**Scale/Scope**: one new API package (seven kinds plus five helper files);
one struct relocated; ~45 non-test and ~80 test files switch import; three
pause fields and ~20 code sites removed; one shared prune helper replacing
four; one new reconciler, two metrics, one Event; two options; one CLI verb,
one CRD plan action, one `status` line; one gate check; eighteen samples
renamed; seven `PROJECT` entries, the CSV, seven allowlist and seven
upgrade-path entries; two ADRs; eight docs pages; one e2e scenario.

## Constitution Check

*GATE: passed before Phase 0 and re-checked after Phase 1.*

| Principle or constraint | Design evidence | Result |
|---|---|---|
| I. Explicit state | Health intent and verdicts stay in `v1` spec and status; the stable promise is a declared, gated contract. Retention bookkeeping uses labels and one annotation on the report: this is lifecycle metadata about a deleted owner (which check, when gone), not a health semantic, and the verdict and evidence remain in spec. See Complexity Tracking. | Justified |
| II. Git as desired-state boundary | The alpha-to-`v1` move is an export the adopter commits, then applies; nothing is recreated by the tool. Sanctions and stops live in committed, reviewed files. | Pass |
| III. Deterministic operation | All artifacts regenerate from `api/` via pinned tasks and the drift gate; the export orders deterministically; the CRD plan is printed before any change. | Pass |
| IV. Kubernetes-native | A proper GA CRD version, admission rules in the schema, retention through labels and the API, pruning visible as Events and metrics. | Pass |
| V. Compose, don't trap | No new dependency; adopters keep `helm` and `kubectl` paths with the same documented procedure. | Pass |
| VI. Explainable | `replace` prints the inventory and counts; every refusal names the next command; each prune pass leaves an Event with counts and reasons. | Pass |
| VII. Read-only degradation | Retained reports stay readable through `fathomctl reports` and plain label selectors even when the operator is down; `export` and `status` are read-only. | Pass |
| IX. Honest scope | The versioning reference states what is stable, what is alpha, that the adapter contract is independent, and that alpha resources do not survive. | Pass |
| ClusterHealth contract | Version changes, derivation does not (FR-021). | Pass |
| Bounded, idempotent reconciliation | Retention pass has a delete budget and requeues; per-check prune is unchanged in cost; no finalizers. | Pass |
| Minimal RBAC | No new verbs; healthreports delete and events create already granted. | Pass |
| Configuration model | Two options through `Options` and `bindings()`, plus a duration binding kind added to the same table. | Pass |
| CRD API versioning standard | Alpha removal has a zero support window; `v1` is a new version introduced for maturity graduation; the standard's "scaffold alongside, convert, migrate" procedure is not followed because no version is kept served. ADR 0009 records this as the sanctioned alpha-churn path, not a deviation. | Pass |
| Documentation standard | Generated reference drift-gated; two ADRs for the hard-to-reverse choices; the versioning mirror re-synced from the canonical standard with the Fathom row updated. | Pass |
| Adopt before build | crdify, controller-gen, crd-ref-docs and Helm reused; the only new mechanism (retention sweep) has no prior art in the toolchain. | Pass |

Post-design re-check: the data model, contracts and quickstart keep every
gate. The single justified item is carried in Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/014-api-v1-promotion/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── api-v1.md                 # promises, admission behaviour, columns, labels, axes
│   ├── cli-upgrade-export.md     # export verb, replace action, status line
│   └── retention.md              # ownership, window, cap, observability
├── checklists/requirements.md
└── tasks.md                      # created by speckit-tasks, not this plan
```

### Source Code (repository root)

```text
api/
├── v1/                                   # NEW package: the seven kinds + helpers
│   ├── groupversion_info.go              # GroupVersion fathom.skaphos.io/v1, AddToScheme
│   ├── addoncheck_types.go               # no paused; conditions map-list; printcolumns; spec required
│   ├── healthcheck_types.go              # no paused; checkRef.apiVersion enum; spec required
│   ├── clusterhealth_types.go / dnscheck_types.go / nodehealthcheck_types.go
│   ├── nodecertificatecheck_types.go     # no paused
│   ├── healthreport_types.go             # printcolumns; spec required + immutable
│   ├── leaderepoch.go                    # DefinitionLeaderEpoch (moved, research R2)
│   ├── validation.go / aggregate.go / annotations.go / cadence.go   # moved; annotations gains source-uid, source-deleted-at
│   ├── zz_generated.deepcopy.go          # generated
│   └── *_test.go                         # moved CEL/constant tests
└── v1alpha1/                             # keeps AddonDefinition, AddonDefinitionBinding, payloads
    └── addondefinitionbinding_types.go   # LeaderEpoch typed as v1.DefinitionLeaderEpoch

internal/app/
├── run.go                                # register both AddToScheme; retention reconciler setup
├── options.go                            # Retention{ReportWindow, ReportCapPerKind}; duration binding kind
└── runtime_wiring.go                     # v1.DefinitionLeaderEpoch

internal/controller/
├── healthreport_history.go               # NEW shared prune helper (three-label filter) replacing four copies
├── healthreport_retention_controller.go  # NEW namespace-keyed sweep: orphans, window, cap, metrics, Event
├── addoncheck_controller.go              # no Paused; no owner ref; source-uid label; shared prune
├── healthcheck_controller.go             # no Paused; registry/default/watch at v1
├── nodecertificatecheck_controller.go    # reconcilePaused removed; revokeNodeCertAgent kept
├── dnscheck_controller.go / nodehealthcheck_*.go   # no owner ref; source-uid; shared prune
└── *_test.go                             # import switch; new retention envtest specs

internal/metrics/metrics.go               # fathom_healthreports_retained, fathom_healthreports_pruned_total

internal/cli/
├── export.go (+_test)                    # NEW read-only verb (contracts/cli-upgrade-export.md)
├── lifecycle_crds.go                     # replace action: storedVersions check, inventory, delete+create
├── lifecycle_cleanup.go                  # served-version discovery replaces compiled alpha GV
├── upgrade.go / install.go / status.go   # plan rendering for replace; "upgrade required" line
├── kinds.go / run.go / wait.go / describe.go / reports.go   # v1 types; Paused removed; retained marker
└── root.go                               # AddCommand(export)

deploy/helm/fathom-operator/
├── crds/                                 # synced v1 CRDs (helm:sync)
├── upgrade-path.yaml                     # 0.7.0 stop + seven "*" sanctions
└── values.yaml / values.schema.json      # retention.reportWindow, retention.reportCapPerKind -> config

config/
├── crd/bases/                            # regenerated (same file names)
├── samples/fathom_v1_*.yaml              # renamed + kustomization
├── manifests/bases/fathom.clusterserviceversion.yaml   # owned at v1, missing kinds added
└── rbac/role.yaml                        # regenerated, no verb change

scripts/
├── check-crd-compat.sh                   # servedVersionRemoval classifier (path "^")
├── e2e-shards.sh / e2e_shards_gate_test.go / check-version-lockstep.sh   # sample paths
└── upgrade_path_gate_test.go             # unchanged; now matches seven "*" entries

.crd-compat-allowlist.yaml                # seven "*" entries, issue #149
PROJECT                                   # seven entries at v1
Taskfile.yml                              # docs:api-ref source "api"; test:api adds ./api/v1; samples task paths

docs/
├── adr/0009-stable-v1-single-served-version.md     # NEW
├── adr/0010-healthreport-retention.md              # NEW (amends 0002)
├── reference/api.md                      # generated, both group versions
├── reference/api-versioning.md           # 1.0 intent; per-kind table; contract axis independent
├── reference/fathomctl.md                # export; replace; status line; no paused
├── guides/monitoring.md                  # metric rows; no paused
├── guides/*.md, README.md, architecture.md, code-map.md   # v1 strings; delete-as-stop guidance
RELEASE.md                                # "## v1 API migration" before Rollback / Fix Forward

test/e2e/
├── fixtures/*.yaml                       # apiVersion v1 (older-crds baseline untouched)
├── crdvalidation_test.go                 # required spec, enum, SSA conditions, columns, alpha rejected
├── fathomctl_lifecycle_test.go           # upgrade scenario from published 0.6.0
└── *_test.go                             # sample paths, v1 strings
```

**Structure Decision**: one new API package under the existing kubebuilder
layout; everything else is edits in place. Retention lands as one new
controller file plus one shared helper so the four check reconcilers shrink
rather than grow.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Retention bookkeeping in labels and one annotation (constitution I: "health semantics MUST NOT be encoded in annotations") | The clarified decision keeps the `v1` `HealthReport` schema free of retention fields so a later change to retention is not a `v1` change. The data recorded (owner kind, name, UID, time the owner was found gone) is lifecycle provenance; verdict, evidence and timestamps stay in spec. | Explicit `status` fields would freeze the retention design into the stable schema; owner references cannot express "deleted but kept"; a finalizer blocks deletion whenever the operator is down. |

## Delivery sequencing

Four pull requests on the feature branch, each green on its own (research
R16): preparation refactors; the `api/v1` package with all switches and
generated artifacts (`feat!:`); upgrade `replace` plus `export` plus the e2e
upgrade scenario; retention with metrics, Event, options and ADR 0010.
`/speckit-tasks` phases follow this order.

## Open items for tasks

- Confirm the viper decode hook parses `time.Duration` from config strings
  before adding the duration binding; otherwise parse in `Validate`.
- Confirm controller-gen v0.22.0 emits the alpha binding CRD unchanged when
  `LeaderEpoch` is typed from another package (expected; `verify-generated`
  proves it).
- The lifecycle e2e scenario pulls 0.6.0 from GHCR; add a skip with a clear
  message when the registry is unreachable, so local runs without network
  fail honestly rather than silently.
