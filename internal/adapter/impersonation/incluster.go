/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation

import "os"

// serviceAccountTokenPath is the standard in-pod path for the projected
// ServiceAccount token. Presence of this file is the conventional "are we
// running inside a Kubernetes pod?" probe (same signal rest.InClusterConfig
// uses before contacting the API).
const serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// runningInCluster reports whether this process is executing inside a
// Kubernetes pod. Tests override it via SetRunningInClusterForTest.
var runningInCluster = defaultRunningInCluster

func defaultRunningInCluster() bool {
	_, err := os.Stat(serviceAccountTokenPath)
	return err == nil
}

// RunningInCluster reports whether the process is running in a Kubernetes pod.
// Used to fail closed when adapter impersonation cannot be scoped (SKA-162).
func RunningInCluster() bool {
	return runningInCluster()
}

// SetRunningInClusterForTest overrides in-cluster detection for tests. The
// returned restore function puts the previous probe back.
func SetRunningInClusterForTest(inCluster bool) (restore func()) {
	prev := runningInCluster
	runningInCluster = func() bool { return inCluster }
	return func() { runningInCluster = prev }
}
