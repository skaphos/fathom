# Contract: CRD Schema-Compatibility Gate

The CI gate's inputs, outputs, and override semantics. Implements the
api-versioning standard's "Enforcement and tooling" recommendation.
Tool decision and rationale: [research.md](../research.md) R7–R8.

## Invocation

| Surface | Form |
|---|---|
| Local | `go -C tools tool task crd-compat` |
| Script | `scripts/check-crd-compat.sh` (bash, repo root, requires git tags fetched) |
| CI | `crd-compat` job in `.github/workflows/ci.yml`, checkout with `fetch-depth: 0` |
| Tool | `sigs.k8s.io/crdify` pinned in `tools/go.mod` |

## Algorithm (normative)

1. Baseline = highest semver tag matching `v*` reachable in the clone
   (`git tag --list 'v*' --sort=-v:refname | head -1`). No tags → gate exits
   0 with a "no baseline" notice (first-ever release case).
2. For each `config/crd/bases/*.yaml` that also exists at the baseline tag:
   extract the old copy (`git show <tag>:<path>`), run crdify old vs. new.
3. Files new since the baseline → skipped, listed as "new CRD (no baseline)".
4. Each incompatibility crdify reports is matched against
   `.crd-compat-allowlist.yaml` (by `crd` + `path`, `*` wildcard allowed):
   - matched → reported as `SANCTIONED` with the entry's reason + issue; does
     not fail the gate.
   - unmatched → reported as `INCOMPATIBLE`; fails the gate.
5. Allowlist entries that matched nothing → `STALE` warning (exit 0), so the
   file self-prunes in review.

## Exit codes / output

| Condition | Exit | Output requirements |
|---|---|---|
| no schema changes / compatible only | 0 | per-CRD one-line summary |
| incompatible, unsanctioned | 1 | CRD name, property path, crdify's change description, and a pointer to the allowlist mechanism — sufficient to locate the change without re-deriving the diff (SC-006) |
| incompatible, all sanctioned | 0 | each finding labeled `SANCTIONED: <reason> (<issue>)` |
| stale allowlist entries | 0 | `STALE` warning per entry |
| no baseline tag | 0 | notice |

## Pass/fail matrix (asserted by the fixture shell test)

| Change vs. baseline | Verdict |
|---|---|
| none | pass |
| new optional field | pass |
| new CRD file | pass (skipped) |
| field removed / renamed / type changed | fail |
| validation tightened (new CEL rule, raised minimum, added pattern) | fail unless allowlisted |
| enum values removed | fail |
| loosened validation | pass |
| description-only change | pass |
| allowlisted removal | pass, visibly `SANCTIONED` |
| allowlist entry matching nothing | pass with `STALE` warning |

## Initial state

Ships with `.crd-compat-allowlist.yaml` seeded for this feature's own
tightenings (floors, policy bounds) against the v0.4.x baseline — the gate
must be green on the PR that introduces it, with the sanctioned findings
visible in its output, and those entries become `STALE` warnings (then get
pruned) once a release containing this feature is tagged.
