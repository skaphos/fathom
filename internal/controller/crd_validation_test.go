/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// These specs exercise the CRD schema CEL validations (x-kubernetes-validations)
// enforced by the API server (SKA-292). They create real objects against
// envtest and assert the API server accepts valid specs and rejects invalid
// ones. No reconciler is involved — the rules live in the CRD schema, so the
// admission path is the API server itself.

var _ = Describe("CRD schema validation", func() {
	dur := func(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }
	i32 := func(i int32) *int32 { return &i }

	Describe("AddonCheck", func() {
		newAddonCheck := func(spec fathomv1alpha1.AddonCheckSpec) *fathomv1alpha1.AddonCheck {
			spec.AddonType = "coredns"
			return &fathomv1alpha1.AddonCheck{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "vac-", Namespace: "default"},
				Spec:       spec,
			}
		}

		It("accepts a positive timeout within the interval", func() {
			obj := newAddonCheck(fathomv1alpha1.AddonCheckSpec{
				Interval: dur(5 * time.Minute), Timeout: dur(30 * time.Second),
			})
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})

		It("accepts a spec with no interval or timeout set", func() {
			Expect(k8sClient.Create(ctx, newAddonCheck(fathomv1alpha1.AddonCheckSpec{}))).To(Succeed())
		})

		It("rejects a zero timeout", func() {
			err := k8sClient.Create(ctx, newAddonCheck(fathomv1alpha1.AddonCheckSpec{Timeout: dur(0)}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout must be a positive duration"))
		})

		It("rejects a zero interval", func() {
			err := k8sClient.Create(ctx, newAddonCheck(fathomv1alpha1.AddonCheckSpec{Interval: dur(0)}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("interval must be a positive duration"))
		})

		It("rejects a timeout greater than the interval", func() {
			err := k8sClient.Create(ctx, newAddonCheck(fathomv1alpha1.AddonCheckSpec{
				Interval: dur(30 * time.Second), Timeout: dur(time.Minute),
			}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout must not exceed interval"))
		})
	})

	Describe("NodeCertificateCheck", func() {
		newNCC := func(spec fathomv1alpha1.NodeCertificateCheckSpec) *fathomv1alpha1.NodeCertificateCheck {
			return &fathomv1alpha1.NodeCertificateCheck{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "vncc-", Namespace: "default"},
				Spec:       spec,
			}
		}

		It("accepts warnDays >= criticalDays with absolute paths", func() {
			obj := newNCC(fathomv1alpha1.NodeCertificateCheckSpec{
				WarnDays: i32(30), CriticalDays: i32(7),
				Paths: []string{"/etc/kubernetes/pki/apiserver.crt"},
			})
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})

		It("rejects warnDays < criticalDays", func() {
			err := k8sClient.Create(ctx, newNCC(fathomv1alpha1.NodeCertificateCheckSpec{
				WarnDays: i32(5), CriticalDays: i32(10),
			}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("warnDays must be greater than or equal to criticalDays"))
		})

		It("rejects a relative path", func() {
			err := k8sClient.Create(ctx, newNCC(fathomv1alpha1.NodeCertificateCheckSpec{
				Paths: []string{"etc/kubernetes/pki/apiserver.crt"},
			}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("each path must be absolute"))
		})

		It("accepts a positive timeout within the interval", func() {
			obj := newNCC(fathomv1alpha1.NodeCertificateCheckSpec{
				WarnDays: i32(30), CriticalDays: i32(7),
				Interval: dur(time.Hour), Timeout: dur(30 * time.Second),
			})
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})

		It("rejects a zero timeout", func() {
			err := k8sClient.Create(ctx, newNCC(fathomv1alpha1.NodeCertificateCheckSpec{Timeout: dur(0)}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout must be a positive duration"))
		})

		It("rejects a zero interval", func() {
			err := k8sClient.Create(ctx, newNCC(fathomv1alpha1.NodeCertificateCheckSpec{Interval: dur(0)}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("interval must be a positive duration"))
		})

		It("rejects a timeout greater than the interval", func() {
			err := k8sClient.Create(ctx, newNCC(fathomv1alpha1.NodeCertificateCheckSpec{
				Interval: dur(30 * time.Second), Timeout: dur(time.Minute),
			}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeout must not exceed interval"))
		})
	})
})
