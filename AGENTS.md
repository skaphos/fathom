<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Repository Guidelines

This file is the authoritative briefing for AI coding agents and human
contributors working on Fathom. It is also exposed as `CLAUDE.md`, which is a
symlink to `AGENTS.md`.

## What Fathom Is

Fathom is a Kubernetes operator (`skaphos.io` domain) that reconciles
`HealthCheck` and `ClusterHealth` custom resources, and persists
`HealthReport` history. It is scaffolded with `kubebuilder` (`go.kubebuilder.io/v4`)
and packaged via `operator-sdk` for OLM bundle distribution. See `PROJECT` for
the canonical resource list.

## Project Structure & Module Organization

- `api/v1alpha1/`: CRD type definitions. `zz_generated.deepcopy.go` is produced by `controller-gen`; never hand-edit it.
- `cmd/main.go`: thin entrypoint. Constructs the cobra root command via `internal/app` and runs it.
- `cmd/probe/`: the tiny in-cluster network probe binary (`Dockerfile.probe`).
- `cmd/node-agent/`: the node-agent DaemonSet binary that scans on-disk certificates (`Dockerfile.node-agent`); its own dedicated image, never the operator or probe image.
- `internal/app/`: cobra/viper plumbing, options parsing, manager construction, controller registration. The unit-testable seam.
- `internal/controller/`: reconciler implementations (`AddonCheckReconciler`, `HealthCheckReconciler`, `ClusterHealthReconciler`, `NodeCertificateCheckReconciler`).
- `internal/nodecert/`: the on-disk X.509 scan engine and the node-agent↔operator wire contract (no Kubernetes-client deps, so the agent binary stays small).
- `config/`: kustomize overlays, RBAC, CRDs, OLM scaffolding (`config/crd`, `config/manager`, `config/rbac`, `config/manifests`, …).
- `test/e2e/`: Ginkgo suites that run against a Kind cluster.
- `test/utils/`: shared helpers used by the e2e suite.
- `tools/`: pinned tooling launched via `go -C tools tool task ...` (Task, controller-gen, kustomize, setup-envtest, golangci-lint, staticcheck, govulncheck, goimports).
- `hack/boilerplate.go.txt`: SPDX/license header inserted by `controller-gen` into generated Go files.

## Build, Test, and Development Commands

All workflows are wrapped in tasks; never invoke `controller-gen` / `kustomize`
/ `setup-envtest` directly except via tasks so versions stay pinned.

- `go -C tools tool task --list`: list available tasks.
- `go -C tools tool task fmt`: `goimports -w .` + `go fmt ./...`.
- `go -C tools tool task lint`: regenerates manifests + runs `golangci-lint run ./...`.
- `go -C tools tool task vet`: `go vet ./...`.
- `go -C tools tool task test`: unit tests with envtest, writes `coverage.out`. Excludes `./test/e2e`.
- `go -C tools tool task test-e2e`: spins up a Kind cluster (or reuses one), runs `./test/e2e`, tears it down. Requires `kind` and `docker` on `PATH`. The addon stack is tiered: `E2E_ADDONS=<addon>` installs just the core tier plus that addon and runs only its specs (see `test/e2e/fixtures/README.md`).
- `go -C tools tool task staticcheck`: honnef.co/go/tools `staticcheck ./...`.
- `go -C tools tool task vuln`: `govulncheck ./...`.
- `go -C tools tool task ci`: full local CI (lint, test, staticcheck, vuln, build).
- `go -C tools tool task crd-compat`: diff `config/crd/bases` against the latest release tag (crdify) and fail on unsanctioned incompatible schema changes; overrides live in `.crd-compat-allowlist.yaml`.
- `go -C tools tool task build`: `go build -o bin/manager cmd/main.go`.
- `go -C tools tool task run`: run the manager from your host against the current kubeconfig context.
- `go -C tools tool task install` / `uninstall`: apply or remove CRDs in the current cluster.
- `go -C tools tool task deploy` / `undeploy`: render and apply the full operator manifests.
- `go -C tools tool task bundle` / `bundle-build` / `bundle-push`: OLM bundle generation and image publishing (requires `operator-sdk`).
- `go -C tools tool task catalog-build` / `catalog-push`: OLM catalog (requires `opm`).

## Configuration Model

