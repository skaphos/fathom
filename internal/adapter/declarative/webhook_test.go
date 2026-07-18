/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/skaphos/fathom/pkg/adapter"
)

// wiredEntry builds a webhooks[] element with a populated caBundle pointing at
// namespace/name; svcName=="" produces a URL-based clientConfig instead.
func wiredEntry(entryName, svcNamespace, svcName string, caBundle []byte) admissionv1.MutatingWebhook {
	cc := admissionv1.WebhookClientConfig{CABundle: caBundle}
	if svcName == "" {
		url := "https://webhook.example.com/inject"
		cc.URL = &url
	} else {
		cc.Service = &admissionv1.ServiceReference{Namespace: svcNamespace, Name: svcName}
	}
	return admissionv1.MutatingWebhook{Name: entryName, ClientConfig: cc}
}

func mutatingConfig(name string, entries ...admissionv1.MutatingWebhook) *admissionv1.MutatingWebhookConfiguration {
	return &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Webhooks:   entries,
	}
}

func validatingConfig(name string, entries ...admissionv1.MutatingWebhook) *admissionv1.ValidatingWebhookConfiguration {
	cfg := &admissionv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: name}}
	for _, e := range entries {
		cfg.Webhooks = append(cfg.Webhooks, admissionv1.ValidatingWebhook{Name: e.Name, ClientConfig: e.ClientConfig})
	}
	return cfg
}

// runWebhook runs an engine whose single enabled family carries wc as its only
// WebhookCheck, against objs.
func runWebhook(t *testing.T, wc WebhookCheck, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	return runWebhookPolicy(t, wc, nil, objs...)
}

