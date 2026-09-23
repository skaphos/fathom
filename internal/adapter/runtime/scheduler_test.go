/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/types"
)

func work(def, check string) execution.Work {
	return execution.Work{Definition: def, Check: types.NamespacedName{Namespace: "checks", Name: check}}
}
func enqueue(t *testing.T, s *execution.Scheduler, w execution.Work, now time.Time) {
	t.Helper()
	if err := s.Enqueue(w, now); err != nil {
		t.Fatal(err)
	}
}
func admit(t *testing.T, s *execution.Scheduler, now time.Time) *execution.Admission {
	t.Helper()
	a, _ := s.Next(now)
	if a == nil {
		t.Fatal("expected admission")
	}
	return a
}

func TestSchedulerLimitsAndDefinitionFairness(t *testing.T) {
	s := execution.NewScheduler(func() float64 { return 0 })
	now := time.Now()
	for i := 0; i < 10; i++ {
		enqueue(t, s, work("noisy", fmt.Sprint("check-", i)), now)
	}
	for i := 0; i < 4; i++ {
		enqueue(t, s, work(fmt.Sprint("peer-", i), fmt.Sprint("peer-", i)), now)
	}
	first := admit(t, s, now)
	if first.Work().Definition != "noisy" {
		t.Fatal("FIFO definition admission lost")
	}
	active := []*execution.Admission{first}
	for i := 0; i < 3; i++ {
		a := admit(t, s, now)
		active = append(active, a)
		if a.Work().Definition == "noisy" {
			t.Fatal("two runs for one definition")
		}
	}
	if a, _ := s.Next(now); a != nil {
		t.Fatal("fifth concurrent run admitted")
	}
	active[1].Finish(execution.Completed, now)
	next := admit(t, s, now)
	if next.Work().Definition != "peer-3" {
		t.Fatalf("peer starved by noisy definition: %+v", next.Work())
	}
	first.Finish(execution.Completed, now)
	first.Finish(execution.Completed, now) // stale/double completion must not free another slot.
	a := admit(t, s, now)
	if a.Work().Definition != "noisy" {
		t.Fatal("definition did not resume")
	}
	if extra, _ := s.Next(now); extra != nil {
		t.Fatal("double completion released another slot")
	}
}

func TestSchedulerCoalescesChurnAndKeepsCheckSlot(t *testing.T) {
	s := execution.NewScheduler(func() float64 { return 0 })
	now := time.Now()
	w := work("original", "shared")
	enqueue(t, s, w, now)
	active := admit(t, s, now)
	w.Definition = "replacement"
	for i := 0; i < 1000; i++ {
		enqueue(t, s, w, now)
	}
	if a, _ := s.Next(now); a != nil {
		t.Fatal("retargeting bypassed check slot")
	}
	active.Finish(execution.Retry, now)
	if a, delay := s.Next(now); a != nil || delay != 5*time.Second {
		t.Fatalf("churn bypassed backoff: admission=%v delay=%v", a, delay)
	}
	replacement := admit(t, s, now.Add(5*time.Second))
	if replacement.Work() != w {
		t.Fatal("did not retain latest work")
	}
	replacement.Finish(execution.Completed, now.Add(5*time.Second))
	if a, _ := s.Next(now.Add(time.Hour)); a != nil {
		t.Fatal("duplicate queued wakes survived completion")
	}
}

func TestSchedulerBackoffAndMissingInputs(t *testing.T) {
	s := execution.NewScheduler(func() float64 { return 0 })
	now := time.Now()
	w := work("custom", "check")
	enqueue(t, s, w, now)
	for _, delay := range []time.Duration{5, 10, 20, 40, 60, 60} {
		a := admit(t, s, now)
		a.Finish(execution.Retry, now)
		enqueue(t, s, w, now) // informer updates cannot accelerate failure retries.
		if next, wait := s.Next(now); next != nil || wait != delay*time.Second {
			t.Fatalf("want %s got %v/%s", delay*time.Second, next, wait)
		}
		now = now.Add(delay * time.Second)
	}
	a := admit(t, s, now)
	a.Finish(execution.MissingInput, now)
	if next, wait := s.Next(now); next != nil || wait != time.Minute {
		t.Fatalf("missing poll=%s", wait)
	}
}

