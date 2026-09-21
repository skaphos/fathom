/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"container/list"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
	"unicode/utf8"

	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Work names live inputs. Workers resolve their current revision after admission;
// queued events never retain stale compiled objects or impersonated clients.
type Work struct {
	Definition string
	Check      types.NamespacedName
}

type Disposition uint8

const (
	Completed Disposition = iota
	Retry
	MissingInput
)

type queuedCheck struct {
	desired    Work
	active     *Admission
	node       *list.Element
	readyAt    time.Time
	forgotten  bool
	failures   uint8
	lastReason string
}
type definitionQueue struct {
	name   string
	checks list.List
	node   *list.Element
}

// Scheduler is the manager's single runtime admission pool, separate from
// built-in controller workers. It owns no goroutines: workers wait on Wake and
// the delay returned by Next. Stable object names preserve slots/backoff across
// UID/revision churn; authority is independently checked by each admitted run.
type Scheduler struct {
	mu                sync.Mutex
	running           bool
	checks            map[types.NamespacedName]*queuedCheck
	eventTimes        map[types.NamespacedName]time.Time
	definitions       map[string]*definitionQueue
	ring              list.List
	activeDefinitions map[string]int
	active            int
	wake              chan struct{}
	jitter            func() float64
}

func NewScheduler(jitter func() float64) *Scheduler {
	if jitter == nil {
		jitter = rand.Float64
	}
	return &Scheduler{eventTimes: map[types.NamespacedName]time.Time{}, checks: map[types.NamespacedName]*queuedCheck{}, definitions: map[string]*definitionQueue{}, activeDefinitions: map[string]int{}, wake: make(chan struct{}, 1), jitter: jitter}
}
func (s *Scheduler) Wake() <-chan struct{} { return s.wake }
func (s *Scheduler) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Enqueue(work Work, now time.Time) error {
	if len(validation.IsDNS1123Label(work.Definition)) != 0 || len(validation.IsDNS1123Label(work.Check.Namespace)) != 0 || len(validation.IsDNS1123Subdomain(work.Check.Name)) != 0 {
		return fmt.Errorf("InvalidDefinition: scheduling requires definition and namespaced check names")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.checks[work.Check]
	if entry == nil {
		entry = &queuedCheck{desired: work, readyAt: now}
		s.checks[work.Check] = entry
	}
	if entry.desired.Definition != work.Definition {
		s.detach(entry)
	}
	entry.desired = work
	entry.forgotten = false
	s.attach(entry)
	s.notify()
	return nil
}
func (s *Scheduler) attach(entry *queuedCheck) {
	if entry.node != nil {
		return
	}
	name := entry.desired.Definition
	queue := s.definitions[name]
	if queue == nil {
		queue = &definitionQueue{name: name}
		queue.node = s.ring.PushBack(queue)
		s.definitions[name] = queue
	}
	entry.node = queue.checks.PushBack(entry)
}
func (s *Scheduler) detach(entry *queuedCheck) {
	if entry.node == nil {
		return
	}
	queue := s.definitions[entry.desired.Definition]
	queue.checks.Remove(entry.node)
	entry.node = nil
	if queue.checks.Len() == 0 {
		s.ring.Remove(queue.node)
		delete(s.definitions, queue.name)
	}
}

// Admission owns one process, definition and check slot. Finish is idempotent;
// stale completions cannot release a newer run. Work returns a value snapshot.
type Admission struct {
	scheduler *Scheduler
	work      Work
}

func (a *Admission) Work() Work { return a.work }

// Next selects a ready check with round-robin definition fairness. A negative
// delay means only an event/slot release can make progress; otherwise callers
// also arm a timer for the returned delay. It never sleeps or starts work.
func (s *Scheduler) Next(now time.Time) (*Admission, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wait := time.Duration(-1)
	consider := func(at time.Time) {
		d := max(at.Sub(now), 0)
		if wait < 0 || d < wait {
			wait = d
		}
	}
	// Event cooldowns survive successful completion and expire independently of
	// queue entries, so rapid object recreation cannot reset the rate limit.
	for key, last := range s.eventTimes {
		expires := last.Add(limits.InitialRetryBackoff)
		if !expires.After(now) {
			delete(s.eventTimes, key)
		} else {
			consider(expires)
		}
	}
	// Deleted failed checks retain only a short-lived backoff tombstone, preventing
	// delete/recreate from accelerating retries. Next's timer also expires these.
	for key, entry := range s.checks {
		if entry.forgotten && entry.active == nil {
			if !entry.readyAt.After(now) {
				delete(s.checks, key)
			} else {
				consider(entry.readyAt)
			}
		}
	}
	if s.active >= limits.MaxConcurrentRuns {
		return nil, wait
	}
	definitions := s.ring.Len()
	for i := 0; i < definitions; i++ {
		element := s.ring.Front()
		queue := element.Value.(*definitionQueue)
		s.ring.MoveToBack(element)
		if s.activeDefinitions[queue.name] >= limits.MaxRunsPerDefinition {
			continue
		}
		for node := queue.checks.Front(); node != nil; node = node.Next() {
			entry := node.Value.(*queuedCheck)
			if entry.active != nil {
				continue
			}
			if entry.readyAt.After(now) {
				consider(entry.readyAt)
				continue
			}
			s.detach(entry)
			admission := &Admission{scheduler: s, work: entry.desired}
			entry.active = admission
			s.active++
			s.activeDefinitions[admission.work.Definition]++
			// Relay a coalesced wake to another idle worker when capacity remains.
			if s.active < limits.MaxConcurrentRuns && s.ring.Len() > 0 {
				s.notify()
			}
			return admission, 0
		}
	}
	return nil, wait
}

func (a *Admission) Finish(outcome Disposition, now time.Time) {
	if a == nil || a.scheduler == nil {
		return
	}
	s := a.scheduler
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.checks[a.work.Check]
	if entry == nil || entry.active != a {
		return
	}
	entry.active = nil
	s.active--
	s.activeDefinitions[a.work.Definition]--
	if s.activeDefinitions[a.work.Definition] == 0 {
		delete(s.activeDefinitions, a.work.Definition)
	}
	switch outcome {
	case Completed:
		entry.failures = 0
		entry.lastReason = ""
		entry.readyAt = now
	case MissingInput:
		entry.readyAt = now.Add(limits.MissingInputPoll)
	default:
		if entry.failures < 5 {
			entry.failures++
		}
		delay := min(limits.InitialRetryBackoff*time.Duration(1<<uint(entry.failures-1)), limits.MaxRetryBackoff)
		jitter := s.jitter()
		if !(jitter >= 0 && jitter <= 1) {
			jitter = 0
		}
		// Positive jitter never exceeds the absolute retry-delay ceiling.
		delay = min(delay+time.Duration(float64(delay)*jitter*float64(limits.MaxRetryJitterPercent)/100), limits.MaxRetryBackoff)
		entry.readyAt = now.Add(delay)
	}
	if entry.forgotten {
		if !entry.readyAt.After(now) {
			delete(s.checks, a.work.Check)
		}
	} else if outcome != Completed || entry.node != nil {
		s.attach(entry)
	} else {
		delete(s.checks, a.work.Check)
	}
	s.notify()
}

// Forget cancels a queued wake but never releases an active run's slots. Its
// caller separately cancels active evaluation when deletion/revocation is seen.
func (s *Scheduler) Forget(check types.NamespacedName, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.checks[check]
	if entry == nil {
		return
	}
	s.detach(entry)
	entry.forgotten = true
	if entry.active == nil && !entry.readyAt.After(now) {
		delete(s.checks, check)
	}
	s.notify()
}

// ShouldReport deduplicates unchanged failures across revisions and limits
// changed failure events to the minimum retry interval. Messages come from the
// separately bounded diagnostic layer; this method never emits an event.
func (s *Scheduler) ShouldReport(check types.NamespacedName, reason string, now time.Time) bool {
	if reason == "" || len(reason) > limits.MaxConditionReasonBytes || !utf8.ValidString(reason) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.checks[check]
	last := s.eventTimes[check]
	if entry == nil || entry.lastReason == reason || (!last.IsZero() && now.Before(last.Add(limits.InitialRetryBackoff))) {
		return false
	}
	entry.lastReason = reason
	s.eventTimes[check] = now
	s.notify()
	return true
}
