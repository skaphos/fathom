<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Research: Stable `v1` API for the Established Check Kinds

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md) | **Date**: 2026-10-18

Each entry records a decision, why, and what was rejected. Findings come from
reading the tree at the feature branch base (v0.6.0 plus main) and from the
three exploration passes over the API package and controllers, the CLI, chart
and e2e, and the generation and packaging surface.

## R1. Package layout: `api/v1` owns the seven kinds and their helpers

**Decision**: Create `api/v1` holding the seven check kinds and the check-only
helper files (`validation.go` constants, `aggregate.go` `WorstResult`,
`annotations.go` labels and run-now annotation, `cadence.go` defaults,
`groupversion_info.go`). `api/v1alpha1` keeps only `AddonDefinition`,
`AddonDefinitionBinding`, `addondefinition_payloads.go` and its own scheme
registration. The seven kinds' `*_types.go` files are removed from `v1alpha1`,
not kept unserved.

**Rationale**: `controller-gen` scans `./...` for both `manifests` and
`generate` (Taskfile.yml:78-86), so a second API package needs no task change.
Every check-only helper has callers in the controllers and CLI; moving them
with the types keeps one import for check code. Keeping the alpha types in
tree as `+kubebuilder:unservedversion` would make the CRD advertise two
versions and reintroduce the conversion machinery the spec rejects (FR-004).

**Alternatives rejected**: a single mixed-version package (controller-gen keys
on package, not file, so one package cannot serve two group versions);
keeping shared helpers in a third `api/common` package (adds a package for no
caller benefit, and the definition kinds use none of them).

## R2. The one cross-version type: `DefinitionLeaderEpoch`

**Finding**: `AddonCheckEvidenceAuthority.LeaderEpoch` is a
`*DefinitionLeaderEpoch` (addoncheck_types.go:286), a struct defined beside
the alpha binding kind (addondefinitionbinding_types.go:61) and embedded into
`HealthReportAttribution` through `AddonCheckEvidenceAuthority`. It is used at
runtime in `addoncheck_runtime.go` and `internal/app/runtime_wiring.go`.

**Decision**: The struct moves to `api/v1` as the canonical definition. The
alpha `AddonDefinitionBindingStatus.LeaderEpoch` field changes its Go type to
the `v1` struct. The alpha CRD schema is unchanged because the struct's fields
are identical; `verify-generated` proves it.

**Rationale**: A stable schema must not depend on an alpha package: any later
alpha edit to the struct would silently change the frozen `v1` schema. Alpha
depending on stable is the correct direction and costs no conversion code.

**Alternatives rejected**: `v1` importing `v1alpha1` (wrong dependency
direction, freezes the alpha struct by accident); duplicating the struct in
both packages with conversion at the two runtime sites (two copies of one
contract, and `HealthReport` attribution would then carry a type that cannot
be compared to the binding's).

## R3. Upgrade mechanics: version replacement is delete-and-recreate

**Finding**: The API server rejects a CRD update whose `spec.versions` drops a
version still present in `status.storedVersions`. `fathomctl upgrade` applies
CRDs by full-replace `Update` (lifecycle_crds.go:225-256) and never reads
`storedVersions` or counts custom resources. crdify reports the storage
version removal as a CRD-level finding with an empty path, which only an
upgrade-path entry with `path: "*"` can sanction (upgrade.go:115-140,
lifecycle_path.go:74-84). Helm never touches files under `crds/` on upgrade;
`kubectl apply` of the new CRD fails with the same server error.

**Decision**: The CLI's CRD plan gains a third action, `replace`, chosen when
the target CRD drops a version the live CRD stores. `replace` is refused as
`unsanctioned-incompatible` unless an upgrade-path entry with `path: "*"`
covers the CRD, and as `acknowledgement-required` unless
`--accept-incompatible-crds` is passed. Before any change the plan lists every
custom resource of each CRD to be replaced (kind, namespace, name) and states
that they and their report history will be removed. With the flag, `replace`
deletes the CRD, waits for it to be gone (which cascades the alpha
resources), creates the target CRD, and waits for `Established`; the removed
resource count per CRD is printed. The release's upgrade-path entry carries
`stop: true` and one `*` sanction per promoted CRD. The plain-manifest path
documents the same sequence by hand: export, delete CRs, delete the seven
CRDs, apply.

**Rationale**: The server leaves no in-place route. Delete-and-recreate is
what the existing scope-change refusal already points adopters at
(`uninstall --purge-crds --export`), so the behaviour is familiar; doing it
inside `upgrade` keeps the inventory, the consent and the removal in one
explained step (constitution VI). The stop guarantees the 0.7.0 CLI, the only
release that carries the alpha-aware export verb, is the one that performs
the replacement.

**Alternatives rejected**: patching `status.storedVersions` after deleting the
objects (works, but hides the deletion from the plan output and depends on a
status write the CLI has no other reason to make); keeping `v1alpha1` served
with conversion strategy `None` (objects would be readable as `v1` with only
the version rewritten, but `spec.paused` would then be silently dropped and
required-`spec` and the new `checkRef.apiVersion` rule would be bypassed for
stored objects, exactly the half-converted state the spec forbids).

## R4. The compat gate cannot see the version swap today

**Finding**: `scripts/check-crd-compat.sh` classifies only
`propertyComparisons`. With no served version in common between baseline and
target, crdify's same-version checks yield nothing, CRD-level findings are
ignored, and the git baseline carries no `status.storedVersions`, so the
wholesale swap passes the gate silently. `scripts/upgrade_path_gate_test.go`
only requires that `SANCTIONED` lines have entries, so an empty gate run also
passes.

**Decision**: Extend the gate's classifier with one CRD-level check,
`servedVersionRemoval`: any version served at the baseline and absent from
the target is reported on path `^` with that check name and must be
sanctioned by an allowlist entry whose path is `*`. The seven promoted CRDs
get such entries in `.crd-compat-allowlist.yaml` with the tracking issue, and
`upgrade_path_gate_test.go` continues to require the matching `*` entries in
`upgrade-path.yaml`.

**Rationale**: FR-015 and SC-008 need the gate to flag the swap as a
sanctioned change, and the same check is what protects `v1` afterwards: a
future removal of `v1` would be caught. The check needs no CRD status, only
`spec.versions` names, so the git baseline suffices.

**Alternatives rejected**: relying on crdify's `storedVersionRemoval` (needs
status the baseline does not have); hand-adding allowlist entries with no
finding to match (they would report `STALE` immediately).

