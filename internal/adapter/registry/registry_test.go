/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package registry_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"

	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/pkg/adapter"
)

// fakeAdapter is a minimal [adapter.Adapter] used to drive registry tests
// without depending on any concrete adapter implementation.
type fakeAdapter struct {
	name            string
	version         string
	contractVersion string
	addonTypes      []string
	families        []adapter.Family
}

func (f fakeAdapter) Name() string            { return f.name }
func (f fakeAdapter) Version() string         { return f.version }
func (f fakeAdapter) ContractVersion() string { return f.contractVersion }
func (f fakeAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: f.addonTypes, Families: f.families}
}
func (fakeAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{}, nil
}

func newFake(name string, addonTypes ...string) fakeAdapter {
	return fakeAdapter{
		name:            name,
		version:         "1.0.0",
		contractVersion: adapter.ContractVersion,
		addonTypes:      addonTypes,
		families:        []adapter.Family{"system_health"},
	}
}

func TestRegister(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		adapter     adapter.Adapter
		wantErr     bool
		errContains string
	}{
		{
			name:    "happy path single addon type",
			adapter: newFake("cert-manager", "cert-manager"),
		},
		{
			name:    "happy path multi addon type",
			adapter: newFake("dns", "coredns", "node-local-dns"),
		},
		{
			name:        "nil adapter rejected",
			adapter:     nil,
			wantErr:     true,
			errContains: "nil adapter",
		},
		{
			name: "empty addon types rejected",
			adapter: fakeAdapter{
				name:            "noop",
				contractVersion: adapter.ContractVersion,
			},
			wantErr:     true,
			errContains: "advertises no addon types",
		},
		{
			name: "incompatible contract version rejected",
			adapter: fakeAdapter{
				name:            "from-the-future",
				contractVersion: "2.0.0", // current is 1.x; a major mismatch is breaking
				addonTypes:      []string{"future-addon"},
			},
			wantErr:     true,
			errContains: "incompatible",
		},
		{
			name: "invalid contract version rejected",
			adapter: fakeAdapter{
				name:            "garbled",
				contractVersion: "not-a-version",
				addonTypes:      []string{"x"},
			},
			wantErr:     true,
			errContains: "invalid contract version",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := registry.New(logr.Discard())
			err := r.Register(tc.adapter)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Register: want error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("Register: error %q does not contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Register: unexpected error: %v", err)
			}
		})
	}
}

func TestRegister_DuplicateAddonType(t *testing.T) {
	t.Parallel()

	r := registry.New(logr.Discard())
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := r.Register(newFake("cert-manager-fork", "cert-manager"))
	if err == nil {
		t.Fatalf("second Register: want duplicate error, got nil")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("second Register: error %q does not name duplication", err.Error())
	}
	// Both adapter names should appear in the error for operator triage.
	if !strings.Contains(err.Error(), "cert-manager") || !strings.Contains(err.Error(), "cert-manager-fork") {
		t.Fatalf("second Register: error %q should name both adapters", err.Error())
	}
}

// Registering the same adapter (same Name + same addon types) twice is
// idempotent: re-registering must not produce a duplicate-addon-type error.
// The second call must instead surface a log notice so an operator can see
// that a redundant Register reached the registry — this keeps startup
// resilient if a future loader announces an adapter from multiple sources,
// without making the duplicate invisible.
func TestRegister_SameAdapterTwiceLogsNotice(t *testing.T) {
	t.Parallel()

	var logLines []string
	logger := funcr.New(
		func(prefix, args string) { logLines = append(logLines, prefix+" "+args) },
		funcr.Options{},
	)
	r := registry.New(logger)
	a := newFake("cert-manager", "cert-manager")
	if err := r.Register(a); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if got := len(logLines); got != 0 {
		t.Fatalf("first Register: want 0 log lines, got %d (%q)", got, logLines)
	}
	if err := r.Register(a); err != nil {
		t.Fatalf("second Register of same adapter: want nil, got %v", err)
	}
	if got := len(logLines); got != 1 {
		t.Fatalf("second Register: want 1 log line, got %d (%q)", got, logLines)
	}
	if !strings.Contains(logLines[0], "already registered") {
		t.Fatalf("notice should mention duplicate registration; got %q", logLines[0])
	}
	if !strings.Contains(logLines[0], "cert-manager") {
		t.Fatalf("notice should name the adapter; got %q", logLines[0])
	}
}

