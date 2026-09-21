/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	api "github.com/skaphos/fathom/api/v1alpha1"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
)

// This limiter is shared by every runtime transport in this process. Built-in
// adapter traffic does not use it and keeps its existing worker/limiter behavior.
var runtimeRequests = rate.NewLimiter(rate.Limit(limits.RequestsPerSecond), limits.RequestBurst)

// Guard owns immutable binding scope and expected discovery scopes for one run.
// A true expectation means namespaced; false means cluster-scoped.
type Guard struct {
	budget      *Budget
	namespaces  map[string]bool
	cluster     bool
	expected    map[schema.GroupVersionKind]bool
	retryMu     sync.Mutex
	retries     map[string]int
	permissions permissionObservations
}

func NewGuard(b *Budget, scope api.DefinitionBindingScope, expected map[schema.GroupVersionKind]bool) (*Guard, error) {
	if b == nil || len(expected) == 0 || len(scope.Namespaces) > limits.MaxNamespaces || (!scope.AllowClusterScoped && len(scope.Namespaces) == 0) {
		return nil, fmt.Errorf("AuthorizationUnavailable: budget, explicit scope and discovery expectations required")
	}
	g := &Guard{budget: b, namespaces: map[string]bool{}, cluster: scope.AllowClusterScoped, expected: map[schema.GroupVersionKind]bool{}, retries: map[string]int{}}
	for _, ns := range scope.Namespaces {
		if len(validation.IsDNS1123Label(string(ns))) != 0 || g.namespaces[string(ns)] {
			return nil, fmt.Errorf("ScopeDenied: invalid namespace allowlist")
		}
		g.namespaces[string(ns)] = true
	}
	for kind, namespaced := range expected {
		g.expected[kind] = namespaced
	}
	return g, nil
}
func (g *Guard) Wrap(next http.RoundTripper) http.RoundTripper {
	return &guardedTransport{guard: g, next: next}
}

type guardedTransport struct {
	guard   *Guard
	next    http.RoundTripper
	control *ControlTargets
}

func (t *guardedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	g := t.guard
	b := g.budget
	if err := b.Err(); err != nil {
		return nil, err
	}
	if err := request.Context().Err(); err != nil {
		return nil, b.Abort(err)
	}
	route, err := g.route(request)
	if err != nil {
		return nil, b.Fail("ScopeDenied", err.Error())
	}
	if t.control != nil && !t.control.permits(route) {
		return nil, b.Fail("ScopeDenied", "request is outside the run's control-plane metadata targets")
	}
	ctx, done := b.RequestContext(request.Context())
	defer done()
	if err := runtimeRequests.Wait(ctx); err != nil {
		if ctx.Err() != nil {
			return nil, b.Abort(ctx.Err())
		}
		return nil, b.Abort(context.DeadlineExceeded)
	}
	if err := b.ChargeRequest(); err != nil {
		return nil, err
	}
	req := request.Clone(ctx)
	req.Header.Set("Accept", "application/json")
	if route.list {
		query := req.URL.Query()
		limit := limits.MaxPageObjects
		if value := query.Get("limit"); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return nil, b.Fail("ScopeDenied", "invalid list limit")
			}
			if n > 0 && n < limit {
				limit = n
			}
		}
		query.Set("limit", strconv.Itoa(limit))
		req.URL.RawQuery = query.Encode()
	}
	retryKey := req.URL.String()
	g.retryMu.Lock()
	exhausted := g.retries[retryKey] > limits.MaxRequestRetries
	g.retryMu.Unlock()
	if exhausted {
		return nil, b.Fail("WorkLimitExceeded", "per-request retry limit exceeded")
	}
	if t.next == nil {
		return nil, b.Fail("AuthorizationUnavailable", "missing transport")
	}
	permissionStatus := 0
	if t.control == nil {
		g.permissions.begin(route, req.URL.Path)
		defer func() { g.permissions.finish(permissionStatus) }()
	}
	response, err := t.next.RoundTrip(req)
	// A real 403 remains permission evidence even if its body is malformed or
	// exceeds a budget. Keep execution failure precedence separate from diagnostics.
	if response != nil && response.StatusCode == http.StatusForbidden {
		permissionStatus = http.StatusForbidden
	}
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if ctx.Err() != nil {
			return nil, b.Abort(ctx.Err())
		}
		return nil, b.Abort(err)
	}
	if response == nil || response.Body == nil {
		return nil, b.Fail("InputLimitExceeded", "missing API response body")
	}
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	defer func() { _ = response.Body.Close() }()
	reader := io.Reader(response.Body)
	switch strings.ToLower(response.Header.Get("Content-Encoding")) {
	case "", "identity":
	case "gzip":
		decoder, err := gzip.NewReader(response.Body)
		if err != nil {
			return nil, b.Fail("InputLimitExceeded", "invalid compressed API response")
		}
		defer func() { _ = decoder.Close() }()
		reader = decoder
	default:
		return nil, b.Fail("InputLimitExceeded", "unsupported API response encoding")
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, limits.MaxResponseBytes+1))
	if ctx.Err() != nil {
		return nil, b.Abort(ctx.Err())
	}
	if err := b.ChargeResponse(len(data)); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, b.Fail("InputLimitExceeded", "cannot decode API response")
	}
	g.retryMu.Lock()
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		g.retries[retryKey]++
	} else {
		delete(g.retries, retryKey)
	}
	g.retryMu.Unlock()
	if response.StatusCode == http.StatusForbidden {
		return nil, b.Fail("AccessDenied", "delegated API request was forbidden")
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if err := g.inspect(data, route); err != nil {
			return nil, err
		}
		permissionStatus = response.StatusCode
	}
	// Release the network body before exposing a bounded in-memory page to the
	// decoder. The returned body's lifetime is controlled by the ordinary client.
	copy := new(http.Response)
	*copy = *response
	copy.Header = response.Header.Clone()
	copy.Header.Del("Content-Encoding")
	copy.Header.Set("Content-Length", strconv.Itoa(len(data)))
	copy.ContentLength = int64(len(data))
	copy.Uncompressed = true
	copy.Body = io.NopCloser(bytes.NewReader(data))
	return copy, nil
}

