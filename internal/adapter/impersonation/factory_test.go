/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSAUsername(t *testing.T) {
	got := SAUsername("fathom-system", "fathom-addon-coredns")
	want := "system:serviceaccount:fathom-system:fathom-addon-coredns"
	if got != want {
		t.Fatalf("SAUsername = %q, want %q", got, want)
	}
}

func TestClientForSetsImpersonationAndMemoizes(t *testing.T) {
	base := &rest.Config{Host: "https://example.test"}
	var gotConfigs []*rest.Config
	calls := 0
	f := &factory{
		base:   base,
		scheme: runtime.NewScheme(),
		newClient: func(cfg *rest.Config, _ client.Options) (client.Client, error) {
			calls++
			gotConfigs = append(gotConfigs, cfg)
			return fake.NewClientBuilder().Build(), nil
		},
		clients: map[string]client.Client{},
	}

	user := SAUsername("fathom-system", "fathom-addon-coredns")
	c1, err := f.ClientFor(user)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	c2, err := f.ClientFor(user)
	if err != nil {
		t.Fatalf("ClientFor (second): %v", err)
	}

	if c1 != c2 {
		t.Error("expected the same client memoized for a repeated username")
	}
	if calls != 1 {
		t.Errorf("expected newClient called once for a repeated username, got %d", calls)
	}
	if got := gotConfigs[0].Impersonate.UserName; got != user {
		t.Errorf("impersonation username = %q, want %q", got, user)
	}
	// The shared base config must never be mutated — every client copies it.
	if base.Impersonate.UserName != "" {
		t.Errorf("base config was mutated: Impersonate.UserName = %q", base.Impersonate.UserName)
	}

	other := SAUsername("fathom-system", "fathom-addon-cilium")
	c3, err := f.ClientFor(other)
	if err != nil {
		t.Fatalf("ClientFor (other): %v", err)
	}
	if c3 == c1 {
		t.Error("expected a distinct client for a different username")
	}
	if calls != 2 {
		t.Errorf("expected newClient called twice across two distinct usernames, got %d", calls)
	}
}
