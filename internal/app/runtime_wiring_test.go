/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/internal/controller"
	"github.com/skaphos/fathom/pkg/adapter"
)

// This file covers T047's half of specs/012-addon-definition-runtime: the
// connection between the elected leadership session (the single decider) and
// the registry's runtime dispatch gate (the single enforcement point), and the
// manager wiring that hangs the runtime path off the operator's own lifecycle.
//
// Every gate is tested in BOTH directions. A test that only proves runtime
// activates when everything holds would pass just as happily against a wiring
// that activates unconditionally.

const testManagerServiceAccount = "fathom-controller-manager"

// writeManagerToken points the manager-identity probe at a synthetic projected
// token. Nothing verifies it: it states who the API server will see, and that
// is all it is used for.
func writeManagerToken(t *testing.T, subject string) {
	t.Helper()
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"sub":%q}`, subject)))
	token := "eyJhbGciOiJSUzI1NiJ9." + payload + ".not-a-signature"
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	previous := managerTokenPath
	managerTokenPath = path
	t.Cleanup(func() { managerTokenPath = previous })
}

func eligibleRuntimeOptions() Options {
	opts := DefaultOptions()
	opts.RuntimeLoading.Enabled = true
	opts.LeaderElect = true
	opts.Namespace = "fathom-system"
	return opts
}

// The manager identity is read from the operator's own token rather than
// configured, so the value that refuses a binding pointed at the operator's own
// service account cannot drift from the identity the API server actually sees.
func TestServiceAccountFromToken(t *testing.T) {
	encode := func(claims string) string {
		return "header." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".signature"
	}
	for _, tc := range []struct {
		name, token, want string
	}{
		{name: "bound token", token: encode(`{"sub":"system:serviceaccount:fathom-system:fathom-controller-manager"}`), want: testManagerServiceAccount},
		{name: "other claims are ignored", token: encode(`{"aud":["api"],"sub":"system:serviceaccount:ns:reader"}`), want: "reader"},
		{name: "not a jwt", token: "opaque-token"},
		{name: "undecodable payload", token: "header.%%%.signature"},
		{name: "not json", token: encode(`not-json`)},
		{name: "a user, not a service account", token: encode(`{"sub":"kubernetes-admin"}`)},
		{name: "no service account name", token: encode(`{"sub":"system:serviceaccount:fathom-system:"}`)},
		{name: "no namespace segment", token: encode(`{"sub":"system:serviceaccount:fathom-system"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := serviceAccountFromToken(tc.token)
			switch {
			case tc.want == "" && err == nil:
				t.Fatalf("serviceAccountFromToken(%q) = %q, want an error", tc.token, got)
			case tc.want == "":
				return
			case err != nil:
				t.Fatalf("serviceAccountFromToken: %v", err)
			case got != tc.want:
				t.Fatalf("service account = %q, want %q", got, tc.want)
			}
		})
	}
}

// Runtime loading activates only when every static prerequisite holds, and an
// unmet one is a DIAGNOSTIC, never a startup failure: "do not fail manager
// startup or disable built-ins".
func TestRuntimeWiringRequiresEveryPrerequisite(t *testing.T) {
	for _, tc := range []struct {
		name       string
		options    func(*Options)
		token      string
		wantReason string
	}{
		{
			name:       "runtime loading is off by default",
			options:    func(o *Options) { *o = DefaultOptions() },
			wantReason: "RuntimeLoadingDisabled",
		},
		{
			name:       "leader election is mandatory",
			options:    func(o *Options) { o.LeaderElect = false },
			wantReason: "LeaderElectionRequired",
		},
		{
			name:       "an explicit operator namespace is required",
			options:    func(o *Options) { o.Namespace = "" },
			wantReason: "OperatorNamespaceRequired",
		},
		{
			name:       "the operator must be able to name its own identity",
			token:      "not-a-jwt",
			wantReason: reasonAuthorizationUnavailable,
		},
		{name: "every prerequisite holds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.token != "" {
				path := filepath.Join(t.TempDir(), "token")
				if err := os.WriteFile(path, []byte(tc.token), 0o600); err != nil {
					t.Fatalf("write token: %v", err)
				}
				previous := managerTokenPath
				managerTokenPath = path
				t.Cleanup(func() { managerTokenPath = previous })
			} else {
				writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
			}
			opts := eligibleRuntimeOptions()
			if tc.options != nil {
				tc.options(&opts)
			}

			wiring, reason := newRuntimeWiring(opts)
			if tc.wantReason == "" {
				if wiring == nil {
					t.Fatalf("runtime wiring is unavailable (%s) although every prerequisite holds", reason)
				}
				if reason != "" {
					t.Errorf("reason = %q, want empty for an available runtime", reason)
				}
				if wiring.managerServiceAccount != testManagerServiceAccount {
					t.Errorf("manager service account = %q, want %q", wiring.managerServiceAccount, testManagerServiceAccount)
				}
				if wiring.session == nil || wiring.scheduler == nil || wiring.operatorBuild == "" {
					t.Errorf("wiring is incomplete: %+v", wiring)
				}
				return
			}
			if wiring != nil {
				t.Fatalf("runtime wiring was built although %s", tc.wantReason)
			}
			if !strings.HasPrefix(reason, tc.wantReason) {
				t.Fatalf("reason = %q, want it to start with %q", reason, tc.wantReason)
			}
		})
	}
}

