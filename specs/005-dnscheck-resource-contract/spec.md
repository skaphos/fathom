# Feature Specification: DNSCheck Resource Contract

**Feature Branch**: `feature/265-dnscheck-crd-type`

**Created**: 2026-08-08

**Status**: Draft

**Input**: User description: "GitHub issue skaphos/fathom#265 — feat(api): DNSCheck CRD type (v1alpha1): schema, validation, printer columns. Part of epic #205 (DNSCheck). Declare `spec.targets[]` (name, expected record type, optional expected answers, optional resolver override), `spec.resolvers[]` (cluster DNS default, explicit upstream, node resolver), `spec.interval`/`spec.timeout` with the #152 floors and the `timeout <= interval` rule, `spec.policy` consistent with the other kinds, no pause field per #262, status mirroring the established shape with per-target results, printer columns from the start, and every string and list bounded."

## Overview

DNS resolution is the failure mode that takes clusters down quietly. When
cluster DNS degrades, workloads do not stop — they hang, retry, and time out,
and the operator learns about it from application alerts several layers away
from the cause. Fathom already reports on the *health of the CoreDNS
deployment* through its addon adapter, but a healthy CoreDNS Deployment and a
cluster that can actually resolve names are different claims. This feature adds
the resource that lets an operator declare the second claim directly.

`DNSCheck` is the declarative contract: an operator states which names must
resolve, from where, with what expected answers, and how often, and Fathom
converges on continuously answering whether that is true.

**This specification covers two things that must ship together**: the
declarable contract with its admission-time guarantees, and the resolution
capability that makes every declared field real. They are inseparable because
of FR-013 — the published schema must not name a capability that does not
exist. The resolution capability available today performs host lookup against
the ambient resolver only: no record-type selection, no explicitly addressed
resolver, no answer comparison, no negative assertion. Publishing a schema with
those fields while the capability is missing would reproduce, in a brand-new
resource, the exact defect already recorded against the wrapper's target-kind
documentation (finding API-5). So the capability grows in this slice alongside
the contract.

What is *not* here: the controller that runs the check on a cadence and writes
its results, the roll-up into the cluster-wide verdict, and end-to-end
coverage. Those are the following slices of the same epic (see Out of Scope).

## Clarifications

### Session 2026-08-08

- Q: Which DNS record types are admissible in the first release, given the
  resolution capability today performs host lookup only? → A: **The full set —
  A, AAAA, CNAME, SRV, PTR — with the resolution capability widened in this
  same slice to evaluate all of them.** The schema is not narrowed to today's
  capability; the capability is raised to meet the schema. By the same
  reasoning, explicitly addressed resolvers are wired here too, since
  `spec.resolvers[]` would otherwise be equally unhonored.
- Q (raised during planning): what should an unstated record kind mean, given
  that defaulting to `A` would verify IPv4 only? → A: **a sixth value, `Host`,
  meaning an address of either family, as the default.** Defaulting to `A`
  would fail an AAAA-only name for a reason its author would not expect, and
  would make the resource's default and the resolution capability's existing
  default two subtly different behaviours. `Host` collapses them into one.
- Q: Are negative assertions ("this name MUST NOT resolve") part of the first
  release? → A: **Yes.** Each target carries an explicit polarity. Deferring it
  would have forced a later semantic retrofit, because "no expected answers
  declared" already means "any answer satisfies the check" — a negative
  assertion cannot be layered onto that without changing the meaning of an
  existing field.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Declare DNS resolution intent and have it validated (Priority: P1)

A platform operator adds a `DNSCheck` to the cluster's GitOps repository
stating that `kubernetes.default.svc.cluster.local` and the organization's
internal API hostname must resolve, every minute, and that a decommissioned
hostname must no longer resolve at all. Malformed intent — an empty target
list, a hostname that is not a hostname, a timeout longer than the interval, a
one-millisecond interval that would hot-loop the operator, a target that
simultaneously asserts "must not resolve" and "must resolve to these addresses"
— is rejected by the cluster at write time, with a message that names the
offending field and what would have been acceptable. Well-formed intent is
accepted and appears in a standard listing with its own columns.

**Why this priority**: Rejection at admission is the only point where a mistake
is free. Once a malformed check is stored, every consumer downstream — the
controller, the report history, the aggregate verdict — has to carry defensive
handling for state that should never have existed. It is also the slice that
delivers value with no controller at all: an operator can author, review, and
land DNS health intent in Git and know the cluster accepted it.

