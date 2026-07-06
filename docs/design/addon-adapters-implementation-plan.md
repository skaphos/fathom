<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Addon Adapters — Implementation Plan (v2)

Status: **Draft / planning** · Scope: the *Addon Adapters* epic (Linear milestones
"Addon Adapters Wave 1–4", parented under **SKA-46**) · Owner: Fathom maintainers.

> **v2 supersedes v1.** An adversarial review of v1 found three claims contradicted
> by the code and one unexamined architectural fork. This revision folds in five
> decisions that reshape the epic from *"write 17 Go adapters"* into *"build one
> declarative engine + a few Go escape hatches, on an execution model that actually
> re-runs."* The corrections are called out inline as **[correction]**.

This is the axis-A (per-addon passive health) plan. Reachability (SKA-50) and node
checks (SKA-49/48) are out of scope.

---

## 0. Decisions that shape this plan

1. **Execution is periodic *and* on-demand.** Checks re-run on `spec.interval` and
   on an explicit command trigger — not only when the spec changes.
2. **Declarative-first.** One declarative adapter driven by an `AddonDefinition`
   covers the regular addons; a full Go adapter is the escape hatch for cases the
   generic engine can't express.
3. **Absence is explicit.** An addon declared part of the validation is
   **required**; if it's absent the check **fails**. Optional addons report a
   non-failing `Absent` instead.
4. **Per-addon least privilege is real, not aspirational.** Each addon's checks run
   through a client scoped to only that addon's declared reads.
5. **Version is detected.** The engine detects the installed addon version and gates
   coverage against a supported-version range.

---

## 1. Where we are today (and what v1 got wrong)

### 1.1 Shipped

Framework + four adapters (Wave 1 ~96%; only the speculative SKA-426 remains open):

| Adapter | Package | Shape |
| --- | --- | --- |
| cert-manager | `internal/adapter/certmanager` | Deployments + CRDs + managed CRs + webhook validation + **dry-run admission probe** |
| CoreDNS | `internal/adapter/coredns` | Deployments/Services/EndpointSlices + **active DNS probe pod** |
| External Secrets | `internal/adapter/externalsecrets` | Deployments + CRDs + managed CRs |
| Cilium | `internal/adapter/cilium` | Deployment + **DaemonSet** + CRDs |

Supporting: contract `pkg/adapter` at `ContractVersion = 0.2.0`; registry
(`internal/adapter/registry`, compile-time import + runtime `Register`); wiring in
`internal/app/run.go` `builtInAdapters()`; shared CRD helpers
`internal/adapter/crdutil` (SKA-424); reconciler
`internal/controller/addoncheck_controller.go`; per-family Prometheus metric +
OTel span; helmfile e2e stack.

### 1.2 Corrections to the record (from the v1 review)

These are the current-code facts the target architecture must fix:

- **[correction] Checks don't re-run.** `runAddonCheck` fires only when
  `LastRunTime == nil || observedGeneration != Generation`
  (`internal/controller/addoncheck_controller.go:141`); there is **no `RequeueAfter`** and
  **`spec.interval` is unused**. Today an AddonCheck runs once and then never again
  until its spec is edited — a degraded addon keeps its last green report. This is
  the single most important thing to fix (Decision 1).
- **[correction] Absence is handled inconsistently, and "rolls up green" is false.**
  cert-manager / external-secrets / CoreDNS emit **Fail** on NotFound; only Cilium
  emits **Skipped**. The persisted verdict for an all-Skipped run is **`Skipped`**
  (severity table `Pass(1) < Skipped(2) < …`, `api/v1alpha1/healthreport_types.go:35-52`),
  **never `Pass`**. The Cilium package comment "rolls up green" is wrong about the
  persisted result; only the *metrics* layer (`FamilyOutcome`) relabels absent
  families as `Pass`. Decision 3 replaces this with an explicit model.
- **[correction] The adapter client is not scoped.** Every adapter is handed the
  controller's own `r.Client` (`internal/controller/addoncheck_controller.go:217`); all RBAC markers
  aggregate into a single `manager-role` ClusterRole on one ServiceAccount. The
  contract's "least-privilege scoped client" comment is aspirational. Decision 4
  makes it real.
- **[correction] No version detection exists.** "Version" today means CRD
  served-version selection only. Decision 5 adds real detection.
