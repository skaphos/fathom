/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package registry_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"k8s.io/apimachinery/pkg/types"

	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/pkg/adapter"
)

// versionedFake tags the adapter's own Version with the generation it was
// compiled from, so a dispatcher can prove the adapter, revision and
// provenance it received all came from the same published snapshot.
func versionedFake(name, version string, addonTypes ...string) fakeAdapter {
	a := newFake(name, addonTypes...)
	a.version = version
	return a
}

// gatedAdapter parks inside Capabilities until released. SetRuntime reads
// capabilities while building the replacement snapshot, so parking there
// parks construction itself — which is only harmless if construction holds
// neither the built-in lock nor a lock other writers need.
type gatedAdapter struct {
	fakeAdapter
	entered chan struct{}
	release chan struct{}
}

func (g gatedAdapter) Capabilities() adapter.Capabilities {
	select {
	case g.entered <- struct{}{}:
	default:
	}
	<-g.release
	return g.fakeAdapter.Capabilities()
}

func generationVersion(generation int64) string { return fmt.Sprintf("0.%d.0", generation) }

// runtimeEntry builds a complete, valid entry: every revision and provenance
// field populated, adapter version mirroring the declared adapterVersion.
func runtimeEntry(uid string, generation int64, addonTypes ...string) registry.RuntimeEntry {
	version := generationVersion(generation)
	return registry.RuntimeEntry{
		Adapter: versionedFake("adapter-"+uid, version, addonTypes...),
		Revision: registry.RuntimeRevision{
			DefinitionUID:    types.UID(uid),
			Generation:       generation,
			SchemaVersion:    "fathom.skaphos.io/v1alpha1",
			SemanticsVersion: 1,
		},
		Provenance: registry.RuntimeProvenance{
			OperatorBuild:  "v0.0.0-test",
			AdapterVersion: version,
		},
	}
}

func mustSetRuntime(t *testing.T, r *registry.Registry, entry registry.RuntimeEntry) {
	t.Helper()
	if err := r.SetRuntime(entry); err != nil {
		t.Fatalf("SetRuntime(%s): unexpected error: %v", entry.Revision.DefinitionUID, err)
	}
}

// testDriver is the leadership-session identity the tests admit dispatch with.
// The registry never interprets it; it only refuses to open the gate without
// one, so every admitting test has to name a driver exactly as T047 must.
const testDriver = "fathom-0/00000000-0000-0000-0000-000000000000"

// admitted returns a registry whose runtime dispatch barrier is open, which is
// what a synchronized, validated leader session does.
func admittedRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	r := registry.New(logr.Discard())
	if err := r.OpenRuntimeDispatch(testDriver); err != nil {
		t.Fatalf("OpenRuntimeDispatch(%q): %v", testDriver, err)
	}
	return r
}

func TestStartupClaimsFenceBuiltinsUntilCurrentDefinitionIsReconciled(t *testing.T) {
	r := registry.New(logr.Discard())
	for _, name := range []string{"coredns", "cert-manager"} {
		if err := r.Register(newFake(name, name)); err != nil {
			t.Fatal(err)
		}
	}
	r.ReserveStartupClaims([]string{"coredns", "cert-manager"})
	barrier := func(name, reason string) {
		t.Helper()
		_, err := r.Resolve(name)
		var b *registry.DispatchBarrier
		if !errors.As(err, &b) || b.Reason != reason {
			t.Fatalf("resolve %q: barrier %v, want %s", name, err, reason)
		}
	}
	barrier("coredns", registry.ReasonAdmissionClosed)
	barrier("cert-manager", registry.ReasonAdmissionClosed)
	r.ReleaseStartupClaim("cert-manager", "cached-uid", 1)
	barrier("cert-manager", registry.ReasonAdmissionClosed)
	r.ObserveStartupClaim("cert-manager", "recreated-uid", 1)
	barrier("cert-manager", registry.ReasonBuiltinCollision)
	r.ReleaseStartupClaim("cert-manager", "cached-uid", 1)
	barrier("cert-manager", registry.ReasonBuiltinCollision)
	r.ObserveStartupClaim("coredns", "stored-uid", 1)
	r.ObserveStartupClaim("cert-manager", "", 0) // Direct GET found no definition.
	barrier("coredns", registry.ReasonBuiltinCollision)
	if _, err := r.Resolve("cert-manager"); err != nil {
		t.Fatalf("unrelated builtin remains blocked after a successful not-found read: %v", err)
	}
	r.ReleaseStartupClaim("coredns", "stale-uid", 1)
	barrier("coredns", registry.ReasonBuiltinCollision)
	r.ObserveStartupClaim("coredns", "recreated-uid", 1)
	r.ReleaseStartupClaim("coredns", "stored-uid", 1)
	barrier("coredns", registry.ReasonBuiltinCollision)
	r.ReleaseStartupClaim("coredns", "recreated-uid", 1)
	if _, err := r.Resolve("coredns"); err != nil {
		t.Fatalf("current reconciliation did not release temporary startup reservation: %v", err)
	}
	r.ObserveStartupClaim("coredns", "stale-read-uid", 1)
	if _, err := r.Resolve("coredns"); err != nil {
		t.Fatalf("late direct GET resurrected a cleared reservation: %v", err)
	}
}

