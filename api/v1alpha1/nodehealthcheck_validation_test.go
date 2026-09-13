/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package v1alpha1_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// validNodeHealthCheck is the minimal accepted object from the admission
// contract: one item of the cheapest type, every other field left to its
// default.
func validNodeHealthCheck() *fathomv1alpha1.NodeHealthCheck {
	return &fathomv1alpha1.NodeHealthCheck{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "nodehealthcheck-", Namespace: "default"},
		Spec: fathomv1alpha1.NodeHealthCheckSpec{
			Checks: []fathomv1alpha1.NodeHealthCheckItem{{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"}},
		},
	}
}

func items(i ...fathomv1alpha1.NodeHealthCheckItem) []fathomv1alpha1.NodeHealthCheckItem { return i }

// TestNodeHealthCheckAdmission is the admission matrix for the check item's
// discriminated union and the spec-level rules (#269). Each rejection also
// asserts the message names the offending field, because a rejection an
// operator cannot act on is barely better than a silent one.
func TestNodeHealthCheckAdmission(t *testing.T) {
	requireAPIServer(t)
	tests := []struct {
		name       string
		mutate     func(*fathomv1alpha1.NodeHealthCheck)
		wantReject bool
		wantInMsg  string
	}{
		// --- shape of the check list ----------------------------------------
		{name: "1 minimal object", mutate: func(*fathomv1alpha1.NodeHealthCheck) {}},
		{
			name:       "2 empty checks",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks = []fathomv1alpha1.NodeHealthCheckItem{} },
			wantReject: true, wantInMsg: "checks",
		},
		{
			name:       "3 checks omitted",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks = nil },
			wantReject: true, wantInMsg: "checks",
		},
		{
			name: "4 seventeen checks",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = make([]fathomv1alpha1.NodeHealthCheckItem, 17)
				for i := range c.Spec.Checks {
					c.Spec.Checks[i] = fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/log/" + strings.Repeat("x", i+1)}
				}
			},
			wantReject: true, wantInMsg: "checks",
		},
		{
			name: "5 every type at once",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"},
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckInodeHeadroom, Path: "/var/lib/kubelet"},
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition},
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz},
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime},
				)
			},
		},

		// --- the discriminated union -----------------------------------------
		{
			name:       "6 unknown type",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks[0].Type = "Uptime" },
			wantReject: true, wantInMsg: "type",
		},
		{
			name: "7 headroom without a path",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckInodeHeadroom})
			},
			wantReject: true, wantInMsg: "path is required",
		},
		{
			name: "8 kubelet with a path",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz, Path: "/var/lib/kubelet"})
			},
			wantReject: true, wantInMsg: "must be omitted",
		},
		{
			name: "9 thresholds on a non-headroom type",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition, WarnPercentFree: ptr.To[int32](20)})
			},
			wantReject: true, wantInMsg: "apply only to DiskHeadroom",
		},
		{
			name: "10 warn below critical",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks[0].WarnPercentFree = ptr.To[int32](5)
				c.Spec.Checks[0].CriticalPercentFree = ptr.To[int32](10)
			},
			wantReject: true, wantInMsg: "warnPercentFree must be greater than or equal to criticalPercentFree",
		},
		{
			name: "11 warn equal to critical is legal",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks[0].WarnPercentFree = ptr.To[int32](10)
				c.Spec.Checks[0].CriticalPercentFree = ptr.To[int32](10)
			},
		},
		{
			name:       "12 percent above 100",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks[0].WarnPercentFree = ptr.To[int32](101) },
			wantReject: true, wantInMsg: "warnPercentFree",
		},
		{
			name:   "13 zero thresholds are legal (never warn)",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks[0].WarnPercentFree = ptr.To[int32](0) },
		},
		{
			name: "14 conditions on a headroom type",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks[0].Conditions = []string{"Ready"}
			},
			wantReject: true, wantInMsg: "conditions applies only to NodeCondition",
		},
		{
			name: "15 conditions on NodeCondition",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition, Conditions: []string{"Ready", "NetworkUnavailable"}})
			},
		},
		{
			name: "16 seventeen conditions",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				conds := make([]string, 17)
				for i := range conds {
					conds[i] = "C" + strings.Repeat("x", i+1)
				}
				c.Spec.Checks = items(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition, Conditions: conds})
			},
			wantReject: true, wantInMsg: "conditions",
		},
		{
			name: "17 socketPath on a headroom type",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks[0].SocketPath = "/run/containerd/containerd.sock"
			},
			wantReject: true, wantInMsg: "socketPath applies only to ContainerRuntime",
		},

		// --- path allowlist ---------------------------------------------------
		{
			name:       "18 path outside the allowlist",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks[0].Path = "/home" },
			wantReject: true, wantInMsg: "allowed prefix",
		},
		{
			name:       "19 host root",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks[0].Path = "/" },
			wantReject: true, wantInMsg: "allowed prefix",
		},
		{
			name:       "20 traversal inside an allowed prefix",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks[0].Path = "/var/lib/kubelet/../../etc/shadow" },
			wantReject: true, wantInMsg: "traversal-free",
		},
		{
			name:       "21 prefix as a string prefix only",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks[0].Path = "/var/lib/kubelet-evil" },
			wantReject: true, wantInMsg: "allowed prefix",
		},
		{
			name:   "22 subdirectory of an allowed prefix",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Checks[0].Path = "/var/lib/kubelet/pods" },
		},

		// --- socket allowlist -------------------------------------------------
		{
			name: "23 socketPath not a .sock",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime, SocketPath: "/run/containerd/containerd"})
			},
			wantReject: true, wantInMsg: "socketPath must be",
		},
		{
			name: "24 socketPath outside the runtime directories",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime, SocketPath: "/var/run/docker.sock"})
			},
			wantReject: true, wantInMsg: "socketPath must be",
		},
		{
			name: "25 cri-o socket",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime, SocketPath: "/run/crio/crio.sock"})
			},
		},

		// --- uniqueness -------------------------------------------------------
		{
			name: "26 duplicate type and path",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"},
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"},
				)
			},
			wantReject: true, wantInMsg: "unique",
		},
		{
			name: "27 same type, different paths",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"},
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/log"},
				)
			},
		},
		{
			name: "28 duplicate pathless type",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Checks = items(
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz},
					fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz},
				)
			},
			wantReject: true, wantInMsg: "unique",
		},

		// --- cadence ------------------------------------------------------------
		{
			name:       "29 interval below floor",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Interval = duration(time.Millisecond) },
			wantReject: true, wantInMsg: "interval",
		},
		{
			name:       "30 timeout below floor",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.Timeout = duration(100 * time.Millisecond) },
			wantReject: true, wantInMsg: "timeout",
		},
		{
			name: "31 timeout exceeds interval",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Interval = duration(time.Minute)
				c.Spec.Timeout = duration(5 * time.Minute)
			},
			wantReject: true, wantInMsg: "timeout",
		},
		{
			name: "32 timeout equal to interval is legal",
			mutate: func(c *fathomv1alpha1.NodeHealthCheck) {
				c.Spec.Interval = duration(10 * time.Second)
				c.Spec.Timeout = duration(10 * time.Second)
			},
		},
		{
			name:       "33 historyLimit below one",
			mutate:     func(c *fathomv1alpha1.NodeHealthCheck) { c.Spec.HistoryLimit = ptr.To[int32](0) },
			wantReject: true, wantInMsg: "historyLimit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validNodeHealthCheck()
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if !tt.wantReject {
				if err != nil {
					t.Fatalf("expected admission, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected rejection mentioning %q, but the object was admitted", tt.wantInMsg)
			}
			if !strings.Contains(err.Error(), tt.wantInMsg) {
				t.Fatalf("rejection does not name the field: want substring %q in %q", tt.wantInMsg, err.Error())
			}
		})
	}
}