- **Policy misconfig is silently swallowed.** Unknown `policy` family keys are
  ignored, malformed `Thresholds` silently default, invalid `LabelSelector`s
  surface only as per-check `Error`s — never a status condition. (SKA-54.)

`AddonCheckSpec` already carries `Policy map[string]AddonCheckFamilyPolicy`
(`Enabled`/`Namespaces`/`LabelSelector`/`Thresholds`), `Interval`, `Timeout`,
`Paused`, `HistoryLimit`, with CEL duration checks (SKA-292). So the *schema* for
policy exists; the *behavior* (validation, and actually consuming `Interval`) does
not.

---

## 2. Target architecture

### 2.1 Declarative-first adapter (Decision 2)

Instead of one Go package per addon, ship **one declarative engine** plus a small
library of **evaluators**, driven by an **`AddonDefinition`**:

```yaml
# first-party definition (illustrative)
addonType: keda
supportedVersions: ">=2.10 <3.0"          # Decision 5
required: true                             # Decision 3 default
rbac:                                      # Decision 4 — the source for the scoped role
  - {group: apps, resources: [deployments], verbs: [get, list, watch]}
  - {group: keda.sh, resources: [scaledobjects], verbs: [get, list, watch]}
families:
  system_health:
    workloads:
      - {kind: Deployment, namespace: keda, name: keda-operator, required: true}
      - {kind: Deployment, namespace: keda, name: keda-operator-metrics-apiserver}
    apiServices:                           # archetype F
      - {name: v1beta1.external.metrics.k8s.io}
    crds:
      - {group: keda.sh, resource: scaledobjects, servedVersions: [v1alpha1]}
  scaler_health:
    managedResources:
      - {apiVersion: keda.sh/v1alpha1, kind: ScaledObject,
         condition: Ready, expected: "True",
         selector: {namespaces: [], labels: {}}}
```

**Evaluators** (the reframed SKA-523 — a data-driven library, not per-adapter
copy-paste):

| Evaluator | Checks | Covers archetype |
| --- | --- | --- |
| `workload-ready` | Deployment/DaemonSet/StatefulSet + Pods ready, restart-warn | A, C |
| `crd-established` | CRD `Established` + served-version (per-CRD version map) | B |
| `condition-status` | managed-CR `.status.conditions[type]==expected`, roll-up over a selector | B |
| `expiry-threshold` | a date field (e.g. cert `notAfter`) vs warn/fail days | cert-manager passive |
| `recency` | time-since-last-success (e.g. Velero last backup) vs staleness | Velero |
| `apiservice-available` | `APIService.status Available==True` | F |
| `webhook-present` | Validating/Mutating webhook config present & served | E (presence only) |
| `node-annotation` | node annotation/label predicate (e.g. kured reboot lock) | kured |

