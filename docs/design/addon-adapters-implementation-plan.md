<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Addon Adapters — Implementation Plan

Status: **Draft / planning** · Scope: the *Addon Adapters* epic (Linear milestones
"Addon Adapters Wave 1–4", parented under **SKA-46**) · Owner: Fathom maintainers.

This document is the implementation plan for building out Fathom's **addon health
adapters** — the per-addon, passive status-read checks (product axis **A** in the
adapter roadmap). It captures where the machinery stands today, the repeatable
"how to add an adapter" playbook, the cross-cutting enablers that must land before
the adapter count grows, and a wave-by-wave, adapter-by-adapter breakdown with
effort estimates and sequencing.

It is deliberately narrow: reachability/troubleshooting checks (axis **B**,
`ReachabilityCheck`, SKA-50) and node/host checks (axis **C**,
`NodeCertificateCheck` / `NodeHealthCheck`, SKA-49/48) are *out of scope* here and
tracked separately.

---

## 1. Where we are today

### 1.1 Shipped

The adapter framework and the first four adapters are done (Wave 1 is ~96%
complete; the only open Wave 1 item is the deferred, speculative field-path helper
**SKA-426**):

| Adapter | Package | Shape | Ticket |
| --- | --- | --- | --- |
| cert-manager | `internal/adapter/certmanager` | Deployments + CRDs + managed CRs + **webhook validation + dry-run admission probe** | SKA-46 line |
| CoreDNS | `internal/adapter/coredns` | Deployments/Services/EndpointSlices + **active DNS probe pod** | SKA-59 |
| External Secrets | `internal/adapter/externalsecrets` | Deployments + CRDs + managed CRs | SKA-61 |
| Cilium (CNI) | `internal/adapter/cilium` | Deployment + **DaemonSet** + CRDs, "absent → Skipped" posture | SKA-66 |

Supporting pieces already in place:

- **Contract** `pkg/adapter` at **`ContractVersion = 0.2.0`** (adds `Request.ProbeImage`).
- **Registry** `internal/adapter/registry` — compile-time import + runtime `Register`, keyed by addon type.
- **Wiring** `internal/app/run.go` — `builtInAdapters()` list + `BuildAdapterRegistry`.
- **Shared CRD helpers** `internal/adapter/crdutil` — `Established`, `PreferredServedVersion` (SKA-424, done).
- **Reconciler** `internal/controller/addoncheck_controller.go` — resolves the adapter by `spec.addonType`, runs it, maps `Result` → `HealthReport` and mirrors the aggregate into `AddonCheck.status`.
- **Observability** — per-family Prometheus metric `fathom_adapter_run_duration_seconds{outcome}` recorded inside the adapter (controller no longer double-records, SKA-504) + OpenTelemetry span per adapter run (SKA-293).
- **e2e** — helmfile-based addon stack (`test/e2e/fixtures`) with real-install assertions for CoreDNS/cert-manager/external-secrets (SKA-414/415/416).

### 1.2 The `AddonCheck` external contract (already present)

`AddonCheckSpec` already carries the per-family policy structure — `Policy
map[string]AddonCheckFamilyPolicy` with `Enabled`, `Namespaces`, `LabelSelector`,
`Thresholds map[string]string`, plus `Interval`, `Timeout`, `Paused`,
`HistoryLimit`. CEL validation for `timeout`/`interval` landed in SKA-292.

