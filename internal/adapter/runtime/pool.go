/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Run starts exactly four runtime workers and joins every worker on cancellation.
// Callers construct one scheduler for the manager and start it only within the
// elected leadership session. The handler performs uncached pre-validation,
// supervised execution, and final publication fences before returning. It must
// honor the passed context; Run never abandons a handler or releases its slot early.
func (s *Scheduler) Run(ctx context.Context, handle func(context.Context, Work) Disposition) error {
	if handle == nil {
		return fmt.Errorf("runtime worker handler is required")
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("runtime workers already running")
	}
	s.running = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()
	var workers sync.WaitGroup
	for i := 0; i < limits.MaxConcurrentRuns; i++ {
		workers.Go(func() { s.worker(ctx, handle) })
	}
	workers.Wait()
	return nil
}
func (s *Scheduler) worker(ctx context.Context, handle func(context.Context, Work) Disposition) {
	for ctx.Err() == nil {
		admission, delay := s.Next(time.Now())
		if admission != nil {
			s.execute(ctx, admission, handle)
			continue
		}
		var timer *time.Timer
		var tick <-chan time.Time
		if delay >= 0 {
			timer = time.NewTimer(delay)
			tick = timer.C
		}
		select {
		case <-ctx.Done():
		case <-s.Wake():
		case <-tick:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}
func (s *Scheduler) execute(ctx context.Context, admission *Admission, handle func(context.Context, Work) Disposition) {
	outcome := Retry
	defer func() {
		if recovered := recover(); recovered != nil {
			// Panic values can contain untrusted target payloads; never stringify them.
			log.FromContext(ctx).Error(fmt.Errorf("runtime worker recovered a panic"), "runtime attempt failed", "definition", admission.work.Definition, "check", admission.work.Check)
		}
		admission.Finish(outcome, time.Now())
	}()
	child, cancel := context.WithTimeout(ctx, limits.MaxRunDuration)
	defer cancel()
	if child.Err() != nil {
		return
	}
	outcome = handle(child, admission.Work())
}
