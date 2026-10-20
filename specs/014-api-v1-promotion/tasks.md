<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Tasks: Stable `v1` API for the Established Check Kinds

**Input**: Design documents from `specs/014-api-v1-promotion/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Included. The constitution requires direct coverage for new behaviour and a regression test per bug class, and spec FR-022 makes a real-cluster e2e run a gate.

**Organization**: Phases follow the user stories in spec priority order. The plan's four-PR delivery sequence (research R16) maps onto the phases as noted in Implementation Strategy; tasks inside a phase are ordered so the phase is green on its own.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependency on an unfinished task)
- **[Story]**: US1 stable API, US2 upgrade, US3 delete-as-stop and retention, US4 schema enforcement, US5 artifacts and docs
- Every task names its files. Paths are repository-relative.

## Path Conventions

Single Go module, kubebuilder v4 layout: `api/`, `internal/`, `cmd/`, `config/`, `deploy/helm/fathom-operator/`, `test/e2e/`, `scripts/`, `docs/`.

---

## Phase 1: Setup

**Purpose**: Confirm the baseline and the tooling the later phases rely on.

- [X] T001 Run `go -C tools tool task ci` and `go -C tools tool task crd-compat` on the branch base and record both green in `specs/014-api-v1-promotion/checklists/baseline.md` (date, commit, toolchain versions from `tools/go.mod`)
- [X] T002 [P] Confirm the viper decode hook in `internal/app/options.go` (`v.Unmarshal(&opts, weaklyTyped)` near line 440) parses `time.Duration` from config strings; note the result in `specs/014-api-v1-promotion/research.md` under R6
- [X] T003 [P] Confirm `go -C tools tool task docs:api-ref` with `--source-path=api` renders both group versions into one `docs/reference/api.md` on a scratch run (do not commit the output yet); note the result in `specs/014-api-v1-promotion/research.md` under R12

---

## Phase 2: Foundational (behaviour-neutral preparation, PR 1)

**Purpose**: Refactors every story depends on. No schema or behaviour change; `verify-generated` and `crd-compat` stay green with no new allowlist entries.

- [X] T004 Create `internal/controller/healthreport_history.go` with one `pruneHealthReportHistory(ctx, client, namespace, labels map[string]string, limit int32, protect string)` helper: list by labels, sort by `spec.observedAt` then `metadata.creationTimestamp` then name, delete the oldest beyond `limit`, protect the named report, log but do not return delete errors; keep `defaultHealthReportHistoryLimit = 10`
- [X] T005 Replace the four prune implementations with calls to the helper: `(*AddonCheckReconciler).pruneHealthReportHistory` in `internal/controller/addoncheck_controller.go`, `pruneDNSHealthReports` in `internal/controller/dnscheck_controller.go`, `pruneNodeCertHealthReports` in `internal/controller/nodecertificatecheck_helpers.go`, `pruneNodeHealthHealthReports` in `internal/controller/nodehealthcheck_helpers.go`; keep every existing call site
- [X] T006 Add `internal/controller/healthreport_history_test.go` (envtest, Ginkgo) covering ordering, the protected name, limit < 1 early return, and that existing prune specs in `internal/controller/addoncheck_prune_test.go` still pass unchanged
- [X] T007 [P] Add a duration binding kind to `internal/app/options.go`: `isDuration bool` and `durationDef time.Duration` on `flagBinding`, a `fs.Duration` case in `RegisterFlags`, and the matching case in `Load`; no option uses it yet
- [X] T008 [P] Add table tests for the duration kind in `internal/app/options_test.go`: flag beats env beats config file beats default, and `0` round-trips
- [X] T009 [P] Extend the classifier in `scripts/check-crd-compat.sh` with a `servedVersionRemoval` check: compare `spec.versions[].name` of baseline and target per CRD; a version served at the baseline and absent from the target is reported on path `^` with that check name and is sanctioned only by an allowlist entry whose `path` is `*`; `INCOMPATIBLE` otherwise
- [X] T010 [P] Add `scripts/check_crd_compat_test.go` driving the script through `CRD_COMPAT_OLD_DIR` with two fixture CRDs under `scripts/testdata/crd-compat/` (one version swap sanctioned by `*`, one unsanctioned) and asserting the `SANCTIONED ^ (servedVersionRemoval)` line and the exit code
- [X] T011 [P] In `internal/cli/lifecycle_cleanup.go` replace the compiled-in `fathomv1alpha1.GroupVersion` in `fathomResources` and `refuseUnknownKinds`/`servesVersion` with the served version read from each live CRD (`spec.versions` where `served: true`), keeping the purge order in `purgeKinds`; add cases to `internal/cli/lifecycle_cleanup_test.go` for a CRD serving a different version than compiled
- [X] T012 Run `go -C tools tool task ci` and `go -C tools tool task crd-compat`; both green with no allowlist change. Commit as PR 1 (`refactor:` prefix, DCO)

**Checkpoint**: Foundation ready. Stories may start; US1 must precede US2, US3-retention and US4 because they edit the `api/v1` files it creates.

---

## Phase 3: User Story 1 - Adopt a stable health API (Priority: P1) 🎯 MVP

**Goal**: `fathom.skaphos.io/v1` is the only served and stored version of the seven kinds; alpha hardening carried forward unchanged; alpha manifests for promoted kinds are rejected; `ClusterHealth` derivation unchanged.

**Independent Test**: Fresh install, apply the `v1` samples for all seven kinds, each reaches a verdict; a `v1alpha1` manifest for a promoted kind is rejected with "no matches for kind" (quickstart §2).

### Tests for User Story 1

- [X] T013 [P] [US1] Add `api/v1/groupversion_info_test.go` asserting `GroupVersion` is `fathom.skaphos.io/v1` and that `AddToScheme` registers exactly the seven kinds and their lists (mirror `api/v1alpha1/groupversion_info_test.go`)
- [X] T014 [P] [US1] Extend `test/e2e/crdvalidation_test.go` with a spec that applies an `apiVersion: fathom.skaphos.io/v1alpha1` AddonCheck and expects the server error to contain `no matches for kind`
- [X] T015 [P] [US1] Add a spec to `test/e2e/clusterhealth_staleness_test.go` (or a new `test/e2e/clusterhealth_v1_test.go`) asserting a `v1` ClusterHealth's result changes only when a selected HealthCheck's status changes, never when HealthReports are added or removed

### Implementation for User Story 1

- [X] T016 [US1] Create `api/v1/groupversion_info.go` with `GroupVersion = schema.GroupVersion{Group: "fathom.skaphos.io", Version: "v1"}`, the same private `builder` wrapper as `api/v1alpha1/groupversion_info.go`, `SchemeBuilder` and `AddToScheme`, and the `+kubebuilder:object:generate=true` / `+groupName=fathom.skaphos.io` markers
- [X] T017 [US1] Move `api/v1alpha1/{addoncheck,healthcheck,clusterhealth,healthreport,nodecertificatecheck,dnscheck,nodehealthcheck}_types.go` to `api/v1/` with `package v1`, every existing marker, default, CEL rule, bound and `+kubebuilder:storageversion` intact; keep `init()` registration; delete the alpha files
- [X] T018 [US1] Move `api/v1alpha1/{validation.go,aggregate.go,annotations.go,cadence.go}` and their tests (`validation_test.go`, `aggregate_test.go`, `dnscheck_validation_test.go`, `nodehealthcheck_validation_test.go`, `healthreport_types_test.go`, `deepcopy_test.go` entries for the seven kinds) to `api/v1/`; delete the alpha copies
- [X] T019 [US1] Create `api/v1/leaderepoch.go` holding `DefinitionLeaderEpoch` (moved verbatim from `api/v1alpha1/addondefinitionbinding_types.go`); change `api/v1alpha1.AddonDefinitionBindingStatus.LeaderEpoch` to `*fathomv1.DefinitionLeaderEpoch`; keep `api/v1alpha1/addondefinitionbinding_types_test.go` passing
- [X] T020 [US1] Run `go -C tools tool task generate` to produce `api/v1/zz_generated.deepcopy.go` and regenerate `api/v1alpha1/zz_generated.deepcopy.go`; commit both
- [X] T021 [US1] Register both schemes: `internal/app/run.go` `NewScheme` adds `fathomv1.AddToScheme` beside `fathomv1alpha1.AddToScheme` with the error text updated; `internal/cli/client.go` likewise
- [X] T022 [US1] Switch every non-test importer of the seven kinds to `fathomv1 "github.com/skaphos/fathom/api/v1"`: all files in `internal/controller/` (16), `internal/cli/` (12), `internal/adapter/runtime/` (4), `internal/app/` (3), `internal/adapter/declarative/` (2), `internal/adapter/impersonation/` (2), `internal/adapter/rbacgen/` (1), `internal/metrics/metrics.go`, `internal/nodehealth/types.go`; files that also use definition types keep both imports. `addondefinition_controller.go:368` and `addoncheck_runtime.go:565` keep `fathomv1alpha1.GroupVersion.Version` as the runtime SchemaVersion
- [X] T023 [US1] Retype `internal/app/runtime_wiring.go` (`adoptLease`, lines ~383 and ~458) and `internal/controller/addoncheck_runtime.go` (lines ~164, ~167, ~241) to `*fathomv1.DefinitionLeaderEpoch`
- [X] T024 [US1] In `internal/controller/healthcheck_controller.go` move `normalizeHealthCheckTarget` default, `newHealthCheckTargetRegistry` entries, `healthChecksForTarget` matching and `SetupWithManager` watches to `fathomv1.GroupVersion`; update the `UnsupportedAPIVersion` message to name `fathom.skaphos.io/v1`
- [X] T025 [US1] Set `Spec.SourceRef.APIVersion` to `fathomv1.GroupVersion.String()` in the five report builders: `healthReportForAddonCheck` and `runtimeHealthReportForAddonCheck` in `internal/controller/addoncheck_controller.go`, `healthReportForDNSCheck` in `internal/controller/dnscheck_controller.go`, `healthReportForNodeCert` in `internal/controller/nodecertificatecheck_helpers.go`, `healthReportForNodeHealth` in `internal/controller/nodehealthcheck_helpers.go`
- [X] T026 [US1] Switch every test importer (the ~80 `_test.go` files under `internal/`, `pkg/`, `test/e2e/`, `scripts/`) and every literal `fathom.skaphos.io/v1alpha1` for a promoted kind to `v1`; leave definition-kind literals and `test/e2e/fixtures/lifecycle/older-crds/` untouched
- [X] T027 [US1] Update the seven `PROJECT` resource entries to `version: v1` and `path: github.com/skaphos/fathom/api/v1`; leave the two definition entries
- [X] T028 [US1] Run `go -C tools tool task manifests` and `go -C tools tool task helm:sync`; commit `config/crd/bases/*.yaml`, `config/rbac/role.yaml`, `deploy/helm/fathom-operator/crds/*.yaml`, `deploy/helm/fathom-operator/files/manager-rules.yaml`; confirm each promoted CRD lists exactly one version `v1` with `served: true, storage: true` and the two definition CRDs are byte-identical to before
- [X] T029 [US1] Rename `config/samples/fathom_v1alpha1_*.yaml` (18 files) to `fathom_v1_*.yaml` with `apiVersion: fathom.skaphos.io/v1`; update `config/samples/kustomization.yaml`; update the path references in `Taskfile.yml` (`e2e:cluster:samples`), `scripts/e2e-shards.sh` (lines ~114-115), `scripts/e2e_shards_gate_test.go`, `scripts/check-version-lockstep.sh` (line ~110), `test/e2e/fixtures/README.md`, and the thirteen e2e specs that apply samples by path
- [X] T030 [US1] Update `test/e2e/fixtures/nodecertificatecheck.yaml`, `nodehealthcheck.yaml`, `nodehealthcheck-fail.yaml` to `apiVersion: fathom.skaphos.io/v1`
- [X] T031 [US1] Add seven entries to `.crd-compat-allowlist.yaml` (`crd: <name>`, `path: "*"`, reason "served version replaced: v1alpha1 removed, v1 added; alpha resources are deleted with the CRD", `issue: https://github.com/skaphos/fathom/issues/149`); add the `0.7.0` release to `deploy/helm/fathom-operator/upgrade-path.yaml` with `stop: true` and the same seven `*` sanctions (contract: data-model.md "Upgrade-path and compat-gate entries")
- [X] T032 [US1] Update `Taskfile.yml`: `docs:api-ref` `--source-path="$root/api"` and its description; `test:api` runs `./api/v1 ./api/v1alpha1 ./pkg/addondefinition`
- [X] T033 [US1] Write `docs/adr/0009-stable-v1-single-served-version.md` (adr skill format): context (alpha zero support window, no conversion infra, immutable scope), decision (`v1` only served and stored; alpha replaced, not converted; three version axes with the contract independent), consequences (replace action, stop, export), status Accepted
- [X] T034 [US1] Run `go -C tools tool task verify-generated`, `test`, `crd-compat` (expect seven `SANCTIONED ^ (servedVersionRemoval)` lines, zero `INCOMPATIBLE`), `lint`, `staticcheck`; then `go -C tools tool task test-e2e E2E_ADDONS=fathom`

**Checkpoint**: `v1` served alone; alpha rejected; all generated artifacts in sync. This is the MVP but ships together with US3-pause, US4 and US5 as PR 2 because they edit the same `api/v1` files (see Implementation Strategy).

---

## Phase 4: User Story 2 - Upgrade an existing installation without stranding resources (Priority: P1)

**Goal**: `fathomctl upgrade` inventories alpha resources, stops, and with explicit acceptance replaces each promoted CRD; `fathomctl export` renders alpha checks as `v1` manifests; `status` names an outstanding upgrade; the plain-manifest path is documented in the same terms. Contract: `contracts/cli-upgrade-export.md`.

**Independent Test**: quickstart §3 against a published 0.6.0 install in the `lifecycle` e2e shard.

### Tests for User Story 2

- [X] T035 [P] [US2] Add `internal/cli/export_test.go` (fake client with unstructured alpha objects and CRD fixtures): renders six kinds in contract order; rewrites `apiVersion`, drops `spec.paused`, rewrites `spec.checkRef.apiVersion`, drops `status` and server metadata; `nothing-to-export` exit 1 when no CRD serves `v1alpha1`; a decode failure is reported on stderr and exit is 1 while other objects are still written; `--output` writes atomically; `--namespace` filters
- [X] T036 [P] [US2] Add cases to `internal/cli/lifecycle_crds_test.go` and `internal/cli/upgrade_test.go`: a live CRD with `status.storedVersions: [v1alpha1]` and a `v1`-only target yields action `replace`; refused `unsanctioned-incompatible` without a `*` entry; with the entry and an empty inventory it proceeds without the flag and prints `0 objects will be removed`; `acknowledgement-required` with the entry, a non-empty inventory and no flag, and the plan output contains the inventory lines and count; with the flag the fake records delete, wait, create, `Established`; a CRD gone at delete time is treated as replaced; `--dry-run` prints the inventory and changes nothing; `--rollback-on-failure` with `replace` is a usage refusal
- [X] T037 [P] [US2] Add a case to `internal/cli/status_test.go`: a promoted CRD serving `v1alpha1` prints the `upgrade required: ... run fathomctl export, then fathomctl upgrade` line
- [X] T038 [US2] Add the `upgrade` scenario to `test/e2e/fathomctl_lifecycle_test.go` per quickstart §3 steps 1-7: `helm install` 0.6.0 from `oci://ghcr.io/skaphos/charts/fathom-operator` with published images, apply the 0.6.0 alpha samples (checked in under `test/e2e/fixtures/lifecycle/v0.6.0-samples/`), wait for reports, build the scenario's CLI at exactly the newest `releases[].version` in `deploy/helm/fathom-operator/upgrade-path.yaml` (the 0.7.0 stop; research R14 explains why `<appVersion>-e2e.N` cannot be used), run its `export`, `upgrade` (expect refusal with inventory), `upgrade --accept-incompatible-crds`, apply the export, assert verdicts, `storedVersions == [v1]`, second `export` exits 1, `status` clean; skip with a clear message when GHCR is unreachable

### Implementation for User Story 2

- [X] T039 [US2] In `internal/cli/lifecycle_crds.go` add the `replace` action: `planCRDs` compares target `spec.versions` names with live `status.storedVersions`; `crdPlan` gains `Replace bool`, `Inventory []objectRef` and `InventoryErr error`; the inventory lists each stored-version resource as unstructured at the live served version (namespaced kinds across all namespaces; cluster-scoped kinds by name)
- [X] T040 [US2] In `internal/cli/upgrade.go` `buildUpgradePlan`: treat the CRD-level `storedVersionRemoval` finding and `Replace` as sanctionable only by a `*` entry; refuse `unsanctioned-incompatible` otherwise; refuse `acknowledgement-required` without `--accept-incompatible-crds` only when the inventory is non-empty (an empty inventory needs the `*` sanction alone and prints `0 objects will be removed`); render the inventory block (`replace crd/<name> (<old> -> <new>): N objects will be removed`, one indented `ns/name` per object, trailing `and their HealthReport history`) in the plan output and in `--dry-run`; make `--rollback-on-failure` plus `replace` a usage refusal
- [X] T041 [US2] In `internal/cli/lifecycle_crds.go` implement `replaceCRD`: delete the live CRD, poll until not found (bounded by `--timeout`), create the target via the existing `applyCRD` create path, `waitEstablished`; print `replaced crd/<name>: removed N objects`; treat not-found at delete as already replaced
- [X] T042 [US2] In `internal/cli/install.go` (install over leftover CRDs, no release present) refuse a leftover CRD that stores a version the target drops as `leftover-stored-version` with `next: fathomctl export --output <file>; kubectl delete crd <names>; fathomctl install` (leftover CRDs carry no recorded version to sanction against; contract cli-upgrade-export.md); add the case to `internal/cli/install_test.go`
- [X] T043 [US2] Create `internal/cli/export.go`: `newExportCommand(f *factory)`, flags `--output` and `--namespace`, CRD discovery of the served version for the six authorable kinds, unstructured list per kind in contract order, render (apiVersion, `spec.paused` removal, `spec.checkRef.apiVersion` rewrite, `status` removal, `sanitizeForExport` plus the `last-applied-configuration` annotation), stderr reporting per failed object, exit codes per contract; refactor `exportResources` in `internal/cli/lifecycle_cleanup.go` to write to an `io.Writer` and reuse it
- [X] T044 [US2] Register the verb in `internal/cli/root.go` `AddCommand(...)`; add `export` to the help ordering test in `internal/cli/root_test.go`
- [X] T045 [US2] In `internal/cli/status.go` add the per-CRD served-version check and the `upgrade required` line
- [X] T046 [US2] Add the `upgrade` scenario wiring: `Taskfile.yml` `e2e:cluster:lifecycle` also tags and loads the locally built operator, probe and node-agent images under the newest `upgrade-path.yaml` release version (read with the same awk pattern as `E2E_LIFECYCLE_VERSIONS`) so the stop-version CLI finds its images; kind pulls the 0.6.0 images from GHCR; `scripts/e2e-shards.sh` `shard_for_file` keeps `fathomctl_lifecycle_test.go` in `lifecycle`; `test/e2e/fixtures/README.md` documents the GHCR dependency, the stop-version pinning and the skip behaviour
- [X] T047 [US2] Document in `docs/reference/fathomctl.md`: new `## export` section beside `ls`; `replace` row under Lifecycle verbs → CRD handling; the inventory block; the new refusal reason row; the `status` line; the `--rollback-on-failure` interaction
- [X] T048 [US2] Add `## v1 API migration` to `RELEASE.md` before `## Rollback / Fix Forward`: what changed, pre-upgrade `fathomctl export`, the `upgrade` stop and acceptance, the plain-manifest sequence (export, delete CRs, delete the seven CRDs, apply), that alpha resources and report history do not survive, that `spec.paused` is rejected only under strict field validation and pruned otherwise (so manifests must go through `fathomctl export`), verification, rollback (reinstall 0.6.0 and re-apply the exported alpha manifests from Git); mirror the sequence in `docs/guides/getting-started.md` upgrade notes
- [X] T049 [US2] Run `go -C tools tool task test lint staticcheck` and `go -C tools tool task test-e2e E2E_ADDONS=lifecycle`

**Checkpoint**: A 0.6.0 cluster upgrades to `v1` with inventory, consent and export, and the same procedure is documented for manifests.

---

## Phase 5: User Story 3 - Stop a check by deleting it (Priority: P2)

**Goal**: No pause surface anywhere; deletion is the only stop; reports survive deletion for the window, carry identity and deletion time, are bounded by the per-kind cap, and pruning is observable. Contracts: `contracts/retention.md`, data-model.md "HealthReport identity and retention metadata".

**Independent Test**: quickstart §4 and §5.

### Tests for User Story 3

- [X] T050 [P] [US3] Add to `test/e2e/crdvalidation_test.go`: applying a NodeCertificateCheck with `spec.paused: true` using `kubectl apply --validate=strict` fails with an unknown-field error, and the same manifest applied with `--validate=false` is accepted with the field pruned (`kubectl get -o jsonpath='{.spec.paused}'` empty)
- [X] T051 [P] [US3] Add `internal/cli/kinds_test.go` and `internal/cli/run_test.go` assertions that no descriptor exposes a paused accessor and `run` help and outcomes contain no "paused"
- [X] T052 [P] [US3] Add `internal/controller/healthreport_retention_controller_test.go` (envtest): orphan detection by kind+name+UID; `source-deleted-at` stamped once and never rewritten; deletion after the window; window `0` deletes on first pass; a recreated same-name check's reports are untouched and the old ones keep their UID; per-kind cap deletes oldest first across live and orphaned; the 50-per-pass budget requeues; metrics and one Event per pass with the contract message, recorded in the affected namespace regarding the last pruned report
- [X] T053 [P] [US3] Add to `internal/controller/healthreport_history_test.go`: per-check prune filters by all three labels so a predecessor's reports are neither counted nor deleted
- [X] T054 [P] [US3] Add to `internal/app/options_test.go`: `retention.report_window` default `720h`, env `FATHOM_RETENTION_REPORT_WINDOW`, flag `--retention-report-window`, negative rejected; `retention.report_cap_per_kind` default `500`, minimum `1`
- [X] T055 [US3] Add the retention assertions of quickstart §4 to `test/e2e/fathomctl_lifecycle_test.go` (chart values `retention.reportWindow: 2m`, `retention.reportCapPerKind: 5`): deleted-at annotation, `fathomctl reports` retained marker, window prune with the Event (the recreate-with-new-UID boundary, the cap prune and the counters are proven in the envtest retention suite, not on kind)

### Implementation for User Story 3 (pause removal, ships in PR 2)

- [X] T056 [US3] Remove `Paused` from `api/v1/addoncheck_types.go`, `api/v1/healthcheck_types.go`, `api/v1/nodecertificatecheck_types.go`
- [X] T057 [US3] Remove from `internal/controller/addoncheck_controller.go`: `addonCheckConditionPaused`, the condition write (~200-214), the `forgetRuntimeWork` paused branch (~220), the `resolveAddonAdapter` paused branch (~373), the `runtimeBacked` guard (~614), the queued-run no-op (~682)
- [X] T058 [US3] Remove from `internal/controller/healthcheck_controller.go`: the condition constant (~40), the condition write (~429-440), the mirror skip (~443-449)
- [X] T059 [US3] Remove from `internal/controller/nodecertificatecheck_controller.go`: the condition constant (~55), the `reconcilePaused` call (~270-276) and function (~440-461), the `Paused=False/RunEnabled` write (~278), the RBAC comment at ~181; keep `revokeNodeCertAgent` for the invalid-spec path
- [X] T060 [US3] Remove from `internal/cli/`: `kindDescriptor.Paused` and its six rows in `kinds.go`; the three sites in `run.go` (~230, ~261, ~315); `"Paused"` from the terminal-reason set in `wait.go` (~45); the three prints in `describe.go` (~118, ~176, ~248)
- [X] T061 [US3] Remove "paused" from `docs/guides/monitoring.md` (~43, ~583), `docs/reference/fathomctl.md` run semantics, `docs/reference/status-conditions.md`; add "stop a check by deleting it" guidance with the retention summary to `docs/guides/addon-checks.md`, `docs/guides/node-certificate-checks.md`, `docs/guides/dns-checks.md`, `docs/guides/node-health-checks.md`

### Implementation for User Story 3 (retention, ships in PR 4)

- [X] T062 [US3] Add to `api/v1/annotations.go`: `LabelHealthReportSourceUID = "fathom.skaphos.io/source-uid"` and `AnnotationHealthReportSourceDeletedAt = "fathom.skaphos.io/source-deleted-at"` with doc comments
- [X] T063 [US3] In the five report builders (files as in T025) set the `source-uid` label to the check's `metadata.uid` and remove every `controllerutil.SetControllerReference(check, report, ...)` call (`addoncheck_controller.go` ~478 and ~845, `dnscheck_controller.go` ~607, `nodecertificatecheck_controller.go` ~1046, `nodehealthcheck_controller.go` ~1004); pass the three labels to the shared prune helper
- [X] T064 [US3] Add `Retention RetentionOptions` (`ReportWindow time.Duration` mapstructure `report_window`, `ReportCapPerKind int` mapstructure `report_cap_per_kind`) to `internal/app/options.go` with defaults `DefaultRetentionReportWindow = 720 * time.Hour`, `DefaultRetentionReportCapPerKind = 500`, `Validate` rules (window `>= 0`, cap `>= 1`), and two `bindings()` rows using the duration kind from T007
- [X] T065 [US3] Add metrics to `internal/metrics/metrics.go`: gauge `fathom_healthreports_retained{namespace,source_kind}`, counter `fathom_healthreports_pruned_total{namespace,source_kind,reason}`, registered with the others, with `ObserveReportsRetained`, `RecordReportsPruned` and `DeleteRetentionSeries(namespace)` helpers; add rows to `docs/guides/monitoring.md` so `internal/metrics/contract_doc_test.go` passes
- [X] T066 [US3] Create `internal/controller/healthreport_retention_controller.go`: `HealthReportRetentionReconciler{Client, Scheme, Recorder, Window, CapPerKind, Clock}` keyed by namespace; `SetupWithManager` watches the four report-writing kinds (AddonCheck, DNSCheck, NodeCertificateCheck, NodeHealthCheck) with a delete-only predicate mapping to the namespace, plus a 1h periodic resync via `source.Channel` or a ticker; `Reconcile` implements data-model.md "Retention reconciler" (list, group by three labels, resolve check by kind+name, compare UID, stamp or evaluate `source-deleted-at`, enforce cap, at most 50 deletes per pass with immediate requeue), updates both metrics, and emits one Event regarding the most recently pruned report (so it lives in the affected namespace; `reason ReportsPruned`, `action Retain`, message `pruned N HealthReports (window: a, cap: b)`) only when something was deleted
- [X] T067 [US3] Wire the reconciler in `internal/app/run.go` beside the check controllers with `mgr.GetEventRecorder("fathom-healthreport-retention-controller")`, passing `opts.Retention`; add the `Setupper` seam entry so `internal/app/run_test.go` covers registration
- [X] T068 [US3] In `internal/cli/reports.go` and `internal/cli/describe.go` mark reports carrying `source-deleted-at` as `retained (check deleted <time>)` in the listing and print the three source labels and the annotation in the full view; add cases to `internal/cli/reports_test.go`
- [X] T069 [US3] Add `retention.reportWindow` and `retention.reportCapPerKind` to `deploy/helm/fathom-operator/values.yaml`, `values.schema.json` and the ConfigMap template, mapped to `retention.report_window` and `retention.report_cap_per_kind`; document in the chart README and `docs/reference/configuration.md` (or the existing options reference)
- [X] T070 [US3] Write `docs/adr/0010-healthreport-retention.md`: context (owner references cascade history; delete is now the only stop), decision (label identity with UID, no owner reference, namespace-keyed sweep, window and cap as configuration, labels and one annotation as lifecycle provenance, with the explicit boundary that verdict, evidence and timestamps never leave spec and status), consequences, amends ADR 0002 (edit 0002's status line to "Amended by ADR 0010")
- [X] T071 [US3] Run `go -C tools tool task test lint staticcheck` and `go -C tools tool task test-e2e E2E_ADDONS=lifecycle`

**Checkpoint**: Pause gone everywhere; deletion keeps history for the window; cap and observability proven.

---

## Phase 6: User Story 4 - Immutability and merge rules that cannot be bypassed (Priority: P2)

**Goal**: Required `spec`, map-list conditions on AddonCheck, printer columns on AddonCheck and HealthReport, enumerated `checkRef.apiVersion`. Contract: `contracts/api-v1.md` "Admission behaviour added at v1". Ships in PR 2.

**Independent Test**: quickstart §2 bullets two to five.

### Tests for User Story 4

- [X] T072 [P] [US4] Add to `test/e2e/crdvalidation_test.go`: replacing an AddonCheck with a copy lacking `spec` fails with `spec: Required value`; the two-step `checkRef` edit on a HealthCheck fails at step one; `checkRef.apiVersion: example.com/v1` and `checkRef.apiVersion: fathom.skaphos.io/v1alpha1` are each rejected naming `fathom.skaphos.io/v1`; two SSA field managers each applying one AddonCheck condition type leave both present
- [X] T073 [P] [US4] Add to `internal/controller/samples_admission_test.go` (envtest) the same admission cases (including both rejected `checkRef.apiVersion` values) so they run under `task test` without a kind cluster
- [X] T074 [P] [US4] Add a printer-column assertion to `test/e2e/crdvalidation_test.go`: `kubectl get addonchecks` header contains `RESULT ADDON LAST RUN AGE`; `kubectl get healthreports` header contains `RESULT SOURCE NAME OBSERVED AGE`

### Implementation for User Story 4

- [X] T075 [US4] In all seven `api/v1/*_types.go` root types change `Spec ... \`json:"spec,omitempty"\`` to `\`json:"spec"\`` with `// +kubebuilder:validation:Required`; keep the HealthReport spec-level `self == oldSelf` rule on the field
- [X] T076 [US4] In `api/v1/addoncheck_types.go` add `// +listType=map`, `// +listMapKey=type` and `patchStrategy:"merge" patchMergeKey:"type"` to `AddonCheckStatus.Conditions`, matching `api/v1/healthcheck_types.go`
- [X] T077 [P] [US4] Add printer columns to `api/v1/addoncheck_types.go`: Result `.status.lastResult`, Addon `.spec.addonType`, Last Run `.status.lastRunTime` (type date), Age `.metadata.creationTimestamp`
- [X] T078 [P] [US4] Add printer columns to `api/v1/healthreport_types.go`: Result `.spec.result`, Source `.spec.sourceRef.kind`, Name `.spec.sourceRef.name`, Observed `.spec.observedAt` (type date), Age
- [X] T079 [US4] In `api/v1/healthcheck_types.go` `CheckTargetRef.APIVersion`: add `// +kubebuilder:validation:Enum=fathom.skaphos.io/v1`, keep `MaxLength=317`, update the doc comment to "defaults to fathom.skaphos.io/v1"
- [X] T080 [US4] Regenerate (`manifests`, `helm:sync`, `docs:api-ref`) and run `go -C tools tool task verify-generated test crd-compat`; confirm the seven `*` entries from T031 still sanction every finding and no `INCOMPATIBLE` appears

**Checkpoint**: Every US4 admission case rejected or merged as the contract states.

---

## Phase 7: User Story 5 - Generated and written artifacts agree with the shipped API (Priority: P3)

**Goal**: One consistent story across docs, samples, chart, bundle, versioning reference and the epic. Ships in PR 2 except the RELEASE and fathomctl pages owned by US2/US3.

**Independent Test**: quickstart §6.

- [X] T081 [P] [US5] Rewrite `docs/reference/api-versioning.md` per research R13: Fathom row served `v1` (seven kinds) and `v1alpha1` (two), storage likewise; replace the "alpha at 1.0 is acceptable" stance with the 1.0.0-ships-stable-`v1` decision; keep the contract-version exclusion, phrased as the deliberate independent axis; note the canonical standard needs the same row update upstream
- [X] T082 [P] [US5] Update `README.md`, `docs/README.md`, `docs/architecture.md`, `docs/code-map.md`, `docs/authoring-adapters.md`, `docs/guides/getting-started.md`, `docs/guides/addon-checks.md`, `docs/guides/dns-checks.md`, `docs/guides/node-health-checks.md`, `docs/guides/node-certificate-checks.md`, `docs/guides/agent-operations.md`, `docs/reference/status-conditions.md`: every promoted-kind `apiVersion` and `api/v1alpha1` path to `v1`; add the stable/alpha split sentence where kinds are listed
- [X] T083 [P] [US5] Update `docs/.crd-ref-docs.yaml` comment and `.github/copilot-instructions.md` (line ~34) to name `api/v1` and `api/v1alpha1`; update the code comments citing `api/v1alpha1` in `internal/nodehealth/types.go` (~39, ~158), `internal/controller/cadence.go` (~22), `observe.go` (~30), `dnscheck_plan.go` (~25), `nodecertificatecheck_controller.go` (~260), `internal/metrics/metrics.go` (~167)
- [X] T084 [P] [US5] Regenerate `config/manifests/bases/fathom.clusterserviceversion.yaml` with `operator-sdk generate kustomize manifests -q` (via `go -C tools tool task bundle` prerequisites) so `owned` lists the seven kinds at `v1` plus DNSCheck, AddonDefinition and AddonDefinitionBinding; commit the base CSV (requires `operator-sdk` on `PATH`; the CSV is not covered by `verify-generated`, so list it in the PR 2 review checklist)
- [X] T085 [US5] Run `go -C tools tool task docs:api-ref` and commit `docs/reference/api.md` with both group versions; confirm `verify-generated` is green
- [ ] T086 [US5] After explicit user confirmation (outward-facing), edit GitHub issue #149 (via `gh issue edit`) to record: the four already-shipped items, the seven-kind scope, the revised version-axis statement (contract independent at 1.1.0), and a link to `specs/014-api-v1-promotion/spec.md`; add a comment on #262 and #257 pointing at the spec
- [X] T087 [US5] Verify quickstart §6: `grep -rl 'fathom.skaphos.io/v1alpha1' docs README.md` returns only `docs/adr/*`, `docs/rfc/*`, `docs/guides/addon-definitions.md` and the migration section of `RELEASE.md`

**Checkpoint**: Documentation names `v1` for the seven kinds, alpha only for the definition kinds, and states the axes.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [X] T088 Run `graphify update .` and commit `graphify-out/` changes with the feature (per repo guidance)
- [X] T089 Run the full `go -C tools tool task ci` plus `crd-compat`, then the full e2e stack `go -C tools tool task test-e2e` (all shards) on the final branch state; record outcomes in each PR's test plan
- [X] T090 [P] Add a `CHANGELOG`-facing release note draft to the PR 2 description with the `feat!:` subject and a `BREAKING CHANGE:` footer naming the seven kinds, pause removal and the required export/upgrade sequence, so release-please renders it
- [ ] T091 Review the four PRs' Copilot and adversarial review threads, address or resolve each (per the repo's review loop), and run `/speckit-adversarial-review` as the mandatory after-implement hook before reporting completion

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: start immediately.
- **Foundational (Phase 2)**: after Setup; blocks every story. Ships as PR 1.
- **US1 (Phase 3)**: after Phase 2. Creates `api/v1`; everything else edits it.
- **US4 (Phase 6)** and **US3 pause removal (T056-T061)**: after US1 tasks T016-T019; same files, so sequential with US1 and with each other.
- **US5 (Phase 7)**: after US1 and US4 (generated reference must reflect final types).
- **US2 (Phase 4)**: after US1 (needs the `v1` CRDs and the `*` entries) and after T056 (export drops a field the schema must reject). Ships as PR 3.
- **US3 retention (T062-T071)**: after Phase 2 (shared prune helper, duration option) and US1; independent of US2. Ships as PR 4.
- **Polish (Phase 8)**: last.

### User Story Dependencies

- US1: foundation only.
- US2: US1, T056.
- US3: US1 (pause removal shares its files; retention needs the `v1` labels file).
- US4: US1.
- US5: US1, US4.

### Within each story

Tests are written first and must fail before the implementation task that makes them pass. Type edits precede regeneration; regeneration precedes doc and sample edits; `verify-generated` closes each phase.

### Parallel opportunities

- Phase 2: T007+T008, T009+T010, T011 run in parallel with T004-T006.
- US1: T013-T015 in parallel; T029 and T030 in parallel once T017 lands; T031-T033 in parallel with T028.
- US2: T035-T037 in parallel; T043-T044 in parallel with T039-T042; T047-T048 in parallel with T049's prerequisites.
- US3: T050-T054 in parallel; T062, T064, T065 in parallel; T069-T070 in parallel with T066-T068.
- US4: T077+T078 in parallel; T072-T074 in parallel.
- US5: T081-T084 all in parallel.

---

## Parallel Example: User Story 3 retention

```text
Together: T062 (labels), T064 (options), T065 (metrics + doc rows)
Then:     T063 (builders), T066 (reconciler), T067 (wiring)
Together: T068 (CLI marker), T069 (chart values), T070 (ADR 0010)
Then:     T052/T053/T054/T055 go green; T071 runs the gates
```

---

## Implementation Strategy

### MVP first

US1 alone proves the promise: `v1` served alone, alpha rejected, generated artifacts in sync, core e2e green. It is not shippable on its own because the `api/v1` files it creates must also carry the US4 deltas and the pause removal before the schema freezes, so the first release-bound PR is:

### PR mapping (research R16)

| PR | Phases and tasks | Commit type |
|---|---|---|
| 1 | Phase 2 (T004-T012) | `refactor:` |
| 2 | US1 (T013-T034), US3 pause removal (T056-T061), US4 (T072-T080), US5 (T081-T087) | `feat!:` with `BREAKING CHANGE:` footer |
| 3 | US2 (T035-T049) | `feat:` |
| 4 | US3 retention (T050-T055 retention parts, T062-T071) | `feat:` |

Each PR runs `task ci`, `crd-compat` and the e2e shard its diff touches (`fathom` for PR 2, `lifecycle` for PR 3 and PR 4), and states those outcomes in its test plan. PR 2 is the one that must not merge until PR 3's export verb exists in the same release, since the release notes point adopters at it; land PR 2 and PR 3 in the same release train, with PR 4 following before 0.7.0 is cut.
