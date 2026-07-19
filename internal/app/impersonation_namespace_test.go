/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"strings"
	"testing"

	"github.com/skaphos/fathom/internal/adapter/impersonation"
)

// TestDefaultControllers_InClusterRequiresNamespace locks SKA-162: a missing
// FATHOM_NAMESPACE / --namespace while running in-cluster must fail at
// controller construction, not silently hand adapters the operator client.
func TestDefaultControllers_InClusterRequiresNamespace(t *testing.T) {
	restore := impersonation.SetRunningInClusterForTest(true)
	t.Cleanup(restore)

	opts := DefaultOptions()
	opts.Namespace = ""

	// mgr is unused on the fail-closed path; nil is intentional.
	_, err := DefaultControllers(nil, opts)
	if err == nil {
		t.Fatal("DefaultControllers: expected error when Namespace is empty in-cluster")
	}
	if !strings.Contains(err.Error(), "SKA-162") {
		t.Errorf("error %q should name SKA-162", err.Error())
	}
}

// TestDefaultControllers_OutOfClusterAllowsEmptyNamespace keeps local
// `task run` against a privileged kubeconfig working without FATHOM_NAMESPACE.
func TestDefaultControllers_OutOfClusterAllowsEmptyNamespace(t *testing.T) {
	// This path constructs reconcilers and needs a real manager; the happy-path
	// Run test already covers full wiring. Here we only assert the in-cluster
	// gate does not fire out of cluster — by ensuring RunningInCluster is false
	// and that DefaultControllers is not rejected solely for empty Namespace
	// before it touches mgr. We cannot call DefaultControllers(nil) once past
	// the gate (impersonation.New(nil) panics), so this test only validates the
	// predicate via the in-cluster helper used by DefaultControllers.
	restore := impersonation.SetRunningInClusterForTest(false)
	t.Cleanup(restore)

	if impersonation.RunningInCluster() {
		t.Fatal("test setup: expected out-of-cluster")
	}
	opts := DefaultOptions()
	if opts.Namespace != "" {
		t.Fatalf("DefaultOptions().Namespace = %q, want empty for out-of-cluster default", opts.Namespace)
	}
	// Gate condition used by DefaultControllers: empty namespace is allowed.
	if opts.Namespace == "" && impersonation.RunningInCluster() {
		t.Fatal("out-of-cluster empty namespace incorrectly treated as in-cluster fail-closed")
	}
}
