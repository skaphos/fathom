/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func TestClusterHealthSelectsHealthCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		selector           *metav1.LabelSelector
		namespaces         []string
		excludedNamespaces []string
		hcNamespace        string
		hcLabels           map[string]string
		want               bool
	}{
		{
			name:        "nil selector matches everything in scope",
			hcNamespace: "mine",
			hcLabels:    map[string]string{"team": "a"},
			want:        true,
		},
		{
			name:        "matchLabels hit",
			selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"team": "a"}},
			hcNamespace: "mine",
			hcLabels:    map[string]string{"team": "a"},
			want:        true,
		},
		{
			name:        "matchLabels miss",
			selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"team": "a"}},
			hcNamespace: "mine",
			hcLabels:    map[string]string{"team": "b"},
			want:        false,
		},
		{
			name: "matchExpressions In hit",
			selector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "team", Operator: metav1.LabelSelectorOpIn, Values: []string{"a", "b"},
			}}},
			hcNamespace: "mine",
			hcLabels:    map[string]string{"team": "b"},
			want:        true,
		},
		{
			name: "matchExpressions In miss",
			selector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "team", Operator: metav1.LabelSelectorOpIn, Values: []string{"a", "b"},
			}}},
			hcNamespace: "mine",
			hcLabels:    map[string]string{"team": "c"},
			want:        false,
		},
		{
			// The apiserver structurally validates LabelSelector, so this shape
			// is only reachable in memory — which is why this test builds the
			// objects by hand instead of going through envtest.
			name: "malformed selector matches nothing",
			selector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key: "team", Operator: metav1.LabelSelectorOperator("Bogus"),
			}}},
			hcNamespace: "mine",
			hcLabels:    map[string]string{"team": "a"},
			want:        false,
		},
		{
			name:        "allowlist excludes the namespace",
			selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"team": "a"}},
			namespaces:  []string{"other"},
			hcNamespace: "mine",
			hcLabels:    map[string]string{"team": "a"},
			want:        false,
		},
		{
			name:               "denylist excludes the namespace",
			selector:           &metav1.LabelSelector{MatchLabels: map[string]string{"team": "a"}},
			excludedNamespaces: []string{"mine"},
			hcNamespace:        "mine",
			hcLabels:           map[string]string{"team": "a"},
			want:               false,
		},
		{
			name:               "allow wins over exclude",
			selector:           &metav1.LabelSelector{MatchLabels: map[string]string{"team": "a"}},
			namespaces:         []string{"mine"},
			excludedNamespaces: []string{"mine"},
			hcNamespace:        "mine",
			hcLabels:           map[string]string{"team": "a"},
			want:               true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ch := &fathomv1alpha1.ClusterHealth{
				Spec: fathomv1alpha1.ClusterHealthSpec{
					Selector:           tc.selector,
					Namespaces:         tc.namespaces,
					ExcludedNamespaces: tc.excludedNamespaces,
				},
			}
			hc := &fathomv1alpha1.HealthCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "hc", Namespace: tc.hcNamespace, Labels: tc.hcLabels},
			}
			if got := clusterHealthSelectsHealthCheck(ch, hc); got != tc.want {
				t.Fatalf("clusterHealthSelectsHealthCheck() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestHealthCheckEventHandler drives the installed handler rather than the
// mapper. handler.Funcs treats an unset callback as a silent no-op, so a
// dropped verb compiles and passes every mapper-level test; only this one
// catches it.
func TestHealthCheckEventHandler(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	ch := &fathomv1alpha1.ClusterHealth{
		ObjectMeta: metav1.ObjectMeta{Name: "ch-watch"},
		Spec: fathomv1alpha1.ClusterHealthSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "a"}},
		},
	}
	matching := &fathomv1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "hc", Namespace: "default", Labels: map[string]string{"team": "a"}},
	}
	other := &fathomv1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "hc", Namespace: "default", Labels: map[string]string{"team": "z"}},
	}

	tests := []struct {
		name string
		fire func(context.Context, handler.EventHandler, workqueue.TypedRateLimitingInterface[reconcile.Request])
		want int
	}{
		{
			name: "create enqueues the matching ClusterHealth",
			fire: func(ctx context.Context, h handler.EventHandler, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Create(ctx, event.CreateEvent{Object: matching}, q)
			},
			want: 1,
		},
		{
			name: "delete enqueues so the child leaves status.children",
			fire: func(ctx context.Context, h handler.EventHandler, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Delete(ctx, event.DeleteEvent{Object: matching}, q)
			},
			want: 1,
		},
		{
			name: "generic enqueues",
			fire: func(ctx context.Context, h handler.EventHandler, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Generic(ctx, event.GenericEvent{Object: matching}, q)
			},
			want: 1,
		},
		{
			// #148: only the old object matches, so a new-object-only mapper
			// would leave the queue empty and the rollup stale.
			name: "update enqueues when only the old object matched",
			fire: func(ctx context.Context, h handler.EventHandler, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Update(ctx, event.UpdateEvent{ObjectOld: matching, ObjectNew: other}, q)
			},
			want: 1,
		},
		{
			name: "update enqueues nothing when neither side matches",
			fire: func(ctx context.Context, h handler.EventHandler, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Update(ctx, event.UpdateEvent{ObjectOld: other, ObjectNew: other}, q)
			},
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &ClusterHealthReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ch).Build(),
				Scheme: scheme,
			}
			q := workqueue.NewTypedRateLimitingQueue[reconcile.Request](
				workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer q.ShutDown()

			tc.fire(context.Background(), r.healthCheckEventHandler(), q)

			if got := q.Len(); got != tc.want {
				t.Fatalf("queue length = %d, want %d", got, tc.want)
			}
		})
	}
}
