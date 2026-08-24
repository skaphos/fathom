/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

type failingHealthCheckTargetClient struct {
	client.Client
	err error
}

func (c failingHealthCheckTargetClient) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	switch obj.(type) {
	case *fathomv1alpha1.AddonCheck, *fathomv1alpha1.DNSCheck, *fathomv1alpha1.NodeCertificateCheck:
		return c.err
	default:
		return c.Client.Get(ctx, key, obj, opts...)
	}
}

func TestAddonCheckTargetHandlerCompatibility(t *testing.T) {
	t.Parallel()

	runTime := metav1.NewTime(time.Now().Add(-time.Minute).UTC().Truncate(time.Second))
	target := &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "wrapper-ns"},
		Status: fathomv1alpha1.AddonCheckStatus{
			LastResult:     string(fathomv1alpha1.HealthReportResultPass),
			LastRunTime:    &runTime,
			LastReportName: "source-report",
			Conditions: []metav1.Condition{{
				Type:    healthCheckConditionReady,
				Status:  metav1.ConditionTrue,
				Reason:  "RunCompleted",
				Message: "source is healthy",
			}},
		},
	}
	hc := &fathomv1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "wrapper", Namespace: "wrapper-ns"},
		Spec: fathomv1alpha1.HealthCheckSpec{CheckRef: fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindAddonCheck,
			Name: target.Name,
		}},
	}

	identity := normalizeHealthCheckTarget(hc)
	if identity.APIVersion != fathomv1alpha1.GroupVersion.String() {
		t.Fatalf("normalized apiVersion = %q, want %q", identity.APIVersion, fathomv1alpha1.GroupVersion.String())
	}
	if identity.Namespace != hc.Namespace {
		t.Fatalf("normalized namespace = %q, want %q", identity.Namespace, hc.Namespace)
	}
	if identity.Name != target.Name || identity.Kind != healthCheckTargetKindAddonCheck {
		t.Fatalf("normalized identity = %#v", identity)
	}

	registry := newHealthCheckTargetRegistry()
	handler, ok := registry.lookup(identity.APIVersion, identity.Kind)
	if !ok {
		t.Fatalf("AddonCheck handler not registered for %#v", identity)
	}

	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(target).Build()
	snapshot, err := handler.read(context.Background(), cl, types.NamespacedName{
		Namespace: identity.Namespace,
		Name:      identity.Name,
	})
	if err != nil {
		t.Fatalf("read AddonCheck: %v", err)
	}
	if snapshot.Result != fathomv1alpha1.HealthReportResultPass || snapshot.Summary != "source is healthy" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Interval != defaultAddonCheckInterval {
		t.Fatalf("interval = %v, want %v", snapshot.Interval, defaultAddonCheckInterval)
	}
	if snapshot.SourceObservedAt == nil || !snapshot.SourceObservedAt.Equal(&runTime) {
		t.Fatalf("observed time = %v, want %v", snapshot.SourceObservedAt, &runTime)
	}
	if snapshot.LastReportName != "source-report" {
		t.Fatalf("report name = %q", snapshot.LastReportName)
	}

	hc.Status.Result = fathomv1alpha1.HealthReportResultFail
	hc.Status.Summary = "stale"
	hc.Status.LastReportName = "stale-report"
	applyHealthCheckTargetSnapshot(hc, snapshot)
	if hc.Status.Result != fathomv1alpha1.HealthReportResultPass || hc.Status.Summary != "source is healthy" || hc.Status.LastReportName != "source-report" {
		t.Fatalf("snapshot did not replace status: %#v", hc.Status)
	}

	before := hc.Status.DeepCopy()
	applyHealthCheckTargetSnapshot(hc, snapshot)
	if !equality.Semantic.DeepEqual(before, &hc.Status) {
		t.Fatalf("reapplying an unchanged snapshot changed status: before=%#v after=%#v", before, &hc.Status)
	}
}

