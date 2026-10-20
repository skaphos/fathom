<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# ADR 0009: The check kinds ship a stable `v1` as their only served version

- **Status:** Accepted
- **Date:** 2026-10-20
- **Deciders:** Shawn Stratton
- **Tracking:** skaphos/fathom#149 (epic), #262, #257; spec `specs/014-api-v1-promotion/`
- **Related:** ADR 0002 (HealthReport as a first-class CRD), ADR 0004
  (HealthCheck as wrapper), ADR 0007 (runtime definition loading),
  `docs/reference/api-versioning.md`

## Context

Fathom's seven check kinds — `AddonCheck`, `HealthCheck`, `ClusterHealth`,
`HealthReport`, `NodeCertificateCheck`, `DNSCheck`, `NodeHealthCheck` — were
served at `fathom.skaphos.io/v1alpha1` through release 0.6.0. The 1.0.0 road
map (#149) commits to a stable API for them, so that platform teams can
depend on these resources from GitOps repositories, dashboards and
automation without the next release changing their shape.

Three facts constrain how that promotion can be made:

1. **Alpha carries a zero-length support window.** The CRD API versioning
   standard this repository mirrors allows an alpha version to be removed in
   any release; the heavy machinery of serving two versions (conversion
   webhook, round-trip tests, storage migration, deprecation windows) exists
   for beta and GA versions that already made a promise.
2. **No conversion infrastructure exists.** The operator has never served two
   versions; the webhook options are unused surface.
3. **The API server refuses to drop a version a CRD still stores.** A CRD
   update whose `spec.versions` omits a version listed in
   `status.storedVersions` is rejected. The only way to stop serving
   `v1alpha1` on a cluster that holds alpha objects is to delete the CRD,
   which cascades those objects, and create it anew.

Two further decisions had to be taken with the promotion because they would
be incompatible changes afterwards: `spec.paused` is removed (#262: Fathom
emits signal and does not own alert suppression; deleting a check is the only
stop), and three admission findings from the v0.5.0 adversarial review are
folded in (required top-level `spec`, map-list conditions on `AddonCheck`, an
enumerated `checkRef.apiVersion`).

## Decision

1. `api/v1` is authored as the complete schema of the seven kinds and is
   their **only served and only stored version** from release 0.7.0.
   `v1alpha1` is removed from those CRDs, not kept unserved, and no
   conversion is provided.
2. Every alpha hardening (defaults, cadence floors, immutability rules,
   namespace scoping, bounded lists) is carried into `v1` unchanged; `v1`
   adds only the deltas listed in the spec. From 0.7.0 on, incompatible
   changes to `v1` are forbidden and the `crd-compat` gate reports them as
   unsanctioned findings. A new `servedVersionRemoval` check in that gate
   makes the swap itself a sanctioned finding and protects `v1` from the same
   fate later.
3. Alpha resources **do not survive the upgrade**. `fathomctl upgrade` plans
   a `replace` action for each promoted CRD: it inventories the alpha
   resources, refuses until `--accept-incompatible-crds` is passed (unless the
   inventory is empty), then deletes and recreates the CRD. Release 0.7.0 is
   an upgrade-path **stop**, because its CLI is the one that carries the
   alpha-aware `fathomctl export`, a read-only verb that renders the live
   alpha checks as `v1` manifests for the adopter to commit and apply.
4. The two runtime definition kinds (`AddonDefinition`,
   `AddonDefinitionBinding`) stay at `v1alpha1` on their own track. The one
   type they share with the check kinds, `DefinitionLeaderEpoch`, is owned by
   `api/v1` and referenced from the alpha package, so the stable schema never
   depends on an alpha package.
5. The three version axes are: the operator release (SemVer, 0.7.0 then
   1.0.0), the CRD version (`v1` for the seven kinds, `v1alpha1` for the
   definition kinds) and the adapter contract (`1.1.0`). The first two meet
   at 1.0; the adapter contract stays an **independent axis** and is not
   bumped for this change. This revises the epic's earlier "all three align
   at 1.0" statement.

## Consequences

- Adopters on 0.6.0 run `fathomctl export`, upgrade with the acceptance flag
  (or delete the alpha CRDs by hand on the plain-manifest path), then apply
  the exported manifests. The release notes carry the procedure in the same
  terms the CLI uses.
- `v1` is frozen from 0.7.0 although the operator is still pre-1.0. "Last
  breaking release before 1.0.0" is read literally; a schema fix found
  between 0.7.0 and 1.0.0 is a `v2` conversation, not a patch to `v1`.
- Report history on an upgraded cluster starts empty; the alpha reports are
  deleted with their CRD and are not exported (they are data, not intent).
- Deleting a check no longer deletes its reports; retention after deletion
  is the subject of ADR 0010.
- The versioning reference's table of served versions and its statement
  about alpha at 1.0 are updated; the canonical standard in
  `skaphos-resources` needs the same row update.

## Alternatives considered

- **Serve both with a conversion webhook, deprecate `v1alpha1`.** Rejected:
  alpha has no support window to honour, the webhook infrastructure does not
  exist, and the scope change that cluster-scoped `ClusterHealth` already
  needed would have forced a CRD recreation anyway.
- **Serve both with conversion strategy `None`.** Objects would be readable
  as `v1` with only the version string rewritten, but `spec.paused` would be
  silently dropped and the required-`spec` and enumerated `checkRef`
  rules would be bypassed for stored objects — exactly the half-converted
  state the spec forbids.
- **Patch `status.storedVersions` after deleting the objects.** Works, but
  hides the deletion from the upgrade plan and needs a status write the CLI
  has no other reason to make.
- **Bump the adapter contract to 2.0.0 to "align" the axes.** Rejected:
  nothing in the contract changes; a forced rebuild of every out-of-tree
  adapter would be ceremony.
