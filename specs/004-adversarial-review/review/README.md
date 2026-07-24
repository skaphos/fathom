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
| Full CI | `go -C tools tool task ci` | _pending — recorded below when complete_ |
| CRD compatibility | `go -C tools tool task crd-compat` | **OK** — baseline v0.4.1; 11 findings across `addonchecks` (9) and `nodecertificatechecks` (2), all SANCTIONED by `.crd-compat-allowlist.yaml` entries tied to #152 validation hardening; `clusterhealths`/`healthchecks`/`healthreports` fully compatible |

## Working records

- Candidate files: `candidates-<perspective>.md` (deleted after consolidation into `findings.md`; unified candidate list preserved below)
- Refuted candidates: [refuted.md](refuted.md)
- Confirmed findings: [findings.md](findings.md)
- Coverage statement: [coverage.md](coverage.md)
