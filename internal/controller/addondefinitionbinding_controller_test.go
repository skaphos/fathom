/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/registry"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
)

// This file is the drain-acknowledgement half of
// specs/012-addon-definition-runtime/contracts/leadership.md:
//
//	"For disabled bindings, acknowledge only after observing enabled=false
//	 directly, cancelling/awaiting this session's work, and observing
//	 activeRuns=0. Re-read the Lease and binding through uncached control-plane
//	 reads before publishing the matching generation/epoch, Drained=True and
//	 Ready=False/AuthorizationRevoked. Treat read failure or epoch mismatch as
//	 unverifiable, not drained. Re-enable/spec edit removes acknowledgement
//	 eligibility; status writes do not change authority."
//
// Every fixture therefore keeps the *cached* view and the *uncached* control
// plane in separate fake clients. They are allowed to disagree, which is the
// only way a test can tell whether a decision was made from the informer cache
// or from a direct read — the distinction the contract rests on.

const (
	drainLeaseName    = "2d3dbc4f.skaphos.io"
	drainHolder       = "fathom-operator-0_1f0c7c62-0a1a-4c0f-9d62-2a9a0e5c6f11"
	drainLeaseUID     = "lease-uid-1"
	drainLimitation   = "observed leadership only: a suspended prior holder is not fenced"
	drainOtherHolder  = "fathom-operator-1_8b6f0a1e-77d1-4c2a-9a41-0a0de2b4f000"
	drainOtherLeaseID = "lease-uid-2"
)

// The entries of the fixture's ordered trace. Cancellation is traced next to
// the direct reads because the contract orders them relative to each other:
// "cancelling/awaiting this session's work" comes before the observations the
// acknowledgement rests on, and a trace that recorded only reads could not
// tell a revocation that precedes them from one that follows.
const (
	eventRevoke      = "revoke admission"
	eventReadBinding = "direct read binding"
	eventReadLease   = "direct read lease " + lifecycleNamespace + "/" + drainLeaseName
	eventPublish     = "publish status"
)

var drainAcquired = metav1.NewMicroTime(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))

func drainEpoch() fathomv1alpha1.DefinitionLeaderEpoch {
	return fathomv1alpha1.DefinitionLeaderEpoch{
		LeaseUID:         drainLeaseUID,
		HolderIdentity:   drainHolder,
		AcquireTime:      drainAcquired,
		LeaseTransitions: 3,
	}
}

// drainLeaseObject is the manager's own leader-election Lease, carrying the
// live leadership evidence leadership.md requires: holder, acquireTime,
// renewTime, a positive duration and a transition count.
func drainLeaseObject() *coordinationv1.Lease {
	holder := drainHolder
	renew := metav1.NewMicroTime(drainAcquired.Add(9 * time.Second))
	duration := int32(15)
	transitions := int32(3)
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Namespace: lifecycleNamespace, Name: drainLeaseName, UID: drainLeaseUID},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holder,
			AcquireTime:          &drainAcquired,
			RenewTime:            &renew,
			LeaseDurationSeconds: &duration,
			LeaseTransitions:     &transitions,
		},
	}
}

// --- the leadership session seam -------------------------------------------

// fakeLeadershipSession mirrors internal/app.RuntimeLeadership's observable
// semantics: admissibility gates everything, an unobserved epoch is fatal to an
// acknowledgement, admission must already be revoked, and this session's own
// unreleased runs block the acknowledgement. Mirroring it matters — a fake that
// said yes more readily than the real session would let a controller bug pass.
type fakeLeadershipSession struct {
	lease        types.NamespacedName
	epoch        *fathomv1alpha1.DefinitionLeaderEpoch
	inadmissible error
	runs         map[string]int
	revoked      map[string]bool
	revokes      map[string]int
	restores     map[string]int

	// evidenceEpoch overrides the epoch handed out with an acknowledgement, so
	// a test can model a session whose evidence disagrees with the Lease it
	// validates against — the one thing the published epoch must never be.
	evidenceEpoch *fathomv1alpha1.DefinitionLeaderEpoch
	// afterAcknowledge runs in the window between the acknowledgement and the
	// publication, where leadership can still be lost.
	afterAcknowledge func()

	// onRevoke records cancellation in the fixture's ordered trace. Drain
	// correctness is an ordering property: cancellation precedes every
	// observation the acknowledgement is built from, which is only checkable
	// if a revocation appears in the same trace as the direct reads.
	onRevoke func()
}

func newFakeSession() *fakeLeadershipSession {
	epoch := drainEpoch()
	return &fakeLeadershipSession{
		lease:    types.NamespacedName{Namespace: lifecycleNamespace, Name: drainLeaseName},
		epoch:    &epoch,
		runs:     map[string]int{},
		revoked:  map[string]bool{},
		revokes:  map[string]int{},
		restores: map[string]int{},
	}
}

func (s *fakeLeadershipSession) LeaseRef() types.NamespacedName { return s.lease }

func (s *fakeLeadershipSession) EpochValid(persisted *fathomv1alpha1.DefinitionLeaderEpoch) bool {
	if persisted == nil || s.epoch == nil || s.inadmissible != nil {
		return false
	}
	return s.epoch.LeaseUID == persisted.LeaseUID &&
		s.epoch.HolderIdentity == persisted.HolderIdentity &&
		s.epoch.AcquireTime.Equal(&persisted.AcquireTime) &&
		s.epoch.LeaseTransitions == persisted.LeaseTransitions
}

func (s *fakeLeadershipSession) Revoke(key string) {
	s.revoked[key] = true
	s.revokes[key]++
	if s.onRevoke != nil {
		s.onRevoke()
	}
}

func (s *fakeLeadershipSession) Restore(key string) {
	delete(s.revoked, key)
	s.restores[key]++
}

func (s *fakeLeadershipSession) ActiveRuns(key string) int { return s.runs[key] }

func (s *fakeLeadershipSession) AcknowledgeDrain(key string) (DrainEvidence, error) {
	switch {
	case s.inadmissible != nil:
		return DrainEvidence{}, s.inadmissible
	case s.epoch == nil:
		return DrainEvidence{}, fmt.Errorf("no live leader epoch has been observed for this session")
	case !s.revoked[key]:
		return DrainEvidence{}, fmt.Errorf("runtime admission for %q has not been revoked", key)
	case s.runs[key] > 0:
		return DrainEvidence{}, fmt.Errorf("this leadership session still has %d active runs for %q", s.runs[key], key)
	}
	epoch := *s.epoch
	if s.evidenceEpoch != nil {
		epoch = *s.evidenceEpoch
	}
	if s.afterAcknowledge != nil {
		s.afterAcknowledge()
	}
	return DrainEvidence{Epoch: epoch, Limitation: drainLimitation}, nil
}

var _ RuntimeLeadershipSession = (*fakeLeadershipSession)(nil)

// --- fixture ---------------------------------------------------------------

