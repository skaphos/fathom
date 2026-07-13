/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"

	"github.com/skaphos/fathom/pkg/adapter"
)

// widget builds an example.io/v1 Widget managed CR with an optional
// status.conditions[condType]=status entry (condType == "" leaves the object
// with no conditions).
func widget(namespace, name, condType, status string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.io/v1",
		"kind":       "Widget",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}}
	if condType != "" {
		_ = unstructured.SetNestedSlice(obj.Object,
			[]any{map[string]any{"type": condType, "status": status}},
			"status", "conditions")
	}
	return obj
}

func widgetCheck() ConditionCheck {
	return ConditionCheck{
		APIVersion:       "example.io/v1",
		Kind:             "Widget",
		ListKind:         "WidgetList",
		ListName:         "widgets",
		DefaultNamespace: "default",
		ConditionType:    "Ready",
		ExpectedStatus:   "True",
	}
}

// runManaged runs an engine whose single enabled family carries cc as its only
// ManagedResources evaluator, against objs and the given policy.
func runManaged(t *testing.T, cc ConditionCheck, policy map[adapter.Family]adapter.FamilyPolicy, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	eng := MustEngine(AddonDefinition{
		AddonType:      "widgets",
		AdapterVersion: "0.0.1",
		Families: []FamilyDefinition{{
			Name:             "widget_health",
			DefaultEnabled:   true,
			ManagedResources: []ConditionCheck{cc},
		}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Policy: policy,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Checks
}

// Per-object scoring is tested directly against scoreObject / conditionStatus:
// the controller-runtime fake client's unstructured List is unreliable for these
// dynamic CRs, but scoreObject is a pure function of the object. Evaluate's
// list-level paths (skip / error) are covered through the engine below.

func TestCondition_NilSelectorMatchesAll(t *testing.T) {
	// Per the FamilyPolicy contract a nil LabelSelector means "no label-based
	// narrowing" (match all). metav1.LabelSelectorAsSelector maps nil to
	// Nothing(), so the engine must special-case it — otherwise a family enabled
	// without a labelSelector would silently score zero managed resources.
	checks := runManaged(t, widgetCheck(), nil, widget("default", "w1", "Ready", "True"))
	assertHasOutcome(t, checks, "Widget", "w1", adapter.OutcomePass, "Ready")
	assertNoOutcome(t, checks, "Widget", "widgets", adapter.OutcomeSkipped)
}

func TestCondition_ScoreObject(t *testing.T) {
	cc := widgetCheck()
	ec := EvalContext{Family: "widget_health"}
	one := func(c adapter.CheckResult) []adapter.CheckResult { return []adapter.CheckResult{c} }

	pass := cc.scoreObject(ec, widget("default", "w1", "Ready", "True"), adapter.OutcomeFail, adapter.OutcomeFail, time.Now())
	assertHasOutcome(t, one(pass), "Widget", "w1", adapter.OutcomePass, "Ready")
	assertHasDetail(t, one(pass), "Widget", "w1", "status", "True")
	assertFamily(t, one(pass), "Widget", "w1", adapter.Family("widget_health"))

	mismatch := cc.scoreObject(ec, widget("default", "w2", "Ready", "False"), adapter.OutcomeFail, adapter.OutcomeFail, time.Now())
	assertHasOutcome(t, one(mismatch), "Widget", "w2", adapter.OutcomeFail, "want")
	assertHasDetail(t, one(mismatch), "Widget", "w2", "expectedStatus", "True")

	absentFail := cc.scoreObject(ec, widget("default", "w3", "", ""), adapter.OutcomeFail, adapter.OutcomeFail, time.Now())
	assertHasOutcome(t, one(absentFail), "Widget", "w3", adapter.OutcomeFail, "absent")

	// Custom outcomes: absent -> Warn, mismatch -> Warn.
	absentWarn := cc.scoreObject(ec, widget("default", "w4", "", ""), adapter.OutcomeWarn, adapter.OutcomeFail, time.Now())
	assertHasOutcome(t, one(absentWarn), "Widget", "w4", adapter.OutcomeWarn, "absent")
	mismatchWarn := cc.scoreObject(ec, widget("default", "w5", "Ready", "False"), adapter.OutcomeFail, adapter.OutcomeWarn, time.Now())
	assertHasOutcome(t, one(mismatchWarn), "Widget", "w5", adapter.OutcomeWarn, "want")
}

func TestCondition_ResolveVersion(t *testing.T) {
	cc := ConditionCheck{
		APIVersion:        "external-secrets.io/v1",
		VersionCRD:        "externalsecrets.external-secrets.io",
		SupportedVersions: []string{"v1", "v1beta1"},
	}
	// CRD serves only the legacy version -> list at v1beta1, not the v1 fallback.
	legacy := EvalContext{Ctx: context.Background(),
		Client: newFakeClient(t, establishedCRD("externalsecrets.external-secrets.io", "v1beta1", true, true))}
	if got := cc.resolveVersion(legacy, "v1"); got != "v1beta1" {
		t.Errorf("resolveVersion(v1beta1-only CRD) = %q, want v1beta1", got)
	}
	// No CRD present -> fall back to the passed default.
	absent := EvalContext{Ctx: context.Background(), Client: newFakeClient(t)}
	if got := cc.resolveVersion(absent, "v1"); got != "v1" {
		t.Errorf("resolveVersion(absent CRD) = %q, want v1 fallback", got)
	}
	// VersionCRD unset -> fall back verbatim without reading any CRD.
	if got := (ConditionCheck{}).resolveVersion(legacy, "v1"); got != "v1" {
		t.Errorf("resolveVersion(no VersionCRD) = %q, want v1", got)
	}
}

func TestCondition_ConditionStatus(t *testing.T) {
	if s, ok := conditionStatus(widget("default", "w1", "Ready", "True"), "Ready"); !ok || s != "True" {
		t.Fatalf("conditionStatus present: got (%q,%v), want (True,true)", s, ok)
	}
	if _, ok := conditionStatus(widget("default", "w2", "", ""), "Ready"); ok {
		t.Fatal("conditionStatus absent: got ok=true, want false")
	}
	if _, ok := conditionStatus(widget("default", "w3", "Synced", "True"), "Ready"); ok {
		t.Fatal("conditionStatus wrong type: got ok=true, want false")
	}
}

func TestCondition_NoMatchingObjectsSkipped(t *testing.T) {
	checks := runManaged(t, widgetCheck(), nil) // no objects
	assertHasOutcome(t, checks, "Widget", "widgets", adapter.OutcomeSkipped, "no Widget objects matched")
	assertHasDetail(t, checks, "Widget", "widgets", "skipReason", "NoMatchingObjects")
}

func TestCondition_ListNameFallsBackToKind(t *testing.T) {
	cc := widgetCheck()
	cc.ListName = "" // list-level results should fall back to Kind
	checks := runManaged(t, cc, nil)
	assertHasOutcome(t, checks, "Widget", "Widget", adapter.OutcomeSkipped, "")
}

func TestCondition_InvalidSelectorErrors(t *testing.T) {
	policy := map[adapter.Family]adapter.FamilyPolicy{
		"widget_health": {Enabled: true, LabelSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "k", Operator: "Nonsense"}},
		}},
	}
	checks := runManaged(t, widgetCheck(), policy)
	assertHasOutcome(t, checks, "Widget", "widgets", adapter.OutcomeError, "invalid label selector")
}

