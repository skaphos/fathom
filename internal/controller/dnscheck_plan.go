/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// This file holds the DNSCheck reconciler's arithmetic, kept pure so the
// decisions that shape a run — how many pairs it covers, how its budget is
// split, when the next one is due, and how partial evidence folds — are
// table-testable without an API server. The reconciler in
// dnscheck_controller.go supplies the I/O around them.

const (
	// defaultDNSCheckInterval and defaultDNSCheckTimeout mirror the defaults
	// documented on DNSCheckSpec. They are applied here rather than as CRD
	// defaults because a stored object may predate them.
	defaultDNSCheckInterval = time.Minute
	defaultDNSCheckTimeout  = 10 * time.Second

	// dnsCheckMinRunGap is the floor on the delay between one run starting and
	// the next (FR-107a). Scheduling is anchored to a run's *start*, so a run
	// that consumes its whole budget would otherwise be followed immediately by
	// another, creating probe pods continuously. A run only consumes its whole
	// budget when it is truncating — that is, when the check is misconfigured —
	// so this bounds the cost of a mistake rather than shaping normal operation.
	// Five seconds is half the schema's interval floor: perceptible on the
	// tightest admissible cadence, negligible on any realistic one.
	dnsCheckMinRunGap = 5 * time.Second

	// dnsCheckMinPairBudget is the least time worth giving a pair. Below it the
	// probe pod cannot realistically be scheduled, pull its image, and answer,
	// so launching one would burn the remaining budget to produce an Error that
	// says nothing about DNS. Pairs that cannot be afforded are left Unknown
	// instead — which is what "the run did not reach them" means (FR-106).
	dnsCheckMinPairBudget = 2 * time.Second
)

// dnsVantagePoint is one resolved place a query may be issued from. It is
// spec.resolvers[] plus the implicit "cluster" entry a check gets when it
// declares none.
type dnsVantagePoint struct {
	Name    string
	From    fathomv1alpha1.DNSResolverSource
	Address string
}

// dnsPair is one (target, vantage point) combination: the unit of evaluation,
// of per-target result identity, and of metric series (FR-035).
type dnsPair struct {
	Name         string
	RecordType   fathomv1alpha1.DNSRecordType
	Expected     []string
	Absent       bool
	VantagePoint dnsVantagePoint
}

// dnsPairOutcome couples a pair with the result it produced. The pair is
// retained because DNSTargetResult cannot carry the assertion's polarity — the
// schema is frozen — and the summary must name it (FR-021).
type dnsPairOutcome struct {
	pair   dnsPair
	result fathomv1alpha1.DNSTargetResult
}

// implicitDNSVantagePoint is the vantage point a check resolves from when it
// declares none, and the one the reserved name "cluster" always denotes.
func implicitDNSVantagePoint() dnsVantagePoint {
	return dnsVantagePoint{Name: fathomv1alpha1.DefaultDNSResolverName, From: fathomv1alpha1.DNSResolverCluster}
}

// dnsVantagePoints returns the vantage points a target with no override is
// evaluated against. An empty spec.resolvers still means one vantage point —
// the implicit "cluster" one — not zero.
func dnsVantagePoints(spec *fathomv1alpha1.DNSCheckSpec) []dnsVantagePoint {
	if len(spec.Resolvers) == 0 {
		return []dnsVantagePoint{implicitDNSVantagePoint()}
	}
	points := make([]dnsVantagePoint, 0, len(spec.Resolvers))
	for _, resolver := range spec.Resolvers {
		from := resolver.From
		if from == "" {
			from = fathomv1alpha1.DNSResolverCluster
		}
		points = append(points, dnsVantagePoint{Name: resolver.Name, From: from, Address: resolver.Address})
	}
	return points
}

