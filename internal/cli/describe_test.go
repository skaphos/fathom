/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func TestDescribe_PerKind(t *testing.T) {
	last := metav1.NewTime(time.Now().Add(-90 * time.Second))
	transition := metav1.NewTime(time.Now().Add(-time.Hour))
	ready := []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "RunCompleted", Message: "3 of 3 checks passed", LastTransitionTime: transition}}

	ac := addonCheck("team-a", "coredns")
	ac.Spec.Interval = &metav1.Duration{Duration: 10 * time.Minute}
	ac.Spec.Policy = map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
		"system_health":  {Enabled: ptr.To(true), Namespaces: []string{"kube-system"}},
		"dns_resolution": {Enabled: ptr.To(false)},
	}
	ac.Status = fathomv1alpha1.AddonCheckStatus{LastResult: "Pass", LastRunTime: &last, Conditions: ready, LastReportName: "coredns-abc", LastRunTrigger: "tok-1", DetectedVersion: "1.11.3", Absent: 1}

	dns := &fathomv1alpha1.DNSCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-dns", Namespace: "team-a"},
		Spec: fathomv1alpha1.DNSCheckSpec{
			Targets:   []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: "A", ExpectedAnswers: []string{"10.0.0.1"}}, {Name: "gone.example.com", RecordType: "A", Absent: true, Resolver: "upstream"}},
			Resolvers: []fathomv1alpha1.DNSResolver{{Name: "upstream", From: "Explicit", Address: "10.1.1.1:53"}},
		},
		Status: fathomv1alpha1.DNSCheckStatus{
			LastResult: "Fail", Summary: "1 of 2 pairs failed", LastRunTime: &last, ObservedTargets: 2, LastReportName: "dns-1",
			TargetResults: []fathomv1alpha1.DNSTargetResult{
				{Name: "a.example.com", RecordType: "A", Resolver: "cluster", Result: "Pass", Answers: []string{"10.0.0.1"}, LatencyMillis: 12},
				{Name: "gone.example.com", RecordType: "A", Resolver: "upstream", Result: "Fail", Message: "record still resolves"},
			},
		},
	}

	ncc := &fathomv1alpha1.NodeCertificateCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "node-certs", Namespace: "team-a"},
		Spec:       fathomv1alpha1.NodeCertificateCheckSpec{Paths: []string{"/etc/kubernetes/pki"}, IncludeControlPlaneNodes: ptr.To(true), NodeSelector: map[string]string{"role": "worker"}},
		Status:     fathomv1alpha1.NodeCertificateCheckStatus{LastResult: "Warn", LastRunTime: &last, Conditions: ready, DesiredNodes: 3, ReportingNodes: 3, LastReportName: "ncc-1", LastRunTrigger: "tok-2"},
	}

	hc := healthCheckFor("team-a", "web", "AddonCheck", "coredns")
	hc.Spec.Description = "front door"
	hc.Status = fathomv1alpha1.HealthCheckStatus{Result: "Pass", Summary: "mirrored", SourceObservedAt: &last, SourceInterval: &metav1.Duration{Duration: time.Minute}, LastReportName: "coredns-abc"}

	ch := &fathomv1alpha1.ClusterHealth{
		ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Spec:       fathomv1alpha1.ClusterHealthSpec{Namespaces: []string{"team-a"}, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "core"}}},
		Status: fathomv1alpha1.ClusterHealthStatus{
			Result: "Fail", MatchedCount: 3, ObservedAt: &last,
			Children: []fathomv1alpha1.ClusterHealthChildSummary{
				{Namespace: "team-a", Name: "web", Result: "Pass", Summary: "ok", ObservedAt: &last},
				{Namespace: "team-a", Name: "dns", Result: "Warn", Summary: "slow", ObservedAt: &last},
				{Namespace: "team-a", Name: "certs", Result: "Fail", Summary: "expiring", ObservedAt: &last},
			},
		},
	}

	tests := []struct {
		target string
		obj    client.Object
		want   []string
	}{
		{"addoncheck/coredns", ac, []string{"Kind: AddonCheck", "Addon type: coredns", "Interval: 10m0s", "Timeout: 30s (default)",
			"Policy dns_resolution: disabled", "Policy system_health: enabled, namespaces=kube-system", "Verdict: Pass", "Summary: 3 of 3 checks passed",
			"Consumed trigger: tok-1", "Detected version: 1.11.3", "Absent checks: 1", "Ready", "RunCompleted", "Latest report: coredns-abc (see: fathomctl reports addoncheck/team-a/coredns)"}},
		{"dns/cluster-dns", dns, []string{"Resolver upstream: Explicit 10.1.1.1:53", "Target a.example.com: A, expect=10.0.0.1", "Target gone.example.com: A, must be absent, resolver=upstream",
			"Observed targets: 2", "Target results:", "12ms", "record still resolves", "Latest report: dns-1"}},
		{"ncc/node-certs", ncc, []string{"Warn days: 30 (default)", "Control-plane nodes: true", "Node selector: role=worker", "Paths: /etc/kubernetes/pki",
			"Desired nodes: 3", "Reporting nodes: 3", "Consumed trigger: tok-2", "Interval: 1h0m0s (default)"}},
		{"hc/web", hc, []string{"Check ref: AddonCheck/coredns", "Description: front door", "Verdict: Pass", "Source interval: 1m0s", "Next run:", "Latest report: coredns-abc"}},
		{"clusterhealth/prod", ch, []string{"Kind: ClusterHealth", "Selector: tier=core", "Namespaces: team-a", "Matched: 3", "Children:", "web", "Warn", "expiring", "3 matched, worst Fail"}},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			f, _ := fakeFactory(t, tt.obj)
			raw, _, err := execVerb(f, "describe", tt.target)
			if err != nil {
				t.Fatalf("describe: %v", err)
			}
			out := squash(raw)
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in:\n%s", want, raw)
				}
			}
			if strings.Contains(out, "Namespace:") != (tt.obj.GetNamespace() != "") {
				t.Errorf("namespace line presence wrong for %s:\n%s", tt.target, raw)
			}
		})
	}
}

