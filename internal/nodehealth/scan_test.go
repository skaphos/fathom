/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// fakeStatfs returns a statfs seam that reports the given geometry for every
// path, or err when non-nil.
func fakeStatfs(blocks, bavail, files, ffree uint64, err error) statfsFunc {
	return func(_ string, st *unix.Statfs_t) error {
		if err != nil {
			return err
		}
		*st = unix.Statfs_t{}
		st.Bsize = 4096
		st.Blocks = blocks
		st.Bavail = bavail
		st.Files = files
		st.Ffree = ffree
		return nil
	}
}

func TestHeadroomClassification(t *testing.T) {
	t.Parallel()
	item := func(typ string) Item {
		return Item{Type: typ, Path: "/var/lib/kubelet", WarnPercentFree: 20, CriticalPercentFree: 10}
	}
	tests := []struct {
		name        string
		item        Item
		statfs      statfsFunc
		wantOutcome Outcome
		wantInSum   string
		wantPct     float64
	}{
		{"disk plenty free", item(TypeDiskHeadroom), fakeStatfs(100, 50, 1, 1, nil), OutcomePass, "50.0% of bytes free", 50},
		{"disk at warn boundary", item(TypeDiskHeadroom), fakeStatfs(100, 20, 1, 1, nil), OutcomeWarn, "at or below warnPercentFree 20", 20},
		{"disk at critical boundary", item(TypeDiskHeadroom), fakeStatfs(100, 10, 1, 1, nil), OutcomeFail, "at or below criticalPercentFree 10", 10},
		{"disk nothing free", item(TypeDiskHeadroom), fakeStatfs(100, 0, 1, 1, nil), OutcomeFail, "0.0% of bytes free", 0},
		{"inodes plenty free", item(TypeInodeHeadroom), fakeStatfs(1, 1, 1000, 900, nil), OutcomePass, "90.0% of inodes free", 90},
		{"inodes at critical", item(TypeInodeHeadroom), fakeStatfs(1, 1, 1000, 100, nil), OutcomeFail, "inodes free (at or below criticalPercentFree 10)", 10},
		{"zero thresholds never warn", Item{Type: TypeDiskHeadroom, Path: "/var/log"}, fakeStatfs(100, 1, 1, 1, nil), OutcomePass, "1.0% of bytes free", 1},
		{"zero thresholds still fail at zero free", Item{Type: TypeDiskHeadroom, Path: "/var/log"}, fakeStatfs(100, 0, 1, 1, nil), OutcomeFail, "criticalPercentFree 0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := headroom(context.Background(), tt.item, tt.statfs, nil, tt.item.Type == TypeInodeHeadroom)
			if got.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %s, want %s (%s)", got.Outcome, tt.wantOutcome, got.Summary)
			}
			if !strings.Contains(got.Summary, tt.wantInSum) {
				t.Fatalf("summary %q does not contain %q", got.Summary, tt.wantInSum)
			}
			if got.PercentFree == nil || *got.PercentFree != tt.wantPct {
				t.Fatalf("percentFree = %v, want %v", got.PercentFree, tt.wantPct)
			}
			if got.Type != tt.item.Type || got.Path != tt.item.Path {
				t.Fatalf("result identity = %s/%s, want %s/%s", got.Type, got.Path, tt.item.Type, tt.item.Path)
			}
		})
	}
}

// TestHeadroomStatfsFailures pins the nodecert-consistent verdict for paths the
// agent cannot measure: absent and unreadable paths are Skipped so a
// distribution difference never makes a healthy node report Error, while a
// genuine measurement failure is Error.
func TestHeadroomStatfsFailures(t *testing.T) {
	t.Parallel()
	item := Item{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet", WarnPercentFree: 20, CriticalPercentFree: 10}
	tests := []struct {
		name        string
		err         error
		wantOutcome Outcome
		wantInSum   string
	}{
		{"absent path", syscall.ENOENT, OutcomeSkipped, "does not exist"},
		{"permission denied", syscall.EACCES, OutcomeSkipped, "permission denied"},
		{"io error", syscall.EIO, OutcomeError, "cannot stat filesystem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := headroom(context.Background(), item, fakeStatfs(0, 0, 0, 0, tt.err), nil, false)
			if got.Outcome != tt.wantOutcome || !strings.Contains(got.Summary, tt.wantInSum) {
				t.Fatalf("got %s %q, want %s containing %q", got.Outcome, got.Summary, tt.wantOutcome, tt.wantInSum)
			}
			if got.PercentFree != nil {
				t.Fatal("a failed measurement must not carry a percentage")
			}
		})
	}

	t.Run("zero-block filesystem is an error, not a division", func(t *testing.T) {
		t.Parallel()
		got := headroom(context.Background(), item, fakeStatfs(0, 0, 0, 0, nil), nil, false)
		if got.Outcome != OutcomeError || !strings.Contains(got.Summary, "zero bytes") {
			t.Fatalf("got %s %q", got.Outcome, got.Summary)
		}
	})
}

