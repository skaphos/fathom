# Quickstart Validation: Quorum/Ratio Rollups

Runnable scenarios proving the feature end-to-end. Contracts:
[contracts/ratio-rollup.md](contracts/ratio-rollup.md); entities:
[data-model.md](data-model.md).

## Prerequisites

- Go toolchain per `go.mod`; `kind` + Docker for e2e.
- All commands from the repo root.

## 1. Unit + lint gate

```sh
go -C tools tool task test    # includes pkg/adapter ratio tables + controller aggregation tests
go -C tools tool task lint
```

Expected: green; `pkg/adapter` and `internal/controller` coverage does not
drop below the ratchet.

## 2. Behavior: one broken CR no longer reds the addon (US1)

Against a kind cluster with the operator and an addon exercising a
multi-resource family (e.g. cert-manager certificates):

```sh
kubectl apply -f - <<'EOF'
apiVersion: skaphos.io/v1alpha1
kind: AddonCheck
metadata: { name: certs-ratio, namespace: fathom-system }
spec:
  addonType: cert-manager
  policy:
    certificates:
      thresholds: { warnRatio: "1", failRatio: "5" }
EOF
```

With N healthy certificates and 1 permanently-broken one (< thresholds):

- `kubectl get addoncheck certs-ratio -o jsonpath='{.status.lastResult}'`
  → **not** `Fail`.
- Latest HealthReport contains the broken certificate's individual `Fail`
  entry **and** a `details.rollup: ratio` entry with
  `population`/`unhealthy`/`degraded` counts (FR-006, FR-010).

## 3. Behavior: escalation boundaries (US2)

Drive the degraded fraction past `warnRatio`, then past `failRatio`
(create/break certificates); observe `status.lastResult` transition
Pass → Warn → Fail. Equality does not escalate (strict-exceed).

## 4. Compatibility: no thresholds → no change (US3)

Delete the thresholds from the policy; re-run. Expected: verdicts identical
to the pre-feature build (single Fail → family Fail), and the HealthReport
contains **no** `rollup` entries.

## 5. Validation: invalid values rejected (SC-005)

Set `failRatio: "banana"` (or `"150"`). Expected:
`kubectl get addoncheck certs-ratio -o jsonpath='{.status.conditions[?(@.type=="Accepted")]}'`
shows `status: "False"`, reason `InvalidPolicy`, message naming the family
and key.

## 6. Full e2e (required before PR-ready)

```sh
go -C tools tool task test-e2e
```

This change touches `internal/controller/*` and `pkg/adapter/*` — full-stack
e2e is mandatory per AGENTS.md; record the outcome in the PR test plan.
