/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

var _ = Describe("HealthReport idempotency", func() {
	ctx := context.Background()

	newExpectedReport := func() *fathomv1alpha1.HealthReport {
		return &fathomv1alpha1.HealthReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hr-reuse-collision",
				Namespace: "default",
				Labels: map[string]string{
					labelHealthReportSourceKind: "AddonCheck",
					labelHealthReportSourceName: "expected-check",
				},
			},
			Spec: fathomv1alpha1.HealthReportSpec{
				SourceRef: fathomv1alpha1.HealthReportTargetRef{
					APIVersion: fathomv1alpha1.GroupVersion.String(),
					Kind:       "AddonCheck",
					Namespace:  "default",
					Name:       "expected-check",
				},
				Result:     fathomv1alpha1.HealthReportResultPass,
				ObservedAt: metav1.Now(),
			},
		}
	}

	DescribeTable("rejects mismatched reports before reuse",
		func(mutate func(*fathomv1alpha1.HealthReport), message string) {
			expected := newExpectedReport()
			existing := expected.DeepCopy()
			mutate(existing)

			err := validateReusableHealthReport(existing, expected)
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("sourceRef", func(report *fathomv1alpha1.HealthReport) {
			report.Spec.SourceRef.Name = "other-check"
		}, "mismatched sourceRef"),
		Entry("source label", func(report *fathomv1alpha1.HealthReport) {
			report.Labels[labelHealthReportSourceName] = "other-check"
		}, "mismatched label"),
		Entry("result", func(report *fathomv1alpha1.HealthReport) {
			report.Spec.Result = fathomv1alpha1.HealthReportResultFail
		}, "mismatched result"),
	)

	It("rejects an existing deterministic name collision with different identity", func() {
		expected := newExpectedReport()
		existing := expected.DeepCopy()
		existing.Spec.SourceRef.Name = "other-check"
		existing.Labels[labelHealthReportSourceName] = "other-check"
		Expect(k8sClient.Create(ctx, existing)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, existing))).To(Succeed()) })

		report, created, err := createOrReuseHealthReport(ctx, k8sClient, expected)
		Expect(err).To(MatchError(ContainSubstring("mismatched sourceRef")))
		Expect(created).To(BeFalse())
		Expect(report).To(BeNil())
	})
})