type drainFixture struct {
	t       *testing.T
	scheme  *runtime.Scheme
	cache   client.WithWatch
	live    client.WithWatch
	session *fakeLeadershipSession
	subject *AddonDefinitionBindingReconciler

	// events is the ordered trace of uncached reads and status publications.
	// Drain correctness is an ordering property — read directly, then cancel,
	// then re-read, then publish — so the order is asserted, not just the set.
	events []string
	// liveGetErr injects a control-plane read failure into the uncached reader.
	liveGetErr func(obj client.Object) error
	// statusErr injects a status-write failure.
	statusErr func() error
	// unboundedReads counts direct reads issued without a bounded deadline. A
	// Reconcile must never wait on the control plane indefinitely.
	unboundedReads int
}

func drainDisabledBinding(generation int64) *fathomv1alpha1.AddonDefinitionBinding {
	b := lifecycleBinding()
	b.Spec.Enabled = false
	b.Generation = generation
	return b
}

func newDrainFixture(t *testing.T, objs ...client.Object) *drainFixture {
	t.Helper()
	f := &drainFixture{t: t, scheme: runtime.NewScheme()}
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, coordinationv1.AddToScheme, fathomv1alpha1.AddToScheme,
	} {
		if err := add(f.scheme); err != nil {
			t.Fatalf("scheme: %v", err)
		}
	}

	builder := func(record bool) client.WithWatch {
		copies := make([]client.Object, 0, len(objs))
		for _, obj := range objs {
			copies = append(copies, obj.DeepCopyObject().(client.Object))
		}
		b := fake.NewClientBuilder().
			WithScheme(f.scheme).
			WithStatusSubresource(&fathomv1alpha1.AddonDefinition{}, &fathomv1alpha1.AddonDefinitionBinding{}).
			WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingDefinitionName, indexBindingByDefinitionName).
			WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingServiceAccountName, indexBindingByServiceAccountName).
			WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingServiceAccountUID, indexBindingByServiceAccountUID).
			WithObjects(copies...)
		if record {
			b = b.WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					switch obj.(type) {
					case *fathomv1alpha1.AddonDefinitionBinding:
						f.events = append(f.events, eventReadBinding)
					case *coordinationv1.Lease:
						f.events = append(f.events, "direct read lease "+key.String())
					}
					if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > definitions.MaxRequestDuration {
						f.unboundedReads++
					}
					if f.liveGetErr != nil {
						if err := f.liveGetErr(obj); err != nil {
							return err
						}
					}
					return c.Get(ctx, key, obj, opts...)
				},
			})
		} else {
			b = b.WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					f.events = append(f.events, eventPublish)
					if f.statusErr != nil {
						return f.statusErr()
					}
					return c.SubResource(sub).Update(ctx, obj, opts...)
				},
			})
		}
		return b.Build()
	}
	f.cache = builder(false)
	f.live = builder(true)
	f.session = newFakeSession()
	f.session.onRevoke = func() { f.events = append(f.events, eventRevoke) }
	f.subject = &AddonDefinitionBindingReconciler{
		Client:                f.cache,
		APIReader:             f.live,
		Scheme:                f.scheme,
		Registry:              registry.New(logr.Discard()),
		OperatorNamespace:     lifecycleNamespace,
		ManagerServiceAccount: lifecycleManagerSA,
		CacheSynced:           func(context.Context) bool { return true },
		Leadership:            f.session,
	}
	return f
}

func (f *drainFixture) reconcile() (ctrl.Result, error) {
	f.t.Helper()
	return f.subject.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: lifecycleNamespace, Name: lifecycleAddon},
	})
}

func (f *drainFixture) reconcileOK() ctrl.Result {
	f.t.Helper()
	res, err := f.reconcile()
	if err != nil {
		f.t.Fatalf("reconcile binding: %v", err)
	}
	return res
}

func (f *drainFixture) statusWrites() int {
	count := 0
	for _, e := range f.events {
		if e == eventPublish {
			count++
		}
	}
	return count
}

func (f *drainFixture) reads() []string {
	trace := make([]string, 0, len(f.events))
	for _, e := range f.events {
		if strings.HasPrefix(e, "direct read") {
			trace = append(trace, e)
		}
	}
	return trace
}

// binding returns the object as the reconciler's own writes left it.
func (f *drainFixture) binding() *fathomv1alpha1.AddonDefinitionBinding {
	f.t.Helper()
	var b fathomv1alpha1.AddonDefinitionBinding
	if err := f.cache.Get(context.Background(), types.NamespacedName{Namespace: lifecycleNamespace, Name: lifecycleAddon}, &b); err != nil {
		f.t.Fatalf("get binding: %v", err)
	}
	return &b
}

// put writes obj to both views, keeping the cache and the control plane in
// agreement. Divergence is always explicit, through putLive or putCached.
func (f *drainFixture) put(obj client.Object) {
	f.t.Helper()
	f.putCached(obj)
	f.putLive(obj)
}

func (f *drainFixture) putCached(obj client.Object) {
	f.t.Helper()
	f.write(f.cache, obj)
}

func (f *drainFixture) putLive(obj client.Object) {
	f.t.Helper()
	f.write(f.live, obj)
}

func (f *drainFixture) write(c client.Client, obj client.Object) {
	f.t.Helper()
	ctx := context.Background()
	existing, err := f.scheme.New(mustGVK(f.t, f.scheme, obj))
	if err != nil {
		f.t.Fatalf("new %T: %v", obj, err)
	}
	current := existing.(client.Object)
	key := client.ObjectKeyFromObject(obj)
	switch err := c.Get(ctx, key, current); {
	case apierrors.IsNotFound(err):
		if err := c.Create(ctx, obj.DeepCopyObject().(client.Object)); err != nil {
			f.t.Fatalf("create %s: %v", key, err)
		}
		return
	case err != nil:
		f.t.Fatalf("get %s: %v", key, err)
	}
	next := obj.DeepCopyObject().(client.Object)
	next.SetResourceVersion(current.GetResourceVersion())
	if err := c.Update(ctx, next); err != nil {
		f.t.Fatalf("update %s: %v", key, err)
	}
}

// putStatus performs a status-only write: it touches the status subresource and
// leaves metadata.generation exactly where it was.
func (f *drainFixture) putStatus(mutate func(*fathomv1alpha1.AddonDefinitionBinding)) {
	f.t.Helper()
	ctx := context.Background()
	for _, c := range []client.Client{f.cache, f.live} {
		var b fathomv1alpha1.AddonDefinitionBinding
		if err := c.Get(ctx, types.NamespacedName{Namespace: lifecycleNamespace, Name: lifecycleAddon}, &b); err != nil {
			f.t.Fatalf("get binding for status write: %v", err)
		}
		generation := b.Generation
		mutate(&b)
		if err := c.Status().Update(ctx, &b); err != nil {
			f.t.Fatalf("status write: %v", err)
		}
		var after fathomv1alpha1.AddonDefinitionBinding
		if err := c.Get(ctx, types.NamespacedName{Namespace: lifecycleNamespace, Name: lifecycleAddon}, &after); err != nil {
			f.t.Fatalf("re-read binding after status write: %v", err)
		}
		if after.Generation != generation {
			f.t.Fatalf("a status-only write changed metadata.generation from %d to %d", generation, after.Generation)
		}
	}
}

func mustGVK(t *testing.T, scheme *runtime.Scheme, obj client.Object) schema.GroupVersionKind {
	t.Helper()
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		t.Fatalf("object kinds for %T: %v", obj, err)
	}
	return gvks[0]
}

