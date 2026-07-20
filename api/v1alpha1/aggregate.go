/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1

// WorstResult folds a set of per-item Results into a single worst-case verdict
// for a rollup (a ClusterHealth over its children, an AddonCheck over its
// per-check outcomes, a NodeCertificateCheck over its scanned certificates).
//
// The fold distinguishes *participating* from *informational* inputs:
//
//   - Pass, Warn, Unknown, Fail, Error participate: the result is the worst of
//     them by Severity (Pass < Warn < Unknown < Fail < Error).
//   - Skipped is informational — "this check did not apply" — and never wins
//     while any participating result is present. A Skipped alongside a Pass
//     must not drag a healthy rollup down to Skipped (#160).
//   - Empty ("", never observed) is informational by default and excluded.
//     When coerceEmptyToUnknown is set, an empty input is promoted to a
//     participating Unknown instead of being silently dropped, so a rollup
//     whose only signal is a verdictless child degrades to Unknown rather than
//     vanishing to a green empty result (#161). A live Fail sibling still wins,
//     because Fail outranks Unknown.
//
// When no input participates — an empty set, an all-Skipped set, or an
// all-empty set without coercion — the fold returns Skipped: "ran, observed
// nothing to grade." Callers that need a distinct signal for "nothing matched"
// (ClusterHealth's no-match case) handle it before calling.
func WorstResult(results []HealthReportResult, coerceEmptyToUnknown bool) HealthReportResult {
	var worst HealthReportResult
	worstRank := 0
	for _, r := range results {
		if r == "" && coerceEmptyToUnknown {
			r = HealthReportResultUnknown
		}
		// Skipped and (uncoerced) empty are informational: they never win the
		// fold. Unrecognized values (Severity 0) are ignored the same way.
		if r == HealthReportResultSkipped || r == "" {
			continue
		}
		if rank := r.Severity(); rank > worstRank {
			worst, worstRank = r, rank
		}
	}
	if worstRank == 0 {
		return HealthReportResultSkipped
	}
	return worst
}
