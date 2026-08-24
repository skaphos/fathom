/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// Child-list truncation (skaphos/fathom#277).
//
// Status.Children was unbounded, so a selector matching a large population grew
// the stored object without limit. The cap bounds what the aggregate REPORTS,
// never what it measures — and the ordering matters as much as the cap, because
// an alphabetical cut can drop the single failing or frozen child that explains
// the roll-up.
//
// These are plain unit tests over the fold: the interesting behaviour is in
// truncateChildren, and exercising it through envtest would cost hundreds of
// API objects to prove something a slice already proves.

func childAt(name string, result fathomv1alpha1.HealthReportResult, observed *metav1.Time) fathomv1alpha1.ClusterHealthChildSummary {
	return fathomv1alpha1.ClusterHealthChildSummary{
		Namespace:  "default",
		Name:       name,
		Result:     result,
		ObservedAt: observed,
	}
}

func TestTruncateChildrenLeavesSmallAggregatesAlone(t *testing.T) {
	ch := &fathomv1alpha1.ClusterHealth{}
	now := metav1.Now()
	for i := 0; i < 10; i++ {
		ch.Status.Children = append(ch.Status.Children,
			childAt(string(rune('a'+i)), fathomv1alpha1.HealthReportResultPass, &now))
	}

	truncateChildren(ch)

	if got := len(ch.Status.Children); got != 10 {
		t.Errorf("children = %d, want 10 — an aggregate under the cap must be untouched", got)
	}
	if ch.Status.Children[0].Name != "a" {
		t.Errorf("ordering changed for an untruncated aggregate: first = %q, want %q",
			ch.Status.Children[0].Name, "a")
	}
}

func TestTruncateChildrenCapsAtTheLimit(t *testing.T) {
	ch := &fathomv1alpha1.ClusterHealth{}
	now := metav1.Now()
	total := fathomv1alpha1.MaxClusterHealthChildren + 50
	for i := 0; i < total; i++ {
		ch.Status.Children = append(ch.Status.Children,
			childAt("child", fathomv1alpha1.HealthReportResultPass, &now))
	}
	ch.Status.MatchedCount = int32(total)

	truncateChildren(ch)

	if got := len(ch.Status.Children); got != fathomv1alpha1.MaxClusterHealthChildren {
		t.Errorf("children = %d, want %d", got, fathomv1alpha1.MaxClusterHealthChildren)
	}
	// T032 — truncation must stay detectable.
	if int(ch.Status.MatchedCount) != total {
		t.Errorf("matchedCount = %d, want %d — the full total is the only signal that truncation happened",
			ch.Status.MatchedCount, total)
	}
	if int(ch.Status.MatchedCount) <= len(ch.Status.Children) {
		t.Error("matchedCount must exceed len(children) when truncated, or a consumer cannot tell")
	}
}

// T033 — the entries an operator needs must survive the cut.
func TestTruncateChildrenKeepsTheFailingAndFrozenChildren(t *testing.T) {
	ch := &fathomv1alpha1.ClusterHealth{}
	now := metav1.Now()
	frozen := metav1.NewTime(time.Now().Add(-24 * time.Hour))

	// The needles are named to sort LAST alphabetically, so a naive
	// namespace/name truncation would drop exactly them.
	for i := 0; i < fathomv1alpha1.MaxClusterHealthChildren+40; i++ {
		ch.Status.Children = append(ch.Status.Children,
			childAt("aaa-healthy", fathomv1alpha1.HealthReportResultPass, &now))
	}
	ch.Status.Children = append(ch.Status.Children,
		childAt("zzz-failing", fathomv1alpha1.HealthReportResultFail, &now),
		childAt("zzz-frozen", fathomv1alpha1.HealthReportResultPass, &frozen),
		childAt("zzz-never-ran", fathomv1alpha1.HealthReportResultPass, nil),
	)

	truncateChildren(ch)

	if got := len(ch.Status.Children); got != fathomv1alpha1.MaxClusterHealthChildren {
		t.Fatalf("children = %d, want %d", got, fathomv1alpha1.MaxClusterHealthChildren)
	}

	kept := map[string]bool{}
	for _, c := range ch.Status.Children {
		kept[c.Name] = true
	}
	for _, want := range []string{"zzz-failing", "zzz-frozen", "zzz-never-ran"} {
		if !kept[want] {
			t.Errorf("%s was truncated away; truncation must not hide the children that explain the roll-up", want)
		}
	}

	// Worst verdict ranks ahead of mere staleness.
	if ch.Status.Children[0].Name != "zzz-failing" {
		t.Errorf("first child = %q, want the failing one to sort first", ch.Status.Children[0].Name)
	}
	// A never-observed child is maximally stale, so it outranks a timestamp.
	if ch.Status.Children[1].Name != "zzz-never-ran" {
		t.Errorf("second child = %q, want the never-observed one (maximally stale)", ch.Status.Children[1].Name)
	}
	if ch.Status.Children[2].Name != "zzz-frozen" {
		t.Errorf("third child = %q, want the frozen one", ch.Status.Children[2].Name)
	}
}

