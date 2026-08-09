/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// probe is the tiny in-cluster network probe binary used by Fathom probe pods.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const terminationLog = "/dev/termination-log"

type result struct {
	Outcome string            `json:"outcome"`
	Summary string            `json:"summary"`
	Details map[string]string `json:"details,omitempty"`
}

func main() {
	if err := run(); err != nil {
		writeResult(result{Outcome: "Error", Summary: err.Error()})
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "", "probe mode: dns, tcp-connect, tcp-listen, http-get")
	target := flag.String("target", "", "DNS name, TCP host, or http-get URL")
	port := flag.Int("port", 0, "TCP port")
	timeout := flag.Duration("timeout", 10*time.Second, "probe timeout")
	listenAddress := flag.String("listen-address", "0.0.0.0", "address for tcp-listen")
	expect := flag.String("expect", "", "comma-separated Prometheus metric family names http-get requires in the body")
	recordType := flag.String("record-type", "", "dns record kind: Host (default), A, AAAA, CNAME, SRV, PTR")
	expectAnswers := flag.String("expect-answers", "", "comma-separated answers a dns target must all return")
	absent := flag.Bool("absent", false, "assert the dns target must NOT resolve")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch *mode {
	case "dns":
		return runDNS(ctx, dnsQuery{
			Target:     *target,
			RecordType: *recordType,
			Expect:     splitComma(*expectAnswers),
			Absent:     *absent,
		})
	case "tcp-connect":
		return runTCPConnect(ctx, *target, *port)
	case "tcp-listen":
		return runTCPListen(ctx, *listenAddress, *port)
	case "http-get":
		return runHTTPGet(ctx, *target, *expect)
	default:
		return fmt.Errorf("unsupported probe mode %q", *mode)
	}
}

// DNS record kinds the probe can evaluate. Host is the default and is a plain
// host lookup answering on either address family; A and AAAA narrow to one
// family and are reachable only by naming them. Keeping Host as the default is
// what leaves existing callers — which pass no record kind — untouched.
const (
	recordHost  = "Host"
	recordA     = "A"
	recordAAAA  = "AAAA"
	recordCNAME = "CNAME"
	recordSRV   = "SRV"
	recordPTR   = "PTR"
)

// dnsQuery is one DNS assertion: what to look up, how, and what would count as
// success. The zero value is the pre-existing behavior — a host lookup that
// passes on any non-empty answer.
type dnsQuery struct {
	Target     string
	RecordType string
	Expect     []string
	Absent     bool
}

