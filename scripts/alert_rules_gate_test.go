/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The shipped staleness rule must stay cadence-relative (skaphos/fathom#277).
//
// verify-alert-rules only proves the YAML still builds, so nothing else stops a
// well-meaning edit from reintroducing an absolute threshold. That would be a
// silent regression: an absolute number cannot be right for every kind at once,
// and the failure mode is a rule that quietly false-positives on every check
// slower than the value chosen — which is precisely the bug this replaced.
func TestShippedStalenessRuleIsCadenceRelative(t *testing.T) {
	path := filepath.Join("..", "config", "components", "prometheus-rule", "prometheusrule.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shipped rules: %v", err)
	}
	rules := string(raw)

	if !strings.Contains(rules, "fathom_check_interval_seconds") {
		t.Error("the staleness rule must compare against fathom_check_interval_seconds; " +
			"without it the threshold is absolute and cannot suit every check kind")
	}

	if loc, bare := bareStalenessThreshold(rules); bare {
		t.Errorf("shipped rule compares staleness against a hardcoded threshold (%q); "+
			"express it as a multiple of fathom_check_interval_seconds instead", loc)
	}
}

// bareStalenessThreshold reports a comparison of the last-run gauge against a
// naked constant. "> 3 * fathom_check_interval_seconds" is fine — the 3 is an
// allowance, not a threshold — so the number must be followed by a
// multiplication against the interval gauge to pass.
func bareStalenessThreshold(doc string) (string, bool) {
	// Collapse line folding so a wrapped YAML expression reads as one line.
	flat := strings.Join(strings.Fields(doc), " ")
	cmp := regexp.MustCompile(`fathom_check_last_run_timestamp_seconds > [0-9]+`)
	for _, loc := range cmp.FindAllStringIndex(flat, -1) {
		// Look just past the constant: a legitimate allowance is immediately
		// multiplied by the interval gauge, so that name appears right after.
		tail := flat[loc[1]:min(loc[1]+60, len(flat))]
		if !strings.HasPrefix(strings.TrimSpace(tail), "*") ||
			!strings.Contains(tail, "fathom_check_interval_seconds") {
			return flat[loc[0]:loc[1]], true
		}
	}
	return "", false
}

// The documented rule and the shipped rule must not drift apart: an operator who
// copies the guide should get the behaviour the component actually ships.
func TestDocumentedStalenessRuleMatchesShipped(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "docs", "guides", "monitoring.md"))
	if err != nil {
		t.Fatalf("read monitoring guide: %v", err)
	}
	doc := string(guide)

	if !strings.Contains(doc, "fathom_check_interval_seconds") {
		t.Error("the monitoring guide must document the cadence-relative staleness rule")
	}

	if loc, bare := bareStalenessThreshold(doc); bare {
		t.Errorf("monitoring guide still shows an absolute staleness threshold (%q)", loc)
	}
}

// The cadence-relative clause alone silently loses coverage, so the shipped
// rule must keep its never-ran clause (found by adversarial review of #277).
//
// A check with no resolvable cadence publishes no interval series, so the vector
// join in the first clause drops it entirely. That set includes a ClusterHealth
// whose selector matches nothing — a typo'd selector, which is exactly the
// operator error the staleness alert exists to catch, and which the previous
// absolute-threshold rule did catch via the 0 sentinel. Removing the second
// clause would reintroduce that blind spot with no visible failure.
func TestStalenessRuleStillCatchesNeverRan(t *testing.T) {
	for _, f := range []struct{ label, path string }{
		{"shipped rule", filepath.Join("..", "config", "components", "prometheus-rule", "prometheusrule.yaml")},
		{"monitoring guide", filepath.Join("..", "docs", "guides", "monitoring.md")},
	} {
		raw, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatalf("read %s: %v", f.label, err)
		}
		flat := strings.Join(strings.Fields(string(raw)), " ")
		if !strings.Contains(flat, "fathom_check_last_run_timestamp_seconds == 0") {
			t.Errorf("%s lost its never-ran clause; a check with no resolvable cadence "+
				"(including a ClusterHealth matching nothing) would drop out of the join and never alert", f.label)
		}
	}
}