This matters for the plan: **SKA-54 ("extend AddonCheck schema for per-family
policy controls") is partly done already** — the *schema* exists. The remaining
work is controller/adapter-side *validation* of policy content (unknown family
keys, malformed thresholds, invalid selectors) surfaced as clear status
conditions. See §3.1.

---

## 2. The adapter model (what every adapter is)

An adapter is a small, concurrency-safe Go type implementing `pkg/adapter.Adapter`:

```go
type Adapter interface {
    Name() string
    Version() string
    ContractVersion() string          // returns adapter.ContractVersion
    Capabilities() Capabilities        // { AddonTypes []string; Families []Family }
    Run(ctx context.Context, req Request) (Result, error)
}
```

- `Run` returns an **`error` only for adapter-level failure** (couldn't reach the
  API server). Per-check problems are `CheckResult` entries.
- Each `CheckResult` carries a `Family`, an `Outcome`, a `TargetRef`, a required
  `Summary` (for non-Pass), and string `Details`.
- **Outcomes** — and their roll-up semantics — are load-bearing:
  - `Pass` healthy · `Warn` non-fatal anomaly · `Fail` target is unhealthy ·
    `Error` adapter couldn't determine state · `Skipped` intentionally not run.
  - `adapter.FamilyOutcome` rolls up **per family** to the worst outcome, and
    **treats `Skipped` as `Pass`**. This is why the Cilium adapter emits
    `Skipped` (not `Fail`) when the addon is absent — an `AddonCheck` for an addon
    that isn't installed rolls up green instead of alarming.
- The runtime hands the adapter a **least-privilege scoped client**, the driving
  `Target`, the `Policy map[Family]FamilyPolicy`, a `Timeout`, and a default
  `ProbeImage` (only active-probe adapters use the last one).

### 2.1 Archetypes

Every planned adapter is a combination of a small number of building blocks.
Naming them lets us estimate effort and reuse code:

| Archetype | Building block | Reference adapter |
| --- | --- | --- |
| **A. Workload-only** | one or more Deployments (or a DaemonSet / StatefulSet) + their Pods → ready? | metrics-server-like |
| **B. Workload + CRD + managed-CR** | A, plus CRDs `Established` + served-version check, plus reading managed CR `.status` against thresholds | cert-manager / external-secrets |
| **C. DaemonSet / per-node** | DaemonSet `DesiredNumberScheduled`/`NumberUnavailable`/rollout → ready across nodes | Cilium |
| **D. Active probe pod** | launches a short-lived probe Pod (`req.ProbeImage`) to test behaviour from a workload's perspective | CoreDNS |
| **E. Admission surface** | Validating/Mutating WebhookConfiguration present & served; optional dry-run `Create` as a live admission probe | cert-manager |
| **F. Aggregated API** | an `APIService` (`apiregistration.k8s.io`) `Available` condition is True | *(new; metrics-server, KEDA metrics)* |

Most Wave 3/4 adapters are **A** or **B**. The expensive ones combine C+E (istio,
aqua) or carry rich managed-CR status (argo-cd, velero, prometheus-operator).

---

## 3. Cross-cutting enablers (Phase 0 — land before the adapter count grows)

These set the pattern once. Doing them **before** adding 15+ adapters avoids
paying the fix 15 times. Each is its own PR.

### 3.0 (Recommended, new) Extract a shared adapter "workload-check kit"

**Observed today:** each adapter re-implements ~250 lines of near-identical
boilerplate — `checkDeployment`/`checkPods`/`deploymentAvailable`/`podReady`/
`maxRestartCount`, the DaemonSet variant, `familyPolicy`, the threshold parsers
(`stringThreshold`/`int32Threshold`), the `check`/`skipped` `CheckResult`
constructors, the `tracer` var and `endAdapterRunSpan`. These are copy-paste
siblings across cert-manager/external-secrets/cilium/coredns.

**Proposal:** lift them into `internal/adapter/adapterkit` (name TBD) so a new
adapter is *only* its addon-specific facts: the component/CRD names, the family
set, and the state→outcome thresholds. This cuts per-adapter code from ~500 LOC to
~200 LOC and makes outcome semantics uniform (e.g. everyone agrees "replicas==0 →
Warn, not-Available → Fail").

- Do it **after SKA-425** so the CRD-version handling in the kit is map-shaped
  from day one.
- Migrate the four existing adapters onto the kit in the same PR (or a fast
  follow) so there's one source of truth and coverage doesn't regress.
- Tracked as **SKA-523** (Wave 2, High). Highest-leverage item in this plan.

### 3.1 SKA-54 — Policy validation & status conditions

Schema exists (§1.2). Remaining work:

- Validate `spec.policy` **content** in the reconciler / adapter: reject unknown
  family keys (not in `Capabilities().Families`), malformed `Thresholds` values,
  and invalid `LabelSelector`s, surfacing a clear `Ready=False` condition with a
  specific `reason` (`InvalidPolicy`) rather than silently ignoring.
- Family keys are **adapter-defined**, so they can't be fully CEL-validated at the
  CRD schema level — this is legitimately controller/adapter-side. (CEL still owns
  the structural rules like duration positivity, already shipped.)
- Document the recognized `Thresholds` keys per family (feeds SKA-68).

### 3.2 SKA-425 — Per-CRD version map

Replace each adapter's flat `crds []string` with `map[string][]string` (per-CRD
descending served-version preference), consuming the now-version-list-aware
`crdutil.PreferredServedVersion`. Makes adding a heterogeneously-versioned CRD a
one-line entry. Low effort, unblocks every CRD-backed adapter in Waves 3–4.
Prerequisite SKA-424 is **done**.

### 3.3 SKA-58 — Read-only RBAC profile + CI guard

- Publish the **RBAC matrix** for adapters (which groups/resources, which verbs).
- Enforce least privilege: default to `get;list;watch`. The **only** write paths
  today are documented exceptions and must stay explicit: CoreDNS
  `pods: create;delete` (probe pod) and cert-manager's dry-run `Create` (admission
  probe). Every new write verb needs a justification in the PR.
