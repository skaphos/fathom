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
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type pageReader struct {
	client.Reader
	list func(context.Context, client.ObjectList, ...client.ListOption) error
}

func (r pageReader) List(ctx context.Context, out client.ObjectList, opts ...client.ListOption) error {
	return r.list(ctx, out, opts...)
}

func TestWalkPagesDoesNotPublishTruncatedLists(t *testing.T) {
	for _, tc := range []struct {
		name   string
		repeat bool
	}{{"complete", false}, {"repeated token", true}} {
		t.Run(tc.name, func(t *testing.T) {
			b, done := execution.NewBudget(context.Background(), time.Minute)
			defer done()
			calls, consumed := 0, 0
			r := pageReader{list: func(_ context.Context, out client.ObjectList, opts ...client.ListOption) error {
				options := new(client.ListOptions).ApplyOptions(opts)
				if options.Limit != 100 || options.Namespace != "allowed" {
					t.Fatalf("options=%+v", options)
				}
				if calls == 0 && options.Continue != "" {
					t.Fatal("unexpected initial continuation")
				}
				if calls > 0 && options.Continue != "next" {
					t.Fatal("lost continuation")
				}
				calls++
				page := out.(*corev1.ConfigMapList)
				page.Items = []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("item-%d", calls)}}}
				if calls == 1 || tc.repeat {
					page.Continue = "next"
				}
				return nil
			}}
			err := execution.WalkPages(b.Context(), b, r, &corev1.ConfigMapList{}, func(_ context.Context, page client.ObjectList) error {
				consumed += len(page.(*corev1.ConfigMapList).Items)
				return nil
			}, func(context.Context) error { consumed = 0; return nil }, client.InNamespace("allowed"))
			if (err != nil) != tc.repeat {
				t.Fatalf("err=%v", err)
			}
			if calls != 2 {
				t.Fatalf("requests=%d", calls)
			}
			if !tc.repeat && consumed != 2 {
				t.Fatalf("consumed=%d", consumed)
			}
		})
	}
}

func TestPaginationRestartIsSharedAcrossRun(t *testing.T) {
	b, done := execution.NewBudget(context.Background(), time.Minute)
	defer done()
	calls, resets := 0, 0
	r := pageReader{list: func(_ context.Context, out client.ObjectList, opts ...client.ListOption) error {
		calls++
		if calls == 1 || calls == 3 {
			return apierrors.NewResourceExpired("expired snapshot")
		}
		return nil
	}}
	consume := func(context.Context, client.ObjectList) error { return nil }
	reset := func(context.Context) error { resets++; return nil }
	if err := execution.WalkPages(b.Context(), b, r, &corev1.ConfigMapList{}, consume, reset); err != nil {
		t.Fatal(err)
	}
	if err := execution.WalkPages(b.Context(), b, r, &corev1.ConfigMapList{}, consume, reset); err == nil {
		t.Fatal("second restart permitted in same run")
	}
	if calls != 3 || resets != 1 {
		t.Fatalf("calls=%d resets=%d", calls, resets)
	}
}

