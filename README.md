# fathom
Kubernetes platform integrity validation operator and CLI.

## AddonCheck Example

The built-in cert-manager adapter supports the `system_health` family. It checks
the core cert-manager deployments, their matching pods, required cert-manager
CRDs, and optionally the webhook Service plus admission webhook configuration.

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
```
