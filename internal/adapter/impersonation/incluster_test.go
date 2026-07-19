/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation_test

import (
	"testing"

	"github.com/skaphos/fathom/internal/adapter/impersonation"
)

func TestRunningInCluster_TestOverride(t *testing.T) {
	restore := impersonation.SetRunningInClusterForTest(true)
	t.Cleanup(restore)
	if !impersonation.RunningInCluster() {
		t.Fatal("RunningInCluster() = false after SetRunningInClusterForTest(true)")
	}

	restore2 := impersonation.SetRunningInClusterForTest(false)
	t.Cleanup(restore2)
	if impersonation.RunningInCluster() {
		t.Fatal("RunningInCluster() = true after SetRunningInClusterForTest(false)")
	}
}
