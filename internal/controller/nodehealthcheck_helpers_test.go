/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/nodehealth"
)

func nhCheck(items ...fathomv1alpha1.NodeHealthCheckItem) *fathomv1alpha1.NodeHealthCheck {
	return &fathomv1alpha1.NodeHealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "nh", Namespace: "default"},
		Spec:       fathomv1alpha1.NodeHealthCheckSpec{Checks: items},
	}
}

// TestResolveNodeHealthItems pins the resolution the agent depends on: API
// defaults applied to unset thresholds and sockets, a critical threshold never
// above warn, and a stable (type, path) order so the DaemonSet template hash
// does not churn when a user reorders the spec. Disallowed paths are not
// filtered here: Reconcile rejects the whole spec (rejectedNodeHealthItems).
func TestResolveNodeHealthItems(t *testing.T) {
	t.Parallel()
	check := nhCheck(
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/log"},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet", WarnPercentFree: ptr.To[int32](5), CriticalPercentFree: ptr.To[int32](30)},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition},
	)
	got := resolveNodeHealthItems(check)
	want := []nodehealth.Item{
		{Type: "ContainerRuntime", SocketPath: fathomv1alpha1.DefaultNodeHealthContainerRuntimeSocket},
		{Type: "DiskHeadroom", Path: "/var/lib/kubelet", WarnPercentFree: 5, CriticalPercentFree: 5},
		{Type: "DiskHeadroom", Path: "/var/log", WarnPercentFree: 20, CriticalPercentFree: 10},
		{Type: "KubeletHealthz"},
		{Type: "NodeCondition"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d items %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	agent := nodeHealthAgentItems(got)
	for _, it := range agent {
		if it.Type == nodehealth.TypeNodeCondition {
			t.Fatal("NodeCondition must not be handed to the agent")
		}
	}
	if len(agent) != 4 {
		t.Fatalf("agent items = %d, want 4", len(agent))
	}

	// Two ContainerRuntime items sort by socket, so their order is stable too.
	twoSockets := nhCheck(
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime, SocketPath: "/run/crio/crio.sock"},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime},
	)
	sockets := resolveNodeHealthItems(twoSockets)
	if len(sockets) != 2 || sockets[0].SocketPath != fathomv1alpha1.DefaultNodeHealthContainerRuntimeSocket || sockets[1].SocketPath != "/run/crio/crio.sock" {
		t.Fatalf("runtime items not sorted by socket: %+v", sockets)
	}

	// Reordering the spec must not change the resolved list.
	reordered := nhCheck(check.Spec.Checks[4], check.Spec.Checks[2], check.Spec.Checks[0], check.Spec.Checks[3], check.Spec.Checks[1])
	again := resolveNodeHealthItems(reordered)
	if joinNodeHealthArgs(again) != joinNodeHealthArgs(got) {
		t.Fatalf("resolved items depend on spec order:\n%s\n%s", joinNodeHealthArgs(again), joinNodeHealthArgs(got))
	}
}

func TestNodeHealthConditionTypes(t *testing.T) {
	t.Parallel()
	if got := nodeHealthConditionTypes(nhCheck(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz})); got != nil {
		t.Fatalf("no NodeCondition item should yield nil, got %v", got)
	}
	got := nodeHealthConditionTypes(nhCheck(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition}))
	if strings.Join(got, ",") != strings.Join(fathomv1alpha1.DefaultNodeHealthConditions(), ",") {
		t.Fatalf("default conditions = %v", got)
	}
	got = nodeHealthConditionTypes(nhCheck(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckNodeCondition, Conditions: []string{"Ready", "NetworkUnavailable"}}))
	if strings.Join(got, ",") != "NetworkUnavailable,Ready" {
		t.Fatalf("explicit conditions = %v (want sorted)", got)
	}
}

