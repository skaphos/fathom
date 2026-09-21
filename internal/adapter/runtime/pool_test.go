/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	execution "github.com/skaphos/fathom/internal/adapter/runtime"
)

func TestRuntimePoolStopsAndReleasesAllWorkers(t *testing.T) {
	s := execution.NewScheduler(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan execution.Work, 8)
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, func(ctx context.Context, w execution.Work) execution.Disposition {
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 30*time.Second {
				t.Error("worker lacks outer runtime deadline")
			}
			entered <- w
			<-ctx.Done()
			return execution.Retry
		})
	}()
	for i := 0; i < 8; i++ {
		enqueue(t, s, work(fmt.Sprint("def-", i), fmt.Sprint("check-", i)), time.Now())
	}
	for i := 0; i < 4; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("worker pool did not fill available slots")
		}
	}
	select {
	case <-entered:
		t.Fatal("pool exceeded four workers")
	default:
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pool abandoned canceled worker")
	}
	// Active slots must be released even though canceled attempts retained retries.
	for i := 0; i < 4; i++ {
		_ = admit(t, s, time.Now().Add(2*time.Minute))
	}
}

func TestRuntimePoolRecoversHandlerPanicAndPreservesPeer(t *testing.T) {
	s := execution.NewScheduler(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Now()
	enqueue(t, s, work("bad", "bad"), now)
	enqueue(t, s, work("good", "good"), now)
	healthy := make(chan struct{})
	panicked := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, func(_ context.Context, w execution.Work) execution.Disposition {
			if w.Definition == "bad" {
				close(panicked)
				panic("unexpected setup panic")
			}
			close(healthy)
			return execution.Completed
		})
	}()
	select {
	case <-panicked:
	case <-time.After(time.Second):
		t.Fatal("bad work did not execute")
	}
	select {
	case <-healthy:
	case <-time.After(time.Second):
		t.Fatal("panic stalled peer")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pool did not stop")
	}
}