func TestAddonCheckTargetHandlerBoundsSummary(t *testing.T) {
	t.Parallel()

	message := strings.Repeat("界", healthCheckSummaryMaxLen+10)
	target := &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "default"},
		Status: fathomv1alpha1.AddonCheckStatus{Conditions: []metav1.Condition{{
			Type:    healthCheckConditionReady,
			Status:  metav1.ConditionTrue,
			Message: message,
		}}},
	}
	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(target).Build()
	handler, ok := newHealthCheckTargetRegistry().lookup(fathomv1alpha1.GroupVersion.String(), healthCheckTargetKindAddonCheck)
	if !ok {
		t.Fatal("AddonCheck handler not registered")
	}
	snapshot, err := handler.read(context.Background(), cl, types.NamespacedName{Namespace: target.Namespace, Name: target.Name})
	if err != nil {
		t.Fatalf("read AddonCheck: %v", err)
	}
	if got := []rune(snapshot.Summary); len(got) != healthCheckSummaryMaxLen || got[len(got)-1] != '…' {
		t.Fatalf("bounded summary has %d runes and suffix %q", len(got), string(got[len(got)-1:]))
	}
}

func TestNormalizeHealthCheckTargetPreservesExplicitIdentity(t *testing.T) {
	t.Parallel()

	hc := &fathomv1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Namespace: "wrapper-ns"},
		Spec: fathomv1alpha1.HealthCheckSpec{CheckRef: fathomv1alpha1.CheckTargetRef{
			APIVersion: fathomv1alpha1.GroupVersion.String(),
			Kind:       healthCheckTargetKindAddonCheck,
			Namespace:  "source-ns",
			Name:       "source",
		}},
	}
	got := normalizeHealthCheckTarget(hc)
	want := healthCheckTargetIdentity{
		APIVersion: fathomv1alpha1.GroupVersion.String(),
		Kind:       healthCheckTargetKindAddonCheck,
		Namespace:  "source-ns",
		Name:       "source",
	}
	if got != want {
		t.Fatalf("identity = %#v, want %#v", got, want)
	}
}

func TestDNSCheckTargetHandlerProjection(t *testing.T) {
	t.Parallel()

	runTime := metav1.NewTime(time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second))
	target := &fathomv1alpha1.DNSCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "dns-source", Namespace: "source-ns"},
		Spec: fathomv1alpha1.DNSCheckSpec{
			Interval: &metav1.Duration{Duration: 2 * time.Minute},
		},
		Status: fathomv1alpha1.DNSCheckStatus{
			LastResult:     string(fathomv1alpha1.HealthReportResultFail),
			LastRunTime:    &runTime,
			LastReportName: "dns-source-report",
			Conditions: []metav1.Condition{{
				Type:    healthCheckConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  "RunFailed",
				Message: "required DNS name did not resolve",
			}},
		},
	}
	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(target).Build()
	handler, ok := newHealthCheckTargetRegistry().lookup(fathomv1alpha1.GroupVersion.String(), healthCheckTargetKindDNSCheck)
	if !ok {
		t.Fatal("DNSCheck handler not registered")
	}

	snapshot, err := handler.read(context.Background(), cl, types.NamespacedName{Namespace: target.Namespace, Name: target.Name})
	if err != nil {
		t.Fatalf("read DNSCheck: %v", err)
	}
	if snapshot.Result != fathomv1alpha1.HealthReportResultFail || snapshot.Summary != "required DNS name did not resolve" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Interval != 2*time.Minute {
		t.Fatalf("interval = %v, want 2m", snapshot.Interval)
	}
	if snapshot.SourceObservedAt == nil || !snapshot.SourceObservedAt.Equal(&runTime) {
		t.Fatalf("observed time = %v, want %v", snapshot.SourceObservedAt, &runTime)
	}
	if snapshot.LastReportName != "dns-source-report" {
		t.Fatalf("report name = %q", snapshot.LastReportName)
	}

	for _, tc := range []struct {
		name       string
		apiVersion string
		namespace  string
	}{
		{name: "empty apiVersion defaults", namespace: "source-ns"},
		{name: "explicit current apiVersion", apiVersion: fathomv1alpha1.GroupVersion.String(), namespace: "source-ns"},
		{name: "empty namespace defaults", apiVersion: fathomv1alpha1.GroupVersion.String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hc := &fathomv1alpha1.HealthCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "wrapper", Namespace: "source-ns"},
				Spec: fathomv1alpha1.HealthCheckSpec{CheckRef: fathomv1alpha1.CheckTargetRef{
					APIVersion: tc.apiVersion,
					Kind:       healthCheckTargetKindDNSCheck,
					Namespace:  tc.namespace,
					Name:       target.Name,
				}},
			}
			identity := normalizeHealthCheckTarget(hc)
			if identity.APIVersion != fathomv1alpha1.GroupVersion.String() || identity.Namespace != target.Namespace {
				t.Fatalf("normalized identity = %#v", identity)
			}
		})
	}
}