func requireEpoch(t *testing.T, got *fathomv1alpha1.DefinitionLeaderEpoch, want fathomv1alpha1.DefinitionLeaderEpoch) {
	t.Helper()
	if got == nil {
		t.Fatalf("status.leaderEpoch is absent; a drain acknowledgement without an epoch is unverifiable")
	}
	if got.LeaseUID != want.LeaseUID || got.HolderIdentity != want.HolderIdentity ||
		!got.AcquireTime.Equal(&want.AcquireTime) || got.LeaseTransitions != want.LeaseTransitions {
		t.Fatalf("status.leaderEpoch: want %+v, got %+v", want, *got)
	}
}

func requireNotDrained(t *testing.T, b *fathomv1alpha1.AddonDefinitionBinding) {
	t.Helper()
	if cond, ok := conditionByType(b.Status.Conditions, definitionConditionDrained); ok && cond.Status == metav1.ConditionTrue {
		t.Fatalf("binding is reported Drained=True (%s: %s); unverifiable is not drained", cond.Reason, cond.Message)
	}
	if b.Status.LeaderEpoch != nil {
		t.Fatalf("an unacknowledged binding still carries leader epoch %+v", *b.Status.LeaderEpoch)
	}
	if b.Status.LeaderIdentity != "" {
		t.Fatalf("an unacknowledged binding still names leader %q", b.Status.LeaderIdentity)
	}
}

// --- drain acknowledgement --------------------------------------------------

// The acknowledgement the operator publishes must be exactly the evidence
// `fathomctl definition drain` verifies (contracts/leadership.md step 3):
// enabled=false, observedGeneration == metadata.generation, activeRuns=0,
// Drained=True and Ready=False/AuthorizationRevoked both stamped with that same
// generation, and a leaderEpoch matching the live Lease.
func TestDrainAcknowledgementPublishesTheEvidenceTheCLIVerifies(t *testing.T) {
	f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())

	res := f.reconcileOK()

	if res != (ctrl.Result{}) {
		t.Fatalf("an acknowledged drain is a settled state, not a retry: %+v", res)
	}
	b := f.binding()
	drained := requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionTrue, reasonDrainAcknowledged)
	requireCondition(t, b.Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAuthorizationRevoked)
	for _, cond := range b.Status.Conditions {
		if cond.ObservedGeneration != b.Generation {
			t.Fatalf("condition %q is stamped generation %d, but the object is at %d; the CLI ignores it and reports not drained",
				cond.Type, cond.ObservedGeneration, b.Generation)
		}
	}
	if b.Status.ObservedGeneration != b.Generation {
		t.Fatalf("status.observedGeneration %d does not match generation %d", b.Status.ObservedGeneration, b.Generation)
	}
	if b.Status.ActiveRuns != 0 {
		t.Fatalf("an acknowledged drain must report activeRuns=0, got %d", b.Status.ActiveRuns)
	}
	requireEpoch(t, b.Status.LeaderEpoch, drainEpoch())
	if b.Status.LeaderIdentity != drainHolder {
		t.Fatalf("status.leaderIdentity: want the epoch's holder %q, got %q", drainHolder, b.Status.LeaderIdentity)
	}
	if !strings.Contains(drained.Message, "observed leadership only") {
		t.Fatalf("the acknowledgement must carry the observation limitation, got %q", drained.Message)
	}
	if f.session.revokes[lifecycleAddon] == 0 {
		t.Fatal("admission was never revoked; a drain may only be claimed for work that was cancelled first")
	}

	if f.unboundedReads != 0 {
		t.Fatalf("%d direct read(s) ran without a bounded deadline; a reconcile must not wait on the control plane indefinitely", f.unboundedReads)
	}

	// Idempotent: nothing changed, so nothing is rewritten.
	writes := f.statusWrites()
	f.reconcileOK()
	if f.statusWrites() != writes {
		t.Fatalf("a repeated reconcile rewrote the acknowledgement (%d writes, was %d)", f.statusWrites(), writes)
	}
}

// "acknowledge only after observing enabled=false directly ... re-read the
// Lease and binding through uncached control-plane reads before publishing."
// The informer cache is never sufficient, in either direction.
func TestDrainRequiresUncachedBindingAndLeaseReadsBeforePublishing(t *testing.T) {
	t.Run("order", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())

		f.reconcileOK()

		// The revocation is first: "acknowledge only after ... cancelling /
		// awaiting this session's work". An acknowledgement describes work
		// that was already stopped, so a cancellation issued after the
		// observations would describe a window in which the binding was
		// observed drained while its work was still admitted.
		want := []string{
			eventRevoke,
			eventReadBinding,
			eventReadBinding,
			eventReadLease,
			eventPublish,
		}
		if !slices.Equal(f.events, want) {
			t.Fatalf("drain evidence trace: want %v, got %v", want, f.events)
		}
	})

	t.Run("cache says disabled but the control plane says enabled", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		// The generation is deliberately held equal, so the direct observation
		// of spec.enabled is the only thing that can reject this: a drain must
		// not be acknowledged because a second fence happened to catch it.
		reenabled := drainDisabledBinding(4)
		reenabled.Spec.Enabled = true
		f.putLive(reenabled)
		f.events = nil // the trace below is the reconcile's, not the setup's

		f.reconcileOK()

		requireNotDrained(t, f.binding())
		// The rejection happens at the *first* fence, on the first direct
		// read: one read, no session call, no Lease read. The final fence
		// would reach the same verdict, which is precisely why the outcome
		// alone cannot tell whether the first fence exists — only the trace
		// can, and an ineligible binding that costs three reads and an
		// acknowledgement has spent a budget on a decision already made.
		want := []string{eventRevoke, eventReadBinding, eventPublish}
		if !slices.Equal(f.events, want) {
			t.Fatalf("an ineligible binding must be rejected on the first direct read: want %v, got %v", want, f.events)
		}
		// Admission stays closed. Reopening it belongs to the reconcile that
		// observes the enabled binding and revalidates its authority; a live
		// read of spec.enabled is not an authorization decision.
		if !f.session.revoked[lifecycleAddon] || f.session.restores[lifecycleAddon] != 0 {
			t.Fatalf("admission was reopened from a bare enabled flag (revoked=%v restores=%d)",
				f.session.revoked[lifecycleAddon], f.session.restores[lifecycleAddon])
		}
	})

	t.Run("spec edited between the observation and the publication", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		edited := drainDisabledBinding(5)
		edited.Spec.TargetScope.Namespaces = []fathomv1alpha1.DefinitionDNSLabel{"kube-system"}
		f.putLive(edited)

		f.reconcileOK()

		requireNotDrained(t, f.binding())
	})

	t.Run("binding deleted from the control plane", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		if err := f.live.Delete(context.Background(), drainDisabledBinding(4)); err != nil {
			t.Fatalf("delete live binding: %v", err)
		}

		if _, err := f.reconcile(); err != nil {
			t.Fatalf("a deleted binding is not an error: %v", err)
		}
		requireNotDrained(t, f.binding())
	})

	t.Run("binding is being deleted", func(t *testing.T) {
		// A finalized delete leaves the object readable with a deletion
		// timestamp. There is no durable object to acknowledge on, so the
		// acknowledgement is not published — it would vanish with the object.
		deleting := drainDisabledBinding(4)
		deleting.Finalizers = []string{"test.fathom.skaphos.io/hold"}
		f := newDrainFixture(t, lifecycleDefinition(), deleting, lifecycleServiceAccount(), drainLeaseObject())
		if err := f.live.Delete(context.Background(), deleting.DeepCopy()); err != nil {
			t.Fatalf("delete live binding: %v", err)
		}

		f.reconcileOK()

		requireNotDrained(t, f.binding())
	})
}

