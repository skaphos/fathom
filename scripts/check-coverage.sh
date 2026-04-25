#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Skaphos
# SPDX-License-Identifier: MIT
#
# Aggregate per-package coverage from a Go cover profile and fail if any
# package falls below its minimum threshold. Adapted from skaphos/repokeeper.
#
# Usage:
#   scripts/check-coverage.sh [coverage.out]
#
# Environment:
#   COVERAGE_MIN_DEFAULT  Default per-package threshold (percent). Default: 50.
#
# Per-package overrides and skips are kept in this file (see threshold_for_pkg
# and skip_pkg). Ratchet thresholds upward as coverage improves; do not lower
# them to make a PR pass.

set -euo pipefail

profile="${1:-coverage.out}"
default_threshold="${COVERAGE_MIN_DEFAULT:-50}"

if [[ ! -f "$profile" ]]; then
  echo "coverage profile not found: $profile" >&2
  exit 1
fi

# skip_pkg lists packages excluded from the gate. Reasons must be explicit.
# Remove an entry only when you are also raising real coverage on it.
skip_pkg() {
  local pkg="$1"
  case "$pkg" in
    # internal/controller currently holds kubebuilder scaffold reconcilers
    # (Reconcile is a stub). Re-enable once real reconciliation lands.
    github.com/skaphos/fathom/internal/controller) return 0 ;;
    *) return 1 ;;
  esac
}

# threshold_for_pkg returns the required minimum coverage percentage for a
# given package. Use this to set a higher bar on critical packages.
threshold_for_pkg() {
  local pkg="$1"
  case "$pkg" in
    *) echo "$default_threshold" ;;
  esac
}

mapfile -t coverage_rows < <(
  awk -F'[: ,]+' '
    NR>1 {
      file=$1; stmts=$4; cnt=$5;
      if (stmts == 0) next;
      pkg=file;
      sub(/\/[^\/]+$/, "", pkg);
      total[pkg]+=stmts;
      if (cnt > 0) covered[pkg]+=stmts;
    }
    END {
      for (pkg in total) {
        pct=(covered[pkg]/total[pkg])*100;
        printf "%s %.2f %d %d\n", pkg, pct, covered[pkg], total[pkg];
      }
    }
  ' "$profile" | sort
)

if [[ ${#coverage_rows[@]} -eq 0 ]]; then
  echo "no executable coverage data found in $profile" >&2
  exit 1
fi

echo "Per-package coverage thresholds (default ${default_threshold}%):"
failures=0
for row in "${coverage_rows[@]}"; do
  pkg="$(awk '{print $1}' <<<"$row")"
  pct="$(awk '{print $2}' <<<"$row")"
  covered="$(awk '{print $3}' <<<"$row")"
  total="$(awk '{print $4}' <<<"$row")"

  if skip_pkg "$pkg"; then
    printf "  %-55s %6.2f%% (%s/%s) [skipped]\n" "$pkg" "$pct" "$covered" "$total"
    continue
  fi

  threshold="$(threshold_for_pkg "$pkg")"

  printf "  %-55s %6.2f%% (%s/%s) [min %s%%]\n" "$pkg" "$pct" "$covered" "$total" "$threshold"
  if ! awk -v p="$pct" -v t="$threshold" 'BEGIN { exit (p+0 >= t+0) ? 0 : 1 }'; then
    failures=$((failures + 1))
  fi
done

if [[ "$failures" -gt 0 ]]; then
  echo "coverage threshold check failed: ${failures} package(s) below minimum" >&2
  exit 1
fi

echo "coverage threshold check passed"
