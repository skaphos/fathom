/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	"github.com/skaphos/fathom/internal/adapter/registry"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// This file covers T039 (the pre-run and final uncached control-plane fences)
// and T040 (per-check publication serialization/CAS and the publication
// precedence order) of specs/012-addon-definition-runtime.
//
// Every fixture is driven directly and deterministically: the runner's barrier
// seam holds a run at an exact point instead of sleeping, so the concurrency
// and mid-run mutation rows carry no wall-clock dependency.

const (
	runtimeCheckNamespace = "runtime-checks"
	runtimeCheckName      = "custom-addon-check"
	runtimeCheckUID       = types.UID("addoncheck-uid-1")
	runtimeCheckLease     = "2d3dbc4f.skaphos.io"
	runtimeCheckTimeout   = 3 * time.Second
)

// runtimeCheckClockStart anchors the fixture's injected clock. Evidence ageing
// is measured in intervals, so every freshness row moves this clock instead of
// sleeping; a wall-clock test of a ten-minute window is not a test.
var runtimeCheckClockStart = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

// runtimeCheckAdapter is the compiled adapter a published snapshot dispatches
// to. Its Run is scripted per row so a fixture can fail, stall or complete on
// demand without any timing assumption.
type runtimeCheckAdapter struct {
	addonType string
	run       func(context.Context, adapter.Request) (adapter.Result, error)
}

func (a *runtimeCheckAdapter) Name() string            { return a.addonType }
func (a *runtimeCheckAdapter) Version() string         { return "1.0.0" }
func (a *runtimeCheckAdapter) ContractVersion() string { return adapter.ContractVersion }
func (a *runtimeCheckAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{a.addonType}, Families: []adapter.Family{"health"}}
}

func (a *runtimeCheckAdapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
	if a.run == nil {
		return adapter.Result{Checks: []adapter.CheckResult{{
			Family: "health", Outcome: adapter.OutcomePass, Summary: "controller is available",
		}}}, nil
	}
	return a.run(ctx, req)
}

// fakeRuntimeSession stands in for internal/app.RuntimeLeadership, which cannot
// be imported here: internal/app imports internal/controller, so the dependency
// only runs that way (verified with `go list -deps ./internal/app`). The real
// session is unit-tested in its own package; what matters here is that the
// runner asks it before executing and believes it about the epoch.
type fakeRuntimeSession struct {
	mu        sync.Mutex
	lease     types.NamespacedName
	epoch     *fathomv1alpha1.DefinitionLeaderEpoch
	epochOK   bool
	admitErr  error
	admits    int
	active    map[string]int
	revoked   map[string]bool
	cancelled map[string]context.CancelFunc
}

func newFakeRuntimeSession() *fakeRuntimeSession {
	return &fakeRuntimeSession{
		lease: types.NamespacedName{Namespace: lifecycleNamespace, Name: runtimeCheckLease},
		epoch: &fathomv1alpha1.DefinitionLeaderEpoch{
			LeaseUID:         "lease-uid-1",
			HolderIdentity:   lifecycleLeader,
			AcquireTime:      metav1.NewMicroTime(time.Unix(1700000000, 0).UTC()),
			LeaseTransitions: 3,
		},
		epochOK:   true,
		active:    map[string]int{},
		revoked:   map[string]bool{},
		cancelled: map[string]context.CancelFunc{},
	}
}

func (s *fakeRuntimeSession) LeaseRef() types.NamespacedName { return s.lease }

func (s *fakeRuntimeSession) Epoch() *fathomv1alpha1.DefinitionLeaderEpoch {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.epoch == nil {
		return nil
	}
	observed := *s.epoch
	return &observed
}

func (s *fakeRuntimeSession) EpochValid(persisted *fathomv1alpha1.DefinitionLeaderEpoch) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epochOK && persisted != nil && s.epoch != nil &&
		s.epoch.LeaseUID == persisted.LeaseUID &&
		s.epoch.HolderIdentity == persisted.HolderIdentity &&
		s.epoch.AcquireTime.Equal(&persisted.AcquireTime) &&
		s.epoch.LeaseTransitions == persisted.LeaseTransitions
}

func (s *fakeRuntimeSession) Admit(key string) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admitErr != nil {
		return nil, nil, s.admitErr
	}
	if s.revoked[key] {
		return nil, nil, errors.New("runtime admission is revoked for this binding")
	}
	s.admits++
	s.active[key]++
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelled[key] = cancel
	return ctx, sync.OnceFunc(func() {
		cancel()
		s.mu.Lock()
		s.active[key]--
		s.mu.Unlock()
	}), nil
}

// revoke is an observed revocation: admission closes and this session's work
// for the binding is cancelled.
func (s *fakeRuntimeSession) revoke(key string) {
	s.mu.Lock()
	s.revoked[key] = true
	cancel := s.cancelled[key]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *fakeRuntimeSession) activeRuns(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[key]
}

func (s *fakeRuntimeSession) admissions() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.admits
}

// runtimeCheckFixture wires one runner over one fake API. The manager's cached
// client and the uncached control reader are two distinct wrappers over the
// SAME backing store, so resource versions stay coherent while every read is
// still attributable to the reader that made it.
type runtimeCheckFixture struct {
	t        *testing.T
	scheme   *runtime.Scheme
	store    client.WithWatch
	cached   client.WithWatch
	registry *registry.Registry
	session  *fakeRuntimeSession
	runner   *AddonCheckRuntimeRunner
	stub     *runtimeCheckAdapter

	mu sync.Mutex
	// cachedFenceReads counts fence-kind reads made through the manager's
	// cached client. The contract allows none.
	cachedFenceReads int
	controlReads     int
	statusWrites     int
	budgets          []*execution.Budget
	factoryCalls     int
	evaluatorRuns    int
	// chargeControlReads makes the fake control reader consume the shared run
	// budget the way the real budget-bound control transport does.
	chargeControlReads bool
	// preCharge is work already spent when the per-run clients are built.
	preCharge int
	// onControlRead runs before the nth uncached control-plane read, so a row
	// can change the world at an exact point inside a fence.
	onControlRead func(*execution.Budget, int)
	// clock is the runner's injected wall clock.
	clock time.Time
}

func (f *runtimeCheckFixture) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clock
}

func (f *runtimeCheckFixture) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clock = f.clock.Add(d)
}

func runtimeCheckObject() *fathomv1alpha1.AddonCheck {
	timeout := metav1.Duration{Duration: runtimeCheckTimeout}
	return &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: runtimeCheckNamespace, Name: runtimeCheckName,
			UID: runtimeCheckUID, Generation: 1,
		},
		Spec: fathomv1alpha1.AddonCheckSpec{
			AddonType: lifecycleAddon,
			Timeout:   &timeout,
			Policy:    map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"health": {Enabled: ptr.To(true)}},
		},
	}
}

func isRuntimeFenceKind(obj client.Object) bool {
	switch obj.(type) {
	case *fathomv1alpha1.AddonDefinition, *fathomv1alpha1.AddonDefinitionBinding,
		*fathomv1alpha1.AddonCheck, *corev1.ServiceAccount:
		return true
	}
	return false
}

func newRuntimeCheckFixture(t *testing.T, objs ...client.Object) *runtimeCheckFixture {
	t.Helper()
	f := &runtimeCheckFixture{
		t: t, scheme: runtime.NewScheme(), session: newFakeRuntimeSession(),
		clock: runtimeCheckClockStart,
	}
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, fathomv1alpha1.AddToScheme} {
		if err := add(f.scheme); err != nil {
			t.Fatalf("scheme: %v", err)
		}
	}
	if len(objs) == 0 {
		objs = []client.Object{lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(), runtimeCheckObject()}
	}
	f.store = fake.NewClientBuilder().
		WithScheme(f.scheme).
		WithStatusSubresource(&fathomv1alpha1.AddonCheck{}, &fathomv1alpha1.AddonDefinition{}, &fathomv1alpha1.AddonDefinitionBinding{}).
		WithObjects(objs...).
		Build()

	f.cached = interceptor.NewClient(f.store, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if isRuntimeFenceKind(obj) {
				f.mu.Lock()
				f.cachedFenceReads++
				f.mu.Unlock()
			}
			return c.Get(ctx, key, obj, opts...)
		},
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			f.mu.Lock()
			f.statusWrites++
			f.mu.Unlock()
			return c.SubResource(sub).Update(ctx, obj, opts...)
		},
	})

	f.registry = registry.New(logr.Discard())
	f.stub = &runtimeCheckAdapter{addonType: lifecycleAddon}
	f.runner = &AddonCheckRuntimeRunner{
		Client:                f.cached,
		Registry:              f.registry,
		Session:               f.session,
		Clients:               f.newRunClients,
		OperatorNamespace:     lifecycleNamespace,
		ManagerServiceAccount: lifecycleManagerSA,
		Now:                   f.now,
	}
	return f
}

// newRunClients is the production seam T047 fills with the real uncached
// control reader and the real impersonating factory. Both are built from the
// single Budget the runner hands in, which is the whole point of the seam.
func (f *runtimeCheckFixture) newRunClients(b *execution.Budget, targets execution.ControlTargets) (RuntimeRunClients, error) {
	f.mu.Lock()
	f.factoryCalls++
	f.budgets = append(f.budgets, b)
	preCharge, charge := f.preCharge, f.chargeControlReads
	f.mu.Unlock()
	if targets.OperatorNamespace != f.runner.OperatorNamespace || targets.DefinitionName != lifecycleAddon ||
		targets.LeaseName != runtimeCheckLease || targets.Check.Name != runtimeCheckName ||
		targets.Check.Namespace != runtimeCheckNamespace {
		f.t.Errorf("control targets are not the exact fence targets: %+v", targets)
	}
	for i := 0; i < preCharge; i++ {
		if err := b.ChargeRequest(); err != nil {
			break
		}
	}
	count := func() error {
		f.mu.Lock()
		f.controlReads++
		reads, hook := f.controlReads, f.onControlRead
		f.mu.Unlock()
		if hook != nil {
			hook(b, reads)
		}
		if !charge {
			return nil
		}
		return b.ChargeRequest()
	}
	reader := interceptor.NewClient(f.store, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := count(); err != nil {
				return err
			}
			return c.Get(ctx, key, obj, opts...)
		},
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if err := count(); err != nil {
				return err
			}
			return c.List(ctx, list, opts...)
		},
	})
	control := impersonation.RuntimeControlReader{Reader: reader}
	return RuntimeRunClients{Control: control, Evaluator: &fakeEvaluatorFactory{f: f, control: control}}, nil
}

// fakeEvaluatorFactory mirrors *impersonation.RuntimeFactory: it resolves live
// authority through the very same uncached control reader and refuses to build
// a delegated client without the caller's per-run scope/budget guard.
type fakeEvaluatorFactory struct {
	f       *runtimeCheckFixture
	control client.Reader
}

func (e *fakeEvaluatorFactory) ClientFor(ctx context.Context, name string,
	buildGuard func(*impersonation.RuntimeAuthority) (func(http.RoundTripper) http.RoundTripper, error),
) (client.Client, *impersonation.RuntimeAuthority, error) {
	authority, err := impersonation.ResolveRuntimeAuthority(ctx, e.control,
		e.f.runner.OperatorNamespace, e.f.runner.ManagerServiceAccount, name)
	if err != nil {
		return nil, nil, err
	}
	wrap, err := buildGuard(authority)
	if err != nil {
		return nil, nil, err
	}
	if wrap == nil {
		return nil, nil, errors.New("AuthorizationUnavailable: transport guard missing")
	}
	return e.f.store, authority, nil
}

func (f *runtimeCheckFixture) admit() {
	f.t.Helper()
	if err := f.registry.OpenRuntimeDispatch(lifecycleLeader); err != nil {
		f.t.Fatalf("open runtime dispatch: %v", err)
	}
}

// publish puts the compiled adapter behind the addon identity at generation,
// the way the definition reconciler does after a successful compilation.
func (f *runtimeCheckFixture) publish(generation int64) registry.RuntimeRevision {
	f.t.Helper()
	revision := registry.RuntimeRevision{
		DefinitionUID: lifecycleDefUID, Generation: generation,
		SchemaVersion: fathomv1alpha1.GroupVersion.Version, SemanticsVersion: 1,
	}
	err := f.registry.SetRuntime(registry.RuntimeEntry{
		Adapter: f.stub, Revision: revision,
		Provenance: registry.RuntimeProvenance{OperatorBuild: lifecycleBuild, AdapterVersion: "1.0.0"},
	})
	if err != nil {
		f.t.Fatalf("publish runtime snapshot: %v", err)
	}
	return revision
}

func (f *runtimeCheckFixture) ready() registry.RuntimeRevision {
	f.t.Helper()
	f.admit()
	return f.publish(1)
}

func (f *runtimeCheckFixture) script(run func(context.Context, adapter.Request) (adapter.Result, error)) {
	if run == nil {
		f.stub.run = nil // restore the stub's default healthy run
		return
	}
	f.stub.run = func(ctx context.Context, req adapter.Request) (adapter.Result, error) {
		f.mu.Lock()
		f.evaluatorRuns++
		f.mu.Unlock()
		return run(ctx, req)
	}
}

func (f *runtimeCheckFixture) budget() *execution.Budget {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.budgets) == 0 {
		f.t.Fatal("no run budget was created")
	}
	return f.budgets[len(f.budgets)-1]
}

func (f *runtimeCheckFixture) run() (RuntimeAttempt, error) {
	f.t.Helper()
	return f.runner.Run(context.Background(), f.check())
}

func (f *runtimeCheckFixture) runOK() RuntimeAttempt {
	f.t.Helper()
	attempt, err := f.run()
	if err != nil {
		f.t.Fatalf("runtime check run: %v", err)
	}
	return attempt
}

func (f *runtimeCheckFixture) check() *fathomv1alpha1.AddonCheck {
	f.t.Helper()
	var check fathomv1alpha1.AddonCheck
	key := types.NamespacedName{Namespace: runtimeCheckNamespace, Name: runtimeCheckName}
	if err := f.store.Get(context.Background(), key, &check); err != nil {
		f.t.Fatalf("get AddonCheck: %v", err)
	}
	return &check
}

func (f *runtimeCheckFixture) definition() *fathomv1alpha1.AddonDefinition {
	f.t.Helper()
	var def fathomv1alpha1.AddonDefinition
	if err := f.store.Get(context.Background(), types.NamespacedName{Name: lifecycleAddon}, &def); err != nil {
		f.t.Fatalf("get AddonDefinition: %v", err)
	}
	return &def
}

func (f *runtimeCheckFixture) binding() *fathomv1alpha1.AddonDefinitionBinding {
	f.t.Helper()
	var b fathomv1alpha1.AddonDefinitionBinding
	key := types.NamespacedName{Namespace: lifecycleNamespace, Name: lifecycleAddon}
	if err := f.store.Get(context.Background(), key, &b); err != nil {
		f.t.Fatalf("get AddonDefinitionBinding: %v", err)
	}
	return &b
}

func (f *runtimeCheckFixture) update(obj client.Object) {
	f.t.Helper()
	if err := f.store.Update(context.Background(), obj); err != nil {
		f.t.Fatalf("update %T: %v", obj, err)
	}
}

func (f *runtimeCheckFixture) counters() (cachedFence, control, writes, runs int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cachedFenceReads, f.controlReads, f.statusWrites, f.evaluatorRuns
}

func runtimeReadyCondition(t *testing.T, check *fathomv1alpha1.AddonCheck) *metav1.Condition {
	t.Helper()
	return apiMeta.FindStatusCondition(check.Status.Conditions, addonCheckConditionReady)
}

// ---------------------------------------------------------------------------
// T039 — pre-run and final uncached control-plane fences
// ---------------------------------------------------------------------------

// Every field is mandatory and has no usable zero value. A mis-wired manager
// must refuse before any bounded work rather than execute a definition with a
// guessed namespace, an unattributable identity or no leadership decision.
func TestRuntimeCheckRunnerRefusesToRunMisWired(t *testing.T) {
	for _, tc := range []struct {
		name   string
		break_ func(*AddonCheckRuntimeRunner)
	}{
		{"no client", func(r *AddonCheckRuntimeRunner) { r.Client = nil }},
		{"no registry", func(r *AddonCheckRuntimeRunner) { r.Registry = nil }},
		{"no leadership session", func(r *AddonCheckRuntimeRunner) { r.Session = nil }},
		{"no per-run client factory", func(r *AddonCheckRuntimeRunner) { r.Clients = nil }},
		{"no operator namespace", func(r *AddonCheckRuntimeRunner) { r.OperatorNamespace = "" }},
		{"no manager service account", func(r *AddonCheckRuntimeRunner) { r.ManagerServiceAccount = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			tc.break_(f.runner)

			if _, err := f.run(); err == nil {
				t.Fatal("a mis-wired runner returned no error; the mis-wiring would be invisible")
			}
			_, _, writes, runs := f.counters()
			if runs != 0 || writes != 0 {
				t.Fatalf("a mis-wired runner executed %d times and wrote status %d times", runs, writes)
			}
		})
	}
}

// contracts/runtime.md: "Control-plane metadata/final fences use uncached
// APIReader and never enter the evaluator." A cached read fences nothing: the
// informer may not yet have seen the revocation that matters.
func TestRuntimeFencesNeverReadThroughTheManagerCache(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	attempt := f.runOK()
	if !attempt.Completed {
		t.Fatalf("run did not complete: %+v", attempt)
	}
	cachedFence, control, _, _ := f.counters()
	if cachedFence != 0 {
		t.Errorf("the fences made %d cached reads; every fence read must use the uncached control reader", cachedFence)
	}
	if control == 0 {
		t.Error("the fences made no uncached control reads at all")
	}
}

