/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"strings"
	"testing"

	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/pkg/adapter"
)

// fakePolicyAdapter is a minimal adapter.Adapter whose only meaningful surface
// is the family set it advertises, for validateAddonCheckPolicy tests.
type fakePolicyAdapter struct{ families []adapter.Family }

func (fakePolicyAdapter) Name() string            { return "fake" }
func (fakePolicyAdapter) Version() string         { return "0.0.1" }
func (fakePolicyAdapter) ContractVersion() string { return adapter.ContractVersion }
func (f fakePolicyAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"fake"}, Families: f.families}
}
func (fakePolicyAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{}, nil
}

func checkWithPolicy(policy map[string]fathomv1alpha1.AddonCheckFamilyPolicy) *fathomv1alpha1.AddonCheck {
	return &fathomv1alpha1.AddonCheck{Spec: fathomv1alpha1.AddonCheckSpec{AddonType: "fake", Policy: policy}}
}

func badSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "k", Operator: "Nonsense"}}}
}

func TestValidateAddonCheckPolicy(t *testing.T) {
	adapterWith := func(fams ...adapter.Family) adapter.Adapter { return fakePolicyAdapter{families: fams} }
	goodSelector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}}

	tests := []struct {
		name         string
		policy       map[string]fathomv1alpha1.AddonCheckFamilyPolicy
		adapter      adapter.Adapter
		wantCount    int
		wantContains []string
	}{
		{"empty policy is valid", nil, adapterWith("system_health"), 0, nil},
		{"known family, no selector", map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Enabled: ptr.To(true)}}, adapterWith("system_health"), 0, nil},
		{"known family, good selector", map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {LabelSelector: goodSelector}}, adapterWith("system_health"), 0, nil},
		{"unknown family key", map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"bogus": {Enabled: ptr.To(true)}}, adapterWith("system_health"), 1, []string{`unknown family "bogus"`}},
		{"invalid selector on known family", map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {LabelSelector: badSelector()}}, adapterWith("system_health"), 1, []string{`family "system_health" has an invalid labelSelector:`}},
		{"nil adapter skips family check but validates selector", map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"anything": {LabelSelector: badSelector()}}, nil, 1, []string{`family "anything" has an invalid labelSelector:`}},
		{"unknown family and bad selector both reported", map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"bogus": {LabelSelector: badSelector()}}, adapterWith("system_health"), 2, []string{`unknown family "bogus"`, `family "bogus" has an invalid labelSelector:`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateAddonCheckPolicy(checkWithPolicy(tc.policy), tc.adapter)
			if len(got) != tc.wantCount {
				t.Fatalf("problems: got %d %v, want %d", len(got), got, tc.wantCount)
			}
			joined := strings.Join(got, "\n")
			for _, sub := range tc.wantContains {
				if !strings.Contains(joined, sub) {
					t.Errorf("problems %v missing %q", got, sub)
				}
			}
		})
	}
}

// fakeAdvertisingAdapter additionally implements adapter.ThresholdAdvertiser.
type fakeAdvertisingAdapter struct {
	fakePolicyAdapter
	keys map[adapter.Family][]string
}

func (f fakeAdvertisingAdapter) ThresholdKeys() map[adapter.Family][]string { return f.keys }

func TestValidateAddonCheckPolicy_ThresholdKeys(t *testing.T) {
	advertising := fakeAdvertisingAdapter{
		fakePolicyAdapter: fakePolicyAdapter{families: []adapter.Family{"system_health", "cert_health"}},
		keys:              map[adapter.Family][]string{"system_health": {"restartWarnCount", "workloadName"}},
	}
	plain := fakePolicyAdapter{families: []adapter.Family{"system_health"}}

	tests := []struct {
		name         string
		policy       map[string]fathomv1alpha1.AddonCheckFamilyPolicy
		adapter      adapter.Adapter
		wantCount    int
		wantContains []string
	}{
		{
			"advertised keys accepted",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"restartWarnCount": "3"}}},
			advertising, 0, nil,
		},
		{
			"unknown key on an advertised family rejected",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"restartWarnCont": "3"}}},
			advertising, 1, []string{`family "system_health" has an unknown threshold key "restartWarnCont"`},
		},
		{
			"unknown keys reported deterministically sorted",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"zzz": "1", "aaa": "2"}}},
			advertising, 2, []string{`unknown threshold key "aaa"`, `unknown threshold key "zzz"`},
		},
		{
			"unadvertised family is not validated",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"cert_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"whatever": "x"}}},
			advertising, 0, nil,
		},
		{
			"adapter without ThresholdAdvertiser skips threshold validation",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"whatever": "x"}}},
			plain, 0, nil,
		},
		{
			"nil adapter skips threshold validation",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"anything": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"whatever": "x"}}},
			nil, 0, nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateAddonCheckPolicy(checkWithPolicy(tc.policy), tc.adapter)
			if len(got) != tc.wantCount {
				t.Fatalf("problems: got %d %v, want %d", len(got), got, tc.wantCount)
			}
			joined := strings.Join(got, "\n")
			for _, sub := range tc.wantContains {
				if !strings.Contains(joined, sub) {
					t.Errorf("problems %v missing %q", got, sub)
				}
			}
		})
	}
}

