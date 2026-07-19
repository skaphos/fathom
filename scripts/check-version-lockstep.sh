#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Skaphos
# SPDX-License-Identifier: MIT
#
# Verify the operator<->probe/node-agent version lockstep (SKA-579).
#
# The probe/node-agent image tags the operator compiles in, the Helm chart's
# version/appVersion, the e2e image tags, and the CoreDNS sample probeImage pin
# must all equal the released version recorded in .release-please-manifest.json.
# release-please bumps every annotated site (x-release-please-version) in the
# release PR — see the "extra-files" list in release-please-config.json. This
# gate fails the build if any of those sites drifts out of lockstep, so the
# "update the constants before merging the release PR" contract can no longer
# be silently skipped (it already failed for 0.3.0 and 0.3.1, and a stale sample
# pin broke kind e2e after 0.4.1).
#
# Usage:
#   scripts/check-version-lockstep.sh
#
# Environment:
#   LOCKSTEP_VERSION  Override the expected version (tests only). Defaults to the
#                     root package version in .release-please-manifest.json.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

manifest=".release-please-manifest.json"

# Read the root ("."), package version from the manifest without a jq
# dependency (jq is not guaranteed on contributor machines). LOCKSTEP_VERSION
# overrides it (tests) and skips the manifest read entirely; otherwise the
# manifest must exist, so a partial checkout or a pre-release-please tree fails
# with a clear diagnostic instead of a raw sed error under `set -e`.
if [[ -n "${LOCKSTEP_VERSION:-}" ]]; then
  version="$LOCKSTEP_VERSION"
else
  if [[ ! -f "$manifest" ]]; then
    echo "check-version-lockstep: $manifest not found; run release-please or set LOCKSTEP_VERSION" >&2
    exit 1
  fi
  version="$(sed -n 's/.*"\."[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$manifest")"
fi
if [[ -z "$version" || "$version" == "null" ]]; then
  echo "check-version-lockstep: could not read root package version from $manifest" >&2
  exit 1
fi

probe_ref="ghcr.io/skaphos/fathom-probe:v${version}"
node_agent_ref="ghcr.io/skaphos/fathom-node-agent:v${version}"

failures=0

# check DESC EXPECTED ACTUAL — record a pass/fail row.
check() {
  local desc="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    printf '  %-48s OK   %s\n' "$desc" "$actual"
  else
    printf '  %-48s FAIL got %q want %q\n' "$desc" "$actual" "$expected"
    failures=$((failures + 1))
  fi
}

# first_quoted PATTERN FILE — print the first double-quoted value on the first
# line matching PATTERN. Used to pull image refs out of Go source, YAML, etc.
first_quoted() {
  awk -F'"' -v pat="$1" '$0 ~ pat {print $2; exit}' "$2"
}

# yaml_scalar KEY FILE — print the (optionally quoted) scalar for a top-level
# YAML key like `appVersion: "0.4.0"` or `version: 0.4.0`.
yaml_scalar() {
  awk -v key="$1" '
    $1 == key":" { v=$2; gsub(/"/, "", v); print v; exit }
  ' "$2"
}

echo "Version lockstep check (release version ${version}):"

# Operator-compiled defaults.
check "internal/app/options.go DefaultProbeImage" \
  "$probe_ref" "$(first_quoted 'DefaultProbeImage[[:space:]]*=' internal/app/options.go)"
check "internal/app/options.go DefaultNodeAgentImage" \
  "$node_agent_ref" "$(first_quoted 'DefaultNodeAgentImage[[:space:]]*=' internal/app/options.go)"
check "internal/adapter/coredns/adapter.go fallbackProbeImage" \
  "$probe_ref" "$(first_quoted 'fallbackProbeImage[[:space:]]*=' internal/adapter/coredns/adapter.go)"

# Helm chart: renders operator/probe/node-agent tags from v<appVersion>.
check "deploy/helm/fathom-operator Chart.yaml appVersion" \
  "$version" "$(yaml_scalar appVersion deploy/helm/fathom-operator/Chart.yaml)"
check "deploy/helm/fathom-operator Chart.yaml version" \
  "$version" "$(yaml_scalar version deploy/helm/fathom-operator/Chart.yaml)"

# e2e images loaded into kind must match the compiled defaults (IfNotPresent).
check "Taskfile.yml E2E_PROBE_IMG" \
  "$probe_ref" "$(first_quoted 'E2E_PROBE_IMG' Taskfile.yml)"
check "Taskfile.yml E2E_NODE_AGENT_IMG" \
  "$node_agent_ref" "$(first_quoted 'E2E_NODE_AGENT_IMG' Taskfile.yml)"

# Sample AddonCheck hard-pins probeImage (overrides the operator default). It
# must match E2E_PROBE_IMG / DefaultProbeImage or kind e2e ImagePullBackOffs
# when the sample asks for a tag that was never kind-loaded.
check "config/samples coredns probeImage" \
  "$probe_ref" "$(first_quoted 'probeImage:' config/samples/fathom_v1alpha1_addoncheck_coredns.yaml)"

if [[ "$failures" -gt 0 ]]; then
  echo "::error::version lockstep check failed: ${failures} site(s) do not match release ${version}." >&2
  echo "Every x-release-please-version site (release-please-config.json extra-files) must equal the release version." >&2
  exit 1
fi

echo "version lockstep check passed"
