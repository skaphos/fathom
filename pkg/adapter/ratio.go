/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package adapter

import (
	"fmt"
	"strings"
)

// Reserved, engine-level threshold keys (#159). Unlike every other key in
// [FamilyPolicy.Thresholds], these are consumed by Fathom's own rollup
// aggregation, not by the adapter: they configure a per-family ratio rollup
// so a single broken managed resource does not fail the whole addon check
// via worst-of aggregation. Fathom validates their values centrally and
// exempts them from [ThresholdAdvertiser] key validation; adapters must not
// consume or advertise them.
const (
	// ThresholdKeyWarnRatio holds the warn-level ratio threshold: the family
	// verdict is at least Warn when the degraded (Warn+Fail) fraction of the
	// evaluated population strictly exceeds this percentage.
	ThresholdKeyWarnRatio = "warnRatio"
	// ThresholdKeyFailRatio holds the fail-level ratio threshold: the family
	// verdict is Fail when the unhealthy (Fail) fraction of the evaluated
	// population strictly exceeds this percentage.
	ThresholdKeyFailRatio = "failRatio"
)

// RatioPercent is a percentage threshold parsed from a reserved ratio key.
// It is held in hundredths of a percent so verdict comparisons are
// integer-exact — no float boundary surprises at values like "0" or "2.5".
type RatioPercent struct {
	// basisPoints is the threshold in hundredths of a percent (0..10000).
	basisPoints int64
	// raw is the value exactly as configured, preserved for report details.
	raw string
}

// String returns the threshold exactly as it was configured (e.g. "5" or
// "5%"), so persisted report details echo the operator's spelling.
func (p RatioPercent) String() string { return p.raw }

// exceededBy reports whether count out of population strictly exceeds the
// threshold. Strict ("above"): a fraction exactly equal to the threshold
// does not escalate, so a threshold of 0 reproduces worst-of exactly.
// Implemented as integer cross-multiplication to stay exact.
func (p RatioPercent) exceededBy(count, population int) bool {
	return int64(count)*10_000 > p.basisPoints*int64(population)
}

// RatioThresholds carries the parsed reserved ratio keys for one family.
// The zero value means "not configured": that family keeps plain worst-of
// semantics. Either level may be set alone; the omitted level falls back to
// worst-of for the outcomes it governs (see [FamilyRatioVerdict]).
type RatioThresholds struct {
	// Warn is the warn-level threshold, nil when warnRatio is unset.
	Warn *RatioPercent
	// Fail is the fail-level threshold, nil when failRatio is unset.
	Fail *RatioPercent
}

// Configured reports whether at least one ratio threshold is set — the gate
// for ratio evaluation of a family.
func (rt RatioThresholds) Configured() bool { return rt.Warn != nil || rt.Fail != nil }

// ParseRatioThresholds extracts and validates the reserved ratio keys from a
// family's thresholds map. Keys other than [ThresholdKeyWarnRatio] and
// [ThresholdKeyFailRatio] are ignored (they belong to the adapter). A value
// must be a non-negative decimal percentage in [0, 100] with at most two
// decimal places and an optional trailing "%" — "5", "5%", and "2.5" are all
// valid. The returned error names the offending key so it can surface on the
// AddonCheck Accepted condition verbatim.
func ParseRatioThresholds(thresholds map[string]string) (RatioThresholds, error) {
	var rt RatioThresholds
	for _, key := range []string{ThresholdKeyWarnRatio, ThresholdKeyFailRatio} {
		raw, ok := thresholds[key]
		if !ok {
			continue
		}
		p, err := parseRatioPercent(raw)
		if err != nil {
			return RatioThresholds{}, fmt.Errorf("%s: %w", key, err)
		}
		if key == ThresholdKeyWarnRatio {
			rt.Warn = p
		} else {
			rt.Fail = p
		}
	}
	return rt, nil
}

