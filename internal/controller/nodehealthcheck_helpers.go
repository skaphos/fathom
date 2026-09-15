/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/nodehealth"
)

const (
	// nodeHealthReportFamily is the HealthReportCheck family for node-local
	// health observations.
	nodeHealthReportFamily      = "node_health"
	nodeHealthReportAdapterName = "node-health-check"
	nodeHealthReportAdapterVer  = "0.1.0"

	// nodeHealthAgentSuffix names the per-check agent resources. It differs
	// from NodeCertificateCheck's "-node-agent" so the two kinds can share a
	// check name in one namespace without fighting over a DaemonSet (#206).
	nodeHealthAgentSuffix = "-node-health-agent"

	// nodeHealthAgentComponentValue is this kind's value for the shared
	// component label. NodeCertificateCheck's DaemonSet and NetworkPolicy
	// selectors are {source-name, component} — and a DaemonSet selector is
	// immutable, so they cannot grow a kind label on upgrade. Using a distinct
	// component value is what makes the two kinds' selectors disjoint for a
	// shared check name: neither kind's DaemonSet or NetworkPolicy can ever
	// select the other's pods.
	nodeHealthAgentComponentValue = "node-health-agent"

	// maxNodeHealthAgentInterval caps how rarely an agent re-evaluates. The
	// operator's roll-up cadence follows spec.interval, but report freshness
	// must not scale with it: a 24h interval must not accept a 24h-old
	// measurement of a signal that changes on the order of minutes (#270). The
	// agent therefore runs at min(interval, this) and a report is fresh for
	// that cadence plus the timeout, so a long-interval check still detects a
	// transition within minutes while refreshing liveness on its own cadence.
	maxNodeHealthAgentInterval = fathomv1alpha1.MaxNodeHealthCheckAgentInterval

	// nodeHealthHostMetricsPortMin/Max bound the metrics port an agent binds
	// when it runs on the host network. Container port 8080 would then bind on
	// the node itself and collide with any other host-network listener there,
	// so the port is derived per check from this range instead. Two
	// host-network checks landing on the same port on one node is a hash
	// collision (~1 in 2768) that surfaces as CrashLoopBackOff on the second
	// agent, i.e. AgentReady=False — visible, never silent.
	nodeHealthHostMetricsPortMin = 30000
	nodeHealthHostMetricsPortMax = 32767

	// nodeHealthDefaultNodeConditionMessage is the healthy summary for a node
	// condition that carries its expected value.
	nodeHealthMaxSummaryLen = 1024
	nodeHealthMaxMessageLen = 512
)

// nodeHealthAgentResourceName is the shared name of the per-check
// ServiceAccount, RoleBinding, NetworkPolicy, and DaemonSet.
func nodeHealthAgentResourceName(check *fathomv1alpha1.NodeHealthCheck) string {
	return check.Name + nodeHealthAgentSuffix
}

// nodeHealthAgentSelectorLabels are the immutable DaemonSet selector labels.
// The distinct component value (see nodeHealthAgentComponentValue) is what
// keeps them disjoint from a same-named NodeCertificateCheck's selectors; the
// kind label is carried as well so the pods are self-describing.
func nodeHealthAgentSelectorLabels(check *fathomv1alpha1.NodeHealthCheck) map[string]string {
	return map[string]string{
		nodecert.LabelSourceKind: nodehealth.KindNodeHealthCheck,
		nodecert.LabelSourceName: check.Name,
		nodeAgentComponentLabel:  nodeHealthAgentComponentValue,
	}
}

// nodeHealthAgentLabels are applied to every operator-managed agent object so
// they trace back to their check and are discoverable.
func nodeHealthAgentLabels(check *fathomv1alpha1.NodeHealthCheck) map[string]string {
	labels := nodeHealthAgentSelectorLabels(check)
	labels[nodecert.LabelManagedBy] = nodecert.ManagedByValue
	return labels
}

func nodeHealthInterval(check *fathomv1alpha1.NodeHealthCheck) time.Duration {
	if check.Spec.Interval != nil && check.Spec.Interval.Duration > 0 {
		return clampCadence(check.Spec.Interval.Duration, fathomv1alpha1.MinCheckInterval)
	}
	return fathomv1alpha1.DefaultNodeHealthCheckInterval
}

