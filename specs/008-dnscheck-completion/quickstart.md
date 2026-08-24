<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->

# Quickstart Validation: DNSCheck Completion

This guide validates the implementation; it is not the end-user DNS guide
delivered by the feature.

## Prerequisites

- Go and repository tools available through the pinned task wrappers.
- For e2e: Docker daemon, Kind, Helm, and Helmfile available on `PATH`.
- A clean working tree except for the feature changes being validated.

## 1. Verify generated contracts

```sh
go -C tools tool task fmt
go -C tools tool task docs:api-ref
go -C tools tool task lint
git diff --check
```

Expected results:

- the generated CRD/API reference lists only AddonCheck, DNSCheck, and
  NodeCertificateCheck as supported HealthCheck targets;
- `apiVersion` is bounded but not admission-enumerated;
- generated RBAC contains no permission broader than the existing read access
  for the three specialized resources; and
- a second generation run produces no additional diff.

## 2. Verify projection and watch behavior

```sh
go -C tools tool task test
go -C tools tool task vet
go -C tools tool task staticcheck
go -C tools tool task vuln
```

The tests must demonstrate:

- successful normalized projection for all three supported kinds;
- empty and explicit-current API versions;
- unsupported API version and kind clearing;
- default and explicit namespaces;
- NotFound clearing versus transient-error preservation;
- exact source-event matching for API version, kind, namespace, and name;
- unchanged AddonCheck behavior and summary truncation; and
- semantic no-op reconciliation does not repeatedly update status.

See [the projection contract](contracts/healthcheck-target-projection.md) for
the expected status transitions.

## 3. Verify the real DNS aggregation chain

Run the full stack because HealthCheck watch wiring is a shared controller
surface:

```sh
go -C tools tool task test-e2e
```

The core DNS scenario must:

1. create a resolvable DNSCheck;
2. create a HealthCheck referencing it and a ClusterHealth selecting the
   wrapper;
3. observe DNSCheck, HealthCheck, and ClusterHealth at Pass;
4. change the DNS expectation so the source reaches Fail;
5. observe the same source evidence in HealthCheck and Fail in ClusterHealth;
6. reconcile an unchanged verdict and confirm change-only HealthReport history
   does not grow; and
7. clean up all test resources.

Existing DNS resolution, required-absence, explicit-resolver, truncation,
restricted-policy, and probe-lifecycle specs must continue passing.

## 4. Verify documentation and licensing

```sh
reuse --no-multiprocessing lint
graphify update .
git diff --check
```

Confirm that `docs/guides/dns-checks.md` contains copyable manifests for the
DNSCheck, HealthCheck, and ClusterHealth chain, both expectation polarities,
resolver choices, and actionable troubleshooting. Confirm guide navigation and
architecture link to the completed path.

## Expected completion signal

All commands pass, generated artifacts are stable, the real-cluster Pass/Fail
transition converges within the suite timeout, and no direct specialized-check
or HealthReport dependency appears in ClusterHealth.
