# Refuted Candidates — v0.5.0 Release Gate (#217)

Every one of the 34 candidates raised by the five perspective passes was put
through an adversarial refutation pass: the code at the anchor commit was
re-read (not the finder's summary) and the strongest counter-argument was
constructed for each. **No candidate was fully refuted** — each describes a real
code fact. The finders' own refutation notes (retained in the candidate working
records) already pre-empted the weak cases, which is why the surviving set is
high-signal.

Two candidates had their **severity adjusted** during refutation rather than
being dropped:

| ID | Adjustment | Refutation reasoning |
|----|-----------|----------------------|
| API-2 | high → **medium** | The reserved `warnRatio`/`failRatio` keys and unchanged `ContractVersion = "1.0.0"` are real, but the impact requires a hypothetical out-of-tree adapter (pre-#241) that used those exact key names with different semantics. No in-tree adapter consumes them and Fathom is pre-1.0 with no published external adapters, so the runtime collision is currently hypothetical — a contract-hygiene issue, not an active defect. Deferred as issue #256. |
| RBAC-5 | high → **accepted** | The cluster-wide `daemonsets create/update/delete` grant is accurate and high-blast-radius, but it is inherent to the managed-DaemonSet node-cert feature: the agent is created in arbitrary namespaces, `resourceNames` can't be expressed via `+kubebuilder:rbac`, and the DaemonSet content is hardened. There is no tighter marker to apply, so it is a consciously accepted capability rather than a fixable over-grant. Documented in findings.md. |

## Fully refuted candidates

_None._ All 34 candidates survived as real findings; see
[findings.md](findings.md) for the ranked list and dispositions.
