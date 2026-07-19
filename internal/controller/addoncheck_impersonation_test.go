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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	"github.com/skaphos/fathom/pkg/adapter"
)

// fakeClientFactory records the username it was asked to impersonate and returns
// a canned client, standing in for the real impersonation.ClientFactory.
type fakeClientFactory struct {
	client   client.Client
	lastUser string
	err      error
}

func (f *fakeClientFactory) ClientFor(username string) (client.Client, error) {
	f.lastUser = username
	if f.err != nil {
		return nil, f.err
	}
	return f.client, nil
}

func addonSA(name, namespace, addon string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{adapter.AddonLabel: addon},
		},
	}
}

func TestAdapterClient(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	t.Run("nil factory falls back to the operator client", func(t *testing.T) {
		operatorClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &AddonCheckReconciler{Client: operatorClient, Namespace: "fathom-system"}
		got, err := r.adapterClient(context.Background(), "coredns")
		if err != nil {
			t.Fatalf("adapterClient: %v", err)
		}
		if got != operatorClient {
			t.Error("expected the operator client when AddonClients is nil")
		}
	})

	t.Run("empty namespace out-of-cluster falls back to the operator client", func(t *testing.T) {
		restore := impersonation.SetRunningInClusterForTest(false)
		t.Cleanup(restore)

		operatorClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &AddonCheckReconciler{
			Client:       operatorClient,
			AddonClients: &fakeClientFactory{client: fake.NewClientBuilder().WithScheme(scheme).Build()},
			Namespace:    "",
		}
		got, err := r.adapterClient(context.Background(), "coredns")
		if err != nil {
			t.Fatalf("adapterClient: %v", err)
		}
		if got != operatorClient {
			t.Error("expected the operator client when Namespace is empty out of cluster")
		}
	})

	t.Run("empty namespace in-cluster fails closed", func(t *testing.T) {
		// SKA-162: never hand adapters the operator client when impersonation is
		// configured but FATHOM_NAMESPACE was dropped from the in-cluster pod.
		restore := impersonation.SetRunningInClusterForTest(true)
		t.Cleanup(restore)

		operatorClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &AddonCheckReconciler{
			Client:       operatorClient,
			AddonClients: &fakeClientFactory{client: fake.NewClientBuilder().WithScheme(scheme).Build()},
			Namespace:    "",
		}
		got, err := r.adapterClient(context.Background(), "coredns")
		if err == nil {
			t.Fatal("expected an error when Namespace is empty in-cluster")
		}
		if got != nil {
			t.Error("expected a nil client on fail-closed")
		}
		if !strings.Contains(err.Error(), "SKA-162") {
			t.Errorf("error %q should name SKA-162", err.Error())
		}
	})

	t.Run("resolves the labeled ServiceAccount and impersonates it", func(t *testing.T) {
		sa := addonSA("fathom-addon-coredns", "fathom-system", "coredns")
		operatorClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa).Build()
		scoped := fake.NewClientBuilder().WithScheme(scheme).Build()
		factory := &fakeClientFactory{client: scoped}
		r := &AddonCheckReconciler{Client: operatorClient, AddonClients: factory, Namespace: "fathom-system"}

		got, err := r.adapterClient(context.Background(), "coredns")
		if err != nil {
			t.Fatalf("adapterClient: %v", err)
		}
		if got != scoped {
			t.Error("expected the impersonating client, not the operator client")
		}
		if want := "system:serviceaccount:fathom-system:fathom-addon-coredns"; factory.lastUser != want {
			t.Errorf("impersonation username = %q, want %q", factory.lastUser, want)
		}
	})

	t.Run("errors when the addon ServiceAccount is not installed", func(t *testing.T) {
		operatorClient := fake.NewClientBuilder().WithScheme(scheme).Build() // no SAs
		r := &AddonCheckReconciler{
			Client:       operatorClient,
			AddonClients: &fakeClientFactory{client: fake.NewClientBuilder().WithScheme(scheme).Build()},
			Namespace:    "fathom-system",
		}
		if _, err := r.adapterClient(context.Background(), "coredns"); err == nil {
			t.Fatal("expected an error when no labeled ServiceAccount exists")
		}
	})

	t.Run("errors when multiple ServiceAccounts share the addon label", func(t *testing.T) {
		// Two installs (e.g. two Fathom releases) both labeled for coredns: the
		// resolution is ambiguous, so it must fail closed rather than pick one.
		sa1 := addonSA("fathom-addon-coredns", "fathom-system", "coredns")
		sa2 := addonSA("other-addon-coredns", "fathom-system", "coredns")
		operatorClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa1, sa2).Build()
		r := &AddonCheckReconciler{
			Client:       operatorClient,
			AddonClients: &fakeClientFactory{client: fake.NewClientBuilder().WithScheme(scheme).Build()},
			Namespace:    "fathom-system",
		}
		if _, err := r.adapterClient(context.Background(), "coredns"); err == nil {
			t.Fatal("expected an error when multiple labeled ServiceAccounts exist")
		}
	})

	t.Run("propagates a factory error", func(t *testing.T) {
		sa := addonSA("fathom-addon-coredns", "fathom-system", "coredns")
		operatorClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa).Build()
		r := &AddonCheckReconciler{
			Client:       operatorClient,
			AddonClients: &fakeClientFactory{err: errors.New("boom")},
			Namespace:    "fathom-system",
		}
		if _, err := r.adapterClient(context.Background(), "coredns"); err == nil {
			t.Fatal("expected the factory error to propagate")
		}
	})
}

