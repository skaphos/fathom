<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Tasks: fathomctl CLI

**Input**: Design documents from `specs/009-fathomctl-cli/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md,
contracts/cli-commands.md, contracts/run-trigger.md, quickstart.md

**Tests**: Included. The spec (SC-006, contracts/run-trigger.md "Test
obligations") and the constitution ("new behavior ships with direct test
coverage") require them. Unit tests in `internal/cli` are in-package
(`package cli`) over the controller-runtime fake client, like `internal/app`.

**Organization**: Grouped by user story so each story is an independently
testable increment. Story order follows spec priority: US1 `run`, US2 `ls`,
US3 `describe`, US4 `reports`, US5 install/verify/`version`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no dependency on an incomplete task)
- **[Story]**: US1..US5 from spec.md
- Every Go file carries the SPDX header from `hack/boilerplate.go.txt`; every
  non-Go file its REUSE equivalent. `reuse lint` is a CI gate.
- Every commit: Conventional Commit subject, `git commit -s`, author
  `Shawn Stratton <shawn@skaphos.io>`.

## Path Conventions

Single Go module at the repository root: `cmd/`, `internal/`, `api/`,
`config/`, `scripts/`, `test/e2e/`, `docs/`. Tasks are invoked through
`go -C tools tool task <name>`; never call `controller-gen` or `kustomize`
directly.

---

## Phase 1: Setup (CLI scaffold, #258 part 1)

**Purpose**: The binary, the command tree, client plumbing, output
encoding, and kind addressing that every verb builds on.

- [ ] T001 Create `cmd/fathomctl/main.go`: import `k8s.io/client-go/plugin/pkg/client/auth`, call `cli.NewRootCommand().Execute()`, `os.Exit(1)` on error; add `cmd/fathomctl/main_test.go` mirroring the subprocess pattern in `cmd/main_test.go` (`--help` exits 0, unknown flag exits 1)
- [ ] T002 Create `internal/cli/doc.go` (package doc: seam pattern, flags plus kubeconfig discovery, why not viper) and `internal/cli/root.go`: `NewRootCommand()`, unexported `newRootCommand(f *factory)`, `Use: "fathomctl"`, `SilenceUsage`, persistent flags `--kubeconfig`, `--context`, `-n/--namespace`, `-A/--all-namespaces`, `-o/--output` (custom `pflag.Value` accepting table|json|yaml), `--request-timeout` (default 30s); `PersistentPreRunE` validates `-n`/`-A` exclusivity and non-negative timeout
- [ ] T003 [P] Create `internal/cli/client.go`: `globalOptions` struct; `factory` with unexported seams `clientConfig func(*globalOptions) clientcmd.ClientConfig` (default: `NewDefaultClientConfigLoadingRules` with `ExplicitPath` from `--kubeconfig`, `ConfigOverrides.CurrentContext` from `--context`) and `newClient func(*rest.Config, client.Options) (client.Client, error)` (default `client.New`); `restConfig()` sets `Timeout` and `UserAgent "fathomctl/<version>"`; `client()`; `namespace()` (explicit → all → context namespace → `default`); `newScheme()` = clientgoscheme + `fathomv1alpha1`
- [ ] T004 [P] Create `internal/cli/output.go`: `outputFormat` type with `Set/String/Type`; `encode(w, format, obj)` for json (indented) and yaml (`sigs.k8s.io/yaml`); `wrapList(items []client.Object) *metav1.List`-style helper that emits unmodified objects; `newTable(w)` over `text/tabwriter` with `age(t)` via `k8s.io/apimachinery/pkg/util/duration.HumanDuration` and `truncate(s, 80)`
- [ ] T005 [P] Create `internal/cli/kinds.go`: `kindDescriptor` per data-model.md (Kind, Resource, Aliases, Namespaced, Executable, WritesReports, New, NewList, Paused, DefaultTimeout, Sources placeholder); the descriptor table for AddonCheck/DNSCheck/NodeCertificateCheck/HealthCheck/ClusterHealth with aliases `ac`,`dns`,`ncc`,`hc`,`ch`; `parseKind(s)` (case-insensitive over Kind, Resource, Aliases); `parseTarget(args, opts)` accepting `<kind>/<name>` and `<kind> <name>`, cluster-scoped kinds ignore namespace
- [ ] T006 Create `internal/cli/root_test.go` (flag parsing table incl. `-n` with `-A` error, bad `-o`, negative timeout), `internal/cli/client_test.go` (stub `clientcmd.ClientConfig`; timeout and user agent applied; scheme recognises `HealthCheck` and `Pod`; namespace resolution table; real temp kubeconfig with two contexts resolves namespace and context override), `internal/cli/output_test.go` (json/yaml round-trip, table alignment, truncate), `internal/cli/kinds_test.go` (every accepted spelling per kind, unknown kind error, both target forms)
- [ ] T007 [P] Add to `Taskfile.yml`: task `fathomctl-build` (`CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/skaphos/fathom/internal/cli.Version={{.FATHOMCTL_VERSION}}" -o bin/fathomctl ./cmd/fathomctl` with task-level var `FATHOMCTL_VERSION: sh: git describe --tags --always --dirty 2>/dev/null || echo dev`) and append `- task: fathomctl-build` to `build`
- [ ] T008 [P] Add `config/rbac/fathomctl_viewer_role.yaml` (ClusterRole `fathomctl-viewer-role`: get/list/watch on `addonchecks`, `dnschecks`, `nodecertificatechecks`, `healthchecks`, `clusterhealths`, `healthreports` and their `/status`; get/list on `apps/deployments`) and `config/rbac/fathomctl_runner_role.yaml` (`fathomctl-runner-role`: viewer rules plus `patch` on `addonchecks`, `dnschecks`, `nodecertificatechecks`), each with the "not used by the operator itself" header comment; list both in `config/rbac/kustomization.yaml`
- [ ] T009 Run `go -C tools tool task fmt lint vet test` and confirm `go -C tools tool task build` produces `bin/fathomctl` next to `bin/manager` and `bin/fathomctl --help` lists no verbs yet

