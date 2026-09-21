/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// RuntimeLeadership is the elected session that owns runtime admission, work
// cancellation and drain acknowledgement (contracts/leadership.md, FR-010).
//
// It is deliberately a unit-testable seam rather than manager wiring: the
// manager integration (registering it as a leader-election runnable, supplying
// a resource lock built with Identity, reading the Lease through the uncached
// APIReader and handing the result to AdoptLease) lives in Run.
//
// Everything this session proves is observational. A prior holder that is
// suspended rather than stopped cannot be fenced, so neither the takeover grace
// nor a drain acknowledgement is evidence of exclusive execution — see
// ObservationLimitation.
type RuntimeLeadership struct {
	lease    types.NamespacedName
	identity string
	grace    time.Duration

	// Injected so leadership transitions are testable without a manager, an API
	// server, or a real half-minute wait.
	now              func() time.Time
	elapsedSince     func(time.Time) time.Duration
	sleep            func(context.Context, time.Duration) error
	exit             func(int)
	waitForCacheSync func(context.Context) bool
	workDrainTimeout time.Duration

	ready chan struct{}

	mu sync.Mutex
	// acquiredAt carries a monotonic reading; elapsedSince, never wall-clock
	// arithmetic, measures the takeover grace against it.
	acquiredAt time.Time
	started    bool
	synced     bool
	stopped    bool
	ended      bool
	terminated bool
	cancel     context.CancelFunc
	sessionCtx context.Context
	epoch      *fathomv1alpha1.DefinitionLeaderEpoch
	work       map[string]map[uint64]context.CancelFunc
	revoked    map[string]bool
	nextWork   uint64
	inflight   sync.WaitGroup
}

var (
	_ manager.Runnable               = (*RuntimeLeadership)(nil)
	_ manager.LeaderElectionRunnable = (*RuntimeLeadership)(nil)
)

// Session states callers distinguish. Every one of them is closed: a caller
// that cannot tell why it was refused must not execute runtime work.
var (
	// ErrLeadershipNotHeld means this process has not started an elected session.
	ErrLeadershipNotHeld = errors.New("runtime leadership session is not active")
	// ErrLeadershipEnded means leadership ended; this process never reacquires it.
	ErrLeadershipEnded = errors.New("runtime leadership ended; this process does not reacquire it")
	// ErrCachesNotSynced means the informers backing definitions and bindings
	// have not synchronized, so nothing may be evaluated from them yet.
	ErrCachesNotSynced = errors.New("runtime admission is closed until the informer caches synchronize")
	// ErrTakeoverGracePending means the monotonic takeover grace has not elapsed.
	ErrTakeoverGracePending = fmt.Errorf("runtime admission is closed until %s of monotonic takeover grace elapses", limits.MinTakeoverGrace)
	// ErrAdmissionRevoked means admission for this binding is closed pending reauthorization.
	ErrAdmissionRevoked = errors.New("runtime admission is revoked for this binding")
	// ErrAdmissionOpen means a drain was claimed while the binding could still be admitted.
	ErrAdmissionOpen = errors.New("runtime admission for this binding has not been revoked")
	// ErrRunsActive means this session still holds unreleased work for the binding.
	ErrRunsActive = errors.New("this leadership session still has active runs for the binding")
	// ErrLeaderEpochUnknown means no live Lease epoch has been observed since acquisition.
	ErrLeaderEpochUnknown = errors.New("no live leader epoch has been observed for this session")
	// ErrCacheSyncUnwired means nobody supplied the informer cache-sync gate. It
	// is a wiring defect, not a runtime condition: the session refuses to start
	// and refuses admission rather than assume caches it cannot observe.
	ErrCacheSyncUnwired = errors.New("runtime leadership has no cache-sync gate wired; call WithCacheSync with the manager cache")
)