// namedDNSVantagePoint resolves a target's explicit resolver override. The
// reserved name always denotes the implicit vantage point, whether or not the
// check also declares its own — admission permits "cluster" unconditionally.
// The second return is false when the name matches nothing, which admission
// should have rejected; the caller reports that pair rather than dropping it.
func namedDNSVantagePoint(spec *fathomv1alpha1.DNSCheckSpec, name string) (dnsVantagePoint, bool) {
	if name == fathomv1alpha1.DefaultDNSResolverName {
		return implicitDNSVantagePoint(), true
	}
	for _, point := range dnsVantagePoints(spec) {
		if point.Name == name {
			return point, true
		}
	}
	return dnsVantagePoint{}, false
}

// expandDNSPairs turns a specification into the pairs one run must evaluate
// (FR-035). A target naming a vantage point yields exactly one pair; a target
// naming none yields one per declared vantage point.
//
// Order is deterministic — targets in declaration order, vantage points within
// each — so logs, status, and metric series are reproducible run to run.
//
// A target whose named vantage point does not exist still yields a pair, with
// the vantage point marked unresolved. Admission rejects that specification, so
// reaching this path means a stored object predates the rule; reporting it as a
// failed pair explains itself far better than a missing result would.
func expandDNSPairs(spec *fathomv1alpha1.DNSCheckSpec) []dnsPair {
	points := dnsVantagePoints(spec)
	pairs := make([]dnsPair, 0, len(spec.Targets)*len(points))
	for _, target := range spec.Targets {
		recordType := target.RecordType
		if recordType == "" {
			recordType = fathomv1alpha1.DNSRecordHost
		}
		base := dnsPair{
			Name:       target.Name,
			RecordType: recordType,
			Expected:   append([]string(nil), target.ExpectedAnswers...),
			Absent:     target.Absent,
		}
		if target.Resolver != "" {
			point, ok := namedDNSVantagePoint(spec, target.Resolver)
			if !ok {
				point = dnsVantagePoint{Name: target.Resolver}
			}
			pair := base
			pair.VantagePoint = point
			pairs = append(pairs, pair)
			continue
		}
		for _, point := range points {
			pair := base
			pair.VantagePoint = point
			pairs = append(pairs, pair)
		}
	}
	return pairs
}

// dnsCheckInterval is the cadence, defaulted and raised to the schema floor.
func dnsCheckInterval(check *fathomv1alpha1.DNSCheck) time.Duration {
	if check.Spec.Interval == nil || check.Spec.Interval.Duration <= 0 {
		return defaultDNSCheckInterval
	}
	return clampCadence(check.Spec.Interval.Duration, fathomv1alpha1.MinCheckInterval)
}

// dnsCheckRunBound is how long one run may take in total (FR-104): the whole
// run, not each pair.
//
// It is additionally capped at the cadence so a run can never outlast the tick
// that scheduled it (FR-104a). Admission already rejects timeout > interval, so
// the cap only engages for an object stored before that rule existed — but the
// invariant is what makes overlapping runs impossible, so it is enforced here
// rather than assumed.
func dnsCheckRunBound(check *fathomv1alpha1.DNSCheck) time.Duration {
	bound := defaultDNSCheckTimeout
	if check.Spec.Timeout != nil && check.Spec.Timeout.Duration > 0 {
		bound = clampCadence(check.Spec.Timeout.Duration, fathomv1alpha1.MinCheckTimeout)
	}
	if interval := dnsCheckInterval(check); bound > interval {
		return interval
	}
	return bound
}

// perPairBudget is how long the next pair may take, given the budget still left
// in the run and how many pairs (including it) have yet to start.
//
// The remaining pairs run in ceil(pairsLeft/concurrency) further batches, so
// each batch gets an equal share of what is left. Reading *remaining* rather
// than the original bound is what prevents #150: a pair that overruns its share
// shrinks every later pair's share instead of silently consuming their time, and
// a pair that finishes early hands its unused budget back.
//
// Returns zero when nothing is left to spend or nothing is left to run.
func perPairBudget(remaining time.Duration, pairsLeft, concurrency int) time.Duration {
	if remaining <= 0 || pairsLeft <= 0 {
		return 0
	}
	if concurrency < 1 {
		concurrency = 1
	}
	batches := (pairsLeft + concurrency - 1) / concurrency
	return remaining / time.Duration(batches)
}