func TestNodeCertificateCheckTargetHandlerProjection(t *testing.T) {
	t.Parallel()

	runTime := metav1.NewTime(time.Now().Add(-time.Hour).UTC().Truncate(time.Second))
	targets := []client.Object{
		&fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "default-cadence", Namespace: "source-ns"},
			Status: fathomv1alpha1.NodeCertificateCheckStatus{
				LastResult:     string(fathomv1alpha1.HealthReportResultWarn),
				LastRunTime:    &runTime,
				LastReportName: "node-report",
				Conditions: []metav1.Condition{{
					Type:    healthCheckConditionReady,
					Status:  metav1.ConditionTrue,
					Reason:  "RunCompleted",
					Message: "one certificate expires soon",
				}},
			},
		},
		&fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "clamped-cadence", Namespace: "source-ns"},
			Spec: fathomv1alpha1.NodeCertificateCheckSpec{
				Interval: &metav1.Duration{Duration: time.Second},
			},
		},
		&fathomv1alpha1.NodeCertificateCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-status", Namespace: "source-ns"},
		},
	}
	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(targets...).Build()
	handler, ok := newHealthCheckTargetRegistry().lookup(fathomv1alpha1.GroupVersion.String(), healthCheckTargetKindNodeCertificateCheck)
	if !ok {
		t.Fatal("NodeCertificateCheck handler not registered")
	}

	snapshot, err := handler.read(context.Background(), cl, types.NamespacedName{Namespace: "source-ns", Name: "default-cadence"})
	if err != nil {
		t.Fatalf("read NodeCertificateCheck: %v", err)
	}
	if snapshot.Result != fathomv1alpha1.HealthReportResultWarn || snapshot.Summary != "one certificate expires soon" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.SourceObservedAt == nil || !snapshot.SourceObservedAt.Equal(&runTime) || snapshot.LastReportName != "node-report" {
		t.Fatalf("source evidence = %#v", snapshot)
	}
	if snapshot.Interval != defaultNodeCertInterval {
		t.Fatalf("default interval = %v, want %v", snapshot.Interval, defaultNodeCertInterval)
	}

	clamped, err := handler.read(context.Background(), cl, types.NamespacedName{Namespace: "source-ns", Name: "clamped-cadence"})
	if err != nil {
		t.Fatalf("read clamped NodeCertificateCheck: %v", err)
	}
	if clamped.Interval != fathomv1alpha1.MinCheckInterval {
		t.Fatalf("clamped interval = %v, want %v", clamped.Interval, fathomv1alpha1.MinCheckInterval)
	}

	empty, err := handler.read(context.Background(), cl, types.NamespacedName{Namespace: "source-ns", Name: "empty-status"})
	if err != nil {
		t.Fatalf("read empty NodeCertificateCheck: %v", err)
	}
	if empty.Result != "" || empty.Summary != "" || empty.SourceObservedAt != nil || empty.LastReportName != "" {
		t.Fatalf("empty source invented status: %#v", empty)
	}
	if empty.Interval != defaultNodeCertInterval {
		t.Fatalf("empty source interval = %v, want %v", empty.Interval, defaultNodeCertInterval)
	}
}

