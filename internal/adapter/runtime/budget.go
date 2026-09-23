/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package runtime supplies bounded execution primitives for declarative runtime
// definitions. These primitives do not register or activate runtime loading.
package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	limits "github.com/skaphos/fathom/pkg/addondefinition"
)

// Failure is a bounded execution failure reason suitable for attempt status.
type Failure struct{ Reason, Detail string }

func (e *Failure) Error() string { return e.Reason + ": " + e.Detail }

// Budget shares cancellation and counters across every operation in one run,
// including control-plane fences, delegated discovery, reads, and retries.
type Budget struct {
	ctx                              context.Context
	cancel                           context.CancelCauseFunc
	mu                               sync.Mutex
	requests, objects, visits, bytes int
	paginationRestarts               int
}

// NewBudget applies the caller timeout and the absolute 30-second runtime cap.
func NewBudget(parent context.Context, timeout time.Duration) (*Budget, func()) {
	if timeout <= 0 || timeout > limits.MaxRunDuration {
		timeout = limits.MaxRunDuration
	}
	deadline, stop := context.WithTimeout(parent, timeout)
	ctx, cancel := context.WithCancelCause(deadline)
	return &Budget{ctx: ctx, cancel: cancel}, func() { cancel(context.Canceled); stop() }
}
func (b *Budget) Context() context.Context { return b.ctx }
func (b *Budget) Err() error               { return context.Cause(b.ctx) }

// Fail records the first failure and cancels all child requests. A deadline
// already observed by the run cannot be overwritten by a later parser/size error.
func (b *Budget) Fail(reason, detail string) error {
	if err := b.Err(); err != nil {
		return err
	}
	b.cancel(&Failure{Reason: reason, Detail: detail})
	return b.Err()
}
func (b *Budget) charge(counter *int, n, maximum int, reason, label string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.Err(); err != nil {
		return err
	}
	if n < 0 || n > maximum-*counter {
		return b.Fail(reason, fmt.Sprintf("%s limit %d exceeded", label, maximum))
	}
	*counter += n
	return nil
}
func (b *Budget) ChargeRequest() error {
	return b.charge(&b.requests, 1, limits.MaxRunRequests, "WorkLimitExceeded", "API request")
}
func (b *Budget) ChargeObjects(n int) error {
	return b.charge(&b.objects, n, limits.MaxRunObjects, "WorkLimitExceeded", "object")
}
func (b *Budget) Visit(n int) error {
	return b.charge(&b.visits, n, limits.MaxObjectVisits, "WorkLimitExceeded", "traversal")
}
func (b *Budget) ChargeResponse(n int) error {
	if err := b.Err(); err != nil {
		return err
	}
	if n < 0 || n > limits.MaxResponseBytes {
		return b.Fail("ResponseLimitExceeded", "decoded response exceeds per-request cap")
	}
	return b.charge(&b.bytes, n, limits.MaxRunResponseBytes, "ResponseLimitExceeded", "cumulative decoded bytes")
}
func (b *Budget) Objects() int { b.mu.Lock(); defer b.mu.Unlock(); return b.objects }

// RequestContext retains caller values/cancellation and adds both the shared run
// deadline and the per-request cap. Run revocation also cancels this context.
func (b *Budget) RequestContext(parent context.Context) (context.Context, func()) {
	deadline, _ := b.ctx.Deadline()
	deadline = minTime(deadline, time.Now().Add(limits.MaxRequestDuration))
	ctx, cancel := context.WithDeadline(parent, deadline)
	stop := context.AfterFunc(b.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// Abort cancels a run with a concrete execution or context failure.
func (b *Budget) Abort(err error) error { b.cancel(err); return b.Err() }

// ChargePaginationRestart shares the restart allowance across all lists in a run.
func (b *Budget) ChargePaginationRestart() error {
	return b.charge(&b.paginationRestarts, 1, limits.MaxPaginationRestarts, "WorkLimitExceeded", "pagination restart")
}