// nextDNSRequeue is the delay until the next run, measured from the moment this
// one started (FR-107).
//
// Anchoring to the start is what makes the effective cadence equal the declared
// one: anchoring to completion, as the other check kinds do, adds the run's own
// duration to every interval. The floor keeps a check that cannot finish within
// its cadence to a bounded rate rather than running back-to-back (FR-107a).
func nextDNSRequeue(interval, elapsed, minGap time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	if minGap < 0 {
		minGap = 0
	}
	if remaining := interval - elapsed; remaining > minGap {
		return remaining
	}
	return minGap
}

// foldDNSVerdict folds per-pair outcomes into the check's single verdict
// (FR-108), using the project-wide fold so DNSCheck agrees with every other
// kind on precedence.
//
// Unreached pairs arrive here as Unknown, which participates: it ranks above
// Warn so incomplete evidence degrades the verdict, and below Fail so a genuine
// failure among the pairs that did run still wins (FR-106).
func foldDNSVerdict(outcomes []dnsPairOutcome) fathomv1alpha1.HealthReportResult {
	results := make([]fathomv1alpha1.HealthReportResult, 0, len(outcomes))
	for _, outcome := range outcomes {
		results = append(results, fathomv1alpha1.HealthReportResult(outcome.result.Result))
	}
	return fathomv1alpha1.WorstResult(results, true)
}

// countUnreachedDNSPairs reports how many pairs the run never evaluated. A pair
// still carrying Unknown when the run ends was seeded and never overwritten.
func countUnreachedDNSPairs(outcomes []dnsPairOutcome) int {
	var unreached int
	for _, outcome := range outcomes {
		if outcome.result.Result == string(fathomv1alpha1.HealthReportResultUnknown) {
			unreached++
		}
	}
	return unreached
}

// DNSCheckStatus.Summary carries the same 1024 MaxLength as HealthCheck's, so
// truncateSummary is shared rather than duplicated — and it counts runes, which
// is what OpenAPI maxLength bounds.

// summarizeDNSRun builds the one-line outcome (FR-021).
//
// When the verdict comes from a negative assertion the summary says so
// explicitly: "resolved but is asserted absent" reads as the deliberate policy
// failure it is, where a bare "target failed" would be triaged as a DNS outage.
func summarizeDNSRun(outcomes []dnsPairOutcome, verdict fathomv1alpha1.HealthReportResult, unreached int) string {
	if len(outcomes) == 0 {
		return "no targets to evaluate"
	}

	var passing int
	for _, outcome := range outcomes {
		if outcome.result.Result == string(fathomv1alpha1.HealthReportResultPass) {
			passing++
		}
	}
	scope := fmt.Sprintf("%d/%d pairs passing", passing, len(outcomes))

	if verdict == fathomv1alpha1.HealthReportResultPass {
		return truncateSummary(fmt.Sprintf("all %d (target, vantage point) pairs resolved as expected", len(outcomes)))
	}

	var parts []string
	if lead, ok := firstOutcomeWithResult(outcomes, verdict); ok {
		parts = append(parts, describeDNSFailure(lead, verdict))
	} else {
		parts = append(parts, fmt.Sprintf("check verdict is %s", verdict))
	}
	parts = append(parts, scope)
	if unreached > 0 {
		parts = append(parts,
			fmt.Sprintf("%d pair(s) not reached before the run bound elapsed; raise spec.timeout or declare fewer targets", unreached))
	}
	return truncateSummary(strings.Join(parts, "; "))
}