func TestCondition_InvalidAPIVersionErrors(t *testing.T) {
	cc := widgetCheck()
	cc.APIVersion = "a/b/c" // unparseable group/version
	checks := runManaged(t, cc, nil)
	assertHasOutcome(t, checks, "Widget", "widgets", adapter.OutcomeError, "invalid apiVersion")
}

func TestPolicyNamespaceResolution(t *testing.T) {
	// policyNamespaces + firstNamespace are pure; test them directly (the fake
	// client mishandles multiple same-GVK unstructured objects, so an engine-
	// level multi-namespace list can't be exercised reliably).
	if got := policyNamespaces(adapter.FamilyPolicy{}, "kube-system"); len(got) != 1 || got[0] != "" {
		t.Fatalf("empty policy: got %v, want an all-namespaces selector", got)
	}
	if got := policyNamespaces(adapter.FamilyPolicy{Namespaces: []string{"a", "b"}}, "kube-system"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("explicit namespaces: got %v, want [a b]", got)
	}
	if got := firstNamespace(adapter.FamilyPolicy{Namespaces: []string{"x", "y"}}, "def"); got != "x" {
		t.Fatalf("firstNamespace explicit: got %q, want x", got)
	}
	if got := firstNamespace(adapter.FamilyPolicy{}, "def"); got != "def" {
		t.Fatalf("firstNamespace default: got %q, want def", got)
	}
}

type noMatchListClient struct{ client.Client }

func (c noMatchListClient) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return &apimeta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{
		Group: "example.io", Version: "v1", Resource: "widgets",
	}}
}

func (c noMatchListClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return &apimeta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "example.io", Kind: "Widget"}}
}

func TestCondition_NoMatchUsesAddonAbsencePosture(t *testing.T) {
	cc := widgetCheck()
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
			checks, err := cc.Evaluate(EvalContext{
				Ctx:            context.Background(),
				Client:         noMatchListClient{Client: base},
				Family:         "widget_health",
				DefaultPosture: tc.posture,
			})
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			assertHasOutcome(t, checks, "Widget", "widgets", tc.want, "API is not installed")
			assertHasDetail(t, checks, "Widget", "widgets", adapter.DetailAbsent, "true")
		})
	}
}

