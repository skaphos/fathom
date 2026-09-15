/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// node-agent is the Fathom DaemonSet binary that evaluates node-local signals
// on a single node and publishes a per-node report ConfigMap the operator
// rolls up into a HealthReport. It runs in one of two modes, selected by
// --mode so a cluster running only one node-scoped kind never pays for both:
//
//   - certificates (default): scans on-disk X.509 certificates for
//     NodeCertificateCheck (SKA-519).
//   - health: measures filesystem headroom and probes the kubelet and the
//     container-runtime socket for NodeHealthCheck (#206).
//
// It is intentionally minimal and least-privilege: it reads from read-only
// hostPath mounts, writes exactly one ConfigMap (its own), and serves a
// Prometheus endpoint. All configuration is supplied by the operator via
// flags/env, so the agent needs no read access to either check API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
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
	"github.com/skaphos/fathom/internal/nodehealth"
)

// mode selects which node-scoped kind this agent serves.
type mode string

const (
	modeCertificates mode = "certificates"
	modeHealth       mode = "health"
)

type config struct {
	mode           mode
	checkName      string
	checkNamespace string
	nodeName       string
	configMapName  string
	// certificates mode.
	paths      []string
	thresholds nodecert.Thresholds
	// health mode: the resolved items and the kubelet endpoint to probe.
	healthItems       []nodehealth.Item
	kubeletHealthzURL string

	interval         time.Duration
	timeout          time.Duration
	metricsAddr      string
	once             bool
	fatalMetricsBind bool
	statfsGuard      *nodehealth.StatfsGuard
	// trigger is the run-now token this agent was started with (see
	// nodecert.EnvRunTrigger); stamped into every report it publishes.
	trigger string
}

