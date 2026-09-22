/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package app

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/internal/nodecert"
	"github.com/skaphos/fathom/pkg/adapter"
)

type appFakeAdapter struct{}

func (appFakeAdapter) Name() string            { return "fake-cert-manager" }
func (appFakeAdapter) Version() string         { return "0.1.0" }
func (appFakeAdapter) ContractVersion() string { return adapter.ContractVersion }
func (appFakeAdapter) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{AddonTypes: []string{"cert-manager"}, Families: []adapter.Family{"system_health"}}
}
func (appFakeAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{}, nil
}

// writeSelfSignedCert writes a tls.crt + tls.key pair into dir. The cert is
// minimal but valid PEM, which is enough for certwatcher.New to accept.
func writeSelfSignedCert(t *testing.T, dir string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tls.key"), keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func TestNewScheme_RegistersFathomTypes(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	// AddonCheck exercises the newest API type; if it is registered the package
	// init hooks are being included in the application scheme.
	gvks := scheme.AllKnownTypes()
	found := false
	for gvk := range gvks {
		if gvk.Group == "fathom.skaphos.io" && gvk.Kind == "AddonCheck" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AddonCheck not registered in scheme")
	}
}

// TestNewScheme_RegistersAPIExtensions guards the CRD-presence checks in the
// cert-manager and external-secrets adapters, which Get CustomResourceDefinition
// objects. envtest auto-registers apiextensions/v1 so unit tests against the
// adapters pass even without explicit registration; real-cluster reconciles
// surface "no kind is registered for the type v1.CustomResourceDefinition" if
// NewScheme drops the AddToScheme call below.
func TestNewScheme_RegistersAPIExtensions(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	gvks := scheme.AllKnownTypes()
	found := false
	for gvk := range gvks {
		if gvk.Group == "apiextensions.k8s.io" && gvk.Version == "v1" && gvk.Kind == "CustomResourceDefinition" {
			found = true
			break
		}
	}
	if !found {
		t.Error("apiextensions.k8s.io/v1 CustomResourceDefinition not registered in scheme")
	}
}

func TestBuildManagerOptions_DefaultsHaveNoCertWatchers(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	mgrOpts, watchers, err := BuildManagerOptions(DefaultOptions(), scheme)
	if err != nil {
		t.Fatalf("BuildManagerOptions: %v", err)
	}
	if len(watchers) != 0 {
		t.Errorf("watchers: got %d, want 0", len(watchers))
	}
	if mgrOpts.Metrics.BindAddress != "0" {
		t.Errorf("Metrics.BindAddress: got %q, want 0", mgrOpts.Metrics.BindAddress)
	}
	if !mgrOpts.Metrics.SecureServing {
		t.Error("Metrics.SecureServing should be true by default")
	}
	if mgrOpts.Metrics.FilterProvider == nil {
		t.Error("FilterProvider should be set when Secure=true")
	}
	if mgrOpts.HealthProbeBindAddress != ":8081" {
		t.Errorf("HealthProbeBindAddress: got %q, want :8081", mgrOpts.HealthProbeBindAddress)
	}
}

// TestBuildManagerOptions_ScopesCacheByManagedByLabel is the regression guard
// for SKA-581 / #164: the manager cache must restrict ConfigMap, DaemonSet,
// RoleBinding, and NetworkPolicy informers to Fathom-managed objects
// (managed-by=fathom) so a cluster-wide watch cannot OOM the operator, while
// ServiceAccount must stay unfiltered (AddonCheck lists per-addon SAs that
// never carry that label).
func TestBuildManagerOptions_ScopesCacheByManagedByLabel(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	mgrOpts, _, err := BuildManagerOptions(DefaultOptions(), scheme)
	if err != nil {
		t.Fatalf("BuildManagerOptions: %v", err)
	}
	if mgrOpts.Cache.ByObject == nil {
		t.Fatal("Cache.ByObject is nil; expected scoped selectors")
	}

	byType := map[string]cache.ByObject{}
	for obj, cfg := range mgrOpts.Cache.ByObject {
		switch obj.(type) {
		case *corev1.ConfigMap:
			byType["ConfigMap"] = cfg
		case *appsv1.DaemonSet:
			byType["DaemonSet"] = cfg
		case *rbacv1.RoleBinding:
			byType["RoleBinding"] = cfg
		case *networkingv1.NetworkPolicy:
			byType["NetworkPolicy"] = cfg
		case *corev1.ServiceAccount:
			byType["ServiceAccount"] = cfg
		default:
			t.Errorf("unexpected type scoped in cache: %T", obj)
		}
	}

	managed := labels.Set{nodecert.LabelManagedBy: nodecert.ManagedByValue}
	for _, kind := range []string{"ConfigMap", "DaemonSet", "RoleBinding", "NetworkPolicy"} {
		cfg, ok := byType[kind]
		if !ok {
			t.Errorf("%s: not scoped in Cache.ByObject; expected a managed-by=fathom selector", kind)
			continue
		}
		if cfg.Label == nil {
			t.Errorf("%s: Label selector is nil", kind)
			continue
		}
		if !cfg.Label.Matches(managed) {
			t.Errorf("%s: selector does not match a managed-by=fathom object", kind)
		}
		if cfg.Label.Matches(labels.Set{}) {
			t.Errorf("%s: selector matches an unlabeled object; would still cache the whole cluster", kind)
		}
	}

	if _, ok := byType["ServiceAccount"]; ok {
		t.Error("ServiceAccount must not be label-scoped: AddonCheck lists addon SAs that lack managed-by=fathom")
	}
}