func TestKubeletHealthz(t *testing.T) {
	t.Parallel()

	t.Run("2xx is Pass", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
		defer srv.Close()
		got := kubeletHealthz(context.Background(), srv.Client(), srv.URL+"/healthz", time.Second)
		if got.Outcome != OutcomePass || !strings.Contains(got.Summary, "returned 200") {
			t.Fatalf("got %s %q", got.Outcome, got.Summary)
		}
		if got.Type != TypeKubeletHealthz || got.Path != "" {
			t.Fatalf("result identity = %s/%q", got.Type, got.Path)
		}
	})

	t.Run("non-2xx is Fail", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
		defer srv.Close()
		got := kubeletHealthz(context.Background(), srv.Client(), srv.URL+"/healthz", time.Second)
		if got.Outcome != OutcomeFail || !strings.Contains(got.Summary, "returned 500") {
			t.Fatalf("got %s %q", got.Outcome, got.Summary)
		}
	})

	t.Run("unreachable is Fail, not Error", func(t *testing.T) {
		t.Parallel()
		// Bind then close so the port is known-refused.
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close()
		got := kubeletHealthz(context.Background(), &http.Client{}, "http://"+addr+"/healthz", time.Second)
		if got.Outcome != OutcomeFail || !strings.Contains(got.Summary, "unreachable") {
			t.Fatalf("got %s %q", got.Outcome, got.Summary)
		}
	})

	t.Run("timeout is bounded", func(t *testing.T) {
		t.Parallel()
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { <-release }))
		defer func() { close(release); srv.Close() }()
		start := time.Now()
		got := kubeletHealthz(context.Background(), srv.Client(), srv.URL+"/healthz", 50*time.Millisecond)
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("probe was not bounded by the timeout: took %s", elapsed)
		}
		if got.Outcome != OutcomeFail {
			t.Fatalf("got %s %q", got.Outcome, got.Summary)
		}
	})
}

func TestContainerRuntime(t *testing.T) {
	t.Parallel()
	okDial := func(_ context.Context, network, addr string) (net.Conn, error) {
		if network != "unix" {
			t.Errorf("network = %q, want unix", network)
		}
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
	failDial := func(err error) dialFunc {
		return func(context.Context, string, string) (net.Conn, error) {
			return nil, &net.OpError{Op: "dial", Net: "unix", Err: err}
		}
	}
	tests := []struct {
		name        string
		dial        dialFunc
		wantOutcome Outcome
		wantInSum   string
	}{
		{"accepting connections", okDial, OutcomePass, "accepting connections on /run/containerd/containerd.sock"},
		{"socket absent", failDial(syscall.ENOENT), OutcomeFail, "does not exist"},
		{"permission denied", failDial(syscall.EACCES), OutcomeSkipped, "permission denied"},
		{"connection refused", failDial(syscall.ECONNREFUSED), OutcomeFail, "refused connection"},
		{"other error", failDial(errors.New("boom")), OutcomeFail, "cannot connect to container runtime socket /run/containerd/containerd.sock: dial unix: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := containerRuntime(context.Background(), tt.dial, "/run/containerd/containerd.sock", time.Second)
			if got.Outcome != tt.wantOutcome || !strings.Contains(got.Summary, tt.wantInSum) {
				t.Fatalf("got %s %q, want %s containing %q", got.Outcome, got.Summary, tt.wantOutcome, tt.wantInSum)
			}
			if got.Type != TypeContainerRuntime || got.Path != "/run/containerd/containerd.sock" {
				t.Fatalf("result identity = %s/%s", got.Type, got.Path)
			}
		})
	}
}

