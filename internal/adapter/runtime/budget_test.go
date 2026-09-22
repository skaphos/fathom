/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/internal/adapter/runtime/testutil"
	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// helperDContractRow pins one numeric row of contracts/runtime.md. enforce drives
// the real enforcer at and over the cap. A nil enforce is only legitimate when no
// enforcer in this package can reach the row; owner then names where it lives.
type helperDContractRow struct {
	want    int64
	owner   string
	enforce func(t *testing.T, at, over int64)
}

// helperDSlack absorbs scheduling jitter. Deadlines are read as remaining time,
// so the granted window is (want-helperDSlack, want]: a clamp can never hand out
// more than the cap, and granting far less than asked for is equally wrong.
const helperDSlack = 50 * time.Millisecond

// helperDTargetPath is a single authorized namespaced object read.
const helperDTargetPath = "/api/v1/namespaces/allowed/configmaps/config"

func helperDBudget(t *testing.T) *execution.Budget {
	t.Helper()
	b, done := execution.NewBudget(context.Background(), limits.MaxRunDuration)
	t.Cleanup(done)
	return b
}

// helperDRejects asserts an over-cap operation failed with the contract's reason.
func helperDRejects(t *testing.T, err error, reason string) {
	t.Helper()
	var failure *execution.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("want a %s failure, got %v", reason, err)
	}
	if failure.Reason != reason {
		t.Fatalf("failure reason %q, want %q", failure.Reason, reason)
	}
}

// helperDGrants reads the deadline as time still available. A clamp that hands
// out more than want fails immediately, however small or large the overshoot:
// the measurement can only shrink between construction and this call, never grow.
func helperDGrants(t *testing.T, ctx context.Context, want time.Duration, what string) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("%s carries no deadline", what)
	}
	remaining := time.Until(deadline)
	if remaining > want || remaining <= want-helperDSlack {
		t.Fatalf("%s grants %v, want (%v, %v]", what, remaining, want-helperDSlack, want)
	}
}

// helperDWalkPage feeds one complete page of n items through WalkPages so the
// page-size row is driven by the real enforcer rather than a copy of its constant.
func helperDWalkPage(t *testing.T, b *execution.Budget, items int) error {
	t.Helper()
	reader := pageReader{list: func(_ context.Context, out client.ObjectList, _ ...client.ListOption) error {
		out.(*corev1.ConfigMapList).Items = make([]corev1.ConfigMap, items)
		return nil
	}}
	return execution.WalkPages(b.Context(), b, reader, &corev1.ConfigMapList{},
		func(context.Context, client.ObjectList) error { return nil },
		func(context.Context) error { return nil })
}

// helperDGuard authorizes exactly one namespace and one declared kind.
func helperDGuard(t *testing.T, b *execution.Budget) *execution.Guard {
	t.Helper()
	g, err := execution.NewGuard(b, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"allowed"}},
		map[schema.GroupVersionKind]bool{{Group: "", Version: "v1", Kind: "ConfigMap"}: true})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// helperDStatus answers every attempt with the same status and a fresh body, the
// way a retried request sees a new response each time.
func helperDStatus(status int, body string) http.RoundTripper {
	return roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
}

