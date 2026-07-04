/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package scripts holds guard tests for the repository's build/CI scripts.
// It contains no runtime code — only tests that assert invariants about the
// scripts under scripts/ so they cannot drift silently.
package scripts

import (
	"os"
	"strings"
	"testing"
)

// TestCoverageGateSkipsNoPackages asserts that scripts/check-coverage.sh skips
// no packages from the per-package coverage gate (SKA-302): skip_pkg's case
// block must be exactly the default `*) return 1` arm.
//
// Pinning the whole block is deliberately stricter than grepping for
// `return 0`: a skip can be written many ways that all exit 0 — `pkg) true ;;`,
// `pkg) return ;;` (bash `return` with no arg yields the last status), or
// flipping the default to `*) return 0`. Requiring the exact default-only form
// rejects all of them.
//
// This guard exists so the skip list cannot grow silently: excluding a package
// to dodge a red PR would fail this test, forcing the change to be a
// deliberate, reviewed edit here (with a linked tracking issue).
func TestCoverageGateSkipsNoPackages(t *testing.T) {
	const script = "check-coverage.sh"
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read %s: %v", script, err)
	}
	src := string(data)

	const marker = "skip_pkg() {"
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("%s: skip_pkg() function not found", script)
	}
	body := src[start+len(marker):]
	// The function closes with `}` on its own line at column 0.
	end := strings.Index(body, "\n}")
	if end < 0 {
		t.Fatalf("%s: could not find end of skip_pkg() function", script)
	}
	body = body[:end]

	caseStart := strings.Index(body, "case ")
	// LastIndex for the closing keyword so an "esac" inside an earlier comment
	// can't truncate the block; require it to sit after the opener.
	caseEnd := strings.LastIndex(body, "esac")
	if caseStart < 0 || caseEnd <= caseStart {
		t.Fatalf("%s: skip_pkg() has no case…esac block:\n%s", script, body)
	}
	got := normalizeShell(body[caseStart : caseEnd+len("esac")])

	const want = `case "$pkg" in *) return 1 ;; esac`
	if got != want {
		t.Fatalf("%s: skip_pkg() must skip no packages — its case block must be "+
			"exactly the default arm.\n got:  %s\n want: %s\n\n"+
			"The coverage gate holds every package (minus e2e) to the threshold; "+
			"raise real coverage instead of adding a skip. A genuinely required "+
			"skip must cite a tracking issue and update this guard deliberately.",
			script, got, want)
	}
}

// normalizeShell strips shell comments — both full-line and trailing — and
// blank lines, then collapses all remaining whitespace to single spaces, so the
// assertion ignores indentation and comments but is sensitive to any change in
// the case arms themselves.
func normalizeShell(block string) string {
	var kept []string
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(stripShellComment(line))
		if trimmed == "" {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(strings.Fields(strings.Join(kept, " ")), " ")
}

// stripShellComment removes a trailing shell comment from a line. A '#' begins a
// comment only when it starts a word (start of line, or preceded by
// whitespace), so a '#' embedded in a pattern or string is left intact.
func stripShellComment(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return line[:i]
		}
	}
	return line
}
