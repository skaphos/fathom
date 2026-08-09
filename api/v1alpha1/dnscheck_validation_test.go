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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// The admission matrix runs against a real API server because that is the only
// thing that evaluates CEL and the structural schema. A fake client accepts
// anything, so it would assert nothing about the contract these rules exist to
// enforce.
//
// It also doubles as the CEL cost gate: the API server rejects a schema whose
// estimated validation cost exceeds the per-CRD budget at install time, so if
// the rules ever grow too expensive, TestMain fails here rather than the
// failure surfacing later against a real cluster.

var (
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestMain(m *testing.M) {
	if err := fathomv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		panic("add fathom scheme: " + err.Error())
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	if dir := firstEnvTestBinaryDir(); dir != "" {
		testEnv.BinaryAssetsDirectory = dir
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		panic("start envtest (is KUBEBUILDER_ASSETS set? run via `task test`): " + err.Error())
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		_ = testEnv.Stop()
		panic("build client: " + err.Error())
	}

	code := m.Run()
	_ = testEnv.Stop()
	os.Exit(code)
}

func firstEnvTestBinaryDir() string {
	entries, err := os.ReadDir(filepath.Join("..", "..", "bin", "k8s"))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join("..", "..", "bin", "k8s", entry.Name())
		}
	}
	return ""
}

// validDNSCheck is the minimal accepted object from the admission contract:
// one target, every other field left to its default.
func validDNSCheck() *fathomv1alpha1.DNSCheck {
	return &fathomv1alpha1.DNSCheck{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "dnscheck-", Namespace: "default"},
		Spec: fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{{Name: "kubernetes.default.svc.cluster.local"}},
		},
	}
}

func duration(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }

func targets(t ...fathomv1alpha1.DNSTarget) []fathomv1alpha1.DNSTarget { return t }

