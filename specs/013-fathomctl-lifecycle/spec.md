<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Feature Specification: fathomctl Lifecycle Verbs

**Feature Branch**: `feature/330-fathomctl-lifecycle`

**Created**: 2026-10-11

**Status**: Draft

**Input**: User description: "fathomctl lifecycle verbs (GitHub issue
skaphos/fathom#330, follow-on to #204 / specs/009-fathomctl-cli). Add
`fathomctl install`, `fathomctl upgrade`, `fathomctl uninstall` (and decide on
`fathomctl status`) that own the whole operator lifecycle including the CRD
handling Helm skips. The Helm chart stays the source of truth; every verb runs
as the kubeconfig user, honours the CLI request timeout and reports a rollout
that does not become Available in time as a failure; OLM-managed installs are
out of scope. Open questions: Helm SDK vs embedded kustomize manifests, chart
source offline, whether `uninstall --purge-crds` exports resources first, and
whether `status` lands here."

## Overview

Installing Fathom today is a Helm workflow the user drives by hand, with
ordering rules they have to remember:

- `helm install` the published OCI chart.
- Before any `helm upgrade` that changes a CRD schema, `kubectl apply` the new
  CRDs by hand, because Helm installs CRDs once and never upgrades them.
- `helm uninstall`, then decide what to do with the CRDs Helm leaves behind and
  with the check resources that still use them.
- Know that the node-agent DaemonSets belong to the `NodeCertificateCheck` and
  `NodeHealthCheck` resources rather than to the operator, so they keep running
  after the operator is gone.

Those edges are documented in `docs/guides/getting-started.md` and the README,
but documented is not the same as safe. `docs/guides/agent-operations.md`
already positions Fathom as something an agent installs and operates; an agent
shelling out to three tools with ordering rules it has to remember is exactly
the failure mode `fathomctl` exists to remove.

This feature adds four verbs to `fathomctl` — `install`, `upgrade`,
`uninstall` and `status` — that own the whole sequence, including the CRD
handling Helm refuses to do. They drive the published Helm chart rather than a
second set of manifests, so an install started with `helm` can be continued
with `fathomctl` and the reverse.

## Decisions and Tradeoffs

