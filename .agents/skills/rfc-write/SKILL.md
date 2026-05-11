---
name: rfc-write
description: Use to author, review, or revise Request for Comments (RFC) documents — proposals for non-trivial cross-team or hard-to-reverse changes that need discussion before commitment. RFC is upstream of an ADR (proposal vs. decision). Skip for already-decided choices (use `adr-write`) or single-team implementation that can be settled in a PR.
---

<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<!-- version: 0.1.0 -->
# RFC Mode

## Purpose
Use this skill to write, review, or revise Request for Comments (RFC) documents: structured proposals for non-trivial changes to architecture, protocols, tooling, or process.

An RFC is the artifact that carries a proposal through discussion to a decision. It is upstream of an ADR: the RFC is the *proposal* under debate; the ADR is the *decision* that results. This skill is language- and stack-agnostic.

## Skill Use
- Load this skill when the task is to author a new RFC, review a draft RFC, or revise one based on feedback.
- Treat this skill as the governing contract for RFC shape and quality.
- Keep repository- and organization-specific context (RFC process, numbering, review forum, ratification rules) in the invoking prompt.
- When this skill conflicts with casual document habits, follow this skill.

## Tool Use
This skill is tool-agnostic and works with Claude Code, Codex, OpenCode, and similar assistants. Map its guidance to whatever file-reading, editing, search, and shell-execution tools your environment exposes.

- Read existing RFCs before writing a new one. Style, numbering, and the review process live in prior art.
- When the RFC describes current state, verify that state by reading the code, config, or docs — do not describe the system from memory.
- Issue independent tool calls (RFC directory listing, related code reads, prior-art checks) in parallel.
- When citing metrics, incidents, or constraints, confirm them from a source (dashboard, incident review, ticket) rather than paraphrasing.

## RFC vs. ADR vs. Design Doc
- **RFC**: a proposal under active discussion. Lives in Git as a document; evolves through review; terminates in accepted / rejected / withdrawn. The medium of the discussion.
- **ADR**: a decision, recorded. Short, after-the-fact, immutable once accepted. The outcome of the discussion.
- **Design doc**: an engineering design for implementation work already scoped. Usually not up for debate in the same way — it's working documentation for the team building the thing.

A healthy flow: **RFC → discussion → ADR** records the accepted decision, and optionally a **design doc** details the implementation.

Use this skill for RFCs. Use `adr-write` for ADRs.

## When An RFC Is Warranted
An RFC is warranted when the proposal:
- crosses team boundaries or affects shared platforms
- is hard to reverse (data model, protocol, public API, org-wide tooling change)
- has genuine alternatives worth evaluating publicly
- sets policy or convention that others will be expected to follow
- requires alignment before engineering effort is worthwhile

Do **not** write an RFC when:
- the change affects only your team and is inside your ownership
- the decision is already made and you're retrofitting justification (write an ADR instead)
- the question can be answered with a prototype in an afternoon
- it's a routine implementation choice with no alternatives to evaluate

## Required Inputs
The invoking prompt should provide:
- scope and stakeholders
- the problem being solved, in the author's words
- known constraints (deadlines, compliance, platform dependencies)
- prior art: related RFCs, ADRs, incidents, tickets

If the problem statement is vague ("we should improve X"), stop and ask for specifics. Vague RFCs produce vague discussions.

## RFC Lifecycle
Status values, in order. Every RFC should record its current status at the top.

- **Draft** — author is still iterating; solicit feedback but expect revision.
- **In Review** — open for broad comment from the designated audience.
- **Last Call** — final period for objections before a decision.
- **Accepted** — proposal is adopted; write the ADR and plan implementation.
- **Rejected** — proposal is declined; record the reason.
- **Withdrawn** — author pulled the proposal (e.g., no longer needed, blocked on external change).
- **Superseded by RFC-NNNN** — accepted earlier, now replaced by a newer proposal.

Rules:
- Status transitions happen in discussion forums (PRs, meetings), not silently in the doc. The doc records the outcome.
- An accepted RFC is immutable for content. Corrections after acceptance happen in a superseding RFC.
- Rejected RFCs stay in the repository. The rejection record is valuable to avoid re-litigating.

