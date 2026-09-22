/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type drainReader struct {
	client.Reader
	lease            *coordinationv1.Lease
	binding          *api.AddonDefinitionBinding
	leases, bindings int
	// progressAt is the Lease read from which renewTime advances; zero means
	// the second read, which is the ordinary live-leader case.
	progressAt int
	mode       string
	t          *testing.T
}

func (r *drainReader) Get(ctx context.Context, key types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
	r.t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		r.t.Fatal("unbounded request")
	}
	if key.Namespace != "fathom" {
		r.t.Fatal("wrong namespace")
	}
	if r.mode == "denied" {
		return fmt.Errorf("denied")
	}
	switch target := obj.(type) {
	case *coordinationv1.Lease:
		r.leases++
		if key.Name != "leader" {
			r.t.Fatal("wrong lease")
		}
		*target = *r.lease.DeepCopy()
		// Every renewal writes the Lease, so resourceVersion churns on its own.
		// Verification must tolerate that and judge renewTime plus epoch only.
		target.ResourceVersion = fmt.Sprintf("%d", 1000+r.leases)
		progressAt := r.progressAt
		if progressAt == 0 {
			progressAt = 2
		}
		switch {
		case r.mode == "stalled":
		case r.mode == "regressed" && r.leases > 1:
			target.Spec.RenewTime = &metav1.MicroTime{Time: r.lease.Spec.RenewTime.Add(-time.Second)}
		case (r.mode == "transient regressed" || r.mode == "regressed transitioned") && r.leases == 2:
			target.Spec.RenewTime = &metav1.MicroTime{Time: r.lease.Spec.RenewTime.Add(-time.Second)}
		case r.leases >= progressAt:
			target.Spec.RenewTime = &metav1.MicroTime{Time: r.lease.Spec.RenewTime.Add(time.Second)}
		}
		if (r.mode == "handoff" && r.leases > 1) || (r.mode == "final handoff" && r.leases > 2) {
			holder := "other"
			target.Spec.HolderIdentity = &holder
		}
		// Epoch equality uses all four fields, so each of the remaining three
		// must be able to diverge on its own, both mid-poll and at the reserved
		// final read.
		if damage, ok := epochDamage[r.mode]; ok && r.leases > damage.after {
			damage.apply(target, r.lease)
		}
		// A final read whose renewTime slips behind the last polled one leaves
		// the epoch untouched: only the closing progression check can reject it.
		if r.mode == "final regressed" && r.leases > 2 {
			target.Spec.RenewTime = &metav1.MicroTime{Time: r.lease.Spec.RenewTime.Time}
		}
		if r.mode == "final denied" && r.leases > 2 {
			return fmt.Errorf("denied")
		}
	case *api.AddonDefinitionBinding:
		r.bindings++
		if r.mode == "binding denied" {
			return fmt.Errorf("denied")
		}
		*target = *r.binding.DeepCopy()
	default:
		r.t.Fatalf("unexpected read %T", obj)
	}
	return nil
}

// epochDamage diverges exactly one epoch field from the first observation,
// starting after read `after`: 1 exercises the polling fence, 2 the reserved
// final fence. renewTime keeps advancing throughout, so only the epoch
// comparison can reject these reads.
var epochDamage = map[string]struct {
	after int
	apply func(observed, first *coordinationv1.Lease)
}{
	"recreated":              {1, recreateLease},
	"final recreated":        {2, recreateLease},
	"reacquired":             {1, reacquireLease},
	"final reacquired":       {2, reacquireLease},
	"transitioned":           {1, transitionLease},
	"regressed transitioned": {1, transitionLease},
	"final transitioned":     {2, transitionLease},
}

// recreateLease models a deleted and recreated Lease: a brand new UID behind an
// otherwise identical holder.
func recreateLease(observed, _ *coordinationv1.Lease) { observed.UID = "recreated-lease-uid" }

