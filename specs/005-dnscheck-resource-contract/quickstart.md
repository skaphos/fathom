# Quickstart: Validating the DNSCheck Resource Contract

**Feature**: `specs/005-dnscheck-resource-contract`

How to prove this slice works. Two halves, validated independently — the
declarable contract (admission) and the resolution capability (the probe) —
matching User Story 1 and User Story 2, both P1.

There is no controller in this slice, so nothing here starts a reconcile loop.
That is the point: both halves are testable without one.

## Prerequisites

- Go toolchain per `go.mod` (1.26.5).
- envtest binaries — bootstrapped automatically by the `test` task into
  `bin/k8s/`.
- For the optional cluster walkthrough: `kind` and `docker`.

All tooling is invoked through the pinned task wrappers; never call
`controller-gen`, `kustomize`, or `setup-envtest` directly.

## 1. Regenerate and verify the generated surface

```bash
go -C tools tool task manifests generate
go -C tools tool task docs:api-ref
```

Expected: `api/v1alpha1/zz_generated.deepcopy.go`,
`config/crd/bases/fathom.skaphos.io_dnschecks.yaml`, the sample under
`config/samples/`, and `docs/reference/api.md` all update. None of these are
hand-edited; CI's `verify-generated` job fails if they drift.

```bash
go -C tools tool task crd-compat
```

Expected: passes (FR-029). A brand-new CRD introduces no incompatible change
to an existing one, so a finding here means something touched a neighbouring
kind by accident.

## 2. Prove the CEL rules fit the cost budget — do this early

```bash
go -C tools tool task install
```

Expected: the CRD installs. **This is the gate most likely to fail**, and it
fails at install time rather than at use time. The API server rejects a schema
whose estimated CEL cost exceeds the per-CRD budget, and this repository has
hit that ceiling before (`api/v1alpha1/addoncheck_types.go:36-40`). Running it
before the validation matrix is written avoids discovering a budget problem
after the tests are built around the rules.

If it fails, the lever is bounds, not cleverness: the estimator prices rules by
the declared maximum size of their inputs, so tightening `MaxItems` and
`MaxLength` reduces cost directly (research R4).

## 3. Validate the admission contract (User Story 1)

```bash
go -C tools tool task test
```

The envtest matrix in `api/v1alpha1/` walks every row of
[`contracts/dnscheck-admission.md`](contracts/dnscheck-admission.md) — 32 cases,
each asserting accept or reject, and for rejections that the message names the
offending field.

Spot-check by hand against a live cluster:

```bash
kubectl apply -f config/samples/fathom_v1alpha1_dnscheck.yaml
kubectl get dnschecks
```

Expected columns: `RESULT`, `TARGETS`, `LAST RUN`, `AGE` (FR-026). `RESULT` is
empty — nothing is evaluating it yet, which is correct for this slice.

Confirm the defaults are real rather than documented:

```bash
kubectl get dnscheck cluster-dns -o jsonpath='{.spec.targets[0].recordType} {.spec.interval} {.spec.timeout}'
```

Expected: `A 1m 10s`.

Confirm a rejection names its field:

```bash
kubectl apply -f - <<'EOF'
apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: bad
spec:
  interval: 1m
  timeout: 5m
  targets:
    - name: example.com
EOF
```

Expected: rejected, message naming `timeout` and `interval`.

## 4. Validate the resolution capability (User Story 2)

Unit tests, no cluster needed:

```bash
go test ./cmd/probe/... ./internal/probe/...
```

These cover one case per record kind, answer-match and answer-mismatch, both
polarities, and the outcome matrix in
[`contracts/probe-dns-cli.md`](contracts/probe-dns-cli.md).

Two cases carry more weight than the rest, because both are places where the
naive implementation compiles and returns a wrong answer (research R1):

- **`CNAME` on a name with no CNAME record.** `LookupCNAME` succeeds and
  returns the subject itself. The test asserts this is *not* a pass.
- **`SRV` on an underscore-labelled subject.** `LookupSRV` must be called with
  empty service and proto, or it rewrites the query name. The test asserts the
  queried name is the one declared.

Exercise the binary directly:

```bash
go build -o bin/probe ./cmd/probe

bin/probe -mode dns -target kubernetes.io -record-type A
bin/probe -mode dns -target _https._tcp.example.com -record-type SRV
bin/probe -mode dns -target no-such-name.invalid -absent      # expect Pass
bin/probe -mode dns -target kubernetes.io -absent             # expect Fail
```

Each writes one JSON result to stdout and `/dev/termination-log`.

## 5. Prove the shared path did not move (FR-030)

The highest-risk edit in this slice touches `internal/probe`, which has a live
consumer in `internal/adapter/nodelocaldns/adapter.go:455`.

```bash
go test ./internal/adapter/nodelocaldns/...
go -C tools tool task test-e2e E2E_ADDONS=nodelocaldns
```

Expected: unchanged, both before and after. A default-path `dns` probe must
still perform a host lookup returning both address families — narrowing it to
IPv4 would be a silent behavior change on an existing check, which is exactly
what FR-030 forbids.

`E2E_ADDONS=nodelocaldns` is the scoped run (core tier plus that addon); the
full stack is not required for this slice, since no controller or reconciler
changes.

## 6. Full local CI

```bash
go -C tools tool task ci
```

Runs lint, test, staticcheck, vuln, and build. `reuse lint` must also pass —
every new Go file carries the SPDX header from `hack/boilerplate.go.txt`.

## What this quickstart deliberately does not show

- Starting a `DNSCheck` and watching `status.lastResult` populate — no
  controller exists yet (#266).
- Rolling a `DNSCheck` into a `ClusterHealth` verdict — the wrapper does not
  accept the kind yet (#267).
- End-to-end specs for the kind itself (#268).

Anything asserting those behaviors in this slice would be asserting a
capability that does not exist, which is the defect FR-013 exists to prevent.
</content>