// status.activeRuns carries a 0..4 schema bound, so a session reporting more
// than the contract's concurrency ceiling must be clamped rather than written
// through: an out-of-range status write is rejected by the API server outright,
// which would lose the diagnostic entirely instead of degrading it.
func TestActiveRunsIsClampedToTheSchemaBound(t *testing.T) {
	f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
	f.session.runs[lifecycleAddon] = definitions.MaxConcurrentRuns + 7

	f.reconcileOK()

	if got := f.binding().Status.ActiveRuns; got != definitions.MaxConcurrentRuns {
		t.Fatalf("status.activeRuns = %d, want it clamped to the %d schema bound", got, definitions.MaxConcurrentRuns)
	}
}

// "cancelling/awaiting this session's work, and observing activeRuns=0". The
// await is bounded and requeued rather than blocking: a Reconcile never waits.
func TestDrainRefusedWhileThisSessionHasActiveRuns(t *testing.T) {
	f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
	f.session.runs[lifecycleAddon] = 2

	res := f.reconcileOK()

	if res.RequeueAfter != definitions.InitialRetryBackoff {
		t.Fatalf("an unfinished drain must requeue within a bounded %s, got %+v", definitions.InitialRetryBackoff, res)
	}
	b := f.binding()
	requireNotDrained(t, b)
	requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionFalse, reasonDrainUnverifiable)
	if b.Status.ActiveRuns != 2 {
		t.Fatalf("status.activeRuns must report the runs still unwinding, want 2, got %d", b.Status.ActiveRuns)
	}
	if f.session.revokes[lifecycleAddon] == 0 {
		t.Fatal("the session's work was never cancelled; awaiting uncancelled work is not draining")
	}

	// The cancelled work unwinds and the acknowledgement becomes publishable.
	f.session.runs[lifecycleAddon] = 0
	f.reconcileOK()

	b = f.binding()
	requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionTrue, reasonDrainAcknowledged)
	if b.Status.ActiveRuns != 0 {
		t.Fatalf("an acknowledged drain must report activeRuns=0, got %d", b.Status.ActiveRuns)
	}
}

// "Treat read failure or epoch mismatch as unverifiable, not drained." Epoch
// equality uses all four fields, so each one is mutated on its own.
func TestDrainRejectsStaleMissingAndUnreadableEpochs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		setup     func(f *drainFixture)
		wantError bool
		// wantReady is the Ready reason the row must leave published. Empty
		// means the authority reason a disabled binding always carries.
		wantReady string
	}{
		// The matrix's "RBAC denies an API read | Ready=False/AccessDenied"
		// row covers the drain path's own direct reads. AuthorizationRevoked
		// would tell an administrator the binding is switched off — true, but
		// not why the operator cannot finish, and not something a grant fixes.
		{"lease read denied", func(f *drainFixture) {
			f.liveGetErr = func(obj client.Object) error {
				if _, ok := obj.(*coordinationv1.Lease); ok {
					return apierrors.NewForbidden(schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"}, drainLeaseName, fmt.Errorf("nope"))
				}
				return nil
			}
		}, true, reasonAccessDenied},
		{"binding read denied", func(f *drainFixture) {
			f.liveGetErr = func(obj client.Object) error {
				if _, ok := obj.(*fathomv1alpha1.AddonDefinitionBinding); ok {
					return apierrors.NewForbidden(schema.GroupResource{Group: fathomv1alpha1.GroupVersion.Group, Resource: "addondefinitionbindings"}, lifecycleAddon, fmt.Errorf("nope"))
				}
				return nil
			}
		}, true, reasonAccessDenied},
		// An expired or rejected credential arrives as 401, not 403. It is the
		// same row for the same reason — the operator cannot finish its reads —
		// so it must not be reported as though the binding were switched off.
		{"lease read unauthorized", func(f *drainFixture) {
			f.liveGetErr = func(obj client.Object) error {
				if _, ok := obj.(*coordinationv1.Lease); ok {
					return apierrors.NewUnauthorized("token expired")
				}
				return nil
			}
		}, true, reasonAccessDenied},
		{"binding read unauthorized", func(f *drainFixture) {
			f.liveGetErr = func(obj client.Object) error {
				if _, ok := obj.(*fathomv1alpha1.AddonDefinitionBinding); ok {
					return apierrors.NewUnauthorized("token expired")
				}
				return nil
			}
		}, true, reasonAccessDenied},
		{"lease missing", func(f *drainFixture) {
			if err := f.live.Delete(context.Background(), drainLeaseObject()); err != nil {
				f.t.Fatalf("delete lease: %v", err)
			}
		}, true, ""},
		{"lease uid changed", func(f *drainFixture) {
			lease := drainLeaseObject()
			lease.UID = drainOtherLeaseID
			f.putLive(lease)
		}, false, ""},
		{"holder identity changed", func(f *drainFixture) {
			lease := drainLeaseObject()
			other := drainOtherHolder
			lease.Spec.HolderIdentity = &other
			f.putLive(lease)
		}, false, ""},
		{"acquire time changed", func(f *drainFixture) {
			lease := drainLeaseObject()
			acquired := metav1.NewMicroTime(drainAcquired.Add(time.Minute))
			lease.Spec.AcquireTime = &acquired
			f.putLive(lease)
		}, false, ""},
		{"lease transitions changed", func(f *drainFixture) {
			lease := drainLeaseObject()
			transitions := int32(4)
			lease.Spec.LeaseTransitions = &transitions
			f.putLive(lease)
		}, false, ""},
		// A Lease without renewal evidence is not a settled fact about stored
		// state: the next renewal supplies it, so it is retried.
		{"lease lacks live renewal evidence", func(f *drainFixture) {
			lease := drainLeaseObject()
			lease.Spec.RenewTime = nil
			f.putLive(lease)
		}, true, ""},
		{"session observed no epoch", func(f *drainFixture) { f.session.epoch = nil }, false, ""},
		{"session is not admissible", func(f *drainFixture) {
			f.session.inadmissible = fmt.Errorf("runtime admission is closed until 30s of monotonic takeover grace elapses")
		}, false, ""},
		// The decoy makes the misconfiguration observable: a reconciler that
		// reads whatever Lease it is pointed at would find complete, matching
		// evidence there and acknowledge a drain from outside the operator
		// namespace — the cluster-wide Lease read this feature refuses to add.
		{"configured lease is outside the operator namespace", func(f *drainFixture) {
			decoy := drainLeaseObject()
			decoy.Namespace = "kube-system"
			decoy.ResourceVersion = ""
			if err := f.live.Create(context.Background(), decoy); err != nil {
				f.t.Fatalf("create decoy lease: %v", err)
			}
			f.session.lease = types.NamespacedName{Namespace: "kube-system", Name: drainLeaseName}
		}, true, ""},
		{"uncached reader unwired", func(f *drainFixture) { f.subject.APIReader = nil }, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
			tc.setup(f)

			_, err := f.reconcile()
			if tc.wantError && err == nil {
				t.Fatal("a failed control-plane read must be returned so the reconcile is retried with backoff")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("an epoch mismatch is a stored-state fact, not a retryable error: %v", err)
			}
			requireNotDrained(t, f.binding())
			wantReady := tc.wantReady
			if wantReady == "" {
				wantReady = reasonAuthorizationRevoked
			}
			requireCondition(t, f.binding().Status.Conditions, definitionConditionReady, metav1.ConditionFalse, wantReady)
		})
	}
}