// TestScanDispatchesAndOrders pins that Scan evaluates every agent-side type,
// skips NodeCondition (the operator's job), flags an unknown type as Error
// rather than dropping it, and returns results in a deterministic order.
func TestScanDispatchesAndOrders(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()

	got := Scan(context.Background(), ScanOptions{
		Items: []Item{
			{Type: TypeKubeletHealthz},
			{Type: TypeDiskHeadroom, Path: "/var/log", WarnPercentFree: 20, CriticalPercentFree: 10},
			{Type: TypeNodeCondition},
			{Type: TypeContainerRuntime, SocketPath: "/run/containerd/containerd.sock"},
			{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet", WarnPercentFree: 20, CriticalPercentFree: 10},
			{Type: "Bogus", Path: "/x"},
		},
		KubeletHealthzURL: srv.URL + "/healthz",
		httpClient:        srv.Client(),
		statfs:            fakeStatfs(100, 50, 1, 1, nil),
		dial: func(context.Context, string, string) (net.Conn, error) {
			c, s := net.Pipe()
			_ = s.Close()
			return c, nil
		},
	})

	var order []string
	for _, r := range got {
		order = append(order, r.Type+":"+r.Path)
	}
	want := []string{
		"Bogus:/x",
		"ContainerRuntime:/run/containerd/containerd.sock",
		"DiskHeadroom:/var/lib/kubelet",
		"DiskHeadroom:/var/log",
		"KubeletHealthz:",
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if got[0].Outcome != OutcomeError || !strings.Contains(got[0].Summary, "unknown check type") {
		t.Fatalf("unknown type = %s %q", got[0].Outcome, got[0].Summary)
	}
	for _, r := range got[1:] {
		if r.Outcome != OutcomePass {
			t.Fatalf("%s/%s = %s %q, want Pass", r.Type, r.Path, r.Outcome, r.Summary)
		}
	}
	if WorstOutcome(got) != OutcomeError {
		t.Fatalf("WorstOutcome = %s, want Error", WorstOutcome(got))
	}
	if WorstOutcome(got[1:]) != OutcomePass {
		t.Fatalf("WorstOutcome(passing) = %s, want Pass", WorstOutcome(got[1:]))
	}
	if WorstOutcome(nil) != OutcomeSkipped {
		t.Fatalf("WorstOutcome(nil) = %s, want Skipped", WorstOutcome(nil))
	}
}

// TestScanRealStatfs measures a real directory once, so the unix.Statfs call
// (not just the seam) is exercised and the field-width casts compile and
// behave on this platform.
func TestScanRealStatfs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got := Scan(context.Background(), ScanOptions{Items: []Item{
		{Type: TypeDiskHeadroom, Path: dir},
		{Type: TypeInodeHeadroom, Path: dir},
	}})
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	for _, r := range got {
		if r.Outcome == OutcomeError {
			t.Fatalf("%s on %s: %s", r.Type, dir, r.Summary)
		}
		if r.PercentFree == nil || *r.PercentFree < 0 || *r.PercentFree > 100 {
			t.Fatalf("%s percentFree = %v", r.Type, r.PercentFree)
		}
	}
}

// TestScanProbesRunConcurrently pins that a hung runtime socket cannot exhaust
// the pass budget for the kubelet probe: each network probe gets its own
// timeout under the pass context, so a healthy kubelet passes next to a hung
// runtime, and the pass takes about one timeout, not two.
func TestScanProbesRunConcurrently(t *testing.T) {
	kubelet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer kubelet.Close()
	hungDial := func(ctx context.Context, _, _ string) (net.Conn, error) { <-ctx.Done(); return nil, ctx.Err() }

	const timeout = 300 * time.Millisecond
	start := time.Now()
	results := Scan(context.Background(), ScanOptions{
		Items: []Item{
			{Type: TypeContainerRuntime, SocketPath: "/run/hung.sock"},
			{Type: TypeKubeletHealthz},
		},
		Timeout:           timeout,
		KubeletHealthzURL: kubelet.URL + "/healthz",
		dial:              hungDial,
	})
	elapsed := time.Since(start)
	byType := map[string]CheckResult{}
	for _, r := range results {
		byType[r.Type] = r
	}
	if byType[TypeKubeletHealthz].Outcome != OutcomePass {
		t.Fatalf("kubelet next to a hung runtime = %+v, want Pass", byType[TypeKubeletHealthz])
	}
	if byType[TypeContainerRuntime].Outcome != OutcomeFail || !strings.Contains(byType[TypeContainerRuntime].Summary, "timed out") {
		t.Fatalf("hung runtime = %+v, want Fail naming the timeout, not a refusal", byType[TypeContainerRuntime])
	}
	if elapsed > 2*timeout-50*time.Millisecond {
		t.Fatalf("pass took %v; probes must run concurrently, not serially (2x%v)", elapsed, timeout)
	}
}

// TestInodeHeadroomWithoutInodeTableIsSkipped pins the btrfs case: statfs
// reports f_files == 0 on filesystems without a fixed inode table, which is
// "nothing to measure", not a measurement failure that should outrank a full
// disk elsewhere in the fleet. Zero bytes stays an Error: a mounted
// filesystem with no blocks is a genuine anomaly.
func TestInodeHeadroomWithoutInodeTableIsSkipped(t *testing.T) {
	t.Parallel()
	btrfs := func(_ string, st *unix.Statfs_t) error {
		st.Bsize = 4096
		st.Blocks, st.Bavail = 1000, 500
		st.Files, st.Ffree = 0, 0
		return nil
	}
	got := headroom(context.Background(), Item{Type: TypeInodeHeadroom, Path: "/var/lib/kubelet", WarnPercentFree: 20, CriticalPercentFree: 10}, btrfs, nil, true)
	if got.Outcome != OutcomeSkipped || !strings.Contains(got.Summary, "inode") {
		t.Fatalf("inodes on a btrfs-like filesystem = %+v, want Skipped", got)
	}
	if got := headroom(context.Background(), Item{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet"}, btrfs, nil, false); got.Outcome != OutcomePass {
		t.Fatalf("bytes on the same filesystem = %+v, want Pass (50%% free)", got)
	}
	noBlocks := func(_ string, st *unix.Statfs_t) error { return nil }
	if got := headroom(context.Background(), Item{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet"}, noBlocks, nil, false); got.Outcome != OutcomeError {
		t.Fatalf("zero bytes = %+v, want Error", got)
	}
}

// TestHeadroomStatfsHonoursTheDeadline pins that a statfs which never returns
// (a hung mount, uninterruptible) cannot wedge the pass: the item is graded
// Error within the timeout and the rest of the pass continues.
func TestHeadroomStatfsHonoursTheDeadline(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	defer close(release)
	hung := func(_ string, _ *unix.Statfs_t) error { <-release; return nil }

	start := time.Now()
	results := Scan(context.Background(), ScanOptions{
		Items:   []Item{{Type: TypeDiskHeadroom, Path: "/var/lib/kubelet"}},
		Timeout: 200 * time.Millisecond,
		statfs:  hung,
	})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("a hung statfs must not block the pass; took %v", elapsed)
	}
	if len(results) != 1 || results[0].Outcome != OutcomeError || !strings.Contains(results[0].Summary, "did not return") {
		t.Fatalf("hung statfs = %+v, want Error naming the timeout", results)
	}
}

// TestKubeletHealthzDoesNotFollowRedirects pins that a 3xx from the health
// endpoint is graded on its own status, never on the target it points at.
func TestKubeletHealthzDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer ok.Close()
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ok.URL+"/healthz", http.StatusFound)
	}))
	defer redirecting.Close()
	results := Scan(context.Background(), ScanOptions{
		Items:             []Item{{Type: TypeKubeletHealthz}},
		Timeout:           2 * time.Second,
		KubeletHealthzURL: redirecting.URL + "/healthz",
	})
	if len(results) != 1 || results[0].Outcome != OutcomeFail {
		t.Fatalf("a redirecting health endpoint = %+v, want Fail (its own 302), not the redirect target's 200", results)
	}
}