// compile-time assurance the selector map key type stays client.Object.
var _ map[client.Object]cache.ByObject = cache.Options{}.ByObject

func TestBuildManagerOptions_InsecureMetricsHasNoFilter(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	opts := DefaultOptions()
	opts.Metrics.Secure = false

	mgrOpts, _, err := BuildManagerOptions(opts, scheme)
	if err != nil {
		t.Fatalf("BuildManagerOptions: %v", err)
	}
	if mgrOpts.Metrics.SecureServing {
		t.Error("SecureServing should be false")
	}
	if mgrOpts.Metrics.FilterProvider != nil {
		t.Error("FilterProvider should be nil when Secure=false")
	}
}

func TestBuildManagerOptions_CertWatchers(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}

	dir := t.TempDir()
	writeSelfSignedCert(t, dir)

	opts := DefaultOptions()
	opts.Webhook.CertPath = dir
	opts.Metrics.CertPath = dir

	_, watchers, err := BuildManagerOptions(opts, scheme)
	if err != nil {
		t.Fatalf("BuildManagerOptions: %v", err)
	}
	if len(watchers) != 2 {
		t.Errorf("watchers: got %d, want 2 (webhook + metrics)", len(watchers))
	}
}

func TestBuildManagerOptions_MissingWebhookCert_Errors(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	opts := DefaultOptions()
	opts.Webhook.CertPath = filepath.Join(t.TempDir(), "missing")

	_, _, err = BuildManagerOptions(opts, scheme)
	if err == nil {
		t.Fatal("expected error for missing webhook cert")
	}
	if !strings.Contains(err.Error(), "webhook cert watcher") {
		t.Errorf("error wrap: %v", err)
	}
}

func TestBuildAdapterRegistry_RegistersBuiltInAdapters(t *testing.T) {
	adapterRegistry, err := BuildAdapterRegistry(logr.Discard(), appFakeAdapter{})
	if err != nil {
		t.Fatalf("BuildAdapterRegistry: %v", err)
	}

	got, err := adapterRegistry.Lookup("cert-manager")
	if err != nil {
		t.Fatalf("Lookup(cert-manager): %v", err)
	}
	if got.Name() != "fake-cert-manager" {
		t.Fatalf("adapter name: got %q, want fake-cert-manager", got.Name())
	}
}

func TestBuildAdapterRegistry_WrapsRegistrationErrors(t *testing.T) {
	_, err := BuildAdapterRegistry(logr.Discard(), nil)
	if err == nil {
		t.Fatal("expected error for nil built-in adapter")
	}
	if !strings.Contains(err.Error(), "register built-in adapter") {
		t.Errorf("error wrap: %v", err)
	}
	if !strings.Contains(err.Error(), "<nil>") {
		t.Errorf("adapter identity missing from error: %v", err)
	}
}

func TestBuiltInAdapters_AllLookupable(t *testing.T) {
	adapterRegistry, err := BuildAdapterRegistry(logr.Discard(), BuiltInAdapters()...)
	if err != nil {
		t.Fatalf("BuildAdapterRegistry: %v", err)
	}
	for _, name := range []string{"cert-manager", "coredns", "kube-state-metrics", "external-secrets", "cilium", "external-dns", "metrics-server", "envoy-gateway", "istio"} {
		got, err := adapterRegistry.Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%s): %v", name, err)
		}
		if got.Name() != name {
			t.Fatalf("adapter name: got %q, want %q", got.Name(), name)
		}
	}
}

func TestReadyzCheck(t *testing.T) {
	var synced atomic.Bool

	check := readyzCheck(&synced)
	if err := check(nil); err == nil {
		t.Fatal("readyzCheck before sync: want error, got nil")
	} else if !strings.Contains(err.Error(), "informers not synced") {
		t.Errorf("readyzCheck before sync: error %q should name the unsynced state", err.Error())
	}

	synced.Store(true)
	if err := check(nil); err != nil {
		t.Fatalf("readyzCheck after sync: want nil, got %v", err)
	}
}

