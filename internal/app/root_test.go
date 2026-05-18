/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestSignalContext_PropagatesParentCancellation locks the contract that
// cancelling the parent context propagates to the signalContext-derived
// child within a tight bound. RunE relies on this when an outer scope
// (e.g. a higher-level shutdown coordinator, or a test) drives shutdown
// directly rather than via SIGINT/SIGTERM.
func TestSignalContext_PropagatesParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, stop := signalContext(parent)
	defer stop()

	cancel()

	select {
	case <-ctx.Done():
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("signalContext: child not cancelled within 2s of parent cancel")
	}
}

// TestSignalContext_StopReleasesContext locks the contract that calling
// the returned stop function cancels the context. RunE defers stop()
// after Run returns; without this, a deferred stop would never fire and
// the goroutine signal.NotifyContext spawns would leak past process
// shutdown in long-running test runs.
func TestSignalContext_StopReleasesContext(t *testing.T) {
	ctx, stop := signalContext(context.Background())

	stop()

	select {
	case <-ctx.Done():
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("signalContext: stop() did not cancel ctx within 2s")
	}
}

// TestSignalContext_SIGTERMCancels exercises the shutdown signal path:
// signal.NotifyContext intercepts SIGTERM (the conventional Kubernetes
// pod-termination signal) before the Go runtime's default handler would
// terminate the process. Sending SIGTERM to our own PID is safe in this
// test because signalContext has registered a notifier for it; the
// signal is delivered to the internal channel rather than killing the
// test binary.
func TestSignalContext_SIGTERMCancels(t *testing.T) {
	ctx, stop := signalContext(context.Background())
	defer stop()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess(%d): %v", os.Getpid(), err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Signal(SIGTERM): %v", err)
	}

	select {
	case <-ctx.Done():
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("signalContext: ctx not cancelled within 2s of SIGTERM")
	}
}

// TestSignalContext_SIGINTCancels mirrors TestSignalContext_SIGTERMCancels
// for SIGINT (the conventional Ctrl+C signal). Both signals must drive
// the same graceful-shutdown path.
func TestSignalContext_SIGINTCancels(t *testing.T) {
	ctx, stop := signalContext(context.Background())
	defer stop()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess(%d): %v", os.Getpid(), err)
	}
	if err := proc.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal(SIGINT): %v", err)
	}

	select {
	case <-ctx.Done():
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("signalContext: ctx not cancelled within 2s of SIGINT")
	}
}
