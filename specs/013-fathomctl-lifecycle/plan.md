<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Implementation Plan: fathomctl Lifecycle Verbs

**Branch**: `feature/330-fathomctl-lifecycle` | **Date**: 2026-10-11 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/013-fathomctl-lifecycle/spec.md`

## Summary

Add `install`, `upgrade`, `uninstall` and `status` to `fathomctl`. Each CLI
build embeds the Helm chart of its own release (`deploy/helm`, `go:embed`) and
drives it as a Helm release through the Helm v4 library, so `helm ls` and
`fathomctl` see the same install. The CLI applies the release's CRDs itself,
server-side, before any rollout, after comparing them with the live CRDs using
crdify as a library and an append-only upgrade-path declaration shipped in the
chart: scope changes and unsanctioned incompatible changes are refused,
sanctioned ones need `--accept-incompatible-crds`, and skipped stops are
refused with the `fathomctl` release to run next. Uninstall removes the
release and every object the operator created at run time, including the
unowned admission policy and probe Pods, while keeping CRDs and custom
resources; `--purge-crds` (with optional `--export`) removes everything. The
chart gains image-digest values, and the release workflow injects the
release's digests into the CLI.

## Technical Context

**Language/Version**: Go 1.27.2 (`go.mod`)

**Primary Dependencies**: existing — cobra, pflag, client-go/apimachinery
v0.37.1, controller-runtime v0.25.2, `golang.org/x/term`. New direct —
`helm.sh/helm/v4 v4.3.0` (action, storage/driver, chart/v2/loader, kube),
`sigs.k8s.io/crdify v0.6.0` (runner, config), `k8s.io/cli-runtime v0.37.1`;
`k8s.io/kubectl` pinned to v0.37.1. Trial-verified: no existing requirement
changes version (research R1).

**Storage**: Helm release Secrets in the release namespace (Helm's own format);
the embedded chart; no new CRD fields.

**Testing**: stdlib `testing` in `internal/cli` with the controller-runtime
fake client plus Helm's in-memory storage driver and fake kube client behind an
injected seam; `scripts/*_test.go` for chart rendering (digests, upgrade-path
file) and the upgrade-path/allowlist gate; an import-boundary test; a new e2e
shard `lifecycle` on its own kind cluster.

**Target Platform**: the existing six `fathomctl` archives (linux/darwin/
windows × amd64/arm64); the chart change applies to every install path.

**Project Type**: single Go module — operator plus the `fathomctl` client.

**Performance Goals**: `status` answers in under 5 s on a healthy cluster;
plan computation (CRD comparison of nine CRDs) under 2 s; waits bounded by
`--timeout` (default 5m).

**Constraints**: no `--version`, no run-time chart fetch (clarification D);
every API call bounded by `--request-timeout`; exit codes `0`/`1`;
`internal/cli` must not import `internal/app`, `internal/adapter`,
`internal/controller` (now gated by a test); no operator RBAC change;
`crd-compat` gate unchanged and fed by the same crdify version.

**Scale/Scope**: four verbs, one new embedding package, chart digest values,
one new chart data file, release workflow and dist script changes, nine CRDs
compared, one e2e shard, one ADR, six docs pages updated.

## Constitution Check

*GATE: passed before Phase 0 and re-checked after Phase 1.*

| Principle or constraint | Design evidence | Result |
|---|---|---|
| I. Explicit state | Install state lives in the Helm release Secret, readable by `helm`; the upgrade path and sanctioned changes are a committed, reviewed file; the release records `installed-by=fathomctl`. No side state on the workstation. | Pass |
| II. Git as desired-state boundary | The verbs are an imperative convenience over the same chart; GitOps users keep driving the chart (or Argo/Flux) and the docs say so. Nothing the CLI does is unreachable with `helm` + `kubectl`. | Pass |
| III. Deterministic operation | Each CLI release carries exactly one chart and its image digests; the plan is computed and printed before any change; values are validated by a client-side render before CRDs are touched. | Pass |
| IV. Kubernetes-native | CRDs applied server-side through the API; readiness from kstatus; no proxy. | Pass |
| V. Compose, don't trap | Optional; installs move freely between `helm` and `fathomctl`; OLM and kustomize installs are left to their tools, not taken over. | Pass |
| VI. Explainable | Every refusal names the CRD, field, reason, and next command; `status` and `--dry-run` show the upgrade path. | Pass |
| VII. Read-only degradation | `status` needs read access only; a CLI older than the install still runs `status` and `uninstall`. | Pass |
| IX. Honest scope | OLM and kustomize installs refused and documented; development builds disclosed as tag-pinned; Dynatrace/Sumo-style backends unaffected. | Pass |
| ClusterHealth contract | Untouched. | Pass |
| Bounded, idempotent reconciliation | No controller change; CLI waits bounded; `install` idempotent by version comparison. | Pass |
| Minimal RBAC | Operator role unchanged. The verbs need cluster-admin-level user rights, documented separately from the viewer/runner roles (#323). | Pass |
| Configuration model | The CLI keeps flags plus kubeconfig, no viper — the existing, justified CLI deviation from spec 009. | Justified (unchanged) |
| Adopt before build | Helm and crdify are adopted as libraries rather than reimplementing release management or CRD comparison. | Pass |
| Test and documentation gates | Unit, chart, gate and e2e coverage per verb; docs pages and an ADR. | Pass |

Post-design re-check: the data model, contracts and quickstart keep every gate.
An ADR (`docs/adr/0008-fathomctl-lifecycle-via-helm-library.md`) records the
two hard-to-reverse choices: driving installs through the Helm library from
the CLI, and the exact-version, embedded-chart policy.

## Project Structure

### Documentation (this feature)

```text
specs/013-fathomctl-lifecycle/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── cli-lifecycle.md        # verbs, flags, output, exit codes, messages
│   └── upgrade-path.md         # upgrade-path.yaml schema and path algorithm
├── checklists/requirements.md
└── tasks.md                    # created by speckit-tasks, not this plan
```

### Source Code (repository root)

```text
deploy/helm/
├── chart.go                          # package helmchart: go:embed all:fathom-operator, Load()
├── chart_test.go
└── fathom-operator/
    ├── upgrade-path.yaml             # NEW: append-only stops + sanctioned changes
    ├── values.yaml                   # + image.digest, probeImage.digest, nodeAgent.image.digest
    ├── values.schema.json            # + digest patterns
    └── templates/_helpers.tpl        # repo@digest when digest set

internal/cli/
├── lifecycle.go                      # shared: detect install, release access seam, plan types
├── lifecycle_helm.go                 # Helm v4 action wiring (the only file importing helm)
├── lifecycle_crds.go                 # CRD load/compare (crdify) + server-side apply + Established wait
├── lifecycle_path.go                 # upgrade-path.yaml parsing + path computation
├── lifecycle_cleanup.go              # operator-created object cleanup, purge, export, namespace check
├── install.go / upgrade.go / uninstall.go / status.go
├── digests.go                        # ldflags-injected image digests
└── *_test.go                         # fake client + Helm memory driver

cmd/fathomctl/                        # unchanged entrypoint

scripts/
├── fathomctl-dist.sh                 # + -X digest flags from env
├── helm_digest_test.go               # chart renders repo@digest
├── upgrade_path_gate_test.go         # SANCTIONED allowlist findings ⊆ upgrade-path entries
└── cli_import_boundary_test.go       # go list -deps ./cmd/fathomctl guard

.github/workflows/
├── release.yml                       # pass the three digests into fathomctl-dist
└── e2e.yml                           # + lifecycle shard (no kustomize pre-deploy)

test/e2e/
├── fathomctl_lifecycle_test.go       # Label("lifecycle"); runs only in the lifecycle shard
└── fixtures/lifecycle/               # older CRD fixture for the upgrade case

docs/
├── adr/0008-fathomctl-lifecycle-via-helm-library.md
├── guides/getting-started.md, guides/fathomctl.md, guides/agent-operations.md
└── reference/fathomctl.md
README.md, RELEASE.md                 # verbs, permissions, digest injection, upgrade-path upkeep
```

**Structure Decision**: the verbs live in `internal/cli` beside the existing
ones and reuse its factory, output and confirmation helpers. Helm is confined
to `lifecycle_helm.go` behind an interface the tests replace, so the rest of
the package stays testable with fakes. The chart is embedded from a new tiny
package under `deploy/helm`, the only place `go:embed` can reach it.

## Complexity Tracking

| Deviation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| `fathomctl` grows ~2.5× (42 MB → ~105 MB unstripped) and gains ~55 indirect modules (oras, kubectl, kustomize, wazero, SQL drivers) | Helm release management and CRD comparison are adopted rather than rebuilt (FR-002, "adopt before build") | Shelling out to `helm` adds an undeclared runtime dependency and version skew; embedded kustomize manifests are a second, drifting install path the spec rejects; a hand-written release store would duplicate Helm and break `helm ls` interop. `govulncheck` already gates the added surface. |
| A Go package inside `deploy/helm` | `go:embed` cannot reach a parent directory | Copying the chart into `internal/cli` at build time creates a generated duplicate to keep in sync. |
| A second e2e cluster/shard | The suite pre-deploys with kustomize, which the verbs deliberately refuse | Uninstalling the kustomize deploy mid-suite would break every other spec's assumptions. |
