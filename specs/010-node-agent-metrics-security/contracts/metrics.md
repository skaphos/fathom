<!-- SPDX-FileCopyrightText: 2026 Rillan AI LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Node metrics contract

## Access

Node-agent `/metrics` returns HTTP 404 in both modes, even when requested with a token. `/healthz` remains an unauthenticated liveness response without inventory. Existing listener flags and port allocation remain compatible.

Node detail metrics are exported from the operator endpoint using its existing HTTPS bearer-token authentication and `get /metrics` authorization. Shipped ServiceMonitor configuration applies. Agent SAs acquire no new permissions. Explicit administrator insecure-metrics overrides retain their documented meaning.

## Series

| Name | Labels | Value |
| --- | --- | --- |
| `fathom_node_certificate_expiry_days` | `namespace`, `check`, `node` | minimum known certificate days remaining, negative if expired |
| `fathom_node_health_check_result` | `namespace`, `check`, `node`, `type`, `path`, `result` | existing one-hot state for accepted agent items |
| `fathom_node_health_filesystem_free_percent` | `namespace`, `check`, `node`, `path`, `resource` | measured free percentage |

Certificate path, subject, and issuer are never labels. Health filesystem paths remain necessary to identify the measured filesystem, behind authorized operator access. Unknown certificate expiry and unavailable health percentages are omitted.

## Lifecycle and migration

Values follow accepted reports at reconciliation cadence, not live per-scrape scans. Only expected nodes contribute; partial reporting may yield a partial metric set. Existing check result/age/cadence metrics and alert rules continue unchanged and cover frozen aggregate evidence.

Remove agent PodMonitors/scrape annotations and use the operator ServiceMonitor. Replace certificate path grouping with `min by (namespace, check, node)` and remove path annotations. Use `max by (namespace, check, node, type, path, result)` or equivalent replica-aware aggregation for duplicate health samples where necessary. Keep the normal leader-election deployment pattern.

These are endpoint label names. Prometheus may rename a colliding `namespace` label to `exported_namespace` when the target already has a namespace and `honor_labels` is false, as in normal ServiceMonitor discovery. Preserve the resource namespace in aggregation: use `exported_namespace` in that setup, or explicitly configure label preservation before using the endpoint-name examples. See the [Prometheus scrape label contract](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config). Do not group distinct checks only by the operator's target namespace.

Upgrade both operator and node-agent images and wait for all managed DaemonSets to finish rollout. An old agent image override keeps the old vulnerable endpoint; the operator cannot remove routes from an old binary. Mixed-version rollout is not fully remediated until old pods terminate. Rollback restores the vulnerable agent endpoint and old scrape contract.
