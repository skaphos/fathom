/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
)

var _ = DescribeTable("node-agent RBAC migration through the filtered manager cache",
	func(owner client.Object) {
		owner = owner.DeepCopyObject().(client.Object)
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "node-agent-cache-"}}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, namespace))).To(Succeed()) })

		owner.SetNamespace(namespace.Name)
		owner.SetName("migration")
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())

		roleName := "legacy-" + namespace.Name
		legacyRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: roleName},
			Rules: []rbacv1.PolicyRule{{
				APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create", "get", "update"},
			}},
		}
		Expect(k8sClient.Create(ctx, legacyRole)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, legacyRole))).To(Succeed()) })

		agentName := nodeAgentName(owner)
		binding := legacyNodeAgentBinding(owner, agentName, roleName)
		Expect(k8sClient.Create(ctx, binding)).To(Succeed())

		cachedClient, stopCache := newFilteredNodeAgentClient()
		DeferCleanup(stopCache)
		Expect(cachedClient.Get(ctx, client.ObjectKeyFromObject(binding), &rbacv1.RoleBinding{})).To(MatchError(apierrors.IsNotFound, "be NotFound"))

		Eventually(func(g Gomega) {
			allowed, err := serviceAccountCanGetConfigMap(namespace.Name, agentName, "unrelated")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(allowed).To(BeTrue())
		}).Should(Succeed())

		ensureNodeAgentRBAC(ctx, cachedClient, k8sClient, owner, roleName)
		Eventually(func(g Gomega) {
			got := &rbacv1.RoleBinding{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(binding), got)).To(Succeed())
			g.Expect(got.Subjects).To(BeEmpty())
			allowed, err := serviceAccountCanGetConfigMap(namespace.Name, agentName, "unrelated")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(allowed).To(BeFalse())
		}).Should(Succeed())
	},
	Entry("for NodeCertificateCheck", &fathomv1alpha1.NodeCertificateCheck{}),
	Entry("for NodeHealthCheck", &fathomv1alpha1.NodeHealthCheck{Spec: fathomv1alpha1.NodeHealthCheckSpec{
		Checks: []fathomv1alpha1.NodeHealthCheckItem{{Type: fathomv1alpha1.NodeHealthCheckNodeCondition}},
	}}),
)

var _ = DescribeTable("node-agent teardown through the filtered manager cache",
	func(owner client.Object) {
		owner = owner.DeepCopyObject().(client.Object)
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "node-agent-cache-"}}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, namespace))).To(Succeed()) })

		owner.SetNamespace(namespace.Name)
		owner.SetName("teardown")
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())

		roleName := "legacy-" + namespace.Name
		agentName := nodeAgentName(owner)
		ds := unlabeledNodeAgentDaemonSet(owner, agentName)
		Expect(k8sClient.Create(ctx, ds)).To(Succeed())

		cachedClient, stopCache := newFilteredNodeAgentClient()
		DeferCleanup(stopCache)
		Expect(cachedClient.Get(ctx, client.ObjectKeyFromObject(ds), &appsv1.DaemonSet{})).To(MatchError(apierrors.IsNotFound, "be NotFound"))

		Expect(revokeNodeAgent(ctx, cachedClient, k8sClient, owner, roleName)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(ds), &appsv1.DaemonSet{})
		}).Should(MatchError(apierrors.IsNotFound, "be NotFound"))

		foreignOwner := owner.DeepCopyObject().(client.Object)
		foreignOwner.SetResourceVersion("")
		foreignOwner.SetUID("")
		foreignOwner.SetName("foreign")
		Expect(k8sClient.Create(ctx, foreignOwner)).To(Succeed())
		foreignName := nodeAgentName(foreignOwner)
		foreignBinding := legacyNodeAgentBinding(nil, foreignName, roleName)
		foreignBinding.Namespace = namespace.Name
		Expect(k8sClient.Create(ctx, foreignBinding)).To(Succeed())
		foreignDS := unlabeledNodeAgentDaemonSet(nil, foreignName)
		foreignDS.Namespace = namespace.Name
		Expect(k8sClient.Create(ctx, foreignDS)).To(Succeed())

		err := revokeNodeAgent(ctx, cachedClient, k8sClient, foreignOwner, roleName)
		Expect(err).To(MatchError(And(
			ContainSubstring("rolebinding"),
			ContainSubstring("not controlled"),
			ContainSubstring("DaemonSet"),
		)))
		preservedBinding := &rbacv1.RoleBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(foreignBinding), preservedBinding)).To(Succeed())
		Expect(preservedBinding.Subjects).To(HaveLen(1))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(foreignDS), &appsv1.DaemonSet{})).To(Succeed())
	},
	Entry("for NodeCertificateCheck", &fathomv1alpha1.NodeCertificateCheck{}),
	Entry("for NodeHealthCheck", &fathomv1alpha1.NodeHealthCheck{Spec: fathomv1alpha1.NodeHealthCheckSpec{
		Checks: []fathomv1alpha1.NodeHealthCheckItem{{Type: fathomv1alpha1.NodeHealthCheckNodeCondition}},
	}}),
)