// TestEvaluateNodeConditions pins the grading: Ready wants True, everything
// else wants False, an unreported condition is Skipped rather than Fail, and a
// failing condition's reason/message ride into the summary for triage.
func TestEvaluateNodeConditions(t *testing.T) {
	t.Parallel()
	node := &corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
		{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
		{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue, Reason: "KubeletHasDiskPressure", Message: "kubelet has disk pressure"},
	}}}
	got := evaluateNodeConditions(node, []string{"Ready", "MemoryPressure", "DiskPressure", "PIDPressure"})
	wantOutcome := map[string]nodehealth.Outcome{
		"Ready": nodehealth.OutcomePass, "MemoryPressure": nodehealth.OutcomePass,
		"DiskPressure": nodehealth.OutcomeFail, "PIDPressure": nodehealth.OutcomeSkipped,
	}
	if len(got) != 4 {
		t.Fatalf("got %d results", len(got))
	}
	for _, r := range got {
		if r.Type != nodehealth.TypeNodeCondition {
			t.Errorf("%s type = %s", r.Path, r.Type)
		}
		if r.Outcome != wantOutcome[r.Path] {
			t.Errorf("%s = %s (%s), want %s", r.Path, r.Outcome, r.Summary, wantOutcome[r.Path])
		}
		if r.Path == "DiskPressure" && (!strings.Contains(r.Summary, "KubeletHasDiskPressure") || !strings.Contains(r.Summary, "want False")) {
			t.Errorf("DiskPressure summary lacks reason/expectation: %q", r.Summary)
		}
	}

	notReady := &corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Reason: "NodeStatusUnknown"}}}}
	if r := evaluateNodeConditions(notReady, []string{"Ready"})[0]; r.Outcome != nodehealth.OutcomeFail || !strings.Contains(r.Summary, "Ready=Unknown (want True)") {
		t.Fatalf("Ready=Unknown = %s %q", r.Outcome, r.Summary)
	}
}

// TestMergeNodeHealthEvaluationAndSummary pins the per-node fold, the message
// naming the worst check, the check-level summary, and the status projection
// with its cap.
func TestMergeNodeHealthEvaluationAndSummary(t *testing.T) {
	t.Parallel()
	pct := 8.2
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	healthy := mergeNodeHealthEvaluation(nodehealth.NodeReport{Node: "node-a", ObservedAt: at, Checks: []nodehealth.CheckResult{
		{Type: nodehealth.TypeKubeletHealthz, Outcome: nodehealth.OutcomePass, Summary: "200"},
		{Type: nodehealth.TypeDiskHeadroom, Path: "/var/log", Outcome: nodehealth.OutcomeSkipped, Summary: "absent"},
	}}, []nodehealth.CheckResult{{Type: nodehealth.TypeNodeCondition, Path: "Ready", Outcome: nodehealth.OutcomePass, Summary: "Ready=True"}})
	if healthy.Outcome != nodehealth.OutcomePass || healthy.Message != "" {
		t.Fatalf("healthy node = %s %q", healthy.Outcome, healthy.Message)
	}
	// Merged checks are sorted by (type, path).
	if healthy.Checks[0].Type != nodehealth.TypeDiskHeadroom || healthy.Checks[1].Type != nodehealth.TypeKubeletHealthz || healthy.Checks[2].Type != nodehealth.TypeNodeCondition {
		t.Fatalf("merged order = %+v", healthy.Checks)
	}

	sick := mergeNodeHealthEvaluation(nodehealth.NodeReport{Node: "node-b", ObservedAt: at, Checks: []nodehealth.CheckResult{
		{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomeFail, Summary: "8.2% of bytes free (at or below criticalPercentFree 10)", PercentFree: &pct},
		{Type: nodehealth.TypeKubeletHealthz, Outcome: nodehealth.OutcomeWarn, Summary: "slow"},
	}}, nil)
	if sick.Outcome != nodehealth.OutcomeFail {
		t.Fatalf("sick node = %s", sick.Outcome)
	}
	if sick.Message != "DiskHeadroom /var/lib/kubelet: 8.2% of bytes free (at or below criticalPercentFree 10)" {
		t.Fatalf("message = %q", sick.Message)
	}

	evals := []nodeHealthEvaluation{sick, healthy}
	if agg := aggregateNodeHealth(evals); agg != fathomv1alpha1.HealthReportResultFail {
		t.Fatalf("aggregate = %s", agg)
	}
	if agg := aggregateNodeHealth(nil); agg != fathomv1alpha1.HealthReportResultSkipped {
		t.Fatalf("empty aggregate = %s", agg)
	}
	summary := nodeHealthSummary(evals, fathomv1alpha1.HealthReportResultFail)
	if summary != "1 of 2 node(s) passed; worst: node-b DiskHeadroom /var/lib/kubelet: 8.2% of bytes free (at or below criticalPercentFree 10)" {
		t.Fatalf("summary = %q", summary)
	}
	if s := nodeHealthSummary([]nodeHealthEvaluation{healthy}, fathomv1alpha1.HealthReportResultPass); s != "1 of 1 node(s) passed" {
		t.Fatalf("passing summary = %q", s)
	}
	// Skipped is "nothing graded", never "passed".
	skipped := nodeHealthEvaluation{Node: "node-s", Outcome: nodehealth.OutcomeSkipped}
	if s := nodeHealthSummary([]nodeHealthEvaluation{skipped}, fathomv1alpha1.HealthReportResultSkipped); s != "0 of 1 node(s) passed" {
		t.Fatalf("all-Skipped summary = %q", s)
	}
	if s := nodeHealthSummary([]nodeHealthEvaluation{healthy, skipped}, fathomv1alpha1.HealthReportResultPass); s != "1 of 2 node(s) passed" {
		t.Fatalf("mixed Pass/Skipped summary = %q", s)
	}

	results := nodeHealthNodeResults(evals)
	if len(results) != 2 || results[0].Node != "node-a" || results[1].Node != "node-b" {
		t.Fatalf("results not sorted by node: %+v", results)
	}
	if results[1].Result != "Fail" || results[1].Message != sick.Message || results[1].ObservedAt == nil || !results[1].ObservedAt.Time.Equal(at) {
		t.Fatalf("node-b result = %+v", results[1])
	}

	// The cap bounds the projection, not the fold.
	many := make([]nodeHealthEvaluation, fathomv1alpha1.MaxNodeHealthNodeResults+5)
	for i := range many {
		many[i] = nodeHealthEvaluation{Node: "node-" + strings.Repeat("z", i%3) + string(rune('a'+i%26)) + strings.Repeat("q", i/26), Outcome: nodehealth.OutcomePass}
	}
	many[len(many)-1].Outcome = nodehealth.OutcomeFail
	if got := len(nodeHealthNodeResults(many)); got != fathomv1alpha1.MaxNodeHealthNodeResults {
		t.Fatalf("capped results = %d", got)
	}
	if agg := aggregateNodeHealth(many); agg != fathomv1alpha1.HealthReportResultFail {
		t.Fatalf("truncation must not hide the failing node from the fold: %s", agg)
	}

	report := healthReportForNodeHealth(nhCheck(), evals, fathomv1alpha1.HealthReportResultFail, metav1.NewTime(at))
	if report.Spec.Result != fathomv1alpha1.HealthReportResultFail || len(report.Spec.Checks) != 5 {
		t.Fatalf("report = %+v", report.Spec)
	}
	if report.Labels[fathomv1alpha1.LabelHealthReportSourceKind] != "NodeHealthCheck" || report.Spec.SourceRef.Kind != "NodeHealthCheck" {
		t.Fatalf("report provenance = %v / %+v", report.Labels, report.Spec.SourceRef)
	}
	var sawPct bool
	for _, c := range report.Spec.Checks {
		if c.Family != nodeHealthReportFamily || c.TargetRef.Kind != "Node" {
			t.Errorf("check = %+v", c)
		}
		if c.Details["percentFree"] == "8.2" && c.TargetRef.Name == "node-b" {
			sawPct = true
		}
	}
	if !sawPct {
		t.Fatal("headroom evidence did not reach the HealthReport details")
	}
}