## Numbering And File Layout
Before writing, match the organization's convention. Common patterns:

- `rfcs/0042-topology-spread-baseline.md` — zero-padded sequential, kebab-case title.
- `docs/rfcs/2026-04-23-topology-spread-baseline.md` — date-prefixed.
- `rfc/42/README.md` — directory-per-RFC when the RFC has supporting files (diagrams, sample code).

Adopt the existing convention. If none exists, default to `rfcs/NNNN-kebab-title.md` (zero-padded sequential) and document the convention itself in `rfcs/README.md`.

## RFC Format
Use the template below. Every section except the optional ones is required.

```markdown
# RFC-<NNNN>: <Short title>

- **Status**: draft | in review | last call | accepted | rejected | withdrawn | superseded by RFC-NNNN
- **Authors**: <names or handles>
- **Sponsors**: <optional; senior stakeholders endorsing the proposal>
- **Created**: YYYY-MM-DD
- **Last Updated**: YYYY-MM-DD
- **Target Decision Date**: YYYY-MM-DD  (optional; drives urgency)
- **Audience**: <who is expected to review>
- **Related**: RFC-NNNN, ADR-NNNN, issues, incidents

## Summary
<Two to four sentences. If a reader only reads this block, what should they take away?>

## Motivation
<Why does this matter? What problem are we solving? What is broken, missing, or slow today?
Ground in concrete evidence: incidents, metrics, customer reports, engineering friction. Avoid aspiration.>

## Goals
- <Observable outcome 1>
- <Observable outcome 2>

## Non-Goals
- <Explicitly out of scope>
- <What this RFC is deliberately not solving>

## Current State
<How does the system work today, grounded in code/config? What are the constraints?
A reader without context should finish this section understanding what exists.>

## Proposal
<The core of the RFC. Describe what you propose in concrete terms.
Use diagrams, code sketches, configuration examples, or interface definitions where they help.
Be specific enough that a reader can judge whether this will work; not so specific that you've written the implementation.>

### <Subsection as needed>
<Structure the proposal with headings that match its shape: "API", "Data Model", "Rollout", "Observability", etc.>

## Alternatives Considered
<At least two. For each:>

### <Alternative A>
- <How it would work>
- **Pros**: <…>
- **Cons**: <…>
- **Why not chosen**: <…>

### <Alternative B>
<…>

### Do Nothing
- What happens if we keep the status quo?
- When does "do nothing" become untenable?

## Impact
- **Users**: <who is affected, how>
- **Operators**: <on-call, deployments, monitoring>
- **Compatibility**: <backward/forward compatibility, deprecation plan>
- **Security**: <threat surface changes, auth/authz implications, compliance>
- **Performance**: <expected change, measured if possible>
- **Cost**: <infrastructure, licensing, engineering effort>

## Rollout Plan
<If accepted, how does this land? Phases, gates, feature flags, migration steps.
If the rollout is complex, decompose — or note that a separate design doc will follow.>

### Reversibility
<Can this be rolled back? At what cost? What's the point of no return, if any?>

## Open Questions
- <Questions that the author explicitly wants discussion on>
- <Known unknowns that should be resolved before acceptance>

## Review Plan
- **Review period**: <YYYY-MM-DD to YYYY-MM-DD>
- **Forum**: <PR, meeting, Slack channel>
- **Required reviewers**: <names or roles>
- **Decision mechanism**: <consensus, designated decider, steering committee>

## Prior Art
<Links to related work: internal RFCs/ADRs, external RFCs, industry practice, research.
Grounds the proposal in existing thinking; shows you've looked.>

## Appendix
<Optional. Diagrams, spike code, benchmark data, survey results. Anything that supports the body without cluttering it.>
```