// ObservationLimitation is the caveat every drain acknowledgement carries. The
// grace period and the epoch comparison are operational observations, not
// linearizable fencing: a suspended prior process can still wake up.
const ObservationLimitation = "observed leadership only: a suspended prior holder is not fenced, so this describes the observed epoch rather than proven exclusive execution"

// maxHolderIdentityBytes mirrors the holderIdentity bound on binding status, so
// an identity this session mints always fits the epoch it will later publish.
const maxHolderIdentityBytes = 253

// osHostname is a variable so tests can supply hostile hostnames.
var osHostname = os.Hostname

// DrainAcknowledgement is the session-side evidence a drain may be published
// with: the epoch observed live by this leader, plus the limitation that epoch
// evidence carries. Callers must still verify the binding itself.
type DrainAcknowledgement struct {
	Epoch      fathomv1alpha1.DefinitionLeaderEpoch
	Limitation string
}

// NewRuntimeLeadership builds the session for opts. It fails closed when the
// static runtime-loading prerequisites do not hold (runtime loading off, leader
// election off, or no explicit operator namespace) so callers cannot run
// runtime definitions without an elected, namespaced Lease. Built-in
// controllers are unaffected: the caller logs the reason and skips runtime.
func NewRuntimeLeadership(opts Options) (*RuntimeLeadership, error) {
	if reason := opts.RuntimeLoadingDisabledReason(); reason != "" {
		return nil, fmt.Errorf("runtime leadership unavailable: %s", reason)
	}
	identity, err := processUniqueHolderIdentity()
	if err != nil {
		return nil, err
	}
	return &RuntimeLeadership{
		// Namespace and name are trusted operator configuration. They are never
		// inferred from a binding's status or from a definition.
		lease:        types.NamespacedName{Namespace: opts.Namespace, Name: opts.LeaderElectionID},
		identity:     identity,
		grace:        limits.MinTakeoverGrace,
		now:          time.Now,
		elapsedSince: time.Since,
		sleep:        sleepContext,
		exit:         os.Exit,
		// waitForCacheSync is deliberately nil: every other prerequisite in this
		// file fails closed, and a gate defaulting to "synchronized" would let a
		// caller that forgot WithCacheSync satisfy the contract with a no-op.
		workDrainTimeout: limits.MaxRunDuration,
		ready:            make(chan struct{}),
		work:             map[string]map[uint64]context.CancelFunc{},
		revoked:          map[string]bool{},
	}, nil
}

// processUniqueHolderIdentity mints an identity that is unique per process, not
// per host: two operator processes on one node must never appear as the same
// Lease holder, otherwise an old process could pass another's epoch check.
func processUniqueHolderIdentity() (string, error) {
	host, err := osHostname()
	if err != nil || host == "" {
		return "", fmt.Errorf("resolve hostname for the leader holder identity: %w", err)
	}
	unique := string(uuid.NewUUID())
	if room := maxHolderIdentityBytes - len(unique) - 1; len(host) > room {
		host = host[:room] // hostnames are ASCII, so a byte cut is a rune cut
	}
	return host + "_" + unique, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// WithCacheSync wires the informer cache-sync gate — the manager supplies
// mgr.GetCache().WaitForCacheSync. It must be called before the session is
// registered as a runnable: an unwired session fails Start and refuses every
// admission with ErrCacheSyncUnwired, so a missed wiring is loud rather than
// silently satisfied.
func (l *RuntimeLeadership) WithCacheSync(waitForCacheSync func(context.Context) bool) *RuntimeLeadership {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.waitForCacheSync = waitForCacheSync
	return l
}

func (l *RuntimeLeadership) cacheSync() func(context.Context) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.waitForCacheSync
}

// LeaseRef is the configured namespaced Lease this session observes.
func (l *RuntimeLeadership) LeaseRef() types.NamespacedName { return l.lease }

// Identity is this process's Lease holder identity. The manager must be built
// with a resource lock using exactly this identity (ctrl.Options
// LeaderElectionResourceLockInterface); otherwise AdoptLease refuses every
// Lease and runtime admission stays closed.
func (l *RuntimeLeadership) Identity() string { return l.identity }

