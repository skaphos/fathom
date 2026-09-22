/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
)

func TestRuntimeDelegateErrorsCannotControlPersistedFailureText(t *testing.T) {
	const hostile = "AttackerChosenReason: bearer-token=secret response=<attacker body>"
	for _, tc := range []struct {
		name       string
		err        error
		wantReason string
		wantText   string
	}{
		{
			name:       "unrecognized prefix cannot invent a reason",
			err:        errors.New(hostile),
			wantReason: "ExecutionFailed",
			wantText:   "runtime evaluation failed; no new evidence was published",
		},
		{
			name:       "recognized prefix retains only its classification",
			err:        errors.New("AccessDenied: bearer-token=secret response=<attacker body>"),
			wantReason: reasonAccessDenied,
			wantText:   "runtime authorization failed; no new evidence was published",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
				return adapter.Result{}, tc.err
			})

			attempt := f.runOK()
			if attempt.Reason != tc.wantReason || attempt.Message != tc.wantText {
				t.Fatalf("attempt = %q/%q, want %q/%q", attempt.Reason, attempt.Message, tc.wantReason, tc.wantText)
			}
			status := f.check().Status
			ready := runtimeReadyCondition(t, f.check())
			if status.LatestAttemptReason != tc.wantReason || status.LatestAttemptMessage != tc.wantText {
				t.Errorf("latest attempt = %q/%q, want %q/%q",
					status.LatestAttemptReason, status.LatestAttemptMessage, tc.wantReason, tc.wantText)
			}
			if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != tc.wantReason || ready.Message != tc.wantText {
				t.Errorf("Ready = %+v, want False/%s/%q", ready, tc.wantReason, tc.wantText)
			}
			for _, persisted := range []string{attempt.Message, status.LatestAttemptReason, status.LatestAttemptMessage} {
				if strings.Contains(persisted, "secret") || strings.Contains(persisted, "attacker") {
					t.Errorf("untrusted delegate text reached an attempt or status field: %q", persisted)
				}
			}
		})
	}
}

func TestRuntimeFailureCandidatesRetainOnlyTrustedContext(t *testing.T) {
	t.Run("bounded execution detail", func(t *testing.T) {
		candidate := candidateFor(&execution.Failure{Reason: "ScopeDenied", Detail: "request is outside the declared scope"})
		if candidate.reason != "ScopeDenied" || candidate.message != "request is outside the declared scope" {
			t.Fatalf("candidate = %q/%q", candidate.reason, candidate.message)
		}
	})

	t.Run("authority cause is excluded", func(t *testing.T) {
		candidate := candidateFor(&authorityFailure{
			Reason: reasonAccessDenied, Message: "cannot read AddonCheck default/example",
			Cause: errors.New("bearer-token=secret response=<attacker body>"),
		})
		if candidate.reason != reasonAccessDenied || candidate.message != "cannot read AddonCheck default/example" {
			t.Fatalf("candidate = %q/%q", candidate.reason, candidate.message)
		}
	})
}
