/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
)

func TestParseConfigHealthMode(t *testing.T) {
	base := []string{"--check-name", "nh", "--check-namespace", "fathom-system", "--node-name", "node-1"}

	t.Run("requires --checks", func(t *testing.T) {
		if _, err := parseConfig(append(base, "--mode", "health")); err == nil || !strings.Contains(err.Error(), "--checks is required") {
			t.Fatalf("expected --checks error, got %v", err)
		}
	})

	t.Run("rejects malformed --checks", func(t *testing.T) {
		if _, err := parseConfig(append(base, "--mode", "health", "--checks", "{nope")); err == nil || !strings.Contains(err.Error(), "--checks:") {
			t.Fatalf("expected decode error, got %v", err)
		}
	})

	t.Run("accepts an empty item list (NodeCondition-only spec)", func(t *testing.T) {
		cfg, err := parseConfig(append(base, "--mode", "health", "--checks", "[]"))
		if err != nil {
			t.Fatalf("an empty agent-side list is a valid configuration: %v", err)
		}
		if len(cfg.healthItems) != 0 {
			t.Fatalf("healthItems = %+v, want none", cfg.healthItems)
		}
	})

	t.Run("rejects an unknown mode", func(t *testing.T) {
		if _, err := parseConfig(append(base, "--mode", "bogus")); err == nil || !strings.Contains(err.Error(), "--mode must be") {
			t.Fatalf("expected mode error, got %v", err)
		}
	})

	t.Run("decodes items and derives the kind-qualified ConfigMap name", func(t *testing.T) {
		items, _ := nodehealth.EncodeItems([]nodehealth.Item{{Type: nodehealth.TypeDiskHeadroom, Path: "/var/log", WarnPercentFree: 20, CriticalPercentFree: 10}})
		cfg, err := parseConfig(append(base, "--mode", "health", "--checks", items))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.mode != modeHealth || len(cfg.healthItems) != 1 || cfg.healthItems[0].Path != "/var/log" {
			t.Fatalf("cfg = %+v", cfg)
		}
		if cfg.configMapName != nodehealth.ReportConfigMapName("nh", "node-1") {
			t.Fatalf("configMapName = %q, want the nodehealth canonical name", cfg.configMapName)
		}
		if cfg.configMapName == nodecert.NodeReportConfigMapName("nh", "node-1") {
			t.Fatal("health mode must not share the certificates-mode ConfigMap name")
		}
		if cfg.kubeletHealthzURL != nodehealth.DefaultKubeletHealthzURL {
			t.Fatalf("kubeletHealthzURL = %q", cfg.kubeletHealthzURL)
		}
	})

	t.Run("certificates mode is the default and ignores --checks", func(t *testing.T) {
		cfg, err := parseConfig(base)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.mode != modeCertificates || cfg.configMapName != nodecert.NodeReportConfigMapName("nh", "node-1") {
			t.Fatalf("cfg = %+v", cfg)
		}
	})
}

