/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/skaphos/fathom/pkg/adapter"
)

type appFakeAdapter struct{}

func (appFakeAdapter) Name() string            { return "fake-cert-manager" }
func (appFakeAdapter) Version() string         { return "0.1.0" }
func (appFakeAdapter) ContractVersion() string { return adapter.ContractVersion }
func (appFakeAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"cert-manager"}, Families: []adapter.Family{"system_health"}}
}
func (appFakeAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{}, nil
}

// writeSelfSignedCert writes a tls.crt + tls.key pair into dir. The cert is
// minimal but valid PEM, which is enough for certwatcher.New to accept.
func writeSelfSignedCert(t *testing.T, dir string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tls.key"), keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func TestNewScheme_RegistersFathomTypes(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	// AddonCheck exercises the newest API type; if it is registered the package
	// init hooks are being included in the application scheme.
	gvks := scheme.AllKnownTypes()
	found := false
	for gvk := range gvks {
		if gvk.Group == "fathom.skaphos.io" && gvk.Kind == "AddonCheck" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AddonCheck not registered in scheme")
	}
}

// TestNewScheme_RegistersAPIExtensions guards the CRD-presence checks in the
// cert-manager and external-secrets adapters, which Get CustomResourceDefinition
// objects. envtest auto-registers apiextensions/v1 so unit tests against the
// adapters pass even without explicit registration; real-cluster reconciles
// surface "no kind is registered for the type v1.CustomResourceDefinition" if
// NewScheme drops the AddToScheme call below.
func TestNewScheme_RegistersAPIExtensions(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	gvks := scheme.AllKnownTypes()
	found := false
	for gvk := range gvks {
		if gvk.Group == "apiextensions.k8s.io" && gvk.Version == "v1" && gvk.Kind == "CustomResourceDefinition" {
			found = true
			break
		}
	}
	if !found {
		t.Error("apiextensions.k8s.io/v1 CustomResourceDefinition not registered in scheme")
	}
}

func TestBuildManagerOptions_DefaultsHaveNoCertWatchers(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	mgrOpts, watchers, err := BuildManagerOptions(DefaultOptions(), scheme)
	if err != nil {
		t.Fatalf("BuildManagerOptions: %v", err)
	}
	if len(watchers) != 0 {
		t.Errorf("watchers: got %d, want 0", len(watchers))
	}
	if mgrOpts.Metrics.BindAddress != "0" {
		t.Errorf("Metrics.BindAddress: got %q, want 0", mgrOpts.Metrics.BindAddress)
	}
	if !mgrOpts.Metrics.SecureServing {
		t.Error("Metrics.SecureServing should be true by default")
	}
	if mgrOpts.Metrics.FilterProvider == nil {
		t.Error("FilterProvider should be set when Secure=true")
	}
	if mgrOpts.HealthProbeBindAddress != ":8081" {
		t.Errorf("HealthProbeBindAddress: got %q, want :8081", mgrOpts.HealthProbeBindAddress)
	}
}

func TestBuildManagerOptions_InsecureMetricsHasNoFilter(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	opts := DefaultOptions()
	opts.Metrics.Secure = false

	mgrOpts, _, err := BuildManagerOptions(opts, scheme)
	if err != nil {
		t.Fatalf("BuildManagerOptions: %v", err)
	}
	if mgrOpts.Metrics.SecureServing {
		t.Error("SecureServing should be false")
	}
	if mgrOpts.Metrics.FilterProvider != nil {
		t.Error("FilterProvider should be nil when Secure=false")
	}
}

func TestBuildManagerOptions_CertWatchers(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}

	dir := t.TempDir()
	writeSelfSignedCert(t, dir)

	opts := DefaultOptions()
	opts.Webhook.CertPath = dir
	opts.Metrics.CertPath = dir

	_, watchers, err := BuildManagerOptions(opts, scheme)
	if err != nil {
		t.Fatalf("BuildManagerOptions: %v", err)
	}
	if len(watchers) != 2 {
		t.Errorf("watchers: got %d, want 2 (webhook + metrics)", len(watchers))
	}
}

func TestBuildManagerOptions_MissingWebhookCert_Errors(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	opts := DefaultOptions()
	opts.Webhook.CertPath = filepath.Join(t.TempDir(), "missing")

	_, _, err = BuildManagerOptions(opts, scheme)
	if err == nil {
		t.Fatal("expected error for missing webhook cert")
	}
	if !strings.Contains(err.Error(), "webhook cert watcher") {
		t.Errorf("error wrap: %v", err)
	}
}

