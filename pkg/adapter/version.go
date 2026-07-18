/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package adapter

import (
	"fmt"
	"regexp"
	"strconv"
)

// ContractVersion is the SemVer of the adapter contract this build of Fathom
// understands. Adapters should embed this constant in their ContractVersion()
// method so a contract bump is visible at adapter-build time.
//
// At 1.0.0 the contract is stable: [EnsureCompatible] accepts any adapter
// built against the same major and an equal-or-older minor, so additive
// contract growth (new Request fields, new optional interfaces) no longer
// rejects adapters built against an older 1.x. The reverse is still rejected:
// an adapter targeting a newer minor may rely on surface this host does not
// provide. A major bump remains a breaking rebuild.
const ContractVersion = "1.0.0"

// semverPattern matches MAJOR.MINOR.PATCH with an optional pre-release or
// build suffix. We do not interpret the suffix — only major/minor/patch
// are used for compatibility decisions.
var semverPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$`)

// EnsureCompatible reports whether an adapter's reported contract version is
// compatible with this build of Fathom. It returns nil on compatibility, or
// an actionable error naming both versions and the rule that was violated.
//
// Compatibility rules:
//   - 1.0.0 or later: same major version, and the adapter's minor must not
//     exceed the host's — a newer-minor adapter may depend on contract
//     surface (Request fields, optional interfaces) this host lacks.
//   - 0.x.y (pre-stable): same major AND same minor are required;
//     a minor bump is treated as breaking.
func EnsureCompatible(reported string) error {
	return ensureCompatible(ContractVersion, reported)
}

// ensureCompatible is the injectable-host core of [EnsureCompatible], split
// out so the pre-1.0 rules stay testable now that the shipped host is 1.x.
func ensureCompatible(hostVersion, reported string) error {
	host, err := parseVersion(hostVersion)
	if err != nil {
		return fmt.Errorf("internal: fathom contract version %q is invalid: %w", hostVersion, err)
	}
	got, err := parseVersion(reported)
	if err != nil {
		return fmt.Errorf("adapter reports invalid contract version %q: %w", reported, err)
	}
	if host.major != got.major {
		return fmt.Errorf(
			"adapter contract version %s is incompatible with fathom contract version %s: major version mismatch (rebuild adapter against fathom's contract)",
			reported, hostVersion,
		)
	}
	if host.major == 0 && host.minor != got.minor {
		return fmt.Errorf(
			"adapter contract version %s is incompatible with fathom contract version %s: pre-1.0 minor version mismatch (rebuild adapter against fathom's contract)",
			reported, hostVersion,
		)
	}
	if got.minor > host.minor {
		return fmt.Errorf(
			"adapter contract version %s is newer than fathom contract version %s: the adapter may rely on contract surface this host lacks (upgrade the operator or rebuild the adapter)",
			reported, hostVersion,
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