// TestNodeHealthHostMetricsPort pins that a host-network agent's port is
// deterministic per check (no template churn across reconciles or restarts)
// and inside the reserved range.
func TestNodeHealthHostMetricsPort(t *testing.T) {
	t.Parallel()
	a := nhCheck()
	p1, p2 := nodeHealthHostMetricsPort(a), nodeHealthHostMetricsPort(a)
	if p1 != p2 {
		t.Fatalf("port not deterministic: %d vs %d", p1, p2)
	}
	if p1 < nodeHealthHostMetricsPortMin || p1 > nodeHealthHostMetricsPortMax {
		t.Fatalf("port %d outside [%d, %d]", p1, nodeHealthHostMetricsPortMin, nodeHealthHostMetricsPortMax)
	}
	b := nhCheck()
	b.Name = "other"
	if nodeHealthHostMetricsPort(b) == p1 {
		t.Log("two checks hashed to the same port; allowed but noting it")
	}
	// An explicit port wins over the derived one: the operator's escape hatch
	// for a collision.
	b.Spec.MetricsHostPort = ptr.To[int32](31337)
	if got := nodeHealthHostMetricsPort(b); got != 31337 {
		t.Fatalf("explicit port = %d, want 31337", got)
	}
	// Without a host-network item the shared container port is used.
	if got := nodeHealthMetricsPort(a, []nodehealth.Item{{Type: nodehealth.TypeDiskHeadroom, Path: "/var/log"}}); got != metricsContainerPort {
		t.Fatalf("non-host-network port = %d", got)
	}
	if got := nodeHealthMetricsPort(a, []nodehealth.Item{{Type: nodehealth.TypeKubeletHealthz}}); got != p1 {
		t.Fatalf("host-network port = %d, want %d", got, p1)
	}
}

