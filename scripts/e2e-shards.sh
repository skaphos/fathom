#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Rillan AI LLC
# SPDX-License-Identifier: MIT
#
# Plan which e2e shards must run for a given diff (skaphos/fathom#178).
#
# Each shard is one kind cluster: the always-on `core` shard (Cilium CNI +
# cert-manager + external-secrets + CoreDNS, plus the operator-infrastructure
# specs) or one opt-in addon layered on top of the core stack. The shard name
# is passed to the e2e jobs verbatim as E2E_ADDONS, which scopes both the
# helmfile sync and the Ginkgo label filter (see test/utils).
#
# Usage:
#   scripts/e2e-shards.sh             # no base ref: plan the full matrix
#   scripts/e2e-shards.sh <base-ref>  # plan for `git diff <base-ref>...HEAD`
#
# Prints a JSON array of shard names on stdout. Policy:
#   - files scoped to one opt-in addon           -> that addon's shard
#   - files scoped to a core-tier addon          -> the core shard
#   - files that cannot affect operator runtime  -> no shard
#   - anything else (shared code)                -> the full matrix
#   - the core shard runs whenever any shard runs (always-on anchor)
# Unknown paths deliberately fall through to the full matrix: the planner
# fails open, never silently skipping coverage.
#
# OPT_IN_SHARDS must match OptInAddons() in test/utils/utils.go; the guard
# test scripts/e2e_shards_gate_test.go enforces it.

set -euo pipefail

OPT_IN_SHARDS="external-dns metrics-server envoy-gateway istio argocd node-local-dns azure-workload-identity"

emit_all() {
  printf '["core"'
  for shard in $OPT_IN_SHARDS; do printf ',"%s"' "$shard"; done
  printf ']\n'
}

# shard_for_file classifies one changed path: an opt-in shard name, "core",
# "" (irrelevant to e2e), or "all" (shared surface -> full matrix).
shard_for_file() {
  case "$1" in
    # --- Opt-in addons: adapter definition (+ its unit test), e2e spec,
    # sample, generated RBAC. First match wins.
    internal/adapter/declarative/externaldns*) echo external-dns ;;
    test/e2e/externaldns_test.go) echo external-dns ;;
    config/samples/*_external_dns.yaml) echo external-dns ;;
    config/rbac/addons/addon-external-dns.yaml) echo external-dns ;;

    internal/adapter/declarative/metricsserver*) echo metrics-server ;;
    test/e2e/metricsserver_test.go) echo metrics-server ;;
    config/samples/*_metrics_server.yaml) echo metrics-server ;;
    config/rbac/addons/addon-metrics-server.yaml) echo metrics-server ;;

    internal/adapter/declarative/envoygateway*) echo envoy-gateway ;;
    test/e2e/envoygateway_test.go) echo envoy-gateway ;;
    config/samples/*_envoy_gateway.yaml) echo envoy-gateway ;;
    config/rbac/addons/addon-envoy-gateway.yaml) echo envoy-gateway ;;

    internal/adapter/declarative/istio*) echo istio ;;
    test/e2e/istio_test.go) echo istio ;;
    config/samples/*_addoncheck_istio.yaml) echo istio ;;
    config/rbac/addons/addon-istio.yaml) echo istio ;;

    internal/adapter/declarative/argocd*) echo argocd ;;
    test/e2e/argocd_test.go) echo argocd ;;
    config/samples/*_addoncheck_argocd.yaml) echo argocd ;;
    config/rbac/addons/addon-argocd.yaml) echo argocd ;;
    internal/adapter/nodelocaldns/*) echo node-local-dns ;;
    test/e2e/nodelocaldns_test.go) echo node-local-dns ;;
    config/samples/*_node_local_dns.yaml) echo node-local-dns ;;
    config/rbac/addons/addon-node-local-dns.yaml) echo node-local-dns ;;
    internal/adapter/declarative/azureworkloadidentity*) echo azure-workload-identity ;;
    test/e2e/azureworkloadidentity_test.go) echo azure-workload-identity ;;
    config/samples/*_azure_workload_identity.yaml) echo azure-workload-identity ;;
    config/rbac/addons/addon-azure-workload-identity.yaml) echo azure-workload-identity ;;

    # --- Core-tier addons: their specs run in the core shard.
    internal/adapter/certmanager/*) echo core ;;
    internal/adapter/coredns/*) echo core ;;
    internal/adapter/declarative/cilium*) echo core ;;
    internal/adapter/declarative/externalsecrets*) echo core ;;
    test/e2e/certmanager_test.go) echo core ;;
    test/e2e/coredns_test.go) echo core ;;
    test/e2e/cilium_test.go) echo core ;;
    test/e2e/externalsecrets_test.go) echo core ;;
    test/e2e/nodecert_test.go) echo core ;;
    test/e2e/impersonation_test.go) echo core ;;
    test/e2e/addoncheck_refresh_test.go) echo core ;;
    config/samples/fathom_v1alpha1_addoncheck.yaml) echo core ;;
    config/samples/*_coredns.yaml) echo core ;;
    config/samples/*_cilium.yaml) echo core ;;
    config/samples/*_external_secrets.yaml) echo core ;;
    config/rbac/addons/addon-cert-manager.yaml) echo core ;;
    config/rbac/addons/addon-coredns.yaml) echo core ;;
    config/rbac/addons/addon-cilium.yaml) echo core ;;
    config/rbac/addons/addon-external-secrets.yaml) echo core ;;

    # --- Paths that cannot change operator runtime behaviour. Workflows other
    # than e2e.yml are listed before the catch-all below; e2e.yml itself and
    # this planner fall through to "all".
    .github/workflows/e2e.yml) echo all ;;
    scripts/e2e-shards.sh) echo all ;;
    docs/*) echo "" ;;
    *.md) echo "" ;;
    LICENSE | LICENSES/*) echo "" ;;
    REUSE.toml | .gitignore) echo "" ;;
    .github/*) echo "" ;;
    release-please-config.json | .release-please-manifest.json) echo "" ;;
    hack/*) echo "" ;;

    # --- Everything else is shared surface (api/, internal/, pkg/, cmd/,
    # config/, go.mod, Dockerfiles, Taskfile, shared e2e files, ...).
    *) echo all ;;
  esac
}

base="${1:-}"
if [ -z "$base" ]; then
  emit_all
  exit 0
fi

if ! files="$(git diff --name-only "${base}...HEAD")"; then
  echo "warning: git diff against ${base} failed; planning the full matrix" >&2
  emit_all
  exit 0
fi

want_core=false
selected=""
while IFS= read -r file; do
  [ -z "$file" ] && continue
  shard="$(shard_for_file "$file")"
  case "$shard" in
    "") continue ;;
    all)
      emit_all
      exit 0
      ;;
    core) want_core=true ;;
    *)
      want_core=true
      case " $selected " in
        *" $shard "*) ;;
        *) selected="$selected $shard" ;;
      esac
      ;;
  esac
done <<EOF
$files
EOF

if [ "$want_core" = false ]; then
  printf '[]\n'
  exit 0
fi

printf '["core"'
for shard in $OPT_IN_SHARDS; do
  case " $selected " in
    *" $shard "*) printf ',"%s"' "$shard" ;;
  esac
done
printf ']\n'
