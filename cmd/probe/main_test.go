/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestJoin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"nil", nil, ""},
		{"empty", []string{}, ""},
		{"single", []string{"a"}, "a"},
		{"two", []string{"a", "b"}, "a,b"},
		{"ipv4 and ipv6", []string{"1.2.3.4", "::1"}, "1.2.3.4,::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := join(tc.in); got != tc.want {
				t.Errorf("join(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWriteResultDoesNotPanic(t *testing.T) {
	// Smoke: exercises the encode + os.Stdout write + termination-log write
	// paths. The terminationLog file path doesn't exist outside a probe pod
	// so the os.WriteFile call silently fails — that's the production
	// fallthrough behavior we want to exercise here too.
	writeResult(result{Outcome: "Pass", Summary: "ok", Details: map[string]string{"k": "v"}})
	writeResult(result{Outcome: "Error", Summary: "no details"})
}

func TestRunDNSRequiresTarget(t *testing.T) {
	if err := runDNS(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
}

func TestRunDNSResolvesLocalhost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// localhost is guaranteed to resolve via /etc/hosts on every sane
	// platform; it's the cheapest way to exercise the success path.
	if err := runDNS(ctx, "localhost"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDNSFailsForInvalidName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// RFC 6761 reserves the `.invalid` TLD for guaranteed-not-resolvable
	// names, which makes this deterministic without relying on a specific
	// resolver behavior. An unresolvable name is the exact condition the check
	// exists to catch: it must surface as Outcome=Fail (not Error) and return
	// nil, mirroring runTCPConnect on a refused dial. Error outranks Fail on the
	// severity ladder, so a DNS outage reported as Error would mask real Fails
	// in the ClusterHealth rollup. Regression guard for #158.
	got := captureResult(t, func() {
		if err := runDNS(ctx, "does-not-exist.invalid"); err != nil {
			t.Fatalf("expected nil error for unresolvable name, got %v", err)
		}
	})
	if got.Outcome != "Fail" {
		t.Fatalf("Outcome = %q, want Fail", got.Outcome)
	}
}

func TestRunTCPConnectRequiresTarget(t *testing.T) {
	if err := runTCPConnect(context.Background(), "", 80); err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
}

func TestRunTCPConnectRequiresPort(t *testing.T) {
	if err := runTCPConnect(context.Background(), "localhost", 0); err == nil {
		t.Fatal("expected error for zero port, got nil")
	}
}

func TestRunTCPConnectSucceeds(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	host, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runTCPConnect(ctx, host, port); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTCPConnectReturnsNilOnDialFailure(t *testing.T) {
	// runTCPConnect surfaces a dial failure as Outcome=Fail in the JSON
	// payload and returns nil — only argument-validation errors flow up.
	port := claimAndReleasePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := runTCPConnect(ctx, "127.0.0.1", port); err != nil {
		t.Fatalf("expected nil on dial failure, got %v", err)
	}
}

func TestRunTCPListenRequiresPort(t *testing.T) {
	if err := runTCPListen(context.Background(), "127.0.0.1", 0); err == nil {
		t.Fatal("expected error for zero port, got nil")
	}
}

func TestRunTCPListenCompletesOnContextCancel(t *testing.T) {
	port := claimAndReleasePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := runTCPListen(ctx, "127.0.0.1", port); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTCPListenFailsOnBindCollision(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	// Same port is still bound by us; runTCPListen must fail to bind.
	if err := runTCPListen(context.Background(), "127.0.0.1", port); err == nil {
		t.Fatal("expected error for port collision, got nil")
	}
}

func TestRunRejectsUnsupportedMode(t *testing.T) {
	withFlagReset(t, []string{"probe", "-mode=bogus", "-timeout=50ms"})
	if err := run(); err == nil {
		t.Fatal("expected error for unsupported mode, got nil")
	}
}

func TestRunDispatchesToDNS(t *testing.T) {
	withFlagReset(t, []string{"probe", "-mode=dns", "-target=localhost", "-timeout=2s"})
	if err := run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// captureResult redirects os.Stdout for the duration of fn, then decodes the
// single JSON probe result that writeResult emits. It lets tests assert on the
// Outcome field without exporting a seam from the probe binary.
func captureResult(t *testing.T, fn func()) result {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = oldStdout

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	var got result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode probe result %q: %v", data, err)
	}
	return got
}

// claimAndReleasePort binds to a kernel-assigned port on 127.0.0.1, closes
// the listener, and returns the port. There's a small race window where the
// port can be re-grabbed before the caller uses it; we accept that for
// simplicity rather than refactor runTCPListen to expose a net.Listen seam.
func claimAndReleasePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("Atoi: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close: %v", err)
	}
	return port
}

// withFlagReset swaps in a fresh flag.CommandLine and os.Args so that run()
// can be invoked multiple times across tests without flag-redefinition
// panics from flag.String. The originals are restored via t.Cleanup.
func withFlagReset(t *testing.T, args []string) {
	t.Helper()
	oldArgs := os.Args
	oldFlags := flag.CommandLine
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldFlags
	})
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = args
}
