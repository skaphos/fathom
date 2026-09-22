/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
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

func primeDiscovery(t *testing.T, guard *execution.Guard, path, body string) int {
	t.Helper()
	transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	req, err := http.NewRequest(http.MethodGet, "https://cluster"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("prime guarded discovery: %v", err)
	}
	_ = response.Body.Close()
	return len(body)
}

func mappedRuntimeGuard(t *testing.T, b *execution.Budget, cluster bool) *execution.Guard {
	t.Helper()
	guard := runtimeGuard(t, b, cluster)
	primeDiscovery(t, guard, "/api/v1", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true}]}`)
	return guard
}

// Impersonation hands compiled checks a full client.Client, writers included, so
// the guard's method check is the only thing keeping a runtime definition
// read-only. Every write verb must be refused before any I/O, with the scope
// reason a budget error cannot impersonate.
func TestTransportRejectsUnauthorizedRoutesBeforeIO(t *testing.T) {
	type routeCase struct{ method, path string }
	cases := []routeCase{}
	for _, path := range []string{"/api/v1/namespaces/other/configmaps/config", "/api/v1/nodes/node", "/api/v1/namespaces/allowed/pods/pod/exec", "/api/v1/namespaces/allowed/configmaps?watch=true", "/api/v1/namespaces/allowed/configmaps/%2e%2e", "/healthz",
		// Every path segment and query is validated before a request exists, so
		// a malformed group, version, resource, name or query is a scope denial
		// rather than something the API server gets to interpret.
		"/api/V1/namespaces/allowed/configmaps", "/api/v1-beta/namespaces/allowed/configmaps", "/apis/Bad_Group/v1/widgets", "/apis/apps/V1/deployments", "/api/v1/namespaces/allowed/-configmaps", "/api/v1/namespaces/allowed/configmaps-", "/api/v1/namespaces/allowed/config_maps", "/api/v1/namespaces/allowed/config.maps", "/api/v1/namespaces/allowed/configmaps/Bad_Name", "/api/v1/namespaces/allowed/configmaps?%zz", "/api/v1/namespaces/allowed/configmaps?limit=1&limit=2"} {
		cases = append(cases, routeCase{http.MethodGet, path})
	}
	// An otherwise fully authorized target: only the verb makes these illegal.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		cases = append(cases, routeCase{method, "/api/v1/namespaces/allowed/configmaps/config"})
		cases = append(cases, routeCase{method, "/api/v1/namespaces/allowed/configmaps"})
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			defer close()
			guard := runtimeGuard(t, b, false)
			called := false
			transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, nil }))
			req, err := http.NewRequest(tc.method, "https://cluster"+tc.path, strings.NewReader(`{"kind":"ConfigMap"}`))
			if err != nil {
				t.Fatal(err)
			}
			_, err = transport.RoundTrip(req)
			if err == nil || called {
				t.Fatalf("unauthorized request escaped: called=%v err=%v", called, err)
			}
			if !strings.Contains(err.Error(), "ScopeDenied") {
				t.Fatalf("want ScopeDenied, got %v", err)
			}
		})
	}
}

func TestTransportAllowsHyphenatedResourcePlural(t *testing.T) {
	b, close := execution.NewBudget(context.Background(), time.Minute)
	defer close()
	called := false
	guard := runtimeGuard(t, b, false)
	primeDiscovery(t, guard, "/api/v1", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"policy-rules","kind":"ConfigMap","namespaced":true}]}`)
	transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"items":[]}`))}, nil
	}))
	req, err := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/policy-rules", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if !called {
		t.Fatal("hyphenated resource read did not reach the delegated API transport")
	}
}

func TestTransportAllowsOnlyResourcesMappedToDeclaredKinds(t *testing.T) {
	t.Run("declared resource succeeds and undeclared peer is denied before I/O", func(t *testing.T) {
		b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
		defer closeBudget()
		calls := 0
		transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			body := `{"kind":"ConfigMap","metadata":{"name":"config"}}`
			if r.URL.Path == "/api/v1" {
				body = `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true},{"name":"secrets","kind":"Secret","namespaced":true}]}`
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}))
		request := func(path string) error {
			req, err := http.NewRequest(http.MethodGet, "https://cluster"+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			return err
		}
		if err := request("/api/v1"); err != nil {
			t.Fatalf("discover declared resource: %v", err)
		}
		if err := request("/api/v1/namespaces/allowed/configmaps/config"); err != nil {
			t.Fatalf("read declared resource: %v", err)
		}
		before := calls
		if err := request("/api/v1/namespaces/allowed/secrets/secret"); err == nil || !strings.Contains(err.Error(), "ScopeDenied") {
			t.Fatalf("undeclared same-GV resource was not denied: %v", err)
		}
		if calls != before {
			t.Fatal("undeclared same-GV resource reached the API transport")
		}
	})

	for _, tc := range []struct {
		name, first, second string
	}{
		{
			name:  "ambiguous aliases in one response",
			first: `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true},{"name":"configs","kind":"ConfigMap","namespaced":true}]}`,
		},
		{
			name:   "mapping changes during the run",
			first:  `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true}]}`,
			second: `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configs","kind":"ConfigMap","namespaced":true}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
			defer closeBudget()
			body := tc.first
			transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			discover := func() error {
				req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1", nil)
				response, err := transport.RoundTrip(req)
				if response != nil {
					_ = response.Body.Close()
				}
				return err
			}
			firstErr := discover()
			if tc.second == "" {
				if firstErr == nil || !strings.Contains(firstErr.Error(), "ScopeDenied") {
					t.Fatalf("ambiguous discovery was not denied: %v", firstErr)
				}
				return
			}
			if firstErr != nil {
				t.Fatalf("initial discovery: %v", firstErr)
			}
			body = tc.second
			if err := discover(); err == nil || !strings.Contains(err.Error(), "ScopeDenied") {
				t.Fatalf("changed discovery mapping was not denied: %v", err)
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
			transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
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
	declared := `{"apiVersion":"v1","kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true}]}`
	for _, tc := range []struct {
		name, path, body string
		compressed       bool
		denied           bool
		reason           string
	}{
		{name: "empty page", path: "/api/v1/namespaces/allowed/configmaps", body: `{"apiVersion":"v1","kind":"ConfigMapList","items":[]}`},
		{name: "declared discovery scope", path: "/api/v1", body: declared},
		{name: "compressed discovery body", path: "/api/v1", body: declared, compressed: true},
		{name: "mismatched discovery scope", path: "/api/v1", body: `{"apiVersion":"v1","kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":false}]}`, denied: true, reason: "ScopeDenied"},
		{name: "discovery omits the namespaced flag", path: "/api/v1", body: `{"apiVersion":"v1","kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap"}]}`, denied: true, reason: "ScopeDenied"},
		// Root and group routes carry no groupVersion, so they are bounded and
		// identity-scoped but have no declaration to confirm.
		{name: "root api discovery", path: "/api", body: `{"kind":"APIVersions","versions":["v1"]}`},
		{name: "root apis discovery", path: "/apis", body: `{"kind":"APIGroupList","groups":[]}`},
		{name: "group discovery", path: "/apis/apps", body: `{"kind":"APIGroup","name":"apps"}`},
		// A kind nobody declared is skipped, not denied: the guard only confirms
		// the scopes the definition actually depends on.
		{name: "undeclared kind is skipped", path: "/apis/apps/v1", body: `{"kind":"APIResourceList","groupVersion":"apps/v1","resources":[{"name":"deployments","kind":"Deployment","namespaced":false}]}`},
		{name: "unexpected discovery kind", path: "/api/v1", body: `{"kind":"Status","groupVersion":"v1","resources":[]}`, denied: true, reason: "unexpected resource discovery response"},
		{name: "unexpected discovery groupVersion", path: "/api/v1", body: `{"kind":"APIResourceList","groupVersion":"apps/v1","resources":[]}`, denied: true, reason: "unexpected resource discovery response"},
		{name: "discovery resources not a list", path: "/api/v1", body: `{"kind":"APIResourceList","groupVersion":"v1","resources":5}`, denied: true, reason: "invalid discovery resources"},
		{name: "discovery resource not an object", path: "/api/v1", body: `{"kind":"APIResourceList","groupVersion":"v1","resources":[5]}`, denied: true, reason: "invalid discovery resource"},
		{name: "discovery without resources", path: "/api/v1", body: `{"kind":"APIResourceList","groupVersion":"v1","resources":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			defer close()
			guard := runtimeGuard(t, b, true)
			if tc.path == "/api/v1/namespaces/allowed/configmaps" {
				primeDiscovery(t, guard, "/api/v1", declared)
			}
			payload, headers := helperCPayload(t, tc.body, tc.compressed)
			transport := guard.Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(tc.path, "configmaps") && r.URL.Query().Get("limit") != "100" {
					t.Error("page size not bounded")
				}
				return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(bytes.NewReader(payload))}, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster"+tc.path, nil)
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if (err != nil) != tc.denied {
				t.Fatalf("denied=%v err=%v", tc.denied, err)
			}
			if tc.denied && !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("want %q, got %v", tc.reason, err)
			}
		})
	}
}

func TestTransportTargetObjectAndPageBounds(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		objects          int
		denied           bool
		reason           string
	}{
		{name: "nodes at cap", path: "/api/v1/namespaces/allowed/configmaps/config", body: `{"kind":"ConfigMap","value":` + testutil.JSONNodes(limits.MaxObjectNodes-2) + `}`},
		{name: "nodes over cap", path: "/api/v1/namespaces/allowed/configmaps/config", body: `{"kind":"ConfigMap","value":` + testutil.JSONNodes(limits.MaxObjectNodes-1) + `}`, denied: true, reason: "InputLimitExceeded"},
		{name: "depth at cap", path: "/api/v1/namespaces/allowed/configmaps/config", body: `{"value":` + testutil.JSONDepth(limits.MaxObjectDepth-1) + `}`},
		{name: "depth over cap", path: "/api/v1/namespaces/allowed/configmaps/config", body: `{"value":` + testutil.JSONDepth(limits.MaxObjectDepth) + `}`, denied: true, reason: "InputLimitExceeded"},
		{name: "page at cap", path: "/api/v1/namespaces/allowed/configmaps", body: `{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 100), ",") + `]}`},
		{name: "page over cap", path: "/api/v1/namespaces/allowed/configmaps", body: `{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 101), ",") + `]}`, denied: true, reason: "API page exceeds object limit"},
		// A page that would push the run past 1,000 objects is refused by the
		// object charge itself, with no continuation token in sight: the reason
		// distinguishes it from the at-cap continuation rejection below.
		{name: "run object cap exceeded", path: "/api/v1/namespaces/allowed/configmaps", body: `{"items":[{},{}]}`, objects: limits.MaxRunObjects - 1, denied: true, reason: "object limit " + strconv.Itoa(limits.MaxRunObjects) + " exceeded"},
		{name: "run object cap reached exactly", path: "/api/v1/namespaces/allowed/configmaps", body: `{"items":[{}]}`, objects: limits.MaxRunObjects - 1},
		{name: "run object cap exceeded by a single target", path: "/api/v1/namespaces/allowed/configmaps/config", body: `{"kind":"ConfigMap"}`, objects: limits.MaxRunObjects, denied: true, reason: "object limit " + strconv.Itoa(limits.MaxRunObjects) + " exceeded"},
		// Items and the surrounding envelope are traversed separately, so an
		// oversized object hiding in either one is still rejected.
		{name: "list item over node cap", path: "/api/v1/namespaces/allowed/configmaps", body: `{"items":[{"value":` + testutil.JSONNodes(limits.MaxObjectNodes) + `}]}`, denied: true, reason: "target object exceeds node/depth limit"},
		{name: "list envelope over node cap", path: "/api/v1/namespaces/allowed/configmaps", body: `{"items":[],"value":` + testutil.JSONNodes(limits.MaxObjectNodes) + `}`, denied: true, reason: "target object exceeds node/depth limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			defer close()
			guard := mappedRuntimeGuard(t, b, false)
			if tc.objects > 0 {
				// Guarded discovery consumes one object from the same run cap.
				if err := b.ChargeObjects(tc.objects - 1); err != nil {
					t.Fatal(err)
				}
			}
			transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
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
			if tc.denied && !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("want %q, got %v", tc.reason, err)
			}
		})
	}
}

func TestTransportNeverTruncatesAtContinuationCap(t *testing.T) {
	b, close := execution.NewBudget(context.Background(), time.Minute)
	defer close()
	guard := mappedRuntimeGuard(t, b, false)
	if err := b.ChargeObjects(limits.MaxRunObjects - limits.MaxPageObjects - 1); err != nil {
		t.Fatal(err)
	}
	body := `{"metadata":{"continue":"remaining"},"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, limits.MaxPageObjects), ",") + `]}`
	transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps", nil)
	response, err := transport.RoundTrip(req)
	if err == nil || response != nil {
		t.Fatalf("partial list accepted: response=%v err=%v", response, err)
	}
	if !strings.Contains(err.Error(), "WorkLimitExceeded") || !strings.Contains(err.Error(), "continuation remains at object cap") {
		t.Fatalf("want the at-cap continuation reason, got %v", err)
	}
}

