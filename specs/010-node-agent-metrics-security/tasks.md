<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Tasks: Node-agent metrics security

**Input**: [spec.md](spec.md), [plan.md](plan.md), [metrics contract](contracts/metrics.md)

## Phase 1: Setup

- [x] T001 Trace both agent modes, operator authentication, report validation, and existing monitoring in `research.md`.
- [x] T002 Specify the access/label/lifecycle contract in `contracts/metrics.md` and record the boundary decision in `docs/adr/0006-node-metrics-through-operator.md`.

## Phase 2: Foundational contract

- [x] T003 Add check-scoped collector helpers and label/value/isolation coverage in `internal/metrics/metrics.go` and `internal/metrics/*test.go`.

## Phase 3: US1 — Protect node inventory

**Independent test**: Anonymous requests disclose no metrics; healthz and publication remain functional.

- [x] T004 [US1] Demonstrate a failing-before anonymous metrics regression and liveness control in `cmd/node-agent/*test.go`.
- [x] T005 [US1] Remove agent metric serving/publication while preserving liveness/listener compatibility in `cmd/node-agent/main.go`.
- [x] T006 [P] [US1] Add real-cluster anonymous/unauthorized denial and authorized controls for both modes, including host network, in `test/e2e/nodeagent_metrics_test.go` and existing node suites as needed.

## Phase 4: US2 — Keep monitoring useful

**Independent test**: Authorized operator metrics contain minimum certificate expiry and health measurements with scoped identities and correct lifecycle.

- [x] T007 [US2] Project accepted in-scope reports and clear details on all early exits in `internal/controller/nodecertificatecheck_controller.go` and `internal/controller/nodehealthcheck_controller.go`.
- [x] T008 [US2] Cover accepted/partial/stale/rejected/departed reports, pause/delete/error withdrawal, and unrelated-check isolation in `internal/controller/node_agent_metrics_test.go` or adjacent tests.
- [x] T009 [P] [US2] Validate authenticated series values and labels in `test/e2e/nodeagent_metrics_test.go`.
- [x] T010 [P] [US2] Update scrape/alert migration, endpoint security, report cadence, replica limits, and image-upgrade requirements in `docs/guides/monitoring.md`, adjacent node/network docs, `README.md`, and `RELEASE.md`.

## Phase 5: Review and validation

- [x] T011 Perform one independent bypass/regression review of the candidate source diff and resolve confirmed findings; record evidence in `quickstart.md` or the PR.
- [x] T012 Run focused tests, required local CI/coverage/alert/generated checks, and full Kind e2e per `quickstart.md`; retain exact outcomes for the PR.
- [x] T013 Run `graphify update .`, check the final diff, and reconcile task completion in this file.

## Dependencies and parallel work

T001–T002 precede implementation. T003 and T004 precede their associated implementation. T007 depends on collector contracts in T003. US1 and US2 form one releasable fix: removal must ship with the authorized replacement. E2e work (T006/T009) and documentation (T010) can run in parallel with runtime implementation once the contract is fixed. T011 follows focused checks; T012–T013 finish the candidate.

## Implementation strategy

Implement the shared agent boundary and operator projection together, first proving the disclosure regression. Test lifecycle and valid monitoring controls, then run the complete stack. The smallest shippable scope is both stories; do not ship a slice that silently drops node monitoring.
