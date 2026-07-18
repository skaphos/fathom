/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import "testing"

// TestNodeAgentTemplateNeedsWrite verifies the guard ensureDaemonSet uses to
// decide whether to (re)write the node-agent pod template. The empty-desiredHash
// case is the regression: a marshal failure yields an empty hash, and on create
// the stored annotation is also empty — a plain equality check would then skip
// the template write and produce an invalid DaemonSet. The guard must force the
// write whenever desiredHash is empty (SKA-49 review follow-up).
func TestNodeAgentTemplateNeedsWrite(t *testing.T) {
	tests := []struct {
		name      string
		stored    string
		desired   string
		wantWrite bool
	}{
		{name: "create, hash computed", stored: "", desired: "abc123", wantWrite: true},
		{name: "unchanged hash", stored: "abc123", desired: "abc123", wantWrite: false},
		{name: "changed hash", stored: "abc123", desired: "def456", wantWrite: true},
		{name: "empty hash on create forces write", stored: "", desired: "", wantWrite: true},
		{name: "empty hash with prior stamp forces write", stored: "abc123", desired: "", wantWrite: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeAgentTemplateNeedsWrite(tt.stored, tt.desired); got != tt.wantWrite {
				t.Errorf("nodeAgentTemplateNeedsWrite(%q, %q) = %v, want %v", tt.stored, tt.desired, got, tt.wantWrite)
			}
		})
	}
}
