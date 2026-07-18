/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodecert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)

// makeCertPEM returns a self-signed certificate PEM with the given subject CN,
// expiry, and SANs. Self-signed means issuer == subject, which is enough to
// exercise subject/issuer/SAN extraction.
func makeCertPEM(t *testing.T, cn string, notAfter time.Time, dnsNames []string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestClassifyBoundaries(t *testing.T) {
	th := Thresholds{WarnDays: 30, CriticalDays: 7}
	cases := []struct {
		name  string
		after time.Duration
		want  Outcome
	}{
		{"far future", 365 * 24 * time.Hour, OutcomePass},
		{"just over warn", 31 * 24 * time.Hour, OutcomePass},
		{"at warn boundary", 30 * 24 * time.Hour, OutcomeWarn},
		{"within warn", 20 * 24 * time.Hour, OutcomeWarn},
		{"at critical boundary", 7 * 24 * time.Hour, OutcomeFail},
		{"within critical", 3 * 24 * time.Hour, OutcomeFail},
		{"expired", -24 * time.Hour, OutcomeFail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pemData := makeCertPEM(t, "test", fixedNow.Add(tc.after), nil)
			certs, err := parsePEMCertificates(pemData)
			if err != nil || len(certs) != 1 {
				t.Fatalf("parse: %v len=%d", err, len(certs))
			}
			got := classify(certs[0], fixedNow, th, "/p", "file")
			if got.Outcome != tc.want {
				t.Fatalf("outcome = %s, want %s (days=%d)", got.Outcome, tc.want, got.DaysRemaining)
			}
		})
	}
}

func TestDaysFromDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want int
	}{
		{0, 0},
		{36 * time.Hour, 2},   // ceil 1.5 -> 2
		{24 * time.Hour, 1},   // exactly 1
		{-24 * time.Hour, -1}, // floor -1.0 -> -1
		{-36 * time.Hour, -2}, // floor -1.5 -> -2
		{time.Hour, 1},        // ceil 0.04 -> 1
	}
	for _, tc := range cases {
		if got := daysFromDuration(tc.d); got != tc.want {
			t.Errorf("daysFromDuration(%s) = %d, want %d", tc.d, got, tc.want)
		}
	}
}

func TestScanSingleFile(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "apiserver.crt", makeCertPEM(t, "kube-apiserver", fixedNow.Add(10*24*time.Hour), []string{"kubernetes", "10.0.0.1"}))

	results := Scan(ScanOptions{Paths: []string{p}, Thresholds: Thresholds{WarnDays: 30, CriticalDays: 7}, Now: fixedNow})
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Outcome != OutcomeWarn {
		t.Errorf("outcome = %s, want Warn", r.Outcome)
	}
	if r.Subject == "" || r.Issuer == "" {
		t.Errorf("subject/issuer not populated: %+v", r)
	}
	if len(r.SANs) != 2 {
		t.Errorf("SANs = %v, want 2", r.SANs)
	}
	if r.NotAfter.IsZero() {
		t.Errorf("notAfter not set")
	}
}

func TestScanDirectoryRecursiveAndIgnoresNonCerts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pki/apiserver.crt", makeCertPEM(t, "apiserver", fixedNow.Add(100*24*time.Hour), nil))
	writeFile(t, dir, "pki/etcd/server.crt", makeCertPEM(t, "etcd", fixedNow.Add(100*24*time.Hour), nil))
	writeFile(t, dir, "pki/README.md", []byte("not a cert"))
	writeFile(t, dir, "pki/apiserver.key", []byte("-----BEGIN EC PRIVATE KEY-----\nx\n-----END EC PRIVATE KEY-----\n"))

	results := Scan(ScanOptions{Paths: []string{filepath.Join(dir, "pki")}, Thresholds: Thresholds{WarnDays: 30, CriticalDays: 7}, Now: fixedNow})
	if len(results) != 2 {
		t.Fatalf("want 2 cert results (recursive, ignoring non-certs), got %d: %+v", len(results), results)
	}
	for _, r := range results {
		if r.Outcome != OutcomePass {
			t.Errorf("path %s outcome = %s, want Pass", r.Path, r.Outcome)
		}
	}
}