func TestPaginationRestartDiscardsEarlierEvidence(t *testing.T) {
	b, done := execution.NewBudget(context.Background(), time.Minute)
	defer done()
	calls, resets := 0, 0
	evidence := []string{}
	r := pageReader{list: func(_ context.Context, out client.ObjectList, opts ...client.ListOption) error {
		options := new(client.ListOptions).ApplyOptions(opts)
		calls++
		page := out.(*corev1.ConfigMapList)
		switch calls {
		case 1:
			page.Items = []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{Name: "obsolete"}}}
			page.Continue = "old"
		case 2:
			if options.Continue != "old" {
				t.Fatal("lost old snapshot continuation")
			}
			return apierrors.NewResourceExpired("expired")
		case 3:
			if options.Continue != "" {
				t.Fatal("restart reused expired continuation")
			}
			page.Items = []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{Name: "current"}}}
		default:
			t.Fatal("unexpected request")
		}
		return nil
	}}
	err := execution.WalkPages(b.Context(), b, r, &corev1.ConfigMapList{}, func(_ context.Context, page client.ObjectList) error {
		for _, item := range page.(*corev1.ConfigMapList).Items {
			evidence = append(evidence, item.Name)
		}
		return nil
	}, func(context.Context) error { resets++; evidence = nil; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if resets != 1 || len(evidence) != 1 || evidence[0] != "current" {
		t.Fatalf("resets=%d evidence=%v", resets, evidence)
	}
}

// TestWalkPagesRejectsUnusablePreconditionsBeforeIO covers the prerequisite
// guards: a walk that cannot be accounted for, restarted or completed from the
// beginning must fail closed before the first List reaches the API server.
func TestWalkPagesRejectsUnusablePreconditionsBeforeIO(t *testing.T) {
	type walk struct {
		reader    client.Reader
		prototype client.ObjectList
		consume   func(context.Context, client.ObjectList) error
		reset     func(context.Context) error
	}
	for _, tc := range []struct {
		name       string
		noBudget   bool
		breaks     func(*walk)
		opts       []client.ListOption
		wantReason string
	}{
		{name: "no run budget", noBudget: true, wantReason: "AuthorizationUnavailable"},
		{name: "no reader", breaks: func(w *walk) { w.reader = nil }, wantReason: "AuthorizationUnavailable"},
		{name: "no list prototype", breaks: func(w *walk) { w.prototype = nil }, wantReason: "AuthorizationUnavailable"},
		{name: "no consumer", breaks: func(w *walk) { w.consume = nil }, wantReason: "AuthorizationUnavailable"},
		{name: "no reset", breaks: func(w *walk) { w.reset = nil }, wantReason: "AuthorizationUnavailable"},
		{name: "raw list options", opts: []client.ListOption{&client.ListOptions{Raw: &metav1.ListOptions{}}}, wantReason: "ScopeDenied"},
		{name: "resumed continuation", opts: []client.ListOption{&client.ListOptions{Continue: "resume"}}, wantReason: "ScopeDenied"},
		// client.Limit assigns unconditionally; a negative Limit inside a
		// client.ListOptions struct is dropped by ApplyToList and never arrives.
		{name: "negative page limit", opts: []client.ListOption{client.Limit(-1)}, wantReason: "ScopeDenied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			args := walk{
				reader:    pageReader{list: func(context.Context, client.ObjectList, ...client.ListOption) error { calls++; return nil }},
				prototype: &corev1.ConfigMapList{},
				consume:   func(context.Context, client.ObjectList) error { return nil },
				reset:     func(context.Context) error { return nil },
			}
			if tc.breaks != nil {
				tc.breaks(&args)
			}
			budget := helperDBudget(t)
			ctx := budget.Context()
			if tc.noBudget {
				budget, ctx = nil, context.Background()
			}
			err := execution.WalkPages(ctx, budget, args.reader, args.prototype, args.consume, args.reset, tc.opts...)
			helperDRejects(t, err, tc.wantReason)
			if calls != 0 {
				t.Fatalf("read the API %d times before the precondition was checked", calls)
			}
			if budget != nil && budget.Err() == nil {
				t.Fatal("precondition failure did not cancel the run")
			}
		})
	}
}

