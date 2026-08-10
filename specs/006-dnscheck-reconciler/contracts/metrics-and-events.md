# Contract: DNSCheck Metrics, Events, and RBAC

**Feature**: 006-dnscheck-reconciler

The externally observable surface. Operators build alerts on this, so it is a
compatibility contract, not an implementation detail.

---

## 1. Check-level metrics (inherited, FR-032)

Emitted by the existing `observeCheck` with `kind="DNSCheck"`. No new code, and
no per-kind handling for consumers.

| Series | Labels | Semantics |
|---|---|---|
| `fathom_check_result` | `kind,name,namespace,result` | one-hot; exactly one series is 1 |
| `fathom_check_last_run_timestamp_seconds` | `kind,name,namespace` | unix time; 0 = never evaluated |

Series appear from first observation (`result="Unknown"`) and are removed by
`DeleteCheckSeries` when a reconcile sees the check is gone.

---

## 2. Per-target metric (new, FR-033)

```text
fathom_dnscheck_target_result{namespace,check,name,record_type,resolver,result}
```

One-hot per (target, vantage point) pair — the check-level gauge one level down,
so an operator can alert on a single failing name rather than only on the check
as a whole.

| Label | Value |
|---|---|
| `namespace` | the check's namespace |
| `check` | the `DNSCheck` name |
| `name` | the subject looked up |
| `record_type` | `Host`\|`A`\|`AAAA`\|`CNAME`\|`SRV`\|`PTR` |
| `resolver` | vantage-point name, or `cluster` |
| `result` | `Pass`\|`Warn`\|`Fail`\|`Error`\|`Skipped`\|`Unknown` |

**Ceiling: 288 series per check** (16 × 3 × 6), derivable from the published
schema without observing a running system (FR-034, SC-009). Raising a schema cap
raises this ceiling and **must** be treated as a cardinality change.

**Withdrawal (FR-036)**: each run deletes the check's series by partial match on
`{namespace, check}` and then sets the current pairs. A pair the spec no longer
declares is never re-set, so it disappears — no removal detection, and correct
across an operator restart.

A deleted check withdraws everything: check-level and per-target alike (FR-114).

### Why not reuse `fathom_check_result`

Its label set is fixed at `{kind,name,namespace,result}` and is consumed by
cluster-wide dashboards. Adding target labels there would multiply every other
kind's cardinality and break those consumers. A separate series keeps the
check-level contract stable.

---

## 3. Events (inherited, FR-112)

Emitted by `observeCheck`; DNSCheck gets the whole contract by calling it.

| Reason | Type | When |
|---|---|---|
| `ResultChanged` | Normal, or Warning at `Warn`+ | the folded verdict transitioned |
| *(Ready condition's own reason)* | Warning | `Ready` newly `False`, or its reason changed |
| `ReconcileError` | Warning | terminal reconcile error not already surfaced |
| `CadenceClamped` | Warning | stored cadence below the schema floors, raised at runtime |

A first evaluation reads as a transition **from** `Unknown`, so the initial
verdict produces an event rather than silence. A persistently failing check
produces one event per failure episode, not one per run.

---

## 4. Conditions

| Type | `True` | `False` |
|---|---|---|
| `Accepted` | spec accepted (reason `SpecClamped` if the cadence was raised) | — |
| `Ready` | the controller can evaluate the check | launch or RBAC failure, reason names which |
| `Complete` | every planned pair was reached | **run truncated** — message names the unreached count (FR-106a) |

`Complete` is the actionable half of truncation. The verdict degrading to
`Unknown` says *something* is wrong; `Complete=False` with a count says the run
bound was too small.

---

## 5. RBAC (FR-115 / inherited FR-037)

### Current state

The operator ClusterRole holds `pods: [delete, list]` only — from
`internal/probe/sweeper.go`. **It cannot create a pod.** Probe pods reach tenant
namespaces today solely by impersonating per-addon ServiceAccounts in the
operator's own namespace, and `DNSCheck` has no such identity to borrow.

### Required

```go
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;get;list;delete
```

| Verb | Why | Narrower alternative |
|---|---|---|
| `create` | place the probe pod in the check's namespace (FR-031) | none — this is the capability |
| `get` | `Launcher` polls the pod it created | none; `watch` is **not** requested and must not be |
| `list` | the orphan sweeper (pre-existing) | label scoping cannot be expressed in RBAC |
| `delete` | reclaim the pod (pre-existing) | none |

`watch` is deliberately absent. The launcher polls, and granting a cluster-wide
Pod watch would expose far more than pod placement requires.

### The justification document

FR-115 requires an in-repository document defending the grant. It must state:

1. Why cluster scope is unavoidable — a `DNSCheck` may exist in **any**
   namespace, and the set is unknown when RBAC is rendered.
2. Why per-namespace `Role`/`RoleBinding` provisioning is **worse**: it demands
   cluster-wide write on `roles` and `rolebindings` to achieve a narrower pod
   grant, trading a small privilege for a larger one.
3. Why impersonation does not transfer: the `AddonCheck` pattern needs a
   ServiceAccount in the target namespace, which the operator would have to
   create — the same class of cluster-wide write.
4. Why FR-031 forbids the simple alternative: running probes from the operator's
   namespace would let a check author borrow that namespace's egress posture.
5. That this cost is **inherited, not introduced** (FR-037): the planned
   reachability checks need the same grant with neither endpoint in the
   operator's namespace. DNSCheck is the first to need it, not the reason for it.
6. What is *not* granted: no `watch`, no `pods/exec`, no `pods/log`, no
   `pods/portforward`.
