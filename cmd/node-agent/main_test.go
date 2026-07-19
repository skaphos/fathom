/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/nodecert"
)

func writeCert(t *testing.T, path string, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "apiserver"},
		NotBefore:    notAfter.Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseConfig(t *testing.T) {
	t.Run("requires check identity", func(t *testing.T) {
		if _, err := parseConfig([]string{"--node-name", "n1"}); err == nil {
			t.Fatal("expected error for missing --check-name/--check-namespace")
		}
	})

	t.Run("defaults configmap name from check and node", func(t *testing.T) {
		cfg, err := parseConfig([]string{"--check-name", "nc", "--check-namespace", "fathom-system", "--node-name", "node-1"})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.configMapName != nodecert.NodeReportConfigMapName("nc", "node-1") {
			t.Errorf("configMapName = %q", cfg.configMapName)
		}
		if cfg.thresholds.WarnDays != 30 || cfg.thresholds.CriticalDays != 7 {
			t.Errorf("default thresholds wrong: %+v", cfg.thresholds)
		}
	})

	t.Run("reads node name from env", func(t *testing.T) {
		t.Setenv("NODE_NAME", "env-node")
		cfg, err := parseConfig([]string{"--check-name", "nc", "--check-namespace", "ns"})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.nodeName != "env-node" {
			t.Errorf("nodeName = %q, want env-node", cfg.nodeName)
		}
	})
}

func TestScanAndPublishCreatesAndUpdatesConfigMap(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "apiserver.crt")
	writeCert(t, certPath, time.Now().Add(20*24*time.Hour)) // within warn window

	kube := fake.NewSimpleClientset()
	cfg := config{
		checkName:      "nc",
		checkNamespace: "fathom-system",
		nodeName:       "node-1",
		configMapName:  nodecert.NodeReportConfigMapName("nc", "node-1"),
		paths:          []string{certPath},
		thresholds:     nodecert.Thresholds{WarnDays: 30, CriticalDays: 7},
		timeout:        5 * time.Second,
	}

	report, err := scanAndPublish(context.Background(), kube, cfg, time.Now())
	if err != nil {
		t.Fatalf("scanAndPublish: %v", err)
	}
	if report.Aggregate != nodecert.OutcomeWarn || len(report.Certs) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}

	cm, err := kube.CoreV1().ConfigMaps("fathom-system").Get(context.Background(), cfg.configMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if cm.Labels[nodecert.LabelSourceName] != "nc" || cm.Labels[nodecert.LabelSourceKind] != nodecert.KindNodeCertificateCheck {
		t.Errorf("labels wrong: %v", cm.Labels)
	}
	// The authenticity anchor: the raw node name is stamped as an annotation the
	// operator's ValidatingAdmissionPolicy binds to the writer's token node claim.
	if cm.Annotations[nodecert.AnnotationNodeName] != "node-1" {
		t.Errorf("node-name annotation = %q, want %q", cm.Annotations[nodecert.AnnotationNodeName], "node-1")
	}
	decoded, err := nodecert.DecodeReport(cm.Data[nodecert.ConfigMapReportKey])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Node != "node-1" || len(decoded.Certs) != 1 {
		t.Errorf("decoded report wrong: %+v", decoded)
	}

	// Gauge is populated for the parsed certificate.
	if got := testutil.CollectAndCount(metrics.NodeCertificateExpiryDays); got < 1 {
		t.Errorf("expected at least one expiry-days series, got %d", got)
	}

	// A second publish updates the same ConfigMap (no duplicate, no error).
	if _, err := scanAndPublish(context.Background(), kube, cfg, time.Now()); err != nil {
		t.Fatalf("second publish: %v", err)
	}
	list, err := kube.CoreV1().ConfigMaps("fathom-system").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Errorf("want exactly 1 ConfigMap after re-publish, got %d", len(list.Items))
	}
}

func TestSanitizeLabelValue(t *testing.T) {
	cases := map[string]string{
		"node-1":              "node-1",
		"NODE/With*Bad:Chars": "NODE-With-Bad-Chars",
		"":                    "unknown",
		"---":                 "unknown",
	}
	for in, want := range cases {
		if got := sanitizeLabelValue(in); got != want {
			t.Errorf("sanitizeLabelValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a , b ,, c ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if splitCSV("  ") != nil {
		t.Errorf("blank should be nil")
	}
}
