# Review Run Record — v0.5.0 Release Gate (#217)

**Anchor commit**: `cb845dd64fb480dacdb4c4363fbd4cb47bceea55` (`origin/main`, re-recorded at review start per research R4)
**Review started**: 2026-07-24
**Milestone precondition**: verified 2026-07-24 — `gh issue list --milestone v0.5.0 --state open` returns only #217

Note: the plan was drafted against `54974db`; two commits landed before review
start and are inside the anchor, not drift: `0ac5684` (tools dep bump
go-git/v5) and `cb845dd` (SECURITY.md).

## Baseline gate evidence (T003/T004)

| Gate | Command | Result |
|------|---------|--------|
| Full CI | `go -C tools tool task ci` | **GREEN** (second run) — lint, envtest unit suites (per-package coverage 75–100%), staticcheck, govulncheck, build all pass at the anchor. First run failed on stale golangci-lint cache entries referencing deleted sibling worktrees (environment artifact, not a code issue); cache cleaned and re-run. |
| CRD compatibility | `go -C tools tool task crd-compat` | **OK** — baseline v0.4.1; 11 findings across `addonchecks` (9) and `nodecertificatechecks` (2), all SANCTIONED by `.crd-compat-allowlist.yaml` entries tied to #152 validation hardening; `clusterhealths`/`healthchecks`/`healthreports` fully compatible |

## Working records

- Candidate files: `candidates-<perspective>.md` — the five raw perspective
  passes, retained in full as the working record (they hold each candidate's
  detailed failure scenario, evidence quotes, and the finder's own refutation
  notes; `findings.md` is the ranked, dispositioned distillation).
- Refuted candidates: [refuted.md](refuted.md)
- Confirmed findings: [findings.md](findings.md)
- Coverage statement: [coverage.md](coverage.md)

## Consolidation / duplicate merge (T010)

The 34 candidates were checked for cross-perspective duplicates. **No exact
duplicates** were found — several candidates are *complementary* views of the
same node-agent surface but describe distinct defects and are kept separate:

- SEC-1 (VAP match-condition scope) vs RBAC-2 (node-agent role ConfigMap
  breadth) — one is admission-policy coverage, the other is RBAC; different
  fixes.
- SEC-2 (unauthenticated `/metrics`) vs RBAC-3 (NetworkPolicy egress breadth) —
  both about node-agent network exposure but distinct controls.
- COR-3 / COR-4 / COR-7 interact (NodeCert rollup freshness/completeness) but
  are separate code paths and separately fixable.

No IDs were merged; the originating-perspective IDs are preserved.

## Adversarial refutation (T011)

The 5 high-severity candidates (COR-1, API-1, API-2, RBAC-4, RBAC-5) were
independently re-verified against the code at the anchor (not the finder
summaries) — see the load-bearing reads in the review log. All 34 candidates
survived; two had severity adjusted (API-2 high→medium, RBAC-5 high→accepted).
Details in [refuted.md](refuted.md).