// squash collapses runs of whitespace so tests assert on content, not on
// the column padding of the key/value layout.
func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func TestDescribe_NeverRunAndNotFound(t *testing.T) {
	f, _ := fakeFactory(t, addonCheck("team-a", "fresh"))
	out, _, err := execVerb(f, "describe", "addoncheck/fresh")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Verdict: -", "Last run: never", "(none)"} {
		if !strings.Contains(squash(out), want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Latest report") {
		t.Errorf("no report pointer without a report:\n%s", out)
	}
	if _, _, err := execVerb(f, "describe", "addoncheck/nope"); err == nil || !strings.Contains(err.Error(), `AddonCheck "nope" not found in namespace team-a`) {
		t.Errorf("not-found error = %v", err)
	}
	if _, _, err := execVerb(f, "describe", "clusterhealth/nope"); err == nil || !strings.Contains(err.Error(), `ClusterHealth "nope" not found`) || strings.Contains(err.Error(), "namespace") {
		t.Errorf("cluster-scoped not-found error = %v", err)
	}
	if _, _, err := execVerb(f, "describe", "addoncheck/fresh", "extra"); err == nil || !strings.Contains(err.Error(), "unexpected argument") {
		t.Errorf("extra-arg error = %v", err)
	}
}

func TestDescribe_StructuredIsUnmodified(t *testing.T) {
	ac := addonCheck("team-a", "coredns")
	ac.Status.LastResult = "Pass"
	f, _ := fakeFactory(t, ac)
	out, _, err := execVerb(f, "describe", "ac", "coredns", "-o", "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var got fathomv1alpha1.AddonCheck
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("yaml: %v\n%s", err, out)
	}
	if got.Kind != "AddonCheck" || got.APIVersion != "fathom.skaphos.io/v1alpha1" || got.Name != "coredns" || got.Status.LastResult != "Pass" || got.Spec.AddonType != "coredns" {
		t.Fatalf("object not emitted unmodified: %+v", got)
	}
}

func TestCadenceRendering(t *testing.T) {
	if got := cadence(nil, fathomv1alpha1.MinCheckInterval, 5*time.Minute); got != "5m0s (default)" {
		t.Errorf("nil = %q", got)
	}
	if got := cadence(&metav1.Duration{Duration: time.Second}, fathomv1alpha1.MinCheckInterval, 5*time.Minute); !strings.Contains(got, "clamped") {
		t.Errorf("sub-floor = %q", got)
	}
	if got := cadence(&metav1.Duration{Duration: time.Minute}, fathomv1alpha1.MinCheckInterval, 5*time.Minute); got != "1m0s" {
		t.Errorf("declared = %q", got)
	}
}
