# Release Process

Fathom releases via Release Please on `main` and a tag-triggered publish
workflow that pushes operator, bundle, and catalog images to GHCR.

## Prerequisites

- Push access to `main`.
- Optional: `RELEASE_PLEASE_TOKEN` configured as a GitHub Actions secret with
  permission to open PRs and create tags on this repository. Without it the
  workflow falls back to `github.token`, which maintains the release PR but
  may not reliably trigger the downstream tag-push workflow.
- CI is green on `main`.
- Images publish to `ghcr.io/skaphos/fathom-operator`,
  `ghcr.io/skaphos/fathom-operator-bundle`, and
  `ghcr.io/skaphos/fathom-operator-catalog`. The release workflow authenticates
  with the built-in `GITHUB_TOKEN` and requires `packages: write`. The publishing
  actor must have permission to push to the `skaphos` GHCR namespace.

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

1. Builds and pushes the operator image (`fathom-operator:vX.Y.Z`) to GHCR.
2. Generates `dist/install.yaml` from `config/default`.
3. Builds and pushes the OLM bundle image.
4. Builds and pushes the OLM catalog image (via `opm`).
5. Creates a GitHub Release with `dist/install.yaml` attached and auto-generated
   release notes.

## 5. Verify the Release

- Confirm all three images exist under `ghcr.io/skaphos`.
- Confirm the GitHub Release exists with `install.yaml` attached.
- Optionally install the bundle into a cluster via OLM:

  ```bash
  operator-sdk run bundle ghcr.io/skaphos/fathom-operator-bundle:vX.Y.Z
  ```

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

## Notes

- The release flow is aligned to `Taskfile.yml` targets (`docker-build`,
  `docker-push`, `build-installer`, `bundle`, `bundle-build`, `bundle-push`,
  `catalog-build`, `catalog-push`).
- No Homebrew cask publishing — Fathom is delivered as container/bundle images.