// TestWalkPagesClampsCallerPageLimit stops a caller from asking the API server
// for a larger page than the contract allows, while honoring a smaller request.
func TestWalkPagesClampsCallerPageLimit(t *testing.T) {
	for _, tc := range []struct {
		name            string
		requested, want int64
	}{
		{"below the cap is honored", 10, 10},
		{"over the cap is clamped", limits.MaxPageObjects + 1, limits.MaxPageObjects},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := helperDBudget(t)
			reader := pageReader{list: func(_ context.Context, _ client.ObjectList, opts ...client.ListOption) error {
				if got := new(client.ListOptions).ApplyOptions(opts).Limit; got != tc.want {
					t.Fatalf("page limit %d, want %d", got, tc.want)
				}
				return nil
			}}
			err := execution.WalkPages(b.Context(), b, reader, &corev1.ConfigMapList{},
				func(context.Context, client.ObjectList) error { return nil },
				func(context.Context) error { return nil }, client.Limit(tc.requested))
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestWalkPagesStopsOversizedPagesBeforeConsuming keeps the page-object cap in
// pagination.go in step with the transport's copy: an over-sized page is refused
// and nothing derived from it reaches the consumer.
func TestWalkPagesStopsOversizedPagesBeforeConsuming(t *testing.T) {
	b := helperDBudget(t)
	consumed := 0
	reader := pageReader{list: func(_ context.Context, out client.ObjectList, _ ...client.ListOption) error {
		out.(*corev1.ConfigMapList).Items = make([]corev1.ConfigMap, limits.MaxPageObjects+1)
		return nil
	}}
	err := execution.WalkPages(b.Context(), b, reader, &corev1.ConfigMapList{},
		func(_ context.Context, page client.ObjectList) error {
			consumed += len(page.(*corev1.ConfigMapList).Items)
			return nil
		}, func(context.Context) error { return nil })
	helperDRejects(t, err, "WorkLimitExceeded")
	if consumed != 0 {
		t.Fatalf("consumed %d objects from a rejected page", consumed)
	}
}

// TestWalkPagesRefusesContinuationAtRunObjectCap covers the object-cap arm of the
// continuation guard: a remaining continuation at the cap is WorkLimitExceeded,
// never a truncated success published to the consumer.
func TestWalkPagesRefusesContinuationAtRunObjectCap(t *testing.T) {
	b := helperDBudget(t)
	if err := b.ChargeObjects(limits.MaxRunObjects); err != nil {
		t.Fatal(err)
	}
	calls, consumed := 0, 0
	reader := pageReader{list: func(_ context.Context, out client.ObjectList, _ ...client.ListOption) error {
		calls++
		page := out.(*corev1.ConfigMapList)
		page.Items = []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{Name: "last"}}}
		page.Continue = "more"
		return nil
	}}
	err := execution.WalkPages(b.Context(), b, reader, &corev1.ConfigMapList{},
		func(_ context.Context, page client.ObjectList) error {
			consumed += len(page.(*corev1.ConfigMapList).Items)
			return nil
		}, func(context.Context) error { return nil })
	helperDRejects(t, err, "WorkLimitExceeded")
	if calls != 1 || consumed != 0 {
		t.Fatalf("calls=%d consumed=%d", calls, consumed)
	}
}

// TestWalkPagesStopsWhenTraversalBudgetIsSpent covers the visit charge: the page
// is discarded rather than handed to the consumer once the run cannot pay for it.
func TestWalkPagesStopsWhenTraversalBudgetIsSpent(t *testing.T) {
	b := helperDBudget(t)
	if err := b.Visit(limits.MaxObjectVisits); err != nil {
		t.Fatal(err)
	}
	consumed := 0
	reader := pageReader{list: func(_ context.Context, out client.ObjectList, _ ...client.ListOption) error {
		out.(*corev1.ConfigMapList).Items = []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{Name: "unaffordable"}}}
		return nil
	}}
	err := execution.WalkPages(b.Context(), b, reader, &corev1.ConfigMapList{},
		func(_ context.Context, page client.ObjectList) error {
			consumed += len(page.(*corev1.ConfigMapList).Items)
			return nil
		}, func(context.Context) error { return nil })
	helperDRejects(t, err, "WorkLimitExceeded")
	if consumed != 0 {
		t.Fatalf("consumed %d objects the traversal budget could not pay for", consumed)
	}
}

// TestWalkPagesStopsAtTheRequestCap covers loop exhaustion: a collection that
// keeps issuing fresh continuations ends as a bounded failure, not an endless walk.
func TestWalkPagesStopsAtTheRequestCap(t *testing.T) {
	b := helperDBudget(t)
	calls := 0
	reader := pageReader{list: func(_ context.Context, out client.ObjectList, _ ...client.ListOption) error {
		calls++
		page := out.(*corev1.ConfigMapList)
		page.Items = []corev1.ConfigMap{{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("item-%d", calls)}}}
		page.Continue = fmt.Sprintf("token-%d", calls)
		return nil
	}}
	err := execution.WalkPages(b.Context(), b, reader, &corev1.ConfigMapList{},
		func(context.Context, client.ObjectList) error { return nil },
		func(context.Context) error { return nil })
	helperDRejects(t, err, "WorkLimitExceeded")
	if calls != limits.MaxRunRequests {
		t.Fatalf("made %d requests, want the %d-request cap", calls, limits.MaxRunRequests)
	}
}