// The session must hold the Lease under its OWN identity, in the configured
// namespace and under the configured election ID. A lock built for any other
// identity would leave the elected holder and the runtime decider as two
// different processes in the eyes of AdoptLease, and runtime would never open.
func TestLeaderElectionLockCarriesTheSessionIdentity(t *testing.T) {
	writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
	opts := eligibleRuntimeOptions()
	wiring, reason := newRuntimeWiring(opts)
	if wiring == nil {
		t.Fatalf("runtime wiring unavailable: %s", reason)
	}

	lock, err := wiring.leaderElectionLock(&rest.Config{Host: "https://example.invalid"})
	if err != nil {
		t.Fatalf("leaderElectionLock: %v", err)
	}
	if got := lock.Identity(); got != wiring.session.Identity() {
		t.Errorf("lock identity = %q, want the session's holder identity %q", got, wiring.session.Identity())
	}
	// Exact, not substring: AdoptLease refuses any Lease that is not the one it
	// names, so a lock on "<id>-election" in the right namespace would satisfy a
	// Contains check while leaving the manager holding a different object than the
	// session watches — runtime would stay closed forever with nothing to see.
	ref := wiring.session.LeaseRef()
	if got, want := lock.Describe(), ref.Namespace+"/"+ref.Name; got != want {
		t.Errorf("lock describes %q, want exactly the Lease the session adopts, %q", got, want)
	}
	if ref.Namespace != opts.Namespace || ref.Name != opts.LeaderElectionID {
		t.Errorf("session adopts %s/%s, want the configured namespace %q and election ID %q",
			ref.Namespace, ref.Name, opts.Namespace, opts.LeaderElectionID)
	}
}

// --- the gate --------------------------------------------------------------

// runtimeGateFixture drives a real leadership session and a real registry
// holding a published runtime snapshot, so what the gate does is observed
// through the enforcement point rather than through the gate's own bookkeeping.
type runtimeGateFixture struct {
	t        *testing.T
	session  *RuntimeLeadership
	clock    *fakeClock
	registry *registry.Registry
	gate     *runtimeDispatchGate
	scheme   *runtime.Scheme
}

// gateAddonType is the identity the published snapshot claims.
const gateAddonType = "gate-addon"

type gateAdapter struct{}

func (gateAdapter) Name() string            { return gateAddonType }
func (gateAdapter) Version() string         { return "1.0.0" }
func (gateAdapter) ContractVersion() string { return adapter.ContractVersion }
func (gateAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{gateAddonType}, Families: []adapter.Family{"health"}}
}
func (gateAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{}, nil
}

func newRuntimeGateFixture(t *testing.T, leases ...client.Object) *runtimeGateFixture {
	t.Helper()
	session, clock, _ := newTestLeadership(t)
	reg := registry.New(logr.Discard())
	if err := reg.SetRuntime(registry.RuntimeEntry{
		Adapter: gateAdapter{},
		Revision: registry.RuntimeRevision{
			DefinitionUID: "definition-uid", Generation: 1,
			SchemaVersion: fathomv1alpha1.GroupVersion.Version, SemanticsVersion: 1,
		},
		Provenance: registry.RuntimeProvenance{OperatorBuild: "test", AdapterVersion: "1.0.0"},
	}); err != nil {
		t.Fatalf("publish runtime snapshot: %v", err)
	}
	scheme := runtime.NewScheme()
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if len(leases) == 0 {
		leases = []client.Object{heldLease(session)}
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(leases...).Build()
	return &runtimeGateFixture{
		t: t, session: session, clock: clock, registry: reg, scheme: scheme,
		gate: &runtimeDispatchGate{session: session, registry: reg, reader: reader, log: logr.Discard(), retry: time.Millisecond},
	}
}

// admitted reports what the enforcement point currently allows.
func (f *runtimeGateFixture) admitted() bool {
	f.t.Helper()
	resolution, err := f.registry.Resolve(gateAddonType)
	if err == nil {
		return resolution.Runtime
	}
	var barrier *registry.DispatchBarrier
	if !errors.As(err, &barrier) {
		f.t.Fatalf("resolve %q: %v", gateAddonType, err)
	}
	if barrier.Reason != registry.ReasonAdmissionClosed {
		f.t.Fatalf("barrier reason = %q, want %q", barrier.Reason, registry.ReasonAdmissionClosed)
	}
	return false
}

// startSession runs the session the way the manager does and returns a cancel.
func (f *runtimeGateFixture) startSession(ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() { done <- f.session.Start(ctx) }()
	return done
}

func (f *runtimeGateFixture) startGate(ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() { done <- f.gate.Start(ctx) }()
	return done
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The keystone. Dispatch is barred until the session is admissible, admitted
// under the session's own holder identity once it is, and barred again the
// moment the session ends.
func TestRuntimeDispatchGateIsDrivenByTheLeadershipSession(t *testing.T) {
	f := newRuntimeGateFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	if f.admitted() {
		t.Fatal("runtime dispatch is admitted before any session started; the gate must start closed")
	}
	gateDone := f.startGate(ctx)
	// The gate is running but the session is not: nothing may open.
	time.Sleep(20 * time.Millisecond)
	if f.admitted() {
		t.Fatal("runtime dispatch was admitted without an admissible leadership session")
	}

	sessionDone := f.startSession(ctx)
	waitFor(t, "runtime dispatch to be admitted", f.admitted)

	cancel()
	if err := <-gateDone; err != nil {
		t.Fatalf("gate: %v", err)
	}
	if err := <-sessionDone; err != nil {
		t.Fatalf("session: %v", err)
	}
	if f.admitted() {
		t.Fatal("runtime dispatch is still admitted after the leadership session ended")
	}
}

// Both directions of the three prerequisites the gate cannot open without.
// Each row leaves the session permanently inadmissible for exactly one reason.
func TestRuntimeDispatchGateStaysClosedWithoutElectionSyncOrGrace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		breaks func(*runtimeGateFixture) (startSession bool)
	}{
		{
			name:   "the session never starts (this process is not the leader)",
			breaks: func(*runtimeGateFixture) bool { return false },
		},
		{
			name: "the informer caches never synchronize",
			breaks: func(f *runtimeGateFixture) bool {
				f.session.WithCacheSync(func(ctx context.Context) bool { <-ctx.Done(); return false })
				return true
			},
		},
		{
			name: "the cache-sync gate was never wired",
			breaks: func(f *runtimeGateFixture) bool {
				f.session.WithCacheSync(nil)
				return true
			},
		},
		{
			name: "the monotonic takeover grace has not elapsed",
			breaks: func(f *runtimeGateFixture) bool {
				f.session.sleep = func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() }
				return true
			},
		},
		{
			name: "no live Lease can be adopted",
			breaks: func(f *runtimeGateFixture) bool {
				f.gate.reader = fake.NewClientBuilder().WithScheme(f.scheme).Build()
				return true
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeGateFixture(t)
			start := tc.breaks(f)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			gateDone := f.startGate(ctx)
			var sessionDone <-chan error
			if start {
				sessionDone = f.startSession(ctx)
			}
			time.Sleep(50 * time.Millisecond)
			if f.admitted() {
				t.Fatalf("runtime dispatch was admitted although %s", tc.name)
			}
			cancel()
			<-gateDone
			if sessionDone != nil {
				<-sessionDone
			}
			if f.admitted() {
				t.Fatal("runtime dispatch is admitted after shutdown")
			}
		})
	}
}

