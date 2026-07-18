/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// node-agent is the Fathom DaemonSet binary that scans on-disk X.509
// certificates on a single node and publishes a per-node report ConfigMap that
// the NodeCertificateCheck controller rolls up into a HealthReport (SKA-519).
//
// It is intentionally minimal and least-privilege: it reads certificate files
// from read-only hostPath mounts, writes exactly one ConfigMap (its own), and
// serves a Prometheus endpoint. All scan configuration is supplied by the
// operator via flags/env, so the agent needs no read access to the
// NodeCertificateCheck API.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/nodecert"
)

type config struct {
	checkName      string
	checkNamespace string
	nodeName       string
	configMapName  string
	paths          []string
	thresholds     nodecert.Thresholds
	interval       time.Duration
	timeout        time.Duration
	metricsAddr    string
	once           bool
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		log.Fatalf("node-agent: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	restCfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("node-agent: load in-cluster config: %v", err)
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		log.Fatalf("node-agent: build client: %v", err)
	}

	if err := run(ctx, kube, cfg); err != nil {
		log.Fatalf("node-agent: %v", err)
	}
}

// run serves metrics and drives the scan loop until ctx is cancelled. With
// cfg.once it performs a single scan and returns.
func run(ctx context.Context, kube kubernetes.Interface, cfg config) error {
	srv := &http.Server{Addr: cfg.metricsAddr, Handler: metricsMux(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("node-agent: metrics server: %v", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	scanOnce := func() {
		report, err := scanAndPublish(ctx, kube, cfg, time.Now())
		if err != nil {
			log.Printf("node-agent: publish report: %v", err)
			return
		}
		log.Printf("node-agent: scanned %d certificate(s) on %s, aggregate=%s", len(report.Certs), cfg.nodeName, report.Aggregate)
	}

	scanOnce()
	if cfg.once {
		return nil
	}

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			scanOnce()
		}
	}
}

func metricsMux() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(ctrlmetrics.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	return mux
}

// scanAndPublish runs one scan, updates the expiry gauges, and upserts the
// per-node report ConfigMap. It returns the report it published.
func scanAndPublish(ctx context.Context, kube kubernetes.Interface, cfg config, now time.Time) (nodecert.NodeReport, error) {
	// The scan itself is filesystem-bound and bounded by depth/count limits in
	// the nodecert package; cfg.timeout bounds the ConfigMap publish below.
	publishCtx := ctx
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		publishCtx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}

	results := nodecert.Scan(nodecert.ScanOptions{Paths: cfg.paths, Thresholds: cfg.thresholds, Now: now})
	publishGauges(cfg.nodeName, results)

	report := nodecert.NodeReport{
		Node:       cfg.nodeName,
		CheckName:  cfg.checkName,
		ObservedAt: now.UTC(),
		Aggregate:  nodecert.WorstOutcome(results),
		Certs:      results,
	}
	if err := upsertReportConfigMap(publishCtx, kube, cfg, report); err != nil {
		return report, err
	}
	return report, nil
}

// publishGauges resets and repopulates this node's expiry-day series so a
// certificate that disappears between scans does not leave a stale series.
func publishGauges(node string, results []nodecert.CertResult) {
	metrics.NodeCertificateExpiryDays.Reset()
	for _, r := range results {
		if r.NotAfter.IsZero() {
			continue // Error/Skipped results carry no expiry to gauge.
		}
		metrics.NodeCertificateExpiryDays.WithLabelValues(node, r.Path).Set(float64(r.DaysRemaining))
	}
}