The engine walks the definition, runs the named evaluators (reusing the same
workload/CRD/threshold logic today's adapters copy), and emits `CheckResult`s with
**uniform outcome semantics** — fixing the Fail-vs-Skipped-vs-Warn drift between the
current four adapters.

**Escape hatch (Go).** A hand-written adapter still registers normally and
*overrides* the declarative path for its `addonType`. Reserve it for what the
evaluators can't express:

- **Active probes** — CoreDNS DNS-resolution probe pod; cert-manager admission
  dry-run `Create`. (Both exist already.)
- Anything needing bespoke API calls.

*(Realigned 2026-07-06, SKA-60: "conditional topology — istio
ambient-vs-sidecar" was originally listed here, but per-check
`Absence: Optional` (SKA-526) expresses present-or-absent families
declaratively, and the reserved `WebhookCheck` evaluator shipped with the
istio definition — so istio landed declarative after all, and the escape
hatch shrank to genuinely active probes.)*

Note: the `expiry-threshold`, `recency`, and `node-annotation` evaluators pull
cert-manager (passive parts), Velero, and kured *back into* declarative — the Go
escape hatch shrinks to genuinely active probes.

**Where definitions live.** Phase 1: an **embedded first-party catalog** (Go
`embed` of the YAML), versioned with the operator release. Phase 2: an
**`AddonDefinition` CRD** (or ConfigMap catalog) so operators and third parties add
addons as data, not Go — this is the payoff, and it aligns with ADR-0001's
"future out-of-process loader … registers against the same Registry" and with
SKA-68 (authoring guide becomes "how to write an `AddonDefinition`").

**Selection.** `AddonCheck.spec.addonType` → Go adapter if one is registered for
that type, else the declarative engine bound to the matching definition.

### 2.2 Execution model (Decision 1)

- **Periodic:** consume `spec.interval` (add a sane default, e.g. 5m) and return
  `RequeueAfter: interval` from `Reconcile`; run when `now - LastRunTime >= interval`.
- **On spec change:** keep the generation-triggered immediate run.
- **On demand:** an explicit trigger — an annotation (`fathom.skaphos.io/run-now`)
  or the beaconctl `run-now` verb (SKA-45) — forces a run regardless of interval.
- **Bounded & single-flight:** honor `spec.timeout`; never overlap runs for the
  same AddonCheck; back off on adapter `error`.

### 2.3 Absence semantics (Decision 3)

- Every addon (and, if needed, individual components) is **`required` by default**.
  An addon that may legitimately be absent on some clusters (e.g. Cilium on a
  non-Cilium cluster) sets `optional: true` — an explicit opt-out.
- Absence is surfaced as a first-class **`Absent`** reason on the `CheckResult`,
  and the **verdict is driven by the flag**: required-absent → **`Fail`**;
  optional-absent → non-failing (`Skipped`, reason `Absent`).
- `AddonCheck.status` gains an **`Absent` count** so "not installed" is queryable
  and distinct from "unhealthy" and "disabled".
- Recommended representation: reuse the existing `Fail`/`Skipped` outcomes with a
  `reason=Absent` detail + status counter (no severity-table change). *Alternative
  if a distinct top-level verdict is wanted:* add an `Absent` outcome/severity and
  have aggregation treat required-`Absent` at `Fail` precedence — larger change;
  flagged for a call.
- This makes absence **declared data**, ending the per-adapter NotFound
  inconsistency noted in §1.2.

### 2.4 Per-addon least-privilege client (Decision 4)

Realize the scoped client via **impersonation**:

- Generate a **per-addon ClusterRole + ServiceAccount** from the definition's
  `rbac:` block (data-driven RBAC — the definition is the single source for both
  *what to check* and *what it may read*).
- The operator ServiceAccount holds **only `impersonate`** on those per-addon SAs —
  it carries no addon-read permissions itself, so the aggregate god-role disappears
  from actual use.
- The engine builds an **impersonating client per run**
  (`rest.Config.Impersonate = addon SA`) and hands *that* to the checks. Each
  addon's blast radius is its own declared reads; every read is auditable to the
  addon identity.
- **Phasing:** ship on the current aggregate role first (unblocks build-out), then
  cut over to impersonation as a hardening milestone. SKA-58 evolves from "one
  read-only role" into "per-addon generated roles + impersonation + a CI guard that
  the generated roles stay read-only (with the documented write exceptions:
  CoreDNS probe pod, cert-manager dry-run)."

### 2.5 Version detection + supported-version gating (Decision 5)

- **Detect** the installed version, in priority order:
  1. `app.kubernetes.io/version` label on the addon's workloads/pods (cheap, no
     Secret read) — primary.
  2. container **image tag** on the addon workload — fallback.
  3. *(optional, narrowly scoped)* the **Helm release secret**
     (`type=helm.sh/release.v1`) for chart/appVersion — only if labels/image are
     inconclusive, and only with a narrowly-scoped Secret read that respects §2.4.
- **Gate** against the definition's `supportedVersions` (semver range):
  - in range → proceed normally;
  - out of range → `Warn` (reason `UnsupportedAddonVersion`, detail: detected vs
    supported) — don't hard-Fail; it's Fathom's *coverage* that's uncertain, not
    necessarily the addon;
  - undetectable → `Warn` (reason `VersionUnknown`), proceed best-effort.
- **Surface** `status.detectedVersion` and include it in HealthReport details.
- Pairs with SKA-425/426: version-specific component names / field paths can key
  off the detected version.

---

## 3. Phase 0 — foundation (must land before addon build-out)

Ordered by dependency. Each is its own PR.

