<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# 8. Drive the Fathom lifecycle from fathomctl through the Helm library

- **Status**: accepted
- **Date**: 2026-10-11
- **Deciders**: Shawn Stratton
- **Decision source**: [spec 013](../../specs/013-fathomctl-lifecycle/spec.md),
  [plan](../../specs/013-fathomctl-lifecycle/plan.md) and
  [research](../../specs/013-fathomctl-lifecycle/research.md)

## Context and Problem Statement

Fathom installs through a Helm chart, `dist/install.yaml` (kustomize) or an
OLM bundle. Helm installs CRDs from `crds/` on first install only and never
upgrades or removes them, so every schema change needs a manual `kubectl apply`
before `helm upgrade`, a CRD scope change needs a delete-and-recreate that
destroys resources, and `helm uninstall` leaves behind the DaemonSets, probe
Pods, per-check RBAC, admission policy and Lease the operator created at run
time. Nothing tells an operator whether an upgrade crosses an incompatible CRD
change, or whether a release must be installed on the way to a later one.

`fathomctl install`, `upgrade`, `uninstall` and `status` close that gap
([#330](https://github.com/skaphos/fathom/issues/330)). The question is how
the CLI produces and tracks the install.

## Decision Drivers

- `helm ls`, `helm history` and `helm rollback` keep working on whatever the
  CLI installs, and an install made with `helm` can be upgraded by the CLI and
  the reverse.
- The CLI only plans transitions whose rules it knows: the target's CRDs, its
  sanctioned incompatible changes and its upgrade stops.
- CRD compatibility is judged by the same rules as the repository's
  `crd-compat` gate, so CI and the CLI never disagree.
- No new runtime dependency on a `helm` binary or on registry access to the
  chart.

## Considered Options

1. Link the Helm v4 library and embed the release's own chart (exact version).
2. Shell out to a `helm` binary on `PATH`.
3. Embed the kustomize manifests (`dist/install.yaml`) and apply them.
4. Fetch the published OCI chart at run time.
5. Accept a `--version` flag, so one CLI installs or upgrades to any release.

## Decision Outcome

Choose option 1.

- `fathomctl` links `helm.sh/helm/v4` and stores releases with Helm's Secret
  driver in the release namespace, labelled
  `fathom.skaphos.io/installed-by=fathomctl`. `helm` reads them like any other
  release.
- The chart is embedded at build time. Each `fathomctl` installs and upgrades
  to exactly its own version; there is no `--version` flag and no run-time
  chart fetch. Release builds carry the release's operator, probe and
  node-agent image digests and pin by digest; the CLI always sets the image
  repository, tag and digest and refuses user overrides of them.
- The CLI applies CRDs itself, before the operator rollout. Each CRD is
  replaced whole rather than merged, so a CRD first created by `helm` or
  `kubectl` reaches exactly the release schema; the CLI reads it back and does
  not start the rollout unless the live spec matches. It is stamped with
  `fathom.skaphos.io/version` and awaited until `Established`. Compatibility
  with the live CRD is judged by crdify with the rules in `.crdify.yaml`.
- `deploy/helm/fathom-operator/upgrade-path.yaml` is an append-only
  declaration shipped in the chart: a `floor` (the oldest version the data
  covers, never changed) and per-release `stop` flags and `sanctioned`
  incompatible changes. `upgrade` refuses a skipped stop, a scope change and
  any unsanctioned incompatible change, and needs `--accept-incompatible-crds`
  for sanctioned ones. `scripts/upgrade_path_gate_test.go` keeps released
  entries unchanged and requires an entry for every sanctioned `crd-compat`
  finding.

### Consequences

- **Positive**: one supported path for install, upgrade and removal that
  applies CRDs first, shows the plan before changing anything, removes what the
  operator created, and works offline because the chart is embedded.
- **Positive**: Helm tooling and existing Helm installs stay first-class; the
  CLI adds checks rather than a second release store.
- **Negative**: `fathomctl` grows about 2.5× (roughly 42 MB to 105 MB
  unstripped) and gains about 55 indirect modules, because it links Helm and
  crdify. `govulncheck` gates the added surface.
- **Negative**: a multi-step upgrade (across a declared stop) needs each
  intermediate release's `fathomctl`, so every release's `fathomctl` must stay
  published. Below the `floor`, the first step is a manual Helm upgrade.
- **Negative**: replacing CRDs whole means any change another tool made to a
  Fathom CRD's spec is discarded on the next `install` or `upgrade`. Labels
  and annotations other tools set are kept, except kubectl's
  `last-applied-configuration`, which is removed.
- **Neutral**: kustomize and OLM installs are refused, not taken over; they
  keep their own tooling.

## Pros and Cons of the Options

Shelling out to `helm` avoids the binary-size cost but adds an undeclared
runtime dependency with its own version skew, and still needs the CLI to do
the CRD handling Helm declines to do. Embedding the kustomize manifests would
produce installs `helm` cannot see and create a second install path that drifts
from the chart. Fetching the chart at run time keeps the binary smaller but
needs registry access, fails in air-gapped clusters, and lets a CLI plan an
upgrade to a release whose CRDs, sanctioned changes and stops it has never
seen. A `--version` flag has the same problem, and would need every CLI to
carry older manifests. Exact-version embedding trades flexibility (another
version means another `fathomctl`) for plans the CLI can fully explain.

## Links

- [Lifecycle CLI contract](../../specs/013-fathomctl-lifecycle/contracts/cli-lifecycle.md)
- [Upgrade-path contract](../../specs/013-fathomctl-lifecycle/contracts/upgrade-path.md)
- [fathomctl reference](../reference/fathomctl.md)
