/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// execVerb runs `fathomctl <verb> <args>` and returns stdout, stderr, error.
func execVerb(f *factory, verb string, args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	cmd := newRootCommand(f)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{verb}, args...))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// lsFixtures is one object per kind across two namespaces, with enough status
// to render every column.
func lsFixtures() []client.Object {
	last := metav1.NewTime(time.Now().Add(-3 * time.Minute))
	ac := addonCheck("team-a", "coredns")
	ac.Labels = map[string]string{"tier": "core"}
	ac.Status = fathomv1alpha1.AddonCheckStatus{
		LastResult: "Pass", LastRunTime: &last,
		Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Message: "3 of 3 checks passed"}},
	}
	dns := &fathomv1alpha1.DNSCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-dns", Namespace: "team-a"},
		Status:     fathomv1alpha1.DNSCheckStatus{LastResult: "Fail", Summary: "1 of 2 pairs failed", LastRunTime: &last},
	}
	ncc := &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: "node-certs", Namespace: "team-b"}}
	hc := healthCheckFor("team-b", "web", "AddonCheck", "coredns")
	hc.Status = fathomv1alpha1.HealthCheckStatus{Result: "Warn", Summary: "mirrored", SourceObservedAt: &last}
	ch := &fathomv1alpha1.ClusterHealth{
		ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		Status:     fathomv1alpha1.ClusterHealthStatus{Result: "Fail", MatchedCount: 2, ObservedAt: &last},
	}
	return []client.Object{ac, dns, ncc, hc, ch}
}

func TestLs_GroupedListing(t *testing.T) {
	f, _ := fakeFactory(t, lsFixtures()...)
	out, _, err := execVerb(f, "ls")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasPrefix(lines[0], "KIND") || strings.Contains(lines[0], "NAMESPACE") {
		t.Fatalf("grouped header should start with KIND and omit NAMESPACE without -A:\n%s", out)
	}
	// Namespace team-a only, plus ClusterHealth regardless of namespace, in
	// fixed kind order.
	want := []string{"AddonCheck", "DNSCheck", "ClusterHealth"}
	if len(lines) != 1+len(want) {
		t.Fatalf("expected %d rows, got:\n%s", len(want), out)
	}
	for i, kind := range want {
		if !strings.HasPrefix(lines[i+1], kind) {
			t.Errorf("row %d should be %s:\n%s", i+1, kind, out)
		}
	}
	for _, cell := range []string{"coredns", "Pass", "3 of 3 checks passed", "3m", "cluster-dns", "Fail", "1 of 2 pairs failed", "now", "prod", "2 matched, worst Fail"} {
		if !strings.Contains(out, cell) {
			t.Errorf("expected %q in output:\n%s", cell, out)
		}
	}
	if strings.Contains(out, "node-certs") || strings.Contains(out, "web") {
		t.Errorf("team-b objects must not appear when listing team-a:\n%s", out)
	}
}

func TestLs_AllNamespacesAndNeverRun(t *testing.T) {
	f, _ := fakeFactory(t, lsFixtures()...)
	out, _, err := execVerb(f, "ls", "-A")
	if err != nil {
		t.Fatalf("ls -A: %v", err)
	}
	if !strings.Contains(strings.SplitN(out, "\n", 2)[0], "NAMESPACE") {
		t.Fatalf("-A must add a NAMESPACE column:\n%s", out)
	}
	if !strings.Contains(out, "team-b") || !strings.Contains(out, "node-certs") || !strings.Contains(out, "web") {
		t.Fatalf("-A must list every namespace:\n%s", out)
	}
	// A never-run NodeCertificateCheck shows "-" verdict, not Unknown.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "node-certs") && !strings.Contains(line, " - ") {
			t.Errorf("never-run check must render a dash verdict: %q", line)
		}
	}
	// ClusterHealth has a blank namespace cell.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ClusterHealth") && strings.Contains(line, "team-") {
			t.Errorf("cluster-scoped row must not carry a namespace: %q", line)
		}
	}
}

