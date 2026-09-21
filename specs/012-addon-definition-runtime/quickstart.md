<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Validation guide

This is a future implementation run guide. Planned commands/samples become usable
only when their tasks land; no runtime success is claimed by this spec change.

## Planning checks now

From repository root:

```sh
.specify/scripts/bash/check-prerequisites.sh --json --require-tasks --include-tasks
git diff --check
reuse lint
```

Inspect [contracts/runtime.md](contracts/runtime.md) for exact caps and event cases.

## Implementation checks

Install the Go version in go.mod, a running Docker-compatible daemon, kind, helm
and helmfile. Keep envtest, kind node and docs Kubernetes versions aligned with
module versions as AGENTS.md requires. Use only pinned task wrappers:

```sh
go -C tools tool task generate manifests gen:addon-rbac helm:sync docs:api-ref
go -C tools tool task verify-generated crd-compat
go -C tools tool task ci
go test -race ./internal/adapter/registry/... ./internal/adapter/runtime/...
go -C tools tool task test-e2e
```

The runtime package is planned. Add race coverage for publication/drain to the
controller test seam using envtest assets provisioned by the test task. Record
exact commands and outcomes in execution.md; missing tools are explicit blockers,
not successful skips. Full shared-surface e2e is mandatory, not one addon shard.

## Real-cluster scenarios

1. Install generated CRDs/operator with runtime option default disabled and election enabled. Confirm
   compiled checks unchanged and external definitions inert.
2. Use the future `fathomctl definition` commands to render a non-built-in sample,
   dedicated SA and grants. Review/apply definition and SA; resolve actual UIDs;
   review proposed binding/grants into Git, apply grants then enable binding and
   opt in to runtime loading. Assert attributed completed evidence.
3. Deny discovery/read permissions, select out-of-scope targets, use manager/builtin/
   shared SAs, recreate identities, omit factory/namespace and disable metrics.
   Assert no privileged fallback; test same-UID in-scope granted retargeting succeeds.
4. Execute every lifecycle row using test barriers around reads/publication.
   Verify original evidence survives errors, status-only binding writes are harmless,
   unchanged-verdict revisions update status only, and restart requires fresh validation.
   Assert Pass→Skipped writes a transition report, repeated Skipped updates current
   evidence only, NoChecksEvaluated coverage is explicit, and mixed results retain
   existing aggregation. Current/Ready must not be interpreted as Pass.
5. Generate at-limit and over-limit inputs for every numeric row, including
   decompressed error/discovery bodies, parser aliases/depth, continuation pages,
   policy overrides and panics. Assert failure reasons and healthy-peer progress.
6. Run the target release's `fathomctl definition collisions`; assert it prints
   its own version/build and detects a newly colliding builtin. Unknown build
   metadata must fail visibly; external inventory files are not supported. Confirm startup blocks both
   candidates even when preflight is skipped; resolve explicitly and verify requeue.
7. Disable binding and run `fathomctl definition drain` with the configured namespace
   and election ID; verify live Lease renewal, matching epoch/generation and zero
   active runs per contracts/leadership.md. Missing/denied/stale Lease verification
   must fail closed; election-disabled runtime must remain inactive while built-ins run. Revoke grants, disable loading, preserve unavailable evidence and
   export history before downgrade. Re-enable and verify direct revalidation.

## Release evidence

Link all contract cases to tests/results. Include normal CI, generated output,
compatibility, license, full-kind and security evidence, plus #256's separate
ratio-key compatibility decision and older-adapter regression. A known
input-triggerable fatal process failure blocks enabling/releasing runtime loading.

US2's harness in milestone 2 is a development checkpoint. Only the real-cluster
scenarios above satisfy its independent acceptance test; run them after US3 wiring.