func TestSchedulerJitterEventsAndDeleteRecreate(t *testing.T) {
	s := execution.NewScheduler(func() float64 { return 1 })
	now := time.Now()
	w := work("custom", "check")
	enqueue(t, s, w, now)
	a := admit(t, s, now)
	if !s.ShouldReport(w.Check, "AccessDenied", now) || s.ShouldReport(w.Check, "AccessDenied", now.Add(time.Minute)) {
		t.Fatal("unchanged failure event not deduplicated")
	}
	if s.ShouldReport(w.Check, "Timeout", now.Add(time.Second)) {
		t.Fatal("event rate unbounded")
	}
	a.Finish(execution.Retry, now)
	s.Forget(w.Check, now)
	enqueue(t, s, w, now)
	// The event cooldown expires at five seconds, before the jittered retry.
	if next, wait := s.Next(now); next != nil || wait != 5*time.Second {
		t.Fatalf("maintenance wake=%v/%s", next, wait)
	}
	if next, wait := s.Next(now.Add(5 * time.Second)); next != nil || wait != time.Second {
		t.Fatalf("recreation bypassed jitter/backoff: %v %s", next, wait)
	}
	a = admit(t, s, now.Add(6*time.Second))
	s.Forget(w.Check, now)
	enqueue(t, s, work("changed", "check"), now)
	if next, _ := s.Next(now); next != nil {
		t.Fatal("recreation bypassed active check slot")
	}
	a.Finish(execution.Completed, now.Add(6*time.Second))
	_ = admit(t, s, now.Add(6*time.Second))
}

func TestSchedulerConcurrentAdmissionAndWakeCoalescing(t *testing.T) {
	s := execution.NewScheduler(func() float64 { return 0 })
	now := time.Now()
	for i := 0; i < 1000; i++ {
		enqueue(t, s, work("same", "same"), now)
	}
	<-s.Wake()
	select {
	case <-s.Wake():
		t.Fatal("duplicate notification queued")
	default:
	}
	for i := 0; i < 20; i++ {
		enqueue(t, s, work(fmt.Sprint("def-", i), fmt.Sprint("check-", i)), now)
	}
	admissions := make(chan *execution.Admission, 24)
	var workers sync.WaitGroup
	for i := 0; i < 24; i++ {
		workers.Go(func() {
			if a, _ := s.Next(now); a != nil {
				admissions <- a
			}
		})
	}
	workers.Wait()
	close(admissions)
	count := 0
	for a := range admissions {
		count++
		workers.Go(func() { a.Finish(execution.Completed, now); a.Finish(execution.Completed, now) })
	}
	workers.Wait()
	if count != 4 {
		t.Fatalf("concurrent admissions=%d", count)
	}
	for i := 0; i < 4; i++ {
		_ = admit(t, s, now)
	}
	if extra, _ := s.Next(now); extra != nil {
		t.Fatal("completion corrupted process slots")
	}
}

func TestSchedulerJitterCeilingAndForgottenRetryExpiry(t *testing.T) {
	s := execution.NewScheduler(func() float64 { return 1 })
	now := time.Now()
	w := work("custom", "check")
	enqueue(t, s, w, now)
	for _, seconds := range []time.Duration{6, 12, 24, 48, 60, 60} {
		admit(t, s, now).Finish(execution.Retry, now)
		if a, delay := s.Next(now); a != nil || delay != seconds*time.Second {
			t.Fatalf("jitter delay=%s expected=%s", delay, seconds*time.Second)
		}
		now = now.Add(seconds * time.Second)
	}
	s.Forget(w.Check, now)
	if a, _ := s.Next(now); a != nil {
		t.Fatal("deleted check still queued")
	}
	enqueue(t, s, w, now)
	admit(t, s, now).Finish(execution.Retry, now)
	if _, delay := s.Next(now); delay != 6*time.Second {
		t.Fatalf("expired tombstone retained old retry count: %s", delay)
	}
}

