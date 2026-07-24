<!--
Sync Impact Report
- Version change: (unfilled template) → 1.0.0
- Source: derived from skaphos-resources/standards/constitution.md v1.0.0 (canonical upstream)
- Modified principles: all nine template placeholders replaced with the upstream Skaphos
  principles (I–IX), each kept intact per the upstream Governance derivation rule, with
  Fathom-specific application notes added where this repository already encodes them
- Added sections: Fathom-Specific Constraints; Engineering Constraints;
  Development Workflow & Quality Gates; Specification and Decision Workflow
- Removed sections: none (template placeholder comments removed)
- Templates requiring updates:
  - .specify/templates/plan-template.md ✅ no change needed (Constitution Check gate is
    derived from this file at plan time)
  - .specify/templates/spec-template.md ✅ no constitution references, no change needed
  - .specify/templates/tasks-template.md ✅ no constitution references, no change needed
  - .specify/templates/checklist-template.md ✅ no constitution references, no change needed
- Follow-up TODOs: none
-->

# Fathom Constitution

Fathom is a Skaphos tool. This constitution is **derived from the canonical
organization-level upstream** at
`skaphos-resources/standards/constitution.md` (v1.0.0), per its Governance
section: it restates the upstream principles without weakening or
contradicting them, and adds Fathom-specific constraints. When the upstream
changes, this document is re-synced — propose suite-wide changes upstream
first, mirror here second.

Fathom is the Skaphos health primitive: a Kubernetes operator that reconciles
`HealthCheck` and `ClusterHealth` custom resources and persists `HealthReport`
history.

---

## Core Principles

### I. Explicit State Over Implicit Behavior

Operational concepts MUST be first-class declared primitives — durable objects
with lifecycle, status, policy, and history — not UI labels, wiki sections,
spreadsheet columns, or tribal knowledge. Systems MUST declare and enforce
desired state; behavior that depends on undocumented assumptions is a defect.

*Rationale: a platform is a system that encodes operational intent and
continuously reconciles reality toward it. Intent that is not explicit cannot
be enforced, explained, or recovered.*

**Fathom application**: health intent lives in `HealthCheck` and
`ClusterHealth` CRDs; observed health lives in their `status` and in
`HealthReport` history. Health semantics MUST NOT be encoded in annotations,
naming conventions, or out-of-band configuration.

### II. Git Is the Durable Desired-State Boundary

Every byte of intended platform and application state MUST be derivable from a
commit in a repository the organization controls. Intended state MUST
round-trip through Git before it becomes normal desired state. Parameter
overrides, invisible UI mutations, and edit-live-resource workflows that cannot
be reconstructed later are forbidden. Break-glass paths MUST exist, and MUST be
explicit, auditable, attributable, and reconciled back to Git. No tool may
become an invisible second source of truth.

*Rationale: this keeps recovery simple, audit grounded, and control-plane
behavior explainable. Operators must be able to recover with Git and a CLI.*

**Fathom application**: Fathom's CRDs, RBAC, and deployment manifests are
rendered from this repository via pinned tooling (`config/`, kustomize, OLM
bundle tasks) and MUST remain compatible with GitOps delivery.

### III. Deterministic, Reconstructible Operation

Systems MUST behave predictably and be reconstructible from declared
configuration. Rendering and compilation steps MUST be deterministic: the same
inputs produce the same outputs. Versioned artifacts are immutable and
referenced by digest where the ecosystem allows; identity-defining components
are replaced, not mutated in place.

*Rationale: determinism is what makes reconciliation trustworthy, drift
detectable, and disaster recovery a procedure instead of an archaeology
project.*

**Fathom application**: generated artifacts (`zz_generated*.go`, CRD and OLM
manifests under `config/`) are produced only by the pinned task wrappers and
are never hand-edited. Runtime configuration resolves deterministically:
flag → environment variable → config file → built-in default.

### IV. Kubernetes-Native, Never Obscured

