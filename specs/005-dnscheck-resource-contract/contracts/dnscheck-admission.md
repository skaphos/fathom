# Contract: DNSCheck Admission Validation

The externally observable accept/reject behavior of the API server for
`DNSCheck`. This is the contract the envtest matrix asserts and the generated
API reference documents. Field definitions and bounds live in
[data-model.md](../data-model.md); this file states only what a write does.

Consumers: operators authoring manifests, GitOps dry-run/diff, the envtest
validation matrix, `docs/reference/api.md`.

## Minimal accepted object

Every optional field omitted. Establishes the defaults an operator can rely on
without reading source (SC-003).

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: cluster-dns
spec:
  targets:
    - name: kubernetes.default.svc.cluster.local
```

Effective after defaulting: `recordType: Host`, `absent: false`,
`interval: 1m`, `timeout: 10s`, `historyLimit: 10`, and resolution from cluster
DNS. `Host` is satisfied by an address of either family, so this check does not
silently mean "IPv4 only".

## Fully populated object

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: dns-health
spec:
  interval: 1m
  timeout: 10s
  historyLimit: 20
  resolvers:
    - name: cluster
      from: Cluster
    - name: node
      from: Node
    - name: upstream
      from: Explicit
      address: 10.0.0.10:53
  targets:
    - name: kubernetes.default.svc.cluster.local
      recordType: A
    - name: api.internal.example.com.
      recordType: A
      expectedAnswers: ["10.20.30.40"]
      resolver: upstream
    - name: _https._tcp.example.com.
      recordType: SRV
      resolver: upstream
    - name: 10.20.30.40
      recordType: PTR
      resolver: upstream
    - name: decommissioned.example.com.
      absent: true
      resolver: upstream
```

## Accept / reject matrix

Each row is one envtest case. "Rejected" means the API server refuses the
write; the message must name the offending field (SC-002).

| # | Case | Outcome |
|---|------|---------|
| 1 | minimal object above | accepted, defaults as stated |
| 2 | `targets: []` | rejected — at least one target required |
| 3 | `targets` omitted | rejected — required field |
| 4 | 17 targets | rejected — exceeds `MaxItems=16` |
| 5 | 4 resolvers | rejected — exceeds `MaxItems=3` |
| 6 | `interval: 1ms` | rejected — below the 10s floor, message states the floor |
| 7 | `timeout: 100ms` | rejected — below the 1s floor |
| 8 | `timeout: 5m`, `interval: 1m` | rejected — names both fields |
| 9 | `timeout: 10s`, `interval: 10s` | accepted — equal is legal |
| 10 | `recordType: TXT` | rejected — names the supported set |
| 10a | `recordType` omitted | accepted, defaults to `Host` |
| 10b | `recordType: Host` | accepted |
| 11 | `absent: true` + `expectedAnswers: [...]` | rejected — contradictory intent |
| 12 | `absent: true`, no expected answers | accepted |
| 13 | `name: "not a hostname"` | rejected — malformed subject |
| 14 | `name: "http://example.com/path"` | rejected — malformed subject |
| 15 | `name: "example.com."` (trailing dot) | accepted |
| 16 | `name: "my-svc"` (single label) | accepted |
| 17 | `name: "_https._tcp.example.com."`, `recordType: SRV` | accepted — underscore labels survive |
| 18 | `recordType: PTR`, `name: "example.com"` | rejected — PTR subject must be an address |
| 19 | `recordType: PTR`, `name: "10.20.30.40"` | accepted |
| 20 | `recordType: A`, `name: "10.20.30.40"` | rejected — non-PTR subject must be a name |
| 21 | `from: Explicit`, no `address` | rejected — address required |
| 22 | `from: Cluster` + `address` set | rejected — address only valid for Explicit |
| 23 | `from: Explicit`, `address: "dns.example.com"` | rejected — must be an address, not a name |
| 24 | `from: Explicit`, `address: "10.0.0.10"` | accepted — port optional |
| 25 | `from: Explicit`, `address: "10.0.0.10:53"` | accepted |
| 26 | `from: Explicit`, `address: "[2001:db8::1]:53"` | accepted |
| 27 | two resolvers named `upstream` | rejected — duplicate map key |
| 28 | target `resolver: nope` with no such entry | rejected — must name a declared resolver |
| 29 | `spec.paused: true` | rejected — unknown field (structural schema) |
| 30 | `spec.policy: {}` | rejected — unknown field |
| 31 | `historyLimit: 0` | rejected — `Minimum=1` |
| 32 | 17 `expectedAnswers` on one target | rejected — exceeds `MaxItems=16` |

Rows 29 and 30 need no rule; a structural CRD schema rejects unknown fields.
They are asserted anyway, because FR-019 is a contract promise and a future
refactor could reintroduce the field without anyone noticing.

## Non-guarantees

Stated so the contract is not read as promising more than it does:

- **Reachability is not validated at admission.** A syntactically valid
  resolver address that no longer answers is accepted; that is a runtime
  verdict, not an admission error.
- **Subject existence is not validated at admission.** A name that does not
  resolve is the whole point of the check.
- **Cross-object uniqueness is not enforced.** Two `DNSCheck` objects may
  declare the same target.
- **`isIP()` availability**: row 18/19/23 rules depend on the API server's CEL
  IP library. If it is unavailable on the envtest server, a regex fallback
  delivers the same matrix (research R5) — the matrix is the contract, the
  mechanism is not.
</content>