func TestRun_NilConfig(t *testing.T) {
	err := Run(context.Background(), nil, DefaultOptions(), nil)
	if err == nil {
		t.Fatal("expected error for nil rest.Config")
	}
}

func TestRun_InvalidOptions(t *testing.T) {
	opts := DefaultOptions()
	opts.HealthProbeBindAddress = "" // forces validation failure

	err := Run(context.Background(), &rest.Config{}, opts, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid options") {
		t.Errorf("error wrap: %v", err)
	}
}

func TestRun_ManagerCreationFailure(t *testing.T) {
	// Replace the manager factory with one that always errors so we can
	// exercise Run's error-wrapping without standing up envtest.
	original := managerFactory
	t.Cleanup(func() { managerFactory = original })
	managerFactory = func(*rest.Config, ctrl.Options) (ctrl.Manager, error) {
		return nil, errors.New("synthetic manager failure")
	}

	err := Run(context.Background(), &rest.Config{}, DefaultOptions(), nil)
	if err == nil {
		t.Fatal("expected manager creation error")
	}
	if !strings.Contains(err.Error(), "create manager") {
		t.Errorf("error wrap: %v", err)
	}
	if !strings.Contains(err.Error(), "synthetic manager failure") {
		t.Errorf("underlying error not wrapped: %v", err)
	}
}

// Runtime loading is mandatory-election, and an operator that enables it
// without leader election must still start every built-in controller:
// contracts/leadership.md, "do not fail manager startup or disable built-ins".
func TestRun_RuntimeEnabledWithoutElectionStillStartsBuiltIns(t *testing.T) {
	if envtestCfg == nil {
		t.Skip("envtest unavailable; run via `task test` for full coverage")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	opts := DefaultOptions()
	opts.Metrics.BindAddress = "0"
	opts.HealthProbeBindAddress = "0"
	opts.RuntimeLoading.Enabled = true
	opts.LeaderElect = false
	opts.Namespace = "fathom-system"

	if reason := opts.RuntimeLoadingDisabledReason(); reason != "LeaderElectionRequired" {
		t.Fatalf("reason = %q, want LeaderElectionRequired", reason)
	}
	// The controller set is supplied explicitly because controller-runtime
	// validates controller names process-wide and the built-in set is already
	// registered by TestRun_HappyPath_DefaultControllers. What this test drives
	// is Run's own runtime branch: an unavailable runtime must build no wiring,
	// install no leader-election lock of its own, and still start the manager.
	// That the built-in controller set stays untouched is proven directly by
	// TestRuntimeLoadingUnavailableRegistersNothingRuntime.
	noControllers := func(ctrl.Manager) ([]Setupper, error) { return nil, nil }
	if err := Run(ctx, envtestCfg, opts, noControllers); err != nil {
		t.Fatalf("Run: %v; an unavailable runtime must not fail startup", err)
	}
}

// lockedBuffer collects log output written by a logger that outlives the test
// goroutine, so reading it is not a data race.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// An administrator who turns runtime loading on and does not get it must be
// told why, AT STARTUP. The diagnostic cannot come from a runtime controller:
// contracts/leadership.md requires it "without relying on a leader-gated
// controller that will never start", and every prerequisite this reports is one
// that stops such a controller from ever running.
func TestRunReportsWhyRuntimeLoadingIsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		options    func(*Options)
		wantReason string
	}{
		{
			name: "enabled without leader election",
			options: func(o *Options) {
				o.RuntimeLoading.Enabled, o.LeaderElect, o.Namespace = true, false, "fathom-system"
			},
			wantReason: "LeaderElectionRequired",
		},
		{
			name: "enabled without an explicit operator namespace",
			options: func(o *Options) {
				o.RuntimeLoading.Enabled, o.LeaderElect, o.Namespace = true, true, ""
			},
			wantReason: "OperatorNamespaceRequired",
		},
		{
			// The other direction: default-off is not a problem to report, and
			// an operator that never asked for runtime loading must not be told
			// at every startup that it does not have it.
			name:    "runtime loading was never asked for",
			options: func(*Options) {},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)
			original := managerFactory
			t.Cleanup(func() { managerFactory = original })
			managerFactory = func(*rest.Config, ctrl.Options) (ctrl.Manager, error) {
				return nil, errors.New("synthetic manager failure")
			}

			logs := &lockedBuffer{}
			opts := DefaultOptions()
			opts.Zap.DestWriter = logs
			tc.options(&opts)

			if err := Run(context.Background(), &rest.Config{}, opts, nil); err == nil ||
				!strings.Contains(err.Error(), "synthetic manager failure") {
				t.Fatalf("Run: %v, want the synthetic manager failure", err)
			}

			written := logs.String()
			const diagnostic = "runtime addon loading unavailable"
			switch {
			case tc.wantReason == "" && strings.Contains(written, diagnostic):
				t.Errorf("startup reported runtime loading as unavailable although it was never enabled: %s", written)
			case tc.wantReason == "":
			case !strings.Contains(written, diagnostic):
				t.Errorf("startup never reported why runtime loading is inactive; an administrator has nothing to read. logs: %s", written)
			case !strings.Contains(written, tc.wantReason):
				t.Errorf("startup reported no %q reason; logs: %s", tc.wantReason, written)
			}
		})
	}
}