// reacquireLease models the same process reacquiring the Lease: acquireTime
// moves while holderIdentity, UID and transitions can stay put.
func reacquireLease(observed, first *coordinationv1.Lease) {
	acquired := metav1.NewMicroTime(first.Spec.AcquireTime.Add(time.Second))
	observed.Spec.AcquireTime = &acquired
}

// transitionLease models a handoff away and back inside one observation window:
// only leaseTransitions records that it happened.
func transitionLease(observed, first *coordinationv1.Lease) {
	transitions := *first.Spec.LeaseTransitions + 1
	observed.Spec.LeaseTransitions = &transitions
}

// drainFixtures is a live Lease and the binding that acknowledged drain under
// that exact epoch: the only combination a verified result may be built from.
func drainFixtures(t *testing.T) (*coordinationv1.Lease, *api.AddonDefinitionBinding) {
	t.Helper()
	holder := "leader-process"
	duration := int32(15)
	transitions := int32(2)
	acquired := metav1.NewMicroTime(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
	renewed := metav1.NewMicroTime(acquired.Add(time.Minute))
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Namespace: "fathom", Name: "leader", UID: "lease-uid"},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: &holder, AcquireTime: &acquired, RenewTime: &renewed, LeaseDurationSeconds: &duration, LeaseTransitions: &transitions},
	}
	epoch, err := leaseEpoch(lease)
	if err != nil {
		t.Fatal(err)
	}
	binding := &api.AddonDefinitionBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: "fathom", Name: "custom", Generation: 3},
		Status: api.AddonDefinitionBindingStatus{ObservedGeneration: 3, LeaderIdentity: epoch.HolderIdentity, LeaderEpoch: epoch, Conditions: []api.DefinitionStatusCondition{
			{Type: "Drained", Status: metav1.ConditionTrue, ObservedGeneration: 3},
			{Type: "Ready", Status: metav1.ConditionFalse, Reason: "AuthorizationRevoked", ObservedGeneration: 3},
		}},
	}
	return lease, binding
}