func nodeHealthTimeout(check *fathomv1alpha1.NodeHealthCheck) time.Duration {
	if check.Spec.Timeout != nil && check.Spec.Timeout.Duration > 0 {
		return clampCadence(check.Spec.Timeout.Duration, fathomv1alpha1.MinCheckTimeout)
	}
	return fathomv1alpha1.DefaultNodeHealthCheckTimeout
}

// nodeHealthAgentInterval is the cadence the agent actually re-evaluates at:
// the spec interval, capped so freshness never scales with a long interval.
func nodeHealthAgentInterval(check *fathomv1alpha1.NodeHealthCheck) time.Duration {
	return min(nodeHealthInterval(check), maxNodeHealthAgentInterval)
}

// nodeHealthAgentTimeout bounds one agent pass. It is spec.timeout capped at
// the agent cadence: a pass can never usefully outlast the cadence it runs
// on, and the schema allows timeout == interval, so without this cap a 24h
// interval with a 24h timeout would stretch the freshness bound to a day —
// exactly what the capped cadence exists to prevent.
func nodeHealthAgentTimeout(check *fathomv1alpha1.NodeHealthCheck) time.Duration {
	return min(nodeHealthTimeout(check), nodeHealthAgentInterval(check))
}

// nodeHealthRequeueAfter is the periodic reconcile cadence: the capped agent
// cadence, which is never longer than spec.interval. Reports go stale on that
// cadence, so that is how often a silent agent failure must be looked for.
func nodeHealthRequeueAfter(check *fathomv1alpha1.NodeHealthCheck) time.Duration {
	return nodeHealthAgentInterval(check)
}

// nodeHealthReportMaxAge is how old a node report may be and still count. It
// follows the agent cadence and the agent's (capped) timeout, never the
// roll-up cadence (#270).
//
// The bound is a full cycle, not cadence plus one timeout. observedAt is
// stamped when evaluation completes; the report then takes up to one timeout
// to publish, the next tick can be a whole cadence away, and the next report
// takes up to one timeout to evaluate and one more to publish before it is
// visible. With timeout == interval (legal) a continuously running agent was
// therefore judged stale on every cycle and its coverage frozen.
func nodeHealthReportMaxAge(check *fathomv1alpha1.NodeHealthCheck) time.Duration {
	return nodeHealthAgentInterval(check) + 3*nodeHealthAgentTimeout(check)
}

func nodeHealthReportFresh(observedAt, now time.Time, maxAge time.Duration) bool {
	if observedAt.IsZero() {
		return false
	}
	// observedAt has normally been bounded by the API server's write time
	// already (nodeHealthObservedBound), which is what stops a fast node clock
	// from stretching the window. Without that bound (a ConfigMap with no
	// managedFields) ordinary drift is tolerated by counting the report as
	// observed now, but a stamp beyond the skew allowance fails closed: clamped
	// afresh on every reconcile it would never age, and a dead agent's
	// far-future report would stay fresh forever. Large skew is logged by the
	// collector (nodeHealthClockSkewNotable).
	if observedAt.After(now.Add(maxNodeHealthReportClockSkew)) {
		return false
	}
	if observedAt.After(now) {
		observedAt = now
	}
	return now.Sub(observedAt) <= maxAge
}

// maxNodeHealthReportClockSkew is how far ahead of the operator's clock a
// report's observedAt may be before the collector logs it as a clock problem
// worth an operator's attention; ordinary drift stays below it.
const maxNodeHealthReportClockSkew = 30 * time.Second

// nodeHealthObservedBound returns the time freshness is measured from: the
// report's own observedAt, capped at the API server's most recent write time
// for the ConfigMap (managedFields[].time, stamped from the server's clock).
// A node whose clock runs fast stamps the future; measuring from its stamp
// kept the report fresh for up to 2*maxAge, and rejecting it excluded the node
// entirely. The server's write time is the one clock both sides share, so a
// fast node's report is exactly as fresh as its arrival, no more and no less.
// Without managedFields (an older server, a fake client) the stamp is used.
func nodeHealthObservedBound(observedAt time.Time, cm *corev1.ConfigMap) time.Time {
	var latest time.Time
	for _, mf := range cm.ManagedFields {
		if mf.Time != nil && mf.Time.After(latest) {
			latest = mf.Time.Time
		}
	}
	if !latest.IsZero() && observedAt.After(latest) {
		return latest
	}
	return observedAt
}

