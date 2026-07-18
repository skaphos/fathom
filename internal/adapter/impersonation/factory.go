/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package impersonation builds controller-runtime clients that impersonate a
// per-addon ServiceAccount, so an addon adapter reads the cluster under its own
// least-privilege identity rather than the operator's (SKA-58).
//
// The clients are DIRECT (uncached): controller-runtime's cached client serves
// typed reads from informers under the operator's own identity and does not honor
// rest.Config.Impersonate, so a cached impersonating client would enforce nothing.
// A direct client routes every read through the API server, the one place RBAC —
// and thus impersonation — is evaluated.
package impersonation

import (
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SAUsername returns the impersonation username for a ServiceAccount, e.g.
// SAUsername("fathom-system", "fathom-addon-coredns") ==
// "system:serviceaccount:fathom-system:fathom-addon-coredns".
func SAUsername(namespace, name string) string {
	return "system:serviceaccount:" + namespace + ":" + name
}

// ClientFactory builds (and caches) a direct client that impersonates a given
// ServiceAccount username. Implementations are safe for concurrent use by the
// reconciler's worker pool.
type ClientFactory interface {
	// ClientFor returns a direct, uncached client whose every API request
	// impersonates username. The client is memoized per username.
	ClientFor(username string) (client.Client, error)
}

// factory memoizes one direct impersonating client per ServiceAccount username.
// rest.CopyConfig guarantees the shared base config is never mutated, and the
// clients are uncached so there is no cache-staleness concern.
type factory struct {
	base   *rest.Config
	scheme *runtime.Scheme
	mapper meta.RESTMapper
	// newClient is the client constructor, overridable in tests.
	newClient func(*rest.Config, client.Options) (client.Client, error)

	mu      sync.Mutex
	clients map[string]client.Client
}

// New returns a ClientFactory that derives impersonating clients from the
// manager's rest.Config, scheme, and RESTMapper. The RESTMapper is shared across
// all impersonating clients: it is identity-independent, so sharing it avoids
// re-running API discovery per addon.
func New(mgr ctrl.Manager) ClientFactory {
	return &factory{
		base:      mgr.GetConfig(),
		scheme:    mgr.GetScheme(),
		mapper:    mgr.GetRESTMapper(),
		newClient: client.New,
		clients:   map[string]client.Client{},
	}
}

func (f *factory) ClientFor(username string) (client.Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.clients[username]; ok {
		return c, nil
	}
	cfg := rest.CopyConfig(f.base)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: username}
	// A provided Mapper means client.New performs no API discovery, so holding
	// the lock across this call is cheap.
	c, err := f.newClient(cfg, client.Options{Scheme: f.scheme, Mapper: f.mapper})
	if err != nil {
		return nil, err
	}
	f.clients[username] = c
	return c, nil
}