func helperDSend(t *testing.T, transport http.RoundTripper, path string) error {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, "https://cluster"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

// helperDTarget reads one object body through a real guarded transport, so the
// traversal rows are decided by Guard.inspectObject rather than by a test copy.
func helperDTarget(t *testing.T, body string) error {
	t.Helper()
	b := helperDBudget(t)
	return helperDSend(t, mappedRuntimeGuard(t, b, false).Wrap(helperDStatus(http.StatusOK, body)), helperDTargetPath)
}

// helperDScope reports whether a binding of n namespaces is authorized at all.
func helperDScope(n int) error {
	namespaces := make([]api.DefinitionDNSLabel, 0, n)
	for i := 0; i < n; i++ {
		namespaces = append(namespaces, api.DefinitionDNSLabel(fmt.Sprint("ns-", i)))
	}
	b, done := execution.NewBudget(context.Background(), limits.MaxRunDuration)
	defer done()
	_, err := execution.NewGuard(b, api.DefinitionBindingScope{Namespaces: namespaces},
		map[schema.GroupVersionKind]bool{{Group: "", Version: "v1", Kind: "ConfigMap"}: true})
	return err
}

// helperDResource lists a resource whose path segment is exactly n bytes long.
func helperDResource(t *testing.T, n int) error {
	t.Helper()
	b := helperDBudget(t)
	resource := testutil.Bytes(n)
	guard := helperDGuard(t, b)
	discovered := resource
	if !limits.ValidResourceSegment(resource) {
		discovered = "configmaps"
	}
	primeDiscovery(t, guard, "/api/v1", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"`+discovered+`","kind":"ConfigMap","namespaced":true}]}`)
	transport := guard.Wrap(helperDStatus(http.StatusOK, `{"kind":"ConfigMapList","items":[]}`))
	return helperDSend(t, transport, "/api/v1/namespaces/allowed/"+resource)
}

// helperDRateSpan is measured once: the limiter is process-wide, so every row
// that reads it would otherwise pay the same wall-clock cost all over again.
var (
	helperDRateOnce sync.Once
	helperDRateSpan time.Duration
)

// helperDRate times RequestBurst+RequestsPerSecond requests through a real
// guarded transport. Stored tokens cover only the first RequestBurst of them, so
// the remainder must be refilled at the contract rate and the span cannot be short.
func helperDRate(t *testing.T) time.Duration {
	t.Helper()
	helperDRateOnce.Do(func() {
		b := helperDBudget(t)
		transport := mappedRuntimeGuard(t, b, false).Wrap(helperDStatus(http.StatusOK, `{"kind":"ConfigMap"}`))
		started := time.Now()
		for i := 0; i < limits.RequestBurst+limits.RequestsPerSecond; i++ {
			if err := helperDSend(t, transport, helperDTargetPath); err != nil {
				t.Fatalf("rate probe request %d rejected: %v", i, err)
			}
		}
		helperDRateSpan = time.Since(started)
	})
	return helperDRateSpan
}

// helperDChecks builds n minimal completed observations.
func helperDChecks(n int) adapter.Result {
	result := adapter.Result{Checks: make([]adapter.CheckResult, 0, n)}
	for i := 0; i < n; i++ {
		result.Checks = append(result.Checks, adapter.CheckResult{Family: "health", Outcome: adapter.OutcomePass, Summary: "ok"})
	}
	return result
}

// helperDEvidence builds a result whose serialized form is exactly size bytes,
// so the evidence row is driven at its real boundary rather than near it.
func helperDEvidence(t *testing.T, size int) adapter.Result {
	t.Helper()
	result := helperDChecks(1)
	result.Checks[0].Details = map[string]string{"payload": ""}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if size < len(raw) {
		t.Fatalf("cannot build %d bytes of evidence from a %d-byte envelope", size, len(raw))
	}
	result.Checks[0].Details["payload"] = testutil.Bytes(size - len(raw))
	if raw, err = json.Marshal(result); err != nil {
		t.Fatal(err)
	}
	if len(raw) != size {
		t.Fatalf("evidence fixture is %d bytes, want %d", len(raw), size)
	}
	return result
}

// helperDAdapter is an inert compiled snapshot; the cache only ever stores it.
type helperDAdapter struct{ adapter.Adapter }

func helperDRevision(i int) execution.RevisionKey {
	return execution.RevisionKey{DefinitionUID: types.UID(fmt.Sprint("uid-", i)), Generation: 1, SchemaVersion: "v1alpha1", SemanticsVersion: 1, OperatorBuild: "test-build", AdapterVersion: "1.0.0"}
}

// helperDCacheRecompiles fills an idle cache with n distinct revisions and reports
// whether the least recently used one then had to be compiled a second time.
func helperDCacheRecompiles(t *testing.T, n int) bool {
	t.Helper()
	cache := execution.NewCache()
	compiles := 0
	acquire := func(i int) {
		t.Helper()
		snapshot, err := cache.Acquire(context.Background(), helperDRevision(i), func(context.Context) (adapter.Adapter, error) {
			compiles++
			return helperDAdapter{}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Release()
	}
	for i := 0; i < n; i++ {
		acquire(i)
	}
	before := compiles
	acquire(0)
	return compiles > before
}

func helperDWork(definition, check string) execution.Work {
	return execution.Work{Definition: definition, Check: types.NamespacedName{Namespace: "checks", Name: check}}
}

func helperDEnqueue(t *testing.T, s *execution.Scheduler, w execution.Work, now time.Time) {
	t.Helper()
	if err := s.Enqueue(w, now); err != nil {
		t.Fatal(err)
	}
}

// helperDDrain admits everything the scheduler is willing to start at now.
func helperDDrain(t *testing.T, s *execution.Scheduler, now time.Time) int {
	t.Helper()
	admitted := 0
	for {
		a, _ := s.Next(now)
		if a == nil {
			return admitted
		}
		admitted++
		if admitted > limits.MaxConcurrentRuns {
			t.Fatalf("scheduler admitted %d runs, past its concurrency cap", admitted)
		}
	}
}

// helperDReports reports whether a condition reason of n bytes reaches an event.
func helperDReports(t *testing.T, n int) bool {
	t.Helper()
	s := execution.NewScheduler(func() float64 { return 0 })
	now := time.Now()
	w := helperDWork("definition", "check")
	helperDEnqueue(t, s, w, now)
	return s.ShouldReport(w.Check, testutil.Bytes(n), now)
}

// helperDRetryDelay finishes attempts consecutive retries, advancing past each
// backoff, and returns the delay the scheduler imposes after the last one.
func helperDRetryDelay(t *testing.T, attempts int, jitter float64) time.Duration {
	t.Helper()
	s := execution.NewScheduler(func() float64 { return jitter })
	now := time.Now()
	helperDEnqueue(t, s, helperDWork("definition", "check"), now)
	var wait time.Duration
	for i := 0; i < attempts; i++ {
		a, _ := s.Next(now)
		if a == nil {
			t.Fatalf("no admission for retry %d", i)
		}
		a.Finish(execution.Retry, now)
		if _, wait = s.Next(now); wait <= 0 {
			t.Fatalf("retry %d was rescheduled with no backoff", i)
		}
		now = now.Add(wait)
	}
	return wait
}

func helperDContractRows() map[string]helperDContractRow {
	// Owners of rows this package cannot drive. Each string was verified by
	// grepping every non-test reference to the constant; an inaccurate ledger is
	// worse than none, because it is offered as the authoritative map.
	const (
		unenforced = "not enforced anywhere yet: no non-test source reads the constant, so the contract value is pinned here until an enforcer lands"
		policy     = "enforced by internal/adapter/declarative/runtime_policy.go, in another package"
		parser     = "enforced by internal/adapter/declarative/runtime_parser.go, in another package"
		annotation = "enforced by internal/adapter/declarative/annotation.go, in another package"
		drain      = "enforced by internal/cli/definition_drain.go, in another package"
		compileDur = "the compile deadlines in internal/adapter/runtime/cache.go and runner.go"
		scheduler  = "internal/adapter/runtime/scheduler.go"
		transport  = "internal/adapter/runtime/transport.go"
		results    = "Budget.ValidateResult in internal/adapter/runtime/result.go"
		guard      = "Guard.inspectObject in internal/adapter/runtime/response.go"
		budgetGo   = "internal/adapter/runtime/budget.go"
		cacheGo    = "internal/adapter/runtime/cache.go"
	)
	return map[string]helperDContractRow{
		// Definition, strings/maps/lists, names, versions, selectors. The compiler
		// bounds that already have an enforcer live in the declarative package.
		"MaxSpecBytes":           {want: 256 << 10, owner: unenforced},
		"MaxFamilies":            {want: 16, owner: unenforced},
		"MaxChecksPerFamily":     {want: 32, owner: unenforced},
		"MaxChecks":              {want: 512, owner: unenforced},
		"MaxStringBytes":         {want: 1024, owner: policy},
		"MaxStringRunes":         {want: 1024, owner: unenforced},
		"MaxMapEntries":          {want: 32, owner: policy},
		"MaxListEntries":         {want: 32, owner: unenforced},
		"MaxSpecDepth":           {want: 8, owner: unenforced},
		"MaxIdentifierBytes":     {want: 63, owner: policy},
		"MaxBindingBytes":        {want: 16 << 10, owner: unenforced},
		"MaxVersionRangeBytes":   {want: 256, owner: unenforced},
		"MaxVersionComparators":  {want: 16, owner: unenforced},
		"MaxVersionAlternatives": {want: 8, owner: unenforced},
		"MaxAPIVersions":         {want: 8, owner: unenforced},
		"MaxSelectorTerms":       {want: 32, owner: unenforced},
		"MaxSelectorValues":      {want: 32, owner: unenforced},
		"MaxSelectorValueBytes":  {want: 256, owner: unenforced},
		"MaxFieldSegments":       {want: 16, owner: unenforced},
		"MaxFieldSegmentBytes":   {want: 128, owner: unenforced},
		// Payload bounds from contracts/payloads.md rather than the numeric table.
		"MaxUIDBytes":            {want: 128, owner: unenforced},
		"MaxConditions":          {want: 8, owner: unenforced},
		"MaxConditionTypeBytes":  {want: 64, owner: unenforced},
		"MaxLeaderIdentityBytes": {want: 253, owner: unenforced},
		"MaxAdapterVersionBytes": {want: 256, owner: unenforced},
		"MaxDurationBytes":       {want: 256, owner: unenforced},
		// YAML and annotations.
		"MaxYAMLBytes":       {want: 64 << 10, owner: parser},
		"MaxYAMLNodes":       {want: 4096, owner: parser},
		"MaxYAMLDepth":       {want: 16, owner: parser},
		"MaxAnnotationBytes": {want: 1024, owner: annotation},
		// Leadership takeover. Unlike the drain rows below, no non-test source
		// reads this floor yet; it is a minimum, so an enforcer added here must
		// assert the direction the Boundary doc-comment calls out.
		"MinTakeoverGrace": {want: int64(30 * time.Second), owner: unenforced},
		// Drain verification, enforced by the CLI rather than by the operator.
		"MaxDrainDuration":   {want: int64(15 * time.Second), owner: drain},
		"MaxDrainLeaseReads": {want: 16, owner: drain},
		"DrainPollInterval":  {want: int64(time.Second), owner: drain},

		// Compilation deadline. Both call sites are in this package, so the row is
		// driven, at the cost of one MaxCompileDuration of wall clock.
		"MaxCompileDuration": {want: int64(time.Second), owner: compileDur, enforce: func(t *testing.T, at, _ int64) {
			cache := execution.NewCache()
			started := time.Now()
			_, err := cache.Acquire(context.Background(), helperDRevision(0), func(ctx context.Context) (adapter.Adapter, error) {
				select {
				case <-ctx.Done():
				// A lost deadline must fail this row, not hang the whole suite.
				case <-time.After(5 * time.Duration(at)):
				}
				return nil, ctx.Err()
			})
			elapsed := time.Since(started)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("compilation that outran its budget returned %v", err)
			}
			if elapsed < time.Duration(at)-helperDSlack || elapsed > time.Duration(at)+10*helperDSlack {
				t.Fatalf("compilation was cut off after %v, want about %v", elapsed, time.Duration(at))
			}
		}},
		// Binding scope and API path segments, enforced by the guard.
		"MaxNamespaces": {want: 32, owner: transport, enforce: func(t *testing.T, at, over int64) {
			if err := helperDScope(int(at)); err != nil {
				t.Fatalf("a %d-namespace binding was refused: %v", at, err)
			}
			if err := helperDScope(int(over)); err == nil {
				t.Fatalf("a %d-namespace binding was authorized", over)
			}
		}},
		"MaxResourceSegmentBytes": {want: 253, owner: transport, enforce: func(t *testing.T, at, over int64) {
			if err := helperDResource(t, int(at)); err != nil {
				t.Fatalf("a %d-byte resource segment was denied: %v", at, err)
			}
			helperDRejects(t, helperDResource(t, int(over)), "ScopeDenied")
		}},
		// Target object traversal.
		"MaxObjectNodes": {want: 32768, owner: guard, enforce: func(t *testing.T, at, over int64) {
			// The envelope contributes the root map and one scalar member.
			body := func(n int64) string { return `{"kind":"ConfigMap","value":` + testutil.JSONNodes(int(n)-2) + `}` }
			if err := helperDTarget(t, body(at)); err != nil {
				t.Fatalf("an object of exactly %d nodes was rejected: %v", at, err)
			}
			helperDRejects(t, helperDTarget(t, body(over)), "InputLimitExceeded")
		}},
		"MaxObjectDepth": {want: 64, owner: guard, enforce: func(t *testing.T, at, over int64) {
			// The root map is depth zero, so n-1 nested arrays put a scalar at n.
			body := func(n int64) string { return `{"value":` + testutil.JSONDepth(int(n)-1) + `}` }
			if err := helperDTarget(t, body(at)); err != nil {
				t.Fatalf("an object of exactly depth %d was rejected: %v", at, err)
			}
			helperDRejects(t, helperDTarget(t, body(over)), "InputLimitExceeded")
		}},
		// Results.
		"MaxResults": {want: 1000, owner: results, enforce: func(t *testing.T, at, over int64) {
			if err := helperDBudget(t).ValidateResult(helperDChecks(int(at))); err != nil {
				t.Fatalf("at-cap result rejected: %v", err)
			}
			helperDRejects(t, helperDBudget(t).ValidateResult(helperDChecks(int(over))), "ResultLimitExceeded")
		}},
		"MaxEvidenceBytes": {want: 256 << 10, owner: results, enforce: func(t *testing.T, at, over int64) {
			if err := helperDBudget(t).ValidateResult(helperDEvidence(t, int(at))); err != nil {
				t.Fatalf("evidence of exactly %d bytes rejected: %v", at, err)
			}
			helperDRejects(t, helperDBudget(t).ValidateResult(helperDEvidence(t, int(over))), "ResultLimitExceeded")
		}},
		"MaxMessageBytes": {want: 1024, owner: results, enforce: func(t *testing.T, at, over int64) {
			summarize := func(n int64) adapter.Result {
				r := helperDChecks(1)
				r.Checks[0].Summary = testutil.Bytes(int(n))
				return r
			}
			if err := helperDBudget(t).ValidateResult(summarize(at)); err != nil {
				t.Fatalf("a %d-byte summary was rejected: %v", at, err)
			}
			helperDRejects(t, helperDBudget(t).ValidateResult(summarize(over)), "ResultLimitExceeded")
		}},
		"MaxDetails": {want: 32, owner: results, enforce: func(t *testing.T, at, over int64) {
			detail := func(n int64) adapter.Result {
				r := helperDChecks(1)
				r.Checks[0].Details = testutil.Map(int(n))
				return r
			}
			if err := helperDBudget(t).ValidateResult(detail(at)); err != nil {
				t.Fatalf("%d details were rejected: %v", at, err)
			}
			helperDRejects(t, helperDBudget(t).ValidateResult(detail(over)), "ResultLimitExceeded")
		}},
		// Compiled snapshot cache.
		"MaxCachedRevisions": {want: 128, owner: cacheGo, enforce: func(t *testing.T, at, over int64) {
			if helperDCacheRecompiles(t, int(at)) {
				t.Fatalf("an idle revision was evicted at or below the %d-entry cap", at)
			}
			if !helperDCacheRecompiles(t, int(over)) {
				t.Fatalf("the idle cache retained more than %d revisions", at)
			}
		}},
		"MaxConcurrentRuns": {want: 4, owner: cacheGo + ", pool.go and scheduler.go", enforce: func(t *testing.T, at, over int64) {
			cache := execution.NewCache()
			compile := func(context.Context) (adapter.Adapter, error) { return helperDAdapter{}, nil }
			for i := int64(0); i < at; i++ {
				snapshot, err := cache.Acquire(context.Background(), helperDRevision(int(i)), compile)
				if err != nil {
					t.Fatalf("concurrent snapshot %d rejected: %v", i, err)
				}
				t.Cleanup(snapshot.Release)
			}
			_, err := cache.Acquire(context.Background(), helperDRevision(int(over)), compile)
			helperDRejects(t, err, "WorkLimitExceeded")
		}},
		// Scheduling fairness and per-check slots. The two per-check rows are
		// primarily enforced by structure — queuedCheck holds exactly one active
		// run and one queue node, pinned at compile time in scheduler.go, so a
		// value other than one does not build. The assertions below drive the
		// observable consequence; note that MaxRunsPerDefinition is also one and
		// binds first, so isolating a per-check regression by mutation means
		// relaxing the per-definition cap at the same time.
		"MaxRunsPerDefinition": {want: 1, owner: scheduler, enforce: func(t *testing.T, at, over int64) {
			s := execution.NewScheduler(func() float64 { return 0 })
			now := time.Now()
			for i := int64(0); i < over; i++ {
				helperDEnqueue(t, s, helperDWork("shared", fmt.Sprint("check-", i)), now)
			}
			if admitted := helperDDrain(t, s, now); int64(admitted) != at {
				t.Fatalf("%d ready checks of one definition started %d runs, want %d", over, admitted, at)
			}
		}},
		"MaxRunsPerCheck": {want: 1, owner: scheduler + " and its compile-time pins", enforce: func(t *testing.T, at, over int64) {
			s := execution.NewScheduler(func() float64 { return 0 })
			now := time.Now()
			w := helperDWork("definition", "check")
			for i := int64(0); i < over; i++ {
				helperDEnqueue(t, s, w, now)
			}
			if admitted := helperDDrain(t, s, now); int64(admitted) != at {
				t.Fatalf("%d events for one check started %d concurrent runs, want %d", over, admitted, at)
			}
		}},
		"MaxQueuedWakesPerCheck": {want: 1, owner: scheduler + " and its compile-time pins", enforce: func(t *testing.T, at, over int64) {
			s := execution.NewScheduler(func() float64 { return 0 })
			now := time.Now()
			w := helperDWork("definition", "check")
			helperDEnqueue(t, s, w, now)
			admission, _ := s.Next(now)
			if admission == nil {
				t.Fatal("no admission for the first wake")
			}
			// Churn during the run must collapse into a single pending wake.
			for i := int64(0); i < over; i++ {
				helperDEnqueue(t, s, w, now)
			}
			admission.Finish(execution.Completed, now)
			if admitted := helperDDrain(t, s, now); int64(admitted) != at {
				t.Fatalf("%d events during a run queued %d wakes, want %d", over, admitted, at)
			}
		}},
		"MaxConditionReasonBytes": {want: 128, owner: scheduler, enforce: func(t *testing.T, at, over int64) {
			if !helperDReports(t, int(at)) {
				t.Fatalf("a %d-byte condition reason was refused", at)
			}
			if helperDReports(t, int(over)) {
				t.Fatalf("a %d-byte condition reason was reported", over)
			}
		}},
		// Retry backoff. These rows are delays rather than caps: the enforcer is
		// driven at the configured value and the scheduled delay must match it.
		"InitialRetryBackoff": {want: int64(5 * time.Second), owner: scheduler, enforce: func(t *testing.T, at, _ int64) {
			if delay := helperDRetryDelay(t, 1, 0); delay != time.Duration(at) {
				t.Fatalf("first retry waits %v, want %v", delay, time.Duration(at))
			}
		}},
		"MaxRetryBackoff": {want: int64(time.Minute), owner: scheduler, enforce: func(t *testing.T, at, _ int64) {
			// Five doublings of the initial backoff overshoot the ceiling.
			if delay := helperDRetryDelay(t, 5, 0); delay != time.Duration(at) {
				t.Fatalf("saturated retry waits %v, want %v", delay, time.Duration(at))
			}
		}},
		"MaxRetryJitterPercent": {want: 20, owner: scheduler, enforce: func(t *testing.T, at, _ int64) {
			base := limits.InitialRetryBackoff
			want := base + base*time.Duration(at)/100
			if delay := helperDRetryDelay(t, 1, 1); delay != want {
				t.Fatalf("fully jittered retry waits %v, want %v", delay, want)
			}
		}},
		"MissingInputPoll": {want: int64(time.Minute), owner: scheduler, enforce: func(t *testing.T, at, _ int64) {
			s := execution.NewScheduler(func() float64 { return 0 })
			now := time.Now()
			helperDEnqueue(t, s, helperDWork("definition", "check"), now)
			admission, _ := s.Next(now)
			if admission == nil {
				t.Fatal("no admission to report a missing input from")
			}
			admission.Finish(execution.MissingInput, now)
			if _, wait := s.Next(now); wait != time.Duration(at) {
				t.Fatalf("missing input repolls in %v, want %v", wait, time.Duration(at))
			}
		}},
		// Request rate. Both rows read one shared measurement: the limiter is
		// process-wide, and either an inflated rate or an inflated bucket would
		// let the same request sequence finish without ever waiting.
		"RequestsPerSecond": {want: 10, owner: transport, enforce: func(t *testing.T, at, _ int64) {
			want := time.Second - helperDSlack
			if span := helperDRate(t); span < want {
				t.Fatalf("%d requests took %v; refilling past the burst at %d/s needs at least %v",
					int64(limits.RequestBurst)+at, span, at, want)
			}
		}},
		"RequestBurst": {want: 20, owner: transport, enforce: func(t *testing.T, at, _ int64) {
			want := time.Second - helperDSlack
			if span := helperDRate(t); span < want {
				t.Fatalf("%d requests cleared a %d-token bucket in %v, want at least %v",
					int64(limits.RequestsPerSecond)+at, at, span, want)
			}
		}},
		"MaxRequestRetries": {want: 2, owner: transport, enforce: func(t *testing.T, at, _ int64) {
			// A 5xx feeds the per-URL counter: the initial attempt plus at
			// retries are permitted, and the repetition after them is refused.
			b := helperDBudget(t)
			transport := mappedRuntimeGuard(t, b, false).Wrap(helperDStatus(http.StatusServiceUnavailable, `{"kind":"Status"}`))
			for i := int64(0); i <= at; i++ {
				if err := helperDSend(t, transport, helperDTargetPath); err != nil {
					t.Fatalf("attempt %d rejected: %v", i, err)
				}
			}
			helperDRejects(t, helperDSend(t, transport, helperDTargetPath), "WorkLimitExceeded")
		}},

		// Rows the budget itself enforces, driven at and over their cap.
		"MaxResponseBytes": {want: 2 << 20, owner: budgetGo, enforce: func(t *testing.T, at, over int64) {
			if err := helperDBudget(t).ChargeResponse(int(at)); err != nil {
				t.Fatalf("at-cap response rejected: %v", err)
			}
			helperDRejects(t, helperDBudget(t).ChargeResponse(int(over)), "ResponseLimitExceeded")
		}},
		"MaxRunResponseBytes": {want: 16 << 20, owner: budgetGo, enforce: func(t *testing.T, at, over int64) {
			b := helperDBudget(t)
			for charged := int64(0); charged < at; {
				chunk := min(at-charged, int64(limits.MaxResponseBytes))
				if err := b.ChargeResponse(int(chunk)); err != nil {
					t.Fatalf("at-cap cumulative bytes rejected after %d: %v", charged, err)
				}
				charged += chunk
			}
			helperDRejects(t, b.ChargeResponse(int(over-at)), "ResponseLimitExceeded")
		}},
		"MaxPageObjects": {want: 100, owner: "WalkPages in pagination.go and " + guard, enforce: func(t *testing.T, at, over int64) {
			if err := helperDWalkPage(t, helperDBudget(t), int(at)); err != nil {
				t.Fatalf("at-cap page rejected: %v", err)
			}
			helperDRejects(t, helperDWalkPage(t, helperDBudget(t), int(over)), "WorkLimitExceeded")
		}},
		"MaxRunObjects": {want: 1000, owner: budgetGo, enforce: func(t *testing.T, at, over int64) {
			b := helperDBudget(t)
			if err := b.ChargeObjects(int(at)); err != nil {
				t.Fatalf("at-cap objects rejected: %v", err)
			}
			helperDRejects(t, b.ChargeObjects(int(over-at)), "WorkLimitExceeded")
		}},
		"MaxRunRequests": {want: 100, owner: budgetGo, enforce: func(t *testing.T, at, _ int64) {
			b := helperDBudget(t)
			for i := int64(0); i < at; i++ {
				if err := b.ChargeRequest(); err != nil {
					t.Fatalf("at-cap request %d rejected: %v", i, err)
				}
			}
			helperDRejects(t, b.ChargeRequest(), "WorkLimitExceeded")
		}},
		"MaxObjectVisits": {want: 100000, owner: budgetGo, enforce: func(t *testing.T, at, over int64) {
			b := helperDBudget(t)
			if err := b.Visit(int(at)); err != nil {
				t.Fatalf("at-cap traversal rejected: %v", err)
			}
			helperDRejects(t, b.Visit(int(over-at)), "WorkLimitExceeded")
		}},
		"MaxPaginationRestarts": {want: 1, owner: budgetGo, enforce: func(t *testing.T, at, _ int64) {
			b := helperDBudget(t)
			for i := int64(0); i < at; i++ {
				if err := b.ChargePaginationRestart(); err != nil {
					t.Fatalf("at-cap restart %d rejected: %v", i, err)
				}
			}
			helperDRejects(t, b.ChargePaginationRestart(), "WorkLimitExceeded")
		}},
		"MaxRunDuration": {want: int64(30 * time.Second), owner: budgetGo, enforce: func(t *testing.T, at, over int64) {
			// An over-cap request for time is refused by clamping, never honored.
			// The grossly over-cap rows are what separate a real clamp from one
			// degraded to a bare "timeout <= 0" fallback, which a nanosecond over
			// the cap cannot distinguish.
			for _, tc := range []struct {
				name    string
				timeout time.Duration
			}{
				{"at cap", time.Duration(at)},
				{"just over cap", time.Duration(over)},
				{"far over cap", 10 * time.Duration(at)},
				{"unset", 0},
			} {
				t.Run(tc.name, func(t *testing.T) {
					b, done := execution.NewBudget(context.Background(), tc.timeout)
					defer done()
					helperDGrants(t, b.Context(), time.Duration(at), "run")
				})
			}
		}},
		"MaxRequestDuration": {want: int64(5 * time.Second), owner: "Budget.RequestContext in budget.go", enforce: func(t *testing.T, at, _ int64) {
			// A full run has 30s left, so the per-request cap is what binds here.
			b, done := execution.NewBudget(context.Background(), limits.MaxRunDuration)
			defer done()
			ctx, release := b.RequestContext(context.Background())
			defer release()
			helperDGrants(t, ctx, time.Duration(at), "request")
		}},
	}
}

// TestNumericRowsAreEnforced makes testutil.Boundaries() the enumeration the
// contract asks for: every numeric row is either driven at and over its cap
// against the real enforcer, or pinned to its contract value with the owning
// enforcement site named. An unmapped row fails rather than going unnoticed.
func TestNumericRowsAreEnforced(t *testing.T) {
	rows := helperDContractRows()
	mapped := map[string]bool{}
	for _, boundary := range testutil.Boundaries() {
		row, ok := rows[boundary.Name]
		if !ok {
			t.Errorf("%s: no enforcement mapped", boundary.Name)
			continue
		}
		mapped[boundary.Name] = true
		t.Run(boundary.Name, func(t *testing.T) {
			if boundary.At != row.want {
				t.Fatalf("limits.%s = %d, contract requires %d", boundary.Name, boundary.At, row.want)
			}
			if row.enforce == nil {
				t.Skipf("%s (%s); value pinned here only", row.owner, boundary.Task)
			}
			row.enforce(t, boundary.At, boundary.Over)
		})
	}
	for name := range rows {
		if !mapped[name] {
			t.Errorf("%s: mapped row is no longer enumerated by testutil.Boundaries()", name)
		}
	}
}

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

// TestBudgetDeadlineClamps asserts the run deadline from both sides: granting
// less than the caller asked for is as wrong as granting more than the 30s cap.
// The far-over-cap rows are the ones that notice a clamp reduced to a bare
// "timeout <= 0" fallback; a timeout a nanosecond over the cap would not.
func TestBudgetDeadlineClamps(t *testing.T) {
	at, over := testutil.AtAndOverDuration(limits.MaxRunDuration)
	for _, tc := range []struct {
		name          string
		timeout, want time.Duration
	}{
		{"below cap is honored", time.Second, time.Second},
		{"at cap", at, at},
		{"just over cap is clamped", over, limits.MaxRunDuration},
		{"far over cap is clamped", time.Minute, limits.MaxRunDuration},
		{"hours over cap are clamped", time.Hour, limits.MaxRunDuration},
		{"unset falls back to the cap", 0, limits.MaxRunDuration},
		{"negative falls back to the cap", -time.Second, limits.MaxRunDuration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, done := execution.NewBudget(context.Background(), tc.timeout)
			defer done()
			helperDGrants(t, b.Context(), tc.want, "run")
		})
	}
	t.Run("shorter caller deadline still wins", func(t *testing.T) {
		parent, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		b, done := execution.NewBudget(parent, limits.MaxRunDuration)
		defer done()
		helperDGrants(t, b.Context(), 500*time.Millisecond, "run under a shorter caller")
	})
}

// TestBudgetRequestContextClampsToRemaining covers the contract's "each request
// ≤min(5s, remaining)" row in both directions, and that revoking the run reaches
// a request already in flight.
func TestBudgetRequestContextClampsToRemaining(t *testing.T) {
	for _, tc := range []struct {
		name      string
		run, want time.Duration
	}{
		{"per-request cap binds", limits.MaxRunDuration, limits.MaxRequestDuration},
		{"remaining run time binds", time.Second, time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, done := execution.NewBudget(context.Background(), tc.run)
			defer done()
			ctx, release := b.RequestContext(context.Background())
			defer release()
			helperDGrants(t, ctx, tc.want, "request")
		})
	}
	t.Run("run failure cancels an in-flight request", func(t *testing.T) {
		b := helperDBudget(t)
		ctx, release := b.RequestContext(context.Background())
		defer release()
		if err := b.Fail("WorkLimitExceeded", "object limit exceeded"); err == nil {
			t.Fatal("run failure was not recorded")
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("request outlived the revoked run")
		}
		helperDRejects(t, b.Err(), "WorkLimitExceeded")
	})
}
