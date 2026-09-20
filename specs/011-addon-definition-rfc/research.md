<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Research: Complete AddonDefinition Design RFC

Research date: 2026-09-19. Repository baseline:
`92c5d8bc44df72596703e92652be9df20cfeb333`.

These are planning decisions and proposed starting positions for revising
[RFC 0001](../../docs/rfc/0001-addondefinition-crd.md). They do not approve the
RFC or establish implemented runtime behavior. Independent read-only research
covered authorization and lifecycle/versioning; the coordinator consolidated
and checked the evidence. Graphify provided navigation; current source files
were authoritative. No cluster tests were run.

## R1. Finish the existing RFC through an evidenced decision

**Decision**: Revise RFC 0001, record its designated decider's acceptance, merge
it through a PR, and update existing #280. Use one decision record per principal
question inside the RFC, not six competing RFCs. Record the accepted architecture
in a new ADR; preserve ADR 0001. Recommend a narrowly scoped supersession of
ADR 0001's startup-only loading decision while retaining its in-process contract.

**Rationale**: The user explicitly requires approval, merge and handoff.
[ADR 0001](../../docs/adr/0001-in-process-adapter-contract.md) chose static boot
registration while anticipating future loaders. The RFC already names Shawn
Stratton as decider and [.github/CODEOWNERS](../../.github/CODEOWNERS) names
`@mfacenet`. A merge timestamp alone is not evidence of design acceptance.

**Alternatives considered**: A review-ready draft does not meet the clarified
scope; a duplicate RFC or epic fragments the decision; silently editing an
accepted ADR violates governance. The ADR's exact supersession scope is checked
against the final accepted proposal.

## R2. Treat the permission boundary as a design obligation

**Decision**: Use separately applied, administrator-owned grants and an explicit
binding between definition identity and evaluation identity as the recommended
starting position. Runtime definitions must fail closed when that binding or
scoped client is unavailable. Compare all four grant models in the RFC, including
operator-created grants, rather than claiming there is only one possible model.

**Evidence**:

- [adapterClient](../../internal/controller/addoncheck_controller.go), lines
  442–476, selects a uniquely labeled ServiceAccount; nil client factory and
  empty-namespace local execution can use the broader controller client.
- [factory.go](../../internal/adapter/impersonation/factory.go), lines 57–86,
  creates an uncached impersonating client but shares the manager's REST mapper.
  Consequently API discovery/REST mapping is not the same path as impersonated
  object reads. EndpointSlice reads are object reads, not API discovery.
- [rbacgen.go](../../internal/adapter/rbacgen/rbacgen.go), lines 274–326, generates
  per-addon grants and a named ServiceAccount impersonation allowlist. That
  allowlist alone does not bind an arbitrary new definition to the identity.
- [role.yaml](../../config/rbac/role.yaml) contains namespaced Role/RoleBinding
  write permissions for existing operator functions. The RFC must inspect actual
  grants rather than state that the operator holds no RBAC writes at all.

