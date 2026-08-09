/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/probe"
)

const (
	// dnsCheckKind is the kind label used across the check gauges and events, so
	// DNSCheck appears in cluster-wide check dashboards without per-kind handling.
	dnsCheckKind = "DNSCheck"

	// dnsCheckMaxConcurrentReconciles bounds how many DNSChecks reconcile at
	// once. Together with the per-check probe cap it fixes the cluster-wide
	// ceiling on concurrent probe pods at
	// dnsCheckMaxConcurrentReconciles × Options.DNSCheck.MaxConcurrentProbes,
	// computable from configuration without observing a running system (FR-103a).
	dnsCheckMaxConcurrentReconciles = 4

	// Condition types. Accepted and Ready follow the shape every check kind uses.
	// Complete is specific to DNSCheck: it is the truncation signal (FR-106a).
	dnsCheckConditionAccepted = checkConditionAccepted
	dnsCheckConditionReady    = checkConditionReady
	dnsCheckConditionComplete = "Complete"
)

// dnsProbeRunner is the launcher seam. Production uses *probe.Launcher; tests
// substitute a fake so the reconcile loop is exercised without scheduling pods.
type dnsProbeRunner interface {
	Run(ctx context.Context, req probe.Request) (probe.Result, error)
}

// DNSCheckReconciler reconciles a DNSCheck object.
//
// Each run expands the specification into (target, vantage point) pairs, runs
// one probe pod per pair inside the check's OWN namespace (FR-031), folds the
// per-pair outcomes into a single verdict with the project-wide fold, and
// mirrors the result into status, metrics, events, and — only when the verdict
// changes — a HealthReport.
type DNSCheckReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// ProbeClient creates, polls, and deletes probe pods. It MUST be an
	// uncached client.
	//
	// The manager's own client serves structured reads from the shared informer
	// cache, and scopedCacheOptions() does not list Pod — so a single cached Pod
	// Get would start an unfiltered cluster-wide Pod informer and pull every pod
	// in the cluster into memory, the failure removed in #164/SKA-581. The
	// adapter path avoids this only incidentally, by handing adapters the
	// uncached impersonating client; DNSCheck has no per-addon identity to
	// impersonate, so it must be given an uncached client explicitly.
	ProbeClient client.Client

	// ProbeImage is the container image probe pods run.
	ProbeImage string

	// MaxConcurrentProbes bounds probe pods in flight for a single check
	// (FR-103a). Values below 1 are treated as 1; Options.Validate rejects them
	// at startup, so this is only a guard against a zero-valued struct in tests.
	MaxConcurrentProbes int

	// Launcher runs one probe pod to completion. Optional: nil builds a
	// probe.Launcher over ProbeClient. Tests inject a fake.
	Launcher dnsProbeRunner

	// Tracer creates the per-Reconcile span. Optional; nil falls back to the
	// global provider (a no-op unless tracing is enabled).
	Tracer trace.Tracer

	// Recorder emits the Events contract on DNSCheck resources. Optional; nil
	// disables events without affecting the gauges.
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=dnschecks,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=dnschecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=dnschecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=fathom.skaphos.io,resources=healthreports,verbs=create;get;list;watch;delete
// A DNSCheck resolves from its OWN namespace (FR-031), so the operator must
// place a pod in a namespace it does not own. It cannot borrow the per-addon
// impersonation path: that identity lives in the operator's namespace and no
// equivalent exists for an arbitrary tenant namespace. The grant cannot be
// namespaced either — a DNSCheck may be created anywhere, and the set of
// namespaces is unknown when this manifest is rendered.
//
// create and get are added here; list and delete already exist for the orphan
// sweeper (internal/probe/sweeper.go). get is required because the launcher
// polls the pod it created. watch is deliberately NOT requested: polling needs
// none, and a cluster-wide Pod watch would expose far more than pod placement
// requires. Neither are pods/exec, pods/log, or pods/portforward.
//
// docs/reference/operator-rbac.md carries the full justification, and
// operator_rbac_doc_test.go fails if this marker and that page disagree.
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;get

// Reconcile evaluates a DNSCheck and mirrors the outcome into status, metrics,
// events, and history.
func (r *DNSCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = ctx
	_ = req
	// Body lands with User Story 1 (T022–T027).
	return ctrl.Result{}, nil
}

// launcher returns the configured runner, defaulting to a probe.Launcher over
// the uncached probe client.
func (r *DNSCheckReconciler) launcher() dnsProbeRunner {
	if r.Launcher != nil {
		return r.Launcher
	}
	return &probe.Launcher{Client: r.ProbeClient}
}

// concurrency is the in-flight probe cap, floored at 1 so a zero-valued struct
// cannot deadlock a run.
func (r *DNSCheckReconciler) concurrency() int {
	if r.MaxConcurrentProbes < 1 {
		return 1
	}
	return r.MaxConcurrentProbes
}

// SetupWithManager registers the reconciler.
//
// It deliberately does NOT Own(&corev1.Pod{}): an Owns clause installs an
// informer for that type, which is precisely the cluster-wide Pod watch
// ProbeClient exists to avoid. Probe pods are short-lived and polled directly,
// so nothing needs to watch them — and a pod event could not usefully
// re-trigger a run that is already in flight.
func (r *DNSCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fathomv1alpha1.DNSCheck{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: dnsCheckMaxConcurrentReconciles}).
		Named("dnscheck").
		Complete(r)
}