func TestBuildAdapterRegistry_RegistersBuiltInAdapters(t *testing.T) {
	adapterRegistry, err := BuildAdapterRegistry(logr.Discard(), appFakeAdapter{})
	if err != nil {
		t.Fatalf("BuildAdapterRegistry: %v", err)
	}

	got, err := adapterRegistry.Lookup("cert-manager")
	if err != nil {
		t.Fatalf("Lookup(cert-manager): %v", err)
	}
	if got.Name() != "fake-cert-manager" {
		t.Fatalf("adapter name: got %q, want fake-cert-manager", got.Name())
	}
}

func TestBuildAdapterRegistry_WrapsRegistrationErrors(t *testing.T) {
	_, err := BuildAdapterRegistry(logr.Discard(), nil)
	if err == nil {
		t.Fatal("expected error for nil built-in adapter")
	}
	if !strings.Contains(err.Error(), "register built-in adapter") {
		t.Errorf("error wrap: %v", err)
	}
	if !strings.Contains(err.Error(), "<nil>") {
		t.Errorf("adapter identity missing from error: %v", err)
	}
}

func TestBuiltInAdapters_IncludesCertManager(t *testing.T) {
	adapterRegistry, err := BuildAdapterRegistry(logr.Discard(), builtInAdapters()...)
	if err != nil {
		t.Fatalf("BuildAdapterRegistry: %v", err)
	}
	got, err := adapterRegistry.Lookup("cert-manager")
	if err != nil {
		t.Fatalf("Lookup(cert-manager): %v", err)
	}
	if got.Name() != "cert-manager" {
		t.Fatalf("adapter name: got %q, want cert-manager", got.Name())
	}
	got, err = adapterRegistry.Lookup("coredns")
	if err != nil {
		t.Fatalf("Lookup(coredns): %v", err)
	}
	if got.Name() != "coredns" {
		t.Fatalf("adapter name: got %q, want coredns", got.Name())
	}
	got, err = adapterRegistry.Lookup("external-secrets")
	if err != nil {
		t.Fatalf("Lookup(external-secrets): %v", err)
	}
	if got.Name() != "external-secrets" {
		t.Fatalf("adapter name: got %q, want external-secrets", got.Name())
	}
}

func TestReadyzCheck(t *testing.T) {
	var synced atomic.Bool

	check := readyzCheck(&synced)
	if err := check(nil); err == nil {
		t.Fatal("readyzCheck before sync: want error, got nil")
	} else if !strings.Contains(err.Error(), "informers not synced") {
		t.Errorf("readyzCheck before sync: error %q should name the unsynced state", err.Error())
	}

	synced.Store(true)
	if err := check(nil); err != nil {
		t.Fatalf("readyzCheck after sync: want nil, got %v", err)
	}
}

func TestRun_NilConfig(t *testing.T) {
	err := Run(context.Background(), nil, DefaultOptions(), nil)
	if err == nil {
		t.Fatal("expected error for nil rest.Config")
	}
}

func TestRun_InvalidOptions(t *testing.T) {
	opts := DefaultOptions()
	opts.HealthProbeBindAddress = "" // forces validation failure

	err := Run(context.Background(), &rest.Config{}, opts, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid options") {
		t.Errorf("error wrap: %v", err)
	}
}

func TestRun_ManagerCreationFailure(t *testing.T) {
	// Replace the manager factory with one that always errors so we can
	// exercise Run's error-wrapping without standing up envtest.
	original := managerFactory
	t.Cleanup(func() { managerFactory = original })
	managerFactory = func(*rest.Config, ctrl.Options) (ctrl.Manager, error) {
		return nil, errors.New("synthetic manager failure")
	}

	err := Run(context.Background(), &rest.Config{}, DefaultOptions(), nil)
	if err == nil {
		t.Fatal("expected manager creation error")
	}
	if !strings.Contains(err.Error(), "create manager") {
		t.Errorf("error wrap: %v", err)
	}
	if !strings.Contains(err.Error(), "synthetic manager failure") {
		t.Errorf("underlying error not wrapped: %v", err)
	}
}
