/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	"github.com/skaphos/fathom/pkg/adapter"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// T035 requires hostile input to be exercised against the assembled runtime, not
// against hand-written mocks: the real compiler (declarative.CompileRuntimeScoped),
// the real supervisor (execution.Execute), the real guarded transport and a real
// controller-runtime client. Everything below the fake RoundTripper is production
// code, so a bound that only exists in a stub cannot make these cases pass.

const helperENamespace = "allowed"

func helperEScope() api.DefinitionBindingScope {
	return api.DefinitionBindingScope{Namespaces: []api.DefinitionDNSLabel{helperENamespace}}
}

// helperEClient builds the client the compiled definition actually reads through:
// a real REST client whose transport is the run's guard. A static mapper keeps
// discovery out of the picture so each case makes exactly the reads it declares.
func helperEClient(t *testing.T, g *execution.Guard, next http.RoundTripper) client.Client {
	t.Helper()
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	c, err := client.New(&rest.Config{Host: "https://cluster", Transport: g.Wrap(next)},
		client.Options{Scheme: clientgoscheme.Scheme, Mapper: mapper})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func helperEGuard(t *testing.T, b *execution.Budget) *execution.Guard {
	t.Helper()
	g, err := execution.NewGuard(b, helperEScope(), map[schema.GroupVersionKind]bool{{Version: "v1", Kind: "ConfigMap"}: true})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// helperEResponder serves one canned wire response to every guarded request.
func helperEResponder(status int, body []byte, header http.Header) http.RoundTripper {
	return roundTripFunc(func(*http.Request) (*http.Response, error) {
		out := http.Header{}
		for key, values := range header {
			out[key] = values
		}
		out.Set("Content-Type", "application/json")
		return &http.Response{StatusCode: status, Header: out, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
}

// helperEFieldCheck is the minimum valid list-driven check: it forces exactly one
// guarded LIST against the bound namespace before any evidence can exist.
func helperEFieldCheck(name string, path []string, expected string) api.DefinitionCheck {
	return api.DefinitionCheck{Name: api.DefinitionIdentifier(name), Kind: "Field", Field: &api.DefinitionField{
		Target:        api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{helperENamespace}},
		APIVersion:    "v1",
		Kind:          "ConfigMap",
		ListKind:      "ConfigMapList",
		FieldPath:     path,
		ExpectedValue: api.DefinitionText(expected),
	}}
}

// helperEHostileDefinition sits on every authoring bound at once: the maximum
// families, the maximum checks per family (so the 512-check total cap is exact),
// a maximum-length expectedValue and a maximum-length field path of maximum-length
// segments. It is deliberately still a *valid* definition - the point is that a
// legal-but-maximal definition cannot be used to escape the evaluation bounds.
func helperEHostileDefinition(t *testing.T) *api.AddonDefinition {
	t.Helper()
	deepPath := make([]string, limits.MaxFieldSegments)
	for i := range deepPath {
		deepPath[i] = strings.Repeat("d", limits.MaxFieldSegmentBytes)
	}
	d := &api.AddonDefinition{}
	d.Name = "hostile"
	d.Spec = api.AddonDefinitionSpec{AddonType: "hostile", AdapterVersion: "1.0.0", SemanticsVersion: 1}
	for f := 0; f < limits.MaxFamilies; f++ {
		family := api.DefinitionFamily{Name: api.DefinitionIdentifier(fmt.Sprintf("family-%d", f)), DefaultEnabled: true}
		for c := 0; c < limits.MaxChecksPerFamily; c++ {
			name := fmt.Sprintf("check-%d-%d", f, c)
			if f == 0 && c == 0 {
				family.Checks = append(family.Checks, helperEFieldCheck(name, deepPath, strings.Repeat("v", limits.MaxStringBytes)))
				continue
			}
			family.Checks = append(family.Checks, helperEFieldCheck(name, []string{"data", "state"}, "ok"))
		}
		d.Spec.Families = append(d.Spec.Families, family)
	}
	// A fixture that silently fell under a bound would let every case below pass
	// without ever reaching the maximal shape it claims to test.
	if err := limits.Validate(d); err != nil {
		t.Fatalf("hostile fixture must be a valid maximal definition: %v", err)
	}
	total := 0
	for _, f := range d.Spec.Families {
		total += len(f.Checks)
	}
	if len(d.Spec.Families) != limits.MaxFamilies || total != limits.MaxChecks {
		t.Fatalf("families=%d checks=%d want %d/%d", len(d.Spec.Families), total, limits.MaxFamilies, limits.MaxChecks)
	}
	return d
}

// helperERequireMaximal is what makes the maximal fixture load-bearing rather
// than decorative. Every hostile row below is discriminated by its *body*, so
// swapping the maximal definition for the one-check peer left them all green:
// the 16-family/512-check/deep-path shape was only ever implicitly asserted.
// This pins it on the value actually handed to the run, from two sides - the
// definition the production compiler accepted sits exactly at each authoring
// maximum, and the same compiler rejects one step past any of them.
func helperERequireMaximal(t *testing.T, def *api.AddonDefinition) *api.AddonDefinition {
	t.Helper()
	compiled, err := declarative.CompileRuntimeScoped(context.Background(), def, helperEScope())
	if err != nil {
		t.Fatalf("the maximal definition must compile: %v", err)
	}
	// The compiled snapshot, not the fixture: this is the family set the
	// evaluator would dispatch.
	if got := len(compiled.Capabilities().Families); got != limits.MaxFamilies {
		t.Fatalf("compiled families=%d want the contract maximum %d", got, limits.MaxFamilies)
	}
	total := 0
	for _, family := range def.Spec.Families {
		if len(family.Checks) != limits.MaxChecksPerFamily {
			t.Fatalf("family %q has %d checks, want the contract maximum %d",
				family.Name, len(family.Checks), limits.MaxChecksPerFamily)
		}
		total += len(family.Checks)
	}
	if total != limits.MaxChecks {
		t.Fatalf("compiled checks=%d want the contract maximum %d", total, limits.MaxChecks)
	}
	deepest := 0
	widest := 0
	for _, family := range def.Spec.Families {
		for _, check := range family.Checks {
			if check.Field == nil {
				continue
			}
			segments := 0
			for _, segment := range check.Field.FieldPath {
				if len(segment) == limits.MaxFieldSegmentBytes {
					segments++
				}
			}
			deepest = max(deepest, segments)
			widest = max(widest, len(check.Field.ExpectedValue))
		}
	}
	if deepest != limits.MaxFieldSegments || widest != limits.MaxStringBytes {
		t.Fatalf("deepest maximal-segment path=%d (want %d), longest expectedValue=%d bytes (want %d)",
			deepest, limits.MaxFieldSegments, widest, limits.MaxStringBytes)
	}
	// One step past either authoring maximum must be refused by the very same
	// compiler this run uses, so "at the maximum" above means the edge and not
	// merely some size the compiler happens to like.
	for name, over := range map[string]*api.AddonDefinition{
		"one family over the maximum": helperEWithExtraFamily(def),
		"one check over the maximum":  helperEWithExtraCheck(def),
	} {
		if _, err := declarative.CompileRuntimeScoped(context.Background(), over, helperEScope()); err == nil ||
			!strings.Contains(err.Error(), "InvalidDefinition") {
			t.Fatalf("%s was not rejected as InvalidDefinition: %v", name, err)
		}
	}
	return def
}

func helperEWithExtraFamily(def *api.AddonDefinition) *api.AddonDefinition {
	over := def.DeepCopy()
	over.Spec.Families = append(over.Spec.Families, api.DefinitionFamily{
		Name: "overflow", DefaultEnabled: true,
		Checks: []api.DefinitionCheck{helperEFieldCheck("overflow", []string{"data", "state"}, "ok")}})
	return over
}

func helperEWithExtraCheck(def *api.AddonDefinition) *api.AddonDefinition {
	over := def.DeepCopy()
	over.Spec.Families[0].Checks = append(over.Spec.Families[0].Checks,
		helperEFieldCheck("overflow", []string{"data", "state"}, "ok"))
	return over
}

// helperEGoroutineBaseline drives one complete attempt before sampling, so the
// lazily started machinery behind the first client.New/RESTClient path is
// already running. Charging it to the measured window would report a false
// leak; runner_test.go warms up against the same hazard.
func helperEGoroutineBaseline(t *testing.T) int {
	t.Helper()
	helperERun(t, helperEHealthyDefinition(t), http.StatusOK, []byte(helperEPage()), nil)
	helperEGoroutinesSettle(t, runtime.NumGoroutine())
	return runtime.NumGoroutine()
}

// helperEHealthyDefinition is the peer definition: one family, one check, so a
// well-formed empty page completes it within one request.
func helperEHealthyDefinition(t *testing.T) *api.AddonDefinition {
	t.Helper()
	d := &api.AddonDefinition{}
	d.Name = "healthy"
	d.Spec = api.AddonDefinitionSpec{AddonType: "healthy", AdapterVersion: "1.0.0", SemanticsVersion: 1, Families: []api.DefinitionFamily{
		{Name: "health", DefaultEnabled: true, Checks: []api.DefinitionCheck{helperEFieldCheck("state", []string{"data", "state"}, "ok")}},
	}}
	if err := limits.Validate(d); err != nil {
		t.Fatal(err)
	}
	return d
}

// helperERun drives one complete supervised attempt through the real compiler,
// the real guard and a real client reading the supplied wire response.
func helperERun(t *testing.T, def *api.AddonDefinition, status int, body []byte, header http.Header) (execution.Attempt, int) {
	t.Helper()
	b, done := execution.NewBudget(context.Background(), limits.MaxRunDuration)
	defer done()
	c := helperEClient(t, helperEGuard(t, b), helperEResponder(status, body, header))
	released := 0
	attempt := execution.Execute(b, func(ctx context.Context) (adapter.Adapter, error) {
		return declarative.CompileRuntimeScoped(ctx, def, helperEScope())
	}, adapter.Request{Client: c, Timeout: limits.MaxRunDuration}, func() { released++ })
	return attempt, released
}

func helperEFailureReason(t *testing.T, err error) string {
	t.Helper()
	var failure *execution.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("want a named bounded failure, got %T %v", err, err)
	}
	return failure.Reason
}

// helperENodeItem renders one collection item with exactly n JSON nodes: the six
// fixed members plus n-7 data keys, counting the item object itself.
func helperENodeItem(n int) string {
	keys := make([]string, 0, n-7)
	for i := 0; i < n-7; i++ {
		keys = append(keys, fmt.Sprintf(`"k%d":"v"`, i))
	}
	return `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"hostile","namespace":"` + helperENamespace +
		`"},"data":{` + strings.Join(keys, ",") + `}}`
}

// helperENestedItem renders an item whose deepest value sits at exactly depth d,
// measuring the item object itself as depth 0.
func helperENestedItem(d int) string {
	nested := `"leaf"`
	for i := 0; i < d; i++ {
		nested = `{"a":` + nested + `}`
	}
	return nested
}

func helperEPage(items ...string) string {
	return `{"apiVersion":"v1","kind":"ConfigMapList","metadata":{},"items":[` + strings.Join(items, ",") + `]}`
}

// TestRuntimeSurvivesHostileDefinitionAndResponses is the T035 hostile-input
// component case. Each row feeds a maximal definition a response that violates
// exactly one evaluation bound and requires the named bounded failure, no
// evidence, a released slot, a reserved diagnostic, and no leaked goroutine.
func TestRuntimeSurvivesHostileDefinitionAndResponses(t *testing.T) {
	oversize, gzipHeader := helperCPayload(t, strings.Repeat("x", limits.MaxResponseBytes+1), true)
	for _, tc := range []struct {
		name   string
		status int
		body   []byte
		header http.Header
		reason string
	}{
		{name: "object above the node cap", status: http.StatusOK,
			body:   []byte(helperEPage(helperENodeItem(limits.MaxObjectNodes + 1))),
			reason: "InputLimitExceeded"},
		{name: "object above the depth cap", status: http.StatusOK,
			body:   []byte(helperEPage(helperENestedItem(limits.MaxObjectDepth + 1))),
			reason: "InputLimitExceeded"},
		{name: "page above the object cap", status: http.StatusOK,
			body:   []byte(helperCItems(limits.MaxPageObjects+1, "")),
			reason: "WorkLimitExceeded"},
		{name: "decompressed body above the response cap", status: http.StatusOK,
			body: oversize, header: gzipHeader, reason: "ResponseLimitExceeded"},
		{name: "forbidden delegated read", status: http.StatusForbidden,
			body: []byte(`{"kind":"Status","status":"Failure","code":403}`), reason: "AccessDenied"},
		{name: "body that is not an object", status: http.StatusOK,
			body: []byte(`[1,2,3]`), reason: "InputLimitExceeded"},
		{name: "collection without items", status: http.StatusOK,
			body: []byte(`{"apiVersion":"v1","kind":"ConfigMapList","metadata":{}}`), reason: "InputLimitExceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseline := helperEGoroutineBaseline(t)
			attempt, released := helperERun(t, helperERequireMaximal(t, helperEHostileDefinition(t)), tc.status, tc.body, tc.header)
			if attempt.Completed || len(attempt.Evidence.Checks) != 0 || attempt.Evidence.DetectedVersion != "" {
				t.Fatalf("hostile input produced evidence: %+v", attempt)
			}
			if got := helperEFailureReason(t, attempt.Err); got != tc.reason {
				t.Fatalf("failure reason=%q want %q (%v)", got, tc.reason, attempt.Err)
			}
			if attempt.Summary == "" {
				t.Fatal("bounded failure must reserve a diagnostic summary")
			}
			if released != 1 {
				t.Fatalf("slot released %d times", released)
			}
			helperEGoroutinesSettle(t, baseline)
		})
	}
}

// Negative control for the table above: the same fixtures rendered exactly *at*
// each bound must complete. Without this, a fixture that failed for an unrelated
// reason - a malformed body, an unauthorised route - would still satisfy every
// "+1 is rejected" row and the bounds themselves would go untested.
func TestRuntimeAcceptsResponsesExactlyAtTheirBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"object at the node cap", helperEPage(helperENodeItem(limits.MaxObjectNodes))},
		{"object at the depth cap", helperEPage(helperENestedItem(limits.MaxObjectDepth))},
		{"page at the object cap", helperCItems(limits.MaxPageObjects, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempt, released := helperERun(t, helperEHealthyDefinition(t), http.StatusOK, []byte(tc.body), nil)
			if attempt.Err != nil || !attempt.Completed || released != 1 {
				t.Fatalf("at-bound response rejected: attempt=%+v released=%d", attempt, released)
			}
		})
	}
}