## Writing Rules
- Lead with the summary. Readers decide in 30 seconds whether to read the rest.
- **Motivation is a forcing function, not a wish.** "We should adopt X" is not motivation. "Incidents 2026-01-12 and 2026-02-04 both stemmed from Y, and X closes that class of issue" is.
- State the proposal before the alternatives. Readers want to know what you want to do before they evaluate the menu.
- At least two real alternatives. Straw-man alternatives to make the chosen option look good are worse than none.
- "Do nothing" is an alternative. If you can't articulate the cost of not acting, the motivation isn't strong enough.
- Include honest negatives. Every real proposal has costs; an RFC that only lists benefits will not survive review.
- Be specific about rollout. Vague rollout plans hide the hard parts.
- Write for the reviewer, not for yourself. Ambient context you have in your head has to be on the page.

## Review Rules
When reviewing a draft RFC, check in this order:
1. **Summary clarity** — can a reader outside your team explain the proposal in one sentence after the summary?
2. **Motivation grounding** — is the problem statement anchored in specific incidents, metrics, or constraints?
3. **Current state accuracy** — does the "current state" section match the code, or is it aspirational?
4. **Proposal specificity** — is the proposal concrete enough to judge, or is it hand-wavy?
5. **Real alternatives** — are the alternatives honest and fairly treated, including "do nothing"?
6. **Impact coverage** — are security, compatibility, operations, and cost addressed, not glossed?
7. **Reversibility** — is the point of no return identified?
8. **Open questions** — are there questions listed, or is the RFC pretending everything is answered?
9. **Style consistency** — matches organization conventions (numbering, headings, file layout).

Prose and formatting come last.

## Revising
- Keep revisions traceable. PR-per-revision is the lightest pattern; dedicated `revision history` sections in the doc also work.
- Respond to comments inline or in a `## Resolutions` section when they've produced substantive changes. Reviewers should see their concerns addressed, accepted, or rejected with reasons.
- Do not silently rewrite the RFC between discussions. A revision that invalidates prior comments should re-open the review period.
- When the proposal fundamentally changes, consider withdrawing and opening a new RFC rather than contorting the original.

## Relationship To ADR
- When an RFC is **accepted**, write an ADR (use `adr-write`) that captures the decision concisely. The ADR points to the RFC for context; the RFC points to the ADR for resolution.
- An RFC answers "should we do this, and how?" An ADR records "we did this, here's the decision." Both live in the repo.
- A small proposal with obvious alternatives can skip the RFC and go straight to an ADR. A large proposal without alternatives worth evaluating can skip the RFC and go straight to a design doc.

## Anti-Patterns To Reject
- RFCs that don't state the problem, only the solution
- RFCs with one "considered option" (the author's) and two strawmen
- RFCs that present aspiration as current state
- RFCs with all positive consequences and no honest costs
- RFCs written after the decision was made, used to justify rather than explore
- RFCs that are really design docs (implementation detail dressed up as proposal)
- RFCs that are really ADRs (short statement of fait accompli)
- Open-ended RFCs with no review plan or target decision date
- RFCs that paraphrase Slack threads without distillation
- Multi-topic RFCs that mix unrelated proposals; split them
- RFCs that skip "do nothing" as an alternative
- Revising an accepted RFC in place rather than superseding it

## Quality Checklist
Before considering an RFC ready for review, verify:
- status, authors, and dates are present
- summary is tight and stands alone
- motivation cites concrete evidence
- current state is verifiable
- proposal is specific enough to judge feasibility
- at least two real alternatives plus "do nothing"
- impact addresses security, compatibility, operations, cost
- rollout plan has phases and a reversibility note
- open questions are listed
- review plan has dates, forum, reviewers, and a decision mechanism
- prior art is linked
- the file name and heading match the organization's convention

## Invocation Template
Use this skill with a prompt that supplies context. Example:

```text
Use RFC Mode.
Draft RFC-0043 in /path/to/repo/rfcs/ proposing a shared Kubernetes topology-spread baseline for all platform workloads.
Ground motivation in the node-drain incident from 2026-02-14 and the multi-zone outage on 2026-03-02.
Current state: each team defines their own spread (or none). Proposal: a platform-enforced Kyverno policy plus a shared Kustomize component.
Consider at least three alternatives: Kyverno policy, admission webhook in platform controller, advisory guideline without enforcement.
Target decision date: 2026-05-15. Review in the Platform Architecture channel.
```
