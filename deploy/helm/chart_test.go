/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package helmchart_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	helmchart "github.com/skaphos/fathom/deploy/helm"
)

func TestLoadStampsTheVersion(t *testing.T) {
	c, err := helmchart.Load("v1.2.3")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Name() != "fathom-operator" {
		t.Errorf("chart name = %q, want fathom-operator", c.Name())
	}
	if c.Metadata.Version != "1.2.3" || c.Metadata.AppVersion != "1.2.3" {
		t.Errorf("version/appVersion = %q/%q, want 1.2.3/1.2.3", c.Metadata.Version, c.Metadata.AppVersion)
	}
	if c.Schema == nil {
		t.Error("values.schema.json was not loaded")
	}
}

func TestLoadCarriesAllNineCRDs(t *testing.T) {
	c, err := helmchart.Load("0.6.0")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(c.CRDObjects()); got != 9 {
		t.Errorf("chart carries %d CRDs, want 9", got)
	}
}

// .helmignore is honoured exactly as `helm package` honours it, so the
// embedded chart is the published chart: CI-only values never ship.
func TestLoadAppliesHelmIgnore(t *testing.T) {
	c, err := helmchart.Load("0.6.0")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, f := range c.Raw {
		if strings.HasPrefix(f.Name, "ci/") {
			t.Errorf("embedded chart ships %s, which .helmignore excludes", f.Name)
		}
	}
}

func TestUpgradePathIsEmbedded(t *testing.T) {
	files, err := helmchart.Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if _, err := fs.Stat(files, helmchart.UpgradePathFile); err != nil {
		t.Errorf("%s is not embedded: %v", helmchart.UpgradePathFile, err)
	}
	if _, err := helmchart.ReadFile(helmchart.UpgradePathFile); err != nil {
		t.Errorf("ReadFile(%s): %v", helmchart.UpgradePathFile, err)
	}
}

func TestLoadRefusesNonSemVerVersions(t *testing.T) {
	for _, v := range []string{"", "devel", "devel+abc123", "abc1234", "v1.2"} {
		if _, err := helmchart.Load(v); !errors.Is(err, helmchart.ErrInvalidVersion) {
			t.Errorf("Load(%q) err = %v, want ErrInvalidVersion", v, err)
		}
	}
}

// A git-describe version is a SemVer pre-release and must load.
func TestLoadAcceptsGitDescribeVersions(t *testing.T) {
	if _, err := helmchart.Load("v0.5.1-12-gabc1234"); err != nil {
		t.Errorf("Load(git describe version): %v", err)
	}
}
