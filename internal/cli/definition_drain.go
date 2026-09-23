/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"fmt"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	"github.com/spf13/cobra"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newDefinitionDrainCommand(f *factory) *cobra.Command {
	var name, namespace, lease string
	cmd := &cobra.Command{Use: "drain", Short: "Independently verify a disabled binding is drained; never disable it", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := f.client()
		if err != nil {
			return &commandError{2, err}
		}
		epoch, err := verifyDefinitionDrain(commandContext(cmd), c, namespace, name, lease, waitDrainPoll)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Verified drained: %s/%s leaseUID=%s holder=%s acquired=%s transitions=%d. Observation only; leadership can change after verification.\n", namespace, name, epoch.LeaseUID, epoch.HolderIdentity, epoch.AcquireTime.Format(time.RFC3339Nano), epoch.LeaseTransitions)
		return err
	}}
	cmd.Flags().StringVar(&name, "name", "", "Disabled binding name")
	cmd.Flags().StringVar(&namespace, "operator-namespace", "", "Operator namespace")
	cmd.Flags().StringVar(&lease, "leader-election-id", "2d3dbc4f.skaphos.io", "Configured operator leader Lease name")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("operator-namespace")
	return cmd
}

func waitDrainPoll(ctx context.Context) error {
	timer := time.NewTimer(definitions.DrainPollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func leaseEpoch(lease *coordinationv1.Lease) (*api.DefinitionLeaderEpoch, error) {
	s := lease.Spec
	if lease.UID == "" || s.HolderIdentity == nil || *s.HolderIdentity == "" || s.AcquireTime == nil || s.AcquireTime.IsZero() || s.RenewTime == nil || s.RenewTime.IsZero() || s.LeaseTransitions == nil || *s.LeaseTransitions < 0 || s.LeaseDurationSeconds == nil || *s.LeaseDurationSeconds <= 0 || !lease.DeletionTimestamp.IsZero() {
		return nil, fmt.Errorf("lease lacks live leadership evidence")
	}
	return &api.DefinitionLeaderEpoch{LeaseUID: string(lease.UID), HolderIdentity: *s.HolderIdentity, AcquireTime: *s.AcquireTime, LeaseTransitions: *s.LeaseTransitions}, nil
}

func sameEpoch(a, b *api.DefinitionLeaderEpoch) bool {
	return a != nil && b != nil && a.LeaseUID == b.LeaseUID && a.HolderIdentity == b.HolderIdentity && a.AcquireTime.Equal(&b.AcquireTime) && a.LeaseTransitions == b.LeaseTransitions
}

func verifyDefinitionDrain(ctx context.Context, reader client.Reader, namespace, name, leaseName string, wait func(context.Context) error) (*api.DefinitionLeaderEpoch, error) {
	ctx, cancel := context.WithTimeout(ctx, definitions.MaxDrainDuration)
	defer cancel()
	get := func(key types.NamespacedName, obj client.Object) error {
		req, cancel := context.WithTimeout(ctx, definitions.MaxRequestDuration)
		defer cancel()
		return reader.Get(req, key, obj)
	}
	readLease := func() (*coordinationv1.Lease, *api.DefinitionLeaderEpoch, error) {
		var lease coordinationv1.Lease
		if err := get(types.NamespacedName{Namespace: namespace, Name: leaseName}, &lease); err != nil {
			return nil, nil, err
		}
		epoch, err := leaseEpoch(&lease)
		return &lease, epoch, err
	}
	first, epoch, err := readLease()
	if err != nil {
		return nil, &commandError{2, err}
	}
	progressed := false
	var last *coordinationv1.Lease
	// Initial + at most fourteen renewal reads + one reserved final read = 16.
	for reads := 1; reads < definitions.MaxDrainLeaseReads-1; reads++ {
		if err := wait(ctx); err != nil {
			return nil, &commandError{2, err}
		}
		var observed *api.DefinitionLeaderEpoch
		last, observed, err = readLease()
		if err != nil {
			return nil, &commandError{2, err}
		}
		if !sameEpoch(epoch, observed) {
			return nil, &commandError{2, fmt.Errorf("leadership changed during verification")}
		}
		// renewTime is a writer-supplied wall-clock value. Even direct,
		// resource-version-ordered Lease reads can briefly observe it move
		// backwards within one stable epoch. A lower or equal sample proves
		// nothing, so keep polling from the original high-water baseline; only
		// a later strict advance can establish live renewal.
		if last.Spec.RenewTime.After(first.Spec.RenewTime.Time) {
			progressed = true
			break
		}
	}
	if !progressed {
		return nil, &commandError{2, fmt.Errorf("no progressing lease renewal observed")}
	}
	var binding api.AddonDefinitionBinding
	if err := get(types.NamespacedName{Namespace: namespace, Name: name}, &binding); err != nil {
		return nil, &commandError{2, err}
	}
	final, observed, err := readLease()
	if err != nil {
		return nil, &commandError{2, err}
	}
	if !sameEpoch(epoch, observed) || final.Spec.RenewTime.Before(last.Spec.RenewTime) {
		return nil, &commandError{2, fmt.Errorf("leadership changed after binding read")}
	}
	if !sameEpoch(epoch, binding.Status.LeaderEpoch) || binding.Status.LeaderIdentity != epoch.HolderIdentity {
		return nil, &commandError{2, fmt.Errorf("binding has no acknowledgement for the observed leader epoch")}
	}
	drained, revoked := false, false
	for _, condition := range binding.Status.Conditions {
		if condition.ObservedGeneration != binding.Generation {
			continue
		}
		if condition.Type == "Drained" && condition.Status == metav1.ConditionTrue {
			drained = true
		}
		if condition.Type == "Ready" && condition.Status == metav1.ConditionFalse && condition.Reason == "AuthorizationRevoked" {
			revoked = true
		}
	}
	if binding.Spec.Enabled || binding.Status.ActiveRuns != 0 || binding.Status.ObservedGeneration != binding.Generation || !binding.DeletionTimestamp.IsZero() || !drained || !revoked {
		return nil, &commandError{1, fmt.Errorf("binding is not acknowledged drained at its current generation")}
	}
	return epoch, nil
}