// TestNodeHealthCheckDefaultsAtAdmission pins the schema defaults: the per-type
// thresholds deliberately have none (a default on a union field would break
// the union rule), while the spec-level fields default like every other kind.
func TestNodeHealthCheckDefaultsAtAdmission(t *testing.T) {
	requireAPIServer(t)
	obj := validNodeHealthCheck()
	if err := k8sClient.Create(context.Background(), obj); err != nil {
		t.Fatalf("create: %v", err)
	}
	if obj.Spec.Checks[0].WarnPercentFree != nil || obj.Spec.Checks[0].CriticalPercentFree != nil {
		t.Fatalf("thresholds must not be schema-defaulted (runtime default applies): %+v", obj.Spec.Checks[0])
	}
	if obj.Spec.HistoryLimit == nil || *obj.Spec.HistoryLimit != 10 {
		t.Fatalf("historyLimit default = %v, want 10", obj.Spec.HistoryLimit)
	}
	if obj.Spec.IncludeControlPlaneNodes == nil || *obj.Spec.IncludeControlPlaneNodes {
		t.Fatalf("includeControlPlaneNodes default = %v, want false", obj.Spec.IncludeControlPlaneNodes)
	}
}

// TestNodeHealthNodeResultsCapMatchesSchema pins MaxNodeHealthNodeResults to
// the MaxItems marker on status.nodeResults, the same lockstep ClusterHealth
// keeps for its children cap.
func TestNodeHealthNodeResultsCapMatchesSchema(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "bases", "fathom.skaphos.io_nodehealthchecks.yaml"))
	if err != nil {
		t.Fatalf("read generated CRD: %v", err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatalf("decode CRD: %v", err)
	}
	var found bool
	for _, v := range crd.Spec.Versions {
		status, ok := v.Schema.OpenAPIV3Schema.Properties["status"]
		if !ok {
			continue
		}
		results, ok := status.Properties["nodeResults"]
		if !ok || results.MaxItems == nil {
			t.Fatalf("status.nodeResults has no maxItems in the generated schema")
		}
		found = true
		if got := int(*results.MaxItems); got != fathomv1alpha1.MaxNodeHealthNodeResults {
			t.Fatalf("status.nodeResults maxItems = %d, want MaxNodeHealthNodeResults %d", got, fathomv1alpha1.MaxNodeHealthNodeResults)
		}
	}
	if !found {
		t.Fatal("no served version carried a status schema")
	}
}