// T039 names nine captured facts plus the leadership epoch. Each is a separate
// way a run can be attributed to state that no longer holds, so each is
// asserted individually rather than by one struct comparison.
func TestRuntimeFenceCapturesTheFullPublicationContext(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	revision := f.ready()

	attempt := f.runOK()
	fence := attempt.Fence
	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{"definition UID", fence.DefinitionUID, lifecycleDefUID},
		{"definition generation", fence.DefinitionGeneration, int64(1)},
		{"runtime revision", fence.Revision, revision},
		{"binding UID", fence.BindingUID, lifecycleBindUID},
		{"binding spec generation", fence.BindingSpecGeneration, int64(1)},
		{"service account UID", fence.ServiceAccountUID, lifecycleSAUID},
		{"check UID", fence.CheckUID, runtimeCheckUID},
		{"check generation", fence.CheckGeneration, int64(1)},
		{"check policy", fence.CheckPolicy, addonCheckPolicyFingerprint(f.check())},
	} {
		if tc.got != tc.want {
			t.Errorf("fence %s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
	if fence.Epoch == nil || !f.session.EpochValid(fence.Epoch) {
		t.Fatalf("fence captured no live leadership epoch: %+v", fence.Epoch)
	}
}

// contracts/leadership.md: "status writes do not change authority". A binding
// status write bumps resourceVersion but never metadata.generation, so a fence
// comparing resourceVersion would discard every completed run that raced an
// ordinary status update. The spec-edit row proves the fence is not simply
// blind to binding changes.
func TestStatusOnlyBindingWritesDoNotInvalidateAuthority(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(*runtimeCheckFixture)
		complete bool
		reason   string
	}{
		{
			name: "status-only write keeps authority",
			mutate: func(f *runtimeCheckFixture) {
				b := f.binding()
				b.Status.ObservedGeneration = b.Generation
				b.Status.ActiveRuns = 1
				b.Status.LeaderIdentity = lifecycleLeader
				if err := f.store.Status().Update(context.Background(), b); err != nil {
					f.t.Fatalf("binding status write: %v", err)
				}
			},
			complete: true,
			reason:   reasonRunCompleted,
		},
		{
			name: "spec edit invalidates the captured authority",
			mutate: func(f *runtimeCheckFixture) {
				b := f.binding()
				b.Spec.TargetScope.Namespaces = []fathomv1alpha1.DefinitionDNSLabel{"default", "kube-system"}
				// The fake client does not simulate the API server's generation
				// bump, so the edit sets the generation the API server would.
				b.Generation = 2
				f.update(b)
			},
			complete: false,
			reason:   reasonSuperseded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			f.runner.barrier = func(_ context.Context, phase string) {
				if phase == runtimeBarrierAfterExecute {
					tc.mutate(f)
				}
			}

			attempt := f.runOK()
			if attempt.Completed != tc.complete || attempt.Reason != tc.reason {
				t.Fatalf("attempt completed=%v reason=%q, want completed=%v reason=%q (%s)",
					attempt.Completed, attempt.Reason, tc.complete, tc.reason, attempt.Message)
			}
		})
	}
}

// contracts/leadership.md: a session that has observed no live Lease epoch
// cannot attribute anything, so nothing may execute under it. The refusal has to
// come BEFORE execution: rejecting only at publication would still have spent a
// delegated identity's reads on behalf of a leader nobody can name.
func TestNoRunStartsBeforeALiveLeaderEpochIsObserved(t *testing.T) {
	for _, tc := range []struct {
		name   string
		break_ func(*fakeRuntimeSession)
	}{
		{"no epoch observed yet", func(s *fakeRuntimeSession) { s.epoch = nil }},
		{"the session no longer holds its epoch", func(s *fakeRuntimeSession) { s.epochOK = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			tc.break_(f.session)

			attempt := f.runOK()
			if attempt.Published || attempt.Completed {
				t.Fatalf("a run without a live leader epoch published: %+v", attempt)
			}
			if attempt.Reason != reasonAuthorizationRevoked {
				t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, reasonAuthorizationRevoked, attempt.Message)
			}
			cachedFence, control, writes, runs := f.counters()
			if runs != 0 || writes != 0 || cachedFence != 0 {
				t.Fatalf("an unattributable run executed %d times, wrote %d times and made %d cached reads", runs, writes, cachedFence)
			}
			if control != 0 {
				t.Fatalf("an unattributable run made %d delegated-identity control reads before refusing", control)
			}
		})
	}
}

// T039: "share outer budgets across manager fences and isolated evaluator
// requests". One Budget covers both directions, so work spent on either side is
// visible to the other. Two independent budgets would let each of these runs
// succeed.
func TestControlFencesAndEvaluatorShareOneOuterBudget(t *testing.T) {
	t.Run("fence work is charged to the budget the evaluator runs under", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.ready()
		f.chargeControlReads = true
		f.preCharge = limits.MaxRunRequests // leave the pre-run fence nothing to spend

		attempt := f.runOK()
		if _, _, _, runs := f.counters(); runs != 0 {
			t.Fatalf("the evaluator ran %d times on an exhausted budget", runs)
		}
		if attempt.Completed || attempt.Reason != "WorkLimitExceeded" {
			t.Fatalf("attempt = %+v, want an exhausted-budget WorkLimitExceeded refusal", attempt)
		}
	})

	t.Run("the budget the fences were built with is the budget the evaluator runs under", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.ready()
		f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
			// Spend the budget the per-run clients were built with, exactly as
			// the delegated transport does when it charges a read -- and then
			// return SUCCESS. Nothing else in this run reports a failure, so the
			// only way the attempt below can fail is if that budget really is
			// the one execution.Execute and the final fence use. Two budgets
			// would leave the run completed and published.
			b := f.budget()
			for i := 0; i <= limits.MaxRunRequests; i++ {
				if err := b.ChargeRequest(); err != nil {
					break
				}
			}
			return adapter.Result{}, nil
		})

		attempt := f.runOK()
		if attempt.Published || attempt.Completed {
			t.Fatalf("a run that exhausted the shared budget published anyway: %+v", attempt)
		}
		if attempt.Reason != "WorkLimitExceeded" {
			t.Fatalf("attempt reason = %q, want WorkLimitExceeded (%s)", attempt.Reason, attempt.Message)
		}
	})
}

// contracts/runtime.md: "Time | min(AddonCheck.spec.timeout, 30s) ... One
// deadline covers discovery/reads/evaluation/final validation".
func TestRunDeadlineIsTheCheckTimeoutAndOneBudgetCoversTheWholeRun(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	before := time.Now()
	f.runOK()
	deadline, ok := f.budget().Context().Deadline()
	if !ok {
		t.Fatal("the run budget carries no deadline")
	}
	// before is read just outside the run, so the grant is at most the timeout
	// plus the few microseconds between that reading and NewBudget. The lower
	// bound is far tighter than either direction a lost clamp moves it: dropping
	// the check timeout lands at the 30s runtime cap, and halving it at 1.5s.
	if granted := deadline.Sub(before); granted > runtimeCheckTimeout+time.Second/10 || granted <= runtimeCheckTimeout*3/4 {
		t.Fatalf("the run was granted %v, want (%v, %v]", granted, runtimeCheckTimeout*3/4, runtimeCheckTimeout)
	}
	f.mu.Lock()
	calls := f.factoryCalls
	f.mu.Unlock()
	if calls != 1 {
		t.Fatalf("the per-run client factory was called %d times; one run has exactly one budget", calls)
	}
}

// T039: "failed validation prevents publication". A pre-run fence failure stops
// before the evaluator exists, so nothing runs under authority the control
// plane no longer grants and nothing is written.
func TestFailedPreRunValidationPreventsExecutionAndPublication(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runtimeCheckFixture)
		reason string
	}{
		{"binding disabled", func(f *runtimeCheckFixture) {
			b := f.binding()
			b.Spec.Enabled = false
			f.update(b)
		}, reasonAuthorizationRevoked},
		{"service account replaced", func(f *runtimeCheckFixture) {
			b := f.binding()
			b.Spec.ServiceAccountRef.UID = "a-different-service-account-uid"
			f.update(b)
		}, reasonBindingMismatch},
		{"stored definition is invalid", func(f *runtimeCheckFixture) {
			d := f.definition()
			d.Spec.AdapterVersion = "not-a-semver"
			f.update(d)
		}, reasonInvalidDefinition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			tc.mutate(f)

			attempt := f.runOK()
			if attempt.Published || attempt.Completed {
				t.Fatalf("a failed pre-run fence published anyway: %+v", attempt)
			}
			if attempt.Reason != tc.reason {
				t.Errorf("reason = %q, want %q (%s)", attempt.Reason, tc.reason, attempt.Message)
			}
			_, _, _, runs := f.counters()
			if runs != 0 {
				t.Errorf("the evaluator ran %d times behind a failed pre-run fence", runs)
			}
			// A failed fence records the ATTEMPT (T044) but publishes no
			// evidence: "failed fences never renew completed evidence".
			if evidence := f.check().Status.LastSuccessfulEvaluation; evidence != nil {
				t.Errorf("a failed pre-run fence published evidence: %+v", evidence)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T040 — publication precedence, serialization/CAS and observed revocation
// ---------------------------------------------------------------------------

// contracts/runtime.md: "Publication precedence: authority revoked/mismatched;
// definition or check superseded; invalid input; recorded execution failure."
//
// This is an ORDER, not independent cases, so every row makes SEVERAL of them
// true at once and asserts that the earliest one wins. Each row strictly extends
// the row below it, so reordering any adjacent pair breaks at least one row.
// Three independent objects are what let them hold simultaneously: the binding
// carries the authority failure, the AddonCheck the superseded context, and the
// definition the invalid input.
//
// The fourth rank, a recorded execution failure, cannot be added to these rows:
// execution.Execute aborts the shared budget on any failure, and a run with no
// budget left cannot make the uncached re-reads the other three ranks are
// decided by. That is the matrix's own answer for that case — "Deadline, size,
// parser or read budget exhausted | Attempt Error with specific reason" — and it
// is covered below by TestARecordedExecutionFailureIsLastInThePrecedenceOrder
// and by the ranking table, which pins rank three above rank four directly.
func TestPublicationPrecedenceIsAnOrder(t *testing.T) {
	revokeAuthority := func(f *runtimeCheckFixture) {
		b := f.binding()
		b.Spec.Enabled = false
		f.update(b)
	}
	supersedeCheck := func(f *runtimeCheckFixture) {
		check := f.check()
		check.Spec.Policy = map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"health": {Enabled: ptr.To(false)}}
		// The fake client does not simulate the API server's generation bump,
		// so the edit sets the generation the API server would.
		check.Generation = 2
		f.update(check)
	}
	invalidateDefinition := func(f *runtimeCheckFixture) {
		d := f.definition()
		d.Spec.AdapterVersion = "not-a-semver"
		// A stored spec edit advances the generation. Setting it here is what
		// makes the row load-bearing: an implementation that reported BOTH
		// InvalidDefinition and Superseded for one invalid edit would report
		// the superseded one, and the matrix names the invalid one.
		d.Generation = 2
		f.update(d)
	}

	for _, tc := range []struct {
		name   string
		mutate []func(*runtimeCheckFixture)
		reason string
	}{
		{"authority beats superseded and invalid input",
			[]func(*runtimeCheckFixture){revokeAuthority, supersedeCheck, invalidateDefinition}, reasonAuthorizationRevoked},
		{"superseded beats invalid input",
			[]func(*runtimeCheckFixture){supersedeCheck, invalidateDefinition}, reasonSuperseded},
		{"invalid input is reported when it is the only failure",
			[]func(*runtimeCheckFixture){invalidateDefinition}, reasonInvalidDefinition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			// Every failure lands after the pre-run fence captured the context
			// it validated, so all of them are observed by the final one.
			f.runner.barrier = func(_ context.Context, phase string) {
				if phase != runtimeBarrierAfterExecute {
					return
				}
				for _, apply := range tc.mutate {
					apply(f)
				}
			}

			attempt := f.runOK()
			if attempt.Published {
				t.Fatalf("a run with %d simultaneous failures published: %+v", len(tc.mutate), attempt)
			}
			if attempt.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, tc.reason, attempt.Message)
			}
		})
	}
}

// The last rank: a run that failed nothing but its own execution reports its
// own recorded reason, and an observed revocation still outranks it — that
// second half is what TestObservedRevocationCancelsTheRunAndRequeuesCurrentInputs
// asserts, where the same run both fails and is revoked.
func TestARecordedExecutionFailureIsLastInThePrecedenceOrder(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{}, f.budget().Fail("WorkLimitExceeded", "object limit 1000 exceeded")
	})

	attempt := f.runOK()
	if attempt.Completed || attempt.Published {
		t.Fatalf("a failed run published: %+v", attempt)
	}
	if attempt.Reason != "WorkLimitExceeded" {
		t.Fatalf("reason = %q, want WorkLimitExceeded (%s)", attempt.Reason, attempt.Message)
	}
}

// The precedence order itself, pinned directly over synthetic candidates. It
// covers the one adjacent pair the behavioural table cannot construct — invalid
// input against a recorded execution failure — and proves the election is a
// minimum over the order rather than a first-match scan, by offering the
// candidates in reverse.
func TestPublicationPrecedenceRanking(t *testing.T) {
	authority := publicationCandidate{rank: publicationRankOf(reasonAuthorizationRevoked), reason: reasonAuthorizationRevoked}
	superseded := publicationCandidate{rank: publicationRankOf(reasonSuperseded), reason: reasonSuperseded}
	invalid := publicationCandidate{rank: publicationRankOf(reasonInvalidDefinition), reason: reasonInvalidDefinition}
	execution := publicationCandidate{rank: publicationRankOf("WorkLimitExceeded"), reason: "WorkLimitExceeded"}
	mismatch := publicationCandidate{rank: publicationRankOf(reasonBindingMismatch), reason: reasonBindingMismatch}

	for _, tc := range []struct {
		name       string
		candidates []publicationCandidate
		want       string
	}{
		{"nothing failed", nil, reasonRunCompleted},
		{"execution failure alone", []publicationCandidate{execution}, "WorkLimitExceeded"},
		{"invalid input beats execution failure", []publicationCandidate{execution, invalid}, reasonInvalidDefinition},
		{"superseded beats invalid input", []publicationCandidate{execution, invalid, superseded}, reasonSuperseded},
		{"authority beats everything", []publicationCandidate{execution, invalid, superseded, authority}, reasonAuthorizationRevoked},
		// An order needs a defined tie-break, or two equally-ranked facts —
		// a lost epoch and a replaced service account, say — report whichever
		// re-read happened to finish last. The FIRST candidate offered wins.
		{"a tie keeps the first candidate offered", []publicationCandidate{authority, mismatch}, reasonAuthorizationRevoked},
		{"and the tie-break does not depend on which reason it is", []publicationCandidate{mismatch, authority}, reasonBindingMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := electPublication(tc.candidates); got.reason != tc.want {
				t.Fatalf("elected %q, want %q", got.reason, tc.want)
			}
		})
	}

	// Every reason this path can publish must land somewhere in the order on
	// purpose. An unmapped authority or input reason silently becoming a
	// generic execution failure is exactly the drift this pins.
	for reason, want := range map[string]publicationRank{
		reasonAuthorizationRevoked:     rankAuthority,
		reasonBindingMismatch:          rankAuthority,
		reasonAccessDenied:             rankAuthority,
		reasonDefinitionUnavailable:    rankAuthority,
		reasonAuthorizationUnavailable: rankAuthority,
		reasonSuperseded:               rankSuperseded,
		reasonInvalidDefinition:        rankInvalidInput,
		reasonInvalidBinding:           rankInvalidInput,
		"DefinitionTooLarge":           rankInvalidInput,
		"InputLimitExceeded":           rankInvalidInput,
		"InvalidVersionRange":          rankInvalidInput,
		"ScopeDenied":                  rankInvalidInput,
		"WorkLimitExceeded":            rankExecution,
		"ResponseLimitExceeded":        rankExecution,
		"ResultLimitExceeded":          rankExecution,
		"ExecutionPanic":               rankExecution,
		reasonRuntimeTimeout:           rankExecution,
	} {
		if got := publicationRankOf(reason); got != want {
			t.Errorf("publicationRankOf(%q) = %d, want %d", reason, got, want)
		}
	}
}

// Matrix row: "Edited to valid revision | Replace snapshot; old run becomes
// Superseded". Supersession is measured against the revision the registry
// publishes now, not against a raw generation number, so a run that dispatched
// to a replaced snapshot cannot publish under the new revision's name.
func TestAReplacedSnapshotSupersedesACompletedRun(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.runner.barrier = func(_ context.Context, phase string) {
		if phase == runtimeBarrierAfterExecute {
			f.publish(2)
		}
	}

	attempt := f.runOK()
	if attempt.Published || attempt.Completed {
		t.Fatalf("a superseded run published: %+v", attempt)
	}
	if attempt.Reason != reasonSuperseded {
		t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, reasonSuperseded, attempt.Message)
	}
	if attempt.Requeue <= 0 {
		t.Fatal("a superseded run must enqueue the current revision")
	}
}

// Matrix row: "Edit during evaluation | Final revision/context mismatch discards
// completion | Superseded attempt recorded; enqueue current revision". The three
// ways an AddonCheck's own context can move are independently load-bearing: a
// generation bump with an unchanged policy, a policy change at an unchanged
// generation, and a recreation that resets both to the values the run captured.
func TestASupersededOrRecreatedAddonCheckDiscardsTheRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runtimeCheckFixture)
	}{
		{"the generation advanced with an unchanged policy", func(f *runtimeCheckFixture) {
			check := f.check()
			check.Spec.Paused = true
			// The fake client does not simulate the API server's generation
			// bump, so the edit sets the generation the API server would.
			check.Generation = 2
			f.update(check)
		}},
		{"the policy changed at an unchanged generation", func(f *runtimeCheckFixture) {
			check := f.check()
			check.Spec.Policy = map[string]fathomv1alpha1.AddonCheckFamilyPolicy{"health": {Enabled: ptr.To(false)}}
			f.update(check)
		}},
		{"the check was deleted and recreated at the same generation and policy", func(f *runtimeCheckFixture) {
			if err := f.store.Delete(context.Background(), f.check()); err != nil {
				f.t.Fatalf("delete AddonCheck: %v", err)
			}
			recreated := runtimeCheckObject()
			// A recreation resets everything the fence compares EXCEPT the UID,
			// so the UID is the only thing standing between this run and
			// evidence published against an object it never evaluated.
			recreated.UID = "addoncheck-uid-2"
			if err := f.store.Create(context.Background(), recreated); err != nil {
				f.t.Fatalf("recreate AddonCheck: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			f.runner.barrier = func(_ context.Context, phase string) {
				if phase == runtimeBarrierAfterExecute {
					tc.mutate(f)
				}
			}

			attempt := f.runOK()
			if attempt.Published {
				t.Fatalf("a run published against an AddonCheck it did not evaluate: %+v", attempt)
			}
			if attempt.Reason != reasonSuperseded {
				t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, reasonSuperseded, attempt.Message)
			}
			if attempt.Requeue <= 0 {
				t.Fatal("a superseded run must enqueue the current revision")
			}
		})
	}
}

// contracts/runtime.md: "Deadline wins over later response-limit errors after
// cancellation." The first recorded cause is the run's cause; a size error
// raised by an evaluator that ignored its cancellation cannot rename it.
func TestDeadlineWinsOverLaterResponseLimitErrors(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		b := f.budget()
		// The run observes its deadline first...
		_ = b.Abort(context.DeadlineExceeded)
		// ...and only then does a late evaluator report a size failure.
		return adapter.Result{}, b.Fail("ResponseLimitExceeded", "decoded response exceeds per-request cap")
	})

	attempt := f.runOK()
	if attempt.Published || attempt.Completed {
		t.Fatalf("a timed-out run published: %+v", attempt)
	}
	if attempt.Reason != reasonRuntimeTimeout {
		t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, reasonRuntimeTimeout, attempt.Message)
	}
}

