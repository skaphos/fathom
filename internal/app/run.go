/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/controller"
)

// Setupper is anything that can register itself with a controller-runtime
// Manager. The reconcilers in internal/controller already satisfy this.
type Setupper interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// NewScheme returns a runtime.Scheme populated with every type the operator
// reads or writes. Building it per call keeps tests independent.
func NewScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add client-go scheme: %w", err)
	}
	if err := fathomv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add fathom v1alpha1 scheme: %w", err)
	}
	// +kubebuilder:scaffold:scheme
	return scheme, nil
}

// disableHTTP2 mitigates HTTP/2 Stream Cancellation / Rapid Reset
// (CVE-2023-44487, CVE-2023-39325) by negotiating only HTTP/1.1.
func disableHTTP2(c *tls.Config) {
	c.NextProtos = []string{"http/1.1"}
}

// BuildManagerOptions translates Options into ctrl.Options plus any cert
// watchers that need to be Add()ed to the manager. It performs no I/O against
// the cluster, so it is safe to unit-test.
func BuildManagerOptions(opts Options, scheme *runtime.Scheme) (ctrl.Options, []*certwatcher.CertWatcher, error) {
	var tlsOpts []func(*tls.Config)
	if !opts.EnableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	var watchers []*certwatcher.CertWatcher

	webhookTLSOpts := append([]func(*tls.Config){}, tlsOpts...)
	if opts.Webhook.CertPath != "" {
		w, err := certwatcher.New(
			filepath.Join(opts.Webhook.CertPath, opts.Webhook.CertName),
			filepath.Join(opts.Webhook.CertPath, opts.Webhook.CertKey),
		)
		if err != nil {
			return ctrl.Options{}, nil, fmt.Errorf("init webhook cert watcher: %w", err)
		}
		watchers = append(watchers, w)
		webhookTLSOpts = append(webhookTLSOpts, func(c *tls.Config) {
			c.GetCertificate = w.GetCertificate
		})
	}

	metricsOpts := metricsserver.Options{
		BindAddress:   opts.Metrics.BindAddress,
		SecureServing: opts.Metrics.Secure,
		TLSOpts:       append([]func(*tls.Config){}, tlsOpts...),
	}
	if opts.Metrics.Secure {
		metricsOpts.FilterProvider = filters.WithAuthenticationAndAuthorization
	}
	if opts.Metrics.CertPath != "" {
		w, err := certwatcher.New(
			filepath.Join(opts.Metrics.CertPath, opts.Metrics.CertName),
			filepath.Join(opts.Metrics.CertPath, opts.Metrics.CertKey),
		)
		if err != nil {
			return ctrl.Options{}, nil, fmt.Errorf("init metrics cert watcher: %w", err)
		}
		watchers = append(watchers, w)
		metricsOpts.TLSOpts = append(metricsOpts.TLSOpts, func(c *tls.Config) {
			c.GetCertificate = w.GetCertificate
		})
	}

	return ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsOpts,
		WebhookServer:          webhook.NewServer(webhook.Options{TLSOpts: webhookTLSOpts}),
		HealthProbeBindAddress: opts.HealthProbeBindAddress,
		LeaderElection:         opts.LeaderElect,
		LeaderElectionID:       opts.LeaderElectionID,
	}, watchers, nil
}

// DefaultControllers returns the operator's built-in reconcilers, configured
// against mgr. Tests substitute their own Setupper slice instead.
func DefaultControllers(mgr ctrl.Manager) []Setupper {
	return []Setupper{
		&controller.HealthCheckReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		&controller.ClusterHealthReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
	}
}

// managerFactory builds a manager from a rest.Config and ctrl.Options. It is a
// variable so tests can override it without standing up envtest.
var managerFactory = func(cfg *rest.Config, opts ctrl.Options) (ctrl.Manager, error) {
	return ctrl.NewManager(cfg, opts)
}

// Run starts the operator. It blocks until ctx is cancelled or the manager
// returns. cfg is the Kubernetes REST config to talk to the API server;
// controllersFor returns the reconcilers to register against the built manager
// (use DefaultControllers in production).
func Run(
	ctx context.Context,
	cfg *rest.Config,
	opts Options,
	controllersFor func(ctrl.Manager) []Setupper,
) error {
	if cfg == nil {
		return errors.New("rest.Config must not be nil")
	}
	if controllersFor == nil {
		controllersFor = DefaultControllers
	}
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts.Zap)))

	scheme, err := NewScheme()
	if err != nil {
		return err
	}

	mgrOpts, watchers, err := BuildManagerOptions(opts, scheme)
	if err != nil {
		return err
	}

	mgr, err := managerFactory(cfg, mgrOpts)
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}

	for _, s := range controllersFor(mgr) {
		if err := s.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("setup controller: %w", err)
		}
	}
	// +kubebuilder:scaffold:builder

	for _, w := range watchers {
		if err := mgr.Add(w); err != nil {
			return fmt.Errorf("add cert watcher to manager: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("add readyz check: %w", err)
	}

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager exited with error: %w", err)
	}
	return nil
}