**Checkpoint**: `fathomctl --help` works; every seam is unit-tested; the build task ships the binary.

---

## Phase 2: Foundational (shared normalisation and API constants)

**Purpose**: Everything two or more stories depend on. Blocks all user
stories.

- [ ] T010 Create `api/v1alpha1/annotations.go` with `AnnotationRunNow = "fathom.skaphos.io/run-now"` (doc comment moved from `internal/controller/addoncheck_controller.go`), and `api/v1alpha1/cadence.go` exporting the default cadences currently private in `internal/controller`: `DefaultAddonCheckInterval` (5m), `DefaultNodeCertificateCheckInterval` (1h), the DNSCheck default interval, and each kind's default timeout; switch `internal/controller/addoncheck_controller.go`, `internal/controller/dnscheck_plan.go`, and `internal/controller/nodecertificatecheck_helpers.go` to the exported names (no behaviour change; existing tests must stay green)
- [ ] T011 Create `internal/cli/snapshot.go`: `snapshot` struct (Verdict, Summary, LastRun, NextRun, ReportName, ConsumedTrigger) and one extractor per kind wired into the descriptor table per research.md R4 (AddonCheck/NodeCertificateCheck summary from the `Ready` condition message; DNSCheck `status.summary`; HealthCheck `status.result`/`summary`/`sourceObservedAt`/`sourceInterval`; ClusterHealth derived `"<matched> matched, worst <result>"`); `NextRun` = LastRun + effective interval using the T010 constants; empty verdict stays empty
- [ ] T012 Create `internal/cli/snapshot_test.go`: table-driven, one fixture per kind with and without status, asserting every snapshot field and that never-run yields an empty verdict (renders `-`)

