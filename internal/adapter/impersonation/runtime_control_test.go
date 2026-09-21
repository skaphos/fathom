/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRuntimeControlReaderSharesBudgetWithoutSharingIdentity(t *testing.T) {
	scheme, d, binding, sa := authorityFixture(t)
	var calls, delegatedCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body any
		delegated := false
		switch r.URL.Path {
		case "/apis/fathom.skaphos.io/v1alpha1/addondefinitions/custom":
			body = d
		case "/apis/fathom.skaphos.io/v1alpha1/namespaces/operator/addondefinitionbindings/custom":
			body = binding
		case "/api/v1/namespaces/operator/serviceaccounts/reader":
			body = sa
		case "/apis/fathom.skaphos.io/v1alpha1/namespaces/operator/addondefinitionbindings":
			if r.URL.Query().Get("limit") != "100" {
				t.Error("authority inventory is not bounded")
			}
			body = &api.AddonDefinitionBindingList{Items: []api.AddonDefinitionBinding{*binding}}
		case "/apis/fathom.skaphos.io/v1alpha1/namespaces/checks/addonchecks/check":
			body = &api.AddonCheck{ObjectMeta: metav1.ObjectMeta{Name: "check", Namespace: "checks", UID: "check-uid", Generation: 1}}
		case "/apis/coordination.k8s.io/v1/namespaces/operator/leases/leader":
			body = &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: "leader", Namespace: "operator", UID: "lease-uid"}}
		case "/api":
			delegated = true
			body = &metav1.APIVersions{TypeMeta: metav1.TypeMeta{Kind: "APIVersions", APIVersion: "v1"}, Versions: []string{"v1"}}
		case "/apis":
			delegated = true
			body = &metav1.APIGroupList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupList", APIVersion: "v1"}}
		case "/api/v1":
			delegated = true
			body = &metav1.APIResourceList{TypeMeta: metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"}, GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}}}}
		case "/api/v1/namespaces/target/configmaps/config":
			delegated = true
			body = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: "target"}, Data: map[string]string{"policy": "{}"}}
		default:
			t.Errorf("unexpected API request %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		want := ""
		if delegated {
			delegatedCalls.Add(1)
			want = "system:serviceaccount:operator:reader"
		}
		if r.Header.Get("Impersonate-User") != want {
			t.Errorf("wrong identity on %s", r.URL.Path)
		}
		for key := range r.Header {
			if strings.HasPrefix(strings.ToLower(key), "impersonate-") && key != "Impersonate-User" {
				t.Errorf("inherited identity header: %s", key)
			}
		}
		if r.Header.Get("Authorization") != "Bearer fixture-manager-token" {
			t.Error("manager authentication was not retained")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	base := &rest.Config{Host: server.URL, BearerToken: "fixture-manager-token", Impersonate: rest.ImpersonationConfig{UserName: "inherited-admin", Groups: []string{"system:masters"}}}
	base.WrapTransport = func(http.RoundTripper) http.RoundTripper { t.Error("inherited wrapper reused"); return nil }
	b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
	defer closeBudget()
	targets := execution.ControlTargets{OperatorNamespace: "operator", DefinitionName: "custom", Check: types.NamespacedName{Namespace: "checks", Name: "check"}, LeaseName: "leader"}
	reader, err := impersonation.NewRuntimeControlReader(base, b, targets)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reader.(client.Writer); ok {
		t.Fatal("control reader exposes mutations")
	}
	if err := reader.Get(b.Context(), types.NamespacedName{Namespace: "operator", Name: "config"}, &corev1.ConfigMap{}); err == nil || calls.Load() != 0 {
		t.Fatalf("manager target fallback: calls=%d err=%v", calls.Load(), err)
	}
	factory, err := impersonation.NewRuntimeFactory(base, scheme, reader, "operator", "manager")
	if err != nil {
		t.Fatal(err)
	}
	var guard *execution.Guard
	c, _, err := factory.ClientFor(b.Context(), "custom", func(authority *impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
		expected, err := execution.DiscoveryExpectations(authority.Definition)
		if err != nil {
			return nil, err
		}
		guard, err = execution.NewGuard(b, authority.Binding.Spec.TargetScope, expected)
		if err != nil {
			return nil, err
		}
		return guard.Wrap, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 {
		t.Fatalf("authority reads=%d want=4", calls.Load())
	}
	before, err := guard.Diagnostics(d)
	if err != nil || before.Reason != "NotEvaluated" {
		t.Fatalf("manager reads contaminated diagnostics: %+v %v", before, err)
	}
	if err := c.Get(b.Context(), types.NamespacedName{Namespace: "target", Name: "config"}, &corev1.ConfigMap{}); err != nil {
		t.Fatal(err)
	}
	if delegatedCalls.Load() < 2 {
		t.Fatal("delegated discovery and target were not read")
	}
	if err := reader.Get(b.Context(), targets.Check, &api.AddonCheck{}); err != nil {
		t.Fatal(err)
	}
	if err := reader.Get(b.Context(), types.NamespacedName{Namespace: "operator", Name: "leader"}, &coordinationv1.Lease{}); err != nil {
		t.Fatal(err)
	}
	count := calls.Load()
	for i := int(count); i < limits.MaxRunRequests; i++ {
		if err := b.ChargeRequest(); err != nil {
			t.Fatal(err)
		}
	}
	if err := reader.Get(b.Context(), targets.Check, &api.AddonCheck{}); err == nil || !strings.Contains(err.Error(), "WorkLimitExceeded") {
		t.Fatalf("final fence escaped shared budget: %v", err)
	}
	if calls.Load() != count {
		t.Fatal("over-budget fence reached API")
	}
	if base.Impersonate.UserName != "inherited-admin" {
		t.Fatal("base config mutated")
	}
}

func TestRuntimeControlReaderRequiresExplicitBoundary(t *testing.T) {
	for _, testCase := range []string{"nil config", "opaque transport", "nil budget", "missing namespace", "missing definition", "missing check", "missing lease"} {
		t.Run(testCase, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
			defer closeBudget()
			base := &rest.Config{Host: "https://unused.invalid"}
			targets := execution.ControlTargets{OperatorNamespace: "operator", DefinitionName: "custom", Check: types.NamespacedName{Namespace: "checks", Name: "check"}, LeaseName: "leader"}
			switch testCase {
			case "nil config":
				base = nil
			case "opaque transport":
				base.Transport = http.DefaultTransport
			case "nil budget":
				b = nil
			case "missing namespace":
				targets.OperatorNamespace = ""
			case "missing definition":
				targets.DefinitionName = ""
			case "missing check":
				targets.Check = types.NamespacedName{}
			case "missing lease":
				targets.LeaseName = ""
			}
			if _, err := impersonation.NewRuntimeControlReader(base, b, targets); err == nil {
				t.Fatal("unbounded reader accepted")
			}
		})
	}
}
