<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Research: fathomctl Lifecycle Verbs

Phase 0 for [spec.md](spec.md). Each entry records the decision, why, and what
was rejected. Verified findings come from a dependency trial in a scratch copy
of the module and a survey of this repository (2026-10-11).

## R1. Helm as a library: `helm.sh/helm/v4` v4.3.0

- **Decision**: drive the chart with the Helm v4 Go library, pinned exactly
  (`helm.sh/helm/v4 v4.3.0`), using `pkg/action` (Install, Upgrade, Uninstall,
  Rollback, List, Get, Status, History, GetValues), the Secrets storage driver
  (`pkg/storage/driver`, name `"Secret"`), `pkg/chart/v2/loader`, and the
  kstatus watcher wait strategy (`kube.StatusWatcherStrategy`).
- **Rationale**: the spec requires a Helm release that `helm ls` sees
  (FR-002). v4 is the current line (v3 is past its bug-fix window); the local
  `helm` is v4.2.4. Both lines write the same `helm.sh/release.v1` Secret, so a
  release is readable by `helm` v3 and v4 CLIs. The trial showed v4.3.0 needs
  exactly the repo's `k8s.io/*` v0.37 line, `kube-openapi` pin and
  `structured-merge-diff/v6 v6.4.2`, and builds against controller-runtime
  v0.25.2 with **no existing requirement changing version**.
- **Alternatives**: Helm v3.22.0 (same dependency profile, but end-of-life
  line and older wait/dry-run API); shelling out to a `helm` binary (an
  undeclared runtime dependency and version skew); embedded kustomize
  manifests (rejected by the spec: a second, drifting install path invisible to
  Helm users).
- **Notes**: actions return the `release.Releaser` interface — type-assert to
  `*v1.Release`. `Install.Labels` lets the release record carry
  `fathom.skaphos.io/installed-by=fathomctl`. `cli-runtime` and `kubectl` arrive
  at v0.37.0 while the rest of k8s is v0.37.1; bump both explicitly to keep the
  lockstep policy.

## R2. Embedding the chart

- **Decision**: a new Go package `deploy/helm` (`chart.go`, package
  `helmchart`) embeds `fathom-operator` with `//go:embed all:fathom-operator`
  and exposes `Load() (*chart.Chart, error)`, which walks the `embed.FS` into
  `[]*archive.BufferedFile` and calls `loader.LoadFiles`. At load, the CLI sets
  the chart's `version` and `appVersion` to its own version — the same override
  `helm package --version/--app-version` applies in the release workflow.
- **Rationale**: `go:embed` cannot reach a parent directory, so the embedding
  package must sit beside the chart. Loading from files keeps the chart in the
  repository as the single source; there is no build-time archive to drift.
  Overriding the version matches the published OCI chart exactly.
- **Alternatives**: embedding a `.tgz` produced at build time (an extra build
  artifact and step); fetching from GHCR (rejected by the clarified spec:
  no `--version`, no run-time fetch).

## R3. Image digests

- **Finding**: the chart has **no digest support** — images render as
  `repo:tag` (`_helpers.tpl:52-53,60-61,80-81`); a digest in `tag` produces an
  invalid reference.
- **Decision**: add `image.digest`, `probeImage.digest` and
  `nodeAgent.image.digest` (schema: `^sha256:[0-9a-f]{64}$`, default empty).
  When set, the helpers render `repo@digest` instead of `repo:tag`, for the
  manager image and for the `--probe-image` / `--node-agent-image` args. The
  release workflow already knows all three digests before it builds the CLI
  archives (`release.yml:127-158` precede `211-219`), so `fathomctl-dist.sh`
  injects them with `-X internal/cli.operatorDigest=…` (and probe, node-agent).
  The CLI passes them as values on install/upgrade.
- **Rationale**: FR-011 and the `RELEASE.md` deploy-by-digest contract. Values
  are the chart's public interface, so `helm` users gain digest pinning too.
- **Development builds**: with no injected digests the CLI installs by tag
  (`v<Version>`), and `status` labels the install "tag-pinned (development
  build)". This is stated in the docs (Principle IX).

## R4. CRD compatibility at run time: crdify as a library

- **Decision**: use `sigs.k8s.io/crdify v0.6.0` (`pkg/runner`, `pkg/config`):
  `runner.New(&config.Config{UnhandledEnforcement: Error}, runner.DefaultRegistry()).Run(live, target)`.
  The repo already pins crdify v0.6.0 as a tool, so the CI gate and the CLI
  apply the same rules. The default registry includes the CRD-level `scope`
  check (always a refusal, FR-022) and `storedversionremoval`, which reads the
  live CRD's `status.storedVersions` — the right input when comparing against
  the cluster.
