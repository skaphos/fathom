/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/pkg/adapter"
)

type fakeAddonAdapter struct{}

func (fakeAddonAdapter) Name() string            { return "fake-cert-manager" }
func (fakeAddonAdapter) Version() string         { return "1.2.3" }
func (fakeAddonAdapter) ContractVersion() string { return adapter.ContractVersion }
func (fakeAddonAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"cert-manager"}, Families: []adapter.Family{"system_health"}}
}
func (fakeAddonAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	return adapter.Result{
		Duration: 25 * time.Millisecond,
		Checks: []adapter.CheckResult{{
			Family:  adapter.Family("system_health"),
			Outcome: adapter.OutcomePass,
			TargetRef: adapter.TargetRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Namespace:  "cert-manager",
				Name:       "cert-manager",
			},
			Summary:    "cert-manager deployment is available",
			Details:    map[string]string{"available": "true"},
			ObservedAt: time.Now(),
			Duration:   10 * time.Millisecond,
		}},
	}, nil
}

var _ = Describe("AddonCheck Controller", func() {
	ctx := context.Background()

	It("records accepted and paused status conditions", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-paused",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "cert-manager",
				Paused:    true,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		controllerReconciler := &AddonCheckReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))

		accepted := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionAccepted)
		Expect(accepted).NotTo(BeNil())
		Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
		Expect(accepted.Reason).To(Equal("SpecAccepted"))

		paused := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionPaused)
		Expect(paused).NotTo(BeNil())
		Expect(paused.Status).To(Equal(metav1.ConditionTrue))
		Expect(paused.Reason).To(Equal("Paused"))

		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("Paused"))
	})

	It("sets Ready false when no adapter is registered", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-missing-adapter",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{AddonType: "cert-manager"},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		controllerReconciler := &AddonCheckReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Adapters: registry.New(logr.Discard()),
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("MissingAdapter"))
		Expect(ready.Message).To(ContainSubstring("cert-manager"))
	})

	It("runs a registered adapter and creates a HealthReport", func() {
		typeNamespacedName := types.NamespacedName{
			Name:      "addoncheck-report",
			Namespace: "default",
		}
		resource := &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      typeNamespacedName.Name,
				Namespace: typeNamespacedName.Namespace,
			},
			Spec: fathomv1alpha1.AddonCheckSpec{
				AddonType: "cert-manager",
				Policy: map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
					"system_health": {Enabled: true, Thresholds: map[string]string{"warnDays": "14"}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, resource))).To(Succeed())
		})

		adapters := registry.New(logr.Discard())
		Expect(adapters.Register(fakeAddonAdapter{})).To(Succeed())
		controllerReconciler := &AddonCheckReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Adapters: adapters,
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &fathomv1alpha1.AddonCheck{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		Expect(updated.Status.LastRunTime).NotTo(BeNil())
		Expect(updated.Status.LastReportName).NotTo(BeEmpty())

		report := &fathomv1alpha1.HealthReport{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: updated.Status.LastReportName, Namespace: typeNamespacedName.Namespace}, report)).To(Succeed())
		Expect(report.Spec.SourceRef.Name).To(Equal(typeNamespacedName.Name))
		Expect(report.Spec.AddonType).To(Equal("cert-manager"))
		Expect(report.Spec.AdapterName).To(Equal("fake-cert-manager"))
		Expect(report.Spec.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
		Expect(report.Spec.Checks).To(HaveLen(1))
		Expect(report.Spec.Checks[0].Family).To(Equal("system_health"))
		Expect(report.Spec.Checks[0].Result).To(Equal(fathomv1alpha1.HealthReportResultPass))

		ready := apiMeta.FindStatusCondition(updated.Status.Conditions, addonCheckConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("RunCompleted"))
	})

	It("ignores deleted AddonChecks", func() {
		controllerReconciler := &AddonCheckReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