func TestIndependentDrainVerification(t *testing.T) {
	for _, tc := range []struct {
		mode string
		code int
	}{
		{"healthy", 0}, {"denied", 2}, {"stalled", 2}, {"handoff", 2}, {"final handoff", 2}, {"enabled", 1}, {"active", 1}, {"stale generation", 1}, {"old epoch", 2}, {"missing condition", 1},
		// Added rows: every remaining shape of incomplete, stale or denied
		// evidence contracts/leadership.md enumerates for the CLI verifier.
		{"regressed", 2}, {"transient regressed", 0}, {"regressed transitioned", 2},
		{"binding denied", 2}, {"final denied", 2}, {"identity mismatch", 2}, {"missing epoch", 2},
		{"not drained", 1}, {"wrong reason", 1}, {"stale conditions", 1}, {"deleting", 1},
		// One row per epoch field the contract names, at both fences, plus a
		// final read that regresses without changing the epoch at all.
		{"recreated", 2}, {"final recreated", 2}, {"reacquired", 2}, {"final reacquired", 2},
		{"transitioned", 2}, {"final transitioned", 2}, {"final regressed", 2},
		{"stale observed generation", 1},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			lease, binding := drainFixtures(t)
			switch tc.mode {
			case "enabled":
				binding.Spec.Enabled = true
			case "active":
				binding.Status.ActiveRuns = 1
			case "stale generation":
				binding.Generation = 4
			case "old epoch":
				binding.Status.LeaderEpoch.LeaseUID = "old"
			case "missing condition":
				binding.Status.Conditions = nil
			case "identity mismatch":
				binding.Status.LeaderIdentity = "other"
			case "missing epoch":
				binding.Status.LeaderEpoch = nil
			case "not drained":
				binding.Status.Conditions[0].Status = metav1.ConditionFalse
			case "wrong reason":
				binding.Status.Conditions[1].Reason = "Draining"
			case "stale observed generation":
				// Conditions still carry the current generation; only the
				// status-wide observedGeneration lags, which leadership.md
				// step 3 requires as its own condition.
				binding.Status.ObservedGeneration = 2
			case "stale conditions":
				for i := range binding.Status.Conditions {
					binding.Status.Conditions[i].ObservedGeneration = 2
				}
			case "deleting":
				deleted := metav1.NewTime(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
				binding.DeletionTimestamp = &deleted
				binding.Finalizers = []string{"fathom.skaphos.io/test"}
			}
			reader := &drainReader{lease: lease, binding: binding, mode: tc.mode, t: t}
			epoch, err := verifyDefinitionDrain(context.Background(), reader, "fathom", "custom", "leader", func(context.Context) error { return nil })
			if ExitCode(err) != tc.code {
				t.Fatalf("code=%d want=%d err=%v", ExitCode(err), tc.code, err)
			}
			if reader.leases > 16 || reader.bindings > 1 {
				t.Fatalf("read cap exceeded: %+v", reader)
			}
			if tc.mode == "healthy" && (reader.leases != 3 || reader.bindings != 1) {
				t.Fatal("verification omitted independent reads")
			}
			if tc.mode == "healthy" && (epoch == nil || epoch.LeaseUID != "lease-uid" || epoch.HolderIdentity != "leader-process" || epoch.LeaseTransitions != 2) {
				t.Fatalf("observed epoch not returned for display: %+v", epoch)
			}
			if tc.mode == "stalled" && reader.leases != 15 {
				t.Fatal("did not reserve final read")
			}
			// A permanently regressed renewTime contributes no liveness proof
			// and exhausts the bounded polling window. A transient lower sample
			// is tolerated only after a later value strictly exceeds the original
			// baseline, while an epoch change on that lower sample still fails
			// immediately.
			if tc.mode == "regressed" && (reader.leases != definitions.MaxDrainLeaseReads-1 ||
				!strings.Contains(fmt.Sprint(err), "no progressing lease renewal")) {
				t.Fatalf("permanently regressed renewal: reads=%d err=%v", reader.leases, err)
			}
			if tc.mode == "transient regressed" && (reader.leases != 4 || reader.bindings != 1) {
				t.Fatalf("transient regression recovery: leases=%d bindings=%d, want 4 and 1", reader.leases, reader.bindings)
			}
			if tc.mode == "regressed transitioned" && (reader.leases != 2 || reader.bindings != 0) {
				t.Fatalf("epoch change on lower sample: leases=%d bindings=%d, want 2 and 0", reader.leases, reader.bindings)
			}
		})
	}
}

// TestDrainAcceptsLateRenewalWithinReservedReadBudget drives progress into the
// last pollable read so the reserved final Lease read is the sixteenth: the
// ceiling must still cover the closing fence rather than exhausting on polls.
func TestDrainAcceptsLateRenewalWithinReservedReadBudget(t *testing.T) {
	lease, binding := drainFixtures(t)
	reader := &drainReader{lease: lease, binding: binding, progressAt: definitions.MaxDrainLeaseReads - 1, t: t}
	epoch, err := verifyDefinitionDrain(context.Background(), reader, "fathom", "custom", "leader", func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("late but live renewal rejected: %v", err)
	}
	if epoch == nil {
		t.Fatal("no observed epoch")
	}
	if reader.leases != definitions.MaxDrainLeaseReads || reader.bindings != 1 {
		t.Fatalf("reads=%d bindings=%d want %d and 1", reader.leases, reader.bindings, definitions.MaxDrainLeaseReads)
	}
}

// TestDrainToleratesRenewalResourceVersionChurn pins the contract's explicit
// carve-out: a Lease resourceVersion moves on every renewal, so it is expected
// traffic, not evidence that leadership changed.
func TestDrainToleratesRenewalResourceVersionChurn(t *testing.T) {
	lease, binding := drainFixtures(t)
	reader := &drainReader{lease: lease, binding: binding, t: t}
	if _, err := verifyDefinitionDrain(context.Background(), reader, "fathom", "custom", "leader", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("renewal resourceVersion churn rejected: %v", err)
	}
	if reader.leases < 2 {
		t.Fatal("no second observation, so churn was never presented")
	}
}

