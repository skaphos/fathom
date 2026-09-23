/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
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
	deleting := func(o metav1.Object) {
		now := metav1.Now()
		o.SetDeletionTimestamp(&now)
		o.SetFinalizers([]string{"fathom.skaphos.io/test"})
	}
	for _, tc := range []struct {
		name          string
		edit          func(*api.AddonDefinition, *api.AddonDefinitionBinding, *corev1.ServiceAccount)
		extra         []client.Object
		shared, valid bool
		// wantReason is the contract failure reason from the lifecycle matrix; a
		// rejection that cannot be attributed to a reason is not evidence.
		wantReason string
		verify     func(*testing.T, *impersonation.RuntimeAuthority)
	}{
		{name: "valid", valid: true},
		{name: "definition recreated", wantReason: "BindingMismatch", edit: func(d *api.AddonDefinition, _ *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			d.UID = "recreated"
		}},
		{name: "definition deleted", wantReason: "BindingMismatch", edit: func(d *api.AddonDefinition, _ *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			deleting(d)
		}},
		{name: "binding deleted", wantReason: "BindingMismatch", edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			deleting(b)
		}},
		{name: "SA recreated", wantReason: "BindingMismatch", edit: func(_ *api.AddonDefinition, _ *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			sa.UID = "recreated"
		}},
		{name: "SA deleting", wantReason: "BindingMismatch", edit: func(_ *api.AddonDefinition, _ *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			deleting(sa)
		}},
		{name: "disabled", wantReason: "AuthorizationRevoked", edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			b.Spec.Enabled = false
		}},
		{name: "manager SA", wantReason: "BindingMismatch", edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			b.Spec.ServiceAccountRef.Name = "manager"
			sa.Name = "manager"
		}},
		{name: "builtin SA", wantReason: "BindingMismatch", edit: func(_ *api.AddonDefinition, _ *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			sa.Labels = map[string]string{adapter.AddonLabel: "coredns"}
		}},
		// A reserved account whose label was stripped is still reserved: the
		// shipped name alone disqualifies it, with and without the deploy prefix.
		{name: "shipped SA name without label", wantReason: "BindingMismatch", edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			b.Spec.ServiceAccountRef.Name = api.DefinitionResourceName(adapter.AddonServiceAccountName("coredns"))
			sa.Name = adapter.AddonServiceAccountName("coredns")
		}},
		{name: "prefixed shipped SA name without label", wantReason: "BindingMismatch", edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
			b.Spec.ServiceAccountRef.Name = api.DefinitionResourceName("fathom-" + adapter.AddonServiceAccountName("coredns"))
			sa.Name = "fathom-" + adapter.AddonServiceAccountName("coredns")
		}},
		// A definition claiming a shipped addon name is a conflict barrier, not a
		// binding problem: it is refused before any binding is even read.
		{name: "definition name collides with a built-in", wantReason: "BuiltinCollision", edit: func(d *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			builtin := definitions.BuiltinNames()[0]
			d.Name, d.Spec.AddonType = builtin, api.DefinitionDNSLabel(builtin)
			b.Name, b.Spec.DefinitionRef.Name = builtin, api.DefinitionDNSLabel(builtin)
		}},
		{name: "shared SA", shared: true, wantReason: "BindingMismatch"},
		// The retargeted account exists, so the rejection provably comes from the
		// recorded-UID fence and not from a NotFound on the referenced name.
		{name: "retarget same UID", wantReason: "BindingMismatch", extra: []client.Object{
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "operator", UID: "other-sa-uid"}},
		}, edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			b.Spec.ServiceAccountRef.Name = "other"
		}},
		// Delegated identity is the binding's own name/UID pair, never "whichever
		// object happens to carry that UID": cluster UID uniqueness is not the fence.
		{name: "retarget to the recorded UID keeps canonical identity", valid: true, extra: []client.Object{
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "operator", UID: "sa-uid"}},
		}, edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			b.Spec.ServiceAccountRef.Name = "other"
		}, verify: func(t *testing.T, authority *impersonation.RuntimeAuthority) {
			if authority.ServiceAccount.Name != "other" {
				t.Fatalf("identity %q is not the bound account", authority.ServiceAccount.Name)
			}
		}},
		// Authority is delegated per definition UID, not per revision: a new
		// revision targeting a different kind keeps the same bound identity.
		{name: "new revision of the same UID", valid: true, edit: func(d *api.AddonDefinition, _ *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			d.Generation = 7
			d.Spec.Families = []api.DefinitionFamily{{Name: "health", Checks: []api.DefinitionCheck{{Name: "controller", Kind: "Workload", Workload: &api.DefinitionWorkload{Target: api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"target"}}, Kind: "Deployment", DefaultName: "controller"}}}}}
		}, verify: func(t *testing.T, authority *impersonation.RuntimeAuthority) {
			if authority.ServiceAccount.Name != "reader" || authority.ServiceAccount.UID != "sa-uid" {
				t.Fatalf("revision change altered the bound identity: %+v", authority.ServiceAccount.ObjectMeta)
			}
			if authority.Definition.Generation != 7 {
				t.Fatalf("resolved revision %d", authority.Definition.Generation)
			}
		}},
		{name: "authorized scope beyond declared targets is resolved later", valid: true, edit: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
			// Authority resolution never narrows or judges scope: the extra
			// authorized namespace only matters once effective policy overrides
			// are resolved, and the declared-target intersection happens at the
			// client seam (see TestRuntimeClientRefusesOutOfScopeDefinition).
			b.Spec.TargetScope.Namespaces = []api.DefinitionDNSLabel{"target", "other"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme, d, b, sa := authorityFixture(t)
			if tc.edit != nil {
				tc.edit(d, b, sa)
			}
			objects := append([]client.Object{d, b, sa}, tc.extra...)
			if tc.shared {
				other := b.DeepCopy()
				other.Name = "other"
				other.UID = "other-binding"
				other.Spec.DefinitionRef.Name = "other"
				other.Spec.Enabled = false
				objects = append(objects, other)
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			authority, err := impersonation.ResolveRuntimeAuthority(context.Background(), impersonation.RuntimeControlReader{Reader: reader}, "operator", "manager", d.Name)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if !tc.valid && !strings.HasPrefix(err.Error(), tc.wantReason+":") {
				t.Fatalf("reason=%q want prefix %q", err.Error(), tc.wantReason)
			}
			if tc.verify != nil {
				tc.verify(t, authority)
			}
		})
	}
}

