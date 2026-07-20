/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os"
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
