<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# AddonDefinition RFC quickstart

This guide validates the documentation package for the AddonDefinition design
RFC. It applies from the repository root and is intentionally bounded to
document checks and read-only evidence. It does not implement a CRD, change
runtime behavior, or approve the RFC.

## Prerequisites

Install or make available:

- `git`
- `python3`
- `reuse` (for the repository license check)
- `gh` only when performing the later, read-only GitHub checks

The feature artifacts are [spec.md](spec.md), [plan.md](plan.md),
[research.md](research.md), [data-model.md](data-model.md), and the review
contract at [contracts/rfc-review.md](contracts/rfc-review.md). `tasks.md` is
created by the later spec-kit task-generation step; it is not a prerequisite
for these checks.

Check the expected Phase 1 paths without requiring `tasks.md`:

```sh
for path in \
  specs/011-addon-definition-rfc/spec.md \
  specs/011-addon-definition-rfc/plan.md \
  specs/011-addon-definition-rfc/research.md \
  specs/011-addon-definition-rfc/data-model.md \
  specs/011-addon-definition-rfc/contracts/rfc-review.md
do
  test -f "$path" || { printf 'missing prerequisite: %s\n' "$path" >&2; exit 1; }
done
```

Resolve the spec-kit paths for the current feature without requiring tasks:

```sh
bash .specify/scripts/bash/check-prerequisites.sh --json --paths-only
```

## Local checks

Run these commands from the repository root. They include tracked and
untracked files where the check needs to cover the new documentation.

Check whitespace in the feature directory, including untracked files:

```sh
git diff --check -- specs/011-addon-definition-rfc
python3 - <<'PY'
from pathlib import Path

bad = []
for doc in Path("specs/011-addon-definition-rfc").rglob("*"):
    if doc.is_file():
        for line_no, line in enumerate(doc.read_text().splitlines(), 1):
            if line.endswith((" ", "\t")):
                bad.append(f"{doc}:{line_no}")
if bad:
    raise SystemExit("Trailing whitespace:\n" + "\n".join(bad))
print("tracked and untracked feature-document whitespace: OK")
PY
```

Check that every relative Markdown link in the feature directory resolves:

```sh
python3 - <<'PY'
from pathlib import Path
import re
from urllib.parse import urlsplit

root = Path("specs/011-addon-definition-rfc")
pattern = re.compile(r"\[[^]]+\]\(([^)]+)\)")
bad = []
for doc in root.rglob("*.md"):
    for target in pattern.findall(doc.read_text()):
        target = target.split("#", 1)[0].strip().strip("<>")
        if not target or urlsplit(target).scheme or target.startswith("//"):
            continue
        if not (doc.parent / target).resolve().exists():
            bad.append(f"{doc}: {target}")
if bad:
    raise SystemExit("Broken relative Markdown links:\n" + "\n".join(bad))
print("relative Markdown links: OK")
PY
```

Check for template markers or unresolved planning text. The expected literal
`[NEEDS CLARIFICATION]` marker is excluded from the unresolved-marker scan.

```sh
python3 - <<'PY'
from pathlib import Path
import re

root = Path("specs/011-addon-definition-rfc")
markers = re.compile(
    r"\b(TODO|FIXME|TBD|PLACEHOLDER)\b|\{\{[^}]+\}\}|<INSERT[^>]*>"
    r"|\[(FEATURE|DATE|###[-\w]+|REMOVE IF UNUSED|link)\]"
    r"|\[NEEDS CLARIFICATION:|\bNEEDS CLARIFICATION\b", re.I
)
bad = []
for doc in root.rglob("*.md"):
    fenced = False
    for line_no, line in enumerate(doc.read_text().splitlines(), 1):
        if line.strip().startswith("```"):
            fenced = not fenced
            continue
        if fenced:
            continue
        if "[NEEDS CLARIFICATION]" in line:
            line = line.replace("[NEEDS CLARIFICATION]", "")
        if markers.search(line):
            bad.append(f"{doc}:{line_no}: {line}")
if bad:
    raise SystemExit("Unresolved template markers:\n" + "\n".join(bad))
print("template markers: OK")
PY
```

Run the repository's REUSE license check from the repository root (the tool
does not accept a path argument):

```sh
reuse lint
```

The local checks establish document consistency only. They cannot prove
runtime behavior, Kubernetes authorization behavior, an approved design, or
that the six decisions are sufficient. Manually review each of the six
decision sections (schema, RBAC, contract versioning, identity and precedence,
runtime lifecycle, and bounded failure isolation), then walk the lifecycle,
collision, authorization, and failure scenarios in the review contract.
Confirm that each decision records alternatives, rationale, consequences, and
an observable acceptance example, and that proposed text remains distinguishable
from accepted text.

## Eventual review and handoff checks

After a real PR exists, set `FATHOM_RFC_PR` to that task-specific PR number.
These commands are read-only; they do not create, edit, approve, merge, or
comment on any GitHub object.

```sh
test -n "$FATHOM_RFC_PR" || { printf 'set FATHOM_RFC_PR first\n' >&2; exit 1; }
gh pr view "$FATHOM_RFC_PR" --repo skaphos/fathom --json number,state,isDraft,mergedAt,mergeCommit,reviewDecision,author,baseRefName,body,url
gh api --paginate "repos/skaphos/fathom/issues/$FATHOM_RFC_PR/comments"
gh api --paginate "repos/skaphos/fathom/pulls/$FATHOM_RFC_PR/reviews"
gh issue view 280 --repo skaphos/fathom --json number,state,title,body,comments,url
FATHOM_RFC_MERGE_SHA="$(gh pr view "$FATHOM_RFC_PR" --repo skaphos/fathom --json mergeCommit --jq '.mergeCommit.oid')"
test -n "$FATHOM_RFC_MERGE_SHA" && test "$FATHOM_RFC_MERGE_SHA" != "null" || { printf 'PR is not merged\n' >&2; exit 1; }
gh api -H 'Accept: application/vnd.github.raw+json' \
  "repos/skaphos/fathom/contents/docs/rfc/0001-addondefinition-crd.md?ref=$FATHOM_RFC_MERGE_SHA"
```

Read the final PR text after merge and verify it matches the accepted RFC,
then verify that #280 points to the merged RFC and its ADR, identifies the
scoped implementation work, records the disposition of #256, and preserves
the required e2e validation work. Explicit owner/decider acceptance can be
recorded in the PR conversation; review metadata is supporting evidence, not
the sole proof of acceptance. The RFC is accepted after that explicit approval
and its final text is recorded. Feature completion still waits for merge to
`main`, an accepted-RFC readback, the linked ADR record, and the #280 handoff.
A local pass therefore means “checks pass, RFC still proposed,” not “feature
accepted.”
