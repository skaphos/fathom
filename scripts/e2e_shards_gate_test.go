/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/skaphos/fathom/test/utils"
)

// TestE2EShardPlannerKnowsEveryOptInAddon pins scripts/e2e-shards.sh's
// OPT_IN_SHARDS list to OptInAddons() in test/utils (skaphos/fathom#178).
// The suite's E2E_ADDONS contract and the CI shard planner must agree on the
// opt-in addon set: an addon known only to Go would never get a CI shard (its
// e2e specs would silently stop running on PRs), and a shard known only to
// the script would spin up a kind cluster whose E2E_ADDONS value the suite
// rejects. Adding an opt-in adapter therefore has to touch both lists — this
// guard turns forgetting one of them into a unit-test failure.
func TestE2EShardPlannerKnowsEveryOptInAddon(t *testing.T) {
	const script = "e2e-shards.sh"
	data, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read %s: %v", script, err)
	}

	m := regexp.MustCompile(`(?m)^OPT_IN_SHARDS="([^"]*)"$`).FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("%s: OPT_IN_SHARDS=\"...\" assignment not found", script)
	}
	got := strings.Fields(m[1])
	want := utils.OptInAddons()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s OPT_IN_SHARDS = %v, test/utils OptInAddons() = %v; keep them identical",
			script, got, want)
	}
}

// TestE2EShardPlannerClassifiesPaths pins the planner's per-path routing for
// the paths whose shard is a deliberate choice rather than the fail-open
// default. A typo in one of these case patterns would not fail loudly: the
// path would fall through to the full matrix (expensive) or, worse, to a
// shard that never runs its specs. Each row is one `--classify` invocation.
func TestE2EShardPlannerClassifiesPaths(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		// DNSCheck is core-tier: its specs and sample route to the core shard.
		{path: "test/e2e/dnscheck_test.go", want: "core"},
		{path: "test/e2e/dnscheck_resolution_test.go", want: "core"},
		{path: "test/e2e/dnscheck_aggregation_test.go", want: "core"},
		{path: "test/e2e/dnscheck_restricted_test.go", want: "core"},
		{path: "config/samples/fathom_v1alpha1_dnscheck.yaml", want: "core"},
		// Its reconciler is shared surface and must still fan out.
		{path: "internal/controller/dnscheck_controller.go", want: "all"},
		// Anchors on either side of the DNSCheck rows.
		{path: "test/e2e/externaldns_test.go", want: "external-dns"},
		{path: "test/e2e/coredns_test.go", want: "core"},
		{path: "docs/guides/dns-checks.md", want: ""},
		{path: "scripts/e2e-shards.sh", want: "all"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			out, err := exec.Command("bash", "e2e-shards.sh", "--classify", tc.path).Output()
			if err != nil {
				t.Fatalf("e2e-shards.sh --classify %s: %v", tc.path, err)
			}
			if got := strings.TrimSpace(string(out)); got != tc.want {
				t.Errorf("shard for %s = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
