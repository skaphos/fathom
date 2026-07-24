# Quickstart: Validating CRD Validation Hardening

Runnable scenarios proving the three slices work end-to-end. Contracts:
[crd-validation.md](contracts/crd-validation.md),
[clamp-signal.md](contracts/clamp-signal.md),
[schema-compat-gate.md](contracts/schema-compat-gate.md).

## Prerequisites

- Repo toolchain: `go`, `docker`, `kind` on PATH (tasks pin everything else).
- All commands from the repo root.

## 1. Regenerate and verify no drift

```sh
go -C tools tool task manifests   # CRDs pick up the new markers
go -C tools tool task lint        # includes generation
go -C tools tool task test        # envtest: admission matrices + clamp + samples regression
```

Expected: green; `git status` shows only intended generated changes under
`config/crd/bases/` and `docs/reference/api.md`.

## 2. Admission floors (live cluster)

```sh
go -C tools tool task install     # apply CRDs to current kubeconfig context
kubectl apply -f - <<'EOF'
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata: {name: floor-check, namespace: default}
spec: {addonType: coredns, interval: 1ms}
EOF
```

Expected: rejected; error names `interval` and the 10s minimum. Repeat with
`timeout: 500ms` → rejected naming `timeout` and 1s. With `interval: 5m,
timeout: 5s` → accepted.

## 3. Policy validation (live cluster)

```sh
kubectl apply -f - <<'EOF'
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata: {name: policy-check, namespace: default}
spec:
  addonType: coredns
  policy:
    dns_resolution:
      thresholds: {warnDays: "banana"}
EOF
```

Expected: rejected naming `warnDays`. Variants per the contract matrix:
`namespaces: [Prod_NS]` → rejected; family key `dns_resolution` (underscore)
→ accepted; `matchExpressions` with operator `In` and no `values` → rejected.

Every shipped sample must still admit:

```sh
kubectl apply --dry-run=server -f config/samples/
```

## 4. Runtime clamp (envtest-covered; live check optional)

Unit/envtest coverage is authoritative (admission now blocks sub-floor
values, so a live sub-floor object can't be created on fresh CRDs). Optional
live check on a cluster that had pre-floor CRDs: after upgrading CRDs and
operator with such an object stored,

```sh
kubectl describe addoncheck <name>   # expect Warning CadenceClamped event
kubectl get addoncheck <name> -o jsonpath='{.status.conditions[?(@.type=="Accepted")].reason}'
# expect: SpecClamped
```

## 5. Schema-compat gate

```sh
git fetch --tags
go -C tools tool task crd-compat
```

Expected on this feature branch: exits 0; output lists the floor/policy
tightenings as `SANCTIONED` with reasons and the issue link (seeded
allowlist). Negative check — temporarily delete a field from a CRD YAML under
`config/crd/bases/`, rerun: exits 1 naming the CRD and property path; revert.

Fixture matrix (no cluster, no tags needed):

```sh
./scripts/check-crd-compat_test.sh   # pass/fail matrix per the gate contract
```

## 6. Full e2e (required before PR is ready — CRD types changed)

```sh
go -C tools tool task test-e2e
```

Expected: green across the core tier and addon shards; includes the
admission-rejection smoke spec.
