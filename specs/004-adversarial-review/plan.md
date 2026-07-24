# Implementation Plan: Adversarial Codebase Review for the v0.5.0 Release Gate

**Branch**: `feature/004-adversarial-review` | **Date**: 2026-07-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/004-adversarial-review/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

Execute the v0.5.0 release gate (issue #217): a whole-repository adversarial
review from five independent perspectives (security, correctness/
reconcile-time behavior, API/CRD contract stability, RBAC least-privilege,
supply-chain/CI), with every candidate finding subjected to an explicit
refutation attempt before confirmation. Confirmed findings are ranked
(severity, file:line, failure scenario); every critical/high finding is fixed
in-milestone with a regression test or deferred to a tracked follow-up issue
with written rationale; a coverage statement eliminates silent gaps. Both
deliverables land on #217, which closes last on the milestone.

Technical approach: the review is anchored to a recorded `main` commit
(`54974db93a8cfc3d182a38ce1dcf2ee9f0e5383e` at plan time; re-anchored at
review start if `main` has moved). Each perspective runs as an independent
review pass over its assigned surface, producing structured candidate
findings; a separate skeptical pass attempts to refute each candidate against
the actual code. Existing pinned analysis tooling (`golangci-lint`,
`staticcheck`, `govulncheck`, coverage gate) supplies corroborating evidence
but does not substitute for the manual passes. Fixes ship as focused
conventional-commit PRs gated by `go -C tools tool task ci`, `crd-compat`,
and — for runtime-behavior changes — the CI e2e pipeline.

## Technical Context

**Language/Version**: Go 1.26.5 (per `go.mod`); review artifacts are Markdown

**Primary Dependencies**: kubebuilder v4 / controller-runtime operator stack;
pinned analysis tooling in `tools/` (`golangci-lint`, `staticcheck`,
`govulncheck`, `controller-gen`, crdify via `crd-compat`); `gh` CLI for issue
and PR operations

**Storage**: N/A (deliverables are Markdown files under
`specs/004-adversarial-review/review/` plus GitHub issues/PR records)

**Testing**: envtest unit suites via `go -C tools tool task test`; Ginkgo e2e
via the `e2e.yml` CI pipeline (local e2e unavailable in this environment);
full gate via `go -C tools tool task ci`

**Target Platform**: the Fathom repository itself at the recorded `main`
commit; fixes target Linux/Kubernetes operator runtime

**Project Type**: Kubernetes operator repository; this feature is a review
process + fix batch, not a new runtime component

**Performance Goals**: N/A (process feature); bounded review effort — each
perspective completes over its full assigned surface, no sampling without
disclosure in the coverage statement

**Constraints**: fixes must not break CRD schema compatibility
(`task crd-compat` against the latest release tag) or the `ClusterHealth`
external contract; coverage-gate thresholds never lowered; every fix carries
a regression test that fails before the fix; DCO + conventional commits;
#217 closes last on the milestone

**Scale/Scope**: whole repository (~`api/`, `cmd/`, `internal/`, `pkg/`,
`config/`, `scripts/`, `test/`, `tools/`, `.github/workflows/`, docs);
generated artifacts reviewed for generation-input correctness only

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle / Constraint | Impact of this feature | Status |
|---|---|---|
| I. Explicit state over implicit behavior | Review verifies it (findings against implicit-behavior defects); the review record itself is explicit, durable Markdown + tracked issues | PASS |
| II. Git is the durable desired-state boundary | All deliverables round-trip through Git (spec dir + PRs); dispositions live in tracked issues | PASS |
| III. Deterministic, reconstructible operation | Review anchored to a recorded commit; fixes re-run pinned generation tasks, never hand-edit generated files | PASS |
| IV. Kubernetes-native, never obscured | No runtime surface added; fixes stay within operator/CRD model | PASS |
| V. Compose, don't trap | No new dependencies introduced by the review; any fix adding a dependency is flagged in its PR | PASS |
| VI. Explainable reconciliation, evidence-grade audit | Findings carry evidence (file:line, failure scenario, refutation record); refuted candidates retained with reasons | PASS |
| VII. Read-only degradation over blindness | Reviewed as a correctness lens; no impact from the process itself | PASS |
| VIII. Topology is deployment state | Not implicated; in scope for the correctness lens where relevant | PASS |
| IX. Technical precision, honest scope | Coverage statement states exactly what was and wasn't reviewed — the principle applied to the review itself | PASS |
| `ClusterHealth` contract stability | FR-007: contract-breaking fixes are deferred, never merged in-milestone | PASS |
| Bounded, idempotent reconciliation | Core check of the correctness perspective | PASS |
| Minimal RBAC | Dedicated perspective; any RBAC fix goes through `+kubebuilder:rbac` markers + manifest tasks | PASS |
| Configuration model | Any fix adding options extends `Options` + `bindings()` | PASS |
| Workflow gates (tests, coverage ratchet, DCO, conventional commits, focused PRs) | Encoded in FR-006 and the disposition contract | PASS |

**Initial gate: PASS** — no violations; Complexity Tracking left empty.

**Post-Phase-1 re-check: PASS** — the design artifacts (data model, report
contract, quickstart) introduce no runtime components, no new dependencies,
no RBAC, and no CRD changes; all fix-phase guardrails are encoded in the
contracts.

## Project Structure

### Documentation (this feature)

```text
specs/004-adversarial-review/
├── spec.md              # Feature specification (/speckit-specify output)
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/
│   └── findings-report.md   # Deliverable formats for #217 (findings list, coverage statement, deferral issues)
├── checklists/
│   └── requirements.md  # Spec quality checklist (complete)
├── review/              # Created during implementation
│   ├── findings.md      # Ranked confirmed findings (deliverable 1)
│   ├── refuted.md       # Refuted candidates + reasons (working record, FR-003)
│   └── coverage.md      # Coverage statement (deliverable 3)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

This feature adds no new source directories. The review surface and the
fix-landing zones are the existing tree:

```text
api/v1alpha1/            # CRD types — API/CRD contract + correctness lenses
cmd/                     # operator, probe, node-agent entrypoints — security + correctness
internal/app/            # options, manager construction — correctness + security
internal/controller/     # reconcilers — correctness/reconcile-time lens (primary)
internal/adapter/        # addon adapters + registry — correctness
internal/nodecert/       # cert scan engine + wire contract — security (primary)
internal/probe/          # probe pod lifecycle — security + RBAC
pkg/adapter/             # public adapter contract — API stability
config/                  # RBAC, CRDs, manager, OLM — RBAC + supply-chain lenses
.github/workflows/, scripts/, tools/, Taskfile.yml   # supply-chain/CI lens
test/, docs/, deploy/    # correctness of gates; docs honesty (Principle IX)
```

**Structure Decision**: single existing repository; review deliverables are
new Markdown files under `specs/004-adversarial-review/review/`; fixes land
in-place in the directories above via focused PRs. Generated artifacts
(`zz_generated*.go`, `config/crd/bases`, OLM bundle metadata,
`docs/reference/api.md`, `graphify-out/`) are excluded from line-by-line
review per the spec's assumptions and recorded as such in the coverage
statement.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations — table intentionally empty.