// NeedLeaderElection keeps the runtime admission runnable out of the
// non-elected runnable group: it starts only after election and cache sync.
func (l *RuntimeLeadership) NeedLeaderElection() bool { return true }

// Ready is closed once the caches have synchronized and the takeover grace has
// elapsed. It is a notification only; Admit and AcknowledgeDrain re-check the
// grace themselves.
func (l *RuntimeLeadership) Ready() <-chan struct{} { return l.ready }

// Start runs the elected session until ctx is cancelled. It is registered with
// the manager, which starts it only after election and cache sync; Start then
// keeps admission closed for the whole takeover grace while the informers
// synchronize definitions and bindings.
//
// Returning nil is the graceful-shutdown path. Leadership *loss* arrives
// through OnStoppedLeading instead.
func (l *RuntimeLeadership) Start(ctx context.Context) error {
	waitForCacheSync := l.cacheSync()
	if waitForCacheSync == nil {
		return ErrCacheSyncUnwired
	}
	if err := l.begin(ctx); err != nil {
		return err
	}
	defer l.stopGracefully()

	sessionCtx := l.context()
	if !waitForCacheSync(sessionCtx) {
		if sessionCtx.Err() != nil {
			return nil
		}
		return errors.New("runtime leadership: informer caches did not synchronize")
	}
	l.markSynced()
	if err := l.awaitTakeoverGrace(sessionCtx); err != nil {
		return nil
	}
	close(l.ready)

	<-sessionCtx.Done()
	return nil
}

// begin records acquisition. A session begins exactly once, so it starts with no
// epoch: persisted acknowledgements are distrusted until AdoptLease observes the
// live Lease *and* the takeover grace covering the prior runtime lifetime has
// elapsed (see EpochValid). There is nothing in-process to reset here — the
// previous holder's acknowledgement lives in binding status, and only an epoch
// comparison made by an admissible session can retire it.
func (l *RuntimeLeadership) begin(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.started || l.ended {
		return ErrLeadershipEnded
	}
	l.started = true
	l.acquiredAt = l.now()
	l.sessionCtx, l.cancel = context.WithCancel(ctx)
	return nil
}

// markSynced records that the informers backing runtime definitions and
// bindings are populated. Nothing may be admitted before this.
func (l *RuntimeLeadership) markSynced() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.synced = true
}

func (l *RuntimeLeadership) context() context.Context {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sessionCtx
}

// awaitTakeoverGrace blocks until at least grace of *monotonic* time has passed
// since acquisition. Wall-clock readings are never compared: an NTP step or a
// restored VM snapshot must not shorten the window that covers the maximum
// prior runtime lifetime.
func (l *RuntimeLeadership) awaitTakeoverGrace(ctx context.Context) error {
	for {
		remaining := l.grace - l.elapsed()
		if remaining <= 0 {
			return nil
		}
		if err := l.sleep(ctx, remaining); err != nil {
			return err
		}
	}
}

func (l *RuntimeLeadership) elapsed() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.elapsedSince(l.acquiredAt)
}

// admissibleLocked reports why runtime work may not proceed right now.
func (l *RuntimeLeadership) admissibleLocked() error {
	switch {
	case l.waitForCacheSync == nil:
		return ErrCacheSyncUnwired
	case l.ended:
		return ErrLeadershipEnded
	case !l.started:
		return ErrLeadershipNotHeld
	case l.sessionCtx == nil || l.sessionCtx.Err() != nil:
		return ErrLeadershipEnded
	case !l.synced:
		return ErrCachesNotSynced
	case l.elapsedSince(l.acquiredAt) < l.grace:
		return ErrTakeoverGracePending
	}
	return nil
}