// TestNodeHealthCadence pins the freshness rule from #270: the agent cadence
// is capped, so a long roll-up interval never accepts an old measurement.
func TestNodeHealthCadence(t *testing.T) {
	t.Parallel()
	long := nhCheck()
	long.Spec.Interval = &metav1.Duration{Duration: 24 * time.Hour}
	long.Spec.Timeout = &metav1.Duration{Duration: 30 * time.Second}
	if got := nodeHealthInterval(long); got != 24*time.Hour {
		t.Fatalf("roll-up interval = %v", got)
	}
	if got := nodeHealthAgentInterval(long); got != maxNodeHealthAgentInterval {
		t.Fatalf("agent interval = %v, want the %v cap", got, maxNodeHealthAgentInterval)
	}
	if got := nodeHealthReportMaxAge(long); got != maxNodeHealthAgentInterval+3*30*time.Second+nodeHealthFreshnessSlack {
		t.Fatalf("report max age = %v, must follow the agent cadence, not the 24h interval", got)
	}
	// The requeue follows the agent cadence: a 24h interval is still looked at
	// every 5m, so a silently dead agent is noticed within the freshness bound.
	if got := nodeHealthRequeueAfter(long); got != maxNodeHealthAgentInterval {
		t.Fatalf("requeue = %v, want the %v agent cadence", got, maxNodeHealthAgentInterval)
	}
	// timeout == interval is legal; the agent's timeout is capped at its
	// cadence so the freshness bound stays minutes, never a day.
	long.Spec.Timeout = &metav1.Duration{Duration: 24 * time.Hour}
	if got := nodeHealthAgentTimeout(long); got != maxNodeHealthAgentInterval {
		t.Fatalf("agent timeout = %v, want the %v cadence cap", got, maxNodeHealthAgentInterval)
	}
	if got := nodeHealthReportMaxAge(long); got != 4*maxNodeHealthAgentInterval+nodeHealthFreshnessSlack {
		t.Fatalf("report max age with a 24h timeout = %v, want %v", got, 2*maxNodeHealthAgentInterval)
	}

	short := nhCheck()
	short.Spec.Interval = &metav1.Duration{Duration: time.Minute}
	if got := nodeHealthAgentInterval(short); got != time.Minute {
		t.Fatalf("short agent interval = %v", got)
	}
	unset := nhCheck()
	if nodeHealthInterval(unset) != fathomv1alpha1.DefaultNodeHealthCheckInterval || nodeHealthTimeout(unset) != fathomv1alpha1.DefaultNodeHealthCheckTimeout {
		t.Fatal("defaults not applied")
	}
	sub := nhCheck()
	sub.Spec.Interval = &metav1.Duration{Duration: time.Second}
	sub.Spec.Timeout = &metav1.Duration{Duration: time.Millisecond}
	if nodeHealthInterval(sub) != fathomv1alpha1.MinCheckInterval || nodeHealthTimeout(sub) != fathomv1alpha1.MinCheckTimeout {
		t.Fatal("sub-floor cadence not clamped")
	}

	now := time.Now()
	if !nodeHealthReportFresh(now.Add(-time.Minute), now, 5*time.Minute) {
		t.Fatal("recent report not fresh")
	}
	if nodeHealthReportFresh(now.Add(-6*time.Minute), now, 5*time.Minute) {
		t.Fatal("old report accepted")
	}
	if nodeHealthReportFresh(now.Add(time.Hour), now, 5*time.Minute) {
		t.Fatal("a stamp far beyond the skew allowance with no server bound must fail closed")
	}
	if nodeHealthReportFresh(time.Time{}, now, 5*time.Minute) {
		t.Fatal("zero observedAt accepted")
	}
}