func TestStartupUnknownClaimNeedsDirectConfirmationAfterCachedReconcile(t *testing.T) {
	r := registry.New(logr.Discard())
	if err := r.Register(newFake("builtin", "coredns")); err != nil {
		t.Fatal(err)
	}
	r.ReserveStartupClaims([]string{"coredns"})
	r.ReleaseStartupClaim("coredns", "cached-uid", 1)
	if _, err := r.Resolve("coredns"); !errors.Is(err, registry.ErrDispatchBarred) {
		t.Fatalf("cached reconciliation released an unverified startup claim: %v", err)
	}
	r.ObserveStartupClaim("coredns", "cached-uid", 1)
	if _, err := r.Resolve("coredns"); err != nil {
		t.Fatalf("direct confirmation of reconciled UID did not release claim: %v", err)
	}
}

func TestStartupClaimRejectsStaleGenerationReconciliation(t *testing.T) {
	r := registry.New(logr.Discard())
	if err := r.Register(newFake("builtin", "coredns")); err != nil {
		t.Fatal(err)
	}
	r.ReserveStartupClaims([]string{"coredns"})
	r.ReleaseStartupClaim("coredns", "same-uid", 1)
	r.ObserveStartupClaim("coredns", "same-uid", 2)
	r.ReleaseStartupClaim("coredns", "same-uid", 1)
	if _, err := r.Resolve("coredns"); !errors.Is(err, registry.ErrDispatchBarred) {
		t.Fatalf("old generation cleared a new-generation startup reservation: %v", err)
	}
	r.ReleaseStartupClaim("coredns", "same-uid", 2)
	if _, err := r.Resolve("coredns"); err != nil {
		t.Fatalf("current generation failed to release reservation: %v", err)
	}
}

func TestStartupClaimRecoversWhenNewRevisionFinishesBeforeDirectRead(t *testing.T) {
	for _, tc := range []struct {
		name           string
		oldUID, newUID types.UID
		oldGen, newGen int64
	}{
		{name: "recreated UID", oldUID: "old", newUID: "new", oldGen: 1, newGen: 1},
		{name: "edited generation", oldUID: "same", newUID: "same", oldGen: 1, newGen: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := registry.New(logr.Discard())
			if err := r.Register(newFake("builtin", "coredns")); err != nil {
				t.Fatal(err)
			}
			r.ReserveStartupClaims([]string{"coredns"})
			r.ObserveStartupClaim("coredns", tc.oldUID, tc.oldGen)
			// The cache sees the new revision before the next direct GET.
			r.ReleaseStartupClaim("coredns", tc.newUID, tc.newGen)
			// An in-flight old read must not erase the completed revision.
			r.ObserveStartupClaim("coredns", tc.oldUID, tc.oldGen)
			if _, err := r.Resolve("coredns"); !errors.Is(err, registry.ErrDispatchBarred) {
				t.Fatalf("old observation cleared reservation early: %v", err)
			}
			r.ObserveStartupClaim("coredns", tc.newUID, tc.newGen)
			if _, err := r.Resolve("coredns"); err != nil {
				t.Fatalf("confirmed completed revision stayed barred forever: %v", err)
			}
		})
	}
}

// A runtime entry may only be published with a complete identity: the full
// (definition UID, generation, schema, semantics) revision plus the
// operator-build/adapterVersion provenance recorded at publication. Anything
// missing fails closed and publishes nothing.
func TestSetRuntimeRejectsIncompleteEntries(t *testing.T) {
	t.Parallel()

	complete := func(mutate func(*registry.RuntimeEntry)) registry.RuntimeEntry {
		e := runtimeEntry("uid-a", 3, "custom-addon")
		mutate(&e)
		return e
	}

	tests := []struct {
		name        string
		entry       registry.RuntimeEntry
		errContains string
	}{
		{
			name:        "nil adapter",
			entry:       complete(func(e *registry.RuntimeEntry) { e.Adapter = nil }),
			errContains: "nil adapter",
		},
		{
			name: "no addon types",
			entry: complete(func(e *registry.RuntimeEntry) {
				e.Adapter = versionedFake("adapter-uid-a", "0.3.0")
			}),
			errContains: "advertises no addon types",
		},
		{
			name: "incompatible contract version",
			entry: complete(func(e *registry.RuntimeEntry) {
				a := versionedFake("adapter-uid-a", "0.3.0", "custom-addon")
				a.contractVersion = "2.0.0"
				e.Adapter = a
			}),
			errContains: "incompatible",
		},
		{
			name: "empty addon type advertised",
			entry: complete(func(e *registry.RuntimeEntry) {
				e.Adapter = versionedFake("adapter-uid-a", "0.3.0", "custom-addon", "")
			}),
			errContains: "empty addon type",
		},
		{
			name: "duplicate addon type advertised",
			entry: complete(func(e *registry.RuntimeEntry) {
				e.Adapter = versionedFake("adapter-uid-a", "0.3.0", "custom-addon", "custom-addon")
			}),
			errContains: `advertises addon type "custom-addon" twice`,
		},
		{
			name:        "missing definition UID",
			entry:       complete(func(e *registry.RuntimeEntry) { e.Revision.DefinitionUID = "" }),
			errContains: "definition UID",
		},
		{
			name:        "missing generation",
			entry:       complete(func(e *registry.RuntimeEntry) { e.Revision.Generation = 0 }),
			errContains: "generation",
		},
		{
			name:        "missing schema version",
			entry:       complete(func(e *registry.RuntimeEntry) { e.Revision.SchemaVersion = "" }),
			errContains: "schema version",
		},
		{
			name:        "missing semantics version",
			entry:       complete(func(e *registry.RuntimeEntry) { e.Revision.SemanticsVersion = 0 }),
			errContains: "semantics version",
		},
		{
			name:        "missing operator build",
			entry:       complete(func(e *registry.RuntimeEntry) { e.Provenance.OperatorBuild = "" }),
			errContains: "operator build",
		},
		{
			name:        "missing adapter version",
			entry:       complete(func(e *registry.RuntimeEntry) { e.Provenance.AdapterVersion = "" }),
			errContains: "adapterVersion",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := admittedRegistry(t)
			err := r.SetRuntime(tc.entry)
			if err == nil {
				t.Fatalf("SetRuntime: want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("SetRuntime: error %q does not contain %q", err.Error(), tc.errContains)
			}
			// Fail closed: a rejected entry publishes nothing at all.
			if _, err := r.Resolve("custom-addon"); !errors.Is(err, registry.ErrNotFound) {
				t.Fatalf("Resolve after rejected SetRuntime: want ErrNotFound, got %v", err)
			}
			if got := len(r.RuntimeEntries()); got != 0 {
				t.Fatalf("RuntimeEntries after rejected SetRuntime: want 0, got %d", got)
			}
		})
	}

	// The same entry with every field populated must be accepted, otherwise
	// the cases above could pass for the wrong reason.
	r := admittedRegistry(t)
	mustSetRuntime(t, r, runtimeEntry("uid-a", 3, "custom-addon"))
	res, err := r.Resolve("custom-addon")
	if err != nil {
		t.Fatalf("Resolve(custom-addon): %v", err)
	}
	if !res.Runtime {
		t.Fatalf("Resolve(custom-addon): want a runtime resolution, got built-in")
	}
	want := registry.RuntimeRevision{
		DefinitionUID:    "uid-a",
		Generation:       3,
		SchemaVersion:    "fathom.skaphos.io/v1alpha1",
		SemanticsVersion: 1,
	}
	if res.Revision != want {
		t.Fatalf("Resolve revision: got %+v, want %+v", res.Revision, want)
	}
	if res.Provenance.OperatorBuild != "v0.0.0-test" || res.Provenance.AdapterVersion != "0.3.0" {
		t.Fatalf("Resolve provenance: got %+v", res.Provenance)
	}
}

