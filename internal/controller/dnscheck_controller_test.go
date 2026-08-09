/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/probe"
)

// fakeDNSLauncher stands in for probe.Launcher so the reconcile loop runs
// without scheduling pods. It records every request, tracks peak concurrency —
// the only way to prove the in-flight cap is honoured rather than ignored — and
// lets a test decide each pair's outcome.
type fakeDNSLauncher struct {
	mu          sync.Mutex
	calls       []probe.Request
	inFlight    int
	maxInFlight int

	// respond decides one pair's outcome. Nil means every pair passes.
	respond func(probe.Request) (probe.Result, error)

	// delay holds each call open, so concurrency is observable and a run can be
	// driven past its bound.
	delay time.Duration

	// delayFor overrides delay per request, so one spec can mix pairs that
	// answer promptly with pairs that outlive the run bound.
	delayFor func(probe.Request) time.Duration
}

func (f *fakeDNSLauncher) Run(ctx context.Context, req probe.Request) (probe.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	respond, delay, delayFor := f.respond, f.delay, f.delayFor
	f.mu.Unlock()

	if delayFor != nil {
		delay = delayFor(req)
	}

	release := func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			release()
			return probe.Result{}, ctx.Err()
		}
	}
	release()

	if respond == nil {
		return probe.Result{Outcome: probe.OutcomePass, Summary: "resolved"}, nil
	}
	return respond(req)
}

func (f *fakeDNSLauncher) requests() []probe.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]probe.Request(nil), f.calls...)
}

func (f *fakeDNSLauncher) peakConcurrency() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

func newDNSCheckReconciler(launcher dnsProbeRunner, concurrency int) *DNSCheckReconciler {
	return &DNSCheckReconciler{
		Client:              k8sClient,
		Scheme:              k8sClient.Scheme(),
		ProbeClient:         k8sClient,
		ProbeImage:          "ghcr.io/skaphos/fathom-probe:test",
		MaxConcurrentProbes: concurrency,
		Launcher:            launcher,
	}
}

var dnsNamespaceCounter int

// newDNSNamespace gives each spec its own namespace, so per-check metric series
// and probe requests never bleed between specs.
func newDNSNamespace(ctx context.Context) string {
	dnsNamespaceCounter++
	name := fmt.Sprintf("dnscheck-ns-%d", dnsNamespaceCounter)
	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
	return name
}

func createDNSCheck(ctx context.Context, ns, name string, spec fathomv1alpha1.DNSCheckSpec) *fathomv1alpha1.DNSCheck {
	check := &fathomv1alpha1.DNSCheck{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       spec,
	}
	Expect(k8sClient.Create(ctx, check)).To(Succeed())
	return check
}

func reloadDNSCheck(ctx context.Context, check *fathomv1alpha1.DNSCheck) *fathomv1alpha1.DNSCheck {
	loaded := &fathomv1alpha1.DNSCheck{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: check.Namespace, Name: check.Name}, loaded)).To(Succeed())
	return loaded
}

func reconcileDNSCheck(ctx context.Context, r *DNSCheckReconciler, check *fathomv1alpha1.DNSCheck) reconcile.Result {
	result, err := r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: check.Namespace, Name: check.Name},
	})
	Expect(err).NotTo(HaveOccurred())
	return result
}

// dnsTargetGauge reads back the per-target series for one check, keyed
// "name|record_type|resolver|result". Reading the registry rather than the
// status is the point: FR-033 is about what an operator can alert on.
func dnsTargetGauge(ns, check string) map[string]float64 {
	families, err := ctrlmetrics.Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	out := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "fathom_dnscheck_target_result" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			if labels["namespace"] != ns || labels["check"] != check {
				continue
			}
			key := strings.Join([]string{labels["name"], labels["record_type"], labels["resolver"], labels["result"]}, "|")
			out[key] = metric.GetGauge().GetValue()
		}
	}
	return out
}

