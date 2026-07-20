/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package podutil holds the small pod predicates shared by the addon adapters.
// It depends only on corev1 so every adapter (declarative, cert-manager,
// CoreDNS) can share one definition of "is this a live serving-candidate pod"
// rather than re-deriving it — the divergence that let the same false-positive
// bug live in three copies (#160).
package podutil

import (
	corev1 "k8s.io/api/core/v1"
)

// Active reports whether a pod is a live serving candidate — one whose
// readiness should count toward a workload's pod health. It excludes:
//
//   - terminating pods (DeletionTimestamp set): the old ReplicaSet's pods
//     during a rolling update still match the workload selector but are on
//     their way out and are expected to be not-Ready.
//   - pods in a terminal phase (Failed/Succeeded): a lingering Evicted or
//     Completed pod that was never garbage-collected is not part of the
//     serving set.
//
// A running-but-not-ready pod (a genuine problem, e.g. CrashLoopBackOff) is
// still Active and must be graded by the caller. The authoritative outage
// signal remains the workload-availability check; this predicate only prevents
// churned-out pods from producing spurious per-pod verdicts.
func Active(p *corev1.Pod) bool {
	if p.DeletionTimestamp != nil {
		return false
	}
	switch p.Status.Phase {
	case corev1.PodFailed, corev1.PodSucceeded:
		return false
	default:
		return true
	}
}