// Replacement is owner aware: a new revision from the same definition UID
// replaces the previous snapshot wholesale, including addon types the new
// revision no longer claims.
func TestSetRuntimeReplacementIsOwnerAware(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon", "legacy-addon"))
	mustSetRuntime(t, r, runtimeEntry("uid-a", 2, "custom-addon"))

	if got := len(r.RuntimeEntries()); got != 1 {
		t.Fatalf("RuntimeEntries after replacement: want 1 owner, got %d", got)
	}
	res, err := r.Resolve("custom-addon")
	if err != nil {
		t.Fatalf("Resolve(custom-addon): %v", err)
	}
	if res.Revision.Generation != 2 {
		t.Fatalf("Resolve(custom-addon): want generation 2, got %d", res.Revision.Generation)
	}
	if got := res.Adapter.Version(); got != generationVersion(2) {
		t.Fatalf("Resolve(custom-addon): adapter %q does not belong to the published revision", got)
	}
	// The dropped addon type must stop dispatching.
	if _, err := r.Resolve("legacy-addon"); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("Resolve(legacy-addon) after replacement: want ErrNotFound, got %v", err)
	}

	// A late event carrying an older generation for the same owner must not
	// regress the published revision.
	err = r.SetRuntime(runtimeEntry("uid-a", 1, "custom-addon"))
	if err == nil {
		t.Fatalf("SetRuntime with stale generation: want error, got nil")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Fatalf("SetRuntime with stale generation: error %q should name staleness", err.Error())
	}
	res, err = r.Resolve("custom-addon")
	if err != nil {
		t.Fatalf("Resolve after stale SetRuntime: %v", err)
	}
	if res.Revision.Generation != 2 {
		t.Fatalf("Resolve after stale SetRuntime: want generation 2, got %d", res.Revision.Generation)
	}
}

// A delete event for an old UID must not remove the replacement that a
// recreated definition published under a new UID.
func TestRemoveRuntimeOnlyRemovesMatchingUID(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	mustSetRuntime(t, r, runtimeEntry("uid-new", 1, "custom-addon"))

	if removed := r.RemoveRuntime("uid-old"); removed {
		t.Fatalf("RemoveRuntime(uid-old): want false, got true")
	}
	res, err := r.Resolve("custom-addon")
	if err != nil {
		t.Fatalf("Resolve after unmatched RemoveRuntime: %v", err)
	}
	if res.Revision.DefinitionUID != "uid-new" {
		t.Fatalf("Resolve after unmatched RemoveRuntime: owner is %q", res.Revision.DefinitionUID)
	}

	if removed := r.RemoveRuntime("uid-new"); !removed {
		t.Fatalf("RemoveRuntime(uid-new): want true, got false")
	}
	if _, err := r.Resolve("custom-addon"); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("Resolve after matched RemoveRuntime: want ErrNotFound, got %v", err)
	}
	if got := len(r.RuntimeEntries()); got != 0 {
		t.Fatalf("RuntimeEntries after matched RemoveRuntime: want 0, got %d", got)
	}
}

