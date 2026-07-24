/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/pkg/adapter"
)

// ratioCheckMix builds a family's worth of CheckResults with the given
// outcome counts, each carrying a distinct target name so per-resource
// entries stay distinguishable in report-level assertions.
func ratioCheckMix(family adapter.Family, pass, warn, fail, errored, skipped int) []adapter.CheckResult {
	var out []adapter.CheckResult
	add := func(outcome adapter.Outcome, n int) {
		for i := 0; i < n; i++ {
			out = append(out, adapter.CheckResult{
				Family:    family,
				Outcome:   outcome,
				TargetRef: adapter.TargetRef{Kind: "Widget", Name: string(family) + "-" + string(outcome) + "-" + strings.Repeat("i", i+1)},
			})
		}
	}
	add(adapter.OutcomePass, pass)
	add(adapter.OutcomeWarn, warn)
	add(adapter.OutcomeFail, fail)
	add(adapter.OutcomeError, errored)
	add(adapter.OutcomeSkipped, skipped)
	return out
}

func ratioPolicyCheck(thresholdsByFamily map[string]map[string]string) *fathomv1alpha1.AddonCheck {
	policy := make(map[string]fathomv1alpha1.AddonCheckFamilyPolicy, len(thresholdsByFamily))
	for family, thresholds := range thresholdsByFamily {
		policy[family] = fathomv1alpha1.AddonCheckFamilyPolicy{Thresholds: thresholds}
	}
	return &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "ratio-check", Namespace: "default"},
		Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "fake", Policy: policy},
	}
}

func TestAggregateWithRatioRollups(t *testing.T) {
	const fam = adapter.Family("certificates")
	ratios := func(thresholds map[string]string) map[adapter.Family]adapter.RatioThresholds {
		rt, err := adapter.ParseRatioThresholds(thresholds)
		if err != nil {
			t.Fatalf("ParseRatioThresholds(%v): %v", thresholds, err)
		}
		return map[adapter.Family]adapter.RatioThresholds{fam: rt}
	}

	tests := []struct {
		name        string
		checks      []adapter.CheckResult
		ratios      map[adapter.Family]adapter.RatioThresholds
		wantResult  fathomv1alpha1.HealthReportResult
		wantRollups int
	}{
		{
			"US1: 1 of 200 fail under failRatio 5 aggregates to Pass",
			ratioCheckMix(fam, 199, 0, 1, 0, 0), ratios(map[string]string{"failRatio": "5"}),
			fathomv1alpha1.HealthReportResultPass, 1,
		},
		{
			"US1: 15 of 200 fail over failRatio 5 aggregates to Fail",
			ratioCheckMix(fam, 185, 0, 15, 0, 0), ratios(map[string]string{"failRatio": "5"}),
			fathomv1alpha1.HealthReportResultFail, 1,
		},
		{
			"US2: warn band aggregates to Warn",
			ratioCheckMix(fam, 95, 5, 0, 0, 0), ratios(map[string]string{"warnRatio": "2", "failRatio": "10"}),
			fathomv1alpha1.HealthReportResultWarn, 1,
		},
		{
			"nil ratios preserve worst-of (single Fail fails)",
			ratioCheckMix(fam, 199, 0, 1, 0, 0), nil,
			fathomv1alpha1.HealthReportResultFail, 0,
		},
		{
			"ratio family held below Fail cannot fail the overall check",
			append(ratioCheckMix(fam, 199, 0, 1, 0, 0), ratioCheckMix("issuers", 3, 0, 0, 0, 0)...),
			ratios(map[string]string{"failRatio": "5"}),
			fathomv1alpha1.HealthReportResultPass, 1,
		},
		{
			"non-ratio family still fails the overall check via worst-of",
			append(ratioCheckMix(fam, 199, 0, 1, 0, 0), ratioCheckMix("issuers", 2, 0, 1, 0, 0)...),
			ratios(map[string]string{"failRatio": "5"}),
			fathomv1alpha1.HealthReportResultFail, 1,
		},
		{
			"error in ratio family aggregates to Error",
			ratioCheckMix(fam, 198, 0, 1, 1, 0), ratios(map[string]string{"failRatio": "5"}),
			fathomv1alpha1.HealthReportResultError, 1,
		},
		{
			"all-Skipped ratio family still folds to Skipped (no invented Pass)",
			ratioCheckMix(fam, 0, 0, 0, 0, 3), ratios(map[string]string{"failRatio": "5"}),
			fathomv1alpha1.HealthReportResultSkipped, 0,
		},
		{
			"empty checks fold to Skipped even with ratios configured",
			nil, ratios(map[string]string{"failRatio": "5"}),
			fathomv1alpha1.HealthReportResultSkipped, 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, rollups := aggregateWithRatioRollups(tc.checks, tc.ratios)
			if got != tc.wantResult {
				t.Errorf("result: got %s, want %s", got, tc.wantResult)
			}
			if len(rollups) != tc.wantRollups {
				t.Errorf("rollups: got %d, want %d", len(rollups), tc.wantRollups)
			}
		})
	}
}

