/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation

import (
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
	guard, err := execution.NewControlGuard(budget, targets)
	if err != nil {
		return nil, err
	}
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{api.AddToScheme, corev1.AddToScheme, coordinationv1.AddToScheme} {
		if err := add(scheme); err != nil {
			return nil, err
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
		return nil, err
	}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("runtime control API redirects are forbidden")
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme, Mapper: mapper, HTTPClient: httpClient})
	if err != nil {
		return nil, err
	}
	return RuntimeControlReader{Reader: c}, nil
}

// RuntimeControlReader names the uncached, budget-shared control-plane reader that
// authority resolution requires, so a manager cached client can no longer satisfy
// that seam by accident: wrapping some other reader in this type is a deliberate,
// greppable claim that it is uncached and budget-bound. It embeds only
// client.Reader, so no write method is reachable even by type assertion.
type RuntimeControlReader struct{ client.Reader }
