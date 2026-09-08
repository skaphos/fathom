<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Implementation Plan: fathomctl CLI

**Branch**: `feature/204-fathomctl-cli` | **Date**: 2026-09-07 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/009-fathomctl-cli/spec.md`

## Summary

Add a kubectl-shaped client, `fathomctl`, as `cmd/fathomctl` over a new
`internal/cli` package: five verbs (`ls`, `describe`, `reports`, `run`,
`version`) sharing one client factory, one kind-descriptor table, and one
verdict normalisation. Generalise the existing `fathom.skaphos.io/run-now`
trigger from AddonCheck to DNSCheck and NodeCertificateCheck through a shared
controller helper, carrying the token to the node agents via the DaemonSet
template and back in each node report. Ship the CLI as six signed, attested
release archives from the existing release workflow, with no image.

## Technical Context

**Language/Version**: Go 1.27.1 (`go.mod`)

**Primary Dependencies**: cobra 1.10.2, pflag 1.0.10 (existing);
client-go / apimachinery 0.37.0 for `clientcmd`, `rest`, label selectors,
`wait.PollUntilContextTimeout`, `duration.HumanDuration`;
controller-runtime 0.25.0 `client` (cache-less) and `client/fake`;
`sigs.k8s.io/yaml`; `golang.org/x/term` (already indirect) for the
confirmation prompt. No new direct module.

**Storage**: Kubernetes CRDs only. One additive status field
(`NodeCertificateCheck.status.lastRunTrigger`) and one additive wire field
(`NodeReport.trigger`).

**Testing**: stdlib `testing` with controller-runtime fake client for
`internal/cli` (in-package, like `internal/app`); envtest/Ginkgo for the
controller helper and NodeCertificateCheck rollout path; `scripts/*_gate_test.go`
for the dist script; Kind e2e core-tier spec for the real triggered runs.

**Target Platform**: CLI on linux/darwin/windows × amd64/arm64; operator
changes on the existing Linux images.

**Project Type**: single Go module: operator plus a second client binary.

**Performance Goals**: `ls` on 100 checks under 5 s (five list calls, one per
kind); `run --wait` polls one object every 2 s; no watch, no cache.

**Constraints**: no new operator RBAC; `ClusterHealth` contract untouched;
CRD change must pass `crd-compat`; coverage gate ≥ 50% per package; kubectl
exit-code convention (`0`/`1`); binary must not import `internal/app`,
adapters, or tracing.

**Scale/Scope**: five kinds, five verbs, three controllers touched (one
shared helper), one agent field, one dist script, two auxiliary ClusterRoles,
two new docs pages plus eight existing pages updated.

## Constitution Check

*GATE: passed before Phase 0 and re-checked after Phase 1.*

| Principle or constraint | Design evidence | Result |
|---|---|---|
| I. Explicit state | The trigger stays a declared annotation with its consumed value in status; the CLI adds no side channel. `lastRunTrigger` is added to the one kind that lacked it. | Pass |
| II. Git as desired-state boundary | The CLI never creates or edits checks; `run` writes only the trigger annotation, an explicit, attributable break-glass-style request recorded in status. | Pass |
| III. Deterministic operation | Build flags, archive names, and version injection are fixed; CRD/docs regenerate via pinned tasks; the token is unique per invocation and never re-fires. | Pass |
| IV. Kubernetes-native | Reads CRD status and writes an annotation through the API; discovery via kubeconfig; no proxy or side API. | Pass |
| V. Compose, don't trap | `fathomctl` is optional; `kubectl` remains fully sufficient. No dependency on other Skaphos tools. | Pass |
| VI. Explainable reconciliation | `describe` surfaces conditions, reasons, per-target rows, and the consumed trigger; `run --wait` timeout messages name likely causes. | Pass |
| VII. Read-only degradation | Read verbs need only get/list; `version` degrades to "unavailable (reason)"; a check that cannot run keeps its last verdict. | Pass |
| IX. Honest scope | Aliases are documented as CLI-only; derived-kind propagation and the NodeCertificateCheck rollout cost are documented; no MCP claims (#322). | Pass |
| ClusterHealth contract | Untouched; the CLI reads `status.children` as a consumer. | Pass |
| Bounded, idempotent reconciliation | Helper is a pure comparison; NodeCertificateCheck rollout is hash-gated; completion is per-token. | Pass |
| Minimal RBAC | Operator role unchanged; new ClusterRoles are user-facing auxiliaries documented in `operator-rbac.md`. | Pass |
| Configuration model | The CLI uses flags plus kubeconfig discovery and deliberately not viper; recorded in Complexity Tracking. | Justified |
| Test and documentation gates | Direct tests per verb and per controller path, e2e for the trigger, generated reference regenerated, coverage gate honoured. | Pass |

Post-design re-check: the contracts, data model, and quickstart keep every
gate above. No ADR is required: the trigger mechanism and the CLI's
read-only posture are extensions of existing decisions, not new
hard-to-reverse ones. The one deviation (no viper for the CLI) is justified
below.

## Project Structure

### Documentation (this feature)

```text
specs/009-fathomctl-cli/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── cli-commands.md
│   └── run-trigger.md
└── tasks.md                          # created by speckit-tasks, not this plan
```

### Source Code (repository root)

```text
cmd/fathomctl/
├── main.go                           # thin entrypoint; exits 1 on any error
└── main_test.go                      # subprocess test, mirrors cmd/main_test.go

internal/cli/
├── doc.go                            # package doc: seams, no-viper rationale
├── root.go                           # cobra root, global flags, PersistentPreRunE
├── client.go                         # factory: clientcmd loader, scheme, cache-less client
├── kinds.go                          # descriptor table, alias parsing, checkRef
├── snapshot.go                       # per-kind verdict normalisation
├── output.go                         # table writer, json/yaml, v1.List wrapping
├── ls.go / describe.go / reports.go  # read verbs
├── run.go                            # target resolution, confirmation, token, patch
├── wait.go                           # poll loop, superseded/timeout classification
├── version.go                        # Version var, build-info fallback, operator lookup
└── *_test.go                         # in-package tests over the fake client

api/v1alpha1/
├── annotations.go                    # AnnotationRunNow
├── cadence.go                        # exported default interval/timeout constants
├── nodecertificatecheck_types.go     # + LastRunTrigger
└── zz_generated.deepcopy.go          # regenerated only

internal/controller/
├── runtrigger.go                     # runTriggerDue helper (+ test)
├── addoncheck_controller.go          # use helper
├── dnscheck_controller.go            # record lastRunTrigger
├── nodecertificatecheck_controller.go# stamp template, per-token completion
└── nodecertificatecheck_helpers.go   # DaemonSet env/annotation

internal/nodecert/types.go            # NodeReport.Trigger
cmd/node-agent/main.go                # FATHOM_RUN_TRIGGER → report

config/rbac/
├── fathomctl_viewer_role.yaml
├── fathomctl_runner_role.yaml
└── kustomization.yaml
config/crd/bases/                     # regenerated

scripts/
├── fathomctl-dist.sh
└── fathomctl_dist_gate_test.go

Taskfile.yml                          # fathomctl-build, fathomctl-dist; build includes the CLI
.github/workflows/release.yml         # dist, sign-blob, attest, assets
.gitignore                            # unchanged (bin/, dist/ already ignored)

test/e2e/fathomctl_test.go            # core tier

docs/
├── guides/fathomctl.md               # new user guide
├── guides/README.md, node-certificate-checks.md
├── reference/fathomctl.md            # new: flags, aliases, exit codes, RBAC, trigger, verify
├── reference/operator-rbac.md, status-conditions.md, api.md (generated)
├── README.md, code-map.md
README.md, RELEASE.md, AGENTS.md
```

**Structure Decision**: `internal/cli` is a sibling of `internal/app`, not a
child, so the client binary links only the API types and client libraries.
Shared trigger knowledge moves down into `api/v1alpha1` (annotation key,
cadence defaults) rather than the CLI importing `internal/controller`.

## Phase 0: Research Outcome

[research.md](research.md) resolves every technical-context question. The
two findings that shaped the design: DNSCheck already runs on each reconcile
and only needs to record the token; NodeCertificateCheck needs the token
carried to and back from the agents, done through the existing hash-gated
DaemonSet rollout and a new report field.

## Phase 1: Design Outcome

- [data-model.md](data-model.md): CLI types, descriptor table, token format,
  API and wire additions, operator and wait-loop state transitions, release
  artifact naming.
- [contracts/cli-commands.md](contracts/cli-commands.md): the user-visible
  command contract.
- [contracts/run-trigger.md](contracts/run-trigger.md): the operator-side
  trigger contract per kind and its test obligations.
- [quickstart.md](quickstart.md): build, unit, dist, e2e, release
  verification, and documentation gates.

## Implementation Strategy

Ordered so each step leaves `main` green and maps to the epic's children.

1. **Engine prerequisite (#264)**: export the annotation and cadence
   constants from `api/v1alpha1`; add `runtrigger.go` and switch AddonCheck
   to it; record the token in DNSCheck; add `lastRunTrigger` to
   NodeCertificateCheck, stamp the DaemonSet template, extend the wire type
   and agent, and complete per token. Regenerate CRDs and API reference;
   envtest coverage per kind. Coordinate with #275 (same controller): land
   whichever is ready first and rebase the other; the completion rule here
   already assumes #275's freeze semantics.
2. **Scaffold (#258)**: `cmd/fathomctl`, `internal/cli` root/client/output/
   kinds/version, tasks `fathomctl-build` and `fathomctl-dist`, the dist
   script and gate test, release workflow steps, RBAC roles, docs skeleton.
3. **Read verbs (#259, #260, #261)**: `snapshot.go`, then `ls`, `describe`,
   `reports`, each with fake-client tests for all supported kinds.
4. **`run` (#263)**: resolution, confirmation, patch, wait loop, exit codes;
   e2e spec covering all three executable kinds and the ClusterHealth
   fan-out.
5. **Documentation**: user guide, reference page, README/RELEASE/AGENTS,
   code map, RBAC and status-conditions updates; regenerate; run the full
   quality gate set.

Steps 2 and 3 can proceed in parallel with step 1 on separate PRs; step 4
depends on both.

## Verification Gates

```text
go -C tools tool task fmt
go -C tools tool task manifests generate docs:api-ref
go -C tools tool task crd-compat
go -C tools tool task lint
go -C tools tool task vet
go -C tools tool task test
scripts/check-coverage.sh coverage.out
go -C tools tool task staticcheck
go -C tools tool task vuln
go -C tools tool task build
go -C tools tool task test-e2e
reuse --no-multiprocessing lint
graphify update .
```

The full e2e stack is required before the PR is ready: controllers, CRD
types, the node-agent, and `internal/nodecert` all change. Missing local
prerequisites are recorded in the PR test plan, never skipped silently.

## Risks and Follow-ups

- **NodeCertificateCheck rollout cost**: a forced run restarts one agent pod
  per node. Documented in the node-cert guide; large clusters should pass a
  longer `--timeout`. A restart-free channel is a possible later improvement.
- **#275 overlap**: both change `nodecertificatecheck_controller.go`; merge
  order is a rebase, not a redesign.
- **Helm parity for the fathomctl ClusterRoles**: kustomize-only in this
  feature; file a follow-up to add them to `deploy/helm/fathom-operator`
  behind a values flag.
- **`spec.paused` removal (#262, v0.7.0)**: the paused pre-flight in `run`
  and the descriptor's `Paused` hook are deleted with it.
- **MCP surface**: #322, after the verbs land.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| CLI configuration uses flags plus kubeconfig discovery instead of the cobra + viper `flag → FATHOM_* env → config file → default` model | A client tool has five flags, no ConfigMap to mount, and must behave like kubectl (`KUBECONFIG`, current context). | Wiring viper for five flags would create a second config style with no consumer of env or file precedence, contradicting the "no second config style" note on #258. The operator's model is unchanged. |