// Both retry triggers are bounded: a throttled 429 and a server-side 5xx feed the
// same per-URL counter, and exhaustion is an execution failure, not a silent stop.
func TestTransportLimitsHiddenRetries(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			b, close := execution.NewBudget(context.Background(), time.Minute)
			defer close()
			calls := 0
			transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"0"}}, Body: io.NopCloser(strings.NewReader(`{"kind":"Status"}`))}, nil
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
				if err != nil && !strings.Contains(err.Error(), "WorkLimitExceeded") {
					t.Fatalf("want WorkLimitExceeded, got %v", err)
				}
			}
			if calls != 3 {
				t.Fatalf("attempts=%d want initial plus two retries", calls)
			}
		})
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
	transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
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

func TestIndependentRuntimeTransportsShareRequestRate(t *testing.T) {
	first, closeFirst := execution.NewBudget(context.Background(), 0)
	defer closeFirst()
	second, closeSecond := execution.NewBudget(context.Background(), 0)
	defer closeSecond()
	next := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kind":"ConfigMap"}`))}, nil
	})
	transports := []http.RoundTripper{mappedRuntimeGuard(t, first, false).Wrap(next), mappedRuntimeGuard(t, second, false).Wrap(next)}
	started := time.Now()
	for i := 0; i < 2*limits.RequestBurst; i++ {
		request, err := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := transports[i%2].RoundTrip(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Two independent burst-20 buckets would permit all 40 requests immediately.
	// One shared bucket needs at least two seconds, even when initially full.
	minimum := time.Duration(limits.RequestBurst) * time.Second / time.Duration(limits.RequestsPerSecond)
	if elapsed := time.Since(started); elapsed < minimum-100*time.Millisecond {
		t.Fatalf("per-transport buckets bypassed shared rate: %s", elapsed)
	}
}

// helperCPayload renders a body exactly as the API server would put it on the
// wire, so compressed and identity-encoded cases share one table.
func helperCPayload(t *testing.T, body string, compressed bool) ([]byte, http.Header) {
	t.Helper()
	if !compressed {
		return []byte(body), http.Header{}
	}
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes(), http.Header{"Content-Encoding": []string{"gzip"}}
}

// helperCGuard covers the helper kinds contracts/payloads.md allows a primary
// check to reach: pods in a second binding namespace, EndpointSlices, and
// cluster-scoped CRD resolution behind allowClusterScoped.
func helperCGuard(t *testing.T, b *execution.Budget, cluster bool) *execution.Guard {
	t.Helper()
	g, err := execution.NewGuard(b, api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{"allowed", "second"}, AllowClusterScoped: cluster}, map[schema.GroupVersionKind]bool{
		{Group: "", Version: "v1", Kind: "ConfigMap"}:                                    true,
		{Group: "", Version: "v1", Kind: "Pod"}:                                          true,
		{Group: "discovery.k8s.io", Version: "v1", Kind: "EndpointSlice"}:                true,
		{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func primeHelperDiscovery(t *testing.T, guard *execution.Guard, path string) int {
	t.Helper()
	switch {
	case strings.HasPrefix(path, "/api/v1/"):
		return primeDiscovery(t, guard, "/api/v1", `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true},{"name":"pods","kind":"Pod","namespaced":true}]}`)
	case strings.HasPrefix(path, "/apis/discovery.k8s.io/v1/"):
		return primeDiscovery(t, guard, "/apis/discovery.k8s.io/v1", `{"kind":"APIResourceList","groupVersion":"discovery.k8s.io/v1","resources":[{"name":"endpointslices","kind":"EndpointSlice","namespaced":true}]}`)
	case strings.HasPrefix(path, "/apis/apiextensions.k8s.io/v1/"):
		return primeDiscovery(t, guard, "/apis/apiextensions.k8s.io/v1", `{"kind":"APIResourceList","groupVersion":"apiextensions.k8s.io/v1","resources":[{"name":"customresourcedefinitions","kind":"CustomResourceDefinition","namespaced":false}]}`)
	default:
		t.Fatalf("no discovery fixture for %q", path)
		return 0
	}
}

func chargeResponseBytes(t *testing.T, b *execution.Budget, bytes int) {
	t.Helper()
	for bytes > 0 {
		chunk := min(bytes, limits.MaxResponseBytes)
		if err := b.ChargeResponse(chunk); err != nil {
			t.Fatal(err)
		}
		bytes -= chunk
	}
}

// helperCTornBody delivers part of a body and then fails, the way a connection
// reset mid-response does.
type helperCTornBody struct{ remaining int }

func (r *helperCTornBody) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, errors.New("connection reset by peer")
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}

func (r *helperCTornBody) Close() error { return nil }

// helperCItems builds a page body with n empty objects and an optional token.
func helperCItems(n int, token string) string {
	metadata := `{}`
	if token != "" {
		metadata = `{"continue":"` + token + `"}`
	}
	return `{"kind":"ConfigMapList","metadata":` + metadata + `,"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, n), ",") + `]}`
}

// contracts/runtime.md: explicit scope, budget and discovery expectations are
// mandatory, and the namespace allowlist is exact, deduplicated and at most 32
// DNS-1123 labels. Every missing or malformed input fails closed at construction.
func TestGuardRequiresExplicitScopeAndBudget(t *testing.T) {
	configMaps := map[schema.GroupVersionKind]bool{{Group: "", Version: "v1", Kind: "ConfigMap"}: true}
	labels := func(values []string) []api.DefinitionDNSLabel {
		out := make([]api.DefinitionDNSLabel, 0, len(values))
		for _, v := range values {
			out = append(out, api.DefinitionDNSLabel(v))
		}
		return out
	}
	for _, tc := range []struct {
		name          string
		withoutBudget bool
		scope         api.DefinitionBindingScope
		expected      map[schema.GroupVersionKind]bool
		wantErr       string
	}{
		{name: "namespaced scope", scope: api.DefinitionBindingScope{Namespaces: labels([]string{"allowed"})}, expected: configMaps},
		{name: "cluster scope without namespaces", scope: api.DefinitionBindingScope{AllowClusterScoped: true}, expected: configMaps},
		{name: "namespaces at cap", scope: api.DefinitionBindingScope{Namespaces: labels(testutil.Strings(limits.MaxNamespaces))}, expected: configMaps},
		{name: "namespaces over cap", scope: api.DefinitionBindingScope{Namespaces: labels(testutil.Strings(limits.MaxNamespaces + 1))}, expected: configMaps, wantErr: "AuthorizationUnavailable"},
		{name: "missing budget", withoutBudget: true, scope: api.DefinitionBindingScope{Namespaces: labels([]string{"allowed"})}, expected: configMaps, wantErr: "AuthorizationUnavailable"},
		{name: "missing discovery expectations", scope: api.DefinitionBindingScope{Namespaces: labels([]string{"allowed"})}, expected: map[schema.GroupVersionKind]bool{}, wantErr: "AuthorizationUnavailable"},
		{name: "neither namespaced nor cluster scoped", scope: api.DefinitionBindingScope{}, expected: configMaps, wantErr: "AuthorizationUnavailable"},
		{name: "duplicate namespace", scope: api.DefinitionBindingScope{Namespaces: labels([]string{"allowed", "allowed"})}, expected: configMaps, wantErr: "ScopeDenied"},
		{name: "invalid namespace label", scope: api.DefinitionBindingScope{Namespaces: labels([]string{"Bad_Label"})}, expected: configMaps, wantErr: "ScopeDenied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
			defer closeBudget()
			if tc.withoutBudget {
				b = nil
			}
			guard, err := execution.NewGuard(b, tc.scope, tc.expected)
			if (err != nil) != (tc.wantErr != "") {
				t.Fatalf("want %q, got %v", tc.wantErr, err)
			}
			if tc.wantErr != "" {
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want %q, got %v", tc.wantErr, err)
				}
				if guard != nil {
					t.Fatal("rejected scope still produced a guard")
				}
				return
			}
			if guard == nil {
				t.Fatal("valid scope produced no guard")
			}
		})
	}
}

// T026 names policy overrides: at the transport those are the caller-supplied
// request parameters, which are clamped, preserved or refused, never trusted.
func TestTransportBoundsCallerSuppliedListOptions(t *testing.T) {
	for _, tc := range []struct {
		name, query, wantLimit, reason string
	}{
		{name: "no caller limit", wantLimit: "100"},
		{name: "oversized caller limit is clamped", query: "limit=5000", wantLimit: "100"},
		{name: "smaller caller limit is preserved", query: "limit=5", wantLimit: "5"},
		{name: "zero caller limit is bounded", query: "limit=0", wantLimit: "100"},
		{name: "malformed caller limit", query: "limit=abc", reason: "invalid list limit"},
		{name: "negative caller limit", query: "limit=-1", reason: "invalid list limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
			defer closeBudget()
			called := false
			transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				called = true
				if got := r.URL.Query().Get("limit"); got != tc.wantLimit {
					t.Errorf("outbound limit=%q want %q", got, tc.wantLimit)
				}
				// The decoder only understands JSON, so a caller cannot
				// negotiate protobuf through the guard.
				if got := r.Header.Get("Accept"); got != "application/json" {
					t.Errorf("outbound Accept=%q", got)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"items":[]}`))}, nil
			}))
			url := "https://cluster/api/v1/namespaces/allowed/configmaps"
			if tc.query != "" {
				url += "?" + tc.query
			}
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Accept", "application/vnd.kubernetes.protobuf")
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if tc.reason != "" {
				if err == nil || !strings.Contains(err.Error(), "ScopeDenied") || !strings.Contains(err.Error(), tc.reason) {
					t.Fatalf("want ScopeDenied %q, got %v", tc.reason, err)
				}
				if called {
					t.Fatal("malformed list option reached the API")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("authorized list never reached the API")
			}
		})
	}
}

