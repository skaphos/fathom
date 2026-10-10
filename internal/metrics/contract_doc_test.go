/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package metrics_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/skaphos/fathom/internal/metrics"
)

// contractCollectors is every fathom_* collector the operator registers. The
// metric surface is Fathom's alerting contract (skaphos/fathom#276), and the
// monitoring guide is where that contract is written down, so a metric or a
// label that is not documented there is a contract nobody was told about.
var contractCollectors = []prometheus.Collector{
	metrics.ReconcileTotal,
	metrics.ReconcileDuration,
	metrics.AdapterRunDuration,
	metrics.AdapterRegistered,
	metrics.CheckResult,
	metrics.CheckLastRunTimestamp,
	metrics.CheckInterval,
	metrics.DNSCheckTargetResult,
	metrics.NodeCertificateExpiryDays,
	metrics.NodeHealthCheckResult,
	metrics.NodeHealthFilesystemFreePercent,
}

var descPattern = regexp.MustCompile(`fqName: "([^"]+)".*variableLabels: \{([^}]*)\}`)

// describe returns a collector's metric name and variable label keys.
func describe(t *testing.T, c prometheus.Collector) (string, []string) {
	t.Helper()
	ch := make(chan *prometheus.Desc, 1)
	c.Describe(ch)
	close(ch)
	desc := <-ch
	m := descPattern.FindStringSubmatch(desc.String())
	if m == nil {
		t.Fatalf("cannot parse metric descriptor %s", desc)
	}
	var labels []string
	for l := range strings.SplitSeq(m[2], ",") {
		if l = strings.TrimSpace(l); l != "" {
			labels = append(labels, l)
		}
	}
	return m[1], labels
}

// Every registered fathom_* metric has a row in the guide's metric tables, and
// that row names each of its label keys.
func TestEveryMetricAndLabelIsDocumentedInTheContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "guides", "monitoring.md"))
	if err != nil {
		t.Fatalf("read monitoring guide: %v", err)
	}
	rows := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := regexp.MustCompile("^\\| `(fathom_[a-z_]+)` \\|").FindStringSubmatch(line); m != nil {
			rows[m[1]] = line
		}
	}

	for _, c := range contractCollectors {
		name, labels := describe(t, c)
		row, ok := rows[name]
		if !ok {
			t.Errorf("%s is registered but has no row in docs/guides/monitoring.md; "+
				"document it in the metric contract", name)
			continue
		}
		for _, l := range labels {
			if !strings.Contains(row, "`"+l+"`") {
				t.Errorf("%s label %q is not documented in its monitoring.md row", name, l)
			}
		}
	}
}

// contractCollectors must list every metric metrics.go declares, or a new
// metric would bypass the documentation check above.
func TestContractCollectorsCoverEveryDeclaredMetric(t *testing.T) {
	src, err := os.ReadFile("metrics.go")
	if err != nil {
		t.Fatalf("read metrics.go: %v", err)
	}
	var listed []string
	for _, c := range contractCollectors {
		name, _ := describe(t, c)
		listed = append(listed, name)
	}
	for _, m := range regexp.MustCompile(`Name:\s+"(fathom_[a-z_]+)"`).FindAllStringSubmatch(string(src), -1) {
		if !slices.Contains(listed, m[1]) {
			t.Errorf("metrics.go declares %s but contractCollectors does not list it", m[1])
		}
	}
}
