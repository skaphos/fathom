/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package adapter defines the contract that addon adapters implement to
// participate in Fathom's AddonCheck reconciliation.
//
// An adapter is a Go type that satisfies [Adapter]. It declares a stable
// identity (Name, Version), the contract version it targets (ContractVersion),
// the set of addon kinds and check families it can handle (Capabilities), and
// a Run method that performs checks and returns a [Result] composed of zero
// or more [CheckResult] values.
//
// # Contract versioning
//
// The contract follows Semantic Versioning. Fathom (the host that loads
// adapters) and adapters each report a contract version; an adapter is
// considered compatible with a Fathom build when the two versions share the
// same major component. Minor and patch releases are additive: an adapter
// built against 1.0.0 keeps working on a 1.x host. Use [EnsureCompatible] to
// validate an adapter's reported contract version before invoking it.
//
// The current contract version is exported as [ContractVersion].
//
// # Authoring an adapter
//
// Implement the [Adapter] interface in your own module and import this
// package. Embed [ContractVersion] in your adapter so a contract bump in
// Fathom will be visible at adapter-build time:
//
//	import "github.com/skaphos/fathom/pkg/adapter"
//
//	type MyAdapter struct{}
//
//	func (MyAdapter) Name() string             { return "my-addon" }
//	func (MyAdapter) Version() string          { return "1.2.3" }
//	func (MyAdapter) ContractVersion() string  { return adapter.ContractVersion }
//	func (MyAdapter) Capabilities() adapter.Capabilities {
//	    return adapter.Capabilities{
//	        AddonTypes: []string{"my-addon"},
//	        Families:   []adapter.Family{"system_health"},
//	    }
//	}
//	func (MyAdapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
//	    // ... perform checks, return Result
//	}
//
// # Result shape
//
// A single Run produces one [Result] per AddonCheck reconciliation.
// Each [CheckResult] inside the Result corresponds to a single observed
// signal — typically one resource within one family. The Details map is
// constrained to string values so the result remains serializable across a
// future process boundary; adapters needing structure should encode it as
// JSON in a single key.
//
// # Stability
//
// The current contract version is [ContractVersion]. From 1.0.0 the contract
// is a stable public extension point: breaking changes require a major bump,
// and minor/patch releases only add surface (new Request fields, new optional
// interfaces) that existing adapters may ignore.
package adapter
