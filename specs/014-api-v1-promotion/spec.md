<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Feature Specification: Stable `v1` API for the Established Check Kinds

**Feature Branch**: `feature/149-api-v1-promotion`

**Created**: 2026-10-18

**Status**: Draft

**Input**: User description: "Promote the established Fathom CRDs from v1alpha1 to a stable api/v1 for the 1.0.0 release, as one coordinated change covering GitHub issues #149 (epic), #262 (remove spec.paused) and the API-3, API-4, API-6 findings from #257. Author api/v1 fresh and ship v1 as the only served and stored version, dropping v1alpha1 with no conversion webhook; alpha adopters delete and recreate their CRs on upgrade. Fold in every pending schema fix: policy.enabled default, cluster-scoped ClusterHealth with all-namespace selector semantics, field immutability for addonType/checkRef/HealthReport spec, validation hardening (interval/timeout floors, policy/threshold/selector validation), printer columns and listType=map on conditions, required top-level spec so immutability rules cannot be bypassed by remove-then-re-add, bounded and validated checkRef.apiVersion, and removal of spec.paused plus the Paused condition and reconcilePaused DaemonSet teardown with delete as the only way to stop a check. Decide and record HealthReport history retention when a check is deleted. Align the three version axes at 1.0.0: operator SemVer 1.0.0, CRD GVK v1, adapter ContractVersion 1.0. Update generated CRDs, RBAC, samples, docs/reference/api.md, api-versioning.md, the fathomctl kind table and lifecycle upgrade path, the Helm chart CRD apply, crd-compat allowlist, and RELEASE.md migration guidance. The new AddonDefinition and AddonDefinitionBinding kinds stay on their own v1alpha1 track and are out of scope. e2e on the recreated CRDs is a required gate."

**Tracks**: skaphos/fathom#149 (epic), #262 (remove pause), #257 findings API-3, API-4, API-6. Milestone v0.7.0.

## Context and what is already done

The epic was written on 2026-07-12. Several of the schema fixes it lists have
since shipped at `v1alpha1` during the sanctioned alpha-churn window and are
**not** re-specified here; they are carried forward unchanged into `v1`:

