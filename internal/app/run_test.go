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
	"testing"
	"time"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

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
	// HealthCheck is the canonical CRD; if it's registered the others will be too.
	gvks := scheme.AllKnownTypes()
	found := false
	for gvk := range gvks {
		if gvk.Group == "fathom.skaphos.io" && gvk.Kind == "HealthCheck" {
			found = true
			break
		}
	}
	if !found {
		t.Error("HealthCheck not registered in scheme")
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