Skaphos tools MUST integrate with Kubernetes primitives directly — CRDs,
controllers, reconciliation, status, events, ownership, admission — preferring
operators and controller-runtime over external orchestration where
appropriate. Higher-level APIs MUST preserve the control-plane model rather
than bypass it. Tools MUST NOT obscure or hide Kubernetes behavior; they
clarify and enforce correct operation. Kubernetes-native does not mean
Kubernetes-only UX.

*Rationale: Kubernetes is a control-plane substrate, not a hosting product.
Hiding the API machinery discards the most useful part of the system.*

**Fathom application**: Fathom is a kubebuilder/controller-runtime operator.
Health state is surfaced through CRD `status` and Kubernetes events, not a
side-channel API.

### V. Compose, Don't Trap

Each tool MUST do one important operational job well, expose its state
clearly, and compose with other tools through durable APIs, files, events, or
Git. Every tool MUST be independently adoptable and provide concrete value
standalone. Cross-tool integration points are optional and one-directional:
a primitive (Fathom, Tack, Anchor, Tropis, Exartia) MUST NOT depend on the
control planes that consume it. New hard dependencies between Skaphos tools
require an explicit, documented decision.

*Rationale: Skaphos is an ecosystem of focused platform-control tools built
around one operating model — not a monolith, and not a trap.*

**Fathom application**: Fathom is a primitive. It MUST remain independently
adoptable and MUST NOT take dependencies on the Skaphos control planes that
consume its health signal.

### VI. Explainable Reconciliation, Evidence-Grade Audit

For every sync, promotion, policy decision, or mutation, the system MUST be
able to show: input state, desired-state commit, rendered output, policy
result, actor, target, action taken, observed result, and failure reason.
"Failed" without a reason and a next safe action is a defect. Audit events
MUST be structured, durable, correlated, and emitted by the same system that
made the decision — never reconstructed from log scraping. Policy evaluation
MUST produce evidence.

*Rationale: a control plane that cannot explain its decisions is not
trustworthy, and audit bolted on after the fact is a compliance garnish.*

**Fathom application**: an unhealthy verdict MUST carry its reason. Health
evaluations record what was probed, what was observed, and why the result
followed; `HealthReport` history is the durable evidence trail.

### VII. Read-Only Degradation Over Blindness

When mutation paths are degraded — Git mutation, sync, promotion, or policy
engines unavailable — operators MUST still be able to inspect inventory,
topology, desired state, live state, health, drift, and audit history.
Designs MUST fail toward read-only, never toward blindness.

*Rationale: read-only degradation is a feature; blindness during failure is an
architectural bug.*

**Fathom application**: last-known health status and report history MUST
remain readable even when checks cannot currently execute; a probe failure is
itself reportable state, never a blank.

### VIII. Topology Is Deployment State

Tools that model delivery, policy, health, or audit MUST treat topology —
environment, region, cell, cluster type, failure domain, ingress path, blast
radius — as part of deployment state, encoded in the data model, not
reconstructed from labels or convention.

*Rationale: a platform that cannot model where something runs cannot safely
answer basic operational questions.*

### IX. Technical Precision, Honest Scope

Documentation and specifications MUST describe actual, verified behavior — not
intent or aspiration — and MUST state plainly what a tool is *not* and what
its known limitations are. Marketing language and exaggerated claims are
forbidden in all repository content. Where Skaphos is deliberately
unopinionated (e.g., cluster content), tools say so rather than implying
coverage.

*Rationale: operational credibility is the product. A tool that overclaims is
worse than a tool that does less.*

---

## Fathom-Specific Constraints

Constraints this repository adds on top of the upstream principles. They MUST
NOT be weakened by feature work; a spec or plan that needs an exception
documents it in Complexity Tracking.

- **`ClusterHealth` contract stability**: the `ClusterHealth` external
  contract is derived only from `HealthCheck.status` — never from
  `HealthReport` history. Consumers depend on this; changing it is a breaking
  API change.
- **Bounded, idempotent reconciliation**: reconciler logic MUST be idempotent
  and bounded. `spec.timeout` on health checks is honored, and no unbounded
  work runs inside a `Reconcile` loop.
