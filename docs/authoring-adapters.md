<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Authoring an Adapter

This guide is for **contributors adding a new add-on to Fathom** — a new value
for `AddonCheck.spec.addonType` that validates some platform component
(external-dns, metrics-server, Argo CD, …). It covers the two ways to write an
adapter, the checks you can express, how to declare least-privilege RBAC, and
the versioning and release expectations your PR must meet.

If instead you *run* Fathom and want to configure existing adapters, you want
[Add-on checks](guides/addon-checks.md), not this page. For the runtime shape —
reconcilers, the AddonCheck → HealthCheck → ClusterHealth chain, the probe-pod
model — see [architecture.md](architecture.md).

> **Packaging today.** Adapters are **compiled into the operator**: a definition
> is a Go literal, registered in [`BuiltInAdapters()`](../internal/app/run.go).
> A runtime `AddonDefinition` CRD / ConfigMap catalog is a later phase (see the
> [implementation plan](design/addon-adapters-implementation-plan.md) §2.1). So
> "authoring an adapter" means opening a PR against this repo, not shipping a
> sidecar. The contract in [`pkg/adapter`](../pkg/adapter) is nonetheless
> versioned as if adapters were external, so the seam stays honest.

## Two ways to add an adapter

| Path | Use it when | Cost |
| --- | --- | --- |
| **A — declarative `AddonDefinition`** *(default, start here)* | The add-on's health is "operator workloads healthy + CRDs established + managed-CR `status.conditions` in the expected state." | A data literal + an e2e spec. No new Go check logic. |
| **B — Go adapter** *(escape hatch)* | You need something the evaluators can't express: an **active probe** (create a pod / issue a dry-run request), **conditional topology** (istio ambient-vs-sidecar), or a **bespoke API call**. | A full Go type implementing the [`Adapter`](../pkg/adapter/adapter.go) interface. |

This is an **authoring-time choice, not a runtime fallback**: each `addonType`
is served by exactly one registered adapter. The reconciler just looks the
adapter up by `addonType`, and the registry rejects two adapters that claim the
same type — a declarative `Engine` and a Go adapter can't both answer for one
add-on. Whether that single adapter is a declarative `Engine` or a Go type is
your call. Of the four built-ins, **CoreDNS** (DNS-resolution probe pod) and
**cert-manager** (admission dry-run `create`) are Go; **External Secrets** and
**Cilium** are declarative. The planned build-out is ~14 declarative and ~2–3 Go
(istio being the notable Go one). Default to Path A and only reach for Go when a
check genuinely can't be a read-and-compare.

---

## Path A — write an `AddonDefinition`

An [`AddonDefinition`](../internal/adapter/declarative/definition.go) is the
complete description of one adapter as data. `declarative.MustEngine(def)`
turns it into an `adapter.Adapter`; the engine reproduces the hand-written
adapters' behavior check-for-check.

### Top-level shape

```go
declarative.AddonDefinition{
    AddonType:         "external-dns",         // identity + spec.addonType match key
    AdapterVersion:    "0.1.0",                // this adapter's own SemVer
    Optional:          false,                  // absent add-on -> Fail (default) vs Skipped
    VersionSource:     &declarative.VersionSource{ /* … */ }, // optional release-version detection
    SupportedVersions: "",                     // optional semver RANGE to gate the detected version
    Families:          []declarative.FamilyDefinition{ /* … */ },
    RBAC:              []adapter.PolicyRule{ /* least-privilege reads — see below */ },
}
```

| Field | Meaning |
| --- | --- |
| `AddonType` | The adapter's `Name()`, its single capability, and the `AddonCheck.spec.addonType` match key. Required. |
| `AdapterVersion` | The adapter's own SemVer, surfaced as `Version()`. Bump it when you change what the adapter checks. |
| `Optional` | Makes every component default to the `Optional` posture (a not-installed target → `Skipped`, not `Fail`). Set it for add-ons that may legitimately be absent on a cluster (Cilium on a non-Cilium cluster). A component's own `Absence` always overrides this. |
| `VersionSource` | Names the workload whose version reports the installed add-on release. `nil` disables detection. |
| `SupportedVersions` | A Masterminds/Helm semver **range** (`">=1.14 <2.0"`) the detected release version is gated against. Empty = detect-and-surface only, never `Warn`. This is the add-on *release* version — **not** the per-CRD served-API versions below. |
| `Families` | The check families, evaluated in slice order. `Families[0]` is the "primary" family (where the all-disabled sentinel is emitted). |
| `RBAC` | The least-privilege grants the engine's reads need. This is the **enforced blast radius**, not documentation — see [Declare your RBAC](#declare-your-rbac). |