func TestHealthCheckTargetReferenceFailures(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	longLookupError := errors.New(strings.Repeat("lookup failed ", 4000))

	tests := []struct {
		name         string
		ref          fathomv1alpha1.CheckTargetRef
		cl           client.Client
		wantReason   string
		wantError    bool
		wantPreserve bool
	}{
		{
			name: "unsupported api version",
			ref: fathomv1alpha1.CheckTargetRef{
				APIVersion: "fathom.skaphos.io/v9", Kind: healthCheckTargetKindAddonCheck, Name: "source",
			},
			cl: baseClient, wantReason: "UnsupportedAPIVersion",
		},
		{
			name: "unsupported kind",
			ref: fathomv1alpha1.CheckTargetRef{
				Kind: "ReachabilityCheck", Name: "source",
			},
			cl: baseClient, wantReason: "UnsupportedKind",
		},
	}
	for _, kind := range []string{
		healthCheckTargetKindAddonCheck,
		healthCheckTargetKindDNSCheck,
		healthCheckTargetKindNodeCertificateCheck,
	} {
		tests = append(tests,
			struct {
				name         string
				ref          fathomv1alpha1.CheckTargetRef
				cl           client.Client
				wantReason   string
				wantError    bool
				wantPreserve bool
			}{
				name: "missing " + kind,
				ref:  fathomv1alpha1.CheckTargetRef{Kind: kind, Name: "missing"},
				cl:   baseClient, wantReason: "TargetNotFound",
			},
			struct {
				name         string
				ref          fathomv1alpha1.CheckTargetRef
				cl           client.Client
				wantReason   string
				wantError    bool
				wantPreserve bool
			}{
				name: "transient " + kind,
				ref:  fathomv1alpha1.CheckTargetRef{Kind: kind, Name: "source"},
				cl: failingHealthCheckTargetClient{
					Client: baseClient,
					err:    longLookupError,
				},
				wantReason: "TargetLookupFailed", wantError: true, wantPreserve: true,
			},
		)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hc := &fathomv1alpha1.HealthCheck{
				ObjectMeta: metav1.ObjectMeta{Name: "wrapper", Namespace: "default", Generation: 3},
				Spec:       fathomv1alpha1.HealthCheckSpec{CheckRef: tc.ref},
				Status: fathomv1alpha1.HealthCheckStatus{
					Result:           fathomv1alpha1.HealthReportResultPass,
					Summary:          "last readable summary",
					SourceInterval:   &metav1.Duration{Duration: time.Minute},
					SourceObservedAt: &metav1.Time{Time: time.Unix(100, 0)},
					LastReportName:   "last-readable-report",
				},
			}
			_, err := (&HealthCheckReconciler{Client: tc.cl, Scheme: scheme}).mirrorTarget(context.Background(), hc)
			if tc.wantError != (err != nil) {
				t.Fatalf("error = %v, wantError %v", err, tc.wantError)
			}
			ready := apiMeta.FindStatusCondition(hc.Status.Conditions, healthCheckConditionReady)
			if ready == nil || ready.Reason != tc.wantReason || ready.Status != metav1.ConditionFalse {
				t.Fatalf("Ready = %#v, want reason %s", ready, tc.wantReason)
			}
			if len([]rune(ready.Message)) > healthCheckConditionMessageMaxLen {
				t.Fatalf("Ready message has %d runes, max %d", len([]rune(ready.Message)), healthCheckConditionMessageMaxLen)
			}
			if tc.wantPreserve {
				if hc.Status.Result != fathomv1alpha1.HealthReportResultPass || hc.Status.Summary != "last readable summary" ||
					hc.Status.SourceInterval == nil || hc.Status.SourceObservedAt == nil || hc.Status.LastReportName != "last-readable-report" {
					t.Fatalf("transient failure did not preserve snapshot: %#v", hc.Status)
				}
				return
			}
			if hc.Status.Result != "" || hc.Status.Summary != "" || hc.Status.SourceInterval != nil ||
				hc.Status.SourceObservedAt != nil || hc.Status.LastReportName != "" {
				t.Fatalf("terminal failure did not clear snapshot: %#v", hc.Status)
			}
		})
	}
}
