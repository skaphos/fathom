<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# RFC 1. AddonDefinition as a CRD — make adapters installable, not compiled in

- **Status**: proposed (in review; not accepted)
- **Author and decider**: Shawn Stratton (`@mfacenet`)
- **Created**: 2026-09-08
- **Last updated**: 2026-09-20
- **Audience**: Fathom maintainers; operator/API and RBAC/security reviewers
- **Related**: [design issue #278](https://github.com/skaphos/fathom/issues/278),
  [implementation epic #280](https://github.com/skaphos/fathom/issues/280),
  [ratio contract #256](https://github.com/skaphos/fathom/issues/256),
  [ADR 0001](../adr/0001-in-process-adapter-contract.md)

## Summary

Allow administrators to install typed addon definitions without rebuilding
Fathom. Keep the built-in pack compiled in, and authorize each runtime definition
through an independently administered binding to a dedicated ServiceAccount.
Every evaluation uses an immutable definition revision and identity-scoped reads;
missing authority or superseded inputs prevent fresh success from being published.
This document proposes the contract. It neither implements nor approves it.

## Motivation and current state

The declarative engine already represents adapters as data. The remaining gap
is loading: adopters still need a Go build to add coverage. Leaving this unchanged
avoids a new API and controller, but preserves downstream image maintenance for
every out-of-tree definition. That is the cost of doing nothing; this RFC makes
no claim about customer counts, incident frequency or measured performance.

Evidence at source baseline `92c5d8b` (unchanged by planning commit `103e1a5`):

- [Definition and families](../../internal/adapter/declarative/definition.go)
  provide nine evaluator types in ten buckets; APIService uses ConditionCheck.
  Shared slices are documented as immutable after engine construction.
- [Registry](../../internal/adapter/registry/registry.go) supports startup
  registration and lookup, not replacement/removal. Registering the same adapter
  name can be a no-op. Missing lookup is documented as terminal.
- [AddonCheck controller](../../internal/controller/addoncheck_controller.go)
  preserves prior results on missing lookup, but does not periodically retry
  adapterless checks. Its client selection can fall back to the manager client
  when the factory is absent or the local namespace is empty.
- [Impersonating client factory](../../internal/adapter/impersonation/factory.go)
  uses uncached object reads but shares the manager REST mapper. Runtime
  definitions therefore need a separate discovery path as well as scoped reads.
- [RBAC generator](../../internal/adapter/rbacgen/rbacgen.go) creates dedicated
  identities and a named impersonation allowlist. This is not authorization for
  an arbitrary definition author to select an allowlisted identity.
- [Manager permissions](../../config/rbac/role.yaml) already include Role and
  RoleBinding writes for existing functions; claiming the operator has no RBAC
  writes is incorrect. This feature must not reuse those writes to grant access.
- [Helm metrics RBAC](../../deploy/helm/fathom-operator/templates/metrics-rbac.yaml)
  conditionally grants SubjectAccessReview creation. A runtime authorization
  diagnostic cannot depend on metrics being enabled.
- [Condition lists](../../internal/adapter/declarative/condition.go) and
  [field lists](../../internal/adapter/declarative/field.go) are not currently
  paginated by a runtime-definition work budget. Timeout alone is insufficient.

## Goals and non-goals

A first-release adopter can author one supported typed definition, install its
explicit authority, and obtain bounded, attributable health evidence without a
custom operator image. Definition loss must leave old evidence readable and aged.
`ClusterHealth` continues to derive exclusively from `HealthCheck.status`.

Excluded: arbitrary code, new evaluator kinds, a new user expression language,
replacement or migration of the built-in pack, operator-created privilege grants,
publisher signing, cross-namespace evaluation identities, and implicit local
kubeconfig execution. Cross-namespace *target reads* remain possible only under
explicit scope and administrator-installed grants. Runtime implementation,
release changes and e2e execution belong to #280, not this documentation change.

## Decision ledger

All six decisions are **Proposed** until the final revision is explicitly accepted.

| Decision | Recommendation | Main tradeoff | Reversibility |
| --- | --- | --- | --- |
| 1. Schema | Cluster-scoped typed alpha definitions; CLI rendering and samples | Smaller vocabulary, additional authoring constraints | Alpha changes still require migration planning |
| 2. Authority | Separate admin-owned UID-bound binding and dedicated SA | Extra resource and reauthorization after recreation | Revoke binding; old evidence remains |
| 3. Versions | Separate schema, evaluator semantics, adapter and addon tracks | More explicit compatibility metadata | Semantic breaks require new track |
| 4. Identity | Name equals addon identity; fail closed on built-in collision | Upgrade may suspend colliding checks | Rename/migrate or roll back operator |
| 5. Lifecycle | Immutable snapshots and publication revalidation | Lost work on edits; no atomic RBAC guarantee | Reconcile valid state again |
| 6. Bounds | Fixed first-release caps and fair runtime scheduling | Large installations may need narrower checks | Revisit caps with measurements |

## 1. Schema — Proposed

Use cluster-scoped `fathom.skaphos.io/v1alpha1` AddonDefinition. A definition
represents platform capability rather than a tenant's workload. A namespaced
alternative offers tenant-local names but complicates cross-namespace AddonCheck
selection and authorization; it is deferred. Cluster scope does not grant read
access to target namespaces.

Required spec fields: `addonType`, `adapterVersion` (SemVer), `semanticsVersion`
(initially `1`), and nonempty ordered `families`. `metadata.name` equals the
canonical `addonType` (§4). `optional` defaults false. `supportedVersions` is
optional but requires a resolvable `versionSource`; that source names a family,
workload component and optional container exactly as the existing engine does.
An ambiguous component reference is invalid. Each family has a unique name,
`defaultEnabled` (default false), and an ordered list of uniquely named checks.

Each check requires `kind` and exactly one corresponding typed payload:

| Kind | Payload semantics |
| --- | --- |
| Workload | Deployment, DaemonSet or StatefulSet singleton |
| CRD | Named CRD and served-version requirements |
| Condition | Named or selected resources and a condition predicate |
| Field | Selected resources and a literal nested scalar path |
| Webhook | Admission webhook configuration and service wiring |
| CronJob | Named job, suspension and recency checks |
| ConfigMap | Named data key, bounded YAML parse and policy-version check |
| AnnotationStaleness | Named/selected resources and timestamp age |
| PodProjection | Selected pods and required injection/projection structure |

APIService is a Condition payload targeting `apiregistration.k8s.io`; it is not a
tenth evaluator. Existing typed payload fields are the starting vocabulary, not
an opaque Go-struct serialization. Every payload must declare target scope and
use the bounds in §6. Resource GVK, names, namespace selection, posture, paths,
version lists and condition predicates must survive deterministic conversion.
Checks run in family order then declared check order; sample generation preserves
the engine's existing bucket order. No embedded code or arbitrary expression field.

OpenAPI plus fixed admission CEL validates the closed union, unique names,
required relationships, syntax and collection limits. Unknown kinds are rejected.
The controller repeats semantic validation (including engine construction and
version-source resolution) before activation. Failed compilation means
`Accepted=False/InvalidDefinition`, not “admitted therefore executable.” We do
not require a new webhook solely to reject engine-level failures at admission.

Include a `fathomctl` rendering command and generated built-in examples in #280's
first release. It renders definition, SA, read roles/bindings, external
impersonation grant and proposed authority binding for administrator review; it
never applies them. Live UID resolution is a separate read-only rendering step
after objects exist. Generated samples teach authoring and use non-built-in
identities; they do not change built-in loading or bypass collision rules.

**Alternatives and costs:** Opaque JSON is easier to extend but weaker to validate;
unknown-kind tolerance risks partial coverage presented as success. Docs-only
installation avoids CLI work but makes binding/RBAC mistakes harder to diagnose.
Typed payloads and authoring aids cost schema, conversion and CLI maintenance.
**Acceptance example:** An unknown tenth kind is rejected; an invalid version
reference cannot activate; an APIService Condition is supported.
**Deferred:** New evaluators, namespace scope and expression languages.
**Evidence:** definition.go, engine construction and §6's expression inventory.

## 2. Authority — Proposed

### Explicit administrator binding

Add a namespaced `AddonDefinitionBinding` alpha resource in the configured
operator namespace (normally `fathom-system`). Its name equals the addon identity.
Its spec contains definition name and UID, ServiceAccount name and UID,
`enabled` (default false), and target scope: an explicit namespace list plus
`allowClusterScoped` (default false). Require at least one target scope. Bindings
are owned by the platform administrator; definition authors get no create/update/
delete permission on bindings, SAs, impersonation grants or underlying RBAC.
The operator reads bindings and writes only their status, never their spec.

A binding authorizes future edits of that exact definition UID under the dedicated
SA's authority; this is delegated read authority to the definition author, not
per-revision content approval. Recreation of either object requires a new UID
binding. Administrators must understand this delegation when installing it.
A binding cannot target the manager SA or any built-in SA. Require a dedicated
external SA; reject runtime bindings sharing an SA UID (mark all conflicting
bindings invalid) so changing one definition cannot borrow another's identity.

The install sequence is declarative: apply definition and dedicated SA; read their
UIDs; review and commit the binding and grant manifests to Git; apply grants and
then enable the binding. The renderer assists both stages. UID discovery makes
installation less convenient but prevents silent inheritance after recreation.
The administrator installs both read RoleBindings/ClusterRoleBindings and a
separate non-generated `addon-impersonator-external` Role/RoleBinding allowing the
manager to impersonate only those named SAs. Generated built-in RBAC is untouched.

The operator gains definition/binding read/watch and status permissions. It does
not gain addon-wide standing reads, new RBAC grant writes, or permission to edit
bindings. These permissions must be generated through the repository tasks.
The cluster administrator is trusted; an actor who can edit all these grants can
intentionally delegate more authority. A definition-author role alone cannot.

### All evaluation paths

| Path | Required identity and restriction |
| --- | --- |
| Discovery and REST mapping | Fresh or identity-keyed discovery under the bound SA; no manager mapper or cross-identity cache |
| Addon objects, CRDs, APIServices | Bound SA; declared GVK/scope and binding target intersection |
| Core reads including pods, ConfigMaps, services and EndpointSlices | Same bound SA and scope filter |
| Version detection, fallback API versions and helper reads | Same transport, request counters and identity; no privileged fallback |
| Definition/binding/SA metadata and own status | Manager control-plane client, never passed into the evaluator |

Clear inherited impersonation headers and use only the canonical bound SA user
identity; do not accept arbitrary groups, extras or user names from definitions.
No token minting is needed. Missing factory, binding, namespace or scoped discovery
fails closed (`Ready=False/AuthorizationUnavailable`). Out-of-cluster runtime
loading requires an explicit operator namespace and the same impersonation path;
otherwise it is disabled. Local administrator credentials never become evaluator
credentials. Built-in behavior is not implicitly changed by this RFC.

`spec.requestedReads` is an optional bounded manifest, not a grant or effective
permission ceiling. Restrict it to get/list and exact resource identifiers plus
explicit discovery URLs; reject wildcards, writes and arbitrary non-resource
URLs. A bounded request wrapper additionally enforces target scope and read-only
operations even if the SA is overprivileged. It does not prove least privilege.
Kubernetes grants are additive; administrators remain responsible for SA grants.

Choose **minimum-request diagnostics**, using actual impersonated request results
and local comparison of declared reads to compiled evaluator needs. Do not add
SubjectAccessReview permission for this feature. No speculative reads are made
just for diagnostics. `PermissionsVerified` is Unknown/NotEvaluated until a run;
403 gives False/AccessDenied, transport failure gives Unknown/AccessCheckUnavailable,
and success confirms only the requests actually made, never the effective union
or future access. Declarations omitted/incomplete produce a diagnostic explaining
that limitation. Diagnostics are status/events and work with metrics disabled.

### Grant model comparison

| Model | Administrator action | Operator privilege delta | Tradeoff and disposition |
| --- | --- | --- | --- |
| Pregranted common permission set | Install shared reader | Impersonate shared identity | Simpler bootstrap, but unrelated authors share a blast radius; reject for first release |
| Separate grants and explicit binding | Install dedicated SA, reads, impersonation and binding | Own-resource read/status only | Selected: auditable isolation, more objects and UID staging |
| Operator-generated bounded grants | Delegate a permission ceiling and RBAC writes | Broad standing ceiling or bind/escalate authority | Easier UX, substantially larger trust boundary; reject |
| Trusted publisher admission | Maintain trust roots and author approvals, plus grants | Admission machinery; still needs an authority model | Provenance is useful but does not grant/bound reads; defer |
| Status quo | Rebuild compiled pack | None | No new trust boundary, but no runtime extensibility |

**Acceptance example:** A definition naming an already allowlisted SA has no
usable authority without its independent matching binding; metrics disabled
changes none of this. **Costs:** another CRD, dedicated identities, and GitOps
UID staging. **Deferred:** publisher signatures, effective-union diagnostics and
cross-namespace identity references. **Evidence:** current fallback/factory/RBAC
paths above; [Kubernetes impersonation](https://kubernetes.io/docs/reference/access-authn-authz/user-impersonation/)
and [additive RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#role-and-clusterrole).

## 3. Versions — Proposed

Keep four independent meanings: `apiVersion` versions schema, `semanticsVersion`
versions evaluator interpretation, `adapterVersion` identifies the author's
adapter release, and `supportedVersions` targets the installed addon's releases.
The existing Go `ContractVersion` still versions the in-process interface.
Runtime definitions must request supported schema and semantics; unsupported
values cannot activate. Do not infer compatibility from adapter release numbers.

Additive schema fields can stay on the alpha track with documented defaults.
Changed verdict semantics require a new semantics value; old semantics remain
available during a documented migration or the upgrade must refuse unsupported
definitions visibly. Removing a schema/semantics version requires inventory,
conversion guidance and rollback planning. An author must bump adapterVersion
when changing behavior, but UID/generation still detects edits without that bump.

**#256 disposition:** Defer the code/release change to a separate #256 PR owned
for coordination by Shawn Stratton, required before #280 runtime loading is
released. That PR must choose/document either the issue's 1.1.0 reservation or
explicit retroactive 1.0 policy, test a synthetic older adapter consuming the
ratio keys, and decide migration/rejection if its semantics conflict. A minor
bump that continues loading 1.0 adapters is not proof of semantic compatibility.
This RFC neither closes #256 nor claims its acceptance work has shipped.

**Alternatives:** One version couples independent contracts; schema-only versioning
misses verdict changes without field changes. Separate tracks cost compatibility
tests and metadata. **Acceptance example:** Unknown semantics cannot execute even
with a supported alpha schema; #280 cannot release while #256's disposition is
unimplemented. **Deferred:** beta/GA promotion and the actual version bump.
**Evidence:** [version.go](../../pkg/adapter/version.go),
[ratio.go](../../pkg/adapter/ratio.go), and the open #256 acceptance criteria.

## 4. Identity and precedence — Proposed

Use an exact lowercase DNS-label addon identity, 1–63 characters. Enforce
`metadata.name == spec.addonType`, with immutable addonType; no aliases,
case folding or trimming. Cluster object-name uniqueness prevents two valid
runtime objects from claiming the same identity. The equality constraint is
admission CEL, while the controller also rejects legacy invalid objects.

Built-in identities are reserved. The controller compares the compiled inventory
before activation and on every startup; local CEL cannot inspect that inventory.
A conflicting object may be stored but is never activated and reports
`Accepted=False/BuiltinCollision`. No claim of global rejection at admission.

During upgrade, a runtime identity newly claimed by a built-in produces a conflict
barrier for dispatch to that identity: neither adapter runs until the administrator
resolves it. This prevents silent reinterpretation of an existing AddonCheck.
Unrelated built-ins remain available. The administrator migrates the runtime
identity/checks or explicitly deletes the runtime definition and binding to use
the built-in. Removing the barrier requeues checks; prior evidence retains its
source revision until a new run completes. CLI inventory must expose the collision
before upgrade; startup enforcement remains mandatory if preflight is skipped.

**Alternatives:** Arbitrary names with controller arbitration support aliases but
need winner selection and more state; stateful admission adds availability and
race concerns and still needs upgrade enforcement. Last-writer-wins is not
reconstructible. Built-in-wins is simpler but can silently change a verdict's
meaning. **Costs:** constrained naming and an upgrade remediation step.
**Acceptance example:** A duplicate canonical create gets AlreadyExists; an
upgrade collision suspends that identity visibly and preserves old evidence.
**Deferred:** built-in overrides and pack migration. **Evidence:** registry
name-based conflict behavior and object-local CRD validation.

## 5. Loading, lifecycle and evidence — Proposed

Build and validate off-lock, then atomically swap immutable snapshots. A runtime
revision is `(definition UID, generation, schema, semantics)`; publication also
records operator build and adapterVersion. Deep-copy payloads. Bind snapshots to
`(binding UID, resourceVersion, SA UID)` independently of definition generation.
This is the observed authority context, not a snapshot of every additive RBAC grant.
Registry removals must match the owner UID so an old delete cannot remove a new
object. Index AddonChecks by addonType; definition/binding/SA changes enqueue them.

Before starting, directly re-read the binding, SA and definition, validate scope
and arbitration, and capture immutable input. Before publication, re-read them
again and compare identifiers, then compare-and-swap the AddonCheck status against
its observed resourceVersion and generation. Reject superseded runs rather than
publishing under newer inputs. Serialize publication per AddonCheck and route all
runtime status/report writes through this gate. Reports carry the same revision;
report-write retries use the accepted run identifier, not a new evaluation time.

**Revocation boundary:** API requests are authorized individually. Revocation
observed by the controller invalidates the snapshot and cancels work; a 403 aborts
the run. A changed binding or SA found at the final check rejects publication.
Kubernetes offers no atomic transaction across binding/RBAC reads and status writes:
revocation not yet observed can race with the final check. Do not claim immediate
linearizable revocation, rollback of already permitted reads, or that a successful
run proves current RBAC. Administrators needing an orderly cutoff disable the
binding and wait for `Ready=False/AuthorizationRevoked` plus zero active runs
before removing grants. Status reports the observed context/time, not a stronger
guarantee. An inability to complete final validation prevents publication.

Retain `lastSuccessfulEvaluation` (verdict, observations, original `observedAt`,
revision and context) separately from `latestAttemptAt` and attempted outcome.
For a new object with no evidence, verdict is Unknown. Valid completed Pass/Warn/
Fail is new health evidence. Denial, timeout, missing input or limit exhaustion
updates attempt/condition state without dating old health evidence as new.
Freshness is Current only when the source is still eligible and its age is at
most two effective intervals plus timeout; otherwise Stale, Superseded or
Unavailable with reason. Use the existing bounded effective interval/timeout,
with the runtime ceiling in §6. No fresh success is inferred from retained Pass.
HealthCheck mirrors readiness/freshness through its status; ClusterHealth still
reads only HealthCheck.status, never definitions or history.

| Event | Active state and visible outcome | Evidence and recovery |
| --- | --- | --- |
| Missing definition | No runtime snapshot; Ready=False/UnknownAddonType | Prior evidence unavailable, or Unknown if none; watch plus bounded retry |
| Valid definition added | Activate only after valid binding and compilation | Requeue; Current only after a valid completed run |
| Edited to valid revision | Replace snapshot; old run becomes Superseded | Preserve old evidence with revision; evaluate new snapshot |
| Invalid edit stored | Remove eligibility; Accepted=False/InvalidDefinition | Old evidence unavailable; fix spec to recover; no stale adapter fallback |
| Unknown kind submitted | Admission rejects; stored revision unchanged | Existing valid snapshot remains; if legacy invalid storage is seen, previous row applies |
| Definition deleted | Remove matching UID only; Ready=False/DefinitionUnavailable | Keep original time and revision; recreation requires new UID binding |
| Same name recreated | New UID has no authority inherited from old binding | Ready=False/BindingMismatch until administrator authorizes new UID |
| Binding disabled/deleted or SA replaced | Revoke eligibility; Ready=False/AuthorizationRevoked or BindingMismatch | Cancel active work; final validation rejects changed context; explicit reauthorization recovers |
| RBAC denies an API read | No completed health result; Ready=False/AccessDenied | Old evidence unavailable; bounded retries use actual current permissions |
| Revocation during run | Observed revocation cancels/rejects; unobserved race has limits described above | Latest attempt explains discard; no newly dated old verdict |
| Restart or partial informer sync | No runtime execution until synchronized and directly validated | Persisted evidence stays readable; no manager fallback; reconstruct from API objects |
| Binding/grants recover | Capture new authority context; Ready remains false until evaluation succeeds | New run supplies fresh evidence; history remains attributable |
| Edit during evaluation | Final revision/context mismatch discards completion | Superseded attempt recorded; enqueue current revision |
| Two valid runtime names claim identity | Name equality/uniqueness prohibits it | Invalid legacy collision fails closed; no winner by arrival order |
| New built-in collides | Conflict barrier; Ready=False/BuiltinCollision | Preserve evidence; explicit migration/deletion resolves barrier |
| Deadline, size, parser or read budget exhausted | Attempt Error with specific reason; Ready=False | No partial Pass; bounded retry and other definitions continue |
| Evidence ages out | Freshness=Stale even if stored verdict was Pass | Next eligible run can replace it; last observation time unchanged |

**Alternatives:** Mutable instances can mix revisions; retain-old-on-invalid can
continue unauthorized logic; resetting history loses inspectability. Snapshot and
freshness state costs memory and API/status work. **Acceptance example:** Run A
finishing after B becomes active cannot publish as B or refresh A's timestamp.
**Deferred:** transactional revocation guarantees and out-of-process evaluators.
**Evidence:** registry, engine shared-slice invariant and current controller above.

## 6. Bounds and failure isolation — Proposed

These are conservative first-release design limits, not measured capacity claims.
#280 must verify them on the supported Kubernetes version and representative
fixtures before release. Admission limits bound stored inputs; the compiler and
budgeted transport repeat checks before allocating/evaluating runtime inputs.

| Class | Proposed hard limit and enforcement | Rationale / failure |
| --- | --- | --- |
| Definition | 256 KiB canonical spec; 16 families; 32 checks/family; 512 total | Compiler plus schema structural limits; bounded compilation; DefinitionTooLarge |
| Strings/maps/lists | Strings ≤1,024 UTF-8 bytes and 1,024 code points unless tighter; maps ≤32 entries; nested lists ≤32 except listed limits; depth ≤8 | Schema maxLength/maxProperties/maxItems plus byte/depth compiler checks; InvalidDefinition |
| Names and identifiers | addon/family/component ≤63 ASCII characters; resource/group/path segments ≤253 bytes | Schema grammar; no implicit normalization |
| Binding and target scope | Binding spec ≤16 KiB; ≤32 exact namespace names, each ≤63 characters; no wildcard namespaces | Compile scope intersection before reads; ScopeDenied |
| Version expressions | Range ≤256 bytes, ≤16 comparators across ≤8 alternatives; candidate API versions ≤8 | Validate token counts before SemVer parse; InvalidVersionRange |
| Selectors and field paths | ≤32 selector terms, ≤32 values/term, ≤256-byte values; field path ≤16 literal segments of ≤128 bytes | No JSONPath filters or recursive descent; bounded parsing/traversal |
| YAML and annotations | ConfigMap value ≤64 KiB, ≤4,096 parsed nodes, depth ≤16, reject aliases; annotation values ≤1,024 bytes | Bounded streaming parse before expansion; InputLimitExceeded |
| API response | ≤2 MiB decoded response per request; ≤16 MiB cumulative per run; no response retained beyond its page | Limited reader after decompression before decode, including discovery/error bodies; ResponseLimitExceeded |
| Lists and work | ≤100 objects/page; ≤1,000 objects total/run; ≤100 API requests/run including discovery, auth helpers and retries; ≤100,000 object visits/run | Stream pages; hard counter before each operation; remaining continuation at cap is WorkLimitExceeded, never truncated success |
| Target object traversal | ≤32,768 JSON nodes and depth ≤64/object before evaluator traversal | Bounded decoder; nested pod/YAML loops share visit counter; InputLimitExceeded |
| Time | min(AddonCheck.spec.timeout, 30s); each request ≤min(5s, remaining); compilation ≤1s with cooperative checks | One deadline covers discovery/reads/evaluation/final validation; Timeout |
| Results | ≤1,000 entries and 256 KiB serialized evaluation evidence; each message ≤1,024 bytes, details ≤32 entries | Reserve a bounded failure summary; ResultLimitExceeded without partial healthy verdict |
| Compiled snapshot cache | ≤128 cached revisions/process, plus the ≤4 active-run snapshots; least-recently-used eviction of idle entries | Compile on demand within the worker budget; cache pressure does not disable unrelated definitions |
| Scheduling | ≤4 runtime runs/process, ≤1/definition, ≤1/check; ≤10 requests/s burst 20 across runtime transport | Separate runtime worker pool/limiter; deduplicated queue and round-robin admission by definition; built-ins retain their workers |
| Recovery/retry | Backoff 5s, 10s, 20s, 40s, then 60s; jitter 0–20%; missing input poll 60s; max one queued wake/check; event triggers coalesce | Retry continues while declared state exists, but rate, concurrent work and queue keys are bounded; no goroutine per failure |
| Internal request retry | ≤2 retries/request, each charged to request/time limits; pagination restart ≤1/run | No hidden retry loop outside budget; exhausted attempts report Error |

Check policy overrides and discovered inputs against the same budgets; an author
cannot bypass them by moving an oversized selector or namespace list into
AddonCheck policy. The 512-check ceiling does not guarantee all 512 checks fit a
run: the lower request/object/time ceilings deliberately win. Split or narrow the
definition/check instead of raising a ceiling silently. Total AddonCheck inventory
remains subject to the cluster's administrative quotas; this RFC does not claim
to bound API-server storage or informer memory for arbitrarily many objects.

Expression inventory: existing field paths are literal segment traversal,
selectors are Kubernetes predicates, addon ranges use the existing SemVer grammar,
and annotations use timestamp parsing. These get the limits above. Fixed condition
values and YAML apiVersion comparisons are bounded string comparisons. No runtime
CEL, regex, JSONPath program or new expression language is accepted. Admission CEL
is fixed operator-authored validation code, not definition-supplied code: bound
all iterated inputs, avoid nested cross-products, and require CRD creation and
worst-size admission fixtures to pass Kubernetes' own per-rule/per-object cost
budgets. Those platform budgets are not configurable runtime-evaluation limits.

Within a run stop on the first failure in deterministic evaluation order. At
publication, precedence is revoked/mismatched authority, superseded definition or
check, invalid input, then recorded execution failure. Deadline beats a later
response-limit error after cancellation. No failed family can be averaged into a
Pass by ratio rollups. Definition compile failures affect that definition;
per-run budget failures affect that check. Definition churn invalidates work but
cannot bypass its scheduling slot or backoff. Bound event messages/rate with the
same failure deduplication, avoiding an event on every unchanged retry.

**Alternatives:** Timeout-only lists still allocate large responses; dynamic caps
per author invite evasion; process-per-run isolates faults better but adds pod
startup/privilege/operational costs beyond this release. Fixed limits may reject
legitimate large workloads and require measurements to revise. Shared typed engine
bugs can still crash the process; this is bounded input/work isolation, not a
sandbox against arbitrary Go defects. **Acceptance example:** A 1,001-object
population yields WorkLimitExceeded, never success based on the first 1,000;
a second eligible definition still receives a worker turn.
**Deferred:** larger caps pending measurements and process isolation.
**Evidence:** current evaluator lists and
[Kubernetes validation cost guidance](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#resource-use-by-validation-functions).

## Impact, rollout and reversibility

Users gain installable coverage and attributable revisions, but need platform
administration for bindings and grants. Operators gain collision, authorization,
staleness and budget conditions plus CLI inventory; no metrics dependency.
Costs include two alpha CRDs, conversion/validation, scoped discovery, bounded
transport, lifecycle storage and an additional scheduling pool. No benchmark or
infrastructure-cost savings is asserted. Alpha schema and status compatibility
must be tested before promotion; built-in execution remains available.

If accepted, #280 proceeds through schema/binding/RBAC and authoring tools,
scoped execution with bounds, then registry/watch/status lifecycle. Keep runtime
loading disabled by default until the implementation's security and full kind
e2e gates pass. Test malicious SA selection, local fallback, metrics disabled,
all lifecycle rows, denied discovery, upgrade collisions, response/parse limits,
fair scheduling and identity recreation. Generate manifests and API docs via
pinned tasks; update README, authoring docs, operator RBAC reference and release
guidance. No rollout begins merely because this RFC merges.

Rollback disables runtime loading, revokes bindings and preserves evidence with
Unavailable freshness. Leave CRDs/data installed so users can inspect/export them;
do not delete history as rollback. A prior binary can serve built-ins but will not
maintain runtime freshness/status: users must stop relying on those checks and
record the unavailable state before downgrade. Re-enabling requires revalidation.
The point requiring migration is removing stored API/semantics versions, not
adding the disabled loader. Back up manifests and evidence before that removal.

ADR 0001 remains immutable. Acceptance requires a new linked ADR narrowly
superseding its startup-only loading choice while preserving the in-process Go
interface and registry boundary. No accepted ADR is authored before approval.

## Review plan and decision questions

Review forum: [PR #349](https://github.com/skaphos/fathom/pull/349) on
`docs/addon-definition-rfc`, opened 2026-09-20. Initial proposal revision:
`7a860cb4dffb6c604103ba6a131beab865170f29`; final approval must name the
then-current PR revision. Shawn Stratton
is the decider; `@mfacenet` routes operator/API and RBAC/security perspectives
(the same person may provide both). Review window: 2026-09-20 through
2026-09-25, five business days after the Sunday opening; a changed window needs
an attributable decider decision. Explicit approval must identify the final proposal. Merge, silence and
authorship alone are insufficient; substantive edits require renewed review.

The six sections provide one proposed outcome each. Review must explicitly
accept or revise the UID-bound additional resource and delegation of future
same-UID edits (§2), collision suspension (§4), observed rather than atomic
revocation (§5), and initial limits (§6). These are consequential tradeoffs, not
approved defaults. An immediate atomic-revocation requirement would invalidate
this proposal and require another execution/authorization design.

After acceptance, add the linked ADR, complete checks, merge and read back both
records. Update existing #280 with accepted scope, deferrals, #256 disposition
and e2e needs; publish post-merge evidence through the separate documentation PR
specified by the feature tasks. Close #278 only after that evidence is verified.
Rejection or withdrawal is recorded and does not satisfy feature completion.

## Prior art and review evidence

- [ADR 0001](../adr/0001-in-process-adapter-contract.md): existing loading choice.
- [Declarative implementation plan](../design/addon-adapters-implementation-plan.md): typed engine already adopted; this RFC adds loading, not another evaluator framework.
- [Feature research](../../specs/011-addon-definition-rfc/research.md): source evidence and alternatives.
- [Review contract](../../specs/011-addon-definition-rfc/contracts/rfc-review.md): requirement and scenario traceability.
- [Execution record](../../specs/011-addon-definition-rfc/execution.md): checks and governance state.
