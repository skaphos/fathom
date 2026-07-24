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
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch *mode {
	case "dns":
		return runDNS(ctx, *target)
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

func runDNS(ctx context.Context, target string) error {
	if target == "" {
		return errors.New("dns probe target is required")
	}
	started := time.Now()
	addresses, err := net.DefaultResolver.LookupHost(ctx, target)
	latency := time.Since(started)
	details := map[string]string{"target": target, "latencyMillis": strconv.FormatInt(latency.Milliseconds(), 10)}
	if err != nil {
		details["error"] = err.Error()
		// A DNS-level failure (NXDOMAIN, SERVFAIL, no answer, or a resolver
		// timeout) is precisely the condition a dns_resolution check exists to
		// detect, so it is a Fail — the same as a refused TCP dial in
		// runTCPConnect — not a probe-infrastructure Error. Error outranks Fail
		// on the severity ladder (Pass<Skipped<Warn<Unknown<Fail<Error), so
		// misclassifying a real DNS outage as Error would mask genuine Fails
		// elsewhere in the ClusterHealth rollup. Reserve Error for faults that
		// are not the resolver's answer (anything that is not a *net.DNSError).
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			writeResult(result{Outcome: "Fail", Summary: "DNS resolution failed", Details: details})
			return nil
		}
		writeResult(result{Outcome: "Error", Summary: "DNS resolution failed", Details: details})
		return err
	}
	if len(addresses) == 0 {
		writeResult(result{Outcome: "Fail", Summary: "DNS resolution returned no addresses", Details: details})
		return nil
	}
	details["addresses"] = join(addresses)
	writeResult(result{Outcome: "Pass", Summary: "DNS resolution succeeded", Details: details})
	return nil
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