// contracts/payloads.md: helper reads (pods, EndpointSlices, CRD resolution) use
// the same identity and must be independently authorized, including the
// cluster-scoped CRD lookup a namespaced primary check legitimately needs.
func TestTransportHelperReadsShareIdentityAndScope(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		cluster          bool
		denied           bool
		reachesAPI       bool
		reason           string
	}{
		{name: "cluster-scoped CRD helper", path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com", cluster: true, body: `{"kind":"CustomResourceDefinition"}`, reachesAPI: true},
		{name: "cluster-scoped CRD helper without cluster authority", path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com", body: `{"kind":"CustomResourceDefinition"}`, denied: true, reason: "cluster-scoped read is not authorized"},
		{name: "pod helper in a second binding namespace", path: "/api/v1/namespaces/second/pods", body: `{"kind":"PodList","items":[]}`, reachesAPI: true},
		{name: "pod helper outside the binding namespaces", path: "/api/v1/namespaces/unlisted/pods", body: `{"kind":"PodList","items":[]}`, denied: true, reason: "request namespace is not authorized"},
		{name: "endpointslice helper discovery matches declaration", path: "/apis/discovery.k8s.io/v1", body: `{"kind":"APIResourceList","groupVersion":"discovery.k8s.io/v1","resources":[{"name":"endpointslices","kind":"EndpointSlice","namespaced":true}]}`, reachesAPI: true},
		{name: "endpointslice helper discovery contradicts declaration", path: "/apis/discovery.k8s.io/v1", body: `{"kind":"APIResourceList","groupVersion":"discovery.k8s.io/v1","resources":[{"name":"endpointslices","kind":"EndpointSlice","namespaced":false}]}`, denied: true, reachesAPI: true, reason: "ScopeDenied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
			defer closeBudget()
			called := false
			guard := helperCGuard(t, b, tc.cluster)
			if !strings.HasSuffix(tc.path, "/v1") && tc.reachesAPI {
				primeHelperDiscovery(t, guard, tc.path)
			}
			transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				called = true
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
			if tc.denied && !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("want %q, got %v", tc.reason, err)
			}
			if called != tc.reachesAPI {
				t.Fatalf("reached API=%v want %v", called, tc.reachesAPI)
			}
		})
	}
	t.Run("helper traffic is charged to the run request budget", func(t *testing.T) {
		b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
		defer closeBudget()
		guard := helperCGuard(t, b, true)
		primeHelperDiscovery(t, guard, "/api/v1/namespaces/second/pods")
		primeHelperDiscovery(t, guard, "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com")
		for i := 0; i < limits.MaxRunRequests-3; i++ {
			if err := b.ChargeRequest(); err != nil {
				t.Fatal(err)
			}
		}
		transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kind":"PodList","items":[]}`))}, nil
		}))
		req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/second/pods", nil)
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		req, _ = http.NewRequest(http.MethodGet, "https://cluster/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com", nil)
		if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "WorkLimitExceeded") {
			t.Fatalf("helper read escaped the request budget: %v", err)
		}
	})
	t.Run("helper traffic is charged to the run byte budget", func(t *testing.T) {
		b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
		defer closeBudget()
		guard := helperCGuard(t, b, false)
		discoveryBytes := primeHelperDiscovery(t, guard, "/api/v1/namespaces/second/pods")
		chargeResponseBytes(t, b, limits.MaxRunResponseBytes-discoveryBytes)
		transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kind":"PodList","items":[]}`))}, nil
		}))
		req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/second/pods", nil)
		if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "ResponseLimitExceeded") {
			t.Fatalf("helper read escaped the cumulative byte budget: %v", err)
		}
	})
}

