# Phase 0 Research: DNSCheck Resource Contract

**Feature**: `specs/005-dnscheck-resource-contract`
**Date**: 2026-08-08

Ground truth verified against the tree at `feature/265-dnscheck-crd-type`. Every
claim below cites the file it was read from; nothing here is recalled.

---

## R1 — How to evaluate the record kinds

**Decision**: use the standard library's `net.Resolver`. Every kind in FR-003
is covered without a new dependency:

| Kind | Call | Notes |
|------|------|-------|
| `Host` *(default)* | `LookupHost(ctx, name)` | either family; today's existing path |
| `A` | `LookupIP(ctx, "ip4", name)` | returns `[]net.IP` |
| `AAAA` | `LookupIP(ctx, "ip6", name)` | returns `[]net.IP` |
| `CNAME` | `LookupCNAME(ctx, name)` | see the trap below |
| `SRV` | `LookupSRV(ctx, "", "", name)` | see the trap below |
| `PTR` | `LookupAddr(ctx, ip)` | subject is an address, not a name |

**Rationale**: `cmd/probe/main.go` imports only stdlib today, and the probe
ships as its own minimal image (`Dockerfile.probe`). Keeping it dependency-free
preserves that. The current `runDNS` already uses
`net.DefaultResolver.LookupHost` (`cmd/probe/main.go:72`), so this is a
widening of an existing pattern, not a new one.

**Two traps that must be handled explicitly, or the checks will silently lie:**

1. **`LookupCNAME` does not fail when no CNAME exists.** It follows the chain
   and returns the final canonical name; for a name with no CNAME record it
   returns the name itself (fully qualified). So "is there a CNAME?" cannot be
   answered by error-vs-success. A `CNAME` target must compare the returned
   canonical name against the declared expected answers, and a canonical name
   equal to the queried name (modulo the trailing dot) means *no CNAME record*.
   Treating a successful `LookupCNAME` as a pass would make every `CNAME` check
   pass unconditionally — exactly the class of silent-lie defect FR-013 exists
   to prevent.

2. **`LookupSRV` has two modes and the wrong one rewrites the query.** With a
   non-empty service and proto it constructs `_service._proto.name`. With both
   empty it looks up `name` exactly as given. Since the schema accepts a
   subject that already carries its `_service._proto` labels (an Edge Case in
   the spec), the empty-service form is the correct call. Passing the subject
   as `service` would query a mangled name and fail for reasons unrelated to
   the target's health.

**Accepted limitation**: the stdlib resolver exposes answers only — no TTL, no
response code, no authority section. Pass/fail plus the answer set (what
FR-004, FR-012, and FR-022 need) is fully served; TTL-based or rcode-based
assertions are not possible without a real DNS library.

**Alternatives considered**:

- **`github.com/miekg/dns`** — full DNS protocol library, gives TTL, rcode, and
  arbitrary record types. Rejected: adds a dependency to the deliberately tiny
  probe image for capability the spec does not ask for. Reconsider only if
  TTL/rcode ever becomes a declarable expectation, which the spec explicitly
  places out of scope.
- **Shelling out to `dig`** — rejected outright; the probe image has no shell
  and runs read-only, non-root (`internal/probe/pod.go:115-149`).

---

## R2 — How the three resolver vantage points are realized

**Decision**: all three are **pod-level** concerns, not probe-binary flags.
Two of the three already work.

| Vantage point | Mechanism | Status |
|---------------|-----------|--------|
| Cluster DNS (default) | pod inherits `dnsPolicy: ClusterFirst` | **already works** — the default path today |
| Explicit upstream | `dnsPolicy: None` + `dnsConfig.nameservers` | **already implemented** as `Request.DNSNameservers` (`internal/probe/pod.go:70-76, 163-166`) |
| Node's resolver | `dnsPolicy: Default` | **missing** — small addition to `Pod()` |

**Rationale**: this is the single most plan-shaping finding. The spec's Overview
assumed explicit-resolver support had to be built; it does not. `DNSNameservers`
was added for the node-local DNS checks (SKA-511) and is already exercised by
`internal/adapter/nodelocaldns/adapter.go:455`. The remaining gap is one
`dnsPolicy` value.

**Consequence — a real constraint the schema must carry.** The existing field
comment states it plainly: `dnsPolicy: None` gives the pod *exactly* the listed
nameservers and **no cluster search domains**, so "callers must therefore pass
fully qualified targets." A short name like `my-svc` that resolves fine under
cluster DNS will fail under an explicit resolver — not because anything is
unhealthy, but because there is no search path to complete it. This must be
documented on the field and surfaced in the operator guide; it is a foot-gun
that would otherwise read as a DNS outage.