// TestHungStatfsIsAbandonedOncePerPath pins the leak bound: a permanently
// hung mount costs exactly one abandoned goroutine. The next pass does not
// start a second statfs for the path; it reports the path hung immediately.
func TestHungStatfsIsAbandonedOncePerPath(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	hung := func(_ string, _ *unix.Statfs_t) error { calls.Add(1); <-release; return nil }
	opts := ScanOptions{Items: []Item{{Type: TypeDiskHeadroom, Path: "/var/lib/hung-once"}}, Timeout: 100 * time.Millisecond, statfs: hung, StatfsGuard: NewStatfsGuard()}

	first := Scan(context.Background(), opts)
	second := Scan(context.Background(), opts)
	if calls.Load() != 1 {
		t.Fatalf("statfs started %d times for one hung path, want exactly 1", calls.Load())
	}
	if first[0].Outcome != OutcomeError || second[0].Outcome != OutcomeError || !strings.Contains(second[0].Summary, "previous pass") {
		t.Fatalf("passes = %+v / %+v, want Error both times, the second naming the earlier hung call", first[0], second[0])
	}
}

// TestNetworkProbesAreNotStarvedByAHungStatfs pins that a hung headroom
// path, which sorts before the network items, cannot hand the kubelet probe
// an expired context: the probes are launched before headroom runs inline.
func TestNetworkProbesAreNotStarvedByAHungStatfs(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	hung := func(_ string, _ *unix.Statfs_t) error { <-release; return nil }
	kubelet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer kubelet.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond) // the shared pass context
	defer cancel()
	results := Scan(ctx, ScanOptions{
		Items:             []Item{{Type: TypeDiskHeadroom, Path: "/var/lib/hung-starve"}, {Type: TypeKubeletHealthz}},
		Timeout:           300 * time.Millisecond,
		KubeletHealthzURL: kubelet.URL + "/healthz",
		statfs:            hung,
	})
	byType := map[string]CheckResult{}
	for _, r := range results {
		byType[r.Type] = r
	}
	if byType[TypeKubeletHealthz].Outcome != OutcomePass {
		t.Fatalf("kubelet beside a hung statfs = %+v, want Pass", byType[TypeKubeletHealthz])
	}
	if byType[TypeDiskHeadroom].Outcome != OutcomeError {
		t.Fatalf("hung headroom = %+v, want Error", byType[TypeDiskHeadroom])
	}
}
