#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Rillan AI LLC
# SPDX-License-Identifier: MIT
#
# Cross-compile fathomctl for every supported platform and package one archive
# per platform plus a SHA-256 checksums file. The release workflow signs the
# checksums file with cosign (keyless) and attests provenance for the archives;
# a verifier checks the signature, then the checksums, then the archive.
#
# Usage:
#   VERSION=0.6.0 scripts/fathomctl-dist.sh
#
# Environment:
#   VERSION              Release version, with or without a leading v. Required.
#                        Baked into the binary (`fathomctl version` prints v<VERSION>)
#                        and into every archive name.
#   OUT                  Output directory. Default: dist/fathomctl
#   FATHOMCTL_PLATFORMS  Space-separated GOOS/GOARCH pairs. Default: the six
#                        release targets.
#
# Output (in OUT):
#   fathomctl_<VERSION>_<os>_<arch>.tar.gz   (linux, darwin)
#   fathomctl_<VERSION>_<os>_<arch>.zip      (windows)
#   fathomctl_<VERSION>_checksums.txt

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

version="${VERSION:?VERSION is required, e.g. VERSION=0.6.0}"
version="${version#v}"
out="${OUT:-dist/fathomctl}"
platforms="${FATHOMCTL_PLATFORMS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64}"
ldflags="-s -w -buildid= -X github.com/skaphos/fathom/internal/cli.Version=v${version}"

case "$out" in
  /*) outabs="$out" ;;
  *) outabs="$root/$out" ;;
esac
mkdir -p "$outabs"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

archives=()
for platform in $platforms; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  bin="fathomctl"
  if [[ "$goos" == "windows" ]]; then
    bin="fathomctl.exe"
  fi
  base="fathomctl_${version}_${goos}_${goarch}"
  stage="$work/$base"
  mkdir -p "$stage"

  echo "building $base"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="$ldflags" -o "$stage/$bin" ./cmd/fathomctl
  cp LICENSE "$stage/LICENSE"

  if [[ "$goos" == "windows" ]]; then
    archive="$base.zip"
    (cd "$work" && zip -q -r "$outabs/$archive" "$base")
  else
    archive="$base.tar.gz"
    tar -C "$work" -czf "$outabs/$archive" "$base"
  fi
  archives+=("$archive")
  rm -rf "$stage"
done

if command -v sha256sum >/dev/null 2>&1; then
  sum_cmd=(sha256sum)
else
  sum_cmd=(shasum -a 256)
fi
(cd "$outabs" && "${sum_cmd[@]}" "${archives[@]}" > "fathomctl_${version}_checksums.txt")
echo "wrote $outabs/fathomctl_${version}_checksums.txt"
