/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// stubClientConfig is a clientcmd.ClientConfig that never touches disk.
type stubClientConfig struct {
	cfg       *rest.Config
	cfgErr    error
	namespace string
	nsErr     error
}

func (s stubClientConfig) RawConfig() (clientcmdapi.Config, error) { return clientcmdapi.Config{}, nil }
func (s stubClientConfig) ClientConfig() (*rest.Config, error)     { return s.cfg, s.cfgErr }
func (s stubClientConfig) Namespace() (string, bool, error)        { return s.namespace, false, s.nsErr }
func (s stubClientConfig) ConfigAccess() clientcmd.ConfigAccess    { return nil }

// stubFactory returns a factory whose loader and client constructor are
// replaced: the loader yields cfg, the constructor records the options it
// was handed and returns a fake client over the Fathom scheme.
func stubFactory(t *testing.T, cfg stubClientConfig, objs ...client.Object) (*factory, *client.Options) {
	t.Helper()
	captured := &client.Options{}
	f := newFactory()
	f.clientConfig = func(*globalOptions) clientcmd.ClientConfig { return cfg }
	f.newClient = func(_ *rest.Config, o client.Options) (client.Client, error) {
		*captured = o
		return fake.NewClientBuilder().WithScheme(o.Scheme).WithObjects(objs...).Build(), nil
	}
	return f, captured
}

func TestFactory_RestConfigAppliesTimeoutAndUserAgent(t *testing.T) {
	f, _ := stubFactory(t, stubClientConfig{cfg: &rest.Config{Host: "https://example"}})
	f.opts.requestTimeout = 7 * time.Second

	cfg, err := f.restConfig()
	if err != nil {
		t.Fatalf("restConfig: %v", err)
	}
	if cfg.Timeout != 7*time.Second {
		t.Errorf("Timeout = %s, want 7s", cfg.Timeout)
	}
	if !strings.HasPrefix(cfg.UserAgent, "fathomctl/") {
		t.Errorf("UserAgent = %q, want fathomctl/<version>", cfg.UserAgent)
	}
}

func TestFactory_RestConfigWrapsLoaderError(t *testing.T) {
	boom := errors.New("no kubeconfig here")
	f, _ := stubFactory(t, stubClientConfig{cfgErr: boom})
	_, err := f.restConfig()
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "load kubeconfig") {
		t.Fatalf("restConfig error = %v, want wrapped %v", err, boom)
	}
}

// TestFactory_ClientUsesFathomScheme proves the scheme handed to the client
// constructor knows both the Fathom kinds and the core types, so typed reads
// of checks and of the operator Deployment both work.
func TestFactory_ClientUsesFathomScheme(t *testing.T) {
	f, captured := stubFactory(t, stubClientConfig{cfg: &rest.Config{}})
	if _, err := f.client(); err != nil {
		t.Fatalf("client: %v", err)
	}
	if captured.Scheme == nil {
		t.Fatal("client.Options.Scheme was nil")
	}
	for _, gvk := range []struct{ group, version, kind string }{
		{fathomv1alpha1.GroupVersion.Group, fathomv1alpha1.GroupVersion.Version, "HealthCheck"},
		{fathomv1alpha1.GroupVersion.Group, fathomv1alpha1.GroupVersion.Version, "DNSCheck"},
		{corev1.SchemeGroupVersion.Group, corev1.SchemeGroupVersion.Version, "Pod"},
	} {
		if !captured.Scheme.Recognizes(fathomv1alpha1.GroupVersion.WithKind(gvk.kind)) &&
			!captured.Scheme.Recognizes(corev1.SchemeGroupVersion.WithKind(gvk.kind)) {
			t.Errorf("scheme does not recognise %s/%s %s", gvk.group, gvk.version, gvk.kind)
		}
	}
}

func TestFactory_ClientWrapsConstructorError(t *testing.T) {
	f, _ := stubFactory(t, stubClientConfig{cfg: &rest.Config{}})
	boom := errors.New("dial failed")
	f.newClient = func(*rest.Config, client.Options) (client.Client, error) { return nil, boom }
	_, err := f.client()
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "build kubernetes client") {
		t.Fatalf("client error = %v, want wrapped %v", err, boom)
	}
}

