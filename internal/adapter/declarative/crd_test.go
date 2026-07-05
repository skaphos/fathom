/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"testing"

	"github.com/skaphos/fathom/pkg/adapter"
)

// crdAbsenceEngine builds a single-CRD crd_health family with the given component
// Posture (blank inherits the addon default) and addon-level Optional, to exercise
// CRD absence resolution and the per-component override (SKA-526).
func crdAbsenceEngine(component Posture, addonOptional bool) *Engine {
	return MustEngine(AddonDefinition{
		AddonType:      "app",
		AdapterVersion: "0.0.1",
		Optional:       addonOptional,
		Families: []FamilyDefinition{{
			Name:           "crd_health",
			DefaultEnabled: true,
			CRDs: []CRDCheck{{
				Names:             []string{"widgets.example.com"},
				SupportedVersions: []string{"v1"},
				Absence:           component,
			}},
		}},
	})
}

// TestCRD_AbsenceResolution mirrors TestWorkload_AbsenceResolution for CRDCheck: an
// unset Posture is Required by default (Fail), the addon-level Optional flips the
// default to Skipped, and a component Posture always wins over the addon default.
// Every path, Fail or Skipped, tags the NotFound CRD with the absent marker so
// "not installed" stays queryable independent of the verdict (SKA-526).
func TestCRD_AbsenceResolution(t *testing.T) {
	tests := []struct {
		name          string
		component     Posture
		addonOptional bool
		want          adapter.Outcome
	}{
		{"unset posture defaults to Required -> Fail", "", false, adapter.OutcomeFail},
		{"addon Optional makes unset posture Skipped", "", true, adapter.OutcomeSkipped},
		{"component Required overrides addon Optional", Required, true, adapter.OutcomeFail},
		{"component Optional overrides required default", Optional, false, adapter.OutcomeSkipped},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checks := runEngine(t, crdAbsenceEngine(tc.component, tc.addonOptional), nil) // no objects
			assertHasOutcome(t, checks, "CustomResourceDefinition", "widgets.example.com", tc.want, "not found")
			// Every absence path, Fail or Skipped, carries the absent marker.
			assertHasDetail(t, checks, "CustomResourceDefinition", "widgets.example.com", adapter.DetailAbsent, "true")
		})
	}
}
