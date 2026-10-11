/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package metrics

import (
	"strings"
	"testing"
	"time"
)

// seriesLabels returns every series of one family from the controller-runtime
// registry as label maps, so the tests also prove the rebuilt gauges were
// re-registered where the metrics endpoint serves from.
func seriesLabels(t *testing.T, family string) []map[string]string {
	t.Helper()
	mfs, err := ctrlRegistryGather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var out []map[string]string
	for _, mf := range mfs {
		if mf.GetName() != family {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			out = append(out, labels)
		}
	}
	return out
}

func seriesFor(t *testing.T, family, name string) []map[string]string {
	t.Helper()
	var out []map[string]string
	for _, l := range seriesLabels(t, family) {
		if l["name"] == name {
			out = append(out, l)
		}
	}
	return out
}

// configure applies an allowlist for one test and restores the default gauges
// afterwards, so no other test sees projected labels.
func configure(t *testing.T, keys ...string) {
	t.Helper()
	if err := ConfigureCheckLabels(keys); err != nil {
		t.Fatalf("ConfigureCheckLabels(%v): %v", keys, err)
	}
	t.Cleanup(func() {
		if err := ConfigureCheckLabels(nil); err != nil {
			t.Fatalf("restore default check gauges: %v", err)
		}
	})
}

