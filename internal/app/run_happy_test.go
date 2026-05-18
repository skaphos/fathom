/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/skaphos/fathom/pkg/adapter"
)

var envtestCfg *rest.Config

// TestMain stands up a single envtest environment shared by every test in
// this package that wants a real REST config — Run's happy path needs one
// to construct a manager. Pure-unit tests (NewScheme, BuildManagerOptions,
// adapterName, disableHTTP2, NewRootCommand wiring) don't touch envtestCfg
// and pay nothing for this setup beyond TestMain's startup cost.
func TestMain(m *testing.M) {
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	if dir := firstEnvtestBinaryDir(); dir != "" {
		env.BinaryAssetsDirectory = dir
	}

	cfg, err := env.Start()
	if err != nil {
		// envtest startup can fail when KUBEBUILDER_ASSETS is unset (e.g.,
		// `go test ./...` without the Taskfile wrapper). Skip the envtest-
		// dependent tests rather than fail the whole package — the unit
		// tests that don't need envtest still run.
		envtestCfg = nil
	} else {
		envtestCfg = cfg
	}

	code := m.Run()
	if envtestCfg != nil {
		_ = env.Stop()
	}
	os.Exit(code)
}

func firstEnvtestBinaryDir() string {
	base := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(base, e.Name())
		}
	}
	return ""
}

// TestDisableHTTP2 covers the helper that mitigates HTTP/2 stream-reset
// CVEs by restricting TLS NextProtos to HTTP/1.1.
func TestDisableHTTP2(t *testing.T) {
	c := &tls.Config{}
	disableHTTP2(c)
	if len(c.NextProtos) != 1 || c.NextProtos[0] != "http/1.1" {
		t.Errorf("NextProtos = %v, want [http/1.1]", c.NextProtos)
	}
}

// TestAdapterName_NilReturnsPlaceholder locks the contract that a nil
// adapter formats as "<nil>" rather than panicking — defensive coverage
// for adapter registry error paths that log adapterName(a) when
// registration fails.
func TestAdapterName_NilReturnsPlaceholder(t *testing.T) {
	if got := adapterName(nil); got != "<nil>" {
		t.Errorf("adapterName(nil) = %q, want %q", got, "<nil>")
	}
}

// TestAdapterName_NonNilReturnsName confirms the happy path delegates to
// the adapter's own Name() — symmetric with the nil case.
func TestAdapterName_NonNilReturnsName(t *testing.T) {
	if got := adapterName(appFakeAdapter{}); got != "fake-cert-manager" {
		t.Errorf("adapterName(appFakeAdapter) = %q, want %q", got, "fake-cert-manager")
	}
}

// TestRun_HappyPath_NoControllers exercises Run's manager wiring against
// a real envtest apiserver but with an empty controllers slice. This
// covers the cache-sync goroutine, healthz/readyz registration, the
// watcher loop (which is empty here since no cert paths are configured),
// and manager.Start through to clean shutdown.
//
// Skipped when envtest could not start (e.g., `go test ./...` without
// `task test`'s KUBEBUILDER_ASSETS bootstrap).
func TestRun_HappyPath_NoControllers(t *testing.T) {
	if envtestCfg == nil {
		t.Skip("envtest unavailable; run via `task test` for full coverage")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Cancel after the manager has had time to start; Run returns
		// when ctx is done.
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	opts := DefaultOptions()
	// Bind to ephemeral ports so concurrent test runs don't collide.
	opts.Metrics.BindAddress = "0"
	opts.HealthProbeBindAddress = "0"
	opts.LeaderElect = false

	noControllers := func(ctrl.Manager) ([]Setupper, error) {
		return nil, nil
	}
	if err := Run(ctx, envtestCfg, opts, noControllers); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestRun_HappyPath_DefaultControllers also exercises the
// `controllersFor == nil` branch in Run, which routes through
// DefaultControllers. Together with the no-controllers test this covers
// every branch of Run's controller-wiring path.
func TestRun_HappyPath_DefaultControllers(t *testing.T) {
	if envtestCfg == nil {
		t.Skip("envtest unavailable; run via `task test` for full coverage")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	opts := DefaultOptions()
	opts.Metrics.BindAddress = "0"
	opts.HealthProbeBindAddress = "0"
	opts.LeaderElect = false

	if err := Run(ctx, envtestCfg, opts, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// silence unused-import warnings if the file shrinks during refactors.
var _ adapter.Adapter = appFakeAdapter{}
