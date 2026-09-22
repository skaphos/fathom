<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Opt-in preview qualification guide

Runtime commands and packages are implemented behind the opt-in preview gate. Use
this guide for the opt-in preview; current commands, environments and outcomes
are recorded in [execution.md](execution.md). The pre-compatibility and combined
ContractVersion 1.1 full suites passed; upstream #351 and feature PR gates remain
open.

## Repository checks

From repository root:

```sh
.specify/scripts/bash/check-prerequisites.sh --json --require-tasks --include-tasks
git diff --check
reuse lint
```

Inspect [contracts/runtime.md](contracts/runtime.md) for exact caps and event cases.

## Component and repository checks

Install the Go version in go.mod, a running Docker-compatible daemon, kind, helm
and helmfile. Keep envtest, kind node and docs Kubernetes versions aligned with
module versions as AGENTS.md requires. Use only pinned task wrappers:

```sh
go -C tools tool task generate manifests gen:addon-rbac helm:sync docs:api-ref
go -C tools tool task verify-generated crd-compat
go -C tools tool task ci
go -C tools tool task test
go test -race ./internal/adapter/registry/... ./internal/adapter/runtime/...
go -C tools tool task test-e2e
```

Run `go -C tools tool task test` for the task-provisioned pinned envtest assets and
controller/app envtest coverage, then run the controller/app race targets with the
same `KUBEBUILDER_ASSETS` value; do not invoke `setup-envtest` directly. Record
exact commands and outcomes in [execution.md](execution.md); missing tools are
explicit blockers, not successful skips. Full shared-surface e2e is mandatory, not
one addon shard.

## Real-cluster scenarios

1. Install generated CRDs/operator with runtime option default disabled and election enabled. Confirm
   compiled checks unchanged and external definitions inert.
2. Use the `fathomctl definition` commands to render a non-built-in sample,
   dedicated SA and grants. Review/apply definition and SA; resolve actual UIDs;
   review proposed binding/grants into Git, apply grants then enable binding and
   opt in to runtime loading. Assert attributed completed evidence.
3. Deny discovery/read permissions, select out-of-scope targets, use manager/builtin/
   shared SAs, recreate identities, omit factory/namespace and disable metrics.
   Assert no privileged fallback; test same-UID in-scope granted retargeting succeeds.
   Missing factory/local-namespace fail-closed behavior is proven by component and
   app-wiring tests. The separate host-local trial with a missing projected identity
   passed its AuthorizationUnavailable and built-in progress checks; see
   [operations-qualification.md](operations-qualification.md).
4. Exercise lifecycle rows with ordinary API mutations: edit, delete and recreate
   definitions, bindings and ServiceAccounts; disable/re-enable authority; restart
   the operator; and use test-side held external API responses where a long read is
   required. Verify original evidence survives errors, status-only binding writes
   are harmless, unchanged-verdict revisions update status only, and restart
   requires fresh validation. Exact internal publication interleavings remain
   deterministic component tests with barriers. Assert Pass→Skipped writes a
   transition report, repeated Skipped updates current evidence only,
   NoChecksEvaluated coverage is explicit, and mixed results retain existing
   aggregation. Current/Ready must not be interpreted as Pass.
5. Use deterministic component tests to generate at-limit and over-limit inputs
   for every numeric row, including decompressed error/discovery bodies, parser
   aliases/depth, continuation pages, policy overrides and injected recoverable
   panics. Assert failure reasons, slot release, healthy-peer progress and
   impossible-under-normal-admission collision fixtures. In kind, exercise hostile
   input and verify isolation with real permissions and peers.
6. Run the target release's `fathomctl definition collisions`; assert it prints
   its own version/build and detects a newly colliding builtin. Unknown build
   metadata must fail visibly; external inventory files are not supported. Confirm startup blocks both
   candidates even when preflight is skipped; resolve explicitly and verify requeue.
7. Disable binding and run `fathomctl definition drain` with the configured namespace
   and election ID; verify live Lease renewal, matching epoch/generation and zero
   active runs per contracts/leadership.md. Missing/denied/stale Lease verification
   must fail closed; election-disabled runtime must remain inactive while built-ins
   run. Revoke reader and impersonation grants, export complete AddonCheck objects
   and status plus HealthReports, definitions and bindings, then disable loading
   before any older binary can write. Fathom v0.5.1 lacks the newer status fields,
   and its full status update can remove them; transition-only reports may omit
   newest same-verdict evidence. Use the read-only backup commands in the
   [rollback guide](../../docs/guides/addon-definitions.md#changes-drain-and-rollback),
   plan restoration separately; the bounded trial ran and recorded the exact
   backup commands in [operations-qualification.md](operations-qualification.md).
   Re-enable
   only after direct revalidation. The actual host-local namespace and Lease
   trial passed; see [operations-qualification.md](operations-qualification.md).
   This bounded trial is not a released chart upgrade qualification, and #256
   remains a separate release gate.

## Release evidence

Link all contract cases to tests/results. Include normal CI, generated output,
compatibility, license, full-kind and security evidence, plus #256's separate
ratio-key compatibility decision and older-adapter regression. A known
input-triggerable fatal process failure blocks enabling/releasing runtime loading.

US2's harness in milestone 2 is a development checkpoint. Component tests satisfy
the exact-boundary and injected-panic obligations; only the real-cluster scenarios
above satisfy its authority, hostile-input and lifecycle independent acceptance
after US3 wiring.
