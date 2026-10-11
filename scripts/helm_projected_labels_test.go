/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"slices"
	"strings"
	"testing"
)

// metrics.projectedLabels reaches the manager as one comma-separated flag, and
// the default passes nothing, leaving the operator's own default (no
// projection) in force (#279).
func TestHelmProjectedLabelsFlag(t *testing.T) {
	objects, out, err := renderRuntimeChart(t)
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	for _, arg := range managerArgs(t, objects) {
		if strings.HasPrefix(arg, "--metrics-projected-labels") {
			t.Errorf("default render passes %q", arg)
		}
	}

	objects, out, err = renderRuntimeChart(t,
		"--set-json", `metrics.projectedLabels=["tier","app.kubernetes.io/team"]`)
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	want := "--metrics-projected-labels=tier,app.kubernetes.io/team"
	if args := managerArgs(t, objects); !slices.Contains(args, want) {
		t.Errorf("manager args %v do not contain %q", args, want)
	}
}

// The schema caps the list at the operator's own limit so a values typo fails
// at install time rather than as a crash-looping manager.
func TestHelmProjectedLabelsSchemaCap(t *testing.T) {
	_, out, err := renderRuntimeChart(t,
		"--set-json", `metrics.projectedLabels=["a","b","c","d","e","f","g","h","i","j","k"]`)
	if err == nil {
		t.Fatal("eleven projected labels were accepted")
	}
	if !strings.Contains(out, "projectedLabels") {
		t.Errorf("schema error does not name projectedLabels:\n%s", out)
	}
}
