/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package runtime_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/skaphos/fathom/api/v1alpha1"
	execution "github.com/skaphos/fathom/internal/adapter/runtime"
	limits "github.com/skaphos/fathom/pkg/addondefinition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func diagnosticDefinition() *api.AddonDefinition {
	core := ""
	return &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom-addon"}, Spec: api.AddonDefinitionSpec{
		AddonType: "custom-addon", AdapterVersion: "1.0.0", SemanticsVersion: 1,
		Families: []api.DefinitionFamily{{Name: "health", Checks: []api.DefinitionCheck{{Name: "config", Kind: "ConfigMap", ConfigMap: &api.DefinitionConfigMap{
			Target: api.DefinitionTarget{Scope: "Namespaced", Namespaces: []api.DefinitionDNSLabel{"allowed"}}, DefaultName: "config", Key: "policy.yaml",
		}}}}},
		RequestedReads: []api.DefinitionReadRule{
			{APIGroup: &core, Resources: []string{"configmaps"}, Verbs: []string{"get"}, ResourceNames: []api.DefinitionResourceName{"config"}},
			{NonResourceURLs: []string{"/api/v1"}, Verbs: []string{"get"}},
		},
	}}
}

func TestPermissionsUseOnlyActualRequests(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
		want   metav1.ConditionStatus
		reason string
	}{
		{"success", 200, `{}`, nil, metav1.ConditionTrue, "RequestsSucceeded"},
		{"forbidden", 403, `{}`, nil, metav1.ConditionFalse, "AccessDenied"},
		{"oversized forbidden", 403, strings.Repeat("x", limits.MaxResponseBytes+1), nil, metav1.ConditionFalse, "AccessDenied"},
		{"missing forbidden body", 403, "", nil, metav1.ConditionFalse, "AccessDenied"},
		{"transport failure", 0, "", errors.New("private connection details"), metav1.ConditionUnknown, "AccessCheckUnavailable"},
		{"unavailable", 503, `{}`, nil, metav1.ConditionUnknown, "AccessCheckUnavailable"},
		{"not found", 404, `{}`, nil, metav1.ConditionUnknown, "AccessCheckUnavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
			defer closeBudget()
			guard := runtimeGuard(t, b, false)
			d := diagnosticDefinition()
			before, err := guard.Diagnostics(d)
			if err != nil || before.Status != metav1.ConditionUnknown || before.Reason != "NotEvaluated" {
				t.Fatalf("before request: %+v, %v", before, err)
			}
			calls := 0
			transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.err != nil {
					return nil, tc.err
				}
				response := &http.Response{StatusCode: tc.status, Header: http.Header{}}
				if tc.body != "" {
					response.Body = io.NopCloser(strings.NewReader(tc.body))
				}
				return response, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
			response, _ := transport.RoundTrip(req)
			if response != nil {
				_ = response.Body.Close()
			}
			got, err := guard.Diagnostics(d)
			if err != nil || got.Status != tc.want || got.Reason != tc.reason || calls != 1 {
				t.Fatalf("diagnostic=%+v err=%v calls=%d", got, err, calls)
			}
			if strings.Contains(got.Message, "private") || !strings.Contains(got.Message, "future") || len(got.Message) > 1024 {
				t.Fatalf("unbounded or overclaiming message: %s", got.Message)
			}
		})
	}
}

func TestPermissionDeclarationsAreAdvisory(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		edit       func(*api.AddonDefinition)
	}{
		{"known needs covered", "DeclaredReadsCovered", func(*api.AddonDefinition) {}},
		{"omitted", "RequestedReadsOmitted", func(d *api.AddonDefinition) { d.Spec.RequestedReads = nil }},
		{"missing resource", "RequestedReadsIncomplete", func(d *api.AddonDefinition) { d.Spec.RequestedReads = d.Spec.RequestedReads[1:] }},
		{"missing discovery", "RequestedReadsIncomplete", func(d *api.AddonDefinition) { d.Spec.RequestedReads = d.Spec.RequestedReads[:1] }},
		{"wrong singleton", "RequestedReadsIncomplete", func(d *api.AddonDefinition) { d.Spec.RequestedReads[0].ResourceNames[0] = "other" }},
		{"wrong verb", "RequestedReadsIncomplete", func(d *api.AddonDefinition) { d.Spec.RequestedReads[0].Verbs[0] = "list" }},
		{"override needs review", "RequestedReadsUnresolved", func(d *api.AddonDefinition) { d.Spec.Families[0].Checks[0].ConfigMap.NameThresholdKey = "configName" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
			defer closeBudget()
			guard := runtimeGuard(t, b, false)
			d := diagnosticDefinition()
			tc.edit(d)
			before, err := guard.Diagnostics(d)
			if err != nil || before.DeclarationReason != tc.want {
				t.Fatalf("before=%+v err=%v", before, err)
			}
			transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			}))
			req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
			response, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			got, err := guard.Diagnostics(d)
			if err != nil || got.Status != metav1.ConditionTrue || got.DeclarationReason != tc.want {
				t.Fatalf("after=%+v err=%v", got, err)
			}
		})
	}
}