func TestDecideNodeHealthRollup(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *metav1.Time { mt := metav1.NewTime(now.Add(d)); return &mt }
	tests := []struct {
		name      string
		status    fathomv1alpha1.NodeHealthCheckStatus
		aggregate string
		want      nodeCertRollupDecision
	}{
		{"first roll-up persists", fathomv1alpha1.NodeHealthCheckStatus{}, "Pass", rollupPersist},
		{"transition persists", fathomv1alpha1.NodeHealthCheckStatus{LastReportName: "r", LastResult: "Pass", LastRunTime: at(0)}, "Fail", rollupPersist},
		{"unchanged within interval is a no-op", fathomv1alpha1.NodeHealthCheckStatus{LastReportName: "r", LastResult: "Pass", LastRunTime: at(-time.Minute)}, "Pass", rollupNoop},
		{"unchanged past interval refreshes liveness", fathomv1alpha1.NodeHealthCheckStatus{LastReportName: "r", LastResult: "Pass", LastRunTime: at(-5 * time.Minute)}, "Pass", rollupRefreshLiveness},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := decideNodeHealthRollup(&tt.status, tt.aggregate, 5*time.Minute, now); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeHealthPrivilegeSummary(t *testing.T) {
	t.Parallel()
	if s := nodeHealthPrivilegeSummary(false, false, 0, nil); s != "" {
		t.Fatalf("hardened summary = %q", s)
	}
	s := nodeHealthPrivilegeSummary(true, true, 31337, []string{"/run/containerd/containerd.sock"})
	for _, want := range []string{"hostNetwork", "31337", "runAsUser 0", "/run/containerd/containerd.sock", "NetworkPolicy does not isolate"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary %q lacks %q", s, want)
		}
	}
}

func TestResolveNodeHealthTolerations(t *testing.T) {
	t.Parallel()
	check := nhCheck()
	if got := resolveNodeHealthTolerations(check); len(got) != 0 {
		t.Fatalf("default tolerations = %v", got)
	}
	check.Spec.Tolerations = []corev1.Toleration{{Key: "dedicated", Operator: corev1.TolerationOpExists}}
	check.Spec.IncludeControlPlaneNodes = ptr.To(true)
	got := resolveNodeHealthTolerations(check)
	if len(got) != 3 || got[0].Key != "dedicated" || got[1].Key != "node-role.kubernetes.io/control-plane" {
		t.Fatalf("tolerations = %v", got)
	}
}

// TestCheckForNodeHealthReportConfigMap pins the watch mapper: only a
// NodeHealthCheck report ConfigMap (managed-by=fathom, source-kind of THIS kind,
// a source-name) enqueues its check, so a NodeCertificateCheck report or an
// unrelated ConfigMap never triggers this reconciler.
func TestCheckForNodeHealthReportConfigMap(t *testing.T) {
	t.Parallel()
	cm := func(labels map[string]string) *corev1.ConfigMap {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "ns", Labels: labels}}
	}
	tests := []struct {
		name   string
		labels map[string]string
		want   int
	}{
		{"node-health report enqueues its check", map[string]string{"fathom.skaphos.io/managed-by": "fathom", "fathom.skaphos.io/source-kind": "NodeHealthCheck", "fathom.skaphos.io/source-name": "nh"}, 1},
		{"certificate report is ignored", map[string]string{"fathom.skaphos.io/managed-by": "fathom", "fathom.skaphos.io/source-kind": "NodeCertificateCheck", "fathom.skaphos.io/source-name": "nh"}, 0},
		{"unmanaged ConfigMap is ignored", map[string]string{"fathom.skaphos.io/source-kind": "NodeHealthCheck", "fathom.skaphos.io/source-name": "nh"}, 0},
		{"missing source-name is ignored", map[string]string{"fathom.skaphos.io/managed-by": "fathom", "fathom.skaphos.io/source-kind": "NodeHealthCheck"}, 0},
		{"no labels", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := checkForNodeHealthReportConfigMap(context.Background(), cm(tt.labels))
			if len(got) != tt.want {
				t.Fatalf("requests = %v, want %d", got, tt.want)
			}
			if tt.want == 1 && (got[0].Namespace != "ns" || got[0].Name != "nh") {
				t.Fatalf("request = %v, want ns/nh", got[0])
			}
		})
	}
}

func TestNodeHealthEvaluationsInScope(t *testing.T) {
	t.Parallel()
	evals := []nodeHealthEvaluation{{Node: "a"}, {Node: "b"}, {Node: "c"}}
	got := nodeHealthEvaluationsInScope(evals, map[string]struct{}{"b": {}, "c": {}, "d": {}})
	if len(got) != 2 || got[0].Node != "b" || got[1].Node != "c" {
		t.Fatalf("in scope = %+v, want b,c in order", got)
	}
	if got := nodeHealthEvaluationsInScope(evals, nil); len(got) != 0 {
		t.Fatalf("no expected nodes should yield nothing, got %+v", got)
	}
}

// TestTruncateNodeHealthMessageCountsRunes pins that truncation is measured in
// code points, the unit the schema's MaxLength counts: a non-ASCII message is
// never cut mid-sequence, and one under the cap in runes is never trimmed.
func TestTruncateNodeHealthMessageCountsRunes(t *testing.T) {
	t.Parallel()
	msg := strings.Repeat("é", 10) // 10 runes, 20 bytes
	if got := truncateNodeHealthMessage(msg, 10); got != msg {
		t.Fatalf("a 10-rune message must survive a cap of 10, got %q", got)
	}
	got := truncateNodeHealthMessage(msg, 5)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 5 || !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated = %q (%d runes, valid=%v)", got, utf8.RuneCountInString(got), utf8.ValidString(got))
	}
}

