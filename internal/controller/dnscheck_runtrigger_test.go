/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

var _ = Describe("DNSCheck run-now trigger", func() {
	It("records a due token once and never clears it", func() {
		ns := newDNSNamespace(ctx)
		check := createDNSCheck(ctx, ns, "runnow", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
		})
		r := newDNSCheckReconciler(&fakeDNSLauncher{}, 4)

		reconcileDNSCheck(ctx, r, check)
		Expect(reloadDNSCheck(ctx, check).Status.LastRunTrigger).To(BeEmpty())

		// A new token: the annotation write itself reconciles and runs; the
		// controller's job is to record the token as consumed.
		current := reloadDNSCheck(ctx, check)
		current.Annotations = map[string]string{fathomv1alpha1.AnnotationRunNow: "tok-1"}
		Expect(k8sClient.Update(ctx, current)).To(Succeed())
		reconcileDNSCheck(ctx, r, check)
		Expect(reloadDNSCheck(ctx, check).Status.LastRunTrigger).To(Equal("tok-1"))

		// The same token on a later reconcile is not re-recorded (no change).
		reconcileDNSCheck(ctx, r, check)
		Expect(reloadDNSCheck(ctx, check).Status.LastRunTrigger).To(Equal("tok-1"))

		// A run with the annotation removed preserves the consumed token, so the
		// same value re-applied later cannot fire again.
		current = reloadDNSCheck(ctx, check)
		current.Annotations = nil
		Expect(k8sClient.Update(ctx, current)).To(Succeed())
		reconcileDNSCheck(ctx, r, check)
		Expect(reloadDNSCheck(ctx, check).Status.LastRunTrigger).To(Equal("tok-1"))
	})
})