### Families and check types

A `FamilyDefinition` is one policy-keyed family (`spec.policy.<name>`). It holds
typed slices of checks, evaluated in a **fixed within-family order: Workloads →
CRDs → ManagedResources → APIServices**.

```go
declarative.FamilyDefinition{
    Name:           adapter.Family("system_health"),
    DefaultEnabled: true,   // runs when the AddonCheck policy names no families
    Workloads:      []declarative.WorkloadCheck{ /* … */ },
    CRDs:           []declarative.CRDCheck{ /* … */ },
    ManagedResources: []declarative.ConditionCheck{ /* … */ },
    APIServices:    []declarative.ConditionCheck{ /* … */ },
    // Webhooks: reserved — see note below.
}
```

Family gating: a `nil` policy enables every `DefaultEnabled` family; a non-nil
policy that omits the family key disables it; a present entry is gated by its
`enabled`.

#### `WorkloadCheck` — a controller singleton (+ its pods)

Verifies one `Deployment` / `DaemonSet` / `StatefulSet` and, optionally, its
pods (readiness + restart-warn). Generalizes the `checkDeployment` / `checkPods`
logic across all three kinds.

| Field | Purpose |
| --- | --- |
| `Kind` | `KindDeployment` / `KindDaemonSet` / `KindStatefulSet`. |
| `DefaultNamespace` | Used when `policy.namespaces` is empty. |
| `NameThresholdKey` / `DefaultName` | Let operators point the check at a renamed workload (RKE2/k3s) via a threshold; `DefaultName` is the fallback. |
| `Component` | Recorded in `Details["component"]`. |
| `Absence` | `Required` (→ `Fail`) or `Optional` (→ `Skipped`) when the workload is not found. Overrides the addon-wide `Optional`. |
| `CheckPods` | Enables the selector → ready → restart-warn sub-check (uses the workload's own selector; `policy.labelSelector` is intentionally ignored). |
| `RestartWarnThresholdKey` / `DefaultRestartWarn` | Restart count above which a pod is `Warn`. |

Outcomes: `0` desired replicas → `Warn` ("scaled to zero"); unavailable/short of
desired → `Fail`; a rollout in progress (DaemonSet) → `Warn`; otherwise `Pass`.

#### `CRDCheck` — CRDs established at a supported version

Cluster-scoped; one `CheckResult` per name.

```go
declarative.CRDCheck{
    Names:                     []string{"dnsendpoints.externaldns.k8s.io"},
    SupportedVersions:         []string{"v1alpha1"}, // descending preference; MUST be non-empty
    Absence:                   declarative.Required,
    UnsupportedVersionOutcome: adapter.OutcomeWarn,  // default when empty
}
```

Verifies each CRD is found (else the `Absence` outcome + an absent marker),
`Established`, and serving one of `SupportedVersions` (via
`crdutil.PreferredServedVersion`; else `UnsupportedVersionOutcome`). **Leave
`SupportedVersions` non-empty** — an empty slice matches nothing and silently
fails every CRD, so the engine rejects it at construction. Each CRD carries its
own version slice, so a heterogeneous CRD is a separate entry.

#### `ConditionCheck` — a `status.conditions` predicate (ManagedResources & APIServices)

Lists managed CRs (or `APIService`s) across namespaces and scores
`status.conditions[ConditionType] == ExpectedStatus`.

```go
declarative.ConditionCheck{
    APIVersion:        "externaldns.k8s.io/v1alpha1",
    VersionCRD:        "dnsendpoints.externaldns.k8s.io", // resolve served version before listing
    SupportedVersions: []string{"v1alpha1"},
    Kind:              "DNSEndpoint",
    ListKind:          "DNSEndpointList",
    ListName:          "dnsendpoints",   // stable name on list-level results
    DefaultNamespace:  "external-dns",
    ConditionType:     "Ready",
    ExpectedStatus:    "True",
    // AbsentCondition / Mismatch default to OutcomeFail.
}
```

`VersionCRD` + `SupportedVersions` let the check keep working on clusters that
serve only a legacy API version. An **empty result set** across all namespaces
is `Skipped` (`skipReason=NoMatchingObjects`) — so a check for an add-on with no
CRs yet stays quiet. Use the same type for `APIServices` (map onto
`apiregistration.k8s.io` `APIService`, `Available=True`).

> **`Webhooks` are reserved.** `FamilyDefinition.Webhooks` / `WebhookCheck` fix
> the schema shape but have **no shipped evaluator** yet — don't populate them.
> `ValidatingWebhookConfiguration` checks still need the Go escape hatch (see
> cert-manager).

### Absence semantics

"Not installed" is tracked independently of the verdict. A not-found target
carries `Details["absent"]="true"` (via `adapter.MarkAbsent`) whether it scored
`Fail` (Required) or `Skipped` (Optional), and the count rolls up to
`AddonCheck.status.absent`. Precedence: a component's own `Absence` wins, then
the addon-wide `Optional`, else `Required` (SKA-526).

### Version detection and gating

Set `VersionSource` to detect the installed **release** version (from the
workload's `app.kubernetes.io/version` label, falling back to the container
image tag); it surfaces as `Result.DetectedVersion` →
`AddonCheck.status.detectedVersion`. Add a `SupportedVersions` range to gate it:

| `SupportedVersions` | Detected version | Result |
| --- | --- | --- |
| empty | any | detected + surfaced, **never** `Warn` |
| set | in range | no gate |
| set | out of range | `Warn` (`reason=UnsupportedAddonVersion`) |
| set | undetectable/unparseable | `Warn` (`reason=VersionUnknown`) |

Gating compares the base `major.minor.patch`, so a supported release's RC/build
metadata doesn't falsely `Warn`. The gate emits under a synthetic
`addon_version` family that is not policy-selectable. This is orthogonal to the
per-CRD served-API `SupportedVersions` on `CRDCheck`/`ConditionCheck`.

### Worked example — External Secrets

The full definition lives at
[`internal/adapter/declarative/externalsecrets.go`](../internal/adapter/declarative/externalsecrets.go).
Its shape:

- `AddonType: "external-secrets"`, `AdapterVersion: "0.3.0"`, **not** `Optional`
  (a missing ESO is a `Fail`).
- `VersionSource` on the `external-secrets` Deployment with an empty
  `SupportedVersions` (detect-only; ESO is pre-1.0).
- **RBAC:** reads on `apps/deployments`, `core/pods`,
  `apiextensions/customresourcedefinitions`, and the adapter-unique
  `external-secrets.io/externalsecrets` — whose justification spells out that it
  is *deliberately not* `SecretStores` or the synced `Secret`s, so the addon SA
  can never read secret material.
- `system_health` family: three ESO Deployment `WorkloadCheck`s + a `CRDCheck`
  over the four core ESO CRDs (`{"v1","v1beta1"}`).
- `secret_sync` family: one `ConditionCheck` over `ExternalSecret` `Ready=True`,
  version-resolved through the `externalsecrets.external-secrets.io` CRD.

For the `Optional` pattern (absent add-on → `Skipped`), see
[`cilium.go`](../internal/adapter/declarative/cilium.go).

### Register it

Add a constructor next to the definition and wire it into `BuiltInAdapters()`:

```go
// internal/adapter/declarative/externaldns.go
func NewExternalDNSEngine() *Engine { return MustEngine(ExternalDNSDefinition) }

// internal/app/run.go — BuiltInAdapters()
return []adapter.Adapter{
    certmanager.New(), coredns.New(),
    declarative.NewExternalSecretsEngine(), declarative.NewCiliumEngine(),
    declarative.NewExternalDNSEngine(), // <- new
}
```

`MustEngine` validates the definition at construction (non-empty `AddonType`
and `Families`, valid version range and `VersionSource`, unique family names,
known workload kinds, each `CRDCheck` has ≥1 name and a non-empty
`SupportedVersions`) and panics on a malformed literal — so a bad definition
fails the build, not a reconcile. `BuiltInAdapters()` is the single source of
truth consumed by both startup registration and the RBAC generator.

---

## Path B — write a Go adapter (escape hatch)

Reach for Go only when a check can't be a read-and-compare: an active probe,
conditional topology, or a bespoke API call. Implement the five-method
[`Adapter`](../pkg/adapter/adapter.go) interface (the package doc in
[`pkg/adapter/doc.go`](../pkg/adapter/doc.go) has the canonical skeleton):

```go
func (MyAdapter) Name() string            { return "my-addon" }        // == addonType
func (MyAdapter) Version() string         { return "0.1.0" }           // adapter SemVer
func (MyAdapter) ContractVersion() string { return adapter.ContractVersion }
func (MyAdapter) Capabilities() adapter.Capabilities {
    return adapter.Capabilities{AddonTypes: []string{"my-addon"}, Families: []adapter.Family{"system_health"}}
}
func (MyAdapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) { /* … */ }
```

Contract rules that matter while writing `Run`:

- **`Run`'s `error` is for adapter-level aborts only** (a failure that prevents
  producing any verdict). Per-check problems are `CheckResult`s, never a
  returned error.