// T040: per-check publication is serialized. A second publisher for the same
// check is refused outright rather than queued, so no reconcile blocks on
// another run and two writers can never interleave on one check.
func TestPublicationIsSerializedPerCheck(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	held := make(chan struct{})
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	// The second publisher must never be made to wait by the FIXTURE, or a
	// runner that dropped its serialization would deadlock here instead of
	// failing the assertion below. Only the first caller holds.
	var holding atomic.Bool
	defer releaseOnce()
	f.runner.barrier = func(_ context.Context, phase string) {
		if phase != runtimeBarrierBeforePublish {
			return
		}
		if holding.CompareAndSwap(false, true) {
			close(held)
			<-release
		}
	}

	first := make(chan RuntimeAttempt, 1)
	go func() {
		attempt, err := f.runner.Run(context.Background(), runtimeCheckObject())
		if err != nil {
			t.Error(err)
		}
		first <- attempt
	}()
	<-held

	second, err := f.runner.Run(context.Background(), runtimeCheckObject())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Published {
		t.Fatal("two publications for one check overlapped")
	}
	if second.Reason != reasonPublicationInFlight {
		t.Fatalf("second run reason = %q, want %q (%s)", second.Reason, reasonPublicationInFlight, second.Message)
	}
	if second.Requeue <= 0 {
		t.Fatal("a refused publication must requeue; otherwise the result is silently dropped")
	}

	releaseOnce()
	if attempt := <-first; !attempt.Published {
		t.Fatalf("the first run did not publish: %+v", attempt)
	}
}

// T040: publication is a compare-and-swap against the exact object the final
// fence read. A write landing in between loses the swap and publishes nothing,
// rather than clobbering a newer status.
func TestPublicationCompareAndSwapsAgainstTheFencedRevision(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	var once sync.Once
	f.runner.barrier = func(_ context.Context, phase string) {
		if phase != runtimeBarrierBeforePublish {
			return
		}
		once.Do(func() {
			check := f.check()
			check.Status.LastRunTrigger = "written-by-somebody-else"
			if err := f.store.Status().Update(context.Background(), check); err != nil {
				t.Fatalf("competing status write: %v", err)
			}
		})
	}

	attempt, err := f.run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if attempt.Published {
		t.Fatal("publication overwrote a status written after the fence read it")
	}
	if attempt.Reason != reasonPublicationConflict || attempt.Requeue <= 0 {
		t.Fatalf("attempt = %+v, want a requeued %s", attempt, reasonPublicationConflict)
	}
	if got := f.check().Status.LastRunTrigger; got != "written-by-somebody-else" {
		t.Fatalf("the competing write was clobbered: lastRunTrigger = %q", got)
	}
	if runtimeReadyCondition(t, f.check()) != nil {
		t.Fatal("a lost compare-and-swap still published a Ready condition")
	}
}

// Matrix row: "Revocation during run | Observed revocation cancels/rejects ...
// Latest attempt explains discard; no newly dated old verdict". The run is
// cancelled, nothing is published, the leadership slot is released so drain can
// reach zero, and the current inputs are requeued.
func TestObservedRevocationCancelsTheRunAndRequeuesCurrentInputs(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(func(ctx context.Context, _ adapter.Request) (adapter.Result, error) {
		f.session.revoke(lifecycleAddon)
		b := f.binding()
		b.Spec.Enabled = false
		f.update(b)
		<-ctx.Done()
		return adapter.Result{}, ctx.Err()
	})

	attempt := f.runOK()
	if attempt.Published || attempt.Completed || len(attempt.Evidence.Checks) != 0 {
		t.Fatalf("a revoked run published or carried evidence: %+v", attempt)
	}
	if attempt.Reason != reasonAuthorizationRevoked {
		t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, reasonAuthorizationRevoked, attempt.Message)
	}
	if attempt.Requeue <= 0 {
		t.Fatal("an observed revocation must requeue the current inputs")
	}
	if evidence := f.check().Status.LastSuccessfulEvaluation; evidence != nil {
		t.Fatalf("a revoked run published evidence: %+v", evidence)
	}
	if runs := f.session.activeRuns(lifecycleAddon); runs != 0 {
		t.Fatalf("the run left %d unreleased leadership slots; drain would never reach zero", runs)
	}
}

// An observed revocation outranks the read failure it causes. A run whose
// binding is revoked mid-fence sees its definition read fail too, and that read
// error describes the symptom; publishing it would tell an administrator to go
// looking for a deleted definition when what actually happened is that they
// revoked the binding. contracts/runtime.md puts authority first for exactly
// this reason.
func TestAnObservedRevocationOutranksTheReadFailureItCauses(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.onControlRead = func(b *execution.Budget, reads int) {
		if reads != 1 {
			return
		}
		f.session.revoke(lifecycleAddon)
		if err := f.store.Delete(context.Background(), f.definition()); err != nil {
			t.Errorf("delete definition: %v", err)
		}
		// Rendezvous on the observable condition rather than on a duration:
		// the revocation reaches the run through a context.AfterFunc goroutine,
		// and the fence read below must happen after the run has observed it.
		for i := 0; i < 100000 && b.Err() == nil; i++ {
			time.Sleep(time.Microsecond)
		}
	}

	attempt := f.runOK()
	if attempt.Published || attempt.Completed {
		t.Fatalf("a revoked run published: %+v", attempt)
	}
	if attempt.Reason != reasonAuthorizationRevoked {
		t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, reasonAuthorizationRevoked, attempt.Message)
	}
	if _, _, _, runs := f.counters(); runs != 0 {
		t.Fatalf("a run revoked inside its pre-run fence still executed %d times", runs)
	}
}

// contracts/leadership.md: "Lease loss/unknown epoch rejects publication; failed
// fences never renew completed evidence." A leadership change during a run is
// not observable from the objects the run read, so the final fence asks the live
// session rather than trusting the epoch it captured.
func TestALeadershipChangeDuringTheRunRejectsPublication(t *testing.T) {
	for _, tc := range []struct {
		name   string
		break_ func(*fakeRuntimeSession)
	}{
		{"leadership ended", func(s *fakeRuntimeSession) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.epochOK = false
		}},
		{"the Lease was recreated under a new holder", func(s *fakeRuntimeSession) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.epoch = &fathomv1alpha1.DefinitionLeaderEpoch{
				LeaseUID:         "lease-uid-2",
				HolderIdentity:   "a-different-holder",
				AcquireTime:      metav1.NewMicroTime(time.Unix(1700009999, 0).UTC()),
				LeaseTransitions: 4,
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			f.runner.barrier = func(_ context.Context, phase string) {
				if phase == runtimeBarrierAfterExecute {
					tc.break_(f.session)
				}
			}

			attempt := f.runOK()
			if attempt.Published {
				t.Fatalf("a run published under an epoch its session no longer holds: %+v", attempt)
			}
			if attempt.Reason != reasonAuthorizationRevoked {
				t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, reasonAuthorizationRevoked, attempt.Message)
			}
			if evidence := f.check().Status.LastSuccessfulEvaluation; evidence != nil {
				t.Fatalf("a rejected publication stored evidence: %+v", evidence)
			}
		})
	}
}

// The registry is the single dispatch enforcement point. Nothing it refuses may
// be executed, and a built-in identity is not this path's to run at all.
func TestUnresolvableIdentitiesAreNeverExecuted(t *testing.T) {
	t.Run("no definition and no built-in claims the identity", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.admit()

		attempt := f.runOK()
		if attempt.Reason != reasonUnknownAddonType {
			t.Fatalf("reason = %q, want %q", attempt.Reason, reasonUnknownAddonType)
		}
		if _, _, _, runs := f.counters(); runs != 0 {
			t.Fatalf("an unknown addon type executed %d times", runs)
		}
	})

	t.Run("runtime dispatch is not admitted", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.publish(1) // published, but the leadership session never opened the gate

		attempt := f.runOK()
		if attempt.Reason != registry.ReasonAdmissionClosed {
			t.Fatalf("reason = %q, want %q", attempt.Reason, registry.ReasonAdmissionClosed)
		}
		if f.session.admissions() != 0 {
			t.Fatal("a barred identity still took a leadership admission slot")
		}
	})

	t.Run("a built-in identity belongs to the built-in path", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.admit()
		if err := f.registry.Register(lifecycleStubAdapter{name: "builtin", addonTypes: []string{lifecycleAddon}}); err != nil {
			t.Fatalf("register built-in: %v", err)
		}

		_, err := f.run()
		if !errors.Is(err, errNotRuntimeAddonType) {
			t.Fatalf("err = %v, want errNotRuntimeAddonType so the built-in reconciler still owns the check", err)
		}
		if _, _, writes, runs := f.counters(); runs != 0 || writes != 0 {
			t.Fatalf("the runtime path touched a built-in check (runs=%d writes=%d)", runs, writes)
		}
	})
}

// contracts/runtime.md: "Ready denotes executable/completed, freshness denotes
// recency, and neither means Pass." A completed run whose every check failed is
// still Ready=True: readiness is eligibility and completion, never health.
func TestReadyMeansCompletedEligibilityNotAPassVerdict(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{Checks: []adapter.CheckResult{{
			Family: "health", Outcome: adapter.OutcomeFail, Summary: "deployment is unavailable",
		}}}, nil
	})

	// execution.Execute releases the leadership slot the moment evaluation
	// returns, which cancels the admission context. Releasing normally must not
	// be mistaken for a revocation -- and the mistake would arrive from the
	// watcher's own goroutine, so this waits for it rather than sampling once.
	f.runner.barrier = func(_ context.Context, phase string) {
		if phase != runtimeBarrierAfterExecute {
			return
		}
		for i := 0; i < 200; i++ {
			if f.budget().Err() != nil {
				t.Fatalf("releasing the run's slot cancelled the run: %v", f.budget().Err())
			}
			time.Sleep(100 * time.Microsecond)
		}
		if runs := f.session.activeRuns(lifecycleAddon); runs != 0 {
			t.Fatalf("the slot was still held %d times after evaluation returned", runs)
		}
	}

	attempt := f.runOK()
	if !attempt.Completed || !attempt.Published {
		t.Fatalf("a completed failing run was not published: %+v", attempt)
	}
	if len(attempt.Evidence.Checks) != 1 || attempt.Evidence.Checks[0].Outcome != adapter.OutcomeFail {
		t.Fatalf("evidence lost the failing verdict: %+v", attempt.Evidence)
	}
	ready := runtimeReadyCondition(t, f.check())
	if ready == nil || ready.Status != metav1.ConditionTrue || ready.Reason != reasonRunCompleted {
		t.Fatalf("Ready = %+v, want True/%s for a completed run", ready, reasonRunCompleted)
	}
	if ready.ObservedGeneration != 1 {
		t.Errorf("Ready.observedGeneration = %d, want the fenced generation 1", ready.ObservedGeneration)
	}
}

// A repeated completion with the same verdict advances the observation (data-
// model.md: "a newer valid revision with the same verdict ... current evidence
// updates") but must not re-transition Ready. Churning LastTransitionTime would
// make "since when has this been ready" unanswerable, and the historical report
// this run does not create is transition-only for the same reason.
//
// Note this test previously asserted zero status writes for an unchanged run.
// That is no longer the invariant: T044's latestAttemptAt records when the last
// attempt happened, so it advances on every attempt by construction — that is
// exactly what makes it distinguishable from the evidence's observedAt. The
// invariant that survives, and the one the contract actually names, is that no
// TRANSITION is manufactured.
func TestRepeatedCompletionsAdvanceTheObservationWithoutReTransitioningReady(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	if attempt := f.runOK(); !attempt.Published {
		t.Fatalf("first run did not publish: %+v", attempt)
	}
	first := f.check()
	firstReady := runtimeReadyCondition(t, first)
	if firstReady == nil {
		t.Fatal("the first completion set no Ready condition")
	}

	f.advance(time.Minute)
	if attempt := f.runOK(); !attempt.Completed {
		t.Fatalf("second run did not complete: %+v", attempt)
	}
	second := f.check()
	secondReady := runtimeReadyCondition(t, second)
	if secondReady == nil {
		t.Fatal("the second completion removed the Ready condition")
	}
	if !secondReady.LastTransitionTime.Equal(&firstReady.LastTransitionTime) {
		t.Errorf("Ready re-transitioned on an unchanged verdict: %s -> %s",
			firstReady.LastTransitionTime, secondReady.LastTransitionTime)
	}
	if got, want := second.Status.LastSuccessfulEvaluation.Verdict, first.Status.LastSuccessfulEvaluation.Verdict; got != want {
		t.Errorf("verdict changed on a repeated completion: %q -> %q", want, got)
	}
	if !second.Status.LastSuccessfulEvaluation.ObservedAt.Time.Equal(f.now()) {
		t.Errorf("observedAt = %s, want the second completion's time %s",
			second.Status.LastSuccessfulEvaluation.ObservedAt, f.now())
	}
	if second.Status.LatestAttemptAt == nil || !second.Status.LatestAttemptAt.Time.Equal(f.now()) {
		t.Errorf("latestAttemptAt = %v, want the second attempt's time %s", second.Status.LatestAttemptAt, f.now())
	}
}

// A publication write failing for a reason other than a lost compare-and-swap
// is a real error the caller must see and retry, not a silently dropped result.
func TestPublicationWriteFailuresAreReportedToTheCaller(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.runner.Client = interceptor.NewClient(f.store, interceptor.Funcs{
		SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
			return apierrors.NewInternalError(errors.New("etcd is unavailable"))
		},
	})

	attempt, err := f.run()
	if err == nil {
		t.Fatal("a failed publication write was swallowed")
	}
	if attempt.Published {
		t.Fatalf("attempt claims publication after a failed write: %+v", attempt)
	}
}

// ---------------------------------------------------------------------------
// T044 — completed evidence, separate attempts and derived freshness
// ---------------------------------------------------------------------------
//
// The three axes this section pins are independent on purpose:
//
//   - Ready says a run could execute and complete;
//   - lastSuccessfulEvaluation says what the last COMPLETED run observed, with
//     its ORIGINAL observedAt, revision and authority context;
//   - evidenceFreshness says whether that observation is still recent and still
//     backed by eligible inputs.
//
// Every row below breaks exactly one of them, because the bug this task exists
// to prevent — "old success presented as new coverage" — is always a collapse
// of two of the three into one.

// seedEvidence runs one successful evaluation so later rows have completed
// evidence to preserve, and returns the evidence exactly as stored.
func (f *runtimeCheckFixture) seedEvidence() *fathomv1alpha1.AddonCheckEvidence {
	f.t.Helper()
	if attempt := f.runOK(); !attempt.Published {
		f.t.Fatalf("seed run did not publish evidence: %+v", attempt)
	}
	evidence := f.evidence()
	if evidence == nil {
		f.t.Fatal("seed run published no completed evidence")
	}
	return evidence
}

func (f *runtimeCheckFixture) evidence() *fathomv1alpha1.AddonCheckEvidence {
	f.t.Helper()
	return f.check().Status.LastSuccessfulEvaluation
}

// A completed run records evidence carrying the exact revision and authority
// context the fence captured. data-model.md: "Current status stores
// lastSuccessfulEvaluation (completed Pass/Warn/Fail/Skipped evidence, original
// observedAt, revision and authority)".
func TestCompletedEvidenceCarriesItsOwnRevisionAndAuthorityContext(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	revision := f.ready()

	attempt := f.runOK()
	if !attempt.Published {
		t.Fatalf("a completed run published no evidence: %+v", attempt)
	}
	evidence := f.evidence()
	if evidence == nil {
		t.Fatal("a completed run stored no evidence")
	}
	if evidence.Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictPass {
		t.Errorf("verdict = %q, want Pass", evidence.Verdict)
	}
	if evidence.Coverage != fathomv1alpha1.AddonCheckCoverageChecksEvaluated {
		t.Errorf("coverage = %q, want ChecksEvaluated", evidence.Coverage)
	}
	if !evidence.ObservedAt.Time.Equal(f.now()) {
		t.Errorf("observedAt = %s, want the completion time %s", evidence.ObservedAt, f.now())
	}
	for _, tc := range []struct{ field, got, want string }{
		{"revision.definitionUID", evidence.Revision.DefinitionUID, string(lifecycleDefUID)},
		{"revision.schemaVersion", evidence.Revision.SchemaVersion, revision.SchemaVersion},
		{"revision.operatorBuild", evidence.Revision.OperatorBuild, lifecycleBuild},
		{"revision.adapterVersion", evidence.Revision.AdapterVersion, "1.0.0"},
		{"authority.bindingUID", evidence.Authority.BindingUID, string(attempt.Fence.BindingUID)},
		{"authority.serviceAccountUID", evidence.Authority.ServiceAccountUID, string(attempt.Fence.ServiceAccountUID)},
		{"authority.checkUID", evidence.Authority.CheckUID, string(runtimeCheckUID)},
		{"authority.policyDigest", evidence.Authority.PolicyDigest, attempt.Fence.CheckPolicy},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
	if evidence.Revision.DefinitionGeneration != revision.Generation {
		t.Errorf("revision.definitionGeneration = %d, want %d", evidence.Revision.DefinitionGeneration, revision.Generation)
	}
	if evidence.Revision.SemanticsVersion != revision.SemanticsVersion {
		t.Errorf("revision.semanticsVersion = %d, want %d", evidence.Revision.SemanticsVersion, revision.SemanticsVersion)
	}
	if evidence.Authority.CheckGeneration != attempt.Fence.CheckGeneration {
		t.Errorf("authority.checkGeneration = %d, want %d", evidence.Authority.CheckGeneration, attempt.Fence.CheckGeneration)
	}
	if evidence.Authority.BindingGeneration != attempt.Fence.BindingSpecGeneration {
		t.Errorf("authority.bindingGeneration = %d, want %d", evidence.Authority.BindingGeneration, attempt.Fence.BindingSpecGeneration)
	}
	if evidence.Authority.LeaderEpoch == nil || evidence.Authority.LeaderEpoch.HolderIdentity != lifecycleLeader {
		t.Errorf("authority.leaderEpoch = %+v, want the observed epoch", evidence.Authority.LeaderEpoch)
	}
	status := f.check().Status
	if status.LatestAttemptOutcome != fathomv1alpha1.AddonCheckAttemptCompleted {
		t.Errorf("latestAttemptOutcome = %q, want Completed", status.LatestAttemptOutcome)
	}
	if status.LatestAttemptAt == nil || !status.LatestAttemptAt.Time.Equal(f.now()) {
		t.Errorf("latestAttemptAt = %v, want the attempt time", status.LatestAttemptAt)
	}
	if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceCurrent {
		t.Errorf("evidenceFreshness = %q, want Current immediately after a completed run", status.EvidenceFreshness)
	}
}

// contracts/runtime.md: "Attempt Error still preserves previous completed
// evidence." Each row below is a lifecycle-matrix row whose "Evidence and
// recovery" column says the old observation is retained with its ORIGINAL time,
// revision and context, and freshness becomes Unavailable.
func TestAFailedAttemptPreservesEvidenceAndNeverRenewsItsObservationTime(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runtimeCheckFixture)
		reason string
	}{
		{"binding disabled", func(f *runtimeCheckFixture) {
			b := f.binding()
			b.Spec.Enabled = false
			f.update(b)
		}, reasonAuthorizationRevoked},
		{"service account replaced", func(f *runtimeCheckFixture) {
			b := f.binding()
			b.Spec.ServiceAccountRef.UID = "a-different-service-account-uid"
			f.update(b)
		}, reasonBindingMismatch},
		{"invalid edit stored", func(f *runtimeCheckFixture) {
			d := f.definition()
			d.Spec.AdapterVersion = "not-a-semver"
			f.update(d)
		}, reasonInvalidDefinition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			before := f.seedEvidence()

			// A failed attempt happens strictly later than the completed run,
			// so a renewed observedAt is observable rather than coincidental.
			f.advance(time.Minute)
			tc.mutate(f)

			attempt := f.runOK()
			if attempt.Published || attempt.Completed {
				t.Fatalf("a failed attempt published: %+v", attempt)
			}
			if attempt.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, tc.reason, attempt.Message)
			}
			after := f.evidence()
			if after == nil {
				t.Fatal("a failed attempt destroyed the completed evidence")
			}
			if !equality.Semantic.DeepEqual(before, after) {
				t.Fatalf("a failed attempt rewrote completed evidence:\nbefore %+v\nafter  %+v", before, after)
			}
			status := f.check().Status
			if status.LatestAttemptOutcome != fathomv1alpha1.AddonCheckAttemptError {
				t.Errorf("latestAttemptOutcome = %q, want Error", status.LatestAttemptOutcome)
			}
			if status.LatestAttemptAt == nil || !status.LatestAttemptAt.Time.Equal(f.now()) {
				t.Errorf("latestAttemptAt = %v, want the failed attempt's time %s", status.LatestAttemptAt, f.now())
			}
			if status.LatestAttemptReason != tc.reason {
				t.Errorf("latestAttemptReason = %q, want %q", status.LatestAttemptReason, tc.reason)
			}
			if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceUnavailable {
				t.Errorf("evidenceFreshness = %q, want Unavailable", status.EvidenceFreshness)
			}
			ready := runtimeReadyCondition(t, f.check())
			if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != tc.reason {
				t.Fatalf("Ready = %+v, want False/%s", ready, tc.reason)
			}
		})
	}
}

