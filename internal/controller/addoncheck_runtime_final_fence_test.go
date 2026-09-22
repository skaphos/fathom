/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/skaphos/fathom/pkg/adapter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
)

// Objects can become ineligible after evaluation without changing the identity
// fields already captured by the run. The final fence must reject those states
// before publishing and leave the last completed observation and history intact.
func TestFinalFenceRevalidatesDefinitionAndServiceAccountEligibility(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*runtimeCheckFixture)
		mutate  func(context.Context, *runtimeCheckFixture)
		reason  string
	}{
		{
			name: "definition starts deleting",
			prepare: func(f *runtimeCheckFixture) {
				definition := f.definition()
				definition.Finalizers = []string{"fathom.skaphos.io/final-fence-test"}
				f.update(definition)
			},
			mutate: func(ctx context.Context, f *runtimeCheckFixture) {
				if err := f.store.Delete(ctx, f.definition()); err != nil {
					f.t.Fatalf("delete AddonDefinition: %v", err)
				}
			},
			reason: reasonDefinitionUnavailable,
		},
		{
			name: "service account starts deleting",
			prepare: func(f *runtimeCheckFixture) {
				account := runtimeServiceAccount(t, f)
				account.Finalizers = []string{"fathom.skaphos.io/final-fence-test"}
				f.update(account)
			},
			mutate: func(ctx context.Context, f *runtimeCheckFixture) {
				if err := f.store.Delete(ctx, runtimeServiceAccount(t, f)); err != nil {
					f.t.Fatalf("delete ServiceAccount: %v", err)
				}
			},
			reason: reasonBindingMismatch,
		},
		{
			name: "service account becomes reserved for a built-in",
			mutate: func(_ context.Context, f *runtimeCheckFixture) {
				account := runtimeServiceAccount(t, f)
				account.Labels = map[string]string{adapter.AddonLabel: "reserved"}
				f.update(account)
			},
			reason: reasonBindingMismatch,
		},
		{
			name:   "unchanged eligible objects",
			mutate: func(context.Context, *runtimeCheckFixture) {},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			if tc.prepare != nil {
				tc.prepare(f)
			}
			if _, report := f.runAndRecord(); report == "" {
				t.Fatal("seed run created no history")
			}
			beforeEvidence := f.evidence().DeepCopy()
			beforeReports := f.reports()
			f.advance(time.Minute)
			f.runner.barrier = func(ctx context.Context, phase string) {
				if phase == runtimeBarrierAfterExecute {
					tc.mutate(ctx, f)
				}
			}

			attempt, report := f.runAndRecord()
			if tc.reason == "" {
				if !attempt.Completed || !attempt.Published || attempt.Reason != reasonRunCompleted {
					t.Fatalf("eligible attempt = %+v, want published completion", attempt)
				}
				if evidence := f.evidence(); evidence == nil || !evidence.ObservedAt.After(beforeEvidence.ObservedAt.Time) {
					t.Fatalf("eligible run did not advance evidence: before=%+v after=%+v", beforeEvidence, evidence)
				}
				return
			}

			if attempt.Completed || attempt.Published || attempt.Reason != tc.reason {
				t.Fatalf("ineligible attempt = %+v, want unpublished %s", attempt, tc.reason)
			}
			if report != "" {
				t.Fatalf("ineligible attempt created report %q", report)
			}
			if evidence := f.evidence(); !equality.Semantic.DeepEqual(evidence, beforeEvidence) {
				t.Fatalf("ineligible attempt replaced evidence: before=%+v after=%+v", beforeEvidence, evidence)
			}
			if reports := f.reports(); !equality.Semantic.DeepEqual(reports, beforeReports) {
				t.Fatalf("ineligible attempt changed history: before=%+v after=%+v", beforeReports, reports)
			}
		})
	}
}

func runtimeServiceAccount(t *testing.T, f *runtimeCheckFixture) *corev1.ServiceAccount {
	t.Helper()
	var account corev1.ServiceAccount
	key := types.NamespacedName{Namespace: lifecycleNamespace, Name: lifecycleSAName}
	if err := f.store.Get(context.Background(), key, &account); err != nil {
		t.Fatalf("get ServiceAccount: %v", err)
	}
	return &account
}