// TestRunAddonCheckFailsClosedWhenNamespaceEmptyInCluster locks SKA-162: a
// missing operator namespace while in-cluster must not run the adapter under the
// operator SA. The failure is recorded as AdapterRunFailed, same as other
// scoped-client errors.
func TestRunAddonCheckFailsClosedWhenNamespaceEmptyInCluster(t *testing.T) {
	restore := impersonation.SetRunningInClusterForTest(true)
	t.Cleanup(restore)

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add fathom scheme: %v", err)
	}

	check := &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "no-ns", Namespace: "default"},
		Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "prog-cert-manager"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check).Build()
	prog := &programmableAdapter{}
	r := &AddonCheckReconciler{
		Client:       cl,
		Scheme:       scheme,
		AddonClients: &fakeClientFactory{client: cl},
		Namespace:    "",
	}

	if err := r.runAddonCheck(context.Background(), logr.Discard(), check, prog); err != nil {
		t.Fatalf("runAddonCheck returned an unexpected error: %v", err)
	}
	if prog.runCount() != 0 {
		t.Errorf("adapter Run was invoked %d times; must not run when namespace is empty in-cluster", prog.runCount())
	}
	cond := apimeta.FindStatusCondition(check.Status.Conditions, addonCheckConditionReady)
	if cond == nil {
		t.Fatal("expected a Ready condition to be set")
	}
	if cond.Status != metav1.ConditionFalse || cond.Reason != "AdapterRunFailed" {
		t.Errorf("Ready condition = %s/%s, want False/AdapterRunFailed", cond.Status, cond.Reason)
	}
	if !strings.Contains(cond.Message, "SKA-162") {
		t.Errorf("Ready message %q should name SKA-162", cond.Message)
	}
}

// TestRunAddonCheckFailsClosedWithoutScopedClient locks the security-critical
// fail-closed behavior: when impersonation is configured but the addon's scoped
// ServiceAccount is absent, runAddonCheck must NOT execute the adapter against the
// operator client — it records an adapter-level failure instead. A regression that
// changed this to fall back to r.Client would reopen the door SKA-58 closed, and
// this test would catch it.
func TestRunAddonCheckFailsClosedWithoutScopedClient(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add fathom scheme: %v", err)
	}

	check := &fathomv1alpha1.AddonCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "failclosed", Namespace: "default"},
		Spec:       fathomv1alpha1.AddonCheckSpec{AddonType: "prog-cert-manager"},
	}
	// The fake cluster has the AddonCheck but NO addon ServiceAccount, so the
	// scoped-client lookup fails. AddonClients is set and Namespace is non-empty,
	// so impersonation is active (not the operator-client fallback path).
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(check).Build()
	prog := &programmableAdapter{}
	r := &AddonCheckReconciler{
		Client:       cl,
		Scheme:       scheme,
		AddonClients: &fakeClientFactory{client: cl},
		Namespace:    "fathom-system",
	}

	// runAddonCheck maps adapter failures to a health condition, not a reconcile
	// error, so it returns nil.
	if err := r.runAddonCheck(context.Background(), logr.Discard(), check, prog); err != nil {
		t.Fatalf("runAddonCheck returned an unexpected error: %v", err)
	}
	if prog.runCount() != 0 {
		t.Errorf("adapter Run was invoked %d times; it must NOT run when the scoped client is unavailable", prog.runCount())
	}
	cond := apimeta.FindStatusCondition(check.Status.Conditions, addonCheckConditionReady)
	if cond == nil {
		t.Fatal("expected a Ready condition to be set")
	}
	if cond.Status != metav1.ConditionFalse || cond.Reason != "AdapterRunFailed" {
		t.Errorf("Ready condition = %s/%s, want False/AdapterRunFailed", cond.Status, cond.Reason)
	}
}
