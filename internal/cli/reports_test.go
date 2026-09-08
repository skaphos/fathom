/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func report(ns, source, name string, age time.Duration, result fathomv1alpha1.HealthReportResult, checks ...fathomv1alpha1.HealthReportCheck) *fathomv1alpha1.HealthReport {
	return &fathomv1alpha1.HealthReport{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			Labels: map[string]string{fathomv1alpha1.LabelHealthReportSourceKind: "AddonCheck", fathomv1alpha1.LabelHealthReportSourceName: source},
		},
		Spec: fathomv1alpha1.HealthReportSpec{
			SourceRef:   fathomv1alpha1.HealthReportTargetRef{Kind: "AddonCheck", Name: source},
			Result:      result,
			ObservedAt:  metav1.NewTime(time.Now().Add(-age)),
			Checks:      checks,
			AdapterName: "coredns", AdapterVersion: "1.0.0",
		},
	}
}

func check(family, target string, result fathomv1alpha1.HealthReportResult, summary string) fathomv1alpha1.HealthReportCheck {
	return fathomv1alpha1.HealthReportCheck{Family: family, Result: result, Summary: summary,
		TargetRef: fathomv1alpha1.HealthReportTargetRef{Kind: "Deployment", Namespace: "kube-system", Name: target}}
}

// reportFixtures is five reports for coredns in shuffled creation order, one
// for another check, so ordering and label filtering are both exercised.
func reportFixtures() []client.Object {
	pass := check("system_health", "coredns", "Pass", "available")
	fail := check("system_health", "coredns", "Fail", "0 of 2 replicas available")
	dnsPass := check("dns_resolution", "kubernetes.default", "Pass", "resolved")
	return []client.Object{
		report("team-a", "coredns", "r3", 3*time.Hour, "Fail", fail, dnsPass),
		report("team-a", "coredns", "r1", 30*24*time.Hour, "Pass", pass, dnsPass),
		report("team-a", "coredns", "r5", 10*time.Minute, "Pass", pass, dnsPass),
		report("team-a", "coredns", "r2", 5*24*time.Hour, "Pass", pass, check("dns_resolution", "kubernetes.default", "Warn", "slow")),
		report("team-a", "coredns", "r4", 2*time.Hour, "Fail", fail, dnsPass),
		report("team-a", "other", "o1", time.Hour, "Pass", pass),
	}
}

func TestReports_ListsNewestFirstWithChanges(t *testing.T) {
	f, _ := fakeFactory(t, append(reportFixtures(), addonCheck("team-a", "coredns"))...)
	out, _, err := execVerb(f, "reports", "addoncheck/coredns")
	if err != nil {
		t.Fatalf("reports: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected header + 5 rows, got:\n%s", out)
	}
	wantOrder := []string{"r5", "r4", "r3", "r2", "r1"}
	wantChange := []string{"Fail→Pass", "unchanged", "Pass→Fail", "1 check(s) changed", "first"}
	for i, name := range wantOrder {
		if !strings.HasPrefix(lines[i+1], name+" ") {
			t.Errorf("row %d = %q, want %s", i+1, lines[i+1], name)
		}
		if !strings.Contains(lines[i+1], wantChange[i]) {
			t.Errorf("row %s should say %q: %q", name, wantChange[i], lines[i+1])
		}
	}
	if strings.Contains(out, "o1") {
		t.Errorf("reports for another check leaked in:\n%s", out)
	}
	if !strings.Contains(out, "system_health: 0 of 2 replicas available") || !strings.Contains(out, "2 check(s) Pass") {
		t.Errorf("derived summaries missing:\n%s", out)
	}
}

