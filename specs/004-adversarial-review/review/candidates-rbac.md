# RBAC / Least-Privilege Review — Fathom operator (commit cb845dd)

Reviewer perspective: RBAC least-privilege only. Cross-checked every
`+kubebuilder:rbac` marker against actual client calls in the reconcilers, read
the generated `config/rbac/role.yaml`, all static roles, the per-addon
ClusterRoles + impersonation Role, the runtime node-agent ClusterRole /
RoleBinding / NetworkPolicy the controller synthesizes, and the node-agent
DaemonSet pod spec.

---

### RBAC-1: Operator ClusterRole grants create/update/patch/delete on the three primary CRDs the reconcilers never write (medium)
- Location: markers `internal/controller/healthcheck_controller.go:60`, `internal/controller/clusterhealth_controller.go:64`, `internal/controller/addoncheck_controller.go:129`; generated `config/rbac/role.yaml:55-68`
- Failure scenario: A compromised operator ServiceAccount token can delete every `HealthCheck`, `ClusterHealth`, and `AddonCheck` in the cluster (availability/integrity loss for the whole product), or forge spurious ones — capabilities the operator never exercises in normal operation, so the grant is pure attack surface.
- Evidence: role.yaml grants `create;delete;get;list;patch;update;watch` on `addonchecks, clusterhealths, healthchecks`. But the reconcilers only ever `Get`/`List`/`Watch` these and write **status**: `healthcheck_controller.go:148` and `clusterhealth_controller.go:146` both call `r.Status().Update(...)` only; a repo-wide grep for `r.Create`/`r.Delete`/`r.Update`/`r.Patch` in `internal/controller/*.go` finds no create/delete/spec-update of any of these three primary kinds (the only primary-object `r.Update` is the ConfigMap owner-ref adoption at `nodecertificatecheck_controller.go:791`; the only `r.Delete`s target HealthReports and the node-agent DaemonSet; the only `r.Create` is a HealthReport). The `/status` and `/finalizers` subresources are granted separately, so the top-level `create;update;patch;delete` verbs are unused.
- Refutation notes: Strongest counter — these are Fathom's *own* CRDs, so blast radius is bounded to product resources, not cluster takeover (hence medium not high). One might argue the default kubebuilder scaffold ships full CRUD and it's harmless; but `healthreports` in the same role WAS trimmed to `create;get;list;watch;delete` (no update/patch — exactly what its code uses), proving the project trims markers deliberately, and #153 claims to have "justified and trimmed" the ClusterRole. These three kinds were missed. `update`/`patch` on the primary object would only be needed if a reconciler set finalizers or labels on it — no reconciler calls `AddFinalizer`/`RemoveFinalizer` anywhere, so even the finalizer justification does not apply.

---

### RBAC-2: Runtime node-agent ClusterRole allows get/update of ANY ConfigMap in the operator namespace, though the agent writes exactly one (medium)
- Location: `internal/controller/nodecertificatecheck_controller.go:345-349` (the synthesized `fathom-node-agent-role` ClusterRole); bound per-check via RoleBinding at `:475-483`
- Failure scenario: The node-agent runs on *every* selected node (large, worker-node-exposed token surface). Its token grants `get` + `update` on all ConfigMaps in the check namespace with no `resourceNames`. A compromised agent can read the operator's own mounted config ConfigMap (`/etc/fathom/config.yaml`, a well-known name) and overwrite any other ConfigMap in that namespace — e.g. an addon config or the operator config — while it legitimately only ever touches its single deterministic report ConfigMap.
- Evidence: `role.Rules = []rbacv1.PolicyRule{{APIGroups:{""}, Resources:{"configmaps"}, Verbs:{"create","get","update"}}}` — no `ResourceNames`. The agent's actual write is one CM named `nodecert.NodeReportConfigMapName(checkName, nodeName)` (`cmd/node-agent/main.go:284`, upsert at `:167-189`); the package doc at `cmd/node-agent/main.go:11` states it "writes exactly one ConfigMap (its own)". The ValidatingAdmissionPolicy (`:417-458`) only constrains writes to ConfigMaps carrying the fathom node-report labels (`ObjectSelector` at `:423-426`), so a `get`/`update` on any *unlabelled* ConfigMap is not covered by the VAP at all.
- Refutation notes: `create` genuinely cannot be scoped by `resourceNames` (create-time RBAC has no name to match), and the per-node report name is dynamic across all checks/nodes so a shared ClusterRole can't enumerate them — this is a real Kubernetes limitation and the comment at `:341-344` acknowledges it (drops list/watch/patch to prevent enumeration/tamper). But `get` and `update` *do* support `resourceNames`, and a per-check RoleBinding-scoped Role could be minted per-namespace with the deterministic name set — the current design leaves read/overwrite of arbitrary namespace ConfigMaps on the table. ConfigMaps are not Secrets, which caps severity at medium.

---