// parseRatioPercent parses one percentage value into fixed-point hundredths
// of a percent. Manual digit parsing (rather than ParseFloat) keeps the
// two-decimal grammar exact and rejects float-only forms like "1e1".
func parseRatioPercent(raw string) (*RatioPercent, error) {
	errInvalid := fmt.Errorf("must be a percentage between 0 and 100 with at most two decimal places, got %q", raw)
	s := strings.TrimSuffix(raw, "%")
	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if !isDigits(intPart) || len(intPart) > 3 {
		return nil, errInvalid
	}
	if hasFrac && (!isDigits(fracPart) || len(fracPart) > 2) {
		return nil, errInvalid
	}
	var basis int64
	for _, c := range intPart {
		basis = basis*10 + int64(c-'0')
	}
	basis *= 100
	if hasFrac {
		frac := int64(0)
		for _, c := range fracPart {
			frac = frac*10 + int64(c-'0')
		}
		if len(fracPart) == 1 {
			frac *= 10
		}
		basis += frac
	}
	if basis > 10_000 {
		return nil, errInvalid
	}
	return &RatioPercent{basisPoints: basis, raw: raw}, nil
}

// isDigits reports whether s is non-empty and all ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// RatioRollup is the outcome of evaluating one family under ratio
// thresholds, carrying the population counts that produced the verdict so
// rollups stay explainable from the persisted report alone.
type RatioRollup struct {
	// Verdict is the family verdict after ratio evaluation.
	Verdict Outcome
	// Population counts checks with outcome Pass, Warn, or Fail — the ratio
	// denominator. Skipped is excluded; Error short-circuits evaluation.
	Population int
	// Unhealthy counts Fail outcomes (numerator for the fail threshold).
	Unhealthy int
	// Degraded counts Warn and Fail outcomes (numerator for the warn
	// threshold).
	Degraded int
}

// FamilyRatioVerdict rolls up the checks belonging to family under the given
// ratio thresholds. It is the policy-aware sibling of [FamilyOutcome]:
//
//   - Any Error in the family short-circuits to an Error verdict — adapter
//     blindness is never averaged away. Counts still reflect the non-Error
//     checks that were seen.
//   - Skipped checks are excluded from the population and never escalate.
//   - An empty population yields Pass, matching [FamilyOutcome]'s behavior
//     for a family with no checks.
//   - Fail when the unhealthy fraction strictly exceeds Fail; with Fail
//     unset, any unhealthy check fails the family (worst-of fallback).
//   - Otherwise Warn when the degraded fraction strictly exceeds Warn; with
//     Warn unset, any Warn check warns the family (worst-of fallback).
//   - Otherwise Pass.
//
// Checks for other families are ignored, preserving family independence.
func FamilyRatioVerdict(checks []CheckResult, family Family, rt RatioThresholds) RatioRollup {
	r := RatioRollup{Verdict: OutcomePass}
	errorSeen := false
	warnSeen := false
	for _, c := range checks {
		if c.Family != family {
			continue
		}
		switch c.Outcome {
		case OutcomeError:
			errorSeen = true
		case OutcomeFail:
			r.Population++
			r.Unhealthy++
			r.Degraded++
		case OutcomeWarn:
			r.Population++
			r.Degraded++
			warnSeen = true
		case OutcomePass:
			r.Population++
		}
	}
	switch {
	case errorSeen:
		r.Verdict = OutcomeError
	case r.Population == 0:
		r.Verdict = OutcomePass
	case rt.Fail != nil && rt.Fail.exceededBy(r.Unhealthy, r.Population):
		r.Verdict = OutcomeFail
	case rt.Fail == nil && r.Unhealthy > 0:
		r.Verdict = OutcomeFail
	case rt.Warn != nil && rt.Warn.exceededBy(r.Degraded, r.Population):
		r.Verdict = OutcomeWarn
	case rt.Warn == nil && warnSeen:
		r.Verdict = OutcomeWarn
	}
	return r
}