// leadership.md's required tests include "leader loss ... during drain". The
// acknowledgement is granted, then leadership ends before the publication: the
// session is asked again, against the live Lease, and the drain is dropped.
func TestDrainRefusedWhenLeadershipEndsBeforePublication(t *testing.T) {
	f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
	f.session.afterAcknowledge = func() {
		f.session.inadmissible = fmt.Errorf("runtime leadership ended; this process does not reacquire it")
	}

	f.reconcileOK()

	requireNotDrained(t, f.binding())
}

// The epoch published is the one an administrator will match against the live
// Lease, so it must be the epoch the Lease actually carries — not merely one
// the session was willing to vouch for.
func TestDrainRefusedWhenTheSessionEvidenceDisagreesWithTheLiveLease(t *testing.T) {
	f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
	divergent := drainEpoch()
	divergent.LeaseTransitions = 7
	f.session.evidenceEpoch = &divergent

	f.reconcileOK()

	requireNotDrained(t, f.binding())
}

// "On acquisition, distrust persisted acknowledgements." A Drained=True written
// by an earlier leadership epoch is evidence about that epoch only.
func TestPersistedDrainAcknowledgementIsDistrustedByANewLeader(t *testing.T) {
	stale := fathomv1alpha1.DefinitionLeaderEpoch{
		LeaseUID: drainOtherLeaseID, HolderIdentity: drainOtherHolder,
		AcquireTime: metav1.NewMicroTime(drainAcquired.Add(-time.Hour)), LeaseTransitions: 2,
	}

	t.Run("new session cannot yet verify", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		f.putStatus(func(b *fathomv1alpha1.AddonDefinitionBinding) {
			b.Status.ObservedGeneration = 4
			b.Status.LeaderIdentity = stale.HolderIdentity
			b.Status.LeaderEpoch = stale.DeepCopy()
			b.Status.Conditions = []fathomv1alpha1.DefinitionStatusCondition{{
				Type: definitionConditionDrained, Status: metav1.ConditionTrue, ObservedGeneration: 4,
				Reason: reasonDrainAcknowledged, Message: "acknowledged by the previous leader",
			}}
		})
		f.session.inadmissible = fmt.Errorf("takeover grace pending")

		f.reconcileOK()

		requireNotDrained(t, f.binding())
	})

	t.Run("new session republishes under its own epoch", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		f.putStatus(func(b *fathomv1alpha1.AddonDefinitionBinding) {
			b.Status.LeaderIdentity = stale.HolderIdentity
			b.Status.LeaderEpoch = stale.DeepCopy()
		})

		f.reconcileOK()

		b := f.binding()
		requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionTrue, reasonDrainAcknowledged)
		requireEpoch(t, b.Status.LeaderEpoch, drainEpoch())
		if b.Status.LeaderIdentity != drainHolder {
			t.Fatalf("status.leaderIdentity still names the previous leader %q", b.Status.LeaderIdentity)
		}
	})
}

// "status writes do not change authority". A status subresource write can
// neither revoke a live authorization nor manufacture a drain, and it never
// moves metadata.generation, so it cannot trip the acknowledgement's fence.
func TestStatusOnlyWritesNeitherGrantNorRevokeAuthority(t *testing.T) {
	t.Run("a forged status cannot grant a drain or revoke authority", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), lifecycleBinding(), lifecycleServiceAccount(), drainLeaseObject())
		forged := drainEpoch()
		f.putStatus(func(b *fathomv1alpha1.AddonDefinitionBinding) {
			b.Status.ActiveRuns = 3
			b.Status.LeaderIdentity = drainHolder
			b.Status.LeaderEpoch = forged.DeepCopy()
			b.Status.Conditions = []fathomv1alpha1.DefinitionStatusCondition{{
				Type: definitionConditionDrained, Status: metav1.ConditionTrue, ObservedGeneration: 1,
				Reason: reasonDrainAcknowledged, Message: "forged",
			}}
		})

		f.reconcileOK()

		b := f.binding()
		requireCondition(t, b.Status.Conditions, definitionConditionReady, metav1.ConditionTrue, reasonBindingAuthorized)
		requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionFalse, reasonBindingEnabled)
		if b.Status.LeaderEpoch != nil {
			t.Fatalf("an enabled binding kept a drain epoch %+v", *b.Status.LeaderEpoch)
		}
		if b.Generation != 1 {
			t.Fatalf("status handling changed metadata.generation to %d", b.Generation)
		}
	})

	t.Run("a status write after acknowledgement does not trip the generation fence", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		f.reconcileOK()
		requireCondition(t, f.binding().Status.Conditions, definitionConditionDrained, metav1.ConditionTrue, reasonDrainAcknowledged)

		// Something else touches status only — an active-run counter, say.
		f.putStatus(func(b *fathomv1alpha1.AddonDefinitionBinding) { b.Status.ActiveRuns = 1 })

		f.reconcileOK()

		b := f.binding()
		requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionTrue, reasonDrainAcknowledged)
		requireEpoch(t, b.Status.LeaderEpoch, drainEpoch())
		if b.Status.ObservedGeneration != 4 || b.Generation != 4 {
			t.Fatalf("the generation fence moved: generation %d, observedGeneration %d", b.Generation, b.Status.ObservedGeneration)
		}
		if b.Status.ActiveRuns != 0 {
			t.Fatalf("the republished acknowledgement must restate activeRuns=0, got %d", b.Status.ActiveRuns)
		}
	})
}

// A disabled binding is revoked because it is disabled, not because its
// dependencies resolve. The CLI verifies Ready=False/AuthorizationRevoked
// specifically, so a drained binding whose definition an administrator already
// deleted must still carry that reason rather than DefinitionUnavailable.
func TestDisabledBindingIsRevokedEvenWithoutItsDefinition(t *testing.T) {
	f := newDrainFixture(t, drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())

	f.reconcileOK()

	b := f.binding()
	requireCondition(t, b.Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAuthorizationRevoked)
	requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionTrue, reasonDrainAcknowledged)
	requireEpoch(t, b.Status.LeaderEpoch, drainEpoch())
}

