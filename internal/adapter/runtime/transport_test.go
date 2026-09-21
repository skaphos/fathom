/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/internal/adapter/runtime/testutil"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func runtimeGuard(t *testing.T, b *execution.Budget, cluster bool) *execution.Guard {
	t.Helper()
	g, err := execution.NewGuard(b, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"allowed"}, AllowClusterScoped: cluster}, map[schema.GroupVersionKind]bool{{Group: "", Version: "v1", Kind: "ConfigMap"}: true})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestTransportRejectsUnauthorizedRoutesBeforeIO(t *testing.T) {
	for _, path := range []string{"/api/v1/namespaces/other/configmaps/config", "/api/v1/nodes/node", "/api/v1/namespaces/allowed/pods/pod/exec", "/api/v1/namespaces/allowed/configmaps?watch=true", "/api/v1/namespaces/allowed/configmaps/%2e%2e", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			defer close()
			guard := runtimeGuard(t, b, false)
			called := false
			transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, nil }))
			req, err := http.NewRequest(http.MethodGet, "https://cluster"+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := transport.RoundTrip(req); err == nil || called {
				t.Fatalf("unauthorized request escaped: called=%v err=%v", called, err)
			}
		})
	}
}

func TestTransportBoundsDecodedErrorAndSuccessBodies(t *testing.T) {
	for _, status := range []int{200, 403} {
		for _, compressed := range []bool{false, true} {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			payload := []byte(strings.Repeat("x", limits.MaxResponseBytes+1))
			headers := http.Header{}
			if compressed {
				var buf bytes.Buffer
				writer := gzip.NewWriter(&buf)
				if _, err := writer.Write(payload); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				payload = buf.Bytes()
				headers.Set("Content-Encoding", "gzip")
			}
			transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(bytes.NewReader(payload))}, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
			if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "ResponseLimitExceeded") {
				t.Fatalf("status=%d compressed=%v err=%v", status, compressed, err)
			}
			close()
		}
	}
}

func TestTransportListLimitAndDiscoveryScope(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		denied           bool
	}{
		{"empty page", "/api/v1/namespaces/allowed/configmaps", `{"apiVersion":"v1","kind":"ConfigMapList","items":[]}`, false},
		{"declared discovery scope", "/api/v1", `{"apiVersion":"v1","kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true}]}`, false},
		{"mismatched discovery scope", "/api/v1", `{"apiVersion":"v1","kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":false}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			defer close()
			transport := runtimeGuard(t, b, true).Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(tc.path, "configmaps") && r.URL.Query().Get("limit") != "100" {
					t.Error("page size not bounded")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster"+tc.path, nil)
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if (err != nil) != tc.denied {
				t.Fatalf("denied=%v err=%v", tc.denied, err)
			}
		})
	}
}

func TestTransportTargetObjectAndPageBounds(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		denied           bool
	}{
		{"nodes at cap", "/api/v1/namespaces/allowed/configmaps/config", `{"kind":"ConfigMap","value":` + testutil.JSONNodes(limits.MaxObjectNodes-2) + `}`, false},
		{"nodes over cap", "/api/v1/namespaces/allowed/configmaps/config", `{"kind":"ConfigMap","value":` + testutil.JSONNodes(limits.MaxObjectNodes-1) + `}`, true},
		{"depth at cap", "/api/v1/namespaces/allowed/configmaps/config", `{"value":` + testutil.JSONDepth(limits.MaxObjectDepth-1) + `}`, false},
		{"depth over cap", "/api/v1/namespaces/allowed/configmaps/config", `{"value":` + testutil.JSONDepth(limits.MaxObjectDepth) + `}`, true},
		{"page at cap", "/api/v1/namespaces/allowed/configmaps", `{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 100), ",") + `]}`, false},
		{"page over cap", "/api/v1/namespaces/allowed/configmaps", `{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 101), ",") + `]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			defer close()
			transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster"+tc.path, nil)
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if (err != nil) != tc.denied {
				t.Fatalf("denied=%v err=%v", tc.denied, err)
			}
		})
	}
}

func TestTransportNeverTruncatesAtContinuationCap(t *testing.T) {
	b, close := execution.NewBudget(context.Background(), time.Minute)
	defer close()
	if err := b.ChargeObjects(limits.MaxRunObjects - limits.MaxPageObjects); err != nil {
		t.Fatal(err)
	}
	body := `{"metadata":{"continue":"remaining"},"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, limits.MaxPageObjects), ",") + `]}`
	transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps", nil)
	if response, err := transport.RoundTrip(req); err == nil || response != nil {
		t.Fatalf("partial list accepted: response=%v err=%v", response, err)
	}
}

func TestTransportLimitsHiddenRetries(t *testing.T) {
	b, close := execution.NewBudget(context.Background(), time.Minute)
	defer close()
	calls := 0
	transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": []string{"0"}}, Body: io.NopCloser(strings.NewReader(`{"kind":"Status","code":503}`))}, nil
	}))
	for i := 0; i < 4; i++ {
		req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
		response, err := transport.RoundTrip(req)
		if response != nil {
			_ = response.Body.Close()
		}
		if (err != nil) != (i == 3) {
			t.Fatalf("attempt %d err=%v", i, err)
		}
	}
	if calls != 3 {
		t.Fatalf("attempts=%d want initial plus two retries", calls)
	}
}

func TestTransportHonorsCallerCancellationBeforeIO(t *testing.T) {
	b, close := execution.NewBudget(context.Background(), time.Minute)
	defer close()
	transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("canceled request reached API"); return nil, nil }))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
	if _, err := transport.RoundTrip(req); err != context.Canceled {
		t.Fatalf("cancellation=%v", err)
	}
}

// A body can stall after headers; cancellation must close it to release the
// reader without leaving a helper goroutine or request alive.
func TestTransportCancellationClosesResponseBody(t *testing.T) {
	b, cleanup := execution.NewBudget(context.Background(), time.Minute)
	defer cleanup()
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	entered := make(chan struct{})
	transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		close(entered)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
	finished := make(chan error, 1)
	go func() { _, err := transport.RoundTrip(req); finished <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request never started")
	}
	cancel()
	select {
	case err := <-finished:
		if err != context.Canceled {
			t.Fatalf("got %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled response body remained blocked")
	}
}
