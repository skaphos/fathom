<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Validation guide

Prerequisites: pinned Go tooling, envtest, Docker, Kind, Helm/Helmfile, kubectl. Use an isolated kubeconfig and cluster.

1. Run focused unit and controller metrics regressions; include the failing-before agent disclosure test recorded in the PR.
2. Run `go -C tools tool task ci` and the coverage gate, `go -C tools tool task verify-alert-rules`, `go -C tools tool task crd-compat`, and `reuse lint`.
3. Run `KUBECONFIG=/tmp/fathom-metrics-security-e2e.kubeconfig go -C tools tool task test-e2e E2E_KIND_CLUSTER=fathom-metrics-security`. Full stack is required because both reconcilers and node-agent runtime change.
4. In the security e2e cases, expect agent `/metrics` to return 404 and `/healthz` to remain healthy; expect anonymous/unauthorized operator requests to be denied; expect authorized scrapes to include both node metric families with the contract labels and no certificate path.
5. Run `graphify update .` and review source/documentation diffs; generated graph contents are not hand-reviewed.

Validation evidence for the completed implementation:

- The disclosure regression initially observed HTTP 200 from the agent
  `/metrics` endpoint before the fix; the final behavior is HTTP 404, while
  `/healthz` remains healthy. All four focused stdlib parser/transport tests
  passed.
- `go -C tools tool task ci`, `./scripts/check-coverage.sh coverage.out`,
  `go -C tools tool task verify-generated`, and `reuse lint` passed. Final
  `go -C tools tool task lint` completed with 0 issues.
- `KUBECONFIG=/tmp/fathom-metrics-security-e2e.kubeconfig go -C tools tool task test-e2e E2E_KIND_CLUSTER=fathom-metrics-security`
  passed 95/95 Ginkgo specs with 0 failed, pending, or skipped tests in
  433.444s; including the four focused stdlib parser/transport tests, the
  package completed in 435.516s; the Kind cluster was automatically deleted.
- One independent bypass/regression review found no remaining runtime security
  issue; documentation findings were corrected. The final readiness changes
  were checked by the primary agent with focused tests, lint, and the full
  Kind run.

See [metrics contract](contracts/metrics.md) for scrape migration, replica behavior, report freshness, and upgrade requirements. Never log monitoring bearer tokens in test commands or diagnostic output.