- Add a CI check that scans the generated `config/rbac/role.yaml` for verbs beyond
  an allowlist and fails on unjustified `create/update/patch/delete`.
- Watch **Secret read scope** specifically (velero, ESO-adjacent, dynatrace) —
  prefer namespaced/labelled reads over cluster-wide Secret `list`.

### 3.4 SKA-68 — Adapter authoring guide + release template

Turn §4 of this plan into `docs/guides/adapter-authoring.md`: the step-by-step
recipe, the RBAC declaration convention, contract-compatibility/versioning rules,
the threshold-key documentation convention, and the PR/test checklist. This is the
public-facing artifact for third-party adapter authors.

### 3.5 SKA-426 — Field-path migration helper *(deferred)*

Speculative; **do nothing** until an upstream addon actually renames/relocates a
field across an API version. Keep in the backlog as the documented answer to "what
do we do when a field moves." Activate with a real consumer.

---

## 4. The per-adapter playbook

Every new adapter is the same repeatable change. This is the recipe (and the seed
for SKA-68).

1. **Scaffold the package** `internal/adapter/<addon>/adapter.go`:
   - `Adapter struct{}` + `New()`, the four metadata methods
     (`ContractVersion()` returns `adapter.ContractVersion`).
   - `Capabilities()` advertising `AddonTypes: []string{"<addon>"}` and the family list.
   - `Run(ctx, req)` iterating enabled families; per family, timing +
     `metrics.RecordAdapterRun(Name, family, FamilyOutcome(...), dur)`.
   - Reuse the shared kit (§3.0) for Deployment/DaemonSet/Pod/CRD checks and
     threshold parsing; write only the addon-specific component/CRD names and
     outcome thresholds.
2. **Decide the absent-addon posture.** Optional components → `Skipped` (Cilium
   pattern) so a partially-installed or non-applicable addon rolls up green.
   Core/required components missing → `Fail`.
3. **Declare RBAC** with `+kubebuilder:rbac` markers directly above `Run` — only
   the read permissions the adapter needs. `controller-gen` aggregates them
   module-wide; no central list to edit.
4. **Wire it in** — the only production file touched beyond the package:
   `internal/app/run.go` — add the import and append `<addon>.New()` to
   `builtInAdapters()`.
5. **Regenerate** — `go -C tools tool task manifests` (RBAC) + `task helm:sync`
   (chart rules) + `task docs:api-ref` if schema changed. Commit generated output;
   `verify-generated` gates it in CI.
6. **Scheme** — usually nothing: managed CRs are read via `unstructured`.
   Only touch `NewScheme()` if reading a *typed* new API group.
7. **Unit tests** `adapter_test.go` — self-contained: `newFakeClient`,
   a `healthyObjects()` builder, and `assertHasOutcome`/`assertFamily`/
   `assertHasDetail`. One test per failure mode (missing workload, not-available,
   scaled-to-zero, pod-not-ready, restart-warn, CRD not-established/unsupported,
   managed-CR unhealthy, custom names/namespace). Keep the coverage gate green.
8. **e2e** — **required** for adapter `Run` changes (per repo policy): add the
   addon to the helmfile stack in `test/e2e/fixtures` and a Ginkgo assertion that
   the `AddonCheck` reconciles to the expected result against the real install
   (SKA-414/415/416 pattern). If local kind/helm/helmfile/docker is unavailable,
   say so in the PR test plan — don't silently skip.