**Independent Test**: Apply a corpus of valid and invalid `DNSCheck` manifests
against a cluster with only the resource definition installed — nothing
running — and assert each is accepted or rejected with the expected message.
Fully testable without any execution path.

**Acceptance Scenarios**:

1. **Given** the resource definition is installed, **When** an operator applies
   a `DNSCheck` naming one target with no other fields set, **Then** the object
   is accepted and every unset cadence, polarity, and record-kind field reports
   a documented default when read back.
2. **Given** the resource definition is installed, **When** an operator applies
   a `DNSCheck` with an empty target list, **Then** the write is rejected and
   the message states that at least one target is required.
3. **Given** the resource definition is installed, **When** an operator applies
   a `DNSCheck` whose timeout exceeds its interval, **Then** the write is
   rejected and the message names both fields.
4. **Given** the resource definition is installed, **When** an operator applies
   a `DNSCheck` with an interval below the platform floor, **Then** the write is
   rejected and the message states the floor.
5. **Given** the resource definition is installed, **When** an operator applies
   a target asserting the name must not resolve while also declaring expected
   answers, **Then** the write is rejected as contradictory intent.
6. **Given** the resource definition is installed, **When** an operator applies
   a target naming a record kind outside the supported set, **Then** the write
   is rejected and the message names the supported set.
7. **Given** a `DNSCheck` exists, **When** an operator runs the standard
   listing command, **Then** the verdict, the number of targets, the time of
   the last evaluation, and the object's age are visible without reading the
   full object.
8. **Given** the resource definition is installed, **When** an operator applies
   a `DNSCheck` carrying a field that suspends the check, **Then** the write is
   rejected as an unknown field — suspension is not part of this contract.

---

### User Story 2 - Every declared expectation is actually evaluated (Priority: P1)

The operator's declaration is not decorative. A target declaring an `SRV`
record is resolved as an `SRV` query, not silently downgraded to a host lookup.
A target naming an explicit upstream resolver is resolved against *that*
resolver, not the ambient one. A target declaring expected answers fails when
those answers are absent. A target asserting a name must not resolve passes
precisely when resolution finds nothing.

**Why this priority**: Equal-first with US1, and inseparable from it. A
declared field that is not honored is worse than an absent one: it reads as an
enforced control during review while enforcing nothing. This story is what
makes the schema in US1 honest, and it is the reason the resolution capability
is in this slice rather than the next.

**Independent Test**: Exercise the resolution capability directly against a
controlled DNS fixture — no controller, no cluster resource — asserting one
case per record kind, one case per resolver vantage point, answer-match and
answer-mismatch, and both polarities. Fully testable as a unit.

**Acceptance Scenarios**:

1. **Given** a name with a known `SRV` record, **When** it is evaluated as an
   `SRV` target, **Then** the result reflects the `SRV` answer and not a host
   lookup of the same name.
2. **Given** two resolvers that disagree about a name, **When** a target names
   one of them explicitly, **Then** the result reflects that resolver's answer.
3. **Given** a target declaring expected answers, **When** resolution returns a
   set that does not include all of them, **Then** the result is a failing
   verdict naming what was expected and what was returned.
4. **Given** a target asserting a name must not resolve, **When** the name does
   not resolve, **Then** the result is a passing verdict; **and when** the name
   does resolve, **Then** the result is a failing verdict.
5. **Given** any target, **When** the declared resolver cannot be reached at
   all, **Then** the result distinguishes "resolver unreachable" from "resolver
   answered: no such name".

---

### User Story 3 - Learn that resolution is failing, with evidence (Priority: P2)

The organization's internal API hostname stops resolving because an upstream
forwarder was misconfigured. Within one check interval the `DNSCheck` reports a
failing verdict naming which target failed and what the resolver said, and a
durable report records the evidence so the operator can answer "when did this
start" after the fact.

**Why this priority**: The operational payoff, but it depends on both slices
above existing first. Delivered by the controller slice of the epic; the
contract in US1 already defines the shape of what it will report.

**Independent Test**: Point a check at a name that does not resolve and assert
the verdict, the summary naming the target, and the persisted evidence.

**Acceptance Scenarios**:

1. **Given** an accepted `DNSCheck` with two targets, **When** one target stops
   resolving, **Then** the check's verdict becomes failing and the per-target
   results distinguish the failing target from the passing one.
2. **Given** a check that has been failing, **When** resolution recovers,
   **Then** the verdict returns to passing within one interval and the history
   retains both transitions.

---

### User Story 4 - Roll DNS health into the cluster-wide verdict (Priority: P3)

The operator wraps the `DNSCheck` so that a DNS failure is reflected in the
single cluster-wide health verdict alongside addon and certificate health.