| Epic item | Status today |
| --- | --- |
| `policy.enabled` default true (skaphos-resources#417) | Shipped (`*bool`, schema default). |
| `ClusterHealth` cluster-scoped with allowlist / denylist namespace semantics (#418) | Shipped. |
| Immutability of `addonType`, `checkRef`, and `HealthReport.spec` (#419) | Shipped as field-level rules. |
| Interval / timeout floors, policy and threshold validation, CRD compatibility gate (#436, spec 003) | Shipped. |
| `checkRef.apiVersion` length bound (#267) | Shipped; the *value* is still unchecked (API-6 remainder). |

What this feature actually changes is listed under Requirements. The kind set
has also grown since the epic: Fathom now serves seven check kinds at
`v1alpha1` (`AddonCheck`, `HealthCheck`, `ClusterHealth`, `HealthReport`,
`NodeCertificateCheck`, `DNSCheck`, `NodeHealthCheck`) plus the two runtime
definition kinds, which stay on their own alpha track.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Adopt a stable health API (Priority: P1)

A platform team wants to depend on Fathom's health resources from their own
GitOps repositories, dashboards and automation without fearing that the next
operator release changes the shape of what they wrote or what they read. They
install the release that ships this feature, author `HealthCheck`,
`AddonCheck` and `ClusterHealth` resources against the stable `v1` API, and
receive the published promise that the fields they use will not be removed,
renamed or re-meant for the lifetime of `v1`.

**Why this priority**: This is the whole point of the release. Everything else
in this feature exists so that the promise can be made honestly.

**Independent Test**: Install the operator on a fresh cluster, apply the
shipped `v1` samples for every promoted kind, confirm they are accepted and
reconciled to a verdict, and confirm that a manifest written against
`v1alpha1` for a promoted kind is rejected by the API server with a message
that names the supported version.

**Acceptance Scenarios**:

1. **Given** a fresh cluster with this release installed, **When** a user
   applies the shipped sample for each promoted kind, **Then** each is accepted,
   reconciled, and reaches a verdict exactly as the equivalent `v1alpha1`
   sample did on the previous release.
2. **Given** the same cluster, **When** a user applies a manifest whose
   `apiVersion` names the alpha version of a promoted kind, **Then** the API
   server rejects it, and the rejection names the version that is served.
3. **Given** the published API reference and versioning policy, **When** a
   user reads the entry for a promoted kind, **Then** it states that the kind
   is stable, that incompatible changes are forbidden under `v1`, and which
   kinds (if any) remain alpha.
4. **Given** a `ClusterHealth` on this release, **When** its selected
   `HealthCheck` statuses change, **Then** the aggregate verdict is still
   derived only from `HealthCheck` status, never from report history.

---

### User Story 2 - Upgrade an existing installation without stranding resources (Priority: P1)

A cluster administrator runs the previous release with alpha-versioned checks
and report history in place. They need to move to this release without
discovering, after the fact, that the operator can no longer read their
resources. The upgrade path tells them plainly, before anything changes, that
the alpha resources will not survive, shows which resources are affected,
refuses to proceed until they have acted, and gives them a working procedure
to recreate their checks on the stable version.

**Why this priority**: A stable API promise is worthless if the step that
delivers it silently destroys the adopter's existing intent. The recreate path
is the single hard-to-reverse act in this feature.

**Independent Test**: Start from a cluster running the previous release with
at least one resource of every promoted kind plus accumulated report history,
run the guided upgrade, and confirm that it stops with an inventory of the
alpha resources, that it proceeds only after the resources are removed or the
administrator explicitly accepts their loss, and that recreating the checks
from converted manifests restores equivalent health coverage.

**Acceptance Scenarios**:

1. **Given** an installation of the previous release with alpha resources
   present, **When** the administrator runs the guided upgrade, **Then** the
   upgrade stops before changing the cluster, lists every alpha resource by
   kind, namespace and name, and states that those resources and their
   report history will be removed.
2. **Given** that stop, **When** the administrator removes or exports the
   listed resources and re-runs the upgrade, **Then** the upgrade completes,
   the stable API is served, and no alpha-versioned resource remains readable
   or listed.
3. **Given** that stop, **When** the administrator re-runs the upgrade with the
   explicit acceptance of incompatible changes already offered by the upgrade
   verb, **Then** the upgrade removes the alpha resources as part of the
   version replacement and records that it did so in its output.
4. **Given** an administrator applying the operator manifests without the
   guided upgrade (plain manifests or a GitOps controller), **When** the
   release notes and upgrade documentation are followed, **Then** they
   contain the same pre-conditions, the same inventory command and the same
   recreate procedure, so the two paths cannot diverge.
5. **Given** an alpha `HealthCheck` whose `checkRef` still names the alpha
   version of a promoted target kind, **When** it is recreated on `v1` without
   editing the reference, **Then** it is rejected at admission with a message
   that names the accepted versions.

---

### User Story 3 - Stop a check by deleting it (Priority: P2)

A platform user needs to stop a check from running. On this release there is
no pause switch; the way to stop a check is to delete the resource, and the
way to resume is to recreate it. The user understands up front what deleting
does to the check's report history, because the retention behaviour is a
documented decision rather than a surprise.

**Why this priority**: Pause is removed because Fathom emits signal and does
not own alert suppression; that boundary is part of the contract being
frozen at `v1`. It ranks below the API promise and the upgrade path because it
removes a feature rather than adding one, and the removal costs nothing extra
while `v1` is being authored fresh.

**Independent Test**: Confirm that the pause field is absent from every
promoted kind's schema, that a manifest carrying it is rejected, that no
`Paused` condition is ever written, that the node-agent workload is torn down
only on check deletion, and that report history after a check delete behaves
exactly as the retention decision states.

**Acceptance Scenarios**:

1. **Given** a manifest for a promoted kind that sets the former pause field,
   **When** it is applied, **Then** it is rejected as an unknown field.
2. **Given** a running `NodeCertificateCheck` with an agent workload deployed,
   **When** the check is deleted, **Then** the agent workload is removed, and
   there is no other operator-driven way to remove it.
3. **Given** a check with accumulated report history, **When** the check is
   deleted, **Then** the history behaves as decided in FR-013 and that
   behaviour is stated in the user documentation for reports.
4. **Given** the `fathomctl run` command targeting a check, **When** the check
   is not paused (the only state that now exists), **Then** no "paused"
   outcome can be produced and the command's help no longer mentions one.

---

### User Story 4 - Immutability and merge rules that cannot be bypassed (Priority: P2)

A security-minded reviewer wants the stable schema to actually enforce what it
documents. Fields documented as immutable must stay immutable even when the
whole `spec` is removed and re-added, condition lists must merge the same way
on every kind so multiple writers do not clobber each other, and a reference
to another Fathom resource must not silently accept a foreign API group.

**Why this priority**: These are the API-3, API-4 and API-6 findings from the
v0.5.0 adversarial review. They are cheap to fold into a fresh `v1` and
expensive to fix after `v1` freezes, because tightening validation on a stable
version is an incompatible change.

**Independent Test**: For each promoted kind, attempt the remove-then-re-add
edit against an immutable field and confirm rejection; apply a condition on
`AddonCheck` from two distinct writers and confirm both survive; apply a
`HealthCheck` whose `checkRef.apiVersion` names a foreign group and confirm
rejection.

**Acceptance Scenarios**:

1. **Given** an existing `AddonCheck`, **When** a client replaces the object
   with a copy whose `spec` is absent, **Then** the write is rejected because
   `spec` is required.
2. **Given** an existing `HealthCheck` with `checkRef` set, **When** a client
   submits a two-step edit (remove `spec`, then re-add it with a different
   `checkRef`), **Then** the first step is rejected and the reference never
   changes.
3. **Given** an `AddonCheck`, **When** two writers each set a distinct
   condition type using server-side apply, **Then** both conditions are
   present afterwards, matching how the sibling kinds already behave.
4. **Given** a `HealthCheck` whose `checkRef.apiVersion` names a group other
   than Fathom's, **When** it is applied, **Then** it is rejected with a
   message naming the accepted group and versions.
5. **Given** a `kubectl get` of `AddonCheck` or `HealthReport`, **When** no
   output flags are passed, **Then** the columns shown convey the resource's
   verdict and age at a glance, consistent with the sibling kinds.

---

### User Story 5 - Generated and written artifacts agree with the shipped API (Priority: P3)

A contributor or adopter reading any Fathom documentation, sample, chart,
generated reference or release note sees one consistent story: which kinds
are stable, which are alpha, what the upgrade requires, and what the three
version axes (operator release, CRD version, adapter contract) mean at 1.0.

**Why this priority**: The versioning reference currently states the opposite
intent (alpha-at-1.0 is acceptable; the axes are orthogonal). Leaving stale
guidance in place would make the stable promise unverifiable from the
documentation, which the constitution forbids.

**Independent Test**: The generated-artifact drift gate passes; every
sample, guide, README and reference that names a promoted kind names it at
`v1`; the versioning reference states the 1.0 intent and the table of served
versions matches the shipped CRDs; the release notes carry the upgrade
pre-conditions.

**Acceptance Scenarios**:

1. **Given** the repository at the release commit, **When** the generated
   artifact check runs, **Then** CRDs, deep-copy code, RBAC, samples and the
   API reference are all in sync with the `v1` types.
2. **Given** the user-facing documentation, **When** searched for the alpha
   version string of a promoted kind, **Then** the only remaining mentions are
   in historical decision records and the upgrade / migration guidance.
3. **Given** the versioning reference, **When** read, **Then** it records the
   decision that 1.0.0 ships a stable `v1`, lists the served and storage
   version per kind accurately, and explains how the adapter contract version
   relates to the API version as decided in FR-016.

---

### Edge Cases

- **Alpha resources that exist but the inventory cannot read** (the alpha CRD
  was already removed by hand, or the operator was uninstalled first): the
  upgrade must still succeed, must not report the absence as an error, and the
  documentation must say that history is gone in that case.
- **GitOps controllers still applying alpha manifests after upgrade**: the
  apply is rejected by the API server; the rejection must be the plain
  "no matches for kind at that version" error that GitOps tooling already
  surfaces, with no operator-side masking.
- **Mixed version references**: a `HealthCheck` at `v1` may reference a kind
  that remains alpha (if any kind is left alpha by the Q1 decision). The
  accepted set of `checkRef.apiVersion` values must be exactly the served
  version of each referenced kind, and the acceptance message must list them.
- **A check deleted while an adapter run is in flight**: the run completes or
  is cancelled according to existing bounded-reconcile rules; no report may be
  created for a check that no longer exists.
- **The `HealthReport` retention decision interacts with
  `spec.historyLimit`**: whatever FR-013 decides, the cap on retained reports
  per check must keep applying while the check lives.
- **The compatibility gate on a wholesale version change**: diffing the new
  `v1` CRD against the release baseline that only knew `v1alpha1` must report a
  sanctioned, reviewed change per CRD rather than a wall of unrelated findings.
- **An administrator who skips the guided upgrade and applies the chart or
  manifests directly** must get the same outcome: alpha resources are removed
  with the CRD replacement, and nothing is left half-converted.
- **The runtime definition kinds** (`AddonDefinition`,
  `AddonDefinitionBinding`) must be untouched: their served version, schema,
  bindings and drain state survive the upgrade unchanged.
- **A `v1` object whose `spec` is valid but was authored with fields that only
  existed in alpha** (for example the pause field) is rejected, not silently
  pruned, so that intent is never dropped without the author knowing.

## Requirements *(mandatory)*

### Functional Requirements

**Version promotion**

- **FR-001**: The release MUST serve a stable `v1` version of each promoted
  kind as the only served and only stored version; no alpha version of a
  promoted kind is served after upgrade.
- **FR-002**: The set of promoted kinds MUST be [NEEDS CLARIFICATION: the
  epic named "four existing CRDs" when there were four; there are now seven
  check kinds at alpha. Promote only the five kinds that shipped by v0.5
  (AddonCheck, HealthCheck, ClusterHealth, HealthReport,
  NodeCertificateCheck) and leave the v0.6.0 kinds (DNSCheck,
  NodeHealthCheck) alpha, or promote all seven?]. Kinds not promoted keep
  their alpha version and are documented as alpha.
- **FR-003**: `v1` MUST be authored as the complete schema with every already
  shipped alpha hardening carried forward unchanged (defaults, floors,
  immutability rules, namespace scoping, bounded list sizes). No field,
  default or validation that exists at alpha today may be weakened at `v1`.
- **FR-004**: No conversion between alpha and `v1` MUST be provided. The alpha
  version is removed, not deprecated-and-served, because alpha carries no
  support window under the project's versioning policy.
- **FR-005**: The runtime definition kinds MUST remain at their current alpha
  version with no change to schema, served version or behaviour.

**Schema fixes folded into `v1`**

- **FR-006**: Every promoted kind MUST require `spec` at the top level so that
  field-level immutability rules cannot be bypassed by removing and re-adding
  the whole `spec`.
- **FR-007**: `AddonCheck` status conditions MUST merge by condition type under
  server-side apply, consistent with the sibling kinds.
- **FR-008**: `AddonCheck` and `HealthReport` MUST show a verdict and age in
  default list output, consistent with the sibling kinds.
- **FR-009**: `HealthCheck.checkRef.apiVersion`, when set, MUST be validated
  at admission to name Fathom's API group at a served version of the
  referenced kind; any other value is rejected with a message listing the
  accepted values. When unset it defaults to the served version of the
  referenced kind.

**Pause removal**

- **FR-010**: No promoted kind MUST carry a pause field; a manifest that sets
  one is rejected as an unknown field.
- **FR-011**: The operator MUST never write a `Paused` condition, and the only
  operator action that removes a check's node-agent workload is deletion of
  the check.
- **FR-012**: The CLI MUST no longer read, display or branch on a pause state
  for any kind.
- **FR-013**: The behaviour of a check's report history when the check is
  deleted MUST be [NEEDS CLARIFICATION: today reports are garbage-collected
  with their check. Keep that (delete removes history; adopters export first
  with the reports command), or retain reports for a bounded time after
  deletion so "was this passing before the incident?" survives a deliberate
  stop?] and MUST be stated in the reports documentation and in the delete
  guidance that replaces pause.

**Upgrade and migration**

- **FR-014**: The guided upgrade MUST detect alpha resources of promoted kinds
  before changing the cluster, list them by kind, namespace and name, state
  that they and their report history will be removed, and stop. It proceeds
  only when none remain or when the administrator passes the existing
  explicit acceptance for incompatible CRD changes, in which case it removes
  them as part of the version replacement and reports what it removed.
- **FR-015**: The release's entry in the upgrade-path data MUST mark this
  release as a mandatory stop with every promoted CRD's version replacement
  sanctioned, so every later CLI knows the stop, and the compatibility gate
  MUST report each promoted CRD's change as sanctioned with a reason and the
  tracking issue.
- **FR-016**: The relationship between the operator release, the CRD version
  and the adapter contract version at 1.0 MUST be [NEEDS CLARIFICATION: the
  epic's 2026-07-12 decision says all three align at 1.0, but the adapter
  contract is already at 1.1.0 since the ratio-key reservation shipped. Bump
  the contract to 2.0.0 at this release so the three axes read 1.0.0 / v1 /
  2.0.0 with a documented major, leave the contract at 1.1.0 and document that
  the contract axis stays independent, or something else?] and MUST be
  recorded in the versioning reference and the adapter authoring guide.
- **FR-017**: Release notes and the release process document MUST carry the
  upgrade pre-conditions, the inventory step, the recreate procedure, and the
  statement that alpha resources and history do not survive, in the same terms
  the guided upgrade uses.

**Artifacts and documentation**

- **FR-018**: All generated artifacts (CRD manifests, deep-copy code, RBAC,
  samples, OLM bundle metadata, the API reference) MUST be regenerated from
  the `v1` types by the pinned tooling and pass the drift gate.
- **FR-019**: Every sample, guide, README section and reference page that
  names a promoted kind MUST name it at `v1`; alpha strings for promoted kinds
  remain only in immutable decision records and migration guidance.
- **FR-020**: The versioning reference MUST be updated to state that 1.0.0
  ships a stable `v1` API, list served and storage versions per kind
  accurately, and replace the "alpha at 1.0 is acceptable" stance.
- **FR-021**: The `ClusterHealth` external contract MUST remain derived only
  from `HealthCheck` status; promotion changes its version, not its
  derivation.

**Validation gate**

- **FR-022**: The feature MUST be verified end to end on a real cluster that
  starts on the previous release with alpha resources and report history,
  performs the upgrade, and exercises every promoted kind at `v1`. Unit and
  envtest coverage alone does not satisfy this requirement.

### Key Entities

- **Promoted kind**: a Fathom check or aggregate resource whose served and
  stored version becomes `v1` in this release. Carries the stable-API promise.
- **Alpha resource**: an instance of a promoted kind stored at the alpha
  version on a cluster being upgraded. Must be inventoried and removed; it
  cannot be converted.
- **Report history**: the `HealthReport` records owned by a check. Subject to
  the retention decision in FR-013.
- **Upgrade stop**: a release that every later upgrade must pass through,
  recorded in append-only upgrade-path data. This release is one.
- **Sanctioned change**: a reviewed, reasoned entry that tells the
  compatibility gate an incompatible CRD change is deliberate; one per
  promoted CRD in this release.
- **Version axes**: operator release (SemVer), CRD version (`v1`), adapter
  contract version. Their relationship at 1.0 is decided in FR-016.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the shipped `v1` samples for promoted kinds are accepted
  and reach a verdict on a fresh install, with behaviour identical to their
  alpha equivalents on the previous release.
- **SC-002**: 0 alpha-versioned resources of a promoted kind remain readable
  after a completed upgrade, on both the guided path and the plain-manifest
  path.
- **SC-003**: An administrator following only the release notes can complete
  the upgrade of a cluster with existing checks, including recreating them,
  in under 30 minutes, with no step that requires reading source code.
- **SC-004**: 100% of attempted immutability bypasses (remove-then-re-add of
  `spec`) across promoted kinds are rejected at admission.
- **SC-005**: 0 occurrences of a pause field, a `Paused` condition, or a
  "paused" CLI outcome remain in schemas, operator behaviour, CLI output or
  user-facing documentation.
- **SC-006**: 0 user-facing documents name the alpha version of a promoted
  kind outside decision records and migration guidance.
- **SC-007**: The generated-artifact drift gate, the CRD compatibility gate
  (with sanctioned entries), the upgrade-path gate, and the end-to-end suite
  all pass on the release commit.
- **SC-008**: After this release, any incompatible change to a promoted kind
  is caught by the compatibility gate as an unsanctioned finding, giving the
  stable promise an automated backstop.

## Assumptions

- The stable `v1` API ships in the v0.7.0 operator release and is frozen from
  that point; the operator reaches 1.0.0 in the following milestone without
  further API changes. "Last breaking release before 1.0.0" is read literally.
- Alpha versions carry a zero-length support window under the project's CRD
  versioning policy, so removing `v1alpha1` without a deprecation period is
  within policy and needs no exception.
- No known adopter is running alpha resources that cannot be recreated from
  Git. If one surfaces during review, the serve-both-with-conversion path is
  reconsidered explicitly and noted on the epic; it is not silently adopted.
- The existing "accept incompatible CRDs" acceptance on the upgrade verb is
  the right consent mechanism for removing alpha resources; no new flag is
  introduced.
- The existing reports command is sufficient as the "export before delete"
  tool if FR-013 resolves to keeping garbage collection; no new export verb
  is introduced by this feature.
- The runtime definition kinds are untouched, including their generated
  inventory and the CLI's definition verbs.
- The e2e stack used for this feature is the existing kind-based suite
  extended with an upgrade-from-previous-release scenario; no new test
  infrastructure is assumed.
- The node-agent image, probe image and operator image version lockstep is
  unaffected; this feature changes no image contents beyond the schema.
