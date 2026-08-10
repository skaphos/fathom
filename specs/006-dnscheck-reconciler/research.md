# Phase 0 Research: DNSCheck Reconciler

**Feature**: 006-dnscheck-reconciler · **Date**: 2026-08-09

The clarify session settled the four behavioural forks. This phase settles how
they land against the code that already exists, and records three findings that
contradict assumptions written into issue #266 itself.

---

## D1 — Probe pods carry no ownerReference today, and DNSCheck is the first kind that can give them one

**Decision**: Add an optional owner-reference field to the probe request, set it
from the `DNSCheck`, and keep the existing sweeper as the second line of defence.

**Rationale**: Issue #266 says *"Probe pods must carry an `ownerReference` and be
reaped by the existing leader-elected sweeper (#220)"* as though the first half
already held. It does not — `internal/probe` sets no owner reference anywhere,
and `Sweeper` selects purely on labels (`fathom.skaphos.io/managed-by` plus the
probe label), terminal phase, and age.

That is not an oversight. A namespaced owner must live in the **same namespace**
as the object it owns, and an `AddonCheck`'s probe pod lands in the *addon's*
namespace, which is frequently not the check's. A cross-namespace owner
reference is invalid, so `AddonCheck` structurally cannot use one.

`DNSCheck` is different precisely because of FR-031: resolution runs in the
**check's own namespace**, so the pod and its owner are co-located and the
reference is legal. This feature is therefore the first that can have real
ownership, not merely label attribution.

The two mechanisms cover different failures and both are needed:

| Failure | Covered by |
|---|---|
| Check deleted while pods exist | ownerReference → cascading GC (FR-114) |
| Operator dies between pod Create and its delete defer | Sweeper (FR-113) |

Kubernetes does **not** garbage-collect terminated pods that still have a live
owner, so ownership alone would reintroduce the orphan class #220 removed.

**Alternatives considered**: Labels only, matching `AddonCheck` — rejected
because it leaves check deletion (FR-114) dependent on a sweep that is gated on
age and terminal phase, so a running pod outlives its deleted check. Owner
reference only — rejected per the GC behaviour above.

**Change required**: `probe.Request` gains an owner-reference field; `Pod()`
stamps it. Additive, and every existing caller is unaffected when it is unset.

---

## D2 — Probe pod reads must not go through the manager cache

**Decision**: Give the DNSCheck launcher a dedicated **uncached** client built
from the manager's rest config, rather than `mgr.GetClient()`.

**Rationale**: `Launcher.Run` does Create → poll with Get → Delete. The
controller-runtime default client serves structured reads from the shared
informer cache, and `scopedCacheOptions()` (`internal/app/run.go`) does not list
`Pod` — so the first Pod `Get` through the manager client would start an
**unfiltered cluster-wide Pod informer**. That is exactly the memory blow-up
removed in #164/SKA-581, and `Sweeper`'s own doc comment warns about it in the
same words.

The adapter path avoids this today only incidentally: adapters receive the
uncached *impersonating* client, so their probe Gets never touch the cache.
DNSCheck has no per-addon ServiceAccount to impersonate, so it cannot inherit
that protection and must construct an uncached client explicitly.

**Alternatives considered**: `mgr.GetAPIReader()` — a `client.Reader`, so it
cannot Create or Delete, and `Launcher` needs one `client.Client`. Adding `Pod`
to the cache with a label selector — cheaper-looking, but it still opens a
cluster-wide Pod watch and the selector cannot be expressed in RBAC, so it buys
nothing over an uncached client.

---

## D3 — Fan-out is bounded-concurrency goroutines, not sequential iteration

**Decision**: Evaluate pairs concurrently with a hard in-flight limit
(`errgroup` with `SetLimit`), the limit sourced from configuration.

**Rationale**: `Launcher.Run` blocks for the whole pod lifecycle, so concurrency
must live in the caller. Both existing DNS adapters (`coredns`,
`nodelocaldns`) iterate targets in a plain `for` loop, which is what makes #150
reproducible: each target is handed the *full* timeout while sharing one outer
context, so the budget is gone before the last targets are reached.

Sequential iteration is also simply too slow here. At 48 pairs and ~1–3s of pod
startup each, a serial run needs 48–144s — beyond any admissible run bound,
since the contract caps the bound at the cadence.

**Alternatives considered**: Sequential (the adapter precedent) — rejected on
both counts above. Unbounded goroutines — rejected: 48 simultaneous pods in a
tenant namespace is a quota event, and the cluster-wide total would be
unbounded across checks.

---

## D4 — One run deadline, derived per-pair bounds

**Decision**: Derive one `context.WithTimeout` for the whole run from the
declared bound. Each pair's probe timeout is `min(remaining, perPairCeiling)`.

**Rationale**: This is FR-104 expressed in code, and it is the direct fix for
#150 — no pair can consume budget that later pairs need, because every pair
reads the *remaining* budget at the moment it starts. The per-pair ceiling stops
one unresponsive resolver from consuming a slot for the entire run.

Because the contract already rejects `timeout > interval` at admission, a run
bounded this way cannot outlast its own cadence (FR-104a), which is what makes
overlapping runs structurally impossible rather than merely discouraged.

---

## D5 — Cadence is anchored to run start, with a floor

**Decision**: `RequeueAfter = max(minGap, interval − elapsed)`, measured from the
start of the run.

**Rationale**: FR-107. Every existing reconciler ends with
`ctrl.Result{RequeueAfter: interval}` computed *after* the work
(`nodecertificatecheck_controller.go:292`, `addoncheck_controller.go:261`), so
the effective cadence is `interval + run duration`. For adapters doing fast API
reads the drift is noise; for a kind that launches pods and may run for tens of
seconds it is a ~2× error against what the operator declared.

The floor matters because, under D4, a run only consumes its entire budget when
it is truncating — i.e. when the check is misconfigured. Without a floor, such a
check would run back-to-back forever, creating pods continuously.

**Follow-up, deliberately not taken here**: `AddonCheck` and
`NodeCertificateCheck` have the same drift. Changing a shipped kind's observable
timing belongs in its own issue.

---

## D6 — Unreached pairs are `Unknown`; truncation is also a condition

**Decision**: Seed every declared pair as `Unknown` before the run and overwrite
as results arrive. Whatever is still `Unknown` at the deadline is what the run
did not reach. Raise a distinct condition naming the unreached count.

**Rationale**: FR-106. Against `WorstResult`'s ordering
(`Pass < Warn < Unknown < Fail < Error`, with `Skipped` informational and never
winning), `Skipped` would let a 40-of-48-unevaluated run report `Pass`, and
`Error` would outrank and therefore mask a genuine `Fail` among the pairs that
did run. `Unknown` degrades honestly and loses to a real failure — the same
property `coerceEmptyToUnknown` was added for in #161.

Seed-then-overwrite also delivers FR-036 for free: the result set is built from
the current spec each run and can only contain pairs the spec still declares.

**No schema change**: `DNSCheckStatus` already carries `Conditions` and a
1024-character `Summary`, so the truncation signal needs no new field.

---

## D7 — A new per-target gauge, rebuilt by delete-then-set

**Decision**: Add one `GaugeVec` to `internal/metrics` labelled by namespace,
check, subject, record type, vantage point, and outcome. On every run, delete
the check's series by partial match, then set the current pairs.

**Rationale**: FR-033/034/036. `NodeCertificateExpiryDays` is the precedent for
a kind-specific gauge living in the shared package. Delete-then-set is what
makes withdrawal automatic: a pair the spec no longer declares is simply not
re-set, so its series disappears without anything having to detect the removal.

`DeletePartialMatch` already backs `DeleteCheckSeries`, so the mechanism is
proven. Series ceiling is 16 × 3 × 6 = 288 per check, exactly SC-009.

**Alternatives considered**: Diffing previous against current pairs and deleting
the difference — more code, more state, and it fails after an operator restart
when the previous set is gone.

---

## D8 — History and events reuse the existing seams unchanged

**Decision**: `observeCheck` for the check-level gauges and events;
`decideNodeCertRollup`'s three-way shape for persist-on-change;
`useDeterministicHealthReportName` + `createOrReuseHealthReport` for the record.

**Rationale**: FR-108/111/112 and inherited FR-032. `observeCheck` already takes
`kind` as a plain string and handles first-observation, result transitions,
Ready-condition failures, reconcile errors, and cadence clamps. Nothing about it
is kind-specific, so DNSCheck gets the whole events contract by calling it.

The rollup decision (persist / refresh-liveness / noop) is the established
answer to "only on result change" without freezing `lastRunTime`, and it is a
pure function, so it unit-tests without envtest.

---

## D9 — The RBAC grant, and the document that has to defend it

**Decision**: Widen the pods grant to `create;get;delete` alongside the existing
`list;delete`, cluster-wide, and ship a justification document with it.

**Rationale**: FR-115/FR-037. Today the operator ClusterRole holds only
`pods: [delete, list]` (from `internal/probe/sweeper.go:46`) — it cannot create
a pod at all. Probe pods reach tenant namespaces today only by impersonating
per-addon ServiceAccounts, and DNSCheck has none to impersonate.

The grant cannot be namespace-scoped: a `DNSCheck` may exist in any namespace,
and the set is not known when RBAC is rendered. Per the constitution's Minimal
RBAC constraint and the repository's standing rule, the grant needs an in-repo
document defending why nothing narrower suffices.

`get` is required because the launcher polls the pod it created. `watch` is
**not** required and must not be requested — the launcher polls.

---

## D10 — Configuration needs an integer binding, which does not exist yet

**Decision**: Add an `isInt` variant to the `bindings()` table in
`internal/app/options.go` and expose the concurrency cap through it.

**Rationale**: FR-103a requires the cap to be configurable. The bindings table
supports string, bool, and float only. Adding the integer case keeps flag,
viper key, env var (`FATHOM_*`), and config-file key in sync automatically, as
the constitution's configuration-model constraint requires. Hand-rolling one
flag outside the table would break that invariant.

**Alternatives considered**: A package constant, as
`addonCheckMaxConcurrentReconciles = 4` is — rejected because FR-103a says
configurable, and because the right value depends on cluster size and tenant
quota in a way a compiled-in constant cannot serve.

---

## Open risks carried into design

1. **Probe image reachability in tenant namespaces.** The probe pod runs in the
   check's namespace, which may have an ImagePullSecret regime or a registry
   allowlist the operator's namespace does not. This surfaces as a placement
   failure per pair (FR-103b) rather than a run-level fault, but it is the most
   likely first-contact problem in a real cluster and e2e should cover it.
2. **PodSecurity admission in tenant namespaces.** `Pod()` already builds a
   hardened pod; whether it satisfies a `restricted` namespace is asserted by
   existing tests but has not been exercised against a namespace the operator
   does not own.
3. **Effective per-pair cost is unmeasured.** FR-104b requires documenting the
   run bound a given pair count needs, and the 1–3s figure used above is an
   estimate. The number has to come from a measured e2e run, not from this
   document.
