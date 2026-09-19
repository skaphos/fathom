<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Research: Node-agent metrics security

## Boundary and prior art

The shared `cmd/node-agent/main.go:metricsMux` exposes ctrlmetrics.Registry directly for both certificate and health modes. Certificate scan results become node/path gauge labels. Host-network health agents share the same handler and do not get a NetworkPolicy boundary. The operator already serves metrics with controller-runtime authentication and authorization in `internal/app/run.go`; the shipped ServiceMonitor uses HTTPS and a bearer token.

## Decision: serve node metrics through the operator

Reuse the accepted report stream and existing secure metrics boundary. No new credential distribution, agent TokenReview/SubjectAccessReview grants, or per-agent scrape configuration is required. Both controllers already collect, validate, and scope node reports. Prometheus sees the same health evidence as the controller.

Alternatives considered:

- Authenticate each agent using the controller-runtime filter: preserves direct scraping but requires cluster-scoped auth-delegation grants for dynamic service accounts plus TLS trust/credential lifecycle. This conflicts with the narrow-permission direction established by #255/#274 and adds operational surface.
- Reduce labels alone: does not satisfy denial of anonymous metrics access, and leaves health-agent details exposed.
- Rely on NetworkPolicy: does not protect non-enforcing CNIs or host-network agents.

## Decision: reduce certificate inventory

Keep `fathom_node_certificate_expiry_days` as the minimum known days remaining per namespace/check/node, omitting path entirely. Negative values remain meaningful for expired certificates. Missing expiry emits no sample. Detailed certificate diagnostics remain in authorized CR/report reads.

## Decision: use existing metric projection lifecycle

Scoped GaugeVec deletion/repopulation follows DNS target metrics in `internal/metrics`. Clear at reconcile entry so all early exits withdraw details. Populate accepted reports only for expected nodes, regardless of aggregate completeness. No global Reset from one check. Report freshness stays with current validators; no new clock or scrape-time Kubernetes I/O. Operator replicas/restarts follow the existing operator metrics model, and staleness remains visible through check timestamps.

## Verification

Prove the anonymous metrics regression fails before route removal, while `/healthz` and report publication work. Cover label reduction, check isolation, accepted subset, rejected/stale/departed reports, pause/deletion/error withdrawal. Kind tests request agent URLs from an ordinary pod, request operator metrics anonymously and without authorization, then use authorized scraping to validate both node metric families. Keep requests bounded and credentials out of logs.