// dnsHealthReports lists the history records one check has produced.
func dnsHealthReports(ctx context.Context, ns, check string) []fathomv1alpha1.HealthReport {
	var reports fathomv1alpha1.HealthReportList
	Expect(k8sClient.List(ctx, &reports,
		client.InNamespace(ns),
		client.MatchingLabels{
			labelHealthReportSourceKind: dnsCheckKind,
			labelHealthReportSourceName: check,
		},
	)).To(Succeed())
	return reports.Items
}

// checkResultGauge reads back the shared check-level one-hot series for one
// check, so a deletion can be shown to withdraw both gauges, not just the
// DNSCheck-specific one.
func checkResultGauge(kind, ns, name string) map[string]float64 {
	families, err := ctrlmetrics.Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	out := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "fathom_check_result" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			if labels["kind"] != kind || labels["namespace"] != ns || labels["name"] != name {
				continue
			}
			out[labels["result"]] = metric.GetGauge().GetValue()
		}
	}
	return out
}

// Events are drained with drainEvents from check_observability_test.go — the
// Events contract is shared across every check kind, so its helper is too.

// targetResultFor finds one pair's reported result by its identity triple.
func targetResultFor(status fathomv1alpha1.DNSCheckStatus, name, recordType, resolver string) *fathomv1alpha1.DNSTargetResult {
	for i := range status.TargetResults {
		r := status.TargetResults[i]
		if r.Name == name && string(r.RecordType) == recordType && r.Resolver == resolver {
			return &status.TargetResults[i]
		}
	}
	return nil
}

