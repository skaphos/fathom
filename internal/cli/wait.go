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

// waitForRun polls the target until the operator has consumed exactly this
// token. Every controller writes status.lastRunTrigger in the same status
// update as the run's verdict, so equality on the token is equality on
// completion. If the annotation stops equalling the token, a later trigger
// superseded this one and waiting further would never end.
func (f *factory) waitForRun(ctx context.Context, c client.Client, t runTarget, token string, timeout time.Duration) waitResult {
	var res waitResult
	key := types.NamespacedName{Namespace: t.ref.Namespace, Name: t.ref.Name}
	err := wait.PollUntilContextTimeout(ctx, f.pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		obj := t.ref.Kind.New()
		if err := c.Get(ctx, key, obj); err != nil {
			return false, fmt.Errorf("re-read %s: %w", t.ref, err)
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
