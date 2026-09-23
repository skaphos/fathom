/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// fakeClock separates the wall clock from monotonic elapsed time so a test can
// move one without the other. The takeover grace must follow the monotonic
// reading only: a wall clock can jump (NTP step, VM restore) in either
// direction and must never shorten the grace.
type fakeClock struct {
	mu       sync.Mutex
	elapsed  time.Duration
	wall     time.Time
	wallStep time.Duration
	sleeps   int
}

func newFakeClock() *fakeClock {
	return &fakeClock{wall: time.Date(2040, time.March, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	at := c.wall
	c.wall = c.wall.Add(c.wallStep)
	return at
}

func (c *fakeClock) since(time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.elapsed
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.elapsed += d
}

// sleep advances monotonic elapsed time by the requested wait. It fails after a
// handful of calls so a grace loop that never makes progress reports a failure
// instead of hanging the suite.
func (c *fakeClock) sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sleeps++
	if c.sleeps > 8 {
		return errors.New("takeover grace slept without making progress")
	}
	c.elapsed += d
	return nil
}

type exitRecorder struct {
	mu    sync.Mutex
	codes []int
}

func (r *exitRecorder) exit(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codes = append(r.codes, code)
}

func (r *exitRecorder) observed() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.codes...)
}

func runtimeLeadershipOptions() Options {
	opts := DefaultOptions()
	opts.RuntimeLoading.Enabled = true
	opts.Namespace = "fathom-system"
	return opts
}

// newTestLeadership builds a session whose clock, process exit and cache-sync
// gate are all injected, so leadership transitions are exercised without a
// manager or an API server.
func newTestLeadership(t *testing.T) (*RuntimeLeadership, *fakeClock, *exitRecorder) {
	t.Helper()
	session, err := NewRuntimeLeadership(runtimeLeadershipOptions())
	if err != nil {
		t.Fatalf("NewRuntimeLeadership: %v", err)
	}
	clock, exits := newFakeClock(), &exitRecorder{}
	session.now = clock.now
	session.elapsedSince = clock.since
	session.sleep = clock.sleep
	session.exit = exits.exit
	session.WithCacheSync(func(context.Context) bool { return true })
	session.workDrainTimeout = 2 * time.Second
	return session, clock, exits
}

// beginSyncedSession puts a session in the state the manager leaves it in right
// after election and cache sync: acquired, synchronized, grace still running.
func beginSyncedSession(t *testing.T, session *RuntimeLeadership) {
	t.Helper()
	if err := session.begin(t.Context()); err != nil {
		t.Fatalf("begin: %v", err)
	}
	session.markSynced()
}

func heldLease(session *RuntimeLeadership) *coordinationv1.Lease {
	acquired := metav1.NewMicroTime(time.Date(2040, time.March, 1, 0, 0, 0, 0, time.UTC))
	renewed := metav1.NewMicroTime(acquired.Add(2 * time.Second))
	holder, duration, transitions := session.Identity(), int32(15), int32(7)
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       session.LeaseRef().Namespace,
			Name:            session.LeaseRef().Name,
			UID:             "lease-uid",
			ResourceVersion: "100",
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holder,
			AcquireTime:          &acquired,
			RenewTime:            &renewed,
			LeaseDurationSeconds: &duration,
			LeaseTransitions:     &transitions,
		},
	}
}

// epochOf is the acknowledgement a caller persists for a Lease: exactly the four
// fields the contract compares, so renewTime and resourceVersion stay out of it.
func epochOf(lease *coordinationv1.Lease) *fathomv1alpha1.DefinitionLeaderEpoch {
	return &fathomv1alpha1.DefinitionLeaderEpoch{
		LeaseUID:         string(lease.UID),
		HolderIdentity:   *lease.Spec.HolderIdentity,
		AcquireTime:      *lease.Spec.AcquireTime,
		LeaseTransitions: *lease.Spec.LeaseTransitions,
	}
}

// sessionStarted reports whether the session recorded an acquisition.
func sessionStarted(session *RuntimeLeadership) bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.started
}

// sessionEnded reads the end flag the graceful-stop and loss paths set, so a
// test can prove it is exercising the window *before* the session records its
// end rather than the already-ended state.
func sessionEnded(session *RuntimeLeadership) bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.ended
}

func TestRuntimeLeadershipUsesTheConfiguredNamespacedLease(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*Options)
		wantErr   string
		wantLease types.NamespacedName
	}{
		{
			name:      "default election id",
			mutate:    func(*Options) {},
			wantLease: types.NamespacedName{Namespace: "fathom-system", Name: "2d3dbc4f.skaphos.io"},
		},
		{
			name:      "explicitly supplied non-default election id",
			mutate:    func(o *Options) { o.LeaderElectionID = "tenant-a.skaphos.io" },
			wantLease: types.NamespacedName{Namespace: "fathom-system", Name: "tenant-a.skaphos.io"},
		},
		{
			name:      "configured operator namespace",
			mutate:    func(o *Options) { o.Namespace = "ops" },
			wantLease: types.NamespacedName{Namespace: "ops", Name: "2d3dbc4f.skaphos.io"},
		},
		{name: "runtime loading disabled", mutate: func(o *Options) { o.RuntimeLoading.Enabled = false }, wantErr: "RuntimeLoadingDisabled"},
		{name: "leader election disabled", mutate: func(o *Options) { o.LeaderElect = false }, wantErr: "LeaderElectionRequired"},
		{name: "operator namespace missing", mutate: func(o *Options) { o.Namespace = "" }, wantErr: "OperatorNamespaceRequired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := runtimeLeadershipOptions()
			tc.mutate(&opts)
			session, err := NewRuntimeLeadership(opts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err=%v want it to name %s", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewRuntimeLeadership: %v", err)
			}
			if got := session.LeaseRef(); got != tc.wantLease {
				t.Fatalf("lease=%v want %v", got, tc.wantLease)
			}
		})
	}
}

