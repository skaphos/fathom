/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/internal/adapter/certmanager"
	"github.com/skaphos/fathom/internal/adapter/coredns"
	"github.com/skaphos/fathom/internal/adapter/declarative"
	"github.com/skaphos/fathom/internal/adapter/impersonation"
	"github.com/skaphos/fathom/internal/adapter/kubestatemetrics"
	"github.com/skaphos/fathom/internal/adapter/nodelocaldns"
	"github.com/skaphos/fathom/internal/adapter/registry"
	"github.com/skaphos/fathom/internal/controller"
	"github.com/skaphos/fathom/internal/metrics"
	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/internal/probe"
	"github.com/skaphos/fathom/internal/tracing"
	"github.com/skaphos/fathom/pkg/adapter"
)

// tracingShutdownTimeout bounds the graceful flush of buffered spans on operator
// shutdown so a slow or unreachable collector cannot stall process exit.
const tracingShutdownTimeout = 5 * time.Second

// readyzCheck returns a [healthz.Checker] that returns nil only once synced has
// been set. Used by [Run] to gate /readyz on informer cache sync rather than
// letting the manager report Ready before its caches are populated — without
// this, rolling-update traffic can route to a not-actually-ready replica.
func readyzCheck(synced *atomic.Bool) healthz.Checker {
	return func(_ *http.Request) error {
		if !synced.Load() {
			return errors.New("informers not synced")
		}
		return nil
	}
}

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
	// apiextensions/v1 is required by the cert-manager and external-secrets
	// adapters, which Get CustomResourceDefinition objects to verify the
	// addon's CRDs are installed. client-go's default scheme does not
	// include apiextensions, so we register it explicitly here. Without
	// this every adapter that reads CRDs surfaces an Error outcome with:
	//   no kind is registered for the type v1.CustomResourceDefinition
	// in real-cluster reconciles, even though envtest unit tests pass
	// because envtest auto-registers it.
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add apiextensions v1 scheme: %w", err)
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
		Cache:                  scopedCacheOptions(),
	}, watchers, nil
}

// scopedCacheOptions restricts the manager's shared informer cache for the
// generic Kubernetes types the operator manages, so it never lists or caches
// the whole cluster's objects of those kinds. Without this, the nodecert
// controller's cluster-wide ConfigMap watch would pull every ConfigMap body
// (e.g. tens of thousands of Helm release ConfigMaps) into memory and OOM the
// operator on large clusters (SKA-581 / #164).
//
// The label selector is safe only for kinds whose every cached read targets a
// Fathom-managed object carrying fathom.skaphos.io/managed-by=fathom:
//   - ConfigMap: only the node-agent report CMs are cached (List+Watch in the
//     NodeCertificateCheck controller); the node-agent stamps managed-by=fathom.
//     Adapters read arbitrary addon ConfigMaps through the uncached impersonating
//     client, so they are unaffected by the cache filter.
//   - DaemonSet / RoleBinding: only the node-agent DaemonSet and RoleBinding are
//     cached (Owns + CreateOrUpdate); both carry managed-by=fathom via agentLabels.
//     Addon DaemonSets/RBAC are read via the uncached impersonating client.
//
// ServiceAccount is deliberately NOT filtered here: AddonCheck reconciliation
// does a *cached* List of per-addon ServiceAccounts labeled
// fathom.skaphos.io/addon=<addon> (never managed-by=fathom), so a
// managed-by=fathom selector would empty that list and break every AddonCheck.
// ServiceAccounts are also small and few relative to ConfigMaps, so leaving them
// cluster-wide is not a meaningful memory cost.
func scopedCacheOptions() cache.Options {
	managedByFathom := cache.ByObject{
		Label: labels.SelectorFromSet(labels.Set{
			nodecert.LabelManagedBy: nodecert.ManagedByValue,
		}),
	}
	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}:   managedByFathom,
			&appsv1.DaemonSet{}:   managedByFathom,
			&rbacv1.RoleBinding{}: managedByFathom,
		},
	}
}

// DefaultControllers returns the operator's built-in reconcilers, configured
// against mgr. Tests substitute their own Setupper slice instead. opts carries
// values that reconcilers need to relay into per-Run adapter requests (e.g.
// the operator-level probe image).
//
// In-cluster deployments must set opts.Namespace (FATHOM_NAMESPACE / --namespace)
// so per-addon impersonation cannot silently fall open to the operator SA
// (SKA-162). Out-of-cluster runs may leave it empty.
func DefaultControllers(mgr ctrl.Manager, opts Options) ([]Setupper, error) {
	if opts.Namespace == "" && impersonation.RunningInCluster() {
		return nil, impersonation.ErrNamespaceRequiredInCluster
	}
	adapterRegistry, err := BuildAdapterRegistry(mgr.GetLogger().WithName("adapter-registry"), BuiltInAdapters()...)
	if err != nil {
		return nil, err
	}
	// Resolve a reconciler tracer from the global provider Run installed. When
	// tracing is disabled the provider is a no-op, so this is effectively free.
	tracer := otel.Tracer(controller.TracerScope)
	return []Setupper{
		&controller.AddonCheckReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			Adapters:     adapterRegistry,
			ProbeImage:   opts.ProbeImage,
			Tracer:       tracer,
			AddonClients: impersonation.New(mgr),
			Namespace:    opts.Namespace,
		},
		&controller.HealthCheckReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Tracer: tracer},
		&controller.ClusterHealthReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Tracer: tracer},
		&controller.NodeCertificateCheckReconciler{
			Client:         mgr.GetClient(),
			Scheme:         mgr.GetScheme(),
			NodeAgentImage: opts.NodeAgentImage,
			Tracer:         tracer,
		},
	}, nil
}

