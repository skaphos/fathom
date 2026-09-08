<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Feature Specification: fathomctl CLI

**Feature Branch**: `feature/204-fathomctl-cli`

**Created**: 2026-09-07

**Status**: Draft

**Input**: User description: "fathomctl: a command-line client for the Fathom
operator (GitHub epic #204, milestone v0.6.0). Operators and platform
engineers need a kubectl-style CLI to see and drive Fathom checks without
hand-reading CRD status. MVP verb set: `ls`, `describe`, `reports`,
`run [--wait]`, `version`. Global flags: `--kubeconfig`, `--context`,
`-n/--namespace`, `-A/--all-namespaces`, `-o/--output (table|json|yaml)`.
Output must be scriptable and human-readable. Read-only verbs need only
get/list on Fathom kinds; `run` needs patch on the target kind. Active
on-demand validation is a first-class delivery surface. `pause`/`resume` are
out of scope (decision #262). The run-now trigger exists only on AddonCheck
today and must be extended to every check kind (#264). fathomctl ships as
per-platform release archives with checksums, signatures and provenance; it
does not ship as a container image."

## Overview

Fathom publishes every verdict it produces as Kubernetes resource status:
`AddonCheck`, `DNSCheck`, and `NodeCertificateCheck` each carry a result,
`HealthCheck` mirrors one of them, `ClusterHealth` rolls the mirrors up, and
`HealthReport` records each change. That is the right durable contract, but
it is a poor day-to-day interface. Reading a verdict today means knowing which
kind to `kubectl get`, which status field carries the result for that kind,
and how to page through reports by hand. Forcing a check to re-evaluate right
now means knowing the trigger annotation and hand-writing a fresh value, and
it only works on some of the executable kinds.

`fathomctl` closes that gap with a small, kubectl-shaped client. Its read
verbs give one consistent view of verdicts across every kind. Its `run` verb
makes on-demand validation a first-class operation: an operator can ask Fathom
to go and look now, wait for the answer, and use the result in a script or a
runbook. Nothing the CLI shows or does bypasses the operator; it reads the
same status the operator writes and drives the same trigger the operator
already honours.

## Decisions and Tradeoffs

| Decision | Choice | Rationale |
|---|---|---|
| Verb set | Exactly `ls`, `describe`, `reports`, `run`, `version` | Each verb maps to one operator question ("what is the state", "why", "what changed", "check again now", "what am I talking to"). `pause`/`resume` are excluded by decision #262: Fathom emits signal and does not own suppression, so stopping a check means deleting it. |
| Kinds covered by read verbs | `AddonCheck`, `DNSCheck`, `NodeCertificateCheck`, `HealthCheck`, `ClusterHealth`; `HealthReport` only through `reports` | These are the kinds that exist in the current API. Issue #259 predates `DNSCheck` and lists four; the spec follows the API, not the stale issue. Kinds that do not exist yet (`NodeHealthCheck`) are not advertised. |
| Structured output is the raw object | `-o json` / `-o yaml` on `ls`, `describe`, and `reports` emit the underlying resources unmodified | A CLI-specific schema would be a second contract to version. The resource schema is already the public contract and already generated into the API reference; `jq` and `yq` compose with it directly. |
| `run` honours the existing trigger contract | `run` writes a fresh, unique value to the existing on-demand trigger and `--wait` matches on that value being consumed | The operator already implements consume-once semantics for `AddonCheck` and `DNSCheck`. Introducing a second trigger path would violate Principle I (state lives in the resource, not in a side channel). Extending the same contract to every executable kind (#264) is part of this feature. |
| Exit codes are the scripting contract | kubectl style: `0` on success, `1` on anything else; `run --wait` exits zero only for a non-failing verdict | This is what makes `run` usable as a gate in CI and runbooks, and it matches what every kubectl user already expects. A richer code table (distinct timeout and usage codes) was considered and rejected: it is not the Kubernetes convention, and the message carries the reason. Fixed here so scripts written against the MVP do not break later. |
| Distribution | Per-platform archives with checksums, keyless signatures and build provenance attached to the same release as the operator images; no container image | A client tool runs on an engineer's workstation or in CI, where an archive is the natural unit. The supply-chain posture matches the operator images so a verifier follows one procedure. The CLI version is the operator release version so skew is visible. |
| Derived kinds under `run` | Propagate to the sources: a `HealthCheck` triggers its referenced check; a `ClusterHealth` fans out to the source of every selected `HealthCheck` | `HealthCheck` and `ClusterHealth` have no work of their own to redo, so the only way `run` can mean "check again" on them is to re-run what they observe. This gives the operator "re-check everything now" in one command, at the cost of needing write access on every executable kind and of launching many runs at once; the fan-out is bounded by the existing child cap. Rejecting the verb on derived kinds was smaller, and re-mirroring without re-running was cheaper, but neither produces a fresh observation. |

Doing nothing leaves on-demand validation reachable only by users who know
the annotation, on some kinds, and leaves every verdict reachable only by
users who know per-kind status layouts. Both are the kind of tribal knowledge
the constitution forbids.

## Clarifications

### Session 2026-09-07

- Q: What should the value written by `fathomctl run` as the on-demand trigger look like? → A: A UTC timestamp plus a short random suffix (e.g. `2026-09-07T18:04:05Z-7f3a1c`); unique, sortable, human-readable in status, and carries no caller identity.
- Q: Which exact exit codes should `fathomctl` use? → A: kubectl style: `0` on success (a non-failing verdict for `run --wait`), `1` for everything else (failing verdict, wait timeout, usage, permission, or cluster error). The reason is distinguished by the message, not the code.
- Q: Should `run` confirm or cap when it would trigger many checks at once? → A: Interactive confirmation when more than 10 checks would be triggered, bypassed with `--yes`; `--dry-run` lists the set without writing anything.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ask Fathom to validate now and get the answer (Priority: P1)

As a platform engineer rolling out a change to an add-on, I can tell Fathom
to re-check that add-on immediately, wait for the verdict, and use the exit
code to decide whether the rollout proceeds.

**Why this priority**: On-demand validation is the reason the CLI exists as
a delivery surface. Without it, Fathom is only a continuous monitor that
alarms after the fact.

**Independent Test**: Against a cluster with one check of each executable
kind, run `fathomctl run` with and without `--wait`, confirm a fresh run
occurs each time, and confirm the verdict and exit code match what the
resource status shows afterward.

**Acceptance Scenarios**:

1. **Given** an executable check that last ran some time ago, **When** I run `fathomctl run <kind>/<name>`, **Then** the command returns promptly with an acknowledgement, and the check's next reconcile performs a fresh run rather than waiting for its interval.
2. **Given** the same check, **When** I run `fathomctl run <kind>/<name> --wait`, **Then** the command blocks until *that* trigger has been consumed and the run has completed, prints the resulting verdict and summary, and exits zero for a passing verdict.
3. **Given** a check whose fresh run fails, **When** I run it with `--wait`, **Then** the verdict is printed and the exit code is `1`.
4. **Given** a check that never consumes the trigger within the wait timeout, **When** I run it with `--wait --timeout <d>`, **Then** the command exits `1` with a message that says it timed out and names the check, the elapsed time, and the most likely causes.
5. **Given** the same trigger value already consumed, **When** the check next reconciles on its interval, **Then** it does not run again on account of that token.
6. **Given** a label selector or `--all`, **When** I run `fathomctl run` with it, **Then** every matching check is triggered and the outcome is reported per check.
7. **Given** a `ClusterHealth` that selects several `HealthCheck`s, **When** I run `fathomctl run clusterhealth/<name> --wait`, **Then** every source check behind those `HealthCheck`s is triggered with the same value, each outcome is reported, and the exit code is the worst of them.
8. **Given** a selector that matches more than 10 checks, **When** I run `fathomctl run -l <selector>` in a terminal, **Then** I am shown the count and asked to confirm before anything is written; **When** I run the same command with `--yes` or from a script with `--dry-run`, **Then** it proceeds without asking or lists the set without writing, respectively.

---

### User Story 2 - See every verdict in the cluster at a glance (Priority: P1)

As an operator opening a cluster for the first time that day, I can list
every Fathom check and read its current verdict, one-line summary, and how
long ago it last ran, without knowing which kind carries which status field.

**Why this priority**: This is the entry point for every other verb, and it
is the read path that Principle VII requires to keep working when nothing
else does.

**Independent Test**: Create at least one resource of each kind with mixed
verdicts, run `fathomctl ls` with no arguments, with a kind argument, with
`-n`, with `-A`, and with a selector, and compare the output to the resource
statuses.

**Acceptance Scenarios**:

1. **Given** checks of several kinds in the current namespace, **When** I run `fathomctl ls`, **Then** I see them grouped by kind with name, namespace where applicable, verdict, summary, and last-run age in aligned columns.
2. **Given** checks across namespaces, **When** I run `fathomctl ls -A`, **Then** every namespace's checks are listed, and `ClusterHealth` appears once regardless of namespace scope.
3. **Given** a kind argument such as `fathomctl ls dnschecks`, **When** it runs, **Then** only that kind is listed and the kind name is accepted in the same singular, plural, and short forms as kubectl uses.
4. **Given** no matching checks, **When** I run `fathomctl ls`, **Then** I see a clear "no checks found" message and a zero exit code, not an empty table.
5. **Given** `-o json`, **When** I run `fathomctl ls`, **Then** the output is a machine-parseable list of the underlying resources that `jq` can filter without a CLI-specific schema.

---

### User Story 3 - Understand why a check has the verdict it has (Priority: P2)

As an operator responding to a failing verdict, I can see one check in full:
what it was configured to do, what it observed, which conditions are set,
each per-target or per-node result, and where the most recent evidence
record is.

**Why this priority**: `ls` tells the operator that something is wrong;
`describe` tells them why. Principle VI requires that the reason is always
available.

**Independent Test**: For one resource of each kind, including a
`ClusterHealth` with children of mixed verdicts, run `fathomctl describe` and
confirm every status field that the resource carries is represented.

**Acceptance Scenarios**:

1. **Given** an executable check, **When** I run `fathomctl describe <kind>/<name>`, **Then** I see its schedule and policy summary, its verdict, summary, conditions, last run time and trigger, any detected version, and every per-target or per-node detail row.
2. **Given** a `ClusterHealth`, **When** I describe it, **Then** I see which `HealthCheck`s contributed and what each contributed.
3. **Given** a check with a most recent report, **When** I describe it, **Then** the output names that report and points me at `fathomctl reports`.
4. **Given** a name that does not exist, **When** I describe it, **Then** I get a clear not-found error and a non-zero exit.
5. **Given** `-o yaml`, **When** I describe a check, **Then** the output is the unmodified resource.

---

### User Story 4 - See how a check's verdict has changed over time (Priority: P2)

As an operator investigating a flapping or recently-degraded check, I can
list its recorded history newest-first, limit or window it, and open one
record in full.

**Why this priority**: History is the evidence trail. It already exists as
resources; the CLI makes it navigable from the check rather than by hand.

**Independent Test**: For a check with several reports and one with none,
run `fathomctl reports` with default, `--limit`, `--since`, and `--report`
and compare to the stored reports.

**Acceptance Scenarios**:

1. **Given** a check with report history, **When** I run `fathomctl reports <kind>/<name>`, **Then** I see up to the default number of reports newest-first with timestamp, verdict, summary, and what changed from the prior report.
2. **Given** `--limit` or `--since`, **When** I run `reports`, **Then** the list is bounded accordingly.
3. **Given** `--report <name>`, **When** I run `reports`, **Then** that single report is shown in full.
4. **Given** a check with no reports yet, **When** I run `reports`, **Then** I see an explicit "no reports yet" message, not an empty table.
5. **Given** the help text, **When** I read it, **Then** it tells me that reports record verdict changes, not every interval, so gaps are not missed runs.

---

### User Story 5 - Install the CLI, trust it, and know what I am talking to (Priority: P3)

As an engineer picking up the CLI for the first time, I can download a
release archive for my platform, verify it came from the Fathom release
pipeline, and confirm which CLI and operator versions I am working with.

**Why this priority**: Everything else depends on the binary being
obtainable and trustworthy, but on its own it delivers only the smallest
slice of value.

**Independent Test**: Follow the published install and verification steps on
each supported platform, then run `fathomctl version` with and without a
reachable cluster.

**Acceptance Scenarios**:

1. **Given** a published release, **When** I follow the documented steps, **Then** I can download the archive for my platform, verify its checksum, verify the checksum file's signature and provenance against the Fathom release identity, and run the binary.
2. **Given** a reachable cluster with Fathom installed, **When** I run `fathomctl version`, **Then** I see the CLI version and the operator version, and which namespace and deployment the operator version came from.
3. **Given** no reachable cluster, **When** I run `fathomctl version`, **Then** I still see the CLI version, the operator is reported as unavailable with the reason, and the exit code is zero.
4. **Given** `--client`, **When** I run `fathomctl version`, **Then** no cluster contact is attempted.

### Edge Cases

- `-n` and `-A` given together is a usage error, not a silent precedence.
- With neither `-n` nor `-A`, the namespace is the kubeconfig context's
  namespace, falling back to `default`, exactly as kubectl behaves.
- `ClusterHealth` is cluster-scoped: it is never namespace-qualified, it is
  never filtered out by `-n`, and `fathomctl ls -n foo clusterhealth` still
  lists it.
- A check that exists but has never run has no verdict; `ls` and `describe`
  show that explicitly rather than inventing one.
- A check whose status is truncated by the API's own bounds (many targets,
  many nodes) is rendered readably; the CLI does not add unbounded detail.
- `run` on a paused check fails fast with an explanation: the operator does
  not run paused checks, so the trigger would never be consumed and `--wait`
  would only time out.
- `run` against an operator that predates trigger support for the given kind
  cannot be detected up front; the `--wait` timeout message names operator
  version compatibility as a likely cause and suggests `fathomctl version`.
- `run` without `--wait` on a check that is mid-run: the token is recorded
  and will be consumed on the next reconcile; the CLI does not wait for the
  in-flight run.
- `run` on a `HealthCheck` whose target is missing, or on a `ClusterHealth`
  that selects nothing, triggers nothing and says so with a non-zero exit.
- `run` on a `ClusterHealth` where two selected `HealthCheck`s share one
  source triggers that source once, not twice, and counts it once toward the
  confirmation threshold.
- `run` that would trigger more than 10 checks from a non-interactive
  context (CI, a pipe) without `--yes` aborts before writing anything and
  says that `--yes` is required.
- Two `run` invocations in quick succession produce two distinct tokens; the
  second supersedes the first before it is consumed, and `--wait` on the
  first reports that its token was superseded rather than waiting forever.
- Lack of permission on a verb produces the API server's forbidden error
  with the verb and resource named, never a misleading "not found".
- Windows archives use the platform's archive convention and the binary name
  carries the platform's executable suffix.
- The CLI never prints kubeconfig contents, tokens, or credentials in any
  output or error.

## Requirements *(mandatory)*

### Functional Requirements

**Command surface**

- **FR-001**: The CLI MUST provide exactly the verbs `ls`, `describe`, `reports`, `run`, and `version`, plus standard help and shell-completion support. It MUST NOT provide `pause` or `resume`.
- **FR-002**: Every verb MUST accept the global flags `--kubeconfig`, `--context`, `-n/--namespace`, `-A/--all-namespaces`, and `-o/--output` with the values `table` (default), `json`, and `yaml`; an unsupported output value MUST be a usage error.
- **FR-003**: Cluster access MUST follow the standard Kubernetes client discovery rules (explicit flag, then the `KUBECONFIG` environment variable, then the default path; explicit context override) so the CLI works wherever kubectl works, including in-cluster.
- **FR-004**: `-n` and `-A` MUST be mutually exclusive. With neither, the namespace MUST resolve from the kubeconfig context.
- **FR-005**: Check kinds MUST be addressable as `<kind>/<name>` and as `<kind> <name>`, and kind names MUST be accepted in singular, plural, and short forms consistent with the resource definitions.
- **FR-006**: Every verb MUST bound its API requests with a timeout and MUST NOT block indefinitely; `run --wait` MUST honour an explicit `--timeout` with a documented default.
- **FR-007**: Errors MUST state what failed and the next action. Exit codes follow the kubectl convention: `0` on success and `1` on any error; the kind of failure (usage, permission, cluster, timeout, failing verdict) MUST be distinguishable from the message, not the code.

**Read verbs**

- **FR-008**: `ls` MUST list `AddonCheck`, `DNSCheck`, `NodeCertificateCheck`, `HealthCheck`, and `ClusterHealth`; with no kind argument it MUST list all of them grouped by kind; with a kind argument it MUST list only that kind.
- **FR-009**: `ls` table output MUST show name, namespace where the kind is namespaced, verdict, a bounded summary, and last-run age, and MUST show next-run where the kind has an interval.
- **FR-010**: `ls` MUST honour `-n`, `-A`, and a label selector flag, and MUST include `ClusterHealth` in the default listing regardless of namespace scope.
- **FR-011**: `ls` MUST print an explicit "no checks found" message and exit zero when nothing matches.
- **FR-012**: The verdict, summary, and last-run values shown for a given check MUST be identical across `ls`, `describe`, and `run --wait`; there is one normalisation per kind, not one per verb.
- **FR-013**: `describe` MUST show, for one check: the schedule and policy summary and target reference from its spec; verdict, summary, conditions, last run time, last consumed trigger, and detected version from its status; every per-target or per-node detail row; and the name of the most recent report with a pointer to `reports`.
- **FR-014**: `describe` on a `ClusterHealth` MUST show each contributing `HealthCheck` and its contribution.
- **FR-015**: `reports` MUST list a check's report history newest-first with timestamp, verdict, summary, and what changed from the prior report; MUST support `--limit` (default 10) and `--since`; and MUST show one report in full with `--report <name>`.
- **FR-016**: `reports` help text MUST explain that reports record verdict changes, not every interval.
- **FR-017**: `reports` MUST print an explicit "no reports yet" message when a check has no history.
- **FR-018**: On `ls`, `describe`, and `reports`, `-o json` and `-o yaml` MUST emit the underlying resources unmodified.

**On-demand validation**

- **FR-019**: `run <kind>/<name>` MUST request an immediate re-evaluation through the existing on-demand trigger contract by writing a fresh, unique value, and MUST return as soon as the request is accepted, printing the value it wrote. The value MUST be the UTC time of the request in RFC 3339 form followed by a short random suffix (for example `2026-09-07T18:04:05Z-7f3a1c`), so that it is unique across concurrent invocations, sorts chronologically, reads as "when was this triggered" in status output, and carries no caller identity.
- **FR-020**: `run --wait` MUST wait until the operator has consumed *that* value and the resulting run has completed, then print the verdict and summary. It MUST match on the value being consumed, not merely on the last-run time advancing.
- **FR-021**: `run --wait` MUST exit `0` when the resulting verdict is `Pass`, `Warn`, or `Skipped`, and `1` when it is `Fail`, `Error`, or `Unknown`, when the wait times out, or when the command fails for any other reason. `run` without `--wait` MUST exit `0` once every requested trigger is accepted and `1` otherwise.
- **FR-022**: `run` MUST accept `--all` or a label selector to trigger a set of checks, MUST refuse to run with no target and no selector, and MUST report the outcome per check.
- **FR-022a**: Before writing any trigger, `run` MUST resolve the full set of executable checks it would touch (including sources reached through a derived kind) and print the count. When that set exceeds 10 checks, `run` MUST ask for interactive confirmation and MUST abort with exit `1` if the answer is not affirmative or if no interactive terminal is available; `--yes` skips the confirmation. `--dry-run` MUST print the resolved set and exit `0` without writing anything.
- **FR-023**: `run` on a paused check MUST fail fast with an explanation and MUST NOT write a trigger.
- **FR-024**: Every executable check kind (`AddonCheck`, `DNSCheck`, `NodeCertificateCheck`) MUST honour the on-demand trigger with the same semantics: a new value forces a run; the consumed value is recorded; a periodic run never re-fires a consumed value; a periodic run never clears a consumed value. For `NodeCertificateCheck` the forced run MUST cause a fresh node scan, not a re-read of stale reports.
- **FR-025**: `run` on a derived kind MUST propagate to its sources: `run` on a `HealthCheck` MUST trigger the executable check it references, and `run` on a `ClusterHealth` MUST trigger the source of every `HealthCheck` it currently selects. The derived kind itself is never triggered; the same fresh value is written to every source so `--wait` can match on each. The command MUST list the sources it triggered, MUST report a source it could not trigger (missing, paused, or forbidden) without abandoning the others, and with `--wait` MUST report per-source outcomes with the overall exit code being the worst of them. Propagation MUST be bounded by the existing `ClusterHealth` child cap, and this behaviour MUST be documented in the reference documentation.
- **FR-026**: The read verbs MUST require only read access (get, list, watch) to the Fathom kinds, and `run` MUST additionally require only the ability to update the metadata of the executable kinds it ends up triggering (for a derived kind, its sources). The feature MUST NOT require any new operator permission, and the CLI's own permission needs MUST be documented.

**Version and distribution**

- **FR-027**: `version` MUST always print the CLI version, MUST print the operator's version, namespace, and deployment when a cluster is reachable and Fathom is installed, MUST report the operator as unavailable with a reason (not fail) otherwise, and MUST skip cluster contact with `--client`.
- **FR-028**: Each release MUST publish CLI archives for Linux, macOS, and Windows on 64-bit x86 and 64-bit ARM, a checksum file covering them, a keyless signature over the checksum file, and build provenance for the archives, all attached to the same release as the operator images.
- **FR-029**: The CLI MUST NOT ship as a container image.
- **FR-030**: The CLI's reported version MUST be the operator release version it was built from; a locally built binary MUST identify itself as a development build.
- **FR-031**: The release documentation MUST describe how to verify a CLI download, and the user documentation MUST cover installation, every verb, the global flags, and the exit-code contract.

### Key Entities

- **Check**: any of `AddonCheck`, `DNSCheck`, `NodeCertificateCheck`, `HealthCheck`, or `ClusterHealth`, identified by kind, namespace (where namespaced), and name.
- **Executable check**: a check that performs its own evaluation (`AddonCheck`, `DNSCheck`, `NodeCertificateCheck`) and therefore can be triggered.
- **Derived check**: a check whose status is computed from other checks (`HealthCheck`, `ClusterHealth`).
- **Verdict**: the normalised outcome of a check (`Pass`, `Warn`, `Fail`, `Error`, `Skipped`, `Unknown`) plus its summary and last-run time, extracted identically by every verb.
- **Run trigger**: the existing on-demand trigger contract: a value written on the check that, when it differs from the last consumed value, forces a run; the operator records the consumed value. The CLI writes a UTC timestamp plus a short random suffix.
- **HealthReport**: the immutable change-history record for a check.
- **Release archive**: a per-platform bundle of the CLI binary and licence, covered by a checksum file, a signature, and provenance.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can list every check in a cluster and read each verdict with one command, and for a cluster of 100 checks the listing completes in under 5 seconds.
- **SC-002**: `run --wait` returns the fresh verdict within the check's own configured timeout plus 30 seconds of polling overhead for every executable kind, verified on a real cluster.
- **SC-003**: A CI job can gate on a check using only `fathomctl run --wait` and its zero or non-zero exit code, with no parsing of output, and the exit-code contract is covered by automated tests.
- **SC-004**: 100% of verbs offer `json` and `yaml` output that a standard JSON or YAML parser accepts.
- **SC-005**: A first-time user can install, verify, and run `fathomctl version` on any of the six supported platform targets in under 5 minutes following only the published documentation.
- **SC-006**: Every verb is covered by automated tests for every kind it supports without a live cluster, and a real-cluster test proves a triggered run for every executable kind.
- **SC-007**: The change adds no permission to the operator's own role, and the CLI's required permissions are documented in the reference pages.
- **SC-008**: The same trigger value never causes more than one run in any real-cluster or automated test.

## Assumptions and Dependencies

- The kinds in scope are those present in the current API
  (`fathom.skaphos.io/v1alpha1`). `NodeHealthCheck` (#206) is not part of
  this feature; when it lands it is added under the same rules.
- The on-demand trigger contract already implemented for `AddonCheck` and
  `DNSCheck` (a metadata value with consume-once semantics recorded in
  status) is the contract that is generalised; no second trigger mechanism
  is introduced.
- `spec.paused` still exists on some kinds in v0.6.0 and is removed in
  v0.7.0 (#262); the paused edge case in FR-023 is retired with it.
- Report history persists on verdict change only, per the periodic-execution
  work; the CLI reads and never writes reports.
- Verdict severity ordering is the existing
  `Pass < Skipped < Warn < Unknown < Fail < Error`.
- The CLI's default `--wait` timeout is the check's configured timeout plus
  30 seconds (the margin SC-002 measures against); when several checks are
  waited on, the default is the largest of their individual defaults. An
  explicit `--timeout` overrides it.
- Where `run` targets several checks, the per-check outcomes are reported and
  the overall exit code is the worst of them.
- The CLI supports operators within one minor version; behaviour against an
  older operator that lacks trigger support for a kind is a `--wait` timeout
  with a message pointing at version compatibility, not a pre-flight check.
- Distribution reuses the release pipeline that already signs and attests the
  operator, probe, and node-agent images; the CLI artifacts are added to it,
  not published separately.
- The operator's version is discoverable from its deployment in the cluster
  by the labels the supported install methods already apply.

## Out of Scope

- `pause` and `resume` verbs, or any alert-suppression surface (#262).
- Creating, editing, or deleting checks; the CLI is read-only apart from the
  run trigger. Authoring stays in Git per Principle II.
- A CLI-specific output schema, plugin system, or interactive UI.
- A container image for the CLI.
- Support for kinds that do not exist in the current API.
- Changing report retention, deduplication, or the `ClusterHealth` contract.

## References

- GitHub epic #204 and child issues #258, #259, #260, #261, #263, #264
- Decision #262: Fathom does not own alert suppression
- `specs/008-dnscheck-completion/` (target-kind boundary and projection semantics)
- `docs/reference/status-conditions.md` (verdict and condition meanings)
- Constitution principles I, VI, VII