func TestRuntimeLeadershipHolderIdentityIsProcessUnique(t *testing.T) {
	restore := osHostname
	t.Cleanup(func() { osHostname = restore })
	osHostname = func() (string, error) { return strings.Repeat("n", 300), nil }

	// Two sessions stand in for two operator processes scheduled onto the same
	// host: a hostname-only identity would let them share one Lease holder.
	first, err := NewRuntimeLeadership(runtimeLeadershipOptions())
	if err != nil {
		t.Fatalf("NewRuntimeLeadership: %v", err)
	}
	second, err := NewRuntimeLeadership(runtimeLeadershipOptions())
	if err != nil {
		t.Fatalf("NewRuntimeLeadership: %v", err)
	}
	if first.Identity() == second.Identity() {
		t.Fatalf("two sessions on one host share holder identity %q", first.Identity())
	}
	for _, identity := range []string{first.Identity(), second.Identity()} {
		if len(identity) > maxHolderIdentityBytes {
			t.Fatalf("holder identity is %d bytes, over the %d-byte bound", len(identity), maxHolderIdentityBytes)
		}
		if !strings.HasPrefix(identity, "n") || !strings.Contains(identity, "_") {
			t.Fatalf("identity %q does not carry the host prefix and a unique suffix", identity)
		}
	}

	osHostname = func() (string, error) { return "", errors.New("no hostname") }
	if _, err := NewRuntimeLeadership(runtimeLeadershipOptions()); err == nil {
		t.Fatal("an unresolvable hostname must fail closed instead of yielding a shared identity")
	}
}

func TestRuntimeAdmissionAndDrainWaitForTheTakeoverGrace(t *testing.T) {
	for _, tc := range []struct {
		name    string
		elapsed time.Duration
		want    bool
	}{
		{name: "immediately after acquisition", elapsed: 0},
		{name: "one nanosecond short", elapsed: limits.MinTakeoverGrace - time.Nanosecond},
		{name: "exactly the grace", elapsed: limits.MinTakeoverGrace, want: true},
		{name: "well past the grace", elapsed: 2 * limits.MinTakeoverGrace, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, clock, _ := newTestLeadership(t)
			beginSyncedSession(t, session)
			if _, err := session.AdoptLease(heldLease(session)); err != nil {
				t.Fatalf("AdoptLease: %v", err)
			}
			session.Revoke("addon")
			clock.advance(tc.elapsed)

			_, release, err := session.Admit("other-addon")
			if got := err == nil; got != tc.want {
				t.Fatalf("Admit admitted=%v (%v) want %v", got, err, tc.want)
			}
			if release != nil {
				release()
			}
			_, drainErr := session.AcknowledgeDrain("addon")
			if got := drainErr == nil; got != tc.want {
				t.Fatalf("AcknowledgeDrain acknowledged=%v (%v) want %v", got, drainErr, tc.want)
			}
			if !tc.want && !errors.Is(err, ErrTakeoverGracePending) {
				t.Fatalf("Admit error %v must be ErrTakeoverGracePending", err)
			}
			if !tc.want && !errors.Is(drainErr, ErrTakeoverGracePending) {
				t.Fatalf("AcknowledgeDrain error %v must be ErrTakeoverGracePending", drainErr)
			}
		})
	}
}

func TestTakeoverGraceIgnoresWallClockJumps(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	// Every wall-clock read jumps an hour ahead; monotonic elapsed stays at zero.
	clock.wallStep = time.Hour
	beginSyncedSession(t, session)
	if _, err := session.AdoptLease(heldLease(session)); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	session.Revoke("addon")

	if _, _, err := session.Admit("addon-two"); !errors.Is(err, ErrTakeoverGracePending) {
		t.Fatalf("a wall-clock jump satisfied the takeover grace: %v", err)
	}
	if _, err := session.AcknowledgeDrain("addon"); !errors.Is(err, ErrTakeoverGracePending) {
		t.Fatalf("a wall-clock jump satisfied the drain grace: %v", err)
	}
}