// Register must be atomic per call: if any addon type in the list collides,
// none of the addon types from the failing call are added.
func TestRegister_PartialFailureLeavesRegistryUnchanged(t *testing.T) {
	t.Parallel()

	r := registry.New(logr.Discard())
	if err := r.Register(newFake("first", "shared")); err != nil {
		t.Fatalf("seed Register: %v", err)
	}
	// "second" advertises ["fresh", "shared"]; the "shared" collision must
	// also prevent "fresh" from being added.
	err := r.Register(newFake("second", "fresh", "shared"))
	if err == nil {
		t.Fatalf("colliding Register: want error, got nil")
	}
	if _, err := r.Lookup("fresh"); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("Lookup(fresh): want ErrNotFound after rolled-back Register, got %v", err)
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()

	r := registry.New(logr.Discard())
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Lookup("cert-manager")
	if err != nil {
		t.Fatalf("Lookup(cert-manager): %v", err)
	}
	if got.Name() != "cert-manager" {
		t.Fatalf("Lookup(cert-manager): got adapter %q, want %q", got.Name(), "cert-manager")
	}

	_, err = r.Lookup("does-not-exist")
	if !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("Lookup(does-not-exist): want ErrNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("Lookup miss: error %q should name the missing addon type", err.Error())
	}
}

func TestCapabilities(t *testing.T) {
	t.Parallel()

	r := registry.New(logr.Discard())
	cm := fakeAdapter{
		name:            "cert-manager",
		version:         "1.14.0",
		contractVersion: adapter.ContractVersion,
		addonTypes:      []string{"cert-manager"},
		families:        []adapter.Family{"system_health", "issuer_health", "certificate_health"},
	}
	dns := fakeAdapter{
		name:            "dns",
		version:         "0.1.0",
		contractVersion: adapter.ContractVersion,
		addonTypes:      []string{"coredns", "node-local-dns"},
		families:        []adapter.Family{"system_health"},
	}
	for _, a := range []adapter.Adapter{cm, dns} {
		if err := r.Register(a); err != nil {
			t.Fatalf("Register(%s): %v", a.Name(), err)
		}
	}

	caps := r.Capabilities()
	if len(caps) != 2 {
		t.Fatalf("Capabilities: want 2 adapters, got %d", len(caps))
	}
	if got := caps["cert-manager"].Families; len(got) != 3 {
		t.Fatalf("cert-manager families: want 3, got %d", len(got))
	}
	if got := caps["dns"].AddonTypes; len(got) != 2 {
		t.Fatalf("dns addon types: want 2, got %d", len(got))
	}

	// Snapshot must be a copy: mutating the result must not affect the registry.
	caps["dns"].AddonTypes[0] = "mutated"
	again := r.Capabilities()
	if again["dns"].AddonTypes[0] != "coredns" {
		t.Fatalf("Capabilities: snapshot must not alias registry state; got %q after mutation", again["dns"].AddonTypes[0])
	}
}

// TestConcurrentAccess exercises the RWMutex contract. Run under -race to
// catch any shared-state regressions.
func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	r := registry.New(logr.Discard())
	if err := r.Register(newFake("cert-manager", "cert-manager")); err != nil {
		t.Fatalf("seed Register: %v", err)
	}

	const readers = 16
	const lookupsPerReader = 500
	var wg sync.WaitGroup
	wg.Add(readers + 1)

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < lookupsPerReader; j++ {
				if _, err := r.Lookup("cert-manager"); err != nil {
					t.Errorf("concurrent Lookup: %v", err)
					return
				}
			}
		}()
	}
	go func() {
		defer wg.Done()
		// Register a different adapter concurrently with reads.
		if err := r.Register(newFake("dns", "coredns")); err != nil {
			t.Errorf("concurrent Register: %v", err)
		}
	}()
	wg.Wait()
}