func TestLs_SingleKindSpellingsAndSelector(t *testing.T) {
	f, _ := fakeFactory(t, lsFixtures()...)
	for _, spelling := range []string{"AddonCheck", "addonchecks", "ac"} {
		out, _, err := execVerb(f, "ls", spelling)
		if err != nil {
			t.Fatalf("ls %s: %v", spelling, err)
		}
		if strings.HasPrefix(out, "KIND") || !strings.Contains(out, "coredns") || strings.Contains(out, "cluster-dns") {
			t.Errorf("ls %s should list only AddonChecks without a KIND column:\n%s", spelling, out)
		}
	}
	out, _, err := execVerb(f, "ls", "-A", "-l", "tier=core")
	if err != nil || strings.Contains(out, "cluster-dns") || !strings.Contains(out, "coredns") {
		t.Fatalf("selector should keep only the labelled check: err=%v\n%s", err, out)
	}
	out, _, err = execVerb(f, "ls", "clusterhealth", "-n", "anything")
	if err != nil || !strings.Contains(out, "prod") {
		t.Fatalf("ClusterHealth must be listed under any -n: err=%v\n%s", err, out)
	}
	if _, _, err := execVerb(f, "ls", "pods"); err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("unknown kind error = %v", err)
	}
}

func TestLs_EmptyMessages(t *testing.T) {
	f, _ := fakeFactory(t)
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"ls"}, "No checks found in namespace team-a."},
		{[]string{"ls", "-A"}, "No checks found in any namespace."},
		{[]string{"ls", "dnschecks"}, "No dnschecks found in namespace team-a."},
		{[]string{"ls", "ch"}, "No clusterhealths found."},
	}
	for _, tt := range tests {
		out, _, err := execVerb(f, tt.args[0], tt.args[1:]...)
		if err != nil {
			t.Errorf("%v: %v", tt.args, err)
		}
		if strings.TrimSpace(out) != tt.want {
			t.Errorf("%v = %q, want %q", tt.args, strings.TrimSpace(out), tt.want)
		}
	}
}

func TestLs_StructuredOutputIsUnmodifiedList(t *testing.T) {
	f, _ := fakeFactory(t, lsFixtures()...)
	out, _, err := execVerb(f, "ls", "-A", "-o", "json")
	if err != nil {
		t.Fatalf("ls -o json: %v", err)
	}
	var list struct {
		Kind  string `json:"kind"`
		Items []struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Metadata   struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status map[string]any `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if list.Kind != "List" || len(list.Items) != 5 {
		t.Fatalf("expected a List of 5, got %s with %d items", list.Kind, len(list.Items))
	}
	for _, it := range list.Items {
		if it.APIVersion != "fathom.skaphos.io/v1alpha1" || it.Kind == "" {
			t.Errorf("item %s lacks GVK: %+v", it.Metadata.Name, it)
		}
	}
	out, _, err = execVerb(f, "ls", "-o", "yaml")
	if err != nil || !strings.Contains(out, "kind: List") {
		t.Fatalf("yaml: err=%v\n%s", err, out)
	}
	// An empty structured result is still a valid, parseable List.
	empty, _ := fakeFactory(t)
	out, _, err = execVerb(empty, "ls", "-o", "json")
	if err != nil || !strings.Contains(out, `"items": []`) {
		t.Fatalf("empty json list: err=%v\n%s", err, out)
	}
}

// TestLs_HundredChecksIsFast bounds the client-side path for SC-001: with
// 100 objects per kind the listing renders well inside the 5 s budget (the
// API round-trips, absent here, are the only other cost).
func TestLs_HundredChecksIsFast(t *testing.T) {
	var objs []client.Object
	for i := range 100 {
		objs = append(objs,
			addonCheck("team-a", fmt.Sprintf("ac-%03d", i)),
			&fathomv1alpha1.DNSCheck{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("dns-%03d", i), Namespace: "team-a"}},
			&fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ncc-%03d", i), Namespace: "team-a"}},
			healthCheckFor("team-a", fmt.Sprintf("hc-%03d", i), "AddonCheck", "x"),
			&fathomv1alpha1.ClusterHealth{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ch-%03d", i)}},
		)
	}
	f, _ := fakeFactory(t, objs...)
	start := time.Now()
	out, _, err := execVerb(f, "ls", "-A")
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("ls over 500 objects took %s", elapsed)
	}
	if got := strings.Count(out, "\n"); got < 500 {
		t.Fatalf("expected at least 500 rows, got %d", got)
	}
}