func TestFactory_NamespaceResolution(t *testing.T) {
	tests := []struct {
		name     string
		opts     globalOptions
		cfg      stubClientConfig
		want     string
		wantErr  bool
		errMatch string
	}{
		{name: "explicit flag wins", opts: globalOptions{namespace: "flag"}, cfg: stubClientConfig{namespace: "ctx"}, want: "flag"},
		{name: "all namespaces is empty", opts: globalOptions{allNamespaces: true}, cfg: stubClientConfig{namespace: "ctx"}, want: ""},
		{name: "context namespace", cfg: stubClientConfig{namespace: "ctx"}, want: "ctx"},
		{name: "context without namespace falls back to default", cfg: stubClientConfig{}, want: defaultNamespace},
		{name: "loader error is wrapped", cfg: stubClientConfig{nsErr: errors.New("bad file")}, wantErr: true, errMatch: "resolve namespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := stubFactory(t, tt.cfg)
			*f.opts = tt.opts
			got, err := f.namespace()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), tt.errMatch) {
					t.Fatalf("namespace() error = %v, want containing %q", err, tt.errMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("namespace(): %v", err)
			}
			if got != tt.want {
				t.Fatalf("namespace() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDefaultClientConfig_RealKubeconfig exercises the production loader
// against a temporary kubeconfig with two contexts, offline: --kubeconfig is
// honoured as the explicit path, --context overrides the current context,
// and the context namespace and server flow through.
func TestDefaultClientConfig_RealKubeconfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	kubeconfig := `apiVersion: v1
kind: Config
current-context: one
clusters:
- name: c1
  cluster: {server: "https://one.example"}
- name: c2
  cluster: {server: "https://two.example"}
users:
- name: u
  user: {token: t}
contexts:
- name: one
  context: {cluster: c1, user: u, namespace: team-one}
- name: two
  context: {cluster: c2, user: u, namespace: team-two}
`
	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	// Make sure the ambient environment cannot leak into the test.
	t.Setenv("KUBECONFIG", "")

	t.Run("explicit path and current context", func(t *testing.T) {
		cc := defaultClientConfig(&globalOptions{kubeconfig: path})
		ns, _, err := cc.Namespace()
		if err != nil || ns != "team-one" {
			t.Fatalf("Namespace() = %q, %v; want team-one", ns, err)
		}
		cfg, err := cc.ClientConfig()
		if err != nil || cfg.Host != "https://one.example" {
			t.Fatalf("ClientConfig() host = %q, %v; want https://one.example", cfg.Host, err)
		}
	})

	t.Run("context override", func(t *testing.T) {
		cc := defaultClientConfig(&globalOptions{kubeconfig: path, context: "two"})
		ns, _, err := cc.Namespace()
		if err != nil || ns != "team-two" {
			t.Fatalf("Namespace() = %q, %v; want team-two", ns, err)
		}
		cfg, err := cc.ClientConfig()
		if err != nil || cfg.Host != "https://two.example" {
			t.Fatalf("ClientConfig() host = %q, %v; want https://two.example", cfg.Host, err)
		}
	})

	t.Run("KUBECONFIG env is honoured when no flag is given", func(t *testing.T) {
		t.Setenv("KUBECONFIG", path)
		cc := defaultClientConfig(&globalOptions{})
		cfg, err := cc.ClientConfig()
		if err != nil || cfg.Host != "https://one.example" {
			t.Fatalf("ClientConfig() host = %q, %v; want https://one.example", cfg.Host, err)
		}
	})
}

func TestNewScheme(t *testing.T) {
	s, err := newScheme()
	if err != nil {
		t.Fatalf("newScheme: %v", err)
	}
	for _, kind := range []string{"AddonCheck", "DNSCheck", "NodeCertificateCheck", "HealthCheck", "ClusterHealth", "HealthReport"} {
		if !s.Recognizes(fathomv1alpha1.GroupVersion.WithKind(kind)) {
			t.Errorf("scheme missing %s", kind)
		}
	}
}
