# Contributing Guidelines

Thanks for contributing to Fathom.

## Development Setup

- Go version: see `go.mod`.
- Tool versions: see `.tool-versions` (Go, golangci-lint, operator-sdk).
- Run task targets without installing Task globally:
  - `go -C tools tool task --list`

Fathom is scaffolded with operator-sdk (go/v4 plugin) for OLM bundle support.
Kubernetes tooling (controller-gen, kustomize, setup-envtest) is pinned and
invoked via `go run`. `operator-sdk`, `opm`, `kind`, `kubectl`, and `docker`
must be on `PATH` when you run the corresponding tasks.

## Branching and Commits

- Create focused branches from `main`.
- Keep commits small and scoped.
- Use DCO sign-offs on every commit:
  - `git commit --signoff ...`
  - Required trailer format: `Signed-off-by: Your Name <you@example.com>`
- Use Conventional Commits on commits that land on `main`:
  - `feat:` -> minor
  - `fix:` / `perf:` -> patch
  - `docs:`, `test:`, `ci:`, `chore:`, `refactor:` -> no bump by default
- If you use squash merges, the final squash commit message must also follow
  Conventional Commit format.

## Coding Standards

- Follow Go conventions and keep code readable.
- Keep REUSE metadata valid:
  - Source files should include SPDX headers (`SPDX-License-Identifier: MIT`).
  - Use `reuse lint` to validate licensing metadata.
- Format code:
  - `go -C tools tool task fmt`
- Lint code:
  - `go -C tools tool task lint`

## Testing

Run before opening a PR:

- `go -C tools tool task test`
- `go -C tools tool task staticcheck`
- `go -C tools tool task vuln`

Or run full local CI:

- `go -C tools tool task ci`

End-to-end tests (`task test-e2e`) require a local `kind` cluster and Docker.

## Pull Requests

PRs should include:

- Summary of what changed
- Why the change is needed
- Testing performed (commands and results)
- Docs updates when behavior changes (`README.md`, `DESIGN.md`)

## Safety Expectations

- Check handlers must honor `spec.timeout` and run bounded work.
- Do not introduce cluster-wide RBAC beyond what the operator needs.
- Keep the `ClusterHealth` external contract stable; derive it only from
  `HealthCheck.status` (never from `HealthReport` history).