var _ = Describe("DNSCheckReconciler", func() {
	var ns string

	BeforeEach(func() {
		ns = newDNSNamespace(ctx)
	})

	// T015 / US1
	It("reports a verdict, pair count, and generation after one run", func() {
		check := createDNSCheck(ctx, ns, "basic", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "b.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			},
		})

		r := newDNSCheckReconciler(&fakeDNSLauncher{}, 4)
		reconcileDNSCheck(ctx, r, check)

		got := reloadDNSCheck(ctx, check).Status
		Expect(got.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultPass)))
		Expect(got.ObservedTargets).To(BeEquivalentTo(2))
		Expect(got.ObservedGeneration).To(Equal(check.Generation))
		Expect(got.LastRunTime).NotTo(BeNil())
		Expect(got.Summary).NotTo(BeEmpty())
		Expect(apiMeta.IsStatusConditionTrue(got.Conditions, dnsCheckConditionAccepted)).To(BeTrue())
		Expect(apiMeta.IsStatusConditionTrue(got.Conditions, dnsCheckConditionComplete)).To(BeTrue())
	})

	// T015 / US1 — the requeue is anchored to run start, not completion.
	It("requeues on the declared cadence", func() {
		check := createDNSCheck(ctx, ns, "cadence", fathomv1alpha1.DNSCheckSpec{
			Targets:  []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
			Interval: &metav1.Duration{Duration: time.Minute},
			Timeout:  &metav1.Duration{Duration: 10 * time.Second},
		})

		r := newDNSCheckReconciler(&fakeDNSLauncher{}, 4)
		result := reconcileDNSCheck(ctx, r, check)

		// A near-instant fake run should leave almost the whole interval.
		Expect(result.RequeueAfter).To(BeNumerically(">", 50*time.Second))
		Expect(result.RequeueAfter).To(BeNumerically("<=", time.Minute))
	})

	// T016 / US1 — FR-025: a resolver's negative answer is the check's finding,
	// not an operator-side fault.
	It("folds an unresolvable target to Fail, not Error", func() {
		check := createDNSCheck(ctx, ns, "mixed", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "good.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "bad.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			},
		})

		launcher := &fakeDNSLauncher{respond: func(req probe.Request) (probe.Result, error) {
			if req.Target == "bad.example.com" {
				return probe.Result{Outcome: probe.OutcomeFail, Summary: "DNS resolution failed"}, nil
			}
			return probe.Result{Outcome: probe.OutcomePass, Summary: "resolved"}, nil
		}}
		r := newDNSCheckReconciler(launcher, 4)
		reconcileDNSCheck(ctx, r, check)

		got := reloadDNSCheck(ctx, check).Status
		Expect(got.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultFail)))
		Expect(targetResultFor(got, "good.example.com", "A", "cluster").Result).To(Equal("Pass"))
		Expect(targetResultFor(got, "bad.example.com", "A", "cluster").Result).To(Equal("Fail"))
	})

	// T017 / US1 — FR-014's sharpest case. The probe already reports Error with
	// "absence cannot be proven" when a resolver does not answer under an absent
	// assertion; the controller must not launder that into a Pass.
	It("never lets an unreachable resolver satisfy a negative assertion", func() {
		check := createDNSCheck(ctx, ns, "absent-unreachable", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "retired.example.com", RecordType: fathomv1alpha1.DNSRecordA, Absent: true},
			},
			Resolvers: []fathomv1alpha1.DNSResolver{
				{Name: "upstream", From: fathomv1alpha1.DNSResolverExplicit, Address: "10.255.255.1:53"},
			},
		})

		launcher := &fakeDNSLauncher{respond: func(probe.Request) (probe.Result, error) {
			return probe.Result{
				Outcome: probe.OutcomeError,
				Summary: "resolver did not answer; absence cannot be proven",
			}, nil
		}}
		r := newDNSCheckReconciler(launcher, 4)
		reconcileDNSCheck(ctx, r, check)

		got := reloadDNSCheck(ctx, check).Status
		Expect(got.LastResult).NotTo(Equal(string(fathomv1alpha1.HealthReportResultPass)),
			"an unreachable resolver must never read as a satisfied absence assertion (FR-014)")
		Expect(got.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultError)))
		Expect(targetResultFor(got, "retired.example.com", "A", "upstream").Result).To(Equal("Error"))
	})

	// T018 / US1
	It("reflects a spec edit on the next run and advances observedGeneration", func() {
		check := createDNSCheck(ctx, ns, "edited", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
		})

		r := newDNSCheckReconciler(&fakeDNSLauncher{}, 4)
		reconcileDNSCheck(ctx, r, check)
		Expect(reloadDNSCheck(ctx, check).Status.ObservedTargets).To(BeEquivalentTo(1))

		edited := reloadDNSCheck(ctx, check)
		edited.Spec.Targets = append(edited.Spec.Targets,
			fathomv1alpha1.DNSTarget{Name: "b.example.com", RecordType: fathomv1alpha1.DNSRecordA})
		Expect(k8sClient.Update(ctx, edited)).To(Succeed())

		reconcileDNSCheck(ctx, r, edited)
		got := reloadDNSCheck(ctx, check).Status
		Expect(got.ObservedTargets).To(BeEquivalentTo(2))
		Expect(got.ObservedGeneration).To(Equal(edited.Generation))
	})

	// T019 / US1 — FR-035: the fan-out is over pairs, each evaluated once.
	It("evaluates every (target, vantage point) pair exactly once", func() {
		check := createDNSCheck(ctx, ns, "fanout", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "b.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			},
			Resolvers: []fathomv1alpha1.DNSResolver{
				{Name: "cluster-dns", From: fathomv1alpha1.DNSResolverCluster},
				{Name: "node-dns", From: fathomv1alpha1.DNSResolverNode},
			},
		})

		launcher := &fakeDNSLauncher{}
		r := newDNSCheckReconciler(launcher, 4)
		reconcileDNSCheck(ctx, r, check)

		Expect(launcher.requests()).To(HaveLen(4))
		got := reloadDNSCheck(ctx, check).Status
		Expect(got.ObservedTargets).To(BeEquivalentTo(4))
		Expect(got.TargetResults).To(HaveLen(4))
		for _, name := range []string{"a.example.com", "b.example.com"} {
			for _, resolver := range []string{"cluster-dns", "node-dns"} {
				Expect(targetResultFor(got, name, "A", resolver)).
					NotTo(BeNil(), "missing pair %s@%s", name, resolver)
			}
		}
	})

	// T020 / US1 — FR-103a. Without a peak-concurrency assertion a cap that is
	// silently ignored passes every other spec in this file.
	It("never exceeds the configured in-flight probe cap", func() {
		spec := fathomv1alpha1.DNSCheckSpec{
			Interval: &metav1.Duration{Duration: time.Minute},
			Timeout:  &metav1.Duration{Duration: 50 * time.Second},
		}
		for i := range 12 {
			spec.Targets = append(spec.Targets, fathomv1alpha1.DNSTarget{
				Name: fmt.Sprintf("n%02d.example.com", i), RecordType: fathomv1alpha1.DNSRecordA,
			})
		}
		check := createDNSCheck(ctx, ns, "capped", spec)

		launcher := &fakeDNSLauncher{delay: 20 * time.Millisecond}
		r := newDNSCheckReconciler(launcher, 3)
		reconcileDNSCheck(ctx, r, check)

		Expect(launcher.requests()).To(HaveLen(12))
		Expect(launcher.peakConcurrency()).To(BeNumerically("<=", 3),
			"in-flight probes exceeded the configured cap")
		Expect(launcher.peakConcurrency()).To(BeNumerically(">", 1),
			"pairs ran serially; the fan-out is not concurrent at all")
	})

	// T021 / US1 — FR-103b: fault isolation between pairs.
	It("isolates a launch failure to its own pair", func() {
		check := createDNSCheck(ctx, ns, "isolated", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "ok-1.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "doomed.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "ok-2.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			},
		})

		launcher := &fakeDNSLauncher{respond: func(req probe.Request) (probe.Result, error) {
			if req.Target == "doomed.example.com" {
				return probe.Result{}, &probe.LaunchError{Err: fmt.Errorf("exceeded quota: pods")}
			}
			return probe.Result{Outcome: probe.OutcomePass, Summary: "resolved"}, nil
		}}
		r := newDNSCheckReconciler(launcher, 4)
		reconcileDNSCheck(ctx, r, check)

		got := reloadDNSCheck(ctx, check).Status
		Expect(launcher.requests()).To(HaveLen(3), "a failed pair must not abort the run")
		Expect(targetResultFor(got, "doomed.example.com", "A", "cluster").Result).To(Equal("Error"))
		Expect(targetResultFor(got, "ok-1.example.com", "A", "cluster").Result).To(Equal("Pass"))
		Expect(targetResultFor(got, "ok-2.example.com", "A", "cluster").Result).To(Equal("Pass"))
		Expect(got.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultError)))
	})

	// T028 / US2 — SC-104: the failing name is identifiable from the resource
	// and the metrics alone, with no log reading.
	It("identifies the single failing target in both status and metrics", func() {
		check := createDNSCheck(ctx, ns, "pinpoint", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "healthy.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "broken.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "also-healthy.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			},
		})

		launcher := &fakeDNSLauncher{respond: func(req probe.Request) (probe.Result, error) {
			if req.Target == "broken.example.com" {
				return probe.Result{
					Outcome: probe.OutcomeFail,
					Summary: "the resolver reports the name does not exist",
					Details: map[string]string{"answers": ""},
				}, nil
			}
			return probe.Result{
				Outcome: probe.OutcomePass,
				Summary: "resolved",
				Details: map[string]string{"answers": "10.0.0.1,10.0.0.2"},
			}, nil
		}}
		r := newDNSCheckReconciler(launcher, 4)
		reconcileDNSCheck(ctx, r, check)

		got := reloadDNSCheck(ctx, check).Status
		broken := targetResultFor(got, "broken.example.com", "A", "cluster")
		Expect(broken.Result).To(Equal("Fail"))
		Expect(broken.Message).To(ContainSubstring("does not exist"), "the pair must carry its own evidence")

		healthy := targetResultFor(got, "healthy.example.com", "A", "cluster")
		Expect(healthy.Answers).To(ConsistOf("10.0.0.1", "10.0.0.2"))
		Expect(healthy.LatencyMillis).To(BeNumerically(">=", 0))

		gauge := dnsTargetGauge(ns, "pinpoint")
		Expect(gauge["broken.example.com|A|cluster|Fail"]).To(Equal(1.0))
		Expect(gauge["broken.example.com|A|cluster|Pass"]).To(Equal(0.0))
		Expect(gauge["healthy.example.com|A|cluster|Pass"]).To(Equal(1.0))
	})

	// T029 / US2 — FR-036 and SC-105: a pair the spec no longer declares must
	// leave both the status and the registry, within one run.
	It("withdraws a removed target's result and metric series", func() {
		check := createDNSCheck(ctx, ns, "shrinking", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "keeper.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "doomed.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			},
		})

		r := newDNSCheckReconciler(&fakeDNSLauncher{}, 4)
		reconcileDNSCheck(ctx, r, check)

		Expect(dnsTargetGauge(ns, "shrinking")).To(HaveKey("doomed.example.com|A|cluster|Pass"))
		Expect(targetResultFor(reloadDNSCheck(ctx, check).Status, "doomed.example.com", "A", "cluster")).NotTo(BeNil())

		edited := reloadDNSCheck(ctx, check)
		edited.Spec.Targets = []fathomv1alpha1.DNSTarget{
			{Name: "keeper.example.com", RecordType: fathomv1alpha1.DNSRecordA},
		}
		Expect(k8sClient.Update(ctx, edited)).To(Succeed())
		reconcileDNSCheck(ctx, r, edited)

		got := reloadDNSCheck(ctx, check).Status
		Expect(got.TargetResults).To(HaveLen(1))
		Expect(targetResultFor(got, "doomed.example.com", "A", "cluster")).
			To(BeNil(), "a dropped pair must not freeze at its last verdict")

		gauge := dnsTargetGauge(ns, "shrinking")
		for key := range gauge {
			Expect(key).NotTo(HavePrefix("doomed.example.com|"),
				"a dropped pair must not keep asserting a metric series")
		}
		Expect(gauge["keeper.example.com|A|cluster|Pass"]).To(Equal(1.0))
	})

	// T030 / US2 — FR-021. Without the polarity a deliberate "this name must be
	// gone" failure gets triaged as a DNS outage.
	It("names assertion polarity in a negative-assertion failure summary", func() {
		check := createDNSCheck(ctx, ns, "polarity", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "decommissioned.example.com", RecordType: fathomv1alpha1.DNSRecordA, Absent: true},
			},
		})

		launcher := &fakeDNSLauncher{respond: func(probe.Request) (probe.Result, error) {
			return probe.Result{Outcome: probe.OutcomeFail, Summary: "the name resolved"}, nil
		}}
		r := newDNSCheckReconciler(launcher, 4)
		reconcileDNSCheck(ctx, r, check)

		got := reloadDNSCheck(ctx, check).Status
		Expect(got.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultFail)))
		Expect(got.Summary).To(ContainSubstring("asserted absent but resolved"))
		Expect(got.Summary).To(ContainSubstring("decommissioned.example.com"))
	})

	// T031 / US2 — FR-106 and FR-106a end to end. Two pairs answer promptly and
	// two outlive the run bound, so the run truncates partially: the verdict must
	// degrade to Unknown rather than reporting the two passes as green, and the
	// Complete condition must name how many were missed.
	It("degrades to Unknown and reports the unreached count when a run truncates", func() {
		check := createDNSCheck(ctx, ns, "truncated", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "fast-1.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "fast-2.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "slow-1.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "slow-2.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			},
			Interval: &metav1.Duration{Duration: 10 * time.Second},
			Timeout:  &metav1.Duration{Duration: time.Second},
		})

		launcher := &fakeDNSLauncher{delayFor: func(req probe.Request) time.Duration {
			if strings.HasPrefix(req.Target, "slow-") {
				return 10 * time.Second // outlives the 1s run bound
			}
			return 5 * time.Millisecond
		}}
		r := newDNSCheckReconciler(launcher, 2)
		r.MinPairBudget = 50 * time.Millisecond
		reconcileDNSCheck(ctx, r, check)

		got := reloadDNSCheck(ctx, check).Status
		Expect(got.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultUnknown)),
			"a truncated run must not report the pairs it did reach as the whole story")
		Expect(targetResultFor(got, "fast-1.example.com", "A", "cluster").Result).To(Equal("Pass"))
		Expect(targetResultFor(got, "slow-1.example.com", "A", "cluster").Result).To(Equal("Unknown"))

		complete := apiMeta.FindStatusCondition(got.Conditions, dnsCheckConditionComplete)
		Expect(complete).NotTo(BeNil())
		Expect(complete.Status).To(Equal(metav1.ConditionFalse))
		Expect(complete.Reason).To(Equal("RunTruncated"))
		Expect(complete.Message).To(ContainSubstring("2 of 4"))
		Expect(got.Summary).To(ContainSubstring("not reached"))

		// Ready stays True: the bound was too small, which is a configuration
		// problem the operator can fix, not Fathom failing to function.
		Expect(apiMeta.IsStatusConditionTrue(got.Conditions, dnsCheckConditionReady)).To(BeTrue())
	})

	// T036 / US3 — FR-111, SC-103. History records transitions, not runs.
	It("writes one HealthReport for a stable verdict while lastRunTime keeps advancing", func() {
		check := createDNSCheck(ctx, ns, "stable", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{{Name: "steady.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
		})

		r := newDNSCheckReconciler(&fakeDNSLauncher{}, 4)
		reconcileDNSCheck(ctx, r, check)
		first := reloadDNSCheck(ctx, check).Status
		Expect(first.LastReportName).NotTo(BeEmpty())
		Expect(dnsHealthReports(ctx, ns, "stable")).To(HaveLen(1))

		for range 3 {
			reconcileDNSCheck(ctx, r, reloadDNSCheck(ctx, check))
		}

		later := reloadDNSCheck(ctx, check).Status
		Expect(dnsHealthReports(ctx, ns, "stable")).To(HaveLen(1),
			"an unchanged verdict must not grow history")
		Expect(later.LastReportName).To(Equal(first.LastReportName))
		Expect(later.LastRunTime.Time).To(BeTemporally(">=", first.LastRunTime.Time),
			"liveness must keep advancing even when history does not")
	})

	// T037 / US3 — one record and one event per transition.
	It("writes exactly one HealthReport and one event per verdict change", func() {
		check := createDNSCheck(ctx, ns, "changing", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{{Name: "flappy.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
		})

		var failing bool
		launcher := &fakeDNSLauncher{respond: func(probe.Request) (probe.Result, error) {
			if failing {
				return probe.Result{Outcome: probe.OutcomeFail, Summary: "the resolver reports the name does not exist"}, nil
			}
			return probe.Result{Outcome: probe.OutcomePass, Summary: "resolved"}, nil
		}}
		recorder := events.NewFakeRecorder(16)
		r := newDNSCheckReconciler(launcher, 4)
		r.Recorder = recorder

		reconcileDNSCheck(ctx, r, check)
		Expect(dnsHealthReports(ctx, ns, "changing")).To(HaveLen(1))
		passReport := reloadDNSCheck(ctx, check).Status.LastReportName

		failing = true
		reconcileDNSCheck(ctx, r, reloadDNSCheck(ctx, check))

		got := reloadDNSCheck(ctx, check).Status
		Expect(got.LastResult).To(Equal(string(fathomv1alpha1.HealthReportResultFail)))
		Expect(dnsHealthReports(ctx, ns, "changing")).To(HaveLen(2))
		Expect(got.LastReportName).NotTo(Equal(passReport))

		// One ResultChanged event per transition: Unknown->Pass, then Pass->Fail.
		Expect(drainEvents(recorder)).To(HaveLen(2))
	})

	// T036 / US3 — history is bounded by the declared limit.
	It("prunes HealthReport history to spec.historyLimit", func() {
		limit := int32(2)
		check := createDNSCheck(ctx, ns, "bounded", fathomv1alpha1.DNSCheckSpec{
			Targets:      []fathomv1alpha1.DNSTarget{{Name: "churn.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
			HistoryLimit: &limit,
		})

		var failing bool
		launcher := &fakeDNSLauncher{respond: func(probe.Request) (probe.Result, error) {
			if failing {
				return probe.Result{Outcome: probe.OutcomeFail, Summary: "failed"}, nil
			}
			return probe.Result{Outcome: probe.OutcomePass, Summary: "resolved"}, nil
		}}
		r := newDNSCheckReconciler(launcher, 4)

		// Five transitions would leave five records without pruning.
		for i := range 5 {
			failing = i%2 == 0
			reconcileDNSCheck(ctx, r, reloadDNSCheck(ctx, check))
		}

		Expect(dnsHealthReports(ctx, ns, "bounded")).To(HaveLen(int(limit)))
	})

	// T040 / US4 — FR-113. Ownership is what makes a probe pod accountable to
	// the check that caused it, and it is legal only because FR-031 puts the pod
	// in the check's own namespace.
	It("owns every probe pod it launches, in the check's own namespace", func() {
		check := createDNSCheck(ctx, ns, "owned", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{
				{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA},
				{Name: "b.example.com", RecordType: fathomv1alpha1.DNSRecordA},
			},
		})

		launcher := &fakeDNSLauncher{}
		r := newDNSCheckReconciler(launcher, 4)
		reconcileDNSCheck(ctx, r, check)

		stored := reloadDNSCheck(ctx, check)
		requests := launcher.requests()
		Expect(requests).To(HaveLen(2))
		for _, req := range requests {
			Expect(req.Namespace).To(Equal(ns),
				"FR-031: resolution must happen in the check's own namespace")
			Expect(req.OwnerReferences).To(HaveLen(1))
			owner := req.OwnerReferences[0]
			Expect(owner.Kind).To(Equal(dnsCheckKind))
			Expect(owner.Name).To(Equal(check.Name))
			Expect(owner.UID).To(Equal(stored.UID), "a stale UID would make the reference dangling")
			Expect(owner.Controller).NotTo(BeNil())
			Expect(*owner.Controller).To(BeTrue())
		}

		// Pod names must be distinct per pair and valid as DNS-1123 subdomains.
		Expect(requests[0].Name).NotTo(Equal(requests[1].Name))
		for _, req := range requests {
			Expect(len(req.Name)).To(BeNumerically("<=", 63))
			Expect(req.Name).To(MatchRegexp(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`))
		}
	})

	// T040 / US4 — successive runs must not collide with an orphan left behind
	// by a crashed operator, which would surface as AlreadyExists on every run
	// until the sweeper's minimum age elapsed.
	It("gives each run distinct probe pod names", func() {
		check := createDNSCheck(ctx, ns, "distinct", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
		})

		launcher := &fakeDNSLauncher{}
		r := newDNSCheckReconciler(launcher, 4)
		reconcileDNSCheck(ctx, r, check)
		reconcileDNSCheck(ctx, r, reloadDNSCheck(ctx, check))

		requests := launcher.requests()
		Expect(requests).To(HaveLen(2))
		Expect(requests[0].Name).NotTo(Equal(requests[1].Name),
			"a deterministic name would collide with an orphan from a crashed run")
	})

	// T041 / US4 — FR-114. A deleted check must stop asserting anything.
	It("withdraws every metric series when the check is gone", func() {
		check := createDNSCheck(ctx, ns, "vanishing", fathomv1alpha1.DNSCheckSpec{
			Targets: []fathomv1alpha1.DNSTarget{{Name: "a.example.com", RecordType: fathomv1alpha1.DNSRecordA}},
		})

		r := newDNSCheckReconciler(&fakeDNSLauncher{}, 4)
		reconcileDNSCheck(ctx, r, check)
		Expect(dnsTargetGauge(ns, "vanishing")).NotTo(BeEmpty())
		Expect(checkResultGauge(dnsCheckKind, ns, "vanishing")).NotTo(BeEmpty())

		Expect(k8sClient.Delete(ctx, reloadDNSCheck(ctx, check))).To(Succeed())
		// The reconcile that observes the deletion is what withdraws the series.
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: ns, Name: "vanishing"},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(dnsTargetGauge(ns, "vanishing")).To(BeEmpty(),
			"a deleted check must not keep asserting per-target results")
		Expect(checkResultGauge(dnsCheckKind, ns, "vanishing")).To(BeEmpty(),
			"a deleted check must not keep asserting a check-level result")
	})
})