// A compressed page must reach the decoder as plain bounded bytes: the encoding
// headers are rewritten so client-go cannot decompress a second time.
func TestTransportDeliversDecompressedBodies(t *testing.T) {
	atCap := `{"padding":"` + strings.Repeat("x", limits.MaxResponseBytes-len(`{"padding":""}`)) + `"}`
	for _, tc := range []struct {
		name, body string
		compressed bool
	}{
		{name: "compressed page under cap", body: `{"apiVersion":"v1","kind":"ConfigMap","data":{"key":"value"}}`, compressed: true},
		{name: "compressed body decoding exactly to the per-request cap", body: atCap, compressed: true},
		{name: "identity encoding", body: `{"apiVersion":"v1","kind":"ConfigMap"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
			defer closeBudget()
			payload, headers := helperCPayload(t, tc.body, tc.compressed)
			transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: headers.Clone(), Body: io.NopCloser(bytes.NewReader(payload))}, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
			response, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if got := response.Header.Get("Content-Encoding"); got != "" {
				t.Fatalf("Content-Encoding survived decompression: %q", got)
			}
			if !response.Uncompressed {
				t.Fatal("decoded response is not marked uncompressed")
			}
			if response.ContentLength != int64(len(tc.body)) || response.Header.Get("Content-Length") != strconv.Itoa(len(tc.body)) {
				t.Fatalf("length=%d header=%q want %d", response.ContentLength, response.Header.Get("Content-Length"), len(tc.body))
			}
			delivered, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(delivered) != tc.body {
				t.Fatalf("delivered %d bytes, want the %d decoded bytes", len(delivered), len(tc.body))
			}
		})
	}
}

// Every way a body can be undeliverable is an explicit bounded failure; none of
// them may reach the evaluator as a partial page.
func TestTransportRejectsUndecodableResponses(t *testing.T) {
	for _, tc := range []struct {
		name, path, encoding, body, reason string
		torn, missingTransport             bool
	}{
		{name: "corrupt gzip stream", path: "/api/v1/namespaces/allowed/configmaps/config", encoding: "gzip", body: "not a gzip stream", reason: "invalid compressed API response"},
		{name: "deflate encoding", path: "/api/v1/namespaces/allowed/configmaps/config", encoding: "deflate", body: `{}`, reason: "unsupported API response encoding"},
		{name: "brotli encoding", path: "/api/v1/namespaces/allowed/configmaps/config", encoding: "br", body: `{}`, reason: "unsupported API response encoding"},
		{name: "body torn mid-read", path: "/api/v1/namespaces/allowed/configmaps/config", torn: true, reason: "cannot decode API response"},
		{name: "missing inner transport", path: "/api/v1/namespaces/allowed/configmaps/config", missingTransport: true, reason: "AuthorizationUnavailable"},
		{name: "invalid JSON", path: "/api/v1/namespaces/allowed/configmaps/config", body: `{`, reason: "invalid JSON API response"},
		{name: "trailing data", path: "/api/v1/namespaces/allowed/configmaps/config", body: `{} {}`, reason: "trailing API response data"},
		{name: "response is not an object", path: "/api/v1/namespaces/allowed/configmaps/config", body: `[]`, reason: "API response must be an object"},
		{name: "collection without items", path: "/api/v1/namespaces/allowed/configmaps", body: `{"kind":"ConfigMapList"}`, reason: "collection response has no items"},
		{name: "items is not an array", path: "/api/v1/namespaces/allowed/configmaps", body: `{"items":5}`, reason: "invalid collection items"},
		{name: "item is not an object", path: "/api/v1/namespaces/allowed/configmaps", body: `{"items":[5]}`, reason: "collection item is not an object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
			defer closeBudget()
			guard := mappedRuntimeGuard(t, b, false)
			var transport http.RoundTripper
			if tc.missingTransport {
				transport = guard.Wrap(nil)
			} else {
				transport = guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
					headers := http.Header{}
					if tc.encoding != "" {
						headers.Set("Content-Encoding", tc.encoding)
					}
					body := io.NopCloser(strings.NewReader(tc.body))
					if tc.torn {
						body = &helperCTornBody{remaining: 64}
					}
					return &http.Response{StatusCode: 200, Header: headers, Body: body}, nil
				}))
			}
			req, _ := http.NewRequest(http.MethodGet, "https://cluster"+tc.path, nil)
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("want %q, got %v", tc.reason, err)
			}
		})
	}
	t.Run("null items is an empty page", func(t *testing.T) {
		b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
		defer closeBudget()
		transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kind":"ConfigMapList","items":null}`))}, nil
		}))
		req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps", nil)
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	})
}