func TestProjectedLabelName(t *testing.T) {
	for key, want := range map[string]string{
		"tier":                    "label_tier",
		"app.kubernetes.io/team":  "label_app_kubernetes_io_team",
		"example.com/cost-center": "label_example_com_cost_center",
	} {
		if got := ProjectedLabelName(key); got != want {
			t.Errorf("ProjectedLabelName(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestValidateProjectedLabels(t *testing.T) {
	tooMany := make([]string, MaxProjectedLabels+1)
	for i := range tooMany {
		tooMany[i] = "key" + strings.Repeat("x", i)
	}
	for _, tc := range []struct {
		name    string
		keys    []string
		wantErr string
	}{
		{"empty is the default", nil, ""},
		{"valid keys", []string{"tier", "app.kubernetes.io/team"}, ""},
		{"exactly the cap", tooMany[:MaxProjectedLabels], ""},
		{"over the cap", tooMany, "exceeds the maximum"},
		{"invalid key", []string{"not a key"}, "not a valid Kubernetes label key"},
		{"too many slashes", []string{"a/b/c"}, "not a valid Kubernetes label key"},
		{"sanitization collision", []string{"a.b", "a_b"}, "both map to the metric label \"label_a_b\""},
		{"duplicate", []string{"tier", "tier"}, "both map"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProjectedLabels(tc.keys)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

// Every check series carries every projected label; a key the CR lacks
// projects as "" so the label set stays uniform. The three gauges carry the
// same projected labels, so the staleness join still needs no relabeling.
func TestObserveCheckProjectsAllowlistedLabels(t *testing.T) {
	configure(t, "tier", "app.kubernetes.io/team")
	ObserveCheck("AddonCheck", "default", "projected", "Pass", time.Now(), 5*time.Minute,
		map[string]string{"tier": "gold", "unlisted": "ignored"})

	for _, family := range []string{"fathom_check_result", "fathom_check_last_run_timestamp_seconds", "fathom_check_interval_seconds"} {
		series := seriesFor(t, family, "projected")
		if len(series) == 0 {
			t.Fatalf("%s: no series", family)
		}
		for _, l := range series {
			if l["label_tier"] != "gold" {
				t.Errorf("%s: label_tier = %q, want gold", family, l["label_tier"])
			}
			if v, ok := l["label_app_kubernetes_io_team"]; !ok || v != "" {
				t.Errorf("%s: unset label_app_kubernetes_io_team = %q (present %v), want present and empty", family, v, ok)
			}
			if _, ok := l["label_unlisted"]; ok {
				t.Errorf("%s: a label outside the allowlist was projected", family)
			}
		}
	}
	if got := len(seriesFor(t, "fathom_check_result", "projected")); got != len(checkResultValues) {
		t.Errorf("one-hot set has %d series, want %d", got, len(checkResultValues))
	}
}

// A changed CR label must replace the check's series set, not add a second
// one alongside it: two sets would be two checks to every alert rule.
func TestObserveCheckReplacesSeriesWhenAProjectedLabelChanges(t *testing.T) {
	configure(t, "tier")
	labels := func(tier string) map[string]string { return map[string]string{"tier": tier} }
	ObserveCheck("AddonCheck", "default", "relabelled", "Fail", time.Now(), time.Minute, labels("gold"))
	ObserveCheck("AddonCheck", "default", "relabelled", "Fail", time.Now(), time.Minute, labels("silver"))

	for _, family := range []string{"fathom_check_result", "fathom_check_last_run_timestamp_seconds", "fathom_check_interval_seconds"} {
		for _, l := range seriesFor(t, family, "relabelled") {
			if l["label_tier"] != "silver" {
				t.Errorf("%s: series with label_tier=%q survived the change to silver", family, l["label_tier"])
			}
		}
	}
	if got := len(seriesFor(t, "fathom_check_result", "relabelled")); got != len(checkResultValues) {
		t.Errorf("after relabel the check has %d result series, want exactly one set of %d", got, len(checkResultValues))
	}
}

// Deleting a check withdraws its series whatever projected values they carry,
// and forgets them, so a recreated check with the same name starts clean.
func TestDeleteCheckSeriesWithProjectedLabels(t *testing.T) {
	configure(t, "tier")
	ObserveCheck("AddonCheck", "default", "deleted", "Pass", time.Now(), time.Minute, map[string]string{"tier": "gold"})
	DeleteCheckSeries("AddonCheck", "default", "deleted")
	for _, family := range []string{"fathom_check_result", "fathom_check_last_run_timestamp_seconds", "fathom_check_interval_seconds"} {
		if s := seriesFor(t, family, "deleted"); len(s) != 0 {
			t.Errorf("%s: %d series survived deletion", family, len(s))
		}
	}
	if _, ok := projected.last[checkID{"AddonCheck", "default", "deleted"}]; ok {
		t.Error("deleted check is still remembered")
	}
}

// An unresolvable cadence withdraws the interval series even though it carries
// projected label values (DeleteLabelValues on the base labels alone would
// silently miss it).
func TestIntervalWithdrawnWithProjectedLabels(t *testing.T) {
	configure(t, "tier")
	ObserveCheck("HealthCheck", "default", "cadence", "Pass", time.Now(), time.Minute, map[string]string{"tier": "gold"})
	ObserveCheck("HealthCheck", "default", "cadence", "Pass", time.Now(), 0, map[string]string{"tier": "gold"})
	if s := seriesFor(t, "fathom_check_interval_seconds", "cadence"); len(s) != 0 {
		t.Errorf("interval series survived an unresolvable cadence: %v", s)
	}
}

// The default — no allowlist — keeps today's exact label sets.
func TestNoProjectionKeepsBaseLabels(t *testing.T) {
	configure(t)
	ObserveCheck("AddonCheck", "default", "plain", "Pass", time.Now(), time.Minute, map[string]string{"tier": "gold"})
	for _, l := range seriesFor(t, "fathom_check_result", "plain") {
		for key := range l {
			if strings.HasPrefix(key, "label_") {
				t.Errorf("default gauges carry projected label %q", key)
			}
		}
	}
}

func TestConfigureCheckLabelsRejectsAnInvalidAllowlist(t *testing.T) {
	if err := ConfigureCheckLabels([]string{"a.b", "a_b"}); err == nil {
		t.Fatal("a colliding allowlist was accepted")
	}
	// The rejected call must leave the working gauges in place.
	ObserveCheck("AddonCheck", "default", "survivor", "Pass", time.Now(), 0, nil)
	if len(seriesFor(t, "fathom_check_result", "survivor")) == 0 {
		t.Fatal("a rejected allowlist broke the check gauges")
	}
}