// Anything handed out is an immutable snapshot: neither a caller's mutation
// nor a later replacement may change what an earlier holder observes.
func TestRuntimeSnapshotHandedOutIsImmutable(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon"))

	before := r.RuntimeEntries()
	if len(before) != 1 || len(before[0].AddonTypes) != 1 {
		t.Fatalf("RuntimeEntries: got %+v", before)
	}
	held, err := r.Resolve("custom-addon")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// A caller mutating its copy must not reach registry state.
	before[0].AddonTypes[0] = "mutated"
	before[0].Revision.Generation = 99
	if got := r.RuntimeEntries(); got[0].AddonTypes[0] != "custom-addon" || got[0].Revision.Generation != 1 {
		t.Fatalf("registry state aliased a handed-out snapshot: %+v", got[0])
	}

	// A replacement must not retroactively change the earlier snapshot.
	mustSetRuntime(t, r, runtimeEntry("uid-a", 2, "custom-addon"))
	r.RemoveRuntime("uid-a")
	if held.Revision.Generation != 1 || held.Provenance.AdapterVersion != generationVersion(1) {
		t.Fatalf("held resolution mutated under replacement: %+v", held)
	}
	if held.Adapter.Version() != generationVersion(1) {
		t.Fatalf("held adapter mutated under replacement: %q", held.Adapter.Version())
	}
	snapshot := r.RuntimeEntries()
	if len(snapshot) != 0 {
		t.Fatalf("RuntimeEntries after removal: want 0, got %d", len(snapshot))
	}
}

// A built-in newly claiming an identity a runtime definition already owns is a
// conflict barrier: neither candidate dispatches until an administrator
// resolves it, while unrelated built-ins keep working.
func TestRuntimeCollisionWithBuiltinSuppressesBothCandidates(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "cert-manager"))
	if _, err := r.Resolve("cert-manager"); err != nil {
		t.Fatalf("Resolve before the built-in upgrade: %v", err)
	}

	// Built-in registration semantics do not change: the upgrade still
	// registers successfully, it only raises a dispatch barrier.
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("Register built-in over a runtime identity: %v", err)
	}
	if err := r.Register(newFake("dns", "coredns")); err != nil {
		t.Fatalf("Register unrelated built-in: %v", err)
	}

	for _, probe := range []string{"Resolve", "Lookup"} {
		var err error
		if probe == "Resolve" {
			_, err = r.Resolve("cert-manager")
		} else {
			_, err = r.Lookup("cert-manager")
		}
		if !errors.Is(err, registry.ErrDispatchBarred) {
			t.Fatalf("%s(cert-manager): want a dispatch barrier, got %v", probe, err)
		}
		var barrier *registry.DispatchBarrier
		if !errors.As(err, &barrier) {
			t.Fatalf("%s(cert-manager): want *DispatchBarrier, got %T", probe, err)
		}
		if barrier.Reason != registry.ReasonBuiltinCollision {
			t.Fatalf("%s(cert-manager): reason %q, want %q", probe, barrier.Reason, registry.ReasonBuiltinCollision)
		}
		if len(barrier.Claimants) != 2 {
			t.Fatalf("%s(cert-manager): want both claimants named, got %v", probe, barrier.Claimants)
		}
	}

	// Unrelated built-ins remain available.
	if _, err := r.Lookup("coredns"); err != nil {
		t.Fatalf("Lookup(coredns) under an unrelated barrier: %v", err)
	}

	// Closing runtime admission must not silently hand the identity back to
	// the built-in: only explicit migration or deletion resolves the barrier.
	r.CloseRuntimeDispatch("leadership lost")
	if _, err := r.Resolve("cert-manager"); !errors.Is(err, registry.ErrDispatchBarred) {
		t.Fatalf("Resolve(cert-manager) with admission closed: want the barrier to hold, got %v", err)
	}

	// Deleting the runtime definition resolves it.
	if removed := r.RemoveRuntime("uid-a"); !removed {
		t.Fatalf("RemoveRuntime(uid-a): want true")
	}
	res, err := r.Resolve("cert-manager")
	if err != nil {
		t.Fatalf("Resolve(cert-manager) after the barrier is resolved: %v", err)
	}
	if res.Runtime {
		t.Fatalf("Resolve(cert-manager): want the built-in, got a runtime resolution")
	}
}

// Two runtime owners claiming one identity is prohibited by name uniqueness;
// a legacy collision must fail closed with no winner decided by arrival order.
func TestTwoRuntimeOwnersFailClosedWithoutArrivalOrderWinner(t *testing.T) {
	t.Parallel()

	orders := [][]string{{"uid-a", "uid-b"}, {"uid-b", "uid-a"}}
	for _, order := range orders {
		order := order
		t.Run(strings.Join(order, "-then-"), func(t *testing.T) {
			t.Parallel()
			r := admittedRegistry(t)
			mustSetRuntime(t, r, runtimeEntry(order[0], 1, "custom-addon"))
			err := r.SetRuntime(runtimeEntry(order[1], 1, "custom-addon"))
			if err == nil {
				t.Fatalf("second SetRuntime: want a collision error, got nil")
			}
			var barrier *registry.DispatchBarrier
			if !errors.As(err, &barrier) || barrier.Reason != registry.ReasonRuntimeCollision {
				t.Fatalf("second SetRuntime: want a RuntimeCollision barrier, got %v", err)
			}

			if _, err := r.Resolve("custom-addon"); !errors.Is(err, registry.ErrDispatchBarred) {
				t.Fatalf("Resolve under a runtime collision: want the barrier, got %v", err)
			}
			if _, err := r.Lookup("custom-addon"); !errors.Is(err, registry.ErrDispatchBarred) {
				t.Fatalf("Lookup under a runtime collision: want the barrier, got %v", err)
			}

			// Removing the arriving owner restores the survivor; the point is
			// that resolution follows ownership, never arrival order.
			if removed := r.RemoveRuntime(types.UID(order[1])); !removed {
				t.Fatalf("RemoveRuntime(%s): want true", order[1])
			}
			res, err := r.Resolve("custom-addon")
			if err != nil {
				t.Fatalf("Resolve after the collision is resolved: %v", err)
			}
			if string(res.Revision.DefinitionUID) != order[0] {
				t.Fatalf("Resolve: owner %q, want %q", res.Revision.DefinitionUID, order[0])
			}
		})
	}
}