// nodeHealthClockSkewNotable reports whether observedAt is far enough ahead of
// now that the node's clock, not the report, is what needs fixing.
func nodeHealthClockSkewNotable(observedAt, now time.Time) bool {
	return observedAt.After(now.Add(maxNodeHealthReportClockSkew))
}

// resolveNodeHealthItems turns the spec's items into the resolved wire items
// the agent receives: API defaults applied to every unset threshold and
// socket, and the list sorted by (type, path) so the DaemonSet arguments — and
// hence the template hash — are stable across spec reorderings. Items the
// allowlist refuses are not filtered here; Reconcile rejects the whole spec
// (rejectedNodeHealthItems) so nothing is ever silently dropped.
func resolveNodeHealthItems(check *fathomv1alpha1.NodeHealthCheck) []nodehealth.Item {
	items := make([]nodehealth.Item, 0, len(check.Spec.Checks))
	for _, c := range check.Spec.Checks {
		it := nodehealth.Item{Type: string(c.Type), Path: c.Path}
		switch c.Type {
		case fathomv1alpha1.NodeHealthCheckDiskHeadroom, fathomv1alpha1.NodeHealthCheckInodeHeadroom:
			it.WarnPercentFree = fathomv1alpha1.DefaultNodeHealthWarnPercentFree
			it.CriticalPercentFree = fathomv1alpha1.DefaultNodeHealthCriticalPercentFree
			if c.WarnPercentFree != nil {
				it.WarnPercentFree = *c.WarnPercentFree
			}
			if c.CriticalPercentFree != nil {
				it.CriticalPercentFree = *c.CriticalPercentFree
			}
			// The Fail boundary can never sit above the Warn boundary.
			if it.CriticalPercentFree > it.WarnPercentFree {
				it.CriticalPercentFree = it.WarnPercentFree
			}
		case fathomv1alpha1.NodeHealthCheckContainerRuntime:
			it.SocketPath = c.SocketPath
			if it.SocketPath == "" {
				it.SocketPath = fathomv1alpha1.DefaultNodeHealthContainerRuntimeSocket
			}
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return nodehealth.ItemKey(items[i]) < nodehealth.ItemKey(items[j])
	})
	return items
}

// nodeHealthAgentItems returns the items the agent evaluates (everything but
// NodeCondition).
func nodeHealthAgentItems(items []nodehealth.Item) []nodehealth.Item {
	out := make([]nodehealth.Item, 0, len(items))
	for _, it := range items {
		if it.AgentEvaluated() {
			out = append(out, it)
		}
	}
	return out
}

// nodeHealthConditionTypes returns the node condition types the check asserts,
// or nil when the spec declares no NodeCondition item. Duplicate items cannot
// exist (admission rejects them), so at most one item contributes.
func nodeHealthConditionTypes(check *fathomv1alpha1.NodeHealthCheck) []string {
	for _, c := range check.Spec.Checks {
		if c.Type != fathomv1alpha1.NodeHealthCheckNodeCondition {
			continue
		}
		if len(c.Conditions) > 0 {
			out := append([]string(nil), c.Conditions...)
			sort.Strings(out)
			return out
		}
		return fathomv1alpha1.DefaultNodeHealthConditions()
	}
	return nil
}

func nodeHealthNeedsHostNetwork(items []nodehealth.Item) bool {
	for _, it := range items {
		if it.NeedsHostNetwork() {
			return true
		}
	}
	return false
}

func nodeHealthNeedsRoot(items []nodehealth.Item) bool {
	for _, it := range items {
		if it.NeedsRoot() {
			return true
		}
	}
	return false
}

