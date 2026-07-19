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

// The out-of-cluster path (empty Namespace allowed, full reconciler wiring
// constructed) is covered end-to-end by TestRun_HappyPath_DefaultControllers,
// which runs DefaultControllers against envtest — inherently out-of-cluster, so
// the SKA-162 gate does not fire. No separate out-of-cluster gate test is needed
// here.
