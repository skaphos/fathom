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
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

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

	// runtimeMu serializes runtime snapshot writers only. Readers never take
	// it: they load the published snapshot atomically, so a slow replacement
	// cannot stall dispatch.
	runtimeMu sync.Mutex
	runtime   atomic.Pointer[runtimeSnapshot]
	admission atomic.Pointer[admissionState]
	startup   atomic.Pointer[map[string]startupClaim]
}

// New returns a Registry ready for [Registry.Register] calls. The logger is
// used to surface non-fatal events such as idempotent re-registration; pass
// [logr.Discard] in tests that do not care.
//
// Runtime dispatch starts barred: [Registry.OpenRuntimeDispatch] admits it
// only when an elected leadership session drives it, after that session has
// synchronized its informers and validated authority directly. Built-in
// dispatch is never gated by that barrier.
func New(logger logr.Logger) *Registry {
	r := &Registry{logger: logger, byAddon: map[string]adapter.Adapter{}}
	r.runtime.Store(buildRuntimeSnapshot(map[types.UID]*runtimeOwner{}))
	r.admission.Store(&admissionState{reason: "runtime dispatch has not been admitted"})
	claims := map[string]startupClaim{}
	r.startup.Store(&claims)
	return r
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

// Lookup returns the adapter that may serve addonType, or [ErrNotFound] if
// none is registered. Callers must not retain the returned adapter beyond
// the reconciliation that retrieved it: a runtime replacement may supersede
// it. When a dispatch barrier covers addonType, Lookup returns that barrier
// rather than a candidate; see [Registry.Resolve] for the full resolution.
func (r *Registry) Lookup(addonType string) (adapter.Adapter, error) {
	res, err := r.Resolve(addonType)
	if err != nil {
		return nil, err
	}
	return res.Adapter, nil
}

// builtin returns the built-in adapter registered for addonType.
func (r *Registry) builtin(addonType string) (adapter.Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byAddon[addonType]
	return a, ok
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

// --- runtime snapshots ---------------------------------------------------
//
// Runtime adapters are compiled from AddonDefinition objects rather than
// linked into the binary, so their identity can change while Fathom runs.
// They are therefore kept out of the built-in map entirely: an owner-keyed,
// copy-on-write snapshot is built off-lock and published with a single atomic
// store, which keeps dispatch lock-free and makes any snapshot a reader
// already holds immutable for as long as it holds it.

// Barrier reasons. They are operator-visible condition reasons, so they are
// spelled exactly as the runtime contract spells them.
const (
	// ReasonBuiltinCollision marks an identity claimed by both a built-in
	// adapter and a runtime definition. Neither dispatches until an
	// administrator migrates the definition or deletes it.
	ReasonBuiltinCollision = "BuiltinCollision"
	// ReasonRuntimeCollision marks an identity claimed by two runtime
	// definitions. Cluster name uniqueness prohibits it, so this can only be
	// legacy or invalid state: it fails closed with no winner decided by
	// arrival order.
	ReasonRuntimeCollision = "RuntimeCollision"
	// ReasonAdmissionClosed marks runtime dispatch that is not admitted:
	// before informer synchronization and direct validation, or after
	// leadership, drain or configuration withdrew it.
	ReasonAdmissionClosed = "RuntimeAdmissionClosed"
)

// ErrDispatchBarred matches every [DispatchBarrier] under [errors.Is], so a
// caller can separate "cannot dispatch right now" from [ErrNotFound] without
// switching on the reason.
var ErrDispatchBarred = errors.New("registry: dispatch barrier")

// DispatchBarrier suppresses dispatch for a single addon type. It is returned
// instead of a candidate, never alongside one — a barrier suppresses every
// candidate for that identity, which is what stops an upgrade from silently
// reinterpreting an existing AddonCheck.
type DispatchBarrier struct {
	AddonType string
	// Reason is one of the Reason* constants above.
	Reason string
	// Claimants names every suppressed candidate, for operator triage.
	Claimants []string
	// Detail carries the caller-supplied explanation for a closed admission.
	Detail string
}

func (b *DispatchBarrier) Error() string {
	msg := fmt.Sprintf("registry: %s: addon type %q cannot be dispatched", b.Reason, b.AddonType)
	if len(b.Claimants) > 0 {
		msg += ": claimed by " + strings.Join(b.Claimants, " and ")
	}
	if b.Detail != "" {
		msg += ": " + b.Detail
	}
	return msg
}

// Is reports ErrDispatchBarred so callers can match every barrier reason.
func (b *DispatchBarrier) Is(target error) bool { return target == ErrDispatchBarred }

// RuntimeRevision is exactly the tuple that makes a compiled snapshot
// attributable: the definition's UID, the generation it was compiled from and
// the schema and semantics versions that fixed its meaning. Names are
// deliberately absent — a recreated definition is a different revision.
type RuntimeRevision struct {
	DefinitionUID    types.UID
	Generation       int64
	SchemaVersion    string
	SemanticsVersion int32
}

// RuntimeProvenance is the additional publication context: the operator build
// that compiled the snapshot and the definition's declared adapterVersion. It
// is recorded with, but is not part of, the revision.
type RuntimeProvenance struct {
	OperatorBuild  string
	AdapterVersion string
}

// RuntimeEntry is one owned runtime adapter: the compiled adapter plus the
// revision and provenance it was published with.
type RuntimeEntry struct {
	Adapter    adapter.Adapter
	Revision   RuntimeRevision
	Provenance RuntimeProvenance
	// AddonTypes is derived from the adapter's capabilities when the entry is
	// published; any value supplied to [Registry.SetRuntime] is ignored.
	// Copies handed back by [Registry.RuntimeEntries] own their slice.
	AddonTypes []string
}

// Resolution is the outcome of resolving an addon type for dispatch. Revision
// and Provenance are zero for built-ins, which have no definition identity.
type Resolution struct {
	Adapter    adapter.Adapter
	Runtime    bool
	Revision   RuntimeRevision
	Provenance RuntimeProvenance
}

// runtimeOwner is an immutable published entry. Snapshots share owners by
// pointer, so an owner is never mutated after publication — a change builds a
// new owner and a new snapshot.
type runtimeOwner struct{ entry RuntimeEntry }

func (o *runtimeOwner) describe() string {
	return fmt.Sprintf("runtime definition %q (adapter %q, generation %d)",
		o.entry.Revision.DefinitionUID, o.entry.Adapter.Name(), o.entry.Revision.Generation)
}

// runtimeSnapshot is immutable once published.
type runtimeSnapshot struct {
	owners  map[types.UID]*runtimeOwner
	byAddon map[string]*runtimeOwner
	// barred records the claimants of an addon type owned by more than one
	// runtime definition. Such an identity dispatches to nobody.
	barred map[string][]string
}

// admissionState is the runtime dispatch gate. driver names the leadership
// session that admitted dispatch; reason explains a closed gate.
type admissionState struct {
	open   bool
	driver string
	reason string
}

// buildRuntimeSnapshot indexes owners by addon type and precomputes the
// runtime-versus-runtime collisions. Callers must not mutate owners
// afterwards. Work is proportional to the number of published definitions and
// happens off the dispatch path.
func buildRuntimeSnapshot(owners map[types.UID]*runtimeOwner) *runtimeSnapshot {
	snap := &runtimeSnapshot{
		owners:  owners,
		byAddon: make(map[string]*runtimeOwner, len(owners)),
		barred:  map[string][]string{},
	}
	uids := make([]types.UID, 0, len(owners))
	for uid := range owners {
		uids = append(uids, uid)
	}
	// Deterministic claimant order keeps barrier messages stable regardless of
	// the order definitions happened to arrive in.
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	claims := map[string][]*runtimeOwner{}
	for _, uid := range uids {
		for _, addonType := range owners[uid].entry.AddonTypes {
			claims[addonType] = append(claims[addonType], owners[uid])
		}
	}
	for addonType, claimants := range claims {
		if len(claimants) == 1 {
			snap.byAddon[addonType] = claimants[0]
			continue
		}
		for _, c := range claimants {
			snap.barred[addonType] = append(snap.barred[addonType], c.describe())
		}
	}
	return snap
}

// SetRuntime publishes entry as the snapshot owned by its definition UID,
// replacing whatever that UID published before — including addon types the new
// revision no longer claims. Other owners and every built-in are untouched.
//
// The entry is rejected, publishing nothing, when:
//
//   - its adapter is nil, advertises no addon types, or reports an
//     incompatible contract version ([adapter.EnsureCompatible]);
//   - any part of the (definition UID, generation, schema, semantics) revision
//     or of the operator-build/adapterVersion provenance is missing;
//   - a strictly later generation from the same owner is already published — a
//     stale event must not regress a replacement. An equal generation is not
//     stale and does replace: republishing a generation is how a recompile of
//     the same definition revision — an operator upgrade that changed only the
//     schema, semantics or provenance it compiles under — reaches dispatch, and
//     refusing it would pin dispatch to the superseded compilation.
//
// The entry is published but suppressed, and a *[DispatchBarrier] is returned,
// when its identity is also claimed by a built-in or by another runtime
// definition. The caller surfaces that as Ready=False with the barrier reason;
// dropping the entry instead would let the surviving candidate silently
// reinterpret the identity.
func (r *Registry) SetRuntime(entry RuntimeEntry) error {
	if entry.Adapter == nil {
		return errors.New("registry: cannot publish a runtime snapshot with a nil adapter")
	}
	rev, prov := entry.Revision, entry.Provenance
	switch {
	case rev.DefinitionUID == "":
		return errors.New("registry: runtime snapshot requires a definition UID")
	case rev.Generation < 1:
		return fmt.Errorf("registry: runtime snapshot for %q requires a definition generation", rev.DefinitionUID)
	case rev.SchemaVersion == "":
		return fmt.Errorf("registry: runtime snapshot for %q requires a schema version", rev.DefinitionUID)
	case rev.SemanticsVersion < 1:
		return fmt.Errorf("registry: runtime snapshot for %q requires a semantics version", rev.DefinitionUID)
	case prov.OperatorBuild == "":
		return fmt.Errorf("registry: runtime snapshot for %q requires the operator build provenance", rev.DefinitionUID)
	case prov.AdapterVersion == "":
		return fmt.Errorf("registry: runtime snapshot for %q requires adapterVersion provenance", rev.DefinitionUID)
	}
	name := entry.Adapter.Name()
	if err := adapter.EnsureCompatible(entry.Adapter.ContractVersion()); err != nil {
		return fmt.Errorf("registry: runtime adapter %q rejected: %w", name, err)
	}
	// Read capabilities before taking any lock: it is adapter-supplied code,
	// and neither dispatch nor another owner's publication may wait behind it.
	addonTypes := append([]string(nil), entry.Adapter.Capabilities().AddonTypes...)
	if len(addonTypes) == 0 {
		return fmt.Errorf("registry: runtime adapter %q advertises no addon types", name)
	}
	seen := map[string]struct{}{}
	for _, at := range addonTypes {
		if at == "" {
			return fmt.Errorf("registry: runtime adapter %q advertises an empty addon type", name)
		}
		if _, dup := seen[at]; dup {
			return fmt.Errorf("registry: runtime adapter %q advertises addon type %q twice", name, at)
		}
		seen[at] = struct{}{}
	}
	entry.AddonTypes = addonTypes

	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	current := r.runtime.Load()
	// Strictly older only. An equal generation is a recompile of the same
	// definition revision under a new schema, semantics or operator build, and
	// must be allowed to replace what it supersedes.
	if existing := current.owners[rev.DefinitionUID]; existing != nil &&
		existing.entry.Revision.Generation > rev.Generation {
		return fmt.Errorf(
			"registry: stale runtime snapshot for definition %q: generation %d is older than the published generation %d",
			rev.DefinitionUID, rev.Generation, existing.entry.Revision.Generation,
		)
	}
	owners := make(map[types.UID]*runtimeOwner, len(current.owners)+1)
	for uid, owner := range current.owners {
		owners[uid] = owner
	}
	owners[rev.DefinitionUID] = &runtimeOwner{entry: entry}
	next := buildRuntimeSnapshot(owners)
	// Publish first: the barrier below is a property of what is now published,
	// and both colliding candidates must be suppressed either way.
	r.runtime.Store(next)
	for _, at := range addonTypes {
		if barrier := r.barrierFor(next, at); barrier != nil {
			return barrier
		}
	}
	return nil
}

// RemoveRuntime removes the snapshot owned by uid and reports whether one was
// removed. It is owner aware on purpose: a delete event that arrives after a
// recreated definition published a new UID matches nothing and must leave the
// replacement in place.
func (r *Registry) RemoveRuntime(uid types.UID) bool {
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	current := r.runtime.Load()
	if _, ok := current.owners[uid]; !ok {
		return false
	}
	owners := make(map[types.UID]*runtimeOwner, len(current.owners)-1)
	for owned, owner := range current.owners {
		if owned == uid {
			continue
		}
		owners[owned] = owner
	}
	r.runtime.Store(buildRuntimeSnapshot(owners))
	return true
}

// RuntimeEntries returns a copy of every published runtime entry, ordered by
// definition UID. The copy owns its slices: later replacements never change
// what an earlier caller was handed, and caller mutation never reaches the
// registry. Intended for diagnostics and collision reporting.
func (r *Registry) RuntimeEntries() []RuntimeEntry {
	snap := r.runtime.Load()
	out := make([]RuntimeEntry, 0, len(snap.owners))
	for _, owner := range snap.owners {
		entry := owner.entry
		entry.AddonTypes = append([]string(nil), owner.entry.AddonTypes...)
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision.DefinitionUID < out[j].Revision.DefinitionUID })
	return out
}

// startupClaim holds one bounded, provisional built-in reservation.
type startupClaim struct {
	uid                 types.UID
	generation          int64
	completedUID        types.UID
	completedGeneration int64
}

// ReserveStartupClaims closes built-in dispatch while the enabled runtime
// loader discovers whether stored definitions claim those identities. An empty
// UID means the direct API read has not succeeded yet.
func (r *Registry) ReserveStartupClaims(addonTypes []string) {
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	claims := make(map[string]startupClaim, len(addonTypes))
	for _, name := range addonTypes {
		claims[name] = startupClaim{}
	}
	r.startup.Store(&claims)
}

// StartupClaims returns the unresolved startup reservations. Callers may use
// the copy for direct API reads without holding a registry lock.
func (r *Registry) StartupClaims() map[string]types.UID {
	out := map[string]types.UID{}
	for name, claim := range *r.startup.Load() {
		out[name] = claim.uid
	}
	return out
}

// ObserveStartupClaim records a direct API read for an existing reservation.
// A not-found read removes it; a successful read records the observed UID.
// Once normal reconciliation releases a claim, an older API read cannot
// recreate it.
func (r *Registry) ObserveStartupClaim(name string, uid types.UID, generation int64) {
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	old := *r.startup.Load()
	if _, exists := old[name]; !exists {
		return
	}
	next := make(map[string]startupClaim, len(old))
	for key, value := range old {
		if key != name {
			next[key] = value
		}
	}
	if uid != "" {
		claim := old[name]
		if claim.completedUID != uid || claim.completedGeneration != generation {
			// An earlier direct read may finish after the cache already
			// reconciled a newer revision. Keep that completion until a
			// direct read confirms it or a later reconciliation replaces it.
			claim.uid = uid
			claim.generation = generation
			next[name] = claim
		}
	}
	r.startup.Store(&next)
}

// ReleaseStartupClaim is called only after normal reconciliation established
// eligibility for the observed object. An event for an older UID cannot clear
// the reservation of a recreated definition.
func (r *Registry) ReleaseStartupClaim(name string, uid types.UID, generation int64) {
	r.runtimeMu.Lock()
	defer r.runtimeMu.Unlock()
	old := *r.startup.Load()
	claim, exists := old[name]
	if !exists {
		return
	}
	next := make(map[string]startupClaim, len(old))
	for key, value := range old {
		if key != name {
			next[key] = value
		}
	}
	if claim.uid == "" || claim.uid != uid || claim.generation != generation {
		// A cached reconciliation cannot certify an API read that failed.
		// It may also precede a direct read of a newly recreated or edited
		// revision. Remember the completed revision in either case and
		// release only when a later direct read confirms that exact pair.
		claim.completedUID = uid
		claim.completedGeneration = generation
		next[name] = claim
	}
	r.startup.Store(&next)
}

// ErrAdmissionDriverRequired is returned by [Registry.OpenRuntimeDispatch]
// when no driver identity is supplied. The gate stays closed, so a seam wired
// without a leadership session fails loudly instead of admitting dispatch that
// nobody is driving.
var ErrAdmissionDriverRequired = errors.New(
	"registry: runtime dispatch admission requires the identity of the leadership session driving it")

// OpenRuntimeDispatch admits runtime dispatch on behalf of driver, the elected
// leadership session that has already synchronized its informers and validated
// authority directly.
//
// The barrier lives here because the registry is where dispatch happens, and a
// barrier is only worth anything at the enforcement point. The decision does
// not: the registry deliberately does not observe leadership, Leases or drain
// state, and has no opinion about them. The leadership session is the single
// decider and the only intended driver of this method — one decider driving one
// enforcement point is what keeps two gates from disagreeing.
//
// driver is that session's holder identity, recorded so an admitted gate is
// attributable. An empty driver is a mis-wiring: it leaves the gate closed and
// returns [ErrAdmissionDriverRequired] rather than admitting dispatch silently.
func (r *Registry) OpenRuntimeDispatch(driver string) error {
	if driver == "" {
		return ErrAdmissionDriverRequired
	}
	r.admission.Store(&admissionState{open: true, driver: driver})
	r.logger.Info("runtime dispatch admitted", "driver", driver)
	return nil
}

// CloseRuntimeDispatch bars runtime dispatch and records why (leadership loss,
// drain, configuration). Unlike admitting it, closing needs no driver identity:
// barring dispatch is the safe direction and must never be refused. Published
// snapshots are kept so evidence stays attributable; built-in dispatch is
// unaffected.
func (r *Registry) CloseRuntimeDispatch(reason string) {
	if reason == "" {
		reason = "runtime dispatch is not admitted"
	}
	previous := r.admission.Swap(&admissionState{reason: reason})
	if previous != nil && previous.open {
		r.logger.Info("runtime dispatch revoked", "driver", previous.driver, "reason", reason)
	}
}

// Resolve selects the adapter that may serve addonType and reports the
// revision and provenance a runtime candidate was published with, so a caller
// can fence publication against the exact snapshot it dispatched to.
//
// It returns a *[DispatchBarrier] when the identity is contested or runtime
// dispatch is not admitted, and [ErrNotFound] when nobody claims it.
func (r *Registry) Resolve(addonType string) (Resolution, error) {
	if claim, reserved := (*r.startup.Load())[addonType]; reserved {
		if builtin, ok := r.builtin(addonType); ok {
			if claim.uid == "" {
				return Resolution{}, &DispatchBarrier{AddonType: addonType, Reason: ReasonAdmissionClosed,
					Claimants: []string{fmt.Sprintf("built-in adapter %q", builtin.Name())}, Detail: "startup definition inventory has not been read"}
			}
			return Resolution{}, &DispatchBarrier{AddonType: addonType, Reason: ReasonBuiltinCollision,
				Claimants: []string{fmt.Sprintf("built-in adapter %q", builtin.Name()), fmt.Sprintf("stored runtime definition %q", claim.uid)}}
		}
	}
	snap := r.runtime.Load()
	if barrier := r.barrierFor(snap, addonType); barrier != nil {
		return Resolution{}, barrier
	}
	if builtin, ok := r.builtin(addonType); ok {
		return Resolution{Adapter: builtin}, nil
	}
	owner := snap.byAddon[addonType]
	if owner == nil {
		return Resolution{}, fmt.Errorf("%w: %q", ErrNotFound, addonType)
	}
	if state := r.admission.Load(); !state.open {
		return Resolution{}, &DispatchBarrier{
			AddonType: addonType,
			Reason:    ReasonAdmissionClosed,
			Claimants: []string{owner.describe()},
			Detail:    state.reason,
		}
	}
	return Resolution{
		Adapter:    owner.entry.Adapter,
		Runtime:    true,
		Revision:   owner.entry.Revision,
		Provenance: owner.entry.Provenance,
	}, nil
}

// barrierFor reports the collision barrier covering addonType within snap, if
// any. Collisions are evaluated against live built-in registration rather than
// baked into the snapshot, so a built-in registered by a later upgrade raises
// the barrier without any runtime write. The barrier holds even while runtime
// dispatch is closed: only explicit migration or deletion resolves it.
func (r *Registry) barrierFor(snap *runtimeSnapshot, addonType string) *DispatchBarrier {
	builtin, hasBuiltin := r.builtin(addonType)
	if claimants := snap.barred[addonType]; len(claimants) > 0 {
		claimants = append([]string(nil), claimants...)
		if hasBuiltin {
			claimants = append(claimants, fmt.Sprintf("built-in adapter %q", builtin.Name()))
		}
		return &DispatchBarrier{AddonType: addonType, Reason: ReasonRuntimeCollision, Claimants: claimants}
	}
	owner := snap.byAddon[addonType]
	if owner == nil || !hasBuiltin {
		return nil
	}
	return &DispatchBarrier{
		AddonType: addonType,
		Reason:    ReasonBuiltinCollision,
		Claimants: []string{fmt.Sprintf("built-in adapter %q", builtin.Name()), owner.describe()},
	}
}
