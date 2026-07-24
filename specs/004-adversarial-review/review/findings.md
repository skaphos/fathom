# Adversarial Review Findings — v0.5.0 Release Gate (#217)

**Anchor commit**: cb845dd64fb480dacdb4c4363fbd4cb47bceea55
**Review dates**: 2026-07-24
**Perspectives**: security, correctness, api-contract, rbac, supply-chain

## Summary

34 candidate findings were raised across five independent perspective passes
and each was put through an adversarial refutation pass against the code at the
anchor commit. **No candidate was fully refuted** — the finders were
disciplined and every candidate describes a real code fact (see
[refuted.md](refuted.md) for the empty-refutation record and the two severity
adjustments made during refutation). Distribution after refutation: **0
critical, 4 high, 16 medium, 14 low** (API-2 was downgraded high→medium; RBAC-5
was reclassified from a high over-grant to an accepted, unavoidable capability).

Disposition of the critical/high set (the gate's mandate — FR-005):

| ID | Title | Severity | Disposition |
|----|-------|----------|-------------|
| API-1 | Mirror wedges on >1024-char summary | high | **fixed** — PR #252 |
| COR-1 | Truncated declarative run persists wrong verdict | high | **fixed** — PR #253 |
| RBAC-4 | Cluster-wide clusterroles/rolebindings write | high | **deferred** — issue #255 |
| RBAC-5 | daemonsets create/update/delete | high→accepted | **accepted** — inherent, documented below |

Additional in-milestone fix (clean constitutional win): **RBAC-1** (medium) —
PR #254. Confirmed mediums are tracked in issue #257; lows are recorded here for
opportunistic cleanup.

`go -C tools tool task ci` and `task crd-compat` are green at the anchor (see
[README.md](README.md)); each fix PR carries a regression test that fails
before the fix.

---

## Critical

_No confirmed findings._

## High

### API-1: HealthCheck status.summary MaxLength=1024 wedges mirroring on long condition messages (high)

- **Location**: `api/v1alpha1/healthcheck_types.go:83`; `internal/controller/healthcheck_controller.go:209,230-237`; source at `internal/controller/addoncheck_controller.go:439`
- **Perspective**: api-contract (duplicates merged: none)
- **Failure scenario**: An adapter `Run` fails with an error > 1024 chars (wrapped client/apiserver errors are additive and routinely exceed 1KiB). `addoncheck_controller.go:439` writes `runErr.Error()` verbatim into the AddonCheck Ready condition (bound 32768). `mirrorTarget` copies it untruncated into `HealthCheckStatus.Summary`, which has `MaxLength=1024`. The API server rejects the whole status update; `Reconcile` retries forever with the same payload — a permanent, non-self-healing mirror wedge that freezes the child's contribution to ClusterHealth at a stale verdict.
- **Refutation attempted**: Verified `summarizeFromConditions` (`:230-237`) returns `c.Message` with no truncation, `Summary` marker is 1024 (`healthcheck_types.go:83`), and source message is unbounded. The only counter — "no test exhibits a >1024 error today" — does not hold: nothing bounds `runErr.Error()` and the failure is deterministic once triggered. Survived.
- **Disposition**: **fixed (PR #252)**
- **Evidence**: rune-based truncation at the mirror boundary; regression test `TestSummarizeFromConditionsBoundsLength` fails before the fix (32768- and 4000-rune cases), passes after; `go test ./internal/controller/` green; CI `kind e2e` on the PR (e2e-mandatory surface).

### COR-1: Declarative engine silently truncates a Run on ctx expiry between families, persisting a wrong verdict from partial data (high)

- **Location**: `internal/adapter/declarative/engine.go:296-300`
- **Perspective**: correctness (duplicates merged: none)
- **Failure scenario**: For any engine-backed addon (externalsecrets, cilium, istio, keda, …), if `spec.timeout` expires in the gap between families, `Run` breaks and returns the partial check list **with `err == nil`**. The controller folds a verdict from only the families that ran first — e.g. a persisted Pass while the failing family never executed — with nothing marking the run truncated. Recurs every interval when an early family reliably consumes the whole timeout.
- **Refutation attempted**: Confirmed the deliberate `break`→nil-return path, and that `healthReportForAddonCheck:737-738` sets an Error verdict **only** when `runErr != nil`, so the nil return is exactly what produces the wrong Pass. Deadline-inside-a-family is separately safe (evaluator reads emit OutcomeError), but the inter-family gap is real and untested (`engine_test.go` had no cancel case). Survived.
- **Disposition**: **fixed (PR #253)**
- **Evidence**: engine now returns the context error on truncation, so an incomplete run records Error; regression test `TestEngine_TruncatedRunReturnsError` fails before (nil), passes after; declarative package green; CI `kind e2e` on the PR (e2e-mandatory surface).

### RBAC-4: Operator holds cluster-wide create/get/list/update/watch on ClusterRoles and RoleBindings (high)

- **Location**: `internal/controller/nodecertificatecheck_controller.go:152-153`; `config/rbac/role.yaml`
- **Perspective**: rbac (duplicates merged: none)
- **Failure scenario**: A compromised operator token can enumerate every ClusterRole in the cluster (full RBAC recon) and update any of them / create arbitrary ClusterRoles and RoleBindings — needed only to manage the single shared `fathom-node-agent-role` and per-check RoleBindings.
- **Refutation attempted**: Verified role.yaml grants and that the operator holds no cluster-wide escalate/bind/impersonate, so escalation-prevention bounds the *write* risk. But unrestricted read of all cluster RBAC + ClusterRole write is a lateral-movement enabler; and `+kubebuilder:rbac` cannot express `resourceNames`, so the breadth is real given the current cluster-scoped-role design. Survived at high.
- **Disposition**: **deferred (issue #255)**
- **Evidence**: fix is architectural (per-namespace Role dropping the `clusterroles` grant), touches the e2e-mandatory node-cert reconcile path, and is escalation-bounded — deferral rationale in #255.

### RBAC-5: Operator holds cluster-wide apps/daemonsets create/update/delete — a DaemonSet-on-every-node primitive (high → accepted)

- **Location**: `internal/controller/nodecertificatecheck_controller.go:144`; `config/rbac/role.yaml`
- **Perspective**: rbac (duplicates merged: none)
- **Failure scenario**: A compromised operator token can create an arbitrary (privileged, hostPath) DaemonSet on every node — full cluster compromise. Highest-value grant in the role.
- **Refutation attempted**: The finder itself flagged this as "genuinely required and unavoidable… included so reviewers consciously accept the blast radius, not because a tighter marker exists." Verified: the node-cert feature **is** a managed DaemonSet, the agent is created in arbitrary check namespaces (can't be namespace-scoped), `resourceNames` can't be expressed via markers, and the DaemonSet the operator authors is hardened (non-root, RO-rootfs, drop-ALL, seccomp, RO hostPath confined to admission-allowlisted prefixes). RBAC cannot constrain the *content* of a DaemonSet the operator is authorized to create. Survived as accurate, but there is no tighter grant to apply.
- **Disposition**: **accepted (documented)** — this is a necessary capability of the node-certificate feature, not a fixable over-grant. Recorded here as a conscious acceptance of blast radius per the review's mandate ("no silent coverage gaps"). If the node-cert DaemonSet feature is ever made optional/removable, this grant should be gated with it.

## Medium

Confirmed medium findings are tracked for follow-up in **issue #257** (not
release-blocking per FR-008). Each is listed with location; full failure
scenarios and refutation records are in the candidate files preserved under
this review (see [README.md](README.md) working records).

| ID | Title | Location |
|----|-------|----------|
| SEC-1 | Report-authenticity VAP exempts non-`*-node-agent` writers → report forgery | `nodecertificatecheck_controller.go:438-441,748-760` |
| SEC-2 | node-agent `/metrics` unauthenticated; leaks per-node cert inventory | `cmd/node-agent/main.go:82,120-125` |
| COR-2 | NodeCert ensure-failure sets Ready=False in memory but never persists it | `nodecertificatecheck_controller.go:233-260` |
| COR-3 | Incomplete-report window wipes last-known NodeCert verdict (churn + backward LastRunTime) | `nodecertificatecheck_controller.go:275-277,918-922` |
| COR-4 | Count-based completeness lets a departed node's report cover a new node's gap | `nodecertificatecheck_controller.go:908-916` |
| API-2 | Reserved ratio keys added without ContractVersion bump (downgraded high→medium) | `pkg/adapter/ratio.go:13-29`, `version.go:24` — deferred **issue #256** |
| API-3 | Optional top-level `spec` lets immutability CEL be bypassed via remove-then-re-add | `api/v1alpha1/*_types.go` |
| API-4 | AddonCheck `status.conditions` atomic (no `listType=map`), breaks SSA merge | `api/v1alpha1/addoncheck_types.go:116-119` |
| API-5 | CheckTargetRef docs advertise kinds the controller rejects; NodeCert excluded from ClusterHealth | `api/v1alpha1/healthcheck_types.go:12-14` |
| API-6 | `checkRef.apiVersion` unbounded and silently ignored | `api/v1alpha1/healthcheck_types.go:16-19` |
| RBAC-2 | node-agent role get/update any ConfigMap in namespace (no resourceNames) | `nodecertificatecheck_controller.go:345-349` |
| RBAC-3 | node-agent NetworkPolicy egress 443/6443 to any destination | `nodecertificatecheck_controller.go:531-536` |
| RBAC-6 | Impersonated addon SAs grant operator cluster-wide pod-create via side door | `config/rbac/addons/*.yaml` |
| SCM-1 | SBOMs are release assets only — not OCI-attached nor signed | `.github/workflows/release.yml:271-304` |
| SCM-2 | `release.yml` ships signed release from any `v*` tag, no on-main guard | `.github/workflows/release.yml:3-9` |

(API-2 is separately deferred with its own issue #256; the rest are in #257.)

## Low

Recorded for opportunistic cleanup (not tracked as issues per FR-008). All
survived refutation as real code facts; each finder's own refutation note caps
the severity.

| ID | Title | Location |
|----|-------|----------|
| SEC-3 | node-agent hostPath `DirectoryOrCreate` seeds root-owned dirs on nodes | `nodecertificatecheck_controller.go:629-636` |
| SEC-4 | User-controlled DNS targets/resolver drive arbitrary DNS egress from probes | `internal/adapter/coredns/adapter.go:470`, `nodelocaldns/adapter.go:409` |
| COR-5 | Status-update conflict causes a full adapter re-run (probe pods) on retry | `addoncheck_controller.go:264-272,278-289` |
| COR-6 | History pruning can delete the just-created report on same-second ties | `addoncheck_controller.go:533-544` |
| COR-7 | `nodeCertReportFresh` accepts future timestamps up to maxAge (2×maxAge fresh) | `nodecertificatecheck_controller.go:883-891` |
| COR-8 | HealthCheck watch mapping swallows List errors silently | `healthcheck_controller.go:261-264` |
| COR-9 | #248 transient path stamps new ObservedGeneration over prev checkRef's fields | `healthcheck_controller.go:108,195-204` |
| API-7 | `timeout <= interval` CEL unenforced when interval unset (no schema default) | `api/v1alpha1/addoncheck_types.go:63` |
| API-8 | Ratio-threshold doc misattributes range validation; out-of-range stops whole check | `api/v1alpha1/addoncheck_types.go:47-56` |
| API-9 | AddonCheck and HealthReport have no printcolumns | `api/v1alpha1/addoncheck_types.go:158-159`, `healthreport_types.go:157-158` |
| RBAC-7 | Unused `<kind>/finalizers` update grants (no finalizer logic exists) | `config/rbac/role.yaml`; markers across all four reconcilers |
| SCM-3 | Release/e2e tool binaries verified only against same-origin checksums file | `.github/workflows/release.yml:45-75`, `e2e.yml:97-111` |
| SCM-4 | `checkout` lacks `persist-credentials: false`; write token persists across steps | `.github/workflows/release.yml:37-39` et al. |
| SCM-5 | Coverage filter `grep -v /e2e` is a substring match (latent package exclusion) | `Taskfile.yml:206` |

---

## Disposition index

- **Fixed in v0.5.0**: API-1 (#252), COR-1 (#253), RBAC-1 (#254).
- **Deferred with rationale**: RBAC-4 (#255), API-2 (#256).
- **Accepted (inherent capability)**: RBAC-5.
- **Tracked mediums**: #257.
- **Lows**: documented above for opportunistic cleanup.

RBAC-1 (medium, PR #254): the operator ClusterRole granted
`create;update;patch;delete` on addonchecks/clusterhealths/healthchecks that the
reconcilers never exercise (they only Get/List/Watch + write `/status`). Trimmed
to `get;list;watch`; fixed opportunistically because it directly serves the
constitution's minimal-RBAC constraint and carries no runtime-behavior risk.
