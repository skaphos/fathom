# Phase 0 Research: Adversarial Codebase Review for the v0.5.0 Release Gate

**Date**: 2026-07-24 | **Plan**: [plan.md](plan.md)

No `NEEDS CLARIFICATION` markers remained in the Technical Context; the open
questions were methodology choices. Each is resolved below as a recorded
decision.

## R1. Review perspective set and surface assignment

**Decision**: Five perspectives, exactly as issue #217 names them, each with a
primary surface (all perspectives may raise findings anywhere):

| Perspective | Primary surface |
|---|---|
| Security | `internal/nodecert/`, `internal/probe/`, `cmd/probe/`, `cmd/node-agent/`, `internal/app/` (serving/TLS/leader-election), input handling from CR specs |
| Correctness / reconcile-time behavior | `internal/controller/`, `internal/adapter/`, `internal/app/run.go`, timeout/requeue/idempotency, status semantics |
| API / CRD contract stability | `api/v1alpha1/`, `pkg/adapter/`, `config/crd/bases` (as generation output), `ClusterHealth` derivation rule |
| RBAC least-privilege | `config/rbac/`, `+kubebuilder:rbac` markers, node-agent DaemonSet privileges, NetworkPolicy |
| Supply-chain / CI | `.github/workflows/`, `Taskfile.yml`, `tools/` pinning, `scripts/`, Dockerfiles, release/bundle pipeline |

**Rationale**: matches the issue's explicit scope list; assigning a primary
surface guarantees every repository area has at least one owning lens
(supports SC-004) while cross-reporting stays allowed.

**Alternatives considered**: a single monolithic review pass (rejected — the
issue explicitly prefers several diverse lenses over one pass); more lenses
(performance, docs) as standalone perspectives (rejected — folded into
correctness and Principle-IX spot checks to keep the gate bounded; the
coverage statement will say so).

## R2. Independence and refutation protocol