func TestRuntimeLeadershipOpensOnlyAfterCacheSyncAndGrace(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	synced, syncStarted := make(chan struct{}), make(chan struct{})
	var once sync.Once
	session.WithCacheSync(func(ctx context.Context) bool {
		once.Do(func() { close(syncStarted) })
		select {
		case <-synced:
			return true
		case <-ctx.Done():
			return false
		}
	})
	// Even with the grace already satisfied, admission stays closed until the
	// informers report synchronized.
	clock.advance(limits.MinTakeoverGrace)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- session.Start(ctx) }()
	select {
	case <-syncStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the session never waited for the informer caches to synchronize")
	}

	select {
	case <-session.Ready():
		t.Fatal("admission opened before the informer caches synchronized")
	case <-time.After(50 * time.Millisecond):
	}
	if _, _, err := session.Admit("addon"); !errors.Is(err, ErrCachesNotSynced) {
		t.Fatalf("Admit before the caches synchronized returned %v", err)
	}
	close(synced)
	select {
	case <-session.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("admission never opened after sync and grace")
	}
	if _, _, err := session.Admit("addon"); err != nil {
		t.Fatalf("Admit after sync and grace: %v", err)
	}
	cancel()
	if err := <-stopped; err != nil {
		t.Fatalf("graceful stop returned %v", err)
	}
}

func TestStartWaitsTheFullGraceBeforeOpening(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- session.Start(ctx) }()
	select {
	case <-session.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("session never opened admission")
	}
	// The fake clock only advances while the session sleeps, so reaching Ready
	// proves the session waited out the whole grace rather than skipping it.
	if elapsed := clock.since(time.Time{}); elapsed < limits.MinTakeoverGrace {
		t.Fatalf("opened after %s of monotonic time, want at least %s", elapsed, limits.MinTakeoverGrace)
	}
	cancel()
	if err := <-stopped; err != nil {
		t.Fatalf("graceful stop returned %v", err)
	}
}

func TestRuntimeLeadershipInvalidatesPersistedEpochs(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	beginSyncedSession(t, session)
	clock.advance(limits.MinTakeoverGrace)
	live := heldLease(session)
	persisted := &fathomv1alpha1.DefinitionLeaderEpoch{
		LeaseUID:         string(live.UID),
		HolderIdentity:   *live.Spec.HolderIdentity,
		AcquireTime:      *live.Spec.AcquireTime,
		LeaseTransitions: *live.Spec.LeaseTransitions,
	}

	// Acquisition distrusts every acknowledgement written before this session
	// observed the live Lease for itself.
	if session.EpochValid(persisted) {
		t.Fatal("a persisted epoch was trusted before the live Lease was observed")
	}
	if _, err := session.AcknowledgeDrain("addon"); !errors.Is(err, ErrLeaderEpochUnknown) {
		t.Fatalf("drain acknowledgement before epoch adoption returned %v", err)
	}
	if _, err := session.AdoptLease(live); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	if !session.EpochValid(persisted) {
		t.Fatal("the adopted epoch does not validate its own persisted acknowledgement")
	}

	otherIdentity := "another-operator_42"
	newerAcquire := metav1.NewMicroTime(live.Spec.AcquireTime.Add(time.Second))
	moreTransitions := *live.Spec.LeaseTransitions + 1
	zeroDuration := int32(0)
	for _, tc := range []struct {
		name   string
		mutate func(*fathomv1alpha1.DefinitionLeaderEpoch)
	}{
		{name: "recreated lease uid", mutate: func(e *fathomv1alpha1.DefinitionLeaderEpoch) { e.LeaseUID = "recreated" }},
		{name: "restarted or handed-over holder", mutate: func(e *fathomv1alpha1.DefinitionLeaderEpoch) { e.HolderIdentity = otherIdentity }},
		{name: "changed acquire time", mutate: func(e *fathomv1alpha1.DefinitionLeaderEpoch) { e.AcquireTime = newerAcquire }},
		{name: "changed lease transitions", mutate: func(e *fathomv1alpha1.DefinitionLeaderEpoch) { e.LeaseTransitions = moreTransitions }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stale := *persisted
			tc.mutate(&stale)
			if session.EpochValid(&stale) {
				t.Fatalf("%s still validated against the live epoch", tc.name)
			}
		})
	}
	if session.EpochValid(nil) {
		t.Fatal("a missing epoch must never validate")
	}

	// Ordinary renewal changes renewTime and resourceVersion only; it must not
	// invalidate the acknowledgement written for this epoch.
	renewed := live.DeepCopy()
	renewedAt := metav1.NewMicroTime(live.Spec.RenewTime.Add(2 * time.Second))
	renewed.Spec.RenewTime = &renewedAt
	renewed.ResourceVersion = "101"
	if _, err := session.AdoptLease(renewed); err != nil {
		t.Fatalf("AdoptLease after renewal: %v", err)
	}
	if !session.EpochValid(persisted) {
		t.Fatal("ordinary Lease renewal invalidated a still-current acknowledgement")
	}

	for _, tc := range []struct {
		name   string
		mutate func(*coordinationv1.Lease)
	}{
		{name: "lease in another namespace", mutate: func(l *coordinationv1.Lease) { l.Namespace = "elsewhere" }},
		{name: "lease with another election id", mutate: func(l *coordinationv1.Lease) { l.Name = "someone-elses.skaphos.io" }},
		{name: "lease held by another process", mutate: func(l *coordinationv1.Lease) { l.Spec.HolderIdentity = &otherIdentity }},
		{name: "lease without a holder", mutate: func(l *coordinationv1.Lease) { l.Spec.HolderIdentity = nil }},
		{name: "lease without an acquire time", mutate: func(l *coordinationv1.Lease) { l.Spec.AcquireTime = nil }},
		{name: "lease without a renew time", mutate: func(l *coordinationv1.Lease) { l.Spec.RenewTime = nil }},
		{name: "lease without a positive duration", mutate: func(l *coordinationv1.Lease) { l.Spec.LeaseDurationSeconds = &zeroDuration }},
		{name: "lease without a uid", mutate: func(l *coordinationv1.Lease) { l.UID = "" }},
		{name: "lease being deleted", mutate: func(l *coordinationv1.Lease) {
			deleted := metav1.Now()
			l.DeletionTimestamp = &deleted
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := live.DeepCopy()
			tc.mutate(candidate)
			if _, err := session.AdoptLease(candidate); err == nil {
				t.Fatalf("%s was adopted as this session's leader epoch", tc.name)
			}
		})
	}
}

