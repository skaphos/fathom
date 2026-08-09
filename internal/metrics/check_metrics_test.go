/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package metrics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// gatherCheckSeries returns the label→value map for one metric family from
// the package collectors, keyed by "kind|name|namespace|result" (result empty
// for the timestamp gauge).
func gatherCheckSeries(t *testing.T, family string) map[string]float64 {
	t.Helper()
	mfs, err := ctrlRegistryGather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() != family {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			key := labels["kind"] + "|" + labels["name"] + "|" + labels["namespace"] + "|" + labels["result"]
			out[key] = m.GetGauge().GetValue()
		}
	}
	return out
}

func gatherOneHot(t *testing.T, kind, name, namespace string) map[string]float64 {
	t.Helper()
	series := gatherCheckSeries(t, "fathom_check_result")
	out := map[string]float64{}
	prefix := kind + "|" + name + "|" + namespace + "|"
	for key, v := range series {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			out[key[len(prefix):]] = v
		}
	}
	return out
}

func TestObserveCheckOneHotInvariant(t *testing.T) {
	CheckResult.Reset()
	CheckLastRunTimestamp.Reset()

	now := time.Now()
	ObserveCheck("AddonCheck", "default", "cm", "Fail", now)

	oneHot := gatherOneHot(t, "AddonCheck", "cm", "default")
	if len(oneHot) != len(checkResultValues) {
		t.Fatalf("expected %d series, got %d: %v", len(checkResultValues), len(oneHot), oneHot)
	}
	ones := 0
	for result, v := range oneHot {
		switch v {
		case 1:
			ones++
			if result != "Fail" {
				t.Errorf("series %q is 1, want Fail", result)
			}
		case 0:
		default:
			t.Errorf("series %q has non-binary value %v", result, v)
		}
	}
	if ones != 1 {
		t.Errorf("one-hot violated: %d series at 1", ones)
	}

	ts := gatherCheckSeries(t, "fathom_check_last_run_timestamp_seconds")["AddonCheck|cm|default|"]
	if want := float64(now.Unix()); ts != want {
		t.Errorf("last-run = %v, want %v", ts, want)
	}
}

func TestObserveCheckSentinels(t *testing.T) {
	CheckResult.Reset()
	CheckLastRunTimestamp.Reset()

	// Empty result and zero time are the discovery sentinels: Unknown / 0.
	ObserveCheck("HealthCheck", "default", "fresh", "", time.Time{})

	oneHot := gatherOneHot(t, "HealthCheck", "fresh", "default")
	if oneHot["Unknown"] != 1 {
		t.Errorf("empty result should coerce to Unknown=1, got %v", oneHot)
	}
	if ts := gatherCheckSeries(t, "fathom_check_last_run_timestamp_seconds")["HealthCheck|fresh|default|"]; ts != 0 {
		t.Errorf("never-ran sentinel = %v, want 0", ts)
	}

	// An unrecognized value must not mint a new label value.
	ObserveCheck("HealthCheck", "default", "fresh", "Bogus", time.Time{})
	oneHot = gatherOneHot(t, "HealthCheck", "fresh", "default")
	if _, ok := oneHot["Bogus"]; ok {
		t.Errorf("unrecognized result minted a series: %v", oneHot)
	}
	if oneHot["Unknown"] != 1 {
		t.Errorf("unrecognized result should coerce to Unknown=1, got %v", oneHot)
	}
}

func TestObserveCheckFlipsResult(t *testing.T) {
	CheckResult.Reset()

	ObserveCheck("AddonCheck", "default", "flip", "Pass", time.Now())
	ObserveCheck("AddonCheck", "default", "flip", "Fail", time.Now())

	oneHot := gatherOneHot(t, "AddonCheck", "flip", "default")
	if oneHot["Fail"] != 1 || oneHot["Pass"] != 0 {
		t.Errorf("after flip want Fail=1 Pass=0, got %v", oneHot)
	}
}

