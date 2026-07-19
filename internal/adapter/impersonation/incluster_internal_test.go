/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package impersonation

import (
	"fmt"
	"io/fs"
	"testing"

	"k8s.io/client-go/rest"
)

// TestInClusterFromConfigErr locks the SKA-162 detection semantics: the probe
// must match client-go's own signal and fail closed on anything but the
// definitive "not in a pod" sentinel, so a broken in-cluster pod (env vars set
// but the ServiceAccount token unreadable) is still gated rather than falling
// open to the operator identity.
func TestInClusterFromConfigErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "ErrNotInCluster is definitively out-of-cluster",
			err:  rest.ErrNotInCluster,
			want: false,
		},
		{
			name: "wrapped ErrNotInCluster is still out-of-cluster",
			err:  fmt.Errorf("load config: %w", rest.ErrNotInCluster),
			want: false,
		},
		{
			name: "nil (healthy in-cluster config) is in-cluster",
			err:  nil,
			want: true,
		},
		{
			name: "env vars set but token unreadable fails closed as in-cluster",
			err:  fmt.Errorf("open token: %w", fs.ErrNotExist),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inClusterFromConfigErr(tt.err); got != tt.want {
				t.Errorf("inClusterFromConfigErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