Fathom uses **cobra + viper** for runtime configuration. Precedence
(highest → lowest): **command-line flag → environment variable → config file → built-in default**.

- Config file: `--config /path/to/file` (default `/etc/fathom/config.yaml`, typically a mounted ConfigMap). Missing default-path files are ignored; an explicit `--config` whose target is missing is a hard error.
- Environment variables: `FATHOM_*` with dots in the viper key replaced by `_`. Example: `metrics.bind_address` → `FATHOM_METRICS_BIND_ADDRESS`.
- Flags retain their kubebuilder names (`--metrics-bind-address`, `--leader-elect`, …) for backwards compatibility with existing deployment manifests.
- Add new options by extending `Options` in `internal/app/options.go` and the corresponding row in the `bindings()` table; the flag, viper key, env var, and config-file key stay in sync automatically.

## Coding Style & Naming Conventions

- Go version: `go.mod` is the source of truth.
- Formatting: `gofmt` and `goimports` are enforced via `golangci-lint`.
- Naming: standard Go (`PascalCase` exported, `camelCase` unexported). CRD types follow kubebuilder conventions; reconcilers are named `<Kind>Reconciler`.
- File headers: every Go source file (and most non-generated text files) carries the SPDX header at `hack/boilerplate.go.txt` (or its REUSE equivalent). `reuse lint` is enforced in CI.
- Generated files (`zz_generated*.go`, OLM bundle metadata, manifests under `config/`) are produced by tooling — re-run the appropriate task instead of editing them.

## Engineering Guardrails

- Keep cognitive load low: prefer small functions, clear names, early returns, simple control flow over clever abstractions.
- Comment intent (invariants, edge cases, non-obvious tradeoffs), not mechanics.
- Reconciler logic should be idempotent and bounded; honor `spec.timeout` on health checks and never run unbounded work in a `Reconcile` loop.
- Do not introduce cluster-wide RBAC beyond what the operator strictly needs; new permissions must show up under `config/rbac/` via `+kubebuilder:rbac` markers.
- Keep the `ClusterHealth` external contract stable. It is derived only from `HealthCheck.status` — never from `HealthReport` history.

## Testing Guidelines

- Frameworks: Ginkgo v2 + Gomega for envtest and e2e suites; `testing` (stdlib) for plain unit tests in `internal/app`.
- Unit tests live next to source as `*_test.go`. Suite bootstraps follow `*_suite_test.go`.
- Test-package convention: default to an **external** `_test` package (`package foo_test`) so tests exercise the public API — this is the rule for `pkg/*` and library-style packages (e.g. `pkg/adapter`, `internal/adapter/registry`, `internal/adapter/crdutil`). Use an **in-package** white-box test (`package foo`) only when the test genuinely needs unexported access. The two may coexist in one directory: in `pkg/adapter`, the public contract is tested from `adapter_test` (`adapter_test.go`) while the unexported semver parser is tested in-package (`version_test.go`). Don't add a white-box test just to skip the package qualifier.
- New behavior must ship with direct test coverage. Bug fixes should add a regression test that fails before the fix.
- Prefer table-driven tests for branching logic. Mock injection seams (`managerFactory` in `internal/app/run.go`, the `Setupper` interface for controllers) exist precisely so unit tests don't need envtest.
- envtest binaries are managed by `setup-envtest`; CI installs them automatically. Locally the `test` task bootstraps them in `bin/k8s/`.
- The Kubernetes version used for testing (envtest + e2e) must be kept in lockstep with the `k8s.io/*` module versions declared in `go.mod` (and the `sigs.k8s.io/controller-runtime` version). When bumping the client libraries, update these together:
  - `ENVTEST_K8S_VERSION` in `Taskfile.yml`
  - `kindest/node` image in `test/e2e/fixtures/kind-cluster.yaml`
  - `kubernetesVersion` in `docs/.crd-ref-docs.yaml`
  See the comments in `kind-cluster.yaml` and `test/e2e/fixtures/README.md` for the explicit "bump in lockstep" policy. Mismatches can hide API compatibility issues.
- Coverage gate: `scripts/check-coverage.sh` aggregates `coverage.out` per package against a configurable per-package minimum (`COVERAGE_MIN_DEFAULT`). The gate runs on Linux in CI. Ratchet thresholds upward as coverage improves; do not lower them to make a PR pass.

### Run e2e after major changes