// TestAggregateWithRatioRollups_MatchesLegacyWithoutRatios is the US3
// regression net: with no ratio thresholds the family-aware path must be
// bit-for-bit the legacy fold across the outcome matrix.
func TestAggregateWithRatioRollups_MatchesLegacyWithoutRatios(t *testing.T) {
	const fam = adapter.Family("certificates")
	cases := [][]adapter.CheckResult{
		nil,
		ratioCheckMix(fam, 3, 0, 0, 0, 0),
		ratioCheckMix(fam, 2, 1, 0, 0, 0),
		ratioCheckMix(fam, 2, 0, 1, 0, 0),
		ratioCheckMix(fam, 2, 0, 0, 1, 0),
		ratioCheckMix(fam, 0, 0, 0, 0, 2),
		ratioCheckMix(fam, 1, 1, 1, 1, 1),
	}
	for _, checks := range cases {
		legacy := aggregateHealthReportResult(checks)
		got, rollups := aggregateWithRatioRollups(checks, nil)
		if got != legacy {
			t.Errorf("checks %v: got %s, legacy %s", checks, got, legacy)
		}
		if rollups != nil {
			t.Errorf("checks %v: unexpected rollups %v", checks, rollups)
		}
	}
}

func TestHealthReportForAddonCheck_RatioRollupEntries(t *testing.T) {
	const fam = "certificates"
	check := ratioPolicyCheck(map[string]map[string]string{fam: {"warnRatio": "1", "failRatio": "5"}})
	checks := ratioCheckMix(adapter.Family(fam), 197, 2, 1, 0, 0)
	result := adapter.Result{Checks: checks}

	report := healthReportForAddonCheck(check, fakePolicyAdapter{families: []adapter.Family{fam}}, result, metav1.Now(), nil)

	// Overall verdict: 1/200 unhealthy (0.5%) ≤ 5, 3/200 degraded (1.5%) > 1 → Warn.
	if report.Spec.Result != fathomv1alpha1.HealthReportResultWarn {
		t.Errorf("report result: got %s, want Warn", report.Spec.Result)
	}

	// Per-resource entries are untouched: one report check per adapter check.
	if got, want := len(report.Spec.Checks), len(checks)+1; got != want {
		t.Fatalf("report checks: got %d, want %d (%d per-resource + 1 rollup)", got, want, len(checks))
	}
	for i, c := range checks {
		entry := report.Spec.Checks[i]
		if entry.TargetRef.Name != c.TargetRef.Name || string(entry.Result) != string(healthReportResult(c.Outcome)) {
			t.Errorf("per-resource entry %d modified: %+v", i, entry)
		}
		if entry.Details[detailRollup] != "" {
			t.Errorf("per-resource entry %d unexpectedly carries the rollup discriminator", i)
		}
	}

	// The rollup entry is last, targets the AddonCheck, and explains itself.
	rollup := report.Spec.Checks[len(report.Spec.Checks)-1]
	if rollup.Family != fam {
		t.Errorf("rollup family: got %q, want %q", rollup.Family, fam)
	}
	if rollup.Result != fathomv1alpha1.HealthReportResultWarn {
		t.Errorf("rollup result: got %s, want Warn", rollup.Result)
	}
	if rollup.TargetRef.Kind != "AddonCheck" || rollup.TargetRef.Name != check.Name {
		t.Errorf("rollup targetRef: got %+v, want the driving AddonCheck", rollup.TargetRef)
	}
	wantDetails := map[string]string{
		detailRollup:           detailRollupRatio,
		detailRollupPopulation: "200",
		detailRollupUnhealthy:  "1",
		detailRollupDegraded:   "3",
		"warnRatio":            "1",
		"failRatio":            "5",
	}
	for k, want := range wantDetails {
		if got := rollup.Details[k]; got != want {
			t.Errorf("rollup details[%s]: got %q, want %q", k, got, want)
		}
	}
	for _, frag := range []string{"1 unhealthy", "3 degraded", "200 evaluated", "failRatio 5", "warnRatio 1", "-> Warn"} {
		if !strings.Contains(rollup.Summary, frag) {
			t.Errorf("rollup summary %q missing %q", rollup.Summary, frag)
		}
	}
}

func TestHealthReportForAddonCheck_NoThresholdsNoRollups(t *testing.T) {
	const fam = "certificates"
	check := ratioPolicyCheck(map[string]map[string]string{fam: {"warnDays": "14"}})
	checks := ratioCheckMix(adapter.Family(fam), 199, 0, 1, 0, 0)

	report := healthReportForAddonCheck(check, fakePolicyAdapter{families: []adapter.Family{fam}}, adapter.Result{Checks: checks}, metav1.Now(), nil)

	if report.Spec.Result != fathomv1alpha1.HealthReportResultFail {
		t.Errorf("report result: got %s, want Fail (worst-of preserved without ratio thresholds)", report.Spec.Result)
	}
	if got, want := len(report.Spec.Checks), len(checks); got != want {
		t.Errorf("report checks: got %d, want %d (no rollup entries)", got, want)
	}
	for i, entry := range report.Spec.Checks {
		if entry.Details[detailRollup] != "" {
			t.Errorf("entry %d unexpectedly carries the rollup discriminator", i)
		}
	}
}

// TestHealthReportForAddonCheck_RunErrStillError guards the runErr override:
// an adapter-level failure is Error regardless of ratio thresholds.
func TestHealthReportForAddonCheck_RunErrStillError(t *testing.T) {
	const fam = "certificates"
	check := ratioPolicyCheck(map[string]map[string]string{fam: {"failRatio": "100"}})
	checks := ratioCheckMix(adapter.Family(fam), 10, 0, 0, 0, 0)

	report := healthReportForAddonCheck(check, fakePolicyAdapter{families: []adapter.Family{fam}}, adapter.Result{Checks: checks}, metav1.Now(), errors.New("adapter blew up"))

	if report.Spec.Result != fathomv1alpha1.HealthReportResultError {
		t.Errorf("report result: got %s, want Error", report.Spec.Result)
	}
}