func TestScanBundleEmitsPerCert(t *testing.T) {
	dir := t.TempDir()
	bundle := append(makeCertPEM(t, "leaf", fixedNow.Add(50*24*time.Hour), nil), makeCertPEM(t, "intermediate", fixedNow.Add(3*24*time.Hour), nil)...)
	p := writeFile(t, dir, "chain.pem", bundle)

	results := Scan(ScanOptions{Paths: []string{p}, Thresholds: Thresholds{WarnDays: 30, CriticalDays: 7}, Now: fixedNow})
	if len(results) != 2 {
		t.Fatalf("want 2 results from bundle, got %d", len(results))
	}
	if WorstOutcome(results) != OutcomeFail {
		t.Errorf("worst = %s, want Fail (intermediate expires in 3 days)", WorstOutcome(results))
	}
}

func TestScanKubeconfig(t *testing.T) {
	dir := t.TempDir()
	clientCert := makeCertPEM(t, "admin", fixedNow.Add(40*24*time.Hour), nil)
	caCert := makeCertPEM(t, "kubernetes-ca", fixedNow.Add(500*24*time.Hour), nil)
	kubeconfig := "apiVersion: v1\nkind: Config\n" +
		"clusters:\n- name: default\n  cluster:\n    certificate-authority-data: " + base64.StdEncoding.EncodeToString(caCert) + "\n" +
		"users:\n- name: admin\n  user:\n    client-certificate-data: " + base64.StdEncoding.EncodeToString(clientCert) + "\n"
	p := writeFile(t, dir, "admin.conf", []byte(kubeconfig))

	results := Scan(ScanOptions{Paths: []string{p}, Thresholds: Thresholds{WarnDays: 30, CriticalDays: 7}, Now: fixedNow})
	if len(results) != 2 {
		t.Fatalf("want 2 results (client + ca), got %d: %+v", len(results), results)
	}
	var sawClient, sawCA bool
	for _, r := range results {
		switch r.Source {
		case "kubeconfig:client:admin":
			sawClient = true
		case "kubeconfig:ca:default":
			sawCA = true
		}
	}
	if !sawClient || !sawCA {
		t.Errorf("missing client/ca sources: %+v", results)
	}
}

func TestScanMissingPathIsSilent(t *testing.T) {
	results := Scan(ScanOptions{Paths: []string{"/nonexistent/path/here"}, Now: fixedNow})
	if len(results) != 0 {
		t.Fatalf("missing path should yield no results, got %+v", results)
	}
}

func TestScanUnparsableFileIsError(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "garbage.crt", []byte("this is not a PEM certificate"))
	results := Scan(ScanOptions{Paths: []string{p}, Now: fixedNow})
	if len(results) != 1 || results[0].Outcome != OutcomeError {
		t.Fatalf("want 1 Error result, got %+v", results)
	}
}

func TestScanPermissionDeniedIsSkippedNotError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root cannot exercise permission-denied; covered by e2e on the non-root agent")
	}
	dir := t.TempDir()

	// An unreadable certificate file must be Skipped, never Error, so a
	// root-only cert does not dominate a healthy node's aggregate.
	file := writeFile(t, dir, "secret.crt", makeCertPEM(t, "x", fixedNow.Add(100*24*time.Hour), nil))
	if err := os.Chmod(file, 0o000); err != nil {
		t.Fatal(err)
	}
	if got := Scan(ScanOptions{Paths: []string{file}, Now: fixedNow}); len(got) != 1 || got[0].Outcome != OutcomeSkipped {
		t.Fatalf("unreadable file: want 1 Skipped, got %+v", got)
	}

	// An unreadable configured directory root must yield a single Skipped, not Error.
	sub := filepath.Join(dir, "rootonly")
	writeFile(t, sub, "a.crt", makeCertPEM(t, "y", fixedNow.Add(100*24*time.Hour), nil))
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) }) // let TempDir cleanup recurse
	if got := Scan(ScanOptions{Paths: []string{sub}, Now: fixedNow}); len(got) != 1 || got[0].Outcome != OutcomeSkipped {
		t.Fatalf("unreadable dir: want 1 Skipped, got %+v", got)
	}

	// A configured file whose parent directory is not searchable cannot even be
	// stat'd (the os.Stat in scanPath fails with EACCES). That is a permission
	// verdict too and must be Skipped, never Error.
	noexec := filepath.Join(dir, "noexec")
	inner := writeFile(t, noexec, "inner.crt", makeCertPEM(t, "z", fixedNow.Add(100*24*time.Hour), nil))
	if err := os.Chmod(noexec, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(noexec, 0o755) })
	if got := Scan(ScanOptions{Paths: []string{inner}, Now: fixedNow}); len(got) != 1 || got[0].Outcome != OutcomeSkipped {
		t.Fatalf("unstattable file (non-searchable parent): want 1 Skipped, got %+v", got)
	}
}