func TestTruncateChildrenIsDeterministic(t *testing.T) {
	// Both fixtures must carry identical inputs, so the timestamps are pinned
	// rather than sampled per build — two metav1.Now() calls would differ and
	// the comparison would fail for reasons unrelated to ordering.
	base := metav1.NewTime(time.Unix(1_700_000_000, 0).UTC())

	build := func() *fathomv1alpha1.ClusterHealth {
		ch := &fathomv1alpha1.ClusterHealth{}
		for i := 0; i < fathomv1alpha1.MaxClusterHealthChildren+20; i++ {
			// A spread of verdicts and ages, so ties are actually exercised
			// rather than every child ranking identically.
			observed := metav1.NewTime(base.Add(time.Duration(i%7) * time.Minute))
			result := fathomv1alpha1.HealthReportResultPass
			if i%5 == 0 {
				result = fathomv1alpha1.HealthReportResultWarn
			}
			ch.Status.Children = append(ch.Status.Children,
				childAt(fmt.Sprintf("child-%03d", i), result, &observed))
		}
		return ch
	}

	a, b := build(), build()
	truncateChildren(a)
	truncateChildren(b)

	if len(a.Status.Children) != len(b.Status.Children) {
		t.Fatalf("lengths differ: %d vs %d", len(a.Status.Children), len(b.Status.Children))
	}
	for i := range a.Status.Children {
		x, y := a.Status.Children[i], b.Status.Children[i]
		// Compare by identity and rank; the summaries hold pointers, so a
		// struct comparison would test pointer equality rather than ordering.
		if x.Namespace != y.Namespace || x.Name != y.Name || x.Result != y.Result {
			t.Fatalf("truncation is not deterministic at index %d: %s/%s vs %s/%s",
				i, x.Namespace, x.Name, y.Namespace, y.Name)
		}
	}
}

// A HealthCheck that has never reconciled carries an EMPTY verdict and no
// observation. Empty ranks Severity()=0 — below Pass — so ranking on the raw
// value made it the FIRST entry truncated away, inverting the whole point of
// ordered truncation: the child with no verdict at all is the strongest
// staleness signal an aggregate has. Found by adversarial review of #277; the
// original test masked it by giving the never-observed child a Pass verdict.
func TestTruncateChildrenKeepsTheNeverReconciledChild(t *testing.T) {
	ch := &fathomv1alpha1.ClusterHealth{}
	now := metav1.Now()
	for i := 0; i < fathomv1alpha1.MaxClusterHealthChildren+40; i++ {
		ch.Status.Children = append(ch.Status.Children,
			childAt(fmt.Sprintf("healthy-%03d", i), fathomv1alpha1.HealthReportResultPass, &now))
	}
	// The real shape of a never-reconciled child, named to sort last.
	ch.Status.Children = append(ch.Status.Children,
		childAt("zzz-never-reconciled", "", nil))

	truncateChildren(ch)

	for _, c := range ch.Status.Children {
		if c.Name == "zzz-never-reconciled" {
			return
		}
	}
	t.Fatal("the never-reconciled child was truncated away; an empty verdict must rank as Unknown, " +
		"not below Pass, or the one child that explains a stale roll-up is the first one dropped")
}

// An empty verdict must rank exactly where the roll-up fold already puts it.
func TestRankSeverityCoercesEmptyToUnknown(t *testing.T) {
	if got, want := rankSeverity(""), fathomv1alpha1.HealthReportResultUnknown.Severity(); got != want {
		t.Errorf("rankSeverity(\"\") = %d, want %d (Unknown)", got, want)
	}
	if rankSeverity("") <= rankSeverity(fathomv1alpha1.HealthReportResultPass) {
		t.Error("an unevaluated child must outrank a passing one for truncation")
	}
	if rankSeverity("") >= rankSeverity(fathomv1alpha1.HealthReportResultFail) {
		t.Error("a live Fail must still outrank an unevaluated child")
	}
}