// TestDesiredDaemonSetNeverMountsOnePathTwice pins the older-CRD guard: a
// headroom item at the runtime socket path is rejected by admission today, but
// an object stored under an older CRD must still produce a pod the kubelet
// accepts, not two mounts at one mountPath.
func TestDesiredDaemonSetNeverMountsOnePathTwice(t *testing.T) {
	t.Parallel()
	check := nhCheck(
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/run/containerd/containerd.sock"},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime},
	)
	r := &NodeHealthCheckReconciler{NodeAgentImage: "img"}
	ds := r.desiredDaemonSet(check, "sa", resolveNodeHealthItems(check))
	seen := map[string]int{}
	for _, m := range ds.Spec.Template.Spec.Containers[0].VolumeMounts {
		seen[m.MountPath]++
	}
	for path, n := range seen {
		if n != 1 {
			t.Fatalf("mountPath %s appears %d times", path, n)
		}
	}
	if len(ds.Spec.Template.Spec.Volumes) != len(ds.Spec.Template.Spec.Containers[0].VolumeMounts) {
		t.Fatalf("volumes (%d) and mounts (%d) must pair one to one", len(ds.Spec.Template.Spec.Volumes), len(ds.Spec.Template.Spec.Containers[0].VolumeMounts))
	}
}

// TestNodeHealthReportFreshClampsTheFuture pins the clock-skew handling: a
// report stamped ahead of the operator's clock counts as observed on arrival —
// it neither extends the freshness window (measured from the future it stayed
// fresh for up to 2*maxAge) nor excludes the node (rejecting it left a
// fast-clocked node permanently missing from coverage) — and a large skew is
// flagged separately for the collector to log.
func TestNodeHealthReportFreshClampsTheFuture(t *testing.T) {
	t.Parallel()
	now := time.Now()
	const maxAge = 10 * time.Minute
	if !nodeHealthReportFresh(now.Add(10*time.Second), now, maxAge) {
		t.Fatal("ordinary drift ahead of the operator must count as observed on arrival")
	}
	// Beyond the skew allowance with no server bound the report fails closed:
	// clamped afresh on every reconcile it would otherwise never age out.
	if nodeHealthReportFresh(now.Add(2*time.Minute), now, maxAge) {
		t.Fatal("an unbounded far-future stamp must not be fresh")
	}
	if !nodeHealthReportFresh(now.Add(-maxAge), now, maxAge) || nodeHealthReportFresh(now.Add(-maxAge-time.Second), now, maxAge) {
		t.Fatal("the past bound is inclusive at maxAge")
	}
	if nodeHealthClockSkewNotable(now.Add(10*time.Second), now) || !nodeHealthClockSkewNotable(now.Add(2*time.Minute), now) {
		t.Fatal("skew beyond 30s must be flagged; ordinary drift must not")
	}
}

// TestDesiredDaemonSetLivenessAndFatalBind pins two agent-lifecycle details:
// every health agent carries a liveness probe on its metrics port (a wedged
// pass must be restarted, not left Running), and only a host-network agent —
// whose metrics port is a host port — is told to die on a bind failure.
func TestDesiredDaemonSetLivenessAndFatalBind(t *testing.T) {
	t.Parallel()
	r := &NodeHealthCheckReconciler{NodeAgentImage: "img"}
	plain := nhCheck(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet"})
	ds := r.desiredDaemonSet(plain, "sa", resolveNodeHealthItems(plain))
	c := ds.Spec.Template.Spec.Containers[0]
	// Exec against loopback, not an HTTP probe from the kubelet: the per-check
	// NetworkPolicy would deny a kubelet-originated probe on an enforcing CNI.
	if c.LivenessProbe == nil || c.LivenessProbe.Exec == nil || !slices.Contains(c.LivenessProbe.Exec.Command, "--probe-healthz") ||
		!slices.Contains(c.LivenessProbe.Exec.Command, "http://127.0.0.1:"+strconv.Itoa(int(c.Ports[0].ContainerPort))+"/healthz") {
		t.Fatalf("liveness probe = %+v, want an exec probe of /node-agent --probe-healthz against loopback on the metrics port", c.LivenessProbe)
	}
	if slices.Contains(c.Args, "--fatal-metrics-bind") {
		t.Fatal("a pod-network agent must tolerate a metrics bind failure")
	}
	hostNet := nhCheck(fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz})
	ds = r.desiredDaemonSet(hostNet, "sa", resolveNodeHealthItems(hostNet))
	if !ds.Spec.Template.Spec.HostNetwork || !slices.Contains(ds.Spec.Template.Spec.Containers[0].Args, "--fatal-metrics-bind") {
		t.Fatal("a host-network agent must be told a metrics bind failure is fatal")
	}
}