| # | Work | Ticket | Notes |
| --- | --- | --- | --- |
| P0-A | **Periodic + on-demand execution** — consume `spec.interval` (+ default), `RequeueAfter`, run-now trigger | **new (blocker)** | Nothing else matters until checks re-run. |
| P0-B | **Declarative engine + evaluator library + `AddonDefinition` schema** (embedded catalog) | **SKA-523 reframed** | Was "shared kit"; now the engine. Migrate the 4 existing adapters onto it (each its own e2e-gated PR — see §6). |
| P0-C | **Absence semantics** — `required`/`optional`, `Absent` reason + status count | **new** | Ends the NotFound inconsistency. |
| P0-D | **Definition/policy validation + status conditions** | **SKA-54** | Largely CEL on the `AddonDefinition`/policy + engine checks (unknown family, bad thresholds → `Accepted=False`). |
| P0-E | **Per-addon scoped client (impersonation) + generated per-addon RBAC + read-only CI guard** | **SKA-58 evolved** | Phased behind the aggregate role. |
| P0-F | **Version detection + supported-version gating** | **new** (relates SKA-425/426) | |
| P0-G | **Per-CRD version map** | **SKA-425** | Folds into the `crd-established` evaluator. |
| P0-H | **Authoring guide** → "how to write an `AddonDefinition` (+ when to escape to Go)" | **SKA-68** | Trails P0-B by one addon. |

SKA-424 is done; SKA-426 stays deferred until a real upstream field rename.

---

## 4. Addon build-out (reframed)

With the engine in place, most addons are a **YAML `AddonDefinition` + an e2e
fixture entry** — not a Go package. The few that need bespoke logic get a Go escape
hatch (and several of *those* fold back to declarative once the `expiry-threshold`
/ `recency` / `node-annotation` evaluators exist).

| Addon | Ticket | Delivery | Why |
| --- | --- | --- | --- |
| istio mesh | SKA-60 | Declarative *(realigned 2026-07-06; was Go)* | ambient/sidecar topology via `Absence: Optional` (SKA-526) + the `WebhookCheck` evaluator for injector/validator wiring; `mesh_status` (proxy sync — needs XDS/metrics, not API reads) deferred to a future active probe that would compose with the engine |
| external-dns | SKA-62 | Declarative | Deployment + optional DNSEndpoint status |
| metrics-server | SKA-65 | Declarative | Deployment + `apiservice-available` |
| envoy-gateway | SKA-507 | Declarative* | Gateway/HTTPRoute conditions; *dynamically-named proxy Deployments may need a label-selector workload evaluator |
| KEDA | SKA-508 | Declarative | Deployments + APIService + ScaledObject `Ready` |
| VPA | SKA-509 | Declarative | Deployments (+ webhook presence) + VPA condition |
| descheduler | SKA-510 | Declarative* | *needs a CronJob/last-Job-success evaluator for the periodic mode |
| node-local-dns | SKA-511 | Declarative | DaemonSet ready |
| kured | SKA-512 | Declarative | DaemonSet + `node-annotation` (reboot lock) |
| Argo CD | SKA-513 | Declarative | many Deployments + StatefulSet + Application `health`/`sync` (see §6 noise note) |
| Trident | SKA-514 | Declarative | Deployment + DaemonSet + CSIDriver + backend `condition-status` |
| kube-state-metrics | SKA-515 | Declarative | Deployment ready |
| Prometheus Operator | SKA-64 | Declarative | Deployment (+ webhook) + Prometheus/Alertmanager status |
| Velero | SKA-67 | Declarative | Deployment + BSL `condition-status` + `recency` (last backup) |
| Aqua | SKA-516 | Declarative | DaemonSet + Deployment (+ webhook) |
| Azure WI webhook | SKA-517 | Declarative | Deployment + `webhook-present` |
| Dynatrace | SKA-518 | Declarative | Deployments + DaemonSet + StatefulSet + DynaKube condition |
| cert-manager | (shipped) | **Go** (partial) | admission dry-run probe; passive parts are declarative-able |
| CoreDNS | (shipped) | **Go** | active DNS probe pod |
| Cilium | (shipped) | Declarative-able | pure workload/CRD reads; good first migration target for P0-B |

`*` = declarative once the noted evaluator lands. Net: ~14 declarative, ~2–3 Go.

---

## 5. Sequencing

