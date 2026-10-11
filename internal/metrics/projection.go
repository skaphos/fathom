/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package metrics

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Label projection lets a cluster administrator copy an explicit allowlist of
// check-resource labels onto the fathom_check_* gauges, so adopters can route
// alerts on dimensions they already keep in the CR (tier, team, owner) instead
// of encoding them in check names (skaphos/fathom#279).
//
// Cardinality stays bounded: projection adds dimensions, never series. Every
// check still has exactly one series set; a projected label only names what
// that set is about. The allowlist is administrator-set and capped, and the
// default — no projection — leaves the gauges exactly as they were.

// MaxProjectedLabels caps the allowlist. Each projected key widens every check
// series; past a handful, routing belongs in recording rules, not in the
// operator's gauges.
const MaxProjectedLabels = 10

// projectedLabelPrefix namespaces projected label names the way
// kube-state-metrics does (label_<sanitized key>). It also makes collision with
// the base labels (kind, name, namespace, result) impossible.
const projectedLabelPrefix = "label_"

var invalidLabelNameChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// ProjectedLabelName returns the Prometheus label name a Kubernetes label key
// is projected as: "app.kubernetes.io/team" becomes "label_app_kubernetes_io_team".
func ProjectedLabelName(key string) string {
	return projectedLabelPrefix + invalidLabelNameChars.ReplaceAllString(key, "_")
}

// ValidateProjectedLabels checks an allowlist of Kubernetes label keys: at most
// MaxProjectedLabels, each a valid qualified label key, and distinct after
// sanitization (so "a.b" and "a_b" cannot both claim label_a_b).
func ValidateProjectedLabels(keys []string) error {
	var errs []error
	if len(keys) > MaxProjectedLabels {
		errs = append(errs, fmt.Errorf("%d projected labels exceeds the maximum of %d", len(keys), MaxProjectedLabels))
	}
	claimed := map[string]string{}
	for _, key := range keys {
		if msgs := validation.IsQualifiedName(key); len(msgs) > 0 {
			errs = append(errs, fmt.Errorf("projected label %q is not a valid Kubernetes label key: %v", key, msgs))
			continue
		}
		name := ProjectedLabelName(key)
		if other, ok := claimed[name]; ok {
			errs = append(errs, fmt.Errorf("projected labels %q and %q both map to the metric label %q", other, key, name))
			continue
		}
		claimed[name] = key
	}
	return errors.Join(errs...)
}

// projection is the configured allowlist and the per-check values last
// published, so a changed CR label withdraws its old series instead of leaving
// a duplicate set behind.
type projection struct {
	mu    sync.Mutex
	keys  []string
	names []string
	// last maps a check identity to the projected values its series carry.
	last map[checkID][]string
}

type checkID struct{ kind, namespace, name string }

var projected = &projection{last: map[checkID][]string{}}

// checkGauges registers the three check gauges with the controller-runtime
// registry as one UNCHECKED collector (it describes nothing) that collects
// whichever gauges are current.
//
// Registering the GaugeVecs directly would pin their label names: the
// Prometheus registry keeps a metric name's label dimensions even after
// Unregister, so a gauge rebuilt with projected labels could never be
// registered again. Unchecked collectors skip that registration-time check;
// duplicate or inconsistent series are still rejected at gather time.
type checkGauges struct{}

func (checkGauges) Describe(chan<- *prometheus.Desc) {}

func (checkGauges) Collect(ch chan<- prometheus.Metric) {
	CheckResult.Collect(ch)
	CheckLastRunTimestamp.Collect(ch)
	CheckInterval.Collect(ch)
}

// ConfigureCheckLabels sets the allowlist of CR label keys projected onto the
// check gauges and rebuilds those gauges with the extra label names, dropping
// any series observed so far. It must run once at startup, before the metrics
// endpoint is served or any reconcile observes a check: a GaugeVec's label
// names are fixed when it is created, and the gauges are swapped without
// locking. An empty allowlist restores the default gauges.
func ConfigureCheckLabels(keys []string) error {
	if err := ValidateProjectedLabels(keys); err != nil {
		return err
	}
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = ProjectedLabelName(key)
	}

	CheckResult, CheckLastRunTimestamp, CheckInterval = newCheckGauges(names)

	projected.mu.Lock()
	defer projected.mu.Unlock()
	projected.keys = slices.Clone(keys)
	projected.names = names
	projected.last = map[checkID][]string{}
	return nil
}

// values returns the projected label values for one check, in allowlist order.
// A key the CR does not carry projects as "" — the series set keeps a uniform
// label set rather than dropping the dimension.
//
// changed reports whether these differ from what the check's series last
// carried; the caller must then withdraw the old series before writing new ones.
func (p *projection) values(id checkID, objLabels map[string]string) (vals []string, changed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) == 0 {
		return nil, false
	}
	vals = make([]string, len(p.keys))
	for i, key := range p.keys {
		vals[i] = objLabels[key]
	}
	prev, seen := p.last[id]
	p.last[id] = vals
	return vals, seen && !slices.Equal(prev, vals)
}

func (p *projection) forget(id checkID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.last, id)
}