func TestDeleteCheckSeries(t *testing.T) {
	CheckResult.Reset()
	CheckLastRunTimestamp.Reset()

	ObserveCheck("ClusterHealth", "", "platform", "Pass", time.Now())
	ObserveCheck("AddonCheck", "default", "keep", "Pass", time.Now())
	DeleteCheckSeries("ClusterHealth", "", "platform")

	if got := gatherOneHot(t, "ClusterHealth", "platform", ""); len(got) != 0 {
		t.Errorf("deleted check still has result series: %v", got)
	}
	tsSeries := gatherCheckSeries(t, "fathom_check_last_run_timestamp_seconds")
	if _, ok := tsSeries["ClusterHealth|platform||"]; ok {
		t.Error("deleted check still has last-run series")
	}
	if got := gatherOneHot(t, "AddonCheck", "keep", "default"); len(got) != len(checkResultValues) {
		t.Errorf("unrelated check lost series: %v", got)
	}
}

// gatherDNSTargetSeries returns the per-target gauge keyed by
// "namespace|check|name|record_type|resolver|result".
func gatherDNSTargetSeries(t *testing.T) map[string]float64 {
	t.Helper()
	mfs, err := ctrlRegistryGather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() != "fathom_dnscheck_target_result" {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			key := strings.Join([]string{
				labels["namespace"], labels["check"], labels["name"],
				labels["record_type"], labels["resolver"], labels["result"],
			}, "|")
			out[key] = m.GetGauge().GetValue()
		}
	}
	return out
}

func TestObserveDNSTargetOneHotInvariant(t *testing.T) {
	DNSCheckTargetResult.Reset()

	ObserveDNSTarget("tenant-a", "internal-names", "api.example.com", "A", "cluster", "Fail")

	series := gatherDNSTargetSeries(t)
	if len(series) != len(checkResultValues) {
		t.Fatalf("got %d series, want one per result value (%d): %v", len(series), len(checkResultValues), series)
	}
	var ones int
	for key, value := range series {
		switch {
		case strings.HasSuffix(key, "|Fail"):
			if value != 1 {
				t.Errorf("%s = %v, want 1", key, value)
			}
			ones++
		case value != 0:
			t.Errorf("%s = %v, want 0", key, value)
		}
	}
	if ones != 1 {
		t.Errorf("got %d series set to 1, want exactly 1", ones)
	}
}

// TestObserveDNSTargetCoercesUnknownResult mirrors ObserveCheck's sentinel
// handling: an outcome outside the vocabulary must land on Unknown rather than
// creating a series nothing can alert on.
func TestObserveDNSTargetCoercesUnknownResult(t *testing.T) {
	DNSCheckTargetResult.Reset()

	ObserveDNSTarget("tenant-a", "c", "n", "A", "cluster", "")
	ObserveDNSTarget("tenant-a", "c", "m", "A", "cluster", "Bogus")

	series := gatherDNSTargetSeries(t)
	for _, name := range []string{"n", "m"} {
		key := "tenant-a|c|" + name + "|A|cluster|Unknown"
		if series[key] != 1 {
			t.Errorf("%s = %v, want 1", key, series[key])
		}
	}
}

// TestObserveDNSTargetDistinguishesPairs is the FR-035 guarantee at the metric
// layer: the same name from two vantage points is two pairs, not one series
// overwriting the other.
func TestObserveDNSTargetDistinguishesPairs(t *testing.T) {
	DNSCheckTargetResult.Reset()

	ObserveDNSTarget("tenant-a", "c", "api.example.com", "A", "cluster", "Pass")
	ObserveDNSTarget("tenant-a", "c", "api.example.com", "A", "upstream", "Fail")

	series := gatherDNSTargetSeries(t)
	if got := series["tenant-a|c|api.example.com|A|cluster|Pass"]; got != 1 {
		t.Errorf("cluster vantage point: got %v, want 1", got)
	}
	if got := series["tenant-a|c|api.example.com|A|upstream|Fail"]; got != 1 {
		t.Errorf("upstream vantage point: got %v, want 1", got)
	}
}

