/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/internal/adapter/impersonation"
)

// This suite proves the mechanism the whole least-privilege model rests on
// (SKA-58): a client that impersonates a per-addon ServiceAccount is authorized
// AS that ServiceAccount, so it can read only what the SA's ClusterRole grants.
// envtest runs the apiserver with authorization-mode=RBAC, so the scoping is real
// — the admin client is system:masters (superuser) but that bypass does not carry
// through impersonation.
var _ = Describe("Per-addon ServiceAccount impersonation", func() {
	const (
		nsName   = "fathom-imp-test"
		saName   = "addon-scoped-reader"
		roleName = "addon-scoped-reader"
	)

	BeforeEach(func() {
		By("creating the namespace, ServiceAccount, and a ClusterRole that grants configmaps reads only")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())

		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: nsName}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, sa))).To(Succeed())

		role := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: roleName},
			Rules: []rbacv1.PolicyRule{{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "list", "watch"},
			}},
		}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, role))).To(Succeed())

		binding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: roleName},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: roleName},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: nsName}},
		}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, binding))).To(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: roleName}})
		_ = k8sClient.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: roleName}})
	})

	newImpersonatingClient := func() client.Client {
		cfgCopy := rest.CopyConfig(cfg)
		cfgCopy.Impersonate = rest.ImpersonationConfig{UserName: impersonation.SAUsername(nsName, saName)}
		c, err := client.New(cfgCopy, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		return c
	}

	It("permits reads the ServiceAccount's role grants", func() {
		impClient := newImpersonatingClient()
		// RBAC binding propagation to the authorizer can lag object creation.
		Eventually(func() error {
			return impClient.List(ctx, &corev1.ConfigMapList{}, client.InNamespace(nsName))
		}).Should(Succeed(), "impersonated client should be allowed to list configmaps")
	})

	It("denies reads outside the ServiceAccount's role with Forbidden", func() {
		impClient := newImpersonatingClient()
		// The role grants configmaps only; secrets must be Forbidden. If envtest
		// were NOT enforcing RBAC (it is, authorization-mode=RBAC) this would pass
		// and the assertion would (correctly) fail, flagging a lost guarantee.
		Eventually(func() bool {
			err := impClient.List(ctx, &corev1.SecretList{}, client.InNamespace(nsName))
			return apierrors.IsForbidden(err)
		}).Should(BeTrue(), "impersonated client must be Forbidden from listing secrets it was not granted")
	})
})
