/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// This file is an in-package (white-box) test — package `adapter`, not the
// external `adapter_test` used for the public API in adapter_test.go — because
// it exercises unexported internals: the semver parser (parseVersion) and the
// version type. See the test-package convention in AGENTS.md.

package adapter

import (
	"strings"
	"testing"
)

func TestContractVersionParses(t *testing.T) {
	if _, err := parseVersion(ContractVersion); err != nil {
		t.Fatalf("ContractVersion %q must parse: %v", ContractVersion, err)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    version
		wantErr bool
	}{
		{name: "simple", input: "1.2.3", want: version{1, 2, 3}},
		{name: "zero", input: "0.0.0", want: version{0, 0, 0}},
		{name: "large", input: "10.20.30", want: version{10, 20, 30}},
		{name: "with pre-release", input: "1.2.3-alpha.1", want: version{1, 2, 3}},
		{name: "with build metadata", input: "1.2.3+build.42", want: version{1, 2, 3}},
		{name: "empty", input: "", wantErr: true},
		{name: "not semver", input: "v1.2.3", wantErr: true},
		{name: "missing patch", input: "1.2", wantErr: true},
		{name: "trailing dot", input: "1.2.3.", wantErr: true},
		{name: "negative", input: "-1.2.3", wantErr: true},
		{name: "non-numeric", input: "a.b.c", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseVersion(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseVersion(%q) = %v, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVersion(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parseVersion(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestEnsureCompatible(t *testing.T) {
	// ContractVersion is "1.0.0": the stable regime, where only the major
	// component must match. Minor and patch drift in either direction is
	// additive-compatible. The pre-1.0 minor-breaking rule remains covered
	// because adapters may still report 0.x versions.
	if ContractVersion != "1.0.0" {
		t.Logf("note: ContractVersion is %q; some stable-regime cases below may no longer apply", ContractVersion)
	}

	tests := []struct {
		name        string
		reported    string
		wantErr     bool
		errContains string
	}{
		{name: "exact match", reported: "1.0.0"},
		{name: "same major, newer minor", reported: "1.5.2"},
		{name: "same major, newer patch", reported: "1.0.7"},
		{name: "same major, pre-release", reported: "1.0.0-rc.1"},
		{
			name:        "pre-stable adapter against stable host",
			reported:    "0.2.0",
			wantErr:     true,
			errContains: "major version mismatch",
		},
		{
			name:        "major bump",
			reported:    "2.0.0",
			wantErr:     true,
			errContains: "major version mismatch",
		},
		{
			name:        "empty",
			reported:    "",
			wantErr:     true,
			errContains: "invalid contract version",
		},
		{
			name:        "garbage",
			reported:    "not-a-version",
			wantErr:     true,
			errContains: "invalid contract version",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := EnsureCompatible(tc.reported)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("EnsureCompatible(%q) unexpected error: %v", tc.reported, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("EnsureCompatible(%q) = nil, want error", tc.reported)
			}
			if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("EnsureCompatible(%q) error %q does not contain %q",
					tc.reported, err.Error(), tc.errContains)
			}
		})
	}
}
