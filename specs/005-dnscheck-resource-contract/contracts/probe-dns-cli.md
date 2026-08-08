# Contract: Probe `dns` Mode CLI

The invocation contract for `cmd/probe -mode dns` after this feature, and the
outcome it writes to the termination log. Consumers: `internal/probe.Pod()`
(which builds the argv), the future `DNSCheckReconciler` (#266), and the
existing `nodelocaldns` adapter, which must keep working unchanged (FR-030).

Outcome semantics derive from research R3; record-kind mechanics from R1.

## Flags

Existing flags are unchanged. New flags are all optional, and every default
reproduces today's behavior exactly.

| Flag | Values | Default | Meaning |
|------|--------|---------|---------|
| `-mode` | `dns` | — | unchanged |
| `-target` | subject | — | unchanged; a name, or an IP when `-record-type=PTR` |
| `-timeout` | duration | `10s` | unchanged |
| `-record-type` | `Host`\|`A`\|`AAAA`\|`CNAME`\|`SRV`\|`PTR` | `Host` | **new** — which query to issue |
| `-expect-answers` | comma-separated | empty | **new** — answers that must all be present |
| `-absent` | bool | `false` | **new** — assert the subject must NOT resolve |

**`Host` is the default and is exactly today's `LookupHost`** — both address
families, no narrowing. This is what protects the `nodelocaldns` caller
(FR-030): it passes no `-record-type`, gets `Host`, and behaves precisely as
before.

`A` and `AAAA` narrow to one family via `LookupIP(ctx, "ip4"|"ip6", …)` and are
reachable only by naming them. Making `A` the default would have changed the
existing consumer's behaviour from dual-family to IPv4-only — a silent
regression on a live check, and the concrete failure FR-030 forbids. The
resource schema defaults `recordType` to `Host` for the same reason, so the CRD
default and the probe default are one behaviour rather than two.

## Vantage point is not a probe flag

The resolver a query goes to is set by the **pod**, not by argv (research R2):

| Vantage | Pod spec | Status |
|---------|----------|--------|
| Cluster | `dnsPolicy: ClusterFirst` (inherited) | already the default |
| Node | `dnsPolicy: Default` | new, in `internal/probe.Pod()` |
| Explicit | `dnsPolicy: None` + `dnsConfig.nameservers` | already implemented |

The probe binary always queries whatever resolver its pod was given, and never
needs to know which of the three it is.

**Search-domain caveat**: under `Explicit`, the pod gets no cluster search
domains, so subjects must be fully qualified. The probe cannot detect or
compensate for this; it surfaces as an ordinary resolution failure.

## Outcome mapping

`Details` always carries `target`, `recordType`, and `latencyMillis`; `answers`
on success, `error` on failure.

| Situation | `-absent=false` | `-absent=true` |
|-----------|-----------------|----------------|
| Answers returned, expectations satisfied | `Pass` | `Fail` |
| Answers returned, an expected answer missing | `Fail` | *n/a — rejected at admission* |
| `net.DNSError` with `IsNotFound` | `Fail` | `Pass` |
| Zero answers, no error | `Fail` | `Pass` |
| `net.DNSError`, not `IsNotFound` (timeout, temporary) | `Fail` | **`Error`** |
| Not a `net.DNSError` | `Error` | `Error` |

The one asymmetry is deliberate and is FR-014: under a negative assertion, an
unreachable resolver proves nothing, so it must never be reported as `Pass`.
Under a positive assertion it stays `Fail`, preserving the ruling already
commented at `cmd/probe/main.go:77-89` — `Error` outranks `Fail` on the
severity ladder, so classifying a real outage as `Error` would mask genuine
failures elsewhere in the rollup.

## Per-record-kind behavior

| Kind | Call | Answers recorded | Notes |
|------|------|------------------|-------|
| `Host` *(default)* | `LookupHost` | addresses, either family | unchanged path — the FR-030 guarantee |
| `A` | `LookupIP(ctx, "ip4", …)` | IPv4 addresses | narrowing, explicit only |
| `AAAA` | `LookupIP(ctx, "ip6", …)` | IPv6 addresses | narrowing, explicit only |
| `CNAME` | `LookupCNAME` | canonical name | **no CNAME record ⇒ returns the subject itself; that is "absent", not a pass** |
| `SRV` | `LookupSRV(ctx, "", "", …)` | `target:port` per record | empty service/proto — otherwise the query name is rewritten |
| `PTR` | `LookupAddr` | names | subject is an address |

The two annotated rows are the traps identified in research R1. Both are cases
where the naive call compiles, runs, and reports the wrong answer.

## Answer matching

Containment, not equality (spec Assumptions): the check passes when every
declared expected answer is present. Extra answers do not fail it, because
multi-address and round-robin records legitimately return supersets.

Comparison is normalized before matching — trailing dots stripped, ASCII case
folded, IP addresses compared as parsed addresses rather than strings so
`10.20.30.40` and equivalent textual forms agree.

## Invariants

- **One result per invocation.** The existing termination-log contract holds:
  exactly one JSON document, written by `writeResult`.
- **`-absent` with `-expect-answers` is contradictory.** Admission rejects it,
  but the probe must not be trusted to only ever see valid input: it treats the
  combination as `Error` rather than silently preferring one.
- **No new dependencies.** Standard library only (research R1).
</content>
