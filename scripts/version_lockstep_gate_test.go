/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os"
	"os/exec"
	"testing"
)

// runLockstep runs scripts/check-version-lockstep.sh from the scripts/ package
// directory (its own working directory) and reports success and combined output.
// The script re-anchors to the repo root via BASH_SOURCE, so running it from
// here exercises the exact invocation CI uses.
func runLockstep(t *testing.T, versionOverride string) (bool, string) {
	t.Helper()

	cmd := exec.Command("bash", "check-version-lockstep.sh")
	cmd.Env = os.Environ()
	if versionOverride != "" {
		cmd.Env = append(cmd.Env, "LOCKSTEP_VERSION="+versionOverride)
	}
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}

// TestVersionLockstepInSync is the regression guard for SKA-579: the compiled
// probe/node-agent defaults, the Helm chart version/appVersion, the e2e image
// tags, and the CoreDNS sample probeImage pin must all equal the released
// version in .release-please-manifest.json. Bumping one lockstep site (or the
// release) without the others turns this test red.
func TestVersionLockstepInSync(t *testing.T) {
	ok, out := runLockstep(t, "")
	if !ok {
		t.Fatalf("check-version-lockstep.sh reported drift against the released version:\n%s", out)
	}
}

// TestVersionLockstepDetectsDrift proves the gate actually fails on drift (not
// just that it happens to pass today): forcing an expected version that no
// lockstep site carries must make the script exit non-zero.
func TestVersionLockstepDetectsDrift(t *testing.T) {
	ok, out := runLockstep(t, "9999.0.0")
	if ok {
		t.Fatalf("check-version-lockstep.sh passed for a version no site carries; the gate does not detect drift:\n%s", out)
	}
}