## R5. Report retention replaces owner-reference garbage collection

**Finding**: Every report builder sets a controller owner reference on the
check (five sites), so deleting a check cascades its reports. Reports carry
`source-kind` and `source-name` labels but no owner UID. Per-check history
pruning is implemented four times, listing by the two labels and deleting the
oldest beyond `spec.historyLimit` (default 10). No controller watches
`HealthReport`, none uses finalizers.

**Decision**: `v1` report builders stop setting owner references and add a
third label, `fathom.skaphos.io/source-uid`, carrying the owning check's UID.
The four prune functions collapse into one shared helper that filters by all
three labels, so a recreated same-name check never prunes or counts its
predecessor's reports (FR-013, US3 scenario 5). A new namespace-keyed
`HealthReportRetentionReconciler` is triggered by delete events on the four
report-writing kinds and by a periodic resync. Per pass it lists the namespace's
reports, groups by source labels, and treats a group as orphaned when no
check with that kind, name and UID exists. First observation of an orphan
stamps the annotation `fathom.skaphos.io/source-deleted-at` with the current
time (best available when the operator was down); groups older than the
window are deleted. It then enforces the per-namespace per-kind cap by
deleting the oldest reports of a kind beyond N, live or orphaned alike. Work
per pass is bounded by the cap itself plus a per-pass delete budget; the pass
requeues if the budget is exhausted.

**Rationale**: Without a finalizer the operator cannot intercept the delete,
so the only way to keep reports is to never make them dependents. Labels are
what the CLI already selects on, and a UID label is the cheapest identity
that distinguishes a recreated check. A namespace-keyed reconciler keeps the
work bounded and matches the per-namespace cap's shape. This amends ADR 0002's
cleanup choice and is recorded in a new ADR.

**Alternatives rejected**: a finalizer on every check kind that strips owner
references at delete time (blocks deletion whenever the operator is down,
adds six finalizer paths and an uninstall hazard, and still needs the sweep
for the cap); keeping owner references and setting `blockOwnerDeletion` or
`orphan` propagation (propagation is the deleter's choice, not the operator's).

## R6. Retention configuration

**Decision**: Two operator options under a new `retention` group:
`retention.report_window` (duration, default `720h`, `0` disables retention
so orphans are pruned on the first pass) and `retention.report_cap_per_kind`
(int, default `500`, minimum `1`). Flags `--retention-report-window` and
`--retention-report-cap-per-kind`; env `FATHOM_RETENTION_REPORT_WINDOW` and
`FATHOM_RETENTION_REPORT_CAP_PER_KIND`. The options framework gains a duration
binding kind (`isDuration`, `durationDef`) since none exists; the viper decode
hook is checked to parse `time.Duration` from strings.

