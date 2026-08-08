---

description: "Task list for the DNSCheck resource contract"
---

# Tasks: DNSCheck Resource Contract

**Input**: Design documents from `specs/005-dnscheck-resource-contract/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/)

**Tests**: Included. Not optional here — the constitution requires new behaviour
to ship with direct test coverage, and both P1 stories define their independent
test criteria as executable matrices in `contracts/`.

**Organization**: Grouped by user story. The two P1 stories touch disjoint
code and can be implemented in either order, or concurrently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on incomplete work)
- **[Story]**: US1 = declarable contract, US2 = resolution capability

## Path Conventions

Standard kubebuilder layout, unchanged: `api/v1alpha1/`, `cmd/probe/`,
`internal/probe/`, `config/`. All tooling runs through the pinned wrappers
(`go -C tools tool task …`) — never `controller-gen` or `kustomize` directly.

---

## Phase 1: Setup

**Purpose**: Register the new kind with the scaffolding.

- [ ] T001 Add the `DNSCheck` resource entry to `PROJECT`, matching the shape of the existing five kinds (group `fathom.skaphos.io`, version `v1alpha1`, namespaced)

---

## Phase 2: Foundational (Blocking Prerequisites)

**None required.**

This is a real finding, not an omission. The two P1 stories touch disjoint
files — US1 lives entirely in `api/v1alpha1/` and `config/`, US2 entirely in
`cmd/probe/` and `internal/probe/` — and share no Go type. The probe
deliberately does **not** import `api/v1alpha1`: it takes its record kind as a
plain string flag, keeping the probe binary free of Kubernetes client
dependencies for the same reason `internal/nodecert` is (`AGENTS.md`, project
structure). Introducing a shared enum type would couple them for no benefit and
would grow the probe image.

The one value that must agree across the boundary — the record-kind spelling —
is pinned by [`contracts/probe-dns-cli.md`](contracts/probe-dns-cli.md) and
asserted independently on both sides.

---

## Phase 3: User Story 1 — Declare DNS intent and have it validated (P1)

**Goal**: An operator can author a `DNSCheck`, have malformed intent rejected at
write time with a message naming the offending field, and see the result in a
standard listing.

**Independent test**: Apply the valid/invalid corpus against a cluster with only
the CRD installed — no controller running — and assert each accept/reject.
Fully testable with no execution path.

### Types

- [ ] T002 [US1] Create `api/v1alpha1/dnscheck_types.go` with the SPDX header from `hack/boilerplate.go.txt`, `DNSCheck`, `DNSCheckList`, `DNSCheckSpec`, `DNSCheckStatus`, and `SchemeBuilder.Register` in `init()`, following `nodecertificatecheck_types.go` for shape
- [ ] T003 [US1] Add `DNSTarget`, `DNSResolver`, `DNSTargetResult`, and the `DNSRecordType` (`Host;A;AAAA;CNAME;SRV;PTR`) and `DNSResolverSource` (`Cluster;Node;Explicit`) enums to `api/v1alpha1/dnscheck_types.go` per [data-model.md](data-model.md)
- [ ] T004 [US1] Add field-level markers to `api/v1alpha1/dnscheck_types.go`: `MaxItems` (targets 16, resolvers 3, expectedAnswers 16, targetResults 48), `MaxLength` on every string, `Minimum` on `historyLimit`, defaults (`recordType: Host`, `absent: false`, `historyLimit: 10`), and `listType=map` with `listMapKey` on `resolvers` (`name`) and `targetResults` (`name`,`recordType`,`resolver`)
- [ ] T005 [US1] Add printer columns (`Result`, `Targets`, `Last Run`, `Age`) and `+kubebuilder:resource:categories=…` matching the existing kinds, plus `+kubebuilder:subresource:status`, to `api/v1alpha1/dnscheck_types.go`

### Validation rules

- [ ] T006 [US1] Add the three cadence CEL rules to `DNSCheckSpec` in `api/v1alpha1/dnscheck_types.go` — `interval >= 10s`, `timeout >= 1s`, `timeout <= interval` — reusing the exact literals already used by `AddonCheckSpec` so the floors cannot drift from `MinCheckInterval`/`MinCheckTimeout`
- [ ] T007 [US1] Add the per-target CEL rules to `api/v1alpha1/dnscheck_types.go`: `absent == true` forbids `expectedAnswers` (FR-005), and `recordType == PTR` requires an IP subject while other kinds require a DNS name (FR-002, research R5)
- [ ] T008 [US1] Add the resolver CEL rules to `api/v1alpha1/dnscheck_types.go`: `address` required iff `from == Explicit`, `address` must be `IP` or `IP:port` and never a hostname (FR-009), and each target's `resolver` must name a declared entry

### Generation and the cost gate

- [ ] T009 [US1] Run `go -C tools tool task manifests generate` to produce `api/v1alpha1/zz_generated.deepcopy.go` and `config/crd/bases/fathom.skaphos.io_dnschecks.yaml`
- [ ] T010 [US1] **Early gate** — run `go -C tools tool task install` and confirm the CRD installs. A per-CRD CEL cost rejection surfaces here, before tests are built around the rules. If it fails, tighten `MaxItems`/`MaxLength` in T004 and repeat from T009 (research R4, quickstart step 2)
- [ ] T011 [US1] Write `config/samples/fathom_v1alpha1_dnscheck.yaml` covering the fully populated object from [contracts/dnscheck-admission.md](contracts/dnscheck-admission.md), and wire it into `config/samples/kustomization.yaml`

### Tests

- [ ] T012 [US1] Add the 34-row envtest admission matrix to `api/v1alpha1/dnscheck_validation_test.go`, one case per row of [contracts/dnscheck-admission.md](contracts/dnscheck-admission.md), asserting for each rejection that the message names the offending field
- [ ] T013 [P] [US1] Add `fullyPopulatedDNSCheck` and its `TestDeepCopy_DNSCheck` / round-trip cases to `api/v1alpha1/deepcopy_test.go`, following the existing `fullyPopulatedNodeCertificateCheck` pattern
- [ ] T014 [P] [US1] Extend `api/v1alpha1/groupversion_info_test.go` so `DNSCheck` and `DNSCheckList` are covered by the scheme-registration assertions alongside the existing kinds

**Checkpoint**: `kubectl get dnschecks` shows the columns, defaults read back as
`Host 1m 10s`, and every malformed manifest is rejected by name.

---

## Phase 4: User Story 2 — Every declared expectation is evaluated (P1)

**Goal**: A declared record kind is queried as that kind, a declared resolver is
the one asked, declared answers are compared, and polarity is honoured.

**Independent test**: Exercise the probe binary and pod builder directly against
a controlled fixture — no CRD, no cluster resource, no controller.

### Regression guard first

- [ ] T015 [US2] **Before touching `cmd/probe` or `internal/probe`**, add the FR-030 baseline assertions: `cmd/probe/main_test.go` asserts a `dns` run with no `-record-type` performs a host lookup answering on either address family, and `internal/probe/pod_test.go` asserts a `Request` with no vantage point set produces a pod with no `dnsPolicy` override and one with `DNSNameservers` still produces `dnsPolicy: None` + `dnsConfig.nameservers`

### Probe binary

- [ ] T016 [US2] Add the `-record-type` flag (default `Host`) to `cmd/probe/main.go` and dispatch `runDNS` per kind, keeping `Host` bound to the existing `LookupHost` call so the default path is byte-for-byte today's behaviour
- [ ] T017 [US2] Implement `A`/`AAAA` via `LookupIP(ctx, "ip4"|"ip6", …)` and `PTR` via `LookupAddr` in `cmd/probe/main.go`, recording answers into `Details`
- [ ] T018 [US2] Implement `CNAME` in `cmd/probe/main.go`, treating a canonical name equal to the queried subject (modulo trailing dot) as *no CNAME record* rather than a pass — `LookupCNAME` succeeds in that case (research R1, trap 1)
- [ ] T019 [US2] Implement `SRV` in `cmd/probe/main.go` via `LookupSRV(ctx, "", "", name)` with empty service and proto so the queried name is not rewritten, recording answers as `target:port` (research R1, trap 2)
- [ ] T020 [US2] Add the `-expect-answers` flag to `cmd/probe/main.go` with containment matching — every declared answer must be present, extras do not fail — normalising trailing dots, ASCII case, and comparing IPs as parsed addresses
- [ ] T021 [US2] Add the `-absent` flag to `cmd/probe/main.go` and implement the full outcome matrix from [contracts/probe-dns-cli.md](contracts/probe-dns-cli.md), branching on `net.DNSError.IsNotFound` so that under a negative assertion an unreachable resolver is `Error` and never `Pass` (FR-014)
- [ ] T022 [US2] Make `-absent` combined with `-expect-answers` an `Error` in `cmd/probe/main.go` rather than silently preferring one — admission rejects the combination, but the probe must not assume valid input

### Pod builder

- [ ] T023 [US2] Add a `DNSFrom` vantage-point selector (`Cluster`/`Node`/`Explicit`, zero value `Cluster`) to `Request` in `internal/probe/pod.go`, with `Node` setting `dnsPolicy: Default` and `Explicit` keeping today's `dnsPolicy: None` + `dnsConfig.nameservers`; a non-empty `DNSNameservers` with `DNSFrom` unset must continue to mean `Explicit` so the existing caller is unaffected
- [ ] T024 [US2] Thread `RecordType`, `ExpectedAnswers`, and `Absent` through `Request` and the `args()` builder in `internal/probe/pod.go`, emitting a flag only when the value is non-default so existing callers' argv is unchanged

### Tests

- [ ] T025 [US2] Add the per-kind table test to `cmd/probe/main_test.go` covering `Host`, `A`, `AAAA`, `CNAME`, `SRV`, `PTR`, including the named trap cases: a `CNAME` lookup on a name with no CNAME record must not pass, and an `SRV` lookup on an underscore-labelled subject must query the declared name
- [ ] T026 [US2] Add the outcome-matrix table test to `cmd/probe/main_test.go` covering both polarities against every row of [contracts/probe-dns-cli.md](contracts/probe-dns-cli.md), with the unreachable-under-negation case asserted explicitly as `Error`
- [ ] T027 [P] [US2] Add answer-matching tests to `cmd/probe/main_test.go` for containment, superset-passes, missing-answer-fails, and normalisation of trailing dots, case, and IP textual forms
- [ ] T028 [P] [US2] Add vantage-point tests to `internal/probe/pod_test.go` asserting the pod spec produced for each of `Cluster`, `Node`, and `Explicit`
- [ ] T029 [US2] Run `go test ./internal/adapter/nodelocaldns/...` and confirm it passes unchanged, proving the shared-path edit did not move the live consumer (FR-030)

**Checkpoint**: `bin/probe -mode dns …` honours every flag, and the nodelocaldns
adapter behaves exactly as before.

---

## Phase 5: Polish & Cross-Cutting

- [ ] T030 Run `go -C tools tool task docs:api-ref` to regenerate `docs/reference/api.md`, and confirm the `DNSCheck` section describes only fields that are honoured (FR-013, SC-005)
- [ ] T031 [P] Run `go -C tools tool task crd-compat` and confirm no finding against any pre-existing kind (FR-029)
- [ ] T032 [P] Run `reuse lint` and confirm every new Go file carries the SPDX header from `hack/boilerplate.go.txt`
- [ ] T033 Run `go -C tools tool task ci` (lint, test, staticcheck, vuln, build) and confirm green
- [ ] T034 Run the scoped end-to-end check `go -C tools tool task test-e2e E2E_ADDONS=nodelocaldns` — required because `internal/probe/*` changed and that is the live consumer of the shared path. The full stack is not required: no controller, reconciler, or scheme registration changed in this slice
- [ ] T035 Run `graphify update .` to refresh the knowledge graph after the code changes, and include the result in the PR per the repository's PR workflow

---

## Dependencies

```text
T001 (PROJECT)
  │
  ├─► Phase 3 (US1): T002 → T003 → T004 → T005 → T006 → T007 → T008
  │                    → T009 (generate) → T010 (install: COST GATE)
  │                    → T011, T012, T013, T014
  │
  └─► Phase 4 (US2): T015 (regression guard, FIRST)
                       → T016 → {T017, T018, T019} → T020 → T021 → T022
                       → T023 → T024
                       → T025, T026, T027, T028 → T029

Phase 3 and Phase 4 are independent — disjoint files, no shared type.

Phase 5 requires both.
```

**The one ordering that matters most**: T010 before T012. The CEL cost budget is
enforced at CRD install, so a too-expensive rule set fails there. Discovering it
after 34 test cases are written against those rules means rewriting both.

**The second**: T015 before T016. The FR-030 guard has to capture current
behaviour *before* the change, or it is not a regression test.

## Parallel opportunities

- **Across stories**: all of Phase 3 runs concurrently with all of Phase 4.
- **Within Phase 3**: T013 and T014 touch different existing test files and are
  independent once T009 has generated the deep-copy code.
- **Within Phase 4**: T017, T018, and T019 are separate record kinds; they touch
  the same file, so treat them as sequential edits unless split by function.
  T027 and T028 are in different files and genuinely parallel.
- **Within Phase 5**: T031 and T032 are read-only checks over different
  surfaces.

## Implementation strategy

**MVP is both P1 stories, not one.** This is the unusual case where the
minimum shippable increment spans two stories, and it is deliberate: FR-013
forbids publishing a schema field the evaluation path ignores. Shipping US1
alone would produce exactly the documented-but-unhonored defect already
recorded as finding API-5. Shipping US2 alone delivers probe flags nothing can
reach.

**Suggested order**: start Phase 4 (US2) first despite Phase 3 being US1. The
probe work has no gate that can force a rewrite, whereas Phase 3's cost gate
(T010) can. Getting US2 to green first means the cost gate, if it bites, bites
against a smaller unfinished surface.

**Out of this feature entirely** — spec User Stories 3 and 4:

- The `DNSCheckReconciler`, cadence-driven execution, status persistence,
  report history, run-now trigger → #266
- Generalizing `CheckTargetRef` so a `DNSCheck` mirrors into `ClusterHealth`,
  plus its RBAC grant and justification row → #267
- End-to-end specs for the kind and the operator guide → #268

## Task summary

| Phase | Tasks | Story |
|-------|-------|-------|
| 1 — Setup | T001 | — |
| 2 — Foundational | none (justified above) | — |
| 3 — Declare and validate | T002–T014 (13) | US1 |
| 4 — Evaluate what was declared | T015–T029 (15) | US2 |
| 5 — Polish | T030–T035 (6) | — |
| **Total** | **35** | |
</content>
