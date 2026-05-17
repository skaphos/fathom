# fathom
Kubernetes platform integrity validation operator and CLI.

## AddonCheck Example

The built-in cert-manager adapter supports `system_health`, `issuer_health`, and
`certificate_health` families. `system_health` checks the core cert-manager
deployments, their matching pods, required cert-manager CRDs, and optionally the
webhook Service plus admission webhook configuration. `issuer_health` checks
`Issuer` and `ClusterIssuer` readiness. `certificate_health` checks Certificate
readiness, renewal timing, expiry thresholds, issuer references, and secret
linkage. Set the cert-manager name thresholds for distributions that rename the
controller, webhook, cainjector, Service, or webhook configuration objects.

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
        controllerName: "cert-manager"
        webhookName: "cert-manager-webhook"
        cainjectorName: "cert-manager-cainjector"
        webhookServiceName: "cert-manager-webhook"
        webhookConfigName: "cert-manager-webhook"
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

The built-in CoreDNS adapter supports `system_health` and `dns_resolution`
families. `system_health` checks the CoreDNS deployment, matching pods,
`kube-dns` Service, EndpointSlices, and optionally a node-count autoscaler
deployment. Set `deploymentName`, `serviceName`, and `autoscalerName` for
distributions such as RKE2 that rename CoreDNS objects. `dns_resolution`
launches a short-lived probe Pod per target in the AddonCheck's namespace
(per ADR-0003, so the resolver topology matches workloads rather than the
operator pod) and records the per-target outcome plus resolver latency.
Override `probeImage` if the default image tag is not pullable from your
cluster.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: coredns-health
spec:
  addonType: coredns
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      namespaces:
        - kube-system
      thresholds:
        deploymentName: "coredns"
        serviceName: "kube-dns"
        restartWarnCount: "3"
        autoscalerName: ""
    dns_resolution:
      enabled: true
      thresholds:
        targets: "kubernetes.default.svc.cluster.local"
        probeImage: "ghcr.io/skaphos/fathom-probe:v0.0.2"
```

The built-in External Secrets Operator adapter supports `system_health` and
`secret_sync` families. `system_health` checks controller deployments, pods, and
required ESO CRDs. `secret_sync` checks `ExternalSecret` readiness, stale refresh
state, failure reasons, store references, and target secret linkage.

```yaml
apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: external-secrets-health
spec:
  addonType: external-secrets
  interval: 5m
  timeout: 30s
  policy:
    system_health:
      enabled: true
      thresholds:
        restartWarnCount: "3"
    secret_sync:
      enabled: true
      thresholds:
        staleMinutes: "60"
```

## Probe Pods

Fathom has a shared lightweight probe-pod path under `internal/probe` plus a
tiny Go probe binary in `cmd/probe`. The probe image is built from
`Dockerfile.probe` and runs on `scratch` as a static binary with no shell or
package manager.

Supported probe modes are currently:

- `dns`: resolve a DNS name from inside the probe Pod
- `tcp-connect`: attempt a TCP connection to a target/port
- `tcp-listen`: run a TCP listener for peer connectivity tests

Build helpers:

```sh
go -C tools tool task probe-build
go -C tools tool task probe-docker-build PROBE_IMG=example.com/fathom-probe:latest
```

The shared pod builder applies the default hardening profile used for probe
workloads: no service account token, non-root UID, read-only root filesystem,
all capabilities dropped, no privilege escalation, runtime-default seccomp, and
small CPU/memory requests. It also supports pod anti-affinity so future network
checks can place client/server probe Pods on different nodes.
