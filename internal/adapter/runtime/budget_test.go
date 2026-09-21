/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

func TestRunBudgetBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		charge func(*execution.Budget, int) error
	}{
		{"requests", limits.MaxRunRequests, func(b *execution.Budget, n int) error {
			for i := 0; i < n; i++ {
				if err := b.ChargeRequest(); err != nil {
					return err
				}
			}
			return nil
		}},
		{"objects", limits.MaxRunObjects, func(b *execution.Budget, n int) error { return b.ChargeObjects(n) }},
		{"visits", limits.MaxObjectVisits, func(b *execution.Budget, n int) error { return b.Visit(n) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			defer close()
			if err := tc.charge(b, tc.limit); err != nil {
				t.Fatal(err)
			}
			if err := tc.charge(b, 1); err == nil {
				t.Fatal("over-limit work accepted")
			}
			if b.Err() == nil {
				t.Fatal("failure did not cancel run")
			}
		})
	}
	b, close := execution.NewBudget(context.Background(), time.Minute)
	defer close()
	for i := 0; i < 8; i++ {
		if err := b.ChargeResponse(limits.MaxResponseBytes); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.ChargeResponse(1); err == nil {
		t.Fatal("cumulative response byte cap bypassed")
	}
}

func TestBudgetDeadlineWinsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	b, close := execution.NewBudget(ctx, time.Minute)
	defer close()
	if err := b.ChargeResponse(limits.MaxResponseBytes + 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline masked by response failure: %v", err)
	}
}

func TestBudgetDeadlineClamps(t *testing.T) {
	for _, timeout := range []time.Duration{time.Second, time.Minute} {
		start := time.Now()
		b, close := execution.NewBudget(context.Background(), timeout)
		deadline, ok := b.Context().Deadline()
		if !ok || deadline.Sub(start) > min(timeout, limits.MaxRunDuration)+10*time.Millisecond {
			t.Fatal("unbounded run")
		}
		close()
	}
}