// helperBUrequest is one delegated request as the API server saw it.
type helperBUrequest struct{ path, user string }

// helperBUdelegatedServer serves the minimal discovery surface plus one ConfigMap
// per authorized namespace and records the impersonated identity of every request.
// When deny is set, resource reads answer 403 while discovery keeps succeeding, so
// a denial can be attributed to the read rather than to unusable discovery.
func helperBUdelegatedServer(t *testing.T, deny *atomic.Bool) (*httptest.Server, func() []helperBUrequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []helperBUrequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, helperBUrequest{path: r.URL.Path, user: r.Header.Get("Impersonate-User")})
		mu.Unlock()
		var body any
		switch {
		case r.URL.Path == "/api":
			body = &metav1.APIVersions{TypeMeta: metav1.TypeMeta{Kind: "APIVersions", APIVersion: "v1"}, Versions: []string{"v1"}}
		case r.URL.Path == "/apis":
			body = &metav1.APIGroupList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupList", APIVersion: "v1"}}
		case r.URL.Path == "/api/v1":
			body = &metav1.APIResourceList{TypeMeta: metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"}, GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}}}}
		case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") && strings.Contains(r.URL.Path, "/configmaps/"):
			if deny != nil && deny.Load() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403,"message":"denied"}`))
				return
			}
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/"), "/")
			body = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: parts[0], Name: parts[2]}, Data: map[string]string{"policy": "{}"}}
		default:
			t.Errorf("unexpected delegated request %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	}))
	return server, func() []helperBUrequest {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
}

// Permission diagnostics are derived from the requests this run actually made.
// No metrics provider, registry or collector is registered anywhere in this test
// (FR-005): diagnostics and request accounting must hold with metrics off.
func TestRuntimeDiagnosticsSurviveWithMetricsOff(t *testing.T) {
	scheme, d, b, sa := authorityFixture(t)
	var deny atomic.Bool
	server, recorded := helperBUdelegatedServer(t, &deny)
	defer server.Close()
	reader := impersonation.RuntimeControlReader{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()}
	factory, err := impersonation.NewRuntimeFactory(&rest.Config{Host: server.URL}, scheme, reader, "operator", "manager")
	if err != nil {
		t.Fatal(err)
	}
	delegated := func(budget *execution.Budget) (client.Client, *execution.Guard) {
		t.Helper()
		var guard *execution.Guard
		c, _, err := factory.ClientFor(budget.Context(), "custom", func(authority *impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
			expected, err := execution.DiscoveryExpectations(authority.Definition)
			if err != nil {
				return nil, err
			}
			guard, err = execution.NewGuard(budget, authority.Binding.Spec.TargetScope, expected)
			if err != nil {
				return nil, err
			}
			return guard.Wrap, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return c, guard
	}
	budget, closeBudget := execution.NewBudget(context.Background(), definitions.MaxRunDuration)
	defer closeBudget()
	c, guard := delegated(budget)
	if err := c.Get(budget.Context(), types.NamespacedName{Namespace: "target", Name: "config"}, &corev1.ConfigMap{}); err != nil {
		t.Fatal(err)
	}
	made := len(recorded())
	if made == 0 {
		t.Fatal("no delegated request reached the API")
	}
	// Every request the server saw was charged to the run, so exactly the
	// remainder of the request budget is left.
	for i := made; i < definitions.MaxRunRequests; i++ {
		if err := budget.ChargeRequest(); err != nil {
			t.Fatalf("delegated requests were not charged: %v after %d of %d", err, i, made)
		}
	}
	if err := budget.ChargeRequest(); err == nil || !strings.Contains(err.Error(), "WorkLimitExceeded") {
		t.Fatalf("request budget did not account for %d delegated requests: %v", made, err)
	}
	if diagnostic, err := guard.Diagnostics(d); err != nil || diagnostic.Reason != "RequestsSucceeded" {
		t.Fatalf("successful reads produced %+v %v", diagnostic, err)
	}
	deny.Store(true)
	denied, closeDenied := execution.NewBudget(context.Background(), definitions.MaxRunDuration)
	defer closeDenied()
	forbidden, deniedGuard := delegated(denied)
	before := len(recorded())
	err = forbidden.Get(denied.Context(), types.NamespacedName{Namespace: "target", Name: "config"}, &corev1.ConfigMap{})
	if err == nil || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("forbidden delegated read: %v", err)
	}
	if len(recorded()) <= before {
		t.Fatal("denied run made no request of its own")
	}
	diagnostic, err := deniedGuard.Diagnostics(d)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.Reason != "AccessDenied" || diagnostic.Status != metav1.ConditionFalse {
		t.Fatalf("denial evidence lost with metrics off: %+v", diagnostic)
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
	reader := impersonation.RuntimeControlReader{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()}
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
	reader := impersonation.RuntimeControlReader{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()}
	for _, tc := range []struct {
		name, namespace, manager string
		config                   *rest.Config
		reader                   client.Reader
	}{
		{name: "no config", namespace: "operator", manager: "manager", reader: reader},
		{name: "no namespace", manager: "manager", config: &rest.Config{}, reader: reader},
		{name: "no manager identity", namespace: "operator", config: &rest.Config{}, reader: reader},
		{name: "no uncached reader", namespace: "operator", manager: "manager", config: &rest.Config{}},
		// A manager cached client satisfies client.Reader, so only the purpose-built
		// control reader may be accepted here.
		{name: "cached manager reader", namespace: "operator", manager: "manager", config: &rest.Config{}, reader: fake.NewClientBuilder().WithScheme(scheme).Build()},
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
	// ResolveRuntimeAuthority is exported and callable without the factory, so its
	// own argument fence and every unavailable or invalid stored object must fail
	// closed with the reason the lifecycle matrix names.
	all := func(d *api.AddonDefinition, b *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) []client.Object {
		return []client.Object{d, b, sa}
	}
	for _, tc := range []struct {
		name, namespace, manager, resolve, wantReason string
		nilReader                                     bool
		objects                                       func(*api.AddonDefinition, *api.AddonDefinitionBinding, *corev1.ServiceAccount) []client.Object
	}{
		{name: "nil reader", nilReader: true, namespace: "operator", manager: "manager", resolve: "custom", wantReason: "AuthorizationUnavailable"},
		{name: "authority without namespace", manager: "manager", resolve: "custom", wantReason: "AuthorizationUnavailable"},
		{name: "namespace is not a label", namespace: "Not_A_Label", manager: "manager", resolve: "custom", wantReason: "AuthorizationUnavailable"},
		{name: "authority without manager identity", namespace: "operator", resolve: "custom", wantReason: "AuthorizationUnavailable"},
		{name: "manager identity is not a subdomain", namespace: "operator", manager: "Manager Identity", resolve: "custom", wantReason: "AuthorizationUnavailable"},
		{name: "no definition name", namespace: "operator", manager: "manager", wantReason: "AuthorizationUnavailable"},
		{name: "definition name is not a label", namespace: "operator", manager: "manager", resolve: "Not_A_Label", wantReason: "AuthorizationUnavailable"},
		{name: "definition missing", namespace: "operator", manager: "manager", resolve: "custom", wantReason: "DefinitionUnavailable",
			objects: func(_ *api.AddonDefinition, b *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) []client.Object {
				return []client.Object{b, sa}
			}},
		{name: "stored definition is invalid", namespace: "operator", manager: "manager", resolve: "custom", wantReason: "InvalidDefinition",
			objects: func(d *api.AddonDefinition, b *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) []client.Object {
				d.Spec.AdapterVersion = "not-a-semantic-version"
				return all(d, b, sa)
			}},
		{name: "binding missing", namespace: "operator", manager: "manager", resolve: "custom", wantReason: "BindingUnavailable",
			objects: func(d *api.AddonDefinition, _ *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) []client.Object {
				return []client.Object{d, sa}
			}},
		{name: "stored binding is invalid", namespace: "operator", manager: "manager", resolve: "custom", wantReason: "InvalidBinding",
			objects: func(d *api.AddonDefinition, b *api.AddonDefinitionBinding, sa *corev1.ServiceAccount) []client.Object {
				b.Spec.TargetScope = api.DefinitionBindingScope{}
				return all(d, b, sa)
			}},
		{name: "service account missing", namespace: "operator", manager: "manager", resolve: "custom", wantReason: "BindingMismatch",
			objects: func(d *api.AddonDefinition, b *api.AddonDefinitionBinding, _ *corev1.ServiceAccount) []client.Object {
				return []client.Object{d, b}
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme, d, b, sa := authorityFixture(t)
			build := tc.objects
			if build == nil {
				build = all
			}
			var reader client.Reader
			if !tc.nilReader {
				reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(build(d, b, sa)...).Build()
			}
			_, err := impersonation.ResolveRuntimeAuthority(context.Background(), impersonation.RuntimeControlReader{Reader: reader}, tc.namespace, tc.manager, tc.resolve)
			if err == nil {
				t.Fatal("authority resolved without its prerequisites")
			}
			if !strings.HasPrefix(err.Error(), tc.wantReason+":") {
				t.Fatalf("reason=%q want prefix %q", err.Error(), tc.wantReason)
			}
		})
	}
}

// The declared targets are fenced before a delegated client exists, so a caller
// that ignores the binding's target scope cannot obtain one. The transport Guard
// still fences effective policy scope against the requests actually issued.
func TestRuntimeClientRefusesOutOfScopeDefinition(t *testing.T) {
	scheme, d, b, sa := authorityFixture(t)
	b.Spec.TargetScope.Namespaces = []api.DefinitionDNSLabel{"other"}
	reader := impersonation.RuntimeControlReader{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()}
	factory, err := impersonation.NewRuntimeFactory(&rest.Config{Host: "https://unused.invalid"}, scheme, reader, "operator", "manager")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := factory.ClientFor(context.Background(), "custom", func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
		return func(next http.RoundTripper) http.RoundTripper { return next }, nil
	})
	if err == nil || !strings.HasPrefix(err.Error(), "ScopeDenied") {
		t.Fatalf("unauthorized declared target accepted: %v", err)
	}
	if c != nil {
		t.Fatal("client handed out for an unauthorized declared target")
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

// helperBPagedReader answers binding inventory lists from a caller-supplied page
// function. The controller-runtime fake client ignores client.Limit/client.Continue,
// so it can never exercise the multi-page path the run budget has to survive.
type helperBPagedReader struct {
	client.Reader
	t     *testing.T
	page  func(index int, continuation string) ([]api.AddonDefinitionBinding, string, error)
	calls int
}

func (r *helperBPagedReader) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	bindings, ok := list.(*api.AddonDefinitionBindingList)
	if !ok {
		r.t.Errorf("unexpected inventory list %T", list)
		return fmt.Errorf("unexpected list")
	}
	options := &client.ListOptions{}
	for _, option := range opts {
		option.ApplyToList(options)
	}
	if options.Limit != int64(definitions.MaxPageObjects) {
		r.t.Errorf("inventory page limit %d is not bounded", options.Limit)
	}
	if options.Namespace != "operator" {
		r.t.Errorf("inventory escaped the operator namespace: %q", options.Namespace)
	}
	items, continuation, err := r.page(r.calls, options.Continue)
	r.calls++
	if err != nil {
		return err
	}
	bindings.Items = items
	bindings.Continue = continuation
	return nil
}

// helperBUnrelatedBindings builds distinct bindings that share no identity with
// the resolved one, so only the caps can end the inventory walk.
func helperBUnrelatedBindings(page, count int) []api.AddonDefinitionBinding {
	items := make([]api.AddonDefinitionBinding, 0, count)
	for i := range count {
		name := fmt.Sprintf("other-%d-%d", page, i)
		items = append(items, api.AddonDefinitionBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "operator", UID: types.UID(name + "-uid")},
			Spec: api.AddonDefinitionBindingSpec{
				DefinitionRef:     api.DefinitionReference{Name: api.DefinitionDNSLabel(name), UID: name + "-definition"},
				ServiceAccountRef: api.DefinitionObjectReference{Name: api.DefinitionResourceName(name), UID: name + "-sa"},
			},
		})
	}
	return items
}

func TestRuntimeAuthorityInventoryStaysInsideRunBudget(t *testing.T) {
	for _, tc := range []struct {
		name, wantReason, wantDetail string
		wantCalls                    int
		page                         func(int, string) ([]api.AddonDefinitionBinding, string, error)
	}{
		{
			name: "repeated continuation token", wantReason: "WorkLimitExceeded", wantDetail: "incomplete binding identity inventory", wantCalls: 2,
			page: func(index int, _ string) ([]api.AddonDefinitionBinding, string, error) {
				return helperBUnrelatedBindings(index, 1), "stuck", nil
			},
		},
		{
			// A page that overshoots the total object cap ends the run before its
			// contents are trusted, rather than being silently truncated.
			name: "pages exceed the run object cap", wantReason: "WorkLimitExceeded", wantDetail: "binding identity inventory", wantCalls: 2,
			page: func(index int, _ string) ([]api.AddonDefinitionBinding, string, error) {
				return helperBUnrelatedBindings(index, definitions.MaxRunObjects/2+100), fmt.Sprintf("page-%d", index+1), nil
			},
		},
		{
			name: "continuation outlives the request cap", wantReason: "WorkLimitExceeded", wantDetail: "binding inventory request cap", wantCalls: definitions.MaxRunRequests - 3,
			page: func(index int, _ string) ([]api.AddonDefinitionBinding, string, error) {
				return helperBUnrelatedBindings(index, 1), fmt.Sprintf("page-%d", index+1), nil
			},
		},
		{
			name: "inventory read fails", wantReason: "AuthorizationUnavailable", wantDetail: "cannot establish dedicated identity", wantCalls: 1,
			page: func(int, string) ([]api.AddonDefinitionBinding, string, error) {
				return nil, "", fmt.Errorf("inventory unavailable")
			},
		},
		{
			// The shared-identity fence must survive pagination: a competing binding
			// on a later page is still a rejection.
			name: "shared SA appears on the second page", wantReason: "BindingMismatch", wantDetail: "service account is shared by binding competitor", wantCalls: 2,
			page: func(index int, _ string) ([]api.AddonDefinitionBinding, string, error) {
				if index == 0 {
					return helperBUnrelatedBindings(index, 2), "page-1", nil
				}
				competitor := api.AddonDefinitionBinding{
					ObjectMeta: metav1.ObjectMeta{Name: "competitor", Namespace: "operator", UID: "competitor-uid"},
					Spec: api.AddonDefinitionBindingSpec{
						DefinitionRef:     api.DefinitionReference{Name: "competitor", UID: "competitor-definition"},
						ServiceAccountRef: api.DefinitionObjectReference{Name: "reader", UID: "sa-uid"},
					},
				}
				return []api.AddonDefinitionBinding{competitor}, "", nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme, d, b, sa := authorityFixture(t)
			objects := fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()
			reader := &helperBPagedReader{Reader: objects, t: t, page: tc.page}
			_, err := impersonation.ResolveRuntimeAuthority(context.Background(), impersonation.RuntimeControlReader{Reader: reader}, "operator", "manager", "custom")
			if err == nil {
				t.Fatal("unbounded or shared binding inventory accepted")
			}
			if !strings.HasPrefix(err.Error(), tc.wantReason+":") || !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("reason=%q want %q/%q", err.Error(), tc.wantReason, tc.wantDetail)
			}
			if reader.calls != tc.wantCalls {
				t.Fatalf("inventory made %d list requests, want %d", reader.calls, tc.wantCalls)
			}
		})
	}
}

// Every construction step is a fence: a failure at any of them yields no client
// rather than one that silently falls back to the manager identity.
func TestRuntimeClientConstructionFailsClosed(t *testing.T) {
	passthrough := func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
		return func(next http.RoundTripper) http.RoundTripper { return next }, nil
	}
	for _, tc := range []struct {
		// Every row states the contract reason AND the detail that identifies the
		// fence that refused: a rejection nobody can attribute is not evidence, and
		// a reason with no detail cannot tell one construction step from another.
		// wantCause names a fragment of the wrapped library error, so a reason that
		// replaced the cause instead of wrapping it is caught too.
		name, wantReason, wantDetail, wantCause string
		withoutBinding                          bool
		config                                  func() *rest.Config
		build                                   func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error)
	}{
		{name: "authority failure propagates", withoutBinding: true, wantReason: "BindingUnavailable", wantDetail: "not found", build: passthrough},
		{name: "transport guard cannot be built", wantReason: "ScopeDenied", wantDetail: "guard refused this run", build: func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
			return nil, fmt.Errorf("ScopeDenied: guard refused this run")
		}},
		{name: "transport guard is missing", wantReason: "AuthorizationUnavailable", wantDetail: "transport guard missing", build: func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
			return nil, nil
		}},
		// Discovery and credential failures are lifecycle failures like every other
		// step here: the run never obtains a delegated identity, so they carry the
		// same AuthorizationUnavailable reason rather than an unattributed error.
		{name: "discovery mapper cannot be built", wantReason: "AuthorizationUnavailable", wantDetail: "runtime discovery mapper cannot be built", wantCause: "://bad", build: passthrough, config: func() *rest.Config {
			return &rest.Config{Host: "://bad"}
		}},
		{name: "credentials cannot build a transport", wantReason: "AuthorizationUnavailable", wantDetail: "runtime credentials cannot build a transport", wantCause: "absent-ca.pem", build: passthrough, config: func() *rest.Config {
			return &rest.Config{Host: "https://unused.invalid", TLSClientConfig: rest.TLSClientConfig{CAFile: filepath.Join(t.TempDir(), "absent-ca.pem")}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme, d, b, sa := authorityFixture(t)
			objects := []client.Object{d, b, sa}
			if tc.withoutBinding {
				objects = []client.Object{d, sa}
			}
			config := &rest.Config{Host: "https://unused.invalid"}
			if tc.config != nil {
				config = tc.config()
			}
			reader := impersonation.RuntimeControlReader{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()}
			factory, err := impersonation.NewRuntimeFactory(config, scheme, reader, "operator", "manager")
			if err != nil {
				t.Fatal(err)
			}
			c, authority, err := factory.ClientFor(context.Background(), "custom", tc.build)
			if err == nil {
				t.Fatal("client constructed despite a failed prerequisite")
			}
			if !strings.HasPrefix(err.Error(), tc.wantReason+": ") || !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("reason=%q want prefix %q and detail %q", err.Error(), tc.wantReason, tc.wantDetail)
			}
			if tc.wantCause != "" && !strings.Contains(err.Error(), tc.wantCause) {
				t.Fatalf("reason=%q dropped the underlying cause %q", err.Error(), tc.wantCause)
			}
			if c != nil || authority != nil {
				t.Fatal("failed construction still returned a client or authority")
			}
		})
	}
}

// A redirected runtime request could leave the API server, the bound identity and
// the scope fence behind, so the dedicated client refuses to follow one.
func TestRuntimeClientRefusesRedirects(t *testing.T) {
	scheme, d, b, sa := authorityFixture(t)
	var followed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("redirected") != "" {
			followed.Add(1)
		}
		http.Redirect(w, r, r.URL.Path+"?redirected=1", http.StatusFound)
	}))
	defer server.Close()
	reader := impersonation.RuntimeControlReader{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()}
	factory, err := impersonation.NewRuntimeFactory(&rest.Config{Host: server.URL}, scheme, reader, "operator", "manager")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := factory.ClientFor(context.Background(), "custom", func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
		return func(next http.RoundTripper) http.RoundTripper { return next }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = c.Get(context.Background(), types.NamespacedName{Namespace: "target", Name: "config"}, &corev1.ConfigMap{})
	if err == nil || !strings.Contains(err.Error(), "redirects are forbidden") {
		t.Fatalf("redirect was not refused: %v", err)
	}
	if followed.Load() != 0 {
		t.Fatal("the delegated client followed a redirect")
	}
}

// Discovery is per identity by construction (a fresh mapper built from that
// identity's own client), which only means something if a second identity really
// re-issues discovery under its own impersonation rather than reusing the first.
func TestRuntimeDiscoveryIsKeyedPerIdentity(t *testing.T) {
	scheme, d, b, sa := authorityFixture(t)
	second := d.DeepCopy()
	second.Name, second.UID = "second", "second-definition-uid"
	second.Spec.AddonType = "second"
	secondBinding := b.DeepCopy()
	secondBinding.Name, secondBinding.UID = "second", "second-binding-uid"
	secondBinding.Spec.DefinitionRef = api.DefinitionReference{Name: "second", UID: "second-definition-uid"}
	secondBinding.Spec.ServiceAccountRef = api.DefinitionObjectReference{Name: "reader-two", UID: "sa-two-uid"}
	secondSA := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "reader-two", Namespace: "operator", UID: "sa-two-uid"}}
	server, recorded := helperBUdelegatedServer(t, nil)
	defer server.Close()
	reader := impersonation.RuntimeControlReader{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa, second, secondBinding, secondSA).Build()}
	factory, err := impersonation.NewRuntimeFactory(&rest.Config{Host: server.URL}, scheme, reader, "operator", "manager")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"custom", "second"} {
		c, _, err := factory.ClientFor(context.Background(), name, func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
			return func(next http.RoundTripper) http.RoundTripper { return next }, nil
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: "target", Name: "config"}, &corev1.ConfigMap{}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	identities := map[string]map[string]bool{}
	for _, request := range recorded() {
		if request.user == "" {
			t.Fatalf("request to %s fell back to the manager identity", request.path)
		}
		if identities[request.path] == nil {
			identities[request.path] = map[string]bool{}
		}
		identities[request.path][request.user] = true
	}
	for _, path := range []string{"/api", "/api/v1"} {
		for _, user := range []string{"system:serviceaccount:operator:reader", "system:serviceaccount:operator:reader-two"} {
			if !identities[path][user] {
				t.Fatalf("discovery of %s was not re-issued as %s: %v", path, user, identities[path])
			}
		}
	}
}

// A definition whose declared namespaces cannot fit in any binding's target scope
// can never be authorized, so it is refused as ScopeDenied rather than reported as
// an ordinary validation problem after a client already exists.
func TestRuntimeClientRefusesUnsatisfiableTargetScope(t *testing.T) {
	scheme, d, b, sa := authorityFixture(t)
	// Every check stays inside its own limits; only their union exceeds the
	// largest scope any binding is allowed to authorize.
	authorized := make([]api.DefinitionDNSLabel, 0, definitions.MaxNamespaces)
	families := []api.DefinitionFamily{{Name: "health"}, {Name: "extra"}}
	for i := range definitions.MaxNamespaces + 2 {
		namespace := api.DefinitionDNSLabel(fmt.Sprintf("ns-%02d", i))
		if i < definitions.MaxNamespaces {
			authorized = append(authorized, namespace)
		}
		check := api.DefinitionCheck{Name: api.DefinitionIdentifier(fmt.Sprintf("config-%02d", i)), Kind: "ConfigMap", ConfigMap: &api.DefinitionConfigMap{
			Target:      api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{namespace}},
			DefaultName: "config", Key: "policy",
		}}
		family := &families[i%len(families)]
		family.Checks = append(family.Checks, check)
	}
	d.Spec.Families = families
	b.Spec.TargetScope.Namespaces = authorized
	reader := impersonation.RuntimeControlReader{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(d, b, sa).Build()}
	factory, err := impersonation.NewRuntimeFactory(&rest.Config{Host: "https://unused.invalid"}, scheme, reader, "operator", "manager")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := factory.ClientFor(context.Background(), "custom", func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error) {
		t.Error("transport guard was built for an unsatisfiable scope")
		return func(next http.RoundTripper) http.RoundTripper { return next }, nil
	})
	// The reason is added by the caller here (the ceiling itself reports no
	// reason), so the wrap must keep the underlying limit visible rather than
	// replace it with a bare label.
	if err == nil || !strings.HasPrefix(err.Error(), "ScopeDenied: ") || !strings.Contains(err.Error(), fmt.Sprintf("combined target namespaces exceed %d", definitions.MaxNamespaces)) {
		t.Fatalf("unsatisfiable declared scope: %v", err)
	}
	if c != nil {
		t.Fatal("client handed out for an unsatisfiable declared scope")
	}
}