// Admit registers one unit of runtime work for key (the binding's name) and
// returns a context cancelled when leadership ends or the binding is revoked,
// plus an idempotent release. It refuses while the takeover grace is pending,
// after leadership ends, and for revoked bindings.
func (l *RuntimeLeadership) Admit(key string) (context.Context, func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.admissibleLocked(); err != nil {
		return nil, nil, err
	}
	if l.revoked[key] {
		return nil, nil, ErrAdmissionRevoked
	}
	ctx, cancel := context.WithCancel(l.sessionCtx)
	id := l.nextWork
	l.nextWork++
	if l.work[key] == nil {
		l.work[key] = map[uint64]context.CancelFunc{}
	}
	l.work[key][id] = cancel
	l.inflight.Add(1)
	return ctx, sync.OnceFunc(func() {
		cancel()
		l.release(key, id)
	}), nil
}

func (l *RuntimeLeadership) release(key string, id uint64) {
	l.mu.Lock()
	if runs := l.work[key]; runs != nil {
		delete(runs, id)
		if len(runs) == 0 {
			delete(l.work, key)
		}
	}
	l.mu.Unlock()
	l.inflight.Done()
}

// Revoke closes admission for key and cancels its in-flight work. It does not
// wait: callers observe ActiveRuns and requeue, so no reconcile blocks on a run.
func (l *RuntimeLeadership) Revoke(key string) {
	l.mu.Lock()
	l.revoked[key] = true
	cancels := make([]context.CancelFunc, 0, len(l.work[key]))
	for _, cancel := range l.work[key] {
		cancels = append(cancels, cancel)
	}
	l.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// Restore reopens admission for key after explicit reauthorization (the binding
// was re-enabled or its spec was edited). It grants nothing on its own: the
// caller must still revalidate authority before admitting work.
func (l *RuntimeLeadership) Restore(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.revoked, key)
}

// ActiveRuns counts this session's unreleased work for key.
func (l *RuntimeLeadership) ActiveRuns(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.work[key])
}

// AcknowledgeDrain returns the evidence needed to publish Drained for key. It
// requires a live session past its takeover grace, an observed leader epoch,
// revoked admission, and zero active runs in this session. It never inspects
// the binding: enabled=false, the observed generation and the uncached re-reads
// remain the caller's fences.
func (l *RuntimeLeadership) AcknowledgeDrain(key string) (DrainAcknowledgement, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.admissibleLocked(); err != nil {
		return DrainAcknowledgement{}, err
	}
	if l.epoch == nil {
		return DrainAcknowledgement{}, ErrLeaderEpochUnknown
	}
	if !l.revoked[key] {
		return DrainAcknowledgement{}, ErrAdmissionOpen
	}
	if runs := len(l.work[key]); runs > 0 {
		return DrainAcknowledgement{}, fmt.Errorf("%w: %d unreleased", ErrRunsActive, runs)
	}
	return DrainAcknowledgement{Epoch: *l.epoch, Limitation: ObservationLimitation}, nil
}

// AdoptLease records the live Lease as this session's leader epoch. The Lease
// must be the configured one, held by this process, and carry complete
// leadership evidence. Callers read it through the uncached APIReader; renewal
// is expected to change renewTime and resourceVersion, neither of which is part
// of the epoch.
func (l *RuntimeLeadership) AdoptLease(lease *coordinationv1.Lease) (*fathomv1alpha1.DefinitionLeaderEpoch, error) {
	if lease == nil {
		return nil, errors.New("no Lease to adopt")
	}
	if got := (types.NamespacedName{Namespace: lease.Namespace, Name: lease.Name}); got != l.lease {
		return nil, fmt.Errorf("lease %s is not the configured leader election Lease %s", got, l.lease)
	}
	spec := lease.Spec
	switch {
	case lease.UID == "", !lease.DeletionTimestamp.IsZero():
		return nil, errors.New("lease has no durable identity")
	case spec.HolderIdentity == nil || *spec.HolderIdentity != l.identity:
		return nil, errors.New("lease is not held by this process")
	case spec.AcquireTime == nil || spec.AcquireTime.IsZero(),
		spec.RenewTime == nil || spec.RenewTime.IsZero(),
		spec.LeaseDurationSeconds == nil || *spec.LeaseDurationSeconds <= 0,
		spec.LeaseTransitions == nil || *spec.LeaseTransitions < 0:
		return nil, errors.New("lease lacks live leadership evidence")
	}

	epoch := fathomv1alpha1.DefinitionLeaderEpoch{
		LeaseUID:         string(lease.UID),
		HolderIdentity:   *spec.HolderIdentity,
		AcquireTime:      *spec.AcquireTime,
		LeaseTransitions: *spec.LeaseTransitions,
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.ended:
		return nil, ErrLeadershipEnded
	case !l.started:
		return nil, ErrLeadershipNotHeld
	}
	l.epoch = &epoch
	adopted := epoch
	return &adopted, nil
}