**Why this priority**: Aggregation is what makes the signal actionable at the
fleet level, but it is a distinct, already-specified mechanism that requires
generalizing the existing wrapper. Delivered by the mirroring slice of the
epic.

**Independent Test**: Wrap a failing `DNSCheck` and assert the cluster-wide
verdict degrades.

**Acceptance Scenarios**:

1. **Given** a failing `DNSCheck` wrapped for aggregation, **When** the
   aggregate is evaluated, **Then** the cluster-wide verdict reflects the
   failure and names the source.

---

### Edge Cases

- **Empty intent**: a check with no targets is meaningless and is rejected
  rather than silently reporting a vacuous pass.
- **Cadence that starves the operator**: an interval below the platform floor
  is rejected, and stored objects predating the floor are treated as if set to
  the floor rather than honored literally.
- **Timeout that can never fire**: a timeout longer than the interval would
  overlap runs; it is rejected.
- **A target that is a name, syntactically**: `not a hostname` and
  `http://example.com/path` are rejected; a trailing-dot fully-qualified name
  and a single-label service name are accepted.
- **Contradictory polarity**: a target asserting a name must not resolve while
  also declaring the answers it must return is self-contradictory and is
  rejected at write time rather than resolved by precedence.
- **Reverse lookups are addresses, not names**: a `PTR` target's subject is an
  IP address, not a hostname, so it is validated against a different syntax
  than the other record kinds.
- **Service records are underscore-labelled**: an `SRV` target's subject
  conventionally carries `_service._proto` labels, which must survive name
  validation rather than being rejected as malformed.
- **Explicit resolver address shape**: a declared upstream resolver must be a
  routable address with an optional port; a hostname that itself requires
  resolution to reach is a bootstrapping trap and is rejected.
- **Unbounded payload**: an operator (or a compromised tenant) submitting
  thousands of targets, or a single multi-megabyte target string, is rejected
  by explicit item and length caps rather than accepted and paid for at
  evaluation time.
- **Resolver reachability is not resolution**: a declared upstream resolver
  that cannot be reached at all is a distinct condition from a resolver that
  answers "no such name". Under a positive assertion both fail, but they must
  be distinguishable in the evidence; under a negative assertion they are
  emphatically not equivalent — an unreachable resolver must never be read as
  proof that a name is gone.
- **Duplicate targets**: the same name declared twice, or declared once per
  resolver, must produce deterministic per-target result identity rather than
  colliding entries.
- **A negative assertion that never becomes true**: an operator asserting a
  name must not resolve, against a resolver that always answers, produces a
  persistent failing verdict — correct behavior, and the reason the summary
  must name the polarity so the failure is not misread as a DNS outage.

## Requirements *(mandatory)*

### Functional Requirements

**Declaring intent**

- **FR-001**: An operator MUST be able to declare one or more DNS targets, as a
  non-empty list; a check declaring none MUST be rejected at write time.
- **FR-002**: Each declared target's subject MUST be constrained at write time
  to the syntax appropriate for its record kind — a DNS name for forward
  lookups, an IP address for reverse lookups — so malformed entries are
  rejected before storage.
- **FR-003**: An operator MUST be able to state, per target, which kind of DNS
  record is expected, drawn from the supported set: `Host`, `A`, `AAAA`,
  `CNAME`, `SRV`, and `PTR`. A kind outside that set MUST be rejected at write
  time with the supported set named. When unstated, the default MUST be `Host`
  — satisfied by an address of either family — so that a target on a
  dual-stack or IPv6-only name is not failed by a default that silently means
  "IPv4 only". `A` and `AAAA` narrow to a single family only when named
  explicitly.
- **FR-004**: An operator MUST be able to state, per target, the specific
  answers expected, and a check MUST fail when the returned answer set does not
  satisfy that expectation. When no expected answers are stated, any non-empty
  answer satisfies the check.
- **FR-005**: An operator MUST be able to state, per target, whether the
  assertion is positive (the name must resolve) or negative (the name must not
  resolve). The default MUST be positive. A negative assertion combined with
  declared expected answers is contradictory and MUST be rejected at write
  time.
- **FR-006**: An operator MUST be able to declare where resolution is performed
  from — the cluster's own DNS service, an explicitly addressed upstream
  resolver, or the resolver the node itself uses — and MUST be able to declare
  more than one so the same targets are verified from multiple vantage points.
- **FR-007**: When no resolution vantage point is declared, the check MUST
  default to the cluster's own DNS service, and that default MUST be documented
  on the field rather than implied.