**Checkpoint**: One normalisation exists and is tested; `ls`, `describe`, and `run --wait` all read from it.

---

## Phase 3: User Story 1 - Ask Fathom to validate now and get the answer (Priority: P1) 🎯 MVP

**Goal**: `fathomctl run` writes a fresh trigger to every executable kind,
propagates from derived kinds, guards bulk runs, and `--wait` returns the
verdict with kubectl-style exit codes. Delivers #264 and #263.

**Independent Test**: Against a Kind cluster with one AddonCheck, one
DNSCheck, and one NodeCertificateCheck, `fathomctl run <kind>/<name>` causes
a fresh run, `--wait` prints the verdict and exits per FR-021, and
`status.lastRunTrigger` equals the printed token afterwards.

### Engine: generalise the trigger (#264, contracts/run-trigger.md)

- [ ] T013 [US1] Create `internal/controller/runtrigger.go`: `runTriggerDue(annotations map[string]string, lastConsumed string) (token string, due bool)` returning the annotation value and whether it is non-empty and differs from `lastConsumed`; `internal/controller/runtrigger_test.go` covering empty, new, same, and cleared-annotation cases
- [ ] T014 [US1] Refactor `internal/controller/addoncheck_controller.go`: delete the private `annotationRunNow`, read `fathomv1alpha1.AnnotationRunNow`, make `addonCheckDueForRun` use `runTriggerDue`; keep the "only overwrite `LastRunTrigger` on non-empty" rule; existing tests in `internal/controller/addoncheck_controller_test.go` stay green
- [ ] T015 [US1] Update `internal/controller/dnscheck_controller.go`: after the run, if `runTriggerDue(check.Annotations, check.Status.LastRunTrigger)` reports a token, set `check.Status.LastRunTrigger` in the same status update; add envtest cases in `internal/controller/dnscheck_controller_test.go` for new value recorded, same value not re-recorded, no annotation preserves the stored value
- [ ] T016 [US1] Add `LastRunTrigger string` (`+optional`, `+kubebuilder:validation:MaxLength=253`, same doc comment as DNSCheck) to `NodeCertificateCheckStatus` in `api/v1alpha1/nodecertificatecheck_types.go`; run `go -C tools tool task generate manifests docs:api-ref crd-compat` and commit the regenerated `api/v1alpha1/zz_generated.deepcopy.go`, `config/crd/bases/`, `deploy/helm/fathom-operator/crds/`, and `docs/reference/api.md`
- [ ] T017 [P] [US1] Add `Trigger string` (`json:"trigger,omitempty"`, doc: the run-now token the agent started with, empty on routine ticks) to `NodeReport` in `internal/nodecert/types.go`; in `cmd/node-agent/main.go` read `FATHOM_RUN_TRIGGER` into `config` and stamp it into every report in `scanAndPublish`; unit test in `cmd/node-agent/main_test.go` (or the existing test file) that set and unset env produce the expected field
- [ ] T018 [US1] Update `internal/controller/nodecertificatecheck_helpers.go` `desiredDaemonSet`: when the check carries a run-now token, add pod-template annotation `fathom.skaphos.io/run-now: <token>` and container env `FATHOM_RUN_TRIGGER` via downward API `fieldRef metadata.annotations['fathom.skaphos.io/run-now']`; confirm `nodeAgentSpecHash` covers the template so the rollout fires; unit test that the hash changes with the token and is stable without it
- [ ] T019 [US1] Update `internal/controller/nodecertificatecheck_controller.go`: compute the pending token with `runTriggerDue`; in the rollup path, set `Status.LastRunTrigger = token` only when every desired node's fresh report has `Trigger == token`; until then keep the prior verdict and `LastRunTrigger` (freeze, aligned with #275); envtest cases in `internal/controller/nodecertificatecheck_controller_test.go`: token stamped on the DaemonSet, completion only when all nodes report the token, empty-trigger reports never complete, same token does not re-roll the DaemonSet, paused leaves the token unconsumed
- [ ] T020 [US1] Document the trigger contract: new section "On-demand runs" in `docs/reference/status-conditions.md` (`lastRunTrigger` on all three kinds, consume-once rule) and a "Forcing a scan" note in `docs/guides/node-certificate-checks.md` (rolling restart cost, `--timeout` advice)