func TestResourceAbsent_DoesNotTreatMissingNamespaceAsMissingAPI(t *testing.T) {
	err := apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "missing")
	if resourceAbsent(err) {
		t.Fatal("missing namespace must remain an evaluation error, not addon absence")
	}
}

func TestCondition_NamedNoMatchUsesAddonAbsencePosture(t *testing.T) {
	cc := widgetCheck()
	cc.Names = []string{"primary"}
	checks, err := cc.Evaluate(EvalContext{
		Ctx:            context.Background(),
		Client:         noMatchListClient{Client: newFakeClient(t)},
		Family:         "widget_health",
		DefaultPosture: Optional,
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertHasOutcome(t, checks, "Widget", "primary", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, checks, "Widget", "primary", adapter.DetailAbsent, "true")
}

func TestCondition_ClusterScopedListsWithoutNamespace(t *testing.T) {
	cc := widgetCheck()
	cc.ClusterScoped = true
	// Exercises the cluster-scoped (no-namespace) list path. The fake client's
	// cluster-scoped unstructured handling differs from a real API server, so we
	// assert the branch runs and yields the list-level Skipped when nothing
	// matches, rather than a specific object hit.
	checks := runManaged(t, cc, nil)
	assertHasOutcome(t, checks, "Widget", "widgets", adapter.OutcomeSkipped, "no Widget objects matched")
}

// --- named (get-by-name) mode ---

func TestCondition_NamedGetScoresFoundObject(t *testing.T) {
	cc := widgetCheck()
	cc.Names = []string{"w1"}
	checks := runManaged(t, cc, nil, widget("default", "w1", "Ready", "True"))
	assertHasOutcome(t, checks, "Widget", "w1", adapter.OutcomePass, "Ready")

	cc.Names = []string{"w2"}
	checks = runManaged(t, cc, nil, widget("default", "w2", "Ready", "False"))
	assertHasOutcome(t, checks, "Widget", "w2", adapter.OutcomeFail, "want")
}

func TestCondition_NamedNotFoundScoredByPosture(t *testing.T) {
	cc := widgetCheck()
	cc.Names = []string{"w1"}

	// Required (the default): a missing named singleton is a Fail — never the
	// list mode's NoMatchingObjects skip — and carries the absent marker.
	checks := runManaged(t, cc, nil)
	assertHasOutcome(t, checks, "Widget", "w1", adapter.OutcomeFail, "not found")
	assertHasDetail(t, checks, "Widget", "w1", adapter.DetailAbsent, "true")

	// The explicit Optional opt-out scores Skipped, still with the marker.
	cc.Absence = Optional
	checks = runManaged(t, cc, nil)
	assertHasOutcome(t, checks, "Widget", "w1", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, checks, "Widget", "w1", adapter.DetailAbsent, "true")
}

func TestCondition_NamedIgnoresLabelSelector(t *testing.T) {
	// policy.LabelSelector does not apply to named gets (like WorkloadCheck):
	// a malformed selector must not error a check that never uses it.
	cc := widgetCheck()
	cc.Names = []string{"w1"}
	policy := map[adapter.Family]adapter.FamilyPolicy{
		"widget_health": {Enabled: true, LabelSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "k", Operator: "Nonsense"}},
		}},
	}
	checks := runManaged(t, cc, policy, widget("default", "w1", "Ready", "True"))
	assertHasOutcome(t, checks, "Widget", "w1", adapter.OutcomePass, "Ready")
}

func TestCondition_NamedClusterScopedGet(t *testing.T) {
	// The metrics-server shape: a cluster-scoped named APIService.
	cc := ConditionCheck{
		APIVersion:     "apiregistration.k8s.io/v1",
		Kind:           "APIService",
		ListKind:       "APIServiceList",
		ListName:       "apiservices",
		ClusterScoped:  true,
		Names:          []string{"v1beta1.metrics.k8s.io"},
		ConditionType:  "Available",
		ExpectedStatus: "True",
	}
	checks := runManaged(t, cc, nil, apiService("v1beta1.metrics.k8s.io", "Available", "True"))
	assertHasOutcome(t, checks, "APIService", "v1beta1.metrics.k8s.io", adapter.OutcomePass, "Available")
}

// MustEngine panics on an invalid definition; NewEngine returns the error.
func TestMustEngine_PanicsOnInvalid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustEngine: expected panic on invalid definition")
		}
	}()
	MustEngine(AddonDefinition{}) // empty AddonType -> validation error -> panic
}