// The pool may not exist outside an admissible session: no worker, no run.
func TestRuntimeWorkerPoolRunsOnlyInsideAnAdmissibleSession(t *testing.T) {
	for _, admissible := range []bool{false, true} {
		name := "inadmissible session"
		if admissible {
			name = "admissible session"
		}
		t.Run(name, func(t *testing.T) {
			session, _, _ := newTestLeadership(t)
			if !admissible {
				session.WithCacheSync(func(ctx context.Context) bool { <-ctx.Done(); return false })
			}
			var calls int64
			var mu sync.Mutex
			scheduler := execution.NewScheduler(func() float64 { return 0 })
			pool := &runtimeWorkerPool{
				session:   session,
				scheduler: scheduler,
				handle: func(context.Context, execution.Work) execution.Disposition {
					mu.Lock()
					calls++
					mu.Unlock()
					return execution.Completed
				},
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			poolDone := make(chan error, 1)
			go func() { poolDone <- pool.Start(ctx) }()
			sessionDone := make(chan error, 1)
			go func() { sessionDone <- session.Start(ctx) }()

			work := execution.Work{Definition: "an-addon", Check: types.NamespacedName{Namespace: "ns", Name: "check"}}
			if err := scheduler.Enqueue(work, time.Now()); err != nil {
				t.Fatalf("enqueue: %v", err)
			}
			observed := func() int64 {
				mu.Lock()
				defer mu.Unlock()
				return calls
			}
			if admissible {
				waitFor(t, "the pool to admit the enqueued run", func() bool { return observed() > 0 })
			} else {
				time.Sleep(50 * time.Millisecond)
				if got := observed(); got != 0 {
					t.Fatalf("the pool ran %d units of work outside an admissible session", got)
				}
			}
			cancel()
			<-poolDone
			<-sessionDone
		})
	}
}

// --- manager wiring --------------------------------------------------------

// recordingManager counts what is registered against a real manager, so
// "nothing runtime-related was registered" is OBSERVED rather than asserted
// from the shape of the code.
//
// Its cache and its client are recording wrappers, because two of this file's
// properties are about IDENTITY rather than shape: the cache-sync gate handed
// to the session and to both runtime reconcilers must be THE MANAGER'S cache
// (a constant-true stand-in satisfies every non-nil assertion while promising a
// synchronization nobody waited for), and the worker pool's handler must be the
// AddonCheck reconciler's own RunRuntimeWork, which is observable because it is
// the only thing in the process that reads the enqueued AddonCheck.
type recordingManager struct {
	ctrl.Manager
	mu      sync.Mutex
	added   []manager.Runnable
	indexes []string

	syncCache *recordingCache
	recClient *recordingClient
}

func (m *recordingManager) Add(r manager.Runnable) error {
	m.mu.Lock()
	m.added = append(m.added, r)
	m.mu.Unlock()
	return m.Manager.Add(r)
}

func (m *recordingManager) GetFieldIndexer() client.FieldIndexer {
	return recordingIndexer{m: m, inner: m.Manager.GetFieldIndexer()}
}

func (m *recordingManager) GetCache() cache.Cache { return m.syncCache }

func (m *recordingManager) GetClient() client.Client { return m.recClient }

// recordingCache makes the manager's own cache-sync gate observable. The wiring
// must hand the session and both runtime reconcilers this cache's
// WaitForCacheSync; calling the wired function is what proves it.
type recordingCache struct {
	cache.Cache
	mu    sync.Mutex
	calls int
}

func (c *recordingCache) WaitForCacheSync(ctx context.Context) bool {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.Cache.WaitForCacheSync(ctx)
}

func (c *recordingCache) syncCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// recordingClient records the AddonCheck reads made through the manager's
// client. RunRuntimeWork opens with exactly one of them, so a worker pool
// draining the shared queue with the reconciler's own handler leaves a trace
// no stand-in handler can forge.
type recordingClient struct {
	client.Client
	mu   sync.Mutex
	gets []types.NamespacedName
}

func (c *recordingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*fathomv1alpha1.AddonCheck); ok {
		c.mu.Lock()
		c.gets = append(c.gets, key)
		c.mu.Unlock()
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *recordingClient) readAddonCheck(key types.NamespacedName) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Contains(c.gets, key)
}

func (m *recordingManager) runnables() []manager.Runnable {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]manager.Runnable(nil), m.added...)
}

func (m *recordingManager) registeredIndexes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.indexes...)
}

type recordingIndexer struct {
	m     *recordingManager
	inner client.FieldIndexer
}

func (i recordingIndexer) IndexField(ctx context.Context, obj client.Object, field string, extract client.IndexerFunc) error {
	i.m.mu.Lock()
	i.m.indexes = append(i.m.indexes, fmt.Sprintf("%T/%s", obj, field))
	i.m.mu.Unlock()
	return i.inner.IndexField(ctx, obj, field, extract)
}

