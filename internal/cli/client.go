/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// defaultNamespace is what kubectl falls back to when the context names none.
const defaultNamespace = "default"

// factory builds Kubernetes clients from the resolved global options. Its
// function fields are the injection seams: tests replace them so no verb
// needs a kubeconfig or a live API server.
type factory struct {
	opts *globalOptions

	// clientConfig turns the global options into a deferred-loading kubeconfig
	// view. The default honours --kubeconfig, --context, $KUBECONFIG, the home
	// directory default, and in-cluster configuration, exactly as kubectl does.
	clientConfig func(*globalOptions) clientcmd.ClientConfig

	// newClient constructs the cache-less controller-runtime client. The
	// default is client.New; tests supply the fake client.
	newClient func(*rest.Config, client.Options) (client.Client, error)
}

func newFactory() *factory {
	return &factory{
		opts:         &globalOptions{},
		clientConfig: defaultClientConfig,
		newClient:    client.New,
	}
}

// defaultClientConfig mirrors kubectl's discovery rules. ExplicitPath wins
// over $KUBECONFIG and the home default; an empty --context keeps the file's
// current context.
func defaultClientConfig(o *globalOptions) clientcmd.ClientConfig {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if o.kubeconfig != "" {
		rules.ExplicitPath = o.kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: o.context}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
}

// newScheme registers the client-go core types plus the Fathom API so typed
// Get/List/Patch calls work for every kind the CLI addresses. It deliberately
// omits apiextensions: the CLI never reads CRD objects.
func newScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("add client-go scheme: %w", err)
	}
	if err := fathomv1alpha1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("add fathom v1alpha1 scheme: %w", err)
	}
	return s, nil
}

// restConfig loads the kubeconfig and applies the CLI-wide request bound and
// user agent. Loading is deferred until here so `fathomctl version --client`
// and `--help` never touch a kubeconfig.
func (f *factory) restConfig() (*rest.Config, error) {
	cfg, err := f.clientConfig(f.opts).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	cfg.Timeout = f.opts.requestTimeout
	cfg.UserAgent = "fathomctl/" + clientVersion()
	return cfg, nil
}

// client returns a cache-less client over the Fathom scheme. A new client per
// call is fine: verbs make a handful of requests and the rest.Config is the
// expensive part to build only once per process anyway.
func (f *factory) client() (client.Client, error) {
	cfg, err := f.restConfig()
	if err != nil {
		return nil, err
	}
	scheme, err := newScheme()
	if err != nil {
		return nil, err
	}
	c, err := f.newClient(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}
	return c, nil
}

// namespace resolves the namespace a namespaced verb reads from: an explicit
// --namespace wins, --all-namespaces yields "" (every namespace), otherwise
// the kubeconfig context's namespace, then "default".
func (f *factory) namespace() (string, error) {
	switch {
	case f.opts.allNamespaces:
		return "", nil
	case f.opts.namespace != "":
		return f.opts.namespace, nil
	}
	ns, _, err := f.clientConfig(f.opts).Namespace()
	if err != nil {
		return "", fmt.Errorf("resolve namespace from kubeconfig: %w", err)
	}
	if ns == "" {
		ns = defaultNamespace
	}
	return ns, nil
}