// Below the run object cap a continuation is ordinary paging: the token reaches
// the caller untouched and each page is charged as it arrives.
func TestTransportForwardsContinuationBelowCap(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
	defer closeBudget()
	// Guarded discovery charges one object; precharge one fewer so the paging
	// assertion retains its exact 950-object boundary.
	if err := b.ChargeObjects(799); err != nil {
		t.Fatal(err)
	}
	pages := []string{helperCItems(limits.MaxPageObjects, "next"), helperCItems(50, "")}
	attempt := 0
	transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Errorf("page %d limit=%q", attempt, got)
		}
		if got := r.URL.Query().Get("continue"); (attempt == 1) != (got == "next") {
			t.Errorf("page %d continue=%q", attempt, got)
		}
		body := pages[attempt]
		attempt++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps", nil)
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Metadata struct {
			Continue string `json:"continue"`
		} `json:"metadata"`
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if page.Metadata.Continue != "next" || len(page.Items) != limits.MaxPageObjects {
		t.Fatalf("continuation token=%q items=%d", page.Metadata.Continue, len(page.Items))
	}
	req, _ = http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps?continue=next", nil)
	response, err = transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if attempt != 2 {
		t.Fatalf("pages fetched=%d", attempt)
	}
	if got := b.Objects(); got != 950 {
		t.Fatalf("objects charged=%d want 950 across both pages", got)
	}
}

