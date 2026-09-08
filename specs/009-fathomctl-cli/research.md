<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Research: fathomctl CLI

Every finding below was verified against the tree on 2026-09-07 (branch
`feature/204-fathomctl-cli`, based on `main` at `45c6ea9`).

## R1: Binary and package layout

**Decision**: Add `cmd/fathomctl/main.go` as a thin entrypoint over a new
`internal/cli` package, mirroring how `cmd/main.go` delegates to
`internal/app`. `internal/cli` owns the cobra tree, global options, client
construction, per-kind descriptors, verdict normalisation, output rendering,
and the five verbs. Test seams are unexported function fields on the client
factory (kubeconfig loader, client constructor, poll interval, stdin,
terminal detection), tested in-package like `internal/app`.

**Rationale**: `AGENTS.md` names `internal/app` as "the unit-testable seam"
and the issue (#258) asks for the same pattern. Keeping the CLI out of
`internal/app` avoids pulling controller-runtime manager, adapters, and
tracing into the client binary.

**Alternatives considered**:

- A `pkg/fathomctl` public package: no external consumer exists; `pkg/` is
  reserved for real external consumers.
- Reusing `internal/app`'s viper plumbing: the CLI has five flags and no
  config file; a second viper surface would be a second config style.

## R2: Cluster client

**Decision**: Build a `*rest.Config` with `clientcmd`'s default loading rules
(`--kubeconfig` sets `ExplicitPath`, `--context` sets `CurrentContext`), then
a cache-less controller-runtime `client.Client` over a scheme containing
client-go's types plus `fathom.skaphos.io/v1alpha1`. Set `rest.Config.Timeout`
from a `--request-timeout` flag (default 30s) and a `fathomctl/<version>`
user agent. Import `k8s.io/client-go/plugin/pkg/client/auth` for exec and
OIDC plugins, as `cmd/main.go` does.

**Rationale**: `clientcmd` gives kubectl-identical discovery (`KUBECONFIG`,
`~/.kube/config`, in-cluster) for free. The controller-runtime client accepts
the typed CRD structs already in `api/v1alpha1`, and its `fake` package is
the test double the repository already uses.

**Alternatives considered**:

- A generated typed clientset: none exists in the repository and generating
  one adds a code-generation pipeline for one consumer.
- `unstructured` access: loses compile-time checks the typed structs give.

## R3: Kind addressing and aliases

**Decision**: A single descriptor table in `internal/cli` maps user input to
kind. Accepted spellings are the Kind (`DNSCheck`), its lowercase form, the
plural resource name (`dnschecks`), and CLI-only short aliases: `ac`,
`dns`, `ncc`, `hc`, `ch`. Both `<kind>/<name>` and `<kind> <name>` are
parsed. Each descriptor records scope (`ClusterHealth` is cluster-scoped),
whether the kind is executable, how to read `spec.paused`, and how to
normalise status.

**Rationale**: The CRDs declare no `shortNames`
(`+kubebuilder:resource:categories=fathom` only), so kubectl has none either.
Adding CRD short names is a schema change gated by `crd-compat` and is out
of scope. The reference page states that the aliases are CLI-only.

## R4: Verdict normalisation per kind

**Decision**: One `snapshot` type (verdict, summary, last run, next run,
report name, consumed trigger) with one extractor per kind:

| Kind | Verdict | Summary | Last run | Interval |
|---|---|---|---|---|
| AddonCheck | `status.lastResult` | `Ready` condition message | `status.lastRunTime` | `spec.interval` (default 5m) |
| DNSCheck | `status.lastResult` | `status.summary` | `status.lastRunTime` | `spec.interval` |
| NodeCertificateCheck | `status.lastResult` | `Ready` condition message | `status.lastRunTime` | `spec.interval` (default 1h) |
| HealthCheck | `status.result` | `status.summary` | `status.sourceObservedAt` | `status.sourceInterval` |
| ClusterHealth | `status.result` | derived: `<matched> matched, worst <result>` | `status.observedAt` | none |

An empty verdict renders as `-` (never run), never as `Unknown`.

**Rationale**: FR-012 requires one normalisation shared by `ls`, `describe`,
and `run --wait`. The field names above are the current status types; the
AddonCheck and NodeCertificateCheck types carry no summary field, so the
`Ready` condition message is the bounded human-readable line the operator
already writes.

## R5: HealthReport history lookup

**Decision**: `reports` lists `HealthReport`s in the check's namespace with
the label selector `fathom.skaphos.io/source-kind=<Kind>,
fathom.skaphos.io/source-name=<name>`, sorts by `spec.observedAt`
descending, applies `--since` and `--limit` client-side, and computes "what
changed" by comparing each report's `spec.result` and per-check results to
the next-older report. `--report <name>` does a direct `Get`.

**Rationale**: All three report builders (`healthReportForAddonCheck`,
`healthReportForDNSCheck`, `healthReportForNodeCert`) stamp exactly these
two labels, so the lookup is server-side filtered and needs no index.
Derived kinds do not write reports; `reports` on a HealthCheck follows
`spec.checkRef` to its source and says so, and on a ClusterHealth it is
rejected with a pointer to the sources.

**Alternatives considered**:

- Following `status.lastReportName` only: gives one report, not history.
- Owner references: reports carry a controller reference to the check, but
  label selection is the cheaper server-side filter.

## R6: Generalising the on-demand trigger (#264)

**Decision**: Move the annotation key to `api/v1alpha1` as
`AnnotationRunNow = "fathom.skaphos.io/run-now"` and add a small shared
helper in `internal/controller` (`runTriggerDue(annotations, lastConsumed)
(token string, due bool)`). Then:

- **AddonCheck**: unchanged semantics; `addonCheckDueForRun` calls the
  helper instead of reading the annotation inline.
- **DNSCheck**: the reconciler already evaluates on every reconcile, and its
  `For()` watch has no generation predicate, so an annotation write already
  causes a run. The gap is that `status.lastRunTrigger` is declared but
  never written. The reconciler records the token after the run.
- **NodeCertificateCheck**: add `status.lastRunTrigger`. The reconciler
  copies the token into the DaemonSet pod template as an annotation and a
  downward-API environment variable (`FATHOM_RUN_TRIGGER`). The template is
  part of the spec hash, so the existing rollout path restarts the agents;
  each agent scans on start and stamps the trigger into its `NodeReport`
  (new `trigger` field on the wire type). The operator records the token in
  status only once every desired node's fresh report carries it; until then
  the prior verdict is kept (the freeze semantics #275 is introducing).
- **HealthCheck / ClusterHealth**: no controller change. Propagation is a
  CLI concern (R7).

**Rationale**: One helper, no per-controller copies (the #264 acceptance).
The DaemonSet rollout is the only existing channel from operator to agents,
it is already hash-gated so it cannot loop, and the agents are tiny. Carrying
the token back in the report is what makes completion per-token and per-node
rather than "some reports look newer".

**Alternatives considered**:

- Agents watching their own DaemonSet or a ConfigMap for a trigger: adds
  watch RBAC and a control loop to the agent for one feature.
- Operator patching each agent pod's annotations with a downward-API volume
  the agent tails: avoids restarts but needs `pods/patch` on every node pod
  and a file watcher in the agent.
- Treating "reports newer than the trigger time" as completion: relies on
  clock agreement between operator and nodes and cannot distinguish a
  routine tick from the forced scan.

## R7: Propagation from derived kinds

**Decision**: The CLI resolves sources before writing anything. For a
HealthCheck it reads `spec.checkRef` (defaulting namespace to the
HealthCheck's). For a ClusterHealth it reads `status.children`
(namespace/name of every selected HealthCheck, capped by the controller),
fetches each HealthCheck, and collects the distinct `checkRef`s. It then
treats the resolved set exactly like a selector match: count, confirmation
above 10, one token written to each, per-source `--wait`.

**Rationale**: `status.children` is the operator's own record of selection,
so the CLI never re-implements namespace and label filtering. Using the
capped child list is what "bounded by the existing child cap" means in
FR-025.

## R8: `--wait` loop

**Decision**: Poll with `wait.PollUntilContextTimeout` every 2 seconds (the
interval is a factory field so tests use milliseconds). Each tick re-reads
the object and decides: annotation no longer equals the token → superseded
(fail); `status.lastRunTrigger == token` → complete, print the snapshot;
otherwise keep polling. Default timeout is the check's effective
`spec.timeout` plus 30 seconds; for several targets, the largest.

**Rationale**: Every controller writes `lastRunTrigger` in the same status
update as the run's result, so equality on the token is equality on
completion. Polling avoids a watch client and cache; at 2 seconds against a
handful of objects it is negligible load. The per-kind default timeouts
live in `internal/controller` today; the plan exports them from
`api/v1alpha1` so the CLI and controllers share one value.

**Alternatives considered**:

- `client.WithWatch`: less latency, more plumbing and a harder test story
  for a wait that is bounded anyway.

## R9: Confirmation prompt

**Decision**: When the resolved target set exceeds 10, the CLI prints the
count and reads one line from stdin only if stdin is a terminal (checked
with `golang.org/x/term`, already an indirect dependency); otherwise it
aborts with "use --yes". `--yes` skips the prompt; `--dry-run` prints the
set and exits 0. Both are `run`-local flags.

## R10: Operator version discovery

**Decision**: `version` lists Deployments labelled
`control-plane=controller-manager` (in `-n` if given, else all namespaces),
keeps those whose `app.kubernetes.io/name` starts with `fathom` or whose
container image contains `fathom-operator`, and reports
`app.kubernetes.io/version` when present (Helm sets it), else the image tag
of the `manager` container, else the image digest. Any failure is reported
in the output as `unavailable (<reason>)` with exit 0.

**Rationale**: Both supported installs (kustomize `config/manager` and the
Helm chart) carry `control-plane: controller-manager`; only Helm carries the
version label. `fathomctl` therefore needs `get/list` on `apps/deployments`
for this one verb; the viewer role grants it and the docs say the operator
line degrades to unavailable without it.

## R11: Distribution

**Decision**: A `scripts/fathomctl-dist.sh` cross-compiles six targets
(`linux`, `darwin`, `windows` × `amd64`, `arm64`) with
`-trimpath -ldflags "-s -w -X github.com/skaphos/fathom/internal/cli.Version=v<ver>"`,
packages `fathomctl_<ver>_<os>_<arch>.tar.gz` (`.zip` on Windows) containing
the binary and `LICENSE`, and writes `fathomctl_<ver>_checksums.txt`. Task
`fathomctl-dist` wraps it; task `fathomctl-build` builds `bin/fathomctl`
with `git describe` as the version and is added to `build`. The release
workflow runs the dist task, `cosign sign-blob --bundle` on the checksum
file, `actions/attest-build-provenance` with `subject-path` on the archives,
and adds `dist/fathomctl/*` to the release assets. No Dockerfile.

**Rationale**: Matches the keyless, OIDC-bound posture of the image
pipeline and gives a verifier one identity to check. A shell script is
testable from `scripts/*_gate_test.go` like the other gates.

**Alternatives considered**:

- goreleaser: a new tool and config to pin for six archives.
- A `fathomctl` image: the spec forbids it; a client tool has no in-cluster
  role.

## R12: RBAC for CLI users

**Decision**: Ship two auxiliary ClusterRoles under `config/rbac/`:
`fathomctl-viewer-role` (get/list/watch on all five check kinds,
`healthreports`, and `apps/deployments`) and `fathomctl-runner-role`
(the viewer rules plus `patch` on `addonchecks`, `dnschecks`,
`nodecertificatechecks`). Document them and the per-verb needs in
`docs/reference/fathomctl.md` and the "Auxiliary roles" section of
`docs/reference/operator-rbac.md`. Helm parity for these convenience roles
is a follow-up issue, recorded in the plan.

**Rationale**: The operator's own role is untouched (FR-026, SC-007). The
existing kubebuilder viewer roles cover four kinds and omit AddonCheck and
DNSCheck, so a purpose-built pair is clearer than patching the scaffold.

## R13: End-to-end coverage

**Decision**: One core-tier Ginkgo spec `test/e2e/fathomctl_test.go` builds
`bin/fathomctl` once with `go build`, runs it through `utils.Run` against
the Kind cluster's current context, and covers `version`, `ls -A`,
`describe`, `reports`, and `run --wait` on one AddonCheck, one DNSCheck, and
one NodeCertificateCheck, asserting exit codes and that
`status.lastRunTrigger` equals the token the CLI printed.

**Rationale**: The controllers and CRD types change, which `AGENTS.md` lists
as requiring e2e, and SC-006 requires a real triggered run per executable
kind. The core tier already installs CoreDNS, a DNSCheck fixture, and the
node-agent path.
