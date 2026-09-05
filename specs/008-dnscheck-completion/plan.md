<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Implementation Plan: DNSCheck Completion

**Branch**: `feature/267-dnscheck-completion` | **Date**: 2026-08-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/008-dnscheck-completion/spec.md`

## Summary

Complete the existing thin-wrapper architecture by teaching the
`HealthCheckReconciler` to normalize `AddonCheck`, `DNSCheck`, and
`NodeCertificateCheck` status through explicit typed target handlers. Resolve
an empty `checkRef.apiVersion` to the current Fathom version, reject unsupported
group/versions and kinds as terminal reference errors, and add source watches
that enqueue only exact references. Preserve the existing `HealthCheckStatus`
and `ClusterHealth` contracts, then prove DNSCheck-to-ClusterHealth propagation
in Kind and document the complete authoring workflow.

## Technical Context

**Language/Version**: Go 1.27.0

**Primary Dependencies**: Kubernetes API machinery/client-go 0.36.4,
controller-runtime 0.24.1, kubebuilder v4 markers, Ginkgo 2.32.1, Gomega 1.42.1

**Storage**: Kubernetes CRDs and status subresources; `HealthReport` CRs retain
change history

**Testing**: stdlib tests, Ginkgo/Gomega envtest, Kind e2e through pinned Task
wrappers

**Target Platform**: Kubernetes operator on Linux; Kubernetes test baseline
locked to the `k8s.io/*` 0.36 module line

**Project Type**: Single Go Kubernetes operator with CRD API, controllers,
generated manifests, and user documentation

**Performance Goals**: At most one target `Get` per active, supported
HealthCheck reconciliation; source events enqueue only exact
group/version/kind/namespace/name references; no new polling loop or unbounded
work

**Constraints**: Keep `ClusterHealth` derived only from `HealthCheck.status`;
preserve snapshots on pause and transient API failures; clear them for terminal
reference failures; bound summaries to 1,024 Unicode code points; add no net
new cluster-wide RBAC

**Scale/Scope**: Three in-tree specialized kinds, one normalized snapshot per
HealthCheck, existing list-and-filter watch mapping over HealthChecks, and one
new core-tier DNS aggregation e2e scenario

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1.*

| Principle or constraint | Design evidence | Result |
|---|---|---|
| Explicit state | `checkRef` remains the declared source; normalized evidence remains in status and HealthReport history. | Pass |
| Git/deterministic operation | CRD and docs outputs are regenerated with pinned tasks; no live-only configuration is introduced. | Pass |
| Kubernetes-native | Uses typed CRD reads, status conditions, controller-runtime watches, and existing events/metrics. | Pass |
| Compose, don't trap | Specialized checks remain independently useful; the wrapper is a one-directional projection. | Pass |
| Explainable reconciliation | Unsupported versions/kinds, missing targets, and lookup failures retain distinct bounded Ready reasons. | Pass |
| Read-only degradation | Paused and transient-read paths preserve the last snapshot; history remains untouched. | Pass |
| Honest scope | Only the three resource kinds that exist are advertised; phantom kinds are removed. | Pass |
| ClusterHealth stability | No direct DNSCheck or HealthReport read is added to ClusterHealth. | Pass |
| Bounded/idempotent reconciliation | Each handler performs one typed `Get`; status writes remain semantic-change-only. | Pass |
| Minimal RBAC | HealthCheck markers cover reads already present in the aggregate operator role; generated RBAC is verified for no net expansion. | Pass |
| Test and documentation gates | Direct handler/watch tests, generated-reference verification, full unit suite, and required Kind e2e are planned. | Pass |

Post-design re-check: the target projection contract, data model, and quickstart
retain every gate above. No constitution exception or new ADR is required;
ADR-0004 already decides the wrapper architecture.

## Project Structure

### Documentation (this feature)

```text
specs/008-dnscheck-completion/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── healthcheck-target-projection.md
└── tasks.md                         # created by speckit-tasks, not this plan
```

### Source Code (repository root)

```text
api/v1alpha1/
├── healthcheck_types.go             # accurate kinds + bounded apiVersion
└── zz_generated.deepcopy.go         # regenerated only

internal/controller/
├── healthcheck_controller.go        # registry, typed handlers, shared mapper
├── healthcheck_controller_test.go   # projection/failure/watch coverage
├── healthcheck_target_handlers_test.go # typed projection/failure coverage
├── healthcheck_watch_test.go        # exact source-event mapping coverage
├── addoncheck_controller.go         # existing cadence helper, unchanged
├── dnscheck_plan.go                  # existing cadence helper, unchanged
└── nodecertificatecheck_helpers.go  # existing cadence helper, unchanged

config/
├── crd/bases/                        # regenerated CRD schema
└── rbac/                             # regenerated and checked for no expansion

test/e2e/
└── dnscheck_aggregation_test.go      # DNSCheck → HealthCheck → ClusterHealth

README.md                             # repository-level aggregation overview

docs/
├── guides/
│   ├── README.md
│   └── dns-checks.md
├── architecture.md
└── reference/api.md                  # generated only
```

**Structure Decision**: Extend the existing API and HealthCheck controller in
place. The normalized handler is private controller code because it is not a
new public extension API. No new package or ClusterHealth dependency is needed.

## Phase 0: Research Outcome

Research is recorded in [research.md](research.md). All technical-context
questions are resolved. The selected design uses an explicit in-tree handler
registry rather than unstructured objects, reflection, or direct aggregation.

## Phase 1: Design Outcome

- [data-model.md](data-model.md) defines reference normalization, handler
  identity, the normalized snapshot, and lifecycle transitions.
- [contracts/healthcheck-target-projection.md](contracts/healthcheck-target-projection.md)
  defines the external reference/status/watch behavior.
- [quickstart.md](quickstart.md) defines runnable validation for generated
  artifacts, unit/envtest coverage, and the required Kind path.

## Implementation Strategy

1. Correct `CheckTargetRef` documentation, add a 317-character length bound to
   `apiVersion` (253-character group, slash, 63-character version), and
   regenerate CRD/reference artifacts.
2. Introduce a private handler descriptor keyed by normalized API version and
   kind. Each typed handler performs one `Get` and returns the same private
   normalized snapshot.
3. Keep terminal-versus-transient handling in one orchestration path so every
   kind clears or preserves fields consistently.
4. Register one controller-runtime watch per handler object and reuse a shared
   exact-reference map function parameterized by the handler identity.
5. Extend envtest coverage across all handlers, versions, namespaces, failure
   classes, and watch matching without weakening existing AddonCheck tests.
6. Add a core-tier Kind scenario that transitions DNS from Pass to Fail and
   observes HealthCheck and ClusterHealth convergence without duplicate
   change-history records.
7. Add the DNS guide and update guide navigation/architecture; regenerate API
   reference and manifests, then run the prescribed quality gates.

## Verification Gates

```text
go -C tools tool task fmt
go -C tools tool task docs:api-ref
go -C tools tool task lint
go -C tools tool task vet
go -C tools tool task test
go -C tools tool task staticcheck
go -C tools tool task vuln
go -C tools tool task test-e2e
reuse --no-multiprocessing lint
graphify update .
```

The full e2e run is required because this changes a reconciler body and watch
wiring shared by all target kinds. If Kind, Docker, Helm, or Helmfile is not
available, record the exact missing prerequisite in the PR test plan rather
than treating the e2e gate as passed.

## Complexity Tracking

No constitution violations require justification.
