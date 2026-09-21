/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"

	api "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/spf13/cobra"
)

type commandError struct {
	code int
	err  error
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }

// ExitCode preserves ordinary CLI errors while exposing preflight's documented
// collision (1) versus unverifiable/read failure (2) distinction.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded *commandError
	if errors.As(err, &coded) {
		return coded.code
	}
	return 1
}

// BuildRevision may be stamped by release tooling; normal builds use Go VCS data.
var BuildRevision string

func releaseInfo() (string, string) {
	revision := BuildRevision
	if revision == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			revision = buildSetting(info, "vcs.revision")
		}
	}
	return clientVersion(), revision
}

func newDefinitionCollisionsCommand(f *factory) *cobra.Command {
	return &cobra.Command{Use: "collisions", Short: "Check live definitions against this release's bundled inventory", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		version, build := f.definitionReleaseInfo()
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "fathomctl version=%s build=%s\n", version, build); err != nil {
			return &commandError{2, err}
		}
		if version == "" || strings.Contains(version, "devel") || version == "unknown" || build == "" || build == "unknown" {
			return &commandError{2, fmt.Errorf("target-release version and build metadata are required")}
		}
		names := f.definitionInventory()
		if len(names) == 0 {
			return &commandError{2, fmt.Errorf("bundled built-in inventory is unavailable")}
		}
		builtins := make(map[string]bool, len(names))
		for _, name := range names {
			builtins[name] = true
		}
		c, err := f.client()
		if err != nil {
			return &commandError{2, err}
		}
		var list api.AddonDefinitionList
		if err := c.List(commandContext(cmd), &list); err != nil {
			return &commandError{2, err}
		}
		var collisions []string
		for _, d := range list.Items {
			if builtins[d.Name] || builtins[string(d.Spec.AddonType)] {
				collisions = append(collisions, d.Name)
			}
		}
		sort.Strings(collisions)
		for _, name := range collisions {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "collision: %s\n", name); err != nil {
				return &commandError{2, err}
			}
		}
		if len(collisions) > 0 {
			return &commandError{1, fmt.Errorf("%d built-in collision(s)", len(collisions))}
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "No built-in collisions.")
		return err
	}}
}
