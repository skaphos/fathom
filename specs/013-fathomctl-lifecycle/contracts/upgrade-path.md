<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: Upgrade-Path Declaration

File: `deploy/helm/fathom-operator/upgrade-path.yaml`, shipped in the chart and
embedded in every `fathomctl`. Append-only: entries for a released version are
never edited or removed.

Illustrative only — the real 0.6.0 entry is `stop: false` (tasks T012); the
0.8.0 entry below is hypothetical.

```yaml
# Stops and sanctioned incompatible CRD changes, one entry per release that has
# either. Maintained in the release PR; see RELEASE.md.
floor: 0.5.0
releases:
  - version: 0.8.0
    stop: true
    sanctioned:
      - crd: dnschecks.fathom.skaphos.io
        path: ^.spec.targets
        reason: example — targets capped at 8
        issue: https://github.com/skaphos/fathom/issues/<n>
```

| Field | Required | Rule |
|---|---|---|
| `floor` | yes | Semver without `v`; the oldest version the recorded data covers (`0.5.0`: the v0.5.x baseline T012 records against). Never changed once released. |
| `releases[].version` | yes | Semver without `v`; strictly increasing. |
| `releases[].stop` | no (`false`) | An upgrade from below this version to above it must install it first. |
| `releases[].sanctioned[].crd` | yes | CRD `metadata.name`. |
| `releases[].sanctioned[].path` | yes | crdify property path, or `*`. |
| `releases[].sanctioned[].reason`, `.issue` | yes | Shown verbatim in plans and refusals. |

## Path algorithm

Given installed version A and CLI version B (A < B):

0. If A < `floor`, refuse `below-floor`: no recorded data covers A. `next:`
   names the documented `helm` upgrade (CRDs applied by hand) to the latest
   patch of the floor's minor, after which this `fathomctl` applies. The
   oldest version B upgrades from directly is the later of `floor` and the
   last stop below B.
1. `stops` = every entry with `stop: true` and A < version < B, ascending.
2. If `stops` is empty the path is `direct`; otherwise the path is
   `A -> stops… -> B` and `upgrade` refuses with `next:` naming the first stop's
   `fathomctl` release.
3. A finding is **sanctioned** when an entry with A < version ≤ B lists the same
   CRD and a matching path (`*` matches all). Sanctioned findings need
   `--accept-incompatible-crds`; others refuse.
4. A `scope` finding is never sanctionable.

## Gates

- `scripts/upgrade_path_gate_test.go`: the file parses, versions increase,
  `floor` is set; entries released at the latest `v*` tag are unchanged versus
  that tag's copy (skipped when the tag predates the file, as v0.5.1 does);
  every `SANCTIONED` finding the crd-compat gate reports has a matching
  `sanctioned` entry among the **unreleased** entries (version greater than
  the latest tag), of which there is at most one; and when
  `.release-please-manifest.json` is ahead of the latest tag (a release PR),
  the unreleased entry, if any, carries exactly the manifest's version. The
  manifest itself only records the last release, so the gate never derives
  the upcoming version from it outside a release PR.
- `RELEASE.md` adds the upkeep step: when a release sanctions an incompatible
  change, add its entry and decide whether it is a stop (it must be one if
  existing resources need the intermediate operator to migrate).