// Lifecycle matrix, "Missing definition": "Prior evidence retained with original
// time/revision/context, freshness=Unavailable; Unknown verdict only if none."
// Both halves of that row are asserted here, because the second half is the one
// that forbids inventing a verdict for a check that never ran.
func TestUnknownAddonTypeRetainsEvidenceAndReportsUnknownWhenThereIsNone(t *testing.T) {
	t.Run("prior evidence is retained and marked unavailable", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.ready()
		before := f.seedEvidence()

		f.advance(time.Minute)
		f.registry.RemoveRuntime(lifecycleDefUID)

		attempt := f.runOK()
		if attempt.Reason != reasonUnknownAddonType {
			t.Fatalf("reason = %q, want %q", attempt.Reason, reasonUnknownAddonType)
		}
		after := f.evidence()
		if after == nil || !equality.Semantic.DeepEqual(before, after) {
			t.Fatalf("a missing definition rewrote evidence:\nbefore %+v\nafter  %+v", before, after)
		}
		status := f.check().Status
		if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceUnavailable {
			t.Errorf("evidenceFreshness = %q, want Unavailable", status.EvidenceFreshness)
		}
		ready := runtimeReadyCondition(t, f.check())
		if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != reasonUnknownAddonType {
			t.Fatalf("Ready = %+v, want False/UnknownAddonType", ready)
		}
	})

	t.Run("a check that never completed has no evidence and no verdict", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.admit() // nothing published: the identity resolves to nothing at all

		attempt := f.runOK()
		if attempt.Reason != reasonUnknownAddonType {
			t.Fatalf("reason = %q, want %q", attempt.Reason, reasonUnknownAddonType)
		}
		status := f.check().Status
		if status.LastSuccessfulEvaluation != nil {
			t.Fatalf("a check that never completed invented evidence: %+v", status.LastSuccessfulEvaluation)
		}
		if status.LastResult != "" {
			t.Errorf("lastResult = %q, want empty; a never-run check has an Unknown verdict, not a stored one", status.LastResult)
		}
		if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceUnavailable {
			t.Errorf("evidenceFreshness = %q, want Unavailable when there is no evidence", status.EvidenceFreshness)
		}
	})
}

// Freshness answers two questions, and this is the first: what does an
// attempt's reason say about the ELIGIBILITY of the inputs the stored evidence
// came from? Every reason the runner can produce is enumerated, because a
// reason that silently falls through to the age test would present evidence
// from a deleted definition as merely recent.
func TestEvidenceEligibilityByAttemptReason(t *testing.T) {
	for _, tc := range []struct {
		reason string
		want   fathomv1alpha1.AddonCheckEvidenceFreshness
	}{
		// "Prior evidence retained ..., freshness=Unavailable": no dispatchable
		// snapshot claims the identity at all.
		{reasonUnknownAddonType, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{registry.ReasonBuiltinCollision, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{registry.ReasonRuntimeCollision, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{registry.ReasonAdmissionClosed, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		// Authority withdrawn, mismatched, denied or unestablished.
		{reasonAuthorizationRevoked, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{reasonBindingMismatch, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{reasonAccessDenied, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{reasonDefinitionUnavailable, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{reasonAuthorizationUnavailable, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		// Invalid input: "Invalid edit stored | ... freshness=Unavailable".
		{reasonInvalidDefinition, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{reasonInvalidBinding, fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{"InputLimitExceeded", fathomv1alpha1.AddonCheckEvidenceUnavailable},
		{"ScopeDenied", fathomv1alpha1.AddonCheckEvidenceUnavailable},
		// The revision or context was replaced, not the inputs withdrawn.
		{reasonSuperseded, fathomv1alpha1.AddonCheckEvidenceSuperseded},
		// A run that could not complete leaves its inputs perfectly eligible.
		// The matrix row "Deadline, size, parser or read budget exhausted"
		// names an attempt error and says nothing about the definition or
		// binding, so age alone decides here.
		{reasonRuntimeTimeout, fathomv1alpha1.AddonCheckEvidenceCurrent},
		{reasonRunCanceled, fathomv1alpha1.AddonCheckEvidenceCurrent},
		{reasonIncompleteEvaluation, fathomv1alpha1.AddonCheckEvidenceCurrent},
		{"WorkLimitExceeded", fathomv1alpha1.AddonCheckEvidenceCurrent},
		{"ResponseLimitExceeded", fathomv1alpha1.AddonCheckEvidenceCurrent},
		{"ExecutionFailed", fathomv1alpha1.AddonCheckEvidenceCurrent},
		{reasonRunCompleted, fathomv1alpha1.AddonCheckEvidenceCurrent},
	} {
		if got := addonCheckEvidenceEligibility(tc.reason); got != tc.want {
			t.Errorf("eligibility(%q) = %q, want %q", tc.reason, got, tc.want)
		}
	}
}

// The window is EFFECTIVE, not declared: a missing interval defaults and a
// stored sub-floor cadence is clamped, exactly as the run itself is paced, so
// the window always describes the schedule the check really runs on.
func TestFreshnessWindowIsTwoEffectiveIntervalsPlusOneEffectiveTimeout(t *testing.T) {
	for _, tc := range []struct {
		name     string
		interval *metav1.Duration
		timeout  *metav1.Duration
		want     time.Duration
	}{
		{"declared cadence", &metav1.Duration{Duration: time.Hour}, &metav1.Duration{Duration: 20 * time.Second},
			2*time.Hour + 20*time.Second},
		{"defaulted cadence", nil, nil,
			2*fathomv1alpha1.DefaultAddonCheckInterval + fathomv1alpha1.DefaultAddonCheckTimeout},
		{"sub-floor cadence is clamped, not honoured", &metav1.Duration{Duration: time.Second}, &metav1.Duration{Duration: time.Millisecond},
			2*fathomv1alpha1.MinCheckInterval + fathomv1alpha1.MinCheckTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check := runtimeCheckObject()
			check.Spec.Interval, check.Spec.Timeout = tc.interval, tc.timeout
			if got := AddonCheckEvidenceWindow(check); got != tc.want {
				t.Errorf("window = %s, want %s", got, tc.want)
			}
		})
	}

	// Evidence that does not exist is not aged. The distinction matters to the
	// HealthCheck mirror, which re-derives staleness at read time: "no evidence"
	// is Unavailable and an Unknown verdict, which is a different statement
	// from "this observation got old", and collapsing them would show a check
	// that has never run as one whose success expired.
	t.Run("absent evidence is not aged", func(t *testing.T) {
		check := runtimeCheckObject()
		if AddonCheckEvidenceAged(check, runtimeCheckClockStart.Add(1000*time.Hour)) {
			t.Error("a check with no evidence reported aged evidence")
		}
		check.Status.LastSuccessfulEvaluation = &fathomv1alpha1.AddonCheckEvidence{
			Verdict: fathomv1alpha1.AddonCheckEvidenceVerdictPass,
			// A zero observedAt is what a hand-edited or partially-migrated
			// status carries; it must read as ancient, never as brand new.
		}
		if !AddonCheckEvidenceAged(check, runtimeCheckClockStart) {
			t.Error("evidence with a zero observation time was treated as current")
		}
	})
}

// data-model.md: "Freshness is Current only for eligible input aged at most two
// effective intervals plus timeout; otherwise Stale". The boundary is inclusive
// and is asserted from both sides, so neither a strict comparison nor a wrong
// multiplier survives.
func TestFreshnessIsCurrentWithinTwoIntervalsPlusTimeoutAndStaleBeyond(t *testing.T) {
	// Spelled out from the contract rather than taken from the function under
	// test: the ages below must be independent of the formula they exercise,
	// or a wrong formula would move the boundary and the rows with it. The
	// formula itself is pinned separately, in
	// TestFreshnessWindowIsTwoEffectiveIntervalsPlusOneEffectiveTimeout.
	const window = 2*fathomv1alpha1.DefaultAddonCheckInterval + runtimeCheckTimeout

	for _, tc := range []struct {
		name string
		age  time.Duration
		want fathomv1alpha1.AddonCheckEvidenceFreshness
	}{
		{"just observed", 0, fathomv1alpha1.AddonCheckEvidenceCurrent},
		{"one interval short of the window", window - fathomv1alpha1.DefaultAddonCheckInterval, fathomv1alpha1.AddonCheckEvidenceCurrent},
		{"exactly at the window", window, fathomv1alpha1.AddonCheckEvidenceCurrent},
		{"one nanosecond past the window", window + time.Nanosecond, fathomv1alpha1.AddonCheckEvidenceStale},
		{"long past the window", 10 * window, fathomv1alpha1.AddonCheckEvidenceStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			observed := f.now()
			f.seedEvidence()

			// A second attempt at the chosen age re-derives freshness. The
			// evaluator fails, so the only thing that can change is freshness:
			// a completed run would reset the age to zero.
			f.advance(tc.age)
			f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
				return adapter.Result{}, errors.New("ExecutionFailed: the addon's API server is unreachable")
			})

			if attempt := f.runOK(); attempt.Completed {
				t.Fatalf("the ageing attempt completed and reset the evidence age: %+v", attempt)
			}
			status := f.check().Status
			if status.EvidenceFreshness != tc.want {
				t.Errorf("evidenceFreshness = %q at age %s, want %q", status.EvidenceFreshness, tc.age, tc.want)
			}
			if evidence := status.LastSuccessfulEvaluation; evidence == nil || !evidence.ObservedAt.Time.Equal(observed) {
				t.Errorf("observedAt = %v, want the original observation %s; ageing never renews it", evidence, observed)
			}
		})
	}
}

// Lifecycle matrix: "Evidence ages out | Freshness=Stale even if stored verdict
// was Pass | Next eligible run can replace it; last observation time unchanged."
// Stale is about recency alone, so the stored Pass survives verbatim.
func TestAgedPassEvidenceGoesStaleWithoutLosingItsPassVerdict(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	before := f.seedEvidence()
	if before.Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictPass {
		t.Fatalf("seed verdict = %q, want Pass", before.Verdict)
	}

	f.advance(2*fathomv1alpha1.DefaultAddonCheckInterval + runtimeCheckTimeout + time.Second)
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{}, errors.New("ExecutionFailed: the addon's API server is unreachable")
	})
	f.runOK()

	status := f.check().Status
	if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceStale {
		t.Fatalf("evidenceFreshness = %q, want Stale", status.EvidenceFreshness)
	}
	if !equality.Semantic.DeepEqual(before, status.LastSuccessfulEvaluation) {
		t.Fatalf("ageing rewrote the evidence:\nbefore %+v\nafter  %+v", before, status.LastSuccessfulEvaluation)
	}

	// A next eligible run replaces it, which is the second half of the row.
	f.script(nil)
	f.advance(time.Minute)
	if attempt := f.runOK(); !attempt.Published {
		t.Fatalf("the next eligible run did not replace aged evidence: %+v", attempt)
	}
	status = f.check().Status
	if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceCurrent {
		t.Errorf("evidenceFreshness = %q after a fresh completion, want Current", status.EvidenceFreshness)
	}
	if evidence := status.LastSuccessfulEvaluation; evidence == nil || !evidence.ObservedAt.Time.Equal(f.now()) {
		t.Errorf("observedAt = %v, want the new completion time %s", evidence, f.now())
	}
}

// contracts/runtime.md, "Clarification additions": "completed all-Skipped runs
// replace current evidence with Skipped and NoChecksEvaluated coverage" and
// "All-Skipped observedAt/revision/context advances after successful publication
// fences". This is the row that separates completed Skipped evidence from an
// attempt error: the one REPLACES, the other preserves.
func TestCompletedAllSkippedRunReplacesEvidenceAndAdvancesItsObservation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result adapter.Result
	}{
		{"every check skipped", adapter.Result{Checks: []adapter.CheckResult{
			{Family: "health", Outcome: adapter.OutcomeSkipped, Summary: "family disabled"},
			{Family: "health", Outcome: adapter.OutcomeSkipped, Summary: "no matching objects"},
		}}},
		// data-model.md: "Zero enabled checks follows the engine's explicit
		// Skipped sentinel." The engine never publishes an empty result --
		// runtime.SealResult rejects one as ExecutionFailed -- so a definition
		// with nothing enabled arrives here as exactly this one sentinel.
		{"the engine's Skipped sentinel for zero enabled checks", adapter.Result{Checks: []adapter.CheckResult{
			{Family: "health", Outcome: adapter.OutcomeSkipped, Summary: "no families are enabled"},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			seeded := f.seedEvidence()
			if seeded.Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictPass {
				t.Fatalf("seed verdict = %q, want Pass so the replacement is observable", seeded.Verdict)
			}

			f.advance(time.Minute)
			f.script(func(context.Context, adapter.Request) (adapter.Result, error) { return tc.result, nil })

			attempt := f.runOK()
			if !attempt.Completed || !attempt.Published {
				t.Fatalf("a completed all-Skipped run did not publish: %+v", attempt)
			}
			evidence := f.evidence()
			if evidence == nil {
				t.Fatal("a completed all-Skipped run removed the evidence instead of replacing it")
			}
			if evidence.Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictSkipped {
				t.Errorf("verdict = %q, want Skipped", evidence.Verdict)
			}
			if evidence.Coverage != fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated {
				t.Errorf("coverage = %q, want NoChecksEvaluated", evidence.Coverage)
			}
			if evidence.Message != fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage {
				t.Errorf("message = %q, want %q", evidence.Message, fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage)
			}
			if !evidence.ObservedAt.Time.Equal(f.now()) {
				t.Errorf("observedAt = %s, want the all-Skipped run's own time %s; it advances after the fences succeed",
					evidence.ObservedAt, f.now())
			}
			if evidence.Authority.LeaderEpoch == nil || evidence.Revision.DefinitionUID != string(lifecycleDefUID) {
				t.Errorf("all-Skipped evidence lost its revision/context: %+v", evidence)
			}
			status := f.check().Status
			if status.LatestAttemptOutcome != fathomv1alpha1.AddonCheckAttemptCompleted {
				t.Errorf("latestAttemptOutcome = %q, want Completed; all-Skipped is evidence, not a failed attempt", status.LatestAttemptOutcome)
			}
			if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceCurrent {
				t.Errorf("evidenceFreshness = %q, want Current", status.EvidenceFreshness)
			}
			ready := runtimeReadyCondition(t, f.check())
			if ready == nil || ready.Status != metav1.ConditionTrue {
				t.Errorf("Ready = %+v, want True: an all-Skipped run completed with eligible inputs", ready)
			}
		})
	}
}

// "Mixed results use existing aggregate semantics" and "Skipped never turns a
// Warn/Fail into Pass". A run with even one evaluated check is not
// NoChecksEvaluated, however many Skipped results sit beside it.
func TestMixedSkippedRunsKeepTheExistingAggregateAndReportEvaluatedCoverage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome adapter.Outcome
		want    fathomv1alpha1.AddonCheckEvidenceVerdict
	}{
		{"one pass beside skips", adapter.OutcomePass, fathomv1alpha1.AddonCheckEvidenceVerdictPass},
		{"one warn beside skips", adapter.OutcomeWarn, fathomv1alpha1.AddonCheckEvidenceVerdictWarn},
		{"one fail beside skips", adapter.OutcomeFail, fathomv1alpha1.AddonCheckEvidenceVerdictFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
				return adapter.Result{Checks: []adapter.CheckResult{
					{Family: "health", Outcome: adapter.OutcomeSkipped, Summary: "family disabled"},
					{Family: "health", Outcome: tc.outcome, Summary: "the one check that ran"},
					{Family: "health", Outcome: adapter.OutcomeSkipped, Summary: "no matching objects"},
				}}, nil
			})

			if attempt := f.runOK(); !attempt.Published {
				t.Fatalf("a mixed run did not publish: %+v", attempt)
			}
			evidence := f.evidence()
			if evidence == nil {
				t.Fatal("a mixed run stored no evidence")
			}
			if evidence.Verdict != tc.want {
				t.Errorf("verdict = %q, want %q; Skipped never raises or lowers an evaluated verdict", evidence.Verdict, tc.want)
			}
			if evidence.Coverage != fathomv1alpha1.AddonCheckCoverageChecksEvaluated {
				t.Errorf("coverage = %q, want ChecksEvaluated", evidence.Coverage)
			}
			if evidence.Message == fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage {
				t.Errorf("a run that evaluated a check claimed %q", fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage)
			}
			// The pre-existing external surface still describes the most recent
			// run, unchanged in meaning: lastResult is "the aggregate result
			// from the most recent adapter run".
			status := f.check().Status
			if status.LastResult != string(tc.want) {
				t.Errorf("lastResult = %q, want %q", status.LastResult, tc.want)
			}
			if status.LastRunTime == nil || !status.LastRunTime.Time.Equal(f.now()) {
				t.Errorf("lastRunTime = %v, want the completion time %s", status.LastRunTime, f.now())
			}
		})
	}
}