// TestDrainRejectsIncompleteLeaseEvidence covers step 1 of the contract: a
// Lease that cannot prove live leadership is unverifiable, never "not drained".
func TestDrainRejectsIncompleteLeaseEvidence(t *testing.T) {
	deleted := metav1.NewTime(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
	empty := ""
	zeroMicro := metav1.MicroTime{}
	zeroDuration := int32(0)
	negative := int32(-1)
	for name, damage := range map[string]func(*coordinationv1.Lease){
		"no uid":                 func(l *coordinationv1.Lease) { l.UID = "" },
		"no holder":              func(l *coordinationv1.Lease) { l.Spec.HolderIdentity = nil },
		"empty holder":           func(l *coordinationv1.Lease) { l.Spec.HolderIdentity = &empty },
		"no acquire time":        func(l *coordinationv1.Lease) { l.Spec.AcquireTime = nil },
		"zero acquire time":      func(l *coordinationv1.Lease) { l.Spec.AcquireTime = &zeroMicro },
		"no renew time":          func(l *coordinationv1.Lease) { l.Spec.RenewTime = nil },
		"zero renew time":        func(l *coordinationv1.Lease) { l.Spec.RenewTime = &zeroMicro },
		"no duration":            func(l *coordinationv1.Lease) { l.Spec.LeaseDurationSeconds = nil },
		"non-positive duration":  func(l *coordinationv1.Lease) { l.Spec.LeaseDurationSeconds = &zeroDuration },
		"no transitions":         func(l *coordinationv1.Lease) { l.Spec.LeaseTransitions = nil },
		"negative transitions":   func(l *coordinationv1.Lease) { l.Spec.LeaseTransitions = &negative },
		"lease being terminated": func(l *coordinationv1.Lease) { l.DeletionTimestamp = &deleted },
	} {
		t.Run(name, func(t *testing.T) {
			lease, binding := drainFixtures(t)
			damage(lease)
			if _, err := leaseEpoch(lease); err == nil {
				t.Fatal("incomplete lease accepted as leadership evidence")
			}
			reader := &drainReader{lease: lease, binding: binding, t: t}
			if _, err := verifyDefinitionDrain(context.Background(), reader, "fathom", "custom", "leader", func(context.Context) error { return nil }); ExitCode(err) != 2 {
				t.Fatalf("code=%d want=2 err=%v", ExitCode(err), err)
			}
			if reader.bindings != 0 {
				t.Fatal("binding read on evidence that already failed closed")
			}
		})
	}
}

// TestDrainBoundsDeadlineAndPollBudget proves the outer 15s monotonic deadline
// covers every read and that a poll separates consecutive Lease reads.
func TestDrainBoundsDeadlineAndPollBudget(t *testing.T) {
	lease, binding := drainFixtures(t)
	reader := &drainReader{lease: lease, binding: binding, mode: "stalled", t: t}
	polls := 0
	_, err := verifyDefinitionDrain(context.Background(), reader, "fathom", "custom", "leader", func(ctx context.Context) error {
		polls++
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("verification ran without a deadline")
		}
		if remaining := time.Until(deadline); remaining > definitions.MaxDrainDuration {
			t.Fatalf("remaining budget %s exceeds the %s drain deadline", remaining, definitions.MaxDrainDuration)
		}
		return nil
	})
	if ExitCode(err) != 2 {
		t.Fatalf("code=%d want=2 err=%v", ExitCode(err), err)
	}
	if polls != reader.leases-1 {
		t.Fatalf("polls=%d reads=%d; each re-read must be paced by exactly one poll", polls, reader.leases)
	}
}

// TestDrainAbortsWhenTheDeadlineExpiresDuringAPoll proves a slow renewal ends
// as a conservative unverifiable result instead of running past the deadline.
func TestDrainAbortsWhenTheDeadlineExpiresDuringAPoll(t *testing.T) {
	lease, binding := drainFixtures(t)
	reader := &drainReader{lease: lease, binding: binding, mode: "stalled", t: t}
	_, err := verifyDefinitionDrain(context.Background(), reader, "fathom", "custom", "leader", func(ctx context.Context) error {
		return context.DeadlineExceeded
	})
	if ExitCode(err) != 2 {
		t.Fatalf("code=%d want=2 err=%v", ExitCode(err), err)
	}
	if reader.leases != 1 {
		t.Fatalf("reads=%d; an expired deadline must stop further reads", reader.leases)
	}
}

