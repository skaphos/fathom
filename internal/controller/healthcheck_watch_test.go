/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func TestHealthChecksForAddonCheckUsesExactNormalizedReference(t *testing.T) {
	t.Parallel()

	checks := []client.Object{
		newWatchHealthCheck("default-version", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindAddonCheck, Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("explicit-version", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: healthCheckTargetKindAddonCheck, Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("default-namespace", "source-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindAddonCheck, Name: "source",
		}),
		newWatchHealthCheck("wrong-kind", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: "DNSCheck", Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("wrong-namespace", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindAddonCheck, Namespace: "other", Name: "source",
		}),
		newWatchHealthCheck("wrong-name", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindAddonCheck, Namespace: "source-ns", Name: "other",
		}),
	}

	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(checks...).Build()
	r := &HealthCheckReconciler{Client: cl, Scheme: scheme}
	handler, ok := newHealthCheckTargetRegistry().lookup(fathomv1alpha1.GroupVersion.String(), healthCheckTargetKindAddonCheck)
	if !ok {
		t.Fatal("AddonCheck handler not registered")
	}

	source := &fathomv1alpha1.AddonCheck{ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns"}}
	requests := r.healthChecksForTarget(context.Background(), source, handler)
	got := make([]string, 0, len(requests))
	for _, request := range requests {
		got = append(got, request.Namespace+"/"+request.Name)
	}
	want := map[string]bool{
		"wrapper-ns/default-version":  true,
		"wrapper-ns/explicit-version": true,
		"source-ns/default-namespace": true,
	}
	if len(got) != len(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for _, request := range got {
		if !want[request] {
			t.Fatalf("unexpected request %q in %v", request, got)
		}
	}

	wrongType := &fathomv1alpha1.DNSCheck{ObjectMeta: source.ObjectMeta}
	if got := r.healthChecksForTarget(context.Background(), wrongType, handler); len(got) != 0 {
		t.Fatalf("wrong concrete source type enqueued %v", got)
	}
}

func newWatchHealthCheck(name, namespace string, ref fathomv1alpha1.CheckTargetRef) *fathomv1alpha1.HealthCheck {
	return &fathomv1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       fathomv1alpha1.HealthCheckSpec{CheckRef: ref},
	}
}

func TestHealthChecksForDNSCheckUsesExactNormalizedReference(t *testing.T) {
	t.Parallel()

	checks := []client.Object{
		newWatchHealthCheck("default-version", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindDNSCheck, Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("explicit-version", "source-ns", fathomv1alpha1.CheckTargetRef{
			APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: healthCheckTargetKindDNSCheck, Name: "source",
		}),
		newWatchHealthCheck("wrong-version", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			APIVersion: "fathom.skaphos.io/v9", Kind: healthCheckTargetKindDNSCheck, Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("wrong-kind", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindAddonCheck, Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("wrong-namespace", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindDNSCheck, Namespace: "other", Name: "source",
		}),
		newWatchHealthCheck("wrong-name", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindDNSCheck, Namespace: "source-ns", Name: "other",
		}),
	}

	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	r := &HealthCheckReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(checks...).Build(),
		Scheme: scheme,
	}
	handler, ok := newHealthCheckTargetRegistry().lookup(fathomv1alpha1.GroupVersion.String(), healthCheckTargetKindDNSCheck)
	if !ok {
		t.Fatal("DNSCheck handler not registered")
	}

	source := &fathomv1alpha1.DNSCheck{ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns"}}
	requests := r.healthChecksForTarget(context.Background(), source, handler)
	got := make(map[string]bool, len(requests))
	for _, request := range requests {
		got[request.Namespace+"/"+request.Name] = true
	}
	want := map[string]bool{
		"wrapper-ns/default-version": true,
		"source-ns/explicit-version": true,
	}
	if len(got) != len(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for request := range want {
		if !got[request] {
			t.Fatalf("missing request %q in %v", request, got)
		}
	}
}

func TestHealthChecksForNodeCertificateCheckUsesExactNormalizedReference(t *testing.T) {
	t.Parallel()

	checks := []client.Object{
		newWatchHealthCheck("matching", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindNodeCertificateCheck, Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("wrong-version", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			APIVersion: "fathom.skaphos.io/v9", Kind: healthCheckTargetKindNodeCertificateCheck, Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("wrong-kind", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindDNSCheck, Namespace: "source-ns", Name: "source",
		}),
		newWatchHealthCheck("wrong-namespace", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindNodeCertificateCheck, Namespace: "other", Name: "source",
		}),
		newWatchHealthCheck("wrong-name", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
			Kind: healthCheckTargetKindNodeCertificateCheck, Namespace: "source-ns", Name: "other",
		}),
	}

	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	r := &HealthCheckReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(checks...).Build(),
		Scheme: scheme,
	}
	handler, ok := newHealthCheckTargetRegistry().lookup(fathomv1alpha1.GroupVersion.String(), healthCheckTargetKindNodeCertificateCheck)
	if !ok {
		t.Fatal("NodeCertificateCheck handler not registered")
	}

	source := &fathomv1alpha1.NodeCertificateCheck{ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns"}}
	requests := r.healthChecksForTarget(context.Background(), source, handler)
	if len(requests) != 1 || requests[0].Namespace != "wrapper-ns" || requests[0].Name != "matching" {
		t.Fatalf("requests = %v, want wrapper-ns/matching", requests)
	}
}

func TestHealthChecksForTargetDeletionEnqueuesExactReference(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	matching := newWatchHealthCheck("matching", "wrapper-ns", fathomv1alpha1.CheckTargetRef{
		Kind: healthCheckTargetKindAddonCheck, Namespace: "source-ns", Name: "deleted-source",
	})
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(matching).Build()
	r := &HealthCheckReconciler{Client: cl, Scheme: scheme}
	handler, ok := newHealthCheckTargetRegistry().lookup(fathomv1alpha1.GroupVersion.String(), healthCheckTargetKindAddonCheck)
	if !ok {
		t.Fatal("AddonCheck handler not registered")
	}
	deletedAt := metav1.NewTime(time.Now())
	deleted := &fathomv1alpha1.AddonCheck{ObjectMeta: metav1.ObjectMeta{
		Name: "deleted-source", Namespace: "source-ns", DeletionTimestamp: &deletedAt,
	}}
	requests := r.healthChecksForTarget(context.Background(), deleted, handler)
	if len(requests) != 1 || requests[0].Namespace != matching.Namespace || requests[0].Name != matching.Name {
		t.Fatalf("deletion requests = %v, want %s/%s", requests, matching.Namespace, matching.Name)
	}
}
