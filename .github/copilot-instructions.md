<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->

# GitHub Copilot Instructions for Fathom

Fathom is a Kubernetes operator (`skaphos.io` domain) that reconciles
`HealthCheck` and `ClusterHealth` custom resources and persists `HealthReport`
history. It is built with `kubebuilder` (`go.kubebuilder.io/v4`) and packaged
via `operator-sdk` for OLM bundle distribution. See `AGENTS.md` for the full
contributor briefing — this file is the short version for Copilot.

## What Good Changes Look Like

- Small, focused pull requests with one logical change.
- Straightforward code: small functions, clear names, early returns, simple control flow.
- Standard Go layout and naming conventions used throughout the repository.
- Reconciliation logic that is idempotent, bounded, and respects `spec.timeout` on health checks.
- Configuration changes routed through `internal/app` (cobra + viper) rather than ad-hoc flag parsing.

## Safety Rules

- Do not introduce cluster-wide RBAC beyond what the operator strictly needs; new permissions must come from `+kubebuilder:rbac` markers.
- Keep the `ClusterHealth` external contract stable. It is derived only from `HealthCheck.status` — never from `HealthReport` history.
- Reconcilers must avoid unbounded work and side effects outside the cluster API.
- Do not hand-edit generated files (`zz_generated_*.go`, OLM bundle metadata, manifests under `config/`). Re-run the relevant task instead.

## Codebase Shape

- `cmd/main.go`: thin entrypoint.
- `internal/app/`: cobra/viper plumbing, manager construction, controller registration.
- `internal/controller/`: reconciler implementations.
- `api/v1alpha1/`: CRD type definitions.
- `config/`: kustomize overlays, RBAC, CRDs, OLM scaffolding.
- `test/e2e/`: Ginkgo suite that runs against a Kind cluster.
- `tools/`: pinned tooling launched via `go -C tools tool task ...`.

## Testing Expectations

Before proposing a PR, prefer the task-based checks:

- `go -C tools tool task fmt`
- `go -C tools tool task lint`
- `go -C tools tool task test`
- `go -C tools tool task staticcheck`
- `go -C tools tool task vuln`

Or run the full local CI sweep: `go -C tools tool task ci`.

E2E tests (`task test-e2e`) require a local Kind cluster and Docker.

If a change is small, run the narrowest relevant tests. New behavior must
include direct test coverage. Use the existing seams in `internal/app`
(`managerFactory`, the `Setupper` interface) so unit tests do not need
envtest.

## Documentation Expectations

- Update `README.md` for user-visible behavior changes (flags, CRD schema, outputs).
- Update `RELEASE.md` for release or packaging changes.
- Update `CONTRIBUTING.md` and `AGENTS.md` when contributor workflow or AI-agent expectations change.

## Go and Repository Conventions

- Use the Go version declared in `go.mod`.
- Keep files `gofmt` and `goimports` clean.
- Maintain REUSE/SPDX metadata. New source files should include the SPDX header at `hack/boilerplate.go.txt` (or its REUSE equivalent for non-Go files).
- Tests should follow Ginkgo v2 + Gomega conventions where applicable, or stdlib `testing` for plain unit tests.

## Pull Request Instructions

When drafting a pull request:

- Explain what changed and why.
- Summarize user-visible or behavior changes clearly.
- List the exact tests and checks that were run, with outcomes.
- Call out doc updates when behavior changed.
- Mention residual risks, limitations, or follow-up work if relevant.

## Commit and Branch Guidance

- Never target direct commits to `main`; changes land through pull requests.
- Branch names use a change-type prefix: `feature/`, `fix/`, `chore/`, `docs/`, `ci/`, `refactor/`, `test/`.
- Conventional Commit subjects: `feat:`, `fix:`, `perf:`, `docs:`, `test:`, `ci:`, `chore:`, `refactor:`.
- All commits MUST carry a DCO sign-off (`git commit --signoff`). Cryptographic signing (`-S`) is encouraged.

## When Unsure

- Choose the safer behavior.
- Avoid expanding scope beyond the requested change.
- Match existing command patterns, test style, and output conventions instead of inventing new ones.