// contracts/runtime.md: "Ratio aggregation cannot average a failed family into
// Pass." Evidence uses the existing FAMILY-AWARE aggregate, not the plain
// worst-of fold, and the two rows below differ only in where the unhealthy
// ratio sits relative to the configured threshold — so a verdict computed by
// the wrong aggregate is wrong on exactly one of them.
func TestRatioThresholdedEvidenceUsesTheFamilyAwareAggregate(t *testing.T) {
	// One unhealthy member in a population of 20 is 5%: under a 10% failRatio
	// the family is Pass, which the plain worst-of fold would call Fail.
	// Twelve in 20 is 60%: past the threshold, and no averaging may rescue it.
	for _, tc := range []struct {
		name      string
		unhealthy int
		want      fathomv1alpha1.AddonCheckEvidenceVerdict
	}{
		{"unhealthy ratio under the fail threshold", 1, fathomv1alpha1.AddonCheckEvidenceVerdictPass},
		{"unhealthy ratio past the fail threshold", 12, fathomv1alpha1.AddonCheckEvidenceVerdictFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check := runtimeCheckObject()
			check.Spec.Policy = map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
				"health": {Enabled: ptr.To(true), Thresholds: map[string]fathomv1alpha1.ThresholdValue{
					adapter.ThresholdKeyFailRatio: "10",
				}},
			}
			f := newRuntimeCheckFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(), check)
			f.ready()
			f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
				var checks []adapter.CheckResult
				for i := 0; i < 20; i++ {
					outcome := adapter.OutcomePass
					if i < tc.unhealthy {
						outcome = adapter.OutcomeFail
					}
					checks = append(checks, adapter.CheckResult{Family: "health", Outcome: outcome, Summary: "member"})
				}
				return adapter.Result{Checks: checks}, nil
			})

			if attempt := f.runOK(); !attempt.Published {
				t.Fatalf("the ratio run did not publish: %+v", attempt)
			}
			evidence := f.evidence()
			if evidence == nil || evidence.Verdict != tc.want {
				t.Fatalf("verdict = %+v, want %q for %d/20 unhealthy against a 10%% failRatio",
					evidence, tc.want, tc.unhealthy)
			}
		})
	}
}

// A check-level Error cannot reach this package: runtime.SealResult rejects a
// result carrying one as ExecutionFailed before evidence is ever sealed. That
// is asserted here rather than assumed, because the narrow evidence enum
// (Pass/Warn/Fail/Skipped) only stays complete while it holds.
func TestACheckLevelErrorNeverBecomesCompletedEvidence(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	before := f.seedEvidence()

	f.advance(time.Minute)
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{Checks: []adapter.CheckResult{
			{Family: "health", Outcome: adapter.OutcomeError, Summary: "the target's state is undeterminable"},
		}}, nil
	})

	attempt := f.runOK()
	if attempt.Completed || attempt.Published {
		t.Fatalf("a run carrying a check-level Error was sealed as completed evidence: %+v", attempt)
	}
	after := f.evidence()
	if after == nil || !equality.Semantic.DeepEqual(before, after) {
		t.Fatalf("an undeterminable run overwrote evidence:\nbefore %+v\nafter  %+v", before, after)
	}
	status := f.check().Status
	if status.LatestAttemptOutcome != fathomv1alpha1.AddonCheckAttemptError {
		t.Errorf("latestAttemptOutcome = %q, want Error", status.LatestAttemptOutcome)
	}
}

// The fold and the narrowing are pure, so they are driven directly. The
// narrowing's Error/Unknown rows are the schema guard: an aggregate outside the
// completed-evidence enum must never be written, because a status carrying an
// out-of-enum verdict is rejected by the API server outright — a health
// observation would become a controller error.
func TestCompletedEvidenceFoldAndNarrowing(t *testing.T) {
	t.Run("fold", func(t *testing.T) {
		skipped := adapter.CheckResult{Family: "health", Outcome: adapter.OutcomeSkipped}
		for _, tc := range []struct {
			name     string
			checks   []adapter.CheckResult
			verdict  fathomv1alpha1.HealthReportResult
			coverage fathomv1alpha1.AddonCheckEvidenceCoverage
			message  string
		}{
			{"no checks at all", nil,
				fathomv1alpha1.HealthReportResultSkipped,
				fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated,
				fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage},
			{"only skips", []adapter.CheckResult{skipped, skipped},
				fathomv1alpha1.HealthReportResultSkipped,
				fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated,
				fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage},
			{"one pass among skips", []adapter.CheckResult{skipped, {Family: "health", Outcome: adapter.OutcomePass}},
				fathomv1alpha1.HealthReportResultPass,
				fathomv1alpha1.AddonCheckCoverageChecksEvaluated, "2 checks evaluated"},
			{"one fail among skips", []adapter.CheckResult{skipped, {Family: "health", Outcome: adapter.OutcomeFail}},
				fathomv1alpha1.HealthReportResultFail,
				fathomv1alpha1.AddonCheckCoverageChecksEvaluated, "2 checks evaluated"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				verdict, coverage, message := addonCheckEvidenceOutcome(
					runtimeCheckObject(), adapter.Result{Checks: tc.checks})
				if verdict != tc.verdict || coverage != tc.coverage || message != tc.message {
					t.Errorf("fold = (%q, %q, %q), want (%q, %q, %q)",
						verdict, coverage, message, tc.verdict, tc.coverage, tc.message)
				}
			})
		}
	})

	t.Run("narrowing", func(t *testing.T) {
		for _, tc := range []struct {
			aggregate fathomv1alpha1.HealthReportResult
			want      fathomv1alpha1.AddonCheckEvidenceVerdict
			ok        bool
		}{
			{fathomv1alpha1.HealthReportResultPass, fathomv1alpha1.AddonCheckEvidenceVerdictPass, true},
			{fathomv1alpha1.HealthReportResultWarn, fathomv1alpha1.AddonCheckEvidenceVerdictWarn, true},
			{fathomv1alpha1.HealthReportResultFail, fathomv1alpha1.AddonCheckEvidenceVerdictFail, true},
			{fathomv1alpha1.HealthReportResultSkipped, fathomv1alpha1.AddonCheckEvidenceVerdictSkipped, true},
			{fathomv1alpha1.HealthReportResultError, "", false},
			{fathomv1alpha1.HealthReportResultUnknown, "", false},
			{"", "", false},
		} {
			got, ok := addonCheckCompletedVerdict(tc.aggregate)
			if got != tc.want || ok != tc.ok {
				t.Errorf("narrow(%q) = (%q, %v), want (%q, %v)", tc.aggregate, got, ok, tc.want, tc.ok)
			}
		}
	})
}

// A superseded completion may not publish, and the freshness it reports names
// supersession rather than unavailability: the inputs are fine, the revision
// this run was attributed to is not.
func TestASupersededRunReportsSupersededFreshnessAndKeepsItsEvidence(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	before := f.seedEvidence()

	f.advance(time.Minute)
	f.runner.barrier = func(_ context.Context, phase string) {
		if phase == runtimeBarrierAfterExecute {
			f.publish(2) // a valid edit compiled to a newer revision
		}
	}

	attempt := f.runOK()
	if attempt.Published || attempt.Reason != reasonSuperseded {
		t.Fatalf("attempt = %+v, want an unpublished Superseded outcome", attempt)
	}
	after := f.evidence()
	if after == nil || !equality.Semantic.DeepEqual(before, after) {
		t.Fatalf("a superseded run rewrote evidence:\nbefore %+v\nafter  %+v", before, after)
	}
	status := f.check().Status
	if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceSuperseded {
		t.Errorf("evidenceFreshness = %q, want Superseded", status.EvidenceFreshness)
	}
}

// A completed run is described by publication alone. It must never also be
// recorded as an attempt — that would overwrite Ready=True/RunCompleted with
// Ready=False and claim latestAttemptOutcome=Error for a run that succeeded.
//
// The clock is deliberately frozen so the second run's computed status is
// byte-identical to the stored one and publication makes NO write at all. That
// removes the compare-and-swap that would otherwise mask the mistake: with a
// write behind it, a stray attempt record loses the swap and disappears; with
// no write behind it, it lands.
func TestACompletedRunIsNeverAlsoRecordedAsAFailedAttempt(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.seedEvidence()

	writesAfterSeed := func() int {
		_, _, writes, _ := f.counters()
		return writes
	}
	seeded := writesAfterSeed()

	if attempt := f.runOK(); !attempt.Completed {
		t.Fatalf("the second run did not complete: %+v", attempt)
	}
	if writes := writesAfterSeed(); writes != seeded {
		t.Fatalf("an identical completed run wrote status %d more times", writes-seeded)
	}
	status := f.check().Status
	if status.LatestAttemptOutcome != fathomv1alpha1.AddonCheckAttemptCompleted {
		t.Errorf("latestAttemptOutcome = %q, want Completed", status.LatestAttemptOutcome)
	}
	if status.LatestAttemptReason != reasonRunCompleted {
		t.Errorf("latestAttemptReason = %q, want %q", status.LatestAttemptReason, reasonRunCompleted)
	}
	ready := runtimeReadyCondition(t, f.check())
	if ready == nil || ready.Status != metav1.ConditionTrue || ready.Reason != reasonRunCompleted {
		t.Fatalf("Ready = %+v, want True/%s", ready, reasonRunCompleted)
	}
}

// Recording an attempt is a compare-and-swap against the object the caller
// read. Losing it means something newer already describes this check, and an
// attempt record is worth strictly less than whatever won — so the swap is
// dropped. Any OTHER write failure is a real error the caller must see, or a
// broken status path would look like a check that simply never ran.
func TestAttemptRecordingDropsALostSwapButSurfacesARealWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		wantErr bool
	}{
		{"lost compare-and-swap", apierrors.NewConflict(
			schema.GroupResource{Group: fathomv1alpha1.GroupVersion.Group, Resource: "addonchecks"},
			runtimeCheckName, errors.New("the object has been modified")), false},
		{"the status path is broken", apierrors.NewInternalError(errors.New("etcd is unavailable")), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			b := f.binding()
			b.Spec.Enabled = false
			f.update(b)
			f.runner.Client = interceptor.NewClient(f.store, interceptor.Funcs{
				SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
					return tc.failure
				},
			})

			attempt, err := f.run()
			if tc.wantErr && err == nil {
				t.Fatal("a broken status path was swallowed")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("a lost compare-and-swap was reported as an error: %v", err)
			}
			// Either way the attempt itself still explains what happened.
			if attempt.Reason != reasonAuthorizationRevoked {
				t.Errorf("reason = %q, want %q", attempt.Reason, reasonAuthorizationRevoked)
			}
		})
	}
}

// A process that is not the leader does not write this check's status at all.
// contracts/leadership.md: losing the session "closes admission, cancels workers
// and prevents further evidence/drain publication".
func TestANonLeaderRecordsNothingAtAll(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.session.epochOK = false

	attempt := f.runOK()
	if attempt.Reason != reasonAuthorizationRevoked {
		t.Fatalf("reason = %q, want %q", attempt.Reason, reasonAuthorizationRevoked)
	}
	if _, _, writes, _ := f.counters(); writes != 0 {
		t.Fatalf("a process without a live epoch wrote this check's status %d times", writes)
	}
}

// contracts/runtime.md bounds every status string: "Strings <=1,024 UTF-8 bytes
// and 1,024 code points". An unbounded adapter message would be rejected by the
// API server's maxLength, turning a health observation into a write failure.
func TestRecordedTextIsBoundedToTheSchemaLimits(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{}, errors.New("ExecutionFailed: " + strings.Repeat("é", 4000))
	})

	f.runOK()
	status := f.check().Status
	for _, tc := range []struct {
		field string
		value string
		max   int
	}{
		{"latestAttemptMessage", status.LatestAttemptMessage, addonCheckStatusTextLimit},
		{"latestAttemptReason", status.LatestAttemptReason, addonCheckStatusReasonLimit},
		{"evidenceFreshnessReason", status.EvidenceFreshnessReason, addonCheckStatusTextLimit},
	} {
		if len(tc.value) > tc.max {
			t.Errorf("%s is %d bytes, over the %d-byte bound", tc.field, len(tc.value), tc.max)
		}
		if n := utf8.RuneCountInString(tc.value); n > tc.max {
			t.Errorf("%s is %d code points, over the %d-code-point bound", tc.field, n, tc.max)
		}
		if !utf8.ValidString(tc.value) {
			t.Errorf("%s was truncated mid-rune and is not valid UTF-8", tc.field)
		}
	}
	if status.LatestAttemptMessage == "" {
		t.Error("bounding dropped the message entirely")
	}
}

// The schema markers are only worth what a real API server enforces, and no
// fake client enforces any of them: it neither validates enums nor applies
// maxLength. This spec runs against the envtest API server with the generated
// config/crd/bases installed, so it proves the bounds the runner truncates to
// are the bounds that actually exist — and that an out-of-enum verdict is
// rejected rather than silently stored.
var _ = Describe("AddonCheck runtime evidence schema", func() {
	newCheck := func(name string) *fathomv1alpha1.AddonCheck {
		return &fathomv1alpha1.AddonCheck{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
			Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "schema-probe"},
		}
	}

	It("stores completed evidence, the latest attempt and derived freshness", func(ctx SpecContext) {
		check := newCheck("evidence-roundtrip")
		Expect(k8sClient.Create(ctx, check)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed())
		})

		observed := metav1.NewTime(runtimeCheckClockStart)
		check.Status.LastSuccessfulEvaluation = &fathomv1alpha1.AddonCheckEvidence{
			Verdict:    fathomv1alpha1.AddonCheckEvidenceVerdictSkipped,
			Coverage:   fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated,
			Message:    fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage,
			ObservedAt: observed,
			Revision: fathomv1alpha1.AddonCheckEvidenceRevision{
				DefinitionUID: string(lifecycleDefUID), DefinitionGeneration: 3,
				SchemaVersion: "v1alpha1", SemanticsVersion: 1,
				AdapterVersion: "1.2.3", OperatorBuild: lifecycleBuild,
			},
			Authority: fathomv1alpha1.AddonCheckEvidenceAuthority{
				BindingUID: "binding-uid-1", BindingGeneration: 2,
				ServiceAccountUID: "sa-uid-1", CheckUID: "check-uid-1", CheckGeneration: 4,
				PolicyDigest: strings.Repeat("a", 64),
				LeaderEpoch: &fathomv1alpha1.DefinitionLeaderEpoch{
					LeaseUID: "lease-uid-1", HolderIdentity: lifecycleLeader,
					AcquireTime: metav1.NewMicroTime(runtimeCheckClockStart), LeaseTransitions: 3,
				},
			},
		}
		check.Status.LatestAttemptAt = &metav1.Time{Time: runtimeCheckClockStart.Add(time.Hour)}
		check.Status.LatestAttemptOutcome = fathomv1alpha1.AddonCheckAttemptError
		check.Status.LatestAttemptReason = reasonAuthorizationRevoked
		check.Status.LatestAttemptMessage = strings.Repeat("m", addonCheckStatusTextLimit)
		check.Status.EvidenceFreshness = fathomv1alpha1.AddonCheckEvidenceUnavailable
		check.Status.EvidenceFreshnessReason = strings.Repeat("r", addonCheckStatusTextLimit)
		Expect(k8sClient.Status().Update(ctx, check)).To(Succeed())

		var stored fathomv1alpha1.AddonCheck
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(check), &stored)).To(Succeed())
		Expect(stored.Status.LastSuccessfulEvaluation).NotTo(BeNil())
		Expect(stored.Status.LastSuccessfulEvaluation.Verdict).
			To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictSkipped))
		Expect(stored.Status.LastSuccessfulEvaluation.Coverage).
			To(Equal(fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated))
		Expect(stored.Status.LastSuccessfulEvaluation.Message).
			To(Equal(fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage))
		Expect(stored.Status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally("==", observed.Time))
		Expect(stored.Status.LastSuccessfulEvaluation.Authority.LeaderEpoch).NotTo(BeNil())
		Expect(stored.Status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptError))
		Expect(stored.Status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceUnavailable))
		// The pre-existing surface is untouched by the new fields.
		Expect(stored.Status.LastResult).To(BeEmpty())
	})

	DescribeTable("rejects values outside the declared bounds",
		func(ctx SpecContext, name string, mutate func(*fathomv1alpha1.AddonCheckStatus)) {
			check := newCheck(name)
			Expect(k8sClient.Create(ctx, check)).To(Succeed())
			DeferCleanup(func(ctx SpecContext) {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, check))).To(Succeed())
			})
			mutate(&check.Status)
			Expect(k8sClient.Status().Update(ctx, check)).NotTo(Succeed())
		},
		// Error is not completed evidence, and the enum is what makes that
		// unrepresentable rather than merely discouraged.
		//
		// Every evidence row below fills ObservedAt: it is a required field, and
		// a zero metav1.Time marshals to null, so omitting it would make the
		// write fail for a reason that has nothing to do with the bound under
		// test.
		Entry("an Error verdict", "reject-error-verdict", func(s *fathomv1alpha1.AddonCheckStatus) {
			s.LastSuccessfulEvaluation = &fathomv1alpha1.AddonCheckEvidence{
				Verdict:    "Error",
				Coverage:   fathomv1alpha1.AddonCheckCoverageChecksEvaluated,
				ObservedAt: metav1.NewTime(runtimeCheckClockStart),
			}
		}),
		Entry("an unknown coverage", "reject-unknown-coverage", func(s *fathomv1alpha1.AddonCheckStatus) {
			s.LastSuccessfulEvaluation = &fathomv1alpha1.AddonCheckEvidence{
				Verdict:    fathomv1alpha1.AddonCheckEvidenceVerdictPass,
				Coverage:   "SomeChecksEvaluated",
				ObservedAt: metav1.NewTime(runtimeCheckClockStart),
			}
		}),
		Entry("an unknown freshness", "reject-unknown-freshness", func(s *fathomv1alpha1.AddonCheckStatus) {
			s.EvidenceFreshness = "Fresh"
		}),
		Entry("an unknown attempt outcome", "reject-unknown-outcome", func(s *fathomv1alpha1.AddonCheckStatus) {
			s.LatestAttemptOutcome = "Failed"
		}),
		// The runner truncates to exactly these limits; one code point past
		// each must be refused, or the truncation would be decorative.
		Entry("an over-long attempt message", "reject-long-message", func(s *fathomv1alpha1.AddonCheckStatus) {
			s.LatestAttemptMessage = strings.Repeat("m", addonCheckStatusTextLimit+1)
		}),
		Entry("an over-long attempt reason", "reject-long-reason", func(s *fathomv1alpha1.AddonCheckStatus) {
			s.LatestAttemptReason = strings.Repeat("r", addonCheckStatusReasonLimit+1)
		}),
		Entry("an over-long freshness reason", "reject-long-freshness-reason", func(s *fathomv1alpha1.AddonCheckStatus) {
			s.EvidenceFreshnessReason = strings.Repeat("f", addonCheckStatusTextLimit+1)
		}),
		Entry("an over-long identifier", "reject-long-identifier", func(s *fathomv1alpha1.AddonCheckStatus) {
			s.LastSuccessfulEvaluation = &fathomv1alpha1.AddonCheckEvidence{
				Verdict:    fathomv1alpha1.AddonCheckEvidenceVerdictPass,
				Coverage:   fathomv1alpha1.AddonCheckCoverageChecksEvaluated,
				ObservedAt: metav1.NewTime(runtimeCheckClockStart),
				Authority: fathomv1alpha1.AddonCheckEvidenceAuthority{
					PolicyDigest: strings.Repeat("d", addonCheckStatusIdentifierLimit+1),
				},
			}
		}),
	)
})

