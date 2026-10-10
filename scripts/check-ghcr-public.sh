#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Rillan AI LLC
# SPDX-License-Identifier: MIT
#
# Fail unless every named GHCR package is anonymously pullable (skaphos/fathom#333).
#
# GHCR creates every NEW package name private, even when a public repository's
# workflow pushes it with GITHUB_TOKEN and the artifact links back to the repo
# (org.opencontainers.image.source). There is no org-level "new packages are
# public" default and no REST API to change visibility, so a first publish of a
# new image or chart name silently ships an anonymous-pull failure. This script
# turns that into a red release instead.
#
# Usage:
#   scripts/check-ghcr-public.sh <package> [<package> ...]
#
# A package is the path under the namespace, e.g. `fathom-operator` or
# `charts/fathom-operator`. For each one the script requests an anonymous pull
# token from the GHCR token endpoint with no credentials:
#   200       -> public (pass)
#   401 / 403 -> private or missing (fail)
#   anything else (5xx, transport error, truncated body, 200 without a
#   token) -> retried, then fail
# Every non-200 is retried with exponential backoff, since a just-pushed
# package can take a moment to settle.
#
# Environment:
#   GHCR_NAMESPACE       Registry namespace (default: skaphos).
#   GHCR_TOKEN_ENDPOINT  Token endpoint (default: https://ghcr.io/token).
#   GHCR_CHECK_ATTEMPTS  Attempts per package (default: 5).
#   GHCR_CHECK_DELAY     Initial backoff in seconds, doubled per retry (default: 5).

set -euo pipefail

namespace="${GHCR_NAMESPACE:-skaphos}"
endpoint="${GHCR_TOKEN_ENDPOINT:-https://ghcr.io/token}"
attempts="${GHCR_CHECK_ATTEMPTS:-5}"
initial_delay="${GHCR_CHECK_DELAY:-5}"

if [[ $# -eq 0 ]]; then
  echo "usage: $0 <package> [<package> ...]" >&2
  exit 2
fi

body_file=$(mktemp)
trap 'rm -f "${body_file}"' EXIT

# anon_status prints the HTTP status of an unauthenticated pull-token request
# for one package. It prints 000 when the transfer itself failed (curl exits
# non-zero even if a 200 status line already arrived, e.g. a truncated body)
# and 200-notoken when the 200 response carries no token, so only a complete
# response that actually grants an anonymous token counts as public.
anon_status() {
  local pkg="$1" status
  # No credentials on purpose: this is exactly what an anonymous
  # `docker pull` / `helm install oci://...` does first.
  if ! status=$(curl --silent --output "${body_file}" \
    --max-time 20 --write-out '%{http_code}' \
    "${endpoint}?scope=repository:${namespace}/${pkg}:pull" 2>/dev/null); then
    echo "000"
    return
  fi
  if [[ "${status}" == "200" ]] && ! grep -Eq '"(access_)?token"[[:space:]]*:[[:space:]]*"[^"]+"' "${body_file}"; then
    echo "200-notoken"
    return
  fi
  echo "${status:-000}"
}

check_package() {
  local pkg="$1" delay="${initial_delay}" status try
  for ((try = 1; try <= attempts; try++)); do
    status=$(anon_status "${pkg}")
    if [[ "${status}" == "200" ]]; then
      echo "ok: ghcr.io/${namespace}/${pkg} is public (anonymous pull token: HTTP 200)"
      return 0
    fi
    if ((try < attempts)); then
      echo "retry: ghcr.io/${namespace}/${pkg} returned HTTP ${status} (attempt ${try}/${attempts}); retrying in ${delay}s" >&2
      sleep "${delay}"
      delay=$((delay * 2))
    fi
  done

  {
    echo "error: ghcr.io/${namespace}/${pkg} is not anonymously pullable (HTTP ${status} after ${attempts} attempts)."
    case "${status}" in
      401 | 403)
        echo "  The package is private (or does not exist). New GHCR packages are created private."
        ;;
      *)
        echo "  The GHCR token endpoint did not answer cleanly; re-run once GHCR is healthy."
        ;;
    esac
    echo "  Fix: github.com/orgs/${namespace} -> Packages -> ${pkg} -> Package settings ->"
    echo "       Danger Zone -> Change visibility -> Public, then re-run this check"
    # Point back at this script rather than a raw curl: a bare status-code
    # probe would report 200 for the truncated / token-less responses the
    # gate deliberately rejects.
    if [[ "${namespace}" == "skaphos" ]]; then
      echo "       (from the repo root): scripts/check-ghcr-public.sh ${pkg}"
    else
      echo "       (from the repo root): GHCR_NAMESPACE=${namespace} scripts/check-ghcr-public.sh ${pkg}"
    fi
  } >&2
  return 1
}

failed=()
for pkg in "$@"; do
  if ! check_package "${pkg}"; then
    failed+=("${pkg}")
  fi
done

if ((${#failed[@]} > 0)); then
  echo "error: ${#failed[@]} GHCR package(s) are not public: ${failed[*]}" >&2
  echo "  See RELEASE.md, 'First publish of a new GHCR package'." >&2
  exit 1
fi
echo "all $# GHCR package(s) are public"
