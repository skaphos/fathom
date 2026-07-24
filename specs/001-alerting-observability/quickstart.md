# Quickstart Validation: Alerting-Grade Observability

Runnable scenarios proving the feature end-to-end. Contracts:
[metrics](contracts/metrics.md), [events](contracts/events.md).

## Prerequisites

- `kind` + `docker` on PATH (e2e stack), or any cluster + `task run` from a
  host kubeconfig
- Unit/envtest path needs nothing beyond the repo toolchain

## 1. Unit + envtest gates

```sh
go -C tools tool task test        # metrics helpers + reconciler event/gauge coverage
go -C tools tool task lint
go -C tools tool task staticcheck
```

Expected: new tests cover the one-hot invariant, the Unknown/0 sentinels,
series deletion, and FakeRecorder-observed transition/failure events.

## 2. Metrics on a live cluster

```sh
go -C tools tool task test-e2e E2E_ADDONS=cert-manager   # or: task run + apply a sample
kubectl -n fathom-system port-forward deploy/fathom-controller-manager 8443:8443 &
curl -sk https://localhost:8443/metrics | grep '^fathom_check_'
```

Expected:
- six `fathom_check_result{...}` series per check, exactly one `1`
- `fathom_check_last_run_timestamp_seconds` present and recent
- a freshly created check (before its first run) shows `result="Unknown"` at
  `1` and last-run `0`

## 3. Series lifecycle

```sh
kubectl delete addoncheck <name>
curl -sk https://localhost:8443/metrics | grep 'name="<name>"'   # expect: nothing
```

## 4. Events in kubectl describe

```sh
kubectl describe addoncheck <name>
```

Expected: `ResultChanged` event for the first result (`Unknown → Pass`);
break the addon (e.g. scale cert-manager to 0), wait one interval, expect a
Warning `ResultChanged` (`Pass → Fail`); repeated identical failures
aggregate (count increases, no per-run spam).

## 5. Alert-rule component

```sh
kustomize build config/components/prometheus-rule/    # via the pinned task wrapper
```

Expected: renders a valid `PrometheusRule` with `FathomCheckFailing` and
`FathomCheckStale`; `config/default` continues to build with the component
commented out (opt-in, no prometheus-operator CRD dependency).

## 6. Docs

`docs/guides/monitoring.md` §2 documents both gauges; §4 no longer says
"read the status, not a metric" — it shows the shipped rules and points at
the opt-in component.
