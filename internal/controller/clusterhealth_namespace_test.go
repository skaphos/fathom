/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func TestClusterHealthCoversNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		namespaces         []string
		excludedNamespaces []string
		ns                 string
		want               bool
	}{
		{name: "open includes any", ns: "tenant-a", want: true},
		{name: "open includes another", ns: "kube-system", want: true},

		{name: "allowlist hit", namespaces: []string{"tenant-a", "tenant-b"}, ns: "tenant-a", want: true},
		{name: "allowlist miss", namespaces: []string{"tenant-a"}, ns: "tenant-b", want: false},

		{name: "denylist hit excludes", excludedNamespaces: []string{"scratch", "tmp"}, ns: "scratch", want: false},
		{name: "denylist miss includes", excludedNamespaces: []string{"scratch"}, ns: "tenant-a", want: true},

		// Allow is definitive: denylist is ignored when allowlist is set.
		{
			name:               "allow wins over exclude on listed ns",
			namespaces:         []string{"tenant-a"},
			excludedNamespaces: []string{"tenant-a"},
			ns:                 "tenant-a",
			want:               true,
		},
		{
			name:               "allow wins over exclude on unlisted ns",
			namespaces:         []string{"tenant-a"},
			excludedNamespaces: []string{"tenant-b"},
			ns:                 "tenant-b",
			want:               false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ch := &fathomv1alpha1.ClusterHealth{
				Spec: fathomv1alpha1.ClusterHealthSpec{
					Namespaces:         tc.namespaces,
					ExcludedNamespaces: tc.excludedNamespaces,
				},
			}
			if got := clusterHealthCoversNamespace(ch, tc.ns); got != tc.want {
				t.Fatalf("clusterHealthCoversNamespace(%q) = %v, want %v", tc.ns, got, tc.want)
			}
		})
	}
}
