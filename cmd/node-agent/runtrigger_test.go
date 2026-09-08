/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/skaphos/fathom/internal/nodecert"
)

// TestParseConfigReadsRunTrigger locks the wire contract for on-demand runs:
// the operator hands the token to the agent through nodecert.EnvRunTrigger
// (downward API from the DaemonSet template), and an unset variable yields
// an empty trigger so routine ticks never look like a forced run.
func TestParseConfigReadsRunTrigger(t *testing.T) {
	args := []string{"--check-name", "nc", "--check-namespace", "ns", "--node-name", "n1"}

	t.Setenv(nodecert.EnvRunTrigger, "")
	cfg, err := parseConfig(args)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.trigger != "" {
		t.Fatalf("trigger = %q, want empty when %s is unset", cfg.trigger, nodecert.EnvRunTrigger)
	}

	t.Setenv(nodecert.EnvRunTrigger, "2026-09-07T18:04:05Z-7f3a1c")
	cfg, err = parseConfig(args)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.trigger != "2026-09-07T18:04:05Z-7f3a1c" {
		t.Fatalf("trigger = %q, want the env value", cfg.trigger)
	}
}

// TestScanAndPublishStampsTrigger proves the token reaches the report the
// operator reads back, which is what completes a run-now trigger per node.
func TestScanAndPublishStampsTrigger(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "apiserver.crt")
	writeCert(t, certPath, time.Now().Add(400*24*time.Hour))

	kube := fake.NewSimpleClientset()
	cfg := config{
		checkName:      "nc",
		checkNamespace: "fathom-system",
		nodeName:       "node-1",
		configMapName:  nodecert.NodeReportConfigMapName("nc", "node-1"),
		paths:          []string{certPath},
		thresholds:     nodecert.Thresholds{WarnDays: 30, CriticalDays: 7},
		timeout:        5 * time.Second,
		trigger:        "tok-9",
	}
	if _, err := scanAndPublish(context.Background(), kube, cfg, time.Now()); err != nil {
		t.Fatalf("scanAndPublish: %v", err)
	}
	cm, err := kube.CoreV1().ConfigMaps("fathom-system").Get(context.Background(), cfg.configMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	decoded, err := nodecert.DecodeReport(cm.Data[nodecert.ConfigMapReportKey])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Trigger != "tok-9" {
		t.Fatalf("report trigger = %q, want tok-9", decoded.Trigger)
	}
}