// Runtime dispatch is barred until the caller has synchronized and validated,
// and is barred again when leadership or admission is withdrawn. Built-in
// dispatch is unaffected in every state.
func TestRuntimeDispatchBarrierClosedUntilAdmitted(t *testing.T) {
	t.Parallel()

	r := registry.New(logr.Discard())
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon"))

	err := mustBarred(t, r, "custom-addon")
	if err.Reason != registry.ReasonAdmissionClosed {
		t.Fatalf("Resolve before admission: reason %q, want %q", err.Reason, registry.ReasonAdmissionClosed)
	}
	if _, err := r.Lookup("cert-manager"); err != nil {
		t.Fatalf("built-in dispatch must be unaffected before admission: %v", err)
	}

	if err := r.OpenRuntimeDispatch(testDriver); err != nil {
		t.Fatalf("OpenRuntimeDispatch(%q): %v", testDriver, err)
	}
	if _, err := r.Resolve("custom-addon"); err != nil {
		t.Fatalf("Resolve after OpenRuntimeDispatch: %v", err)
	}

	r.CloseRuntimeDispatch("leadership lost")
	barrier := mustBarred(t, r, "custom-addon")
	if barrier.Reason != registry.ReasonAdmissionClosed {
		t.Fatalf("Resolve after CloseRuntimeDispatch: reason %q", barrier.Reason)
	}
	if !strings.Contains(barrier.Error(), "leadership lost") {
		t.Fatalf("Resolve after CloseRuntimeDispatch: error %q should carry the reason", barrier.Error())
	}
	if _, err := r.Lookup("cert-manager"); err != nil {
		t.Fatalf("built-in dispatch must be unaffected after closing admission: %v", err)
	}
	// An unknown identity is still simply unknown, not barred.
	if _, err := r.Resolve("nobody"); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("Resolve(nobody): want ErrNotFound, got %v", err)
	}
}

func mustBarred(t *testing.T, r *registry.Registry, addonType string) *registry.DispatchBarrier {
	t.Helper()
	_, err := r.Resolve(addonType)
	var barrier *registry.DispatchBarrier
	if !errors.As(err, &barrier) {
		t.Fatalf("Resolve(%s): want *DispatchBarrier, got %v", addonType, err)
	}
	if !errors.Is(err, registry.ErrDispatchBarred) {
		t.Fatalf("Resolve(%s): barrier must match ErrDispatchBarred", addonType)
	}
	if errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("Resolve(%s): a barrier must not also match ErrNotFound; callers"+
			" branch on that to tell a contested identity from an unknown one", addonType)
	}
	return barrier
}

// Built-in registration, lookup and capability reporting must behave exactly as
// they did before runtime snapshots existed.
func TestBuiltinSemanticsUnchangedByRuntimeEntries(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon"))
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Duplicate built-in registration is still decided among built-ins only.
	if err := r.Register(newFake("cert-manager-fork", "cert-manager")); err == nil {
		t.Fatalf("duplicate built-in Register: want error, got nil")
	}
	// A built-in may not claim a runtime identity through the built-in path
	// either; it still succeeds as registration and becomes a barrier.
	if err := r.Register(newFake("custom-addon-builtin", "custom-addon")); err != nil {
		t.Fatalf("Register built-in over a runtime identity: %v", err)
	}

	got, err := r.Lookup("cert-manager")
	if err != nil {
		t.Fatalf("Lookup(cert-manager): %v", err)
	}
	if got.Name() != "cert-manager" {
		t.Fatalf("Lookup(cert-manager): got %q", got.Name())
	}
	// Capabilities reports built-ins; runtime snapshots have their own
	// accessor and must not leak into the built-in inventory.
	caps := r.Capabilities()
	if _, ok := caps["adapter-uid-a"]; ok {
		t.Fatalf("Capabilities leaked a runtime adapter: %v", caps)
	}
	if len(caps) != 2 {
		t.Fatalf("Capabilities: want 2 built-ins, got %d (%v)", len(caps), caps)
	}
}

// Snapshot construction must happen off-lock: while one replacement is parked
// mid-construction, dispatch must continue and an unrelated owner must still
// be able to publish.
func TestRuntimeConstructionDoesNotBlockDispatch(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon"))

	gated := runtimeEntry("uid-gated", 1, "gated-addon")
	gate := gatedAdapter{
		fakeAdapter: versionedFake("adapter-uid-gated", "0.1.0", "gated-addon"),
		entered:     make(chan struct{}, 4),
		release:     make(chan struct{}),
	}
	gated.Adapter = gate

	setDone := make(chan error, 1)
	go func() { setDone <- r.SetRuntime(gated) }()

	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("SetRuntime never reached capability construction")
	}

	progress := make(chan error, 3)
	go func() {
		_, err := r.Lookup("cert-manager")
		progress <- err
	}()
	go func() {
		_, err := r.Resolve("custom-addon")
		progress <- err
	}()
	go func() { progress <- r.SetRuntime(runtimeEntry("uid-b", 1, "other-addon")) }()
	for i := 0; i < 3; i++ {
		select {
		case err := <-progress:
			if err != nil {
				t.Fatalf("progress during parked construction: %v", err)
			}
		case <-time.After(5 * time.Second):
			close(gate.release)
			t.Fatalf("dispatch or an unrelated publication blocked on parked snapshot construction")
		}
	}

	close(gate.release)
	if err := <-setDone; err != nil {
		t.Fatalf("parked SetRuntime: %v", err)
	}
	if _, err := r.Resolve("gated-addon"); err != nil {
		t.Fatalf("Resolve(gated-addon) after release: %v", err)
	}
}