// TestDrainKeepsNoResultAcrossInvocations: the contract forbids a cached
// verdict surviving into another invocation.
func TestDrainKeepsNoResultAcrossInvocations(t *testing.T) {
	lease, binding := drainFixtures(t)
	stub := func(context.Context) error { return nil }
	if _, err := verifyDefinitionDrain(context.Background(), &drainReader{lease: lease, binding: binding, t: t}, "fathom", "custom", "leader", stub); err != nil {
		t.Fatalf("first verification: %v", err)
	}
	second := &drainReader{lease: lease, binding: binding, mode: "denied", t: t}
	if _, err := verifyDefinitionDrain(context.Background(), second, "fathom", "custom", "leader", stub); ExitCode(err) != 2 {
		t.Fatalf("code=%d want=2 err=%v; an earlier success was reused", ExitCode(err), err)
	}
}

// TestDrainPollPacesAtMostOnePerSecond exercises the production pacer: reads
// are at most one per second, and a cancelled deadline returns immediately.
func TestDrainPollPacesAtMostOnePerSecond(t *testing.T) {
	if definitions.DrainPollInterval < time.Second {
		t.Fatalf("poll interval %s polls the API server faster than once per second", definitions.DrainPollInterval)
	}
	start := time.Now()
	if err := waitDrainPoll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < definitions.DrainPollInterval {
		t.Fatalf("poll returned after %s, faster than the %s interval", elapsed, definitions.DrainPollInterval)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start = time.Now()
	if err := waitDrainPoll(ctx); err == nil {
		t.Fatal("poll ignored a cancelled deadline")
	}
	if elapsed := time.Since(start); elapsed >= definitions.DrainPollInterval {
		t.Fatalf("cancelled poll still slept %s", elapsed)
	}
}

// writeRecorder counts every mutating call reaching the API. `definition drain`
// verifies; it must never disable a binding or touch the Lease.
type writeRecorder struct {
	t      *testing.T
	writes []string
}

func (w *writeRecorder) record(verb string, obj client.Object) error {
	w.t.Helper()
	name := ""
	if obj != nil {
		name = obj.GetName()
	}
	w.writes = append(w.writes, verb+" "+name)
	return fmt.Errorf("%s is a write", verb)
}

// drainCommandFactory wires the real command onto a fake API whose Lease
// renews (and so churns resourceVersion) on every read, and whose every write
// verb is recorded and refused.
func drainCommandFactory(t *testing.T, objs ...client.Object) (*factory, *writeRecorder) {
	t.Helper()
	scheme, err := newScheme()
	if err != nil {
		t.Fatal(err)
	}
	recorder := &writeRecorder{t: t}
	reads := 0
	funcs := interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return err
			}
			lease, ok := obj.(*coordinationv1.Lease)
			if !ok {
				return nil
			}
			reads++
			renewed := metav1.NewMicroTime(lease.Spec.RenewTime.Add(time.Duration(reads) * time.Second))
			lease.Spec.RenewTime = &renewed
			lease.ResourceVersion = fmt.Sprintf("%d", 1000+reads)
			return nil
		},
		Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			return recorder.record("create", obj)
		},
		Update: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.UpdateOption) error {
			return recorder.record("update", obj)
		},
		Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
			return recorder.record("delete", obj)
		},
		DeleteAllOf: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteAllOfOption) error {
			return recorder.record("deleteallof", obj)
		},
		Patch: func(_ context.Context, _ client.WithWatch, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
			return recorder.record("patch", obj)
		},
		Apply: func(_ context.Context, _ client.WithWatch, _ runtime.ApplyConfiguration, _ ...client.ApplyOption) error {
			return recorder.record("apply", nil)
		},
		SubResourceCreate: func(_ context.Context, _ client.Client, name string, obj client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
			return recorder.record(name+" create", obj)
		},
		SubResourceUpdate: func(_ context.Context, _ client.Client, name string, obj client.Object, _ ...client.SubResourceUpdateOption) error {
			return recorder.record(name+" update", obj)
		},
		SubResourcePatch: func(_ context.Context, _ client.Client, name string, obj client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
			return recorder.record(name+" patch", obj)
		},
		SubResourceApply: func(_ context.Context, _ client.Client, name string, _ runtime.ApplyConfiguration, _ ...client.SubResourceApplyOption) error {
			return recorder.record(name+" apply", nil)
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
	f := newFactory()
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig {
		return stubClientConfig{cfg: &rest.Config{}, namespace: "default"}
	}
	f.newClient = func(*rest.Config, client.Options) (client.Client, error) { return fc, nil }
	return f, recorder
}