// ---------------------------------------------------------------------------
// T045 — verdict transitions, HealthReport attribution and transition-only
// reporting
// ---------------------------------------------------------------------------
//
// Evidence answers "what did the last completed run observe"; a HealthReport
// answers "when did that answer CHANGE". contracts/runtime.md: "No-change
// verdicts do not create reports", and spec.md's clarification: a Pass->Skipped
// transition creates a report while "Skipped->Skipped only updates status".
//
// Every row below therefore asserts two things that are easy to conflate: what
// the run did to current evidence, and what it did (or did not) add to history.

// reports returns every stored HealthReport for the fixture's check, ordered by
// observation so "the first report" stays meaningful across rows.
func (f *runtimeCheckFixture) reports() []fathomv1alpha1.HealthReport {
	f.t.Helper()
	var list fathomv1alpha1.HealthReportList
	if err := f.store.List(context.Background(), &list, client.InNamespace(runtimeCheckNamespace)); err != nil {
		f.t.Fatalf("list HealthReports: %v", err)
	}
	sort.Slice(list.Items, func(i, j int) bool {
		if !list.Items[i].Spec.ObservedAt.Equal(&list.Items[j].Spec.ObservedAt) {
			return list.Items[i].Spec.ObservedAt.Before(&list.Items[j].Spec.ObservedAt)
		}
		return list.Items[i].Name < list.Items[j].Name
	})
	return list.Items
}

// runAndRecord performs one run and then offers its outcome to the AddonCheck
// reconciler's transition-only report path, exactly as T047's wiring will: the
// evidence stored BEFORE the run is what the transition is measured against,
// and the returned report name is persisted by the caller rather than by the
// runner, which knows nothing about history.
func (f *runtimeCheckFixture) runAndRecord() (RuntimeAttempt, string) {
	f.t.Helper()
	previous := f.evidence()
	attempt := f.runOK()

	published := f.check()
	name, err := (&AddonCheckReconciler{Client: f.cached, Scheme: f.scheme}).
		recordRuntimeTransition(context.Background(), logr.Discard(), published, previous, attempt)
	if err != nil {
		f.t.Fatalf("record runtime transition: %v", err)
	}
	if name != "" {
		if published.Status.LastReportName != name {
			f.t.Errorf("lastReportName = %q, want the created report %q; the caller persists what the path names",
				published.Status.LastReportName, name)
		}
		if err := f.store.Status().Update(context.Background(), published); err != nil {
			f.t.Fatalf("persist lastReportName: %v", err)
		}
	}
	return attempt, name
}

// skippedRun scripts a completed run that evaluated nothing.
func skippedRun(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{Checks: []adapter.CheckResult{
		{Family: "health", Outcome: adapter.OutcomeSkipped, Summary: "no families are enabled"},
	}}, nil
}

// assertReportAttribution pins a report to the exact evidence it was produced
// from. contracts/runtime.md's recovery column requires that "history remains
// attributable": a report that cannot name its revision and authority cannot be
// told apart from one produced under authority since revoked.
func assertReportAttribution(t *testing.T, report *fathomv1alpha1.HealthReport, evidence *fathomv1alpha1.AddonCheckEvidence) {
	t.Helper()
	attribution := report.Spec.Attribution
	if attribution == nil {
		t.Fatalf("report %s carries no attribution; it is not traceable to the revision that produced it", report.Name)
	}
	if !equality.Semantic.DeepEqual(attribution.Revision, evidence.Revision) {
		t.Errorf("attribution.revision = %+v, want the evidence revision %+v", attribution.Revision, evidence.Revision)
	}
	if !equality.Semantic.DeepEqual(attribution.Authority, evidence.Authority) {
		t.Errorf("attribution.authority = %+v, want the evidence authority %+v", attribution.Authority, evidence.Authority)
	}
	if attribution.Coverage != evidence.Coverage {
		t.Errorf("attribution.coverage = %q, want %q", attribution.Coverage, evidence.Coverage)
	}
	if string(report.Spec.Result) != string(evidence.Verdict) {
		t.Errorf("report result = %q, want the published verdict %q", report.Spec.Result, evidence.Verdict)
	}
	if !report.Spec.ObservedAt.Time.Equal(evidence.ObservedAt.Time) {
		t.Errorf("report observedAt = %s, want the evidence's own observation %s", report.Spec.ObservedAt, evidence.ObservedAt)
	}
}

// Pass->Skipped. spec.md: "A Pass->Skipped transition creates a report."
// The completed all-Skipped run REPLACES current evidence, and because the
// verdict changed it is also a history entry — attributed to the revision and
// authority that produced it, not to the Pass it replaced.
func TestPassToSkippedReplacesEvidenceAndCreatesATransitionReport(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	_, first := f.runAndRecord()
	if first == "" {
		t.Fatal("the first completed evaluation created no report; Unknown->Pass is a transition")
	}
	passEvidence := f.evidence()

	f.advance(time.Minute)
	f.script(skippedRun)
	_, second := f.runAndRecord()
	if second == "" || second == first {
		t.Fatalf("Pass->Skipped created no new report (first %q, second %q)", first, second)
	}

	skipped := f.evidence()
	if skipped.Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictSkipped ||
		skipped.Coverage != fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated {
		t.Fatalf("evidence = %+v, want Skipped/NoChecksEvaluated", skipped)
	}

	reports := f.reports()
	if len(reports) != 2 {
		t.Fatalf("stored %d reports, want 2 (Unknown->Pass and Pass->Skipped)", len(reports))
	}
	// The producing adapter's identity comes from the fenced publication
	// provenance, not from a live registry lookup that may by now describe a
	// different revision entirely.
	for _, tc := range []struct{ field, got, want string }{
		{"addonType", reports[1].Spec.AddonType, lifecycleAddon},
		{"adapterName", reports[1].Spec.AdapterName, lifecycleAddon},
		{"adapterVersion", reports[1].Spec.AdapterVersion, skipped.Revision.AdapterVersion},
		{"contractVersion", reports[1].Spec.ContractVersion, adapter.ContractVersion},
	} {
		if tc.got != tc.want {
			t.Errorf("report %s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
	assertReportAttribution(t, &reports[1], skipped)
	if reports[1].Spec.Attribution.Message != fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage {
		t.Errorf("the Skipped report's coverage message = %q, want %q",
			reports[1].Spec.Attribution.Message, fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage)
	}
	// The superseded report keeps its own provenance: history is never
	// rewritten by the run that replaced it.
	assertReportAttribution(t, &reports[0], passEvidence)
}

// Skipped->Skipped. spec.md: "Repeated Skipped results update current" evidence
// only — "Skipped->Skipped only updates status". The second run's observation
// advances (it is completed evidence, not a failed attempt) while history stays
// exactly one entry deep.
func TestSkippedToSkippedUpdatesEvidenceWithoutCreatingAReport(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(skippedRun)

	_, first := f.runAndRecord()
	if first == "" {
		t.Fatal("the first completed all-Skipped evaluation created no report; Unknown->Skipped is a transition")
	}
	firstObservation := f.evidence().ObservedAt

	f.advance(time.Minute)
	_, second := f.runAndRecord()
	if second != "" {
		t.Errorf("Skipped->Skipped created report %q; no-change verdicts do not create reports", second)
	}

	evidence := f.evidence()
	if !evidence.ObservedAt.Time.Equal(f.now()) {
		t.Errorf("observedAt = %s, want the repeat run's own time %s; a completed run always advances its observation",
			evidence.ObservedAt, f.now())
	}
	if evidence.ObservedAt.Time.Equal(firstObservation.Time) {
		t.Error("the repeat all-Skipped run left the observation frozen")
	}
	if reports := f.reports(); len(reports) != 1 {
		t.Fatalf("stored %d reports, want 1", len(reports))
	}
	if got := f.check().Status.LastReportName; got != first {
		t.Errorf("lastReportName = %q, want the unchanged %q: it names the report capturing the current verdict", got, first)
	}
}

// Mixed Skipped. "Mixed results use existing aggregate semantics" and "Skipped
// never turns a Warn/Fail into Pass" — so the transition, the report's verdict
// and the report's per-check entries all follow the pre-existing aggregate, and
// the Skipped entries ride along in history rather than being dropped.
func TestMixedSkippedTransitionsUseTheExistingAggregateAndKeepEverySkippedEntry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome adapter.Outcome
		want    fathomv1alpha1.HealthReportResult
	}{
		{"one pass beside skips", adapter.OutcomePass, fathomv1alpha1.HealthReportResultPass},
		{"one warn beside skips", adapter.OutcomeWarn, fathomv1alpha1.HealthReportResultWarn},
		{"one fail beside skips", adapter.OutcomeFail, fathomv1alpha1.HealthReportResultFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			f.script(skippedRun)
			f.runAndRecord()

			f.advance(time.Minute)
			f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
				return adapter.Result{Checks: []adapter.CheckResult{
					{Family: "health", Outcome: adapter.OutcomeSkipped, Summary: "family disabled"},
					{Family: "health", Outcome: tc.outcome, Summary: "the one check that ran"},
					{Family: "health", Outcome: adapter.OutcomeSkipped, Summary: "no matching objects"},
				}}, nil
			})
			_, name := f.runAndRecord()
			if name == "" {
				t.Fatalf("Skipped->%s created no report", tc.want)
			}

			evidence := f.evidence()
			if evidence.Coverage != fathomv1alpha1.AddonCheckCoverageChecksEvaluated {
				t.Errorf("coverage = %q, want ChecksEvaluated: one evaluated check is coverage", evidence.Coverage)
			}
			reports := f.reports()
			if len(reports) != 2 {
				t.Fatalf("stored %d reports, want 2", len(reports))
			}
			report := &reports[1]
			if report.Spec.Result != tc.want {
				t.Errorf("report result = %q, want %q; Skipped neither raises nor lowers an evaluated verdict",
					report.Spec.Result, tc.want)
			}
			if len(report.Spec.Checks) != 3 {
				t.Fatalf("report carries %d checks, want all 3 including the Skipped ones", len(report.Spec.Checks))
			}
			var skipped int
			for _, entry := range report.Spec.Checks {
				if entry.Result == fathomv1alpha1.HealthReportResultSkipped {
					skipped++
				}
			}
			if skipped != 2 {
				t.Errorf("report carries %d Skipped entries, want 2", skipped)
			}
			assertReportAttribution(t, report, evidence)
		})
	}
}

// Zero enabled checks. data-model.md: "Zero enabled checks follows the engine's
// explicit Skipped sentinel" — it is COMPLETED evidence with NoChecksEvaluated
// coverage and a Ready=True/RunCompleted condition, not a failed attempt, and
// its first appearance is a transition worth a report.
func TestZeroEnabledChecksIsCompletedSkippedEvidenceAndATransition(t *testing.T) {
	check := runtimeCheckObject()
	check.Spec.Policy = map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
		"health": {Enabled: ptr.To(false)},
	}
	f := newRuntimeCheckFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(), check)
	f.ready()
	f.script(skippedRun)

	attempt, name := f.runAndRecord()
	if !attempt.Completed || !attempt.Published {
		t.Fatalf("a run with nothing enabled was not completed evidence: %+v", attempt)
	}
	if name == "" {
		t.Fatal("the first completed evaluation created no report")
	}
	evidence := f.evidence()
	if evidence.Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictSkipped ||
		evidence.Coverage != fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated ||
		evidence.Message != fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage {
		t.Fatalf("evidence = %+v, want Skipped/NoChecksEvaluated/%q",
			evidence, fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage)
	}
	if status := f.check().Status; status.LatestAttemptOutcome != fathomv1alpha1.AddonCheckAttemptCompleted {
		t.Errorf("latestAttemptOutcome = %q, want Completed", status.LatestAttemptOutcome)
	}
	ready := runtimeReadyCondition(t, f.check())
	if ready == nil || ready.Status != metav1.ConditionTrue || ready.Reason != reasonRunCompleted {
		t.Errorf("Ready = %+v, want True/RunCompleted: eligibility completed, which is not a Pass", ready)
	}
	reports := f.reports()
	if len(reports) != 1 {
		t.Fatalf("stored %d reports, want 1", len(reports))
	}
	if reports[0].Spec.Result != fathomv1alpha1.HealthReportResultSkipped {
		t.Errorf("report result = %q, want Skipped", reports[0].Spec.Result)
	}
	assertReportAttribution(t, &reports[0], evidence)
}

// Unchanged verdict, NEW revision. spec.md acceptance 2: "Given a newer valid
// revision with the same verdict, when it completes, then current evidence
// updates but no transition-only historical report is added."
//
// This is the row that separates the two axes: the revision that produced the
// current answer changed, the answer did not, and history must record only the
// second of those — with the earlier report still naming the revision it was
// actually produced under.
func TestAnUnchangedVerdictUnderANewRevisionUpdatesEvidenceButAddsNoReport(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.admit()
	first := f.publish(1)

	_, name := f.runAndRecord()
	if name == "" {
		t.Fatal("the first completed evaluation created no report")
	}
	before := f.evidence()

	// A valid edit: the definition reconciler republishes the snapshot at a new
	// generation, and the check's own spec is untouched.
	definition := f.definition()
	definition.Generation = 2
	f.update(definition)
	second := f.publish(2)
	if second == first {
		t.Fatal("the fixture published the same revision twice; the row needs a real revision change")
	}
	f.advance(time.Minute)

	_, repeat := f.runAndRecord()
	if repeat != "" {
		t.Errorf("an unchanged verdict created report %q; only a verdict change is history", repeat)
	}

	after := f.evidence()
	if after.Verdict != before.Verdict {
		t.Fatalf("verdict changed from %q to %q; this row requires it unchanged", before.Verdict, after.Verdict)
	}
	if after.Revision.DefinitionGeneration != second.Generation {
		t.Errorf("evidence revision generation = %d, want the new revision %d; current evidence tracks the revision that produced it",
			after.Revision.DefinitionGeneration, second.Generation)
	}
	if !after.ObservedAt.Time.Equal(f.now()) {
		t.Errorf("observedAt = %s, want the new run's time %s", after.ObservedAt, f.now())
	}

	reports := f.reports()
	if len(reports) != 1 {
		t.Fatalf("stored %d reports, want 1", len(reports))
	}
	if got := reports[0].Spec.Attribution.Revision.DefinitionGeneration; got != first.Generation {
		t.Errorf("the stored report was re-attributed to generation %d; history is never rewritten by a later run (want %d)",
			got, first.Generation)
	}
	if !reports[0].Spec.ObservedAt.Time.Equal(before.ObservedAt.Time) {
		t.Errorf("the stored report's observedAt moved to %s; a later run may not re-date history (want %s)",
			reports[0].Spec.ObservedAt, before.ObservedAt)
	}
}

// A failed attempt is not a verdict. It preserves evidence, and it adds nothing
// to history — the stored report keeps its original result, attribution and
// observation, because "Attempt Error still preserves previous completed
// evidence" and a report may only be attributed to a completed run.
func TestAFailedAttemptAddsNoReportAndNeverRedatesTheStoredOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runtimeCheckFixture)
	}{
		{"authority revoked mid-run", func(f *runtimeCheckFixture) {
			binding := f.binding()
			binding.Spec.Enabled = false
			f.update(binding)
		}},
		{"the definition is deleted", func(f *runtimeCheckFixture) {
			if err := f.store.Delete(context.Background(), f.definition()); err != nil {
				f.t.Fatalf("delete definition: %v", err)
			}
		}},
		{"the run fails its budget", func(f *runtimeCheckFixture) {
			f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
				return adapter.Result{}, errors.New("Timeout: the run deadline elapsed")
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			_, name := f.runAndRecord()
			if name == "" {
				t.Fatal("the seed evaluation created no report")
			}
			before := f.evidence()
			stored := f.reports()

			f.advance(time.Minute)
			tc.mutate(f)
			attempt, repeat := f.runAndRecord()
			if attempt.Published {
				t.Fatalf("the failed attempt published evidence: %+v", attempt)
			}
			if repeat != "" {
				t.Errorf("a failed attempt created report %q", repeat)
			}

			after := f.evidence()
			if after == nil || !equality.Semantic.DeepEqual(before, after) {
				t.Fatalf("the failed attempt changed evidence:\nbefore %+v\nafter  %+v", before, after)
			}
			now := f.reports()
			if len(now) != len(stored) {
				t.Fatalf("history grew from %d to %d reports on a failed attempt", len(stored), len(now))
			}
			if !equality.Semantic.DeepEqual(stored[0].Spec, now[0].Spec) {
				t.Errorf("the stored report was rewritten:\nbefore %+v\nafter  %+v", stored[0].Spec, now[0].Spec)
			}
		})
	}
}

// The attribution block is only worth what the API server stores and protects.
// This spec installs the generated config/crd/bases and proves two things no
// fake client can: the revision/authority/coverage context survives a round
// trip, and the pre-existing spec-immutability rule now covers it — so the
// "historical provenance is preserved when superseded" requirement is enforced
// by the API server rather than by the operator's good manners.
var _ = Describe("runtime HealthReport attribution", func() {
	newReport := func(name string) *fathomv1alpha1.HealthReport {
		return &fathomv1alpha1.HealthReport{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
			Spec: fathomv1alpha1.HealthReportSpec{
				SourceRef: fathomv1alpha1.HealthReportTargetRef{
					APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: "AddonCheck",
					Namespace: "default", Name: runtimeCheckName,
				},
				AddonType:  lifecycleAddon,
				Result:     fathomv1alpha1.HealthReportResultSkipped,
				ObservedAt: metav1.NewTime(runtimeCheckClockStart),
				Attribution: &fathomv1alpha1.HealthReportAttribution{
					Revision: fathomv1alpha1.AddonCheckEvidenceRevision{
						DefinitionUID: string(lifecycleDefUID), DefinitionGeneration: 7,
						SchemaVersion: "v1alpha1", SemanticsVersion: 1,
						AdapterVersion: "1.0.0", OperatorBuild: lifecycleBuild,
					},
					Authority: fathomv1alpha1.AddonCheckEvidenceAuthority{
						BindingUID: "binding-uid-1", BindingGeneration: 2,
						ServiceAccountUID: "sa-uid-1", CheckUID: string(runtimeCheckUID), CheckGeneration: 3,
						PolicyDigest: strings.Repeat("d", 64),
						LeaderEpoch: &fathomv1alpha1.DefinitionLeaderEpoch{
							LeaseUID: "lease-uid-1", HolderIdentity: lifecycleLeader,
							AcquireTime: metav1.NewMicroTime(runtimeCheckClockStart), LeaseTransitions: 3,
						},
					},
					Coverage: fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated,
					Message:  fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage,
				},
			},
		}
	}

	It("stores the producing revision, authority and coverage", func(ctx SpecContext) {
		report := newReport("hr-attribution-roundtrip")
		Expect(k8sClient.Create(ctx, report)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, report))).To(Succeed())
		})

		var stored fathomv1alpha1.HealthReport
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(report), &stored)).To(Succeed())
		Expect(stored.Spec.Attribution).NotTo(BeNil())
		Expect(stored.Spec.Attribution.Revision.DefinitionUID).To(Equal(string(lifecycleDefUID)))
		Expect(stored.Spec.Attribution.Revision.DefinitionGeneration).To(Equal(int64(7)))
		Expect(stored.Spec.Attribution.Authority.LeaderEpoch).NotTo(BeNil())
		Expect(stored.Spec.Attribution.Authority.LeaderEpoch.HolderIdentity).To(Equal(lifecycleLeader))
		Expect(stored.Spec.Attribution.Coverage).
			To(Equal(fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated))
		Expect(stored.Spec.Attribution.Message).
			To(Equal(fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage))
	})

	It("refuses to let a later run re-attribute stored history", func(ctx SpecContext) {
		report := newReport("hr-attribution-immutable")
		Expect(k8sClient.Create(ctx, report)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, report))).To(Succeed())
		})

		var stored fathomv1alpha1.HealthReport
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(report), &stored)).To(Succeed())
		stored.Spec.Attribution.Revision.DefinitionGeneration = 8
		// Named explicitly so the rejection cannot be satisfied by an unrelated
		// failure (a conflict, say) and quietly stop proving immutability.
		Expect(k8sClient.Update(ctx, &stored)).To(MatchError(ContainSubstring("spec is immutable")))

		stored = fathomv1alpha1.HealthReport{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(report), &stored)).To(Succeed())
		Expect(stored.Spec.Attribution.Revision.DefinitionGeneration).To(Equal(int64(7)))
	})

	It("rejects an over-long coverage message", func(ctx SpecContext) {
		report := newReport("hr-attribution-long-message")
		report.Spec.Attribution.Message = strings.Repeat("m", addonCheckStatusTextLimit+1)
		Expect(k8sClient.Create(ctx, report)).
			To(MatchError(ContainSubstring("spec.attribution.message")))
	})
})