func main() {
	// --probe-healthz turns the binary into its own liveness probe: the
	// kubelet execs it inside the pod's network namespace, where loopback is
	// not subject to the per-check NetworkPolicy, and it exits non-zero when
	// /healthz does not answer 200.
	if url := probeHealthzArg(os.Args[1:]); url != "" {
		if err := probeHealthz(url, 5*time.Second); err != nil {
			log.Fatalf("node-agent: liveness: %v", err)
		}
		return
	}
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

// run serves metrics and drives the evaluation loop until ctx is cancelled.
// With cfg.once it performs a single pass and returns.
func run(ctx context.Context, kube kubernetes.Interface, cfg config) error {
	// A metrics bind failure is fatal only when the operator says so
	// (--fatal-metrics-bind, set for a host-network agent whose port is a host
	// port): there a collision must surface as a crashing pod, AgentReady=False
	// on the check, rather than an agent that keeps publishing while its
	// metrics silently never serve. Everywhere else — the certificate agent on
	// its fixed pod-network port in particular — the listener is incidental to
	// publishing, and a bind failure is logged and tolerated exactly as before.
	liveness := newLiveness(cfg.interval, cfg.timeout)
	if cfg.statfsGuard == nil {
		// One guard for the agent's lifetime: a hung mount is abandoned once.
		cfg.statfsGuard = nodehealth.NewStatfsGuard()
	}
	srv := &http.Server{Addr: cfg.metricsAddr, Handler: metricsMux(liveness), ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serveErr <- fmt.Errorf("metrics server on %s: %w", cfg.metricsAddr, err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// scanOnce reports whether the pass published its report. Liveness follows
	// publication, not merely execution: an agent that evaluates but can no
	// longer write its ConfigMap (RBAC revoked, API unreachable) is not doing
	// its job, and must not answer /healthz as if it were — the kubelet
	// restarting it is what turns a silent coverage gap into a visible
	// AgentReady=False.
	var scanOnce func() bool
	switch cfg.mode {
	case modeHealth:
		scanOnce = func() bool {
			report, err := scanAndPublishHealth(ctx, kube, cfg, time.Now())
			if err != nil {
				log.Printf("node-agent: publish report: %v", err)
				return false
			}
			log.Printf("node-agent: evaluated %d health check(s) on %s, aggregate=%s", len(report.Checks), cfg.nodeName, report.Aggregate)
			return true
		}
	default:
		scanOnce = func() bool {
			report, err := scanAndPublish(ctx, kube, cfg, time.Now())
			if err != nil {
				log.Printf("node-agent: publish report: %v", err)
				return false
			}
			log.Printf("node-agent: scanned %d certificate(s) on %s, aggregate=%s", len(report.Certs), cfg.nodeName, report.Aggregate)
			return true
		}
	}

	published := scanOnce()
	liveness.record(published)
	if cfg.once {
		// A one-shot run exists to publish one report; whether the metrics
		// listener bound is irrelevant to that and must not fail it — but a
		// pass that did not publish is exactly the failure a Job or CLI caller
		// needs to see.
		select {
		case err := <-serveErr:
			log.Printf("node-agent: %v (ignored for a one-shot run)", err)
		default:
		}
		if !published {
			return errors.New("the pass did not publish its report")
		}
		return nil
	}

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-serveErr:
			if cfg.fatalMetricsBind {
				return err
			}
			log.Printf("node-agent: %v (continuing; metrics will not serve)", err)
		case <-ticker.C:
			liveness.record(scanOnce())
		}
	}
}

// probeHealthzArg returns the URL given to --probe-healthz (in either
// "--probe-healthz URL" or "--probe-healthz=URL" form), or "" when the flag is
// absent. It is parsed ahead of the normal flag set because the probe must
// not require the agent's other flags.
func probeHealthzArg(argv []string) string {
	for i, a := range argv {
		if a == "--probe-healthz" && i+1 < len(argv) {
			return argv[i+1]
		}
		if v, ok := strings.CutPrefix(a, "--probe-healthz="); ok {
			return v
		}
	}
	return ""
}

// probeHealthz GETs the agent's own /healthz over loopback and reports
// anything but a 2xx as an error, for use as the container's exec probe.
func probeHealthz(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	return nil
}

// liveness answers /healthz from the agent's own progress rather than
// unconditionally. A pass that never returns — a statfs wedged on a hung mount
// in uninterruptible sleep, which no context can cancel — used to leave the
// pod Running and Ready forever while it published nothing; the kubelet's
// liveness probe now restarts it once no pass has published within two
// cadences plus a timeout — one whole missed pass is tolerated (a slow API
// write, a probe running to its deadline) before a restart, so a single
// slow pass is not a false positive.
type liveness struct {
	lastPass atomic.Int64 // unix nanoseconds of the last completed pass (or start)
	maxAge   time.Duration
}

func newLiveness(interval, timeout time.Duration) *liveness {
	l := &liveness{maxAge: 2*interval + timeout}
	l.lastPass.Store(time.Now().UnixNano())
	return l
}

func (l *liveness) passed() { l.lastPass.Store(time.Now().UnixNano()) }

// record advances liveness only for a pass that published its report; a
// failed publication leaves the clock running toward a restart.
func (l *liveness) record(published bool) {
	if published {
		l.passed()
	}
}

// healthy reports whether the last completed pass (or process start) is
// recent enough that the agent is demonstrably still making progress.
func (l *liveness) healthy(now time.Time) bool {
	return now.Sub(time.Unix(0, l.lastPass.Load())) <= l.maxAge
}

func metricsMux(l *liveness) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(ctrlmetrics.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if l != nil && !l.healthy(time.Now()) {
			http.Error(w, "no completed pass within the liveness window", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// scanAndPublish runs one certificate scan, updates the expiry gauges, and
// upserts the per-node report ConfigMap. It returns the report it published.
func scanAndPublish(ctx context.Context, kube kubernetes.Interface, cfg config, now time.Time) (nodecert.NodeReport, error) {
	// The scan itself is filesystem-bound and bounded by depth/count limits in
	// the nodecert package; cfg.timeout bounds the ConfigMap publish below.
	publishCtx, cancel := boundedContext(ctx, cfg.timeout)
	defer cancel()

	results := nodecert.Scan(nodecert.ScanOptions{Paths: cfg.paths, Thresholds: cfg.thresholds, Now: now})
	publishGauges(cfg.nodeName, results)

	report := nodecert.NodeReport{
		Node:       cfg.nodeName,
		CheckName:  cfg.checkName,
		ObservedAt: now.UTC(),
		Aggregate:  nodecert.WorstOutcome(results),
		Certs:      results,
		Trigger:    cfg.trigger,
	}
	encoded, err := nodecert.EncodeReport(report)
	if err != nil {
		return report, err
	}
	if err := upsertReportConfigMap(publishCtx, kube, cfg, nodecert.KindNodeCertificateCheck, encoded); err != nil {
		return report, err
	}
	return report, nil
}

// scanAndPublishHealth runs one node-health evaluation, updates the
// per-check gauges, and upserts the per-node report ConfigMap. Unlike the
// certificate scan, the evaluation itself reaches the network (kubelet,
// runtime socket), so the evaluation is bounded by cfg.timeout — and the
// publish gets a bound of its own, derived from the parent. A probe that
// runs the evaluation to its deadline yields Fail results, and those must
// still reach the operator: publishing under the exhausted evaluation
// context would drop exactly the report that carries the failure and leave
// the operator with a coverage gap instead of a verdict.
func scanAndPublishHealth(ctx context.Context, kube kubernetes.Interface, cfg config, now time.Time) (nodehealth.NodeReport, error) {
	scanCtx, cancelScan := boundedContext(ctx, cfg.timeout)
	defer cancelScan()
	scanStart := time.Now()
	results := nodehealth.Scan(scanCtx, nodehealth.ScanOptions{
		Items:             cfg.healthItems,
		Timeout:           cfg.timeout,
		KubeletHealthzURL: cfg.kubeletHealthzURL,
		StatfsGuard:       cfg.statfsGuard,
	})
	cancelScan()
	// ObservedAt is the evaluation's completion time, which is what the
	// operator's freshness bound (one full cycle: agent cadence plus three
	// effective timeouts) is measured from.
	// Stamping the start time would make a probe that used most of its
	// timeout publish an already nearly-stale report. now is the injected
	// base clock; the elapsed scan time is added to it.
	observedAt := now.Add(time.Since(scanStart)).UTC()
	publishHealthGauges(cfg.nodeName, results)

	passCtx, cancel := boundedContext(ctx, cfg.timeout)
	defer cancel()

	report := nodehealth.NodeReport{
		Node:        cfg.nodeName,
		CheckName:   cfg.checkName,
		ObservedAt:  observedAt,
		Aggregate:   nodehealth.WorstOutcome(results),
		Checks:      results,
		Trigger:     cfg.trigger,
		ItemsDigest: nodehealth.ItemsDigest(cfg.healthItems, cfg.timeout),
	}
	encoded, err := nodehealth.EncodeReport(report)
	if err != nil {
		return report, err
	}
	if err := upsertReportConfigMap(passCtx, kube, cfg, nodehealth.KindNodeHealthCheck, encoded); err != nil {
		return report, err
	}
	return report, nil
}

// boundedContext derives a context limited to timeout when timeout is
// positive; otherwise it returns ctx with a no-op cancel.
func boundedContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
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

// publishHealthGauges resets and repopulates this node's per-check result
// series and the headroom percentages behind them, so a check or path the
// spec dropped does not leave a stale series.
func publishHealthGauges(node string, results []nodehealth.CheckResult) {
	metrics.ResetNodeHealthSeries()
	for _, r := range results {
		metrics.ObserveNodeHealthCheck(node, r.Type, r.Path, string(r.Outcome))
		if r.PercentFree == nil {
			continue
		}
		resource := "bytes"
		if r.Type == nodehealth.TypeInodeHeadroom {
			resource = "inodes"
		}
		metrics.ObserveNodeHealthFilesystem(node, r.Path, resource, *r.PercentFree)
	}
}

// upsertReportConfigMap writes the encoded report under the shared wire
// contract: the same label keys, node-name annotation, and data key for both
// node-scoped kinds, distinguished only by sourceKind and the ConfigMap name.
func upsertReportConfigMap(ctx context.Context, kube kubernetes.Interface, cfg config, sourceKind, encoded string) error {
	labels := map[string]string{
		nodecert.LabelManagedBy:  nodecert.ManagedByValue,
		nodecert.LabelSourceKind: sourceKind,
		nodecert.LabelSourceName: cfg.checkName,
		nodecert.LabelNode:       sanitizeLabelValue(cfg.nodeName),
	}
	// The node-name annotation is the report's authenticity anchor: the operator's
	// ValidatingAdmissionPolicy requires it to equal this agent's node claim from
	// its ServiceAccount token, so it must carry the exact (unsanitized) node name.
	annotations := map[string]string{nodecert.AnnotationNodeName: cfg.nodeName}
	data := map[string]string{nodecert.ConfigMapReportKey: encoded}

	cms := kube.CoreV1().ConfigMaps(cfg.checkNamespace)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, getErr := cms.Get(ctx, cfg.configMapName, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			_, createErr := cms.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: cfg.configMapName, Namespace: cfg.checkNamespace, Labels: labels, Annotations: annotations},
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
		if updated.Annotations == nil {
			updated.Annotations = map[string]string{}
		}
		for k, v := range annotations {
			updated.Annotations[k] = v
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
		modeFlag      = fs.String("mode", string(modeCertificates), "which node-scoped kind this agent serves: certificates (NodeCertificateCheck) or health (NodeHealthCheck)")
		checkName     = fs.String("check-name", "", "name of the check that scheduled this agent (required)")
		checkNS       = fs.String("check-namespace", "", "namespace of the check and the report ConfigMap (required)")
		nodeName      = fs.String("node-name", "", "node name (defaults to $NODE_NAME)")
		configMapName = fs.String("configmap-name", "", "name of the per-node report ConfigMap to upsert (defaults to a deterministic name derived from mode + check + node)")
		pathsCSV      = fs.String("paths", "", "certificates mode: comma-separated certificate files/directories to scan (defaults to the built-in set)")
		warnDays      = fs.Int("warn-days", 30, "certificates mode: days-to-expiry at or below which a certificate is Warn")
		criticalDays  = fs.Int("critical-days", 7, "certificates mode: days-to-expiry at or below which a certificate is Fail")
		checksJSON    = fs.String("checks", "", "health mode: JSON-encoded NodeHealthCheck items with thresholds already resolved (required in health mode)")
		kubeletURL    = fs.String("kubelet-healthz-url", nodehealth.DefaultKubeletHealthzURL, "health mode: kubelet health endpoint a KubeletHealthz check probes")
		interval      = fs.Duration("interval", time.Hour, "re-evaluation cadence")
		timeout       = fs.Duration("timeout", 30*time.Second, "bound on the report publish (health mode: bounds the evaluation and, separately, the publish, so one pass takes at most twice this)")
		metricsAddr   = fs.String("metrics-bind-address", ":8080", "address for the Prometheus metrics endpoint")
		fatalBind     = fs.Bool("fatal-metrics-bind", false, "exit when the metrics endpoint cannot bind (set by the operator for host-network agents, whose port is a host port)")
		once          = fs.Bool("once", false, "run a single pass and exit")
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
		mode:              mode(*modeFlag),
		checkName:         *checkName,
		checkNamespace:    *checkNS,
		nodeName:          node,
		configMapName:     *configMapName,
		paths:             splitCSV(*pathsCSV),
		thresholds:        nodecert.Thresholds{WarnDays: *warnDays, CriticalDays: *criticalDays},
		kubeletHealthzURL: *kubeletURL,
		interval:          *interval,
		timeout:           *timeout,
		metricsAddr:       *metricsAddr,
		once:              *once,
		fatalMetricsBind:  *fatalBind,
		// The token arrives through the downward API from the DaemonSet pod
		// template, so it is an env var rather than a flag: the operator does
		// not rewrite the args when only the trigger changes.
		trigger: os.Getenv(nodecert.EnvRunTrigger),
	}
	if cfg.checkName == "" || cfg.checkNamespace == "" {
		return config{}, fmt.Errorf("--check-name and --check-namespace are required")
	}
	if cfg.nodeName == "" {
		return config{}, fmt.Errorf("node name is empty: set --node-name or the NODE_NAME env var")
	}
	switch cfg.mode {
	case modeCertificates:
		if cfg.configMapName == "" {
			cfg.configMapName = nodecert.NodeReportConfigMapName(cfg.checkName, cfg.nodeName)
		}
	case modeHealth:
		if strings.TrimSpace(*checksJSON) == "" {
			return config{}, fmt.Errorf("--checks is required in health mode")
		}
		items, err := nodehealth.DecodeItems(*checksJSON)
		if err != nil {
			return config{}, fmt.Errorf("--checks: %w", err)
		}
		// An empty list is legal: a spec with only NodeCondition items has
		// nothing for the agent to evaluate, but the agent must still publish
		// (an empty, Skipped) report so the node counts as covered and the
		// operator can grade its conditions.
		cfg.healthItems = items
		if cfg.configMapName == "" {
			cfg.configMapName = nodehealth.ReportConfigMapName(cfg.checkName, cfg.nodeName)
		}
	default:
		return config{}, fmt.Errorf("--mode must be %q or %q, got %q", modeCertificates, modeHealth, cfg.mode)
	}
	if cfg.interval <= 0 {
		cfg.interval = time.Hour
	}
	if cfg.timeout <= 0 {
		// A non-positive timeout would leave evaluation and publication
		// unbounded and hash the report with a timeout the operator never
		// resolves, so the report would never be consumed. The operator always
		// passes a positive value; this only guards a hand-run agent.
		cfg.timeout = 30 * time.Second
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
