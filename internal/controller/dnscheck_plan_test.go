/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func dur(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }

// pairKey renders a pair the way status and metrics key it, so assertions read
// as the identity triple rather than as struct dumps.
func pairKey(p dnsPair) string {
	return fmt.Sprintf("%s|%s|%s", p.Name, p.RecordType, p.VantagePoint.Name)
}

func pairKeys(pairs []dnsPair) []string {
	keys := make([]string, 0, len(pairs))
	for _, p := range pairs {
		keys = append(keys, pairKey(p))
	}
	return keys
}

func TestExpandDNSPairs(t *testing.T) {
	cases := []struct {
		name string
		spec fathomv1alpha1.DNSCheckSpec
		want []string
	}{
		{
			// FR-007 / FR-038: no declared vantage point still means one — the
			// implicit "cluster" one — not zero.
			name: "no resolvers yields one implicit cluster pair per target",
			spec: fathomv1alpha1.DNSCheckSpec{Targets: []fathomv1alpha1.DNSTarget{
				{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "b.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			}},
			want: []string{"a.example.com|A|cluster", "b.example.com|A|cluster"},
		},
		{
			// FR-035: a target naming nothing fans out across every declared
			// vantage point.
			name: "target without an override fans out across all vantage points",
			spec: fathomv1alpha1.DNSCheckSpec{
				Targets:   []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
				Resolvers: []fathomv1alpha1.DNSResolver{{Name: "up"}, {Name: "node", From: fathomv1alpha1.DNSResolverNode}},
			},
			want: []string{"a.example.com|A|up", "a.example.com|A|node"},
		},
		{
			// FR-035: naming one means exactly one, not one-plus-the-fan-out.
			name: "target naming a vantage point yields exactly one pair",
			spec: fathomv1alpha1.DNSCheckSpec{
				Targets:   []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA, Resolver: "node"}},
				Resolvers: []fathomv1alpha1.DNSResolver{{Name: "up"}, {Name: "node", From: fathomv1alpha1.DNSResolverNode}},
			},
			want: []string{"a.example.com|A|node"},
		},
		{
			// Admission permits "cluster" unconditionally, so it must resolve to
			// the implicit vantage point even when the check declares its own.
			name: "reserved cluster name resolves even alongside declared resolvers",
			spec: fathomv1alpha1.DNSCheckSpec{
				Targets:   []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA, Resolver: "cluster"}},
				Resolvers: []fathomv1alpha1.DNSResolver{{Name: "up"}},
			},
			want: []string{"a.example.com|A|cluster"},
		},
		{
			name: "record type defaults to Host",
			spec: fathomv1alpha1.DNSCheckSpec{Targets: []fathomv1alpha1.DNSTarget{{Name: "a.example.com"}}},
			want: []string{"a.example.com|Host|cluster"},
		},
		{
			name: "mixed overrides preserve declaration order",
			spec: fathomv1alpha1.DNSCheckSpec{
				Targets: []fathomv1alpha1.DNSTarget{
					{Name: "fan.example.com", RecordType: fathomv1alpha1.DNSRecordA},
					{Name: "pin.example.com", RecordType: fathomv1alpha1.DNSRecordA, Resolver: "up"},
				},
				Resolvers: []fathomv1alpha1.DNSResolver{{Name: "up"}, {Name: "node", From: fathomv1alpha1.DNSResolverNode}},
			},
			want: []string{"fan.example.com|A|up", "fan.example.com|A|node", "pin.example.com|A|up"},
		},
		{
			// Admission rejects this, so it can only arrive on a stored object
			// that predates the rule. A reported pair explains itself; a silently
			// missing one does not.
			name: "unknown resolver still yields a pair rather than vanishing",
			spec: fathomv1alpha1.DNSCheckSpec{
				Targets:   []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA, Resolver: "ghost"}},
				Resolvers: []fathomv1alpha1.DNSResolver{{Name: "up"}},
			},
			want: []string{"a.example.com|A|ghost"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pairKeys(expandDNSPairs(&tc.spec))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("pairs:\n got %v\nwant %v", got, tc.want)
			}
		})
	}
}