**Rationale**: 30 days is the clarified default. 500 per kind per namespace is
the default history limit (10) times fifty checks, well above any shipped
sample set and small enough that a list stays cheap. Both belong in the
configuration model, not on any `v1` kind (spec assumption).

## R7. The `v1` schema deltas

**Decision**, per kind, beyond carrying every alpha rule forward (FR-003):

- All seven: `Spec` serialised as `json:"spec"` with
  `+kubebuilder:validation:Required` (FR-006). `HealthReport` keeps its
  spec-level `self == oldSelf` rule on the now-required field.
- `AddonCheck`: conditions gain `+listType=map`, `+listMapKey=type`,
  `patchStrategy:"merge" patchMergeKey:"type"` (FR-007); printer columns
  Result `.status.lastResult`, Addon `.spec.addonType`, Last Run
  `.status.lastRunTime`, Age (FR-008); `spec.paused` removed (FR-010).
- `HealthReport`: printer columns Result `.spec.result`, Source
  `.spec.sourceRef.kind`, Name `.spec.sourceRef.name`, Observed
  `.spec.observedAt`, Age (FR-008).
- `HealthCheck`: `spec.paused` removed; `checkRef.apiVersion` gains
  `+kubebuilder:validation:Enum=fathom.skaphos.io/v1` with the doc comment
  updated (FR-009). The controller's target registry, default and watch
  mapping move to `v1`.
- `NodeCertificateCheck`: `spec.paused` removed.
- `ClusterHealth`, `DNSCheck`, `NodeHealthCheck`: version change only.

**Rationale**: An enum is the narrowest admission-time rule that still allows
a compatible widening later (adding `v2` is an additive enum change). Printer
column choices follow the sibling kinds' `lastResult` / `lastRunTime` pattern;
`HealthReport` has no status, so its verdict column reads `spec.result`.

## R8. Pause removal footprint

**Finding** (non-test): three type fields; `AddonCheck` controller condition
constant, condition write, runtime-wake withdrawal, `resolveAddonAdapter`
branch, `runtimeBacked` guard and queued-run no-op; `HealthCheck` controller
condition write and mirror skip; `NodeCertificateCheck` `reconcilePaused`
(the only caller that tears the DaemonSet down outside invalid-spec handling,
which keeps `revokeNodeCertAgent`); CLI kind descriptor `Paused` accessor and
its six rows, `run.go` three sites, `wait.go` terminal-reason set,
`describe.go` three prints; `docs/guides/monitoring.md` two mentions;
`docs/reference/fathomctl.md` run semantics.

**Decision**: Remove all of the above; `revokeNodeCertAgent` stays for the
invalid-spec path. The `Paused` descriptor field is deleted rather than
stubbed so no kind can grow it back silently.

## R9. The export verb

**Decision**: `fathomctl export` (top-level, read-only). It discovers the
live CRD for each of the six authorable kinds (`ClusterHealth`, `HealthCheck`,
`AddonCheck`, `NodeCertificateCheck`, `DNSCheck`, `NodeHealthCheck`; reports
are data and are skipped), refuses with `nothing-to-export` and exit 1 when
none serves `v1alpha1`, lists each kind as unstructured at the alpha version,
and renders every object with `apiVersion` rewritten to `v1`, `spec.paused`
removed, `spec.checkRef.apiVersion` rewritten when present, status dropped and
server metadata stripped through the existing `sanitizeForExport`. Output is
multi-document YAML to stdout or `--output <file>` (atomic write, reusing the
`exportResources` writer refactored onto an `io.Writer`). Objects that cannot
be rendered are reported on stderr with kind, namespace, name and reason, and
the exit code is 1 while the rest are still emitted. Flags: `--output`,
`--namespace` (default all), the global kubeconfig and timeout flags.

**Rationale**: Matches the "render offline, never apply" shape of the
definition verbs and the spec's clarification. Discovering the served version
from the live CRD rather than assuming it keeps the verb honest on a cluster
already upgraded.

**Alternatives rejected**: `uninstall --purge-crds --export` (exports reports
and definitions too, and only as a side effect of purging); a `migrate`
subcommand group (one verb does not justify a group).

## R10. CLI version handling after promotion

**Finding**: `refuseUnknownKinds` refuses any Fathom CRD that does not serve
the compiled-in alpha version, and `fathomResources` lists purge kinds at the
alpha GV (lifecycle_cleanup.go:203-347).

**Decision**: Both read the served version from the live CRD instead of a
compiled constant: the seven kinds are expected at `v1`, the definition kinds
at `v1alpha1`, and purge enumerates whatever each live CRD serves. The kind
descriptor table is retyped to `v1`.

## R11. Observability of pruning