// Run must hand the manager the wiring it built ITSELF: the same session must
// both hold the manager's Lease and gate runtime dispatch, and the gate, the
// pool and the session must all reach the manager.
//
// Two mutations this closes, both of which every other test in the package
// survives: Run calling defaultControllers with a nil wiring (runtime loading
// becomes dead code in the shipped binary, however correct the wiring is), and
// Run locking the manager's Lease under a SECOND wiring's identity (AdoptLease
// refuses any Lease not held by exactly its own identity, so runtime stays
// permanently inert behind one Error log).
func TestRunWiresTheRuntimePathItBuilt(t *testing.T) {
	if envtestCfg == nil {
		t.Skip("envtest unavailable; run via `task test` for full coverage")
	}
	writeManagerToken(t, "system:serviceaccount:fathom-system:"+testManagerServiceAccount)

	opts := DefaultOptions()
	opts.Metrics.BindAddress = "0"
	opts.HealthProbeBindAddress = "0"
	opts.RuntimeLoading.Enabled = true
	opts.LeaderElect = true
	opts.Namespace = "fathom-run-wiring"

	// The elected Lease lives in the configured operator namespace.
	live, err := client.New(envtestCfg, client.Options{})
	if err != nil {
		t.Fatalf("build a direct client: %v", err)
	}
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: opts.Namespace}}
	if err := live.Create(t.Context(), namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create the operator namespace: %v", err)
	}

	original := managerFactory
	t.Cleanup(func() { managerFactory = original })
	var (
		captured ctrl.Options
		recorder *recordingManager
	)
	managerFactory = func(cfg *rest.Config, mgrOpts ctrl.Options) (ctrl.Manager, error) {
		captured = mgrOpts
		// Controller names are validated process-wide and the built-in set is
		// already registered by TestRun_HappyPath_DefaultControllers. Skipping
		// that validation is what lets Run's own controllersFor==nil branch —
		// the production path — be exercised here.
		mgrOpts.Controller.SkipNameValidation = ptr.To(true)
		mgr, err := ctrl.NewManager(cfg, mgrOpts)
		if err != nil {
			return nil, err
		}
		recorder = &recordingManager{
			Manager:   mgr,
			syncCache: &recordingCache{Cache: mgr.GetCache()},
			recClient: &recordingClient{Client: mgr.GetClient()},
		}
		return recorder, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Second)
		cancel()
	}()
	if err := Run(ctx, envtestCfg, opts, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if recorder == nil {
		t.Fatal("Run never built a manager")
	}

	// The wiring reached the manager at all: attach registers exactly these
	// three runnables, and it runs only if Run passed the wiring it built.
	var anyRuntimeRunnable bool
	for _, runnable := range recorder.runnables() {
		switch runnable.(type) {
		case *RuntimeLeadership, *runtimeDispatchGate, *runtimeWorkerPool:
			anyRuntimeRunnable = true
		}
	}
	if !anyRuntimeRunnable {
		t.Fatal("Run built a runtime wiring and gave the manager none of it: runtime loading is dead code in this binary")
	}
	session, gate, pool := runtimeRunnables(t, recorder)
	if gate.session != session || pool.session != session {
		t.Fatal("the gate and the pool are not driven by the session Run registered")
	}

	// ... and the manager holds its Lease under that same session's identity.
	lock := captured.LeaderElectionResourceLockInterface
	if lock == nil {
		t.Fatal("Run installed no leader-election lock although runtime loading is available")
	}
	if got, want := lock.Identity(), session.Identity(); got != want {
		t.Errorf("the manager holds its Lease as %q while runtime dispatch is gated by session %q; AdoptLease refuses every Lease not held by exactly its own identity, so runtime would never open", got, want)
	}

	// The gate admits on the registry the built-in path resolves through.
	for _, builtin := range BuiltInAdapters() {
		for _, addonType := range builtin.Capabilities().AddonTypes {
			if _, err := gate.registry.Resolve(addonType); err != nil {
				t.Fatalf("the registered gate admits on a registry that does not know built-in %q: %v", addonType, err)
			}
		}
	}
}