func TestSchedulerEventRateSurvivesSuccessfulRunAndRecreation(t *testing.T) {
	s := execution.NewScheduler(nil)
	now := time.Now()
	w := work("custom", "check")
	enqueue(t, s, w, now)
	if !s.ShouldReport(w.Check, "AccessDenied", now) {
		t.Fatal("initial event suppressed")
	}
	admit(t, s, now).Finish(execution.Completed, now)
	enqueue(t, s, w, now.Add(time.Second))
	if s.ShouldReport(w.Check, "AccessDenied", now.Add(time.Second)) {
		t.Fatal("completion/recreation bypassed event rate")
	}
	if !s.ShouldReport(w.Check, "AccessDenied", now.Add(5*time.Second)) {
		t.Fatal("changed failure remained suppressed after cooldown")
	}
}

// The scheduling row caps a single check at one in-flight run and one queued
// wake no matter how many events arrive or how often the check is retargeted.
// Both are enforced structurally (one active slot, one queue node per check), so
// the observed counts are asserted against the constants that state the contract:
// a retune the scheduler cannot honour fails here instead of passing silently.
//
// Churn is delivered in two windows. The first only exercises coalescing into a
// single queue node. The second arrives while the admitted run is still held and
// retargets the check onto a definition with no active run, so the entry sits at
// the head of a ring queue with its own admission outstanding — the only state in
// which a second concurrent run for one check could be handed out, and therefore
// the state the run cap has to be asserted against.
func TestSchedulerPerCheckRunAndWakeCaps(t *testing.T) {
	s := execution.NewScheduler(func() float64 { return 0 })
	now := time.Now()
	// Every drain is bounded by the global concurrency cap: at a fixed instant the
	// scheduler can never hold more admissions than that. A scheduler that keeps
	// handing out work therefore fails an assertion instead of spinning forever.
	drain := func() []*execution.Admission {
		t.Helper()
		var admitted []*execution.Admission
		for len(admitted) <= limits.MaxConcurrentRuns {
			a, _ := s.Next(now)
			if a == nil {
				return admitted
			}
			admitted = append(admitted, a)
		}
		t.Fatalf("scheduler kept admitting past the %d concurrent-run cap", limits.MaxConcurrentRuns)
		return nil
	}
	churn := func(prefix string) {
		t.Helper()
		w := work(prefix+"-0", "shared")
		for i := 0; i < 1000; i++ {
			w.Definition = fmt.Sprint(prefix, "-", i%3)
			enqueue(t, s, w, now)
		}
	}
	churn("def")
	live := drain()
	if len(live) != limits.MaxRunsPerCheck {
		t.Fatalf("concurrent runs for one check=%d want %d", len(live), limits.MaxRunsPerCheck)
	}
	// The admission above is still outstanding, so these events re-queue a check
	// that already owns its run slot, under an otherwise idle definition.
	churn("alt")
	if extra := drain(); len(extra) != 0 {
		t.Fatalf("concurrent runs for one check=%d want %d", len(live)+len(extra), limits.MaxRunsPerCheck)
	}
	// Completing a run releases the check slot; the 2000 coalesced events behind
	// it must have collapsed into one queued wake rather than a backlog.
	wakes := 0
	for len(live) > 0 {
		if wakes > limits.MaxQueuedWakesPerCheck {
			t.Fatalf("queued wakes for one check exceeded %d", limits.MaxQueuedWakesPerCheck)
		}
		admission := live[0]
		live = live[1:]
		admission.Finish(execution.Completed, now)
		woken := drain()
		wakes += len(woken)
		live = append(live, woken...)
	}
	if wakes != limits.MaxQueuedWakesPerCheck {
		t.Fatalf("queued wakes for one check=%d want %d", wakes, limits.MaxQueuedWakesPerCheck)
	}
}