**Decision**: Two metrics in `internal/metrics`: gauge
`fathom_healthreports_retained{namespace,source_kind}` (orphaned reports
currently held) and counter
`fathom_healthreports_pruned_total{namespace,source_kind,reason}` with reason
`window` or `cap`. One Event per namespace per pass that removed anything,
reason `ReportsPruned`, action `Retain`, recorded regarding the most recently
pruned report: an Event whose involved object is cluster-scoped must live in
`default`, so the Namespace cannot be the subject, while an Event may
reference an object that no longer exists (Pod events do), which keeps it in
the affected namespace. Message names counts per reason. Both metrics get rows in
`docs/guides/monitoring.md`, which `contract_doc_test.go` enforces.

## R12. Generation, docs and packaging switches

**Decision**: `docs:api-ref` source path becomes `api` so one reference
covers both group versions; `test:api` adds `./api/v1`; samples are renamed
`fathom_v1_*.yaml` and every path reference follows (Taskfile
`e2e:cluster:samples`, `scripts/e2e-shards.sh`, `scripts/e2e_shards_gate_test.go`,
`scripts/check-version-lockstep.sh`, `test/e2e/fixtures/README.md`, thirteen
e2e specs); e2e fixtures carrying the alpha `apiVersion` move to `v1` except
the `lifecycle/older-crds` baseline, which is history; `PROJECT` entries for
the seven kinds move to `v1`; the OLM base CSV's `owned` list is regenerated
and gains the three kinds it is missing today. `.github/copilot-instructions.md`
and code comments naming the alpha path are updated.

## R13. The adapter contract axis

**Decision**: `pkg/adapter.ContractVersion` stays `1.1.0`. The versioning
reference drops the "alpha at 1.0 is acceptable" stance, states that 1.0.0
ships a stable `v1`, records per-kind served and storage versions, and keeps
its existing "what this does not govern" entry for the contract, now phrased
as the deliberate independent axis. The epic (#149) is edited to match.

## R14. e2e: a real upgrade from the previous release

**Finding**: No e2e installs a prior release; the `lifecycle` shard builds two
CLIs from the current source and both embed the same chart. Kind clusters in
that shard already reach the network for helmfile.

**Decision**: Add an `upgrade` scenario to the `lifecycle` shard: install
0.6.0 from the published OCI chart with its published images, apply the
alpha samples, wait for reports, then with the locally built CLI run
`export`, expect `upgrade` to stop with the inventory, run it with
`--accept-incompatible-crds`, apply the exported manifests, and assert
verdicts, retained reports with the deleted-at annotation, and `v1`-only
serving. A short retention window is set through the chart values so pruning
is observed inside the run.

**Rationale**: FR-022 demands the real previous release; nothing else proves
the stored-version replacement against a genuine server.

**Version pinning**: the lifecycle shard builds its CLIs at
`<appVersion>-e2e.N`, which sorts below the chart's own version, so against
a published 0.6.0 install the upgrade would read as a downgrade and the
0.7.0 `*` sanctions (matched by A < entry ≤ B) would never apply. The
alpha-to-`v1` scenario therefore builds its CLI at exactly the newest
`releases[].version` in `upgrade-path.yaml` (the stop itself, 0.7.0) and the
Taskfile tags the locally built images under that version too. The scenario
is specific to the 0.6.0 to `v1` replacement and is retired when 0.6.0 drops
below the upgrade-path floor.

## R15. Decision records

**Decision**: Two ADRs. `0009-stable-v1-single-served-version.md`: `v1` is the
only served and stored version, alpha is replaced not converted, the three
version axes and their relationship. `0010-healthreport-retention.md`: owner
references give way to label identity plus a retention sweep, amending ADR
0002's cleanup rationale, with the window and cap as configuration.

## R16. Delivery sequencing

**Decision**: One feature branch, delivered as four reviewable pull requests
in dependency order, each green on its own:

1. **Preparation, behaviour-neutral**: shared prune helper, duration option
   kind, `servedVersionRemoval` in the compat gate, CLI served-version
   discovery, `DefinitionLeaderEpoch` relocation groundwork.
2. **`api/v1`**: the package, every importer switched, pause removed, schema
   deltas, generated artifacts, samples, docs, PROJECT, CSV, allowlist and
   upgrade-path entries, ADR 0009.
3. **Upgrade and export**: `replace` action with inventory and consent,
   `fathomctl export`, docs, the e2e upgrade scenario.
4. **Retention**: UID label, no owner references, retention reconciler, cap,
   metrics, Event, options, docs, ADR 0010.

**Rationale**: The constitution asks for focused PRs; the spec's scope is one
feature but four separable review surfaces. PR 2 is the breaking one and
carries the `feat!:` commit.
