/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package adapter

import (
	"fmt"
	"regexp"
	"strconv"
)

// ContractVersion is the SemVer of the adapter contract this build of Beacon
// understands. Adapters should embed this constant in their ContractVersion()
// method so a contract bump is visible at adapter-build time.
//
// Pre-1.0: minor bumps are treated as breaking by [EnsureCompatible].
//
// 0.2.0 added [Request.ProbeImage], which adapters that launch probe pods may
// consult as the operator-supplied default image. Adapters built against
// 0.1.0 cannot observe that field and so are rejected at registration time.
const ContractVersion = "0.2.0"

// semverPattern matches MAJOR.MINOR.PATCH with an optional pre-release or
// build suffix. We do not interpret the suffix — only major/minor/patch
// are used for compatibility decisions.
var semverPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$`)

// EnsureCompatible reports whether an adapter's reported contract version is
// compatible with this build of Beacon. It returns nil on compatibility, or
// an actionable error naming both versions and the rule that was violated.
//
// Compatibility rules:
//   - 1.0.0 or later: same major version is compatible.
//   - 0.x.y (pre-stable): same major AND same minor are required;
//     a minor bump is treated as breaking.
func EnsureCompatible(reported string) error {
	host, err := parseVersion(ContractVersion)
	if err != nil {
		return fmt.Errorf("internal: beacon contract version %q is invalid: %w", ContractVersion, err)
	}
	got, err := parseVersion(reported)
	if err != nil {
		return fmt.Errorf("adapter reports invalid contract version %q: %w", reported, err)
	}
	if host.major != got.major {
		return fmt.Errorf(
			"adapter contract version %s is incompatible with beacon contract version %s: major version mismatch (rebuild adapter against beacon's contract)",
			reported, ContractVersion,
		)
	}
	if host.major == 0 && host.minor != got.minor {
		return fmt.Errorf(
			"adapter contract version %s is incompatible with beacon contract version %s: pre-1.0 minor version mismatch (rebuild adapter against beacon's contract)",
			reported, ContractVersion,
		)
	}
	return nil
}

type version struct {
	major, minor, patch int
}

func parseVersion(s string) (version, error) {
	if s == "" {
		return version{}, fmt.Errorf("empty version")
	}
	m := semverPattern.FindStringSubmatch(s)
	if m == nil {
		return version{}, fmt.Errorf("not a SemVer MAJOR.MINOR.PATCH: %q", s)
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return version{}, fmt.Errorf("major component: %w", err)
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return version{}, fmt.Errorf("minor component: %w", err)
	}
	patch, err := strconv.Atoi(m[3])
	if err != nil {
		return version{}, fmt.Errorf("patch component: %w", err)
	}
	return version{major: major, minor: minor, patch: patch}, nil
}