### RBAC-3: Node-agent NetworkPolicy egress permits TCP/443 + TCP/6443 to ANY destination, not just the API server (medium)
- Location: `internal/controller/nodecertificatecheck_controller.go:531-536` (`ensureAgentNetworkPolicy`)
- Failure scenario: The egress rule specifies ports but no `To` peers, so under Kubernetes NetworkPolicy semantics it allows those ports to *every* destination (0.0.0.0/0). A compromised node-agent can exfiltrate scanned certificate material — or beacon — to any external host reachable on 443/6443, despite the policy's stated intent to confine the agent to the API server.
- Evidence: The rule is `Egress: [{Ports: [{TCP,443},{TCP,6443}]}]` with an empty `From/To`. The doc comment at `:490-500` asserts egress is "only the API server ports … the agent talks to nothing else," but destination IP is unconstrained. Contrast the ingress rule directly above (`:523-530`) which *does* pin a `NamespaceSelector` peer.
- Refutation notes: Pinning `To` to the API server is genuinely awkward — the ClusterIP/endpoint IPs vary per cluster and the code already notes (`:504-510`) it must allow both the Service port (443) and the post-DNAT endpoint port (6443) because CNIs police differently. An `ipBlock` for the API server would need cluster-specific values. Still, 443-to-anywhere is a meaningful exfil channel and at minimum warrants a documented caveat or an operator-tunable CIDR; the current comment overstates the isolation actually enforced.

---

### RBAC-4: Operator holds cluster-wide create/get/list/update/watch on ClusterRoles and RoleBindings with no resourceNames (high)
- Location: markers `internal/controller/nodecertificatecheck_controller.go:152-153`; generated `config/rbac/role.yaml:119-129`
- Failure scenario: A compromised operator token can enumerate *every* ClusterRole in the cluster (full RBAC recon) and `update` any of them / create arbitrary ClusterRoles and RoleBindings — needed only to manage the single shared `fathom-node-agent-role` ClusterRole and per-check RoleBindings.
- Evidence: role.yaml grants `create;get;list;update;watch` on `clusterroles` and `rolebindings` cluster-wide. The operator's only ClusterRole write is the one `fathom-node-agent-role` object (`ensureNodeAgentClusterRole`, `:337-353`); its only RoleBinding writes are per-check, owner-referenced, in the check namespace (`:475-486`).
- Refutation notes: RBAC escalation-prevention bounds what the operator can *write* into a role to verbs it already holds, and the operator holds no `escalate`/`bind`/`impersonate` (cluster-wide), so it cannot mint a cluster-admin-equivalent role — this substantially caps the write risk. `+kubebuilder:rbac` markers cannot express `resourceNames`, so the generated grant is unavoidably broad *if* a ClusterRole must be managed at runtime. However the design choice to use one cluster-scoped `fathom-node-agent-role` (referenced by namespaced RoleBindings) is what forces the `clusterroles` write; a per-namespace `Role` would drop the `clusterroles` grant entirely (namespaced `roles` write only), materially shrinking the recon/tamper surface. Flagged high because unrestricted read of all cluster RBAC + ClusterRole write is a classic lateral-movement enabler even when escalation-bounded.

---

### RBAC-5: Operator holds cluster-wide apps/daemonsets create/update/delete — a DaemonSet-on-every-node takeover primitive (high)
- Location: marker `internal/controller/nodecertificatecheck_controller.go:144`; generated `config/rbac/role.yaml:44-54`
- Failure scenario: A compromised operator token can create an arbitrary DaemonSet (any pod spec — privileged, hostPath `/`, hostPID) in any namespace, landing attacker-controlled pods on every node = full cluster compromise. This is the single highest-value grant in the operator role.
- Evidence: role.yaml grants `create;delete;get;list;update;watch` on `apps/daemonsets` cluster-wide. The operator uses it only to converge its own hardened, owner-referenced node-agent DaemonSet (`ensureDaemonSet`, `:543-582`) and delete it when paused (`:300`).
- Refutation notes: This is genuinely required and unavoidable — the node-cert feature *is* a managed DaemonSet, `resourceNames` can't be expressed via markers, and the agent is created in arbitrary check namespaces so it can't be namespace-scoped. The DaemonSet the operator authors is hardened (non-root, readOnlyRootFilesystem, drop ALL, seccomp RuntimeDefault, read-only hostPath mounts confined to admission-allowlisted cert prefixes — `desiredDaemonSet` `:651-704`, path allowlist in the CRD CEL at `api/v1alpha1/nodecertificatecheck_types.go:22`). RBAC cannot constrain the *content* of a future DaemonSet the operator is authorized to create. Included as a candidate because reviewers should consciously accept the blast radius, not because a tighter marker exists.

---