// TestDeleteDNSCheckTargetSeries covers the rebuild half of FR-036: a dropped
// pair's series must disappear, and an unrelated check in the same namespace
// must be untouched.
func TestDeleteDNSCheckTargetSeries(t *testing.T) {
	DNSCheckTargetResult.Reset()

	ObserveDNSTarget("tenant-a", "doomed", "a.example.com", "A", "cluster", "Pass")
	ObserveDNSTarget("tenant-a", "doomed", "b.example.com", "A", "cluster", "Fail")
	ObserveDNSTarget("tenant-a", "keep", "c.example.com", "A", "cluster", "Pass")
	ObserveDNSTarget("tenant-b", "doomed", "d.example.com", "A", "cluster", "Pass")

	DeleteDNSCheckTargetSeries("tenant-a", "doomed")

	series := gatherDNSTargetSeries(t)
	for key := range series {
		if strings.HasPrefix(key, "tenant-a|doomed|") {
			t.Errorf("deleted check still has series %s", key)
		}
	}
	if got := series["tenant-a|keep|c.example.com|A|cluster|Pass"]; got != 1 {
		t.Errorf("sibling check in the same namespace lost its series: got %v", got)
	}
	if got := series["tenant-b|doomed|d.example.com|A|cluster|Pass"]; got != 1 {
		t.Errorf("same-named check in another namespace lost its series: got %v", got)
	}
}

// TestDNSCheckTargetSeriesCeiling pins SC-009: 16 targets × 3 vantage points ×
// 6 results = 288 series per check, computable from the schema caps alone. If a
// schema cap is raised, this fails — which is the point: it is a cardinality
// change, not merely a limits change.
func TestDNSCheckTargetSeriesCeiling(t *testing.T) {
	DNSCheckTargetResult.Reset()

	const (
		maxTargets   = 16 // DNSCheckSpec.Targets MaxItems
		maxResolvers = 3  // DNSCheckSpec.Resolvers MaxItems
	)
	for target := range maxTargets {
		for resolver := range maxResolvers {
			ObserveDNSTarget("tenant-a", "saturated",
				fmt.Sprintf("name-%02d.example.com", target), "A",
				fmt.Sprintf("resolver-%d", resolver), "Pass")
		}
	}

	want := maxTargets * maxResolvers * len(checkResultValues)
	if want != 288 {
		t.Fatalf("ceiling arithmetic drifted: got %d, want 288", want)
	}
	if got := len(gatherDNSTargetSeries(t)); got != want {
		t.Errorf("series count: got %d, want %d", got, want)
	}
}

// TestCheckResultValuesMatchAPIVocabulary pins checkResultValues to the
// api/v1alpha1 HealthReportResult constants. The literal exists so this
// package stays free of API/apimachinery imports (the node-agent binary
// serves these metrics); this test is the sync guarantee (FR-002/SC-004).
func TestCheckResultValuesMatchAPIVocabulary(t *testing.T) {
	api := []string{
		string(fathomv1alpha1.HealthReportResultPass),
		string(fathomv1alpha1.HealthReportResultWarn),
		string(fathomv1alpha1.HealthReportResultFail),
		string(fathomv1alpha1.HealthReportResultError),
		string(fathomv1alpha1.HealthReportResultSkipped),
		string(fathomv1alpha1.HealthReportResultUnknown),
	}
	if len(api) != len(checkResultValues) {
		t.Fatalf("vocabulary drift: metrics has %d values, api has %d", len(checkResultValues), len(api))
	}
	for i, want := range api {
		if checkResultValues[i] != want {
			t.Errorf("checkResultValues[%d] = %q, want %q", i, checkResultValues[i], want)
		}
	}
}

// ctrlRegistryGather gathers the controller-runtime registry the package
// collectors register into via init().
func ctrlRegistryGather() ([]*dto.MetricFamily, error) {
	return ctrlmetrics.Registry.Gather()
}