func newFilteredNodeAgentClient() (client.Client, func()) {
	filter := cache.ByObject{Label: labels.SelectorFromSet(labels.Set{
		nodecert.LabelManagedBy: nodecert.ManagedByValue,
	})}
	testCache, err := cache.New(cfg, cache.Options{
		Scheme: k8sscheme.Scheme,
		ByObject: map[client.Object]cache.ByObject{
			&appsv1.DaemonSet{}:   filter,
			&rbacv1.RoleBinding{}: filter,
		},
	})
	Expect(err).NotTo(HaveOccurred())

	cacheCtx, stop := context.WithCancel(ctx)
	stopped := make(chan error, 1)
	go func() { stopped <- testCache.Start(cacheCtx) }()
	Expect(testCache.WaitForCacheSync(cacheCtx)).To(BeTrue())

	cachedClient, err := client.New(cfg, client.Options{
		Scheme: k8sscheme.Scheme,
		Cache:  &client.CacheOptions{Reader: testCache},
	})
	Expect(err).NotTo(HaveOccurred())
	return cachedClient, func() {
		stop()
		Eventually(stopped).Should(Receive(BeNil()))
	}
}

func nodeAgentName(owner client.Object) string {
	switch check := owner.(type) {
	case *fathomv1alpha1.NodeCertificateCheck:
		return agentResourceName(check)
	case *fathomv1alpha1.NodeHealthCheck:
		return nodeHealthAgentResourceName(check)
	default:
		panic("unsupported node check type")
	}
}

func ensureNodeAgentRBAC(ctx context.Context, c client.Client, reader client.Reader, owner client.Object, roleName string) {
	switch check := owner.(type) {
	case *fathomv1alpha1.NodeCertificateCheck:
		r := &NodeCertificateCheckReconciler{Client: c, APIReader: reader, Scheme: k8sscheme.Scheme, NodeAgentRoleName: roleName}
		_, err := r.ensureAgentRBAC(ctx, check)
		Expect(err).NotTo(HaveOccurred())
	case *fathomv1alpha1.NodeHealthCheck:
		r := &NodeHealthCheckReconciler{Client: c, APIReader: reader, Scheme: k8sscheme.Scheme, NodeAgentRoleName: roleName}
		_, err := r.ensureAgentRBAC(ctx, check)
		Expect(err).NotTo(HaveOccurred())
	default:
		panic("unsupported node check type")
	}
}

func revokeNodeAgent(ctx context.Context, c client.Client, reader client.Reader, owner client.Object, roleName string) error {
	switch check := owner.(type) {
	case *fathomv1alpha1.NodeCertificateCheck:
		return (&NodeCertificateCheckReconciler{Client: c, APIReader: reader, NodeAgentRoleName: roleName}).revokeNodeCertAgent(ctx, check)
	case *fathomv1alpha1.NodeHealthCheck:
		return (&NodeHealthCheckReconciler{Client: c, APIReader: reader, NodeAgentRoleName: roleName}).revokeAgent(ctx, check)
	default:
		panic("unsupported node check type")
	}
}

