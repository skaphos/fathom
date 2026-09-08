/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// summaryColumnWidth bounds the SUMMARY column so one verbose check cannot
// push every other column off the terminal. The full text is always
// available through describe and the structured formats.
const summaryColumnWidth = 80

type lsOptions struct {
	selector string
}

// lsRow is one listed check with its normalised verdict.
type lsRow struct {
	kind *kindDescriptor
	obj  client.Object
	snap snapshot
}

func newLsCommand(f *factory) *cobra.Command {
	opts := &lsOptions{}
	cmd := &cobra.Command{
		Use:   "ls [kind]",
		Short: "List checks with their current verdict",
		Long: `ls lists Fathom checks with the verdict the operator last published.

With no kind it lists every kind, grouped: AddonCheck, DNSCheck,
NodeCertificateCheck, HealthCheck, then ClusterHealth. ClusterHealth is
cluster-scoped and is always included regardless of --namespace. With a kind
(any of its spellings, e.g. dnschecks or dns) only that kind is listed.

-o json and -o yaml emit the resources unmodified inside a List, so the output
composes with jq and yq the way kubectl get -o json does.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLs(cmd, f, opts, args)
		},
	}
	cmd.Flags().StringVarP(&opts.selector, "selector", "l", "", "Only list checks matching this label selector.")
	return cmd
}

func runLs(cmd *cobra.Command, f *factory, opts *lsOptions, args []string) error {
	ctx := commandContext(cmd)
	c, err := f.client()
	if err != nil {
		return err
	}
	ns, err := f.namespace()
	if err != nil {
		return err
	}
	selected := kinds
	single := false
	if len(args) == 1 {
		k, err := parseKind(args[0])
		if err != nil {
			return err
		}
		selected = []*kindDescriptor{k}
		single = true
	}
	selOpt, err := selectorListOption(opts.selector)
	if err != nil {
		return err
	}

	var rows []lsRow
	for _, k := range selected {
		list := k.NewList()
		listOpts := []client.ListOption{}
		if selOpt != nil {
			listOpts = append(listOpts, selOpt)
		}
		if k.Namespaced {
			listOpts = append(listOpts, client.InNamespace(ns))
		}
		if err := c.List(ctx, list, listOpts...); err != nil {
			return fmt.Errorf("list %s: %w", k.Resource, err)
		}
		items := k.Items(list)
		sort.Slice(items, func(i, j int) bool {
			if items[i].GetNamespace() != items[j].GetNamespace() {
				return items[i].GetNamespace() < items[j].GetNamespace()
			}
			return items[i].GetName() < items[j].GetName()
		})
		for _, o := range items {
			rows = append(rows, lsRow{kind: k, obj: o, snap: k.Snapshot(o)})
		}
	}

	out := cmd.OutOrStdout()
	if f.opts.output.structured() {
		objs := make([]client.Object, 0, len(rows))
		for _, r := range rows {
			objs = append(objs, r.obj)
		}
		list, err := newObjectList(c.Scheme(), objs)
		if err != nil {
			return err
		}
		return encode(out, f.opts.output, list)
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(out, emptyListMessage(selected, single, ns, f.opts.allNamespaces))
		return err
	}
	return renderLsTable(out, rows, !single, f.opts.allNamespaces, time.Now())
}

// selectorListOption parses a -l value into a list option, or nil when
// empty.
func selectorListOption(selector string) (client.ListOption, error) {
	if selector == "" {
		return nil, nil
	}
	sel, err := labels.Parse(selector)
	if err != nil {
		return nil, fmt.Errorf("invalid selector %q: %w", selector, err)
	}
	return client.MatchingLabelsSelector{Selector: sel}, nil
}

func emptyListMessage(selected []*kindDescriptor, single bool, ns string, allNamespaces bool) string {
	what := "checks"
	if single {
		what = selected[0].Resource
		if !selected[0].Namespaced {
			return fmt.Sprintf("No %s found.", what)
		}
	}
	if allNamespaces {
		return fmt.Sprintf("No %s found in any namespace.", what)
	}
	return fmt.Sprintf("No %s found in namespace %s.", what, ns)
}

func renderLsTable(w io.Writer, rows []lsRow, grouped, allNamespaces bool, now time.Time) error {
	tb := newTable(w)
	var header []string
	if grouped {
		header = append(header, "KIND")
	}
	if allNamespaces {
		header = append(header, "NAMESPACE")
	}
	header = append(header, "NAME", "VERDICT", "SUMMARY", "LAST RUN", "NEXT RUN")
	tb.row(header...)
	for _, r := range rows {
		var cells []string
		if grouped {
			cells = append(cells, r.kind.Kind)
		}
		if allNamespaces {
			cells = append(cells, r.obj.GetNamespace())
		}
		cells = append(cells,
			r.obj.GetName(),
			orDash(string(r.snap.Verdict)),
			orDash(truncate(r.snap.Summary, summaryColumnWidth)),
			age(r.snap.LastRun, now),
			until(r.snap.NextRun, now),
		)
		tb.row(cells...)
	}
	return tb.flush()
}