**Design note**: `Request` should gain an explicit vantage-point selector
rather than a second boolean. `DNSNameservers` already implies "explicit", so
adding a bare `UseNodeResolver bool` creates two fields that can contradict
each other. A single enum-typed field with `DNSNameservers` valid only in the
explicit case keeps the invalid states unrepresentable.

---

## R3 — Outcome mapping, including negative assertions

**Decision**: classify from `*net.DNSError`, and branch on `IsNotFound` rather
than on error-vs-nil.

| Situation | Positive assertion | Negative assertion |
|-----------|--------------------|--------------------|
| Answers returned, expectations satisfied | `Pass` | `Fail` |
| Answers returned, declared answers missing | `Fail` | *(rejected at admission — FR-005)* |
| Resolver answered "no such name" (`IsNotFound`) | `Fail` | **`Pass`** |
| Resolution returned zero answers without error | `Fail` | `Pass` |
| Resolver unreachable / timeout / temporary failure | `Fail` | **`Error`** |
| Not a `*net.DNSError` at all | `Error` | `Error` |

**Rationale**: the positive column preserves the ruling already encoded and
commented at `cmd/probe/main.go:77-89` — a DNS-level failure is the condition
the check exists to detect, so it is `Fail`, not `Error`, because `Error`
outranks `Fail` on the severity ladder (`Pass < Skipped < Warn < Unknown < Fail
< Error`) and misclassifying would mask genuine failures elsewhere in the
rollup. FR-025 restates this; nothing about it moves.

The negative column is where FR-014 lives, and it is the subtle one. Under a
negative assertion, "I could not reach the resolver" is **not** evidence that a
name is gone — it is evidence of nothing. Mapping it to `Pass` would turn a
network fault into false proof that a decommissioned hostname was retired.
`Error` is the honest answer. This is the one place where the two columns
deliberately diverge in kind rather than in polarity.

`net.DNSError` exposes `IsNotFound`, `IsTimeout`, and `IsTemporary`, which is
exactly the discrimination FR-014 requires. `IsNotFound` means the resolver
answered authoritatively; the others mean it did not answer.

**Alternatives considered**:

- **Map unreachable-under-negation to `Unknown`** — sits below `Fail` on the
  ladder, so a wedged resolver would be quieter than a passing check is loud.
  Rejected: it under-reports a condition where the check has genuinely stopped
  working.
- **Treat any error as satisfying a negative assertion** — the naive reading.
  Rejected explicitly by FR-014; this is the defect the requirement was written
  to prevent.

---

## R4 — Keeping CEL inside the cost budget

**Decision**: express as much as possible in plain OpenAPI constraints
(`Enum`, `Pattern`, `MaxLength`, `MaxItems`, `Minimum`), and reserve
`XValidation` for genuinely cross-field rules.

**Rationale**: this repository has already hit the ceiling. Two comments record
it: `AddonCheckFamilyPolicy.LabelSelector` notes that a CEL rule for the
structural checks "exceeds the API server's per-CRD cost budget, because the
imported LabelSelector schema carries no size bounds the estimator could use"
(`api/v1alpha1/addoncheck_types.go:36-40`), and `ThresholdValue` exists purely
so the estimator has "a real input size" (`addoncheck_types.go:12-16`). The
estimator prices a rule by the *declared* maximum size of its inputs, so an
unbounded string or list makes any rule touching it expensive.

Cost control therefore falls out of FR-018 rather than fighting it: bounding
every string and list is both the security requirement and the thing that keeps
the rules affordable.

**Cross-field rules that genuinely need CEL** (the same three the other kinds
already carry, plus two new):

1. `timeout >= 1s` (FR-016)
2. `interval >= 10s` (FR-016)
3. `timeout <= interval` (FR-017)
4. negative assertion must not declare expected answers (FR-005)
5. a `PTR` subject must be an address; the others must not be (FR-002)

Rules 4 and 5 are per-item over `targets`, so they run under `.all()` — the
construct most likely to blow the budget. Bounding `targets` tightly (R6) is
what keeps them affordable.

**Verification is mandatory, not assumed**: cost is rejected at CRD *install*
time, so a too-expensive rule fails `task install` and envtest, loudly. The
plan schedules an explicit early install check rather than discovering this at
the end.

---

## R5 — Subject syntax validation

**Decision**: validate per record kind, since FR-002 requires the subject match
"the syntax appropriate for its record kind".

- **`A` / `AAAA` / `CNAME`** — DNS name. Allow a trailing dot (fully qualified
  form) and standard LDH labels, `MaxLength: 253`.
- **`SRV`** — the same, but labels may begin with `_` (`_https._tcp.example.com`).
  Per the spec's Edge Cases these must survive validation rather than be
  rejected as malformed.
- **`PTR`** — an IP address, not a name.

