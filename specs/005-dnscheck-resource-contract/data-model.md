# Phase 1 Data Model: DNSCheck Resource Contract

**Feature**: `specs/005-dnscheck-resource-contract`
**Date**: 2026-08-08

Shapes below are the intended schema. Go field names and JSON tags are given
because they *are* the contract; the generated CRD, deep-copy, RBAC, samples,
and `docs/reference/api.md` all derive from them and are never hand-edited.

---

## Entity: `DNSCheck`

`api/v1alpha1/dnscheck_types.go`, group `fathom.skaphos.io`, version
`v1alpha1`, namespaced. Modelled on `NodeCertificateCheck` for status shape and
on `AddonCheck` for cadence, with the deliberate divergences noted.

### `DNSCheckSpec`

| Field | Type | Req | Default | Bounds / rules |
|-------|------|-----|---------|----------------|
| `targets` | `[]DNSTarget` | yes | — | `MinItems=1`, `MaxItems=16` (R6) |
| `resolvers` | `[]DNSResolver` | no | cluster DNS | `MaxItems=3` (R6) |
| `interval` | `*metav1.Duration` | no | `1m` | `>= 10s` (`MinCheckInterval`) |
| `timeout` | `*metav1.Duration` | no | `10s` | `>= 1s` (`MinCheckTimeout`), `<= interval` |
| `historyLimit` | `*int32` | no | `10` | `Minimum=1` |

**Deliberately absent**: `paused`. FR-019 — stopping a check means deleting it.
Every other check kind still carries `paused`; this one is the first authored
without it, ahead of its removal elsewhere in #262. A structural CRD schema
rejects the unknown field automatically, so no rule is needed to enforce this.

**Deliberately absent**: `policy`. The issue text asked for "spec.policy
consistent with the other kinds", but the only kind that has one is
`AddonCheck`, where it selects *adapter-defined check families*
(`addoncheck_types.go:88-97`). `DNSCheck` has no families — the targets list is
the unit of selection, and a policy map would be an empty abstraction with no
consumer. Adding it would violate YAGNI and publish a field with no behavior,
which FR-013 forbids. Recorded here rather than silently dropped.

**Default interval of `1m`** rather than `AddonCheck`'s `5m`: DNS failures are
fast-onset and cheap to test. It is above the `10s` floor with room to spare.

### `DNSTarget`

One declared subject plus the expectation attached to it.

| Field | Type | Req | Default | Bounds / rules |
|-------|------|-----|---------|----------------|
| `name` | `string` | yes | — | `MinLength=1`, `MaxLength=253`, DNS-name or IP pattern |
| `recordType` | `DNSRecordType` | no | `A` | `Enum=A;AAAA;CNAME;SRV;PTR` |
| `expectedAnswers` | `[]string` | no | — | `MaxItems=16`, items `MaxLength=253`, `listType=set` |
| `absent` | `*bool` | no | `false` | polarity — `true` asserts the name must NOT resolve |
| `resolver` | `*string` | no | — | names an entry in `spec.resolvers`; `MaxLength=63` |

**`absent` over a `polarity` enum**: a two-valued enum (`Present`/`Absent`)
carries no more information than a bool and reads worse at the call site. The
field is named `absent` rather than `mustNotResolve` to match the vocabulary
already used in `AddonCheckStatus.Absent` for "target not installed"
(`addoncheck_types.go:130-137`).

**`resolver` is a reference, not an inline address.** FR-008 allows overriding
the vantage point per target. Referencing a named entry in `spec.resolvers`
keeps a single place where resolver addresses are declared and validated, and
avoids the same upstream being spelled three different ways across a target
list. The referential rule — "`resolver` must name a declared entry" — is a
cross-field CEL check.

**Validation rules on this entity:**

1. `absent == true` ⇒ `expectedAnswers` must be empty (FR-005, contradictory
   intent). Cheap: a per-item `.all()` over a 16-item bound.
2. `recordType == PTR` ⇒ `name` must be an IP address; otherwise `name` must be
   a DNS name (FR-002, R5).

### `DNSResolver`

A declared vantage point.

| Field | Type | Req | Default | Bounds / rules |
|-------|------|-----|---------|----------------|
| `name` | `string` | yes | — | `MaxLength=63`, DNS-1123 label; unique within the list |
| `from` | `DNSResolverSource` | no | `Cluster` | `Enum=Cluster;Node;Explicit` |
| `address` | `string` | cond | — | required iff `from == Explicit`; `MaxLength=45+port` |

**Validation rules:**

1. `from == Explicit` ⇒ `address` set; `from != Explicit` ⇒ `address` empty.
2. `address` is `IP` or `IP:port` — never a hostname (FR-009). A resolver
   address that itself needs resolving is a bootstrapping trap.
3. `name` unique across the list (`listType=map`, `listMapKey=name` gives this
   for free at the API-server level rather than costing a CEL rule).

