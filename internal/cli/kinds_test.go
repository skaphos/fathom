/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// TestParseKind_AcceptedSpellings locks every spelling the contract promises:
// the Kind, its lowercase form, the plural resource, and the CLI alias, all
// case-insensitively.
func TestParseKind_AcceptedSpellings(t *testing.T) {
	tests := map[string][]string{
		"AddonCheck":           {"AddonCheck", "addoncheck", "addonchecks", "ac", "AC"},
		"DNSCheck":             {"DNSCheck", "dnscheck", "dnschecks", "dns"},
		"NodeCertificateCheck": {"NodeCertificateCheck", "nodecertificatecheck", "nodecertificatechecks", "ncc"},
		"NodeHealthCheck":      {"NodeHealthCheck", "nodehealthcheck", "nodehealthchecks", "nhc", "NHC"},
		"HealthCheck":          {"HealthCheck", "healthcheck", "healthchecks", "hc"},
		"ClusterHealth":        {"ClusterHealth", "clusterhealth", "clusterhealths", "ch"},
	}
	for kind, spellings := range tests {
		for _, s := range spellings {
			k, err := parseKind(s)
			if err != nil {
				t.Errorf("parseKind(%q): %v", s, err)
				continue
			}
			if k.Kind != kind {
				t.Errorf("parseKind(%q) = %s, want %s", s, k.Kind, kind)
			}
		}
	}
	for _, bad := range []string{"", "pod", "healthreport", "checks"} {
		if _, err := parseKind(bad); err == nil {
			t.Errorf("parseKind(%q) should fail", bad)
		}
	}
	if _, err := parseKind("nope"); err == nil || !strings.Contains(err.Error(), "dnschecks/dnscheck/dns") {
		t.Errorf("unknown-kind error should list accepted spellings, got %v", err)
	}
}

func TestKindTable_Invariants(t *testing.T) {
	if got := len(kinds); got != 6 {
		t.Fatalf("expected 6 kinds, got %d", got)
	}
	exec := executableKinds()
	if len(exec) != 4 {
		t.Fatalf("expected 4 executable kinds, got %d", len(exec))
	}
	for _, k := range kinds {
		if k.Executable != k.WritesReports {
			t.Errorf("%s: Executable=%v WritesReports=%v; today these coincide", k.Kind, k.Executable, k.WritesReports)
		}
		if k.New() == nil || k.NewList() == nil {
			t.Errorf("%s: constructors returned nil", k.Kind)
		}
		if got := k.Items(k.NewList()); len(got) != 0 {
			t.Errorf("%s: Items(empty list) = %d items", k.Kind, len(got))
		}
		if kindByName(k.Kind) != k {
			t.Errorf("kindByName(%s) did not round-trip", k.Kind)
		}
	}
	if ch := kindByName("ClusterHealth"); ch.Namespaced {
		t.Error("ClusterHealth must be cluster-scoped")
	}
	if kindByName("Nope") != nil {
		t.Error("kindByName(unknown) should be nil")
	}
}

func TestKindTable_ItemsAndPaused(t *testing.T) {
	ac := kindByName("AddonCheck")
	list := &fathomv1alpha1.AddonCheckList{Items: []fathomv1alpha1.AddonCheck{
		{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: fathomv1alpha1.AddonCheckSpec{Paused: true}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
	}}
	items := ac.Items(list)
	if len(items) != 2 || items[0].GetName() != "a" {
		t.Fatalf("Items = %v", items)
	}
	if !ac.Paused(items[0]) || ac.Paused(items[1]) {
		t.Error("Paused should read spec.paused")
	}
	// Items must alias the list entries, not copy them, so later mutation of
	// the returned object (GVK stamping) is visible through either handle.
	items[1].SetName("renamed")
	if list.Items[1].Name != "renamed" {
		t.Error("Items should return pointers into the list, not copies")
	}

	hc := kindByName("HealthCheck")
	if !hc.Paused(&fathomv1alpha1.HealthCheck{Spec: fathomv1alpha1.HealthCheckSpec{Paused: true}}) {
		t.Error("HealthCheck Paused should read spec.paused")
	}
	ncc := kindByName("NodeCertificateCheck")
	if !ncc.Paused(&fathomv1alpha1.NodeCertificateCheck{Spec: fathomv1alpha1.NodeCertificateCheckSpec{Paused: true}}) {
		t.Error("NodeCertificateCheck Paused should read spec.paused")
	}
	for _, k := range []string{"DNSCheck", "NodeHealthCheck", "ClusterHealth"} {
		d := kindByName(k)
		if d.Paused(d.New()) {
			t.Errorf("%s has no paused field; Paused must be false", k)
		}
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		ns       string
		wantRef  string
		wantRest []string
		wantErr  string
	}{
		{name: "slash form", args: []string{"dnscheck/coredns"}, ns: "team-a", wantRef: "dnscheck/team-a/coredns"},
		{name: "two-arg form", args: []string{"ac", "coredns", "extra"}, ns: "team-a", wantRef: "addoncheck/team-a/coredns", wantRest: []string{"extra"}},
		{name: "cluster-scoped drops namespace", args: []string{"clusterhealth/prod"}, ns: "team-a", wantRef: "clusterhealth/prod"},
		{name: "alias with slash", args: []string{"hc/web"}, ns: "x", wantRef: "healthcheck/x/web"},
		{name: "inline namespace overrides the resolved one", args: []string{"dnscheck/prod/coredns"}, ns: "team-a", wantRef: "dnscheck/prod/coredns"},
		{name: "inline namespace works under -A", args: []string{"ac/prod/coredns"}, ns: "", wantRef: "addoncheck/prod/coredns"},
		{name: "printed form round-trips", args: []string{checkRef{Kind: kindByName("NodeCertificateCheck"), Namespace: "n", Name: "c"}.String()}, ns: "", wantRef: "nodecertificatecheck/n/c"},
		{name: "namespaced kind under -A without inline namespace", args: []string{"dnscheck/coredns"}, ns: "", wantErr: "needs a namespace"},
		{name: "cluster-scoped kind rejects an inline namespace", args: []string{"clusterhealth/ns/prod"}, wantErr: "is cluster-scoped"},
		{name: "too many segments", args: []string{"a/b/c/d"}, wantErr: "cannot parse"},
		{name: "no args", args: nil, wantErr: "a check is required"},
		{name: "kind only", args: []string{"dnscheck"}, wantErr: "a name is required"},
		{name: "empty name after slash", args: []string{"dnscheck/"}, wantErr: "a name is required"},
		{name: "unknown kind", args: []string{"pod/x"}, wantErr: "unknown kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, rest, err := parseTarget(tt.args, tt.ns)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTarget: %v", err)
			}
			if ref.String() != tt.wantRef {
				t.Errorf("ref = %s, want %s", ref, tt.wantRef)
			}
			if len(rest) != len(tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}

// compile-time check that every constructor yields a client.Object.
var _ = []client.Object{&fathomv1alpha1.AddonCheck{}, &fathomv1alpha1.ClusterHealth{}}