```
Phase 0 (foundation, mostly serial where noted):
  P0-A periodic/on-demand exec ───────────────► BLOCKER, first
  P0-B declarative engine + evaluators ◄── needs nothing but unblocks all addons
  P0-G per-CRD version map ──┐ (into crd-established evaluator)
  P0-C absence  P0-D validation  P0-F version-detect  (parallel, on top of P0-B)
  P0-E scoped-client/impersonation (parallel; phased, can trail)
  P0-H authoring guide (trails P0-B by one addon)

Phase 1  migrate the 4 shipped adapters onto the engine (Cilium first; each e2e-gated PR)
Phase 2  declarative definitions batch — cheap YAML PRs, highly parallel
Phase 3  add expiry/recency/CronJob evaluators to reclaim cert-manager-passive
         / Velero / descheduler as declarative
         (istio left this phase 2026-07-06: it shipped declarative in Phase 2
         via Optional absence + the WebhookCheck evaluator — see §2.1/§4)
```

---

## 6. Risks & open questions (corrected)

1. **Periodic execution is a prerequisite, not a feature.** Until P0-A lands, every
   adapter ships a check that runs once. Confirm the current behavior directly and
   land P0-A first.
2. **Managed-CR aggregation noise.** Rolling "worst-of" across hundreds of managed
   CRs (Argo CD Applications, Velero Backups) means one bad user workload fails the
   whole addon-health check. The `condition-status` evaluator needs **scoping +
   quorum/ratio semantics** (e.g. "Fail if >5% Degraded"), not just worst-of. Open
   design point for P0-B.
3. **Impersonation is itself a privilege.** `impersonate` is powerful; scope it to
   the specific per-addon SAs, audit it, and keep the generated roles read-only
   (CI-guarded). Net blast radius still shrinks vs. today's shared god-role.
4. **Declarative expressiveness ceiling.** The escape hatch exists precisely because
   some checks won't fit; the risk is scope-creeping the `AddonDefinition` schema
   toward a general-purpose DSL. Hold the line: add an evaluator only when ≥2
   addons need it; otherwise write Go.
5. **e2e at scale** — unchanged from v1: tier/matrix the addon stack (**SKA-524**)
   so per-addon e2e stays fast. Declarative definitions still need a real install to
   validate.
6. **Version-detection false signals.** Labels/image tags can be absent or
   mislabeled; that's why out-of-range/unknown is `Warn`, not `Fail`.
7. **Absence representation** (§2.3) — reason-tagged reuse vs. a distinct `Absent`
   verdict — is a call to make in P0-C.
8. **Non-risk (v1 overstated):** HealthReport volume (~N×10 CRs) and metric
   cardinality (labels `{adapter, family, outcome}`, no per-object label) are both
   bounded — not a scaling concern.

## 7. Definition of done

**Per declarative addon:** `AddonDefinition` (embedded); correct `required`/optional
posture; declared `rbac:` generates a read-only scoped role; e2e fixture entry +
assertion (or documented reason it couldn't run locally); README addon-list entry.
**Per Go escape:** the above minus the definition, plus the bespoke adapter + unit
suite; e2e required. **Every addon:** one focused PR, Conventional Commit, DCO
signed-off.

## 8. Ticket implications

- **New tickets to file:** P0-A periodic/on-demand execution *(blocker)*; P0-C
  absence semantics; P0-F version detection + gating.
- **Reframe:** SKA-523 → declarative engine + evaluator library (was "shared kit");
  SKA-58 → per-addon generated RBAC + impersonation + read-only CI guard; SKA-54 →
  `AddonDefinition`/policy validation + status conditions; SKA-425 → the
  `crd-established` evaluator's version map.
- **Per-adapter tickets** (SKA-60/62/65/507/508/509/510/511/512/64/67/513–518) →
  restated as "add `AddonDefinition` for X" (declarative) or "write Go adapter for
  X" (istio; cert-manager/CoreDNS already shipped). SKA-524 (e2e tiering) unchanged.
- SKA-424 done; SKA-426 deferred.

### Ticket cross-reference

Foundation: **new** (periodic exec), SKA-523 (engine), **new** (absence), SKA-54
(validation), SKA-58 (scoped client), **new** (version detect), SKA-425 (CRD
version map), SKA-68 (authoring guide), SKA-524 (e2e tiering). Addons: SKA-60
(Go), SKA-62/65/507/508/509/510/511/512/64/67/513/514/515/516/517/518 (declarative).