// "Re-enable/spec edit removes acknowledgement eligibility."
func TestReEnableWithdrawsTheAcknowledgementAndReopensAdmission(t *testing.T) {
	f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
	f.reconcileOK()
	requireCondition(t, f.binding().Status.Conditions, definitionConditionDrained, metav1.ConditionTrue, reasonDrainAcknowledged)

	reenabled := drainDisabledBinding(5)
	reenabled.Spec.Enabled = true
	f.put(reenabled)

	f.reconcileOK()

	b := f.binding()
	requireNotDrained(t, b)
	requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionFalse, reasonBindingEnabled)
	requireCondition(t, b.Status.Conditions, definitionConditionReady, metav1.ConditionTrue, reasonBindingAuthorized)
	if f.session.restores[lifecycleAddon] == 0 {
		t.Fatal("explicit reauthorization must reopen admission for the binding")
	}
}

// A status write that fails is retried, except when the object is gone —
// deletion is itself the revocation, and a deleted binding needs no status.
func TestFailedStatusPublicationIsRetriedUnlessTheBindingIsGone(t *testing.T) {
	t.Run("conflict is retried", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		f.statusErr = func() error {
			return apierrors.NewConflict(schema.GroupResource{Group: fathomv1alpha1.GroupVersion.Group, Resource: "addondefinitionbindings"},
				lifecycleAddon, fmt.Errorf("object was modified"))
		}

		if _, err := f.reconcile(); err == nil {
			t.Fatal("a failed status publication must be returned so the reconcile is retried")
		}
	})

	t.Run("a vanished binding is settled", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		f.statusErr = func() error {
			return apierrors.NewNotFound(schema.GroupResource{Group: fathomv1alpha1.GroupVersion.Group, Resource: "addondefinitionbindings"}, lifecycleAddon)
		}

		if _, err := f.reconcile(); err != nil {
			t.Fatalf("a binding deleted under the reconcile is not an error: %v", err)
		}
	})
}

// Lifecycle row: "Binding disabled/deleted or SA replaced | Revoke eligibility
// ... Cancel active work". An enabled binding whose identity no longer matches
// is not authority: admission closes for it, and nothing reopens it until the
// binding is authorized again.
func TestUnauthorizedEnabledBindingKeepsAdmissionClosed(t *testing.T) {
	replaced := lifecycleServiceAccountNamed(lifecycleSAName, "service-account-uid-replaced")
	f := newDrainFixture(t, lifecycleDefinition(), lifecycleBinding(), replaced, drainLeaseObject())

	f.reconcileOK()

	b := f.binding()
	requireCondition(t, b.Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonBindingMismatch)
	requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionFalse, reasonBindingEnabled)
	if !f.session.revoked[lifecycleAddon] || f.session.restores[lifecycleAddon] != 0 {
		t.Fatalf("admission for an unauthorized binding stayed open (revoked=%v restores=%d)",
			f.session.revoked[lifecycleAddon], f.session.restores[lifecycleAddon])
	}
}

// Runtime loading is default-off: with no elected session wired, the binding
// reconciler behaves exactly as it did before drain existed — it reports
// authority and claims nothing about draining.
func TestWithoutALeadershipSessionNoDrainIsClaimed(t *testing.T) {
	f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
	f.subject.Leadership = nil
	f.subject.APIReader = nil

	f.reconcileOK()

	b := f.binding()
	requireCondition(t, b.Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAuthorizationRevoked)
	if _, ok := conditionByType(b.Status.Conditions, definitionConditionDrained); ok {
		t.Fatal("a process that holds no leadership must not report a Drained condition at all")
	}
	if len(f.reads()) != 0 {
		t.Fatalf("no direct control-plane read belongs to a non-leader: %v", f.reads())
	}
}

// requireDrainClaimMatchesTheCLI asserts the conjunction
// internal/cli/definition_drain.go requires before it reports exit 0: a
// Drained=True stamped with the current generation only means "drained" if
// Ready=False/AuthorizationRevoked is published beside it at that same
// generation, with observedGeneration current, activeRuns=0 and a leaderEpoch.
//
// The CLI is the contract's independent verifier. Any state in which the
// operator says "drained" and the verifier says "not drained" is a bug in one
// of them, so every drain outcome in this file is checked against it.
func requireDrainClaimMatchesTheCLI(t *testing.T, b *fathomv1alpha1.AddonDefinitionBinding) {
	t.Helper()
	drained, ok := conditionByType(b.Status.Conditions, definitionConditionDrained)
	if !ok || drained.Status != metav1.ConditionTrue || drained.ObservedGeneration != b.Generation {
		return
	}
	ready, ok := conditionByType(b.Status.Conditions, definitionConditionReady)
	if !ok || ready.Status != metav1.ConditionFalse || ready.Reason != reasonAuthorizationRevoked ||
		ready.ObservedGeneration != b.Generation {
		t.Fatalf("the operator published Drained=True, but `fathomctl definition drain` reads Ready=%+v and reports not drained", ready)
	}
	if b.Spec.Enabled || b.Status.ActiveRuns != 0 || b.Status.ObservedGeneration != b.Generation ||
		b.Status.LeaderEpoch == nil || !b.DeletionTimestamp.IsZero() {
		t.Fatalf("Drained=True with evidence the CLI rejects: enabled=%v activeRuns=%d observedGeneration=%d (generation %d) epoch=%v",
			b.Spec.Enabled, b.Status.ActiveRuns, b.Status.ObservedGeneration, b.Generation, b.Status.LeaderEpoch)
	}
}

// Lifecycle row: "Binding disabled/deleted or SA replaced | Revoke eligibility
// ... | Cancel active work". Withdrawing the runtime snapshot is the definition
// reconciler's half of a deletion. This half cancels the work already running
// under the deleted binding's dedicated identity — without it a run admitted a
// moment before the delete keeps reading the cluster under an authority the
// administrator has just taken away, and finishes.
func TestDeletingABindingCancelsThisSessionsWork(t *testing.T) {
	for _, tc := range []struct {
		name      string
		finalizer bool
	}{
		{"hard delete", false},
		// A finalizer-held delete leaves the object readable. Authority ends
		// when the deletion timestamp is set, not when the object finally
		// disappears, so the work must be cancelled on this pass rather than
		// whenever the finalizer happens to be removed.
		{"held open by a finalizer", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := lifecycleBinding()
			if tc.finalizer {
				binding.Finalizers = []string{"test.fathom.skaphos.io/hold"}
			}
			f := newDrainFixture(t, lifecycleDefinition(), binding, lifecycleServiceAccount(), drainLeaseObject())
			// A run admitted before the delete. It is this session's, so this
			// session is the only thing that can stop it.
			f.session.runs[lifecycleAddon] = 1
			for _, c := range []client.Client{f.cache, f.live} {
				if err := c.Delete(context.Background(), binding.DeepCopy()); err != nil {
					t.Fatalf("delete binding: %v", err)
				}
			}

			res := f.reconcileOK()

			if !f.session.revoked[lifecycleAddon] {
				t.Fatal("the deleted binding's in-flight work was never cancelled; it runs to completion under authority nobody holds")
			}
			if res != (ctrl.Result{}) {
				t.Fatalf("a deleted binding is settled, not requeued: %+v", res)
			}
			// Nothing is published on an object that is leaving: a status
			// written now vanishes with it, and the reads that would back it
			// are budget spent on a decision already made.
			if f.statusWrites() != 0 {
				t.Fatalf("a terminating binding was given %d status write(s)", f.statusWrites())
			}
			if reads := f.reads(); len(reads) != 0 {
				t.Fatalf("a terminating binding cost direct control-plane reads: %v", reads)
			}
			if f.session.restores[lifecycleAddon] != 0 {
				t.Fatal("admission was reopened for a deleted binding")
			}
		})
	}
}

