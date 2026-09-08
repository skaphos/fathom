/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// defaultRequestTimeout bounds every API request the CLI makes. It is applied
// to rest.Config.Timeout, so no verb can hang on an unresponsive API server.
const defaultRequestTimeout = 30 * time.Second

// globalOptions holds the persistent flags every verb inherits. It is resolved
// once by cobra flag parsing and read through the factory.
type globalOptions struct {
	kubeconfig     string
	context        string
	namespace      string
	allNamespaces  bool
	output         outputFormat
	requestTimeout time.Duration
}

// validate rejects flag combinations that have no sensible precedence.
func (o *globalOptions) validate() error {
	var errs []error
	if o.namespace != "" && o.allNamespaces {
		errs = append(errs, errors.New("--namespace and --all-namespaces are mutually exclusive"))
	}
	if o.requestTimeout < 0 {
		errs = append(errs, fmt.Errorf("--request-timeout must not be negative (got %s)", o.requestTimeout))
	}
	return errors.Join(errs...)
}

// registerGlobalFlags binds the persistent flags onto fs. The output format
// is a pflag.Value so an unsupported value is rejected at parse time, before
// any verb runs.
func registerGlobalFlags(fs *pflag.FlagSet, o *globalOptions) {
	fs.StringVar(&o.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file. Defaults to $KUBECONFIG, then ~/.kube/config, then in-cluster configuration.")
	fs.StringVar(&o.context, "context", "",
		"The name of the kubeconfig context to use. Defaults to the current context.")
	fs.StringVarP(&o.namespace, "namespace", "n", "",
		"The namespace to read from. Defaults to the kubeconfig context's namespace. Ignored by cluster-scoped kinds.")
	fs.BoolVarP(&o.allNamespaces, "all-namespaces", "A", false,
		"List checks across all namespaces. Cannot be combined with --namespace.")
	fs.VarP(&o.output, "output", "o",
		"Output format. One of: table, json, yaml. json and yaml emit the underlying resources unmodified.")
	fs.DurationVar(&o.requestTimeout, "request-timeout", defaultRequestTimeout,
		"Timeout applied to every API request.")
}

// NewRootCommand returns the fathomctl top-level command with the production
// client factory. Verbs are registered by their own constructors as they
// land; the root itself only holds the global flags.
func NewRootCommand() *cobra.Command {
	return newRootCommand(newFactory())
}

// newRootCommand is the injectable form used by tests: the factory carries
// every seam a verb needs, so a fake client can stand in for a cluster.
func newRootCommand(f *factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fathomctl",
		Short: "Command-line client for the Fathom operator",
		Long: `fathomctl reads and drives Fathom health checks from the command line.

It shows the verdict of every check the operator publishes, explains why a
check has the verdict it has, walks the report history, and asks the operator
to validate a check again right now. It reads the same status the operator
writes and never bypasses it.

Cluster access follows kubectl: --kubeconfig, then $KUBECONFIG, then
~/.kube/config, then in-cluster configuration. Exit codes follow kubectl too:
0 on success, 1 on any error.`,
		// Setup failures aren't usage problems; don't dump usage on RunE errors.
		SilenceUsage: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return f.opts.validate()
		},
	}
	registerGlobalFlags(cmd.PersistentFlags(), f.opts)
	return cmd
}
