/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type uninstallOptions struct {
	purgeCRDs       bool
	export          string
	deleteNamespace bool
	yes             bool
	timeout         time.Duration
}

func newUninstallCommand(f *factory) *cobra.Command {
	opts := &uninstallOptions{}
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Fathom and everything its operator started",
		Long: `uninstall removes the Fathom Helm release and every object the operator
created at run time — node-agent DaemonSets, probe Pods (including unowned
ones in kube-system), per-check ServiceAccounts, Roles, RoleBindings,
NetworkPolicies and node-report ConfigMaps, the shared admission policy and
binding, and the leader-election Lease. Every CRD and every Fathom resource
is kept, so a later install picks them up again; the namespace is kept too.

--purge-crds also deletes every Fathom resource and CRD, after showing how many
of each kind will go and asking for confirmation (--yes skips the prompt;
without a terminal, --yes is required). --export <file> first writes every
resource to a file you can kubectl apply after a reinstall; nothing is
deleted if writing it fails. --delete-namespace deletes the release
namespace afterwards, listing and confirming anything in it Fathom did not
create. Declining the confirmation changes nothing at all.

uninstall works with any fathomctl version: it needs no chart.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUninstall(commandContext(cmd), cmd, f, opts)
		},
	}
	fs := cmd.Flags()
	fs.BoolVar(&opts.purgeCRDs, "purge-crds", false, "Also delete every Fathom resource and CRD.")
	fs.StringVar(&opts.export, "export", "", "With --purge-crds, write every Fathom resource to this file first.")
	fs.BoolVar(&opts.deleteNamespace, "delete-namespace", false, "Delete the release namespace after the operator is removed.")
	fs.BoolVarP(&opts.yes, "yes", "y", false, "Skip the confirmation prompt.")
	fs.DurationVar(&opts.timeout, "timeout", defaultLifecycleTimeout, "Bound on every wait (object and CRD removal).")
	return cmd
}

func runUninstall(ctx context.Context, cmd *cobra.Command, f *factory, opts *uninstallOptions) error {
	if opts.export != "" && !opts.purgeCRDs {
		return usageError{errors.New("--export only applies with --purge-crds: a plain uninstall keeps every resource")}
	}
	if opts.timeout <= 0 {
		return usageError{fmt.Errorf("--timeout must be positive (got %s)", opts.timeout)}
	}
	c, err := f.client()
	if err != nil {
		return err
	}
	rc, err := f.newReleaseClient(f)
	if err != nil {
		return err
	}
	in, err := detectInstall(ctx, c, rc, f.opts.namespace)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	switch in.ManagedBy {
	case managedByHelm:
	case managedByNone:
		crds, err := fathomCRDs(ctx, c)
		if err != nil {
			return err
		}
		// An earlier uninstall that stopped after removing the release can
		// leave operator-created objects behind; a rerun must finish the job
		// (FR-030), not report that there is nothing to remove.
		leftovers, err := operatorCreatedObjects(ctx, c, nil)
		if err != nil {
			return err
		}
		if len(leftovers) == 0 && (!opts.purgeCRDs || len(crds) == 0) {
			_, _ = fmt.Fprintln(out, "Fathom is not installed; nothing to remove.")
			return nil
		}
		if len(leftovers) > 0 && !opts.purgeCRDs {
			_, _ = fmt.Fprintf(out, "Fathom is not installed, but %d operator-created object(s) remain from an earlier uninstall; removing them.\n", len(leftovers))
		}
	default:
		return notHelmRefusal(in)
	}
	// The preflight runs even when no release namespace is known (a rerun
	// after the release was removed): the cluster-wide cleanup and a purge
	// must not start and then stop part-way on a missing permission.
	ns := in.Namespace
	scope := permissionScope{purgeCRDs: opts.purgeCRDs, deleteNamespace: opts.deleteNamespace && ns != ""}
	if err := preflightPermissions(ctx, f, c, lifecyclePermissions("uninstall", ns, scope)); err != nil {
		return err
	}

	// Everything that needs confirming is gathered before anything changes.
	var resources map[string][]unstructured.Unstructured
	if opts.purgeCRDs {
		if resources, err = fathomResources(ctx, c); err != nil {
			return err
		}
	}
	var inv *namespaceInventory
	if opts.deleteNamespace && ns != "" {
		if inv, err = inventoryNamespace(ctx, f, c, ns, in); err != nil {
			return err
		}
	}
	if prompt := confirmationText(opts, resources, inv, ns); prompt != "" && !opts.yes {
		if !f.isTerminal() {
			return refusal{
				Reason: "confirmation-required",
				Detail: "this uninstall deletes data and no terminal is available to confirm it:\n" + prompt,
				Next:   "rerun with --yes",
			}
		}
		if !confirmPrompt(f.stdin, cmd.ErrOrStderr(), prompt) {
			return errors.New("aborted: not confirmed; nothing was changed")
		}
	}
	if opts.export != "" {
		n, err := exportResources(opts.export, resources)
		if err != nil {
			return fmt.Errorf("%w; nothing was deleted", err)
		}
		_, _ = fmt.Fprintf(out, "Exported %d Fathom resource(s) to %s.\n", n, opts.export)
	}

	if in.ManagedBy == managedByHelm {
		if in.pending() {
			_, _ = fmt.Fprintf(out, "Release %s in %s is %s; uninstalling it anyway.\n", in.Release, ns, in.ReleaseStatus)
		}
		if err := rc.Uninstall(ctx, in.Release, ns, opts.timeout); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Removed release %s from %s.\n", in.Release, ns)
		// Helm deletes in the background; a terminating manager can still
		// reconcile and recreate what we are about to remove.
		if err := waitManagerGone(ctx, f, c, ns, in.Release, opts.timeout); err != nil {
			return err
		}
	}
	n, err := deleteOperatorCreated(ctx, f, c, in, opts.timeout)
	if err != nil {
		return fmt.Errorf("remove operator-created objects: %w", err)
	}
	_, _ = fmt.Fprintf(out, "Removed %d operator-created object(s).\n", n)

	if opts.purgeCRDs {
		if err := purgeResources(ctx, f, c, resources, opts.timeout); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Deleted %d Fathom resource(s) and every fathom.skaphos.io CRD.\n", countResources(resources))
	} else {
		_, _ = fmt.Fprintln(out, "Kept every Fathom CRD and resource; a later install picks them up.")
	}

	switch {
	case ns == "":
	case opts.deleteNamespace:
		if err := deleteAndWait(ctx, f, c, []client.Object{&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}}, opts.timeout); err != nil {
			return fmt.Errorf("delete namespace %s: %w", ns, err)
		}
		_, _ = fmt.Fprintf(out, "Deleted namespace %s.\n", ns)
	default:
		_, _ = fmt.Fprintf(out, "Kept namespace %s (pass --delete-namespace to remove it).\n", ns)
	}
	return nil
}

// confirmationText describes everything this uninstall would delete that
// needs consent, as one prompt (FR-031, FR-035). Empty means no consent is
// needed.
func confirmationText(opts *uninstallOptions, resources map[string][]unstructured.Unstructured, inv *namespaceInventory, ns string) string {
	var b strings.Builder
	if opts.purgeCRDs {
		_, _ = fmt.Fprintln(&b, "--purge-crds deletes every Fathom CRD and these resources:")
		for _, kind := range purgeKinds {
			_, _ = fmt.Fprintf(&b, "  %-24s %d\n", kind, len(resources[kind]))
		}
		if opts.export == "" {
			_, _ = fmt.Fprintln(&b, "Nothing will be saved; pass --export <file> to keep a copy you can re-apply.")
		}
	}
	if inv != nil {
		if len(inv.Foreign) > 0 {
			_, _ = fmt.Fprintf(&b, "--delete-namespace deletes namespace %s, which also holds objects Fathom did not create:\n", ns)
			for _, e := range inv.Foreign {
				_, _ = fmt.Fprintf(&b, "  %s\n", e)
			}
		}
		if len(inv.Automatic) > 0 && (len(inv.Foreign) > 0 || b.Len() > 0) {
			_, _ = fmt.Fprintf(&b, "  (and objects Kubernetes creates in every namespace: %s)\n", joinEntries(inv.Automatic))
		}
	}
	return b.String()
}

func joinEntries(es []namespaceEntry) string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.String()
	}
	return strings.Join(parts, ", ")
}

func countResources(resources map[string][]unstructured.Unstructured) int {
	n := 0
	for _, items := range resources {
		n += len(items)
	}
	return n
}

// confirmPrompt prints text and asks for a yes.
func confirmPrompt(in io.Reader, w io.Writer, text string) bool {
	_, _ = fmt.Fprint(w, text)
	_, _ = fmt.Fprint(w, "Continue? [y/N] ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