func TestScanDefaultsWhenNoPaths(t *testing.T) {
	// No paths and (almost certainly) none of the default host paths exist in
	// the test sandbox -> no results, but the call must not panic or error.
	results := Scan(ScanOptions{Now: fixedNow})
	for _, r := range results {
		if r.Outcome == OutcomePass && r.NotAfter.IsZero() {
			t.Errorf("pass result without notAfter: %+v", r)
		}
	}
}

func TestMinimalMountDirs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "collapses descendants under common ancestor",
			in:   []string{"/etc/kubernetes/pki", "/etc/kubernetes/pki/etcd", "/etc/kubernetes/admin.conf"},
			want: []string{"/etc/kubernetes"},
		},
		{
			name: "keeps disjoint dirs",
			in:   []string{"/var/lib/kubelet/pki", "/etc/etcd/pki"},
			want: []string{"/etc/etcd/pki", "/var/lib/kubelet/pki"},
		},
		{
			name: "file under root is dropped (refuses to mount /)",
			in:   []string{"/foo.crt"},
			want: nil,
		},
		{
			name: "relative paths ignored",
			in:   []string{"etc/kubernetes", "/etc/kubernetes/pki"},
			want: []string{"/etc/kubernetes/pki"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MinimalMountDirs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestWorstOutcome(t *testing.T) {
	if WorstOutcome(nil) != OutcomeSkipped {
		t.Errorf("empty should be Skipped")
	}
	results := []CertResult{{Outcome: OutcomePass}, {Outcome: OutcomeWarn}, {Outcome: OutcomeFail}}
	if WorstOutcome(results) != OutcomeFail {
		t.Errorf("want Fail")
	}
	results = append(results, CertResult{Outcome: OutcomeError})
	if WorstOutcome(results) != OutcomeError {
		t.Errorf("Error outranks Fail")
	}
}

func TestNodeReportConfigMapNameDeterministicAndSafe(t *testing.T) {
	a := NodeReportConfigMapName("node-certs", "ip-10-0-1-23.ec2.internal")
	b := NodeReportConfigMapName("node-certs", "ip-10-0-1-23.ec2.internal")
	c := NodeReportConfigMapName("node-certs", "other-node")
	if a != b {
		t.Errorf("not deterministic: %q vs %q", a, b)
	}
	if a == c {
		t.Errorf("different nodes collided: %q", a)
	}
	if len(a) > 253 || a == "" {
		t.Errorf("invalid length: %q", a)
	}
	// Upper-case and unusual chars must be sanitized away.
	weird := NodeReportConfigMapName("Check", "NODE/With*Bad:Chars")
	for _, r := range weird {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.'
		if !valid {
			t.Errorf("name has invalid rune %q in %q", r, weird)
		}
	}
}

func TestEncodeDecodeReportRoundTrip(t *testing.T) {
	in := NodeReport{
		Node:       "node-1",
		CheckName:  "node-certs",
		ObservedAt: fixedNow,
		Aggregate:  OutcomeWarn,
		Certs:      []CertResult{{Path: "/etc/kubernetes/pki/apiserver.crt", Outcome: OutcomeWarn, DaysRemaining: 12, NotAfter: fixedNow}},
	}
	encoded, err := EncodeReport(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := DecodeReport(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Node != in.Node || out.Aggregate != in.Aggregate || len(out.Certs) != 1 || out.Certs[0].DaysRemaining != 12 {
		t.Errorf("round trip mismatch: %+v", out)
	}
}
