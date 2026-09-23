/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func TestPruneHealthReportHistoryProtectsCreatedReportAndOrdersTies(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	limit := int32(2)
	check := &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "default"},
		Spec:       fathomv1alpha1.AddonCheckSpec{HistoryLimit: &limit},
	}
	oldest := metav1.NewTime(time.Unix(1_699_999_999, 0))
	tied := metav1.NewTime(time.Unix(1_700_000_000, 0))
	reports := []client.Object{
		healthReportForPruneTest("new-report", tied),
		healthReportForPruneTest("seed-a", tied),
		healthReportForPruneTest("seed-b", tied),
		healthReportForPruneTest("oldest", oldest),
	}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(reports...).Build()
	ordered := interceptor.NewClient(base, interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if err := c.List(ctx, list, opts...); err != nil {
				return err
			}
			items := list.(*fathomv1alpha1.HealthReportList).Items
			movePruneTestReport(items, "new-report", 0)
			// Reverse the tied seeds so the expected survivor also proves that
			// the name tie-break, rather than API list order, decides retention.
			movePruneTestReport(items, "seed-b", 1)
			return nil
		},
	})

	(&AddonCheckReconciler{Client: ordered}).pruneHealthReportHistory(
		context.Background(), logr.Discard(), check, "new-report",
	)

	var survivors fathomv1alpha1.HealthReportList
	if err := base.List(context.Background(), &survivors); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(survivors.Items))
	for i := range survivors.Items {
		got[survivors.Items[i].Name] = true
	}
	if len(got) != int(limit) || !got["new-report"] || !got["seed-b"] {
		t.Fatalf("survivors = %v, want protected new-report and deterministic newest tie seed-b", got)
	}
}

func movePruneTestReport(reports []fathomv1alpha1.HealthReport, name string, target int) {
	for i := range reports {
		if reports[i].Name == name {
			reports[target], reports[i] = reports[i], reports[target]
			return
		}
	}
}

func healthReportForPruneTest(name string, created metav1.Time) *fathomv1alpha1.HealthReport {
	return &fathomv1alpha1.HealthReport{ObjectMeta: metav1.ObjectMeta{
		Name:              name,
		Namespace:         "default",
		CreationTimestamp: created,
		Labels: map[string]string{
			fathomv1alpha1.LabelHealthReportSourceKind: "AddonCheck",
			fathomv1alpha1.LabelHealthReportSourceName: "check",
		},
	}}
}
