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
// no packages from the per-package coverage gate (SKA-302). A skip arm returns
// 0 from skip_pkg(); the gate should hold every package (minus e2e) to the
// threshold, so skip_pkg's body must contain no `return 0`.
//
// This guard exists so the skip list cannot grow silently: adding a skip to
// dodge a red PR would fail this test, forcing the change to be a deliberate,
// reviewed edit here (with a linked tracking issue) rather than a quiet one.
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
	rest := src[start+len(marker):]
	// The function closes with `}` on its own line at column 0.
	end := strings.Index(rest, "\n}")
	if end < 0 {
		t.Fatalf("%s: could not find end of skip_pkg() function", script)
	}
	body := rest[:end]

	if strings.Contains(body, "return 0") {
		t.Fatalf("%s: skip_pkg() skips a package (found `return 0`). The coverage "+
			"gate must skip no packages; raise real coverage instead. If a skip is "+
			"genuinely required, cite a tracking issue and update this guard test "+
			"deliberately.\nskip_pkg body:%s", script, body)
	}
}