// runWebhookPolicy is runWebhook with an explicit family policy, for the
// threshold-override and namespace-resolution paths.
func runWebhookPolicy(t *testing.T, wc WebhookCheck, policy map[adapter.Family]adapter.FamilyPolicy, objs ...clientObject) []adapter.CheckResult {
	t.Helper()
	eng := MustEngine(AddonDefinition{
		AddonType:      "webhooktest",
		AdapterVersion: "0.0.1",
		Families: []FamilyDefinition{{
			Name:           "webhook_health",
			DefaultEnabled: true,
			Webhooks:       []WebhookCheck{wc},
		}},
	})
	res, err := eng.Run(context.Background(), adapter.Request{
		Client: newFakeClient(t, objs...),
		Logger: logr.Discard(),
		Policy: policy,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.Checks
}

func TestWebhook_WiredMutatingPasses(t *testing.T) {
	wc := WebhookCheck{
		Kind:             KindMutatingWebhookConfiguration,
		Name:             "istio-sidecar-injector",
		ExpectedService:  "istiod",
		ServiceNamespace: "istio-system",
	}
	cfg := mutatingConfig("istio-sidecar-injector",
		wiredEntry("rev.namespace.sidecar-injector.istio.io", "istio-system", "istiod", []byte("ca")),
		wiredEntry("rev.object.sidecar-injector.istio.io", "istio-system", "istiod", []byte("ca")),
	)

	checks := runWebhook(t, wc, cfg)
	assertHasOutcome(t, checks, KindMutatingWebhookConfiguration, "istio-sidecar-injector", adapter.OutcomePass, "wired")
	assertHasDetail(t, checks, KindMutatingWebhookConfiguration, "istio-sidecar-injector", "webhookCount", "2")
	assertHasDetail(t, checks, KindMutatingWebhookConfiguration, "istio-sidecar-injector", "expectedService", "istio-system/istiod")
	assertFamily(t, checks, KindMutatingWebhookConfiguration, "istio-sidecar-injector", adapter.Family("webhook_health"))
}

func TestWebhook_WiredValidatingPasses(t *testing.T) {
	wc := WebhookCheck{
		Kind:             KindValidatingWebhookConfiguration,
		Name:             "istio-validator-istio-system",
		ExpectedService:  "istiod",
		ServiceNamespace: "istio-system",
	}
	cfg := validatingConfig("istio-validator-istio-system",
		wiredEntry("rev.validation.istio.io", "istio-system", "istiod", []byte("ca")),
	)

	checks := runWebhook(t, wc, cfg)
	assertHasOutcome(t, checks, KindValidatingWebhookConfiguration, "istio-validator-istio-system", adapter.OutcomePass, "wired")
	assertHasDetail(t, checks, KindValidatingWebhookConfiguration, "istio-validator-istio-system", "webhookCount", "1")
}

func TestWebhook_UnpopulatedCABundleFails(t *testing.T) {
	wc := WebhookCheck{Kind: KindMutatingWebhookConfiguration, Name: "injector"}
	cfg := mutatingConfig("injector",
		wiredEntry("populated.example.com", "ns", "svc", []byte("ca")),
		wiredEntry("empty.example.com", "ns", "svc", nil),
	)

	checks := runWebhook(t, wc, cfg)
	assertHasOutcome(t, checks, KindMutatingWebhookConfiguration, "injector", adapter.OutcomeFail, "caBundle")
	assertHasDetail(t, checks, KindMutatingWebhookConfiguration, "injector", "unpopulatedCABundle", "empty.example.com")
}

func TestWebhook_MisdirectedServiceFails(t *testing.T) {
	wc := WebhookCheck{
		Kind:             KindMutatingWebhookConfiguration,
		Name:             "injector",
		ExpectedService:  "istiod",
		ServiceNamespace: "istio-system",
	}
	cfg := mutatingConfig("injector",
		wiredEntry("wrong-svc.example.com", "istio-system", "other", []byte("ca")),
		wiredEntry("url-based.example.com", "", "", []byte("ca")),
		wiredEntry("wired.example.com", "istio-system", "istiod", []byte("ca")),
	)

	checks := runWebhook(t, wc, cfg)
	assertHasOutcome(t, checks, KindMutatingWebhookConfiguration, "injector", adapter.OutcomeFail, "backing service")
	assertHasDetail(t, checks, KindMutatingWebhookConfiguration, "injector",
		"misdirectedWebhooks", "wrong-svc.example.com,url-based.example.com")
}

func TestWebhook_BothProblemsOneResult(t *testing.T) {
	wc := WebhookCheck{
		Kind:             KindMutatingWebhookConfiguration,
		Name:             "injector",
		ExpectedService:  "istiod",
		ServiceNamespace: "istio-system",
	}
	cfg := mutatingConfig("injector",
		wiredEntry("bad.example.com", "istio-system", "other", nil),
	)

	checks := runWebhook(t, wc, cfg)
	if len(checks) != 1 {
		t.Fatalf("want one combined result, got %d: %#v", len(checks), checks)
	}
	c := checks[0]
	if c.Outcome != adapter.OutcomeFail {
		t.Fatalf("outcome: got %s, want Fail", c.Outcome)
	}
	if !strings.Contains(c.Summary, "caBundle") || !strings.Contains(c.Summary, "backing service") {
		t.Fatalf("summary should name both problems, got %q", c.Summary)
	}
}

func TestWebhook_NoEntriesFails(t *testing.T) {
	wc := WebhookCheck{Kind: KindValidatingWebhookConfiguration, Name: "empty"}
	checks := runWebhook(t, wc, validatingConfig("empty"))
	assertHasOutcome(t, checks, KindValidatingWebhookConfiguration, "empty", adapter.OutcomeFail, "no webhook entries")
}

func TestWebhook_URLEntryWithoutCABundlePasses(t *testing.T) {
	// ExpectedService unset + a URL-based entry with NO caBundle: legal per
	// the admissionregistration API (the API server falls back to its system
	// trust roots), so the caBundle score must not apply — only
	// service-based entries depend on the addon's CA injection.
	wc := WebhookCheck{Kind: KindMutatingWebhookConfiguration, Name: "external"}
	cfg := mutatingConfig("external", wiredEntry("url.example.com", "", "", nil))

	checks := runWebhook(t, wc, cfg)
	assertHasOutcome(t, checks, KindMutatingWebhookConfiguration, "external", adapter.OutcomePass, "wired")
}

func TestWebhook_NameThresholdOverride(t *testing.T) {
	// A renamed configuration (istio's revisioned/relocated
	// istio-validator-<rev>-<ns>) is reachable via the name threshold.
	wc := WebhookCheck{
		Kind:             KindValidatingWebhookConfiguration,
		Name:             "istio-validator-istio-system",
		NameThresholdKey: "validatorWebhookName",
	}
	cfg := validatingConfig("istio-validator-istio-mesh",
		wiredEntry("rev.validation.istio.io", "istio-mesh", "istiod", []byte("ca")))

	policy := map[adapter.Family]adapter.FamilyPolicy{
		"webhook_health": {Enabled: true, Thresholds: map[string]string{"validatorWebhookName": "istio-validator-istio-mesh"}},
	}
	checks := runWebhookPolicy(t, wc, policy, cfg)
	assertHasOutcome(t, checks, KindValidatingWebhookConfiguration, "istio-validator-istio-mesh", adapter.OutcomePass, "wired")
}

func TestWebhook_PolicyNamespaceOverridesServiceNamespace(t *testing.T) {
	// The expected backing-service namespace follows the family's resolved
	// namespace: an addon installed outside its default namespace passes
	// when the policy points the family there.
	wc := WebhookCheck{
		Kind:             KindMutatingWebhookConfiguration,
		Name:             "injector",
		ExpectedService:  "istiod",
		ServiceNamespace: "istio-system",
	}
	cfg := mutatingConfig("injector",
		wiredEntry("inject.istio.io", "istio-mesh", "istiod", []byte("ca")))

	policy := map[adapter.Family]adapter.FamilyPolicy{
		"webhook_health": {Enabled: true, Namespaces: []string{"istio-mesh"}},
	}
	checks := runWebhookPolicy(t, wc, policy, cfg)
	assertHasOutcome(t, checks, KindMutatingWebhookConfiguration, "injector", adapter.OutcomePass, "wired")
	assertHasDetail(t, checks, KindMutatingWebhookConfiguration, "injector", "expectedService", "istio-mesh/istiod")
}

func TestWebhook_AbsenceResolution(t *testing.T) {
	required := WebhookCheck{Kind: KindMutatingWebhookConfiguration, Name: "missing"}
	checks := runWebhook(t, required) // no objects
	assertHasOutcome(t, checks, KindMutatingWebhookConfiguration, "missing", adapter.OutcomeFail, "not found")
	assertHasDetail(t, checks, KindMutatingWebhookConfiguration, "missing", adapter.DetailAbsent, "true")

	optional := WebhookCheck{Kind: KindValidatingWebhookConfiguration, Name: "missing", Absence: Optional}
	checks = runWebhook(t, optional)
	assertHasOutcome(t, checks, KindValidatingWebhookConfiguration, "missing", adapter.OutcomeSkipped, "not found")
	assertHasDetail(t, checks, KindValidatingWebhookConfiguration, "missing", adapter.DetailAbsent, "true")
}

func TestNewEngine_WebhookValidation(t *testing.T) {
	base := func(wc WebhookCheck) AddonDefinition {
		return AddonDefinition{
			AddonType:      "webhooktest",
			AdapterVersion: "0.0.1",
			Families: []FamilyDefinition{{
				Name:           "webhook_health",
				DefaultEnabled: true,
				Webhooks:       []WebhookCheck{wc},
			}},
		}
	}
	cases := []struct {
		name    string
		check   WebhookCheck
		wantErr string
	}{
		{
			name:    "unknown kind",
			check:   WebhookCheck{Kind: "AdmissionConfiguration", Name: "x"},
			wantErr: "unknown kind",
		},
		{
			name:    "empty name",
			check:   WebhookCheck{Kind: KindMutatingWebhookConfiguration},
			wantErr: "empty name",
		},
		{
			name:    "service name without namespace",
			check:   WebhookCheck{Kind: KindMutatingWebhookConfiguration, Name: "x", ExpectedService: "svc"},
			wantErr: "together",
		},
		{
			name:    "service namespace without name",
			check:   WebhookCheck{Kind: KindMutatingWebhookConfiguration, Name: "x", ServiceNamespace: "ns"},
			wantErr: "together",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEngine(base(tc.check))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("NewEngine: got %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}
