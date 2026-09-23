# Release Process

Fathom releases via stock Release Please on `main`
(`.github/workflows/release-please.yml`) and a tag-triggered publish workflow
(`.github/workflows/release.yml`) that builds and pushes the operator, probe,
node-agent, bundle, and catalog images plus the Helm chart to GHCR.

## Prerequisites

- Push access to `main`.
- The `skaphos-release-bot` GitHub App must be installed with `RELEASE_BOT_APP_ID`
  (repo/org variable) and `RELEASE_BOT_PRIVATE_KEY` (secret) configured. The
  Release Please workflow mints a short-lived app token from these to open the
  release PR and push the release tag. This is not optional: a tag pushed by the
  default `GITHUB_TOKEN` does **not** trigger other workflows, so the app-token
  tag push is what cascades into `release.yml`.
- CI is green on `main`.
- Images publish to `ghcr.io/skaphos/fathom-operator`,
  `ghcr.io/skaphos/fathom-probe`, `ghcr.io/skaphos/fathom-node-agent`,
  `ghcr.io/skaphos/fathom-operator-bundle`, and
  `ghcr.io/skaphos/fathom-operator-catalog`. The release workflow authenticates
  with the built-in `GITHUB_TOKEN` and requires `packages: write`. The
  publishing actor must have permission to push to the `skaphos` GHCR namespace.

## 1. Land Releasable Commits on `main`

Release Please maintains the release PR from commits merged to `main`.

- Use Conventional Commits so Release Please can compute the next version.
- `feat:` -> minor
- `fix:` / `perf:` -> patch
- `docs:`, `test:`, `ci:`, `chore:`, `refactor:` -> no bump by default
- Squash-merged PRs must also have a Conventional Commit title.

## 2. Run Local Release Checks

- `go -C tools tool task ci`

## 3. Review and Merge the Release PR

When Release Please detects releasable commits, it opens or updates a release
PR. Review the changelog and version bump, then merge when correct. Merging
creates the `vX.Y.Z` tag.

## 4. Tag-Triggered Publish

Tag creation triggers `.github/workflows/release.yml`, which:

1. Builds and pushes the operator image (`fathom-operator:vX.Y.Z`) to GHCR as a
   multi-arch manifest (`linux/amd64`, `linux/arm64`) via `docker buildx`.
2. Builds and pushes the probe image (`fathom-probe:vX.Y.Z`) to GHCR as a
   multi-arch manifest (`linux/amd64`, `linux/arm64`) via `docker buildx`.
3. Builds and pushes the node-agent image (`fathom-node-agent:vX.Y.Z`) to GHCR
   as a multi-arch manifest (`linux/amd64`, `linux/arm64`) via `docker buildx`.
4. Generates `dist/install.yaml` from `config/default`.
5. Builds and pushes the OLM bundle image.
6. Builds and pushes the OLM catalog image (via `opm`).
7. Packages and pushes the Helm chart to `oci://ghcr.io/skaphos/charts`.
8. Cross-compiles the `fathomctl` CLI for `linux`, `darwin`, and `windows` on
   `amd64` and `arm64` (`scripts/fathomctl-dist.sh` via the `fathomctl-dist`
   task) into one archive per platform plus
   `fathomctl_X.Y.Z_checksums.txt`. The CLI is a client tool and ships as
   archives only; it is never published as a container image.
9. Signs every published image and the chart with keyless cosign signatures and
   records SLSA build provenance (`actions/attest-build-provenance`) for the
   operator image, probe image, node-agent image, OLM bundle, OLM catalog, and
   Helm chart. Signs the `fathomctl` checksums file with `cosign sign-blob`
   (bundle `fathomctl_X.Y.Z_checksums.txt.sigstore.json`) and records build
   provenance for every `fathomctl` archive.
10. Generates SPDX SBOMs for the operator, probe, and node-agent images.
11. Creates a GitHub Release with `dist/install.yaml`, the SBOMs, and the
   `fathomctl` archives, checksums, and signature bundle attached, plus
   auto-generated release notes.

## 5. Verify the Release

- Confirm all five images exist under `ghcr.io/skaphos` (operator, probe,
  node-agent, bundle, catalog).
- Confirm `ghcr.io/skaphos/fathom-operator:vX.Y.Z` advertises both `linux/amd64`
  and `linux/arm64` (`docker buildx imagetools inspect …`). All three runtime
  images must carry both platforms — the operator Deployment, the probe Pods,
  and the node-agent DaemonSet all have to schedule on arm64 nodes.
