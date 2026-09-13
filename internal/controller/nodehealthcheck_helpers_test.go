/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"strings"
	"testing"
	"time"

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
// above warn, disallowed paths filtered (defense-in-depth behind admission),
// and a stable (type, path) order so the DaemonSet template hash does not
// churn when a user reorders the spec.
func TestResolveNodeHealthItems(t *testing.T) {
	t.Parallel()
	check := nhCheck(
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckKubeletHealthz},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/log"},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckDiskHeadroom, Path: "/var/lib/kubelet", WarnPercentFree: ptr.To[int32](5), CriticalPercentFree: ptr.To[int32](30)},
		fathomv1alpha1.NodeHealthCheckItem{Type: fathomv1alpha1.NodeHealthCheckInodeHeadroom, Path: "/home"}, // slipped past admission
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

	// Reordering the spec must not change the resolved list.
	reordered := nhCheck(check.Spec.Checks[4], check.Spec.Checks[2], check.Spec.Checks[0], check.Spec.Checks[5], check.Spec.Checks[1])
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
	if got := nodeHealthReportMaxAge(long); got != maxNodeHealthAgentInterval+30*time.Second {
		t.Fatalf("report max age = %v, must follow the agent cadence, not the 24h interval", got)
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
		t.Fatal("future report accepted")
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
