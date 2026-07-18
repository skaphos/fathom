<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Monitoring & Alerting

Fathom turns platform integrity into Kubernetes resource status. This guide
covers the three ways a platform team consumes that: reading status with
`kubectl`, scraping Prometheus metrics, and tracing reconciles. It closes with
practical alerting patterns and an honest note on what is and isn't a metric
today.

## 1. Read status with `kubectl`

The fastest signal is the resource status itself.

```sh
# One verdict for the namespace:
kubectl -n fathom-system get clusterhealth

# Per add-on:
kubectl -n fathom-system get addoncheck
kubectl -n fathom-system describe addoncheck cert-manager-system-health

# Node certificates — newer kind; only on builds that include
# NodeCertificateCheck (printer columns show result + coverage):
kubectl -n fathom-system get nodecertificatecheck

# History / detail for any check:
kubectl -n fathom-system get healthreport \
  -l fathom.skaphos.io/source-name=cert-manager-system-health
```

`status.conditions` on each resource explains *why* it is in its current state
(`Accepted`, `Ready`, `Paused`, and the failure reasons in
[Add-on checks → Troubleshooting](addon-checks.md#troubleshooting)). For the
complete condition reason table across all CRDs, see
[Status and conditions](../reference/status-conditions.md).

## 2. Scrape Prometheus metrics

Fathom serves metrics with controller-runtime's built-in authn/authz filter
(TokenReview + SubjectAccessReview) — there is **no** kube-rbac-proxy sidecar.

- Default endpoint: HTTPS on `:8443` (`metrics.bindAddress`, `metrics.secure`).
- Scraping requires a token whose RBAC permits access; plaintext on a
  cluster-routable port is refused unless you explicitly opt in
  (`metrics.allowInsecure`). See
  [Configuration → Options](../reference/configuration.md#options).

### Wiring up a ServiceMonitor (Helm)

The chart can create the metrics `Service` and a Prometheus-Operator
`ServiceMonitor` for you:

```sh
helm upgrade fathom oci://ghcr.io/skaphos/charts/fathom-operator \
  -n fathom-system \
  --reuse-values \
  --set metrics.service.enabled=true \
  --set metrics.serviceMonitor.enabled=true
```

- `metrics.service.enabled` (default `true`) exposes a ClusterIP Service on
  port `8443`.
- `metrics.serviceMonitor.enabled` (default `false`) creates the
  `ServiceMonitor`. Tune `interval`, `scrapeTimeout`, `labels`, and `tlsConfig`.
  The default `tlsConfig.insecureSkipVerify: true` trusts the self-signed
  serving cert; for a CA-signed metrics cert set `caFile` / `serverName` and
  flip it to `false`.

If you deploy via kustomize instead, the Prometheus `ServiceMonitor` is an
opt-in overlay under `config/components/prometheus`.

### Operator metrics

Registered with the controller-runtime registry (so the built-in
controller-runtime and Go metrics are exposed alongside them):

| Metric | Type | Labels | Use |
| --- | --- | --- | --- |
| `fathom_reconcile_total` | counter | `kind`, `outcome` | Reconcile volume and error rate per resource kind. |
| `fathom_reconcile_duration_seconds` | histogram | `kind` | Reconcile latency per kind. |
| `fathom_adapter_run_duration_seconds` | histogram | `adapter`, `family`, `outcome` | How long adapter runs take, and their outcome distribution. |
| `fathom_adapter_registered` | gauge | `adapter` | `1` for each adapter registered at startup — confirms the operator loaded the adapters you expect. |

### Node-agent metric

> Applies only to builds that include the `NodeCertificateCheck` kind — see
> [Node certificate checks → Availability](node-certificate-checks.md#availability).

Each node-agent (from a `NodeCertificateCheck`) exports an expiry gauge on its
own metrics endpoint:

| Metric | Type | Labels | Use |
| --- | --- | --- | --- |
| `fathom_node_certificate_expiry_days` | gauge | `node`, `path` | **Days until each certificate expires.** The cleanest signal for proactive cert-expiry alerts. Labeled only by node and path — certificate subject/issuer DNs are kept off this unauthenticated endpoint (and out of the label cardinality) and live in the `HealthReport` detail instead. |

The node-agents are a DaemonSet the controller creates at runtime, so how you
scrape them depends on your Prometheus setup — a `PodMonitor` selecting the
agent pods, or pod scrape annotations, rather than the operator's
`ServiceMonitor`.

## 3. Tracing

The operator can emit OpenTelemetry spans for each reconcile and adapter run,
exported via OTLP/gRPC. It is **off by default** (a no-op tracer, ~zero
overhead). Enable it with the operator flags `--tracing-enabled` and
`--tracing-otlp-endpoint=<host:port>` — or the equivalent `FATHOM_TRACING_*`
environment variables / config-file keys (`tracing.enabled`,
`tracing.otlp_endpoint`). With the Helm chart, set these through the rendered
config file (`config.enabled=true` with `config.data.tracing.*`).

Spans emitted:

- one per reconcile — `addoncheck.reconcile`, `healthcheck.reconcile`,
  `clusterhealth.reconcile` — tagged `fathom.kind` / `fathom.namespace` /
  `fathom.name`;
- one per adapter run — `<adapter>.run` (e.g. `coredns.run`) — tagged
  `fathom.adapter`, `fathom.outcome`, and `fathom.adapter.check_count`, nested
  under the AddonCheck reconcile span.

Full setup (endpoint, sampling ratio, TLS) is in
[Configuration → Tracing](../reference/configuration.md#tracing).

## 4. Alerting patterns

### Certificate expiry (the clean case)

`fathom_node_certificate_expiry_days` is a true numeric signal, so it alerts
naturally:

```yaml
groups:
  - name: fathom-node-certs
    rules:
      - alert: NodeCertificateExpiringSoon
        expr: min by (node, path) (fathom_node_certificate_expiry_days) <= 14
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Certificate on {{ $labels.node }} expires in <= 14 days"
          description: "{{ $labels.path }} on {{ $labels.node }}"
      - alert: NodeCertificateExpiringCritical
        expr: min by (node, path) (fathom_node_certificate_expiry_days) <= 3
        for: 10m
        labels:
          severity: critical
```

### Reconcile / adapter errors

Catch the operator failing to run checks at all:

```yaml
      - alert: FathomReconcileErrors
        expr: rate(fathom_reconcile_total{outcome="error"}[15m]) > 0
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "Fathom {{ $labels.kind }} reconciles are erroring"
```

### Add-on check results — read the status, not a metric

There is **no operator gauge for "current check result"** today. The result of
an `AddonCheck` / `HealthCheck` / `ClusterHealth` lives in the resource
**status**, not in a Prometheus series. `fathom_adapter_run_duration_seconds`
carries an `outcome` label, which tells you the distribution of recent run
outcomes — useful, but it is not the same as "is cert-manager currently `Fail`."

To alert on check *results*, expose the CRD status to Prometheus with
[kube-state-metrics custom resource state](https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/extend/customresourcestate-metrics.md),
mapping `status.lastResult` / `ClusterHealth.status.result` to a gauge, then
alert on that. (A first-class result metric is a natural future addition; for
now, status is the source of truth.)

## 5. Deployment gates

Because `ClusterHealth.status.result` is one machine-readable verdict, you can
gate a deploy or promotion on it — e.g. a CI/CD step that waits for a
`ClusterHealth` to read `Pass` before proceeding:

```sh
kubectl get clusterhealth platform \
  -o jsonpath='{.status.result}'
```

Treat anything other than `Pass`/`Skipped` as a stop. For a just-in-time verdict,
force a fresh `AddonCheck` run with a new `fathom.skaphos.io/run-now` annotation
value before reading the aggregate result.

## Reference

- [Configuration reference](../reference/configuration.md) — metrics, tracing,
  and every other flag.
- [Add-on checks](addon-checks.md) and
  [Node certificate checks](node-certificate-checks.md) — what each result means.