func newRecordingManager(t *testing.T) *recordingManager {
	t.Helper()
	if envtestCfg == nil {
		t.Skip("envtest unavailable; run via `task test` for full coverage")
	}
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	mgr, err := ctrl.NewManager(envtestCfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Cache:                  scopedCacheOptions(),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return &recordingManager{
		Manager:   mgr,
		syncCache: &recordingCache{Cache: mgr.GetCache()},
		recClient: &recordingClient{Client: mgr.GetClient()},
	}
}

func setupperTypes(setuppers []Setupper) []string {
	names := make([]string, 0, len(setuppers))
	for _, s := range setuppers {
		names = append(names, fmt.Sprintf("%T", s))
	}
	return names
}

// THE default-off property, observed end to end rather than asserted: for the
// default configuration and for every configuration that leaves runtime loading
// unavailable, the controller set is the built-in one and NOTHING else
// happened — no runnable added, no index registered, no dispatch gate opened,
// and an AddonCheck reconciler carrying neither a runner nor a pool queue.
func TestRuntimeLoadingUnavailableRegistersNothingRuntime(t *testing.T) {
	for _, tc := range []struct {
		name       string
		options    func(*Options)
		wantReason string
	}{
		{
			name:       "the shipped default",
			options:    func(o *Options) { o.Namespace = "fathom-system" },
			wantReason: "RuntimeLoadingDisabled",
		},
		{
			name: "enabled but explicitly disabled by configuration",
			options: func(o *Options) {
				o.Namespace, o.LeaderElect = "fathom-system", true
			},
			wantReason: "RuntimeLoadingDisabled",
		},
		{
			name: "enabled without leader election",
			options: func(o *Options) {
				o.RuntimeLoading.Enabled, o.Namespace, o.LeaderElect = true, "fathom-system", false
			},
			wantReason: "LeaderElectionRequired",
		},
		{
			name: "enabled without an explicit operator namespace",
			options: func(o *Options) {
				o.RuntimeLoading.Enabled, o.LeaderElect = true, true
			},
			wantReason: "OperatorNamespaceRequired",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
			opts := DefaultOptions()
			tc.options(&opts)

			wiring, reason := newRuntimeWiring(opts)
			if wiring != nil {
				t.Fatalf("runtime wiring was built although %s", tc.wantReason)
			}
			if reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tc.wantReason)
			}

			mgr := newRecordingManager(t)
			controllers, err := defaultControllers(mgr, opts, wiring)
			if err != nil {
				t.Fatalf("defaultControllers: %v", err)
			}
			public, err := DefaultControllers(mgr, opts)
			if err != nil {
				t.Fatalf("DefaultControllers: %v", err)
			}
			if got, want := setupperTypes(controllers), setupperTypes(public); !slices.Equal(got, want) {
				t.Fatalf("controller set = %v, want the built-in set %v", got, want)
			}
			if len(controllers) != 6 {
				t.Fatalf("registered %d controllers, want the 6 built-ins: %v", len(controllers), setupperTypes(controllers))
			}
			addonCheck, ok := controllers[0].(*controller.AddonCheckReconciler)
			if !ok {
				t.Fatalf("controllers[0] = %T, want *controller.AddonCheckReconciler", controllers[0])
			}
			if addonCheck.Runtime != nil || addonCheck.RuntimeQueue != nil {
				t.Errorf("the AddonCheck reconciler is runtime-wired although %s: runner=%v queue=%v",
					tc.wantReason, addonCheck.Runtime != nil, addonCheck.RuntimeQueue != nil)
			}
			if runnables := mgr.runnables(); len(runnables) != 0 {
				t.Errorf("%d runnables were added although %s: %v", len(runnables), tc.wantReason, runnables)
			}
			for _, index := range mgr.registeredIndexes() {
				if strings.Contains(index, "AddonDefinition") {
					t.Errorf("index %q was registered although %s", index, tc.wantReason)
				}
			}
		})
	}
}