// Epoch returns the observed leader epoch, or nil before one is observed.
func (l *RuntimeLeadership) Epoch() *fathomv1alpha1.DefinitionLeaderEpoch {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.epoch == nil {
		return nil
	}
	observed := *l.epoch
	return &observed
}

// EpochValid reports whether a persisted acknowledgement belongs to this
// session's observed epoch. Equality uses all four fields; resourceVersion is
// excluded because ordinary renewal changes it. A restarted holder, a recreated
// Lease and a handoff all produce a different epoch, so their acknowledgements
// stop validating — as does every acknowledgement written before this session
// observed the Lease, and every one presented after leadership ends.
//
// Acquisition distrusts persisted acknowledgements: validity demands the same
// admissibility this session demands of runtime work, so nothing carried over
// from a prior runtime lifetime is trusted until the caches have synchronized
// and the monotonic takeover grace covering that lifetime has fully elapsed.
func (l *RuntimeLeadership) EpochValid(persisted *fathomv1alpha1.DefinitionLeaderEpoch) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if persisted == nil || l.epoch == nil || l.admissibleLocked() != nil {
		return false
	}
	return l.epoch.LeaseUID == persisted.LeaseUID &&
		l.epoch.HolderIdentity == persisted.HolderIdentity &&
		l.epoch.AcquireTime.Equal(&persisted.AcquireTime) &&
		l.epoch.LeaseTransitions == persisted.LeaseTransitions
}

// OnStoppedLeading is wired into ctrl.Options.OnStoppedLeading. Losing the
// Lease closes admission, cancels in-flight work, waits a bounded interval for
// it to unwind, and terminates the process: this session must never publish
// evidence again, and this process never reacquires leadership.
//
// controller-runtime stops leader-election runnables and waits for them before
// it cancels the leader elector, so a callback that arrives after Start has
// already returned is a graceful shutdown, not a loss, and must not terminate.
func (l *RuntimeLeadership) OnStoppedLeading() {
	l.mu.Lock()
	if l.stopped || !l.started || l.terminated {
		l.mu.Unlock()
		return
	}
	l.ended = true
	l.terminated = true
	cancel := l.cancel
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	l.awaitWorkDrain()
	l.exit(1)
}

// stopGracefully ends the session when the runnable stops without a leadership
// loss. Admission closes and work is cancelled and awaited, but the process is
// left to shut down normally. The session still never restarts in this process.
func (l *RuntimeLeadership) stopGracefully() {
	l.mu.Lock()
	l.stopped = true
	l.ended = true
	cancel := l.cancel
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	l.awaitWorkDrain()
}

// awaitWorkDrain waits for cancelled work to unwind, bounded by the longest a
// single run may legally take. Work that ignores cancellation cannot stall
// shutdown past that bound.
func (l *RuntimeLeadership) awaitWorkDrain() {
	drained := make(chan struct{})
	go func() {
		l.inflight.Wait()
		close(drained)
	}()
	timer := time.NewTimer(l.workDrainTimeout)
	defer timer.Stop()
	select {
	case <-drained:
	case <-timer.C:
	}
}