// TestDNSCheckAdmission walks every row of
// specs/005-dnscheck-resource-contract/contracts/dnscheck-admission.md.
// Each rejection also asserts the message names the offending field, because a
// rejection an operator cannot act on is barely better than a silent one.
func TestDNSCheckAdmission(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*fathomv1alpha1.DNSCheck)
		wantReject bool
		wantInMsg  string
	}{
		// --- shape of the target list -----------------------------------
		{name: "1 minimal object", mutate: func(*fathomv1alpha1.DNSCheck) {}},
		{
			name:       "2 empty targets",
			mutate:     func(c *fathomv1alpha1.DNSCheck) { c.Spec.Targets = []fathomv1alpha1.DNSTarget{} },
			wantReject: true, wantInMsg: "targets",
		},
		{
			name:       "3 targets omitted",
			mutate:     func(c *fathomv1alpha1.DNSCheck) { c.Spec.Targets = nil },
			wantReject: true, wantInMsg: "targets",
		},
		{
			name: "4 seventeen targets",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = make([]fathomv1alpha1.DNSTarget, 17)
				for i := range c.Spec.Targets {
					c.Spec.Targets[i] = fathomv1alpha1.DNSTarget{Name: "h" + strings.Repeat("x", i) + ".example.com"}
				}
			},
			wantReject: true, wantInMsg: "targets",
		},
		{
			name: "5 four resolvers",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}}
			},
			wantReject: true, wantInMsg: "resolvers",
		},

		// --- cadence ------------------------------------------------------
		{
			name:       "6 interval below floor",
			mutate:     func(c *fathomv1alpha1.DNSCheck) { c.Spec.Interval = duration(time.Millisecond) },
			wantReject: true, wantInMsg: "interval",
		},
		{
			name:       "7 timeout below floor",
			mutate:     func(c *fathomv1alpha1.DNSCheck) { c.Spec.Timeout = duration(100 * time.Millisecond) },
			wantReject: true, wantInMsg: "timeout",
		},
		{
			name: "8 timeout exceeds interval",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Interval = duration(time.Minute)
				c.Spec.Timeout = duration(5 * time.Minute)
			},
			wantReject: true, wantInMsg: "timeout",
		},
		{
			name: "9 timeout equal to interval is legal",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Interval = duration(10 * time.Second)
				c.Spec.Timeout = duration(10 * time.Second)
			},
		},

		// --- record kinds ---------------------------------------------------
		{
			name: "10 unsupported record type",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "example.com", RecordType: "TXT"})
			},
			wantReject: true, wantInMsg: "recordType",
		},
		{name: "10a record type omitted defaults to Host", mutate: func(*fathomv1alpha1.DNSCheck) {}},
		{
			name: "10b explicit Host",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "example.com", RecordType: fathomv1alpha1.DNSRecordHost})
			},
		},

		// --- polarity -------------------------------------------------------
		{
			name: "11 absent with expected answers is contradictory",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "gone.example.com", Absent: true, ExpectedAnswers: []string{"1.2.3.4"}})
			},
			wantReject: true, wantInMsg: "absent",
		},
		{
			name: "12 absent alone",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "gone.example.com", Absent: true})
			},
		},

		// --- subject syntax --------------------------------------------------
		{
			name: "13 not a hostname",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "not a hostname"})
			},
			wantReject: true, wantInMsg: "name",
		},
		{
			name: "14 a URL is not a hostname",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "http://example.com/path"})
			},
			wantReject: true, wantInMsg: "name",
		},
		{
			name: "15 trailing dot accepted",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "example.com."})
			},
		},
		{
			name: "16 single label accepted",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "my-svc"})
			},
		},
		{
			name: "17 underscore SRV labels survive",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "_https._tcp.example.com.", RecordType: fathomv1alpha1.DNSRecordSRV})
			},
		},
		{
			name: "18 PTR subject must be an address",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "example.com", RecordType: fathomv1alpha1.DNSRecordPTR})
			},
			wantReject: true, wantInMsg: "PTR",
		},
		{
			name: "19 PTR subject as an address",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "10.20.30.40", RecordType: fathomv1alpha1.DNSRecordPTR})
			},
		},
		{
			// The name pattern has to admit IPv6 literals for this to be
			// expressible at all, which is why a separate colon clause stops
			// them being accepted as forward-lookup subjects.
			name: "19a PTR subject as an IPv6 address",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "2001:db8::1", RecordType: fathomv1alpha1.DNSRecordPTR})
			},
		},
		{
			name: "19b a colon-bearing non-address is not a forward subject",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "abc:def", RecordType: fathomv1alpha1.DNSRecordA})
			},
			wantReject: true, wantInMsg: "PTR",
		},
		{
			name: "20 non-PTR subject must be a name",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "10.20.30.40", RecordType: fathomv1alpha1.DNSRecordA})
			},
			wantReject: true, wantInMsg: "PTR",
		},

		// --- resolvers --------------------------------------------------------
		{
			name: "21 explicit without address",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "up", From: fathomv1alpha1.DNSResolverExplicit}}
			},
			wantReject: true, wantInMsg: "address",
		},
		{
			name: "22 address on a non-explicit resolver",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "up", From: fathomv1alpha1.DNSResolverCluster, Address: "10.0.0.10"}}
			},
			wantReject: true, wantInMsg: "address",
		},
		{
			name: "23 hostname as a resolver address",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "up", From: fathomv1alpha1.DNSResolverExplicit, Address: "dns.example.com"}}
			},
			wantReject: true, wantInMsg: "address",
		},
		{
			name: "24 bare IP address",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "up", From: fathomv1alpha1.DNSResolverExplicit, Address: "10.0.0.10"}}
			},
		},
		{
			name: "25 IP with port",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "up", From: fathomv1alpha1.DNSResolverExplicit, Address: "10.0.0.10:53"}}
			},
		},
		{
			name: "26 bracketed IPv6 with port",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "up", From: fathomv1alpha1.DNSResolverExplicit, Address: "[2001:db8::1]:53"}}
			},
		},
		{
			name: "27 duplicate resolver names",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "up"}, {Name: "up"}}
			},
			wantReject: true, wantInMsg: "up",
		},
		{
			name: "27a cluster is a reserved resolver name",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Resolvers = []fathomv1alpha1.DNSResolver{{Name: "cluster"}}
			},
			wantReject: true, wantInMsg: "reserved",
		},
		{
			name: "28 target references an undeclared resolver",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "example.com", Resolver: "nope"})
			},
			wantReject: true, wantInMsg: "resolver",
		},
		{
			name: "28a the reserved cluster name is always addressable",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "example.com", Resolver: "cluster"})
			},
		},

		// --- fields that must not exist ----------------------------------------
		{
			name: "31 history limit below one",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				zero := int32(0)
				c.Spec.HistoryLimit = &zero
			},
			wantReject: true, wantInMsg: "historyLimit",
		},
		{
			name: "32 too many expected answers",
			mutate: func(c *fathomv1alpha1.DNSCheck) {
				answers := make([]string, 17)
				for i := range answers {
					answers[i] = "10.0.0." + strings.Repeat("1", i%3+1)
				}
				c.Spec.Targets = targets(fathomv1alpha1.DNSTarget{Name: "example.com", ExpectedAnswers: answers})
			},
			wantReject: true, wantInMsg: "expectedAnswers",
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := validDNSCheck()
			tc.mutate(obj)

			err := k8sClient.Create(ctx, obj)
			if !tc.wantReject {
				if err != nil {
					t.Fatalf("expected the object to be accepted, got: %v", err)
				}
				_ = k8sClient.Delete(ctx, obj)
				return
			}
			if err == nil {
				_ = k8sClient.Delete(ctx, obj)
				t.Fatal("expected the write to be rejected, but it was accepted")
			}
			if tc.wantInMsg != "" && !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Fatalf("rejection message must name %q so an operator can act on it; got: %v", tc.wantInMsg, err)
			}
		})
	}
}

