/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/go-logr/logr"

	"github.com/skaphos/fathom/pkg/adapter"
)

// gauge builds an example.io/v1 Gauge CR whose status.state.phase carries the
// given value (value == "" leaves the field unset).
func gauge(namespace, name, value string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.io/v1",
		"kind":       "Gauge",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}}
	if value != "" {
		_ = unstructured.SetNestedField(obj.Object, value, "status", "state", "phase")
	}
	return obj
}

func gaugeCheck() FieldCheck {
	return FieldCheck{
		APIVersion:    "example.io/v1",
		Kind:          "Gauge",
		ListKind:      "GaugeList",
		ListName:      "gauges",
		FieldPath:     []string{"status", "state", "phase"},
		ExpectedValue: "Steady",
		ValueOutcomes: map[string]adapter.Outcome{
			"Broken":   adapter.OutcomeFail,
			"Settling": adapter.OutcomeWarn,
		},
	}
}

// runFields runs an engine whose single enabled family carries fc as its only
// Fields evaluator, against objs.
func runFields(t *testing.T, fc FieldCheck, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	eng := MustEngine(AddonDefinition{
		AddonType:      "gauges",
		AdapterVersion: "0.0.1",
		Families: []FamilyDefinition{{
			Name:           "gauge_health",
			DefaultEnabled: true,
			Fields:         []FieldCheck{fc},
		}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Checks
}

// Per-object scoring is table-tested directly against scoreObject (a pure
// function of the object, like ConditionCheck's); Evaluate's list-level paths
// (skip / absent API) are covered through the engine and a NoMatch client.

func TestField_ScoreObject(t *testing.T) {
	fc := gaugeCheck()
	ec := EvalContext{Family: "gauge_health"}

	cases := []struct {
		name            string
		value           string
		wantOutcome     adapter.Outcome
		wantSummary     string
		wantValueDetail string
	}{
		{name: "expected value passes", value: "Steady", wantOutcome: adapter.OutcomePass, wantSummary: "status.state.phase is Steady", wantValueDetail: "Steady"},
		{name: "mapped fail value fails", value: "Broken", wantOutcome: adapter.OutcomeFail, wantSummary: `want "Steady"`, wantValueDetail: "Broken"},
		{name: "mapped warn value warns", value: "Settling", wantOutcome: adapter.OutcomeWarn, wantSummary: `want "Steady"`, wantValueDetail: "Settling"},
		{name: "unmapped value defaults to warn", value: "Sideways", wantOutcome: adapter.OutcomeWarn, wantSummary: `want "Steady"`, wantValueDetail: "Sideways"},
		{name: "absent field defaults to warn", value: "", wantOutcome: adapter.OutcomeWarn, wantSummary: "is not set"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := []adapter.CheckResult{fc.scoreObject(ec, gauge("default", "g", tc.value), time.Now())}
			assertHasOutcome(t, got, "Gauge", "g", tc.wantOutcome, tc.wantSummary)
			assertHasDetail(t, got, "Gauge", "g", "field", "status.state.phase")
			if tc.wantValueDetail != "" {
				assertHasDetail(t, got, "Gauge", "g", "value", tc.wantValueDetail)
			}
			assertFamily(t, got, "Gauge", "g", adapter.Family("gauge_health"))
		})
	}
}

func TestField_ScoreObjectCustomOutcomes(t *testing.T) {
	fc := gaugeCheck()
	fc.AbsentOutcome = adapter.OutcomeFail
	fc.OtherOutcome = adapter.OutcomeFail
	ec := EvalContext{Family: "gauge_health"}

	absent := []adapter.CheckResult{fc.scoreObject(ec, gauge("default", "g1", ""), time.Now())}
	assertHasOutcome(t, absent, "Gauge", "g1", adapter.OutcomeFail, "is not set")

	other := []adapter.CheckResult{fc.scoreObject(ec, gauge("default", "g2", "Sideways"), time.Now())}
	assertHasOutcome(t, other, "Gauge", "g2", adapter.OutcomeFail, `want "Steady"`)
	assertHasDetail(t, other, "Gauge", "g2", "expectedValue", "Steady")
}

func TestField_ScoreObjectNonStringValueTreatedAbsent(t *testing.T) {
	obj := gauge("default", "g", "")
	_ = unstructured.SetNestedField(obj.Object, int64(42), "status", "state", "phase")
	got := []adapter.CheckResult{gaugeCheck().scoreObject(EvalContext{Family: "gauge_health"}, obj, time.Now())}
	assertHasOutcome(t, got, "Gauge", "g", adapter.OutcomeWarn, "is not set")
}

func TestField_NoMatchingObjectsSkipped(t *testing.T) {
	checks := runFields(t, gaugeCheck())
	assertHasOutcome(t, checks, "Gauge", "gauges", adapter.OutcomeSkipped, "no Gauge objects matched")
	assertHasDetail(t, checks, "Gauge", "gauges", "skipReason", "NoMatchingObjects")
	assertHasDetail(t, checks, "Gauge", "gauges", "field", "status.state.phase")
}

func TestField_ListedObjectsScored(t *testing.T) {
	checks := runFields(t, gaugeCheck(),
		gauge("team-a", "ok", "Steady"),
		gauge("team-b", "bad", "Broken"))
	assertHasOutcome(t, checks, "Gauge", "ok", adapter.OutcomePass, "Steady")
	assertHasOutcome(t, checks, "Gauge", "bad", adapter.OutcomeFail, `want "Steady"`)
}

func TestField_NoMatchUsesAddonAbsencePosture(t *testing.T) {
	fc := gaugeCheck()
	base := newFakeClient(t)

	for _, tc := range []struct {
		name    string
		posture Posture
		want    adapter.Outcome
	}{
		{name: "required", posture: Required, want: adapter.OutcomeFail},
		{name: "optional", posture: Optional, want: adapter.OutcomeSkipped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checks, err := fc.Evaluate(EvalContext{
				Ctx:            context.Background(),
				Client:         noMatchListClient{Client: base},
				Family:         "gauge_health",
				DefaultPosture: tc.posture,
			})
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			assertHasOutcome(t, checks, "Gauge", "gauges", tc.want, "API is not installed")
			assertHasDetail(t, checks, "Gauge", "gauges", adapter.DetailAbsent, "true")
		})
	}
}

func TestField_InvalidSelectorErrors(t *testing.T) {
	// A malformed labelSelector must surface as an Error result, not abort the Run.
	eng := MustEngine(AddonDefinition{
		AddonType:      "gauges",
		AdapterVersion: "0.0.1",
		Families: []FamilyDefinition{{
			Name:           "gauge_health",
			DefaultEnabled: true,
			Fields:         []FieldCheck{gaugeCheck()},
		}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t),
		Logger: logr.Discard(),
		Policy: map[adapter.Family]adapter.FamilyPolicy{
			"gauge_health": {Enabled: true, LabelSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "k", Operator: "Nonsense"}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertHasOutcome(t, res.Checks, "Gauge", "gauges", adapter.OutcomeError, "invalid label selector")
}