- **FR-008**: An operator MUST be able to override the resolution vantage point
  for an individual target, so that one check can assert both "internal names
  resolve internally" and "external names resolve upstream".
- **FR-009**: An explicitly addressed resolver MUST be constrained at write
  time to a routable address with an optional port; a resolver address that
  would itself require name resolution MUST be rejected.

**Evaluating intent**

- **FR-010**: Every record kind admissible under FR-003 MUST be evaluated as
  that kind of query. A declared kind MUST NOT be substituted, downgraded, or
  ignored at evaluation time.
- **FR-011**: An explicitly addressed resolver declared under FR-006 or FR-008
  MUST be queried directly, rather than deferring to the ambient resolver.
- **FR-012**: Expected-answer comparison and assertion polarity MUST be
  evaluated where the query is performed, so the outcome carries both the
  verdict and the evidence that produced it.
- **FR-013**: The published field documentation MUST describe only behavior
  that exists. Any capability named in the schema MUST be honored by the
  evaluation path in the same release, or MUST NOT appear in the schema.
- **FR-014**: The evaluation path MUST distinguish "the resolver could not be
  reached" from "the resolver answered that the name does not exist", and MUST
  never treat the former as satisfying a negative assertion.

**Cadence and bounds**

- **FR-015**: An operator MUST be able to declare how often the check runs and
  how long a single run may take, and both MUST default to documented values
  when unset.
- **FR-016**: The declared cadence MUST be rejected at write time when it is
  faster than the platform-wide minimum interval, and the declared run bound
  MUST be rejected when it is shorter than the platform-wide minimum timeout.
  These are the same floors the other check kinds enforce.
- **FR-017**: A run bound longer than the cadence MUST be rejected at write
  time.
- **FR-018**: Every string field and every list in the resource MUST carry an
  explicit maximum, so that no accepted object can impose unbounded cost on the
  operator or on admission-time evaluation.

**What the contract deliberately excludes**

- **FR-019**: The resource MUST NOT offer a field that suspends or pauses the
  check. Stopping a check means deleting it; a suspension field is being
  removed from the API surface generally and MUST NOT be reintroduced here.

**Reporting outcomes**

- **FR-020**: The resource MUST report a single current verdict drawn from the
  same outcome vocabulary as every other Fathom check, so that consumers need
  no DNSCheck-specific knowledge to interpret it.
- **FR-021**: The resource MUST report a human-readable one-line summary of the
  current outcome, bounded in length. When a failure stems from a negative
  assertion, the summary MUST make the polarity explicit so it is not misread
  as a resolution outage.
- **FR-022**: The resource MUST report per-target results, so an operator can
  tell which of several declared targets is failing without consulting history.
- **FR-023**: The resource MUST report when it was last evaluated, which
  on-demand trigger it last consumed, the most recent durable report it
  produced, and structured conditions covering whether its specification was
  accepted.
- **FR-024**: The resource MUST report the generation of the specification it
  last acted on, so consumers can distinguish a current verdict from one
  predating the latest edit.
- **FR-025**: A resolution failure under a positive assertion MUST be reported
  as a failing verdict, not as an operator-side error. Error is reserved for
  faults that are not the resolver's answer. This preserves the existing
  severity ordering so a real DNS outage cannot mask unrelated failures in the
  aggregate.

**Operability**

- **FR-026**: The current verdict, an indication of scope (how many targets are
  covered), the time of last evaluation, and the object's age MUST be visible
  in a standard listing without reading the full object.
- **FR-027**: The resource MUST be reachable by the same category grouping as
  the other Fathom check kinds, so a single listing command surfaces every
  check kind.
- **FR-028**: The resource MUST be introduced on its own version track at the
  earliest maturity level, and MUST NOT be promoted alongside the pre-existing
  kinds until it has field experience.
- **FR-029**: Introducing the resource MUST NOT alter the schema of any
  existing kind in a way the compatibility gate would flag.
- **FR-030**: Widening the resolution capability MUST NOT change the outcome of
  any existing check that uses it. The existing host-lookup behavior remains
  the default path and its failure classification is unchanged.

### Key Entities

- **DNSCheck**: An operator's declaration that a named set of DNS targets must
  (or must not) resolve, from declared vantage points, at a declared cadence.
  Owns its current verdict, per-target results, and pointers into its own
  history.
- **Target**: One declared subject plus the expectation attached to it — which
  record kind, whether the assertion is positive or negative, optionally which
  specific answers, and optionally which vantage point overrides the
  check-level default.
