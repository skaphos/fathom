/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os/exec"
	"strings"
	"testing"
)

// fathomctl is a workstation client: it must never link the operator's
// runtime (skaphos/fathom#330 R12). The rule was documentation-only until the
// lifecycle verbs pulled in Helm, which makes an accidental import costlier
// to notice by binary size alone.
var forbiddenCLIImports = []string{
	"github.com/skaphos/fathom/internal/app",
	"github.com/skaphos/fathom/internal/adapter",
	"github.com/skaphos/fathom/internal/controller",
}

func TestFathomctlDoesNotImportOperatorRuntime(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "../cmd/fathomctl").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/fathomctl: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		for _, forbidden := range forbiddenCLIImports {
			if pkg == forbidden || strings.HasPrefix(pkg, forbidden+"/") {
				t.Errorf("fathomctl links %s; internal/cli must not import %s", pkg, forbidden)
			}
		}
	}
}