// TestDefinitionDrainCommandDisplaysObservedEpochAndNeverWrites runs the verb
// end to end against a live leader: exit 0, the observed epoch printed with its
// race caveat, and not one write issued.
func TestDefinitionDrainCommandDisplaysObservedEpochAndNeverWrites(t *testing.T) {
	lease, binding := drainFixtures(t)
	binding.Generation = 1
	binding.Status.ObservedGeneration = 1
	for i := range binding.Status.Conditions {
		binding.Status.Conditions[i].ObservedGeneration = 1
	}
	f, recorder := drainCommandFactory(t, lease, binding)
	out, _, err := execVerb(f, "definition", "drain", "--name", "custom", "--operator-namespace", "fathom", "--leader-election-id", "leader")
	if err != nil {
		t.Fatalf("exit=%d err=%v out=%q", ExitCode(err), err, out)
	}
	for _, want := range []string{"leaseUID=lease-uid", "holder=leader-process", "transitions=2", "leadership can change after verification"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q omits %q", out, want)
		}
	}
	// The acquisition instant is printed in the reader's own zone, so compare
	// instants rather than text.
	acquired, ok := fieldValue(out, "acquired=")
	if !ok {
		t.Fatalf("output %q omits the observed acquisition time", out)
	}
	shown, err := time.Parse(time.RFC3339Nano, acquired)
	if err != nil {
		t.Fatalf("acquisition time %q is not a timestamp: %v", acquired, err)
	}
	if !shown.Equal(lease.Spec.AcquireTime.Time) {
		t.Fatalf("displayed acquisition %s is not the observed %s", shown, lease.Spec.AcquireTime.Time)
	}
	if len(recorder.writes) != 0 {
		t.Fatalf("verification wrote to the cluster: %v", recorder.writes)
	}
}

// fieldValue pulls the value of a `key=value` token out of command output.
func fieldValue(out, key string) (string, bool) {
	for _, field := range strings.Fields(out) {
		if rest, found := strings.CutPrefix(field, key); found {
			return strings.TrimSuffix(rest, "."), true
		}
	}
	return "", false
}

// TestDefinitionDrainCommandExitCodes pins the documented codes end to end:
// 1 for an undrained binding, 2 when the configured Lease cannot be read.
func TestDefinitionDrainCommandExitCodes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		objects func(*coordinationv1.Lease, *api.AddonDefinitionBinding) []client.Object
		code    int
	}{
		{"not drained", func(l *coordinationv1.Lease, b *api.AddonDefinitionBinding) []client.Object {
			b.Spec.Enabled = true
			return []client.Object{l, b}
		}, 1},
		{"missing lease", func(_ *coordinationv1.Lease, b *api.AddonDefinitionBinding) []client.Object {
			return []client.Object{b}
		}, 2},
		{"missing binding", func(l *coordinationv1.Lease, _ *api.AddonDefinitionBinding) []client.Object {
			return []client.Object{l}
		}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lease, binding := drainFixtures(t)
			binding.Generation = 1
			binding.Status.ObservedGeneration = 1
			for i := range binding.Status.Conditions {
				binding.Status.Conditions[i].ObservedGeneration = 1
			}
			f, recorder := drainCommandFactory(t, tc.objects(lease, binding)...)
			_, _, err := execVerb(f, "definition", "drain", "--name", "custom", "--operator-namespace", "fathom", "--leader-election-id", "leader")
			if ExitCode(err) != tc.code {
				t.Fatalf("exit=%d want=%d err=%v", ExitCode(err), tc.code, err)
			}
			if len(recorder.writes) != 0 {
				t.Fatalf("verification wrote to the cluster: %v", recorder.writes)
			}
		})
	}
}

