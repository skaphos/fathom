/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation

import (
	"context"
	"fmt"
	"net/http"

	api "github.com/skaphos/fathom/api/v1alpha1"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewRuntimeControlReader creates a fresh uncached manager-identity reader for a
// single run. Share its budget with the delegated Guard; never pass this reader
// to an evaluator. Its fixed mapper and transport expose only fence metadata.
func NewRuntimeControlReader(base *rest.Config, budget *execution.Budget, targets execution.ControlTargets) (client.Reader, error) {
	if base == nil || base.Transport != nil {
		return nil, fmt.Errorf("AuthorizationUnavailable: explicit credentials without an opaque transport required")
	}
	return newRuntimeControlReader(rest.CopyConfig(base), budget, targets)
}

func newRuntimeControlReader(base *rest.Config, budget *execution.Budget, targets execution.ControlTargets) (RuntimeControlReader, error) {
	guard, err := execution.NewControlGuard(budget, targets)
	if err != nil {
		return RuntimeControlReader{}, err
	}
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{api.AddToScheme, corev1.AddToScheme, coordinationv1.AddToScheme} {
		if err := add(scheme); err != nil {
			return RuntimeControlReader{}, err
		}
	}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{api.GroupVersion, corev1.SchemeGroupVersion, coordinationv1.SchemeGroupVersion})
	for _, mapping := range []struct {
		gv                     schema.GroupVersion
		kind, plural, singular string
		scope                  meta.RESTScope
	}{
		{api.GroupVersion, "AddonDefinition", "addondefinitions", "addondefinition", meta.RESTScopeRoot},
		{api.GroupVersion, "AddonDefinitionBinding", "addondefinitionbindings", "addondefinitionbinding", meta.RESTScopeNamespace},
		{api.GroupVersion, "AddonCheck", "addonchecks", "addoncheck", meta.RESTScopeNamespace},
		{corev1.SchemeGroupVersion, "ServiceAccount", "serviceaccounts", "serviceaccount", meta.RESTScopeNamespace},
		{coordinationv1.SchemeGroupVersion, "Lease", "leases", "lease", meta.RESTScopeNamespace},
	} {
		mapper.AddSpecific(mapping.gv.WithKind(mapping.kind), mapping.gv.WithResource(mapping.plural), mapping.gv.WithResource(mapping.singular), mapping.scope)
	}
	cfg := rest.CopyConfig(base)
	cfg.Impersonate = rest.ImpersonationConfig{}
	cfg.RateLimiter, cfg.QPS, cfg.Burst = nil, -1, 0
	cfg.Timeout = definitions.MaxRequestDuration
	cfg.ContentType, cfg.AcceptContentTypes = "application/json", "application/json"
	cfg.WrapTransport = func(next http.RoundTripper) http.RoundTripper {
		return guard.Wrap(runtimeIdentityTransport{next: next})
	}
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return RuntimeControlReader{}, err
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("runtime control API redirects are forbidden")
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme, Mapper: mapper, HTTPClient: httpClient})
	if err != nil {
		return RuntimeControlReader{}, err
	}
	return RuntimeControlReader{
		Reader:  c,
		targets: targets,
		derive: func(scoped execution.ControlTargets) (RuntimeControlReader, error) {
			return newRuntimeControlReader(base, budget, scoped)
		},
	}, nil
}

// RuntimeControlReader names the uncached, budget-shared control-plane reader that
// authority resolution requires, so a manager cached client can no longer satisfy
// that seam by accident: wrapping some other reader in this type is a deliberate,
// greppable claim that it is uncached and budget-bound. It embeds only
// client.Reader, so no write method is reachable even by type assertion.
type RuntimeControlReader struct {
	client.Reader
	targets execution.ControlTargets
	derive  func(execution.ControlTargets) (RuntimeControlReader, error)
}

// WithServiceAccount returns an immutable reader that permits the one account
// named by an already validated binding. Each authority resolution derives from
// the original deny-all reader, so names cannot accumulate across fences.
func (r RuntimeControlReader) WithServiceAccount(namespace, name string) (RuntimeControlReader, error) {
	if r.Reader == nil || len(validation.IsDNS1123Label(namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(name)) != 0 {
		return RuntimeControlReader{}, fmt.Errorf("AuthorizationUnavailable: exact service account target required")
	}
	if r.targets.OperatorNamespace != "" && namespace != r.targets.OperatorNamespace {
		return RuntimeControlReader{}, fmt.Errorf("AuthorizationUnavailable: service account target is outside the operator namespace")
	}
	scoped := r.targets
	scoped.OperatorNamespace = namespace
	scoped.ServiceAccountName = name
	if r.derive != nil {
		return r.derive(scoped)
	}
	// Tests may make an explicit uncached-reader claim with a fake Reader. Keep
	// that seam exact at the Reader boundary rather than falling back to broad
	// access when no production transport builder is present.
	r.targets = scoped
	return r, nil
}

func (r RuntimeControlReader) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.ServiceAccount); ok &&
		(key.Namespace != r.targets.OperatorNamespace || key.Name == "" || key.Name != r.targets.ServiceAccountName) {
		return fmt.Errorf("AuthorizationUnavailable: service account %s is outside the exact control target", key)
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

func (r RuntimeControlReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.ServiceAccountList); ok {
		return fmt.Errorf("AuthorizationUnavailable: service account list is outside the exact control target")
	}
	return r.Reader.List(ctx, list, opts...)
}