**The `Explicit` foot-gun, documented on the field** (R2): an explicit resolver
runs the probe pod with `dnsPolicy: None`, which supplies *no cluster search
domains*. Short names that resolve fine under cluster DNS will not resolve
here. Targets checked against an `Explicit` resolver must be fully qualified.
This is stated on the field doc so it lands in `kubectl explain` and the
generated reference, not only in a guide.

### `DNSCheckStatus`

Mirrors the established shape. Fields marked *(same as existing kinds)* are
copied deliberately so consumers need no per-kind knowledge (FR-020).

| Field | Type | Notes |
|-------|------|-------|
| `observedGeneration` | `int64` | *(same)* FR-024 |
| `conditions` | `[]metav1.Condition` | *(same)* `listType=map`, key `type`; `Accepted` |
| `lastRunTime` | `*metav1.Time` | *(same)* FR-023 |
| `lastResult` | `string` | *(same)* `Enum=Pass;Warn;Fail;Error;Skipped;Unknown` |
| `summary` | `string` | `MaxLength=1024`; names polarity on a negative failure (FR-021) |
| `lastReportName` | `string` | *(same)* `MaxLength=253` |
| `lastRunTrigger` | `string` | *(same)* FR-023, run-now annotation |
| `targetResults` | `[]DNSTargetResult` | FR-022; `MaxItems=48` (16 targets × 3 resolvers) |
| `observedTargets` | `int32` | scope for the printer column (FR-026) |

### `DNSTargetResult`

One (target, resolver) pair on the most recent evaluation.

| Field | Type | Notes |
|-------|------|-------|
| `name` | `string` | the subject queried, `MaxLength=253` |
| `recordType` | `DNSRecordType` | echoed so a result is self-describing |
| `resolver` | `string` | which declared vantage point, `MaxLength=63` |
| `result` | `string` | `Enum=Pass;Warn;Fail;Error;Skipped;Unknown` |
| `message` | `string` | `MaxLength=512`, what was asked and what came back |
| `answers` | `[]string` | `MaxItems=16`, items `MaxLength=253` — the evidence |
| `latencyMillis` | `int64` | recorded as evidence; **not** a pass/fail input |

**Result identity is `(name, recordType, resolver)`**, which is what makes the
spec's duplicate-target Edge Case deterministic: the same name declared twice
against different resolvers produces two distinct entries rather than
colliding. Declared as `listType=map` with those three keys so the API server
enforces it.

**`latencyMillis` is evidence only.** The spec puts latency thresholds out of
scope; recording the number without acting on it is the honest middle ground
and matches what `runDNS` already emits (`cmd/probe/main.go:74`).

---

## Printer columns (FR-026)

Matching `NodeCertificateCheck`'s pattern
(`nodecertificatecheck_types.go:171-174`):

| Column | JSONPath |
|--------|----------|
| `Result` | `.status.lastResult` |
| `Targets` | `.status.observedTargets` |
| `Last Run` | `.status.lastRunTime` |
| `Age` | `.metadata.creationTimestamp` |

Plus `+kubebuilder:resource:categories=...` for FR-027 — matching whatever
category the existing kinds already declare, so one listing surfaces every
check kind.

---

## Changes to existing types

### `internal/probe.Request` — vantage point selector

`DNSNameservers` (`internal/probe/pod.go:76`) already delivers the `Explicit`
case. Two additions:

| Field | Type | Purpose |
|-------|------|---------|
| `DNSFrom` | `DNSSource` (`Cluster`/`Node`/`Explicit`) | selects the vantage point |
| `RecordType`, `ExpectedAnswers`, `Absent` | | passed through to probe flags |

`DNSFrom: Node` sets `dnsPolicy: Default`; `Explicit` keeps today's
`dnsPolicy: None` + `dnsConfig.nameservers` behavior.

**Backward compatibility (FR-030)**: the zero value of `DNSFrom` must behave
exactly as today. `Cluster` is the zero value, and `DNSNameservers` non-empty
must continue to imply `Explicit` even when `DNSFrom` is unset, so the existing
`nodelocaldns` caller (`internal/adapter/nodelocaldns/adapter.go:455`) keeps
working untouched. This is the single highest-risk edit in the slice — it is a
shared code path with a live consumer — and it is why FR-030 exists.

### `cmd/probe` — dns mode flags

New flags on the existing `dns` mode; every one optional, defaults reproducing
current behavior. The full CLI contract is in
[`contracts/probe-dns-cli.md`](contracts/probe-dns-cli.md).

### `PROJECT`

One new resource entry for `DNSCheck`, matching the existing kinds' shape.

---

## What this model does not include

- No controller, no reconciler, no watch wiring — #266.
- No `CheckTargetRef` change to make `DNSCheck` mirrorable — #267.
- No `HealthReport` production; the status fields that point at reports exist,
  but nothing writes them until the controller lands.
</content>
