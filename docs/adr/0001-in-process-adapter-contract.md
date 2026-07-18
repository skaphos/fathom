<!--
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
-->
# 1. In-process Go interface as the AddonAdapter contract

- **Status**: accepted
- **Date**: 2026-05-10
- **Deciders**: Shawn Stratton

## Context and Problem Statement

Fathom reconciles `AddonCheck` resources by dispatching to per-addon
implementations ("adapters") that know how to interrogate cert-manager,
CoreDNS, External Secrets, and so on. Each adapter must:

- declare which addon types it handles and which check families it can run,
- accept a per-run `Request` (logger, namespaced client, policy, timeout),
- return a structured `Result` whose entries become a `HealthReport`,
- run safely from multiple goroutines (one per `AddonCheck`),
- be versioned independently from Fathom so a contract bump is observable.

The `AddonCheckReconciler` needs a way to look an adapter up by addon type
and invoke it inside the reconcile loop. The choice of *how* adapters are
loaded and dispatched constrains every future adapter and the operator's
deployment surface.

## Decision Drivers

- v0.1 ships three in-tree adapters; the loading model needs to be simple
  enough that adding a fourth is a one-line `Register` call, not a build-
  system change.
- The operator is shipped as a single OLM bundle. A loading model that
  requires a sidecar, separate deployment, or registry of binaries
  multiplies the install/upgrade surface.
- Adapter authors will iterate faster than Fathom releases. The contract
  must let an adapter assert which version it was built against and let
  Fathom refuse incompatible adapters at startup, not at first invocation.
- The decision should not foreclose a future out-of-process plugin model.

## Considered Options

- **A. In-process Go interface, statically registered at boot.** Adapters
  implement `pkg/adapter.Adapter`; Fathom's `internal/app.Run` calls
  `registry.Register` for each compiled-in adapter. Contract version is a
  string returned by the adapter and validated against `pkg/adapter.ContractVersion`.
- **B. Out-of-process gRPC plugins.** Adapters are separate binaries; Fathom
  discovers them at startup, exchanges a handshake, and dispatches via gRPC
  (HashiCorp go-plugin shape).
- **C. OCI bundle adapters launched as Pods per run.** Adapters are
  container images referenced by `AddonCheck.spec`; the reconciler launches
  a Pod for each run.
- **D. Go `plugin` package (`.so` dlopen).** Adapters are shared objects
  loaded at runtime.

## Decision Outcome

Chosen option: **A. In-process Go interface, statically registered at
boot**, because it is the smallest model that satisfies the v0.1 drivers
(simple deployment, fast dispatch, type-safe contract, semver handshake)
without foreclosing B or C as future loaders against the same `Registry`.

The contract lives in `pkg/adapter` (importable). The registry lives in
`internal/adapter/registry` (Fathom-internal). The contract carries an
explicit `ContractVersion()` method and `pkg/adapter.EnsureCompatible`
treats pre-1.0 minor bumps as breaking. `Registry.Register` rejects
incompatible adapters at boot rather than at first reconcile.

### Consequences

- **Positive**: one binary to ship; no sidecar; reconcile dispatch is a map
  lookup plus a method call. Type changes to `Request`/`Result` are caught
  at adapter build time, not runtime. Contract-version handshake makes
  drift visible at process start.
- **Negative**: adapter authors must use Go and rebuild against each
  Fathom contract bump. Out-of-tree adapters cannot ship today — a future
  loader is needed before that becomes practical (deferred). A buggy
  adapter can panic the operator pod; cross-adapter isolation is process-
  level, not boundary-enforced.
- **Neutral**: `internal/adapter/registry` is sealed off as internal so
  the registry's API can evolve without breaking out-of-tree adapter
  authors, who only import `pkg/adapter`.

## Pros and Cons of the Options

### A. In-process Go interface

- Good, because dispatch is sub-microsecond and shares process credentials,
  logger, and controller-runtime client.
- Good, because the contract is checked at compile time and at boot.
- Bad, because it forces Go and forces rebuilds on contract bumps.
- Bad, because adapter faults are not isolated from the operator process.

### B. Out-of-process gRPC plugins

- Good, because adapters can be written in any language, deployed
  independently, and isolated from operator faults.
- Bad, because every Fathom install needs an adapter discovery and
  lifecycle story (where do the plugin binaries live, how are they
  upgraded, how is the gRPC channel secured).
- Bad, because v0.1 has no concrete out-of-tree adapter author asking for
  this; the cost is paid up front for hypothetical demand.

### C. OCI bundle adapters launched as Pods per run

- Good, because adapters are language-agnostic and isolated by Pod
  boundary; contract version is part of the image tag.
- Bad, because every reconcile pays Pod-create latency. AddonCheck runs
  cadence on the order of minutes, but launching a Pod per run still
  multiplies API server load and image-pull traffic.
- Bad, because adapters lose access to the operator's controller-runtime
  client and informer caches; each Pod re-establishes its own.

### D. Go `plugin` package

- Good, because dispatch is in-process and dynamic.
- Bad, because Go plugins are notoriously fragile: build flags, exact
  compiler version, glibc version, and module versions must match between
  the host and the plugin. Operationally a non-starter for a v0.1 ship.

## Links

- `pkg/adapter/adapter.go` — `Adapter` interface and `Request`/`Result` types
- `pkg/adapter/version.go` — `ContractVersion`, `EnsureCompatible`
- `internal/adapter/registry/registry.go` — `Registry`, `Register`, `Lookup`
- Commits: `4bafbd7` (contract + handshake), `a61e1f8` (registry),
  `c98c268` (registry wired into manager)
- Related Linear: SKA-52, SKA-53, SKA-291
