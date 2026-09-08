<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Quickstart Validation: fathomctl CLI

This guide validates the implementation. The end-user guide delivered by the
feature is `docs/guides/fathomctl.md`.

## Prerequisites

- Go and the pinned repository tools via `go -C tools tool task …`.
- For e2e: a Docker-compatible engine, Kind, Helm, and Helmfile on `PATH`
  (see `test/e2e/fixtures/README.md`; mise shims may need to be on `PATH`
  for background runs).
- `cosign` for the release-verification step (optional locally).

## 1. Build and generated artifacts

```sh
go -C tools tool task build          # must produce bin/manager and bin/fathomctl
go -C tools tool task fmt
go -C tools tool task manifests generate helm:sync docs:api-ref
go -C tools tool task verify-generated
go -C tools tool task crd-compat
git diff --check
```

Expected:

- `bin/fathomctl version --client` prints a `git describe` style version.
- The regenerated CRD for `NodeCertificateCheck` gains
  `status.lastRunTrigger`; `crd-compat` reports the change as compatible with
  no allowlist entry.
- `docs/reference/api.md` documents the new field; a second generation run
  produces no diff.

## 2. Unit coverage without a cluster

```sh
go -C tools tool task lint
go -C tools tool task test
go -C tools tool task staticcheck
go -C tools tool task vuln
scripts/check-coverage.sh coverage.out
```

The tests must demonstrate, per [contracts/cli-commands.md](contracts/cli-commands.md):

- global flag parsing, `-n`/`-A` exclusivity, `-o` validation, kubeconfig
  and context resolution from a temporary kubeconfig file;
- kind alias resolution for every accepted spelling and rejection of unknown
  kinds;
- `ls` for all five kinds, grouped and single-kind, `-A`, selector, empty
  result, and `json`/`yaml` emitting unmodified objects;
- `describe` for all five kinds including a ClusterHealth with mixed
  children, and not-found;
- `reports` ordering, `--limit`, `--since`, `--report`, no-history, the
  HealthCheck redirect, and the ClusterHealth rejection;
- `run` target resolution (direct, HealthCheck, ClusterHealth fan-out,
  de-duplication, paused exclusion), the >10 confirmation with and without a
  terminal, `--yes`, `--dry-run`, token format, per-target outcomes;
- `run --wait` completion, superseded, and timeout paths with a fast poll
  interval, and exit codes per verdict;
- `version` offline, `--client`, Helm-labelled and kustomize-labelled
  operators, and unavailable-with-reason;
- the controller helper: new value runs, same value does not, no annotation
  preserves the stored value, paused leaves it unconsumed, for each kind;
- node-agent: `FATHOM_RUN_TRIGGER` lands in the report; unset yields empty.

`internal/cli` and every touched package must meet the coverage gate's
default threshold (50%) or better.

## 3. Distribution script

```sh
VERSION=0.0.0-test OUT=/tmp/fathomctl-dist FATHOMCTL_PLATFORMS="$(go env GOOS)/$(go env GOARCH)" scripts/fathomctl-dist.sh
(cd /tmp/fathomctl-dist && sha256sum --check fathomctl_0.0.0-test_checksums.txt)
tar -xzf /tmp/fathomctl-dist/fathomctl_0.0.0-test_*.tar.gz -C /tmp/fathomctl-dist
/tmp/fathomctl-dist/fathomctl_0.0.0-test_*/fathomctl version --client   # prints v0.0.0-test
```

`scripts/fathomctl_dist_gate_test.go` runs the same flow for the host
platform under `go test ./scripts/...` (skipped with `-short`).

## 4. Real cluster (required: controllers and CRD types change)

```sh
go -C tools tool task test-e2e E2E_ADDONS=core
```

`test/e2e/fathomctl_test.go` (core tier) must show:

- `fathomctl version` reports both versions against the Kind operator;
- `fathomctl ls -A` lists the fixtures of every kind with verdicts;
- `fathomctl describe` and `fathomctl reports` succeed on an AddonCheck;
- `fathomctl run --wait` on one AddonCheck, one DNSCheck, and one
  NodeCertificateCheck exits according to the verdict, and afterwards
  `status.lastRunTrigger` equals the printed token;
- re-applying the same token produces no additional run (no new
  `lastRunTime`, no new HealthReport);
- `fathomctl run clusterhealth/<name> --yes --wait` triggers every source
  once.

Run the full stack (`go -C tools tool task test-e2e`) before the PR is
marked ready, because `internal/controller/*`, `api/v1alpha1/*_types.go`,
`internal/nodecert/*`, and `cmd/node-agent/*` are all touched. If a
prerequisite is missing locally, record it in the PR test plan.

## 5. Release verification (after the first tagged release)

```sh
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

All three commands succeed, and the extracted binary reports `v${VERSION}`.

## 6. Documentation gates

```sh
reuse --no-multiprocessing lint
graphify update .
```

Confirm `README.md`, `docs/README.md`, `docs/code-map.md`,
`docs/guides/README.md`, `docs/guides/fathomctl.md`,
`docs/reference/fathomctl.md`, `docs/reference/operator-rbac.md`,
`docs/reference/status-conditions.md`, `docs/guides/node-certificate-checks.md`,
`RELEASE.md`, and `AGENTS.md` describe the shipped behaviour and nothing
more.