// TestValidateAddonCheckPolicy_DeterministicOrder guards the stable ordering the
// Accepted-condition message depends on (map iteration is randomized).
func TestValidateAddonCheckPolicy_DeterministicOrder(t *testing.T) {
	policy := map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
		"zeta": {Enabled: ptr.To(true)}, "alpha": {Enabled: ptr.To(true)}, "mu": {Enabled: ptr.To(true)},
	}
	adp := fakePolicyAdapter{families: []adapter.Family{"system_health"}}
	want := []string{`unknown family "alpha"`, `unknown family "mu"`, `unknown family "zeta"`}
	for i := 0; i < 5; i++ {
		got := validateAddonCheckPolicy(checkWithPolicy(policy), adp)
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("order mismatch at %d: got %v, want %v", j, got, want)
			}
		}
	}
}

// TestValidateAddonCheckPolicy_RatioThresholds covers the reserved
// engine-level ratio keys (#159): valid on every family regardless of what
// the adapter advertises, with values validated centrally.
func TestValidateAddonCheckPolicy_RatioThresholds(t *testing.T) {
	advertising := fakeAdvertisingAdapter{
		fakePolicyAdapter: fakePolicyAdapter{families: []adapter.Family{"system_health"}},
		keys:              map[adapter.Family][]string{"system_health": {"restartWarnCount"}},
	}
	plain := fakePolicyAdapter{families: []adapter.Family{"system_health"}}

	tests := []struct {
		name         string
		policy       map[string]fathomv1alpha1.AddonCheckFamilyPolicy
		adapter      adapter.Adapter
		wantCount    int
		wantContains []string
	}{
		{
			"reserved keys accepted on an advertising adapter that does not list them",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"warnRatio": "1", "failRatio": "5%"}}},
			advertising, 0, nil,
		},
		{
			"reserved keys accepted on a non-advertising adapter",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"failRatio": "2.5"}}},
			plain, 0, nil,
		},
		{
			"reserved keys coexist with advertised adapter keys",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"restartWarnCount": "3", "failRatio": "5"}}},
			advertising, 0, nil,
		},
		{
			"non-numeric ratio value rejected, naming family and key",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"failRatio": "banana"}}},
			plain, 1, []string{`family "system_health" has an invalid ratio threshold: failRatio:`, `"banana"`},
		},
		{
			"out-of-range ratio value rejected",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"warnRatio": "150"}}},
			plain, 1, []string{`family "system_health" has an invalid ratio threshold: warnRatio:`, `"150"`},
		},
		{
			"ratio values validated even with a nil adapter",
			map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"anything": {Thresholds: map[string]fathomv1alpha1.ThresholdValue{"failRatio": "-1"}}},
			nil, 1, []string{`family "anything" has an invalid ratio threshold: failRatio:`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateAddonCheckPolicy(checkWithPolicy(tc.policy), tc.adapter)
			if len(got) != tc.wantCount {
				t.Fatalf("problems: got %d %v, want %d", len(got), got, tc.wantCount)
			}
			joined := strings.Join(got, "\n")
			for _, sub := range tc.wantContains {
				if !strings.Contains(joined, sub) {
					t.Errorf("problems %v missing %q", got, sub)
				}
			}
		})
	}
}

func TestSetAddonCheckAccepted(t *testing.T) {
	check := &fathomv1alpha1.AddonCheck{}
	check.Generation = 3

	// Valid policy -> Accepted True/SpecAccepted.
	setAddonCheckAccepted(check, nil)
	cond := apiMeta.FindStatusCondition(check.Status.Conditions, addonCheckConditionAccepted)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != "SpecAccepted" {
		t.Fatalf("valid: got %+v, want True/SpecAccepted", cond)
	}
	if cond.ObservedGeneration != 3 {
		t.Errorf("ObservedGeneration: got %d, want 3", cond.ObservedGeneration)
	}

	// Invalid policy -> Accepted False/InvalidPolicy carrying the problems.
	setAddonCheckAccepted(check, []string{`unknown family "bogus"`})
	cond = apiMeta.FindStatusCondition(check.Status.Conditions, addonCheckConditionAccepted)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "InvalidPolicy" {
		t.Fatalf("invalid: got %+v, want False/InvalidPolicy", cond)
	}
	if !strings.Contains(cond.Message, `unknown family "bogus"`) {
		t.Errorf("message missing problem: %q", cond.Message)
	}
}