- **Minimal RBAC**: no cluster-wide permissions beyond what the operator
  strictly needs. Every new permission enters via `+kubebuilder:rbac` markers
  and materializes under `config/rbac/` through the manifest tasks.
- **Configuration model**: cobra + viper with precedence
  flag → `FATHOM_*` env var → config file → default. New options extend
  `Options` and the `bindings()` table in `internal/app/options.go` so flag,
  viper key, env var, and config-file key stay in sync.

## Engineering Constraints

The upstream standards in `skaphos-resources/standards/` are normative for
this repository; this section indexes how they land here.

- **Stack**: Go (version per `go.mod`), kubebuilder v4 scaffolding,
  controller-runtime, cobra + viper. Tooling versions are pinned in `tools/`
  and invoked only via `go -C tools tool task …`.
- **CRD API versioning**: per `crd-api-versioning-standard.md`. The version
  string is a compatibility contract; the maturity ladder is a ratchet, and
  in-place breaking changes to beta or GA versions are forbidden.
- **Documentation**: per `documentation-standard.md`. Generated references are
  drift-gated, never hand-written; hard-to-reverse decisions get immutable
  ADRs.
- **Repository governance**: per `repository-governance.md`. All changes land
  by pull request; commits carry DCO sign-off (cryptographic signing
  encouraged); CI (`lint`, `test`, `staticcheck`, `vuln`, coverage gate,
  `reuse lint`) is a required gate.
- **Licensing**: every source file carries the SPDX header
  (`hack/boilerplate.go.txt` or REUSE equivalent).

## Development Workflow & Quality Gates

- New behavior ships with direct test coverage; bug fixes add a regression
  test that fails before the fix.
- The coverage gate (`scripts/check-coverage.sh`) only ratchets upward —
  thresholds are never lowered to make a PR pass.
- Injection seams (`managerFactory`, `Setupper`) exist so unit tests avoid
  envtest; envtest and Ginkgo e2e suites cover the rest.
- Conventional Commits govern anything landing on `main` so release-please
  can infer versions; branch names carry change-type prefixes.
- PRs are focused (one logical change), and state the exact checks run with
  their outcomes.

## Specification and Decision Workflow

- `skaphos-resources` is the canonical upstream for suite-level context:
  `FACTS.md`, `PROPOSAL.md`, `DECISIONS.md` and ADRs, `tools/BUILD-PLAN.md`,
  and `tools/ECOSYSTEM.md`.
- Feature work follows the spec-driven flow (specify → plan → tasks) and is
  gated against this constitution. Specs cite the relevant `FACTS.md` /
  `ECOSYSTEM.md` findings rather than re-researching settled questions, and
  MUST NOT contradict an accepted ADR without proposing its supersession.
- **Adopt before build**: where `ECOSYSTEM.md` records mature prior art, a
  plan that builds instead of adopting MUST document why the verdict does not
  apply.
- Hard-to-reverse decisions get an ADR; ADRs are immutable and superseded,
  never rewritten.

## Governance

This document is derived from the Skaphos constitution
(`skaphos-resources/standards/constitution.md`, currently v1.0.0). It may add
Fathom-specific principles and constraints and MUST NOT weaken or contradict
the upstream. When the upstream changes, this file is re-synced; suite-wide
changes are proposed upstream first and mirrored here second.

**Amendment**: Fathom-specific amendments land by pull request against this
file with rationale in the PR description. Version semantics: MAJOR for
removing or redefining a principle, MINOR for adding a principle or section,
PATCH for clarifications that change no requirement. An upstream re-sync bumps
this version according to what the sync changes here.

**Compliance**: specs and plans are gated against this constitution. A
deviation is either (a) justified in writing in the plan's Complexity
Tracking, or (b) a proposed amendment — silent divergence is not an option.
Where this derivation drifts from the upstream, this document is the bug and
gets fixed first.

**Version**: 1.0.0 | **Ratified**: 2026-07-23 | **Last Amended**: 2026-07-23