func legacyNodeAgentBinding(owner client.Object, name, roleName string) *rbacv1.RoleBinding {
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: roleName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: name}},
	}
	if owner == nil {
		return binding
	}
	binding.Namespace = owner.GetNamespace()
	binding.Subjects[0].Namespace = owner.GetNamespace()
	Expect(controllerutil.SetControllerReference(owner, binding, k8sscheme.Scheme)).To(Succeed())
	return binding
}

func unlabeledNodeAgentDaemonSet(owner client.Object, name string) *appsv1.DaemonSet {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"agent": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"agent": name}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: "example.invalid/agent:test"}}},
			},
		},
	}
	if owner == nil {
		return ds
	}
	ds.Namespace = owner.GetNamespace()
	Expect(controllerutil.SetControllerReference(owner, ds, k8sscheme.Scheme)).To(Succeed())
	return ds
}

func serviceAccountCanGetConfigMap(namespace, serviceAccount, name string) (bool, error) {
	review := &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{
		User: "system:serviceaccount:" + namespace + ":" + serviceAccount,
		ResourceAttributes: &authorizationv1.ResourceAttributes{
			Namespace: namespace, Verb: "get", Resource: "configmaps", Name: name,
		},
	}}
	if err := k8sClient.Create(ctx, review); err != nil {
		return false, err
	}
	return review.Status.Allowed, nil
}

func TestScopedReportAccessNameBoundsMaximumServiceAccount(t *testing.T) {
	serviceAccount := strings.Repeat("a", 253)
	got := scopedReportAccessName(serviceAccount)
	if len(got) != 53 || !strings.HasPrefix(got, "fathom-report-access-") {
		t.Fatalf("scopedReportAccessName(maximum ServiceAccount) = %q (len %d)", got, len(got))
	}
	if got != scopedReportAccessName(serviceAccount) {
		t.Fatal("scopedReportAccessName is not deterministic")
	}
	if got == scopedReportAccessName(strings.Repeat("b", 253)) {
		t.Fatal("distinct maximum-length ServiceAccounts produced the same scoped name")
	}
}

func TestActiveAgentReportNamesFiltersAndDeduplicatesPods(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	controller := true
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns", UID: types.UID("agent-uid")}}
	owned := []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: &controller}}
	deletedAt := metav1.NewTime(time.Now())
	labels := map[string]string{"agent": "one"}
	pods := []client.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "active-a", Namespace: "ns", Labels: labels, OwnerReferences: owned}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "active-a-rollout", Namespace: "ns", Labels: labels, OwnerReferences: owned}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "ns", Labels: labels}, Spec: corev1.PodSpec{NodeName: "node-b"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "unscheduled", Namespace: "ns", Labels: labels, OwnerReferences: owned}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "terminating", Namespace: "ns", Labels: labels, OwnerReferences: owned, DeletionTimestamp: &deletedAt, Finalizers: []string{"test"}}, Spec: corev1.PodSpec{NodeName: "node-c"}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pods...).Build()
	names, err := activeAgentReportNames(context.Background(), c, "ns", labels, ds, func(node string) string { return "report-" + node })
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "report-node-a" {
		t.Fatalf("activeAgentReportNames() = %v, want [report-node-a]", names)
	}
}

func TestEnsureScopedReportRBACRejectsUnexpectedImmutableRoleRef(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	name := scopedReportAccessName("agent")
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", CreationTimestamp: metav1.Now()},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "foreign"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, binding).Build()
	err := ensureScopedReportRBAC(context.Background(), c, scheme, owner, nil, "agent", []string{"report-node-a"})
	if err == nil || !strings.Contains(err.Error(), "immutable roleRef") {
		t.Fatalf("ensureScopedReportRBAC() error = %v, want immutable roleRef error", err)
	}
}

func TestClearNodeAgentAccessDoesNotCreateMissingRBAC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
	if err := clearNodeAgentAccess(context.Background(), c, c, owner, "never-provisioned", defaultNodeAgentRoleName); err != nil {
		t.Fatalf("clearNodeAgentAccess() error = %v", err)
	}
	var roles rbacv1.RoleList
	if err := c.List(context.Background(), &roles, client.InNamespace("ns")); err != nil {
		t.Fatal(err)
	}
	if len(roles.Items) != 0 {
		t.Fatalf("clearNodeAgentAccess() created roles: %v", roles.Items)
	}
	var bindings rbacv1.RoleBindingList
	if err := c.List(context.Background(), &bindings, client.InNamespace("ns")); err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 0 {
		t.Fatalf("clearNodeAgentAccess() created bindings: %v", bindings.Items)
	}
}