// The other direction, seam by seam. Each assertion below is a clause of T047's
// task sentence, and each one is independently mutation-provable.
func TestRuntimeWiringAttachesEverySeam(t *testing.T) {
	writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
	opts := eligibleRuntimeOptions()
	wiring, reason := newRuntimeWiring(opts)
	if wiring == nil {
		t.Fatalf("runtime wiring unavailable: %s", reason)
	}
	mgr := newRecordingManager(t)
	controllers, err := defaultControllers(mgr, opts, wiring)
	if err != nil {
		t.Fatalf("defaultControllers: %v", err)
	}

	// Preserve unrelated built-ins: the six built-in reconcilers are still
	// there, still first, still in their original order.
	if len(controllers) != 8 {
		t.Fatalf("registered %d controllers, want 6 built-ins plus the 2 runtime reconcilers: %v",
			len(controllers), setupperTypes(controllers))
	}
	baseline, err := DefaultControllers(mgr, opts)
	if err != nil {
		t.Fatalf("DefaultControllers: %v", err)
	}
	if got, want := setupperTypes(controllers[:6]), setupperTypes(baseline); !slices.Equal(got, want) {
		t.Fatalf("built-in controller set changed: %v, want %v", got, want)
	}

	addonCheck, ok := controllers[0].(*controller.AddonCheckReconciler)
	if !ok {
		t.Fatalf("controllers[0] = %T", controllers[0])
	}
	if addonCheck.Runtime == nil {
		t.Fatal("the AddonCheck reconciler has no runtime runner, so no runtime-backed check can execute")
	}
	// Shared pool: the reconciler enqueues into the very scheduler the elected
	// worker pool drains, not into one of its own.
	if addonCheck.RuntimeQueue != wiring.scheduler {
		t.Errorf("the AddonCheck reconciler's queue is not the shared runtime pool")
	}
	// Shared registry: the runner dispatches through the same registry the
	// built-in adapters are registered in.
	runtimeRegistry, _ := addonCheck.Runtime.Registry.(*registry.Registry)
	builtinRegistry, _ := any(addonCheck.Adapters).(*registry.Registry)
	if runtimeRegistry == nil || runtimeRegistry != builtinRegistry {
		t.Errorf("the runtime runner resolves through a different registry than the built-in path")
	}
	// ... and that one registry still answers for every built-in adapter.
	for _, builtin := range BuiltInAdapters() {
		for _, addonType := range builtin.Capabilities().AddonTypes {
			resolution, err := runtimeRegistry.Resolve(addonType)
			if err != nil || resolution.Runtime || resolution.Adapter == nil {
				t.Errorf("built-in addon type %q resolves to %+v (err %v); runtime wiring must not disturb built-in dispatch",
					addonType, resolution, err)
			}
		}
	}
	if addonCheck.Runtime.OperatorNamespace != opts.Namespace {
		t.Errorf("runner operator namespace = %q, want the configured %q", addonCheck.Runtime.OperatorNamespace, opts.Namespace)
	}
	if addonCheck.Runtime.ManagerServiceAccount != testManagerServiceAccount {
		t.Errorf("runner manager service account = %q, want %q", addonCheck.Runtime.ManagerServiceAccount, testManagerServiceAccount)
	}

	// The runtime reconcilers get the same explicit namespace and identity, the
	// same shared registry, and — for drain acknowledgement — the manager's
	// UNCACHED APIReader. A cached reader there would let the informer answer
	// for state a drain claims to have observed directly.
	if wiring.definitions == nil || wiring.bindings == nil {
		t.Fatal("the runtime reconcilers were never built")
	}
	if wiring.bindings.APIReader == nil || wiring.bindings.APIReader != mgr.GetAPIReader() {
		t.Errorf("the binding reconciler's direct reader is %T, want the manager's uncached APIReader", wiring.bindings.APIReader)
	}
	if wiring.bindings.Leadership == nil {
		t.Error("the binding reconciler has no leadership session, so it can acknowledge no drain")
	}
	if wiring.definitions.Registry != runtimeRegistry || wiring.bindings.Registry != runtimeRegistry {
		t.Error("the runtime reconcilers publish into a different registry than the one dispatch reads")
	}
	// The cache-sync gate must be the MANAGER'S cache, not merely non-nil: a
	// constant-true stand-in satisfies both "CacheSynced != nil" and "Start
	// does not return ErrCacheSyncUnwired" while promising a synchronization
	// nobody ever waited for. Each wired gate is therefore CALLED, and the
	// manager's own cache has to be the thing that answers.
	if wiring.definitions.CacheSynced == nil || wiring.bindings.CacheSynced == nil {
		t.Fatal("a runtime reconciler has no cache-synchronization gate; it would publish from a partial informer sync")
	}
	sessionSync := wiring.session.cacheSync()
	if sessionSync == nil {
		t.Fatal("the leadership session has no cache-synchronization gate")
	}
	syncCtx, cancelSync := context.WithTimeout(context.Background(), time.Second)
	observedBefore := mgr.syncCache.syncCalls()
	wiring.definitions.CacheSynced(syncCtx)
	wiring.bindings.CacheSynced(syncCtx)
	sessionSync(syncCtx)
	cancelSync()
	if got := mgr.syncCache.syncCalls() - observedBefore; got != 3 {
		t.Errorf("the manager's cache answered %d of the 3 wired cache-sync calls, want 3; a gate that is not the manager's cache waits for a synchronization that never happens", got)
	}
	if wiring.definitions.OperatorBuild == "" {
		t.Error("the definition reconciler has no operator build, so no snapshot it publishes is attributable")
	}
	for _, namespace := range []string{wiring.definitions.OperatorNamespace, wiring.bindings.OperatorNamespace} {
		if namespace != opts.Namespace {
			t.Errorf("operator namespace = %q, want the configured %q", namespace, opts.Namespace)
		}
	}
	for _, account := range []string{wiring.definitions.ManagerServiceAccount, wiring.bindings.ManagerServiceAccount} {
		if account != testManagerServiceAccount {
			t.Errorf("manager service account = %q, want %q", account, testManagerServiceAccount)
		}
	}

	// Informer sync: an unwired session refuses to start with
	// ErrCacheSyncUnwired, which is exactly how a forgotten WithCacheSync would
	// present. A cancelled context ends a WIRED session quietly instead.
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	if err := wiring.session.Start(ended); errors.Is(err, ErrCacheSyncUnwired) {
		t.Errorf("the leadership session's informer-sync gate was never wired: %v", err)
	}

	// Manager APIReader fences: the per-run control reader is the uncached
	// control-plane reader, and specifically NOT the manager's cached client.
	budget, closeBudget := execution.NewBudget(context.Background(), time.Second)
	defer closeBudget()
	clients, err := addonCheck.Runtime.Clients(budget, execution.ControlTargets{
		OperatorNamespace: opts.Namespace,
		DefinitionName:    "an-addon",
		LeaseName:         opts.LeaderElectionID,
		Check:             types.NamespacedName{Namespace: "checks", Name: "an-addon-check"},
	})
	if err != nil {
		t.Fatalf("build per-run clients: %v", err)
	}
	switch {
	case clients.Control.Reader == nil:
		t.Error("the per-run control reader is empty; both fences would have nothing to read through")
	case clients.Control.Reader == client.Reader(mgr.GetClient()):
		t.Error("the fences read through the manager's CACHED client")
	case clients.Control.Reader == mgr.GetAPIReader():
		t.Error("the fences read through the manager's shared reader rather than a budget-bound run reader")
	}
	if clients.Evaluator == nil {
		t.Error("the per-run evaluator factory is missing")
	}

	// Three leader-elected runnables: the session, the gate it drives, and the
	// worker pool. Every one of them must be leader-gated.
	var session, gate, pool bool
	for _, runnable := range mgr.runnables() {
		elected, ok := runnable.(manager.LeaderElectionRunnable)
		if !ok || !elected.NeedLeaderElection() {
			t.Errorf("%T is registered without leader election; runtime admission must never run un-elected", runnable)
		}
		switch runnable.(type) {
		case *RuntimeLeadership:
			session = true
		case *runtimeDispatchGate:
			gate = true
		case *runtimeWorkerPool:
			pool = true
		}
	}
	if !session || !gate || !pool {
		t.Errorf("registered runnables: session=%v gate=%v pool=%v, want all three", session, gate, pool)
	}

	// Watches and indexes: every indexed lookup the two reconcilers make must
	// have its index, or the lookup silently becomes an unbounded List.
	for _, setupper := range controllers[6:] {
		if err := setupper.SetupWithManager(mgr); err != nil {
			t.Fatalf("SetupWithManager(%T): %v", setupper, err)
		}
	}
	indexes := mgr.registeredIndexes()
	for _, field := range []string{
		controller.IndexBindingDefinitionName,
		controller.IndexBindingServiceAccountName,
		controller.IndexBindingServiceAccountUID,
	} {
		want := "*v1alpha1.AddonDefinitionBinding/" + field
		if !slices.Contains(indexes, want) {
			t.Errorf("index %q was never registered; its lookup would become an unbounded List. registered: %v", want, indexes)
		}
	}
}

