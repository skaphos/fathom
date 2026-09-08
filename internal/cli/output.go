/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
)

// outputFormat is the value of -o/--output. It implements pflag.Value so an
// unsupported format is a parse-time usage error rather than a late failure
// after the cluster has been contacted.
type outputFormat string

const (
	outputTable outputFormat = "table"
	outputJSON  outputFormat = "json"
	outputYAML  outputFormat = "yaml"
)

func (o *outputFormat) String() string {
	if *o == "" {
		return string(outputTable)
	}
	return string(*o)
}

func (o *outputFormat) Set(s string) error {
	switch outputFormat(strings.ToLower(s)) {
	case outputTable, outputJSON, outputYAML:
		*o = outputFormat(strings.ToLower(s))
		return nil
	default:
		return fmt.Errorf("unsupported output format %q (want table, json, or yaml)", s)
	}
}

func (o *outputFormat) Type() string { return "format" }

// structured reports whether the format wants machine-readable output rather
// than a table.
func (o outputFormat) structured() bool {
	return o == outputJSON || o == outputYAML
}

// encode writes v as indented JSON or YAML. Table rendering is the verb's
// job because only the verb knows its columns.
func encode(w io.Writer, format outputFormat, v any) error {
	switch format {
	case outputJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case outputYAML:
		b, err := yaml.Marshal(v)
		if err != nil {
			return fmt.Errorf("encode yaml: %w", err)
		}
		_, err = w.Write(b)
		return err
	default:
		return fmt.Errorf("encode: format %q has no structured encoding", format)
	}
}

// objectList is the shape `-o json` and `-o yaml` emit for list verbs: the
// familiar `kind: List` envelope around the unmodified resources, so the
// output composes with jq and yq the way `kubectl get -o json` does.
type objectList struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Items      []client.Object `json:"items"`
}

// withGVK stamps apiVersion/kind on a typed object. The typed decoder strips
// TypeMeta on the way in, so without this the structured output would lose
// the two fields every consumer keys on.
func withGVK(scheme *runtime.Scheme, obj client.Object) error {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		return fmt.Errorf("resolve kind for %T: %w", obj, err)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return nil
}

// newObjectList builds the List envelope, stamping every item's GVK. Items is
// never nil so an empty result encodes as `"items": []`, not `null`.
func newObjectList(scheme *runtime.Scheme, items []client.Object) (*objectList, error) {
	if items == nil {
		items = []client.Object{}
	}
	for _, it := range items {
		if err := withGVK(scheme, it); err != nil {
			return nil, err
		}
	}
	return &objectList{APIVersion: "v1", Kind: "List", Items: items}, nil
}

// table is a thin tabwriter wrapper with the column conventions the verbs
// share: tab-separated cells, two-space padding, left-aligned, header first.
type table struct {
	tw *tabwriter.Writer
}

func newTable(w io.Writer) *table {
	return &table{tw: tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)}
}

func (t *table) row(cells ...string) {
	_, _ = fmt.Fprintln(t.tw, strings.Join(cells, "\t"))
}

func (t *table) flush() error {
	return t.tw.Flush()
}

// age renders how long ago t was, kubectl style ("5m", "2h", "3d"), or "-"
// when there is no timestamp.
func age(t *metav1.Time, now time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return duration.HumanDuration(now.Sub(t.Time))
}

// until renders how far in the future t is, or "-" when unknown. A time
// already in the past renders as "now" so an overdue run reads honestly.
func until(t *time.Time, now time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	if !t.After(now) {
		return "now"
	}
	return duration.HumanDuration(t.Sub(now))
}

// truncate bounds s to max runes, marking the cut with an ellipsis, and
// flattens newlines so a multi-line summary stays on one table row.
func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// orDash returns "-" for an empty string so empty table cells stay visible.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