func TestClearNodeAgentAccessAttemptsBothGrantsWhenUpdatesFail(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	controller := true
	owned := []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: owner.Name, UID: owner.UID, Controller: &controller}}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: scopedReportAccessName("agent"), Namespace: "ns", OwnerReferences: owned},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}}},
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns", OwnerReferences: owned},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: defaultNodeAgentRoleName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "agent", Namespace: "ns"}},
	}
	roleDenied := errors.New("role update denied")
	bindingDenied := errors.New("binding update denied")
	var roleUpdates, bindingUpdates int
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, role, binding).
		WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			switch obj.(type) {
			case *rbacv1.Role:
				roleUpdates++
				return roleDenied
			case *rbacv1.RoleBinding:
				bindingUpdates++
				return bindingDenied
			default:
				return c.Update(ctx, obj, opts...)
			}
		}}).Build()
	err := clearNodeAgentAccess(context.Background(), c, c, owner, "agent", defaultNodeAgentRoleName)
	if !errors.Is(err, roleDenied) || !errors.Is(err, bindingDenied) {
		t.Fatalf("clearNodeAgentAccess() error = %v, want both update failures", err)
	}
	if roleUpdates != 1 || bindingUpdates != 1 {
		t.Fatalf("update attempts = Role %d, RoleBinding %d; want one each", roleUpdates, bindingUpdates)
	}
}

func TestClearSharedAgentBindingAccessRefusesForeignBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: types.UID("owner-uid")}}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: defaultNodeAgentRoleName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "victim", Namespace: "ns"}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, binding).Build()
	if err := clearSharedAgentBindingAccess(context.Background(), c, c, owner, "agent", defaultNodeAgentRoleName); err == nil || !strings.Contains(err.Error(), "not controlled") {
		t.Fatalf("clearSharedAgentBindingAccess() error = %v, want ownership refusal", err)
	}
	got := &rbacv1.RoleBinding{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(binding), got); err != nil {
		t.Fatal(err)
	}
	if len(got.Subjects) != 1 || got.Subjects[0].Name != "victim" {
		t.Fatalf("foreign RoleBinding subjects changed: %+v", got.Subjects)
	}
}

func TestDeleteOwnedNodeAgentDaemonSetPreservesForeignAndHandlesConcurrentDeletion(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: "owner-uid"}}
	controller := true

	t.Run("foreign DaemonSet is not deleted", func(t *testing.T) {
		ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns", UID: "foreign-uid"}}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, ds).Build()
		err := deleteOwnedNodeAgentDaemonSet(context.Background(), c, c, owner, ds.Name)
		if err == nil || !strings.Contains(err.Error(), "not controlled") {
			t.Fatalf("deleteOwnedNodeAgentDaemonSet() error = %v, want ownership refusal", err)
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(ds), &appsv1.DaemonSet{}); err != nil {
			t.Fatalf("foreign DaemonSet was deleted: %v", err)
		}
	})

	t.Run("concurrent disappearance is successful", func(t *testing.T) {
		ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
			Name: "agent", Namespace: "ns", UID: "agent-uid", ResourceVersion: "7",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: owner.Name, UID: owner.UID, Controller: &controller}},
		}}
		base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, ds).Build()
		c := interceptor.NewClient(base, interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				deleteOptions := &client.DeleteOptions{}
				for _, opt := range opts {
					opt.ApplyToDelete(deleteOptions)
				}
				if deleteOptions.Preconditions == nil || deleteOptions.Preconditions.UID == nil || *deleteOptions.Preconditions.UID != ds.UID || deleteOptions.Preconditions.ResourceVersion == nil || *deleteOptions.Preconditions.ResourceVersion != ds.ResourceVersion {
					t.Fatalf("delete preconditions = %+v, want observed UID and resourceVersion", deleteOptions.Preconditions)
				}
				return apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, obj.GetName())
			},
		})
		if err := deleteOwnedNodeAgentDaemonSet(context.Background(), c, c, owner, ds.Name); err != nil {
			t.Fatalf("deleteOwnedNodeAgentDaemonSet() error = %v, want concurrent NotFound ignored", err)
		}
	})
}