// The leader-election lock carrying the runtime session's identity is installed
// ONLY when runtime loading is available. Default-off must leave
// controller-runtime's own lock — and therefore the operator's existing
// election behaviour — exactly as it was.
func TestRunLeaderElectionLockIsInstalledOnlyForRuntime(t *testing.T) {
	for _, tc := range []struct {
		name     string
		options  func(*Options)
		wantLock bool
	}{
		{name: "default off", options: func(*Options) {}},
		{
			name:    "runtime enabled without an operator namespace",
			options: func(o *Options) { o.RuntimeLoading.Enabled = true },
		},
		{
			name: "runtime available",
			options: func(o *Options) {
				o.RuntimeLoading.Enabled, o.Namespace = true, "fathom-system"
			},
			wantLock: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
			original := managerFactory
			t.Cleanup(func() { managerFactory = original })
			var captured ctrl.Options
			managerFactory = func(_ *rest.Config, opts ctrl.Options) (ctrl.Manager, error) {
				captured = opts
				return nil, errors.New("synthetic manager failure")
			}

			opts := DefaultOptions()
			tc.options(&opts)
			err := Run(context.Background(), &rest.Config{}, opts, nil)
			if err == nil || !strings.Contains(err.Error(), "synthetic manager failure") {
				t.Fatalf("Run: %v, want the synthetic manager failure", err)
			}
			if got := captured.LeaderElectionResourceLockInterface != nil; got != tc.wantLock {
				t.Fatalf("runtime leader-election lock installed = %v, want %v", got, tc.wantLock)
			}
			if !captured.LeaderElection {
				t.Error("leader election was disabled; runtime loading requires the manager's own election")
			}
		})
	}
}

// --- the gate and the pool attach ACTUALLY registered -----------------------

// runtimeRunnables returns the three runtime runnables attach handed the
// manager. The tests below assert against THESE objects — the ones the
// operator will really start — never against hand-built copies, because a
// hand-built gate proves only that the gate type works, not that the wiring
// connected it to anything.
func runtimeRunnables(t *testing.T, mgr *recordingManager) (*RuntimeLeadership, *runtimeDispatchGate, *runtimeWorkerPool) {
	t.Helper()
	var (
		session *RuntimeLeadership
		gate    *runtimeDispatchGate
		pool    *runtimeWorkerPool
	)
	for _, runnable := range mgr.runnables() {
		switch r := runnable.(type) {
		case *RuntimeLeadership:
			session = r
		case *runtimeDispatchGate:
			gate = r
		case *runtimeWorkerPool:
			pool = r
		}
	}
	if session == nil || gate == nil || pool == nil {
		t.Fatalf("registered runnables: session=%v gate=%v pool=%v, want all three", session != nil, gate != nil, pool != nil)
	}
	return session, gate, pool
}