// Lifecycle row: "Restart or partial informer sync | No runtime execution until
// synchronized and directly validated". The gate is the whole of that row on
// this reconciler: before it, the cached binding is whatever a partially
// synchronized informer happens to hold, and a drain acknowledged from that is
// a claim about a view, not about the cluster.
func TestNothingIsObservedOrPublishedBeforeTheCacheIsSynchronized(t *testing.T) {
	t.Run("unsynchronized informers requeue and observe nothing", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		f.subject.CacheSynced = func(context.Context) bool { return false }

		res := f.reconcileOK()

		if res.RequeueAfter != runtimeSyncRequeue {
			t.Fatalf("an unsynchronized cache must requeue within a bounded %s, got %+v", runtimeSyncRequeue, res)
		}
		if len(f.events) != 0 {
			t.Fatalf("an unsynchronized reconcile read or published something: %v", f.events)
		}
		if f.session.revokes[lifecycleAddon] != 0 || f.session.restores[lifecycleAddon] != 0 {
			t.Fatalf("an unsynchronized reconcile changed runtime admission (revokes=%d restores=%d)",
				f.session.revokes[lifecycleAddon], f.session.restores[lifecycleAddon])
		}
		if conds := f.binding().Status.Conditions; len(conds) != 0 {
			t.Fatalf("an unsynchronized reconcile published conditions %+v", conds)
		}
	})

	t.Run("an unwired gate is refused outright", func(t *testing.T) {
		f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
		f.subject.CacheSynced = nil

		_, err := f.reconcile()

		if !errors.Is(err, errCacheSyncUnwired) {
			t.Fatalf("a reconciler with no synchronization gate must refuse to run, got %v", err)
		}
		if len(f.events) != 0 {
			t.Fatalf("a reconciler with no synchronization gate read or published something: %v", f.events)
		}
	})
}

// contracts/leadership.md acknowledges a drain by publishing "Drained=True and
// Ready=False/AuthorizationRevoked", and internal/cli/definition_drain.go
// requires exactly that pair. A disabled binding whose stored spec is invalid
// reports Ready=False/InvalidDefinition, so a Drained=True beside it is a claim
// the contract's own verifier rejects: the operator would say "drained" while
// `fathomctl definition drain` exits 1.
func TestDisabledBindingWithAnInvalidStoredSpecIsNotAcknowledgedAsDrained(t *testing.T) {
	invalid := drainDisabledBinding(4)
	// A legacy stored spec with an empty target scope: stored by an older API
	// server, rejected by the validation every runtime decision runs first.
	invalid.Spec.TargetScope = fathomv1alpha1.DefinitionBindingScope{}
	f := newDrainFixture(t, lifecycleDefinition(), invalid, lifecycleServiceAccount(), drainLeaseObject())

	f.reconcileOK()

	b := f.binding()
	requireCondition(t, b.Status.Conditions, definitionConditionAccepted, metav1.ConditionFalse, reasonInvalidDefinition)
	requireCondition(t, b.Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonInvalidDefinition)
	requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionFalse, reasonDrainUnverifiable)
	requireNotDrained(t, b)
	requireDrainClaimMatchesTheCLI(t, b)
	// The work still stops: the binding is disabled, and a refusal to claim a
	// drain is not a reason to keep admitting runs.
	if !f.session.revoked[lifecycleAddon] {
		t.Fatal("a disabled binding kept runtime admission open because its spec was invalid")
	}

	// Fixing the spec is the recovery path, and it resolves to a real drain.
	f.put(drainDisabledBinding(5))

	f.reconcileOK()

	b = f.binding()
	requireCondition(t, b.Status.Conditions, definitionConditionReady, metav1.ConditionFalse, reasonAuthorizationRevoked)
	requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionTrue, reasonDrainAcknowledged)
	requireDrainClaimMatchesTheCLI(t, b)
}

// "Re-enable/spec edit removes acknowledgement eligibility." The re-enable
// lands after this session has already granted the acknowledgement, in the
// window the final fence exists for: the first fence saw a disabled binding,
// and only the second read can catch what changed while the work unwound.
func TestDrainRefusedWhenTheBindingIsReEnabledAfterTheAcknowledgement(t *testing.T) {
	f := newDrainFixture(t, lifecycleDefinition(), drainDisabledBinding(4), lifecycleServiceAccount(), drainLeaseObject())
	f.session.afterAcknowledge = func() {
		// Same generation: only the live enabled flag rejects this, so the
		// generation fence cannot stand in for the enabled check.
		reenabled := drainDisabledBinding(4)
		reenabled.Spec.Enabled = true
		f.putLive(reenabled)
	}

	f.reconcileOK()

	b := f.binding()
	requireNotDrained(t, b)
	requireCondition(t, b.Status.Conditions, definitionConditionDrained, metav1.ConditionFalse, reasonDrainUnverifiable)
	requireDrainClaimMatchesTheCLI(t, b)
	if f.session.restores[lifecycleAddon] != 0 {
		t.Fatal("admission was reopened from a live enabled flag rather than from a revalidated authority")
	}
}

// --- the eligibility decision itself ----------------------------------------