type apiRoute struct {
	list, discovery           bool
	groupVersion              string
	resource, name, namespace string
}

func (g *Guard) route(req *http.Request) (apiRoute, error) {
	if req.Method != http.MethodGet {
		return apiRoute{}, fmt.Errorf("only get/list requests are permitted")
	}
	path := req.URL.Path
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "//") || req.URL.RawPath != "" || req.URL.EscapedPath() != path {
		return apiRoute{}, fmt.Errorf("noncanonical API path")
	}
	query, err := urlQuery(req.URL.RawQuery)
	if err != nil {
		return apiRoute{}, err
	}
	if query["watch"] != "" && query["watch"] != "false" && query["watch"] != "0" {
		return apiRoute{}, fmt.Errorf("watch requests are not permitted")
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	var tail []string
	var groupVersion string
	switch parts[0] {
	case "api":
		if len(parts) == 1 {
			return apiRoute{discovery: true}, nil
		}
		if !apiToken(parts[1]) {
			return apiRoute{}, fmt.Errorf("invalid core API version")
		}
		groupVersion = parts[1]
		if len(parts) == 2 {
			return apiRoute{discovery: true, groupVersion: groupVersion}, nil
		}
		tail = parts[2:]
	case "apis":
		if len(parts) == 1 {
			return apiRoute{discovery: true}, nil
		}
		if len(validation.IsDNS1123Subdomain(parts[1])) != 0 {
			return apiRoute{}, fmt.Errorf("invalid API group")
		}
		if len(parts) == 2 {
			return apiRoute{discovery: true}, nil
		}
		if !apiToken(parts[2]) {
			return apiRoute{}, fmt.Errorf("invalid API version")
		}
		groupVersion = parts[1] + "/" + parts[2]
		if len(parts) == 3 {
			return apiRoute{discovery: true, groupVersion: groupVersion}, nil
		}
		tail = parts[3:]
	default:
		return apiRoute{}, fmt.Errorf("non-API URL is forbidden")
	}
	namespace := ""
	if len(tail) >= 3 && tail[0] == "namespaces" {
		namespace = tail[1]
		tail = tail[2:]
	}
	if len(tail) < 1 || len(tail) > 2 || !apiToken(tail[0]) {
		return apiRoute{}, fmt.Errorf("subresources and malformed resource paths are forbidden")
	}
	if len(tail) == 2 && len(validation.IsDNS1123Subdomain(tail[1])) != 0 {
		return apiRoute{}, fmt.Errorf("invalid target name")
	}
	if namespace != "" {
		if !g.namespaces[namespace] {
			return apiRoute{}, fmt.Errorf("namespace %q is not authorized", namespace)
		}
	} else if !g.cluster {
		return apiRoute{}, fmt.Errorf("cluster-scoped read is not authorized")
	}
	route := apiRoute{list: len(tail) == 1, groupVersion: groupVersion, resource: tail[0], namespace: namespace}
	if len(tail) == 2 {
		route.name = tail[1]
	}
	return route, nil
}
func apiToken(s string) bool {
	if len(s) == 0 || len(s) > limits.MaxResourceSegmentBytes || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}
