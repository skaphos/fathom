/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// waitResult classifies how a wait ended. Exactly one of err, superseded,
// timedOut, or a filled snap applies.
type waitResult struct {
	snap       snapshot
	superseded bool
	timedOut   bool
	err        error
}

// blockingReasons are Ready=False reasons under which the operator will never
// consume a trigger: the check is misconfigured or has nothing to run on.
// Waiting further would only time out, so --wait fails fast and says why.
var blockingReasons = map[string]bool{
	"InvalidPolicy":       true,
	"MissingAdapter":      true,
	"AdapterLookupFailed": true,
	"NoMatchingNodes":     true,
	"Paused":              true,
}

// waitForRun polls the target until the operator has consumed exactly this
// token. Every controller writes status.lastRunTrigger in the same status
// update as the run's verdict, so equality on the token is equality on
// completion. If the annotation stops equalling the token, a later trigger
// superseded this one and waiting further would never end.
//
// Transient API errors do not end the wait: the trigger is already written,
// and the deadline bounds the retries. Only a vanished object or a permission
// failure is final.
func (f *factory) waitForRun(ctx context.Context, c client.Client, t runTarget, token string, timeout time.Duration) waitResult {
	var res waitResult
	key := types.NamespacedName{Namespace: t.ref.Namespace, Name: t.ref.Name}
	err := wait.PollUntilContextTimeout(ctx, f.pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		obj := t.ref.Kind.New()
		if err := c.Get(ctx, key, obj); err != nil {
			if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return false, fmt.Errorf("re-read %s: %w", t.ref, err)
			}
			return false, nil
		}
		if obj.GetAnnotations()[fathomv1alpha1.AnnotationRunNow] != token {
			res.superseded = true
			return true, nil
		}
		snap := t.ref.Kind.Snapshot(obj)
		if snap.ConsumedTrigger == token {
			res.snap = snap
			return true, nil
		}
		if reason, msg := blockedBy(conditionsOf(obj)); reason != "" {
			return false, fmt.Errorf("the operator cannot run %s (%s): %s", t.ref, reason, msg)
		}
		return false, nil
	})
	switch {
	case err == nil:
		return res
	case errors.Is(err, context.DeadlineExceeded) || wait.Interrupted(err):
		return waitResult{timedOut: true}
	default:
		return waitResult{err: err}
	}
}

// blockedBy returns the Ready=False reason and message when it is one the
// operator will never recover from without a spec change.
func blockedBy(conds []metav1.Condition) (string, string) {
	ready := apimeta.FindStatusCondition(conds, readyCondition)
	if ready == nil || ready.Status != metav1.ConditionFalse || !blockingReasons[ready.Reason] {
		return "", ""
	}
	return ready.Reason, ready.Message
}