- **`Fail` vs `Error`.** `OutcomeFail` = the target exists but is unhealthy;
  `OutcomeError` = the adapter *couldn't determine* health (transport error, bad
  selector). Keep them distinct — dashboards and roll-ups rely on it.
- **Every non-`Pass` `CheckResult` needs a `Summary`.** Set `Family` and a
  `TargetRef` on every result.
- **Use the markers**, don't reinvent them: `adapter.MarkAbsent(details)` for
  not-installed, `adapter.MarkVersionGate(...)` for version gating. `Details`
  values are strings (encode structure as JSON in one key).
- **Honor `ctx` and `req.Timeout`;** read only through `req.Client` (a
  least-privilege client that impersonates your addon SA — see below).
- **Reuse the primitives:** `internal/adapter/crdutil` (`Established`,
  `PreferredServedVersion`) and, for probe pods, `internal/probe`
  (`probe.Launcher`, [ADR-0003](adr/0003-probe-pod-model.md)).

The two Go built-ins are the reference implementations:
[cert-manager](../internal/adapter/certmanager/adapter.go) (admission dry-run
`create`) and [CoreDNS](../internal/adapter/coredns/adapter.go) (DNS-resolution
probe pod). Register a Go adapter the same way — its `New()` in
`BuiltInAdapters()`.

