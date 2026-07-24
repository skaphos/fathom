/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package adapter_test

import (
	"strings"
	"testing"

	"github.com/skaphos/fathom/pkg/adapter"
)

func TestParseRatioThresholds_Valid(t *testing.T) {
	tests := []struct {
		name       string
		thresholds map[string]string
		wantWarn   string
		wantFail   string
	}{
		{"both unset", nil, "", ""},
		{"unrelated keys ignored", map[string]string{"warnDays": "14"}, "", ""},
		{"integer", map[string]string{"failRatio": "5"}, "", "5"},
		{"percent suffix", map[string]string{"failRatio": "5%"}, "", "5%"},
		{"one decimal", map[string]string{"warnRatio": "2.5"}, "2.5", ""},
		{"two decimals with percent", map[string]string{"warnRatio": "0.25%"}, "0.25%", ""},
		{"zero", map[string]string{"failRatio": "0"}, "", "0"},
		{"hundred", map[string]string{"failRatio": "100"}, "", "100"},
		{"both set", map[string]string{"warnRatio": "1", "failRatio": "10"}, "1", "10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := adapter.ParseRatioThresholds(tc.thresholds)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := stringOrEmpty(rt.Warn); got != tc.wantWarn {
				t.Errorf("Warn: got %q, want %q", got, tc.wantWarn)
			}
			if got := stringOrEmpty(rt.Fail); got != tc.wantFail {
				t.Errorf("Fail: got %q, want %q", got, tc.wantFail)
			}
			wantConfigured := tc.wantWarn != "" || tc.wantFail != ""
			if rt.Configured() != wantConfigured {
				t.Errorf("Configured: got %v, want %v", rt.Configured(), wantConfigured)
			}
		})
	}
}

func stringOrEmpty(p *adapter.RatioPercent) string {
	if p == nil {
		return ""
	}
	return p.String()
}

func TestParseRatioThresholds_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"non-numeric", "failRatio", "banana"},
		{"over 100", "failRatio", "150"},
		{"just over 100 with decimals", "failRatio", "100.01"},
		{"negative", "warnRatio", "-1"},
		{"empty", "warnRatio", ""},
		{"bare percent", "failRatio", "%"},
		{"double percent", "failRatio", "5%%"},
		{"three decimals", "warnRatio", "2.555"},
		{"trailing dot", "warnRatio", "5."},
		{"scientific notation", "failRatio", "1e1"},
		{"internal space", "failRatio", "5 %"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adapter.ParseRatioThresholds(map[string]string{tc.key: tc.value})
			if err == nil {
				t.Fatalf("expected error for %s=%q, got nil", tc.key, tc.value)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error %q does not name the key %q", err, tc.key)
			}
			if !strings.Contains(err.Error(), tc.value) {
				t.Errorf("error %q does not echo the value %q", err, tc.value)
			}
		})
	}
}

// mustParse builds RatioThresholds from key/value pairs, failing the test on
// parse errors, so verdict tables read declaratively.
func mustParse(t *testing.T, thresholds map[string]string) adapter.RatioThresholds {
	t.Helper()
	rt, err := adapter.ParseRatioThresholds(thresholds)
	if err != nil {
		t.Fatalf("ParseRatioThresholds(%v): %v", thresholds, err)
	}
	return rt
}

// familyChecks builds n checks for family with the given outcome.
func familyChecks(family adapter.Family, outcome adapter.Outcome, n int) []adapter.CheckResult {
	out := make([]adapter.CheckResult, n)
	for i := range out {
		out[i] = adapter.CheckResult{Family: family, Outcome: outcome}
	}
	return out
}