// TestDNSCheckDefaults asserts the effective values an operator gets from the
// minimal object, so the documented defaults are real rather than aspirational
// (SC-003).
func TestDNSCheckDefaults(t *testing.T) {
	ctx := context.Background()
	obj := validDNSCheck()
	if err := k8sClient.Create(ctx, obj); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = k8sClient.Delete(ctx, obj) }()

	var got fathomv1alpha1.DNSCheck
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if want := fathomv1alpha1.DNSRecordHost; got.Spec.Targets[0].RecordType != want {
		t.Errorf("recordType = %q, want %q — defaulting to A would silently mean IPv4 only", got.Spec.Targets[0].RecordType, want)
	}
	if got.Spec.Targets[0].Absent {
		t.Error("absent = true, want false")
	}
	if got.Spec.HistoryLimit == nil || *got.Spec.HistoryLimit != 10 {
		t.Errorf("historyLimit = %v, want 10", got.Spec.HistoryLimit)
	}
}

// TestDNSCheckHasNoPauseField pins FR-019. A structural schema rejects unknown
// fields for free, so this needs no rule — but the promise is part of the
// contract, and a future refactor could reintroduce the field without anyone
// noticing.
func TestDNSCheckHasNoPauseField(t *testing.T) {
	for _, field := range []string{"paused", "policy"} {
		t.Run(field, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "fathom.skaphos.io/v1alpha1",
				"kind":       "DNSCheck",
				"metadata":   map[string]any{"generateName": "dnscheck-", "namespace": "default"},
				"spec": map[string]any{
					"targets": []any{map[string]any{"name": "example.com"}},
					field:     true,
				},
			}}
			err := k8sClient.Create(context.Background(), obj, client.FieldValidation(metav1.FieldValidationStrict))
			if err == nil {
				_ = k8sClient.Delete(context.Background(), obj)
				t.Fatalf("spec.%s was accepted; this kind must not carry it", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Fatalf("rejection must name the unknown field %q; got: %v", field, err)
			}
		})
	}
}