- **Rationale**: the spec says the upgrade verdict uses the `crd-compat` gate's
  rules. Importing `pkg/runner` links none of crdify's CLI dependencies.
- **Risks**: v0.x API — pin exactly. Fallback, if ever needed: an in-repo
  comparator over `apiextensionsv1` for scope change, removed served/stored
  versions, removed properties, type changes, newly required fields and enum
  narrowing.

## R5. Upgrade-path declaration

- **Finding**: there is no durable per-release record of sanctioned
  incompatible changes; `.crd-compat-allowlist.yaml` is relative to the latest
  tag and is pruned as baselines advance.
- **Decision**: a new append-only file shipped inside the chart,
  `deploy/helm/fathom-operator/upgrade-path.yaml` (listed so `helm package`
  includes it; not a template). It records, for every release, the
  incompatible CRD changes that release sanctions (crd, path, reason, issue)
  and whether the release is a **stop** — a version an upgrade must not skip.
  The CLI reads it from its embedded chart: the path from installed version A
  to its own version B is every stop S with A < S < B, in order; no stops means
  direct. A top-level `floor` (0.5.0) marks the oldest version the data
  covers; anything older is refused rather than compared against changes that
  were never recorded. A gate test fails CI when a `SANCTIONED` finding in the
  crd-compat gate has no matching unreleased entry (version above the latest
  tag), so the record cannot fall behind the allowlist.
- **Rationale**: FR-027/FR-028 need the record in the target release, which is
  always the CLI's own (clarification D). Shipping it in the chart keeps one
  artifact. Append-only means any later CLI knows every earlier stop.
- **Alternatives**: deriving stops from the allowlist (pruned, so history is
  lost); a separate CLI-only file (a second artifact to keep in step).

## R6. Detecting the install and its manager

- **Decision**: (1) list Deployments cluster-wide for the manager
  (`control-plane=controller-manager` + `isFathomOperator`) and read Helm
  releases with the Secrets driver only in those namespaces and the `-n`
  namespace (which catches a release whose Deployment was never created),
  selecting those whose chart name is `fathom-operator` — so `status` needs no
  cluster-wide Secret access; (2) if none,
  find the manager with the CLI's existing lookup
  (`control-plane=controller-manager` + `isFathomOperator`, `version.go`) and
  classify it: `olm.owner*` labels → OLM; `app.kubernetes.io/managed-by:
  kustomize` → kustomize (`dist/install.yaml`); otherwise "unmanaged". Non-Helm
  installs are refused for install/upgrade/uninstall (FR-006) and reported by
  `status`. More than one Fathom release is refused as an unsupported topology.
- **Rationale**: the Helm release Secret is the authoritative signal; labels
  identify the alternatives well enough to name them.

## R7. What uninstall must remove

- **Finding**: besides the chart's objects, the operator creates at run time:
  - in check namespaces, owned by the check: node-agent DaemonSets,
    ServiceAccounts, NetworkPolicies, report Roles/RoleBindings — labelled
    `fathom.skaphos.io/managed-by=fathom` (`agentLabels`);
  - node-report ConfigMaps labelled `managed-by=fathom`;
  - probe Pods labelled `fathom.skaphos.io/managed-by=fathom` — DNSCheck probes
    owned by the check, addon-adapter probes in `kube-system` **with no owner**;
  - a cluster-scoped `ValidatingAdmissionPolicy` and binding,
    `fathom-node-report-authenticity`, **with no owner** — they survive
    `helm uninstall`;
  - the leader-election Lease `2d3dbc4f.skaphos.io` in the operator namespace.
  - No finalizers anywhere.
- **Decision**: after `helm uninstall` completes, delete every object carrying
  `fathom.skaphos.io/managed-by=fathom` of the kinds above across all
  namespaces, the two cluster-scoped admission objects, and the Lease.
  `HealthReport`s and other custom resources are kept (they are data). Deletion
  uses foreground propagation and is bounded by `--timeout`.
- **Rationale**: FR-030 / SC-004 ("zero Fathom workloads remain"). Removing the
  operator first guarantees nothing recreates them.

## R8. Waiting and failure handling

- **Decision**: Helm's `StatusWatcherStrategy` with `Timeout = --timeout`
  (default `5m`); every API call keeps `--request-timeout`. On timeout,
  `upgrade` leaves the release (FR-026) unless `--rollback-on-failure`, which
  sets Helm's `RollbackOnFailure` and is refused before any change when the
  plan applies an incompatible CRD change (FR-029). Recovery text names
  `kubectl -n <ns> describe deploy/<name>`, a longer `--timeout`, and
  `helm rollback <release> -n <ns>`.