// nodeHealthSocketPaths lists the distinct CRI sockets the agent must have
// mounted, sorted for a stable template.
func nodeHealthSocketPaths(items []nodehealth.Item) []string {
	seen := map[string]struct{}{}
	for _, it := range items {
		if it.Type == nodehealth.TypeContainerRuntime && it.SocketPath != "" {
			seen[it.SocketPath] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// nodeHealthHostMetricsPort is the metrics port for a host-network agent: the
// explicit spec.metricsHostPort when set, otherwise one derived from the
// check's namespaced name so it is stable across reconciles and operator
// restarts (no template churn) without any state. The derived port is a hash,
// so two host-network checks on one node can collide deterministically; the
// explicit field is the operator's way out, and the AgentPrivileged condition
// names the port in effect so the collision is diagnosable.
func nodeHealthHostMetricsPort(check *fathomv1alpha1.NodeHealthCheck) int32 {
	if check.Spec.MetricsHostPort != nil {
		return *check.Spec.MetricsHostPort
	}
	sum := sha256.Sum256([]byte(check.Namespace + "/" + check.Name))
	span := uint32(nodeHealthHostMetricsPortMax - nodeHealthHostMetricsPortMin + 1)
	return int32(nodeHealthHostMetricsPortMin + binary.BigEndian.Uint32(sum[:4])%span)
}

// resolveNodeHealthTolerations mirrors resolveTolerations for this kind:
// spec.tolerations verbatim, control-plane tolerations only on explicit opt-in.
func resolveNodeHealthTolerations(check *fathomv1alpha1.NodeHealthCheck) []corev1.Toleration {
	var tolerations []corev1.Toleration
	if len(check.Spec.Tolerations) > 0 {
		tolerations = append(tolerations, check.Spec.Tolerations...)
	}
	if check.Spec.IncludeControlPlaneNodes != nil && *check.Spec.IncludeControlPlaneNodes {
		tolerations = append(tolerations, controlPlaneTolerations()...)
	}
	return tolerations
}

// evaluateNodeConditions grades a node's status conditions: Ready must be
// True, every other named condition must be False (the healthy value for all
// pressure and unavailability conditions). A condition the node does not
// report is Skipped, not Fail — a cloud-provider condition simply absent from a
// bare-metal node is not evidence of a problem.
func evaluateNodeConditions(node *corev1.Node, conditionTypes []string) []nodehealth.CheckResult {
	out := make([]nodehealth.CheckResult, 0, len(conditionTypes))
	for _, typ := range conditionTypes {
		res := nodehealth.CheckResult{Type: nodehealth.TypeNodeCondition, Path: typ}
		var found *corev1.NodeCondition
		for i := range node.Status.Conditions {
			if string(node.Status.Conditions[i].Type) == typ {
				found = &node.Status.Conditions[i]
				break
			}
		}
		if found == nil {
			res.Outcome = nodehealth.OutcomeSkipped
			res.Summary = fmt.Sprintf("node does not report a %s condition", typ)
			out = append(out, res)
			continue
		}
		want := corev1.ConditionFalse
		if typ == string(corev1.NodeReady) {
			want = corev1.ConditionTrue
		}
		if found.Status == want {
			res.Outcome = nodehealth.OutcomePass
			res.Summary = fmt.Sprintf("%s=%s", typ, found.Status)
		} else {
			res.Outcome = nodehealth.OutcomeFail
			res.Summary = fmt.Sprintf("%s=%s (want %s)", typ, found.Status, want)
			if found.Reason != "" {
				res.Summary += ": " + found.Reason
			}
			if found.Message != "" {
				res.Summary += " — " + found.Message
			}
		}
		out = append(out, res)
	}
	return out
}

// nodeHealthEvaluation is one node's merged evidence for the current roll-up:
// the agent's report plus the operator-evaluated node conditions.
type nodeHealthEvaluation struct {
	Node       string
	ObservedAt time.Time
	Checks     []nodehealth.CheckResult
	Outcome    nodehealth.Outcome
	Message    string
}

// worstCheck returns the check the node's outcome comes from, for the
// per-node message and the check summary.
func (e nodeHealthEvaluation) worstCheck() *nodehealth.CheckResult {
	var worst *nodehealth.CheckResult
	worstRank := 0
	for i := range e.Checks {
		c := &e.Checks[i]
		if c.Outcome == nodehealth.OutcomeSkipped {
			continue
		}
		if rank := nodeOutcomeToResult(c.Outcome).Severity(); rank > worstRank {
			worst, worstRank = c, rank
		}
	}
	return worst
}

// mergeNodeHealthEvaluation folds an agent report and the node-condition
// results into one per-node evaluation. The merged list is sorted by (type,
// path) so HealthReport checks and messages are stable.
func mergeNodeHealthEvaluation(report nodehealth.NodeReport, conditions []nodehealth.CheckResult) nodeHealthEvaluation {
	checks := make([]nodehealth.CheckResult, 0, len(report.Checks)+len(conditions))
	checks = append(checks, report.Checks...)
	checks = append(checks, conditions...)
	sort.SliceStable(checks, func(i, j int) bool {
		if checks[i].Type != checks[j].Type {
			return checks[i].Type < checks[j].Type
		}
		return checks[i].Path < checks[j].Path
	})
	e := nodeHealthEvaluation{Node: report.Node, ObservedAt: report.ObservedAt, Checks: checks}
	e.Outcome = nodehealth.WorstOutcome(checks)
	if worst := e.worstCheck(); worst != nil && worst.Outcome != nodehealth.OutcomePass {
		e.Message = truncateNodeHealthMessage(nodeHealthCheckLabel(*worst)+": "+worst.Summary, nodeHealthMaxMessageLen)
	}
	return e
}

// nodeHealthCheckLabel names a check for messages: "DiskHeadroom
// /var/lib/kubelet", "NodeCondition Ready", "KubeletHealthz".
func nodeHealthCheckLabel(c nodehealth.CheckResult) string {
	if c.Path == "" {
		return c.Type
	}
	return c.Type + " " + c.Path
}

// truncateNodeHealthMessage bounds s to max code points — the unit the
// schema's MaxLength counts — so a non-ASCII condition message is never cut
// mid-sequence and a message shorter than the cap in runes is never trimmed.
func truncateNodeHealthMessage(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

// aggregateNodeHealth folds every node's outcome into the check's verdict via
// the shared WorstResult fold: Skipped is informational, and no nodes at all
// (nothing in scope reported) yields Skipped.
func aggregateNodeHealth(evals []nodeHealthEvaluation) fathomv1alpha1.HealthReportResult {
	results := make([]fathomv1alpha1.HealthReportResult, 0, len(evals))
	for _, e := range evals {
		results = append(results, nodeOutcomeToResult(e.Outcome))
	}
	return fathomv1alpha1.WorstResult(results, false)
}

// nodeHealthNodeResults projects the evaluations into status.nodeResults,
// sorted by node and capped at MaxNodeHealthNodeResults. The verdict is folded
// across every evaluation before this truncation, so the cap bounds only what
// is enumerated.
func nodeHealthNodeResults(evals []nodeHealthEvaluation) []fathomv1alpha1.NodeHealthNodeResult {
	sorted := append([]nodeHealthEvaluation(nil), evals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Node < sorted[j].Node })
	if len(sorted) > fathomv1alpha1.MaxNodeHealthNodeResults {
		sorted = sorted[:fathomv1alpha1.MaxNodeHealthNodeResults]
	}
	out := make([]fathomv1alpha1.NodeHealthNodeResult, 0, len(sorted))
	for _, e := range sorted {
		r := fathomv1alpha1.NodeHealthNodeResult{
			Node:    e.Node,
			Result:  string(nodeOutcomeToResult(e.Outcome)),
			Message: e.Message,
		}
		if !e.ObservedAt.IsZero() {
			t := metav1.NewTime(e.ObservedAt)
			r.ObservedAt = &t
		}
		out = append(out, r)
	}
	return out
}

// nodeHealthSummary is the one-line status.summary: how many nodes passed,
// and when some did not, the worst of them. Only Pass counts as passed: a
// node whose checks produced no participating result folds to Skipped, which
// is "nothing graded", not "healthy".
func nodeHealthSummary(evals []nodeHealthEvaluation, aggregate fathomv1alpha1.HealthReportResult) string {
	passed := 0
	var worst *nodeHealthEvaluation
	worstRank := 0
	for i := range evals {
		e := &evals[i]
		r := nodeOutcomeToResult(e.Outcome)
		if r == fathomv1alpha1.HealthReportResultPass {
			passed++
		}
		if r == fathomv1alpha1.HealthReportResultSkipped {
			continue
		}
		if rank := r.Severity(); rank > worstRank {
			worst, worstRank = e, rank
		}
	}
	s := fmt.Sprintf("%d of %d node(s) passed", passed, len(evals))
	if aggregate != fathomv1alpha1.HealthReportResultPass && aggregate != fathomv1alpha1.HealthReportResultSkipped && worst != nil && worst.Message != "" {
		s += "; worst: " + worst.Node + " " + worst.Message
	}
	return truncateNodeHealthMessage(s, nodeHealthMaxSummaryLen)
}

// healthReportForNodeHealth builds the rolled-up HealthReport: one check per
// (node, item), aggregated into a single worst-case Result.
func healthReportForNodeHealth(check *fathomv1alpha1.NodeHealthCheck, evals []nodeHealthEvaluation, aggregate fathomv1alpha1.HealthReportResult, observedAt metav1.Time) *fathomv1alpha1.HealthReport {
	var checks []fathomv1alpha1.HealthReportCheck
	for _, e := range evals {
		nodeObservedAt := observedAt
		if !e.ObservedAt.IsZero() {
			nodeObservedAt = metav1.NewTime(e.ObservedAt)
		}
		for _, c := range e.Checks {
			details := map[string]string{
				"node": e.Node,
				"type": c.Type,
			}
			putIfNotEmpty(details, "path", c.Path)
			if c.PercentFree != nil {
				details["percentFree"] = strconv.FormatFloat(*c.PercentFree, 'f', 1, 64)
				details["total"] = strconv.FormatUint(c.Total, 10)
				details["free"] = strconv.FormatUint(c.Free, 10)
			}
			checks = append(checks, fathomv1alpha1.HealthReportCheck{
				Family:     nodeHealthReportFamily,
				Result:     nodeOutcomeToResult(c.Outcome),
				TargetRef:  fathomv1alpha1.HealthReportTargetRef{APIVersion: "v1", Kind: "Node", Name: e.Node},
				Summary:    nodeHealthCheckLabel(c) + ": " + c.Summary,
				Details:    details,
				ObservedAt: nodeObservedAt,
			})
		}
	}

	return &fathomv1alpha1.HealthReport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    check.Namespace,
			GenerateName: check.Name + "-",
			Labels: map[string]string{
				fathomv1alpha1.LabelHealthReportSourceKind: nodehealth.KindNodeHealthCheck,
				fathomv1alpha1.LabelHealthReportSourceName: check.Name,
			},
		},
		Spec: fathomv1alpha1.HealthReportSpec{
			SourceRef: fathomv1alpha1.HealthReportTargetRef{
				APIVersion: fathomv1alpha1.GroupVersion.String(),
				Kind:       nodehealth.KindNodeHealthCheck,
				Namespace:  check.Namespace,
				Name:       check.Name,
			},
			AdapterName:    nodeHealthReportAdapterName,
			AdapterVersion: nodeHealthReportAdapterVer,
			Result:         aggregate,
			Checks:         checks,
			ObservedAt:     observedAt,
		},
	}
}

// decideNodeHealthRollup applies the transition-only contract (#157) to this
// kind's status fields; the decision vocabulary is shared with
// NodeCertificateCheck.
func decideNodeHealthRollup(status *fathomv1alpha1.NodeHealthCheckStatus, aggregate string, interval time.Duration, now time.Time) nodeCertRollupDecision {
	if status.LastReportName == "" || status.LastRunTime == nil || status.LastResult != aggregate {
		return rollupPersist
	}
	if now.Sub(status.LastRunTime.Time) >= interval {
		return rollupRefreshLiveness
	}
	return rollupNoop
}

// pruneNodeHealthHealthReports enforces Spec.HistoryLimit by deleting the
// oldest HealthReports for this check beyond the cap. Failures are logged, not
// returned: the new HealthReport already landed and the next reconcile retries.
func pruneNodeHealthHealthReports(ctx context.Context, c client.Client, log logr.Logger, check *fathomv1alpha1.NodeHealthCheck) {
	limit := defaultHealthReportHistoryLimit
	if check.Spec.HistoryLimit != nil {
		limit = int(*check.Spec.HistoryLimit)
	}
	if limit < 1 {
		return
	}

	var reports fathomv1alpha1.HealthReportList
	if err := c.List(ctx, &reports,
		client.InNamespace(check.Namespace),
		client.MatchingLabels{
			fathomv1alpha1.LabelHealthReportSourceKind: nodehealth.KindNodeHealthCheck,
			fathomv1alpha1.LabelHealthReportSourceName: check.Name,
		},
	); err != nil {
		log.Error(err, "list HealthReports for retention pruning failed; will retry on next reconcile")
		return
	}
	if len(reports.Items) <= limit {
		return
	}

	sort.Slice(reports.Items, func(i, j int) bool {
		return reports.Items[i].CreationTimestamp.Before(&reports.Items[j].CreationTimestamp)
	})
	excess := len(reports.Items) - limit
	for i := 0; i < excess; i++ {
		victim := &reports.Items[i]
		if err := c.Delete(ctx, victim); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "delete old HealthReport failed", "name", victim.Name)
		}
	}
	log.V(1).Info("pruned NodeHealthCheck HealthReport history", "deleted", excess, "limit", limit)
}

