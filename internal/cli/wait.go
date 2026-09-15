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
	// NodeHealthCheck: an item the operator's allowlist refuses (an object stored
	// under an older CRD). No agent is provisioned and the trigger is never
	// consumed, so waiting can only time out.
	"ItemsRejected":       true,
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
		return pollRun(ctx, c, t, key, token, &res)
	})
	return classifyWaitResult(res, err)
}

// waitForRunWithDeadlineGrowth anchors the deadline to the original start.
// During an implicit NodeHealthCheck wait, a newly observed larger fleet may
// increase the budget; unchanged or smaller status can never move it again.
// Explicit timeouts pass grow=false and remain absolute.
func (f *factory) waitForRunWithDeadlineGrowth(ctx context.Context, c client.Client, t runTarget, token string, timeout time.Duration, grow bool, now func() time.Time) waitResult {
	if !grow || t.ref.Kind.Kind != "NodeHealthCheck" {
		return f.waitForRun(ctx, c, t, token, timeout)
	}

	var res waitResult
	key := types.NamespacedName{Namespace: t.ref.Namespace, Name: t.ref.Name}
	started := now()
	budget := timeout
	deadlineCtx, cancelDeadline := context.WithCancel(ctx)
	deadlineTimer := time.AfterFunc(budget, cancelDeadline)
	defer func() {
		deadlineTimer.Stop()
		cancelDeadline()
	}()

	for {
		if deadlineCtx.Err() != nil {
			return waitResult{timedOut: true}
		}
		obj := t.ref.Kind.New()
		if err := c.Get(deadlineCtx, key, obj); err != nil {
			if deadlineCtx.Err() != nil {
				return waitResult{timedOut: true}
			}
			if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return waitResult{err: fmt.Errorf("re-read %s: %w", t.ref, err)}
			}
		} else {
			observed := saturatingDurationAdd(t.ref.Kind.DefaultWaitEstimate(obj), waitMargin)
			if observed > budget {
				budget = observed
				remaining := budget - now().Sub(started)
				if remaining <= 0 {
					return waitResult{timedOut: true}
				}
				if !deadlineTimer.Reset(remaining) && deadlineCtx.Err() != nil {
					return waitResult{timedOut: true}
				}
			}
			if now().Sub(started) >= budget {
				return waitResult{timedOut: true}
			}
			done, err := inspectRunObject(t, obj, token, &res)
			if err != nil {
				return waitResult{err: err}
			}
			if done {
				return res
			}
		}

		timer := time.NewTimer(f.pollInterval)
		select {
		case <-deadlineCtx.Done():
			timer.Stop()
			return waitResult{timedOut: true}
		case <-timer.C:
		}
	}
}

func pollRun(ctx context.Context, c client.Client, t runTarget, key types.NamespacedName, token string, res *waitResult) (bool, error) {
	obj := t.ref.Kind.New()
	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return false, fmt.Errorf("re-read %s: %w", t.ref, err)
		}
		return false, nil
	}
	return inspectRunObject(t, obj, token, res)
}

func inspectRunObject(t runTarget, obj client.Object, token string, res *waitResult) (bool, error) {
	if obj.GetAnnotations()[fathomv1alpha1.AnnotationRunNow] != token {
		res.superseded = true
		return true, nil
	}
	snap := t.ref.Kind.Snapshot(obj)
	if snap.ConsumedTrigger == token {
		res.snap = snap
		return true, nil
	}
	if reason, msg := blockedBy(conditionsOf(obj), obj.GetGeneration()); reason != "" {
		return false, fmt.Errorf("the operator cannot run %s (%s): %s", t.ref, reason, msg)
	}
	return false, nil
}

func classifyWaitResult(res waitResult, err error) waitResult {
	switch {
	case err == nil:
		return res
	case errors.Is(err, context.DeadlineExceeded) || wait.Interrupted(err):
		return waitResult{timedOut: true}
	default:
		return waitResult{err: err}
	}
}

// blockedBy returns a current-generation Ready=False reason when it is one the
// operator will never recover from without a spec change. A prior generation's
// condition must not abort a run while the new spec is still reconciling.
func blockedBy(conds []metav1.Condition, generation int64) (string, string) {
	ready := apimeta.FindStatusCondition(conds, readyCondition)
	if ready == nil || ready.ObservedGeneration != generation || ready.Status != metav1.ConditionFalse || !blockingReasons[ready.Reason] {
		return "", ""
	}
	return ready.Reason, ready.Message
}