// Replacement races dispatch: every resolution must observe one self-consistent
// published revision, never a torn mixture of two, and a snapshot taken before
// a replacement must keep its contents.
func TestConcurrentReplacementAndDispatch(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon"))

	held := r.RuntimeEntries()
	heldTypes := append([]string(nil), held[0].AddonTypes...)
	heldRevision := held[0].Revision

	const generations = 200
	const readers = 8
	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(stop)
		for gen := int64(2); gen <= generations; gen++ {
			if err := r.SetRuntime(runtimeEntry("uid-a", gen, "custom-addon")); err != nil {
				t.Errorf("concurrent SetRuntime(gen %d): %v", gen, err)
				return
			}
			if gen%25 == 0 {
				// A delete event for a foreign UID must never disturb the owner.
				r.RemoveRuntime("uid-stale")
			}
		}
	}()

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				res, err := r.Resolve("custom-addon")
				if err != nil {
					t.Errorf("concurrent Resolve: %v", err)
					return
				}
				want := generationVersion(res.Revision.Generation)
				if res.Adapter.Version() != want || res.Provenance.AdapterVersion != want {
					t.Errorf("torn snapshot: revision %+v, adapter %q, provenance %+v",
						res.Revision, res.Adapter.Version(), res.Provenance)
					return
				}
				if res.Revision.DefinitionUID != "uid-a" || res.Revision.SemanticsVersion != 1 {
					t.Errorf("torn revision: %+v", res.Revision)
					return
				}
				if _, err := r.Lookup("cert-manager"); err != nil {
					t.Errorf("built-in dispatch during replacement: %v", err)
					return
				}
				for _, e := range r.RuntimeEntries() {
					if len(e.AddonTypes) != 1 || e.AddonTypes[0] != "custom-addon" {
						t.Errorf("torn entry snapshot: %+v", e)
						return
					}
				}
			}
		}()
	}
	wg.Wait()

	if len(held) != 1 || held[0].Revision != heldRevision || strings.Join(held[0].AddonTypes, ",") != strings.Join(heldTypes, ",") {
		t.Fatalf("snapshot taken before the replacements mutated: %+v", held)
	}
	res, err := r.Resolve("custom-addon")
	if err != nil {
		t.Fatalf("final Resolve: %v", err)
	}
	if res.Revision.Generation != generations {
		t.Fatalf("final Resolve: generation %d, want %d", res.Revision.Generation, generations)
	}
}

// The ordinary install order is the reverse of an upgrade: built-ins register
// at process start and definitions publish later. Publishing onto an identity a
// built-in already holds must therefore report the BuiltinCollision barrier
// from SetRuntime itself, not only from a later Resolve.
func TestSetRuntimePublishedOverAnExistingBuiltinReportsTheBarrier(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	err := r.SetRuntime(runtimeEntry("uid-a", 1, "cert-manager"))
	if err == nil {
		t.Fatalf("SetRuntime onto a registered built-in: want a collision barrier, got nil")
	}
	var barrier *registry.DispatchBarrier
	if !errors.As(err, &barrier) {
		t.Fatalf("SetRuntime onto a registered built-in: want *DispatchBarrier, got %T (%v)", err, err)
	}
	if barrier.Reason != registry.ReasonBuiltinCollision {
		t.Fatalf("SetRuntime onto a registered built-in: reason %q, want %q",
			barrier.Reason, registry.ReasonBuiltinCollision)
	}
	if len(barrier.Claimants) != 2 ||
		!strings.Contains(barrier.Claimants[0], `built-in adapter "cert-manager"`) ||
		!strings.Contains(barrier.Claimants[1], `"uid-a"`) {
		t.Fatalf("SetRuntime barrier must name the built-in then the definition, got %v", barrier.Claimants)
	}

	// The entry is published but suppressed: dropping it would let the built-in
	// silently reinterpret the identity with no evidence of the conflict.
	if got := len(r.RuntimeEntries()); got != 1 {
		t.Fatalf("RuntimeEntries after a barred publication: want the entry retained, got %d", got)
	}
	for _, probe := range []string{"Resolve", "Lookup"} {
		var perr error
		if probe == "Resolve" {
			_, perr = r.Resolve("cert-manager")
		} else {
			_, perr = r.Lookup("cert-manager")
		}
		if !errors.Is(perr, registry.ErrDispatchBarred) {
			t.Fatalf("%s(cert-manager) after a barred publication: want the barrier, got %v", probe, perr)
		}
	}

	// Deleting the definition — an administrator's explicit resolution — hands
	// the identity back to the built-in.
	if removed := r.RemoveRuntime("uid-a"); !removed {
		t.Fatalf("RemoveRuntime(uid-a): want true")
	}
	res, err := r.Resolve("cert-manager")
	if err != nil {
		t.Fatalf("Resolve(cert-manager) after the barrier is resolved: %v", err)
	}
	if res.Runtime {
		t.Fatalf("Resolve(cert-manager): want the built-in, got a runtime resolution")
	}
}