// nodeHealthEvaluationsInScope keeps only evaluations for nodes in expected,
// preserving order.
func nodeHealthEvaluationsInScope(evals []nodeHealthEvaluation, expected map[string]struct{}) []nodeHealthEvaluation {
	out := make([]nodeHealthEvaluation, 0, len(evals))
	for _, e := range evals {
		if _, ok := expected[e.Node]; ok {
			out = append(out, e)
		}
	}
	return out
}

// nodeHealthReportCoversSpec reports whether a fresh report carries a result
// for every agent-side item of the current spec. The agent emits exactly one
// result per item (Skipped and Error included), so a report missing an item
// predates the current template: it is a spec-change window, not evidence,
// and consuming it would let a newly added check — or a failing one the old
// template never had — be absent from the roll-up.
//
// Two bindings, both required. The digest binds semantics: it is computed over
// the resolved items, thresholds included, so a report evaluated under a
// previous threshold is never graded as if it carried the current one. The
// structural check binds shape: exactly one result per current item and
// nothing else, so a removed item's result cannot linger in the roll-up.
func nodeHealthReportCoversSpec(report nodehealth.NodeReport, agentItems []nodehealth.Item) bool {
	return report.ItemsDigest == nodehealth.ItemsDigest(agentItems) &&
		nodehealth.ReportCovers(report, agentItems)
}

