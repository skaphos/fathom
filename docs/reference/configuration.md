<!--
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
-->
# Configuration Reference

Fathom's operator process resolves its runtime configuration with
[cobra](https://github.com/spf13/cobra) + [viper](https://github.com/spf13/viper).
This page documents every option, where it can be set, and its default. The
authoritative source is the `bindings()` table in
[`internal/app/options.go`](../../internal/app/options.go); this page is kept in
sync with it.

## Precedence

For any single option, the highest-priority source that sets it wins:

```
command-line flag  >  environment variable  >  config file  >  built-in default
```

- Flags keep their kubebuilder names (e.g. `--metrics-bind-address`) for
  backwards compatibility with existing deployment manifests.
- A flag only overrides lower-priority sources when it is actually set on the
  command line — viper binds via each flag's `Changed` state, so an unset flag
  falls through to env / config / default.

## Environment Variables

Environment variables are read with the prefix `FATHOM` and the viper key's
dots replaced by underscores, then upper-cased. The rule is:

```
FATHOM_<VIPER_KEY with "." replaced by "_", upper-cased>
```

For example the viper key `metrics.bind_address` is read from
`FATHOM_METRICS_BIND_ADDRESS`. This is configured in `Load` via
`SetEnvPrefix("FATHOM")` and `SetEnvKeyReplacer(".", "_")` with `AutomaticEnv`.

## Config File

- Default path: `/etc/fathom/config.yaml` (`DefaultConfigPath`). Operators
  typically mount this as a ConfigMap.
- Format: any format viper understands (YAML/JSON/TOML); keys mirror the viper
  keys below, nested by the `.` separators (e.g. `metrics.bind_address` becomes
  a `bind_address` key under a `metrics` map).
- **Missing-file behavior:** a missing file at the *default* path is treated as
  "no config" and ignored. A missing file at a path passed explicitly via
  `--config` is a **hard error** — the operator refuses to start. This is
  enforced in `Load` (the `configExplicit` branch).

Example `config.yaml`:

```yaml
metrics:
  bind_address: ":8443"
  secure: true
leader_elect: true
probe_image: "ghcr.io/skaphos/fathom-probe:v0.0.2"
```

## Options

Every row below is one entry in `bindings()`. The env var column is derived from
the viper key by the rule above.

| Flag | Viper key | Env var | Default | Meaning |
| --- | --- | --- | --- | --- |
| `--metrics-bind-address` | `metrics.bind_address` | `FATHOM_METRICS_BIND_ADDRESS` | `0` | Address the metrics endpoint binds to (`:8443` HTTPS, `:8080` HTTP). `0` disables the metrics server entirely. |
| `--metrics-secure` | `metrics.secure` | `FATHOM_METRICS_SECURE` | `true` | Serve metrics over HTTPS with the authn/authz filter. `--metrics-secure=false` serves plaintext HTTP. |
| `--metrics-allow-insecure` | `metrics.allow_insecure` | `FATHOM_METRICS_ALLOW_INSECURE` | `false` | Explicit opt-in to expose plaintext metrics on a cluster-routable port (i.e. `--metrics-secure=false` with a non-`0` bind address). Intended for service-mesh-fronted deployments; `Validate` rejects insecure-on-network otherwise. |
| `--metrics-cert-path` | `metrics.cert_path` | `FATHOM_METRICS_CERT_PATH` | _(empty)_ | Directory containing the metrics server certificate. |
| `--metrics-cert-name` | `metrics.cert_name` | `FATHOM_METRICS_CERT_NAME` | `tls.crt` | Metrics server certificate file name. |
| `--metrics-cert-key` | `metrics.cert_key` | `FATHOM_METRICS_CERT_KEY` | `tls.key` | Metrics server key file name. |
| `--webhook-cert-path` | `webhook.cert_path` | `FATHOM_WEBHOOK_CERT_PATH` | _(empty)_ | Directory containing the webhook certificate. |
| `--webhook-cert-name` | `webhook.cert_name` | `FATHOM_WEBHOOK_CERT_NAME` | `tls.crt` | Webhook certificate file name. |
| `--webhook-cert-key` | `webhook.cert_key` | `FATHOM_WEBHOOK_CERT_KEY` | `tls.key` | Webhook key file name. |
| `--health-probe-bind-address` | `health_probe_bind_address` | `FATHOM_HEALTH_PROBE_BIND_ADDRESS` | `:8081` | Address the health probe endpoint (`/healthz`, `/readyz`) binds to. `0` disables it. |
| `--leader-elect` | `leader_elect` | `FATHOM_LEADER_ELECT` | `false` | Enable leader election so only one manager replica is active. |
| `--leader-election-id` | `leader_election_id` | `FATHOM_LEADER_ELECTION_ID` | `2d3dbc4f.skaphos.io` | Name of the lease resource used for leader election (must be a DNS-1123 subdomain). |
| `--enable-http2` | `enable_http2` | `FATHOM_ENABLE_HTTP2` | `false` | Enable HTTP/2 for the metrics and webhook servers. Off by default to mitigate CVE-2023-44487 / CVE-2023-39325. |
| `--probe-image` | `probe_image` | `FATHOM_PROBE_IMAGE` | `ghcr.io/skaphos/fathom-probe:v0.0.2` | Container image used by adapters that launch probe pods. See [Probe image default](#probe-image-default). |

> Note: the zap logging flags (e.g. `--zap-log-level`, `--zap-devel`) are also
> registered, but they are bound directly to `zap.Options` via the stdlib `flag`
> package and are **not** routed through viper — they have no env var or config
> key. See the `Zap` field comment in `options.go`.

## Validation

`Options.Validate` runs before the manager starts and rejects internally
inconsistent configuration, including:

- Insecure metrics on a cluster-routable port without `metrics.allow_insecure`
  (SKA-287).
- A `*.cert_path` set without the corresponding `cert_name` / `cert_key`.
- A `metrics.bind_address` or `health_probe_bind_address` that is not a valid
  `host:port` (use `0` to disable; SKA-299).
- A `leader_election_id` that is not a valid DNS-1123 subdomain.

## Probe Image Default

`--probe-image` (default `ghcr.io/skaphos/fathom-probe:v0.0.2`,
`DefaultProbeImage` in `options.go`) is the cluster-wide default container image
for adapter probe pods. It is forwarded into each adapter run as
`adapter.Request.ProbeImage`. Adapters resolve the actual image with this
precedence:

```
per-AddonCheck probeImage threshold  >  --probe-image (Request.ProbeImage)  >  adapter-hardcoded fallback
```

The hardcoded fallback (also `ghcr.io/skaphos/fathom-probe:v0.0.2`) lives in the
CoreDNS adapter so a probe-using check still has an image when neither the
operator default nor a per-check override is set. Operators running a private
GHCR mirror set `--probe-image` once instead of on every `AddonCheck`.

## Adding a New Option

Extend the `Options` struct and add one row to the `bindings()` table in
`internal/app/options.go`. The flag, viper key, env var, and config-file key are
derived from that single entry, so they stay in sync automatically. Add a
default in `DefaultOptions()` and, if the option can be invalid, a check in
`Validate()`. Document the new row in the table above.
