<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Operator RBAC

This page justifies every rule in the operator's own ClusterRole
(`config/rbac/role.yaml`, generated from the `+kubebuilder:rbac` markers), under
the same doctrine that governs [addon adapter RBAC](rbac.md): **every grant
carries a defensive justification** — why it is needed and why a narrower grant
would not suffice. A guard test
(`internal/controller/operator_rbac_doc_test.go`) fails when a rule is added,
widened, or removed without updating the table below, so this page cannot
silently drift from the manifest.

The operator ServiceAccount is `fathom-controller-manager` (namespace
`fathom-system`). It deliberately holds **no read access to addon workloads and
no Secret access of any kind**: addon checks run through per-addon impersonated
ServiceAccounts (see [Addon adapter RBAC](rbac.md)), so each addon's blast
radius is its own declared rules, not the operator's.

## Why these grants are cluster-scoped

Fathom's check resources (`HealthCheck`, `AddonCheck`, `NodeCertificateCheck`,
`ClusterHealth`, `HealthReport`) are **namespaced CRDs that may be created in
any namespace**. Everything the operator provisions at runtime follows the
check into its namespace: the node-agent DaemonSet, its ServiceAccount and
RoleBinding, its NetworkPolicy, and the per-node report ConfigMaps the agents
publish. A namespaced Role in `fathom-system` would silently break every check
created outside `fathom-system`, so the grants on those kinds must be
cluster-scoped. The [namespace-scoping analysis](#namespace-scoping-analysis)
below covers what bounds the blast radius in practice and what a restricted
install can do.

## Operator ClusterRole rules

API group `core` is the empty group (`""`). Each row corresponds to exactly one
rule in `config/rbac/role.yaml`.

<!-- operator-clusterrole-table:begin -->

| API group | Resources | Verbs | Justification (why this, and why not less) |
| --- | --- | --- | --- |
| core | configmaps, serviceaccounts | create, get, list, update, watch | ConfigMaps: node-agents publish one report ConfigMap per node in the check's namespace; the controller lists and watches them **by Fathom labels only** to roll reports up into a HealthReport, and updates them to stamp the owner reference that garbage-collects them with the check. The operator never creates a report ConfigMap itself — `create` is forced by RBAC escalation prevention: the operator creates the `fathom-node-agent-role` ClusterRole conferring `create`/`get`/`update` on ConfigMaps, and the API server requires a grantor to hold every verb it confers. ServiceAccounts: the per-check node-agent ServiceAccount is created/converged via CreateOrUpdate (owner-referenced; deletion rides garbage collection), and AddonCheck reconciliation lists the static per-addon impersonation ServiceAccounts by label. No `patch`, no `delete` on either kind; the shared informer only caches objects labeled `fathom.skaphos.io/managed-by=fathom`, so foreign ConfigMaps are never cached or read in practice. ConfigMaps are not Secrets — the operator holds no Secret grant anywhere. |
| core | pods | delete, list | The leader-elected probe sweeper (ADR-0003, #163): probe pods are created in the *target addon's* namespace by the impersonated addon ServiceAccounts (never by the operator), and the launcher deletes them inline — but an operator crash between create and deferred delete would leak them. The sweeper lists pods cluster-wide by the reserved probe labels and deletes only pods that also match the probe pod shape (single `probe` container). `list` cannot be namespaced because probes follow the addon under check; no `get`/`watch`/`create`/`update` — the sweep is a bounded periodic list, and pod creation authority stays with the per-addon ServiceAccounts. |
| core, events.k8s.io | events | create, patch | The Kubernetes Events contract (#154): check-result transitions and operational failures are recorded as Events on the check resources. `patch` is how the recorder folds repeats into an EventSeries instead of minting a new object per occurrence. Both API groups are required because the events.k8s.io recorder resolves through either depending on cluster version (#243). |
| admissionregistration.k8s.io | validatingadmissionpolicies, validatingadmissionpolicybindings | create, get, list, update, watch | The report-authenticity policy (#155): a cluster-scoped CEL policy binding each node-report ConfigMap to the writing node-agent's ServiceAccount-token node claim, so a compromised node cannot forge or suppress another node's certificate verdict. Created at runtime (not shipped statically) so kustomize namePrefix and OLM transforms cannot rename the policy out from under its binding. Creating a VAP confers no privilege of its own, so this does not trip the RBAC escalation check. No `delete`: the policy is a stable singleton that stays correct across check churn. |
| apps | daemonsets | create, delete, get, list, update, watch | The per-check node-agent DaemonSet is provisioned and converged at runtime in the check's namespace. `delete` is used exactly once: a paused check tears its DaemonSet down while leaving RBAC and reports in place. `watch` (via Owns) repairs drift and deletion; no `patch` — all writes are CreateOrUpdate. The informer cache is label-scoped to `fathom.skaphos.io/managed-by=fathom`, so only Fathom's own DaemonSets are cached. |
| fathom.skaphos.io | addonchecks, clusterhealths, healthchecks | create, delete, get, list, patch, update, watch | The operator's own primary resources: full lifecycle authority is the reconciler contract — HealthCheck materializes and prunes the AddonChecks its spec implies, and ClusterHealth is derived from HealthCheck status. Scoped to exactly the three kinds the controllers own. |
| fathom.skaphos.io | addonchecks/finalizers, clusterhealths/finalizers, healthchecks/finalizers, nodecertificatechecks/finalizers | update | Finalizer maintenance on the operator's own kinds, required for owner-reference and deletion-flow correctness. `update` is the only verb the finalizer subresource supports. |
| fathom.skaphos.io | addonchecks/status, clusterhealths/status, healthchecks/status, nodecertificatechecks/status | get, patch, update | Status subresource writes for the operator's own kinds — conditions, observedGeneration, and roll-up state. Status is written through the dedicated subresource so spec and status authority stay separate. |
| fathom.skaphos.io | healthreports | create, delete, get, list, watch | HealthReport history (ADR-0002): reports are created on result transitions and deleted only by retention pruning (`historyLimit`). Deliberately **no `update`/`patch`** — a persisted report is immutable evidence. |
| fathom.skaphos.io | nodecertificatechecks | get, list, patch, update, watch | Reconciling user-created NodeCertificateChecks: spec reads plus metadata writes (owner references, finalizer bookkeeping). Deliberately **no `create`/`delete`** — the operator never creates or removes user checks. |
| networking.k8s.io | networkpolicies | create, get, list, update, watch | The per-check NetworkPolicy that isolates node-agent pods (#153): metrics ingress only from namespaces labeled `metrics: enabled`, egress only to the API server. It must be authored at runtime because it lives in the check's namespace, which is known only when the check is created. The policy's pod selector is fixed by the controller to the agent labels, so the operator only ever isolates its own pods. No `delete` — the policy is owner-referenced and garbage-collected with the check. |
| rbac.authorization.k8s.io | clusterroles, rolebindings | create, get, list, update, watch | Node-agent provisioning: the operator creates the `fathom-node-agent-role` ClusterRole at runtime (a static manifest would be renamed by kustomize namePrefix/OLM transforms, breaking every RoleBinding that references it) and one RoleBinding per check in the check's namespace. The ClusterRole grants only `create`/`get`/`update` on ConfigMaps and is only ever bound via namespaced RoleBindings, so its verbs never apply cluster-wide. Escalation prevention holds because the operator already possesses every verb it confers. **No ClusterRoleBinding grant, no `bind`, no `escalate`, no `delete`** — the RoleBinding is garbage-collected with its check and the ClusterRole is a stable singleton. |

<!-- operator-clusterrole-table:end -->

## Namespace-scoping analysis

This is the "hard look" the 1.0 security review asked for (#153): which of the
cluster-wide grants could be replaced by a namespaced Role in `fathom-system`,
and what bounds the ones that cannot.

**Why not a namespaced Role.** The ConfigMap, ServiceAccount, RoleBinding,
DaemonSet, and NetworkPolicy grants exist for `NodeCertificateCheck`
provisioning, and that CRD is namespaced by design — the managed resources are
created in `check.Namespace` so they are garbage-collected with the check and
so multiple teams can own their own checks. Scoping those grants to
`fathom-system` would make every check outside `fathom-system` fail RBAC at
reconcile time. The `pods` grant follows probe pods into whatever namespace
the addon under check occupies, which is precisely the property that makes the
probe signal representative (ADR-0003).

**What bounds the blast radius instead:**

- **Verb minimalism.** No `patch`/`delete` on ServiceAccounts, RoleBindings,
  ClusterRoles, or NetworkPolicies; no `update` on HealthReports; no
  `create`/`delete` on user checks; no Secret access at all.
- **Cache label-scoping.** The shared informer caches ConfigMaps, DaemonSets,
  RoleBindings, and NetworkPolicies only when labeled
  `fathom.skaphos.io/managed-by=fathom` (`internal/app/run.go`,
  `scopedCacheOptions`), so the operator neither holds foreign objects in
  memory nor reads them on its cached paths (SKA-581 / #164).
- **Report authenticity.** The cluster-wide ConfigMap write surface for
  node-agents is policed by the report-authenticity
  ValidatingAdmissionPolicy (#155): an agent token can only publish a report
  attributed to its own node.
- **Impersonation for addon reads.** Addon state is read through per-addon
  ServiceAccounts the operator may only `impersonate`
  ([Addon adapter RBAC](rbac.md)), never through the operator's own role.

**Restricted installs.** A cluster that runs checks only in `fathom-system`
can tighten further by replacing the ClusterRole rules for
`configmaps`/`serviceaccounts`/`daemonsets`/`rolebindings`/`networkpolicies`
with an equivalent Role + RoleBinding in `fathom-system` via a kustomize
overlay. The operator degrades loudly, not silently: a check in another
namespace surfaces `Forbidden` errors in its `Ready` condition. This overlay is
deliberately not the default because it changes the CRD's documented contract;
if you want it shipped as a supported variant, please open an issue.

## Auxiliary roles shipped alongside the operator

These are static manifests under `config/rbac/`, separate from the manager
ClusterRole:

- **`leader-election-role`** (namespaced Role in `fathom-system`): Leases and
  ConfigMaps CRUD plus Event creation, the standard controller-runtime leader
  election contract. Namespaced, so the lease surface never leaves
  `fathom-system`.
- **`metrics-auth-role`** (ClusterRole): `create` on TokenReviews and
  SubjectAccessReviews — the operator's metrics endpoint authenticates and
  authorizes scrapers in-process (`filters.WithAuthenticationAndAuthorization`).
  These are ephemeral review APIs; the role reads no cluster state.
- **`metrics-reader`** (ClusterRole): `get` on the `/metrics` non-resource URL.
  Not bound by default; bind it to your monitoring ServiceAccount to authorize
  scrapes.
- **`fathom-addon-impersonator`** (namespaced Role in `fathom-system`):
  `impersonate` on the per-addon ServiceAccounts only — see
  [Addon adapter RBAC](rbac.md#operator-impersonation-grant).
- **Per-addon ServiceAccounts and roles** (`config/rbac/addons/`): generated
  from adapter declarations; every grant is justified in
  [Addon adapter RBAC](rbac.md).
- **`{clusterhealth,healthcheck,healthreport,nodecertificatecheck}-{admin,editor,viewer}`**
  ClusterRoles: aggregation-label convenience roles for cluster admins to hand
  out. Not bound by default and not used by the operator.

## Runtime-created RBAC

The operator creates exactly one RBAC object kind pair at runtime, both for the
node-agent:

- **`fathom-node-agent-role`** (ClusterRole, singleton): `create`, `get`,
  `update` on ConfigMaps — the exact verbs `cmd/node-agent` uses to upsert its
  own report ConfigMap. No `list`/`watch`/`patch`/`delete`, so an agent (which
  runs on every node) cannot enumerate or tamper with other ConfigMaps even
  within its check's namespace. Created at runtime so its name survives
  kustomize/OLM name rewriting.
- **`<check>-node-agent`** (RoleBinding, per check): binds the ClusterRole to
  the per-check agent ServiceAccount **in the check's namespace only** — the
  ClusterRole's verbs never apply cluster-wide.