func TestDrainAcknowledgementRequiresRevokedAdmissionAndZeroActiveRuns(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	beginSyncedSession(t, session)
	if _, err := session.AdoptLease(heldLease(session)); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	clock.advance(limits.MinTakeoverGrace)

	if _, err := session.AcknowledgeDrain("addon"); !errors.Is(err, ErrAdmissionOpen) {
		t.Fatalf("drain acknowledged while admission was still open: %v", err)
	}
	work, release, err := session.Admit("addon")
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	session.Revoke("addon")
	if work.Err() == nil {
		t.Fatal("revocation did not cancel in-flight work for the binding")
	}
	if _, _, err := session.Admit("addon"); !errors.Is(err, ErrAdmissionRevoked) {
		t.Fatalf("revoked binding still admitted new work: %v", err)
	}
	if session.ActiveRuns("addon") != 1 {
		t.Fatalf("activeRuns=%d want 1 before the cancelled run is released", session.ActiveRuns("addon"))
	}
	if _, err := session.AcknowledgeDrain("addon"); !errors.Is(err, ErrRunsActive) {
		t.Fatalf("drain acknowledged with an unreleased run: %v", err)
	}

	release()
	if session.ActiveRuns("addon") != 0 {
		t.Fatalf("activeRuns=%d want 0 after release", session.ActiveRuns("addon"))
	}
	ack, err := session.AcknowledgeDrain("addon")
	if err != nil {
		t.Fatalf("AcknowledgeDrain: %v", err)
	}
	if ack.Epoch.HolderIdentity != session.Identity() || ack.Epoch.LeaseUID != "lease-uid" {
		t.Fatalf("acknowledgement names epoch %+v, not this session's", ack.Epoch)
	}
	// The acknowledgement is an observation, not a fence: it must carry that
	// limitation rather than claim exclusive execution.
	if !strings.Contains(strings.ToLower(ack.Limitation), "observ") {
		t.Fatalf("acknowledgement limitation %q does not preserve the observation-based limitation", ack.Limitation)
	}

	// Re-authorization reopens admission for the binding.
	session.Restore("addon")
	readmitted, release, err := session.Admit("addon")
	if err != nil {
		t.Fatalf("Admit after re-authorization: %v", err)
	}
	if readmitted.Err() != nil {
		t.Fatal("re-authorized work started already cancelled")
	}
	release()
}

func TestLeadershipLossTerminatesWithoutReacquisition(t *testing.T) {
	session, clock, exits := newTestLeadership(t)
	beginSyncedSession(t, session)
	if _, err := session.AdoptLease(heldLease(session)); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	clock.advance(limits.MinTakeoverGrace)
	work, release, err := session.Admit("addon")
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}

	terminated := make(chan struct{})
	go func() {
		session.OnStoppedLeading()
		close(terminated)
	}()

	select {
	case <-work.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("leadership loss did not cancel in-flight work")
	}
	if _, _, err := session.Admit("other"); !errors.Is(err, ErrLeadershipEnded) {
		t.Fatalf("admission stayed open after leadership loss: %v", err)
	}
	if _, err := session.AcknowledgeDrain("addon"); !errors.Is(err, ErrLeadershipEnded) {
		t.Fatalf("drain acknowledged after leadership loss: %v", err)
	}
	select {
	case <-terminated:
		t.Fatal("the process terminated before in-flight work was released")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case <-terminated:
	case <-time.After(5 * time.Second):
		t.Fatal("leadership loss never terminated the process")
	}
	if got := exits.observed(); len(got) != 1 || got[0] == 0 {
		t.Fatalf("exit codes %v want exactly one non-zero termination", got)
	}

	// No reacquisition in this process, and a second callback cannot exit twice.
	// The restart context is already cancelled so a session that wrongly
	// restarts fails the assertion instead of blocking on a live session.
	restart, cancelRestart := context.WithCancel(t.Context())
	cancelRestart()
	if err := session.Start(restart); !errors.Is(err, ErrLeadershipEnded) {
		t.Fatalf("the session restarted after losing leadership: %v", err)
	}
	session.OnStoppedLeading()
	if got := exits.observed(); len(got) != 1 {
		t.Fatalf("exit codes %v want exactly one termination", got)
	}
	if session.EpochValid(session.Epoch()) {
		t.Fatal("the lost session still validated its own epoch")
	}
}