---

## Declare your RBAC

Both paths declare their reads as
[`adapter.PolicyRule`](../pkg/adapter/rbac.go)s — declarative adapters via
`AddonDefinition.RBAC`, Go adapters by implementing `RBACDeclarer.RBACRules()`.
These grants are **real and enforced**: the operator ServiceAccount holds *no*
add-on reads; the generator emits a per-addon read-only `ClusterRole` bound to a
per-addon ServiceAccount, and the reconciler **impersonates that ServiceAccount**
(via a direct, uncached client) when it runs your adapter (SKA-58). An adapter
with no rules gets no read access and fails the guard.

```go
{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch"},
    Justification: "Read the add-on's controller Deployment to score workload health. list/watch scope the read to this group; read-only."},
```

Rules for every grant:

- **A `Justification` is required** on every rule — say *why* the grant is
  needed and *why less won't do*. The read-only guard fails any empty
  justification.
- **Default to read-only** (`get`/`list`/`watch`). Any write verb must prefix
  its `Justification` with **`WRITE EXCEPTION`** and explain why the write is
  unavoidable (e.g. cert-manager's admission dry-run `create`, CoreDNS's probe
  pod `create`/`delete`). The guard rejects an unmarked write.
- The addon's ServiceAccount is named `addon-<addonType>` and labeled
  `fathom.skaphos.io/addon: <addonType>` (the reconciler resolves it by the
  label, so a `namePrefix` rename is fine).