9. **Docs** — add the addon to the README adapter list + any supported-versions
   note. The CRD API ref is generated.
10. **One PR per adapter** (repo convention: one logical change per PR),
    Conventional Commit `feat(adapter): <addon> …`, DCO signed-off.

**Effort per adapter after §3.0 lands:** ~200 LOC adapter + ~300–500 LOC tests +
one helmfile entry + one e2e spec. Archetype A/B/C ≈ **S–M**; C+E or rich
managed-CR ≈ **M–L**.

---

## 5. Wave-by-wave breakdown

Component/CRD names below are the well-known upstream defaults and **must be
pinned to the supported chart version at implementation time** (captured in the
e2e helmfile and each adapter's `supportedAPIVersions`/CRD map). Treat them as the
starting checklist, not gospel.

### Wave 2 — Foundational networking + enablers (`Addon Adapters Wave 2`)

Enablers §3.1–3.4 (+ recommended §3.0) land first. Then:

| Adapter | Ticket | Arch | Families (proposed) | Key objects | Size |
| --- | --- | --- | --- | --- | --- |
| Cilium | SKA-66 | C | control_plane / agent / crd | done | ✅ |
| **istio mesh** | SKA-60 | C+E | system_health · ztunnel_health · istio_cni_health · mesh_status | istiod Deployment; ztunnel + istio-cni-node DaemonSets; sidecar-injector (mutating) + istiod (validating) webhooks; networking/security CRDs; PeerAuthentication | **L** |
| **external-dns** | SKA-62 | A/B | system_health · record_sync | external-dns Deployment; optional `dnsendpoints.externaldns.k8s.io` + DNSEndpoint status | **S–M** |
| **metrics-server** | SKA-65 | A+F | system_health · api_availability | metrics-server Deployment; APIService `v1beta1.metrics.k8s.io` `Available=True` | **S** |
| **envoy-gateway** | SKA-507 | B/E | system_health · gateway_status · crd_health | envoy-gateway Deployment; dynamically-named envoy proxy Deployments (by label); Gateway/HTTPRoute `Accepted`+`Programmed`; gateway.networking.k8s.io + gateway.envoyproxy.io CRDs | **M–L** |
| ~~ingress-nginx~~ | SKA-63 | — | dropped (gen-legacy) | — | ❌ |

Notes:
- **istio** is the anchor and the hardest: mixed sidecar/ambient topologies mean
  ztunnel/istio-cni families should be `Skipped` when absent. Keep `mesh_status` to
  *config-object* sanity (PeerAuthentication/mTLS mode) in v1 — live proxy-sync
  needs xDS and belongs with reachability (SKA-50), not here.
- **metrics-server** introduces archetype **F** (APIService availability); make
  that a reusable kit helper — KEDA's external-metrics APIService reuses it.
- **envoy-gateway** proxy Deployments are created per-Gateway with generated
  names → enumerate by `app.kubernetes.io/managed-by=envoy-gateway`, don't
  hardcode names.

**Suggested Wave 2 order:** metrics-server (quick, validates the post-enabler
playbook + adds archetype F) → external-dns → envoy-gateway → istio (most complex
last, unless strategically pulled forward).

### Wave 3 — Cluster-shaping / autoscaling (`Addon Adapters Wave 3`)

All passive status-read, highly parallelizable, mostly S–M. Batch them.

| Adapter | Ticket | Arch | Families (proposed) | Key objects | Size |
| --- | --- | --- | --- | --- | --- |
| **KEDA** | SKA-508 | B+F | system_health · scaler_health · api_availability | keda-operator + metrics-apiserver + admission-webhooks Deployments; APIService `v1beta1.external.metrics.k8s.io`; ScaledObject/ScaledJob `Ready` cond; keda.sh CRDs | **M** |
| **VPA** | SKA-509 | B/E | system_health · recommendation_health | recommender/updater/admission-controller Deployments (+ admission webhook); VerticalPodAutoscaler `RecommendationProvided`; autoscaling.k8s.io CRDs | **M** |
| **descheduler** | SKA-510 | A | system_health | Deployment **or** CronJob/Job mode — read `batch/v1` CronJob + last Job success/`lastScheduleTime` staleness | **S–M** |
| **node-local-dns** | SKA-511 | C | system_health | node-local-dns DaemonSet (kube-system), ready across nodes | **S** |
| **kured** | SKA-512 | C | system_health · (reboot_status) | kured DaemonSet; optional Warn when nodes stuck on the reboot lock/annotation | **S** |

Notes:
- **descheduler** adds the reusable "job-based addon" check (CronJob last-run
  success + staleness) — useful later for any periodic-job addon.
- **kured** `reboot_status` (nodes needing/awaiting reboot) is a nice-to-have Warn
  family; defer to a follow-up if it slips.

### Wave 4 — Delivery / storage / observability / security (`Addon Adapters Wave 4`)

| Adapter | Ticket | Arch | Families (proposed) | Key objects | Size |
| --- | --- | --- | --- | --- | --- |
| **Argo CD** | SKA-513 | B | system_health · application_health | server/repo-server/applicationset/notifications/dex Deployments + application-controller StatefulSet; Application `health.status`/`sync.status` (Degraded→Fail, OutOfSync→Warn); argoproj.io CRDs | **L** |
| **Trident (CSI)** | SKA-514 | B/C | system_health · backend_health | trident-controller Deployment + trident-node DaemonSet + CSIDriver; TridentBackendConfig `phase=Bound`; trident.netapp.io CRDs | **M–L** |
| **kube-state-metrics** | SKA-515 | A | system_health | Deployment (or sharded StatefulSet) ready | **S** |
| **Prometheus Operator** | SKA-64 | B/E | system_health · resource_health | prometheus-operator Deployment (+ admission webhook); Prometheus/Alertmanager status (`availableReplicas`, `Available`/`Reconciled`); monitoring.coreos.com CRDs | **M–L** |
| **Velero** | SKA-67 | B | system_health · backup_health | velero Deployment (+ optional node-agent DaemonSet); BackupStorageLocation `phase=Available`, recent Backup `Completed` vs `Failed`, Schedule freshness (`staleHours` threshold); velero.io CRDs | **M–L** |
| **Aqua Enforcer / KubeEnforcer** | SKA-516 | C/E | system_health | aqua-enforcer DaemonSet + kube-enforcer Deployment (+ webhook); operator.aquasec.com CRDs | **M** |
| **Azure Workload Identity webhook** | SKA-517 | E | system_health | azure-wi-webhook Deployment + MutatingWebhookConfiguration; no CRDs | **S** |
| **Dynatrace Operator** | SKA-518 | B/C/E | system_health · dynakube_health | operator + webhook Deployments, oneagent DaemonSet, activegate StatefulSet; DynaKube status; dynatrace.com CRDs | **M–L** |

Notes:
- **Argo CD / Velero / Prometheus Operator** carry the richest managed-CR status —
  most of their value is in the second family (application/backup/resource health),
  so budget test effort there.
- **Velero `backup_health`** is the strongest threshold candidate (`staleHours`,
  fail-on-`Unavailable` BSL) — good showcase for the policy/thresholds work (§3.1).

---

## 6. Sequencing & dependencies

```
Phase 0  (enablers, mostly parallel):
  SKA-425 per-CRD version map ──┐
  §3.0   shared adapter kit  ───┼──> unblocks all CRD-backed + workload adapters
  SKA-58  RBAC profile + CI guard
  SKA-54  policy validation/conditions
  SKA-68  authoring guide (can trail one adapter behind)

Phase 1  Wave 2:  metrics-server → external-dns → envoy-gateway → istio
Phase 2  Wave 3:  KEDA · VPA · descheduler · node-local-dns · kured   (parallel)
Phase 3  Wave 4:  argo-cd · velero · prometheus-operator · trident ·
                  kube-state-metrics · aqua · azwi · dynatrace          (parallel)
```

Dependency notes:
- §3.0 and SKA-425 should land **before** the Wave 3/4 batch so those adapters are
  written against the kit + version-map from the start (no rework/migration).
- SKA-58's CI guard should exist before the RBAC surface fans out across 15
  adapters — cheaper to enforce than to retrofit.
- Adapters within a wave are independent — good candidates for parallel work
  (each is one PR).

---

## 7. Testing, RBAC, and CI at scale

- **Unit/envtest:** every adapter ships its own suite; coverage gate
  (`scripts/check-coverage.sh`) must stay green. The shared kit (§3.0) gets its own
  thorough tests so per-adapter suites focus on addon-specific outcomes.
- **e2e is required** for adapter `Run` changes. The scaling risk: 17 addons in one
  helmfile stack = a heavy, slow, flaky CI cluster.
  - **Tracked as SKA-524** (Wave 3): tier the e2e stack (a small always-on core +
    per-adapter opt-in jobs, or a CI matrix) rather than one monolithic cluster.
- **RBAC:** the aggregated `manager-role` grows with every adapter. SKA-58's guard
  keeps it least-privilege; review Secret-read scope specifically.
- **Metrics/tracing:** free per adapter — the kit records
  `fathom_adapter_run_duration_seconds` and the span; don't re-record in the
  controller (SKA-504).

---

## 8. Risks & open questions

1. **e2e cluster cost/flake at 17 addons** — needs a tiered/matrixed strategy
   (see §7). *Biggest operational risk.*
2. **Upstream version drift** — component/CRD names change across chart versions.
   Mitigated by pinning in the helmfile + `supportedAPIVersions`/CRD map
   (SKA-425); cross-version field renames by SKA-426 (when activated).
3. **Skipped-vs-Fail correctness** — getting the absent-component posture wrong
   turns "not installed" into a false alarm. The Cilium pattern is the reference;
   make it a kit default.
4. **Untyped thresholds** — `Thresholds map[string]string` is validated per
   adapter, not by the schema. SKA-54 + SKA-68 must document recognized keys and
   fail loudly on bad values.
5. **RBAC sprawl / Secret reads** — velero/dynatrace/ESO-adjacent adapters may
   want broad Secret access; keep it namespaced/labelled.
6. **Scope creep into reachability** — istio `mesh_status` and node-local-dns can
   tempt active/data-plane probing. Keep this epic passive; push live-path checks
   to SKA-50.

## 9. Definition of done (per adapter)

- [ ] `internal/adapter/<addon>` implements the contract, reuses the shared kit.
- [ ] Correct absent-addon posture (Skipped for optional, Fail for required).
- [ ] `+kubebuilder:rbac` markers minimal; RBAC regenerated + helm-synced; passes SKA-58 guard.
- [ ] Wired into `builtInAdapters()`; registration test updated.
- [ ] Unit suite covers every family × outcome; coverage gate green.
- [ ] e2e helmfile entry + assertion (or documented reason it couldn't run locally).
- [ ] README adapter list + supported-versions note updated; API ref regenerated.
- [ ] One focused PR, Conventional Commit, DCO signed-off.

## 10. Effort summary

| Wave | Adapters | Rough size | Notes |
| --- | --- | --- | --- |
| Phase 0 enablers | 4–5 items | M total | §3.0 kit is the leverage multiplier |
| Wave 2 | istio (L), envoy-gateway (M–L), external-dns (S–M), metrics-server (S) | ~1 L + rest S–M | istio dominates |
| Wave 3 | KEDA/VPA (M), descheduler (S–M), node-local-dns/kured (S) | mostly S–M | fully parallelizable |
| Wave 4 | argo-cd (L), velero/prom-operator/trident/dynatrace (M–L), ksm/azwi (S), aqua (M) | mixed | rich managed-CR status drives cost |

**Bottom line:** land the Phase 0 enablers — especially the shared adapter kit and
the per-CRD version map — before the adapter count grows; after that, each adapter
is a bounded, ~200-LOC, single-PR change following the §4 playbook, and Waves 3–4
parallelize cleanly.

---

### Ticket cross-reference

Enablers: SKA-523 (shared adapter kit), SKA-425, SKA-58, SKA-54, SKA-68 (+ SKA-424
done, SKA-426 deferred), SKA-524 (e2e stack tiering). Wave 2: SKA-60, SKA-62,
SKA-65, SKA-507 (SKA-66 done, SKA-63 cancelled). Wave 3: SKA-508, SKA-509, SKA-510,
SKA-511, SKA-512. Wave 4: SKA-64, SKA-67, SKA-513, SKA-514, SKA-515, SKA-516,
SKA-517, SKA-518.
