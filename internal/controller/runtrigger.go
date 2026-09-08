/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// runTriggerDue is the one place the on-demand trigger contract is decided:
// the fathom.skaphos.io/run-now annotation is due when it is set and differs
// from the value the check's status records as consumed. Every executable
// kind's reconciler calls this rather than reading the annotation itself, so
// consume-once semantics cannot drift between kinds.
//
// The returned token is the annotation value even when not due (so callers can
// stamp it where the rollout needs it, e.g. the node-agent DaemonSet template)
// and empty when the annotation is absent.
func runTriggerDue(annotations map[string]string, lastConsumed string) (token string, due bool) {
	token = annotations[fathomv1alpha1.AnnotationRunNow]
	return token, token != "" && token != lastConsumed
}
