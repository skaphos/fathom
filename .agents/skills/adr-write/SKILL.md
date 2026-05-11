---
name: adr-write
description: Use to author, review, or supersede Architecture Decision Records (ADRs) — short, immutable records of architecturally significant decisions, their constraints, options considered, and consequences. Stack-agnostic. Skip for routine implementation choices (no ADR needed) or behavior documentation (use a docs skill).
---

<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<!-- version: 0.1.0 -->
# ADR Mode

## Purpose
Use this skill to write, review, or supersede Architecture Decision Records (ADRs).

An ADR captures a single architecturally significant decision, the constraints that forced it, the options considered, and the consequences. This skill is language- and stack-agnostic and pairs with any language or platform skill.

## Skill Use
- Load this skill when the task is to author a new ADR, review a proposed ADR, or supersede an accepted one.
- Treat this skill as the governing contract for ADR format, storage, and lifecycle unless the repository has stricter local conventions.
- Keep repository-specific context (decision driver, deadline, stakeholders) in the invoking prompt.
- When this skill conflicts with casual convenience, follow this skill.

## Tool Use
This skill is tool-agnostic and works with Claude Code, Codex, OpenCode, and similar assistants. Map its guidance to whatever file-reading, editing, search, and shell-execution tools your environment exposes.

- Before writing a new ADR, list existing ADRs and read the ones that touch adjacent decisions. Supersession chains matter.
- Ground every claim in evidence: read the code, config, incident, or issue that motivated the decision rather than paraphrasing from memory.
- Issue independent tool calls (directory listing, multiple ADR reads, related code inspection) in parallel.
- When the ADR proposes a change, verify the described current state by inspecting the actual files — do not describe a state you have not seen.

## When To Use
Use this skill for:
- documenting a decision that constrains future work (framework choice, boundary placement, storage model, protocol, deployment topology)
- superseding an existing ADR whose constraints or assumptions have changed
- reviewing a draft ADR for completeness, grounding, and honest tradeoffs
- retrofitting an ADR for a decision already in place but undocumented

Do not use this skill for:
- routine implementation choices that don't constrain other work
- documenting behavior (use a docs skill)
- making the decision itself — an ADR records a decision; it does not substitute for the discussion that produced it

## When An ADR Is Warranted
An ADR is warranted when the decision:
- is hard to reverse (data model, API contract, deployment boundary, auth model)
- forecloses alternatives for other teams or future work
- emerges from tradeoffs rather than one obviously correct answer
- would surprise a future reader who did not see the discussion

A decision that can be cleanly changed in a later PR usually does not need an ADR.

## Directory And Naming Discovery
Before writing, determine the repository's ADR conventions:
- Common locations: `docs/adr/`, `docs/architecture/decisions/`, `adr/`, `decisions/`.
- Common numbering: zero-padded sequential (`0001-...`, `0002-...`) or date-prefixed (`2026-04-23-...`).
- Common filename style: kebab-case title, e.g., `0012-use-postgres-for-billing.md`.

Match the existing convention. Only introduce a new convention if none exists, and when introducing one, prefer zero-padded sequential with kebab-case titles.

## ADR Format
Use a MADR-compatible structure. The minimum required sections are marked **required**; others are recommended when relevant.

```markdown
# <Sequential number>. <Short decision statement>

- **Status**: proposed | accepted | deprecated | superseded by ADR-NNNN
- **Date**: YYYY-MM-DD
- **Deciders**: <names or roles>
- **Consulted**: <optional>
- **Informed**: <optional>

## Context and Problem Statement  *(required)*
<What forces the decision? What constraints, incidents, deadlines, or prior ADRs apply? Describe the world as it is, not as you wish it were.>

## Decision Drivers  *(required when non-obvious)*
- <driver 1>
- <driver 2>

## Considered Options  *(required)*
- <option A>
- <option B>
- <option C>

## Decision Outcome  *(required)*
Chosen option: "<option>", because <short justification anchored to drivers>.

### Consequences
- **Positive**: <what this unlocks or makes simpler>
- **Negative**: <what this costs or forecloses>
- **Neutral**: <side effects worth recording>

## Pros and Cons of the Options

### <Option A>
- Good, because <…>
- Bad, because <…>

### <Option B>
- Good, because <…>
- Bad, because <…>

## Links
- Related ADRs: <ADR-NNNN>
- Issues, RFCs, incidents, or prior art
```

Adapt when the repository already uses a different template (e.g., Nygard's original Status/Context/Decision/Consequences). Match the local format rather than forcing MADR.

## Writing Rules
- One ADR, one decision. Split compound decisions into separate ADRs.
- Context describes the forcing function; the decision is the response. If the context doesn't force a decision, the ADR probably isn't warranted.
- State the decision plainly. Avoid hedging verbs like "we might" or "we could consider."
- Record at least two serious options. "We'll use X" with no considered alternatives is rarely honest.
- Consequences must include real costs. An ADR with only positive consequences is suspect.
- Date the ADR and record who decided. Undated ADRs rot faster.
- Keep ADRs short. One to two screens is typical; longer ADRs should carry their length for a reason.

## Review Rules
When reviewing a draft ADR, check in this order:
1. **Warranted** — is an ADR the right artifact, or is this implementation detail?
2. **Grounded** — is the context anchored to specific code, config, incidents, or prior ADRs?
3. **Honest alternatives** — are the other options real, or strawmen to justify the chosen one?
4. **Consequences** — are the negatives stated plainly, not buried or euphemized?
5. **Scoped** — does it decide one thing, or has it drifted into several?
6. **Discoverable** — does it link to related ADRs and will the next reader find them?
7. **Format** — matches local conventions, has status and date, numbered consistently.

Style and prose come last.

## Superseding Rules
- Never edit an accepted ADR's decision. Create a new ADR that supersedes it.
- In the new ADR, state explicitly which ADR it supersedes and why.
- In the old ADR, update only the `Status` line to `superseded by ADR-NNNN` and add a link. Keep the rest of the old ADR intact so the history stays readable.
- Record what changed in the world that invalidated the prior decision, not just the new choice.
- If the supersession is partial (e.g., the old decision still applies to some systems), say so in the new ADR.

## Deprecation
Use `deprecated` when the decision is no longer active but no replacement is planned. Update the `Status` line, keep the body intact, and record why it was retired.

## Quality Checklist
Before considering an ADR complete, verify:
- status and date are present and accurate
- context describes the forcing function with concrete anchors
- at least two real options are considered
- decision is stated plainly with justification tied to drivers
- consequences include genuine negatives
- filename and numbering match the repository's convention
- related ADRs and prior art are linked
- no implementation instructions, operational runbooks, or how-to content leaked in

## Anti-Patterns To Reject
- ADRs that describe what was built instead of why
- ADRs that list only one option
- ADRs with no negative consequences
- ADRs edited in place after acceptance to change the decision
- ADRs recording routine choices that don't constrain future work
- ADRs that paraphrase tickets, chat history, or meeting notes without distillation
- ADRs whose context is aspiration ("we want to be cloud-native") rather than constraint
- Undated or unattributed ADRs
- Compound ADRs that mix multiple decisions and become impossible to supersede cleanly

## Invocation Template
Use this skill with a prompt that supplies repository-specific context. Example:

```text
Use ADR Mode.
Write an ADR for /path/to/repo/docs/adr/ documenting the decision to move session state out of Postgres into Redis.
Ground the context in the latency incident on 2026-03-18 and the existing ADR-0007 on session lifecycle.
Consider at least: keep in Postgres, move to Redis, move to DynamoDB.
Record honest negatives including operational surface area and failure modes.
```