// resolveBindingAuthority is the single place both runtime reconcilers decide
// whether a binding authorizes its definition, and the definition reconciler
// compiles and publishes a dispatchable snapshot on its "yes". Each check below
// is the only thing standing between one broken state and that publication.
func TestResolveBindingAuthorityRefusesAuthorityItCannotTrust(t *testing.T) {
	cfg := bindingAuthority{
		Namespace:      lifecycleNamespace,
		ManagerAccount: lifecycleManagerSA,
		Builtins:       func() []string { return nil },
	}

	for _, tc := range []struct {
		name string
		// mutate adjusts the objects before they are stored.
		mutate func(binding *fathomv1alpha1.AddonDefinitionBinding, sa *corev1.ServiceAccount)
		// terminate deletes whichever object must carry a deletion timestamp.
		terminate func(t *testing.T, c client.Client, binding *fathomv1alpha1.AddonDefinitionBinding, sa *corev1.ServiceAccount)
		// wantReason is empty when the row must authorize.
		wantReason string
	}{
		{
			name:   "authorized",
			mutate: func(*fathomv1alpha1.AddonDefinitionBinding, *corev1.ServiceAccount) {},
		},
		{
			// A binding under deletion is revoked authority: the administrator
			// has already asked for it to go away, and the object outliving
			// that request by a finalizer does not extend the grant.
			name: "binding held open by a finalizer",
			mutate: func(binding *fathomv1alpha1.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
				binding.Finalizers = []string{"test.fathom.skaphos.io/hold"}
			},
			terminate: func(t *testing.T, c client.Client, binding *fathomv1alpha1.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
				t.Helper()
				if err := c.Delete(context.Background(), binding.DeepCopy()); err != nil {
					t.Fatalf("delete binding: %v", err)
				}
			},
			wantReason: reasonAuthorizationRevoked,
		},
		{
			// A ServiceAccount being deleted is not a usable dedicated reader:
			// the identity a run would be bound to is on its way out, and its
			// name is about to become available to something else.
			name: "dedicated service account being deleted",
			mutate: func(_ *fathomv1alpha1.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
				sa.Finalizers = []string{"test.fathom.skaphos.io/hold"}
			},
			terminate: func(t *testing.T, c client.Client, _ *fathomv1alpha1.AddonDefinitionBinding, sa *corev1.ServiceAccount) {
				t.Helper()
				if err := c.Delete(context.Background(), sa.DeepCopy()); err != nil {
					t.Fatalf("delete service account: %v", err)
				}
			},
			wantReason: reasonBindingMismatch,
		},
		{
			// This is the definition reconciler's only semantic check of the
			// binding whose scope it is about to compile against. An empty
			// target scope would otherwise be compiled and published.
			name: "stored binding spec is invalid",
			mutate: func(binding *fathomv1alpha1.AddonDefinitionBinding, _ *corev1.ServiceAccount) {
				binding.Spec.TargetScope = fathomv1alpha1.DefinitionBindingScope{}
			},
			wantReason: reasonBindingMismatch,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := lifecycleDefinition()
			binding := lifecycleBinding()
			sa := lifecycleServiceAccount()
			tc.mutate(binding, sa)

			scheme := runtime.NewScheme()
			for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, fathomv1alpha1.AddToScheme} {
				if err := add(scheme); err != nil {
					t.Fatalf("scheme: %v", err)
				}
			}
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingDefinitionName, indexBindingByDefinitionName).
				WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingServiceAccountName, indexBindingByServiceAccountName).
				WithIndex(&fathomv1alpha1.AddonDefinitionBinding{}, IndexBindingServiceAccountUID, indexBindingByServiceAccountUID).
				WithObjects(def, binding, sa).
				Build()
			if tc.terminate != nil {
				tc.terminate(t, c, binding, sa)
			}

			got, err := resolveBindingAuthority(context.Background(), c, cfg, def)

			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("a well-formed binding must authorize its definition: %v", err)
				}
				if got == nil || got.Name != lifecycleAddon {
					t.Fatalf("resolved binding: %+v", got)
				}
				return
			}
			if got != nil {
				t.Fatalf("authority was granted by %q, which the definition reconciler compiles and publishes on", got.Name)
			}
			var failure *authorityFailure
			if !errors.As(err, &failure) {
				t.Fatalf("want an *authorityFailure carrying a lifecycle-matrix reason, got %v", err)
			}
			if failure.Reason != tc.wantReason {
				t.Fatalf("authority failure reason: want %q, got %q (%s)", tc.wantReason, failure.Reason, failure.Message)
			}
		})
	}
}

// --- envtest: what only a real API server can prove -------------------------

// The acknowledgement is only worth what the API server stores. A real server
// round-trips the microsecond-precision acquireTime, enforces the status
// schema's bounds, and is the uncached read the contract demands — none of
// which a fake client can prove.
var _ = Describe("AddonDefinitionBinding drain acknowledgement", Ordered, func() {
	const (
		namespace = "fathom-drain-envtest"
		addon     = "drain-envtest-addon"
		leaseName = "drain-envtest.skaphos.io"
		holder    = "fathom-operator-0_drain-envtest"
	)

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())

		holderID, duration, transitions := holder, int32(15), int32(2)
		acquire := metav1.NewMicroTime(time.Now().Add(-time.Minute))
		renew := metav1.NewMicroTime(time.Now())
		Expect(k8sClient.Create(ctx, &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: leaseName},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: &holderID, AcquireTime: &acquire, RenewTime: &renew,
				LeaseDurationSeconds: &duration, LeaseTransitions: &transitions,
			},
		})).To(Succeed())

		Expect(k8sClient.Create(ctx, &fathomv1alpha1.AddonDefinitionBinding{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: addon},
			Spec: fathomv1alpha1.AddonDefinitionBindingSpec{
				DefinitionRef:     fathomv1alpha1.DefinitionReference{Name: addon, UID: "definition-uid-drain-envtest"},
				ServiceAccountRef: fathomv1alpha1.DefinitionObjectReference{Name: addon + "-reader", UID: "service-account-uid-drain-envtest"},
				Enabled:           false,
				TargetScope:       fathomv1alpha1.DefinitionBindingScope{Namespaces: []fathomv1alpha1.DefinitionDNSLabel{"default"}},
			},
		})).To(Succeed())
	})

	AfterAll(func() {
		_ = k8sClient.Delete(ctx, &fathomv1alpha1.AddonDefinitionBinding{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: addon}})
		_ = k8sClient.Delete(ctx, &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: leaseName}})
	})

	It("publishes evidence the API server stores and the CLI can match against the live Lease", func() {
		var lease coordinationv1.Lease
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: leaseName}, &lease)).To(Succeed())
		epoch, err := leaderEpochFromLease(&lease)
		Expect(err).NotTo(HaveOccurred())

		session := newFakeSession()
		session.lease = types.NamespacedName{Namespace: namespace, Name: leaseName}
		session.epoch = epoch
		reconciler := &AddonDefinitionBindingReconciler{
			Client: k8sClient, APIReader: k8sClient, Scheme: k8sClient.Scheme(),
			OperatorNamespace:     namespace,
			ManagerServiceAccount: lifecycleManagerSA,
			CacheSynced:           func(context.Context) bool { return true },
			Leadership:            session,
		}

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: addon}})
		Expect(err).NotTo(HaveOccurred())

		var stored fathomv1alpha1.AddonDefinitionBinding
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: addon}, &stored)).To(Succeed())
		Expect(stored.Status.ObservedGeneration).To(Equal(stored.Generation))
		Expect(stored.Status.ActiveRuns).To(BeNumerically("==", 0))
		Expect(stored.Status.LeaderIdentity).To(Equal(holder))
		Expect(stored.Status.LeaderEpoch).NotTo(BeNil())
		// The CLI compares all four fields, acquireTime included: a stored
		// timestamp that lost precision would never match the live Lease again.
		Expect(stored.Status.LeaderEpoch.LeaseUID).To(Equal(epoch.LeaseUID))
		Expect(stored.Status.LeaderEpoch.HolderIdentity).To(Equal(epoch.HolderIdentity))
		Expect(stored.Status.LeaderEpoch.AcquireTime.Equal(&epoch.AcquireTime)).To(BeTrue(),
			"stored acquireTime %s does not equal the live Lease's %s", stored.Status.LeaderEpoch.AcquireTime, epoch.AcquireTime)
		Expect(stored.Status.LeaderEpoch.LeaseTransitions).To(Equal(epoch.LeaseTransitions))

		drained, ok := conditionByType(stored.Status.Conditions, definitionConditionDrained)
		Expect(ok).To(BeTrue())
		Expect(drained.Status).To(Equal(metav1.ConditionTrue))
		Expect(drained.Reason).To(Equal(reasonDrainAcknowledged))
		Expect(drained.ObservedGeneration).To(Equal(stored.Generation))
		ready, ok := conditionByType(stored.Status.Conditions, definitionConditionReady)
		Expect(ok).To(BeTrue())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(reasonAuthorizationRevoked))
		Expect(ready.ObservedGeneration).To(Equal(stored.Generation))
	})
})
