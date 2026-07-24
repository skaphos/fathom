/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// The CRD schema rejects sub-floor interval/timeout values at admission, but
// objects stored before the floors existed (or applied while relaxed CRDs
// were installed) still reconcile. clampCadence is the runtime backstop: the
// cadence helpers route every read — requeue period, run deadline, node-agent
// arguments — through it, so nothing can schedule faster than the floors in
// api/v1alpha1. Zero and negative values keep their historical treat-as-unset
// semantics in the callers (the defaults all exceed the floors).

// clampCadence returns d raised to floor when 0 < d < floor, otherwise d.
func clampCadence(d, floor time.Duration) time.Duration {
	if d > 0 && d < floor {
		return floor
	}
	return d
}

// cadenceClampMessages describes each configured cadence field the clamp
// raises, in the fixed field order interval, timeout. Empty when nothing is
// clamped. The message names the field, the configured value, and the
// effective value so the Event and the Accepted condition explain themselves.
func cadenceClampMessages(interval, timeout *metav1.Duration) []string {
	var msgs []string
	if interval != nil && interval.Duration > 0 && interval.Duration < fathomv1alpha1.MinCheckInterval {
		msgs = append(msgs, fmt.Sprintf("spec.interval %s is below the minimum %s; using %s",
			interval.Duration, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.MinCheckInterval))
	}
	if timeout != nil && timeout.Duration > 0 && timeout.Duration < fathomv1alpha1.MinCheckTimeout {
		msgs = append(msgs, fmt.Sprintf("spec.timeout %s is below the minimum %s; using %s",
			timeout.Duration, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.MinCheckTimeout))
	}
	return msgs
}