## R9. Ordering of an upgrade

1. Detect the install; read the release, its values and the live CRDs.
2. Compute the path (R5) and the CRD plan (R4); refuse on stops, scope
   changes, unsanctioned or unacknowledged incompatible changes.
3. Render the release with Helm client dry-run to validate values against the
   chart schema (FR-007) — nothing changed yet.
4. `--dry-run` stops here and prints the plan.
5. Server-side apply the CRDs (field manager `fathomctl`) and wait for each to
   be `Established`.
6. `helm upgrade` with `SkipCRDs`, `ReuseValues` plus overrides, the injected
   digests, `WaitStrategy`, and `RollbackOnFailure` when requested.

Install follows the same steps, then `helm install` with `SkipCRDs`. When no
Fathom CRDs exist, step 2 has nothing to compare. When CRDs remain (a plain
`uninstall`), step 2 runs in full with A taken from each CRD's
`fathom.skaphos.io/version` annotation; a newer A is `crd-downgrade`, and CRDs
without the annotation are compared without a path (FR-014). Applying CRDs
ourselves on install too keeps one code path and lets a later upgrade own them
with the same field manager.

Step 5 detail (FR-015, FR-020): every applied CRD gets the
`fathom.skaphos.io/version` annotation. Server-side apply only removes fields
the applier owned before, so a CRD whose `managedFields` show another manager
(`helm`, `kubectl-client-side-apply`, …) is first replaced whole with an
Update carrying its `resourceVersion` (dropping the last-applied annotation),
after which SSA with manager `fathomctl` owns every field. After applying, the
live `spec` is compared semantically with the target and any difference fails
the verb before the rollout.

## R10. Export format for `--purge-crds --export`

- **Decision**: one multi-document YAML file, kinds in dependency order
  (`AddonDefinition`, `AddonDefinitionBinding`, then executable checks,
  `HealthCheck`, `ClusterHealth`, `HealthReport`), with server-populated
  metadata removed (`uid`, `resourceVersion`, `managedFields`,
  `creationTimestamp`, `generation`, every `ownerReferences` entry — check
  controllers set controller references on `HealthReport`s, and a reference to
  a purged owner's UID would have the garbage collector delete the restored
  report) and `status` omitted, so
  `kubectl apply -f` re-creates the resources after a reinstall. Written to a
  temp file and renamed, before any deletion (FR-032).

## R11. Namespace deletion check

- **Decision**: with `--delete-namespace`, enumerate namespaced resource types
  through discovery and list each in the namespace; objects that are neither
  Helm-release objects nor `managed-by=fathom`, excluding the namespace's
  automatic objects (`default` ServiceAccount, `kube-root-ca.crt` ConfigMap,
  `istio-ca-root-cert` and similar are reported, not hidden), are listed and
  require confirmation (FR-035).

## R12. Enforcing the CLI import boundary

- **Finding**: the rule that `internal/cli` must not import `internal/app`,
  `internal/adapter` or `internal/controller` is documented (`AGENTS.md`) but
  not enforced.
- **Decision**: add a test that lists the transitive imports of
  `./cmd/fathomctl` (`go list -deps`) and fails on those packages. Adding Helm
  makes an accidental import costlier, so the rule earns a gate.

## R13. End-to-end testing

- **Finding**: the e2e suite deploys the operator with kustomize before the
  specs run; the new verbs refuse non-Helm installs.
- **Decision**: a separate e2e shard, `lifecycle`, on its own kind cluster that
  skips the kustomize deploy (`scripts/e2e-shards.sh`). It builds `fathomctl`
  with an e2e version whose image tags match the images loaded into kind, and
  covers install → status → re-install no-op → upgrade across a CRD schema
  change (an older CRD fixture applied first) → uninstall (no workloads, CRDs
  kept) → `--purge-crds --export --yes`. Refusals (stops, scope,
  unsanctioned, acknowledgement) are covered by unit tests with synthetic
  declarations and CRDs.

## R14. Cost: binary size and dependency surface

- **Finding**: `fathomctl` grows from 42 MB to roughly 100–110 MB unstripped
  (76 MB stripped in the trial) and gains ~55 indirect modules (oras, kubectl,
  kustomize, fluxcd/cli-utils, wazero, SQL drivers). Licences of all 122
  linked modules are permissive; `cyphar/filepath-securejoin` is
  BSD-3-Clause AND MPL-2.0 with only the BSD packages linked.
- **Decision**: accept, recorded in Complexity Tracking. `govulncheck` already
  gates CI and will cover the new surface.
