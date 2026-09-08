/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"runtime/debug"
)

// Version is the fathomctl release version, injected at build time:
//
//	-ldflags "-X github.com/skaphos/fathom/internal/cli.Version=v0.6.0"
//
// The fathomctl-build and fathomctl-dist tasks set it; a plain `go build`
// leaves it empty and clientVersion falls back to what the Go toolchain
// recorded, so a development binary identifies itself as such.
var Version string

// devVersion is reported when neither the ldflag nor the module build info
// carries a usable version.
const devVersion = "devel"

// clientVersion returns the version fathomctl reports for itself. Order of
// preference: the ldflag, the main module version stamped by `go install
// pkg@version`, then "devel" plus a short VCS revision when the toolchain
// recorded one.
func clientVersion() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return devVersion
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		v = devVersion
	}
	if rev := buildSetting(info, "vcs.revision"); rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		v += "+" + rev
		if buildSetting(info, "vcs.modified") == "true" {
			v += ".dirty"
		}
	}
	return v
}

func buildSetting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
