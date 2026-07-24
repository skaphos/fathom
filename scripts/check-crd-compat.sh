#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Skaphos
# SPDX-License-Identifier: MIT
#
# CRD schema-compatibility gate (issue #152; docs/reference/api-versioning.md
# "Enforcement and tooling").
#
# Compares every generated CRD under config/crd/bases against the same file at
# the most recent release tag using crdify (pinned in tools/go.mod), so an
# accidental incompatible schema change under an unchanged served version
# fails CI instead of shipping. Deliberate, sanctioned breakage (v1alpha1
# alpha-churn or a new API version) is acknowledged through the committed
# allowlist .crd-compat-allowlist.yaml — the override lands in the reviewed
# diff, never in out-of-band CI state. Allowlist entries that match no finding
# are reported STALE so the file self-prunes as release baselines advance.
#
# Check configuration lives in .crdify.yaml (description-only changes are
# compatible; unhandled changes are errors).
#
# Usage:
#   scripts/check-crd-compat.sh
#
# Environment (tests/fixtures):
#   CRD_COMPAT_BASELINE_REF  Baseline git ref. Defaults to the highest v* semver tag.
#   CRD_COMPAT_OLD_DIR       Directory holding baseline CRDs; skips git entirely.
#   CRD_COMPAT_NEW_DIR       Directory holding current CRDs (default config/crd/bases).
#   CRD_COMPAT_ALLOWLIST     Allowlist file (default .crd-compat-allowlist.yaml).
#   CRD_COMPAT_CONFIG        crdify check config (default .crdify.yaml).
#   CRDIFY                   crdify invocation (default: go -C <root>/tools tool crdify).
#
# Requires python3 (present on CI runners and contributor machines; jq is not
# guaranteed, per repo convention).

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

new_dir="${CRD_COMPAT_NEW_DIR:-config/crd/bases}"
allowlist="${CRD_COMPAT_ALLOWLIST:-.crd-compat-allowlist.yaml}"
crdify_config="${CRD_COMPAT_CONFIG:-.crdify.yaml}"
default_crdify="go -C $root/tools tool crdify"
crdify_cmd="${CRDIFY:-$default_crdify}"