// TestExpandDNSPairsAtSchemaMaximum pins the 48-pair ceiling SC-009's series cap
// is computed from. If a schema cap moves, this fails — deliberately, because
// the ceiling is part of the published contract.
func TestExpandDNSPairsAtSchemaMaximum(t *testing.T) {
	spec := fathomv1alpha1.DNSCheckSpec{}
	for i := range 16 {
		spec.Targets = append(spec.Targets, fathomv1alpha1.DNSTarget{
			Name: fmt.Sprintf("name-%02d.example.com", i), RecordType: fathomv1alpha1.DNSRecordA,
		})
	}
	for i := range 3 {
		spec.Resolvers = append(spec.Resolvers, fathomv1alpha1.DNSResolver{Name: fmt.Sprintf("r%d", i)})
	}
	if got := len(expandDNSPairs(&spec)); got != 48 {
		t.Errorf("pairs at schema maximum: got %d, want 48", got)
	}
}

func TestDNSCheckCadence(t *testing.T) {
	cases := []struct {
		name          string
		interval      *metav1.Duration
		timeout       *metav1.Duration
		wantInterval  time.Duration
		wantRunBound  time.Duration
		wantRationale string
	}{
		{
			name: "unset uses documented defaults", wantInterval: time.Minute, wantRunBound: 10 * time.Second,
		},
		{
			name:     "declared values pass through",
			interval: dur(2 * time.Minute), timeout: dur(30 * time.Second),
			wantInterval: 2 * time.Minute, wantRunBound: 30 * time.Second,
		},
		{
			name:     "sub-floor values are raised, not rejected",
			interval: dur(time.Second), timeout: dur(100 * time.Millisecond),
			wantInterval: fathomv1alpha1.MinCheckInterval, wantRunBound: fathomv1alpha1.MinCheckTimeout,
		},
		{
			// FR-104a. Admission rejects timeout > interval, so this can only be
			// a stored object predating the rule — but the invariant is what makes
			// overlapping runs impossible, so it is enforced rather than assumed.
			name:     "run bound is capped at the cadence",
			interval: dur(30 * time.Second), timeout: dur(5 * time.Minute),
			wantInterval: 30 * time.Second, wantRunBound: 30 * time.Second,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := &fathomv1alpha1.DNSCheck{Spec: fathomv1alpha1.DNSCheckSpec{Interval: tc.interval, Timeout: tc.timeout}}
			if got := dnsCheckInterval(check); got != tc.wantInterval {
				t.Errorf("interval: got %v, want %v", got, tc.wantInterval)
			}
			if got := dnsCheckRunBound(check); got != tc.wantRunBound {
				t.Errorf("run bound: got %v, want %v", got, tc.wantRunBound)
			}
			if dnsCheckRunBound(check) > dnsCheckInterval(check) {
				t.Error("FR-104a violated: run bound exceeds the cadence that schedules it")
			}
		})
	}
}