func TestEnsureAgentRBACDrainsLegacyBindingsWithoutClusterRoleRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ownerKind  string
		roleRules  []rbacv1.PolicyRule
		newOwner   func() client.Object
		agentName  func(client.Object) string
		ensureRBAC func(client.Client, *runtime.Scheme, client.Object) error
	}{
		{
			name:      "certificate agent with create-only ClusterRole",
			ownerKind: nodecert.KindNodeCertificateCheck,
			roleRules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create"}}},
			newOwner: func() client.Object {
				return &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: "nc-migrate", Namespace: "ns", UID: "nc-uid"}}
			},
			agentName: func(owner client.Object) string {
				return agentResourceName(owner.(*fathomv1alpha1.NodeCertificateCheck))
			},
			ensureRBAC: func(c client.Client, scheme *runtime.Scheme, owner client.Object) error {
				r := &NodeCertificateCheckReconciler{Client: c, Scheme: scheme}
				_, err := r.ensureAgentRBAC(context.Background(), owner.(*fathomv1alpha1.NodeCertificateCheck))
				return err
			},
		},
		{
			name:      "health agent with old broad ClusterRole",
			ownerKind: nodehealth.KindNodeHealthCheck,
			roleRules: []rbacv1.PolicyRule{{
				APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create", "get", "update"},
			}},
			newOwner: func() client.Object {
				return &fathomv1alpha1.NodeHealthCheck{ObjectMeta: metav1.ObjectMeta{Name: "nh-migrate", Namespace: "ns", UID: "nh-uid"}}
			},
			agentName: func(owner client.Object) string {
				return nodeHealthAgentResourceName(owner.(*fathomv1alpha1.NodeHealthCheck))
			},
			ensureRBAC: func(c client.Client, scheme *runtime.Scheme, owner client.Object) error {
				r := &NodeHealthCheckReconciler{Client: c, Scheme: scheme}
				_, err := r.ensureAgentRBAC(context.Background(), owner.(*fathomv1alpha1.NodeHealthCheck))
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scheme := newProvisioningScheme(t)
			owner := tc.newOwner()
			agentName := tc.agentName(owner)
			controller := true
			binding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: agentName, Namespace: owner.GetNamespace(),
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: tc.ownerKind,
						Name: owner.GetName(), UID: owner.GetUID(), Controller: &controller,
					}},
				},
				RoleRef:  rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: defaultNodeAgentRoleName},
				Subjects: []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: agentName, Namespace: owner.GetNamespace()}},
			}
			role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: defaultNodeAgentRoleName}, Rules: tc.roleRules}
			var clusterRoleRequests int
			base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, binding, role).Build()
			c := interceptor.NewClient(base, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*rbacv1.ClusterRole); ok {
						clusterRoleRequests++
					}
					return c.Get(ctx, key, obj, opts...)
				},
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*rbacv1.ClusterRole); ok {
						clusterRoleRequests++
					}
					return c.Create(ctx, obj, opts...)
				},
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*rbacv1.ClusterRole); ok {
						clusterRoleRequests++
					}
					return c.Update(ctx, obj, opts...)
				},
			})
			if err := tc.ensureRBAC(c, scheme, owner); err != nil {
				t.Fatal(err)
			}
			if clusterRoleRequests != 0 {
				t.Fatalf("ensureAgentRBAC made %d ClusterRole request(s)", clusterRoleRequests)
			}
			got := &rbacv1.RoleBinding{}
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(binding), got); err != nil {
				t.Fatal(err)
			}
			if len(got.Subjects) != 0 || got.RoleRef != binding.RoleRef {
				t.Fatalf("legacy RoleBinding was not drained in place: %+v", got)
			}
			if err := c.Get(context.Background(), client.ObjectKey{Name: agentName, Namespace: owner.GetNamespace()}, &corev1.ServiceAccount{}); err != nil {
				t.Fatalf("ServiceAccount was not ensured: %v", err)
			}
		})
	}
}