// TestNodeHealthObservedBoundUsesTheServerClock pins how a fast node clock is
// neutralised: freshness is measured from the report's stamp capped at the
// API server's write time, so a future stamp neither extends the window
// (2*maxAge) nor excludes the node, while an honest stamp is used as is.
func TestNodeHealthObservedBoundUsesTheServerClock(t *testing.T) {
	t.Parallel()
	server := time.Now()
	dataWrite := metav1.NewFieldsV1(`{"f:data":{"f:report.json":{}},"f:metadata":{"f:labels":{}}}`)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ManagedFields: []metav1.ManagedFieldsEntry{
		{Manager: "node-agent", Operation: metav1.ManagedFieldsOperationUpdate, Time: ptr.To(metav1.NewTime(server.Add(-time.Hour))), FieldsV1: dataWrite},
		{Manager: "node-agent", Operation: metav1.ManagedFieldsOperationUpdate, Time: ptr.To(metav1.NewTime(server)), FieldsV1: dataWrite},
	}}}
	if got := nodeHealthObservedBound(server.Add(5*time.Minute), cm); !got.Equal(server) {
		t.Fatalf("future stamp bound = %v, want the server write time %v", got, server)
	}
	// An honest stamp is superseded by the server write time too: the server
	// clock is the one both sides share, so a slow node clock cannot make a
	// current report read as stale either.
	lagging := server.Add(-10 * time.Minute)
	if got := nodeHealthObservedBound(lagging, cm); !got.Equal(server) {
		t.Fatalf("lagging stamp bound = %v, want the server write time %v", got, server)
	}
	if got := nodeHealthObservedBound(server.Add(5*time.Minute), &corev1.ConfigMap{}); !got.Equal(server.Add(5 * time.Minute)) {
		t.Fatal("without managedFields the stamp is used (the freshness clamp still applies)")
	}
	// End to end: a stamp 5m ahead on a server write 9m ago is 9m old, so it
	// is fresh at maxAge 10m and stale at 8m — never "fresh for 15m".
	old := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ManagedFields: []metav1.ManagedFieldsEntry{{Time: ptr.To(metav1.NewTime(server.Add(-9 * time.Minute))), FieldsV1: dataWrite}}}}
	bound := nodeHealthObservedBound(server.Add(5*time.Minute), old)
	if !nodeHealthReportFresh(bound, server, 10*time.Minute) || nodeHealthReportFresh(bound, server, 8*time.Minute) {
		t.Fatal("age must follow the server write time, not the future stamp")
	}
}

// TestNodeHealthReportMaxAgeCoversAFullCycle pins the worst-case timeline for
// timeout == interval, which the schema allows: report k is stamped at the end
// of its evaluation, publishes for up to one timeout, the next tick arrives up
// to one cadence later, and report k+1 evaluates and publishes for a timeout
// each before it is visible. Report k must stay fresh until then.
func TestNodeHealthReportMaxAgeCoversAFullCycle(t *testing.T) {
	t.Parallel()
	check := nhCheck()
	check.Spec.Interval = &metav1.Duration{Duration: 10 * time.Second}
	check.Spec.Timeout = &metav1.Duration{Duration: 10 * time.Second}
	maxAge := nodeHealthReportMaxAge(check)
	stamp := time.Now()
	nextVisible := stamp.Add(10*time.Second /*publish k*/ + 10*time.Second /*wait for tick*/ + 10*time.Second /*evaluate k+1*/ + 10*time.Second /*publish k+1*/)
	if !nodeHealthReportFresh(stamp, nextVisible, maxAge) {
		t.Fatalf("report k must still be fresh when report k+1 becomes visible (maxAge %v)", maxAge)
	}
	// Ordinary scheduler/API latency at the boundary must not make it stale.
	if !nodeHealthReportFresh(stamp, nextVisible.Add(10*time.Second), maxAge) {
		t.Fatal("a few seconds of latency at the cycle boundary must not make report k stale")
	}
	if nodeHealthReportFresh(stamp, nextVisible.Add(nodeHealthFreshnessSlack+time.Second), maxAge) {
		t.Fatal("beyond the cycle plus the latency allowance the report is stale")
	}
}