// Staleness is decided strictly: only an older generation is refused. An equal
// generation republished under a new schema, semantics or operator build is a
// recompile of the same definition revision and must replace what it
// supersedes, otherwise an operator upgrade would pin dispatch to the
// compilation the previous build produced.
func TestSetRuntimeEqualGenerationRepublishesTheRecompile(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	mustSetRuntime(t, r, runtimeEntry("uid-a", 2, "custom-addon"))

	recompiled := runtimeEntry("uid-a", 2, "custom-addon")
	recompiled.Adapter = versionedFake("adapter-uid-a", "9.9.9", "custom-addon")
	recompiled.Revision.SemanticsVersion = 2
	recompiled.Provenance = registry.RuntimeProvenance{
		OperatorBuild:  "v0.2.0-upgrade",
		AdapterVersion: "9.9.9",
	}
	if err := r.SetRuntime(recompiled); err != nil {
		t.Fatalf("SetRuntime republishing generation 2: want acceptance, got %v", err)
	}

	res, err := r.Resolve("custom-addon")
	if err != nil {
		t.Fatalf("Resolve after republication: %v", err)
	}
	if res.Revision.SemanticsVersion != 2 {
		t.Fatalf("Resolve after republication: semantics version %d, want the recompiled 2",
			res.Revision.SemanticsVersion)
	}
	if res.Provenance.OperatorBuild != "v0.2.0-upgrade" || res.Provenance.AdapterVersion != "9.9.9" {
		t.Fatalf("Resolve after republication: provenance %+v, want the recompiled build", res.Provenance)
	}
	if got := res.Adapter.Version(); got != "9.9.9" {
		t.Fatalf("Resolve after republication: adapter %q, want the recompiled adapter", got)
	}
	if got := len(r.RuntimeEntries()); got != 1 {
		t.Fatalf("RuntimeEntries after republication: want 1 owner, got %d", got)
	}

	// The boundary still holds on the other side: generation 1 is stale.
	if err := r.SetRuntime(runtimeEntry("uid-a", 1, "custom-addon")); err == nil ||
		!strings.Contains(err.Error(), "stale") {
		t.Fatalf("SetRuntime with an older generation: want a staleness error, got %v", err)
	}
}

// The published addon-type set is derived from the adapter's capabilities; a
// value on the caller's RuntimeEntry is ignored. Honouring it would let a
// controller route an identity to an adapter that never advertised it.
func TestSetRuntimeIgnoresCallerSuppliedAddonTypes(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	entry := runtimeEntry("uid-a", 1, "custom-addon")
	entry.AddonTypes = []string{"caller-supplied", "also-not-advertised"}
	mustSetRuntime(t, r, entry)

	entries := r.RuntimeEntries()
	if len(entries) != 1 {
		t.Fatalf("RuntimeEntries: want 1 owner, got %d", len(entries))
	}
	if len(entries[0].AddonTypes) != 1 || entries[0].AddonTypes[0] != "custom-addon" {
		t.Fatalf("published addon types %v, want only the adapter's capabilities", entries[0].AddonTypes)
	}
	if _, err := r.Resolve("custom-addon"); err != nil {
		t.Fatalf("Resolve(custom-addon): the advertised identity must dispatch: %v", err)
	}
	for _, claimed := range entry.AddonTypes {
		if _, err := r.Resolve(claimed); !errors.Is(err, registry.ErrNotFound) {
			t.Fatalf("Resolve(%s): a caller-supplied addon type must not dispatch, got %v", claimed, err)
		}
	}
}

// RuntimeEntries is documented as ordered by definition UID. Diagnostics and
// collision reports read it, so the order must not follow map iteration or
// arrival.
func TestRuntimeEntriesAreOrderedByDefinitionUID(t *testing.T) {
	t.Parallel()

	r := admittedRegistry(t)
	for _, uid := range []string{"uid-c", "uid-a", "uid-b"} {
		mustSetRuntime(t, r, runtimeEntry(uid, 1, uid+"-addon"))
	}

	var got []string
	for _, e := range r.RuntimeEntries() {
		got = append(got, string(e.Revision.DefinitionUID))
	}
	want := []string{"uid-a", "uid-b", "uid-c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("RuntimeEntries order: got %v, want ascending definition UID %v", got, want)
	}
}

// Barrier claimants are operator triage material: their order must depend only
// on definition UID, never on arrival order, and a built-in contesting the same
// identity must be named alongside the runtime claimants.
func TestCollisionClaimantsAreOrderedAndNameTheBuiltin(t *testing.T) {
	t.Parallel()

	orders := [][]string{{"uid-a", "uid-b"}, {"uid-b", "uid-a"}}
	for _, order := range orders {
		order := order
		t.Run(strings.Join(order, "-then-"), func(t *testing.T) {
			t.Parallel()

			r := admittedRegistry(t)
			mustSetRuntime(t, r, runtimeEntry(order[0], 1, "custom-addon"))
			if err := r.SetRuntime(runtimeEntry(order[1], 1, "custom-addon")); err == nil {
				t.Fatalf("second SetRuntime: want a collision barrier, got nil")
			}

			barrier := mustBarred(t, r, "custom-addon")
			if barrier.Reason != registry.ReasonRuntimeCollision {
				t.Fatalf("reason %q, want %q", barrier.Reason, registry.ReasonRuntimeCollision)
			}
			if len(barrier.Claimants) != 2 ||
				!strings.Contains(barrier.Claimants[0], `"uid-a"`) ||
				!strings.Contains(barrier.Claimants[1], `"uid-b"`) {
				t.Fatalf("claimants %v: want uid-a before uid-b regardless of publication order %v",
					barrier.Claimants, order)
			}

			// A built-in claiming the same contested identity joins the barrier:
			// omitting it would hide a candidate the administrator must migrate.
			if err := r.Register(newFake("shared-builtin", "custom-addon")); err != nil {
				t.Fatalf("Register built-in over a contested identity: %v", err)
			}
			barrier = mustBarred(t, r, "custom-addon")
			if barrier.Reason != registry.ReasonRuntimeCollision {
				t.Fatalf("reason with a built-in involved: %q, want %q",
					barrier.Reason, registry.ReasonRuntimeCollision)
			}
			if len(barrier.Claimants) != 3 {
				t.Fatalf("claimants %v: want both definitions and the built-in", barrier.Claimants)
			}
			if !strings.Contains(barrier.Claimants[0], `"uid-a"`) ||
				!strings.Contains(barrier.Claimants[1], `"uid-b"`) ||
				!strings.Contains(barrier.Claimants[2], `built-in adapter "shared-builtin"`) {
				t.Fatalf("claimants %v: want uid-a, uid-b, then the built-in", barrier.Claimants)
			}
			if !strings.Contains(barrier.Error(), `built-in adapter "shared-builtin"`) {
				t.Fatalf("barrier message %q must name the built-in claimant", barrier.Error())
			}
		})
	}
}