// TestDefinitionDrainCommandRequiresExplicitTarget: namespace and binding name
// are trusted operator configuration, never inferred from a kubeconfig context.
func TestDefinitionDrainCommandRequiresExplicitTarget(t *testing.T) {
	lease, binding := drainFixtures(t)
	for _, args := range [][]string{
		{"definition", "drain"},
		{"definition", "drain", "--name", "custom"},
		{"definition", "drain", "--operator-namespace", "fathom"},
	} {
		f, recorder := drainCommandFactory(t, lease, binding)
		// An incomplete target must be refused before any client is built, so
		// the verb can never fall back on a kubeconfig default namespace.
		contacted := false
		build := f.newClient
		f.newClient = func(cfg *rest.Config, o client.Options) (client.Client, error) {
			contacted = true
			return build(cfg, o)
		}
		_, _, err := execVerb(f, args[0], args[1:]...)
		if err == nil {
			t.Fatalf("%v accepted without an explicit target", args)
		}
		if contacted {
			t.Fatalf("%v reached the API server despite an incomplete target: %v", args, err)
		}
		if len(recorder.writes) != 0 {
			t.Fatalf("verification wrote to the cluster: %v", recorder.writes)
		}
	}
}

// TestSameEpochComparesEveryContractField pins leadership.md: epoch equality
// uses all four fields. A comparison that drops one silently accepts a
// different leader, so each field is diverged on its own here.
func TestSameEpochComparesEveryContractField(t *testing.T) {
	base := func() *api.DefinitionLeaderEpoch {
		return &api.DefinitionLeaderEpoch{LeaseUID: "lease-uid", HolderIdentity: "leader-process", AcquireTime: metav1.NewMicroTime(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)), LeaseTransitions: 2}
	}
	if !sameEpoch(base(), base()) {
		t.Fatal("identical epochs compared unequal")
	}
	for name, diverge := range map[string]func(*api.DefinitionLeaderEpoch){
		"leaseUID":       func(e *api.DefinitionLeaderEpoch) { e.LeaseUID = "recreated-lease-uid" },
		"holderIdentity": func(e *api.DefinitionLeaderEpoch) { e.HolderIdentity = "other-process" },
		"acquireTime": func(e *api.DefinitionLeaderEpoch) {
			e.AcquireTime = metav1.NewMicroTime(e.AcquireTime.Add(time.Microsecond))
		},
		"leaseTransitions": func(e *api.DefinitionLeaderEpoch) { e.LeaseTransitions++ },
	} {
		t.Run(name, func(t *testing.T) {
			other := base()
			diverge(other)
			if sameEpoch(base(), other) || sameEpoch(other, base()) {
				t.Fatalf("epochs differing only in %s compared equal", name)
			}
		})
	}
	if sameEpoch(nil, base()) || sameEpoch(base(), nil) || sameEpoch(nil, nil) {
		t.Fatal("a missing epoch compared equal to an observed one")
	}
}

// moduleRoot walks up from the test's working directory to the module root, so
// the source-level defaults below are located without hardcoding a path depth.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no module root above the test's working directory")
		}
		dir = parent
	}
}