**Decision**: Each perspective produces candidate findings without seeing the
other perspectives' output. Refutation is a separate skeptical pass per
candidate, performed against the actual code (not the finder's summary), with
the burden on the finding: the refuter attempts to construct the concrete
reason the finding is wrong (unreachable path, existing guard, test coverage,
misread semantics). Outcomes: `confirmed`, `refuted` (with reason, retained
in `review/refuted.md`), or `contested` (kept, dispositioned at the higher
proposed severity per the spec's edge case).

**Rationale**: FR-002/FR-003 require independence and adversarial framing;
recording refuted candidates preserves the working record the issue asks for.

**Alternatives considered**: majority-vote panels per finding (rejected as
default — heavier than needed for a repo this size; reserved for findings
where a single refutation pass is inconclusive); trusting finder output
without re-reading code (rejected — that is exactly the
plausible-but-false failure mode the issue targets).

## R3. Severity rubric

**Decision**: Four levels, judged at the point of confirmation:

- **Critical**: exploitable security defect (privilege escalation, secret
  exposure, unauthenticated access), data loss, or operator crash-loop /
  cluster-impacting failure in a default configuration.
- **High**: security weakness needing non-default but realistic conditions;
  correctness defect producing wrong health verdicts or wrong `ClusterHealth`
  contract output; unbounded reconcile work; RBAC grant materially beyond
  need; CI/supply-chain gap that would let unsigned/unreviewed code ship.
- **Medium**: defect with limited blast radius, misleading-but-recoverable
  status, hardening gaps with compensating controls, flaky gates.
- **Low**: code-quality, docs-honesty, or defense-in-depth polish.

**Rationale**: maps the spec's assumption (exploitability/impact for
security, user-visible damage for correctness) onto the operator's actual
failure domains; the critical/high boundary is what triggers mandatory
disposition (FR-005).

**Alternatives considered**: CVSS scoring (rejected — most findings here are
correctness/process, where CVSS is noise; the rubric keeps ranking arguable
in one sentence per finding).

## R4. Anchoring and drift handling

**Decision**: The review anchors to the `main` commit recorded at review
start (`origin/main` = `54974db93a8cf` at plan time; the implementing task
re-records it). The v0.5.0 milestone was verified otherwise clear on
2026-07-24 (#217 is the only open issue), so the fix phase may start
immediately. If commits land on `main` mid-review, the affected perspective
re-examines only the touched surface or the coverage statement lists the
delta as out of scope, per the spec's edge case.

**Rationale**: FR-001; determinism (Principle III) requires a reproducible
review subject.

**Alternatives considered**: freezing `main` via branch protection during the
review (rejected — unnecessary process weight; the delta-handling rule covers
the realistic case of merging this review's own fix PRs).

## R5. Tool-assisted evidence

**Decision**: Run the existing pinned gates once against the anchor commit
before manual review: `go -C tools tool task ci` (lint, unit tests,
staticcheck, govulncheck, build), plus `task crd-compat`. Their output seeds
candidate findings for the relevant perspectives but no perspective may
consist solely of tool output; each lens does a manual pass over its primary
surface.

**Rationale**: the tools are already pinned and CI-required (constitution:
Engineering Constraints), so they are free corroborating evidence; the issue
demands a skeptical human-style pass beyond them.

**Alternatives considered**: adding new scanners (gosec, semgrep, trivy,
checkov) to the repo for this review (rejected — introducing new tooling is
scope creep for a release gate; individual perspectives may still consult
such tools ad hoc as evidence without adding them to the build).

## R6. Disposition workflow for confirmed findings

**Decision**: Critical/high findings are fixed on this milestone via focused
conventional-commit PRs (one logical change each, regression test that fails
before the fix, DCO sign-off), or deferred by opening a follow-up issue
labeled with severity and a written deferral rationale, linked from #217.
Contract-breaking fixes (CRD schema, `ClusterHealth` derivation) are always
deferred (FR-007). Medium/low findings are recorded in `review/findings.md`
with a triage note; fixing them is optional and never blocks the gate.
`review/findings.md` tracks disposition state per finding
(`open → fix-in-progress → fixed(PR#)` or `deferred(issue#)`).

**Rationale**: FR-005/FR-006/FR-008 and the repository's PR governance;
one-PR-per-finding keeps the "focused PR" constitution rule intact and lets
CI/e2e evidence attach cleanly to each disposition.

**Alternatives considered**: one omnibus fix PR (rejected — violates the
focused-PR rule and makes regression attribution murky); fixing medium/low
by default (rejected — spec explicitly scopes mandatory fixes to
critical/high).

## R7. e2e evidence for runtime-behavior fixes

**Decision**: Fixes touching the surfaces AGENTS.md lists as e2e-mandatory
(`internal/app/run.go`, adapters, controllers, `pkg/adapter`, CRD types,
probe/nodecert, fixtures) record their e2e evidence as the `kind e2e` CI
workflow run on the fix PR (local kind+podman cannot run the stack in this
environment). The PR description states the exact runs and outcomes.

**Rationale**: FR-006 and AGENTS.md policy; the CI pipeline is the
project-sanctioned e2e path and produces durable, linkable evidence.

**Alternatives considered**: none viable — local e2e is documented as
unavailable on this machine.

## R8. Deliverable placement

**Decision**: Deliverables live in-repo under
`specs/004-adversarial-review/review/` (`findings.md`, `refuted.md`,
`coverage.md`) and are summarized in a closing comment on #217 that links to
them and enumerates every disposition (PR/issue numbers). #217 closes only
after every critical/high disposition is merged/filed and the milestone shows
it as the last open issue.

**Rationale**: FR-009/FR-010 and Principle II (deliverables round-trip
through Git; the issue comment is the discoverable pointer, not the durable
store).

**Alternatives considered**: issue-comment-only deliverables (rejected — not
Git-durable; long tables age badly in issue threads).
