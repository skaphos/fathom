<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Code Map

A tour of the repository for new contributors: what each module is responsible
for and the key entrypoints to start reading from. For the design rationale see
[architecture.md](architecture.md); for the CRD field reference see
[reference/api.md](reference/api.md).

## Top-level layout

| Path | Responsibility |
| --- | --- |
| `cmd/` | Binary entrypoints (`main.go` operator, `probe/` probe binary). |
| `api/v1alpha1/` | CRD Go types and generated deepcopy. |
| `internal/app/` | cobra/viper wiring, options, scheme, manager construction. |
| `internal/controller/` | The three reconcilers. |
| `internal/adapter/` | Adapter registry and built-in adapters. |
| `internal/probe/` | Probe-pod manifest builder and launcher. |
| `pkg/adapter/` | The public, in-process adapter contract. |
| `config/` | kustomize bases/overlays, RBAC, CRDs, OLM, samples. |
| `test/` | Ginkgo e2e suite and shared helpers. |
| `tools/` | Pinned tooling, launched via `go -C tools tool task …`. |
| `docs/` | This documentation set, the ADRs, and the generated API ref. |

## `cmd/` — entrypoints

- `cmd/main.go` — thin operator entrypoint. Builds the cobra root via
  `app.NewRootCommand().Execute()` and exits non-zero on error. All real logic
  is in `internal/app`.
- `cmd/probe/main.go` — the standalone probe binary. Parses `-mode`
  (`dns`/`tcp-connect`/`tcp-listen`), runs the check, writes a JSON result to
  `/dev/termination-log`, and exits. Built into the probe image
  (`Dockerfile.probe`, `scratch` base). Key funcs: `run`, `runDNS`,
  `runTCPConnect`, `runTCPListen`.

## `api/v1alpha1/` — CRD types

Defines the four kinds in group `fathom.skaphos.io/v1alpha1`. One file per kind
(`addoncheck_types.go`, `healthcheck_types.go`, `clusterhealth_types.go`,
`healthreport_types.go`), plus `groupversion_info.go` (scheme registration) and
the generated `zz_generated.deepcopy.go` (**never hand-edit**).

Key types: `AddonCheckSpec/Status`, `HealthCheckSpec/Status`,
`ClusterHealthSpec/Status`, `HealthReportSpec`, and the `HealthReportResult`
enum with its `Severity()` ordering used by worst-case aggregation. The kubebuilder
markers on these types drive both the generated CRDs in `config/crd/bases/` and
the field descriptions in [reference/api.md](reference/api.md), so doc comments
here are load-bearing.

## `internal/app/` — process plumbing

The unit-testable seam between `cmd/main.go` and controller-runtime.

- `root.go` — `NewRootCommand` builds the cobra command, registers flags
  (`RegisterFlags`), resolves options (`Load`), wires signal-aware context
  (`signalContext`), and calls `Run`.
- `options.go` — `Options`, `DefaultOptions`, the `bindings()` flag/viper table,
  `RegisterFlags`, `Load` (viper precedence), and `Validate`. `DefaultProbeImage`
  and `DefaultConfigPath` live here. See
  [reference/configuration.md](reference/configuration.md).
- `run.go` — `NewScheme` (registers client-go, fathom v1alpha1, and
  apiextensions/v1), `BuildManagerOptions` (Options → `ctrl.Options` + cert
  watchers), `DefaultControllers` (constructs the three reconcilers),
  `BuildAdapterRegistry` / `builtInAdapters` (registry assembly), and `Run`
  (starts the manager, gates `/readyz` on cache sync). `managerFactory` is a
  package var so tests can swap in a fake manager.

## `internal/controller/` — reconcilers

