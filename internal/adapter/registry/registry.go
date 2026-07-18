/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package registry holds the set of [adapter.Adapter] implementations Fathom
// can dispatch to during AddonCheck reconciliation. It is a Fathom-internal
// runtime concern; out-of-tree adapter authors interact only with the
// contract in [github.com/skaphos/fathom/pkg/adapter].
//
// The current loading model is in-process and explicit: Fathom's manager
// startup constructs a [Registry] and calls [Registry.Register] for each
// compiled-in adapter. A future out-of-process loader can register adapters
// against the same Registry without changing this package's external API.
package registry

import (
	"errors"
	"fmt"
	"sync"

	"github.com/go-logr/logr"

	"github.com/skaphos/fathom/pkg/adapter"
)

// ErrNotFound is returned by [Registry.Lookup] when no adapter is registered
// for the requested addon type. Callers should treat this as terminal under
// the v0.1 loading model — adapters are registered at process start, so a
// missing entry will not appear later in the same Fathom process.
var ErrNotFound = errors.New("registry: no adapter registered for addon type")

// Registry is the in-memory index of adapters keyed by addon type. The zero
// value is not usable; construct with [New].
//
// Register/Lookup/Capabilities are safe for concurrent use, but Register is
// expected to be called only at process startup. Holding the write lock
// during reconciliation would block dispatch.
type Registry struct {
	mu      sync.RWMutex
	logger  logr.Logger
	byAddon map[string]adapter.Adapter
}

// New returns a Registry ready for [Registry.Register] calls. The logger is
// used to surface non-fatal events such as idempotent re-registration; pass
// [logr.Discard] in tests that do not care.
func New(logger logr.Logger) *Registry {
	return &Registry{logger: logger, byAddon: map[string]adapter.Adapter{}}
}

// Register adds a to the registry, keyed by every addon type it advertises.
//
// Register fails if any of the following holds, and in each case no addon
// type from a is added (the registry is left unchanged):
//
//   - a is nil.
//   - a.ContractVersion() is incompatible with this build of Fathom, as
//     determined by [adapter.EnsureCompatible].
//   - a.Capabilities().AddonTypes is empty — an adapter that handles no
//     addon types cannot be dispatched to.
//   - any of a's addon types is already registered to a different adapter
//     (matched by [adapter.Adapter.Name]).
func (r *Registry) Register(a adapter.Adapter) error {
	if a == nil {
		return errors.New("registry: cannot register nil adapter")
	}
	name := a.Name()
	if err := adapter.EnsureCompatible(a.ContractVersion()); err != nil {
		return fmt.Errorf("registry: adapter %q rejected: %w", name, err)
	}
	addonTypes := a.Capabilities().AddonTypes
	if len(addonTypes) == 0 {
		return fmt.Errorf("registry: adapter %q advertises no addon types", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	alreadyMine := 0
	for _, at := range addonTypes {
		existing, ok := r.byAddon[at]
		if !ok {
			continue
		}
		if existing.Name() != name {
			return fmt.Errorf(
				"registry: addon type %q is already registered to adapter %q; cannot also register to %q",
				at, existing.Name(), name,
			)
		}
		alreadyMine++
	}
	// If every advertised addon type is already mapped to this adapter Name,
	// treat the call as an idempotent no-op and surface it via a log notice
	// rather than an error. This keeps startup resilient when a future loader
	// announces an adapter from multiple sources.
	if alreadyMine == len(addonTypes) {
		r.logger.Info(
			"adapter already registered; ignoring duplicate Register call",
			"adapter", name,
			"addonTypes", addonTypes,
		)
		return nil
	}
	for _, at := range addonTypes {
		r.byAddon[at] = a
	}
	return nil
}

// Lookup returns the adapter registered for addonType, or [ErrNotFound] if
// none is registered. Callers must not retain the returned adapter beyond
// the reconciliation that retrieved it: future loaders may invalidate it.
func (r *Registry) Lookup(addonType string) (adapter.Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byAddon[addonType]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, addonType)
	}
	return a, nil
}

// Capabilities returns a snapshot of every registered adapter's capabilities,
// keyed by [adapter.Adapter.Name]. The returned maps and slices are safe for
// the caller to retain; mutation will not affect the registry.
//
// Intended for diagnostics and for surfacing the supported addon-type set in
// HealthReport metadata. Not on the per-reconcile hot path.
func (r *Registry) Capabilities() map[string]adapter.Capabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]struct{}{}
	out := map[string]adapter.Capabilities{}
	for _, a := range r.byAddon {
		name := a.Name()
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		caps := a.Capabilities()
		out[name] = adapter.Capabilities{
			AddonTypes: append([]string(nil), caps.AddonTypes...),
			Families:   append([]adapter.Family(nil), caps.Families...),
		}
	}
	return out
}