After changing rules, **regenerate** and commit the output:

```console
$ go -C tools tool task gen:addon-rbac
```

This writes `config/rbac/addons/addon-<name>.yaml` (SA + ClusterRole +
binding), refreshes the operator impersonate Role, the Helm data file, and the
generated matrix at [`docs/reference/rbac.md`](reference/rbac.md) (generated —
don't hand-edit). `task verify-generated` (CI-gated) fails if the committed
output drifts from your rules.

---

## Versioning and release expectations

| Version | What it is | When to bump |
| --- | --- | --- |
| `AdapterVersion` / `Version()` | Your adapter's SemVer. | When you change what the adapter checks. |
| `ContractVersion` | The [`pkg/adapter`](../pkg/adapter/version.go) contract you target — currently **`0.2.0`**. Embed the constant; don't hard-code a string. | Never by you — it tracks the contract. |

Contract compatibility is enforced at registration by `EnsureCompatible`:
`>=1.0.0` needs a matching major; **pre-1.0 (`0.x.y`) needs matching major *and*
minor** — a `0.1.0` adapter is rejected against a `0.2.0` host. Because you
embed `adapter.ContractVersion`, a contract bump surfaces at build time.

Repo expectations for the PR:

- **Conventional Commits** (`feat:` for a new adapter) so `release-please`
  infers the version bump.
- **Run e2e.** Adapter code (`internal/adapter/*/adapter.go`, `pkg/adapter/*`,
  the declarative engine) is on the CLAUDE.md "requires e2e" list — envtest can
  miss reconcile-time bugs. Add the add-on to the e2e stack and run
  `go -C tools tool task test-e2e`, or note in the PR why it couldn't run
  locally (CI runs `kind e2e`).
- **Unit coverage** for new logic; the per-package gate
  (`scripts/check-coverage.sh`) must stay green.

## Ship it — new-adapter PR checklist

Copy this into your PR description and tick it off:

```markdown
### New adapter checklist (SKA-68)
- [ ] Definition (`AddonDefinition` literal) or Go `Adapter` added under `internal/adapter/…`
- [ ] Constructor wired into `BuiltInAdapters()` in `internal/app/run.go`
- [ ] `AdapterVersion` set; `ContractVersion()` returns `adapter.ContractVersion`
- [ ] `Capabilities()` lists the real `addonType` and families
- [ ] Every check sets `Family`, `TargetRef`, and a `Summary` on non-Pass results
- [ ] Absence handled (`Required`/`Optional` posture; `MarkAbsent`)
- [ ] Version detection/gating set if applicable (`VersionSource` (+ `SupportedVersions`))
- [ ] RBAC declared with a `Justification` on every grant; writes marked `WRITE EXCEPTION`
- [ ] `task gen:addon-rbac` run; `config/rbac/addons/`, Helm data, and `docs/reference/rbac.md` committed
- [ ] Unit tests added; coverage gate green (`task test` + `scripts/check-coverage.sh`)
- [ ] Add-on added to the e2e stack; `task test-e2e` run (or a note why not)
- [ ] `task lint`, `task staticcheck`, `task verify-generated` clean
- [ ] Docs: new `addonType` + families added to `docs/guides/addon-checks.md`
- [ ] Conventional-commit title (`feat(adapter): …`); DCO sign-off on every commit
```

## Reference

- [`internal/adapter/declarative/definition.go`](../internal/adapter/declarative/definition.go) — the `AddonDefinition` schema (authoritative).
- [`pkg/adapter/adapter.go`](../pkg/adapter/adapter.go) / [`rbac.go`](../pkg/adapter/rbac.go) — the adapter contract and RBAC declaration surface.
- [Add-on checks](guides/addon-checks.md) — the operator-facing view of the adapters you ship.
- [RBAC reference](reference/rbac.md) — the generated per-addon RBAC matrix.
- [API versioning](reference/api-versioning.md) — CRD API-versioning policy (distinct from the adapter contract version).
- [Implementation plan](design/addon-adapters-implementation-plan.md) — the declarative-first design and the evaluator roadmap.
