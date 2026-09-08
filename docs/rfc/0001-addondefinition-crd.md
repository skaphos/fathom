<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# RFC 1. AddonDefinition as a CRD — make adapters installable, not compiled in

- **Status**: proposed
- **Date**: 2026-09-08
- **Deciders**: Shawn Stratton
- **Issue**: [#278](https://github.com/skaphos/fathom/issues/278)
- **Implementation**: [#280](https://github.com/skaphos/fathom/issues/280)

## Summary

Fathom's adapters are already **data** — an `AddonDefinition` value plus an
evaluator vocabulary — but that data is compiled into the operator binary. An
adopter who needs an adapter Fathom does not ship must fork, write Go, rebuild,
and maintain a downstream image. This RFC decides how a definition becomes a
cluster resource.

The central finding is that **the hard problem is already solved.** Fathom does
not read addon resources with its own identity; it impersonates a per-addon
ServiceAccount, and the operator's impersonate grant is a namespaced Role scoped
by `resourceNames` to an explicit list. A runtime definition therefore cannot
obtain any permission the cluster administrator did not already grant, with no
new code and no new trust decision. That collapses the RBAC design space to one
viable option and makes the rest of this RFC mostly schema and lifecycle work.

## Decisions

| # | Question | Decision |
| --- | --- | --- |
| 1 | Schema | Discriminated union over the existing 9 evaluator kinds; unknown kinds rejected at admission. |
| 2 | RBAC | Two-step admin install. The operator never mints RBAC. `spec.rbac` is advisory only. |
| 3 | Contract versioning | Decouple from `ContractVersion`. The CRD's `apiVersion` is the schema contract. #256 unblocked immediately. |
| 4 | Precedence | A cluster definition may not claim a compiled-in `addonType`. Rejected at admission. |
| 5 | Lifecycle | `ErrNotFound` becomes retryable; a deleted definition **freezes** the verdict. |
| 6 | Failure modes | Bounded definition size, CEL cost limits, and a per-definition share of `spec.timeout`. |

## Context

The v2 declarative pivot (`docs/design/addon-adapters-implementation-plan.md`)
made adapters data. `internal/adapter/declarative/definition.go` defines
`AddonDefinition` with nine evaluator kinds — `WorkloadCheck`, `CRDCheck`,
`ConditionCheck`, `FieldCheck`, `WebhookCheck`, `CronJobCheck`,
`ConfigMapCheck`, `AnnotationStalenessCheck`, `PodProjectionCheck` — and
`internal/adapter/registry` indexes them by addon type.

What did not change is the loading model. From the registry package doc:

> The current loading model is in-process and explicit: Fathom's manager startup
> constructs a `Registry` and calls `Register` for each compiled-in adapter. A
> future out-of-process loader can register adapters against the same Registry
> without changing this package's external API.

The seam is anticipated; only the loader is missing. A "default pack" today is
not a pack you apply — it is a build you consume.

## 1. Schema

`AddonDefinition` is authored at **`v1alpha1`** on its own version track,
consistent with the DNSCheck and NodeHealthCheck decisions and independent of
[#149](https://github.com/skaphos/fathom/issues/149), which promotes only the
four established kinds. A brand-new public extension contract should not be
frozen at `v1` with no field experience behind it.

The Go struct is the starting point, translated to a CRD as follows:

- `spec.addonType`, `spec.adapterVersion`, `spec.optional`,
  `spec.supportedVersions`, `spec.versionSource` map across directly.
- `spec.families[]` carries the existing `FamilyDefinition` shape.
- Each check within a family becomes a **discriminated union**: a required
  `kind` field enumerated to the nine evaluator kinds, plus exactly one matching
  payload field. CEL validation enforces that the payload matches `kind`.
- Every unbounded collection gets a `maxItems`, and every string a `maxLength`
  (see §6).

**Unknown evaluator kinds are rejected at admission**, not tolerated for forward
compatibility. Tolerating them would mean a definition that silently evaluates
less than it claims — the failure mode a health product can least afford. An
older operator meeting a newer definition should fail loudly and visibly.

The `kind` enum is closed in v1: cluster definitions compose the existing
vocabulary, they do not extend it. Adding an evaluator kind remains an in-tree
change with a `ContractVersion` bump.

## 2. RBAC — the escalation analysis

### What the operator holds today

Three facts, verified in the tree:

1. **`config/rbac/role.yaml` grants the operator no read on any addon-owned
   resource.** It holds core plumbing only — `configmaps`, `serviceaccounts`,
   `pods`, `events`, `validatingadmissionpolicies`, `daemonsets` — plus its own
   `fathom.skaphos.io` kinds. There is no `cert-manager.io`, no
   `networking.istio.io`, no `keda.sh`.
2. **Addon reads happen exclusively through impersonation.**
   `AddonDefinition.RBAC []adapter.PolicyRule` declares what a definition needs;
   `internal/adapter/rbacgen` emits a per-addon read-only ClusterRole and
   ServiceAccount from it; the reconciler impersonates that ServiceAccount when
   it runs the engine. Those rules are the enforced blast radius, not
   documentation.
3. **The operator's impersonate grant is scoped by name.**
   `renderImpersonator` emits a *namespaced* Role in `fathom-system` with
   `resources: [serviceaccounts]`, `verbs: [impersonate]`, and a
   `resourceNames` list — currently the 16 generated addon ServiceAccounts.
   Its own comment: "only these named ServiceAccounts, and only in
   fathom-system."

### Why runtime definitions are already safe

Fact 3 is decisive. A cluster-supplied `AddonDefinition` naming a ServiceAccount
absent from `operator-impersonate.yaml` **cannot be impersonated** — the
operator's Role does not name it, and the API server refuses. A hostile
definition therefore cannot read anything; it fails closed, today, with no new
code.

The escalation analysis in full:

- **Can a definition grant itself permissions?** No. It contains no grant
  mechanism. `spec.rbac` is data the operator reads, never applies.
- **Can a definition borrow an existing powerful ServiceAccount?** No, unless an
  administrator has already added that ServiceAccount to the impersonate Role's
  `resourceNames` — an explicit, auditable, out-of-band act.
- **Can a definition escalate through the operator's own identity?** No. The
  operator holds no addon reads to lend, and the engine runs under the
  impersonated identity, not the manager's.
- **What if impersonation is misconfigured?** The check errors. Fail-closed is
  the correct and existing behavior.

### The decision

**Two-step admin install.** Applying an `AddonDefinition` is inert until an
administrator also applies the ServiceAccount, the read-only ClusterRole, and
the `resourceNames` entry that lets the operator impersonate it. This is
GitOps-friendly and requires no new operator privilege.

This is not chosen as a tradeoff against more convenient options — it is the
only option the existing architecture permits.

**Rejected: the operator generates RBAC in response to an applied CR.** This
would require granting the operator `clusterroles`/`rolebindings` **write**,
which is exactly the privilege-escalation primitive
[#255](https://github.com/skaphos/fathom/issues/255) exists to remove. Fathom
cannot pursue #255 and this option simultaneously. Relying on the API server's
escalation-prevention check is not a rescue: escalation prevention only stops
the operator granting *more* than it holds, so the model still requires the
operator to hold every permission any definition might ever need — the standing
cluster-wide read the impersonation design was built to avoid. **This door is
closed.** Record it here so it is not reopened on convenience grounds.

**Rejected for v1: a signing or trusted-publisher admission gate.** Provenance
machinery earns little when a definition cannot grant itself anything. Revisit
if precedence over in-tree definitions is ever allowed (§4).

### The one real implementation problem

`operator-impersonate.yaml` is *generated* by `rbacgen` from the compiled-in
adapters and is CI-guarded read-only, so an out-of-tree ServiceAccount has
nowhere to be listed. RBAC has no label selectors; `resourceNames` is the only
scoping mechanism.

**Decision: a second, admin-owned, non-generated impersonate Role**
(`addon-impersonator-external`) bound to the same manager ServiceAccount, whose
`resourceNames` the administrator maintains as part of the two-step install. The
generated Role stays untouched and CI-guarded. The operator's impersonate
capability remains an explicit, reviewable list; it simply has two sources.

### `spec.rbac` is advisory

Because a definition's declared rules are self-asserted, `spec.rbac` on the CRD
is a **manifest of what the definition claims to need**, never a grant. Two uses:

- generating the RBAC an administrator should apply (`fathomctl` can render it);
- diagnostics — the controller issues a `SubjectAccessReview` for the named
  ServiceAccount at admission and reports "definition needs X, ServiceAccount
  lacks it" as a status condition. Real feedback, zero granting.

The `UnjustifiedGrants` guard in `rbacgen` — every rule carries a
`Justification`, every write is prefixed — continues to apply to in-tree
definitions. Cluster definitions are outside that CI guard by construction;
the `SubjectAccessReview` diagnostic is their equivalent.

## 3. Contract versioning — and #256

`ContractVersion` (`pkg/adapter/version.go`) versions a **Go interface**. The
CRD's own `apiVersion` (`v1alpha1`) is a **schema** contract, versioned and
evolved by Kubernetes' own machinery and already policed by `task crd-compat`.

**Decision: keep them separate.** Making `ContractVersion` do double duty
conflates two contracts that need to move independently: the Go interface
changes when the adapter *plumbing* changes; the CRD schema changes when the
*evaluator vocabulary* changes.

**Consequence for [#256](https://github.com/skaphos/fathom/issues/256): unblock
it now.** #256 asks whether reserving `warnRatio`/`failRatio` as engine-level
keys warrants a `ContractVersion` bump. Under the decision above that question
is entirely about the Go interface and does not depend on this RFC. Bump
`ContractVersion` to **1.1.0** with a release note recording the reservation;
`EnsureCompatible` still accepts 1.0.0 adapters, so it is safe and low-effort.
It has been waiting on a large RFC for no benefit.

## 4. Precedence and identity

**A cluster `AddonDefinition` may not claim an `addonType` that a compiled-in
definition already claims.** Rejected at admission with a message naming the
conflict.

Rationale: overriding an in-tree adapter is a supply-chain surface — redefine
`cert-manager` to always `Pass` and the health product confidently reports
health it never measured — and there is no v1 user need for it. The v1 story is
*adding* coverage Fathom lacks, not *replacing* coverage it ships.

The in-tree pack **stays compiled in**. #278 floats the alternative of shipping
it as applied CRs so there is exactly one mechanism. That is genuinely more
elegant, and it is deferred: it would turn every install into an RBAC-and-CR
bootstrap of 16 addons, and widen the blast radius of a malformed definition from
"one adopter's addon" to "Fathom does not work." Revisit once the CR path has
field experience.

A later opt-in override (an explicit operator flag, plus the signing gate from
§2) can relax this without a schema change.

## 5. Loading and lifecycle

- **`ErrNotFound` stops being terminal.** Its doc comment says a missing entry
  "will not appear later in the same Fathom process" — true only under the
  startup-registration model. It becomes a retryable condition surfaced as
  `Ready=False/UnknownAddonType`.
- **The registry gains runtime mutation.** `Register` is currently documented as
  startup-only because holding the write lock during reconciliation would block
  dispatch. The loader must therefore build a replacement adapter off the lock
  and swap it in, rather than mutating in place.
- **Watch wiring:** the `AddonCheck` reconciler watches `AddonDefinition` and
  enqueues every check whose `addonType` matches a changed definition.
- **A definition edited underneath a running check** takes effect on the next
  reconcile. Nothing is retroactive; the existing transition-only `HealthReport`
  contract handles a changed verdict normally.
- **A definition deleted underneath a running check freezes the prior verdict**
  and reports the loss on a condition. It does not wipe the verdict to
  `Unknown`.

That last rule is not new doctrine. It is exactly what
[#275](https://github.com/skaphos/fathom/issues/275) (COR-3) establishes for
`NodeCertificateCheck`: losing your inputs is not evidence that the world
changed, and churning a verdict through `Unknown` flaps every mirroring
`HealthCheck` and `ClusterHealth`. **Fathom should have one answer to "I lost my
inputs" across every kind**; this RFC adopts #275's.

## 6. Failure modes

A definition authored by someone who is not a Fathom maintainer must not be able
to wedge the operator, exhaust the API server, or launch unbounded work.

- **Bounded size.** `maxItems` on `spec.families` (16), on checks per family
  (32), and on every nested collection; `maxLength` on every string.
- **Bounded evaluation.** CEL cost limits on any expression-bearing field.
- **Bounded time.** Each definition's evaluation draws from the check's existing
  `spec.timeout` budget; the engine already honors it and must not be given a
  path around it.
- **Bounded blast radius.** A malformed definition fails *that* `AddonCheck`
  only. A definition that fails to compile is rejected at admission; one that
  fails at evaluation produces `Error` for its own check and nothing else.
- **No new capability.** The evaluator vocabulary is closed (§1), so a
  definition cannot introduce a new way to touch the cluster — only a new
  combination of existing read operations, under an impersonated identity that
  bounds them.

## Explicitly out of scope for v1

- Overriding or replacing compiled-in definitions (§4).
- Shipping the in-tree pack as applied CRs (§4).
- New evaluator kinds authored from a CR (§1).
- Operator-generated RBAC in any form (§2).
- Signing / trusted-publisher admission (§2).
- Cross-namespace ServiceAccount impersonation — the impersonate Role is
  namespaced to `fathom-system` and stays that way.

## Consequences

**Good.** Adopters get self-service adapters without forking. The operator takes
on no new privilege — arguably the strongest possible outcome for a feature that
sounded like it required a lot. `rbacgen`'s guard and the impersonation model
keep working unchanged for the in-tree pack.

**Costs.** Installing an out-of-tree adapter is a three-object admin action, not
`kubectl apply -f definition.yaml`. This is real friction and the honest price of
not making the operator a privilege-granting service. `fathomctl` rendering the
required RBAC (§2) is the mitigation, and should be in the v1 scope.

**Risk.** The `addon-impersonator-external` Role is hand-maintained, so it can
drift from the definitions that need it. The `SubjectAccessReview` diagnostic is
what makes that drift visible rather than silent.

## Open questions

1. Does `fathomctl` render the RBAC (`fathomctl addon-definition rbac <name>`),
   or is it documentation only? Recommend the command — it makes the two-step
   install a copy-paste rather than a construction task.
2. Should an `AddonDefinition` be cluster-scoped or namespaced? Leaning
   cluster-scoped: it describes an addon, not a tenant's workload, and the
   ServiceAccount it references lives in `fathom-system` regardless.
3. Does the v1 scope include a conversion path for the 16 in-tree definitions,
   so an adopter can start from one? Recommend yes, as *generated samples* under
   `config/samples/`, without changing how they load.
