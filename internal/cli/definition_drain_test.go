/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type drainReader struct {
	client.Reader
	lease            *coordinationv1.Lease
	binding          *api.AddonDefinitionBinding
	leases, bindings int
	mode             string
	t                *testing.T
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
		if r.mode != "stalled" && r.leases > 1 {
			target.Spec.RenewTime = &metav1.MicroTime{Time: r.lease.Spec.RenewTime.Add(time.Second)}
		}
		if (r.mode == "handoff" && r.leases > 1) || (r.mode == "final handoff" && r.leases > 2) {
			holder := "other"
			target.Spec.HolderIdentity = &holder
		}
	case *api.AddonDefinitionBinding:
		r.bindings++
		*target = *r.binding.DeepCopy()
	default:
		r.t.Fatalf("unexpected read %T", obj)
	}
	return nil
}

func TestIndependentDrainVerification(t *testing.T) {
	for _, tc := range []struct {
		mode string
		code int
	}{
		{"healthy", 0}, {"denied", 2}, {"stalled", 2}, {"handoff", 2}, {"final handoff", 2}, {"enabled", 1}, {"active", 1}, {"stale generation", 1}, {"old epoch", 2}, {"missing condition", 1},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			holder := "leader-process"
			duration := int32(15)
			transitions := int32(2)
			acquired := metav1.NewMicroTime(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
			renewed := metav1.NewMicroTime(acquired.Add(time.Minute))
			lease := &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{UID: "lease-uid"}, Spec: coordinationv1.LeaseSpec{HolderIdentity: &holder, AcquireTime: &acquired, RenewTime: &renewed, LeaseDurationSeconds: &duration, LeaseTransitions: &transitions}}
			epoch, err := leaseEpoch(lease)
			if err != nil {
				t.Fatal(err)
			}
			binding := &api.AddonDefinitionBinding{ObjectMeta: metav1.ObjectMeta{Generation: 3}, Status: api.AddonDefinitionBindingStatus{ObservedGeneration: 3, LeaderIdentity: holder, LeaderEpoch: epoch, Conditions: []api.DefinitionStatusCondition{
				{Type: "Drained", Status: metav1.ConditionTrue, ObservedGeneration: 3}, {Type: "Ready", Status: metav1.ConditionFalse, Reason: "AuthorizationRevoked", ObservedGeneration: 3},
			}}}
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
			}
			reader := &drainReader{lease: lease, binding: binding, mode: tc.mode, t: t}
			_, err = verifyDefinitionDrain(context.Background(), reader, "fathom", "custom", "leader", func(context.Context) error { return nil })
			if ExitCode(err) != tc.code {
				t.Fatalf("code=%d want=%d err=%v", ExitCode(err), tc.code, err)
			}
			if reader.leases > 16 || reader.bindings > 1 {
				t.Fatalf("read cap exceeded: %+v", reader)
			}
			if tc.mode == "healthy" && (reader.leases != 3 || reader.bindings != 1) {
				t.Fatal("verification omitted independent reads")
			}
			if tc.mode == "stalled" && reader.leases != 15 {
				t.Fatal("did not reserve final read")
			}
		})
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