// A hostile neighbour must not disable an unrelated definition: the peer runs
// after the worst case above and still completes with owned evidence.
func TestRuntimeHealthyPeerCompletesAfterHostileNeighbour(t *testing.T) {
	baseline := helperEGoroutineBaseline(t)
	hostile, released := helperERun(t, helperERequireMaximal(t, helperEHostileDefinition(t)), http.StatusOK,
		[]byte(helperEPage(helperENodeItem(limits.MaxObjectNodes+1))), nil)
	if hostile.Completed || hostile.Err == nil || released != 1 {
		t.Fatalf("hostile attempt=%+v released=%d", hostile, released)
	}
	peer, peerReleased := helperERun(t, helperEHealthyDefinition(t), http.StatusOK,
		[]byte(helperEPage()), nil)
	if peer.Err != nil || !peer.Completed || peerReleased != 1 {
		t.Fatalf("healthy peer attempt=%+v released=%d", peer, peerReleased)
	}
	if len(peer.Evidence.Checks) != 1 || peer.Evidence.Checks[0].Outcome == adapter.OutcomeError {
		t.Fatalf("peer evidence=%+v", peer.Evidence.Checks)
	}
	helperEGoroutinesSettle(t, baseline)
}

// The wire is bounded before the compiled definition ever sees a page, so a
// hostile body can never be partially consumed into evidence. Walking a run's
// worth of full pages through the real client must stop at the object cap with
// the continuation still outstanding rather than truncating to a "complete" list.
func TestRuntimeWalkStopsAtRunObjectCapThroughRealClient(t *testing.T) {
	b, done := execution.NewBudget(context.Background(), limits.MaxRunDuration)
	defer done()
	pages := 0
	c := helperEClient(t, helperEGuard(t, b), roundTripFunc(func(*http.Request) (*http.Response, error) {
		pages++
		header := http.Header{}
		header.Set("Content-Type", "application/json")
		// Distinct tokens: a repeated token is refused as a loop long before the
		// object cap, which would hide whether the cap itself is enforced.
		return &http.Response{StatusCode: http.StatusOK, Header: header,
			Body: io.NopCloser(strings.NewReader(helperCItems(limits.MaxPageObjects, fmt.Sprintf("page-%d", pages))))}, nil
	}))
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMapList"})
	consumed := 0
	err := execution.WalkPages(b.Context(), b, c, list,
		func(_ context.Context, page client.ObjectList) error {
			items, err := meta.ExtractList(page)
			consumed += len(items)
			return err
		},
		func(context.Context) error { return nil },
		client.InNamespace(helperENamespace))
	if got := helperEFailureReason(t, err); got != "WorkLimitExceeded" {
		t.Fatalf("reason=%q err=%v", got, err)
	}
	if b.Objects() != limits.MaxRunObjects {
		t.Fatalf("objects=%d want %d", b.Objects(), limits.MaxRunObjects)
	}
	if want := limits.MaxRunObjects / limits.MaxPageObjects; pages != want {
		t.Fatalf("pages=%d want %d", pages, want)
	}
	if consumed >= limits.MaxRunObjects {
		t.Fatalf("consumed=%d: the capped page must not be delivered as a complete collection", consumed)
	}
}

// helperEGoroutinesSettle enforces the "no unsupervised evaluator goroutines"
// rule. Runtime bookkeeping goroutines can outlive the call that spawned them by
// a scheduling quantum, so allow a bounded settle window before declaring a leak.
func helperEGoroutinesSettle(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		current := runtime.NumGoroutine()
		if current <= baseline {
			return
		}
		if time.Now().After(deadline) {
			buffer := make([]byte, 1<<16)
			t.Fatalf("goroutines=%d baseline=%d: unsupervised goroutines outlived the attempt\n%s",
				current, baseline, buffer[:runtime.Stack(buffer, true)])
		}
		time.Sleep(10 * time.Millisecond)
	}
}