// T027: the 100-request run budget covers discovery and retries, not just the
// reads a definition explicitly declares.
func TestTransportChargesRetriesAndDiscoveryToRunBudget(t *testing.T) {
	prechargeAllButOne := func(t *testing.T, b *execution.Budget) {
		t.Helper()
		for i := 0; i < limits.MaxRunRequests-1; i++ {
			if err := b.ChargeRequest(); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Run("discovery consumes a run request", func(t *testing.T) {
		b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
		defer closeBudget()
		prechargeAllButOne(t, b)
		transport := runtimeGuard(t, b, true).Wrap(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := `{"kind":"ConfigMap"}`
			if r.URL.Path == "/api/v1" {
				body = `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","kind":"ConfigMap","namespaced":true}]}`
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}))
		req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1", nil)
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		req, _ = http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
		if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "WorkLimitExceeded") {
			t.Fatalf("discovery was not charged to the run budget: %v", err)
		}
	})
	t.Run("a retry consumes a run request", func(t *testing.T) {
		b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
		defer closeBudget()
		guard := mappedRuntimeGuard(t, b, false)
		for i := 0; i < limits.MaxRunRequests-2; i++ {
			if err := b.ChargeRequest(); err != nil {
				t.Fatal(err)
			}
		}
		calls := 0
		transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kind":"Status"}`))}, nil
		}))
		req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		req, _ = http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
		if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "WorkLimitExceeded") {
			t.Fatalf("retry was not charged to the run budget: %v", err)
		}
		if calls != 1 {
			t.Fatalf("attempts reaching the API=%d want 1", calls)
		}
	})
	t.Run("a success clears the per-request retry counter", func(t *testing.T) {
		b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
		defer closeBudget()
		statuses := []int{503, 503, 200, 503, 503, 503}
		calls := 0
		transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
			status := statuses[calls]
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kind":"ConfigMap"}`))}, nil
		}))
		for i := 0; i <= len(statuses); i++ {
			req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if (err != nil) != (i == len(statuses)) {
				t.Fatalf("attempt %d err=%v", i, err)
			}
		}
		if calls != len(statuses) {
			t.Fatalf("attempts reaching the API=%d want %d; the success did not reset the retry counter", calls, len(statuses))
		}
	})
}