// firstOutcomeWithResult returns the first pair matching the folded verdict, in
// deterministic pair order, so the same failing run always names the same pair.
func firstOutcomeWithResult(outcomes []dnsPairOutcome, result fathomv1alpha1.HealthReportResult) (dnsPairOutcome, bool) {
	for _, outcome := range outcomes {
		if outcome.result.Result == string(result) {
			return outcome, true
		}
	}
	return dnsPairOutcome{}, false
}

func describeDNSFailure(outcome dnsPairOutcome, verdict fathomv1alpha1.HealthReportResult) string {
	subject := fmt.Sprintf("%s (%s) from %s", outcome.pair.Name, outcome.pair.RecordType, outcome.pair.VantagePoint.Name)
	switch {
	case verdict == fathomv1alpha1.HealthReportResultFail && outcome.pair.Absent:
		// FR-021: name the polarity. This check asserts the name is GONE, so a
		// failure means it resolved — the opposite of an outage.
		return "asserted absent but resolved: " + subject
	case verdict == fathomv1alpha1.HealthReportResultFail:
		return "did not resolve as expected: " + subject
	case verdict == fathomv1alpha1.HealthReportResultError:
		return "could not be evaluated: " + subject
	case verdict == fathomv1alpha1.HealthReportResultUnknown:
		return "not evaluated: " + subject
	default:
		return string(verdict) + ": " + subject
	}
}

// dnsNameserverAddress converts a declared resolver address into the form a
// Pod's dnsConfig can carry, which is a bare IP and nothing else.
//
// The contract admits an optional port (spec 005 FR-009 validates 1-65535), but
// Kubernetes `dnsConfig.nameservers` has no way to express one — the API server
// rejects anything that is not a plain address. So :53 is stripped, and any
// other port is refused rather than silently dropped: dropping it would send the
// query to port 53 and report a confident Pass or Fail about a resolver the
// check never actually asked.
func dnsNameserverAddress(address string) (string, error) {
	if address == "" {
		return "", errors.New("an explicit vantage point requires an address")
	}
	if net.ParseIP(address) != nil {
		return address, nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("resolver address %q is not an IP or IP:port", address)
	}
	if net.ParseIP(host) == nil {
		return "", fmt.Errorf("resolver address %q does not contain a valid IP", address)
	}
	if port != "53" {
		return "", fmt.Errorf(
			"resolver port %s is not supported: a probe pod's DNS configuration can only query port 53, "+
				"so declare %s without a port or run the resolver on 53", port, host)
	}
	return host, nil
}

// dnsCheckShouldPersistReport decides whether a completed run writes a new
// HealthReport (FR-111): only when the verdict changed, or when there is no
// report yet.
//
// It reads the status as it was BEFORE this run, not the version the run just
// built — comparing the new verdict against itself would always look unchanged.
//
// This is deliberately simpler than NodeCertificateCheck's three-way decision.
// That kind is watch-driven, with roughly one reconcile per node-agent write per
// interval, so it needs a throttle to stop an unchanged verdict rewriting status
// on every event. A DNSCheck reconciles on its own cadence — the interval *is*
// the throttle — so every run legitimately advances lastRunTime and the only
// question left is whether history gains an entry.
//
// A report whose creation failed leaves lastReportName empty, so the next run
// retries rather than silently skipping the transition.
func dnsCheckShouldPersistReport(before *fathomv1alpha1.DNSCheckStatus, verdict fathomv1alpha1.HealthReportResult) bool {
	if before == nil {
		return true
	}
	return before.LastReportName == "" || before.LastResult != string(verdict)
}

// dnsTargetResults projects outcomes onto the status field, in pair order. The
// slice is rebuilt from the current specification every run and replaces the
// stored one wholesale, so a pair the specification no longer declares cannot
// survive (FR-036).
func dnsTargetResults(outcomes []dnsPairOutcome) []fathomv1alpha1.DNSTargetResult {
	if len(outcomes) == 0 {
		return nil
	}
	results := make([]fathomv1alpha1.DNSTargetResult, 0, len(outcomes))
	for _, outcome := range outcomes {
		results = append(results, outcome.result)
	}
	return results
}
