<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Contract: Upgrade-Path Declaration

File: `deploy/helm/fathom-operator/upgrade-path.yaml`, shipped in the chart and
embedded in every `fathomctl`. Append-only: entries for a released version are
never edited or removed.

```yaml
# Stops and sanctioned incompatible CRD changes, one entry per release that has
# either. Maintained in the release PR; see RELEASE.md.
releases:
  - version: 0.6.0
    stop: true
    sanctioned:
      - crd: addonchecks.fathom.skaphos.io
        path: ^.spec.policy
        reason: policy family cap (32) and family-key format rule added
        issue: https://github.com/skaphos/fathom/issues/152
```

| Field | Required | Rule |
|---|---|---|
| `releases[].version` | yes | Semver without `v`; strictly increasing. |
| `releases[].stop` | no (`false`) | An upgrade from below this version to above it must install it first. |
| `releases[].sanctioned[].crd` | yes | CRD `metadata.name`. |
| `releases[].sanctioned[].path` | yes | crdify property path, or `*`. |
| `releases[].sanctioned[].reason`, `.issue` | yes | Shown verbatim in plans and refusals. |

## Path algorithm

Given installed version A and CLI version B (A < B):

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
  released entries are unchanged versus the latest tag, and every
  `SANCTIONED` finding the crd-compat gate reports for the upcoming release has
  a matching `sanctioned` entry for that release.
- `RELEASE.md` adds the upkeep step: when a release sanctions an incompatible
  change, add its entry and decide whether it is a stop (it must be one if
  existing resources need the intermediate operator to migrate).