// contracts/runtime.md: within-run failures stop at the first failure, so a run
// that already failed issues no further work and keeps its original reason.
// cRecordingContext records every consultation of the request's own context.
// It is the only thing that separates the two ways RoundTrip can refuse a
// failed run; see TestTransportStopsAfterFirstFailure.
type cRecordingContext struct {
	context.Context
	mu        sync.Mutex
	consulted []string
}

func (c *cRecordingContext) note(method string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consulted = append(c.consulted, method)
}
func (c *cRecordingContext) Deadline() (time.Time, bool) {
	c.note("Deadline")
	return c.Context.Deadline()
}
func (c *cRecordingContext) Done() <-chan struct{} { c.note("Done"); return c.Context.Done() }
func (c *cRecordingContext) Err() error            { c.note("Err"); return c.Context.Err() }
func (c *cRecordingContext) Value(key any) any     { c.note("Value"); return c.Context.Value(key) }
func (c *cRecordingContext) touched() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.consulted...)
}

// A run that has already failed is refused at the budget, before the transport
// does any other work at all.
//
// "No inner I/O, first reason preserved" is necessary but not sufficient to pin
// that: the budget is first-failure-wins, so every later refusal in RoundTrip -
// the request-context check, the route check, the shared rate limiter aborting
// on the run's cancelled context - also returns the identical *Failure without
// ever calling the inner transport. Those assertions alone therefore stay green
// even with the entry check deleted. The request's own context is what tells
// the paths apart: the entry check returns before anything reads it, while
// every path past it must consult it - directly, and again to derive the
// per-request context handed to the limiter.
func TestTransportStopsAfterFirstFailure(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
	defer closeBudget()
	if err := b.Fail("AuthorizationRevoked", "binding disabled mid-run"); err == nil {
		t.Fatal("budget did not record the failure")
	}
	called := false
	transport := runtimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
	spy := &cRecordingContext{Context: req.Context()}
	_, err := transport.RoundTrip(req.WithContext(spy))
	if err == nil || called {
		t.Fatalf("failed run kept working: called=%v err=%v", called, err)
	}
	if !strings.Contains(err.Error(), "AuthorizationRevoked") || !strings.Contains(err.Error(), "binding disabled mid-run") {
		t.Fatalf("first failure was rewritten: %v", err)
	}
	if consulted := spy.touched(); len(consulted) != 0 {
		t.Fatalf("failed run worked past the budget check: request context consulted %v", consulted)
	}
}