# `go -C tools tool crdify` runs with tools/ as its working directory, so every
# path handed to crdify must be absolute.
case "$crdify_config" in /*) ;; *) crdify_config="$root/$crdify_config" ;; esac

if [[ ! -d "$new_dir" ]]; then
  echo "check-crd-compat: current CRD directory $new_dir not found" >&2
  exit 1
fi

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

# Resolve where baseline CRDs come from: an explicit fixture directory, or the
# most recent release tag. No tag at all (fresh fork, pre-first-release) means
# there is nothing to be incompatible with.
old_dir="${CRD_COMPAT_OLD_DIR:-}"
if [[ -n "$old_dir" ]]; then
  old_dir="$(cd "$old_dir" && pwd)"
fi
baseline_desc=""
baseline_ref=""
if [[ -n "$old_dir" ]]; then
  baseline_desc="fixture directory $old_dir"
else
  baseline_ref="${CRD_COMPAT_BASELINE_REF:-$(git tag --list 'v*' --sort=-v:refname | head -1)}"
  if [[ -z "$baseline_ref" ]]; then
    echo "check-crd-compat: no v* release tag reachable — no baseline to compare against; gate passes"
    exit 0
  fi
  baseline_desc="release tag $baseline_ref"
fi
echo "check-crd-compat: baseline is $baseline_desc"

# Run crdify per CRD file, collecting one JSON report per compared CRD. A CRD
# with no counterpart at the baseline is new — nothing to be incompatible with.
compared=()
for new_file in "$new_dir"/*.yaml; do
  name="$(basename "$new_file")"
  old_file=""
  if [[ -n "$old_dir" ]]; then
    [[ -f "$old_dir/$name" ]] && old_file="$old_dir/$name"
  elif git cat-file -e "$baseline_ref:config/crd/bases/$name" 2>/dev/null; then
    old_file="$workdir/old-$name"
    git show "$baseline_ref:config/crd/bases/$name" > "$old_file"
  fi
  if [[ -z "$old_file" ]]; then
    echo "  NEW $name: no baseline counterpart; skipped"
    continue
  fi

  # crdify exits non-zero when it finds incompatibilities; the JSON report is
  # the interesting part either way, so tolerate the exit code here and let
  # the classifier below decide what fails the gate.
  if ! $crdify_cmd --config "$crdify_config" "file://$old_file" "file://$(cd "$(dirname "$new_file")" && pwd)/$name" -o json \
      > "$workdir/report-$name.json" 2> "$workdir/stderr-$name.txt"; then
    if [[ ! -s "$workdir/report-$name.json" ]]; then
      echo "check-crd-compat: crdify failed for $name:" >&2
      cat "$workdir/stderr-$name.txt" >&2
      exit 1
    fi
  fi
  compared+=("$name")
done

if [[ "${#compared[@]}" -eq 0 ]]; then
  echo "check-crd-compat: no CRDs shared with the baseline; gate passes"
  exit 0
fi

# Classify findings against the allowlist and render the report. Kept in
# python3 because the crdify report is JSON and jq is not a repo dependency.
# The allowlist parser understands exactly the format the file documents: a
# flat list of flat key/value mappings.
python3 - "$workdir" "$allowlist" "${compared[@]}" <<'PYEOF'
import json
import os
import sys

workdir, allowlist_path, names = sys.argv[1], sys.argv[2], sys.argv[3:]


def crd_name(filename):
    # controller-gen names files <group>_<plural>.yaml; the CRD's
    # metadata.name is <plural>.<group>.
    group, plural = filename.removesuffix(".yaml").split("_", 1)
    return f"{plural}.{group}"


def load_allowlist(path):
    entries = []
    if not os.path.exists(path):
        return entries
    entry = None
    for lineno, raw in enumerate(open(path, encoding="utf-8"), 1):
        line = raw.split("#", 1)[0].rstrip()
        if not line.strip():
            continue
        if line.startswith("- "):
            entry = {}
            entries.append(entry)
            line = "  " + line[2:]
        if entry is None or ":" not in line:
            sys.exit(f"check-crd-compat: {path}:{lineno}: expected `- key: value` list entries")
        key, _, value = line.strip().partition(":")
        entry[key.strip()] = value.strip().strip("'\"")
    for i, e in enumerate(entries):
        missing = {"crd", "path", "reason", "issue"} - set(e)
        if missing:
            sys.exit(f"check-crd-compat: {path}: entry {i + 1} is missing {sorted(missing)}")
    return entries


allowlist = load_allowlist(allowlist_path)
matched = [False] * len(allowlist)
incompatible = 0

for filename in names:
    crd = crd_name(filename)
    with open(os.path.join(workdir, f"report-{filename}.json"), encoding="utf-8") as f:
        report = json.load(f)

    findings, warnings = [], []
    for section in report.values():
        for version_result in section or []:
            for pc in version_result.get("propertyComparisons") or []:
                for cr in pc.get("comparisonResults") or []:
                    if cr.get("errors"):
                        findings.append((pc["property"], cr["name"], cr["errors"]))
                    if cr.get("warnings"):
                        warnings.append((pc["property"], cr["name"], cr["warnings"]))

    if not findings and not warnings:
        print(f"  OK {crd}: compatible")
        continue
    print(f"  {crd}: {len(findings)} finding(s)")
    for prop, check, errors in findings:
        sanction = None
        for i, e in enumerate(allowlist):
            if e["crd"] == crd and e["path"] in ("*", prop):
                sanction, matched[i] = e, True
                break
        if sanction:
            print(f"    SANCTIONED {prop} ({check}): {sanction['reason']} — {sanction['issue']}")
        else:
            incompatible += 1
            print(f"    INCOMPATIBLE {prop} ({check}):")
            for err in errors:
                print("      " + err.replace("\n", "\n      "))
    for prop, check, warns in warnings:
        for w in warns:
            print(f"    WARN {prop} ({check}): {w}")

for i, e in enumerate(allowlist):
    if not matched[i]:
        print(f"  STALE allowlist entry (matched nothing): {e['crd']} {e['path']} — prune it ({e['issue']})")

if incompatible:
    print(
        f"check-crd-compat: {incompatible} unsanctioned incompatible change(s) to a served CRD version.\n"
        "  An incompatible change outside sanctioned alpha churn must ride a new API version\n"
        "  (docs/reference/api-versioning.md). If this change IS sanctioned, add a reviewed\n"
        "  entry to .crd-compat-allowlist.yaml citing the reason and tracking issue."
    )
    sys.exit(1)
print("check-crd-compat: OK")
PYEOF