// Every field of the gate and the pool, by IDENTITY. Type-level assertions ("a
// *runtimeDispatchGate was registered, and it is leader-gated") pass unchanged
// against a gate driven by a second, never-started session, or one admitting on
// a private registry nothing dispatches through — the operator would log
// "runtime dispatch admitted" while every Resolve still returned a
// closed-admission barrier. That silent disagreement is what T047 exists to
// close, so each seam is pinned to the one object it must be.
func TestAttachRegistersTheGateAndPoolItBuilt(t *testing.T) {
	writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
	opts := eligibleRuntimeOptions()
	wiring, reason := newRuntimeWiring(opts)
	if wiring == nil {
		t.Fatalf("runtime wiring unavailable: %s", reason)
	}
	mgr := newRecordingManager(t)
	controllers, err := defaultControllers(mgr, opts, wiring)
	if err != nil {
		t.Fatalf("defaultControllers: %v", err)
	}
	addonCheck, ok := controllers[0].(*controller.AddonCheckReconciler)
	if !ok {
		t.Fatalf("controllers[0] = %T, want *controller.AddonCheckReconciler", controllers[0])
	}
	shared, _ := any(addonCheck.Adapters).(*registry.Registry)
	if shared == nil {
		t.Fatalf("the AddonCheck reconciler resolves through %T, want the shared *registry.Registry", addonCheck.Adapters)
	}
	session, gate, pool := runtimeRunnables(t, mgr)

	// The ONE decider: the registered session is the wiring's session, and it
	// is the session both the gate and the pool wait on. A gate driven by a
	// second session — one nothing registers and nothing starts — blocks
	// forever on a Ready() that never closes, and runtime is inert in silence.
	if session != wiring.session {
		t.Error("the registered leadership session is not the wiring's session")
	}
	if gate.session != wiring.session {
		t.Error("the dispatch gate waits on a different leadership session than the one the manager starts")
	}
	if pool.session != wiring.session {
		t.Error("the worker pool waits on a different leadership session than the one the manager starts")
	}

	// The ONE enforcement point: the gate opens the registry every dispatch
	// decision is actually made in, not a private one.
	if gate.registry != shared {
		t.Error("the dispatch gate admits on a different registry than the one built-ins and runtime dispatch through")
	}
	if runtimeRegistry, _ := addonCheck.Runtime.Registry.(*registry.Registry); runtimeRegistry != shared {
		t.Error("the runtime runner resolves through a different registry than the gate admits on")
	}

	// The Lease is read UNCACHED. contracts/leadership.md forbids adding
	// cluster-wide Lease reads for this feature, and a cached Get is exactly
	// that: it starts an informer over every Lease in the cluster (one per
	// kubelet), needing cluster-scoped list/watch the operator does not hold.
	if gate.reader == nil || gate.reader != mgr.GetAPIReader() {
		t.Errorf("the dispatch gate reads its Lease through %T, want the manager's uncached APIReader", gate.reader)
	}
	if gate.reader == client.Reader(mgr.GetClient()) {
		t.Error("the dispatch gate reads its Lease through the manager's CACHED client, starting a cluster-wide Lease informer")
	}

	// The ONE pool: the scheduler the pool drains is the scheduler the
	// reconciler enqueues into. A second scheduler in either position leaves
	// checks piling up in a queue no worker drains.
	if pool.scheduler != wiring.scheduler {
		t.Error("the worker pool drains a different scheduler than the shared runtime pool")
	}
	if addonCheck.RuntimeQueue != wiring.scheduler {
		t.Error("the AddonCheck reconciler enqueues into a different scheduler than the worker pool drains")
	}

	// ... and the pool executes the reconciler's own runtime handler. A
	// stand-in that simply reports Completed drains the queue without ever
	// reaching a runtime evaluation.
	if pool.handle == nil {
		t.Fatal("the worker pool has no handler")
	}
	if got, want := reflect.ValueOf(pool.handle).Pointer(), reflect.ValueOf(addonCheck.RunRuntimeWork).Pointer(); got != want {
		t.Error("the worker pool's handler is not the AddonCheck reconciler's RunRuntimeWork")
	}

	// The gate and the pool are not the only consumers of the ONE decider. The
	// runner validates every epoch, drain and admission against its Session, and
	// the binding reconciler attributes drain acknowledgement to its Leadership.
	// A second, never-started session planted in either place is the same defect
	// shape as a second session behind the gate: silently inert authority.
	if addonCheck.Runtime == nil {
		t.Fatal("the AddonCheck reconciler has no runtime runner")
	}
	if addonCheck.Runtime.Session != controller.RuntimeExecutionSession(wiring.session) {
		t.Error("the runtime runner validates authority against a different session than the one the manager starts")
	}
	if got, want := addonCheck.Runtime.ProbeImage, opts.ProbeImage; got != want {
		t.Errorf("the runtime runner carries probe image %q, want the configured %q", got, want)
	}
	if wiring.bindings.Leadership == nil {
		t.Fatal("the binding reconciler has no leadership session; drain could never be acknowledged")
	}
	if got := wiring.bindings.Leadership.(drainSession).RuntimeLeadership; got != wiring.session {
		t.Error("drain acknowledgement is attributed to a different session than the one the manager starts")
	}
}

// publishGateSnapshot puts a runtime adapter in the registry so admission has
// something to be observed through: with the gate closed Resolve returns a
// closed-admission barrier; with it open the same call resolves to a runtime
// adapter.
func publishGateSnapshot(t *testing.T, reg *registry.Registry) {
	t.Helper()
	if err := reg.SetRuntime(registry.RuntimeEntry{
		Adapter: gateAdapter{},
		Revision: registry.RuntimeRevision{
			DefinitionUID: "definition-uid", Generation: 1,
			SchemaVersion: fathomv1alpha1.GroupVersion.Version, SemanticsVersion: 1,
		},
		Provenance: registry.RuntimeProvenance{OperatorBuild: "test", AdapterVersion: "1.0.0"},
	}); err != nil {
		t.Fatalf("publish runtime snapshot: %v", err)
	}
}