// Non-2xx bodies are byte-capped but deliberately not node/depth inspected; see
// the scoping comment on that branch in transport.go. This pins the decision: a
// hostile error body from an aggregated APIService or a conversion webhook is
// bounded and passed through rather than failing the run with a traversal reason.
func TestTransportPassesThroughBoundedErrorBodies(t *testing.T) {
	body := `{"kind":"Status","code":500,"details":` + testutil.JSONNodes(limits.MaxObjectNodes+1) + `}`
	b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
	defer closeBudget()
	transport := mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	delivered, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(delivered) != len(body) {
		t.Fatalf("delivered %d bytes want %d", len(delivered), len(body))
	}
	if b.Objects() != 1 {
		t.Fatalf("error body changed the one-object discovery charge: %d", b.Objects())
	}
	// The byte cap still applies unconditionally, which is what keeps the
	// uninspected body bounded.
	oversized := strings.Repeat("x", limits.MaxResponseBytes+1)
	transport = mappedRuntimeGuard(t, b, false).Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(oversized))}, nil
	}))
	req, _ = http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
	if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "ResponseLimitExceeded") {
		t.Fatalf("error body escaped the byte cap: %v", err)
	}
}

// The discovered resource scope is part of the allowlist. Cluster authority
// permits cluster-scoped helpers, but never turns a namespaced resource into an
// all-namespaces read or a cluster-scoped resource into a namespaced one.
func TestTransportRequiresDiscoveredResourceScope(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		allowed    bool
	}{
		{name: "namespaced resource cannot be listed across all namespaces", path: "/api/v1/configmaps"},
		{name: "cluster resource cannot use a namespace", path: "/apis/apiextensions.k8s.io/v1/namespaces/allowed/customresourcedefinitions/widgets.example.com"},
		{name: "cluster resource with cluster authority", path: "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com", allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), time.Minute)
			defer closeBudget()
			guard := helperCGuard(t, b, true)
			primeHelperDiscovery(t, guard, "/api/v1/namespaces/allowed/configmaps/config")
			primeHelperDiscovery(t, guard, "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com")
			calls := 0
			transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kind":"CustomResourceDefinition"}`))}, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster"+tc.path, nil)
			response, err := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if tc.allowed {
				if err != nil || calls != 1 {
					t.Fatalf("allowed cluster resource: calls=%d err=%v", calls, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "ScopeDenied") || calls != 0 {
				t.Fatalf("resource scope mismatch accepted: calls=%d err=%v", calls, err)
			}
		})
	}
}
