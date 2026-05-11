/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

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
	// ContractVersion at the time this test was written is "0.2.0", which
	// puts us in the pre-stable regime: minor bumps are breaking. The cases
	// below are written against that fact and will need to be reconsidered
	// when the contract reaches 1.0.0.
	if ContractVersion != "0.2.0" {
		t.Logf("note: ContractVersion is %q; some pre-stable cases below may no longer apply", ContractVersion)
	}

	tests := []struct {
		name        string
		reported    string
		wantErr     bool
		errContains string
	}{
		{name: "exact match", reported: "0.2.0"},
		{name: "same major+minor, newer patch", reported: "0.2.5"},
		{name: "same major+minor, pre-release", reported: "0.2.0-rc.1"},
		{
			name:        "pre-1.0 minor bump down",
			reported:    "0.1.0",
			wantErr:     true,
			errContains: "pre-1.0 minor version mismatch",
		},
		{
			name:        "pre-1.0 minor bump up",
			reported:    "0.3.0",
			wantErr:     true,
			errContains: "pre-1.0 minor version mismatch",
		},
		{
			name:        "major bump",
			reported:    "1.0.0",
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