func TestPerPairBudget(t *testing.T) {
	cases := []struct {
		name        string
		remaining   time.Duration
		pairsLeft   int
		concurrency int
		want        time.Duration
	}{
		{name: "single batch splits nothing", remaining: 60 * time.Second, pairsLeft: 4, concurrency: 4, want: 60 * time.Second},
		{name: "two batches halve the budget", remaining: 60 * time.Second, pairsLeft: 8, concurrency: 4, want: 30 * time.Second},
		{name: "partial final batch still counts", remaining: 60 * time.Second, pairsLeft: 9, concurrency: 4, want: 20 * time.Second},
		{name: "serial fan-out divides by pair count", remaining: 60 * time.Second, pairsLeft: 6, concurrency: 1, want: 10 * time.Second},
		{name: "exhausted budget yields zero", remaining: 0, pairsLeft: 4, concurrency: 4, want: 0},
		{name: "negative budget yields zero", remaining: -time.Second, pairsLeft: 4, concurrency: 4, want: 0},
		{name: "nothing left to run yields zero", remaining: 60 * time.Second, pairsLeft: 0, concurrency: 4, want: 0},
		{name: "zero concurrency is treated as serial", remaining: 60 * time.Second, pairsLeft: 3, concurrency: 0, want: 20 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := perPairBudget(tc.remaining, tc.pairsLeft, tc.concurrency); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPerPairBudgetNeverStarvesTheLastPair is the #150 regression: the CoreDNS
// and node-local-dns adapters hand every sequential target the *full* timeout
// while sharing one context, so the budget is gone before the last targets run.
// Here each pair reads what is actually left, so the budget survives to the end
// even when early pairs overrun their share.
func TestPerPairBudgetNeverStarvesTheLastPair(t *testing.T) {
	const (
		bound       = 60 * time.Second
		pairs       = 12
		concurrency = 3
	)
	remaining := bound
	for left := pairs; left > 0; left -= concurrency {
		budget := perPairBudget(remaining, left, concurrency)
		if budget <= 0 {
			t.Fatalf("pair with %d left got no budget; %v remained", left, remaining)
		}
		// Simulate a batch overrunning its share by half again.
		spent := budget + budget/2
		if spent > remaining {
			spent = remaining
		}
		remaining -= spent
	}
	if remaining < 0 {
		t.Errorf("run overran its bound by %v", -remaining)
	}
}

func TestNextDNSRequeue(t *testing.T) {
	cases := []struct {
		name     string
		interval time.Duration
		elapsed  time.Duration
		want     time.Duration
	}{
		{
			// FR-107 / SC-108: anchored to start, so a fast run does not push the
			// next one out by its own duration.
			name:     "slack yields the declared cadence minus the run",
			interval: time.Minute, elapsed: 5 * time.Second, want: 55 * time.Second,
		},
		{name: "instant run yields the full cadence", interval: time.Minute, elapsed: 0, want: time.Minute},
		{
			// FR-107a: a run that consumed its cadence must not be followed
			// immediately, or a misconfigured check creates pods continuously.
			name:     "run consuming the cadence falls back to the floor",
			interval: time.Minute, elapsed: time.Minute, want: dnsCheckMinRunGap,
		},
		{name: "overrun still yields the floor", interval: time.Minute, elapsed: 90 * time.Second, want: dnsCheckMinRunGap},
		{name: "near-exhaustion clamps up to the floor", interval: 30 * time.Second, elapsed: 29 * time.Second, want: dnsCheckMinRunGap},
		{name: "zero interval disables requeue", interval: 0, elapsed: time.Second, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextDNSRequeue(tc.interval, tc.elapsed, dnsCheckMinRunGap); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// outcomes builds a pair-outcome slice from result strings, all on distinct
// synthetic pairs.
func outcomes(results ...string) []dnsPairOutcome {
	out := make([]dnsPairOutcome, 0, len(results))
	for i, r := range results {
		name := fmt.Sprintf("n%02d.example.com", i)
		out = append(out, dnsPairOutcome{
			pair:   dnsPair{Name: name, RecordType: fathomv1alpha1.DNSRecordA, VantagePoint: implicitDNSVantagePoint()},
			result: fathomv1alpha1.DNSTargetResult{Name: name, RecordType: fathomv1alpha1.DNSRecordA, Resolver: "cluster", Result: r},
		})
	}
	return out
}

func repeat(result string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = result
	}
	return out
}

func TestFoldDNSVerdict(t *testing.T) {
	cases := []struct {
		name    string
		results []string
		want    fathomv1alpha1.HealthReportResult
	}{
		{name: "all passing", results: []string{"Pass", "Pass"}, want: fathomv1alpha1.HealthReportResultPass},
		{name: "one failure dominates passes", results: []string{"Pass", "Fail", "Pass"}, want: fathomv1alpha1.HealthReportResultFail},
		{name: "error outranks failure", results: []string{"Fail", "Error"}, want: fathomv1alpha1.HealthReportResultError},
		{
			// FR-106, the case that decided the clarify question. Skipped would
			// have reported Pass here — green on one sixth of the evidence.
			name:    "truncated run degrades to Unknown rather than reporting green",
			results: append(repeat("Pass", 8), repeat("Unknown", 40)...),
			want:    fathomv1alpha1.HealthReportResultUnknown,
		},
		{
			// The other half of FR-106: incomplete evidence must not mask a real
			// failure among the pairs that did run.
			name:    "a real failure still wins against mostly-unreached",
			results: append([]string{"Fail"}, repeat("Unknown", 47)...),
			want:    fathomv1alpha1.HealthReportResultFail,
		},
		{name: "unknown outranks warn", results: []string{"Warn", "Unknown"}, want: fathomv1alpha1.HealthReportResultUnknown},
		{name: "skipped never wins against a pass", results: []string{"Pass", "Skipped"}, want: fathomv1alpha1.HealthReportResultPass},
		{name: "all skipped folds to skipped", results: []string{"Skipped", "Skipped"}, want: fathomv1alpha1.HealthReportResultSkipped},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := foldDNSVerdict(outcomes(tc.results...)); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCountUnreachedDNSPairs(t *testing.T) {
	if got := countUnreachedDNSPairs(outcomes("Pass", "Unknown", "Fail", "Unknown")); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if got := countUnreachedDNSPairs(outcomes("Pass", "Fail")); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestSummarizeDNSRun(t *testing.T) {
	t.Run("passing run states scope", func(t *testing.T) {
		got := summarizeDNSRun(outcomes("Pass", "Pass"), fathomv1alpha1.HealthReportResultPass, 0)
		if !strings.Contains(got, "2") || !strings.Contains(got, "resolved as expected") {
			t.Errorf("summary: %q", got)
		}
	})

	// FR-021: the polarity must be explicit, or a deliberate "this name must be
	// gone" failure gets triaged as a DNS outage.
	t.Run("negative assertion failure names its polarity", func(t *testing.T) {
		out := outcomes("Fail")
		out[0].pair.Absent = true
		got := summarizeDNSRun(out, fathomv1alpha1.HealthReportResultFail, 0)
		if !strings.Contains(got, "asserted absent but resolved") {
			t.Errorf("summary must name the polarity, got %q", got)
		}
	})

	t.Run("positive assertion failure reads as a resolution failure", func(t *testing.T) {
		got := summarizeDNSRun(outcomes("Fail"), fathomv1alpha1.HealthReportResultFail, 0)
		if !strings.Contains(got, "did not resolve as expected") {
			t.Errorf("summary: %q", got)
		}
		if strings.Contains(got, "absent") {
			t.Errorf("positive assertion summary must not mention absence, got %q", got)
		}
	})

	// FR-106a: the verdict alone says something is wrong; the count is what tells
	// an operator their run bound was too small.
	t.Run("truncated run names the unreached count and the remedy", func(t *testing.T) {
		got := summarizeDNSRun(outcomes(append(repeat("Pass", 2), repeat("Unknown", 3)...)...),
			fathomv1alpha1.HealthReportResultUnknown, 3)
		if !strings.Contains(got, "3 pair(s) not reached") {
			t.Errorf("summary must name the unreached count, got %q", got)
		}
		if !strings.Contains(got, "spec.timeout") {
			t.Errorf("summary must point at the remedy, got %q", got)
		}
	})

	t.Run("empty run says so", func(t *testing.T) {
		if got := summarizeDNSRun(nil, fathomv1alpha1.HealthReportResultSkipped, 0); got != "no targets to evaluate" {
			t.Errorf("got %q", got)
		}
	})

	// The status field caps Summary at 1024 code points; exceeding it fails the
	// write outright.
	t.Run("summary stays within the schema bound", func(t *testing.T) {
		long := outcomes("Fail")
		long[0].pair.Name = strings.Repeat("ü", 4000)
		got := summarizeDNSRun(long, fathomv1alpha1.HealthReportResultFail, 0)
		if n := len([]rune(got)); n > 1024 {
			t.Errorf("summary is %d runes, exceeds the 1024 bound", n)
		}
	})
}

// TestDNSCheckShouldPersistReport covers FR-111: history gains an entry only
// when the verdict changes.
func TestDNSCheckShouldPersistReport(t *testing.T) {
	cases := []struct {
		name    string
		before  *fathomv1alpha1.DNSCheckStatus
		verdict fathomv1alpha1.HealthReportResult
		want    bool
	}{
		{
			name: "nil status is a first run", before: nil,
			verdict: fathomv1alpha1.HealthReportResultPass, want: true,
		},
		{
			name:   "no report yet is a first run",
			before: &fathomv1alpha1.DNSCheckStatus{}, verdict: fathomv1alpha1.HealthReportResultPass, want: true,
		},
		{
			name:    "unchanged verdict writes nothing",
			before:  &fathomv1alpha1.DNSCheckStatus{LastReportName: "r-1", LastResult: "Pass"},
			verdict: fathomv1alpha1.HealthReportResultPass, want: false,
		},
		{
			name:    "changed verdict writes a record",
			before:  &fathomv1alpha1.DNSCheckStatus{LastReportName: "r-1", LastResult: "Pass"},
			verdict: fathomv1alpha1.HealthReportResultFail, want: true,
		},
		{
			name:    "recovery is a change too",
			before:  &fathomv1alpha1.DNSCheckStatus{LastReportName: "r-1", LastResult: "Fail"},
			verdict: fathomv1alpha1.HealthReportResultPass, want: true,
		},
		{
			// A report whose creation failed leaves the name empty, so the next
			// run must retry rather than treat the transition as recorded.
			name:    "a failed write is retried on the next run",
			before:  &fathomv1alpha1.DNSCheckStatus{LastReportName: "", LastResult: "Fail"},
			verdict: fathomv1alpha1.HealthReportResultFail, want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dnsCheckShouldPersistReport(tc.before, tc.verdict); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDNSNameserverAddress covers the gap between what the contract admits and
// what a Pod's dnsConfig can carry. Admission validates an optional port
// 1-65535, but dnsConfig.nameservers takes a bare IP only and the API server
// rejects anything else — so a ported resolver failed every run with an opaque
// "spec.dnsConfig.nameservers[0]: Invalid value" until this conversion existed.
func TestDNSNameserverAddress(t *testing.T) {
	cases := []struct {
		name    string
		address string
		want    string
		wantErr string
	}{
		{name: "bare IPv4 passes through", address: "10.0.0.10", want: "10.0.0.10"},
		{name: "bare IPv6 passes through", address: "2001:db8::1", want: "2001:db8::1"},
		{name: "explicit port 53 is stripped", address: "10.0.0.10:53", want: "10.0.0.10"},
		{name: "bracketed IPv6 with port 53 is stripped", address: "[2001:db8::1]:53", want: "2001:db8::1"},
		{
			// Silently dropping the port would query 53 instead and report a
			// confident verdict about a resolver the check never asked.
			name:    "a non-53 port is refused, not silently dropped",
			address: "10.0.0.10:5353", wantErr: "port 5353 is not supported",
		},
		{name: "empty address is refused", address: "", wantErr: "requires an address"},
		{name: "hostname is refused", address: "resolver.example.com", wantErr: "not an IP"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dnsNameserverAddress(tc.address)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("got %q, want an error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDNSTargetResults(t *testing.T) {
	if got := dnsTargetResults(nil); got != nil {
		t.Errorf("empty outcomes should project to nil, got %#v", got)
	}
	got := dnsTargetResults(outcomes("Pass", "Fail"))
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].Result != "Pass" || got[1].Result != "Fail" {
		t.Errorf("projection lost order or results: %#v", got)
	}
}