// A contested identity and an unknown one are different operator problems, so
// the two errors must stay disjoint under errors.Is. Callers branch on exactly
// this to avoid reporting a collision as "no adapter is registered".
func TestDispatchBarrierAndNotFoundAreDisjoint(t *testing.T) {
	t.Parallel()

	r := registry.New(logr.Discard())
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon"))

	_, barred := r.Resolve("custom-addon") // admission has never been opened
	if !errors.Is(barred, registry.ErrDispatchBarred) {
		t.Fatalf("Resolve(custom-addon): want a barrier, got %v", barred)
	}
	if errors.Is(barred, registry.ErrNotFound) {
		t.Fatalf("a dispatch barrier must not match ErrNotFound: %v", barred)
	}

	_, unknown := r.Resolve("nobody")
	if !errors.Is(unknown, registry.ErrNotFound) {
		t.Fatalf("Resolve(nobody): want ErrNotFound, got %v", unknown)
	}
	if errors.Is(unknown, registry.ErrDispatchBarred) {
		t.Fatalf("ErrNotFound must not match a dispatch barrier: %v", unknown)
	}
	var barrier *registry.DispatchBarrier
	if errors.As(unknown, &barrier) {
		t.Fatalf("Resolve(nobody): want no *DispatchBarrier, got %+v", barrier)
	}
}

// The gate is the single enforcement point, driven by the leadership session
// and nothing else. Opening it without naming that session is a mis-wiring: it
// must fail loudly and leave runtime dispatch barred.
// The driver identity is recorded so an admitted gate stays attributable: when
// dispatch is later barred, the operator needs to know which session had opened
// it. Recording it and never surfacing it would be indistinguishable from not
// recording it at all, so pin it through the revocation the registry reports.
func TestAdmittedDispatchIsAttributableToItsDriver(t *testing.T) {
	t.Parallel()

	var logLines []string
	logger := funcr.New(
		func(prefix, args string) { logLines = append(logLines, prefix+" "+args) },
		funcr.Options{},
	)
	r := registry.New(logger)
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon"))

	if err := r.OpenRuntimeDispatch(testDriver); err != nil {
		t.Fatalf("OpenRuntimeDispatch(%q): %v", testDriver, err)
	}
	r.CloseRuntimeDispatch("leadership lost")

	var revoked string
	for _, line := range logLines {
		if strings.Contains(line, "runtime dispatch revoked") {
			revoked = line
		}
	}
	if revoked == "" {
		t.Fatalf("withdrawing an admitted gate reported nothing; log was %q", logLines)
	}
	if !strings.Contains(revoked, testDriver) {
		t.Fatalf("revocation %q does not name the driver %q that had admitted dispatch", revoked, testDriver)
	}
}

func TestOpenRuntimeDispatchRequiresALeadershipDriver(t *testing.T) {
	t.Parallel()

	r := registry.New(logr.Discard())
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mustSetRuntime(t, r, runtimeEntry("uid-a", 1, "custom-addon"))

	if err := r.OpenRuntimeDispatch(""); !errors.Is(err, registry.ErrAdmissionDriverRequired) {
		t.Fatalf("OpenRuntimeDispatch(%q): want ErrAdmissionDriverRequired, got %v", "", err)
	}
	barrier := mustBarred(t, r, "custom-addon")
	if barrier.Reason != registry.ReasonAdmissionClosed {
		t.Fatalf("after a refused admission: reason %q, want %q",
			barrier.Reason, registry.ReasonAdmissionClosed)
	}
	if _, err := r.Lookup("cert-manager"); err != nil {
		t.Fatalf("built-in dispatch must be unaffected by a refused admission: %v", err)
	}

	// A named session admits it, and withdrawal bars it again.
	if err := r.OpenRuntimeDispatch(testDriver); err != nil {
		t.Fatalf("OpenRuntimeDispatch(%q): %v", testDriver, err)
	}
	if _, err := r.Resolve("custom-addon"); err != nil {
		t.Fatalf("Resolve after a driven admission: %v", err)
	}
	r.CloseRuntimeDispatch("leadership lost")
	if got := mustBarred(t, r, "custom-addon").Reason; got != registry.ReasonAdmissionClosed {
		t.Fatalf("Resolve after CloseRuntimeDispatch: reason %q", got)
	}
}