// runDNS evaluates one DNS assertion and writes a single result.
//
// Outcome classification follows two rules that are easy to get backwards.
// First, under a positive assertion a resolution failure is a Fail, not an
// Error: it is precisely the condition the check exists to detect, and Error
// outranks Fail on the severity ladder (Pass<Skipped<Warn<Unknown<Fail<Error),
// so reporting a real DNS outage as Error would mask genuine Fails elsewhere
// in the ClusterHealth rollup.
//
// Second, under a negative assertion the two failure modes stop being
// equivalent. "The resolver says this name does not exist" proves the
// assertion; "I could not reach the resolver" proves nothing at all. Reporting
// the latter as Pass would turn a network fault into false evidence that a
// decommissioned name had been retired, so it is an Error.
func runDNS(ctx context.Context, q dnsQuery) error {
	if q.Target == "" {
		return errors.New("dns probe target is required")
	}
	recordType := q.RecordType
	if recordType == "" {
		recordType = recordHost
	}
	details := map[string]string{"target": q.Target, "recordType": recordType}
	if q.Absent && len(q.Expect) > 0 {
		// Admission rejects this pairing, but the probe must not assume it only
		// ever sees valid input. Silently honouring one side would answer a
		// question nobody asked.
		writeResult(result{Outcome: "Error", Summary: "an absent assertion cannot declare expected answers", Details: details})
		return nil
	}

	started := time.Now()
	answers, err := resolveRecord(ctx, recordType, q.Target)
	details["latencyMillis"] = strconv.FormatInt(time.Since(started).Milliseconds(), 10)

	if err != nil {
		details["error"] = err.Error()
		var dnsErr *net.DNSError
		if !errors.As(err, &dnsErr) {
			// Not the resolver's answer at all — an unsupported record kind, a
			// malformed subject, a probe-infrastructure fault.
			writeResult(result{Outcome: "Error", Summary: "DNS resolution failed", Details: details})
			return err
		}
		if dnsErr.IsNotFound {
			return writeAbsence(q.Absent, details, "the resolver reports the name does not exist")
		}
		// Timeout, temporary failure, SERVFAIL: the resolver did not answer.
		if q.Absent {
			writeResult(result{Outcome: "Error", Summary: "resolver did not answer; absence cannot be proven", Details: details})
			return nil
		}
		writeResult(result{Outcome: "Fail", Summary: "DNS resolution failed", Details: details})
		return nil
	}

	if len(answers) == 0 {
		return writeAbsence(q.Absent, details, "the resolver returned no answers")
	}
	details["answers"] = join(answers)
	if q.Absent {
		writeResult(result{Outcome: "Fail", Summary: "name resolves but was asserted absent", Details: details})
		return nil
	}
	if missing := missingAnswers(q.Expect, answers); len(missing) > 0 {
		details["missingAnswers"] = join(missing)
		writeResult(result{Outcome: "Fail", Summary: "expected answers are missing", Details: details})
		return nil
	}
	writeResult(result{Outcome: "Pass", Summary: "DNS resolution succeeded", Details: details})
	return nil
}

// writeAbsence records the outcome for a subject that did not resolve, which
// satisfies a negative assertion and fails a positive one.
func writeAbsence(absent bool, details map[string]string, reason string) error {
	if absent {
		writeResult(result{Outcome: "Pass", Summary: "name does not resolve, as asserted", Details: details})
		return nil
	}
	writeResult(result{Outcome: "Fail", Summary: "DNS resolution failed: " + reason, Details: details})
	return nil
}

// resolveRecord issues the query for one record kind and returns its answers
// as text. Errors are returned unwrapped so the caller can classify them from
// the *net.DNSError they carry.
func resolveRecord(ctx context.Context, recordType, target string) ([]string, error) {
	switch recordType {
	case recordHost:
		return net.DefaultResolver.LookupHost(ctx, target)
	case recordA:
		return lookupIPs(ctx, "ip4", target)
	case recordAAAA:
		return lookupIPs(ctx, "ip6", target)
	case recordCNAME:
		return lookupCNAME(ctx, target)
	case recordSRV:
		return lookupSRV(ctx, target)
	case recordPTR:
		return net.DefaultResolver.LookupAddr(ctx, target)
	default:
		return nil, fmt.Errorf("unsupported dns record type %q", recordType)
	}
}

func lookupIPs(ctx context.Context, network, target string) ([]string, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, network, target)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out, nil
}

// lookupCNAME returns the canonical name for target, or no answers when target
// has no CNAME record.
//
// The trap: LookupCNAME does not fail in the no-record case. It follows the
// chain and returns the queried name itself, fully qualified, with a nil
// error. Treating that as an answer would make every CNAME check pass
// unconditionally — a check that can only ever succeed is worse than no check,
// because it reads as coverage.
func lookupCNAME(ctx context.Context, target string) ([]string, error) {
	cname, err := net.DefaultResolver.LookupCNAME(ctx, target)
	if err != nil {
		return nil, err
	}
	if cname == "" || sameDNSName(cname, target) {
		return nil, nil
	}
	return []string{cname}, nil
}