func TestGracefulShutdownClosesAdmissionWithoutTerminating(t *testing.T) {
	session, _, exits := newTestLeadership(t)
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- session.Start(ctx) }()
	select {
	case <-session.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("session never opened admission")
	}
	epoch, err := session.AdoptLease(heldLease(session))
	if err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	work, release, err := session.Admit("addon")
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	release()

	cancel()
	if err := <-stopped; err != nil {
		t.Fatalf("graceful stop returned %v", err)
	}
	if work.Err() == nil {
		t.Fatal("shutdown left the admitted work context alive")
	}
	if _, _, err := session.Admit("addon"); !errors.Is(err, ErrLeadershipEnded) {
		t.Fatalf("shutdown left admission open: %v", err)
	}
	// The stopped session is no longer the authority for its own epoch.
	if session.EpochValid(epoch) {
		t.Fatal("a stopped session still validated its epoch")
	}
	session.Revoke("addon")
	if _, err := session.AcknowledgeDrain("addon"); !errors.Is(err, ErrLeadershipEnded) {
		t.Fatalf("a stopped session acknowledged a drain: %v", err)
	}
	// controller-runtime stops leader-election runnables before it cancels the
	// leader elector, so the callback that follows a graceful stop is shutdown,
	// not leadership loss, and must not exit non-zero.
	session.OnStoppedLeading()
	if got := exits.observed(); len(got) != 0 {
		t.Fatalf("graceful shutdown terminated the process with %v", got)
	}
}

func TestRuntimeLeadershipRunsOnlyAsAnElectedRunnable(t *testing.T) {
	session, _, _ := newTestLeadership(t)
	if !session.NeedLeaderElection() {
		t.Fatal("the runtime admission runnable must be gated on leader election")
	}
}

// The cache-sync gate is the one prerequisite a caller supplies rather than the
// constructor: an unwired session must refuse everything instead of assuming
// caches it never observed. The default may not be a no-op that silently
// satisfies "starts only after election and cache sync".
func TestUnwiredCacheSyncGateRefusesToStartAndToAdmit(t *testing.T) {
	session, err := NewRuntimeLeadership(runtimeLeadershipOptions())
	if err != nil {
		t.Fatalf("NewRuntimeLeadership: %v", err)
	}
	if session.cacheSync() != nil {
		t.Fatal("a freshly constructed session already claims a cache-sync gate; a T047 that forgets WithCacheSync would satisfy the cache-sync contract with a no-op")
	}
	// Start must refuse immediately. It is asserted with a bound rather than by
	// waiting: a session that runs on an assumed cache sync blocks for the whole
	// grace and then for the life of its context, which is a hang, not a result.
	startCtx, cancelStart := context.WithCancel(t.Context())
	defer cancelStart()
	refused := make(chan error, 1)
	go func() { refused <- session.Start(startCtx) }()
	select {
	case err := <-refused:
		if !errors.Is(err, ErrCacheSyncUnwired) {
			t.Fatalf("Start of an unwired session returned %v, want ErrCacheSyncUnwired", err)
		}
	case <-time.After(5 * time.Second):
		cancelStart()
		t.Fatal("Start of an unwired session did not refuse; it is running against caches it never observed")
	}
	if sessionStarted(session) {
		t.Fatal("an unwired session recorded an acquisition")
	}

	// Even hand-driven past acquisition, sync and grace, an unwired session
	// admits nothing: the refusal names the wiring defect.
	clock := newFakeClock()
	session.now, session.elapsedSince, session.sleep = clock.now, clock.since, clock.sleep
	session.exit = (&exitRecorder{}).exit
	beginSyncedSession(t, session)
	clock.advance(limits.MinTakeoverGrace)
	if _, _, err := session.Admit("addon"); !errors.Is(err, ErrCacheSyncUnwired) {
		t.Fatalf("an unwired session admitted work (%v), want ErrCacheSyncUnwired", err)
	}
	live := heldLease(session)
	if _, err := session.AdoptLease(live); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	session.Revoke("addon")
	if _, err := session.AcknowledgeDrain("addon"); !errors.Is(err, ErrCacheSyncUnwired) {
		t.Fatalf("an unwired session acknowledged a drain (%v), want ErrCacheSyncUnwired", err)
	}
	if session.EpochValid(epochOf(live)) {
		t.Fatal("an unwired session validated a persisted acknowledgement")
	}

	// Wiring the gate is what opens the session.
	session.WithCacheSync(func(context.Context) bool { return true })
	work, release, err := session.Admit("addon-two")
	if err != nil {
		t.Fatalf("Admit after the cache-sync gate was wired: %v", err)
	}
	if work.Err() != nil {
		t.Fatal("admitted work started already cancelled")
	}
	release()
}

// The contract states the grace as a literal 30 seconds covering the maximum
// prior runtime lifetime. This file is the first non-test reader of the
// constant, so it pins the figure and not just the symbol.
func TestTakeoverGraceHonoursTheContractMinimum(t *testing.T) {
	if limits.MinTakeoverGrace < 30*time.Second {
		t.Fatalf("MinTakeoverGrace=%s is below the 30s the contract requires", limits.MinTakeoverGrace)
	}
	session, _, _ := newTestLeadership(t)
	if session.grace != limits.MinTakeoverGrace {
		t.Fatalf("session grace=%s want %s", session.grace, limits.MinTakeoverGrace)
	}
}