// data-model.md: "Historical reports are retained under existing retention
// policy, never rewritten by this run." Runtime transitions are ordinary
// history, so spec.historyLimit bounds them exactly as it bounds a built-in
// adapter's — a definition that flaps between verdicts cannot grow a check's
// history without limit.
func TestRuntimeTransitionsObeyTheExistingRetentionLimit(t *testing.T) {
	check := runtimeCheckObject()
	check.Spec.HistoryLimit = ptr.To(int32(1))
	f := newRuntimeCheckFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(), check)
	f.ready()

	// Pass, then Skipped, then Pass again: three transitions, one allowed
	// report.
	f.runAndRecord()
	for _, run := range []func(context.Context, adapter.Request) (adapter.Result, error){skippedRun, nil} {
		f.advance(time.Minute)
		f.script(run)
		if _, name := f.runAndRecord(); name == "" {
			t.Fatal("a verdict change created no report")
		}
	}

	// Which report survives is decided by the existing oldest-first prune on
	// metadata.creationTimestamp, and under this fixture's fake API every
	// report is created within the same second — so this asserts the bound the
	// retention policy actually promises, not an ordering the fake clock cannot
	// express.
	if reports := f.reports(); len(reports) != 1 {
		t.Fatalf("stored %d reports, want the configured historyLimit of 1", len(reports))
	}
}

// Only the run that actually PUBLISHED evidence may report it.
//
// The dangerous shape is not a failed attempt beside its own preserved
// evidence — that verdict has not moved, so nothing would be reported anyway.
// It is a run that failed (or whose publication was refused) while ANOTHER run
// published a different verdict: the verdict has genuinely changed, so a path
// that only compared verdicts would write a history entry carrying this run's
// empty fence as the attribution for someone else's observation. Publication
// precedence exists to stop exactly that kind of borrowed credit.
func TestOnlyTheRunThatPublishedEvidenceMayReportIt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		attempt RuntimeAttempt
	}{
		{"the run failed", RuntimeAttempt{
			Reason: reasonAccessDenied, Message: "the dedicated reader was denied a required read",
		}},
		{"the run completed but its publication was refused", RuntimeAttempt{
			Completed: true, Reason: reasonPublicationInFlight,
			Message: "another runtime run is already publishing this AddonCheck",
		}},
		// Unreachable through the runner today — Published is only ever set
		// beside Completed — but both halves are checked, because an attempt
		// that claims a publication it never completed is exactly the kind of
		// borrowed credit this guard exists to refuse.
		{"the attempt claims a publication it never completed", RuntimeAttempt{
			Published: true, Reason: reasonRunCompleted,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			if _, name := f.runAndRecord(); name == "" {
				t.Fatal("the seed evaluation created no report")
			}
			previous := f.evidence()

			// A concurrent publisher moves current evidence to Skipped. Its own
			// transition is deliberately not recorded here: this row is about
			// who may claim it.
			f.advance(time.Minute)
			f.script(skippedRun)
			if attempt := f.runOK(); !attempt.Published {
				t.Fatalf("the concurrent run did not publish: %+v", attempt)
			}
			published := f.check()
			if published.Status.LastSuccessfulEvaluation.Verdict == previous.Verdict {
				t.Fatal("the concurrent run left the verdict unchanged; the row needs a real change")
			}

			name, err := (&AddonCheckReconciler{Client: f.cached, Scheme: f.scheme}).
				recordRuntimeTransition(context.Background(), logr.Discard(), published, previous, tc.attempt)
			if err != nil {
				t.Fatalf("record runtime transition: %v", err)
			}
			if name != "" {
				t.Errorf("a run that published nothing created report %q", name)
			}
			if reports := f.reports(); len(reports) != 1 {
				t.Fatalf("stored %d reports, want the seed one only", len(reports))
			}
		})
	}
}

// A verdict that flaps records EVERY transition. The report name is derived
// rather than generated, so that a retried create of the same publication
// reuses one object instead of duplicating it — which means the derivation has
// to include the observation itself. Keyed on the verdict pair alone, the
// second Pass->Skipped below would collide with the first and silently reuse a
// report dated an hour earlier, presenting an old observation as the current
// one: the same class of bug as renewing evidence on a failed attempt.
func TestARepeatedTransitionRecordsItsOwnReport(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	var names []string
	for _, run := range []func(context.Context, adapter.Request) (adapter.Result, error){
		nil, skippedRun, nil, skippedRun,
	} {
		f.advance(time.Minute)
		f.script(run)
		_, name := f.runAndRecord()
		if name == "" {
			t.Fatalf("a verdict change after %d transitions created no report", len(names))
		}
		names = append(names, name)
	}

	unique := map[string]int{}
	for _, name := range names {
		unique[name]++
	}
	if len(unique) != len(names) {
		t.Fatalf("the four transitions produced %d distinct reports: %v", len(unique), names)
	}
	reports := f.reports()
	if len(reports) != 4 {
		t.Fatalf("stored %d reports, want one per transition", len(reports))
	}
	// Each entry is dated by its own observation, in order.
	for i := 1; i < len(reports); i++ {
		if !reports[i].Spec.ObservedAt.After(reports[i-1].Spec.ObservedAt.Time) {
			t.Errorf("report %d is dated %s, not after its predecessor %s",
				i, reports[i].Spec.ObservedAt, reports[i-1].Spec.ObservedAt)
		}
	}
}

// A runtime report is an ordinary report: the reserved warnRatio/failRatio
// rollup (#159) produces its synthetic per-family entry here exactly as it does
// for a built-in adapter, so the ratio verdict stays explainable from the
// persisted report alone. "Ratio aggregation cannot average a failed family
// into Pass" holds in history as well as in status.
func TestRuntimeTransitionReportsCarryTheRatioRollupEntry(t *testing.T) {
	check := runtimeCheckObject()
	check.Spec.Policy = map[string]fathomv1alpha1.AddonCheckFamilyPolicy{
		"health": {Enabled: ptr.To(true), Thresholds: map[string]fathomv1alpha1.ThresholdValue{
			adapter.ThresholdKeyFailRatio: "10",
		}},
	}
	f := newRuntimeCheckFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(), check)
	f.ready()
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		var checks []adapter.CheckResult
		for i := 0; i < 20; i++ {
			outcome := adapter.OutcomePass
			if i < 12 {
				outcome = adapter.OutcomeFail
			}
			checks = append(checks, adapter.CheckResult{Family: "health", Outcome: outcome, Summary: "member"})
		}
		return adapter.Result{Checks: checks}, nil
	})

	if _, name := f.runAndRecord(); name == "" {
		t.Fatal("the first completed evaluation created no report")
	}
	reports := f.reports()
	if len(reports) != 1 {
		t.Fatalf("stored %d reports, want 1", len(reports))
	}
	report := &reports[0]
	if report.Spec.Result != fathomv1alpha1.HealthReportResultFail {
		t.Errorf("report result = %q, want Fail: 12 of 20 unhealthy is past a 10%% failRatio", report.Spec.Result)
	}

	var rollup *fathomv1alpha1.HealthReportCheck
	for i := range report.Spec.Checks {
		if report.Spec.Checks[i].Details[detailRollup] == detailRollupRatio {
			rollup = &report.Spec.Checks[i]
		}
	}
	if rollup == nil {
		t.Fatalf("the report carries no ratio rollup entry among its %d checks", len(report.Spec.Checks))
	}
	if rollup.Result != fathomv1alpha1.HealthReportResultFail {
		t.Errorf("rollup entry result = %q, want Fail", rollup.Result)
	}
	if rollup.Details[detailRollupPopulation] != "20" || rollup.Details[detailRollupUnhealthy] != "12" {
		t.Errorf("rollup entry counts = %v, want population 20 / unhealthy 12", rollup.Details)
	}
	if !rollup.ObservedAt.Time.Equal(f.evidence().ObservedAt.Time) {
		t.Errorf("rollup entry observedAt = %s, want the evidence's observation %s",
			rollup.ObservedAt, f.evidence().ObservedAt)
	}
}

// --- Review follow-up: the properties the first pass asserted only indirectly.
//
// Each test below exists because the production code was mutated in a way the
// rest of this file did not notice. They are grouped here rather than merged
// into the rows above because each one pins a DIFFERENT axis than its
// neighbours, and merging them back would recreate the single-axis rows that
// let the mutation through.

// The freshness ORDER: eligibility is asked FIRST and age SECOND.
//
// Evidence that is both aged and produced from ineligible inputs is the STEADY
// STATE of a prolonged revocation — evidence stops being refreshed precisely
// while the binding is revoked — so it is not a corner case, it is what the
// operator sees for as long as the revocation lasts. The lifecycle matrix names
// freshness=Unavailable for every one of those rows ("Prior evidence retained
// with original time/revision/context, freshness=Unavailable"), with no age
// qualifier, so Stale is wrong no matter how old the observation is: Stale says
// "this would be refreshed by the next run", and these inputs cannot produce a
// next run at all.
//
// Every other freshness row in this file varies ONE axis — age with eligible
// inputs, or ineligibility at age zero — so an implementation that asked age
// first satisfied all of them.
func TestIneligibleInputsOutrankEvidenceAgeing(t *testing.T) {
	// Spelled from the contract, not from AddonCheckEvidenceWindow: this test
	// must age the evidence even if that formula is wrong.
	const window = 2*fathomv1alpha1.DefaultAddonCheckInterval + runtimeCheckTimeout

	// The control row. It shares the age of every row below and differs only in
	// that its inputs stay eligible, which is what makes the rows below a
	// decision about ORDER rather than a decision about age.
	t.Run("aged evidence from still-eligible inputs is Stale", func(t *testing.T) {
		f := newRuntimeCheckFixture(t)
		f.ready()
		f.seedEvidence()

		f.advance(window + time.Minute)
		f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
			return adapter.Result{}, errors.New("ExecutionFailed: the addon's API server is unreachable")
		})
		f.runOK()

		if got := f.check().Status.EvidenceFreshness; got != fathomv1alpha1.AddonCheckEvidenceStale {
			t.Fatalf("evidenceFreshness = %q at age %s with eligible inputs, want Stale", got, window+time.Minute)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(*runtimeCheckFixture)
		reason string
	}{
		// "Missing definition | Prior evidence retained ...; freshness=Unavailable".
		{"missing definition", func(f *runtimeCheckFixture) {
			f.registry.RemoveRuntime(lifecycleDefUID)
		}, reasonUnknownAddonType},
		// "Invalid edit stored | Remove eligibility; Accepted=False/InvalidDefinition".
		{"invalid edit stored", func(f *runtimeCheckFixture) {
			d := f.definition()
			d.Spec.AdapterVersion = "not-a-semver"
			d.Generation = 2
			f.update(d)
		}, reasonInvalidDefinition},
		// "Binding disabled/deleted | Revoke authority".
		{"binding disabled", func(f *runtimeCheckFixture) {
			b := f.binding()
			b.Spec.Enabled = false
			f.update(b)
		}, reasonAuthorizationRevoked},
		{"service account replaced", func(f *runtimeCheckFixture) {
			b := f.binding()
			b.Spec.ServiceAccountRef.UID = "a-different-service-account-uid"
			f.update(b)
		}, reasonBindingMismatch},
	} {
		t.Run("aged evidence from ineligible inputs is Unavailable: "+tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			before := f.seedEvidence()

			// Age it PAST the window first, so the age test would fire if it
			// were asked first, and only then withdraw eligibility.
			f.advance(window + time.Minute)
			if !AddonCheckEvidenceAged(f.check(), f.now()) {
				t.Fatalf("the row did not age the evidence past the %s window; it proves nothing", window)
			}
			tc.mutate(f)

			attempt := f.runOK()
			if attempt.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, tc.reason, attempt.Message)
			}
			status := f.check().Status
			if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceUnavailable {
				t.Fatalf("evidenceFreshness = %q for aged evidence whose inputs are no longer eligible, want Unavailable; "+
					"Stale would promise a refresh these inputs can no longer deliver", status.EvidenceFreshness)
			}
			// The explanation must be the ELIGIBILITY one, not the age one. The
			// right enum with the wrong reason is still a wrong answer: the age
			// wording tells an operator to wait for the next run.
			if !strings.Contains(status.EvidenceFreshnessReason, "no longer eligible") {
				t.Errorf("evidenceFreshnessReason = %q, want the eligibility wording", status.EvidenceFreshnessReason)
			}
			if !strings.Contains(status.EvidenceFreshnessReason, before.ObservedAt.UTC().Format(time.RFC3339)) {
				t.Errorf("evidenceFreshnessReason = %q, want it to name the ORIGINAL observation %s",
					status.EvidenceFreshnessReason, before.ObservedAt.UTC().Format(time.RFC3339))
			}
			if !equality.Semantic.DeepEqual(before, status.LastSuccessfulEvaluation) {
				t.Fatalf("the ineligible attempt rewrote retained evidence:\nbefore %+v\nafter  %+v",
					before, status.LastSuccessfulEvaluation)
			}
		})
	}
}

// The all-Skipped coverage message is fixed by the spec clarification as a
// LITERAL: "explicit 'no checks evaluated' coverage". Every other assertion in
// this file compares against the constant, which means the assertion moves with
// the value and a reworded constant stays green — so the literal is spelled out
// once, here, and tied to the end-to-end run that produces it.
func TestTheNoChecksEvaluatedMessageIsTheContractLiteral(t *testing.T) {
	const literal = "no checks evaluated"
	if fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage != literal {
		t.Fatalf("AddonCheckNoChecksEvaluatedMessage = %q, want the contract literal %q",
			fathomv1alpha1.AddonCheckNoChecksEvaluatedMessage, literal)
	}

	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(skippedRun)

	if attempt := f.runOK(); !attempt.Published {
		t.Fatalf("a completed all-Skipped run published nothing: %+v", attempt)
	}
	evidence := f.evidence()
	if evidence == nil {
		t.Fatal("a completed all-Skipped run stored no evidence")
	}
	if evidence.Message != literal {
		t.Errorf("evidence message = %q, want the contract literal %q", evidence.Message, literal)
	}
	if evidence.Coverage != fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated {
		t.Errorf("coverage = %q, want NoChecksEvaluated", evidence.Coverage)
	}
	if evidence.Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictSkipped {
		t.Errorf("verdict = %q, want Skipped", evidence.Verdict)
	}
}

// Publication precedence is an ORDER, and an order needs a defined tie-break.
// Two candidates of the same rank are routine — losing the Lease while the
// binding's service account is also replaced produces exactly that — and the
// reported reason must not depend on which re-read happened to finish last.
// The first candidate offered wins, because candidates are gathered in the
// order the contract states the facts: the session's own view of authority
// before any object's.
func TestATieInThePrecedenceOrderKeepsTheFirstCandidateOffered(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.runner.barrier = func(_ context.Context, phase string) {
		if phase != runtimeBarrierAfterExecute {
			return
		}
		// Two independent rankAuthority facts, gathered in this order: the
		// epoch first, the binding's service account second.
		f.session.mu.Lock()
		f.session.epochOK = false
		f.session.mu.Unlock()
		b := f.binding()
		b.Spec.ServiceAccountRef.UID = "a-different-service-account-uid"
		f.update(b)
	}

	attempt := f.runOK()
	if attempt.Published {
		t.Fatalf("a run with two authority failures published: %+v", attempt)
	}
	if attempt.Reason != reasonAuthorizationRevoked {
		t.Fatalf("reason = %q, want %q: a tie keeps the FIRST candidate, and the lost epoch is offered before the "+
			"replaced service account (%s)", attempt.Reason, reasonAuthorizationRevoked, attempt.Message)
	}
}

// publish takes two inputs that are easy to conflate: the OBJECT the final
// fence read, which is the compare-and-swap base, and the FENCE, which is the
// context the run was actually attributed to. Ready's observedGeneration comes
// from the fence.
//
// rereadFence keeps the two generations equal on the live path — it refuses to
// hand back an object whose generation moved — so publish is driven directly
// here. That is the only way to observe the distinction at all, and the
// distinction is the contract: a publication describes the revision it
// evaluated, never whatever the object happens to hold when the write lands.
func TestPublicationAttributesTheRunToTheFencedGeneration(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	revision := f.ready()

	fenced := f.check()
	fenced.Generation = 9
	fence := RuntimeFence{
		Revision:        revision,
		CheckUID:        fenced.UID,
		CheckGeneration: 3,
		Epoch:           f.session.Epoch(),
	}

	attempt, err := f.runner.publish(context.Background(), fenced, fence,
		"runtime evaluation completed with eligible inputs", adapter.Result{})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !attempt.Published {
		t.Fatalf("publish wrote nothing: %+v", attempt)
	}

	stored := f.check()
	ready := runtimeReadyCondition(t, stored)
	if ready == nil {
		t.Fatal("publication recorded no Ready condition")
	}
	if ready.ObservedGeneration != fence.CheckGeneration {
		t.Errorf("Ready.observedGeneration = %d, want the FENCED generation %d, not the object's %d",
			ready.ObservedGeneration, fence.CheckGeneration, fenced.Generation)
	}
	if evidence := stored.Status.LastSuccessfulEvaluation; evidence == nil ||
		evidence.Authority.CheckGeneration != fence.CheckGeneration {
		t.Errorf("evidence authority generation = %+v, want the fenced generation %d",
			evidence, fence.CheckGeneration)
	}
}

