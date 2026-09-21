/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	"github.com/skaphos/fathom/pkg/adapter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func authorityFixture(t *testing.T) (*runtime.Scheme, *api.AddonDefinition, *api.AddonDefinitionBinding, *corev1.ServiceAccount) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	d := &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom", UID: "definition-uid", Generation: 2}, Spec: api.AddonDefinitionSpec{AddonType: "custom", AdapterVersion: "1.0.0", SemanticsVersion: 1, Families: []api.DefinitionFamily{{Name: "health", Checks: []api.DefinitionCheck{{Name: "config", Kind: "ConfigMap", ConfigMap: &api.DefinitionConfigMap{Target: api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"target"}}, DefaultName: "config", Key: "policy"}}}}}}}
	b := &api.AddonDefinitionBinding{ObjectMeta: metav1.ObjectMeta{Name: "custom", Namespace: "operator", UID: "binding-uid", Generation: 3}, Spec: api.AddonDefinitionBindingSpec{DefinitionRef: api.DefinitionReference{Name: "custom", UID: "definition-uid"}, ServiceAccountRef: api.DefinitionObjectReference{Name: "reader", UID: "sa-uid"}, Enabled: true, TargetScope: api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"target"}}}}
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: "operator", UID: "sa-uid"}}
	return scheme, d, b, sa
}

func TestRuntimeAuthorityRejectsIdentityConfusion(t *testing.T) {
	for _, tc := range []struct {
		name          string
		edit          func(*api.AddonDefinition, *api.AddonDefinitionBinding, *corev1.ServiceAccount)
		shared, valid bool
	}{
		{name: "valid", valid: true},
		{name: "definition recreated", edit: func(d *api.AddonDefinition, _ *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			d.UID = "recreated"
		}},
		{name: "SA recreated", edit: func(_ *api.AddonDefinition, _ *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			sa.UID = "recreated"
		}},
		{name: "disabled", edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			b.Spec.Enabled = false
		}},
		{name: "manager SA", edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			b.Spec.ServiceAccountRef.Name = "manager"
			sa.Name = "manager"
		}},
		{name: "builtin SA", edit: func(_ *api.AddonDefinition, _ *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			sa.Labels = map[string]string{adapter.AddonLabel: "coredns"}
		}},
		{name: "shared SA", shared: true},
		{name: "retarget same UID", edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			b.Spec.ServiceAccountRef.Name = "other"
		}},
		{name: "effective policy scope is checked separately", valid: true, edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			b.Spec.TargetScope.Namespaces = []api.DefinitionDNSLabel{"other"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme, d, b, sa := authorityFixture(t)
			if tc.edit != nil {
				tc.edit(d, b, sa)
			}
			objects := []client.Object{d, b, sa}
			if tc.shared {
				other := b.DeepCopy()
				other.Name = "other"
				other.UID = "other-binding"
				other.Spec.DefinitionRef.Name = "other"
				other.Spec.Enabled = false
				objects = append(objects, other)
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			_, err := impersonation.ResolveRuntimeAuthority(context.Background(), reader, "operator", "manager", "custom")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestRuntimeDiscoveryUsesOnlyBoundIdentity(t *testing.T) {
	scheme, d, b, sa := authorityFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Impersonate-User") != "system:serviceaccount:operator:reader" {
			t.Errorf("incorrect identity: %q", r.Header.Get("Impersonate-User"))
		}
		for _, key := range []string{"Impersonate-Group", "Impersonate-Uid", "Impersonate-Extra-Privileged"} {
			if r.Header.Get(key) != "" {
				t.Errorf("inherited impersonation %s", key)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403,"message":"denied"}`))
	}))
	defer server.Close()
	base := &rest.Config{Host: server.URL, Impersonate: rest.ImpersonationConfig{UserName: "admin", UID: "admin-uid", Groups: []string{"system:masters"}, Extra: map[string][]string{"privileged": {"yes"}}}}
	base.WrapTransport = func(http.RoundTripper) http.RoundTripper { t.Fatal("inherited wrapper reused"); return nil }
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()
	factory, err := impersonation.NewRuntimeFactory(base, scheme, reader, "operator", "manager")
	if err != nil {
		t.Fatal(err)
	}
	c, authority, err := factory.ClientFor(context.Background(), "custom", func(input *impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
		input.ServiceAccount.Name = "changed"
		return func(next http.RoundTripper) http.RoundTripper { return injectedHeaders{next: next} }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if authority.ServiceAccount.Name != "reader" {
		t.Fatal("transport builder mutated captured identity")
	}
	err = c.Get(context.Background(), types.NamespacedName{Namespace: "target", Name: "config"}, &corev1.ConfigMap{})
	if err == nil || calls.Load() == 0 {
		t.Fatal("expected denied delegated discovery, without fallback")
	}
	if base.Impersonate.UserName != "admin" || len(base.Impersonate.Groups) != 1 {
		t.Fatal("base config mutated")
	}
	if _, _, err := factory.ClientFor(context.Background(), "custom", nil); err == nil {
		t.Fatal("missing budget/scope transport accepted")
	}
}

func TestRuntimeFactoryFailsClosedWithoutPrerequisites(t *testing.T) {
	scheme, d, b, sa := authorityFixture(t)
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()
	for _, tc := range []struct {
		name, namespace, manager string
		config                   *rest.Config
		reader                   client.Reader
	}{
		{name: "no config", namespace: "operator", manager: "manager", reader: reader},
		{name: "no namespace", manager: "manager", config: &rest.Config{}, reader: reader},
		{name: "no manager identity", namespace: "operator", config: &rest.Config{}, reader: reader},
		{name: "no uncached reader", namespace: "operator", manager: "manager", config: &rest.Config{}},
		{name: "opaque transport", namespace: "operator", manager: "manager", config: &rest.Config{Transport: http.DefaultTransport}, reader: reader},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := impersonation.NewRuntimeFactory(tc.config, scheme, tc.reader, tc.namespace, tc.manager); err == nil {
				t.Fatal("missing isolation prerequisite accepted")
			}
		})
	}
	var absent *impersonation.RuntimeFactory
	if _, _, err := absent.ClientFor(context.Background(), "custom", func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
		return func(next http.RoundTripper) http.RoundTripper { return next }, nil
	}); err == nil {
		t.Fatal("nil runtime factory fell back")
	}
	missing := fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, sa).Build()
	if _, err := impersonation.ResolveRuntimeAuthority(context.Background(), missing, "operator", "manager", "custom"); err == nil {
		t.Fatal("missing binding accepted")
	}
}

type injectedHeaders struct{ next http.RoundTripper }

func (t injectedHeaders) RoundTrip(request *http.Request) (*http.Response, error) {
	req := request.Clone(request.Context())
	req.Header.Set("Impersonate-User", "admin")
	req.Header.Set("Impersonate-Group", "system:masters")
	req.Header.Set("Impersonate-Uid", "admin-uid")
	req.Header.Set("Impersonate-Extra-Privileged", "yes")
	return t.next.RoundTrip(req)
}