// Acquisition distrusts persisted acknowledgements. Observing the live Lease is
// necessary but not sufficient: the caches must be populated and the whole
// monotonic grace covering the prior runtime lifetime must have elapsed before
// anything carried over in binding status is trusted.
func TestPersistedAcknowledgementsAreDistrustedUntilTheSessionIsAdmissible(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	beginSyncedSession(t, session)
	live := heldLease(session)
	persisted := epochOf(live)

	if session.EpochValid(persisted) {
		t.Fatal("a persisted acknowledgement was trusted before the live Lease was observed")
	}
	if _, err := session.AdoptLease(live); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	if session.EpochValid(persisted) {
		t.Fatal("a persisted acknowledgement was trusted during the takeover grace, before this session could have superseded the prior runtime lifetime")
	}
	clock.advance(limits.MinTakeoverGrace - time.Nanosecond)
	if session.EpochValid(persisted) {
		t.Fatal("a persisted acknowledgement was trusted one nanosecond short of the takeover grace")
	}
	clock.advance(time.Nanosecond)
	if !session.EpochValid(persisted) {
		t.Fatal("the session refuses its own epoch after sync and the full grace")
	}

	// Cache sync is the other half of admissibility: a session that never saw
	// populated definition and binding caches trusts nothing either.
	unsynced, unsyncedClock, _ := newTestLeadership(t)
	if err := unsynced.begin(t.Context()); err != nil {
		t.Fatalf("begin: %v", err)
	}
	unsyncedClock.advance(limits.MinTakeoverGrace)
	unsyncedLive := heldLease(unsynced)
	if _, err := unsynced.AdoptLease(unsyncedLive); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	if unsynced.EpochValid(epochOf(unsyncedLive)) {
		t.Fatal("a persisted acknowledgement was trusted before the informer caches synchronized")
	}
}

// Revocation, restoration and active-run accounting are per binding: the drain
// contract's "observing activeRuns=0" is a statement about one binding, and
// re-enabling one binding must not reopen admission for another revoked one.
func TestRevocationAndActiveRunAccountingAreScopedPerBinding(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	beginSyncedSession(t, session)
	if _, err := session.AdoptLease(heldLease(session)); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	clock.advance(limits.MinTakeoverGrace)

	firstA, releaseFirstA, err := session.Admit("addon-a")
	if err != nil {
		t.Fatalf("Admit addon-a: %v", err)
	}
	secondA, releaseSecondA, err := session.Admit("addon-a")
	if err != nil {
		t.Fatalf("Admit addon-a again: %v", err)
	}
	onlyB, releaseB, err := session.Admit("addon-b")
	if err != nil {
		t.Fatalf("Admit addon-b: %v", err)
	}
	for _, tc := range []struct {
		key  string
		want int
	}{{"addon-a", 2}, {"addon-b", 1}, {"addon-c", 0}} {
		if got := session.ActiveRuns(tc.key); got != tc.want {
			t.Fatalf("ActiveRuns(%q)=%d want %d; active runs are counted per binding, not per session", tc.key, got, tc.want)
		}
	}

	session.Revoke("addon-a")
	for i, work := range []context.Context{firstA, secondA} {
		select {
		case <-work.Done():
		case <-time.After(5 * time.Second):
			t.Fatalf("run %d of the revoked binding was left running; revocation cancels every in-flight run for that binding", i)
		}
	}
	if onlyB.Err() != nil {
		t.Fatal("revoking one binding cancelled another binding's in-flight work")
	}
	if _, _, err := session.Admit("addon-a"); !errors.Is(err, ErrAdmissionRevoked) {
		t.Fatalf("the revoked binding still admitted work: %v", err)
	}
	stillOpen, releaseStillOpen, err := session.Admit("addon-b")
	if err != nil {
		t.Fatalf("revoking one binding closed admission for another: %v", err)
	}
	releaseStillOpen()
	if stillOpen.Err() == nil {
		t.Fatal("release did not cancel the work it released")
	}
	// Cancellation is not release: the runs still count against their binding.
	if got := session.ActiveRuns("addon-a"); got != 2 {
		t.Fatalf("ActiveRuns(addon-a)=%d want 2 while both cancelled runs are unreleased", got)
	}

	releaseFirstA()
	if got := session.ActiveRuns("addon-a"); got != 1 {
		t.Fatalf("ActiveRuns(addon-a)=%d want 1 after one of two runs was released", got)
	}
	releaseSecondA()
	if got := session.ActiveRuns("addon-a"); got != 0 {
		t.Fatalf("ActiveRuns(addon-a)=%d want 0 after both runs were released", got)
	}
	if got := session.ActiveRuns("addon-b"); got != 1 {
		t.Fatalf("ActiveRuns(addon-b)=%d want 1; addon-b's run is untouched by addon-a's releases", got)
	}
	// addon-b still holds a run, so a session-wide count would wrongly block the
	// drained binding's acknowledgement here.
	if _, err := session.AcknowledgeDrain("addon-a"); err != nil {
		t.Fatalf("a drained binding was refused because another binding still had work: %v", err)
	}

	session.Revoke("addon-b")
	session.Restore("addon-a")
	readmitted, releaseReadmitted, err := session.Admit("addon-a")
	if err != nil {
		t.Fatalf("Admit after re-authorization: %v", err)
	}
	if readmitted.Err() != nil {
		t.Fatal("re-authorized work started already cancelled")
	}
	releaseReadmitted()
	if _, _, err := session.Admit("addon-b"); !errors.Is(err, ErrAdmissionRevoked) {
		t.Fatalf("re-authorizing addon-a reopened admission for revoked addon-b: %v", err)
	}
	releaseB()
	if _, err := session.AcknowledgeDrain("addon-b"); err != nil {
		t.Fatalf("re-authorizing addon-a retired addon-b's drain eligibility: %v", err)
	}
}