// rejectedNodeHealthItems returns the spec items the operator's allowlist
// refuses (a headroom path or a runtime socket outside the approved
// prefixes). Admission rejects these too, so the list is non-empty only for
// an object stored under an older CRD. Such items are never silently
// dropped: a check that cannot measure what it declares must say so, not
// report a vacuous verdict from whatever remained.
func rejectedNodeHealthItems(check *fathomv1alpha1.NodeHealthCheck) []string {
	var rejected []string
	for _, c := range check.Spec.Checks {
		switch c.Type {
		case fathomv1alpha1.NodeHealthCheckDiskHeadroom, fathomv1alpha1.NodeHealthCheckInodeHeadroom:
			if !nodehealth.PathAllowed(c.Path) {
				rejected = append(rejected, string(c.Type)+" path "+c.Path)
			}
		case fathomv1alpha1.NodeHealthCheckContainerRuntime:
			if c.SocketPath != "" && !nodehealth.SocketPathAllowed(c.SocketPath) {
				rejected = append(rejected, string(c.Type)+" socketPath "+c.SocketPath)
			}
		}
	}
	return rejected
}

// nodeHealthNodeNameSet indexes evaluations by node.
func nodeHealthNodeNameSet(evals []nodeHealthEvaluation) map[string]struct{} {
	nodes := make(map[string]struct{}, len(evals))
	for _, e := range evals {
		nodes[e.Node] = struct{}{}
	}
	return nodes
}

