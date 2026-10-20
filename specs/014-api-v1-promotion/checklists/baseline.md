<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Baseline: toolchain and gates before implementation

**Purpose**: Record the branch base state so every later gate result has a
known-good reference (tasks.md T001).
**Created**: 2026-10-20
**Feature**: [spec.md](../spec.md)

| Item | Value |
|---|---|
| Base commit | `2871447` (spec artifacts only; code identical to `main` at `3b83bb0`, release 0.6.0) |
| Go toolchain | `go 1.27.2` per `go.mod` |
| controller-gen | v0.22.0 (`Taskfile.yml`) |
| envtest Kubernetes | 1.37 (`ENVTEST_K8S_VERSION`), assets from `setup-envtest@v0.25.2` |
| crdify | v0.6.0 (`tools/go.mod`) |

## Gate results at the base

- [x] `go -C tools tool task crd-compat`: OK. Six `STALE` allowlist entries
  are reported (0.5.x and 0.6.0 churn that is now part of the baseline tag);
  informational only, pruned when the allowlist is next edited (T031).
- [x] `go -C tools tool task ci`: `lint`, `fmt`, `vet`, `staticcheck`, `vuln`
  and `build` pass.
- [x] `go -C tools tool task test`: the first run failed in
  `internal/controller`'s envtest `BeforeSuite` with `Unauthorized` while
  installing CRDs. No existing-cluster override was set and no stray API
  server was running; the same suite passes alone with the pinned assets
  (`ok internal/controller 38.7s`). Read as a collision between the several
  envtest API servers `task test` starts in parallel, not a code failure.
  Rerun the task when it recurs.

## Notes

- Marker semantics: `[x]` means the gate was run at the base and its result
  recorded above.
- Background runs in this environment need `~/.local/share/mise/shims` on
  `PATH` and an isolated `GOLANGCI_LINT_CACHE` so parallel worktrees do not
  share lint state.
