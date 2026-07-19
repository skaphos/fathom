/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation

import (
	"errors"
	"fmt"
	"sync"

	"k8s.io/client-go/rest"
)

// runningInCluster reports whether this process is executing inside a
// Kubernetes pod. Tests override it via SetRunningInClusterForTest.
var runningInCluster = defaultRunningInCluster

// inClusterOnce memoizes the probe: a process never migrates between in- and
// out-of-cluster during its lifetime, and the underlying rest.InClusterConfig
// reads the ServiceAccount token and parses the root CA on every call, so it is
// resolved once rather than on every reconcile.
var (
	inClusterOnce sync.Once
	inClusterVal  bool
)

// defaultRunningInCluster reports whether the process is running inside a
// Kubernetes pod, using the exact signal client-go itself uses to decide
// whether in-cluster configuration is available. rest.InClusterConfig returns
// rest.ErrNotInCluster only when KUBERNETES_SERVICE_HOST/PORT are unset (the
// authoritative "not in a pod" marker, injected by the kubelet regardless of
// automountServiceAccountToken).
//
// Any other outcome is treated as in-cluster so the impersonation gate fails
// closed, not open (SKA-162): a healthy in-cluster config, but also the env
// vars present while the ServiceAccount token is unreadable (automount
// disabled, a drifted or non-default token projection). Those are broken or
// unusual in-cluster states, and the safe response is to demand a scoped
// namespace rather than fall back to the operator identity.
func defaultRunningInCluster() bool {
	inClusterOnce.Do(func() {
		_, err := rest.InClusterConfig()
		inClusterVal = inClusterFromConfigErr(err)
	})
	return inClusterVal
}

// inClusterFromConfigErr maps the error rest.InClusterConfig returns to an
// in-cluster verdict. Only the rest.ErrNotInCluster sentinel (env vars unset)
// is treated as definitively out-of-cluster; every other outcome — nil, or a
// token/CA read failure while the env vars are set — fails closed as in-cluster
// (SKA-162).
func inClusterFromConfigErr(err error) bool {
	return !errors.Is(err, rest.ErrNotInCluster)
}

// RunningInCluster reports whether the process is running in a Kubernetes pod.
// Used to fail closed when adapter impersonation cannot be scoped (SKA-162).
func RunningInCluster() bool {
	return runningInCluster()
}

// ErrNamespaceRequiredInCluster is the fail-closed error returned wherever an
// empty operator namespace is rejected while running in-cluster (SKA-162). It
// is shared by the startup gate (DefaultControllers) and the per-reconcile gate
// (AddonCheckReconciler.adapterClient) so operators see — and can grep for — one
// consistent message regardless of where the check fires.
var ErrNamespaceRequiredInCluster = fmt.Errorf(
	"operator namespace is empty while running in-cluster; set FATHOM_NAMESPACE (downward API) or --namespace so adapter impersonation cannot fail open (SKA-162)",
)

// SetRunningInClusterForTest overrides in-cluster detection for tests. The
// returned restore function puts the previous probe back.
func SetRunningInClusterForTest(inCluster bool) (restore func()) {
	prev := runningInCluster
	runningInCluster = func() bool { return inCluster }
	return func() { runningInCluster = prev }
}