// The keystone, end to end against a REAL started manager and the objects
// attach actually registered: admission observed at the shared registry,
// execution observed through the AddonCheck reconciler's own queue. Nothing
// here builds a gate, a pool, a session or a registry of its own.
//
// Only the monotonic takeover grace is fast-forwarded (its clock is an injected
// seam); election, informer sync, the live Lease and the Lease read are real.
func TestAttachedGateAndPoolDriveDispatchAndExecution(t *testing.T) {
	if envtestCfg == nil {
		t.Skip("envtest unavailable; run via `task test` for full coverage")
	}
	writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
	opts := eligibleRuntimeOptions()
	opts.Namespace = "fathom-runtime-wiring"
	wiring, reason := newRuntimeWiring(opts)
	if wiring == nil {
		t.Fatalf("runtime wiring unavailable: %s", reason)
	}
	wiring.session.elapsedSince = func(time.Time) time.Duration { return time.Hour }

	mgr := newRecordingManager(t)
	controllers, err := defaultControllers(mgr, opts, wiring)
	if err != nil {
		t.Fatalf("defaultControllers: %v", err)
	}
	addonCheck, ok := controllers[0].(*controller.AddonCheckReconciler)
	if !ok {
		t.Fatalf("controllers[0] = %T, want *controller.AddonCheckReconciler", controllers[0])
	}
	shared, _ := any(addonCheck.Adapters).(*registry.Registry)
	if shared == nil {
		t.Fatalf("the AddonCheck reconciler resolves through %T, want the shared *registry.Registry", addonCheck.Adapters)
	}
	publishGateSnapshot(t, shared)
	admitted := func() bool {
		resolution, err := shared.Resolve(gateAddonType)
		return err == nil && resolution.Runtime
	}
	if admitted() {
		t.Fatal("runtime dispatch is admitted before the manager started")
	}

	// The live Lease the elected session must adopt, held under ITS identity.
	live, err := client.New(envtestCfg, client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		t.Fatalf("build a direct client: %v", err)
	}
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: opts.Namespace}}
	if err := live.Create(t.Context(), namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create the operator namespace: %v", err)
	}
	lease := heldLease(wiring.session)
	lease.ResourceVersion, lease.UID = "", ""
	switch err := live.Create(t.Context(), lease); {
	case apierrors.IsAlreadyExists(err):
		// A previous run of this test in the same envtest apiserver (`-count=N`)
		// left one behind; this session holds it under its own new identity.
		var existing coordinationv1.Lease
		if err := live.Get(t.Context(), client.ObjectKeyFromObject(lease), &existing); err != nil {
			t.Fatalf("read the existing leader election Lease: %v", err)
		}
		existing.Spec = lease.Spec
		if err := live.Update(t.Context(), &existing); err != nil {
			t.Fatalf("take over the leader election Lease: %v", err)
		}
	case err != nil:
		t.Fatalf("create the leader election Lease: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	managerDone := make(chan error, 1)
	go func() { managerDone <- mgr.Start(ctx) }()

	// 1. The decider opened the ONE enforcement point, observed by resolving
	// through the registry the built-in path and the runner both read.
	waitFor(t, "runtime dispatch to be admitted through the shared registry", admitted)

	// 2. Work enqueued through the reconciler's OWN queue reaches the
	// reconciler's OWN handler on the elected pool: RunRuntimeWork opens by
	// reading the enqueued AddonCheck through the manager's client.
	work := execution.Work{
		Definition: gateAddonType,
		Check:      types.NamespacedName{Namespace: opts.Namespace, Name: "runtime-check"},
	}
	if err := addonCheck.RuntimeQueue.Enqueue(work, time.Now()); err != nil {
		t.Fatalf("enqueue runtime work: %v", err)
	}
	waitFor(t, "the elected worker pool to execute the reconciler's runtime handler", func() bool {
		return mgr.recClient.readAddonCheck(work.Check)
	})

	// 3. The session's informer-sync gate is the manager's own cache: it
	// waited on THAT cache before it became admissible.
	if mgr.syncCache.syncCalls() == 0 {
		t.Error("the leadership session never waited on the manager's cache; its sync gate is not the manager's")
	}

	cancel()
	if err := <-managerDone; err != nil {
		t.Fatalf("manager: %v", err)
	}
	if admitted() {
		t.Error("runtime dispatch is still admitted after the elected session ended")
	}
}

type selectiveStartupReader struct {
	client.Reader
	fail string
}

func (r selectiveStartupReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Name == r.fail {
		return errors.New("injected API read failure")
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

func TestStartupInventoryDirectReadsBlockOnlyUnresolvedBuiltin(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatal(err)
	}
	stored := &fathomv1alpha1.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "coredns", UID: "stored-uid"}}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored).Build()
	reg, err := BuildAdapterRegistry(logr.Discard(), BuiltInAdapters()...)
	if err != nil {
		t.Fatal(err)
	}
	reg.ReserveStartupClaims([]string{"coredns", "cert-manager", "external-secrets"})
	refreshStartupClaims(t.Context(), selectiveStartupReader{Reader: reader, fail: "cert-manager"}, reg, logr.Discard())
	for _, tc := range []struct{ name, reason string }{
		{"coredns", registry.ReasonBuiltinCollision},
		{"cert-manager", registry.ReasonAdmissionClosed},
	} {
		_, err := reg.Resolve(tc.name)
		var barrier *registry.DispatchBarrier
		if !errors.As(err, &barrier) || barrier.Reason != tc.reason {
			t.Errorf("%s: got %v, want %s barrier", tc.name, err, tc.reason)
		}
	}
	if _, err := reg.Resolve("external-secrets"); err != nil {
		t.Fatalf("known absent definition blocked unrelated built-in: %v", err)
	}
	refreshStartupClaims(t.Context(), reader, reg, logr.Discard())
	if _, err := reg.Resolve("cert-manager"); err != nil {
		t.Fatalf("recovered API read did not release startup reservation: %v", err)
	}
}

func TestStoredBuiltinCollisionIsBarredBeforeFirstReconcile(t *testing.T) {
	if envtestCfg == nil {
		t.Skip("envtest unavailable; run with KUBEBUILDER_ASSETS")
	}
	writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
	manager := newRecordingManager(t)
	live, err := client.New(envtestCfg, client.Options{Scheme: manager.GetScheme()})
	if err != nil {
		t.Fatal(err)
	}
	definition := &fathomv1alpha1.AddonDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns"},
		Spec: fathomv1alpha1.AddonDefinitionSpec{
			AddonType: "coredns", AdapterVersion: "1.0.0", SemanticsVersion: 1,
			Families: []fathomv1alpha1.DefinitionFamily{{Name: "health", Checks: []fathomv1alpha1.DefinitionCheck{{
				Name: "controller", Kind: "Workload", Workload: &fathomv1alpha1.DefinitionWorkload{
					Target: fathomv1alpha1.DefinitionTarget{Scope: "Namespaced", Namespaces: []fathomv1alpha1.DefinitionDNSLabel{"default"}},
					Kind:   "Deployment", DefaultName: "controller",
				},
			}}}},
		},
	}
	if err := live.Create(t.Context(), definition); err != nil {
		t.Fatalf("create stored definition: %v", err)
	}
	t.Cleanup(func() { _ = live.Delete(context.Background(), definition) })
	wiring, reason := newRuntimeWiring(eligibleRuntimeOptions())
	if wiring == nil {
		t.Fatal(reason)
	}
	controllers, err := defaultControllers(manager, eligibleRuntimeOptions(), wiring)
	if err != nil {
		t.Fatal(err)
	}
	addonCheck := controllers[0].(*controller.AddonCheckReconciler)
	shared := addonCheck.Adapters.(*registry.Registry)
	_, err = shared.Resolve("coredns")
	var barrier *registry.DispatchBarrier
	if !errors.As(err, &barrier) || barrier.Reason != registry.ReasonBuiltinCollision {
		t.Fatalf("stored collision dispatched before definition controller started: %v", err)
	}
	if _, err := shared.Resolve("cert-manager"); err != nil {
		t.Fatalf("unrelated builtin blocked at startup: %v", err)
	}
}