// nodeHealthTriggeredNodeSet indexes the nodes whose fresh report carries
// token. The empty token never matches, so an agent predating the field
// cannot complete a wait.
func nodeHealthTriggeredNodeSet(reports []nodehealth.NodeReport, token string) map[string]struct{} {
	nodes := make(map[string]struct{}, len(reports))
	for _, r := range reports {
		if token != "" && r.Trigger == token {
			nodes[r.Node] = struct{}{}
		}
	}
	return nodes
}

// joinNodeHealthArgs renders the resolved agent items for the DaemonSet
// command line. It is a thin wrapper so the encoding failure (impossible for
// a plain struct) has one place to be handled.
func joinNodeHealthArgs(items []nodehealth.Item) string {
	encoded, err := nodehealth.EncodeItems(items)
	if err != nil {
		// A slice of plain structs always marshals; on the impossible error
		// hand the agent an empty list, which it rejects loudly at startup.
		return "[]"
	}
	return encoded
}

// nodeHealthPrivilegeSummary describes the elevated posture a spec requires,
// for the AgentPrivileged condition. Empty when the agent keeps the hardened
// default profile.
func nodeHealthPrivilegeSummary(hostNetwork, root bool, port int32, sockets []string) string {
	var parts []string
	if hostNetwork {
		parts = append(parts, fmt.Sprintf("hostNetwork (KubeletHealthz; the per-check NetworkPolicy does not isolate host-network pods; metrics on host port %d)", port))
	}
	if root {
		parts = append(parts, fmt.Sprintf("runAsUser 0 with %s mounted (ContainerRuntime)", strings.Join(sockets, ", ")))
	}
	return strings.Join(parts, "; ")
}