Run `go -C tools tool task test-e2e` against a kind cluster after any change that touches operator runtime behavior. envtest auto-registers schemes and CRDs that real clusters do not — bugs that only surface at reconcile time (SKA-422 apiextensions scheme registration; SKA-423 ESO v1beta1 hardcoding) are caught only by e2e.

For a change scoped to a single addon's adapter, the scoped run
`task test-e2e E2E_ADDONS=<addon>` (core tier + that addon) is sufficient;
shared-surface changes (`internal/app`, `pkg/adapter`, the declarative engine,
controllers, CRD types) still warrant the full stack. CI mirrors this: the
`kind e2e` workflow path-filters the diff and shards addons across parallel
kind clusters (`scripts/e2e-shards.sh`; see `test/e2e/fixtures/README.md`).

Changes that **require** an e2e run before the PR is considered ready:

- `internal/app/run.go` — scheme registration, manager construction.
- `internal/adapter/*/adapter.go` — adapter `Run` and per-family check logic.
- `internal/controller/*` — reconciler bodies and watch wiring.
- `pkg/adapter/*` — adapter contract surface.
- `api/v1alpha1/*_types.go` — CRD schema changes.
- `internal/probe/*` — probe pod lifecycle / pod-builder changes.
- `internal/nodecert/*` and `cmd/node-agent/*` — the on-disk cert scan engine and node-agent DaemonSet binary; reads real host certs over hostPath, which only e2e exercises.
- `test/e2e/fixtures/*` — addon stack itself; the fixture change is the test.
- Bumping `ENVTEST_K8S_VERSION`, the `kindest/node` image, or the `kubernetesVersion` used for CRD docs — these must stay aligned with `go.mod` k8s.io modules (see Testing Guidelines).

Changes that **do not** require e2e: test-only edits, comments, docs, CI/tooling, `Taskfile.yml` edits that don't affect runtime, generated files.

If the local toolchain is missing required components (`kind`, `helm`, `helmfile`, a running Docker daemon), say so in the PR's test plan rather than silently skipping. CI integration for this flow is tracked in SKA-417; until that lands, e2e is a contributor-local responsibility.

## Commit & Pull Request Guidelines

- All changes land via pull request. Never push directly to `main`.
- Branch naming uses a change-type prefix:
  - `feature/<short-description>` — new functionality
  - `fix/<short-description>` — bug fixes
  - `chore/<short-description>` — maintenance, deps, tooling
  - `docs/<short-description>` — documentation only
  - `ci/<short-description>` — CI/CD pipeline changes
  - `refactor/<short-description>` — internal restructuring without behaviour change
  - `test/<short-description>` — test-only changes
- Keep PRs focused: one logical change per PR.
- **DCO is mandatory.** Every commit must carry a `Signed-off-by:` trailer (`git commit --signoff`). The CI `dco` job rejects PRs missing trailers on any non-merge commit.
- **Cryptographic signing is encouraged** (`git commit -S -s …`). Configure SSH/GPG signing locally so you can pass `-S` by default.
- Use Conventional Commits on commits that land on `main` so `release-please` can infer the next version:
  - `feat:` → minor bump
  - `fix:` / `perf:` → patch bump
  - `docs:`, `test:`, `ci:`, `chore:`, `refactor:` → no bump by default
  - `!` in the type or a `BREAKING CHANGE:` footer → major bump
- If you squash-merge, the squash commit message must also follow Conventional Commit format.
- PRs should include: summary, motivation, the exact tests/checks that were run with their outcomes, and doc updates (`README.md`, `docs/architecture.md`, the `docs/reference/` pages, `RELEASE.md`) when behavior changes. The CRD API reference (`docs/reference/api.md`) is generated — run `go -C tools tool task docs:api-ref` (enforced by `verify-generated`); never hand-edit it.

## Documentation Expectations

- Update `README.md` for user-visible behavior changes (flags, CRD schema, expected outputs).
- Update `RELEASE.md` when release or packaging behavior changes.
- Update `CONTRIBUTING.md` when contributor workflow changes.
- Update this file (`AGENTS.md`) when project structure, tooling, or expectations for AI agents change.

## When Unsure

- Choose the safer behavior.
- Avoid expanding scope beyond the requested change.
- Match existing patterns (kubebuilder layout, ginkgo specs, task wiring) instead of inventing new ones.