**Open implementation choice, deferred to the task**: whether to use one
permissive name pattern for all four name kinds (simpler, one rule, but lets an
`A` target carry underscore labels) or to gate the underscore form on `SRV`
(stricter, needs a cross-field CEL rule and so costs budget). Recommendation:
**one permissive pattern**. An underscore-labelled `A` target is harmless — it
simply will not resolve, and the check reports that truthfully — whereas the
strict version spends scarce CEL budget to reject something that is not a
safety problem.

For the `PTR` address check, Kubernetes CEL has gained an IP library
(`isIP()`); the k8s.io modules here are `v0.36.3` so it should be present, but
availability must be confirmed against the actual envtest API server before
relying on it. A regex fallback is acceptable if it is not.

---

## R6 — How many probe pods a run costs, and the bounds that follow

**Decision**: **one probe pod per (target, resolver) pair**, with
`targets` capped at 16 and `resolvers` at 3 — a worst case of 48 pods per
evaluation.

**Rationale**: the existing probe contract is one pod, one `Result`
(`internal/probe/pod.go:79-83`, `ParseResult` at `:217`). One pod per pair
falls straight out of it and yields the per-target results FR-022 requires,
with no change to the probe's result shape.

The cost is real and must be bounded at the schema, because the schema is what
an operator can write. Unbounded, a single `DNSCheck` on a 1-minute cadence
could ask for hundreds of pods per minute. Caps of 16 and 3 keep the worst case
at 48 while leaving generous headroom for realistic checks (most will declare a
handful of names against one resolver).

**Alternatives considered**:

- **Batch mode — one pod resolves N targets against one resolver**, making pod
  count equal to resolver count (≤3). Materially better on pod churn, and the
  right answer if churn ever becomes the binding constraint. **Rejected for
  this slice**: it requires a multi-result probe contract, because FR-022 needs
  per-target detail and today's `Result` carries a single outcome plus a flat
  `map[string]string`. Encoding N results into that map would be a hack; doing
  it properly means versioning the probe's output contract, which is a larger
  change than this slice should carry and would touch every existing adapter
  that calls `ParseResult`.
- **Revisit trigger, recorded so it is not forgotten**: if a real check needs
  more than ~16 names, or if probe-pod churn shows up in cluster metrics,
  batching becomes the next change — and it belongs with the reconciler work in
  #266, which is what actually launches the pods.

---

## R7 — Where answer-matching and polarity are evaluated

**Decision**: inside the probe binary, alongside the query.

**Rationale**: FR-012 requires the outcome to carry "both the verdict and the
evidence that produced it". The probe already writes structured evidence into
`Details` (`cmd/probe/main.go:74, 97`) and is the only place that holds the raw
answer set. Comparing in the reconciler instead would mean shipping the full
answer set back through the termination-log contract and re-deriving the
verdict a layer away from the evidence — more moving parts, and a second place
for the two to disagree.

This also keeps the reconciler in #266 thin: it decides *what* to ask and
records *what came back*, without owning DNS semantics.

---

## R8 — Version track and scaffolding

**Decision**: `DNSCheck` at `v1alpha1` in the existing `api/v1alpha1` package,
registered in `PROJECT`, generated artifacts produced only by the pinned tasks.

**Rationale**: FR-028 and the epic both place this kind on its own version
track; #149 promotes only the four pre-existing kinds. Physically it lives in
the same Go package as the others, which is already the case for all five
current kinds — the version track is a property of the CRD's served versions,
not of the directory.

Per `AGENTS.md`, `zz_generated.deepcopy.go`, `config/crd/bases`, RBAC, samples,
and `docs/reference/api.md` are regenerated via
`go -C tools tool task manifests generate docs:api-ref` and never hand-edited.
`task crd-compat` must pass (FR-029) — a brand-new CRD adds no incompatible
change to an existing one, so this should be clean, but it is a gate, not an
assumption.

---

## Resolved unknowns

Every NEEDS CLARIFICATION from Technical Context is resolved above:

| Unknown | Resolved by |
|---------|-------------|
| Can the stdlib evaluate every record kind? | R1 — yes, with two traps to handle |
| Does explicit-resolver support need building? | R2 — no, `DNSNameservers` already exists |
| How is the node vantage point realized? | R2 — `dnsPolicy: Default`, one small addition |
| How is "unreachable" told from "no such name"? | R3 — `net.DNSError.IsNotFound` |
| Will the validation rules fit the CEL budget? | R4 — yes if every field is bounded; verified at install |
| How is a `PTR` subject validated? | R5 — as an address; `isIP()` if available, regex otherwise |
| What does a run cost in pods? | R6 — one per pair, capped at 48 |
| Where does matching happen? | R7 — in the probe, with the evidence |
</content>