// The pre-run fence re-reads the AddonCheck uncached and requires it to still
// name the addon type the run resolved. The object the caller hands in comes
// from the informer cache, so it can name an identity the live object no longer
// does — and running the OLD addon's adapter against the NEW addon's check
// would publish evidence for a check that never asked for it.
//
// The guard is pre-run, so nothing executes and nothing is written.
func TestAPreRunAddonTypeChangeDiscardsTheRunBeforeItExecutes(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	// The cached view the caller reconciles from.
	stale := f.check()

	live := f.check()
	live.Spec.AddonType = "a-different-addon"
	f.update(live)

	attempt, err := f.runner.Run(context.Background(), stale)
	if err != nil {
		t.Fatalf("runtime check run: %v", err)
	}
	if attempt.Published || attempt.Completed {
		t.Fatalf("a run whose check no longer names %q published: %+v", stale.Spec.AddonType, attempt)
	}
	if attempt.Reason != reasonSuperseded {
		t.Fatalf("reason = %q, want %q (%s)", attempt.Reason, reasonSuperseded, attempt.Message)
	}
	if !strings.Contains(attempt.Message, "a-different-addon") {
		t.Errorf("message = %q, want it to name the addon type the live check now carries", attempt.Message)
	}
	if _, _, _, runs := f.counters(); runs != 0 {
		t.Errorf("the evaluator ran %d times for a check that no longer names this addon type", runs)
	}
	if evidence := f.check().Status.LastSuccessfulEvaluation; evidence != nil {
		t.Fatalf("a discarded pre-run fence stored evidence: %+v", evidence)
	}
}

// The final fence's re-reads are charged to the SAME shared counters the run
// already spent, so a budget that has already recorded a failure has nothing
// left to spend on them. Issuing them anyway would mean a run that failed on
// its read budget goes on to make more reads, which is the one thing a budget
// exists to prevent — and the matrix already answers this case without them:
// "Deadline, size, parser or read budget exhausted | Attempt Error with
// specific reason".
func TestAnAlreadyFailedBudgetSpendsNoReadsOnTheFinalFence(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.script(func(context.Context, adapter.Request) (adapter.Result, error) {
		return adapter.Result{}, f.budget().Fail("WorkLimitExceeded", "object limit 1000 exceeded")
	})

	var beforeFinalFence int
	f.runner.barrier = func(_ context.Context, phase string) {
		if phase == runtimeBarrierAfterExecute {
			_, beforeFinalFence, _, _ = f.counters()
		}
	}

	attempt := f.runOK()
	if attempt.Published {
		t.Fatalf("a run on a failed budget published: %+v", attempt)
	}
	if attempt.Reason != "WorkLimitExceeded" {
		t.Fatalf("reason = %q, want WorkLimitExceeded (%s)", attempt.Reason, attempt.Message)
	}
	if beforeFinalFence == 0 {
		t.Fatal("the barrier never observed the pre-run fence's reads; the counter proves nothing")
	}
	if _, total, _, _ := f.counters(); total != beforeFinalFence {
		t.Errorf("the final fence spent %d more control-plane reads on a budget that had already failed",
			total-beforeFinalFence)
	}
}

// evidenceFreshnessReason is DERIVED, so it is rewritten on every pass —
// including the pass that has nothing to say. Current evidence carries no
// explanation, and leaving the previous one behind would show an operator
// "the inputs this evidence was produced from are no longer eligible" next to a
// freshness of Current: two contradictory statements, of which the stale one is
// the more believable.
func TestARecoveredRunClearsTheStaleFreshnessExplanation(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()
	f.seedEvidence()

	f.advance(time.Minute)
	revoked := f.binding()
	revoked.Spec.Enabled = false
	f.update(revoked)
	f.runOK()

	status := f.check().Status
	if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceUnavailable {
		t.Fatalf("evidenceFreshness = %q after revocation, want Unavailable", status.EvidenceFreshness)
	}
	if status.EvidenceFreshnessReason == "" {
		t.Fatal("the revoked attempt recorded no freshness explanation; there is nothing stale to clear")
	}

	restored := f.binding()
	restored.Spec.Enabled = true
	f.update(restored)
	f.advance(time.Minute)
	if attempt := f.runOK(); !attempt.Published {
		t.Fatalf("the recovered run published nothing: %+v", attempt)
	}

	status = f.check().Status
	if status.EvidenceFreshness != fathomv1alpha1.AddonCheckEvidenceCurrent {
		t.Fatalf("evidenceFreshness = %q after a fresh completion, want Current", status.EvidenceFreshness)
	}
	if status.EvidenceFreshnessReason != "" {
		t.Errorf("evidenceFreshnessReason = %q alongside Current freshness; a derived field is cleared, not left behind",
			status.EvidenceFreshnessReason)
	}
}

// contracts/runtime.md: "Control-plane metadata/final fences use uncached
// APIReader and never enter the evaluator."
//
// TestRuntimeFencesNeverReadThroughTheManagerCache proves that property for the
// reader this fixture supplies. This test proves it for every reader T047 could
// ever supply: the seam is a CONCRETE type only impersonation's uncached
// constructor produces, so handing the fences the manager's cached client does
// not compile. An interface-typed seam would be satisfied by that client, and
// the behavioural test would keep passing while production read the cache.
// Typing the seam stops the WRONG reader reaching the fences; it cannot stop a
// factory returning NO reader. T047 fills this seam, and a factory that silently
// yields a zero RuntimeControlReader must discard the run rather than reach a nil
// dereference inside the pre-run fence — the same fail-loudly-on-mis-wiring rule
// the registry's dispatch gate and the leadership session's cache-sync gate hold.
func TestAMisWiredClientFactoryDiscardsTheRunInsteadOfDereferencingNil(t *testing.T) {
	// Each row drops one half of what the real seam returns, so the surviving
	// half is genuinely well-formed and only the missing one is under test.
	for _, tc := range []struct {
		name string
		drop func(RuntimeRunClients) RuntimeRunClients
	}{
		{"no control reader", func(c RuntimeRunClients) RuntimeRunClients {
			c.Control = impersonation.RuntimeControlReader{}
			return c
		}},
		{"no evaluator factory", func(c RuntimeRunClients) RuntimeRunClients {
			c.Evaluator = nil
			return c
		}},
		{"neither", func(RuntimeRunClients) RuntimeRunClients { return RuntimeRunClients{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeCheckFixture(t)
			f.ready()
			real := f.runner.Clients
			f.runner.Clients = func(budget *execution.Budget, targets execution.ControlTargets) (RuntimeRunClients, error) {
				clients, err := real(budget, targets)
				if err != nil {
					return clients, err
				}
				return tc.drop(clients), nil
			}

			attempt := f.runOK()

			if attempt.Published || attempt.Completed {
				t.Fatalf("a mis-wired client factory produced %+v; it must discard the run", attempt)
			}
			if attempt.Reason != reasonAuthorizationUnavailable {
				t.Fatalf("reason %q, want %q: a mis-wiring is an authority failure, not an execution failure",
					attempt.Reason, reasonAuthorizationUnavailable)
			}
			if f.evaluatorRuns != 0 {
				t.Fatalf("a mis-wired factory still ran the evaluator %d times", f.evaluatorRuns)
			}
		})
	}
}

func TestTheControlFenceSeamIsTypedSoACachedClientCannotSatisfyIt(t *testing.T) {
	field, ok := reflect.TypeOf(RuntimeRunClients{}).FieldByName("Control")
	if !ok {
		t.Fatal("RuntimeRunClients has no Control field")
	}
	want := reflect.TypeOf(impersonation.RuntimeControlReader{})
	if field.Type != want {
		t.Fatalf("RuntimeRunClients.Control is %s, want %s; any interface here is satisfied by the manager's "+
			"cached client, so nothing refuses the informer cache at compile time", field.Type, want)
	}
	if field.Type.Kind() == reflect.Interface {
		t.Fatalf("RuntimeRunClients.Control is an interface (%s); the fences must name a concrete uncached reader", field.Type)
	}
}

// ---------------------------------------------------------------------------
// T045 — the lost-transition hole: history is decided against what history
// actually holds, not against the pre-run evidence alone
// ---------------------------------------------------------------------------
//
// contracts/runtime.md requires a report on a verdict change, and "No-change
// verdicts do not create reports" for everything else. Those two clauses are
// only compatible while status and history agree about what the last completed
// run concluded. The runtime path publishes evidence FIRST and creates the
// report SECOND, so exactly one lost write breaks that agreement permanently:
// the next run's `previous` is the already-published evidence, the verdicts
// match, and the transition is never recorded.
//
// The rows below fail that create the way production loses it, and require the
// next run to repair history — without turning the repair into a licence to
// report on every poll.

// failingReportCreateClient fails the first `remaining` HealthReport creates,
// then behaves normally. One failed create stands in for every way the second
// half of a transition is lost: a transient API error, a lease that ended
// between the two writes, a process killed after publication.
type failingReportCreateClient struct {
	client.Client
	remaining *int
}

func (c failingReportCreateClient) Create(
	ctx context.Context, obj client.Object, opts ...client.CreateOption,
) error {
	if _, ok := obj.(*fathomv1alpha1.HealthReport); ok && *c.remaining > 0 {
		*c.remaining--
		return errors.New("etcdserver: request timed out")
	}
	return c.Client.Create(ctx, obj, opts...)
}

// runAndRecordWith is runAndRecord through a caller-supplied client, so a row
// can fail the report create. It returns the transition path's error instead of
// fataling on it: losing that create is the whole point of these rows.
func (f *runtimeCheckFixture) runAndRecordWith(cl client.Client) (RuntimeAttempt, string, error) {
	f.t.Helper()
	previous := f.evidence()
	attempt := f.runOK()

	published := f.check()
	name, err := (&AddonCheckReconciler{Client: cl, Scheme: f.scheme}).
		recordRuntimeTransition(context.Background(), logr.Discard(), published, previous, attempt)
	if err != nil {
		return attempt, "", err
	}
	if name != "" {
		if err := f.store.Status().Update(context.Background(), published); err != nil {
			f.t.Fatalf("persist lastReportName: %v", err)
		}
	}
	return attempt, name, nil
}

// The first transition of a check's life is the one with nothing to fall back
// on: evidence is published, the report create fails, and every later run sees
// its own verdict staring back at it.
func TestALostReportCreateIsBackfilledOnTheNextRun(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	remaining := 1
	lossy := failingReportCreateClient{Client: f.cached, remaining: &remaining}

	_, name, err := f.runAndRecordWith(lossy)
	if err == nil {
		t.Fatal("the fixture did not fail the report create; this row needs the loss it models")
	}
	if name != "" {
		t.Fatalf("a failed create still named report %q", name)
	}
	// The dangerous part is that the PUBLICATION succeeded: status already
	// shows the verdict history is missing.
	seeded := f.evidence()
	if seeded == nil || seeded.Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictPass {
		t.Fatalf("evidence = %+v, want a published Pass", seeded)
	}
	if got := f.check().Status.LastReportName; got != "" {
		t.Fatalf("lastReportName = %q after a failed create, want empty", got)
	}
	if reports := f.reports(); len(reports) != 0 {
		t.Fatalf("stored %d reports after a failed create, want 0", len(reports))
	}

	// The next run observes the SAME verdict. Measured against evidence alone
	// this is a no-change, and Unknown->Pass would never reach history.
	f.advance(time.Minute)
	_, repeat := f.runAndRecord()
	if repeat == "" {
		t.Fatal("the lost transition was never backfilled: a verdict change that did happen holds no report, " +
			"and no later run can ever notice")
	}
	reports := f.reports()
	if len(reports) != 1 {
		t.Fatalf("stored %d reports, want the backfilled 1", len(reports))
	}
	if reports[0].Spec.Result != fathomv1alpha1.HealthReportResultPass {
		t.Errorf("backfilled report result = %q, want Pass", reports[0].Spec.Result)
	}
	if got := f.check().Status.LastReportName; got != repeat {
		t.Errorf("lastReportName = %q, want the backfilled report %q", got, repeat)
	}
	// The repair is attributed to the run that actually produced it, not to the
	// run whose create was lost: history stays attributable.
	assertReportAttribution(t, &reports[0], f.evidence())

	// And the repair is not a licence to report every poll. Once history holds
	// the verdict, a genuine no-change writes nothing again.
	f.advance(time.Minute)
	if _, third := f.runAndRecord(); third != "" {
		t.Errorf("a no-change run after the backfill created report %q; no-change verdicts do not create reports", third)
	}
	if got := len(f.reports()); got != 1 {
		t.Errorf("history grew to %d reports past the single backfilled one", got)
	}
}

// One transition later the same loss leaves a WORSE shape: lastReportName names
// a report whose result flatly contradicts the published verdict. Status says
// Skipped, the report it points at says Pass, and T046's HealthCheck mirror
// republishes both.
func TestAReportPointerThatContradictsTheVerdictIsRepaired(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	_, seed := f.runAndRecord()
	if seed == "" {
		t.Fatal("the seed evaluation created no report")
	}

	remaining := 1
	lossy := failingReportCreateClient{Client: f.cached, remaining: &remaining}
	f.advance(time.Minute)
	f.script(skippedRun)
	if _, _, err := f.runAndRecordWith(lossy); err == nil {
		t.Fatal("the fixture did not fail the report create")
	}
	if f.evidence().Verdict != fathomv1alpha1.AddonCheckEvidenceVerdictSkipped {
		t.Fatal("the Pass->Skipped run published nothing; the row needs the contradiction")
	}
	if got := f.check().Status.LastReportName; got != seed {
		t.Fatalf("lastReportName = %q, want the now-stale %q", got, seed)
	}

	// A no-change against evidence; a repair against history.
	f.advance(time.Minute)
	_, repaired := f.runAndRecord()
	if repaired == "" {
		t.Fatal("lastReportName was left naming a Pass beside a published Skipped")
	}
	reports := f.reports()
	if len(reports) != 2 {
		t.Fatalf("stored %d reports, want the Pass and the repaired Skipped", len(reports))
	}
	current := f.check()
	if current.Status.LastReportName != repaired {
		t.Errorf("lastReportName = %q, want %q", current.Status.LastReportName, repaired)
	}
	var named fathomv1alpha1.HealthReport
	key := client.ObjectKey{Namespace: runtimeCheckNamespace, Name: repaired}
	if err := f.store.Get(context.Background(), key, &named); err != nil {
		t.Fatalf("get the repaired report: %v", err)
	}
	if string(named.Spec.Result) != current.Status.LastResult {
		t.Errorf("lastReportName names a %q report beside status.lastResult %q; the two may not contradict",
			named.Spec.Result, current.Status.LastResult)
	}
	// Repaired by ADDING. The superseded entry keeps its own result and its own
	// observation: "Historical reports are ... never rewritten by this run."
	if reports[0].Spec.Result != fathomv1alpha1.HealthReportResultPass {
		t.Errorf("the seed report became %q; history is repaired by adding, never by rewriting", reports[0].Spec.Result)
	}
	if reports[0].Name != seed {
		t.Errorf("the seed report is now %q, want %q", reports[0].Name, seed)
	}
}

// The repair must not fight retention. A pointer whose report was PRUNED is
// bounded history working as configured, not a lost write — resurrecting that
// entry would re-create and re-prune one report per poll forever.
func TestARetentionPrunedPointerIsNotResurrected(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	_, seed := f.runAndRecord()
	if seed == "" {
		t.Fatal("the seed evaluation created no report")
	}
	var pruned fathomv1alpha1.HealthReport
	key := client.ObjectKey{Namespace: runtimeCheckNamespace, Name: seed}
	if err := f.store.Get(context.Background(), key, &pruned); err != nil {
		t.Fatalf("get the seed report: %v", err)
	}
	if err := f.store.Delete(context.Background(), &pruned); err != nil {
		t.Fatalf("prune the seed report: %v", err)
	}

	f.advance(time.Minute)
	_, again := f.runAndRecord()
	if again != "" {
		t.Errorf("a no-change run re-created the pruned report as %q; retention deletes history on purpose", again)
	}
	if got := len(f.reports()); got != 0 {
		t.Errorf("stored %d reports, want the pruned history left alone", got)
	}
}

// contracts/runtime.md's recovery column requires history to "remain
// attributable": a report records what a run PUBLISHED, so status and history
// can never disagree about what that run concluded. Recomputing the aggregate
// at report time folds the same per-check results through whatever spec.policy
// says NOW and yields a second, independent answer — one that was never
// fenced, never published and never stored as evidence.
//
// On the live path publication and report are microseconds apart, so the two
// answers coincide and no end-to-end row can tell them apart. This drives the
// seam directly with them deliberately diverged, which is the shape a policy
// edit landing between the publication compare-and-swap and the report create
// actually produces.
func TestAReportCarriesThePublishedVerdictNotOneRecomputedAtReportTime(t *testing.T) {
	f := newRuntimeCheckFixture(t)
	f.ready()

	attempt := f.runOK()
	if !attempt.Published {
		t.Fatalf("the seed run published nothing: %+v", attempt)
	}
	published := f.check()

	// What the run published, against what a report-time recomputation of its
	// own checks would conclude.
	published.Status.LastSuccessfulEvaluation.Verdict = fathomv1alpha1.AddonCheckEvidenceVerdictWarn
	recomputed, _ := aggregateWithRatioRollups(attempt.Evidence.Checks, ratioThresholdsByFamily(published))
	if string(recomputed) == string(published.Status.LastSuccessfulEvaluation.Verdict) {
		t.Fatalf("this row needs the published verdict and the recomputed aggregate to differ; both are %q", recomputed)
	}

	name, err := (&AddonCheckReconciler{Client: f.cached, Scheme: f.scheme}).
		recordRuntimeTransition(context.Background(), logr.Discard(), published, nil, attempt)
	if err != nil {
		t.Fatalf("record runtime transition: %v", err)
	}
	if name == "" {
		t.Fatal("the first completed evaluation created no report")
	}

	var report fathomv1alpha1.HealthReport
	key := client.ObjectKey{Namespace: runtimeCheckNamespace, Name: name}
	if err := f.store.Get(context.Background(), key, &report); err != nil {
		t.Fatalf("get the report: %v", err)
	}
	if report.Spec.Result != fathomv1alpha1.HealthReportResultWarn {
		t.Errorf("report result = %q, want the PUBLISHED verdict Warn; %q is what a report-time recomputation produces, "+
			"and a verdict nothing published cannot be attributed to a run",
			report.Spec.Result, recomputed)
	}
	// Not vacuous: the run's own per-check entries are still carried, so this
	// cannot be satisfied by a report that dropped them.
	if len(report.Spec.Checks) != len(attempt.Evidence.Checks) {
		t.Errorf("report carries %d check entries, want the run's %d",
			len(report.Spec.Checks), len(attempt.Evidence.Checks))
	}
	if report.Spec.Attribution == nil ||
		report.Spec.Attribution.Coverage != published.Status.LastSuccessfulEvaluation.Coverage {
		t.Errorf("report attribution = %+v, want the published coverage %q",
			report.Spec.Attribution, published.Status.LastSuccessfulEvaluation.Coverage)
	}
}