| Decision | Choice | Rationale |
|---|---|---|
| Install engine | The Helm chart, driven as a Helm release (release state stored the way `helm` stores it) | The chart is the documented install path and the source of truth (#330 constraint). Recording the release where `helm` records it means `helm ls`, `helm history` and `helm rollback` keep working, and an install can move between `helm` and `fathomctl` in either direction. Rendering embedded kustomize manifests was simpler and needed no release state, but it is a second install path that drifts from the chart and is invisible to every Helm user. |
| Target version and chart source | Each `fathomctl` installs and upgrades to exactly its own version, from the chart embedded in it at release time; there is no `--version` flag | The CLI then knows every rule it applies — the target's CRDs, sanctioned changes and upgrade path are its own — so it never plans against a release it has not seen. It works offline and in air-gapped clusters, and the binary never carries older manifests. Every release's `fathomctl` stays published, so reaching another version means running that release's CLI. Accepting other versions (fetched from the OCI chart) was more flexible but needs registry access and lets an older CLI plan an upgrade whose rules it does not know. |
| CRD handling | `install` and `upgrade` apply the release's CRDs themselves, before the operator rollout, and never leave CRD upgrades to Helm | Helm installs CRDs once and never upgrades them; that gap is the reason this feature exists. Applying CRDs first means the new operator never starts against an older schema. |
| CRD compatibility | Before applying, `upgrade` compares each installed CRD with the target's and refuses an incompatible change the target release does not sanction, and any change of a CRD's scope | Matches the repository's `crd-compat` policy at the point where it matters to a user: an unsanctioned incompatible change is a refusal, not a silent apply. Kubernetes cannot change a CRD's scope in place, so a scope change is always a refusal with the manual path explained. |
| Sanctioned incompatible CRD changes | Applied only with `--accept-incompatible-crds`; `--yes` does not imply it | A sanctioned change can still make existing resources fail validation. A dedicated flag is the hardest to pass by accident and makes an agent opt in explicitly; a generic `--yes` would let routine upgrade scripts accept it silently. |
| Upgrade path guidance | Each release declares the oldest version it can be upgraded from directly; `upgrade` refuses a longer jump and names the version(s) to step through | A sanctioned incompatible change is usually paired with a release that migrates existing resources. Skipping that release can leave resources the new schema rejects. Naming the required intermediate version turns a dead end into a runbook step; leaving users to read changelogs is how upgrades get skipped. |
| Backup before purge | Only on request, with `--export <file>`; the prompt names the flag | Explicit and predictable: a purge does exactly what it says, and the prompt reminds the user before anything is deleted. A default backup was safer but leaves files behind on every purge. |
| `status` verb | Included in this feature | The other three verbs need the same facts (is it installed, which version, by which tool, are the CRDs current) to be idempotent and to print a plan; exposing them as a verb costs little and gives agents and runbooks one read-only question to ask. |
| Installs not made through Helm | Detected and refused, never adopted | An operator installed from kustomize manifests (`dist/install.yaml`) or through OLM has no Helm release to continue. Taking it over silently would put two tools in charge of the same objects. OLM is out of scope (#330); the refusal names the tool that owns the install. |
| Uninstall leaves no Fathom workloads | Without `--purge-crds`, uninstall removes the operator and the workloads the operator created for check resources (node-agent DaemonSets, probe Pods and their per-check ServiceAccounts, roles and NetworkPolicies) but keeps every CRD and every check resource | Those workloads are owned by the check resources, not the operator, so removing the operator alone leaves agents running with nothing to report to. Keeping the check resources means a later reinstall picks them up and recreates the workloads; nothing the user wrote is lost. |

Doing nothing leaves CRD upgrades as a manual step users forget, leaves
agent DaemonSets running after an uninstall, and leaves agents scripting three
tools with ordering rules.

## Clarifications

### Session 2026-10-11

- Q: Should `uninstall --purge-crds` save the resources it is about to delete? → A: Only when asked. `--export <file>` writes every Fathom resource to that file before anything is deleted, and the confirmation prompt names the flag; without it nothing is saved.
- Q: When `upgrade` meets an incompatible CRD change the target release *does* sanction, what must the user do? → A: Pass a dedicated acknowledgement flag, `--accept-incompatible-crds`. Without it the upgrade refuses and the plan names each sanctioned incompatible change; `--yes` does not imply it.
- Q: When an upgrade cannot go straight from the installed version to the target, what guidance does the user get? → A: Every release declares the oldest version it can be upgraded from directly. An upgrade from anything older is refused, and the message names the intermediate version(s) to upgrade through first — the release that applies the incompatible change — so the user is never left to work out the path.
- Q: If the new operator does not become Available within `--timeout` during `upgrade` (after the CRDs were updated), what happens? → A: By default the new release is left in place, the command exits `1`, names the unmet condition and prints the recovery commands. An opt-in `--rollback-on-failure` instead rolls the operator back to the previous release revision, and is refused up front when the upgrade would apply an incompatible CRD change; CRDs are never rolled back.
- Q: Should `uninstall` delete the namespace the operator was installed into? → A: Only with `--delete-namespace`. Without it the namespace is kept and the message says so. With it, the namespace is deleted after the operator is removed; if it holds objects Fathom did not create, they are listed and the same confirmation as `--purge-crds` (prompt or `--yes`) is required first.
- Q: Can `install`/`upgrade` target a version other than the CLI's own? → A: No. Each `fathomctl` installs and upgrades to exactly its own version, from the chart embedded in it; there is no `--version` flag. Every release's `fathomctl` stays published, so another version means running that release's `fathomctl`, and a multi-step upgrade means running each intermediate release's `fathomctl` in turn.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Install Fathom with one command and know it worked (Priority: P1)

A platform engineer (or an agent acting for one) runs `fathomctl install` and
gets a working operator: CRDs present, operator running at the CLI's own
version with images pinned by digest, and a clear success or failure when the
command returns.

**Why this priority**: Installation is the entry point; nothing else in this
feature matters without it, and it alone removes the most common manual
sequence.

**Independent Test**: On an empty cluster, run `fathomctl install
--create-namespace`, then confirm the CRDs exist, the operator Deployment is
Available, `helm ls` shows the release, and running `install` again changes
nothing.

**Acceptance Scenarios**:

1. **Given** a cluster with no Fathom, **When** the user runs `fathomctl
   install -n fathom-system --create-namespace`, **Then** the namespace, every
   Fathom CRD and the operator are created, the operator becomes Available, the
   command prints the installed version and exits `0`.
2. **Given** any install, **When** it completes, **Then** the operator
   version equals the `fathomctl` version, and its images are referenced by
   digest.
3. **Given** Fathom is already installed at the CLI's version, **When**
   the user runs `install` again, **Then** nothing is changed, the command says
   so and exits `0`.
4. **Given** Fathom is installed at an older version, **When** the user runs
   `install`, **Then** it refuses and directs the user to `upgrade`, rather
   than upgrading as a side effect.
5. **Given** the operator does not become Available within `--timeout`,
   **When** the install runs, **Then** the command stops waiting, reports which
   condition was not met and exits `1`.
6. **Given** the target namespace does not exist and `--create-namespace` is
   not set, **When** the user installs, **Then** it refuses before changing
   anything.

---

### User Story 2 - Upgrade safely, CRDs first (Priority: P1)

An operator owner moves an existing install to a new version. The CLI shows
what will change, applies CRD schema changes before rolling the operator, and
refuses changes that would break existing resources.

**Why this priority**: Upgrades are where today's manual sequence fails —
forgetting the CRD step leaves a new operator running against an old schema.
This is the step Helm will not do.

**Independent Test**: Install release N with release N's `fathomctl`, then
run release N+1's `fathomctl upgrade` across a release that changes a CRD
schema; confirm the CRDs were updated
before the new operator Pods started and that existing check resources still
read back.

**Acceptance Scenarios**:

Throughout, A is the installed version and B is the running `fathomctl`'s
own version — the only version it can upgrade to.

1. **Given** an install at version A, **When** the user runs version B's
   `fathomctl upgrade --dry-run`, **Then** it prints the plan — each CRD
   added, changed or unchanged; the operator version change; any refusal — and
   changes nothing.
2. **Given** a compatible CRD change between A and B, **When** the user
   upgrades, **Then** the CRDs are updated, then the operator is rolled out,
   and the command waits for Available before exiting `0`.
3. **Given** B makes an incompatible change to a CRD that B does not sanction,
   **When** the user upgrades, **Then** nothing is changed and the message
   names the CRD, the incompatible field and the manual path.
4. **Given** B sanctions an incompatible change to a CRD, **When** the user
   upgrades without `--accept-incompatible-crds`, **Then** nothing is changed
   and the message lists the change and its stated reason; **When** the user
   passes the flag, **Then** the change is applied before the rollout.
5. **Given** A is older than the oldest version B can be upgraded from
   directly, **When** the user upgrades (with or without
   `--accept-incompatible-crds`), **Then** nothing is changed and the message
   names the version(s) to upgrade through first, in order, and for the next
   step which `fathomctl` release to download and the command to run with it.
6. **Given** B changes a CRD's scope (namespaced ↔ cluster), **When** the user
   upgrades, **Then** nothing is changed and the message explains that the
   scope cannot change in place and what the manual path is.
7. **Given** B is older than A (the CLI is older than the install), **When**
   the user upgrades, **Then** it refuses, explains that downgrades are not
   supported, and tells the user to use the `fathomctl` matching version A or
   newer.
8. **Given** no Fathom install exists, **When** the user upgrades, **Then** it
   refuses and points at `install`.
9. **Given** the new operator does not become Available within `--timeout`,
   **When** the upgrade waits, **Then** by default the new release stays in
   place and the command exits `1` with the unmet condition and recovery
   commands; **When** `--rollback-on-failure` was passed and no incompatible
   CRD change was applied, **Then** the operator is rolled back to the previous
   revision before exiting `1`.

---

### User Story 3 - Uninstall and leave nothing running (Priority: P2)

An operator owner removes Fathom. By default the CRDs and every check resource
stay, so nothing they wrote is lost, but nothing Fathom launched keeps
running. Optionally they remove everything, after an explicit confirmation
that names what will be deleted.

**Why this priority**: Uninstall is less frequent than install or upgrade, but
the current behaviour leaves agent DaemonSets running and orphaned CRDs whose
removal silently deletes every check.

**Independent Test**: Install, create one of each check kind including a
`NodeCertificateCheck`, run `fathomctl uninstall`, and confirm no Fathom
workloads remain while every check resource and CRD still exists; then run
`uninstall --purge-crds --yes` and confirm the CRDs and resources are gone.

**Acceptance Scenarios**:

1. **Given** a running install with check resources, **When** the user runs
   `fathomctl uninstall`, **Then** the operator, its RBAC and every workload
   the operator created for check resources (node-agent DaemonSets, probe Pods
   and their per-check ServiceAccounts, roles and NetworkPolicies) are
   removed, and every CRD and check resource remains.
2. **Given** `--purge-crds` on a terminal, **When** the user runs uninstall,
   **Then** it shows how many resources of each Fathom kind will be deleted,
   asks for confirmation, and deletes nothing if the answer is not yes.
3. **Given** `--purge-crds` without a terminal and without `--yes`, **When**
   the user runs uninstall, **Then** it refuses before deleting anything.
4. **Given** `--purge-crds --yes`, **When** uninstall completes, **Then** no
   `fathom.skaphos.io` CRD, resource or workload remains.
5. **Given** `--purge-crds --export backup.yaml --yes`, **When** uninstall
   runs, **Then** every Fathom resource is written to `backup.yaml` before
   anything is deleted, and re-applying the file after a reinstall restores
   them.
6. **Given** no Fathom install, **When** the user runs uninstall, **Then** it
   reports that there is nothing to remove and exits `0`.
7. **Given** a plain `uninstall`, **When** it completes, **Then** the release
   namespace still exists and the message says so; **Given**
   `--delete-namespace` and a namespace holding only Fathom objects, **Then**
   the namespace is deleted; **Given** `--delete-namespace` and a namespace
   that also holds other objects, **Then** they are listed and nothing is
   deleted without confirmation.

---

### User Story 4 - Ask what is installed (Priority: P2)

A user or agent asks whether Fathom is installed, at which version, by which
tool, whether the CRDs match that version and whether the operator is healthy.

**Why this priority**: It is the read-only question every runbook asks first,
and it is what makes the other verbs' plans and idempotency visible.

**Independent Test**: Run `fathomctl status` against a cluster with no
install, a Helm-managed install, and a kustomize-managed install; each reports
the correct state and exits as specified.

**Acceptance Scenarios**:

1. **Given** a healthy install, **When** the user runs `fathomctl status`,
   **Then** it reports the release name and namespace, the operator version
   and image digests, whether every CRD matches that version, and the
   operator's readiness, and exits `0`.
2. **Given** the CRDs are older than the operator, **When** the user runs
   `status`, **Then** it names the out-of-date CRDs and exits `1`.
3. **Given** an install not managed by Helm, **When** the user runs `status`,
   **Then** it reports the version it can observe and that the install is not
   managed by `fathomctl`.
4. **Given** `-o json` or `-o yaml`, **When** the user runs `status`, **Then**
   the same facts are printed in that format.

---

### Edge Cases

- **Mixed tooling**: an install started with `helm install` is upgraded with
  `fathomctl upgrade`, and the reverse; both must work and `helm history`
  must show both revisions.
- **Values preservation**: `upgrade` keeps the values the release was
  installed with (for example the monitoring and projected-label values)
  unless the user overrides them.
- **Invalid values**: values the chart's schema rejects fail before anything
  in the cluster changes.
- **Interrupted install or upgrade**: a release left in a pending or failed
  state is reported by `status`, and `install`/`upgrade` explain how to
  recover rather than stacking a second change on top.
- **CRDs present without an operator**: a cluster where only the CRDs remain
  (after a plain `uninstall`) can be installed onto; existing check resources
  are kept and picked up.
- **Version skew**: a CLI older than the installed operator refuses
  `install` and `upgrade` and names the matching `fathomctl` release; `status`
  and `uninstall` still work, because they need no chart for another version.
- **Offline and air-gapped clusters**: every verb works without registry
  access to the chart, because the chart is embedded; only the images
  themselves must be pullable (or mirrored by digest).
- **Insufficient permissions**: a verb run without the cluster-admin-level
  permissions it needs fails before changing anything where the check is
  possible, and the message names the missing permission.
- **Non-Helm install**: `install`, `upgrade` and `uninstall` refuse an install
  that was not made through Helm and name the tool that appears to own it.
- **Failed rollout after CRDs changed**: the CRDs stay on the new schema
  whatever happens to the operator; `--rollback-on-failure` is refused when
  the new schema is incompatible with the previous operator.
- **Timeouts**: every API call honours `--request-timeout`; waiting for a
  rollout honours `--timeout`; neither ever waits without bound.

## Requirements *(mandatory)*

### Functional Requirements

**General**

- **FR-001**: `fathomctl` MUST provide `install`, `upgrade`, `uninstall` and
  `status` verbs that use the kubeconfig identity and global flags
  (`--kubeconfig`, `--context`, `-n/--namespace`, `--request-timeout`) already
  defined for the CLI.
- **FR-002**: Every verb MUST install, upgrade or remove the operator through
  the published Helm chart as a Helm release whose state `helm` can read; the
  verbs MUST NOT apply operator manifests from any other source.
- **FR-003**: Each `fathomctl` MUST embed the chart of its own release and
  MUST install and upgrade only to that version; it MUST NOT offer a
  `--version` override or fetch charts at run time. The chart MUST therefore be
  available without network access.
- **FR-004**: Every verb MUST bound each API call by `--request-timeout` and
  every wait by `--timeout`, and MUST report a wait that ends without the
  expected state as a failure, naming the unmet condition.
- **FR-005**: Exit codes MUST follow the CLI's existing contract: `0` on
  success, `1` otherwise.
- **FR-006**: `install`, `upgrade` and `uninstall` MUST refuse an install that
  is not managed by Helm and name the tool that appears to own it.
- **FR-007**: `install` and `upgrade` MUST accept chart values the same ways
  Helm does (values files and individual overrides) and MUST validate them
  against the chart's schema before changing the cluster.

**install**

- **FR-010**: `install` MUST create every Fathom CRD, then the operator, in the
  chosen namespace, creating the namespace only with `--create-namespace`.
- **FR-011**: `install` MUST install the CLI's own version and MUST reference
  the operator, probe and node-agent images by digest.
- **FR-012**: `install` MUST be idempotent: on an install at the CLI's version
  it changes nothing and exits `0`; on an older install it refuses and points
  at `upgrade`; on a newer install it refuses and names the matching
  `fathomctl` release; it MUST never create a second release.
- **FR-013**: `install` MUST wait until the operator is Available, bounded by
  `--timeout`.

**upgrade**

- **FR-020**: `upgrade` MUST apply every CRD change in the target release
  before the operator rollout begins.
- **FR-021**: Before changing anything, `upgrade` MUST compare each installed
  CRD with the target's and MUST refuse an incompatible change the target
  release does not sanction, naming the CRD, the field and the manual path.
- **FR-022**: `upgrade` MUST refuse any change of a CRD's scope, explaining
  that it cannot be applied in place and describing the manual path.
- **FR-023**: For an incompatible change the target release sanctions,
  `upgrade` MUST refuse unless `--accept-incompatible-crds` is passed, and MUST
  list each such change (CRD, field, the release's stated reason) in the
  refusal and in the `--dry-run` plan. `--yes` MUST NOT imply this
  acknowledgement.
- **FR-024**: `upgrade --dry-run` MUST print the full plan (CRDs added,
  changed, unchanged or refused; operator version change; values in effect)
  and change nothing.
- **FR-025**: `upgrade` MUST keep the release's existing values unless the
  user overrides them, and MUST refuse a target version older than the
  installed one.
- **FR-026**: `upgrade` MUST wait until the new operator is Available, bounded
  by `--timeout`. If it is not Available in time, `upgrade` MUST leave the new
  release in place, exit `1`, name the unmet condition and print the recovery
  commands (inspect, retry with a longer `--timeout`, roll the operator back).
- **FR-029**: `upgrade --rollback-on-failure` MUST, when the new operator is
  not Available in time, roll the operator back to the previous release
  revision and then exit `1`. It MUST be refused before anything changes when
  the plan applies an incompatible CRD change (sanctioned or not), because the
  previous operator may not run against the new schema. CRDs MUST never be
  rolled back.
- **FR-027**: Every release MUST declare the oldest version it can be
  upgraded from directly, and the versions whose incompatible changes must not
  be skipped. `upgrade` MUST refuse a transition that starts below that
  minimum or skips such a version, before changing anything, and the message
  MUST name the version(s) to upgrade through, in order, and for the next step
  the `fathomctl` release to download and the command to run with it.
  `--accept-incompatible-crds` MUST NOT bypass this.
- **FR-028**: `upgrade --dry-run` and `status` MUST show the upgrade path from
  the installed version to the running CLI's version: direct, or the
  intermediate `fathomctl` releases required, in order.

**uninstall**

- **FR-030**: `uninstall` MUST remove the operator release and every workload
  the operator created for check resources (node-agent DaemonSets, probe Pods,
  and their per-check ServiceAccounts, roles, role bindings and
  NetworkPolicies), and MUST keep every CRD and every Fathom custom resource.
- **FR-031**: `uninstall --purge-crds` MUST, before deleting anything, show the
  number of resources of each Fathom kind that will be deleted —
  `AddonCheck`, `DNSCheck`, `NodeCertificateCheck`, `NodeHealthCheck`,
  `HealthCheck`, `ClusterHealth`, `HealthReport`, `AddonDefinition` and
  `AddonDefinitionBinding` — and require confirmation in the same shape as
  `fathomctl run`'s large-fan-out guard: an interactive prompt on a terminal,
  `--yes` to skip it, refusal without either.
- **FR-032**: `uninstall --purge-crds --export <file>` MUST write every
  Fathom custom resource to `<file>` in a form that can be re-applied, and
  MUST delete nothing if writing the file fails. The confirmation prompt MUST
  mention `--export` when it is not set; without it nothing is saved.
- **FR-033**: After `uninstall --purge-crds` succeeds, no `fathom.skaphos.io`
  CRD, custom resource or Fathom workload MUST remain.
- **FR-034**: `uninstall` on a cluster without a Fathom install MUST report
  that there is nothing to remove and exit `0`.
- **FR-035**: `uninstall` MUST keep the release namespace unless
  `--delete-namespace` is passed, and say that it was kept. With
  `--delete-namespace` it MUST delete the namespace after the operator is
  removed; if the namespace contains objects Fathom did not create, it MUST
  list them and require the same confirmation as `--purge-crds` (prompt on a
  terminal, `--yes` to skip, refusal without either) before deleting.

**status**

- **FR-040**: `status` MUST report whether Fathom is installed and, when it
  is: the managing tool, release name and namespace, operator version and image
  digests, the operator's readiness, and for each CRD whether it matches the
  operator's version.
- **FR-041**: `status` MUST exit `1` when Fathom is installed but not healthy
  or its CRDs do not match the operator, and `0` when it is installed and
  current or not installed at all.
- **FR-042**: `status` MUST support `-o json` and `-o yaml` with the same
  facts.

**Documentation and permissions**

- **FR-050**: The documentation MUST state the permissions each verb needs
  (install, upgrade and uninstall need cluster-admin-level rights, including
  creating CRDs and cluster RBAC), distinct from the viewer and runner roles of
  #323.
- **FR-051**: `docs/guides/getting-started.md`, `docs/guides/fathomctl.md`,
  `docs/reference/fathomctl.md`, `README.md` and `RELEASE.md` MUST describe the
  verbs, and `docs/guides/agent-operations.md` MUST gain a lifecycle runbook.
  The README's CRD count MUST match the shipped CRDs (nine today).
- **FR-052**: The documentation MUST state plainly that OLM-managed installs
  are not supported by these verbs.

### Key Entities

- **Fathom install**: the operator, its RBAC and its configuration in one
  namespace, recorded as one Helm release; identified by release name and
  namespace; has a version, a values set and a revision history.
- **Fathom CRD set**: the nine `fathom.skaphos.io` CustomResourceDefinitions a
  release ships; cluster-scoped and shared by every install in the cluster;
  each has a schema and a scope.
- **Upgrade path**: what a release declares about how to reach it — the
  oldest version it can be upgraded from directly and which incompatible
  changes it sanctions, with their reasons — from which the CLI derives the
  intermediate versions a longer jump must pass through.
- **Upgrade plan**: for one transition from the installed version A to the CLI's own version B, the per-CRD
  outcome (add, change, unchanged, refuse with reason), the operator change and
  the values in effect.
- **Fathom-managed workloads**: objects the operator creates on behalf of
  check resources — node-agent DaemonSets, probe Pods and their per-check
  ServiceAccounts, roles, role bindings and NetworkPolicies — owned by the
  check resource rather than the operator.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On an empty cluster, one `fathomctl install` command produces a
  working operator with no other tool invoked, and a second identical command
  changes nothing.
- **SC-002**: An upgrade across a release that changes a CRD schema completes
  with one command, and in 100% of such upgrades the CRDs reach the new schema
  before any new operator Pod starts.
- **SC-003**: 100% of unsanctioned incompatible CRD changes and CRD scope
  changes are refused with nothing in the cluster modified, and the message
  names the CRD and the manual path.
- **SC-009**: 100% of upgrades that cannot go straight to the target are
  refused before any change, and the message names the next version to
  upgrade to.
- **SC-004**: After `fathomctl uninstall`, zero Fathom workloads remain and
  100% of the user's check resources and CRDs are still present.
- **SC-005**: After `fathomctl uninstall --purge-crds --yes`, zero
  `fathom.skaphos.io` CRDs, resources or workloads remain; with `--export`,
  100% of the deleted resources are present in the file and re-apply after a
  reinstall.
- **SC-006**: No verb ever waits beyond `--timeout`; a rollout that does not
  become Available in time ends with exit `1` and a named cause.
- **SC-007**: An install started with `helm` can be upgraded and uninstalled
  with `fathomctl`, and an install made with `fathomctl` is listed by `helm ls`
  and can be upgraded with `helm`.
- **SC-008**: An agent following the documented runbook can install, verify,
  upgrade and uninstall Fathom using only `fathomctl`.

## Assumptions

- Users have a kubeconfig with cluster-admin-level permissions for `install`,
  `upgrade` and `uninstall`; `status` needs only read access to the release,
  the CRDs and the operator Deployment.
- One Fathom install per cluster is the supported topology, because the CRDs
  are cluster-scoped and shared.
- Downgrades are out of scope for `upgrade`; `helm rollback` remains available
  for the operator itself and is not extended to CRDs.
- Images are pinned by digest using the digests recorded for each release, per
  the deploy-by-digest contract in `RELEASE.md`.
- The CRD compatibility verdict for an upgrade uses the same rules as the
  repository's `crd-compat` gate. Each release records which incompatible
  changes it sanctions and the oldest version it can be upgraded from
  directly; maintaining that record becomes a release-process step, documented
  in `RELEASE.md`.
- The operator adds no finalizers to Fathom resources, so purging resources and
  CRDs after the operator is gone completes without the operator.

## Out of Scope

- OLM-managed installs (#330): the verbs refuse them and the docs say so.
- Taking over installs made from kustomize manifests (`dist/install.yaml`).
- Downgrading the CRD set.
- Managing more than one Fathom install per cluster.
- Installing or managing the add-ons Fathom checks.

## References

- Issue #330; parent #204 and `specs/009-fathomctl-cli/`; siblings #322 (MCP
  surface) and #323 (chart-shipped viewer/runner ClusterRoles).
- `RELEASE.md` — deploy-by-digest contract and GHCR package publishing.
- `.crd-compat-allowlist.yaml` and `scripts/check-crd-compat.sh` — CRD
  compatibility policy.
- `docs/guides/getting-started.md`, `README.md` — today's manual Helm and CRD
  steps.
- `docs/guides/agent-operations.md` — agent-operated Fathom.
