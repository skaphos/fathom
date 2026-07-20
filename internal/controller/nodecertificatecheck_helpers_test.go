/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
)

func boolPtr(b bool) *bool { return &b }

// TestAggregateNodeReports pins the shared worst-of fold for the node-cert
// roll-up: Skipped certs are informational and no certs at all yields Skipped
// (#160 consistency).
func TestAggregateNodeReports(t *testing.T) {
	report := func(node string, outcomes ...nodecert.Outcome) nodecert.NodeReport {
		certs := make([]nodecert.CertResult, 0, len(outcomes))
		for _, o := range outcomes {
			certs = append(certs, nodecert.CertResult{Outcome: o})
		}
		return nodecert.NodeReport{Node: node, Certs: certs}
	}

	tests := []struct {
		name    string
		reports []nodecert.NodeReport
		want    fathomv1alpha1.HealthReportResult
	}{
		{"no reports", nil, fathomv1alpha1.HealthReportResultSkipped},
		{"node with no certs", []nodecert.NodeReport{report("a")}, fathomv1alpha1.HealthReportResultSkipped},
		{"single pass", []nodecert.NodeReport{report("a", nodecert.OutcomePass)}, fathomv1alpha1.HealthReportResultPass},
		{"pass plus skipped folds to pass", []nodecert.NodeReport{report("a", nodecert.OutcomePass, nodecert.OutcomeSkipped)}, fathomv1alpha1.HealthReportResultPass},
		{"all skipped", []nodecert.NodeReport{report("a", nodecert.OutcomeSkipped)}, fathomv1alpha1.HealthReportResultSkipped},
		{"pass warn fail", []nodecert.NodeReport{report("a", nodecert.OutcomePass, nodecert.OutcomeWarn), report("b", nodecert.OutcomeFail)}, fathomv1alpha1.HealthReportResultFail},
		{"error wins across nodes", []nodecert.NodeReport{report("a", nodecert.OutcomeFail), report("b", nodecert.OutcomeError)}, fathomv1alpha1.HealthReportResultError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aggregateNodeReports(tt.reports); got != tt.want {
				t.Errorf("aggregateNodeReports = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveTolerations pins the #155 hardening: control-plane tolerations are
// only ever applied when the check explicitly opts in, never as a silent default.
func TestResolveTolerations(t *testing.T) {
	custom := corev1.Toleration{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "certs", Effect: corev1.TaintEffectNoSchedule}

	hasControlPlane := func(tols []corev1.Toleration) bool {
		for _, tol := range tols {
			if tol.Key == "node-role.kubernetes.io/control-plane" {
				return true
			}
		}
		return false
	}

	tests := []struct {
		name             string
		spec             fathomv1alpha1.NodeCertificateCheckSpec
		wantCount        int
		wantControlPlane bool
	}{
		{
			name:             "default: no tolerations, no control-plane",
			spec:             fathomv1alpha1.NodeCertificateCheckSpec{},
			wantCount:        0,
			wantControlPlane: false,
		},
		{
			name:             "opt-out explicit false stays off",
			spec:             fathomv1alpha1.NodeCertificateCheckSpec{IncludeControlPlaneNodes: boolPtr(false)},
			wantCount:        0,
			wantControlPlane: false,
		},
		{
			name:             "opt-in adds the two control-plane tolerations",
			spec:             fathomv1alpha1.NodeCertificateCheckSpec{IncludeControlPlaneNodes: boolPtr(true)},
			wantCount:        2,
			wantControlPlane: true,
		},
		{
			name:             "explicit tolerations pass through without control-plane",
			spec:             fathomv1alpha1.NodeCertificateCheckSpec{Tolerations: []corev1.Toleration{custom}},
			wantCount:        1,
			wantControlPlane: false,
		},
		{
			name: "explicit tolerations plus opt-in are combined",
			spec: fathomv1alpha1.NodeCertificateCheckSpec{
				Tolerations:              []corev1.Toleration{custom},
				IncludeControlPlaneNodes: boolPtr(true),
			},
			wantCount:        3,
			wantControlPlane: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			check := &fathomv1alpha1.NodeCertificateCheck{Spec: tc.spec}
			got := resolveTolerations(check)
			if len(got) != tc.wantCount {
				t.Fatalf("toleration count = %d, want %d (%v)", len(got), tc.wantCount, got)
			}
			if hasControlPlane(got) != tc.wantControlPlane {
				t.Fatalf("control-plane toleration present = %v, want %v", hasControlPlane(got), tc.wantControlPlane)
			}
		})
	}
}

// TestResolveCertPathsFiltersDisallowed is the operator-side defense-in-depth for
// the confused-deputy guard: disallowed paths are dropped, and a spec whose paths
// are all disallowed falls back to the safe default set rather than scanning
// nothing (or the attacker's target).
func TestResolveCertPathsFiltersDisallowed(t *testing.T) {
	t.Run("drops disallowed, keeps allowed", func(t *testing.T) {
		check := &fathomv1alpha1.NodeCertificateCheck{
			Spec: fathomv1alpha1.NodeCertificateCheckSpec{
				Paths: []string{"/etc/kubernetes/pki", "/root/.ssh/id_rsa", "/var/lib/kubelet/pki"},
			},
		}
		got := resolveCertPaths(check)
		want := []string{"/etc/kubernetes/pki", "/var/lib/kubelet/pki"}
		if len(got) != len(want) {
			t.Fatalf("resolveCertPaths = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("resolveCertPaths = %v, want %v", got, want)
			}
		}
	})

	t.Run("all disallowed falls back to defaults", func(t *testing.T) {
		check := &fathomv1alpha1.NodeCertificateCheck{
			Spec: fathomv1alpha1.NodeCertificateCheckSpec{Paths: []string{"/root/.ssh", "/proc/1/environ"}},
		}
		got := resolveCertPaths(check)
		want := nodecert.DefaultCertPaths()
		if len(got) != len(want) {
			t.Fatalf("expected fallback to %d default paths, got %d (%v)", len(want), len(got), got)
		}
	})
}