// lookupSRV looks up target exactly as written.
//
// The trap: LookupSRV has two modes. Given a non-empty service and proto it
// builds "_service._proto.name" itself. Our subjects already carry their own
// _service._proto labels, so passing them through those parameters would query
// a mangled name and fail for reasons unrelated to the target's health. The
// empty-service form queries the name verbatim.
func lookupSRV(ctx context.Context, target string) ([]string, error) {
	_, records, err := net.DefaultResolver.LookupSRV(ctx, "", "", target)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, net.JoinHostPort(strings.TrimSuffix(record.Target, "."), strconv.Itoa(int(record.Port))))
	}
	return out, nil
}

// missingAnswers returns the expected answers absent from got, in declared
// order. Matching is containment, not equality: extra answers never fail a
// check, because multi-address and round-robin records legitimately return
// supersets of any set an operator would write down.
func missingAnswers(expected, got []string) []string {
	if len(expected) == 0 {
		return nil
	}
	have := make(map[string]bool, len(got))
	for _, answer := range got {
		have[normalizeAnswer(answer)] = true
	}
	var missing []string
	for _, want := range expected {
		if !have[normalizeAnswer(want)] {
			missing = append(missing, want)
		}
	}
	return missing
}

// normalizeAnswer folds the spellings that mean the same answer: a trailing
// dot on a fully qualified name, ASCII case in a hostname, and the textual
// variants of an IP address such as IPv6 compression. Comparing raw strings
// would fail a check whose declared answer differs from the resolver's only in
// spelling, which reads as an outage rather than as the typo it is.
func normalizeAnswer(value string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(value), ".")
	if addr, err := netip.ParseAddr(trimmed); err == nil {
		return addr.String()
	}
	return strings.ToLower(trimmed)
}

func sameDNSName(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "."), strings.TrimSuffix(b, "."))
}

func runTCPConnect(ctx context.Context, target string, port int) error {
	if target == "" {
		return errors.New("tcp-connect probe target is required")
	}
	if port <= 0 {
		return errors.New("tcp-connect probe port is required")
	}
	started := time.Now()
	dialer := net.Dialer{}
	address := net.JoinHostPort(target, strconv.Itoa(port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	latency := time.Since(started)
	details := map[string]string{"target": target, "port": strconv.Itoa(port), "latencyMillis": strconv.FormatInt(latency.Milliseconds(), 10)}
	if err != nil {
		details["error"] = err.Error()
		writeResult(result{Outcome: "Fail", Summary: "TCP connection failed", Details: details})
		return nil
	}
	_ = conn.Close()
	writeResult(result{Outcome: "Pass", Summary: "TCP connection succeeded", Details: details})
	return nil
}

func runTCPListen(ctx context.Context, listenAddress string, port int) error {
	if port <= 0 {
		return errors.New("tcp-listen probe port is required")
	}
	address := net.JoinHostPort(listenAddress, strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		writeResult(result{Outcome: "Error", Summary: "TCP listener failed", Details: map[string]string{"address": address, "error": err.Error()}})
		return err
	}
	defer func() { _ = listener.Close() }()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				writeResult(result{Outcome: "Pass", Summary: "TCP listener completed", Details: map[string]string{"address": address}})
				return nil
			}
			writeResult(result{Outcome: "Error", Summary: "TCP listener failed", Details: map[string]string{"address": address, "error": err.Error()}})
			return err
		}
		_ = conn.Close()
	}
}

// maxHTTPBodyBytes bounds how much of an http-get response body is scanned.
// Metric bodies are streamed line-by-line (never buffered whole), so this cap
// only guards against an endless or absurdly large response wedging the probe
// until its deadline. 64MiB comfortably covers kube-state-metrics on large
// clusters.
const maxHTTPBodyBytes = 64 << 20

