/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func TestOutputFormat_Set(t *testing.T) {
	var o outputFormat
	if o.String() != "table" {
		t.Fatalf("zero value String() = %q, want table", o.String())
	}
	for _, in := range []string{"table", "json", "YAML"} {
		if err := o.Set(in); err != nil {
			t.Errorf("Set(%q): %v", in, err)
		}
	}
	if o != outputYAML {
		t.Errorf("after Set(YAML) = %q, want yaml", o)
	}
	if err := o.Set("csv"); err == nil || !strings.Contains(err.Error(), "unsupported output format") {
		t.Errorf("Set(csv) error = %v, want unsupported", err)
	}
	if !outputJSON.structured() || outputTable.structured() {
		t.Error("structured() should be true for json/yaml and false for table")
	}
}

// TestEncode_ListEnvelopeRoundTrips proves the structured output is the
// `kind: List` envelope around unmodified resources with apiVersion/kind
// stamped, in both encodings, and that an empty list encodes as [] not null.
func TestEncode_ListEnvelopeRoundTrips(t *testing.T) {
	scheme, err := newScheme()
	if err != nil {
		t.Fatal(err)
	}
	hc := &fathomv1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a"},
		Spec:       fathomv1alpha1.HealthCheckSpec{CheckRef: fathomv1alpha1.CheckTargetRef{Kind: "AddonCheck", Name: "coredns"}},
		Status:     fathomv1alpha1.HealthCheckStatus{Result: fathomv1alpha1.HealthReportResultPass, Summary: "ok"},
	}
	list, err := newObjectList(scheme, []client.Object{hc})
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := encode(&out, outputJSON, list); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Items      []struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Metadata   struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Result string `json:"result"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json output does not parse: %v\n%s", err, out.String())
	}
	if decoded.Kind != "List" || decoded.APIVersion != "v1" || len(decoded.Items) != 1 {
		t.Fatalf("unexpected envelope: %+v", decoded)
	}
	it := decoded.Items[0]
	if it.Kind != "HealthCheck" || it.APIVersion != "fathom.skaphos.io/v1alpha1" || it.Metadata.Name != "web" || it.Status.Result != "Pass" {
		t.Fatalf("item not emitted unmodified with GVK: %+v", it)
	}

	out.Reset()
	if err := encode(&out, outputYAML, list); err != nil {
		t.Fatal(err)
	}
	var yamlDecoded map[string]any
	if err := yaml.Unmarshal(out.Bytes(), &yamlDecoded); err != nil {
		t.Fatalf("yaml output does not parse: %v\n%s", err, out.String())
	}
	if yamlDecoded["kind"] != "List" {
		t.Fatalf("yaml envelope kind = %v", yamlDecoded["kind"])
	}

	empty, err := newObjectList(scheme, nil)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := encode(&out, outputJSON, empty); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"items": []`) {
		t.Fatalf("empty list should encode items as [], got:\n%s", out.String())
	}

	if err := encode(&out, outputTable, list); err == nil {
		t.Fatal("encode(table) should refuse: tables are rendered by the verb")
	}
}

func TestTable_AlignsColumns(t *testing.T) {
	var out bytes.Buffer
	tb := newTable(&out)
	tb.row("NAME", "VERDICT")
	tb.row("a-long-name", "Pass")
	tb.row("b", "Fail")
	if err := tb.flush(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), out.String())
	}
	col := strings.Index(lines[0], "VERDICT")
	for i, l := range lines[1:] {
		if idx := strings.Index(l, []string{"Pass", "Fail"}[i]); idx != col {
			t.Errorf("line %d: verdict column at %d, header at %d:\n%s", i+1, idx, col, out.String())
		}
	}
}

func TestAgeUntilTruncate(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	past := metav1.NewTime(now.Add(-5 * time.Minute))
	if got := age(&past, now); got != "5m" {
		t.Errorf("age = %q, want 5m", got)
	}
	if got := age(nil, now); got != "-" {
		t.Errorf("age(nil) = %q, want -", got)
	}
	future := now.Add(90 * time.Second)
	if got := until(&future, now); got != "90s" {
		t.Errorf("until = %q, want 90s", got)
	}
	overdue := now.Add(-time.Second)
	if got := until(&overdue, now); got != "now" {
		t.Errorf("until(past) = %q, want now", got)
	}
	if got := until(nil, now); got != "-" {
		t.Errorf("until(nil) = %q, want -", got)
	}

	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate(short) = %q", got)
	}
	if got := truncate("line one\nline two", 80); got != "line one line two" {
		t.Errorf("truncate should flatten newlines, got %q", got)
	}
	if got := truncate("ünïcödé text that is long", 8); got != "ünïcödé…" {
		t.Errorf("truncate should be rune-safe, got %q", got)
	}
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(empty) = %q", got)
	}
}
