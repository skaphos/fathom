/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// NewRootCommand returns the operator's top-level cobra command. The returned
// command's RunE starts the controller manager when invoked.
//
// All operator flags are persistent so future subcommands inherit them.
func NewRootCommand() *cobra.Command {
	var (
		configFile     string
		configExplicit bool
		zapOpts        zap.Options
	)

	cmd := &cobra.Command{
		Use:   "fathom",
		Short: "Fathom Kubernetes operator",
		Long:  "Fathom is a Kubernetes operator that reconciles HealthCheck and ClusterHealth resources.",
		// Disable cobra's automatic usage dump on RunE errors; setup failures
		// aren't usage problems.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configExplicit = cmd.Flags().Changed("config")

			opts, err := Load(cmd.Flags(), zapOpts, configFile, configExplicit)
			if err != nil {
				return fmt.Errorf("load options: %w", err)
			}

			cfg, err := ctrl.GetConfig()
			if err != nil {
				return fmt.Errorf("load kubeconfig: %w", err)
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, stop := signalContext(ctx)
			defer stop()
			return Run(ctx, cfg, opts, nil)
		},
	}

	cmd.PersistentFlags().StringVar(&configFile, "config", DefaultConfigPath,
		"Path to a YAML/JSON/TOML config file. Missing default-path files are ignored; explicit ones must exist.")
	RegisterFlags(cmd.PersistentFlags(), &zapOpts)

	return cmd
}

// signalContext is a thin wrapper over signal.NotifyContext that the operator
// RunE uses to merge parent cancellation with the conventional shutdown
// signals (SIGINT/SIGTERM). Factored out as the testable seam for the
// startup/shutdown wiring; SKA-300 traded the hand-rolled goroutine for
// the stdlib primitive but kept this seam so tests can exercise the
// parent-cancel and signal-cancel handoff without driving NewRootCommand
// end-to-end (which would need a kubeconfig).
//
// Behavior delta vs the prior ctrl.SetupSignalHandler wrapper: a second
// signal does not hard-exit the process while the manager shuts down.
// Kubernetes SIGKILLs after terminationGracePeriodSeconds if shutdown
// stalls, so the impatient-Ctrl+C escape hatch isn't load-bearing for
// production.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
