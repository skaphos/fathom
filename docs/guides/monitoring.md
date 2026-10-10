<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# Monitoring & Alerting

Fathom turns platform integrity into Kubernetes resource status. This guide
covers the three ways a platform team consumes that: reading status with
`kubectl`, scraping Prometheus metrics, and tracing reconciles. It closes with
example alerting rules and deployment gates.

Fathom emits signal; it does not fire alerts. What pages whom, at which
severity and after how long, differs between every platform that adopts it, so
that policy is yours. What Fathom promises is the **metric surface** — the
names, labels and semantics in [The metric contract](#the-metric-contract) —
and it holds that surface to an explicit
[stability promise](#stability-promise). The alert rules in this guide, and
the opt-in `PrometheusRule` component, are worked examples you are expected to
adapt.

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

### Wiring up Prometheus (Helm)

The chart can create everything a Prometheus Operator install needs: the
metrics `Service`, a `ServiceMonitor`, the RBAC that lets your Prometheus
scrape, and the sample alert rules:

```yaml
# values.yaml
metrics:
  serviceMonitor:
    enabled: true
    labels:
      release: kube-prometheus-stack   # your Prometheus serviceMonitorSelector
  reader:
    subjects:                          # who may GET /metrics
      - kind: ServiceAccount
        name: kube-prometheus-stack-prometheus
        namespace: monitoring
  prometheusRule:
    enabled: true                      # renders the SAMPLE rules
    labels:
      release: kube-prometheus-stack   # your Prometheus ruleSelector
```

- `metrics.service.enabled` (default `true`) exposes a ClusterIP Service on
  port `8443`.
- `metrics.serviceMonitor.enabled` (default `false`) creates the
  `ServiceMonitor`. Tune `interval`, `scrapeTimeout`, `labels`, and `tlsConfig`.
  The default `tlsConfig.insecureSkipVerify: true` trusts the self-signed
  serving cert; for a CA-signed metrics cert set `caFile` / `serverName` and
  flip it to `false`.
- `metrics.reader.subjects` (default empty) binds each listed identity to the
  chart's `metrics-reader` ClusterRole, which grants `get` on `/metrics` and
  nothing else. Secure metrics refuse any scrape whose token lacks it, so
  list your scraper's ServiceAccount here (or bind the role yourself).
- `metrics.prometheusRule.enabled` (default `false`) creates a
  `PrometheusRule`. By default it carries the **sample** rules from
  [Example alerting rules](#4-example-alerting-rules) — review them first. Set
  `metrics.prometheusRule.groups` to render your own rule groups instead; they
  replace the sample rather than adding to it. Requires the prometheus-operator
  CRDs.

If you deploy via kustomize instead, the `ServiceMonitor` is an opt-in overlay
under `config/components/prometheus` and the sample rules are
`config/components/prometheus-rule`. The chart's rules are synced from that
component (`task helm:sync`), so both install paths ship the same sample.

### Other monitoring backends

Fathom only exposes Prometheus-format metrics over HTTPS with a bearer-token
check; any backend that can scrape that can consume the
[metric contract](#the-metric-contract). The chart has presets for two common
ones under `metrics.integrations`. Each wires **scraping only** — alerting is
configured in the backend itself — and both are off by default.

#### Dynatrace

```yaml
metrics:
  integrations:
    dynatrace:
      enabled: true
      activeGateServiceAccount:     # defaults shown
        name: dynatrace-activegate
        namespace: dynatrace
      caConfigMap:                  # CA of a CA-signed metrics certificate
        name: fathom-metrics-ca
        key: ca.crt
  certSecretName: fathom-metrics-cert   # e.g. issued by cert-manager
  certPath: /tmp/k8s-metrics-server/metrics-certs
```

The preset annotates the metrics `Service` with the `metrics.dynatrace.com/*`
keys Dynatrace's Prometheus scraping reads (`scrape`, `port` — the container
port, as Dynatrace requires on a Service — `path`, `secure`), and with
`http.auth: builtin:default`, which makes the ActiveGate send **its own
ServiceAccount token**. It then binds that ServiceAccount to the
`metrics-reader` role. Anything you set in `metrics.service.annotations`
overrides the preset key by key — for example `metrics.dynatrace.com/filter` to
ingest only some metrics.

Because that token is the ActiveGate's own — usually with broad cluster read —
**the serving certificate must be verified**. The operator's default
certificate is self-signed and regenerated on every start, so it cannot be
pinned: serve a CA-signed metrics certificate (`metrics.certSecretName`) and
point `caConfigMap` at a ConfigMap holding its CA. The preset then sets
`metrics.dynatrace.com/tls.ca.crt` and grants the ActiveGate `get` on that one
ConfigMap, which Dynatrace needs and does not grant by default. With secure
metrics and no `caConfigMap`, the chart **refuses to render** unless you
explicitly accept unverified TLS with
`metrics.integrations.dynatrace.insecureSkipVerify: true`.

- Authenticated scraping needs an **in-cluster ActiveGate that monitors the
  local Kubernetes API**; an ActiveGate outside the cluster cannot scrape this
  endpoint.
- Dynatrace now recommends its OpenTelemetry Collector (Target Allocator) for
  new Prometheus ingestion and for large estates. That path discovers the
  chart's `ServiceMonitor`: enable `metrics.serviceMonitor` and list the
  Collector's ServiceAccount in `metrics.reader.subjects` instead.
- Prometheus labels become Dynatrace dimensions. Confirm the ingested metric
  keys in your tenant before writing alerts against them.

#### Sumo Logic

```yaml
metrics:
  integrations:
    sumologic:
      enabled: true
      releaseName: collection   # the Sumo Logic Kubernetes Collection release
```

The Sumo Logic Kubernetes Collection (v4 and later, OpenTelemetry) discovers
`ServiceMonitor`s through its Target Allocator, selecting `release: <its release
name>`. The preset renders the chart's `ServiceMonitor` — even with
`metrics.serviceMonitor.enabled=false` — with that label. The collection's
metrics collector role already allows `get` on `/metrics`, so no extra RBAC is
needed. Its pod-annotation (`prometheus.io/scrape`) path is HTTP-only and
unauthenticated, so it cannot scrape Fathom; use the preset.

- By default the collection **drops histogram and summary metrics**
  (`dropHistogramBuckets` with `allowHistogramRegex: "^$"`). To keep
  `fathom_reconcile_duration_seconds` and `fathom_adapter_run_duration_seconds`,
  widen `allowHistogramRegex` (for example `^fathom_.*`). The check gauges are
  unaffected.
- Recent OpenTelemetry Operator releases stop honouring file-based
  `ServiceMonitor` credentials such as `bearerTokenFile` by default. If your
  collection's operator does, the scrape is refused with `401`; add a scrape job
  through `sumologic.metrics.collector.otelcol.config.merge` instead, with
  `scheme: https`, `authorization.credentials_file:
  /var/run/secrets/kubernetes.io/serviceaccount/token` and
  `tls_config.insecure_skip_verify: true`.

#### Anything else

Any scraper works if it can send a bearer token over HTTPS: point it at the
metrics `Service` (port `8443`, path `/metrics`), let it skip verification of
the self-signed certificate (or mount a CA-signed one via
`metrics.certSecretName`), and list its identity in `metrics.reader.subjects`.
Annotation conventions differ — use `metrics.service.annotations` to set the
ones your agent reads. The generic `prometheus.io/*` annotations carry no
scheme or credentials in most agents, so on their own they are not enough.

### The metric contract

Registered with the controller-runtime registry (so the built-in
controller-runtime and Go metrics are exposed alongside them). Every `fathom_*`
metric in this guide is part of the contract described under
[Stability promise](#stability-promise):

| Metric | Type | Labels | Use |
| --- | --- | --- | --- |
| `fathom_check_result` | gauge | `kind`, `name`, `namespace`, `result` | **Current result of every check**, one-hot: one series per result value (`Pass`/`Warn`/`Fail`/`Error`/`Skipped`/`Unknown`), exactly one of them `1`. The alerting signal for "is this check failing right now". |
| `fathom_check_last_run_timestamp_seconds` | gauge | `kind`, `name`, `namespace` | Unix time of the most recent completed evaluation backing the check's current result. The staleness signal — see [Example alerting rules](#4-example-alerting-rules). |
| `fathom_check_interval_seconds` | gauge | `kind`, `name`, `namespace` | The cadence a check is currently expected to run at, after per-resource override and floor clamping. For `NodeHealthCheck` this is the **capped agent cadence** (`min(spec.interval, 5m)`), not `spec.interval`: a frozen verdict must read as stale within minutes even on a daily check. Join it against the last-run timestamp to express staleness relative to cadence instead of a fixed threshold. **Absent** — not zero — when the cadence cannot be resolved. |
| `fathom_dnscheck_target_result` | gauge | `namespace`, `check`, `name`, `record_type`, `resolver`, `result` | **`fathom_check_result` one level down**, for `DNSCheck` only: one-hot per (target, vantage point) pair, so you can alert on the single name that broke rather than on the check as a whole. See [Per-target DNS results](#per-target-dns-results) for the cardinality budget. |
| `fathom_reconcile_total` | counter | `kind`, `outcome` | Reconcile volume and error rate per resource kind. |
| `fathom_reconcile_duration_seconds` | histogram | `kind` | Reconcile latency per kind. |
| `fathom_adapter_run_duration_seconds` | histogram | `adapter`, `family`, `outcome` | How long adapter runs take, and their outcome distribution. |
| `fathom_adapter_registered` | gauge | `adapter` | `1` for each adapter registered at startup — confirms the operator loaded the adapters you expect. |

The check gauges cover every check kind (`AddonCheck`, `DNSCheck`,
`NodeCertificateCheck`, `NodeHealthCheck`, `HealthCheck`, `ClusterHealth`;
`ClusterHealth` is cluster-scoped, so its `namespace` label is empty). Series exist from the moment the operator
first observes a check — reporting `result="Unknown"` and last-run `0` until
the first evaluation completes — and are removed when the check is deleted.
For the wrapper kinds the last-run timestamp follows the staleness of the
evidence behind the verdict: a `HealthCheck` carries its mirrored target's
last run time, and a `ClusterHealth` the **stalest** of its children — so a
stale source reads as a stale wrapper, which is what you want to alert on.
Taking the stalest is what makes that guarantee hold: an aggregate folds the
*worst* verdict across its children, so if it published the freshest
observation instead, one healthy child would make a frozen sibling's `Fail`
look perfectly current and no staleness alert would ever fire (#277). A
`ClusterHealth` whose selector matches a check that has never been evaluated
reports last-run `0`, because an unevaluated child is the strongest staleness
signal there is.
`status.children` on a `ClusterHealth` is capped at 100 entries. The cap bounds
only what the aggregate *reports*: `result` and `observedAt` are computed across
every selected check before truncation, so a large aggregate stays correct and
its status write never fails. `matchedCount` remains the full pre-truncation
total, so `matchedCount > len(children)` is how you detect truncation.
Truncation keeps the entries you need — worst verdict first, then stalest — so
the failing or frozen child that explains the roll-up is never the one dropped.

A `ClusterHealth`'s published interval is the **slowest** of its contributing
checks, because a roll-up can only be as current as its least frequently
refreshed contributor. Together with the stalest-observation rule above, that is
what lets one alert cover a mixed-cadence aggregate without false positives: a
healthy hourly child no longer drags a five-minute aggregate into permanent
staleness. Checks whose cadence cannot be resolved publish no interval series, so the
vector join in the first clause drops them — which is why the sample rule
carries a second `== 0` clause. Without it a `ClusterHealth` whose selector
matches nothing would silently stop alerting, and a typo'd selector is exactly
the mistake that rule exists to catch.

Label cardinality is bounded by design: one series set per check resource
(plus the schema-capped per-target and per-item sets below), and label values
never carry observed free text.

### Stability promise

The metric surface is Fathom's alerting contract: you write rules against it,
so it changes only deliberately.

**Covered** — every `fathom_*` metric documented in this guide:

- metric names, types and units;
- label keys, and the documented values of enumerated labels (`kind`,
  `result`, `type`, `resource`, `record_type`, and `outcome`: `success` /
  `error` on `fathom_reconcile_total`, the adapter result
  `Pass` / `Warn` / `Fail` / `Error` on `fathom_adapter_run_duration_seconds`);
- the documented semantics: one-hot result sets with exactly one series at `1`,
  the `0` "never ran" last-run sentinel, an interval that is **absent** rather
  than zero when it cannot be resolved, the wrapper-kind staleness rules
  (`HealthCheck` follows its target, `ClusterHealth` its stalest child and
  slowest cadence), and series that live and die with their resource;
- where label values come from — only three bounded sources:
  - the specs of your own resources: resource names, `DNSCheck` target names,
    `NodeHealthCheck` item `path`s, and a runtime `AddonDefinition`'s addon
    type and family names (the `adapter` / `family` labels), each capped by the
    CRD schema;
  - identifiers Fathom defines: node names, built-in adapter and family names
    (`adapter`, `family`), and the enumerated values above;
  - nothing else. Values Fathom *observes* (messages, reasons, versions,
    discovered certificate paths, subjects, issuers) stay in status, Events and
    `HealthReport`, never in a label.

**Breaking** — made only in a release whose notes flag it as a breaking change,
never silently and never in a patch release:

- renaming or removing a metric or a label;
- changing a metric's type or unit;
- changing what a value means (for example #307, which switched a
  `ClusterHealth`'s last-run timestamp from its freshest to its stalest child,
  shipped as a breaking change);
- adding a label to an existing metric **by default** — it changes series
  identity and breaks `on(...)` joins and recording rules;
- removing a documented value of an enumerated label.

**Not breaking:**

- new metrics;
- opt-in labels that are off unless you enable them;
- new values of an enumerated label that come with a new check kind, item type
  or result, and new built-in adapter or family names — match the values you
  care about (`result=~"Fail|Error"`) rather than assuming the set is closed;
- histogram bucket boundaries;
- the sample alert rules, the `ServiceMonitor` and Helm scrape defaults, and the
  controller-runtime and Go runtime metrics, which belong to their upstreams.

Series continuity across an operator restart is not promised: standard
Prometheus gauge semantics apply.

### Per-target DNS results

`fathom_dnscheck_target_result` is the only metric that goes below the check
level. A `DNSCheck` can cover sixteen names from three vantage points, and
"the check is failing" does not tell an operator *which name* — so this gauge
carries one one-hot set per **(target, vantage point) pair**.

Its ceiling is fixed by the CRD schema, not by runtime behaviour:

```text
16 targets  ×  3 vantage points  ×  6 result values  =  288 series per check
```

That number is computable from a check's own spec **before it is applied**, so
the monitoring cost of a `DNSCheck` is knowable in advance. A realistic check
covering three names from one vantage point costs 18 series. Raising a schema
cap would raise this ceiling and is treated as a cardinality change, not merely
a limits change.

Series are rebuilt on every run rather than accumulated: a target removed from
the spec loses its series on the next evaluation instead of freezing at its last
verdict, and deleting the check withdraws everything it was asserting. Alert on
the pair, not just the check:

```yaml
- alert: FathomDNSTargetFailing
  expr: fathom_dnscheck_target_result{result=~"Fail|Error"} == 1
  for: 10m
  annotations:
    summary: >-
      {{ $labels.name }} ({{ $labels.record_type }}) is {{ $labels.result }}
      from {{ $labels.resolver }} in {{ $labels.namespace }}/{{ $labels.check }}
```

A `result="Unknown"` series means the run did not reach that pair before its
bound elapsed — the check's `Complete` condition names how many. That is a
sizing problem rather than a DNS problem; see
[DNSCheck fan-out](../reference/configuration.md#dnscheck-fan-out).

### Node certificate metrics

> Applies only to builds that include the `NodeCertificateCheck` kind — see
> [Node certificate checks → Availability](node-certificate-checks.md#availability).

The operator exports the earliest known expiry from each accepted
`NodeCertificateCheck` report:

| Metric | Type | Labels | Use |
| --- | --- | --- | --- |
| `fathom_node_certificate_expiry_days` | gauge | `namespace`, `check`, `node` | **Minimum known days until certificate expiry for the check and node.** Negative means expired. Paths, subjects, and issuers remain in the `HealthReport` and never become metric labels. A report with no known expiry emits no sample. |

Scrape this gauge through the same operator `ServiceMonitor` described above.
Remove any node-agent `PodMonitor` or scrape annotations: agents retain their
listener for `/healthz`, but `/metrics` returns 404. The ServiceMonitor sends
its ServiceAccount bearer token over HTTPS; that identity must be bound to the
shipped metrics-reader role.

### Node-health metrics

> Applies only to builds that include the `NodeHealthCheck` kind — see
> [Node health checks → Availability](node-health-checks.md#availability).

The operator exports two gauges from accepted `NodeHealthCheck` agent reports:

| Metric | Type | Labels | Use |
| --- | --- | --- | --- |
| `fathom_node_health_check_result` | gauge (one-hot) | `namespace`, `check`, `node`, `type`, `path`, `result` | **Per-item result on each node, for agent-evaluated types only** (`DiskHeadroom`, `InodeHeadroom`, `KubeletHealthz`, `ContainerRuntime`). `NodeCondition` is graded by the operator and remains visible in check status and the `HealthReport`. Exactly one `result` series per item is 1. Series are bounded by the schema: 16 items × 6 results per node. |
| `fathom_node_health_filesystem_free_percent` | gauge | `namespace`, `check`, `node`, `path`, `resource` (`bytes` \| `inodes`) | **The measured headroom behind a `DiskHeadroom`/`InodeHeadroom` verdict**, so you can graph the trend and alert ahead of the threshold. |

```yaml
      - alert: NodeHealthCheckFailing
        expr: fathom_node_health_check_result{result="Fail"} == 1
        for: 10m
        labels: {severity: warning}
        annotations:
          summary: >-
            {{ $labels.type }} {{ $labels.path }} is failing on {{ $labels.node }}
      - alert: NodeFilesystemHeadroomLow
        expr: fathom_node_health_filesystem_free_percent{resource="bytes"} < 15
        for: 15m
        labels: {severity: warning}
```

These families use the operator `ServiceMonitor`; do not scrape agents. A
`KubeletHealthz` item still puts the agent on the **host network**, and its
liveness listener still binds on a per-check host port (20000–22767 by default;
the `AgentPrivileged` condition names it). That port returns 404 for `/metrics`.

Node-detail values follow report and reconcile cadence rather than a live
scrape. Only fresh, authentic reports for expected nodes contribute. During a
partial fleet window, the accepted subset is exposed without manufacturing
healthy samples for missing nodes, while the last complete aggregate status and
`HealthReport` history remain intact. Reconciliation withdraws a check's old
detail series before rebuilding them, so a scrape can briefly see them absent;
pause, deletion, rejected or expired reports, removed nodes, and removed items
withdraw their series without erasing another check's samples.

With multiple operator replicas, use the normal leader-election deployment and
replica-aware queries such as
`max by (namespace, check, node, type, path, result) (...)`. The existing
check-result and last-run metrics remain the way to detect a stalled controller
or frozen evidence.

The label names above are endpoint labels. Prometheus commonly adds its own
target `namespace` label and, with `honor_labels: false`, renames Fathom's check
namespace to `exported_namespace`. Use that label in grouping, or explicitly
configure label preservation. Never group checks only by the operator target's
namespace, because same-named checks in different namespaces would collide.

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

## 4. Example alerting rules

> **These are starting points, not Fathom policy.** Fathom does not decide what
> pages you. Every rule below is a composition over the
> [metric contract](#the-metric-contract): copy it into your own rule set and
> tune the severity, the `for` window and any threshold to your platform and
> on-call. Rule names and expressions in this section and in the sample
> component may change in any release; the metrics they read change only under
> the [stability promise](#stability-promise).

### Certificate expiry (the clean case)

`fathom_node_certificate_expiry_days` is a true numeric signal, so it alerts
naturally:

```yaml
groups:
  - name: fathom-node-certs
    rules:
      - alert: NodeCertificateExpiringSoon
        expr: min by (namespace, check, node) (fathom_node_certificate_expiry_days) <= 14
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Certificate for {{ $labels.namespace }}/{{ $labels.check }} on {{ $labels.node }} expires in <= 14 days"
      - alert: NodeCertificateExpiringCritical
        expr: min by (namespace, check, node) (fathom_node_certificate_expiry_days) <= 3
        for: 10m
        labels:
          severity: critical
```

If Prometheus has renamed the endpoint's `namespace` label, substitute
`exported_namespace` in these expressions and annotations. The metric already
contains the minimum for each check and node; the `min by` also collapses
duplicate samples during a multi-replica rollout.

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

### Check results and staleness

`fathom_check_result` and `fathom_check_last_run_timestamp_seconds` make both
failing checks and *silently stale* checks first-class alerts. The staleness
rule is the one that catches what nothing else does: a wedged operator, a
paused check, or a selector matching nothing all leave the last recorded
result frozen — the metrics stop advancing even though status still reads
`Pass`:

```yaml
groups:
  - name: fathom-checks
    rules:
      - alert: FathomCheckFailing
        expr: fathom_check_result{result=~"Fail|Error"} == 1
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "Fathom check {{ $labels.kind }}/{{ $labels.name }} reports {{ $labels.result }}"
      - alert: FathomCheckStale
        # Cadence-relative: each check is compared against its OWN published
        # interval, so one rule is correct for a 1m DNSCheck and a 1h
        # NodeCertificateCheck alike. The 3 is the overdue allowance — how many
        # consecutive missed runs before we care. The 0 "never ran" sentinel
        # makes a check that never executed fire this alert too, with no
        # absent() gymnastics.
        expr: >-
          (time() - fathom_check_last_run_timestamp_seconds
          > 3 * fathom_check_interval_seconds)
          or (fathom_check_last_run_timestamp_seconds == 0)
        for: 10m
        labels:
          severity: warning
```

The same two rules are available as an opt-in **sample** kustomize component,
`config/components/prometheus-rule` (requires the prometheus-operator CRDs;
enable it next to the `prometheus` ServiceMonitor component in
`config/default/kustomization.yaml`). It exists so you can see the rules
working end to end; treat it as a template, review it before enabling it, and
prefer copying the rules into a rule set you own. The sample is build-validated
in CI (`task verify-alert-rules`), and a gate test keeps the staleness rule
cadence-relative; it is not exercised by promtool-style rule unit tests. The
Helm chart renders the same sample with `metrics.prometheusRule.enabled=true`
(see [Wiring up Prometheus (Helm)](#wiring-up-prometheus-helm)).

### The same rules in other backends

Dynatrace and Sumo Logic define alerts in the backend, usually as code. These
are the two sample rules translated, as starting points only: **they have not
been run against a live tenant**, so check the metric keys, dimension names and
query syntax in yours before relying on them.

Dynatrace metric event (Terraform, `dynatrace-oss/dynatrace` provider) for
`FathomCheckFailing`:

```hcl
resource "dynatrace_metric_events" "fathom_check_failing" {
  enabled = true
  summary = "Fathom check failing"
  event_template {
    title       = "Fathom check failing"
    description = "{dims}"
    event_type  = "CUSTOM_ALERT"
  }
  model_properties {
    type               = "STATIC_THRESHOLD"
    alert_condition    = "ABOVE"
    threshold          = 0.5
    samples            = 10 # one-minute samples: "for 10m"
    violating_samples  = 10
    dealerting_samples = 10
    alert_on_no_data   = false
  }
  query_definition {
    type            = "METRIC_SELECTOR"
    metric_selector = "fathom_check_result:filter(or(eq(\"result\",\"Fail\"),eq(\"result\",\"Error\"))):splitBy(\"namespace\",\"name\"):max"
  }
}
```

Sumo Logic monitor (Terraform, `sumologic_monitor`) for the same condition:

```hcl
resource "sumologic_monitor" "fathom_check_failing" {
  name         = "Fathom check failing"
  type         = "MonitorsLibraryMonitor"
  monitor_type = "Metrics"
  queries {
    row_id = "A"
    query  = "metric=fathom_check_result (result=Fail OR result=Error) | max by namespace, name"
  }
  trigger_conditions {
    metrics_static_condition {
      critical {
        time_range      = "10m"
        occurrence_type = "Always"
        alert {
          threshold      = 1
          threshold_type = "GreaterThanOrEqual"
        }
        resolution {
          threshold      = 1
          threshold_type = "LessThan"
        }
      }
    }
  }
}
```

The staleness rule translates the same way: alert when
`time() - fathom_check_last_run_timestamp_seconds` exceeds three times
`fathom_check_interval_seconds`, or the last-run timestamp is `0`. How to
express that arithmetic between two metrics differs per backend.

Status remains the source of truth the metric is derived from — for a
just-in-time verdict or a deploy gate, keep reading status
([section 5](#5-deployment-gates)). Result *history* is `HealthReport`'s job,
and each check resource also records Kubernetes Events on result transitions
and operational failures (`kubectl describe addoncheck <name>`), so the
recent story is visible without operator logs.

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