// runHTTPGet fetches target and scans the response as a Prometheus
// text-exposition document. A network failure, a non-200 status, a body with
// no metric samples, or a missing expected metric family is precisely the
// condition a metrics_endpoint check exists to detect, so all of those are
// Fail — mirroring runDNS/runTCPConnect. Error is reserved for
// probe-infrastructure faults (an unparseable target URL, a body read error).
func runHTTPGet(ctx context.Context, target, expect string) error {
	if target == "" {
		return errors.New("http-get probe target is required")
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// One result per probe run: main() writes its own Error result when we
		// return an error, so emit-and-return-nil keeps the termination-log
		// contract (a single JSON document with Details intact).
		writeResult(result{Outcome: "Error", Summary: "http-get target is not a valid URL", Details: map[string]string{"target": target}})
		return nil
	}

	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		writeResult(result{Outcome: "Error", Summary: "failed to build HTTP request", Details: map[string]string{"target": target, "error": err.Error()}})
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	latency := time.Since(started)
	details := map[string]string{"target": target, "latencyMillis": strconv.FormatInt(latency.Milliseconds(), 10)}
	if err != nil {
		// An unreachable endpoint (refused dial, resolver failure, timeout) is
		// the outage the check reports on — Fail, not Error, so a real scrape
		// outage cannot mask genuine Fails elsewhere in the rollup.
		details["error"] = err.Error()
		writeResult(result{Outcome: "Fail", Summary: "HTTP request failed", Details: details})
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	details["statusCode"] = strconv.Itoa(resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		writeResult(result{Outcome: "Fail", Summary: "HTTP status is not 200 OK", Details: details})
		return nil
	}

	missing, samples, truncated, scanErr := scanMetricFamilies(resp.Body, splitComma(expect))
	details["sampleLines"] = strconv.Itoa(samples)
	if truncated {
		details["truncated"] = "true"
	}
	if scanErr != nil {
		details["error"] = scanErr.Error()
		writeResult(result{Outcome: "Error", Summary: "failed to read HTTP response body", Details: details})
		return nil
	}
	if len(missing) > 0 {
		details["missingFamilies"] = join(missing)
		writeResult(result{Outcome: "Fail", Summary: "expected metric families are missing", Details: details})
		return nil
	}
	if samples == 0 {
		writeResult(result{Outcome: "Fail", Summary: "metrics endpoint returned no metric samples", Details: details})
		return nil
	}
	writeResult(result{Outcome: "Pass", Summary: "metrics scrape succeeded", Details: details})
	return nil
}

// scanMetricFamilies streams a Prometheus text-exposition body line by line,
// counting sample lines and checking off the expected metric family names as
// they appear (in `# TYPE`/`# HELP` headers or as the metric name of a sample
// line). It returns the expected names never seen, in input order. Memory is
// bounded by the expected list — the full family set is never accumulated.
func scanMetricFamilies(r io.Reader, expected []string) (missing []string, samples int, truncated bool, err error) {
	pending := make(map[string]bool, len(expected))
	for _, name := range expected {
		pending[name] = true
	}
	lr := &io.LimitedReader{R: r, N: maxHTTPBodyBytes + 1}
	scanner := bufio.NewScanner(lr)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			// "# TYPE <name> <type>" / "# HELP <name> <text>".
			fields := strings.Fields(line)
			if len(fields) >= 3 && (fields[1] == "TYPE" || fields[1] == "HELP") {
				delete(pending, fields[2])
			}
			continue
		}
		samples++
		name := line
		if i := strings.IndexAny(name, "{ "); i >= 0 {
			name = name[:i]
		}
		delete(pending, name)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, samples, lr.N <= 0, scanErr
	}
	for _, name := range expected {
		if pending[name] {
			missing = append(missing, name)
		}
	}
	return missing, samples, lr.N <= 0, nil
}

// splitComma splits a comma-separated list, trimming whitespace and dropping
// empty elements. An empty input yields nil.
func splitComma(list string) []string {
	if list == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(list, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func writeResult(r result) {
	encoded, err := json.Marshal(r)
	if err != nil {
		encoded = []byte(`{"outcome":"Error","summary":"failed to encode probe result"}`)
	}
	_, _ = os.Stdout.Write(append(encoded, '\n'))
	_ = os.WriteFile(terminationLog, encoded, 0o644)
}

func join(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}