func upsertReportConfigMap(ctx context.Context, kube kubernetes.Interface, cfg config, report nodecert.NodeReport) error {
	encoded, err := nodecert.EncodeReport(report)
	if err != nil {
		return err
	}
	labels := map[string]string{
		nodecert.LabelManagedBy:  nodecert.ManagedByValue,
		nodecert.LabelSourceKind: nodecert.KindNodeCertificateCheck,
		nodecert.LabelSourceName: cfg.checkName,
		nodecert.LabelNode:       sanitizeLabelValue(cfg.nodeName),
	}
	data := map[string]string{nodecert.ConfigMapReportKey: encoded}

	cms := kube.CoreV1().ConfigMaps(cfg.checkNamespace)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, getErr := cms.Get(ctx, cfg.configMapName, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			_, createErr := cms.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: cfg.configMapName, Namespace: cfg.checkNamespace, Labels: labels},
				Data:       data,
			}, metav1.CreateOptions{})
			return createErr
		}
		if getErr != nil {
			return getErr
		}
		updated := existing.DeepCopy()
		if updated.Labels == nil {
			updated.Labels = map[string]string{}
		}
		for k, v := range labels {
			updated.Labels[k] = v
		}
		updated.Data = data
		_, updErr := cms.Update(ctx, updated, metav1.UpdateOptions{})
		return updErr
	})
}

// sanitizeLabelValue coerces a node name into a valid label value (<=63 chars,
// alphanumeric edges). Node names are the authoritative copy in the report
// payload; this label only exists for coarse filtering.
func sanitizeLabelValue(v string) string {
	v = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, v)
	if len(v) > validation.LabelValueMaxLength {
		v = v[:validation.LabelValueMaxLength]
	}
	v = strings.Trim(v, "-_.")
	if v == "" {
		return "unknown"
	}
	return v
}

func parseConfig(argv []string) (config, error) {
	fs := flag.NewFlagSet("node-agent", flag.ContinueOnError)
	var (
		checkName     = fs.String("check-name", "", "NodeCertificateCheck name that scheduled this agent (required)")
		checkNS       = fs.String("check-namespace", "", "namespace of the NodeCertificateCheck and the report ConfigMap (required)")
		nodeName      = fs.String("node-name", "", "node name (defaults to $NODE_NAME)")
		configMapName = fs.String("configmap-name", "", "name of the per-node report ConfigMap to upsert (defaults to a deterministic name derived from check + node)")
		pathsCSV      = fs.String("paths", "", "comma-separated certificate files/directories to scan (defaults to the built-in set)")
		warnDays      = fs.Int("warn-days", 30, "days-to-expiry at or below which a certificate is Warn")
		criticalDays  = fs.Int("critical-days", 7, "days-to-expiry at or below which a certificate is Fail")
		interval      = fs.Duration("interval", time.Hour, "re-scan cadence")
		timeout       = fs.Duration("timeout", 30*time.Second, "per-scan publish timeout")
		metricsAddr   = fs.String("metrics-bind-address", ":8080", "address for the Prometheus metrics endpoint")
		once          = fs.Bool("once", false, "run a single scan and exit")
	)
	if err := fs.Parse(argv); err != nil {
		return config{}, err
	}

	node := *nodeName
	if node == "" {
		node = os.Getenv("NODE_NAME")
	}
	if node == "" {
		node, _ = os.Hostname()
	}

	cfg := config{
		checkName:      *checkName,
		checkNamespace: *checkNS,
		nodeName:       node,
		configMapName:  *configMapName,
		paths:          splitCSV(*pathsCSV),
		thresholds:     nodecert.Thresholds{WarnDays: *warnDays, CriticalDays: *criticalDays},
		interval:       *interval,
		timeout:        *timeout,
		metricsAddr:    *metricsAddr,
		once:           *once,
	}
	if cfg.checkName == "" || cfg.checkNamespace == "" {
		return config{}, fmt.Errorf("--check-name and --check-namespace are required")
	}
	if cfg.nodeName == "" {
		return config{}, fmt.Errorf("node name is empty: set --node-name or the NODE_NAME env var")
	}
	if cfg.configMapName == "" {
		cfg.configMapName = nodecert.NodeReportConfigMapName(cfg.checkName, cfg.nodeName)
	}
	if cfg.interval <= 0 {
		cfg.interval = time.Hour
	}
	return cfg, nil
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