// BuildAdapterRegistry constructs the in-process registry used by AddonCheck
// reconciliation and registers every compiled-in adapter.
func BuildAdapterRegistry(logger logr.Logger, adapters ...adapter.Adapter) (*registry.Registry, error) {
	adapterRegistry := registry.New(logger)
	for _, a := range adapters {
		if err := adapterRegistry.Register(a); err != nil {
			return nil, fmt.Errorf("register built-in adapter %q: %w", adapterName(a), err)
		}
		metrics.AdapterRegistered.WithLabelValues(a.Name()).Set(1)
	}
	return adapterRegistry, nil
}

func adapterName(a adapter.Adapter) string {
	if a == nil {
		return "<nil>"
	}
	return a.Name()
}

// BuiltInAdapters returns the set of compiled-in adapters the operator ships,
// in a stable order. It is the single source of truth for "which adapters exist"
// — consumed both at startup (BuildAdapterRegistry) and by the RBAC generator and
// its read-only guard (internal/adapter/rbacgen), so a newly added adapter cannot
// ship without a generated per-addon role (SKA-58).
func BuiltInAdapters() []adapter.Adapter {
	return []adapter.Adapter{certmanager.New(), coredns.New(), kubestatemetrics.New(), nodelocaldns.New(), declarative.NewExternalSecretsEngine(), declarative.NewCiliumEngine(), declarative.NewExternalDNSEngine(), declarative.NewMetricsServerEngine(), declarative.NewEnvoyGatewayEngine(), declarative.NewIstioEngine(), declarative.NewKedaEngine(), declarative.NewVpaEngine(), declarative.NewDeschedulerEngine(), declarative.NewKuredEngine(), declarative.NewArgoCDEngine(), declarative.NewAzureWorkloadIdentityEngine()}
}

// managerFactory builds a manager from a rest.Config and ctrl.Options. It is a
// variable so tests can override it without standing up envtest.
var managerFactory = func(cfg *rest.Config, opts ctrl.Options) (ctrl.Manager, error) {
	return ctrl.NewManager(cfg, opts)
}

// operatorVersion returns the operator's build version for the tracing
// service.version resource attribute. It reads the main module version embedded
// by the Go toolchain, falling back to "dev" for ad-hoc local builds — which
// the toolchain reports as "(devel)" (or "" when build info is unavailable).
func operatorVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

// Run starts the operator. It blocks until ctx is cancelled or the manager
// returns. cfg is the Kubernetes REST config to talk to the API server;
// controllersFor returns the reconcilers to register against the built manager
// (production callers pass nil to use DefaultControllers).
func Run(
	ctx context.Context,
	cfg *rest.Config,
	opts Options,
	controllersFor func(ctrl.Manager) ([]Setupper, error),
) error {
	if cfg == nil {
		return errors.New("rest.Config must not be nil")
	}
	if controllersFor == nil {
		controllersFor = func(mgr ctrl.Manager) ([]Setupper, error) {
			return DefaultControllers(mgr, opts)
		}
	}
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts.Zap)))
	setupLog := ctrl.Log.WithName("setup")

	// Install the global tracer provider before controllers and adapters are
	// constructed so the tracers they obtain from it are wired correctly. When
	// tracing is disabled this installs a no-op provider with no exporter.
	tracingShutdown, err := tracing.Init(ctx, tracing.Config{
		Enabled:        opts.Tracing.Enabled,
		OTLPEndpoint:   opts.Tracing.OTLPEndpoint,
		SamplingRatio:  opts.Tracing.SamplingRatio,
		Insecure:       opts.Tracing.Insecure,
		ServiceVersion: operatorVersion(),
	})
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), tracingShutdownTimeout)
		defer cancel()
		if err := tracingShutdown(shutdownCtx); err != nil {
			setupLog.Error(err, "tracing provider shutdown failed")
		}
	}()

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

	controllers, err := controllersFor(mgr)
	if err != nil {
		return fmt.Errorf("build controllers: %w", err)
	}
	for _, s := range controllers {
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

	// Reap probe pods orphaned by a previous operator crash (#163). The
	// sweeper needs the live API reader: the manager cache has no Pod
	// informer, and listing pods through the cached client would open an
	// unfiltered cluster-wide watch (see scopedCacheOptions).
	if err := mgr.Add(&probe.Sweeper{
		Reader: mgr.GetAPIReader(),
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("probe-sweeper"),
	}); err != nil {
		return fmt.Errorf("add probe sweeper to manager: %w", err)
	}

	var cacheSynced atomic.Bool
	go func() {
		if mgr.GetCache().WaitForCacheSync(ctx) {
			cacheSynced.Store(true)
		}
	}()

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", readyzCheck(&cacheSynced)); err != nil {
		return fmt.Errorf("add readyz check: %w", err)
	}

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager exited with error: %w", err)
	}
	return nil
}
