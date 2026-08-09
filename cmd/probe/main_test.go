/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestJoin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"nil", nil, ""},
		{"empty", []string{}, ""},
		{"single", []string{"a"}, "a"},
		{"two", []string{"a", "b"}, "a,b"},
		{"ipv4 and ipv6", []string{"1.2.3.4", "::1"}, "1.2.3.4,::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := join(tc.in); got != tc.want {
				t.Errorf("join(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWriteResultDoesNotPanic(t *testing.T) {
	// Smoke: exercises the encode + os.Stdout write + termination-log write
	// paths. The terminationLog file path doesn't exist outside a probe pod
	// so the os.WriteFile call silently fails — that's the production
	// fallthrough behavior we want to exercise here too.
	writeResult(result{Outcome: "Pass", Summary: "ok", Details: map[string]string{"k": "v"}})
	writeResult(result{Outcome: "Error", Summary: "no details"})
}

func TestRunDNSRequiresTarget(t *testing.T) {
	if err := runDNS(context.Background(), dnsQuery{}); err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
}

func TestRunDNSResolvesLocalhost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// localhost is guaranteed to resolve via /etc/hosts on every sane
	// platform; it's the cheapest way to exercise the success path.
	if err := runDNS(ctx, dnsQuery{Target: "localhost"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDNSFailsForInvalidName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// RFC 6761 reserves the `.invalid` TLD for guaranteed-not-resolvable
	// names, which makes this deterministic without relying on a specific
	// resolver behavior. An unresolvable name is the exact condition the check
	// exists to catch: it must surface as Outcome=Fail (not Error) and return
	// nil, mirroring runTCPConnect on a refused dial. Error outranks Fail on the
	// severity ladder, so a DNS outage reported as Error would mask real Fails
	// in the ClusterHealth rollup. Regression guard for #158.
	got := captureResult(t, func() {
		if err := runDNS(ctx, dnsQuery{Target: "does-not-exist.invalid"}); err != nil {
			t.Fatalf("expected nil error for unresolvable name, got %v", err)
		}
	})
	if got.Outcome != "Fail" {
		t.Fatalf("Outcome = %q, want Fail", got.Outcome)
	}
}

// TestRunDNSDefaultPathIsHostLookup pins the FR-030 guarantee: a dns probe
// that declares no record kind performs a plain host lookup, answering on
// either address family, exactly as it did before record kinds existed. The
// nodelocaldns adapter depends on this — it passes no record type — so
// narrowing the default to IPv4 would silently change a live check.
//
// The assertion compares against net.DefaultResolver.LookupHost rather than
// against a hardcoded address, so it holds on hosts with and without IPv6
// instead of encoding one environment's answer.
func TestRunDNSDefaultPathIsHostLookup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	want, err := net.DefaultResolver.LookupHost(ctx, "localhost")
	if err != nil {
		t.Fatalf("baseline LookupHost: %v", err)
	}
	got := captureResult(t, func() {
		if err := runDNS(ctx, dnsQuery{Target: "localhost"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if got.Outcome != "Pass" {
		t.Fatalf("Outcome = %q, want Pass", got.Outcome)
	}
	// The detail key moved from "addresses" to "answers" when record kinds
	// arrived, so one key serves every kind rather than one naming addresses
	// and another naming SRV or CNAME answers. Nothing reads it — it is
	// evidence text rendered into HealthReport details — and FR-030 protects
	// outcomes, not evidence key names. The answer *set* is what must not move,
	// and that is what this asserts.
	if got.Details["answers"] != join(want) {
		t.Fatalf("answers = %q, want %q (default path must remain a host lookup)", got.Details["answers"], join(want))
	}
}

// TestRunDNSRecordKinds walks the record kinds whose answers are deterministic
// from /etc/hosts, so the suite stays hermetic rather than depending on any
// zone being reachable.
func TestRunDNSRecordKinds(t *testing.T) {
	tests := []struct {
		name        string
		query       dnsQuery
		wantOutcome string
		wantAnswers string
	}{
		{"host defaults to either family", dnsQuery{Target: "localhost"}, "Pass", "127.0.0.1"},
		{"A narrows to ipv4", dnsQuery{Target: "localhost", RecordType: recordA}, "Pass", "127.0.0.1"},
		{"PTR resolves an address to a name", dnsQuery{Target: "127.0.0.1", RecordType: recordPTR}, "Pass", "localhost"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			got := captureResult(t, func() {
				if err := runDNS(ctx, tc.query); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			})
			if got.Outcome != tc.wantOutcome {
				t.Fatalf("Outcome = %q, want %q", got.Outcome, tc.wantOutcome)
			}
			if got.Details["answers"] != tc.wantAnswers {
				t.Fatalf("answers = %q, want %q", got.Details["answers"], tc.wantAnswers)
			}
			if got.Details["recordType"] == "" {
				t.Fatal("recordType detail must always be recorded, so a result is self-describing")
			}
		})
	}
}

// TestRunDNSCNAMEWithNoRecordDoesNotPass guards the first of the two stdlib
// traps in research R1. LookupCNAME does NOT fail when the subject has no
// CNAME record: it returns the subject itself, fully qualified, with a nil
// error. A naive implementation therefore reports Pass for every CNAME check
// ever written — a check that cannot fail, which is worse than no check
// because it reads as coverage.
//
// localhost has no CNAME record, so this is deterministic from /etc/hosts.
func TestRunDNSCNAMEWithNoRecordDoesNotPass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Confirm the trap is real in this environment before asserting we avoid it.
	cname, err := net.DefaultResolver.LookupCNAME(ctx, "localhost")
	if err != nil || cname == "" {
		t.Skipf("resolver does not exhibit the LookupCNAME no-record behaviour here (cname=%q err=%v)", cname, err)
	}

	got := captureResult(t, func() {
		if err := runDNS(ctx, dnsQuery{Target: "localhost", RecordType: recordCNAME}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if got.Outcome != "Fail" {
		t.Fatalf("Outcome = %q, want Fail — a subject with no CNAME record must not pass a CNAME check", got.Outcome)
	}
}

// TestLookupSRVQueriesTheSubjectVerbatim guards the second stdlib trap.
// LookupSRV builds "_service._proto.name" when given a non-empty service and
// proto; our subjects already carry those labels, so passing them through
// would query a mangled name and fail for reasons unrelated to health.
//
// net.DNSError.Name carries the name actually queried, which lets us prove the
// subject reached the resolver unrewritten without needing a zone that has SRV
// records.
func TestLookupSRVQueriesTheSubjectVerbatim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	const subject = "_https._tcp.does-not-exist.invalid"

	_, err := lookupSRV(ctx, subject)
	if err == nil {
		t.Fatal("expected a lookup error for a reserved .invalid subject")
	}
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		t.Skipf("resolver returned a non-DNSError (%v); cannot inspect the queried name", err)
	}
	if dnsErr.Name != subject {
		t.Fatalf("queried name = %q, want %q — LookupSRV must be called with empty service and proto", dnsErr.Name, subject)
	}
}

// TestRunDNSPolarity covers the negative-assertion column of the outcome
// matrix, including the FR-014 case that the whole requirement exists for.
func TestRunDNSPolarity(t *testing.T) {
	t.Run("absent subject satisfies a negative assertion", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		got := captureResult(t, func() {
			if err := runDNS(ctx, dnsQuery{Target: "does-not-exist.invalid", Absent: true}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if got.Outcome != "Pass" {
			t.Fatalf("Outcome = %q, want Pass", got.Outcome)
		}
	})

	t.Run("resolving subject fails a negative assertion", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		got := captureResult(t, func() {
			if err := runDNS(ctx, dnsQuery{Target: "localhost", Absent: true}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if got.Outcome != "Fail" {
			t.Fatalf("Outcome = %q, want Fail", got.Outcome)
		}
	})

	// FR-014. An unreachable resolver is not evidence that a name is gone.
	// Reporting Pass here would turn a network fault into false proof that a
	// decommissioned hostname had been retired — the single most consequential
	// way to get negative assertions wrong.
	//
	// A cancelled context guarantees the lookup cannot reach an answer, so
	// IsNotFound is necessarily false regardless of what the resolver would
	// have said.
	t.Run("unreachable resolver never proves absence", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got := captureResult(t, func() {
			if err := runDNS(ctx, dnsQuery{Target: "example.com", Absent: true}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if got.Outcome == "Pass" {
			t.Fatal("a resolver that never answered must never satisfy an absent assertion")
		}
		if got.Outcome != "Error" {
			t.Fatalf("Outcome = %q, want Error", got.Outcome)
		}
	})

	t.Run("absent with expected answers is contradictory", func(t *testing.T) {
		got := captureResult(t, func() {
			if err := runDNS(context.Background(), dnsQuery{Target: "localhost", Absent: true, Expect: []string{"127.0.0.1"}}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if got.Outcome != "Error" {
			t.Fatalf("Outcome = %q, want Error", got.Outcome)
		}
	})
}

func TestRunDNSExpectedAnswers(t *testing.T) {
	t.Run("declared answer present passes", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		got := captureResult(t, func() {
			if err := runDNS(ctx, dnsQuery{Target: "localhost", RecordType: recordA, Expect: []string{"127.0.0.1"}}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if got.Outcome != "Pass" {
			t.Fatalf("Outcome = %q, want Pass", got.Outcome)
		}
	})

	t.Run("declared answer absent fails and names what was missing", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		got := captureResult(t, func() {
			if err := runDNS(ctx, dnsQuery{Target: "localhost", RecordType: recordA, Expect: []string{"10.99.99.99"}}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		if got.Outcome != "Fail" {
			t.Fatalf("Outcome = %q, want Fail", got.Outcome)
		}
		if got.Details["missingAnswers"] != "10.99.99.99" {
			t.Fatalf("missingAnswers = %q, want the declared answer named", got.Details["missingAnswers"])
		}
	})
}

func TestRunDNSRejectsUnknownRecordKind(t *testing.T) {
	got := captureResult(t, func() {
		// An unknown kind is not the resolver's answer, so it is an Error
		// rather than a Fail — it says nothing about the target's health.
		_ = runDNS(context.Background(), dnsQuery{Target: "localhost", RecordType: "TXT"})
	})
	if got.Outcome != "Error" {
		t.Fatalf("Outcome = %q, want Error", got.Outcome)
	}
}

func TestMissingAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected []string
		got      []string
		want     []string
	}{
		{"no expectation is always satisfied", nil, []string{"1.2.3.4"}, nil},
		{"exact match", []string{"1.2.3.4"}, []string{"1.2.3.4"}, nil},
		// Containment, not equality: round-robin and multi-address records
		// legitimately return supersets of anything an operator writes down.
		{"superset satisfies", []string{"1.2.3.4"}, []string{"1.2.3.4", "5.6.7.8"}, nil},
		{"missing one is reported", []string{"1.2.3.4", "9.9.9.9"}, []string{"1.2.3.4"}, []string{"9.9.9.9"}},
		{"trailing dot folds", []string{"host.example.com"}, []string{"host.example.com."}, nil},
		{"case folds", []string{"HOST.example.com"}, []string{"host.example.com"}, nil},
		{"ipv6 spelling folds", []string{"2001:db8:0:0:0:0:0:1"}, []string{"2001:db8::1"}, nil},
		{"nothing returned means everything missing", []string{"1.2.3.4"}, nil, []string{"1.2.3.4"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := missingAnswers(tc.expected, tc.got)
			if join(got) != join(tc.want) {
				t.Errorf("missingAnswers(%v, %v) = %v, want %v", tc.expected, tc.got, got, tc.want)
			}
		})
	}
}

func TestSameDNSName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want bool
	}{
		{"localhost", "localhost", true},
		{"localhost.", "localhost", true},
		{"LOCALHOST", "localhost", true},
		{"alias.example.com.", "origin.example.com.", false},
	}
	for _, tc := range tests {
		if got := sameDNSName(tc.a, tc.b); got != tc.want {
			t.Errorf("sameDNSName(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestRunTCPConnectRequiresTarget(t *testing.T) {
	if err := runTCPConnect(context.Background(), "", 80); err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
}

func TestRunTCPConnectRequiresPort(t *testing.T) {
	if err := runTCPConnect(context.Background(), "localhost", 0); err == nil {
		t.Fatal("expected error for zero port, got nil")
	}
}

func TestRunTCPConnectSucceeds(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	host, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runTCPConnect(ctx, host, port); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTCPConnectReturnsNilOnDialFailure(t *testing.T) {
	// runTCPConnect surfaces a dial failure as Outcome=Fail in the JSON
	// payload and returns nil — only argument-validation errors flow up.
	port := claimAndReleasePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := runTCPConnect(ctx, "127.0.0.1", port); err != nil {
		t.Fatalf("expected nil on dial failure, got %v", err)
	}
}

func TestRunTCPListenRequiresPort(t *testing.T) {
	if err := runTCPListen(context.Background(), "127.0.0.1", 0); err == nil {
		t.Fatal("expected error for zero port, got nil")
	}
}

func TestRunTCPListenCompletesOnContextCancel(t *testing.T) {
	port := claimAndReleasePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := runTCPListen(ctx, "127.0.0.1", port); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTCPListenFailsOnBindCollision(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	// Same port is still bound by us; runTCPListen must fail to bind.
	if err := runTCPListen(context.Background(), "127.0.0.1", port); err == nil {
		t.Fatal("expected error for port collision, got nil")
	}
}

func TestRunRejectsUnsupportedMode(t *testing.T) {
	withFlagReset(t, []string{"probe", "-mode=bogus", "-timeout=50ms"})
	if err := run(); err == nil {
		t.Fatal("expected error for unsupported mode, got nil")
	}
}

func TestRunDispatchesToDNS(t *testing.T) {
	withFlagReset(t, []string{"probe", "-mode=dns", "-target=localhost", "-timeout=2s"})
	if err := run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunHTTPGetRequiresTarget(t *testing.T) {
	if err := runHTTPGet(context.Background(), "", ""); err == nil {
		t.Fatal("expected error for empty target, got nil")
	}
}

func TestRunHTTPGetRejectsInvalidURL(t *testing.T) {
	// One result per probe run: runHTTPGet must emit the Error result itself
	// and return nil, or main() would write a second result over the
	// termination log and drop the Details map.
	got := captureResult(t, func() {
		if err := runHTTPGet(context.Background(), "not a url", ""); err != nil {
			t.Errorf("runHTTPGet must return nil after emitting a result, got %v", err)
		}
	})
	if got.Outcome != "Error" {
		t.Fatalf("outcome: got %q, want Error", got.Outcome)
	}
}

func TestRunHTTPGetPassesWithExpectedFamilies(t *testing.T) {
	body := "# HELP kube_node_info Information about a cluster node.\n" +
		"# TYPE kube_node_info gauge\n" +
		"kube_node_info{node=\"a\"} 1\n" +
		"kube_pod_info{pod=\"p\",namespace=\"default\"} 1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	got := captureResult(t, func() {
		if err := runHTTPGet(context.Background(), srv.URL, "kube_node_info,kube_pod_info"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if got.Outcome != "Pass" {
		t.Fatalf("outcome: got %q (summary %q), want Pass", got.Outcome, got.Summary)
	}
	if got.Details["statusCode"] != "200" {
		t.Errorf("statusCode detail: got %q", got.Details["statusCode"])
	}
	if got.Details["sampleLines"] != "2" {
		t.Errorf("sampleLines detail: got %q, want 2", got.Details["sampleLines"])
	}
}

func TestRunHTTPGetFailsOnMissingFamily(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "kube_node_info{node=\"a\"} 1\n")
	}))
	defer srv.Close()

	got := captureResult(t, func() {
		if err := runHTTPGet(context.Background(), srv.URL, "kube_node_info,kube_pod_info"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if got.Outcome != "Fail" {
		t.Fatalf("outcome: got %q, want Fail", got.Outcome)
	}
	if got.Details["missingFamilies"] != "kube_pod_info" {
		t.Errorf("missingFamilies detail: got %q, want kube_pod_info", got.Details["missingFamilies"])
	}
}

func TestRunHTTPGetFailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	got := captureResult(t, func() {
		if err := runHTTPGet(context.Background(), srv.URL, ""); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if got.Outcome != "Fail" {
		t.Fatalf("outcome: got %q, want Fail", got.Outcome)
	}
	if got.Details["statusCode"] != "500" {
		t.Errorf("statusCode detail: got %q, want 500", got.Details["statusCode"])
	}
}

func TestRunHTTPGetFailsOnUnreachableEndpoint(t *testing.T) {
	// A refused dial is the outage the check reports on: Fail, not Error —
	// mirroring runDNS/runTCPConnect so a scrape outage cannot mask genuine
	// Fails elsewhere in the rollup (cf. #158 for DNS).
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := captureResult(t, func() {
		if err := runHTTPGet(ctx, url, ""); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if got.Outcome != "Fail" {
		t.Fatalf("outcome: got %q, want Fail", got.Outcome)
	}
}

func TestRunHTTPGetFailsOnEmptyBody(t *testing.T) {
	// 200 with no samples means "scrapeable but blind" — exactly the silent
	// failure mode a metrics_endpoint check exists to catch.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	got := captureResult(t, func() {
		if err := runHTTPGet(context.Background(), srv.URL, ""); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if got.Outcome != "Fail" {
		t.Fatalf("outcome: got %q, want Fail", got.Outcome)
	}
	if !strings.Contains(got.Summary, "no metric samples") {
		t.Errorf("summary: got %q", got.Summary)
	}
}

func TestScanMetricFamilies(t *testing.T) {
	t.Parallel()

	body := "# HELP a_metric help\n# TYPE a_metric counter\na_metric 1\nb_metric{l=\"v\"} 2\n\n# odd comment\n"
	tests := []struct {
		name        string
		expected    []string
		wantMissing string
		wantSamples int
	}{
		{"no expectations", nil, "", 2},
		{"all present", []string{"a_metric", "b_metric"}, "", 2},
		{"header-only match", []string{"a_metric"}, "", 2},
		{"one missing", []string{"a_metric", "c_metric"}, "c_metric", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			missing, samples, truncated, err := scanMetricFamilies(strings.NewReader(body), tc.expected)
			if err != nil {
				t.Fatalf("scanMetricFamilies: %v", err)
			}
			if truncated {
				t.Fatal("unexpected truncation")
			}
			if got := join(missing); got != tc.wantMissing {
				t.Errorf("missing: got %q, want %q", got, tc.wantMissing)
			}
			if samples != tc.wantSamples {
				t.Errorf("samples: got %d, want %d", samples, tc.wantSamples)
			}
		})
	}
}

func TestSplitComma(t *testing.T) {
	t.Parallel()

	if got := splitComma(""); got != nil {
		t.Errorf("splitComma(\"\") = %v, want nil", got)
	}
	if got := join(splitComma(" a, b ,,c ")); got != "a,b,c" {
		t.Errorf("splitComma trim: got %q, want a,b,c", got)
	}
}

// captureResult redirects os.Stdout for the duration of fn, then decodes the
// single JSON probe result that writeResult emits. It lets tests assert on the
// Outcome field without exporting a seam from the probe binary.
func captureResult(t *testing.T, fn func()) result {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	// Restore stdout and close both pipe ends via defer so an fn() that calls
	// t.Fatal (runtime.Goexit) can't leak the fd or leave os.Stdout dangling for
	// the rest of the package's tests. Deferred funcs still run under Goexit.
	defer func() {
		os.Stdout = oldStdout
		_ = w.Close()
		_ = r.Close()
	}()
	os.Stdout = w
	fn()
	// Close the writer before reading so io.ReadAll observes EOF; the deferred
	// close is then a harmless no-op.
	_ = w.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	var got result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode probe result %q: %v", data, err)
	}
	return got
}

// claimAndReleasePort binds to a kernel-assigned port on 127.0.0.1, closes
// the listener, and returns the port. There's a small race window where the
// port can be re-grabbed before the caller uses it; we accept that for
// simplicity rather than refactor runTCPListen to expose a net.Listen seam.
func claimAndReleasePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("Atoi: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close: %v", err)
	}
	return port
}

// withFlagReset swaps in a fresh flag.CommandLine and os.Args so that run()
// can be invoked multiple times across tests without flag-redefinition
// panics from flag.String. The originals are restored via t.Cleanup.
func withFlagReset(t *testing.T, args []string) {
	t.Helper()
	oldArgs := os.Args
	oldFlags := flag.CommandLine
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldFlags
	})
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = args
}
