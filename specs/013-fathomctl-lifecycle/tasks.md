<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Tasks: fathomctl Lifecycle Verbs

**Input**: Design documents from `specs/013-fathomctl-lifecycle/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md)

**Tests**: Required. The constitution's quality gates require direct test coverage for new behaviour and the coverage gate per package; tests precede implementation within each story.

**Organization**: one phase per user story (US1 install, US2 upgrade, US3 uninstall, US4 status), after shared setup and foundational work.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1–US4 from spec.md
- Paths are repository-relative. Run Go commands with `GOROOT=` and tasks via `go -C tools tool task …`.

## Path Conventions

- CLI: `internal/cli/` (verbs, seams, tests), `cmd/fathomctl/` (entrypoint, unchanged)
- Chart: `deploy/helm/fathom-operator/`, embedding package `deploy/helm/`
- Gates: `scripts/*_test.go`; e2e: `test/e2e/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: dependencies and the import-boundary gate the new dependencies make necessary.

- [ ] T001 Add `helm.sh/helm/v4 v4.3.0`, `sigs.k8s.io/crdify v0.6.0` and `k8s.io/cli-runtime` as direct requirements and pin `k8s.io/cli-runtime` and `k8s.io/kubectl` to `v0.37.1` in `go.mod`/`go.sum` (`GOROOT= go get … && GOROOT= go mod tidy`); verify no existing `k8s.io/*`, `kube-openapi`, `structured-merge-diff`, `controller-runtime`, prometheus, cobra/pflag/viper requirement changed version (research R1)
- [ ] T002 [P] Add `scripts/cli_import_boundary_test.go`: run `go list -deps ./cmd/fathomctl` and fail if any of `github.com/skaphos/fathom/internal/app`, `…/internal/adapter`, `…/internal/controller` (or their subpackages) appear (research R12)
- [ ] T003 [P] Update `AGENTS.md` project-structure notes for `deploy/helm/chart.go`, the lifecycle files in `internal/cli/`, and that the CLI import rule is now gated by `scripts/cli_import_boundary_test.go`

**Checkpoint**: module builds with the new requirements; boundary gate passes on the current tree.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the embedded chart, digest support, upgrade-path declaration, install detection, Helm seam and CRD comparison every verb uses.

**⚠️ CRITICAL**: no user-story work begins until this phase is complete.

### Chart digests and embedding

- [ ] T004 [P] Add digest values to `deploy/helm/fathom-operator/values.yaml` (`image.digest`, `probeImage.digest`, `nodeAgent.image.digest`, default `""`) and `values.schema.json` (pattern `^(sha256:[0-9a-f]{64})?$`)
- [ ] T005 Update image helpers in `deploy/helm/fathom-operator/templates/_helpers.tpl` to render `repo@<digest>` when the digest is set and `repo:<tag>` otherwise, for the manager image and the `--probe-image` / `--node-agent-image` args (research R3)
- [ ] T006 [P] Add `scripts/helm_digest_test.go`: default render uses `repo:v<appVersion>`; with each digest set, the Deployment image and both args use `repo@sha256:…`; a malformed digest fails schema validation
- [ ] T007 Create `deploy/helm/chart.go` (package `helmchart`, SPDX header): `//go:embed all:fathom-operator`, `Files() fs.FS`, `Load(version string) (*chart.Chart, error)` walking the FS into `[]*archive.BufferedFile`, calling `pkg/chart/v2/loader.LoadFiles`, and setting `Metadata.Version` and `Metadata.AppVersion` to `version` (research R2); a non-SemVer `version` returns a typed error the verbs map to the `dev-version` refusal (spec Assumptions)
- [ ] T008 [P] Add `deploy/helm/chart_test.go`: `Load` returns chart name `fathom-operator`, the stamped version, all nine files under `crds/`, and `upgrade-path.yaml` present in `Files()`
- [ ] T009 Add `internal/cli/digests.go`: package vars `operatorDigest`, `probeDigest`, `nodeAgentDigest` (set by `-X`) and `imageValues()` returning chart value overrides that always set repository, tag and digest for all three images (so reused values from a previous revision never survive, FR-007); empty digests mean tag pinning ("development build") with the digest values explicitly `""`
- [ ] T010 Update `scripts/fathomctl-dist.sh` and the `fathomctl-build` task in `Taskfile.yml` to pass `-X github.com/skaphos/fathom/internal/cli.operatorDigest=$FATHOM_OPERATOR_DIGEST` (and probe, node-agent) when those env vars are set
- [ ] T011 Update `.github/workflows/release.yml` "Build fathomctl release archives" step to export `FATHOM_OPERATOR_DIGEST`, `FATHOM_PROBE_DIGEST`, `FATHOM_NODE_AGENT_DIGEST` from `steps.operator.outputs.digest`, `steps.probe.outputs.digest` and `steps.node_agent.outputs.digest`

### Upgrade-path declaration

- [ ] T012 [P] Create `deploy/helm/fathom-operator/upgrade-path.yaml` with the header comment from [contracts/upgrade-path.md](contracts/upgrade-path.md), `floor: 0.5.0`, and one entry for `0.6.0` carrying only the findings `go -C tools tool task crd-compat` reports as `SANCTIONED` against the v0.5.1 baseline — `clusterhealths.fathom.skaphos.io ^.status.children` (maxItems 100) and `healthchecks.fathom.skaphos.io ^.spec.checkRef.apiVersion` (maxLength 317) — with reason and issue copied from `.crd-compat-allowlist.yaml` (STALE entries are older releases and are not recorded); set `stop: false` (decided 2026-10-11: neither change needs an intermediate operator to migrate data, so later releases may upgrade straight from v0.5.x); ensure `.helmignore` does not exclude it
- [ ] T013 [P] Write `internal/cli/lifecycle_path_test.go` (table tests): parse valid file; reject non-increasing versions, missing `floor` and missing `crd`/`path`/`reason`/`issue`; A below `floor` → `below-floor` (e.g. 0.3.1 with floor 0.5.0), A equal to `floor` is allowed; oldest direct version = max(floor, last stop < B); path A→B is `direct` with no stops in (A,B); ordered stops when present; a stop equal to A or B is not a step; sanctioned match by CRD + path and by `*`; a `scope` finding never sanctioned
- [ ] T014 Implement `internal/cli/lifecycle_path.go`: `parseUpgradePath`, `upgradePath(from, to)` and `sanctionedBy(finding, from, to)` per the path algorithm in [contracts/upgrade-path.md](contracts/upgrade-path.md)
- [ ] T015 [P] Add `scripts/upgrade_path_gate_test.go`: file parses; `floor` set; versions strictly increase; entries for versions ≤ the latest `v*` tag are byte-identical to that tag's copy (skipped when the file is absent at that tag, as at v0.5.1); at most one unreleased entry (version > latest tag); every `SANCTIONED` finding from `scripts/check-crd-compat.sh` has a matching entry in the unreleased entry; when `.release-please-manifest.json` is ahead of the latest tag, the unreleased entry's version equals it (per the Gates section of [contracts/upgrade-path.md](contracts/upgrade-path.md)) — run the script (with the same `CRD_COMPAT_*` fixture env vars its own tests use) and parse its `SANCTIONED <crd> <path>` output lines; add a stable line format to the script if it does not already print both fields

### Install detection and Helm seam

- [ ] T016 Define in `internal/cli/lifecycle.go`: the `Install` type (fields `managedBy` ∈ {`helm`,`kustomize`,`olm`,`unmanaged`,`none`}, `release`, `namespace`, `revision`, `releaseStatus`, `installedBy`, `version`, `images`, `pinning` ∈ {`digest`,`tag`}, `ready`), the `releaseClient` interface (`List`, `Get`, `Values`, `Install`, `Upgrade`, `Uninstall`, `Rollback`, `Render`), and a factory field `newReleaseClient` replaced in tests
- [ ] T017 [P] Write `internal/cli/lifecycle_test.go` for detection: one Helm release (chart `fathom-operator`) → `helm`; manager with `olm.owner` labels → `olm`; `app.kubernetes.io/managed-by: kustomize` → `kustomize`; no operator → `none`; two Fathom releases → error `multiple-installs`; `installedBy` from the `fathom.skaphos.io/installed-by` release label
- [ ] T018 Implement `detectInstall` in `internal/cli/lifecycle.go`: find manager Deployments cluster-wide with the existing lookup in `internal/cli/version.go`, then `releaseClient.List` only in those namespaces plus the `-n` namespace (chart name filter) — no cluster-wide Secret list — then classify non-Helm managers (research R6); T017 asserts the fake client sees no cluster-wide Secret list
- [ ] T019 Implement `internal/cli/lifecycle_helm.go` — the only file importing `helm.sh/helm/v4`: `action.Configuration` initialised with the factory's REST config (`genericclioptions` getter), Secrets driver, `kube.StatusWatcherStrategy`, `Timeout` from `--timeout`, `SkipCRDs: true`, `ServerSideApply`, `Labels{"fathom.skaphos.io/installed-by":"fathomctl"}` on install; `Render` via client dry-run; result type-assert to `*v1.Release`
- [ ] T020 [P] Add `internal/cli/lifecycle_helm_test.go` using Helm's in-memory storage driver and fake kube client: install/upgrade/uninstall/rollback round-trip, render error surfaces schema violations, labels recorded

### CRD comparison and application

- [ ] T021 [P] Write `internal/cli/lifecycle_crds_test.go`: identical CRD → `unchanged`; added optional field → `update`, no findings; removed property / type change / new required field → finding; scope change → finding `scope`; missing live CRD → `add`; apply waits for `Established` and times out per `--timeout`; applied CRDs carry `fathom.skaphos.io/version` (FR-015); a live CRD owned by another field manager (`helm`, `kubectl-client-side-apply`) with a property and a served version the target removed ends with exactly the target spec (FR-020); a post-apply spec mismatch fails before any rollout; `recordedVersion` read back (absent → unset)
- [ ] T022 Implement `internal/cli/lifecycle_crds.go`: load target CRDs from the embedded chart `crds/`; read live CRDs (apiextensions/v1 via the controller-runtime client — add apiextensions to `newScheme` in `internal/cli/client.go`); compare with `crdify` `runner.New(&config.Config{UnhandledEnforcement: config.EnforcementPolicyError}, runner.DefaultRegistry()).Run(live, target)`; stamp `fathom.skaphos.io/version`; when `managedFields` show another manager, replace the CRD whole (Update with `resourceVersion`, dropping the last-applied annotation) before server-side apply with field manager `fathomctl` and force-conflicts; verify the live spec equals the target; wait for `Established` bounded by `--timeout` (research R4, R9 step 5)
- [ ] T023 Add the `--timeout` flag (default `5m`) for lifecycle verbs, a shared `waitBounded` helper, and a `preflightPermissions(verb)` helper in `internal/cli/lifecycle.go` that issues SelfSubjectAccessReviews for what the verb needs (create/update CRDs, cluster RBAC, the release namespace and Secrets; delete for uninstall) and refuses before any change, naming each missing permission; ensure every API call still uses `--request-timeout`

**Checkpoint**: embedded chart loads, digests render, path and CRD plans compute in unit tests, Helm seam round-trips against fakes.

---

## Phase 3: User Story 1 - Install Fathom with one command and know it worked (Priority: P1) 🎯 MVP

**Goal**: `fathomctl install` creates CRDs then the operator at the CLI's version, pinned by digest, waits for Available, and is idempotent.

**Independent Test**: on an empty kind cluster, `fathomctl install --create-namespace`; CRDs exist, Deployment Available, `helm ls` lists `fathom`; a second run changes nothing (quickstart §1).

### Tests for User Story 1

- [ ] T024 [P] [US1] Write `internal/cli/install_test.go` (fake client + fake `releaseClient`): fresh install applies CRDs before the release and passes digest values; same version → "already installed … nothing to do", exit 0, no writes; older install → refuse with `next: fathomctl upgrade -n <ns>`; newer install → refuse naming the matching `fathomctl` release; non-Helm install → refuse naming `kustomize`/`OLM`; missing namespace without `--create-namespace` → refuse before any write; invalid values → schema error before CRDs are applied; rollout timeout → exit 1 with unmet condition; `--set image.*` → usage error naming the exact-version reason (FR-007); a `pending-*`/`failed` release → refuse with recovery commands; missing permissions → refuse before any write naming them; more than one Fathom release → refuse naming each (FR-008); no `-n` → `fathom-system`, not the context namespace (FR-010); non-SemVer CLI version → `dev-version`; no release but CRDs present (FR-014): CRDs recording a newer version → `crd-downgrade`, recording a version below `floor` → `below-floor`, a skipped stop → `stop-skipped`, a sanctioned incompatible change → `acknowledgement-required` (accepted with `--accept-incompatible-crds`), compatible → installs and keeps existing check resources, no recorded version with an incompatible finding → refuse; every applied CRD carries the version annotation (FR-015)

### Implementation for User Story 1

- [ ] T025 [US1] Implement `internal/cli/install.go` per [contracts/cli-lifecycle.md](contracts/cli-lifecycle.md) `install`: flags `--create-namespace`, `--release` (default `fathom`), `-f/--values`, `--set`, `--set-string`, `--set-json`, `--timeout`, `--accept-incompatible-crds`; `-n` defaults to `fathom-system`; order detect → existing-CRD checks reusing the T031 plan builder (FR-014) → render (values validation, FR-007) → namespace → CRDs (T022) → `releaseClient.Install` with `imageValues()` (FR-010–FR-013)
- [ ] T026 [US1] Reject user overrides of image repository/tag/digest values in `internal/cli/install.go` (shared helper reused by upgrade) with the exact-version reason
- [ ] T027 [US1] Register `install` in the command tree in `internal/cli/root.go` and print the success summary (version, release, namespace, image references, pinning)

**Checkpoint**: US1 independently testable via unit tests and quickstart §1.

---

## Phase 4: User Story 2 - Upgrade safely, CRDs first (Priority: P1)

**Goal**: `fathomctl upgrade` plans, refuses unsafe transitions, applies CRDs before the rollout, and handles a failed rollout per FR-026/FR-029.

**Independent Test**: install, apply the older-CRD fixture, `fathomctl upgrade --dry-run` then `upgrade`; CRDs Established before new manager Pods; refusals unit-covered (quickstart §2–3).

### Tests for User Story 2

- [ ] T028 [P] [US2] Write `internal/cli/upgrade_test.go` plan cases: compatible → CRDs then rollout, exit 0; sanctioned incompatible without flag → `acknowledgement-required` listing change/reason/issue and `next:` with `--accept-incompatible-crds`; `--yes` does not satisfy it; with the flag → applied; unsanctioned incompatible → `unsanctioned-incompatible` with CRD/path/message; scope change → `scope-change` mentioning `uninstall --purge-crds --export`; skipped stop → `stop-skipped` printing `Upgrade path: A -> S -> B` and `next: download fathomctl vS …`, not bypassed by `--accept-incompatible-crds`; CLI older than install → `downgrade`; no install → points at `install`; release `pending-*`/`failed` → `release-pending`; non-Helm install → refuse naming `kustomize`/`OLM` (FR-006); missing permissions → refuse before any write; more than one Fathom release → refuse (FR-008); installed below `floor` → `below-floor` naming the `helm` step (FR-027); values preservation (FR-025): a release installed with monitoring and projected-label values keeps them across `upgrade` with no flags, and a `--set` override wins over the reused value; reused image values (a `helm`-set `image.tag`, an older release's digests) are replaced by the CLI's own for all three images (FR-007)
- [ ] T029 [P] [US2] Write `internal/cli/upgrade_failure_test.go`: timeout → release left, exit 1, recovery lines (`kubectl -n <ns> describe deploy/<name>`, longer `--timeout`, `helm rollback <release> -n <ns>`); `--rollback-on-failure` → `Rollback` called then exit 1; `--rollback-on-failure` with any incompatible CRD change in the plan → usage refusal before any write; CRDs never reverted
- [ ] T030 [P] [US2] Write `internal/cli/upgrade_output_test.go`: `--dry-run` writes nothing and prints the plan table from the contract; `-o json|yaml` emits the Upgrade plan entity (`from`, `to`, `path`, `crds[].action` ∈ {`add`,`update`,`unchanged`,`refuse`}, `crds[].findings[]{check,path,message,sanctionedBy?}`, `refusals[]{reason,detail,next}`, `operator`, `values` without secrets)

### Implementation for User Story 2

- [ ] T031 [US2] Implement the plan builder `buildUpgradePlan` in `internal/cli/upgrade.go`: detect → path (T014) → CRD findings (T022) matched with `sanctionedBy` → refusals with exact `next:` commands; render with `ReuseValues` plus overrides to validate values (research R9 steps 1–3)
- [ ] T032 [US2] Implement `upgrade` execution in `internal/cli/upgrade.go`: flags `--dry-run`, `--accept-incompatible-crds`, `--rollback-on-failure`, shared value/timeout flags; apply CRDs (stamped, replaced when foreign-owned, verified) and wait Established; `releaseClient.Upgrade` with `SkipCRDs`, `ReuseValues`, `imageValues()`, `RollbackOnFailure` only when requested and allowed (FR-020–FR-029)
- [ ] T033 [US2] Implement the plan and refusal renderers (table, json, yaml) in `internal/cli/upgrade.go` reusing `internal/cli/output.go`, and register `upgrade` in `internal/cli/root.go`

**Checkpoint**: US1 and US2 work independently; upgrade refusals all unit-covered.

---

## Phase 5: User Story 3 - Uninstall and leave nothing running (Priority: P2)

**Goal**: `fathomctl uninstall` removes the release and every operator-created object (including the unowned admission policy, addon probe Pods and Lease) while keeping CRDs and resources; `--purge-crds [--export]` and `--delete-namespace` behave per FR-030–FR-035.

**Independent Test**: install, create a NodeCertificateCheck and a DNSCheck, `fathomctl uninstall`; no `managed-by=fathom` objects, admission policy gone, CRDs and resources kept; then purge with export (quickstart §4–5).

### Tests for User Story 3

- [ ] T034 [P] [US3] Write `internal/cli/uninstall_test.go`: plain uninstall calls `releaseClient.Uninstall` first, then deletes every object labelled `fathom.skaphos.io/managed-by=fathom` of kinds DaemonSet, ServiceAccount, NetworkPolicy, Role, RoleBinding, ConfigMap, Pod across namespaces (including unowned probe Pods in `kube-system`), the cluster-scoped `ValidatingAdmissionPolicy` and binding `fathom-node-report-authenticity`, and the Lease `2d3dbc4f.skaphos.io`; keeps CRDs, custom resources and the namespace (reported); no install → "nothing to remove", exit 0; non-Helm → refuse; CLI older than install → allowed; missing delete permissions → refuse before any delete naming them; more than one Fathom release → refuse (FR-008); `pending-*`/`failed` release → proceeds and reports the state (FR-036); deletion waits honour `--timeout`
- [ ] T035 [P] [US3] Write `internal/cli/uninstall_purge_test.go`: prompt text lists counts for all nine kinds and mentions `--export` when absent; no TTY and no `--yes` → refuse before any delete; `--yes` proceeds; deletion order resources then CRDs, CRD removal awaited within `--timeout`; `--export` file written atomically before any delete, multi-doc YAML in the order `AddonDefinition`, `AddonDefinitionBinding`, `AddonCheck`, `DNSCheck`, `NodeCertificateCheck`, `NodeHealthCheck`, `HealthCheck`, `ClusterHealth`, `HealthReport`, with `uid`, `resourceVersion`, `managedFields`, `creationTimestamp`, `generation`, every `ownerReferences` entry and `status` stripped (a `HealthReport` with a controller reference to its check exports with none, FR-032); export write failure → nothing deleted, exit 1
- [ ] T036 [P] [US3] Write `internal/cli/uninstall_namespace_test.go`: `--delete-namespace` with only Fathom/Helm objects → deleted; with other objects → listed before anything is removed and confirmation required (prompt/`--yes`/refuse); declining or no TTY without `--yes` → nothing changed, release included; `--purge-crds --delete-namespace` → exactly one prompt covering both (FR-035); automatic objects (`default` ServiceAccount, `kube-root-ca.crt`) are reported, not treated as Fathom-owned

### Implementation for User Story 3

- [ ] T037 [US3] Implement `internal/cli/lifecycle_cleanup.go`: `deleteOperatorCreated` (label-selected kinds, admission objects, Lease; foreground propagation; bounded by `--timeout`) per research R7 and the data-model table
- [ ] T038 [US3] Implement purge and export in `internal/cli/lifecycle_cleanup.go`: counts per kind, `exportResources(path)` (atomic temp + rename), delete resources then CRDs and await removal (research R10)
- [ ] T039 [US3] Implement the namespace inventory check in `internal/cli/lifecycle_cleanup.go` via discovery of namespaced list-able types (research R11)
- [ ] T040 [US3] Implement `internal/cli/uninstall.go` with flags `--purge-crds`, `--export`, `--delete-namespace`, `--yes`, `--timeout`; inventory and the single confirmation precede any change (FR-035); reusing the `confirm` helper and TTY seam from `internal/cli/run.go`; register in `internal/cli/root.go`

**Checkpoint**: US3 independently testable; uninstall works even when the CLI is older than the install.

---

## Phase 6: User Story 4 - Ask what is installed (Priority: P2)

**Goal**: `fathomctl status` reports the install, readiness, CRD currency and the upgrade path, with exit codes per FR-041.

**Independent Test**: `status` against no install, a Helm install, and a kustomize install; correct output and exit codes (quickstart §1–2).

### Tests for User Story 4

- [ ] T041 [P] [US4] Write `internal/cli/status_test.go`: healthy Helm install → exit 0 with release, revision, installed-by, version, pinning, Available, `9/9 current`, upgrade path to this CLI; not installed → exit 0; outdated CRDs → exit 1 naming them; not Available / release pending or failed / multiple releases → exit 1; kustomize install → reports version and "not managed by fathomctl"; development build → "tag-pinned (development build)"; CRD state from `fathom.skaphos.io/version` (FR-040): CLI older than the install still reports `outdated`/`newer` correctly, CRDs without the annotation are `current` only when CLI version = operator version and spec matches, else `unknown` (reported, exit unaffected); `-o json|yaml` emits the Install entity plus `crds[]{name,scope,recordedVersion,state∈{current,outdated,missing,newer,unknown}}` and `upgradePath`

### Implementation for User Story 4

- [ ] T042 [US4] Implement `internal/cli/status.go` (read-only: release Secrets, CRDs, manager Deployment) with table/json/yaml output via `internal/cli/output.go`; register in `internal/cli/root.go` (FR-040–FR-042)

**Checkpoint**: all four verbs work independently.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T043 [P] Create e2e shard `lifecycle`: add it to `scripts/e2e-shards.sh` and `.github/workflows/e2e.yml` so it runs on its own kind cluster without the kustomize operator deploy (images still built and loaded) — skipping the deploy needs the suite bootstrap in `test/e2e/e2e_suite_test.go` (BeforeSuite deploy/undeploy keyed off `E2E_ADDONS=lifecycle`) and, if it drives the deploy, the `test-e2e` task in `Taskfile.yml`; document it in `test/e2e/fixtures/README.md`
- [ ] T044 Add `test/e2e/fixtures/lifecycle/older-crds/` (one CRD with an earlier compatible schema) and `test/e2e/fathomctl_lifecycle_test.go` (`Label("lifecycle")`): build `fathomctl` with an e2e version matching the loaded image tags; install → status (assert it returns in under 5 s) → re-install no-op → apply older CRDs with `kubectl apply` (a foreign field manager, so the replace path of FR-020 runs) → upgrade, asserting each CRD's live spec equals the embedded one and carries `fathom.skaphos.io/version`, and every CRD's `Established` condition `lastTransitionTime` precedes the new manager ReplicaSet's `creationTimestamp` (SC-002) → create NodeCertificateCheck + DNSCheck → uninstall (no `managed-by=fathom` objects, admission policy gone, CRDs/resources kept) → reinstall onto the kept CRDs and assert the checks are picked up (edge case "CRDs present without an operator") → `uninstall --purge-crds --export --yes` (no CRDs) → reinstall + `kubectl apply` export restores resources, including `HealthReport`s, which must still exist after a garbage-collection settle period (FR-032). Helm interop (SC-007): `helm upgrade --install` the embedded chart (same version) then `fathomctl upgrade` succeeds and `helm history` shows both revisions; `fathomctl install` then `helm upgrade --reuse-values` succeeds and `fathomctl status` reports it
- [ ] T045 [P] Write `docs/adr/0008-fathomctl-lifecycle-via-helm-library.md`: decision (Helm v4 library; exact-version embedded chart; CLI-applied CRDs with crdify verdicts; append-only upgrade path), alternatives (helm binary, kustomize manifests, run-time chart fetch, `--version`), consequences (binary size ~2.5×, multi-step upgrades via intermediate CLIs)
- [ ] T046 [P] Update `docs/reference/fathomctl.md` with the four verbs, every flag, outputs, refusal reasons and exit codes from [contracts/cli-lifecycle.md](contracts/cli-lifecycle.md), and the permissions each verb needs (cluster-admin-level for install/upgrade/uninstall; for status exactly: list Deployments cluster-wide, get/list Secrets in the release namespace only, get/list CRDs), distinct from the viewer/runner roles; the `fathom.skaphos.io/version` CRD annotation and the `unknown` CRD state; that development builds install by tag and need a SemVer `FATHOMCTL_VERSION` (spec Assumptions)
- [ ] T047 [P] Update `docs/guides/fathomctl.md` and `docs/guides/getting-started.md`: install/upgrade/uninstall with `fathomctl` as the primary path, Helm as equivalent, exact-version policy and multi-step upgrades, OLM and kustomize installs not supported by these verbs, and that resources restored from `--export` carry no owner references (FR-032); moving a `fathomctl`-made install to a newer chart with `helm upgrade` requires `--reset-values` plus the user's own values, or the old image digests carry over (spec Edge "Mixed tooling"); air-gapped clusters mirror images at the container runtime, since image repositories are not overridable (spec Edge "Offline")
- [ ] T048 [P] Update `docs/guides/agent-operations.md` with the lifecycle runbook (status → upgrade --dry-run → upgrade → status; uninstall/purge with export) replacing the manual CRD steps
- [ ] T049 [P] Update `README.md` "Installation details": nine CRDs (not seven), `fathomctl` lifecycle verbs, the CRD-before-rollout behaviour
- [ ] T050 [P] Update `RELEASE.md`: digest injection into `fathomctl`, the `upgrade-path.yaml` upkeep step (add sanctioned entries, decide `stop`; `floor` is never changed), and that every release's `fathomctl` must stay published
- [ ] T051 Run the full gates: `go -C tools tool task ci`, `scripts/check-coverage.sh`, `go -C tools tool task verify-generated` (helm:sync unaffected), `go -C tools tool task crd-compat`, `reuse lint`, `helm lint deploy/helm/fathom-operator`; fix findings
- [ ] T052 Run quickstart §1–6 against a kind cluster and the `lifecycle` e2e shard; record outcomes in the PR test plan

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none.
- **Foundational (Phase 2)**: depends on Setup; blocks every story.
- **US1 (Phase 3)** and **US4 (Phase 6)**: depend only on Foundational.
- **US2 (Phase 4)**: depends on Foundational; shares the image-override helper from T026 (US1).
- **T025 (US1)** reuses the plan builder `buildUpgradePlan` from T031 (US2) for its existing-CRD checks (FR-014): implement T031 before T025.
- **US3 (Phase 5)**: depends on Foundational.
- **Polish (Phase 7)**: T043–T044 after US1–US3; docs after the verbs they describe; T051–T052 last.

### User Story Dependencies

- US1 (P1): MVP; independent after Phase 2 except T025's use of the T031 plan builder.
- US2 (P1): independent after Phase 2 except T026's shared override helper.
- US3 (P2): independent after Phase 2.
- US4 (P2): independent after Phase 2.

### Within Each User Story

- Tests first and failing, then implementation, then registration in `root.go`.

### Parallel Opportunities

- Phase 1: T002, T003.
- Phase 2: T004, T006, T008, T012, T013, T015, T017, T020, T021 (each in its own file); T005 after T004; T007 before T008; T014 after T013; T018 after T016/T017; T022 after T021.
- Stories: after Phase 2, US1, US3 and US4 can proceed in parallel; US2 after T026.
- Polish docs T045–T050 in parallel.

## Parallel Example: User Story 3

```text
Task: "T034 [US3] uninstall_test.go — plain uninstall and cleanup"
Task: "T035 [US3] uninstall_purge_test.go — purge, prompt, export"
Task: "T036 [US3] uninstall_namespace_test.go — namespace deletion"
```

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 → Phase 2 → T031 (plan builder) → Phase 3 (install).
2. Validate with quickstart §1; this alone replaces the manual first-install sequence.

### Incremental Delivery

1. Install (US1) → upgrade (US2) → status (US4) → uninstall (US3), each tested independently.
2. e2e shard and docs once the verbs exist; full gates and quickstart before the PR.

## Notes

- `lifecycle_helm.go` is the only file that imports Helm; everything else uses the `releaseClient` seam.
- Every refusal happens before any cluster change and ends with a `next:` line.
- Commit after each task or logical group, DCO-signed; e2e is required before the PR is ready (`internal/cli` behaviour against a real cluster, chart templates).
