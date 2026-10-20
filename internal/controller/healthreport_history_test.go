/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fathomv1 "github.com/skaphos/fathom/api/v1"
)

var pruneTestLabels = map[string]string{
	fathomv1.LabelHealthReportSourceKind: "AddonCheck",
	fathomv1.LabelHealthReportSourceName: "check",
}

func pruneTestClient(t *testing.T, reports ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := fathomv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(reports...).Build()
}

func remainingPruneTestReports(t *testing.T, c client.Client) []string {
	t.Helper()
	var list fathomv1.HealthReportList
	if err := c.List(context.Background(), &list, client.InNamespace("default")); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(list.Items))
	for _, r := range list.Items {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return names
}

func observedPruneTestReport(name string, created, observed metav1.Time) *fathomv1.HealthReport {
	r := healthReportForPruneTest(name, created)
	r.Spec.ObservedAt = observed
	return r
}

// Creation time (the API server's clock) decides age before spec.observedAt
// (the operator's clock, which can step back across a failover): a report
// persisted earlier is pruned first even when it was observed later.
func TestPruneHealthReportsOrdersByCreationTimeBeforeObservedAt(t *testing.T) {
	t.Parallel()
	early := metav1.NewTime(time.Unix(1_700_000_000, 0))
	late := metav1.NewTime(time.Unix(1_700_000_100, 0))
	c := pruneTestClient(t,
		observedPruneTestReport("observed-early-created-late", late, early),
		observedPruneTestReport("observed-late-created-early", early, late),
		observedPruneTestReport("newest", late, late),
	)

	deleted := pruneHealthReports(context.Background(), c, logr.Discard(), "default", pruneTestLabels, 2, "")

	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if got := remainingPruneTestReports(t, c); len(got) != 2 || got[0] != "newest" || got[1] != "observed-early-created-late" {
		t.Fatalf("remaining = %v, want the two most recently created", got)
	}
}

// The report a reconciler has just persisted is never pruned, even when a
// clock step makes it observe earlier than its predecessor and the limit is
// one: status.lastReportName is about to point at it.
func TestPruneHealthReportsKeepsTheJustPersistedReportAcrossAClockStep(t *testing.T) {
	t.Parallel()
	early := metav1.NewTime(time.Unix(1_700_000_000, 0))
	late := metav1.NewTime(time.Unix(1_700_000_100, 0))
	c := pruneTestClient(t,
		observedPruneTestReport("previous", early, late),
		observedPruneTestReport("persisted", late, early),
	)

	deleted := pruneHealthReports(context.Background(), c, logr.Discard(), "default", pruneTestLabels, 1, "persisted")

	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if got := remainingPruneTestReports(t, c); len(got) != 1 || got[0] != "persisted" {
		t.Fatalf("remaining = %v, want the just-persisted report", got)
	}
}

// The protected report is never a victim, whatever its age.
func TestPruneHealthReportsNeverDeletesTheProtectedReport(t *testing.T) {
	t.Parallel()
	oldest := metav1.NewTime(time.Unix(1_699_999_000, 0))
	newer := metav1.NewTime(time.Unix(1_700_000_000, 0))
	c := pruneTestClient(t,
		healthReportForPruneTest("protected-but-oldest", oldest),
		healthReportForPruneTest("middle", newer),
		healthReportForPruneTest("newest", newer),
	)

	deleted := pruneHealthReports(context.Background(), c, logr.Discard(), "default", pruneTestLabels, 1, "protected-but-oldest")

	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if got := remainingPruneTestReports(t, c); len(got) != 1 || got[0] != "protected-but-oldest" {
		t.Fatalf("remaining = %v, want only the protected report", got)
	}
}

// A limit below one is defended against: nothing is listed or deleted.
func TestPruneHealthReportsIsANoopBelowOne(t *testing.T) {
	t.Parallel()
	created := metav1.NewTime(time.Unix(1_700_000_000, 0))
	c := pruneTestClient(t, healthReportForPruneTest("only", created))

	for _, limit := range []int{0, -1} {
		if deleted := pruneHealthReports(context.Background(), c, logr.Discard(), "default", pruneTestLabels, limit, ""); deleted != 0 {
			t.Fatalf("limit %d: deleted = %d, want 0", limit, deleted)
		}
	}
	if got := remainingPruneTestReports(t, c); len(got) != 1 {
		t.Fatalf("remaining = %v, want the single report untouched", got)
	}
}

// Only reports matching every selector label are candidates; a report from a
// differently named check in the same namespace is left alone.
func TestPruneHealthReportsHonoursTheLabelSelector(t *testing.T) {
	t.Parallel()
	created := metav1.NewTime(time.Unix(1_700_000_000, 0))
	other := healthReportForPruneTest("other-check", created)
	other.Labels[fathomv1.LabelHealthReportSourceName] = "other"
	c := pruneTestClient(t,
		healthReportForPruneTest("a", created),
		healthReportForPruneTest("b", created),
		other,
	)

	deleted := pruneHealthReports(context.Background(), c, logr.Discard(), "default", pruneTestLabels, 1, "")

	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if got := remainingPruneTestReports(t, c); len(got) != 2 || got[0] != "b" || got[1] != "other-check" {
		t.Fatalf("remaining = %v, want b and the other check's report", got)
	}
}

// The per-check prune selects on the UID label too, so a recreated check of
// the same name neither counts nor deletes its predecessor's retained
// history (spec 014, FR-013a).
func TestPruneHealthReportsIgnoresAPredecessorsReports(t *testing.T) {
	t.Parallel()
	created := metav1.NewTime(time.Unix(1_700_000_000, 0))
	withUID := func(name, uid string) *fathomv1.HealthReport {
		r := healthReportForPruneTest(name, created)
		r.Labels[fathomv1.LabelHealthReportSourceUID] = uid
		return r
	}
	c := pruneTestClient(t, withUID("old-a", "old"), withUID("old-b", "old"), withUID("new-a", "new"), withUID("new-b", "new"))
	labels := map[string]string{
		fathomv1.LabelHealthReportSourceKind: "AddonCheck",
		fathomv1.LabelHealthReportSourceName: "check",
		fathomv1.LabelHealthReportSourceUID:  "new",
	}

	deleted := pruneHealthReports(context.Background(), c, logr.Discard(), "default", labels, 1, "")

	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1 (only the live check's surplus)", deleted)
	}
	if got := remainingPruneTestReports(t, c); len(got) != 3 || got[0] != "new-b" || got[1] != "old-a" || got[2] != "old-b" {
		t.Fatalf("remaining = %v, want the predecessor's two reports untouched", got)
	}
}