// Termination on leadership loss is gated on the work actually unwinding, and
// bounded when it does not. Both halves matter: a drain that never observes a
// release turns "terminate on loss" into "terminate one full run-duration after
// loss", widening the overlap with the next leader's takeover grace.
func TestWorkDrainGatesTerminationAndIsBounded(t *testing.T) {
	t.Run("terminates as soon as the work is released", func(t *testing.T) {
		session, clock, exits := newTestLeadership(t)
		// A bound far longer than the assertion window: reaching termination
		// proves the drain observed the release instead of waiting the bound out.
		session.workDrainTimeout = time.Minute
		beginSyncedSession(t, session)
		clock.advance(limits.MinTakeoverGrace)
		work, release, err := session.Admit("addon")
		if err != nil {
			t.Fatalf("Admit: %v", err)
		}

		terminated := make(chan struct{})
		go func() {
			session.OnStoppedLeading()
			close(terminated)
		}()
		select {
		case <-work.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("leadership loss did not cancel in-flight work")
		}
		select {
		case <-terminated:
			t.Fatal("the process terminated before in-flight work was released")
		case <-time.After(50 * time.Millisecond):
		}

		release()
		select {
		case <-terminated:
		case <-time.After(10 * time.Second):
			t.Fatalf("the work drain never observed the release, so termination stalls for the whole %s bound", session.workDrainTimeout)
		}
		if got := exits.observed(); len(got) != 1 || got[0] == 0 {
			t.Fatalf("exit codes %v want exactly one non-zero termination", got)
		}
	})

	t.Run("bounded when the work ignores cancellation", func(t *testing.T) {
		session, clock, exits := newTestLeadership(t)
		session.workDrainTimeout = 250 * time.Millisecond
		beginSyncedSession(t, session)
		clock.advance(limits.MinTakeoverGrace)
		// Admitted and never released: work that ignores its cancellation.
		if _, _, err := session.Admit("addon"); err != nil {
			t.Fatalf("Admit: %v", err)
		}

		terminated := make(chan struct{})
		started := time.Now()
		go func() {
			session.OnStoppedLeading()
			close(terminated)
		}()
		select {
		case <-terminated:
		case <-time.After(10 * time.Second):
			t.Fatalf("work that ignored cancellation stalled termination past the %s drain bound", session.workDrainTimeout)
		}
		if waited := time.Since(started); waited < session.workDrainTimeout {
			t.Fatalf("termination waited %s, less than the %s drain bound: unreleased work was not awaited", waited, session.workDrainTimeout)
		}
		if got := exits.observed(); len(got) != 1 || got[0] == 0 {
			t.Fatalf("exit codes %v want exactly one non-zero termination", got)
		}
	})

	// A graceful stop is the shutdown path that does NOT terminate the process,
	// so nothing downstream forces the work to unwind. If it skipped the drain,
	// manager shutdown would race in-flight evaluator work instead of awaiting it.
	t.Run("a graceful stop also awaits the drain", func(t *testing.T) {
		session, clock, _ := newTestLeadership(t)
		session.workDrainTimeout = 250 * time.Millisecond
		beginSyncedSession(t, session)
		clock.advance(limits.MinTakeoverGrace)
		// Admitted and never released: the drain can only end on its bound.
		if _, _, err := session.Admit("addon"); err != nil {
			t.Fatalf("Admit: %v", err)
		}

		stopped := make(chan struct{})
		started := time.Now()
		go func() {
			session.stopGracefully()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			// Bounded rather than left to the package test deadline: an
			// unbounded drain must fail this assertion, not hang the suite.
			t.Fatalf("stopGracefully stalled past the %s drain bound", session.workDrainTimeout)
		}
		if waited := time.Since(started); waited < session.workDrainTimeout {
			t.Fatalf("stopGracefully returned after %s, less than the %s drain bound: it did not await the work", waited, session.workDrainTimeout)
		}
	})
}

