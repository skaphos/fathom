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

# skip_pkg lists packages excluded from the gate. It is intentionally empty:
# every package under ./... (minus e2e) is held to the coverage threshold.
# internal/controller was previously skipped while its reconcilers were
# kubebuilder stubs; the reconcilers are now real and tested (~81% coverage),
# so the skip is gone (SKA-302).
#
# Do NOT add a skip to make a red PR pass — raise real coverage instead. A
# genuinely justified skip must cite a tracking issue AND update the guard test
# scripts/coverage_gate_test.go, so the skip list cannot grow silently.
skip_pkg() {
  local pkg="$1"
  case "$pkg" in
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

# Read awk output via a `while` loop instead of `mapfile` so the script
# works on Bash 3.2 (the default /bin/bash on macOS).
coverage_rows=()
while IFS= read -r row; do
  coverage_rows+=("$row")
done < <(
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