- Confirm `ghcr.io/skaphos/fathom-probe:vX.Y.Z` advertises both `linux/amd64`
  and `linux/arm64` (`docker buildx imagetools inspect …`).
- Confirm `ghcr.io/skaphos/fathom-node-agent:vX.Y.Z` advertises both
  `linux/amd64` and `linux/arm64` (`docker buildx imagetools inspect …`).
- Confirm the GitHub Release exists with `install.yaml` attached.
- Confirm the six `fathomctl_X.Y.Z_<os>_<arch>` archives,
  `fathomctl_X.Y.Z_checksums.txt`, and its `.sigstore.json` bundle are
  attached, and that a downloaded binary reports `vX.Y.Z` from
  `fathomctl version --client` (see
  [Verify a fathomctl download](#verify-a-fathomctl-download)).
- Optionally install the bundle into a cluster via OLM:

  ```bash
  operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bundle:vX.Y.Z
  ```

## Supply-Chain Verification

Every published artifact — the operator image, probe image, node-agent image,
OLM bundle, OLM catalog, and the Helm chart — is signed with [cosign] keyless
signatures and carries SLSA build provenance generated by
`actions/attest-build-provenance`. SPDX SBOMs for the operator, probe, and
node-agent images are attached to the GitHub
Release. There are no long-lived signing keys: the signing identity is the
release workflow's GitHub OIDC token, bound to this repository and the
`Release` workflow (`.github/workflows/release.yml`).

Pick the artifact to check. Prefer the immutable digest in production; the tag
is shown here for brevity:

```bash
IMAGE=ghcr.io/skaphos/fathom-operator:vX.Y.Z
```

### Verify the cosign signature

```bash
cosign verify \
  --certificate-identity-regexp '^https://github.com/skaphos/fathom/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  "${IMAGE}"
```

The same command verifies `fathom-probe`, `fathom-node-agent`,
`fathom-operator-bundle`, `fathom-operator-catalog`, and the chart
(`ghcr.io/skaphos/charts/fathom-operator:X.Y.Z`, note: no leading `v`).

### Verify build provenance

Provenance is pushed to the registry alongside each artifact, so the GitHub CLI
verifies it directly against the registry:

```bash
gh attestation verify "oci://${IMAGE}" --owner skaphos
```

Or with cosign, matching the SLSA v1 predicate type:

```bash
cosign verify-attestation \
  --type 'https://slsa.dev/provenance/v1' \
  --certificate-identity-regexp '^https://github.com/skaphos/fathom/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  "${IMAGE}"
```

### Verify a fathomctl download

The CLI archives are covered by one keyless signature over the checksums file
and by build provenance on each archive. Verify the signature, then the
checksum of the archive you downloaded, then (optionally) the provenance:

```bash
VERSION=X.Y.Z
gh release download "v${VERSION}" --repo skaphos/fathom --pattern 'fathomctl_*'

cosign verify-blob \
  --bundle "fathomctl_${VERSION}_checksums.txt.sigstore.json" \
  --certificate-identity-regexp '^https://github.com/skaphos/fathom/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  "fathomctl_${VERSION}_checksums.txt"

sha256sum --check --ignore-missing "fathomctl_${VERSION}_checksums.txt"

gh attestation verify "fathomctl_${VERSION}_linux_amd64.tar.gz" --repo skaphos/fathom
```

Then extract and confirm the binary reports the release version:

```bash
tar -xzf "fathomctl_${VERSION}_linux_amd64.tar.gz"
"fathomctl_${VERSION}_linux_amd64/fathomctl" version --client   # prints vX.Y.Z
```

### Inspect the SBOM

Download the SPDX SBOMs attached to the GitHub Release:

```bash
gh release download "vX.Y.Z" --repo skaphos/fathom --pattern '*.spdx.json'
```

You can also regenerate an SBOM from the published image with [syft] and
compare:

```bash
syft "${IMAGE}" -o spdx-json
```

[cosign]: https://github.com/sigstore/cosign
[syft]: https://github.com/anchore/syft

## Adapter ratio-key migration

The Go adapter contract is now 1.1.0. Contract 1.1 reserves `warnRatio` and
`failRatio` for Fathom's engine-level family aggregation. Version 1.0 adapters
remain loadable and continue to run policies that do not contain those keys.
When either key appears in a policy selected for a 1.0 adapter, reconciliation
sets `Accepted=False / InvalidPolicy` and skips `Run`; this also applies to a
disabled family and to an adapter that advertises the key.

Before rebuilding a 1.0 adapter against 1.1, audit its threshold reads. If it
used either reserved name as a private knob, rename that knob and migrate the
affected `AddonCheck` objects first. Confirm the rebuilt adapter neither reads
nor advertises the reserved names, then report contract 1.1.0 and add engine
ratio thresholds as desired. To roll back during migration, remove the reserved
keys: the 1.0 adapter and all its other private thresholds remain compatible.

## Node-agent RBAC migration

Upgrading from a release that used the shared `fathom-node-agent-role`
ClusterRole moves each `NodeCertificateCheck` and `NodeHealthCheck` to its
namespaced `<service-account>-report-access` Role and RoleBinding. The operator
clears Subjects on owner-controlled legacy bindings while leaving their
immutable RoleRef and the old ClusterRole untouched. After all checks have
migrated, verify that no external RoleBinding still relies on that ClusterRole,
or ClusterRoleBinding still relies on it, then remove it manually if desired.
Do not remove it before that verification. A migration error triggers access
revocation and agent teardown. If cleanup also fails, the affected check
reports `Ready=False / AgentRevocationFailed`; resolve migration and cleanup
errors before removing any legacy RBAC.

## Node metrics security migration

This release changes the supported scrape boundary for node detail metrics.
Node-agent `/metrics` now returns 404; `/healthz`, listener flags, container
ports, and host-network port allocation remain compatible. Certificate and
node-health series are projected from accepted reports through the operator's
existing HTTPS endpoint and `get /metrics` authorization.

Upgrade the operator and node-agent images together. If
`--node-agent-image`, `FATHOM_NODE_AGENT_IMAGE`, the config-file
`node_agent_image`, or Helm `nodeAgent.image.*` values override the release
default, update that override explicitly. Then wait for the operator and every
managed agent DaemonSet to finish rolling out; the vulnerability remains while
any old agent pod is running:

```bash
kubectl -n fathom-system rollout status deployment/fathom-controller-manager
set -o pipefail
kubectl get daemonsets -A \
  -l 'fathom.skaphos.io/source-kind in (NodeCertificateCheck,NodeHealthCheck)' \
  -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}' |
while read -r namespace name; do
  [ -n "$namespace" ] && [ -n "$name" ] || continue
  kubectl -n "$namespace" rollout status "daemonset/$name" --timeout=10m || exit 1
done
```

Remove node-agent PodMonitors and scrape annotations, and use the shipped
operator ServiceMonitor with a ServiceAccount bound to the metrics-reader role.
The metric names remain stable, but labels change:

- certificate expiry changes from `node,path` to `namespace,check,node` and is
  the minimum known expiry for that check and node;
- node-health families add `namespace,check` while retaining their existing
  item labels.

Prometheus may rename the endpoint's `namespace` label to
`exported_namespace` when its target labels collide and `honor_labels` is
false. Preserve the actual check namespace in aggregations; never group only
by the operator Service's namespace. See
[Monitoring and alerting](docs/guides/monitoring.md#node-certificate-metrics)
for migrated query examples and replica-aware aggregation.

Rollback restores the old unauthenticated agent endpoint and label contract.
Treat rollback as reopening the disclosure until every rolled-back agent is
again removed or replaced.

## Rollback / Fix Forward

- If the release workflow fails after the tag lands, fix the workflow and
  re-run. Images are idempotent by tag; rerunning is safe.
- If Release Please generated the wrong version or notes, fix the underlying
  commits and let it regenerate the next release PR.
- Manual tag creation should be reserved for emergency recovery only.

## Default Deployment Topology

`config/default` is the source of `dist/install.yaml` and the OLM bundle. By
default it renders:

- The operator Namespace, RBAC, CRDs, and Deployment.
- A `controller-manager-metrics-service` exposing `:8443` (HTTPS).
- The Deployment with `--metrics-bind-address=:8443` injected by
  `manager_metrics_patch.yaml`.
- Per-addon least-privilege RBAC (`config/rbac/addons/`): one ServiceAccount +
  read-only ClusterRole + binding per built-in adapter, plus a namespaced
  `impersonate` Role for the operator. These are **generated** from the adapters
  (`task gen:addon-rbac`, gated by `verify-generated`); the operator impersonates
  each addon ServiceAccount at run time so it reads under least privilege rather
  than an aggregate role. This changes adapter reads from cached to live
  (per-check) API calls — impersonation cannot use the manager cache. The full
  matrix is `docs/reference/rbac.md`.

The operator-side default probe image is `ghcr.io/skaphos/fathom-probe:vX.Y.Z`
(same `vX.Y.Z` as the operator) and is launched on-demand by probe-using
adapters such as the CoreDNS `dns_resolution` family. Override per-AddonCheck
via the `probeImage` threshold or operator-wide via `--probe-image` /
`FATHOM_PROBE_IMAGE` / `probe_image` config.

It does **not** render a Prometheus `ServiceMonitor` by default. To opt in,
uncomment the `components` block in `config/default/kustomization.yaml`:

```yaml
components:
  - ../components/prometheus
```

The component lives at `config/components/prometheus/`. Its
`monitor_tls_patch.yaml` switches the ServiceMonitor from `insecureSkipVerify:
true` to a cert-manager-backed TLS configuration; enable it from the
component's `kustomization.yaml` once cert-manager and the
`cert_metrics_manager_patch` are wired up in the overlay.

## Image Pinning and Deploy-by-Digest Contract

Container builds are hardened for reproducibility and supply-chain integrity:

- **Base images are pinned by digest** (SKA-295). `Dockerfile` pins the
  `golang` builder and the `gcr.io/distroless/static:nonroot` runtime;
  `Dockerfile.probe` and `Dockerfile.node-agent` pin the `golang` builder (their
  runtime is `scratch`). The readable tag is retained alongside the digest
  (`golang:1.27.1@sha256:...`).
  Refresh the digests with `go -C tools tool task images:refresh`, which
  re-resolves each multi-arch index digest (via `crane` or
  `docker buildx imagetools`) and rewrites the pins in place. Run it ad hoc or
  on a schedule and review the diff in a PR.
- **BuildKit cache mounts** (SKA-305) keep the Go module and build caches out of
  image layers. All three Dockerfiles pin the Dockerfile frontend at
  `docker/dockerfile:1.27.0`, so builds require BuildKit (the default for modern
  `docker build` / `buildx`).

**Releases MUST deploy the manager by digest, never by a mutable tag.** The
`config/manager/kustomization.yaml` `images:` transformer maps the placeholder
name `controller` to `ghcr.io/skaphos/fathom-operator`. At release/deploy time,
inject the published digest via the `IMG` variable so kustomize pins it:

```bash
IMG=ghcr.io/skaphos/fathom-operator@sha256:<digest> \
  go -C tools tool task build-installer   # or deploy / bundle
```

This makes `dist/install.yaml` and the OLM bundle reference an immutable,
content-addressed manager image. The `newTag: latest` default in the
transformer is for local development only.

## Probe / Node-Agent Version Lockstep (Automated)

The probe and node-agent image tags the operator falls back to are compiled
into the binary, and the Helm chart renders every image tag from its
`appVersion`. All of these must equal the operator's own release version, or a
plain `kustomize`/`helm install` from a checkout launches stale or unpublished
workloads (this human-gated contract failed for 0.3.0 and 0.3.1, SKA-579).

This is now automated — you no longer hand-edit version tags at release time:

- **release-please bumps every site in the release PR.** Each site carries an
  `x-release-please-version` annotation and is listed under `extra-files` in
  `release-please-config.json`:
  - `DefaultProbeImage` / `DefaultNodeAgentImage` in `internal/app/options.go`
  - `fallbackProbeImage` in `internal/adapter/coredns/adapter.go`
  - `version` and `appVersion` in `deploy/helm/fathom-operator/Chart.yaml`
  - `E2E_PROBE_IMG` / `E2E_NODE_AGENT_IMG` in `Taskfile.yml`
  - pinned `probeImage` sample override in
    `config/samples/fathom_v1alpha1_addoncheck_coredns.yaml` (the sample sets
    `policy.dns_resolution.thresholds.probeImage`, which overrides the operator
    default; the pin must match the kind-loaded e2e probe tag)
- **CI enforces lockstep.** The `version-lockstep` job (and
  `go -C tools tool task verify-version-lockstep`) runs
  `scripts/check-version-lockstep.sh`, which fails the build if any of those
  sites drifts from the version in `.release-please-manifest.json`. A guard test
  (`scripts/version_lockstep_gate_test.go`) also asserts the gate stays in sync
  and actually detects drift.
- **Do not hand-bump the Helm chart version on ordinary PRs.** Chart content
  (templates, values, synced CRDs) can change mid-cycle; `version` /
  `appVersion` stay at the last released value until the release PR. Chart
  testing disables `check-version-increment` (`.github/ct.yaml`) so `ct lint`
  does not fight the lockstep gate.

So the flow is: land Conventional Commits, review the release PR (which already
carries the bumped tags), merge. If you ever hand-bump a tag, the
`version-lockstep` gate catches a missed sibling before merge.

**Follow-ups (not in scope here):** the compiled defaults are pinned by tag, not
digest — digest pinning is impractical for the compiled default because the
probe/node-agent digest does not exist until the release build runs (after the
release PR is cut); if desired, pin at deploy time via `--probe-image` /
`--node-agent-image` with a `@sha256:` reference. The node-agent image is not
yet published at all (SKA-531). User-facing docs and samples that mention the
default image tag are corrected per release but are not yet part of the gate.

## Notes

- The release flow is aligned to `Taskfile.yml` targets (`docker-buildx-push`,
  `probe-docker-buildx-push`, `node-agent-docker-buildx-push`,
  `build-installer`, `bundle`, `bundle-build`, `bundle-push`, `catalog-build`,
  `catalog-push`).
- No Homebrew cask publishing — Fathom is delivered as container/bundle images.

The fathomctl archives also stamp the source Git revision for runtime-definition
collision preflight. `definition collisions` reports both version and build and
refuses verification when either is unknown; use the target release binary.

## Runtime definition operations (qualification preview)

Runtime loading remains off by default. Helm's `runtimeLoading.enabled: true`
emits the operator's `--runtime-loading-enabled` flag; false emits no runtime
flag so environment/config-file opt-in remains available. Enabling it
requires leader election even for one replica and an explicit operator namespace
containing the configured leader Lease. Keep this feature out of a release until
the [full real-cluster installation and rollback qualification](specs/012-addon-definition-runtime/tasks.md)
passes. The separate [#256 ratio-contract decision](https://github.com/skaphos/fathom/issues/256)
is still a release dependency; this feature does not resolve it.

Before upgrading, run `fathomctl definition collisions` from the **target**
release. Its printed version/build and bundled compiled inventory are the
comparison authority. A collision exits 1; missing build metadata, inventory
or cluster verification exits 2. Review and remove conflicting runtime names
before upgrading. Follow the [runtime definition guide](docs/guides/addon-definitions.md)
to render offline grants, install the definition and dedicated reader account,
then create a disabled binding with their live UIDs. Recreating either resource
requires a new reviewed binding; status alone never delegates authority.

For rollback, disable each binding and, while the runtime loader and leader
election still run, independently verify a current leader's matching-generation
drain with `fathomctl definition drain`. Revoke its reader and impersonation
grants, then export complete runtime state before turning runtime loading off or
downgrading. Store the exported files outside the cluster before replacing the
binary. Fathom v0.5.1 lacks the newer `status.lastSuccessfulEvaluation`,
freshness and `latestAttempt` fields; an older binary may replace or remove those
status fields. Transition-only reports may also omit the newest same-verdict
revision/evidence. Back up full AddonCheck objects and status as both YAML and
JSON, not just HealthReports, definitions and bindings. After an older-binary
rollback, stop all old pods and run the new binary with runtime loading off,
bindings disabled and reader/impersonation grants revoked. Restore each affected
AddonCheck's exported raw status through the status subresource only after
checking its unchanged UID and generation; use a JSON Patch resourceVersion test
to reject concurrent writes. Verify the original evidence and `observedAt`, and
leave immutable HealthReports untouched. The exact backup and restore commands
are in the [runtime definition guide](docs/guides/addon-definitions.md#changes-drain-and-rollback).
The bounded source-built v0.5.1 downgrade, guarded restore and fresh same-verdict
re-enable trial passed; see the [operations qualification record](specs/012-addon-definition-runtime/operations-qualification.md).
This does not qualify a released chart upgrade or historical new-builtin behavior.
Give the CLI
principal `get` on the exact operator-namespace Lease and binding. Drain is an
observation with a leadership race after the final read, not atomic RBAC
revocation. Retain definitions, bindings, CRDs and HealthReport history; stale or
unavailable evidence keeps its original source and time. A completed all-Skipped
run records fresh Skipped evidence with `NoChecksEvaluated` coverage. Before
re-enabling, repeat target-release collision preflight, UID and grant review,
Lease configuration checks, and a fresh run. The
[clarification supplement](specs/012-addon-definition-runtime/contracts/decision-supplement.md)
defines the election, Skipped and target-inventory decisions.
Same-UID definition edits retain delegation and require Git review; a recreated
definition or reader account needs a fresh binding. The new resources' alpha
schema track is separate from the
[`v1alpha1` to `v1` freeze issue #149](https://github.com/skaphos/fathom/issues/149).
