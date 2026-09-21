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