// TestTolerationsEmptyAndOmittedAreEquivalent pins the documented contract for
// the nil-vs-empty round-trip (#150): an explicit empty list is normalised
// away by omitempty, and the operator resolves the two identically, so the
// lost distinction has no effect on the agent DaemonSet.
func TestTolerationsEmptyAndOmittedAreEquivalent(t *testing.T) {
	t.Parallel()
	omitted := nhCheck()
	empty := nhCheck()
	empty.Spec.Tolerations = []corev1.Toleration{}
	raw, err := json.Marshal(empty.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "tolerations") {
		t.Fatalf("an empty tolerations list is expected to be normalised away on the wire, got %s", raw)
	}
	if got, want := resolveNodeHealthTolerations(empty), resolveNodeHealthTolerations(omitted); len(got) != 0 || len(want) != 0 {
		t.Fatalf("empty and omitted tolerations must resolve identically to none: %v vs %v", got, want)
	}
	r := &NodeHealthCheckReconciler{NodeAgentImage: "img"}
	a := r.desiredDaemonSet(omitted, "sa", resolveNodeHealthItems(omitted))
	b := r.desiredDaemonSet(empty, "sa", resolveNodeHealthItems(empty))
	if nodeAgentSpecHash(a) != nodeAgentSpecHash(b) {
		t.Fatal("empty and omitted tolerations must produce the same DaemonSet template (no rollout churn)")
	}
}

// TestReportCoversSpecRejectsAnEmptyDigest pins that a report without a
// digest is never consumed: the fail-closed guarantee must not depend on the
// operator's own digest also coming out empty.
func TestReportCoversSpecRejectsAnEmptyDigest(t *testing.T) {
	t.Parallel()
	items := []nodehealth.Item{{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", WarnPercentFree: 20, CriticalPercentFree: 10}}
	report := nodehealth.NodeReport{Checks: []nodehealth.CheckResult{{Type: nodehealth.TypeDiskHeadroom, Path: "/var/lib/kubelet", Outcome: nodehealth.OutcomePass}}}
	if nodeHealthReportCoversSpec(report, items, 30*time.Second) {
		t.Fatal("a report with no digest must not cover the spec")
	}
	report.ItemsDigest = nodehealth.ItemsDigest(items, 30*time.Second)
	if !nodeHealthReportCoversSpec(report, items, 30*time.Second) {
		t.Fatal("a report with the current digest and matching checks must cover the spec")
	}
	// The pass timeout is part of the grading semantics: a report evaluated
	// under a different timeout must not be consumed as current.
	if nodeHealthReportCoversSpec(report, items, 10*time.Second) {
		t.Fatal("a report evaluated under another timeout must not cover the spec")
	}
}

// TestNodeHealthObservedBoundIgnoresAdoptionWrites pins that the operator's
// own owner-reference write does not count as report arrival: only the
// managedFields entry that wrote the payload (f:data) bounds freshness, so
// adopting a report cannot keep a stopped agent's old report fresh.
func TestNodeHealthObservedBoundIgnoresAdoptionWrites(t *testing.T) {
	t.Parallel()
	now := time.Now()
	agentWrite := now.Add(-9 * time.Minute)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ManagedFields: []metav1.ManagedFieldsEntry{
		{Manager: "node-agent", Time: ptr.To(metav1.NewTime(agentWrite)), FieldsV1: metav1.NewFieldsV1(`{"f:data":{"f:report.json":{}}}`)},
		{Manager: "manager", Time: ptr.To(metav1.NewTime(now)), FieldsV1: metav1.NewFieldsV1(`{"f:metadata":{"f:ownerReferences":{}}}`)},
	}}}
	if got := nodeHealthObservedBound(now.Add(time.Hour), cm); !got.Equal(agentWrite) {
		t.Fatalf("bound = %v, want the agent's data write %v, not the adoption write", got, agentWrite)
	}
	// Even an adopter entry that claims f:data (a server attributing more than
	// the changed fields) is never read: the manager name excludes it.
	cm.ManagedFields = append(cm.ManagedFields, metav1.ManagedFieldsEntry{Manager: nodeHealthAdopterFieldManager, Time: ptr.To(metav1.NewTime(now)), FieldsV1: metav1.NewFieldsV1(`{"f:data":{}}`)})
	if got := nodeHealthObservedBound(now.Add(time.Hour), cm); !got.Equal(agentWrite) {
		t.Fatalf("bound = %v, want the agent's write; the adopter's entry must be ignored by name", got)
	}
}

// TestRejectedItemsIncludeSocketCollisions pins that a headroom path equal to
// a ContainerRuntime socket (default included) is a rejected specification on
// an older-CRD object, never a directory mounted over a socket.
func TestRejectedItemsIncludeSocketCollisions(t *testing.T) {
	t.Parallel()
	check := nhCheck(
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/run/containerd/containerd.sock"},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckContainerRuntime},
	)
	rejected := rejectedNodeHealthItems(check)
	if len(rejected) != 1 || !strings.Contains(rejected[0], "collides with ContainerRuntime socketPath") {
		t.Fatalf("rejected = %v, want the socket collision", rejected)
	}
}