// A cancelled session context closes admission immediately — before the deferred
// graceful stop has recorded the end. Work admitted in that window would be born
// cancelled, and a drain acknowledged in it would speak for a session the
// manager has already torn down.
func TestAdmissionClosesAsSoonAsTheSessionContextIsCancelled(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	ctx, cancel := context.WithCancel(t.Context())
	if err := session.begin(ctx); err != nil {
		t.Fatalf("begin: %v", err)
	}
	session.markSynced()
	clock.advance(limits.MinTakeoverGrace)
	live := heldLease(session)
	if _, err := session.AdoptLease(live); err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	session.Revoke("addon")

	cancel()
	if sessionEnded(session) {
		t.Fatal("the session already recorded its end; this case must cover the window before the deferred stop runs")
	}
	if _, _, err := session.Admit("addon-two"); !errors.Is(err, ErrLeadershipEnded) {
		t.Fatalf("admission stayed open after the session context was cancelled: %v", err)
	}
	if _, err := session.AcknowledgeDrain("addon"); !errors.Is(err, ErrLeadershipEnded) {
		t.Fatalf("a cancelled session acknowledged a drain: %v", err)
	}
	if session.EpochValid(epochOf(live)) {
		t.Fatal("a cancelled session still validated a persisted acknowledgement")
	}
}

// AdoptLease is the only way an epoch enters the session, so it must refuse
// outside a live session: a not-yet-started or already-ended session that
// adopted an epoch would hold authority evidence it cannot have observed.
func TestAdoptLeaseRefusesOutsideALiveSession(t *testing.T) {
	t.Run("before the session starts", func(t *testing.T) {
		session, _, _ := newTestLeadership(t)
		if _, err := session.AdoptLease(heldLease(session)); !errors.Is(err, ErrLeadershipNotHeld) {
			t.Fatalf("AdoptLease before acquisition returned %v, want ErrLeadershipNotHeld", err)
		}
		if epoch := session.Epoch(); epoch != nil {
			t.Fatalf("a session that never acquired leadership holds epoch %+v", epoch)
		}
	})

	// A denied or missing uncached Lease read reaches this seam as no Lease at
	// all. It must not become an epoch, and it must not panic the operator.
	t.Run("without a Lease", func(t *testing.T) {
		session, clock, _ := newTestLeadership(t)
		beginSyncedSession(t, session)
		clock.advance(limits.MinTakeoverGrace)

		if _, err := session.AdoptLease(nil); err == nil {
			t.Fatal("a missing or denied Lease read was adopted as this session's leader epoch")
		}
		if epoch := session.Epoch(); epoch != nil {
			t.Fatalf("a missing Lease left the session holding epoch %+v", epoch)
		}
	})

	t.Run("after a graceful stop", func(t *testing.T) {
		session, clock, _ := newTestLeadership(t)
		beginSyncedSession(t, session)
		clock.advance(limits.MinTakeoverGrace)
		session.stopGracefully()

		if _, err := session.AdoptLease(heldLease(session)); !errors.Is(err, ErrLeadershipEnded) {
			t.Fatalf("AdoptLease after a graceful stop returned %v, want ErrLeadershipEnded", err)
		}
		if epoch := session.Epoch(); epoch != nil {
			t.Fatalf("a stopped session adopted epoch %+v", epoch)
		}
	})

	t.Run("after leadership ends", func(t *testing.T) {
		session, clock, _ := newTestLeadership(t)
		beginSyncedSession(t, session)
		clock.advance(limits.MinTakeoverGrace)
		session.OnStoppedLeading()

		if _, err := session.AdoptLease(heldLease(session)); !errors.Is(err, ErrLeadershipEnded) {
			t.Fatalf("AdoptLease after leadership loss returned %v, want ErrLeadershipEnded", err)
		}
		if epoch := session.Epoch(); epoch != nil {
			t.Fatalf("a session that lost leadership adopted epoch %+v", epoch)
		}
	})
}

// Epoch and AdoptLease hand out copies. The caller that publishes status must
// not be able to edit the session's own record of the authority it observed.
func TestObservedEpochIsHandedOutAsACopy(t *testing.T) {
	session, clock, _ := newTestLeadership(t)
	beginSyncedSession(t, session)
	clock.advance(limits.MinTakeoverGrace)
	live := heldLease(session)
	adopted, err := session.AdoptLease(live)
	if err != nil {
		t.Fatalf("AdoptLease: %v", err)
	}
	persisted := epochOf(live)

	tamper := func(epoch *fathomv1alpha1.DefinitionLeaderEpoch) {
		epoch.LeaseUID = "tampered"
		epoch.HolderIdentity = "another-operator_42"
		epoch.LeaseTransitions += 100
		epoch.AcquireTime = metav1.NewMicroTime(epoch.AcquireTime.Add(time.Hour))
	}
	for _, tc := range []struct {
		name  string
		epoch *fathomv1alpha1.DefinitionLeaderEpoch
	}{
		{name: "the value AdoptLease returned", epoch: adopted},
		{name: "the value Epoch returned", epoch: session.Epoch()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.epoch == nil {
				t.Fatal("no epoch to tamper with")
			}
			tamper(tc.epoch)
			if !session.EpochValid(persisted) {
				t.Fatalf("mutating %s rewrote the session's authority record", tc.name)
			}
			observed := session.Epoch()
			if observed == nil {
				t.Fatal("the session lost its epoch")
			}
			if observed.LeaseUID != persisted.LeaseUID ||
				observed.HolderIdentity != persisted.HolderIdentity ||
				observed.LeaseTransitions != persisted.LeaseTransitions ||
				!observed.AcquireTime.Equal(&persisted.AcquireTime) {
				t.Fatalf("the session's epoch is now %+v, want %+v", observed, persisted)
			}
		})
	}
}
