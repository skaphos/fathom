/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// Structural spec.policy admission (issue #152): family-key format and count,
// namespace name format and count, numeric-threshold value shapes, and
// label-selector structure are enforced by the CRD schema — feedback lands at
// kubectl apply, not at the first reconcile. Semantic checks (does the
// adapter know this family/key, label grammar, threshold ranges) stay with
// validateAddonCheckPolicy and the Accepted condition.
var _ = Describe("AddonCheck spec.policy admission", func() {
	newCheck := func(policy map[string]fathomv1alpha1.AddonCheckFamilyPolicy) *fathomv1alpha1.AddonCheck {
		return &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "vpol-", Namespace: "default"},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "coredns", Policy: policy},
		}
	}
	family := func(p fathomv1alpha1.AddonCheckFamilyPolicy) map[string]fathomv1alpha1.AddonCheckFamilyPolicy {
		return map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"system_health": p}
	}

	Describe("family keys", func() {
		DescribeTable("format",
			func(key string, wantErr bool) {
				err := k8sClient.Create(ctx, newCheck(map[string]fathomv1alpha1.AddonCheckFamilyPolicy{key: {}}))
				if wantErr {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("policy keys must be"))
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			},
			Entry("accepts an underscore family (live convention)", "api_availability", false),
			Entry("accepts a hyphenated family", "cert-health", false),
			Entry("accepts a single character", "a", false),
			Entry("rejects uppercase", "Certificates", true),
			Entry("rejects a leading hyphen", "-bad", true),
			Entry("rejects a trailing underscore", "bad_", true),
			Entry("rejects 64 characters", fmt.Sprintf("f%063d", 0), true),
		)

		It("rejects more than 32 families", func() {
			policy := make(map[string]fathomv1alpha1.AddonCheckFamilyPolicy, 33)
			for i := 0; i < 33; i++ {
				policy[fmt.Sprintf("family_%02d", i)] = fathomv1alpha1.AddonCheckFamilyPolicy{}
			}
			err := k8sClient.Create(ctx, newCheck(policy))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Too many"))
		})
	})

	Describe("namespaces", func() {
		It("accepts valid namespace names", func() {
			Expect(k8sClient.Create(ctx, newCheck(family(fathomv1alpha1.AddonCheckFamilyPolicy{
				Namespaces: []string{"kube-system", "default", "team-a"},
			})))).To(Succeed())
		})

		DescribeTable("rejects invalid entries",
			func(ns string) {
				err := k8sClient.Create(ctx, newCheck(family(fathomv1alpha1.AddonCheckFamilyPolicy{
					Namespaces: []string{ns},
				})))
				Expect(err).To(HaveOccurred())
			},
			Entry("uppercase and underscore", "Prod_NS"),
			Entry("leading hyphen", "-prod"),
			Entry("64 characters", fmt.Sprintf("n%063d", 0)),
		)

		It("rejects more than 64 entries", func() {
			entries := make([]string, 65)
			for i := range entries {
				entries[i] = fmt.Sprintf("ns-%02d", i)
			}
			err := k8sClient.Create(ctx, newCheck(family(fathomv1alpha1.AddonCheckFamilyPolicy{Namespaces: entries})))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Too many"))
		})
	})

	Describe("thresholds", func() {
		threshold := func(kv map[string]fathomv1alpha1.ThresholdValue) map[string]fathomv1alpha1.AddonCheckFamilyPolicy {
			return family(fathomv1alpha1.AddonCheckFamilyPolicy{Thresholds: kv})
		}

		DescribeTable("numeric key shapes",
			func(kv map[string]fathomv1alpha1.ThresholdValue, wantErrSubstring string) {
				err := k8sClient.Create(ctx, newCheck(threshold(kv)))
				if wantErrSubstring == "" {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(wantErrSubstring))
				}
			},
			Entry("accepts a whole-number warnDays", map[string]fathomv1alpha1.ThresholdValue{"warnDays": "30"}, ""),
			Entry("accepts a whole-number failDays", map[string]fathomv1alpha1.ThresholdValue{"failDays": "7"}, ""),
			Entry("rejects warnDays: banana", map[string]fathomv1alpha1.ThresholdValue{"warnDays": "banana"}, "warnDays and failDays must be whole numbers"),
			Entry("rejects a negative failDays", map[string]fathomv1alpha1.ThresholdValue{"failDays": "-1"}, "warnDays and failDays must be whole numbers"),
			Entry("rejects a fractional warnDays", map[string]fathomv1alpha1.ThresholdValue{"warnDays": "1.5"}, "warnDays and failDays must be whole numbers"),
			Entry("accepts a percentage warnRatio", map[string]fathomv1alpha1.ThresholdValue{"warnRatio": "99.5"}, ""),
			Entry("accepts a percent-suffixed failRatio", map[string]fathomv1alpha1.ThresholdValue{"failRatio": "80%"}, ""),
			Entry("accepts a small-percentage ratio", map[string]fathomv1alpha1.ThresholdValue{"failRatio": "1.5"}, ""),
			Entry("rejects failRatio: banana", map[string]fathomv1alpha1.ThresholdValue{"failRatio": "banana"}, "warnRatio and failRatio must be percentage values"),
			Entry("rejects a four-digit ratio", map[string]fathomv1alpha1.ThresholdValue{"warnRatio": "1000"}, "warnRatio and failRatio must be percentage values"),
			Entry("accepts unknown keys with any value (adapter-judged)", map[string]fathomv1alpha1.ThresholdValue{"customKnob": "anything goes here"}, ""),
			Entry("accepts camelCase keys (live convention)", map[string]fathomv1alpha1.ThresholdValue{"restartWarnCount": "3"}, ""),
		)

		It("rejects more than 16 keys", func() {
			kv := make(map[string]fathomv1alpha1.ThresholdValue, 17)
			for i := 0; i < 17; i++ {
				kv[fmt.Sprintf("knob%02d", i)] = "1"
			}
			err := k8sClient.Create(ctx, newCheck(threshold(kv)))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Too many"))
		})
	})

	Describe("labelSelector", func() {
		selector := func(sel *metav1.LabelSelector) map[string]fathomv1alpha1.AddonCheckFamilyPolicy {
			return family(fathomv1alpha1.AddonCheckFamilyPolicy{LabelSelector: sel})
		}

		It("accepts a valid selector", func() {
			Expect(k8sClient.Create(ctx, newCheck(selector(&metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "coredns"},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"dns"}},
					{Key: "legacy", Operator: metav1.LabelSelectorOpDoesNotExist},
				},
			})))).To(Succeed())
		})

		// A CEL rule for selector structure exceeds the API server's cost
		// budget (the imported LabelSelector schema is unbounded), so
		// structural mistakes pass admission by design and are caught at
		// reconcile time — this spec pins BOTH halves of that split so the
		// documented behavior can't silently regress in either direction.
		It("admits a structurally invalid selector, which reconcile-time validation then rejects", func() {
			broken := &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "a", Operator: metav1.LabelSelectorOpIn}, // In without values
			}}
			obj := newCheck(selector(broken))
			Expect(k8sClient.Create(ctx, obj)).To(Succeed(), "selector structure is not an admission concern")

			problems := validateAddonCheckPolicy(obj, nil)
			Expect(problems).To(HaveLen(1))
			Expect(problems[0]).To(ContainSubstring("invalid labelSelector"))
		})
	})

	It("accepts an empty policy map", func() {
		Expect(k8sClient.Create(ctx, newCheck(map[string]fathomv1alpha1.AddonCheckFamilyPolicy{}))).To(Succeed())
	})
})
