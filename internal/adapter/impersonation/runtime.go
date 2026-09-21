/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/pkg/adapter"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// RuntimeAuthority captures live control-plane objects for subsequent revision
// fences. Each resolution owns fresh objects; status never confers authority.
type RuntimeAuthority struct {
	Definition     *api.AddonDefinition
	Binding        *api.AddonDefinitionBinding
	ServiceAccount *corev1.ServiceAccount
}

// ResolveRuntimeAuthority requires an uncached control-plane reader. The caller
// wraps that reader with the run's control-plane request budget. Effective policy
// scope is checked after policy resolution and independently by the transport.
func ResolveRuntimeAuthority(ctx context.Context, reader client.Reader, namespace, managerSA, name string) (*RuntimeAuthority, error) {
	if reader == nil || namespace == "" || managerSA == "" || len(validation.IsDNS1123Label(namespace)) != 0 || len(validation.IsDNS1123Subdomain(managerSA)) != 0 || len(validation.IsDNS1123Label(name)) != 0 {
		return nil, fmt.Errorf("AuthorizationUnavailable: explicit control reader, namespace, manager identity and definition name required")
	}
	ctx, cancel := context.WithTimeout(ctx, definitions.MaxRunDuration)
	defer cancel()
	get := func(key types.NamespacedName, obj client.Object) error {
		req, cancel := context.WithTimeout(ctx, definitions.MaxRequestDuration)
		defer cancel()
		return reader.Get(req, key, obj)
	}
	authority := &RuntimeAuthority{Definition: &api.AddonDefinition{}, Binding: &api.AddonDefinitionBinding{}, ServiceAccount: &corev1.ServiceAccount{}}
	if err := get(types.NamespacedName{Name: name}, authority.Definition); err != nil {
		return nil, fmt.Errorf("DefinitionUnavailable: %w", err)
	}
	d := authority.Definition
	if err := definitions.Validate(d); err != nil {
		return nil, err
	}
	for _, builtin := range definitions.BuiltinNames() {
		if name == builtin {
			return nil, fmt.Errorf("BuiltinCollision: %s", name)
		}
	}
	if err := get(types.NamespacedName{Namespace: namespace, Name: name}, authority.Binding); err != nil {
		return nil, fmt.Errorf("BindingUnavailable: %w", err)
	}
	b := authority.Binding
	if err := definitions.ValidateBinding(b); err != nil {
		return nil, err
	}
	if d.UID == "" || b.UID == "" || !d.DeletionTimestamp.IsZero() || !b.DeletionTimestamp.IsZero() || b.Spec.DefinitionRef.UID != string(d.UID) || b.Spec.DefinitionRef.Name != api.DefinitionDNSLabel(d.Name) {
		return nil, fmt.Errorf("BindingMismatch: definition identity changed")
	}
	if !b.Spec.Enabled {
		return nil, fmt.Errorf("AuthorizationRevoked: binding is disabled")
	}
	saName := string(b.Spec.ServiceAccountRef.Name)
	if saName == managerSA {
		return nil, fmt.Errorf("BindingMismatch: manager service account is not dedicated")
	}
	if err := get(types.NamespacedName{Namespace: namespace, Name: saName}, authority.ServiceAccount); err != nil {
		return nil, fmt.Errorf("BindingMismatch: %w", err)
	}
	sa := authority.ServiceAccount
	if sa.UID == "" || string(sa.UID) != b.Spec.ServiceAccountRef.UID || !sa.DeletionTimestamp.IsZero() || sa.Labels[adapter.AddonLabel] != "" {
		return nil, fmt.Errorf("BindingMismatch: service account is replaced, deleting, or reserved for built-ins")
	}
	// Also reject shipped names if a reserved account's label was removed.
	for _, builtin := range definitions.BuiltinNames() {
		base := adapter.AddonServiceAccountName(builtin)
		if saName == base || saName == "fathom-"+base {
			return nil, fmt.Errorf("BindingMismatch: built-in service account is not dedicated")
		}
	}
	continuation := ""
	seen := map[string]bool{}
	count := 0
	pages := 0
	for {
		pages++
		if pages > definitions.MaxRunRequests-3 {
			return nil, fmt.Errorf("WorkLimitExceeded: binding inventory request cap")
		}
		var bindings api.AddonDefinitionBindingList
		req, cancel := context.WithTimeout(ctx, definitions.MaxRequestDuration)
		err := reader.List(req, &bindings, client.InNamespace(namespace), client.Limit(definitions.MaxPageObjects), client.Continue(continuation))
		cancel()
		if err != nil {
			return nil, fmt.Errorf("AuthorizationUnavailable: cannot establish dedicated identity: %w", err)
		}
		count += len(bindings.Items)
		if count > definitions.MaxRunObjects {
			return nil, fmt.Errorf("WorkLimitExceeded: binding identity inventory")
		}
		for _, other := range bindings.Items {
			if other.UID == b.UID && other.Name == b.Name {
				continue
			}
			if other.Spec.ServiceAccountRef.UID == string(sa.UID) || string(other.Spec.ServiceAccountRef.Name) == saName {
				return nil, fmt.Errorf("BindingMismatch: service account is shared by binding %s", other.Name)
			}
		}
		continuation = bindings.Continue
		if continuation == "" {
			break
		}
		if count >= definitions.MaxRunObjects || seen[continuation] {
			return nil, fmt.Errorf("WorkLimitExceeded: incomplete binding identity inventory")
		}
		seen[continuation] = true
	}
	return authority, nil
}