func TestPermissionFailuresSurviveLaterSuccess(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
	defer closeBudget()
	guard := runtimeGuard(t, b, false)
	d := diagnosticDefinition()
	status := 503
	transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	for _, code := range []int{503, 200, 403} {
		status = code
		req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/config", nil)
		response, _ := transport.RoundTrip(req)
		if response != nil {
			_ = response.Body.Close()
		}
		got, err := guard.Diagnostics(d)
		want := metav1.ConditionUnknown
		if code == 403 {
			want = metav1.ConditionFalse
		}
		if err != nil || got.Status != want {
			t.Fatalf("code=%d diagnostic=%+v err=%v", code, got, err)
		}
	}
}

func TestPermissionsObserveOverridesAndConcurrentRequests(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
	defer closeBudget()
	guard := runtimeGuard(t, b, false)
	d := diagnosticDefinition()
	entered := make(chan struct{})
	release := make(chan struct{})
	transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	var wg sync.WaitGroup
	wg.Go(func() {
		req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/allowed/configmaps/overridden", nil)
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Error(err)
		}
		if response != nil {
			_ = response.Body.Close()
		}
	})
	<-entered
	before, err := guard.Diagnostics(d)
	close(release)
	wg.Wait()
	if err != nil || before.Reason != "AccessCheckUnavailable" || before.DeclarationReason != "RequestedReadsIncomplete" {
		t.Fatalf("pending=%+v err=%v", before, err)
	}
	after, err := guard.Diagnostics(d)
	if err != nil || after.Status != metav1.ConditionTrue || after.DeclarationReason != "RequestedReadsIncomplete" {
		t.Fatalf("completed=%+v err=%v", after, err)
	}
}

func TestPermissionsDoNotCountLocallyDeniedRequests(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
	defer closeBudget()
	guard := runtimeGuard(t, b, false)
	transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("scope denial reached the API")
		return nil, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1/namespaces/other/configmaps/config", nil)
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("scope violation accepted")
	}
	got, err := guard.Diagnostics(diagnosticDefinition())
	if err != nil || got.Reason != "NotEvaluated" {
		t.Fatalf("diagnostic=%+v err=%v", got, err)
	}
}

func TestPermissionDeclarationHelpersAndUnknownMappings(t *testing.T) {
	for _, sample := range []string{"workload", "field"} {
		t.Run(sample, func(t *testing.T) {
			data, err := os.ReadFile("../../../config/samples/addondefinition/" + sample + ".yaml")
			if err != nil {
				t.Fatal(err)
			}
			var d api.AddonDefinition
			if err := yaml.UnmarshalStrict(data, &d); err != nil {
				t.Fatal(err)
			}
			b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
			defer closeBudget()
			guard := runtimeGuard(t, b, false)
			if sample == "field" {
				group := "example.org"
				d.Spec.RequestedReads = []api.DefinitionReadRule{
					{APIGroup: &group, Resources: []string{"widgets"}, Verbs: []string{"list"}},
					{NonResourceURLs: []string{"/apis/example.org/v1"}, Verbs: []string{"get"}},
				}
				got, err := guard.Diagnostics(&d)
				if err != nil || got.DeclarationReason != "RequestedReadsUnresolved" || got.Reason != "NotEvaluated" {
					t.Fatalf("unproven custom mapping=%+v err=%v", got, err)
				}
				return
			}
			core, apps := "", "apps"
			d.Spec.Families[0].Checks[0].Workload.CheckPods = true
			d.Spec.RequestedReads = []api.DefinitionReadRule{
				{APIGroup: &apps, Resources: []string{"deployments"}, Verbs: []string{"get"}},
				{NonResourceURLs: []string{"/apis/apps/v1", "/api/v1"}, Verbs: []string{"get"}},
			}
			got, err := guard.Diagnostics(&d)
			if err != nil || got.DeclarationReason != "RequestedReadsIncomplete" {
				t.Fatalf("missing pod helper=%+v err=%v", got, err)
			}
			d.Spec.RequestedReads = append(d.Spec.RequestedReads, api.DefinitionReadRule{APIGroup: &core, Resources: []string{"pods"}, Verbs: []string{"list"}, ResourceNames: []api.DefinitionResourceName{"pod"}})
			got, err = guard.Diagnostics(&d)
			if err != nil || got.DeclarationReason != "RequestedReadsIncomplete" {
				t.Fatalf("restricted helper list=%+v err=%v", got, err)
			}
			d.Spec.RequestedReads[2].ResourceNames = nil
			got, err = guard.Diagnostics(&d)
			if err != nil || got.DeclarationReason != "DeclaredReadsCovered" {
				t.Fatalf("covered helper=%+v err=%v", got, err)
			}
		})
	}
}

func TestPermissionDiscoverySuccessDoesNotClaimTargetAccess(t *testing.T) {
	b, closeBudget := execution.NewBudget(context.Background(), 30*time.Second)
	defer closeBudget()
	guard := runtimeGuard(t, b, false)
	transport := guard.Wrap(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kind":"APIResourceList","groupVersion":"v1","resources":[]}`))}, nil
	}))
	req, _ := http.NewRequest(http.MethodGet, "https://cluster/api/v1", nil)
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	got, err := guard.Diagnostics(diagnosticDefinition())
	if err != nil || got.Status != metav1.ConditionTrue || !strings.Contains(got.Message, "only for the delegated requests") {
		t.Fatalf("discovery overclaim=%+v err=%v", got, err)
	}
}
