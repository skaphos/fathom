/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/pkg/adapter"
)

// A registry resolution is safe to execute only while the live definition
// still describes the exact source revision and adapter version that produced
// it. Otherwise the live authority fence could attribute an old adapter's
// result to a newer definition it never evaluated.
func TestPreRunFenceRejectsASnapshotThatDoesNotMatchTheLiveDefinition(t *testing.T) {
	tests := []struct {
		name    string
		select_ func(*runtimeCheckFixture)
		mutate  func(*runtimeCheckFixture)
	}{
		{
			name: "definition UID",
			mutate: func(f *runtimeCheckFixture) {
				definition := f.definition()
				definition.UID = "replacement-definition-uid"
				f.update(definition)
				binding := f.binding()
				binding.Spec.DefinitionRef.UID = string(definition.UID)
				binding.Generation++
				f.update(binding)
			},
		},
		{
			name: "definition generation",
			mutate: func(f *runtimeCheckFixture) {
				definition := f.definition()
				definition.Generation++
				f.update(definition)
			},
		},
		{
			name: "adapter version",
			mutate: func(f *runtimeCheckFixture) {
				definition := f.definition()
				definition.Spec.AdapterVersion = "1.1.0"
				f.update(definition)
			},
		},
		{
			name: "semantics version",
			select_: func(f *runtimeCheckFixture) {
				definition := f.definition()
				revision := runtimeRevisionForDefinition(definition)
				revision.SemanticsVersion++
				publishRuntimeDefinition(t, f, revision, definition.Spec.AdapterVersion)
			},
		},
		{
			name: "schema version",
			select_: func(f *runtimeCheckFixture) {
				definition := f.definition()
				revision := runtimeRevisionForDefinition(definition)
				revision.SchemaVersion = "v1beta99"
				publishRuntimeDefinition(t, f, revision, definition.Spec.AdapterVersion)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.admit()
			if tc.select_ == nil {
				f.publish(1)
			} else {
				tc.select_(f)
			}
			if tc.mutate != nil {
				tc.mutate(f)
			}
			f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
				return adapter.Result{Checks: []adapter.CheckResult{{
					Family: "health", Outcome: adapter.OutcomePass, Summary: "stale adapter ran",
				}}}, nil
			})

			attempt := f.runOK()
			if attempt.Completed || attempt.Published || attempt.Reason != reasonSuperseded {
				t.Fatalf("mismatched snapshot attempt = %+v, want an unpublished Superseded refusal", attempt)
			}
			if _, _, _, runs := f.counters(); runs != 0 {
				t.Fatalf("the selected stale adapter ran %d times", runs)
			}
			if evidence := f.evidence(); evidence != nil {
				t.Fatalf("a mismatched snapshot stored completed evidence: %+v", evidence)
			}
			if reports := f.reports(); len(reports) != 0 {
				t.Fatalf("a mismatched snapshot created %d history reports", len(reports))
			}

			live := f.definition()
			f.registry.RemoveRuntime(lifecycleDefUID)
			publishRuntimeDefinition(t, f, runtimeRevisionForDefinition(live), live.Spec.AdapterVersion)
			recovered := f.runOK()
			if !recovered.Completed || !recovered.Published || recovered.Reason != reasonRunCompleted {
				t.Fatalf("matching republished snapshot did not recover: %+v", recovered)
			}
			if _, _, _, runs := f.counters(); runs != 1 {
				t.Fatalf("matching republished adapter ran %d times, want 1", runs)
			}
		})
	}
}

// The initial control reader cannot read any ServiceAccount. The final fence
// must narrow a fresh view to the account named by the still-current binding;
// this proves the narrowed read both succeeds and detects a replacement.
func TestFinalFenceUsesTheValidatedBindingServiceAccount(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{Checks: []adapter.CheckResult{{
			Family: "health", Outcome: adapter.OutcomePass, Summary: "evaluation completed",
		}}}, nil
	})
	f.runner.barrier = func(ctx context.Context, phase string) {
		if phase != runtimeBarrierAfterExecute {
			return
		}
		if err := f.store.Delete(ctx, lifecycleServiceAccount()); err != nil {
			t.Fatalf("delete service account: %v", err)
		}
		if err := f.store.Create(ctx, lifecycleServiceAccountNamed(lifecycleSAName, "replacement-service-account-uid")); err != nil {
			t.Fatalf("recreate service account: %v", err)
		}
	}

	attempt := f.runOK()
	if attempt.Completed || attempt.Published || attempt.Reason != reasonBindingMismatch {
		t.Fatalf("service account replacement attempt = %+v, want unpublished BindingMismatch", attempt)
	}
	if _, _, _, runs := f.counters(); runs != 1 {
		t.Fatalf("evaluator ran %d times before the final authority fence, want 1", runs)
	}
	if evidence := f.evidence(); evidence != nil {
		t.Fatalf("service account replacement published evidence: %+v", evidence)
	}
}

func runtimeRevisionForDefinition(definition *fathomv1alpha1.AddonDefinition) registry.RuntimeRevision {
	return registry.RuntimeRevision{
		DefinitionUID:    definition.UID,
		Generation:       definition.Generation,
		SchemaVersion:    fathomv1alpha1.GroupVersion.Version,
		SemanticsVersion: definition.Spec.SemanticsVersion,
	}
}

func publishRuntimeDefinition(
	t *testing.T, f *runtimeCheckFixture, revision registry.RuntimeRevision, adapterVersion string,
) {
	t.Helper()
	if err := f.registry.SetRuntime(registry.RuntimeEntry{
		Adapter:  f.stub,
		Revision: revision,
		Provenance: registry.RuntimeProvenance{
			OperatorBuild: lifecycleBuild, AdapterVersion: adapterVersion,
		},
	}); err != nil {
		t.Fatalf("publish runtime definition: %v", err)
	}
}
