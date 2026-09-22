/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
)

var _ = Describe("runtime authority dependency watches", func() {
	It("withdraws binding and definition readiness when a dedicated ServiceAccount is recreated", func() {
		const namespace = "fathom-runtime-watch-test"
		const addon = "watch-envtest-addon"
		const saName = "watch-envtest-addon-reader"
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}) })

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: k8sClient.Scheme(), Metrics: server.Options{BindAddress: "0"}, HealthProbeBindAddress: "0"})
		Expect(err).NotTo(HaveOccurred())
		reg := registry.New(logr.Discard())
		cacheSynced := mgr.GetCache().WaitForCacheSync
		definition := &AddonDefinitionReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Registry: reg,
			OperatorNamespace: namespace, ManagerServiceAccount: lifecycleManagerSA,
			OperatorBuild: lifecycleBuild, CacheSynced: cacheSynced}
		binding := &AddonDefinitionBindingReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Registry: reg,
			OperatorNamespace: namespace, ManagerServiceAccount: lifecycleManagerSA, CacheSynced: cacheSynced}
		Expect(definition.SetupWithManager(ctx, mgr)).To(Succeed())
		Expect(binding.SetupWithManager(ctx, mgr)).To(Succeed())
		managerCtx, stop := context.WithCancel(ctx)
		finished := make(chan error, 1)
		go func() { finished <- mgr.Start(managerCtx) }()
		DeferCleanup(func() { stop(); Expect(<-finished).To(Succeed()) })

		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: namespace}}
		Expect(k8sClient.Create(ctx, sa)).To(Succeed())
		def := lifecycleDefinitionNamed(addon, "")
		def.UID, def.Generation = "", 0
		Expect(k8sClient.Create(ctx, def)).To(Succeed())
		bound := lifecycleBindingNamed(addon, "", def.UID, saName, sa.UID)
		bound.Namespace, bound.UID, bound.Generation = namespace, "", 0
		Expect(k8sClient.Create(ctx, bound)).To(Succeed())

		ready := func(g Gomega, want metav1.ConditionStatus, reason string) {
			stored := &fathomv1alpha1.AddonDefinitionBinding{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: addon}, stored)).To(Succeed())
			condition, ok := conditionByType(stored.Status.Conditions, definitionConditionReady)
			g.Expect(ok).To(BeTrue())
			g.Expect(condition.Status).To(Equal(want))
			g.Expect(condition.Reason).To(Equal(reason))
		}
		definitionReady := func(g Gomega, want metav1.ConditionStatus, reason string) {
			stored := &fathomv1alpha1.AddonDefinition{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: addon}, stored)).To(Succeed())
			condition, ok := conditionByType(stored.Status.Conditions, definitionConditionReady)
			g.Expect(ok).To(BeTrue())
			g.Expect(condition.Status).To(Equal(want))
			g.Expect(condition.Reason).To(Equal(reason))
		}
		Eventually(ready).WithArguments(metav1.ConditionTrue, reasonBindingAuthorized).Within(15 * time.Second).Should(Succeed())
		Eventually(definitionReady).WithArguments(metav1.ConditionTrue, reasonRuntimePublished).Within(15 * time.Second).Should(Succeed())

		Expect(k8sClient.Delete(ctx, sa)).To(Succeed())
		replacement := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: namespace}}
		Eventually(func() error { return k8sClient.Create(ctx, replacement) }).Within(5 * time.Second).Should(Succeed())
		Expect(replacement.UID).NotTo(Equal(sa.UID))
		Eventually(ready).WithArguments(metav1.ConditionFalse, reasonBindingMismatch).Within(10 * time.Second).Should(Succeed())
		Eventually(definitionReady).WithArguments(metav1.ConditionFalse, reasonBindingMismatch).Within(10 * time.Second).Should(Succeed())

		// The reference is immutable: reauthorization requires replacing the
		// binding under administrator control, never a status-only write.
		Expect(k8sClient.Delete(ctx, bound)).To(Succeed())
		reauthorized := lifecycleBindingNamed(addon, "", def.UID, saName, replacement.UID)
		reauthorized.Namespace, reauthorized.UID, reauthorized.Generation = namespace, "", 0
		Eventually(func() error { return k8sClient.Create(ctx, reauthorized) }).Within(5 * time.Second).Should(Succeed())
		Eventually(ready).WithArguments(metav1.ConditionTrue, reasonBindingAuthorized).Within(10 * time.Second).Should(Succeed())
		Eventually(definitionReady).WithArguments(metav1.ConditionTrue, reasonRuntimePublished).Within(10 * time.Second).Should(Succeed())

		peer := lifecycleBindingNamed("watch-envtest-peer", "", "other-definition-uid", saName, replacement.UID)
		peer.Namespace, peer.UID, peer.Generation = namespace, "", 0
		Expect(k8sClient.Create(ctx, peer)).To(Succeed())
		Eventually(ready).WithArguments(metav1.ConditionFalse, reasonBindingMismatch).Within(10 * time.Second).Should(Succeed())
		Eventually(definitionReady).WithArguments(metav1.ConditionFalse, reasonBindingMismatch).Within(10 * time.Second).Should(Succeed())
		Expect(k8sClient.Delete(ctx, peer)).To(Succeed())
		Eventually(ready).WithArguments(metav1.ConditionTrue, reasonBindingAuthorized).Within(10 * time.Second).Should(Succeed())
		Eventually(definitionReady).WithArguments(metav1.ConditionTrue, reasonRuntimePublished).Within(10 * time.Second).Should(Succeed())
	})
})
