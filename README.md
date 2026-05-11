# fathom
Kubernetes platform integrity validation operator and CLI.

## AddonCheck Example

The built-in cert-manager adapter supports `system_health`, `issuer_health`, and
`certificate_health` families. `system_health` checks the core cert-manager
deployments, their matching pods, required cert-manager CRDs, and optionally the
webhook Service plus admission webhook configuration. `issuer_health` checks
`Issuer` and `ClusterIssuer` readiness. `certificate_health` checks Certificate
readiness, renewal timing, expiry thresholds, issuer references, and secret
linkage.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: cert-manager-system-health
spec:
  addonType: cert-manager
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      thresholds:
        restartWarnCount: "3"
        webhookProbe: "true"
    issuer_health:
      enabled: true
      thresholds:
        kinds: "Issuer,ClusterIssuer"
    certificate_health:
      enabled: true
      thresholds:
        warnDays: "30"
        failDays: "7"
```