// RuntimeFactory owns the isolated runtime client construction seam. It never
// accepts a manager RESTMapper or caches discovery across identities/revisions.
type RuntimeFactory struct {
	base                 *rest.Config
	scheme               *runtime.Scheme
	reader               client.Reader
	namespace, managerSA string
}

// NewRuntimeFactory snapshots credentials but never reuses inherited transport
// wrappers or impersonation. Opaque custom transports cannot establish this
// invariant and are rejected. Built-in client construction remains unchanged.
func NewRuntimeFactory(base *rest.Config, scheme *runtime.Scheme, reader client.Reader, namespace, managerSA string) (*RuntimeFactory, error) {
	if base == nil || scheme == nil || reader == nil || namespace == "" || managerSA == "" {
		return nil, fmt.Errorf("AuthorizationUnavailable: runtime factory prerequisites missing")
	}
	if base.Transport != nil {
		return nil, fmt.Errorf("AuthorizationUnavailable: opaque base transport is unsupported for runtime")
	}
	return &RuntimeFactory{base: rest.CopyConfig(base), scheme: scheme, reader: reader, namespace: namespace, managerSA: managerSA}, nil
}

// ClientFor resolves live identity and requires the caller's per-run scope/budget
// wrapper. Authentication credentials only authenticate impersonation; neither
// evaluator nor discovery requests ever fall back to the manager identity.
func (f *RuntimeFactory) ClientFor(ctx context.Context, name string, buildGuard func(*RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error)) (client.Client, *RuntimeAuthority, error) {
	if f == nil || buildGuard == nil {
		return nil, nil, fmt.Errorf("AuthorizationUnavailable: runtime factory and transport guard required")
	}
	authority, err := ResolveRuntimeAuthority(ctx, f.reader, f.namespace, f.managerSA, name)
	if err != nil {
		return nil, nil, err
	}
	username := SAUsername(f.namespace, authority.ServiceAccount.Name)
	guardInput := &RuntimeAuthority{Definition: authority.Definition.DeepCopy(), Binding: authority.Binding.DeepCopy(), ServiceAccount: authority.ServiceAccount.DeepCopy()}
	wrap, err := buildGuard(guardInput)
	if err != nil {
		return nil, nil, err
	}
	if wrap == nil {
		return nil, nil, fmt.Errorf("AuthorizationUnavailable: transport guard missing")
	}
	cfg := rest.CopyConfig(f.base)
	cfg.Impersonate = rest.ImpersonationConfig{}
	cfg.RateLimiter = nil
	cfg.QPS = -1
	cfg.Burst = 0
	cfg.Timeout = definitions.MaxRequestDuration
	cfg.ContentType = "application/json"
	cfg.AcceptContentTypes = "application/json"
	cfg.WrapTransport = func(next http.RoundTripper) http.RoundTripper {
		return wrap(runtimeIdentityTransport{next: next, username: username})
	}
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, nil, err
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return fmt.Errorf("runtime API redirects are forbidden") }
	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		return nil, nil, err
	}
	c, err := client.New(cfg, client.Options{Scheme: f.scheme, Mapper: mapper, HTTPClient: httpClient})
	if err != nil {
		return nil, nil, err
	}
	return c, authority, nil
}

type runtimeIdentityTransport struct {
	next     http.RoundTripper
	username string
}

func (t runtimeIdentityTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clean := req.Clone(req.Context())
	for key := range clean.Header {
		if strings.HasPrefix(strings.ToLower(key), "impersonate-") {
			delete(clean.Header, key)
		}
	}
	if t.username != "" {
		clean.Header.Set("Impersonate-User", t.username)
	}
	return t.next.RoundTrip(clean)
}
