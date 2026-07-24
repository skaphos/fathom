# Quickstart: Validating the v0.5.0 Adversarial Review Release Gate

**Date**: 2026-07-24 | **Plan**: [plan.md](plan.md) | **Contracts**: [contracts/findings-report.md](contracts/findings-report.md)

This guide verifies the release gate end-to-end. It validates deliverables
and dispositions; it does not re-run the review itself.

## Prerequisites

- Repo checkout with this feature branch (or `main` after merge)
- `gh` CLI authenticated against `skaphos/fathom`
- Go toolchain (version per `go.mod`) for the CI gate

## 1. Milestone precondition (FR-001)

```sh
gh issue list --milestone v0.5.0 --state open --json number,title
```

**Expected**: only #217 remains open. (Verified 2026-07-24 at plan time.)

## 2. Deliverables exist and follow the contract (FR-004, FR-009)

```sh
ls specs/004-adversarial-review/review/
# expected: findings.md  refuted.md  coverage.md
head -6 specs/004-adversarial-review/review/findings.md
```

**Expected**: all three files present; `findings.md` header records the
anchor commit and the five perspectives; findings are ordered
critical → high → medium → low with all required fields per the
[report contract](contracts/findings-report.md); empty severity sections say
`_No confirmed findings._`; `coverage.md`'s Reviewed + Excluded tables
jointly classify every top-level repo path (SC-004).

## 3. Every critical/high finding is dispositioned (FR-005, SC-002)

```sh
grep -n "Disposition" specs/004-adversarial-review/review/findings.md
```

**Expected**: zero critical/high entries with a disposition other than
`fixed (PR #NN)` or `deferred (issue #NN)`. For each referenced PR/issue:

```sh
gh pr view <NN> --json state,mergedAt,title      # state MERGED
gh issue view <NN> --json state,title,body        # OPEN follow-up containing "Deferral rationale"
```

## 4. Fix quality gates (FR-006, SC-003)

For each `fixed` disposition:

- The PR description states the regression test and check outcomes.
- The regression test fails before the fix: verifiable via
  `git stash`-free spot check — check out the PR's parent commit for the test
  file's package and run `go test ./<pkg>/...` (test absent or failing), then
  the merge commit (test passes). CI history on the PR is equivalent
  evidence.
- Runtime-behavior fixes (surfaces listed in AGENTS.md) link a green
  `kind e2e` workflow run.

Full gate at the end of the work:

```sh
go -C tools tool task ci          # lint, unit tests, staticcheck, vuln, build — green
go -C tools tool task crd-compat  # no unsanctioned incompatible CRD changes (FR-007)
```

## 5. Refutation record (FR-003)

```sh
cat specs/004-adversarial-review/review/refuted.md
```

**Expected**: every non-confirmed candidate appears with a concrete
refutation reason; confirmed findings in `findings.md` each carry a
`Refutation attempted` line.

## 6. Gate closure (FR-010, SC-005)

```sh
gh issue view 217 --json state,comments --jq '.state'
gh issue list --milestone v0.5.0 --state open
```

**Expected**: the closing comment on #217 matches contract §5 (deliverable
links + disposition table); after closure, the open list is empty and #217's
`closedAt` is the latest on the milestone.