func TestReports_LimitSinceAndReport(t *testing.T) {
	f, _ := fakeFactory(t, append(reportFixtures(), addonCheck("team-a", "coredns"))...)

	out, _, err := execVerb(f, "reports", "addoncheck/coredns", "--limit", "2")
	if err != nil || strings.Count(out, "\n") != 3 || !strings.Contains(out, "r5") || !strings.Contains(out, "r4") || strings.Contains(out, "r3") {
		t.Fatalf("--limit 2: err=%v\n%s", err, out)
	}
	// The change column is computed over the full history, so the last shown
	// row still compares to the older, hidden report.
	if !strings.Contains(out, "unchanged") {
		t.Fatalf("change column must see beyond --limit:\n%s", out)
	}

	out, _, err = execVerb(f, "reports", "addoncheck/coredns", "--since", "4h")
	if err != nil || strings.Contains(out, "r2") || !strings.Contains(out, "r3") {
		t.Fatalf("--since 4h: err=%v\n%s", err, out)
	}
	out, _, err = execVerb(f, "reports", "addoncheck/coredns", "--since", "1m")
	if err != nil || !strings.Contains(out, "No reports for addoncheck/team-a/coredns within the last 1m0s (5 older).") {
		t.Fatalf("--since with nothing recent: err=%v\n%s", err, out)
	}
	if _, _, err := execVerb(f, "reports", "addoncheck/coredns", "--limit", "0"); err == nil || !strings.Contains(err.Error(), "--limit must be at least 1") {
		t.Fatalf("limit 0 error = %v", err)
	}

	out, _, err = execVerb(f, "reports", "addoncheck/coredns", "--report", "r3")
	if err != nil {
		t.Fatalf("--report: %v", err)
	}
	for _, want := range []string{"Name: r3", "Source: AddonCheck/coredns", "Result: Fail", "Adapter: coredns 1.0.0", "Checks:", "system_health", "Deployment/coredns", "0 of 2 replicas available", "dns_resolution"} {
		if !strings.Contains(squash(out), want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if _, _, err := execVerb(f, "reports", "addoncheck/coredns", "--report", "nope"); err == nil || !strings.Contains(err.Error(), `HealthReport "nope" not found in namespace team-a`) {
		t.Fatalf("--report not found error = %v", err)
	}
}

func TestReports_DerivedKindsAndEmpty(t *testing.T) {
	ch := &fathomv1alpha1.ClusterHealth{
		ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Status:     fathomv1alpha1.ClusterHealthStatus{Children: []fathomv1alpha1.ClusterHealthChildSummary{{Namespace: "team-a", Name: "web"}}},
	}
	orphan := healthCheckFor("team-a", "orphan", "Pod", "x")
	f, _ := fakeFactory(t, append(reportFixtures(), addonCheck("team-a", "coredns"), addonCheck("team-a", "quiet"),
		healthCheckFor("team-a", "web", "AddonCheck", "coredns"), orphan, ch)...)

	out, errOut, err := execVerb(f, "reports", "hc/web")
	if err != nil {
		t.Fatalf("reports hc: %v", err)
	}
	if !strings.Contains(errOut, "Showing reports for addoncheck/team-a/coredns (the source behind healthcheck/team-a/web)") {
		t.Fatalf("redirect notice missing:\n%s", errOut)
	}
	if !strings.Contains(out, "r5") {
		t.Fatalf("source history not shown:\n%s", out)
	}
	if _, _, err := execVerb(f, "reports", "hc/orphan"); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("unsupported source error = %v", err)
	}
	if _, _, err := execVerb(f, "reports", "clusterhealth/prod"); err == nil || !strings.Contains(err.Error(), "has no reports of its own") || !strings.Contains(err.Error(), "addoncheck/team-a/coredns") {
		t.Fatalf("clusterhealth error = %v", err)
	}

	out, _, err = execVerb(f, "reports", "addoncheck/quiet")
	if err != nil || strings.TrimSpace(out) != "No reports yet for addoncheck/team-a/quiet." {
		t.Fatalf("empty history: err=%v\n%s", err, out)
	}
}

func TestReports_HelpAndStructured(t *testing.T) {
	f, _ := fakeFactory(t, append(reportFixtures(), addonCheck("team-a", "coredns"))...)
	out, _, err := execVerb(f, "reports", "--help")
	if err != nil || !strings.Contains(out, "written when a verdict changes, not on every interval") {
		t.Fatalf("help must explain change-only persistence: err=%v\n%s", err, out)
	}

	out, _, err = execVerb(f, "reports", "addoncheck/coredns", "-o", "json", "--limit", "3")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Kind  string `json:"kind"`
		Items []struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if list.Kind != "List" || len(list.Items) != 3 || list.Items[0].Kind != "HealthReport" || list.Items[0].Metadata.Name != "r5" {
		t.Fatalf("unexpected list: %+v", list)
	}
	out, _, err = execVerb(f, "reports", "addoncheck/coredns", "--report", "r1", "-o", "yaml")
	if err != nil || !strings.Contains(out, "kind: HealthReport") || !strings.Contains(out, "name: r1") {
		t.Fatalf("yaml report: err=%v\n%s", err, out)
	}
}

func TestReportChange(t *testing.T) {
	a := report("ns", "s", "a", 0, "Pass", check("f", "t1", "Pass", ""), check("f", "t2", "Pass", ""))
	b := report("ns", "s", "b", 0, "Pass", check("f", "t1", "Pass", ""), check("f", "t2", "Warn", ""))
	c := report("ns", "s", "c", 0, "Pass", check("f", "t1", "Pass", ""))
	d := report("ns", "s", "d", 0, "Fail", check("f", "t1", "Fail", ""))
	tests := []struct {
		newer, older *fathomv1alpha1.HealthReport
		want         string
	}{
		{a, nil, "first"}, {a, a, "unchanged"}, {b, a, "1 check(s) changed"}, {c, a, "1 check(s) changed"}, {a, c, "1 check(s) changed"}, {d, a, "Pass→Fail"},
	}
	for i, tt := range tests {
		if got := reportChange(tt.newer, tt.older); got != tt.want {
			t.Errorf("case %d: %q, want %q", i, got, tt.want)
		}
	}
	if got := reportSummary(report("ns", "s", "e", 0, "Warn", check("f", "t", "Warn", ""))); got != "f: Warn" {
		t.Errorf("summary without check summary = %q", got)
	}
	if got := reportSummary(&fathomv1alpha1.HealthReport{Spec: fathomv1alpha1.HealthReportSpec{Result: "Fail"}}); got != fmt.Sprintf("0 check(s) %s", "Fail") {
		t.Errorf("summary with no checks = %q", got)
	}
}
