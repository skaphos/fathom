/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// Fake token-endpoint behaviours for packages not answering a plain 401.
const (
	ghcrPublic    = "public"    // 200 with a token, as GHCR answers a public package
	ghcrNoToken   = "notoken"   // 200 whose body carries no token
	ghcrTruncated = "truncated" // 200 headers, then the connection drops mid-body
)

// fakeGHCRTokenEndpoint stands in for https://ghcr.io/token. Each package in
// behaviours answers as described above; everything else answers 401, as GHCR
// does for a private package. It also fails the test if a request carries
// credentials, because the guard is only meaningful as an anonymous check.
func fakeGHCRTokenEndpoint(t *testing.T, behaviours map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("token request carried credentials; the check must be anonymous")
		}
		scope := r.URL.Query().Get("scope")
		pkg := strings.TrimSuffix(strings.TrimPrefix(scope, "repository:skaphos/"), ":pull")
		switch behaviours[pkg] {
		case ghcrPublic:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"token":"anonymous-pull-token"}`)
		case ghcrNoToken:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{}`)
		case ghcrTruncated:
			// Promise a longer body than is sent, then drop the connection:
			// curl sees status 200 but exits non-zero (partial transfer).
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"tok`)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			panic(http.ErrAbortHandler)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
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
	srv, _ := fakeGHCRTokenEndpoint(t, map[string]string{
		"fathom-operator":        ghcrPublic,
		"charts/fathom-operator": ghcrPublic,
	})
	ok, out := runGHCRPublicCheck(t, srv.URL, "fathom-operator", "charts/fathom-operator")
	if !ok {
		t.Fatalf("check failed for public packages:\n%s", out)
	}
}

// TestGHCRPublicCheckFailsForPrivatePackage is the regression guard for
// skaphos/fathom#333: one private package among public ones must fail the
// run, name the package, and print the remediation, after retrying.
func TestGHCRPublicCheckFailsForPrivatePackage(t *testing.T) {
	srv, hits := fakeGHCRTokenEndpoint(t, map[string]string{"fathom-operator": ghcrPublic})
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
	srv, _ := fakeGHCRTokenEndpoint(t, nil)
	if ok, out := runGHCRPublicCheck(t, srv.URL); ok {
		t.Fatalf("check passed with no packages:\n%s", out)
	}
}

// TestGHCRPublicCheckRejectsIncompleteSuccess: a 200 status alone is not proof
// of anonymous access. A transfer that drops mid-body (curl exits non-zero
// after seeing 200) or a 200 without a token must fail, after retrying.
func TestGHCRPublicCheckRejectsIncompleteSuccess(t *testing.T) {
	for _, behaviour := range []string{ghcrTruncated, ghcrNoToken} {
		t.Run(behaviour, func(t *testing.T) {
			srv, hits := fakeGHCRTokenEndpoint(t, map[string]string{"fathom-operator": behaviour})
			ok, out := runGHCRPublicCheck(t, srv.URL, "fathom-operator")
			if ok {
				t.Fatalf("check passed on a %s 200 response:\n%s", behaviour, out)
			}
			if !strings.Contains(out, "not public: fathom-operator") {
				t.Errorf("output does not name the failing package:\n%s", out)
			}
			if got := hits.Load(); got != 2 {
				t.Errorf("token endpoint hits = %d, want 2 (failure must be retried)", got)
			}
		})
	}
}
