/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1 "github.com/skaphos/fathom/api/v1"
)

// The admission behaviour the stable v1 contract adds on top of the alpha
// rules (specs/014-api-v1-promotion/contracts/api-v1.md): a required
// top-level spec, map-list conditions on AddonCheck, an enumerated
// checkRef.apiVersion, and the removed pause field. These run against the
// real API server because only it evaluates the structural schema, CEL and
// server-side-apply merge semantics; the same cases are asserted end to end
// in test/e2e/crdvalidation_test.go.
var _ = Describe("v1 API contract", func() {
	const ns = "default"

	unstructuredOf := func(kind, name string, spec map[string]any) *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(fathomv1.GroupVersion.WithKind(kind))
		u.SetName(name)
		u.SetNamespace(ns)
		if spec != nil {
			u.Object["spec"] = spec
		}
		return u
	}

	Describe("required spec", func() {
		It("rejects an AddonCheck created without spec", func() {
			u := unstructuredOf("AddonCheck", "v1-contract-nospec", nil)
			err := k8sClient.Create(ctx, u)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec: Required value"))
		})

		It("rejects an update that removes spec, so immutability cannot be bypassed", func() {
			check := &fathomv1.AddonCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "v1-contract-removespec", Namespace: ns},
				Spec:       fathomv1.AddonCheckSpec{AddonType: "cert-manager"},
			}
			Expect(k8sClient.Create(ctx, check)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, check) })

			stripped := unstructuredOf("AddonCheck", check.Name, nil)
			stripped.SetResourceVersion(check.ResourceVersion)
			err := k8sClient.Update(ctx, stripped)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec: Required value"))

			// And the two-step bypass (remove, then re-add with a new
			// immutable value) therefore never gets past step one.
			var current fathomv1.AddonCheck
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: check.Name, Namespace: ns}, &current)).To(Succeed())
			Expect(current.Spec.AddonType).To(Equal("cert-manager"))
		})

		It("rejects a HealthCheck created without spec", func() {
			u := unstructuredOf("HealthCheck", "v1-contract-hc-nospec", nil)
			err := k8sClient.Create(ctx, u)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec: Required value"))
		})
	})

	Describe("AddonCheck conditions", func() {
		It("merges conditions owned by two server-side-apply managers", func() {
			check := &fathomv1.AddonCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "v1-contract-ssa", Namespace: ns},
				Spec:       fathomv1.AddonCheckSpec{AddonType: "cert-manager"},
			}
			Expect(k8sClient.Create(ctx, check)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, check) })

			applyCondition := func(manager, condType string) {
				u := unstructuredOf("AddonCheck", check.Name, nil)
				u.Object["status"] = map[string]any{"conditions": []any{map[string]any{
					"type": condType, "status": "True", "reason": "Test", "message": manager,
					"lastTransitionTime": metav1.Now().UTC().Format("2006-01-02T15:04:05Z"),
				}}}
				Expect(k8sClient.SubResource("status").Apply(ctx, client.ApplyConfigurationFromUnstructured(u),
					client.FieldOwner(manager), client.ForceOwnership)).To(Succeed(), manager)
			}
			applyCondition("manager-a", "ManagerA")
			applyCondition("manager-b", "ManagerB")

			var current fathomv1.AddonCheck
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: check.Name, Namespace: ns}, &current)).To(Succeed())
			seen := map[string]bool{}
			for _, c := range current.Status.Conditions {
				seen[c.Type] = true
			}
			Expect(seen).To(HaveKey("ManagerA"), "the first manager's condition must survive the second apply")
			Expect(seen).To(HaveKey("ManagerB"))
		})
	})

	Describe("HealthCheck checkRef.apiVersion", func() {
		newHC := func(name, apiVersion string) *fathomv1.HealthCheck {
			return &fathomv1.HealthCheck{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: fathomv1.HealthCheckSpec{CheckRef: fathomv1.CheckTargetRef{
					APIVersion: apiVersion, Kind: "AddonCheck", Name: "target",
				}},
			}
		}

		It("rejects a foreign group", func() {
			err := k8sClient.Create(ctx, newHC("v1-contract-foreign", "example.com/v1"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`supported values: "fathom.skaphos.io/v1"`))
		})

		It("rejects the retired alpha version", func() {
			err := k8sClient.Create(ctx, newHC("v1-contract-alpha", "fathom.skaphos.io/v1alpha1"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`supported values: "fathom.skaphos.io/v1"`))
		})

		It("accepts v1 explicitly and when left empty", func() {
			for _, hc := range []*fathomv1.HealthCheck{
				newHC("v1-contract-explicit", "fathom.skaphos.io/v1"),
				newHC("v1-contract-empty", ""),
			} {
				Expect(k8sClient.Create(ctx, hc)).To(Succeed(), hc.Name)
				DeferCleanup(func() { _ = k8sClient.Delete(ctx, hc) })
			}
		})
	})

	Describe("spec.paused", func() {
		pausedNCC := func(name string) *unstructured.Unstructured {
			return unstructuredOf("NodeCertificateCheck", name, map[string]any{"paused": true})
		}

		It("is rejected as an unknown field under strict field validation", func() {
			err := k8sClient.Create(ctx, pausedNCC("v1-contract-paused-strict"),
				client.FieldValidation(metav1.FieldValidationStrict))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`unknown field "spec.paused"`))
		})

		It("is pruned for a client that does not ask for strict validation", func() {
			u := pausedNCC("v1-contract-paused-pruned")
			Expect(k8sClient.Create(ctx, u, client.FieldValidation(metav1.FieldValidationIgnore))).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, u) })

			var current unstructured.Unstructured
			current.SetGroupVersionKind(fathomv1.GroupVersion.WithKind("NodeCertificateCheck"))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: u.GetName(), Namespace: ns}, &current)).To(Succeed())
			_, found, err := unstructured.NestedFieldNoCopy(current.Object, "spec", "paused")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse(), "the server prunes the retired field; the operator never sees it")
		})
	})
})
