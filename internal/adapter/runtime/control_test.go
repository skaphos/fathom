/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/types"
)

func controlTargets() execution.ControlTargets {
	return execution.ControlTargets{OperatorNamespace: "operator", DefinitionName: "custom", Check: types.NamespacedName{Namespace: "checks", Name: "check"}, LeaseName: "leader"}
}

func serviceAccountControlTargets() execution.ControlTargets {
	targets := controlTargets()
	targets.ServiceAccountName = "reader"
	return targets
}

func TestControlGuardRestrictsManagerReads(t *testing.T) {
	for _, tc := range []struct {
		path    string
		allowed bool
	}{
		{"/apis/fathom.skaphos.io/v1alpha1/addondefinitions/custom", true},
		{"/apis/fathom.skaphos.io/v1alpha1/namespaces/operator/addondefinitionbindings/custom", true},
		{"/apis/fathom.skaphos.io/v1alpha1/namespaces/operator/addondefinitionbindings", true},
		{"/api/v1/namespaces/operator/serviceaccounts/reader", false},
		{"/apis/fathom.skaphos.io/v1alpha1/namespaces/checks/addonchecks/check", true},
		{"/apis/coordination.k8s.io/v1/namespaces/operator/leases/leader", true},
		{"/api/v1/namespaces/operator/configmaps/target", false},
		{"/api/v1/namespaces/checks/serviceaccounts/reader", false},
		{"/api/v1/namespaces/operator/serviceaccounts", false},
		{"/apis/fathom.skaphos.io/v1alpha1/addondefinitions/other", false},
		{"/apis/fathom.skaphos.io/v1alpha1/namespaces/checks/addonchecks/other", false},
		{"/apis/fathom.skaphos.io/v1alpha1/namespaces/checks/addondefinitionbindings", false},
		{"/apis/coordination.k8s.io/v1/namespaces/operator/leases/other", false},
		{"/apis/fathom.skaphos.io/v1alpha1/namespaces/operator/addondefinitionbindings/custom/status", false},
		{"/apis/fathom.skaphos.io/v1alpha1/namespaces/operator/addondefinitionbindings?watch=true", false},
		{"/api/v1", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
			defer closeBudget()
			guard, err := execution.NewControlGuard(b, controlTargets())
			if err != nil {
				t.Fatal(err)
			}
			called := false
			transport := guard.Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				called = true
				if strings.HasSuffix(r.URL.Path, "addondefinitionbindings") && r.URL.Query().Get("limit") != "100" {
					t.Error("binding list is not paginated")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"items":[]}`))}, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster"+tc.path, nil)
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if called != tc.allowed || (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v called=%v err=%v", tc.allowed, called, err)
			}
		})
	}
}

func TestControlGuardAllowsOnlyTheResolvedServiceAccount(t *testing.T) {
	targets := serviceAccountControlTargets()
	b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
	defer closeBudget()
	guard, err := execution.NewControlGuard(b, targets)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		path    string
		allowed bool
	}{
		{"resolved service account", "/api/v1/namespaces/operator/serviceaccounts/reader", true},
		{"unrelated service account", "/api/v1/namespaces/operator/serviceaccounts/other", false},
		{"resolved name in another namespace", "/api/v1/namespaces/other/serviceaccounts/reader", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				called = true
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			}))
			req, err := http.NewRequest(http.MethodGet, "https://cluster"+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if called != tc.allowed || (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v called=%v err=%v", tc.allowed, called, err)
			}
		})
	}
}

func TestControlAndDelegatedRequestsShareBudget(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
	defer closeBudget()
	control, err := execution.NewControlGuard(b, serviceAccountControlTargets())
	if err != nil {
		t.Fatal(err)
	}
	delegated := runtimeGuard(t, b, false)
	for i := 0; i < limits.MaxRunRequests-2; i++ {
		if err := b.ChargeRequest(); err != nil {
			t.Fatal(err)
		}
	}
	next := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/operator/serviceaccounts/reader", nil)
	response, err := control.Wrap(next).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	before, err := delegated.Diagnostics(diagnosticDefinition())
	if err != nil || before.Reason != "NotEvaluated" {
		t.Fatalf("manager read counted as delegated access: %+v %v", before, err)
	}
	primeDiscovery(t, delegated, "/api/v1", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true}]}`)
	req, _ = http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
	if _, err := delegated.Wrap(next).RoundTrip(req); err == nil || !strings.Contains(err.Error(), "WorkLimitExceeded") {
		t.Fatalf("independent request budget: %v", err)
	}
}

func TestControlAndDelegatedRequestsShareResponseBytes(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
	defer closeBudget()
	control, err := execution.NewControlGuard(b, serviceAccountControlTargets())
	if err != nil {
		t.Fatal(err)
	}
	delegated := runtimeGuard(t, b, false)
	discoveryBytes := primeDiscovery(t, delegated, "/api/v1", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true}]}`)
	remaining := limits.MaxRunResponseBytes - limits.MaxResponseBytes - discoveryBytes
	for remaining > 0 {
		charge := min(remaining, limits.MaxResponseBytes)
		if err := b.ChargeResponse(charge); err != nil {
			t.Fatal(err)
		}
		remaining -= charge
	}
	payload := `{"padding":"` + strings.Repeat("x", limits.MaxResponseBytes-len(`{"padding":""}`)) + `"}`
	transport := control.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/operator/serviceaccounts/reader", nil)
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	transport = delegated.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	req, _ = http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
	if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "ResponseLimitExceeded") {
		t.Fatalf("independent byte budget: %v", err)
	}
}

// The shared-deadline test below runs a 50ms budget, so the run deadline is always
// the binding one and the per-request clamp is invisible to it. Control-plane reads
// go through the same Budget.RequestContext as delegated reads, so the 5s cap must
// bind whenever the run has more time left than that.
func TestControlGuardClampsRequestToMaxRequestDuration(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), limits.MaxRunDuration)
	defer closeBudget()
	control, err := execution.NewControlGuard(b, serviceAccountControlTargets())
	if err != nil {
		t.Fatal(err)
	}
	transport := control.Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("control request has no deadline")
		}
		// Read the remaining time rather than measuring from a start stamp: the
		// deadline is fixed at construction, so an over-grant fails however small.
		// The lower bound absorbs scheduling delay under a loaded test binary while
		// staying far tighter than either direction a lost clamp moves it -- the run
		// deadline is 30s out, and halving the cap would land at 2.5s.
		floor := limits.MaxRequestDuration * 3 / 4
		if grants := time.Until(deadline); grants > limits.MaxRequestDuration || grants <= floor {
			t.Errorf("control request grants %v, want (%v, %v]", grants, floor, limits.MaxRequestDuration)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/operator/serviceaccounts/reader", nil)
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("control read: %v", err)
	}
}

func TestControlGuardSharesRemainingDeadline(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), 50*time.Millisecond)
	defer closeBudget()
	control, err := execution.NewControlGuard(b, serviceAccountControlTargets())
	if err != nil {
		t.Fatal(err)
	}
	transport := control.Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		outer, _ := b.Context().Deadline()
		if !ok || deadline.After(outer) {
			t.Error("control request outlives the run")
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/operator/serviceaccounts/reader", nil)
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("expired control request succeeded")
	}
	if b.Err() == nil {
		t.Fatal("control timeout did not cancel the shared run")
	}
}