func TestFamilyRatioVerdict(t *testing.T) {
	const fam = adapter.Family("certificates")
	mix := func(pass, warn, fail, errored, skipped int) []adapter.CheckResult {
		var checks []adapter.CheckResult
		checks = append(checks, familyChecks(fam, adapter.OutcomePass, pass)...)
		checks = append(checks, familyChecks(fam, adapter.OutcomeWarn, warn)...)
		checks = append(checks, familyChecks(fam, adapter.OutcomeFail, fail)...)
		checks = append(checks, familyChecks(fam, adapter.OutcomeError, errored)...)
		checks = append(checks, familyChecks(fam, adapter.OutcomeSkipped, skipped)...)
		return checks
	}

	tests := []struct {
		name           string
		checks         []adapter.CheckResult
		thresholds     map[string]string
		wantVerdict    adapter.Outcome
		wantPopulation int
		wantUnhealthy  int
		wantDegraded   int
	}{
		{
			"US1: 1 of 200 fail under failRatio 5 passes",
			mix(199, 0, 1, 0, 0), map[string]string{"failRatio": "5"},
			adapter.OutcomePass, 200, 1, 1,
		},
		{
			"US1: 15 of 200 fail over failRatio 5 fails",
			mix(185, 0, 15, 0, 0), map[string]string{"failRatio": "5"},
			adapter.OutcomeFail, 200, 15, 15,
		},
		{
			"US2: 1% degraded under warnRatio 2 passes",
			mix(99, 1, 0, 0, 0), map[string]string{"warnRatio": "2", "failRatio": "10"},
			adapter.OutcomePass, 100, 0, 1,
		},
		{
			"US2: 5% degraded over warnRatio 2 warns",
			mix(95, 5, 0, 0, 0), map[string]string{"warnRatio": "2", "failRatio": "10"},
			adapter.OutcomeWarn, 100, 0, 5,
		},
		{
			"US2: 12% unhealthy over failRatio 10 fails",
			mix(88, 0, 12, 0, 0), map[string]string{"warnRatio": "2", "failRatio": "10"},
			adapter.OutcomeFail, 100, 12, 12,
		},
		{
			"boundary equality does not escalate (strict-exceed)",
			mix(95, 0, 5, 0, 0), map[string]string{"failRatio": "5"},
			adapter.OutcomePass, 100, 5, 5,
		},
		{
			"failRatio 0 reproduces worst-of",
			mix(199, 0, 1, 0, 0), map[string]string{"failRatio": "0"},
			adapter.OutcomeFail, 200, 1, 1,
		},
		{
			"warn counts degraded: warns and fails together cross warnRatio",
			mix(94, 3, 3, 0, 0), map[string]string{"warnRatio": "5", "failRatio": "10"},
			adapter.OutcomeWarn, 100, 3, 6,
		},
		{
			"warn-only fleet never fails through ratios",
			mix(0, 100, 0, 0, 0), map[string]string{"warnRatio": "1", "failRatio": "1"},
			adapter.OutcomeWarn, 100, 0, 100,
		},
		{
			"omitted failRatio keeps worst-of for fails",
			mix(199, 0, 1, 0, 0), map[string]string{"warnRatio": "50"},
			adapter.OutcomeFail, 200, 1, 1,
		},
		{
			"omitted warnRatio keeps worst-of for warns",
			mix(199, 1, 0, 0, 0), map[string]string{"failRatio": "5"},
			adapter.OutcomeWarn, 200, 0, 1,
		},
		{
			"fails under threshold with omitted warnRatio and no warns pass",
			mix(199, 0, 1, 0, 0), map[string]string{"failRatio": "5", "warnRatio": "5"},
			adapter.OutcomePass, 200, 1, 1,
		},
		{
			"error short-circuits ratios",
			mix(198, 0, 1, 1, 0), map[string]string{"failRatio": "5"},
			adapter.OutcomeError, 199, 1, 1,
		},
		{
			"skipped excluded from population",
			mix(9, 0, 1, 0, 90), map[string]string{"failRatio": "5"},
			adapter.OutcomeFail, 10, 1, 1,
		},
		{
			"empty population passes",
			mix(0, 0, 0, 0, 3), map[string]string{"failRatio": "5"},
			adapter.OutcomePass, 0, 0, 0,
		},
		{
			"no checks at all passes",
			nil, map[string]string{"failRatio": "5"},
			adapter.OutcomePass, 0, 0, 0,
		},
		{
			"fractional threshold is exact: 0.5% of 1000 is not above 0.5%",
			mix(995, 0, 5, 0, 0), map[string]string{"failRatio": "0.5"},
			adapter.OutcomePass, 1000, 5, 5,
		},
		{
			"fractional threshold is exact: 6 of 1000 is above 0.5%",
			mix(994, 0, 6, 0, 0), map[string]string{"failRatio": "0.5"},
			adapter.OutcomeFail, 1000, 6, 6,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := adapter.FamilyRatioVerdict(tc.checks, fam, mustParse(t, tc.thresholds))
			if got.Verdict != tc.wantVerdict {
				t.Errorf("Verdict: got %s, want %s", got.Verdict, tc.wantVerdict)
			}
			if got.Population != tc.wantPopulation || got.Unhealthy != tc.wantUnhealthy || got.Degraded != tc.wantDegraded {
				t.Errorf("counts: got pop=%d unhealthy=%d degraded=%d, want pop=%d unhealthy=%d degraded=%d",
					got.Population, got.Unhealthy, got.Degraded, tc.wantPopulation, tc.wantUnhealthy, tc.wantDegraded)
			}
		})
	}
}

// TestFamilyRatioVerdict_FamilyIndependence guards that other families' checks
// never leak into a family's ratio arithmetic.
func TestFamilyRatioVerdict_FamilyIndependence(t *testing.T) {
	checks := append(
		familyChecks("certificates", adapter.OutcomePass, 10),
		familyChecks("issuers", adapter.OutcomeFail, 10)...,
	)
	got := adapter.FamilyRatioVerdict(checks, "certificates", mustParse(t, map[string]string{"failRatio": "5"}))
	if got.Verdict != adapter.OutcomePass {
		t.Errorf("Verdict: got %s, want Pass (issuer fails must not taint certificates)", got.Verdict)
	}
	if got.Population != 10 || got.Unhealthy != 0 {
		t.Errorf("counts leaked across families: %+v", got)
	}
}
