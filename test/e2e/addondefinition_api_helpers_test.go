/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import "testing"

func TestDefinitionE2EFirstIPv4(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{name: "single gateway", input: "172.18.0.1\n", want: "172.18.0.1"},
		{name: "empty entry before gateway", input: "\n172.18.0.1\n", want: "172.18.0.1"},
		{name: "IPv6 entry before IPv4 gateway", input: "fd00::1\n172.18.0.1\n", want: "172.18.0.1"},
		{name: "getent columns", input: "172.18.0.1 STREAM host.docker.internal\n172.18.0.1 DGRAM\n", want: "172.18.0.1"},
		{name: "no IPv4 gateway", input: "\nfd00::1\n<no value>\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := definitionE2EFirstIPv4(tc.input); got != tc.want {
				t.Fatalf("definitionE2EFirstIPv4(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