**Rationale**: Impersonation permission authorizes the operator to use an identity;
by itself it does not show which definition an administrator approved to use it.
Kubernetes can scope ServiceAccount impersonation grants by namespace/name, while
the impersonated account may have wider access. This follows the documented
[impersonation model](https://kubernetes.io/docs/reference/access-authn-authz/user-impersonation/).
The definition-binding requirement is an inference for Fathom's proposed model.

**Alternatives considered**: A pre-granted common permission set simplifies
installation but broadens sharing; automatic grants require a separate escalation
analysis; publisher trust addresses provenance but does not replace access
control. Keep these as real tradeoffs, including their operational costs.

**RFC work required**: Specify who can author/update definitions and who can
change bindings; bind the exact canonical identity without borrowing another
allowlisted account. Cover identity recreation and revocation. To satisfy FR-005,
recommend identity-scoped discovery for runtime definitions as well as object
reads; retaining shared operator discovery would require an explicit spec change,
not an undocumented exception. Never inherit a local administrator kubeconfig.

## R3. Separate requested permissions, effective access and diagnostics

**Decision**: Treat declared rules as requested capabilities/diagnostic input,
not an effective authorization ceiling. If access reviews are retained, specify
who performs them and how their permission is installed independently of metrics.

**Evidence**: [metrics-rbac.yaml](../../deploy/helm/fathom-operator/templates/metrics-rbac.yaml)
conditionally installs access-review permissions only with RBAC creation and
secure enabled metrics. [metrics_auth_role.yaml](../../config/rbac/metrics_auth_role.yaml)
is separate from the manager role. The proposed controller diagnostics are not
implemented by the current adapter path.

**Rationale**: [RBAC permissions are additive](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#role-and-clusterrole).
Other bindings may increase an account's access. An access review answers an
asserted authorization question; it does not prove complete declarations, minimal
privilege, future authorization, or administrator approval of a definition.

**Alternatives considered**: Calling access reviews equivalent to the generator's
justification guard is inaccurate. Omitting proactive diagnostics avoids a new
permission dependency but leaves actual request failures as the feedback path;
the RFC must choose one and define denied/error/unknown outcomes.

## R4. Make lifecycle and evidence coherence explicit

**Decision**: Require a lifecycle table covering every spec event. Recommend
immutable definition snapshots identified by object UID and generation, a
publication check against the active revision, and owner-aware deletion. Treat
authorization as a separately changing context, not a transactionally frozen
RBAC snapshot.

**Evidence**:

- [registry.go](../../internal/adapter/registry/registry.go), lines 64–121,
  supports registration/lookup, not replacement/removal. Re-registering the same
  adapter name can be a no-op. Runtime edits need a new lifecycle contract.
- [definition.go](../../internal/adapter/declarative/definition.go), lines 79–86,
  documents shared slices and requires callers not to mutate them.
- [addoncheck_controller.go](../../internal/controller/addoncheck_controller.go),
  lines 231–239 and 268–315, preserves prior result/time on missing lookup but
  supplies no periodic missing-adapter retry. Setup currently watches AddonCheck.
  Authorization failures in the run path become Error results.
- [healthreport_types.go](../../api/v1alpha1/healthreport_types.go), lines 111–150,
  records adapter identity/version but has no runtime definition revision field.

**Rationale**: A deleted/recreated object can have the same name and a new UID.
Old evaluations must not publish as fresh evidence for a new definition. Prior
verdicts must remain readable, and loss of inputs must not refresh success time.
Live authorization can change between individual reads; the RFC must describe
those limits honestly rather than promise atomic revocation across a whole run.

**Alternatives considered**: Mutable shared instances mix revisions; retaining
old adapters indefinitely ignores deletion; resetting all prior evidence loses
inspectability. Publication fencing must state what happens to superseded work,
not claim it retroactively prevents an already-authorized read.

## R5. Separate local validation from global identity arbitration

**Decision**: Require explicit enforcement points for schema checks, runtime
identity conflicts, and built-in collisions introduced by upgrades. Recommend
one exact canonical addon identity with no hidden alias normalization. Compare
name-based uniqueness, controller arbitration and admission with external state.

**Rationale and evidence**: Registry conflict detection compares adapter names;
engines derive their name from addonType. Two definition objects can therefore
claim the same name without current registration providing safe arbitration.
[CRD validation rules](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#validation-rules)
operate on the current object; they cannot consult another definition or the
operator's compiled registry. The RFC's admission-rejection claim needs a concrete
enforcement mechanism and an upgrade-time backstop.

**Alternatives considered**: Schema validation alone cannot establish global
uniqueness; implicit last-writer-wins makes restarts and concurrent updates
unpredictable; silent built-in takeover hides a change in health semantics.

## R6. Bound evaluated work, not just definition size

**Decision**: Require a limits inventory with numeric values, enforcement points,
overflow outcomes and rationale before the RFC can be accepted. Cover strings,
maps, collections, object/list pages, result/report volume, concurrent work,
retries and elapsed time. Do not infer safety from timeout alone.

**Evidence**: [condition.go](../../internal/adapter/declarative/condition.go),
lines 83–105, issues lists without pagination limits and collects returned items.
The field, annotation and pod-projection evaluators also iterate lists.
[engine.go](../../internal/adapter/declarative/engine.go) uses typed evaluators;
the definition vocabulary is not a user-supplied expression language.
[FamilyDefinition](../../internal/adapter/declarative/definition.go), lines
203–251, has ten buckets because APIService checks reuse ConditionCheck; nine
underlying evaluator types does not alone define the future wire vocabulary.

**Rationale**: The RFC's 16-family/32-check examples do not limit target volume
or report size. Admission CEL budgets bound schema validation, not later runtime
list calls. Kubernetes recommends bounded collection/string inputs to validation
rules; see [resource use by validation functions](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#resource-use-by-validation-functions).

**Alternatives considered**: Unbounded lists with a timeout can still allocate
large responses; treating admission CEL as a runtime sandbox conflates two
systems. Recommend no new user expression language in the first release and
explicitly describe APIService mapping in the schema proposal.

## R7. Keep version tracks and follow-on scope separate

**Decision**: Recommend a new alpha schema track independent of the established
Go adapter contract; keep builtin loading available in the first release.
Record #256 as a separate change with an explicit dependency or deferral, not as
implemented by merging this RFC. The existing proposal recommends 1.1.0 plus
release documentation; verify and record the compatibility consequences.

**Evidence**: [version.go](../../pkg/adapter/version.go), lines 14–74, still
uses 1.0.0 and accepts same-major older/equal minor adapters.
[ratio.go](../../pkg/adapter/ratio.go), lines 13–29, reserves `warnRatio` and
`failRatio`. Merely accepting an older version does not prove that reserving a
previously user-owned key preserves its semantics.

**Rationale**: Schema structure, evaluator semantics, addon release versions and
host interface compatibility are related but distinct. The versioning decision
must cover semantic changes, not only wire fields.

**Alternatives considered**: One version for all contracts couples independent
changes; declaring #256 closed without its code/docs changes is misleading.
For first-release scope, compare cluster versus namespace scope and document
administrator ownership; compare documentation-only authoring guidance with CLI
rendering and generated samples. These are required RFC decisions, not permission
to build those tools during this feature.

## Research Resolution

All planning unknowns are resolved: artifact scope, evidence sources, validation
method, reviewer/decider provenance, approval/merge criteria and handoff destination.
The RFC authoring work still must turn the recommendations into six complete
proposals, concrete limits, and a decision on each listed alternative. That is
the planned deliverable, not an unresolved prerequisite to planning. No approval,
merge, epic update or runtime verification is claimed by this research.

## Execution refresh — 2026-09-20

HEAD `103e1a5` has no changes to `internal/`, `api/`, `pkg/`, or `config/`
relative to the research baseline `92c5d8b`. Re-read the registry, client factory,
reconciler fallback/requeue paths, definition vocabulary, ratio/version contracts,
manager role and conditional metrics grants. The findings above still apply.
Kubernetes primary references above were reopened: ServiceAccount impersonation
is namespace-scoped, effective RBAC is additive, and admission CEL has its own
cost budgets. These do not supply runtime evaluation bounds or an atomic
cross-object revocation guarantee.

Issue #256 remains open. Its proposed minor bump explicitly keeps 1.0 adapters
loadable; it cannot by itself detect an adapter's conflicting ratio semantics.
No PR existed for `docs/addon-definition-rfc` at the start of execution.
The proposed binding resource, validation limits and lifecycle below are new
RFC recommendations, not behavior verified in the existing implementation.
