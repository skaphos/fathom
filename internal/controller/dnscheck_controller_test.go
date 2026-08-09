/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
}

func (f *fakeDNSLauncher) Run(ctx context.Context, req probe.Request) (probe.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	respond, delay := f.respond, f.delay
	f.mu.Unlock()

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
})