One reconciler per file; see [architecture.md](architecture.md#reconcilers) for
ownership and watch wiring. Each implements `Reconcile` and `SetupWithManager`.

| File | Type | Notes |
| --- | --- | --- |
| `addoncheck_controller.go` | `AddonCheckReconciler` | Dispatches to adapters, creates + prunes `HealthReport`s. |
| `healthcheck_controller.go` | `HealthCheckReconciler` | Mirrors `AddonCheck.status`; watches `AddonCheck`. |
| `clusterhealth_controller.go` | `ClusterHealthReconciler` | Worst-case roll-up of `HealthCheck.status`; watches `HealthCheck`. |
| `suite_test.go` | — | envtest bootstrap for the Ginkgo controller tests. |

`+kubebuilder:rbac` markers on the reconcilers are the source of the operator's
RBAC; regenerate with `task manifests` when they change.

## `pkg/adapter/` — the adapter contract

The public, importable contract (see
[ADR-0001](adr/0001-in-process-adapter-contract.md) and
[architecture.md](architecture.md#the-in-process-adapter-contract)).

- `adapter.go` — the `Adapter` interface and its data types: `Capabilities`,
  `Family`, `Request`, `FamilyPolicy`, `Result`, `CheckResult`, `Outcome`,
  `TargetRef`.
- `version.go` — `ContractVersion` constant and `EnsureCompatible`, the SemVer
  handshake checked at registration.
- `doc.go` — package overview and an adapter-authoring example.

## `internal/adapter/` — registry and built-in adapters

- `registry/registry.go` — `Registry` keyed by add-on type. `New`, `Register`
  (runs `EnsureCompatible`, rejects nil/empty/conflicting adapters, idempotent
  re-registration), `Lookup` (`ErrNotFound`), `Capabilities`.
- `certmanager/`, `coredns/`, `externalsecrets/`, `cilium/` — built-in adapters.
  Each exposes `New()` and implements the contract; families are listed in
  [architecture.md](architecture.md#built-in-adapters). The CoreDNS adapter is
  the one that launches probe pods (`dns_resolution`) and owns
  `resolveProbeImage`.
- `crdutil/` — shared helper for adapters that verify an add-on's CRDs are
  installed and served.

## `internal/probe/` — probe-pod plumbing

Shared by adapters that run active in-cluster checks (see
[ADR-0003](adr/0003-probe-pod-model.md)).

- `pod.go` — `Pod(Request)` builds the hardened pod manifest; `Mode` constants
  (`ModeDNS`, `ModeTCPConnect`, `ModeTCPListen`); `Result` / `ParseResult`.
- `launcher.go` — `Launcher.Run` creates the pod, waits for terminal phase,
  parses the result, and always deletes the pod (best-effort, NotFound-tolerant).

## `config/` — manifests and packaging

kustomize is the source of truth for all deployable YAML.

| Subdir | Contents |
| --- | --- |
| `config/crd/bases/` | Generated CRDs (`task manifests`). |
| `config/rbac/` | Generated ClusterRole + bindings from `+kubebuilder:rbac` markers. |
| `config/manager/` | Operator Deployment base. |
| `config/default/` | Top-level kustomization composing the others. |
| `config/components/` | Opt-in overlays (e.g. Prometheus ServiceMonitor). |
| `config/network-policy/` | NetworkPolicy for metrics traffic. |
| `config/manifests/`, `config/scorecard/` | OLM bundle + scorecard scaffolding. |
| `config/samples/` | Example `AddonCheck` CRs (cert-manager, CoreDNS, ESO, Cilium). |

## `test/`

- `test/e2e/` — Ginkgo suites that run against a Kind cluster
  (`task test-e2e`), plus `fixtures/` (kind config, helmfile add-on stack).
- `test/utils/` — shared helpers used by the e2e suite.

Unit tests live next to their source as `*_test.go`; the controller suite uses
envtest (`task test`).

## Build and codegen

All workflows are wrapped as tasks (`go -C tools tool task --list`). The ones
that touch this documentation:

- `task docs:api-ref` — regenerates [reference/api.md](reference/api.md) from the
  `api/v1alpha1` doc comments via `crd-ref-docs` (config in
  `docs/.crd-ref-docs.yaml`).
- `task verify-generated` — re-runs all generators (manifests, deepcopy,
  helm:sync, docs:api-ref) and fails if anything drifted from what's committed.