### RBAC-6: Impersonated addon ServiceAccounts hold cluster-wide pods create/delete, granting the operator (via impersonation) a pod-create capability its own role lacks (medium)
- Location: `config/rbac/addons/addon-coredns.yaml:41-47`, `config/rbac/addons/addon-kube-state-metrics.yaml` (pods create/delete rule), `config/rbac/addons/addon-node-local-dns.yaml:41-47`; impersonation Role `config/rbac/addons/operator-impersonate.yaml`
- Failure scenario: The operator's *own* ClusterRole grants pods only `delete;list` (`config/rbac/role.yaml:18-24`) — no `create`. But it may impersonate the addon SAs (scoped `impersonate` on named SAs, `operator-impersonate.yaml`), and those SAs carry cluster-wide `pods create;delete` via ClusterRoleBinding. So a compromised operator can, through impersonation, create pods in *any* namespace — e.g. a pod in `kube-system` mounting a Secret volume — exfiltrating secrets it otherwise could not touch. Pod-create in a namespace is effectively read access to that namespace's Secrets.
- Evidence: addon ClusterRoles bind cluster-wide `{"":pods:create,delete}` to the addon SA (e.g. `addon-coredns.yaml:41-47`) via a ClusterRoleBinding (`:49-64`). The launcher builds probe pods in the target workload's namespace (`internal/probe/pod.go:86-171`, namespace required). Probe SAs are the impersonation targets used by `adapterClient` (`internal/controller/addoncheck_controller.go:451-499`).
- Refutation notes: The probe pods the code actually builds are hardened (`AutomountServiceAccountToken:false`, non-root, drop ALL, no volumes — `pod.go:126-159`), and the target addon genuinely lives in an arbitrary namespace so probe pods can't be namespace-pinned in RBAC. PodSecurity admission may block a hostile pod spec. The impersonate grant itself is tightly scoped by `resourceNames` to the 16 named addon SAs (good). Still, the *net effect* is that the operator gains cluster-wide pod-create through a side door despite its own role deliberately withholding it — worth surfacing since RBAC audits of the operator SA alone would miss this reachable capability.

---

### RBAC-7: Unused finalizer subresource grants across all four reconcilers (low)
- Location: markers `healthcheck_controller.go:62`, `clusterhealth_controller.go:66`, `addoncheck_controller.go:131`, `nodecertificatecheck_controller.go:138`; generated `config/rbac/role.yaml:69-77`
- Failure scenario: None directly — `update` on `<kind>/finalizers` is low-value on its own. Pure dead grant / cleanliness.
- Evidence: role.yaml grants `update` on `addonchecks/finalizers, clusterhealths/finalizers, healthchecks/finalizers, nodecertificatechecks/finalizers`, but no reconciler contains finalizer logic — a repo-wide grep for `AddFinalizer`/`RemoveFinalizer`/`Finalizer` in `internal/controller/*.go` returns nothing. Owner-reference-based GC is used instead (e.g. node-agent SA/RoleBinding/NetworkPolicy/ConfigMap are owner-referenced, per comments at `nodecertificatecheck_controller.go:140-143`).
- Refutation notes: The `/finalizers` update verb is the standard kubebuilder scaffold and confers negligible privilege (can only edit finalizers on those specific kinds), so this is defense-in-depth / tidiness, not a security defect. Listed for completeness since the marker set claims to be the trimmed, strictly-needed set.

---

## Positive observations (examined, no defect)
- `healthreports` grant is correctly trimmed to `create;get;list;watch;delete` — exactly matching code (create at `healthreport_idempotency.go:47`, prune-delete at `addoncheck_controller.go:539` and `nodecertificatecheck_helpers.go:259`), no stray update/patch.
- No `escalate`, `bind`, or wildcard (`*`) verbs/resources anywhere in the operator role or addon roles.
- No `secrets` access in the operator ClusterRole or any addon role (external-secrets addon reads only `externalsecrets` CRs, not `secrets`).
- Impersonation is properly least-privilege: namespaced Role, `impersonate` on `serviceaccounts` scoped by `resourceNames` to the 16 named addon SAs (`operator-impersonate.yaml`), no `users`/`groups` impersonation.
- Node-agent DaemonSet pod hardening is thorough: runAsNonRoot/runAsUser 65532, readOnlyRootFilesystem, allowPrivilegeEscalation false, drop ALL caps, seccomp RuntimeDefault, no hostNetwork/hostPID/privileged, hostPath mounts are `ReadOnly:true` and confined to `MinimalMountDirs` of admission-allowlisted prefixes (`desiredDaemonSet` `:651-704`). `AutomountServiceAccountToken:true` is justified (agent must write its report CM to the API).
- Probe pods set `AutomountServiceAccountToken:false` and mount no volumes.
- `leader_election_role` is namespaced (Role, not ClusterRole) and its configmaps/leases CRUD is standard controller-runtime leader-election need.
- metrics auth/reader roles are minimal (tokenreviews/subjectaccessreviews create; nonResourceURL `/metrics` get).