// operatorDefaultLeaderElectionID reads the operator's own default out of
// internal/app/options.go. internal/cli must not import internal/app (AGENTS.md:
// it pulls in the controllers), so the two defaults cannot be bound at compile
// time from this side of the boundary; parsing the operator's declaration is
// the strongest binding available until the literal moves to a package both
// sides already import.
func operatorDefaultLeaderElectionID(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "internal", "app", "options.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("operator options are unreadable, so the CLI default is unpinned: %v", err)
	}
	found := ""
	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if key, ok := field.Key.(*ast.Ident); !ok || key.Name != "LeaderElectionID" {
			return true
		}
		literal, ok := field.Value.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			t.Fatalf("operator LeaderElectionID default is not a string literal in %s", path)
		}
		id, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		found = id
		return false
	})
	if found == "" {
		t.Fatalf("no LeaderElectionID default found in %s", path)
	}
	return found
}

// documentedLeaderElectionID pulls the default administrators are told to
// expect out of the configuration reference table.
func documentedLeaderElectionID(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "docs", "reference", "configuration.md")
	page, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(page), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 5 || strings.TrimSpace(cells[1]) != "`--leader-election-id`" {
			continue
		}
		return strings.Trim(strings.TrimSpace(cells[4]), "`")
	}
	t.Fatalf("%s documents no default for --leader-election-id", path)
	return ""
}

// TestDefinitionDrainCommandDefaultsToOperatorElectionID keeps the CLI default
// tied to the operator's configured LeaderElectionID (contracts/runtime.md:
// "Default leader election ID matches operator configuration") and to the
// published reference. The expected value is never spelled in this test: it is
// read from the operator source and from the docs, so changing either one alone
// fails here instead of shipping a CLI that verifies the wrong Lease.
func TestDefinitionDrainCommandDefaultsToOperatorElectionID(t *testing.T) {
	cmd := newDefinitionDrainCommand(newFactory())
	flag := cmd.Flags().Lookup("leader-election-id")
	if flag == nil {
		t.Fatal("no --leader-election-id flag")
	}
	if flag.DefValue == "" {
		t.Fatal("the drain verb defaults to no Lease name at all")
	}
	root := moduleRoot(t)
	if operator := operatorDefaultLeaderElectionID(t, root); flag.DefValue != operator {
		t.Fatalf("CLI default %q is not the operator default %q declared in internal/app/options.go", flag.DefValue, operator)
	}
	if documented := documentedLeaderElectionID(t, root); flag.DefValue != documented {
		t.Fatalf("CLI default %q is not the documented default %q", flag.DefValue, documented)
	}
	// The extractor has to track the operator source rather than echo a
	// constant, so a copy of options.go carrying a different default must read
	// back as different: that is exactly the divergence the comparison above
	// catches on the real tree.
	divergent := t.TempDir()
	options, err := os.ReadFile(filepath.Join(root, "internal", "app", "options.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(divergent, "internal", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	moved := strings.Replace(string(options), strconv.Quote(flag.DefValue), strconv.Quote("moved."+flag.DefValue), 1)
	if moved == string(options) {
		t.Fatal("operator options never spell the default the CLI advertises")
	}
	if err := os.WriteFile(filepath.Join(divergent, "internal", "app", "options.go"), []byte(moved), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := operatorDefaultLeaderElectionID(t, divergent); got != "moved."+flag.DefValue {
		t.Fatalf("operator default read as %q from a source declaring %q", got, "moved."+flag.DefValue)
	}
}

func TestDrainEpochSurvivesSubsecondWireRoundTrip(t *testing.T) {
	holder := "holder"
	duration, transitions := int32(15), int32(1)
	instant := metav1.NewMicroTime(time.Date(2040, 1, 1, 0, 0, 0, 123456000, time.UTC))
	lease := &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{UID: "uid"}, Spec: coordinationv1.LeaseSpec{HolderIdentity: &holder, AcquireTime: &instant, RenewTime: &instant, LeaseDurationSeconds: &duration, LeaseTransitions: &transitions}}
	epoch, err := leaseEpoch(lease)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(epoch)
	if err != nil {
		t.Fatal(err)
	}
	var decoded api.DefinitionLeaderEpoch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !sameEpoch(epoch, &decoded) {
		t.Fatalf("persisted epoch lost acquisition precision: %s", data)
	}
}
