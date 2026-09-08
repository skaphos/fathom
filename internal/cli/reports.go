/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

const defaultReportsLimit = 10

type reportsOptions struct {
	limit  int
	since  time.Duration
	report string
}

func newReportsCommand(f *factory) *cobra.Command {
	opts := &reportsOptions{}
	cmd := &cobra.Command{
		Use:   "reports <kind>/<name>",
		Short: "List a check's HealthReport history, newest first",
		Long: `reports walks the HealthReport history of a check.

Reports are written when a verdict changes, not on every interval; a gap is
not a missed run. The CHANGE column says what changed from the next-older
report: the verdict, some per-check results, or nothing.

Only checks that run themselves write reports. For a HealthCheck, reports
follows spec.checkRef to its source and says so; a ClusterHealth has no
reports of its own, and the command lists the sources to query instead.

--report <name> prints one report in full.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReports(cmd, f, opts, args)
		},
	}
	cmd.Flags().IntVar(&opts.limit, "limit", defaultReportsLimit, "Maximum number of reports to list.")
	cmd.Flags().DurationVar(&opts.since, "since", 0, "Only list reports observed within this duration (e.g. 24h).")
	cmd.Flags().StringVar(&opts.report, "report", "", "Print this one report in full instead of listing.")
	return cmd
}

func runReports(cmd *cobra.Command, f *factory, opts *reportsOptions, args []string) error {
	ctx := commandContext(cmd)
	c, err := f.client()
	if err != nil {
		return err
	}
	ns, err := f.namespace()
	if err != nil {
		return err
	}
	ref, rest, err := parseTarget(args, ns)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("unexpected argument %q", rest[0])
	}
	if opts.limit < 1 {
		return fmt.Errorf("--limit must be at least 1 (got %d)", opts.limit)
	}

	source, err := reportSource(ctx, c, ref)
	if err != nil {
		return err
	}
	if source.String() != ref.String() {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Showing reports for %s (the source behind %s)\n", source, ref)
	}
	out := cmd.OutOrStdout()

	if opts.report != "" {
		report := &fathomv1alpha1.HealthReport{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: source.Namespace, Name: opts.report}, report); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("HealthReport %q not found in namespace %s", opts.report, source.Namespace)
			}
			return fmt.Errorf("get HealthReport %s: %w", opts.report, err)
		}
		// The name is global within the namespace; make sure the report is the
		// target's before printing it under the target's banner.
		if kind, name := report.Labels[fathomv1alpha1.LabelHealthReportSourceKind], report.Labels[fathomv1alpha1.LabelHealthReportSourceName]; kind != source.Kind.Kind || name != source.Name {
			return fmt.Errorf("HealthReport %q belongs to %s/%s, not %s", opts.report, strings.ToLower(kind), name, source)
		}
		if f.opts.output.structured() {
			if err := withGVK(c.Scheme(), report); err != nil {
				return err
			}
			return encode(out, f.opts.output, report)
		}
		renderReport(out, report, time.Now())
		return nil
	}

	list := &fathomv1alpha1.HealthReportList{}
	if err := c.List(ctx, list, client.InNamespace(source.Namespace), client.MatchingLabels{
		fathomv1alpha1.LabelHealthReportSourceKind: source.Kind.Kind,
		fathomv1alpha1.LabelHealthReportSourceName: source.Name,
	}); err != nil {
		return fmt.Errorf("list HealthReports for %s: %w", source, err)
	}
	reports := make([]*fathomv1alpha1.HealthReport, 0, len(list.Items))
	for i := range list.Items {
		reports = append(reports, &list.Items[i])
	}
	sortReportsNewestFirst(reports)

	// The change column compares against the next-older report, so it is
	// computed over the full history before --since and --limit trim the view.
	changes := make(map[string]string, len(reports))
	for i, r := range reports {
		var older *fathomv1alpha1.HealthReport
		if i+1 < len(reports) {
			older = reports[i+1]
		}
		changes[r.Name] = reportChange(r, older)
	}

	now := time.Now()
	shown := reports[:0:0]
	for _, r := range reports {
		if opts.since > 0 && now.Sub(r.Spec.ObservedAt.Time) > opts.since {
			continue
		}
		shown = append(shown, r)
		if len(shown) == opts.limit {
			break
		}
	}

	if f.opts.output.structured() {
		objs := make([]client.Object, 0, len(shown))
		for _, r := range shown {
			objs = append(objs, r)
		}
		envelope, err := newObjectList(c.Scheme(), objs)
		if err != nil {
			return err
		}
		return encode(out, f.opts.output, envelope)
	}
	if len(shown) == 0 {
		if len(reports) == 0 {
			_, err := fmt.Fprintf(out, "No reports yet for %s.\n", source)
			return err
		}
		_, err := fmt.Fprintf(out, "No reports for %s within the last %s (%d older).\n", source, opts.since, len(reports))
		return err
	}
	tb := newTable(out)
	tb.row("NAME", "OBSERVED", "RESULT", "SUMMARY", "CHANGE")
	for _, r := range shown {
		observed := r.Spec.ObservedAt
		tb.row(r.Name, age(&observed, now), orDash(string(r.Spec.Result)), truncate(reportSummary(r), summaryColumnWidth), changes[r.Name])
	}
	return tb.flush()
}

// reportSource resolves the check whose history to show: executable kinds
// are their own source, a HealthCheck delegates to its checkRef, and a
// ClusterHealth is refused with the sources to query.
func reportSource(ctx context.Context, c client.Client, ref checkRef) (checkRef, error) {
	if ref.Kind.WritesReports {
		return ref, nil
	}
	obj := ref.Kind.New()
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, obj); err != nil {
		return checkRef{}, describeGetError(ref, err)
	}
	sources, err := ref.Kind.Sources(ctx, c, obj)
	if err != nil {
		return checkRef{}, err
	}
	if ref.Kind.Kind == "HealthCheck" && len(sources) == 1 {
		if sources[0].Skip != "" && !strings.Contains(sources[0].Skip, "paused") {
			return checkRef{}, fmt.Errorf("%s %s", ref, sources[0].Skip)
		}
		return sources[0].Ref, nil
	}
	names := make([]string, 0, len(sources))
	for _, s := range sources {
		if s.Skip == "" || strings.Contains(s.Skip, "paused") {
			names = append(names, s.Ref.String())
		}
	}
	sort.Strings(names)
	return checkRef{}, fmt.Errorf("%s has no reports of its own; query one of its sources: %s", ref, strings.Join(names, ", "))
}

func sortReportsNewestFirst(reports []*fathomv1alpha1.HealthReport) {
	sort.SliceStable(reports, func(i, j int) bool {
		a, b := reports[i].Spec.ObservedAt.Time, reports[j].Spec.ObservedAt.Time
		if !a.Equal(b) {
			return a.After(b)
		}
		return reports[i].Name > reports[j].Name
	})
}

// reportChange describes what a report changed relative to the next-older
// one: the verdict, some per-check results, or nothing.
func reportChange(newer, older *fathomv1alpha1.HealthReport) string {
	if older == nil {
		return "first"
	}
	if newer.Spec.Result != older.Spec.Result {
		return string(older.Spec.Result) + "→" + string(newer.Spec.Result)
	}
	changed := changedCheckCount(newer, older)
	if changed == 0 {
		return "unchanged"
	}
	return fmt.Sprintf("%d check(s) changed", changed)
}

func checkKey(c fathomv1alpha1.HealthReportCheck) string {
	return c.Family + "|" + c.TargetRef.Kind + "/" + c.TargetRef.Namespace + "/" + c.TargetRef.Name
}

func changedCheckCount(newer, older *fathomv1alpha1.HealthReport) int {
	prev := make(map[string]fathomv1alpha1.HealthReportResult, len(older.Spec.Checks))
	for _, c := range older.Spec.Checks {
		prev[checkKey(c)] = c.Result
	}
	changed := 0
	seen := make(map[string]bool, len(newer.Spec.Checks))
	for _, c := range newer.Spec.Checks {
		k := checkKey(c)
		seen[k] = true
		if r, ok := prev[k]; !ok || r != c.Result {
			changed++
		}
	}
	for k := range prev {
		if !seen[k] {
			changed++
		}
	}
	return changed
}

// reportSummary derives a one-line summary: the report itself carries none,
// so the first check that shares the report's verdict explains it.
func reportSummary(r *fathomv1alpha1.HealthReport) string {
	n := len(r.Spec.Checks)
	if r.Spec.Result == fathomv1alpha1.HealthReportResultPass || n == 0 {
		return fmt.Sprintf("%d check(s) %s", n, orDash(string(r.Spec.Result)))
	}
	for _, c := range r.Spec.Checks {
		if c.Result == r.Spec.Result {
			if c.Summary != "" {
				return c.Family + ": " + c.Summary
			}
			return c.Family + ": " + string(c.Result)
		}
	}
	return fmt.Sprintf("%d check(s), worst %s", n, r.Spec.Result)
}

func renderReport(w io.Writer, r *fathomv1alpha1.HealthReport, now time.Time) {
	d := &describer{w: w, now: now}
	d.line("Name", r.Name)
	d.line("Namespace", r.Namespace)
	src := r.Spec.SourceRef
	d.line("Source", src.Kind+"/"+src.Name)
	d.line("Result", orDash(string(r.Spec.Result)))
	observed := r.Spec.ObservedAt
	d.line("Observed", timestamp(&observed, now))
	if r.Spec.Duration != nil {
		d.line("Duration", r.Spec.Duration.Duration.String())
	}
	if r.Spec.AdapterName != "" {
		d.line("Adapter", strings.TrimSpace(r.Spec.AdapterName+" "+r.Spec.AdapterVersion))
	}
	if r.Spec.DetectedVersion != "" {
		d.line("Detected version", r.Spec.DetectedVersion)
	}
	d.section("Checks")
	if len(r.Spec.Checks) == 0 {
		_, _ = fmt.Fprintln(w, "  (none)")
		return
	}
	tb := newTable(w)
	tb.row("  FAMILY", "TARGET", "RESULT", "SUMMARY")
	for _, c := range r.Spec.Checks {
		target := c.TargetRef.Name
		if c.TargetRef.Kind != "" {
			target = c.TargetRef.Kind + "/" + target
		}
		tb.row("  "+c.Family, target, orDash(string(c.Result)), truncate(c.Summary, summaryColumnWidth))
	}
	_ = tb.flush()
}
