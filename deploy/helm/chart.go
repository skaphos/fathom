/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package helmchart embeds the fathom-operator Helm chart so fathomctl
// carries exactly its own release's chart (skaphos/fathom#330). It lives
// beside the chart because go:embed cannot reach a parent directory.
package helmchart

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v4/pkg/chart/loader/archive"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/ignore"
)

// chartDir is the embedded chart's root inside chartFS.
const chartDir = "fathom-operator"

// UpgradePathFile is the chart-relative path of the upgrade-path declaration.
const UpgradePathFile = "upgrade-path.yaml"

// CRDDir is the chart-relative directory holding the release's CRDs.
const CRDDir = "crds"

// The all: prefix keeps dot-files, so .helmignore is embedded and applied
// exactly as `helm package` applies it.
//
//go:embed all:fathom-operator
var chartFS embed.FS

// ErrInvalidVersion reports a CLI version that cannot become a chart version.
// Helm requires SemVer, and the lifecycle verbs compare versions, so a bare
// commit hash or "devel" build cannot install anything.
var ErrInvalidVersion = errors.New("version is not valid SemVer")

// Files returns the embedded chart directory as it sits in the repository.
// .helmignore is applied only by Load; nothing read through Files (CRDs, the
// upgrade-path declaration) is ignored by it.
func Files() (fs.FS, error) {
	return fs.Sub(chartFS, chartDir)
}

// Load builds the chart with its version and appVersion stamped to version,
// mirroring `helm package --version --app-version` in the release workflow. A
// leading "v" is accepted and dropped, since Fathom tags carry one.
func Load(version string) (*chart.Chart, error) {
	v, err := NormalizeVersion(version)
	if err != nil {
		return nil, err
	}
	files, err := bufferedFiles()
	if err != nil {
		return nil, err
	}
	c, err := loader.LoadFiles(files)
	if err != nil {
		return nil, fmt.Errorf("load embedded chart: %w", err)
	}
	c.Metadata.Version = v
	c.Metadata.AppVersion = v
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate embedded chart: %w", err)
	}
	return c, nil
}

// NormalizeVersion strips a leading "v" and checks the rest is strict SemVer.
func NormalizeVersion(version string) (string, error) {
	v := strings.TrimPrefix(version, "v")
	if _, err := semver.StrictNewVersion(v); err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidVersion, version)
	}
	return v, nil
}

// ReadFile returns one chart-relative file from the embedded chart.
func ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(chartFS, path.Join(chartDir, name))
}

func bufferedFiles() ([]*archive.BufferedFile, error) {
	rules, err := helmIgnoreRules()
	if err != nil {
		return nil, err
	}
	var files []*archive.BufferedFile
	err = fs.WalkDir(chartFS, chartDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, chartDir), "/")
		if rel == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if rules.Ignore(rel, info) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(chartFS, p)
		if err != nil {
			return err
		}
		files = append(files, &archive.BufferedFile{Name: rel, ModTime: time.Time{}, Data: data})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read embedded chart: %w", err)
	}
	return files, nil
}

func helmIgnoreRules() (*ignore.Rules, error) {
	rules := ignore.Empty()
	raw, err := fs.ReadFile(chartFS, path.Join(chartDir, ignore.HelmIgnore))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if rules, err = ignore.Parse(bytes.NewReader(raw)); err != nil {
			return nil, fmt.Errorf("parse %s: %w", ignore.HelmIgnore, err)
		}
	}
	rules.AddDefaults()
	return rules, nil
}
