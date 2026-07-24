# Contract: Check Metrics

The user-facing monitoring contract added by this feature. These names and
label sets are stable once released; changing them is a breaking change to
users' alert rules.

## `fathom_check_result`

One-hot current-result state set per check.

```text
fathom_check_result{kind="AddonCheck",name="cert-manager",namespace="fathom-system",result="Pass"} 1
fathom_check_result{kind="AddonCheck",name="cert-manager",namespace="fathom-system",result="Warn"} 0
fathom_check_result{kind="AddonCheck",name="cert-manager",namespace="fathom-system",result="Fail"} 0
fathom_check_result{kind="AddonCheck",name="cert-manager",namespace="fathom-system",result="Error"} 0
fathom_check_result{kind="AddonCheck",name="cert-manager",namespace="fathom-system",result="Skipped"} 0
fathom_check_result{kind="AddonCheck",name="cert-manager",namespace="fathom-system",result="Unknown"} 0
```

- `kind` ∈ {`AddonCheck`, `HealthCheck`, `ClusterHealth`, `NodeCertificateCheck`}
- `namespace` is `""` for `ClusterHealth` (cluster-scoped)
- `result` covers the full CRD vocabulary; exactly one series is `1`
- Before the first completed evaluation, `result="Unknown"` is `1`
- All six series exist iff the check resource exists; deletion removes them

## `fathom_check_last_run_timestamp_seconds`

```text
fathom_check_last_run_timestamp_seconds{kind="AddonCheck",name="cert-manager",namespace="fathom-system"} 1.784162e+09
```

- Unix seconds of the freshest completed evaluation backing the check's
  current result. For the executing kinds (AddonCheck, NodeCertificateCheck)
  that is their own last run; for the wrapper kinds it follows the evidence
  chain — HealthCheck carries its mirrored target's run time, ClusterHealth
  the freshest of its children — so a stale source reads as a stale wrapper
  (research R2, implementation amendment)
- `0` until the first evaluation completes ("never ran")
- Exists iff the check resource exists; deletion removes it

## Canonical alert rules

Shipped in `docs/guides/monitoring.md` and as the opt-in
`config/components/prometheus-rule/` component:

```yaml
- alert: FathomCheckFailing
  expr: fathom_check_result{result=~"Fail|Error"} == 1
  for: 10m
- alert: FathomCheckStale
  expr: time() - fathom_check_last_run_timestamp_seconds > 900
  for: 10m
```

The staleness threshold (900s) is the shipped default — users tune it to
their check intervals; the sentinel `0` makes never-run checks fire it
immediately after the `for` window.

## Non-guarantees

- Series survive only as long as the operator process observes the check;
  scrape continuity across operator restarts is not promised (standard
  Prometheus gauge semantics apply).
- No free-text labels (messages, reasons, versions) will ever be added to
  these series (FR-010); details belong to status, events, and HealthReport.