// TestScanAndPublishHealth drives a full health pass through the real
// nodehealth engine — a real directory for headroom, an httptest kubelet, and a
// real unix socket for the runtime probe — and asserts the report ConfigMap
// carries the NodeHealthCheck source kind, the node-name authenticity
// annotation, and a decodable payload, and that the per-check gauges follow.
func TestScanAndPublishHealth(t *testing.T) {
	dir := t.TempDir()
	kubelet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer kubelet.Close()
	// macOS caps unix socket paths at 104 bytes; keep it short.
	sock := filepath.Join(dir, "r.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	kube := fake.NewSimpleClientset()
	cfg := config{
		mode:           modeHealth,
		checkName:      "nh",
		checkNamespace: "fathom-system",
		nodeName:       "node-1",
		configMapName:  nodehealth.ReportConfigMapName("nh", "node-1"),
		healthItems: []nodehealth.Item{
			{Type: nodehealth.TypeDiskHeadroom, Path: dir, WarnPercentFree: 0, CriticalPercentFree: 0},
			{Type: nodehealth.TypeInodeHeadroom, Path: dir, WarnPercentFree: 0, CriticalPercentFree: 0},
			{Type: nodehealth.TypeKubeletHealthz},
			{Type: nodehealth.TypeContainerRuntime, SocketPath: sock},
			{Type: nodehealth.TypeNodeCondition}, // operator-side; must not appear in the report
		},
		kubeletHealthzURL: kubelet.URL + "/healthz",
		timeout:           5 * time.Second,
		trigger:           "tok-9",
	}

	report, err := scanAndPublishHealth(context.Background(), kube, cfg, time.Now())
	if err != nil {
		t.Fatalf("scanAndPublishHealth: %v", err)
	}
	if report.Aggregate != nodehealth.OutcomePass {
		t.Fatalf("aggregate = %s; checks: %+v", report.Aggregate, report.Checks)
	}
	if len(report.Checks) != 4 {
		t.Fatalf("got %d checks, want 4 (NodeCondition is operator-side): %+v", len(report.Checks), report.Checks)
	}
	for _, c := range report.Checks {
		if c.Type == nodehealth.TypeNodeCondition {
			t.Fatal("NodeCondition must not be evaluated by the agent")
		}
	}
	if report.Trigger != "tok-9" || report.Node != "node-1" || report.CheckName != "nh" {
		t.Fatalf("report header = %+v", report)
	}

	cm, err := kube.CoreV1().ConfigMaps("fathom-system").Get(context.Background(), cfg.configMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get ConfigMap: %v", err)
	}
	if cm.Labels[nodecert.LabelSourceKind] != nodehealth.KindNodeHealthCheck {
		t.Fatalf("source-kind label = %q, want %q", cm.Labels[nodecert.LabelSourceKind], nodehealth.KindNodeHealthCheck)
	}
	if cm.Labels[nodecert.LabelManagedBy] != nodecert.ManagedByValue || cm.Labels[nodecert.LabelSourceName] != "nh" {
		t.Fatalf("labels = %v", cm.Labels)
	}
	if cm.Annotations[nodecert.AnnotationNodeName] != "node-1" {
		t.Fatalf("node-name annotation = %q", cm.Annotations[nodecert.AnnotationNodeName])
	}
	decoded, err := nodehealth.DecodeReport(cm.Data[nodecert.ConfigMapReportKey])
	if err != nil {
		t.Fatalf("decode published report: %v", err)
	}
	if nodehealth.VerifyReportBinding(cm.Name, cm.Annotations[nodecert.AnnotationNodeName], "nh", decoded) != nodecert.ReportAccepted {
		t.Fatal("published report does not satisfy its own authenticity bindings")
	}

	// Gauges: one-hot per (type, path) with Pass=1, and a free-percent series
	// for each headroom result.
	for _, c := range report.Checks {
		got := testutil.ToFloat64(metrics.NodeHealthCheckResult.WithLabelValues("node-1", c.Type, c.Path, "Pass"))
		if got != 1 {
			t.Errorf("%s/%s Pass gauge = %v, want 1", c.Type, c.Path, got)
		}
		if fail := testutil.ToFloat64(metrics.NodeHealthCheckResult.WithLabelValues("node-1", c.Type, c.Path, "Fail")); fail != 0 {
			t.Errorf("%s/%s Fail gauge = %v, want 0", c.Type, c.Path, fail)
		}
	}
	if pct := testutil.ToFloat64(metrics.NodeHealthFilesystemFreePercent.WithLabelValues("node-1", dir, "bytes")); pct <= 0 || pct > 100 {
		t.Errorf("bytes free percent gauge = %v", pct)
	}
	if pct := testutil.ToFloat64(metrics.NodeHealthFilesystemFreePercent.WithLabelValues("node-1", dir, "inodes")); pct <= 0 || pct > 100 {
		t.Errorf("inodes free percent gauge = %v", pct)
	}

	// A second pass updates in place (no duplicate ConfigMap) and a dropped
	// item's series disappears.
	cfg.healthItems = cfg.healthItems[:1]
	if _, err := scanAndPublishHealth(context.Background(), kube, cfg, time.Now()); err != nil {
		t.Fatal(err)
	}
	list, _ := kube.CoreV1().ConfigMaps("fathom-system").List(context.Background(), metav1.ListOptions{})
	if len(list.Items) != 1 {
		t.Fatalf("expected exactly one report ConfigMap, got %d", len(list.Items))
	}
	if n := testutil.CollectAndCount(metrics.NodeHealthCheckResult); n != 6 {
		t.Errorf("after the reset only the surviving item's 6 one-hot series should remain, got %d", n)
	}
}

// TestScanAndPublishHealthFailsOpenOnKubeletDown pins that an unreachable
// kubelet is reported as Fail through the agent path, not swallowed.
func TestScanAndPublishHealthFailsOpenOnKubeletDown(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	kube := fake.NewSimpleClientset()
	cfg := config{
		mode: modeHealth, checkName: "nh", checkNamespace: "ns", nodeName: "node-1",
		configMapName:     nodehealth.ReportConfigMapName("nh", "node-1"),
		healthItems:       []nodehealth.Item{{Type: nodehealth.TypeKubeletHealthz}},
		kubeletHealthzURL: "http://" + addr + "/healthz",
		timeout:           2 * time.Second,
	}
	report, err := scanAndPublishHealth(context.Background(), kube, cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Aggregate != nodehealth.OutcomeFail || len(report.Checks) != 1 || !strings.Contains(report.Checks[0].Summary, "unreachable") {
		t.Fatalf("report = %+v", report)
	}
}

// TestScanAndPublishHealthWithNoAgentItems pins the NodeCondition-only case:
// the agent has nothing to evaluate, but it must still publish a report so
// the node counts toward coverage and the operator can grade its conditions.
func TestScanAndPublishHealthWithNoAgentItems(t *testing.T) {
	kube := fake.NewSimpleClientset()
	cfg := config{
		mode: modeHealth, checkName: "nh", checkNamespace: "ns", nodeName: "node-1",
		configMapName: nodehealth.ReportConfigMapName("nh", "node-1"),
		timeout:       2 * time.Second,
	}
	report, err := scanAndPublishHealth(context.Background(), kube, cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Checks) != 0 || report.Aggregate != nodehealth.OutcomeSkipped {
		t.Fatalf("report = %+v, want no checks and Skipped", report)
	}
	if _, err := kube.CoreV1().ConfigMaps("ns").Get(context.Background(), cfg.configMapName, metav1.GetOptions{}); err != nil {
		t.Fatalf("report ConfigMap must still be published: %v", err)
	}
}

// TestRunFailsWhenMetricsPortIsTaken pins that a metrics bind failure is
// fatal. On the host network the metrics port is a host port, and the
// documented collision behaviour — the second agent crashloops, AgentReady
// goes False — depends on the process exiting rather than logging and
// carrying on.
func TestRunFailsWhenMetricsPortIsTaken(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	kube := fake.NewSimpleClientset()
	cfg := config{
		mode: modeHealth, checkName: "nh", checkNamespace: "ns", nodeName: "node-1",
		configMapName: nodehealth.ReportConfigMapName("nh", "node-1"),
		metricsAddr:   l.Addr().String(),
		interval:      time.Hour,
		timeout:       time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = run(ctx, kube, cfg)
	if err == nil || !strings.Contains(err.Error(), "metrics server") {
		t.Fatalf("run must fail when the metrics port is taken, got %v", err)
	}
}
