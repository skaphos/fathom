<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Implementation Plan: Node-agent metrics security

**Branch**: `fix/node-agent-metrics-security` | **Date**: 2026-09-19 | **Spec**: [spec.md](spec.md)

## Summary

Remove the node-agent metrics route in both modes. Project accepted node reports through the existing operator HTTPS/authentication/authorization boundary. Preserve metric names; use namespace/check/node identity, reduce certificate expiry to the minimum known days remaining, and retain health item dimensions. No new RBAC, credentials, dependencies, CRD fields, or status semantics.

## Technical Context

- **Language/Version**: Go per `go.mod` (1.27).
- **Primary Dependencies**: existing controller-runtime metrics server and Prometheus client_golang.
- **Storage**: existing report ConfigMaps; in-process gauges are reconstructible projections.
- **Testing**: stdlib unit tests, controller Ginkgo/envtest, full Kind e2e, existing alert-render and CI tasks.
- **Target Platform**: Kubernetes, pod-network certificate agents and pod/host-network health agents.
- **Performance Goals**: linear projection of the report set already read by reconciliation; no scrape-time API reads.
- **Constraints**: no agent auth-delegation grants; preserve liveness and report publication; certificate paths never become labels.
- **Scale/Scope**: one certificate series per namespace/check/node; health series retain schema-bounded item dimensions and six result states.

## Constitution Check

Passed before research and after design:

- Standalone operation and existing durable report protocol remain intact (upstream facts/ecosystem linked in spec).
- Adopt existing controller-runtime authentication and monitoring integration rather than build agent TLS/credential management.
- ClusterHealth remains derived from HealthCheck status; metric projection does not alter verdicts or history.
- Bounded reconciliation; no new I/O or cluster-wide RBAC.
- Observable behavior, lifecycle limits, upgrade changes, regression tests, and full e2e are explicit.
- The monitoring boundary decision is recorded in ADR-0006; no accepted ADR is superseded.

## Project Structure

```text
specs/010-node-agent-metrics-security/{spec,plan,research,data-model,quickstart,tasks}.md
specs/010-node-agent-metrics-security/contracts/metrics.md
cmd/node-agent/main.go                    # healthz-only HTTP mux, unchanged publication
internal/metrics/metrics.go              # check-scoped node collectors/helpers
internal/controller/node*check_controller.go # project only accepted in-scope reports
internal/controller/node_agent_metrics_test.go # integration/lifecycle regressions
cmd/node-agent/*test.go                  # disclosure regression and publication controls
internal/metrics/*test.go                # label/value/scoping tests
 test/e2e/nodeagent_metrics_test.go        # real-cluster auth and agent checks
 docs/{guides,reference}/                 # scrape migration and security documentation
 docs/adr/0006-node-metrics-through-operator.md
```

Use existing gauge delete/rebuild conventions from DNS target metrics. Delete only the requested check's node-series at reconcile entry, then repopulate after successful report validation and expected-node discovery. That covers deletion, pause, API/provisioning errors, invalid specs, rejected/missing reports, and removed nodes without affecting unrelated checks. Incomplete fleet windows may expose the accepted subset; stored aggregate status remains frozen by existing policy. Scraping during a reconcile may briefly observe no node-detail series, matching existing projection conventions.

The agent retains its existing listener flags/ports for liveness and upgrade compatibility, but `/metrics` returns 404 and no Prometheus registry is served. NetworkPolicy remains defense in depth. Keep host-network collision/liveness behavior unchanged.

## Complexity Tracking

No constitution exceptions. Do not introduce a separate snapshot store, background expiration loop, or authentication service. Detail gauges reflect the latest successful reconciliation; report staleness is evaluated on existing watch/cadence paths, with existing check timestamps and alerts detecting controller stalls.