### CLI: `run` and `--wait` (#263, contracts/cli-commands.md)

- [ ] T021 [US1] Implement `Sources` in `internal/cli/kinds.go` for HealthCheck (`spec.checkRef`, namespace defaulted to the HealthCheck's) and ClusterHealth (each `status.children` HealthCheck → its `checkRef`), returning executable `checkRef`s; unit tests in `internal/cli/kinds_test.go` with fake-client fixtures including shared sources and a missing target
- [ ] T022 [US1] Create `internal/cli/run.go`: `newRunCommand(f)` with flags `--wait`, `--timeout`, `--yes`, `--dry-run`, `--all`, `-l/--selector`; exactly one of target/`--all`/`-l` required; resolve targets (direct, selector list per executable kind, derived via `Sources`), de-duplicate, exclude paused/missing with a per-target message; print count; >10 → prompt on terminal (`golang.org/x/term.IsTerminal` on stdin fd, seams `stdin io.Reader` and `isTerminal func() bool` on `factory`) or abort "use --yes"; `--dry-run` prints the set and exits 0; `newToken()` = `time.Now().UTC().Format(time.RFC3339) + "-" + 6 hex chars from crypto/rand`; write via `client.MergeFrom` patch on `metadata.annotations[fathom.skaphos.io/run-now]`; per-target `runOutcome`; table and json/yaml output; return error (exit 1) if any target failed to write
- [ ] T023 [US1] Create `internal/cli/wait.go`: `waitForRun(ctx, f, target, token, timeout)` using `wait.PollUntilContextTimeout` with `f.pollInterval` (default 2s); classify each tick: annotation ≠ token → superseded; `snapshot.ConsumedTrigger == token` → complete with snapshot; deadline → timed out with hints (operator version, paused, node-agent rollout); default timeout = max over targets of `DefaultTimeout(obj)` + 30s; exit rule: error unless every verdict ∈ {Pass, Warn, Skipped}
- [ ] T024 [US1] Create `internal/cli/run_test.go` and `internal/cli/wait_test.go`: token format regex; target resolution table (direct, HealthCheck, ClusterHealth fan-out, de-dup, paused excluded, nothing left → error); >10 prompt accepted/declined/non-terminal; `--yes`; `--dry-run` writes nothing; patch lands the annotation on the fake client; wait completes when a goroutine sets `lastRunTrigger`, reports superseded when the annotation changes, times out with the hint text, and the exit outcome per verdict (Pass/Warn/Skipped → nil, Fail/Error/Unknown → error) with a millisecond poll interval
- [ ] T025 [US1] Register `newRunCommand` in `internal/cli/root.go` and extend `internal/cli/root_test.go` to assert the verb list is exactly `run` plus completion/help at this point
- [ ] T026 [US1] Create `test/e2e/fathomctl_test.go` (core tier, `Ordered`, `Label(utils.CoreLabel, "fathomctl")`): `BeforeAll` builds `bin/fathomctl` via `utils.Run(exec.Command("go", "build", "-o", "bin/fathomctl", "./cmd/fathomctl"))`; specs: `run --wait` on the existing AddonCheck, DNSCheck, and NodeCertificateCheck fixtures exits per verdict and `kubectl get ... -o jsonpath={.status.lastRunTrigger}` equals the printed token; re-running with the same token via `kubectl annotate` produces no new `lastRunTime`; `run clusterhealth/<name> --yes --wait` triggers each source once; `run` on a paused AddonCheck exits 1 without writing
- [ ] T027 [US1] Create `docs/reference/fathomctl.md` with the sections "Global flags", "Kind names and aliases (CLI-only)", "Exit codes" (0/1, kubectl convention), "run: propagation from HealthCheck and ClusterHealth", "run: bulk confirmation and --dry-run", "RBAC for fathomctl users" (viewer and runner roles, per-verb verbs); link it from `docs/README.md` reference table

**Checkpoint**: `fathomctl run` works end to end on every executable kind; #264 and #263 acceptance met.

---

## Phase 4: User Story 2 - See every verdict in the cluster at a glance (Priority: P1)

**Goal**: `fathomctl ls` lists all five kinds with normalised verdicts.
Delivers #259.

**Independent Test**: With resources of each kind and mixed verdicts,
`fathomctl ls`, `ls <kind>`, `-n`, `-A`, `-l`, and `-o json` match the
resource statuses; an empty namespace prints "No checks found" and exits 0.

- [ ] T028 [US2] Create `internal/cli/ls.go`: `newLsCommand(f)` with optional kind arg and `-l/--selector`; no kind → list the five kinds in fixed order (ClusterHealth always, regardless of `-n`/`-A`), grouped with a `KIND` column; single kind → that kind only; columns `NAMESPACE` (only with `-A`, blank for ClusterHealth), `NAME`, `VERDICT` (`-` when empty), `SUMMARY` (truncate 80), `LAST RUN`, `NEXT RUN`; empty → `No checks found in namespace <ns>.` / `...in any namespace.` exit 0; `-o json|yaml` → `metav1.List` of unmodified items; register in `internal/cli/root.go`
- [ ] T029 [US2] Create `internal/cli/ls_test.go`: fake client with one object per kind across two namespaces; grouped listing order and columns; each single-kind spelling; `-n`, `-A`, selector; ClusterHealth present under `-n`; empty message and nil error; json output decodes to the same objects
- [ ] T030 [US2] Extend `test/e2e/fathomctl_test.go`: `ls -A` lists every fixture kind with a non-empty verdict and `ls -A -o json | jq` finds the AddonCheck by name
- [ ] T031 [US2] Add the "ls" section (columns, grouping, ClusterHealth rule, empty result) to `docs/reference/fathomctl.md`

**Checkpoint**: `ls` is the working entry point for the other verbs.

---

## Phase 5: User Story 3 - Understand why a check has the verdict it has (Priority: P2)

**Goal**: `fathomctl describe` shows one check in full, including
ClusterHealth children. Delivers #260.

**Independent Test**: `fathomctl describe` on one resource of each kind
(including a ClusterHealth with mixed children) shows every status field
the resource carries; not-found exits 1; `-o yaml` is the unmodified object.

- [ ] T032 [US3] Create `internal/cli/describe.go`: `newDescribeCommand(f)`; sections per contracts/cli-commands.md (identity; Spec: interval, timeout, paused, and kind-specific: AddonCheck addonType/policy families, DNSCheck targets/resolvers, NodeCertificateCheck paths/thresholds/selector, HealthCheck checkRef, ClusterHealth selector/namespaces; Status from `snapshot` plus detected version, observed generation; Conditions table; detail rows: AddonCheck absent count, DNSCheck `targetResults`, NodeCertificateCheck desired/reporting nodes, ClusterHealth children); `Latest report: <name> (see: fathomctl reports <kind>/<name>)`; not-found → error `... not found in namespace ...`; `-o json|yaml` emits the object; register in `internal/cli/root.go`
- [ ] T033 [US3] Create `internal/cli/describe_test.go`: golden-style assertions per kind (each status field present in the output), ClusterHealth with Pass/Warn/Fail children, never-run check, not-found error text, yaml output equals the fixture
- [ ] T034 [US3] Extend `test/e2e/fathomctl_test.go`: `describe addoncheck/<name>` contains `Conditions` and `Latest report`
- [ ] T035 [US3] Add the "describe" section to `docs/reference/fathomctl.md`

**Checkpoint**: Every verdict has a visible reason.

---

## Phase 6: User Story 4 - See how a check's verdict has changed over time (Priority: P2)

**Goal**: `fathomctl reports` lists HealthReport history and opens one
report. Delivers #261.

**Independent Test**: For a check with several reports and one with none,
`reports`, `--limit`, `--since`, and `--report` match the stored reports;
help text states change-only persistence.

- [ ] T036 [US4] Create `internal/cli/reports.go`: `newReportsCommand(f)` with `--limit` (10), `--since`, `--report`; executable kinds → list `HealthReport`s in the check namespace with labels `fathom.skaphos.io/source-kind=<Kind>`, `fathom.skaphos.io/source-name=<name>`, sort by `spec.observedAt` desc, apply `--since` then `--limit`; HealthCheck → follow `spec.checkRef`, print `Showing reports for <source>`; ClusterHealth → error listing the sources; columns `NAME`, `OBSERVED`, `RESULT`, `SUMMARY`, `CHANGE` (`first`, `unchanged`, `<old>→<new>`, `<n> check(s) changed` versus the next-older report); `--report` → `Get` and full render (spec, per-check rows); no history → `No reports yet for <kind>/<name>.` exit 0; `Long` help text states "Reports are written when a verdict changes, not on every interval; a gap is not a missed run."; `-o json|yaml` → `metav1.List`; register in `internal/cli/root.go`
- [ ] T037 [US4] Create `internal/cli/reports_test.go`: fake client with five labelled reports out of order plus one for another check; ordering, `--limit`, `--since`, `--report`, change column for each case, HealthCheck redirect, ClusterHealth rejection, no-reports message, help text contains the change-only sentence
- [ ] T038 [US4] Extend `test/e2e/fathomctl_test.go`: `reports addoncheck/<name>` lists at least one report and `--report <name>` prints its result
- [ ] T039 [US4] Add the "reports" section to `docs/reference/fathomctl.md`

**Checkpoint**: The evidence trail is navigable from the check.

---

## Phase 7: User Story 5 - Install the CLI, trust it, and know what I am talking to (Priority: P3)

**Goal**: `fathomctl version` and signed, attested per-platform release
archives. Delivers #258 part 2 (version and release wiring).

**Independent Test**: Following the published steps, download, verify
checksum, signature, and provenance, run the binary; `version` shows both
versions against a cluster, degrades offline with exit 0, and `--client`
makes no cluster contact.

- [ ] T040 [US5] Create `internal/cli/version.go`: `var Version string` (ldflag); `clientVersion()` = `Version`, else `debug.ReadBuildInfo` module version plus short `vcs.revision`, else `devel`; `newVersionCommand(f)` with `--client`; `lookupOperator` lists Deployments with `control-plane=controller-manager` (in `-n` if set, else all namespaces), filters `app.kubernetes.io/name` prefix `fathom` or image containing `fathom-operator`, version from `app.kubernetes.io/version` label else `manager` container image tag else digest; any error → `operator.error` and table line `Operator: unavailable (<reason>)`, exit 0; json/yaml per contract; register in `internal/cli/root.go`
- [ ] T041 [US5] Create `internal/cli/version_test.go`: `Version` set and unset; `--client` never calls the client seam; Helm-labelled and kustomize-labelled deployments; digest-only image; other operator with the label but no fathom marker → not found; `-n` scoping; loader error → unavailable with nil command error; json shape
- [ ] T042 [P] [US5] Create `scripts/fathomctl-dist.sh` (SPDX header; `VERSION` required, `OUT` default `dist/fathomctl`, `FATHOMCTL_PLATFORMS` default the six targets; `CGO_ENABLED=0 GOOS/GOARCH go build -trimpath -ldflags "-s -w -buildid= -X github.com/skaphos/fathom/internal/cli.Version=v${VERSION}"`; stage `fathomctl[.exe]` + `LICENSE` under `fathomctl_${VERSION}_${os}_${arch}/`; `tar -czf` or `zip -r` for windows; `fathomctl_${VERSION}_checksums.txt` via `sha256sum` or `shasum -a 256`) and `scripts/fathomctl_dist_gate_test.go` (skip in `-short`; host platform only; checksums verify; extracted binary prints `v0.0.0-test`; missing `VERSION` fails)
- [ ] T043 [P] [US5] Add task `fathomctl-dist` to `Taskfile.yml` (`VERSION={{.VERSION}} OUT=dist/fathomctl scripts/fathomctl-dist.sh`)
- [ ] T044 [US5] Update `.github/workflows/release.yml`: after the Helm chart step, "Build fathomctl release archives" (`go -C tools tool task fathomctl-dist VERSION="${VERSION}"`); after image signing, "Sign fathomctl checksums (keyless)" (`cosign sign-blob --yes --bundle "${sums}.sigstore.json" "${sums}"`); "Attest fathomctl archive provenance" (`actions/attest-build-provenance` with `subject-path: dist/fathomctl/*.tar.gz` and `dist/fathomctl/*.zip`, same pinned SHA as the image steps); add `dist/fathomctl/*` to the release `files`
- [ ] T045 [US5] Extend `test/e2e/fathomctl_test.go`: `version` prints `Client:` and an `Operator:` line with the Kind operator's version and namespace; `version --client -o json` has no `operator` key
- [ ] T046 [US5] Update `RELEASE.md`: add the fathomctl archive/checksum/signature/provenance steps to section 4 and a "Verify a fathomctl download" subsection under Supply-Chain Verification (commands from quickstart.md §5); state that no CLI image is published

**Checkpoint**: A release produces trustworthy binaries and `version` explains the pairing.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T047 [P] Create `docs/guides/fathomctl.md` (install from a release, verify, first `ls`, reading `describe`, triggering with `run --wait` in CI, history with `reports`) and add it to `docs/guides/README.md` and `docs/README.md`
- [ ] T048 [P] Update `README.md`: a "fathomctl" section (what it is, install snippet, `run --wait` example, link to the guide); update `docs/code-map.md` (`cmd/` row and bullet for `cmd/fathomctl/main.go`, new `internal/cli/` section); update `AGENTS.md` (project structure entries for `cmd/fathomctl/` and `internal/cli/`, build commands `fathomctl-build`/`fathomctl-dist`, note that `internal/cli` tests are in-package stdlib)
- [ ] T049 [P] Update `docs/reference/operator-rbac.md` "Auxiliary roles shipped alongside the operator" with `fathomctl-viewer-role` and `fathomctl-runner-role` and the justification that the operator's own role is unchanged
- [ ] T050 Complete `docs/reference/fathomctl.md` with the "version" section and "Verifying downloads" cross-link to `RELEASE.md`; proofread the whole page against contracts/cli-commands.md
- [ ] T051 File follow-up issues and link them from the PR: Helm parity for the fathomctl ClusterRoles; a restart-free trigger channel for the node-agent; note #322 (MCP) as downstream
- [ ] T052 Run the full quickstart gate set: `go -C tools tool task fmt manifests generate docs:api-ref crd-compat lint vet test staticcheck vuln build`, `scripts/check-coverage.sh coverage.out` (every touched package ≥ 50%), `reuse --no-multiprocessing lint`, `go -C tools tool task test-e2e` (full stack; record any missing local prerequisite in the PR test plan), then `graphify update .` and commit the `graphify-out/` update
- [ ] T053 Open the PR(s) per the implementation strategy below (ready, not draft), with summary, motivation, exact checks run with outcomes, and doc links; address Copilot and adversarial review comments and resolve threads

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies.
- **Foundational (Phase 2)**: T010 is independent of Phase 1; T011/T012 need T005 (descriptor table). Blocks every story.
- **US1 (Phase 3)**: engine tasks T013–T020 need only T010; CLI tasks T021–T027 need Phase 2 and T013–T019 for e2e (T026).
- **US2 (Phase 4)**: needs Phase 2 only. Independent of US1.
- **US3 (Phase 5)**: needs Phase 2 only.
- **US4 (Phase 6)**: needs Phase 2 only (uses labels the controllers already write).
- **US5 (Phase 7)**: T040/T041 need Phase 1 only; T042–T044 need T007.
- **Polish (Phase 8)**: after the stories you intend to ship.

### User Story Dependencies

- US1 is the only story that changes the operator; it can be split into an
  engine PR (T013–T020) and a CLI PR (T021–T027).
- US2, US3, US4, US5 touch only `internal/cli`, `scripts/`, workflows, and
  docs; they share `internal/cli/root.go` (verb registration) and
  `test/e2e/fathomctl_test.go`, so their registration/e2e tasks are
  sequential while their verb files are parallel.
- `describe`'s "Latest report" hint and `run --wait`'s output reuse
  `snapshot` (Phase 2), not each other.

### Parallel Opportunities

- Phase 1: T003, T004, T005, T007, T008 in parallel after T002.
- Phase 3: T017 alongside T013–T016; T021 alongside T018–T019.
- Across stories: T028 (ls), T032 (describe), T036 (reports), T040 (version),
  T042/T043 (dist) are independent files once Phase 2 is done.
- Phase 8: T047, T048, T049 in parallel.

---

## Parallel Example: after Phase 2

```bash
# Independent verb files (different files, no shared state):
Task: "Create internal/cli/ls.go ..."           # T028
Task: "Create internal/cli/describe.go ..."     # T032
Task: "Create internal/cli/reports.go ..."      # T036
Task: "Create internal/cli/version.go ..."      # T040
Task: "Create scripts/fathomctl-dist.sh ..."    # T042

# Then serialise the shared files:
#   internal/cli/root.go registrations, test/e2e/fathomctl_test.go additions
```

---

## Implementation Strategy

### MVP First (US1 only)

1. Phase 1 (scaffold) and Phase 2 (normalisation, constants).
2. Phase 3 engine half (T013–T020) as PR "feat(engine): extend the run-now
   trigger to every check kind (#264)" — requires the full e2e stack.
3. Phase 3 CLI half (T021–T027) as PR "feat(cli): fathomctl run --wait
   (#263)".
4. Validate US1 independently with quickstart.md §4 before demoing.

### Incremental Delivery (recommended PR sequence)

| PR | Tasks | Closes |
|---|---|---|
| 1 | T001–T012 | #258 (scaffold half; `version` lands in PR 6) |
| 2 | T013–T020 | #264 |
| 3 | T021–T027 | #263 |
| 4 | T028–T031 | #259 |
| 5 | T032–T035, T036–T039 | #260, #261 |
| 6 | T040–T046 | #258 (version + release wiring) |
| 7 | T047–T053 | docs and gates; epic #204 |

PRs 2 and 4–6 can proceed in parallel once PR 1 has merged; PR 3 needs PR 2.

### Parallel Team Strategy

- Engineer A: PR 2 then PR 3 (operator + `run`).
- Engineer B: PR 4 and PR 5 (read verbs).
- Engineer C: PR 6 (version, dist, release workflow) then PR 7.

---

## Notes

- Never hand-edit `zz_generated.deepcopy.go`, `config/crd/bases/`,
  `deploy/helm/fathom-operator/crds/`, or `docs/reference/api.md`; run the
  tasks in T016.
- `spec.paused` is removed in v0.7.0 (#262); the paused exclusion in T022
  and the `Paused` descriptor hook are deleted with it.
- #275 also edits `nodecertificatecheck_controller.go`; whichever lands
  second rebases.
- Keep `internal/cli` free of imports from `internal/app`,
  `internal/adapter`, and `internal/controller`.
