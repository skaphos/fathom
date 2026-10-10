/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeGHCRTokenEndpoint stands in for https://ghcr.io/token. Packages listed in
// public answer 200; everything else answers 401, as GHCR does for a private
// package. It also fails the test if a request carries credentials, because
// the guard is only meaningful as an anonymous check.
func fakeGHCRTokenEndpoint(t *testing.T, public ...string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("token request carried credentials; the check must be anonymous")
		}
		scope := r.URL.Query().Get("scope")
		for _, pkg := range public {
			if scope == "repository:skaphos/"+pkg+":pull" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func runGHCRPublicCheck(t *testing.T, endpoint string, pkgs ...string) (bool, string) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"check-ghcr-public.sh"}, pkgs...)...)
	cmd.Env = append(os.Environ(),
		"GHCR_TOKEN_ENDPOINT="+endpoint,
		"GHCR_CHECK_ATTEMPTS=2",
		"GHCR_CHECK_DELAY=0",
	)
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}

// TestGHCRPublicCheckPassesForPublicPackages: every package answering 200,
// including a nested chart path, passes.
func TestGHCRPublicCheckPassesForPublicPackages(t *testing.T) {
	srv, _ := fakeGHCRTokenEndpoint(t, "fathom-operator", "charts/fathom-operator")
	ok, out := runGHCRPublicCheck(t, srv.URL, "fathom-operator", "charts/fathom-operator")
	if !ok {
		t.Fatalf("check failed for public packages:\n%s", out)
	}
}

// TestGHCRPublicCheckFailsForPrivatePackage is the regression guard for
// skaphos/fathom#333: one private package among public ones must fail the
// run, name the package, and print the remediation, after retrying.
func TestGHCRPublicCheckFailsForPrivatePackage(t *testing.T) {
	srv, hits := fakeGHCRTokenEndpoint(t, "fathom-operator")
	ok, out := runGHCRPublicCheck(t, srv.URL, "fathom-operator", "fathom-probe")
	if ok {
		t.Fatalf("check passed although fathom-probe is private:\n%s", out)
	}
	for _, want := range []string{"ghcr.io/skaphos/fathom-probe", "Change visibility -> Public", "not public: fathom-probe"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// One request for the public package, GHCR_CHECK_ATTEMPTS=2 for the private one.
	if got := hits.Load(); got != 3 {
		t.Errorf("token endpoint hits = %d, want 3 (private package must be retried)", got)
	}
}

// TestGHCRPublicCheckRequiresPackages: no arguments is a usage error, not a
// vacuous pass.
func TestGHCRPublicCheckRequiresPackages(t *testing.T) {
	srv, _ := fakeGHCRTokenEndpoint(t)
	if ok, out := runGHCRPublicCheck(t, srv.URL); ok {
		t.Fatalf("check passed with no packages:\n%s", out)
	}
}