- **Resolver**: A declared vantage point from which resolution is attempted:
  the cluster's DNS service, an explicitly addressed upstream, or the node's
  own resolver.
- **Per-target result**: The outcome for one target/vantage-point pair on the
  most recent evaluation, with enough detail to say what was asked, what came
  back, and which expectation decided the verdict.
- **HealthReport** *(existing)*: The durable evidence record a check produces
  on outcome transitions; `DNSCheck` produces these like every other kind.
- **HealthCheck / ClusterHealth** *(existing)*: The wrapper and aggregate that
  turn an individual check's verdict into the cluster-wide verdict.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can express "these names must resolve, these must
  not, this often, from here, to these answers" in a single reviewable
  declaration, with no field left to convention or annotation.
- **SC-002**: 100% of the malformed-intent cases enumerated in Edge Cases are
  rejected at write time, and each rejection message names the offending field.
- **SC-003**: A well-formed declaration that omits every optional field is
  accepted, and every resulting effective value is discoverable from the
  published field documentation without reading source.
- **SC-004**: An operator can determine the current verdict and the covered
  scope of every DNS check in the cluster from one listing command, without
  opening an individual object.
- **SC-005**: No capability named in the published field documentation is
  unimplemented at release — the count of documented-but-unhonored fields is
  zero. Every record kind and every resolver vantage point in the schema has a
  test demonstrating it is evaluated as declared.
- **SC-006**: No accepted object can exceed the declared caps on list length or
  string length, and admission-time evaluation of the whole resource stays
  within the platform's per-resource validation cost budget.
- **SC-007**: Introducing the resource produces no compatibility-gate finding
  against any pre-existing kind, and no behavioral change to any check that
  already uses the resolution capability.

## Assumptions

- **Audience**: the operator authoring a `DNSCheck` is the same platform
  operator who authors the other Fathom check kinds, works through Git, and
  reviews changes as manifests rather than through a UI.
- **Consistency over novelty**: where this resource can look like the existing
  check kinds — cadence fields and their floors, outcome vocabulary, history
  retention, on-demand trigger, condition shape — it does, and deviations are
  called out rather than invented silently.
- **Expected-answer matching is containment, not equality**: a declared
  expected answer set is satisfied when every declared answer is present.
  Additional answers beyond those declared do not fail the check, because
  multi-address and round-robin records legitimately return supersets. An
  exact-set assertion is not offered in this release.
- **Default record kind is host lookup**: a target that does not name a record
  kind is evaluated as an address lookup satisfied by either address family
  (`Host`), matching both the prior behaviour of the resolution capability and
  the overwhelmingly common case.
- **Multiple vantage points fold to the worst outcome**: when the same targets
  are checked from several resolvers, the check's single verdict is the most
  severe per-pair outcome, matching the fold already used across Fathom.
  Per-pair detail remains available in the per-target results.
- **Retention follows the established default**: history retention behaves as
  it does on the existing kinds, with the same minimum that keeps the latest
  report referenceable.
- **Cluster DNS is discoverable without configuration**: resolving "from the
  cluster's DNS service" requires no operator-supplied address.
- **Scope spans contract and capability, not orchestration**: this slice
  delivers what an operator can declare and the evaluation that honors it. What
  drives evaluation on a cadence, persists its results, and aggregates them is
  the next slice.

## Dependencies

- **Cadence floors** are already established platform-wide and are reused
  verbatim rather than redefined.
- **Outcome vocabulary, history retention, and the on-demand trigger** are
  established by the existing check kinds.
- **Failure-vs-error classification for DNS** is already settled: a resolution
  failure is a failing verdict, not an error. This slice extends that ruling to
  negative assertions without disturbing it.
- **The existing resolution capability** is extended, not replaced; callers
  that use it today must observe no change (FR-030).
- **The compatibility gate** already exists and must pass with the new resource
  present.
- **Generated artifacts** — deep-copy code, resource manifests, RBAC, samples,
  and the published API reference — are produced only by the pinned tooling and
  are never hand-written.

## Out of Scope

- The controller that executes the check on a cadence, persists its results,
  and manages its history.
- Generalizing the wrapper so a `DNSCheck` can be mirrored into the
  cluster-wide aggregate.
- End-to-end coverage and the operator-facing guide.
- DNSSEC validation.
- Latency thresholds as a pass/fail criterion. Resolution latency may be
  recorded as evidence, but "resolution is slower than X" is not a declarable
  expectation in this release.
- Exact-set answer assertions (see Assumptions — containment is the rule).
- Record kinds beyond those in FR-003 (`TXT`, `MX`, `NS`, `CAA`).
</content>
