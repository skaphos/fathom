/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

	t.Run("empty namespace falls back to the operator client", func(t *testing.T) {
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
			t.Error("expected the operator client when Namespace is empty")
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
