/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

func newDescribeCommand(f *factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe <kind>/<name>",
		Short: "Show one check in full: spec, verdict, conditions, and per-target detail",
		Long: `describe explains why a check has the verdict it has: what it was configured
to do, what the operator observed, which conditions are set, every per-target
or per-node result the status carries, and where the most recent HealthReport
is. For a ClusterHealth it shows which HealthChecks contributed and what each
contributed.

-o json and -o yaml emit the resource unmodified.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDescribe(cmd, f, args)
		},
	}
	return cmd
}

func runDescribe(cmd *cobra.Command, f *factory, args []string) error {
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
	obj := ref.Kind.New()
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, obj); err != nil {
		return describeGetError(ref, err)
	}
	out := cmd.OutOrStdout()
	if f.opts.output.structured() {
		if err := withGVK(c.Scheme(), obj); err != nil {
			return err
		}
		return encode(out, f.opts.output, obj)
	}
	d := &describer{w: out, now: time.Now()}
	d.header(ref, obj)
	d.spec(obj)
	d.status(ref.Kind, obj)
	d.conditions(conditionsOf(obj))
	d.details(obj)
	d.report(ref, ref.Kind.Snapshot(obj))
	return nil
}

// describer renders kubectl-describe style key/value sections.
type describer struct {
	w   io.Writer
	now time.Time
}

func (d *describer) line(key, value string) {
	_, _ = fmt.Fprintf(d.w, "%-21s %s\n", key+":", value)
}

func (d *describer) section(title string) {
	_, _ = fmt.Fprintf(d.w, "\n%s:\n", title)
}

func (d *describer) sub(key, value string) {
	_, _ = fmt.Fprintf(d.w, "  %-19s %s\n", key+":", value)
}

func (d *describer) header(ref checkRef, obj client.Object) {
	d.line("Name", obj.GetName())
	if ref.Kind.Namespaced {
		d.line("Namespace", obj.GetNamespace())
	}
	d.line("Kind", ref.Kind.Kind)
	created := obj.GetCreationTimestamp()
	d.line("Age", age(&created, d.now))
	if len(obj.GetLabels()) > 0 {
		d.line("Labels", formatMap(obj.GetLabels()))
	}
}

func (d *describer) spec(obj client.Object) {
	d.section("Spec")
	switch o := obj.(type) {
	case *fathomv1alpha1.AddonCheck:
		d.sub("Addon type", o.Spec.AddonType)
		d.sub("Interval", cadence(o.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultAddonCheckInterval))
		d.sub("Timeout", cadence(o.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultAddonCheckTimeout))
		d.sub("Paused", fmt.Sprint(o.Spec.Paused))
		d.sub("History limit", int32PtrString(o.Spec.HistoryLimit, "10 (default)"))
		families := make([]string, 0, len(o.Spec.Policy))
		for name := range o.Spec.Policy {
			families = append(families, name)
		}
		sort.Strings(families)
		if len(families) == 0 {
			d.sub("Policy", "(adapter defaults)")
		}
		for _, name := range families {
			p := o.Spec.Policy[name]
			enabled := "enabled"
			if p.Enabled != nil && !*p.Enabled {
				enabled = "disabled"
			}
			extra := []string{enabled}
			if len(p.Namespaces) > 0 {
				extra = append(extra, "namespaces="+strings.Join(p.Namespaces, ","))
			}
			if p.LabelSelector != nil {
				extra = append(extra, "selector="+metav1.FormatLabelSelector(p.LabelSelector))
			}
			if len(p.Thresholds) > 0 {
				extra = append(extra, fmt.Sprintf("%d threshold(s)", len(p.Thresholds)))
			}
			d.sub("Policy "+name, strings.Join(extra, ", "))
		}
	case *fathomv1alpha1.DNSCheck:
		d.sub("Interval", cadence(o.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultDNSCheckInterval))
		d.sub("Timeout", cadence(o.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultDNSCheckTimeout))
		d.sub("History limit", int32PtrString(o.Spec.HistoryLimit, "10 (default)"))
		if len(o.Spec.Resolvers) == 0 {
			d.sub("Resolvers", "cluster (implicit)")
		}
		for _, r := range o.Spec.Resolvers {
			from := string(r.From)
			if r.Address != "" {
				from += " " + r.Address
			}
			d.sub("Resolver "+r.Name, from)
		}
		for _, t := range o.Spec.Targets {
			parts := []string{string(t.RecordType)}
			if t.Absent {
				parts = append(parts, "must be absent")
			}
			if t.Resolver != "" {
				parts = append(parts, "resolver="+t.Resolver)
			}
			if len(t.ExpectedAnswers) > 0 {
				parts = append(parts, "expect="+strings.Join(t.ExpectedAnswers, ","))
			}
			d.sub("Target "+t.Name, strings.Join(parts, ", "))
		}
	case *fathomv1alpha1.NodeCertificateCheck:
		d.sub("Interval", cadence(o.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultNodeCertificateCheckInterval))
		d.sub("Timeout", cadence(o.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultNodeCertificateCheckTimeout))
		d.sub("Paused", fmt.Sprint(o.Spec.Paused))
		d.sub("Warn days", int32PtrString(o.Spec.WarnDays, "30 (default)"))
		d.sub("Critical days", int32PtrString(o.Spec.CriticalDays, "7 (default)"))
		d.sub("Control-plane nodes", fmt.Sprint(o.Spec.IncludeControlPlaneNodes != nil && *o.Spec.IncludeControlPlaneNodes))
		if len(o.Spec.NodeSelector) > 0 {
			d.sub("Node selector", formatMap(o.Spec.NodeSelector))
		}
		if len(o.Spec.Paths) == 0 {
			d.sub("Paths", "(built-in default set)")
		} else {
			d.sub("Paths", strings.Join(o.Spec.Paths, ", "))
		}
	case *fathomv1alpha1.NodeHealthCheck:
		d.sub("Interval", cadence(o.Spec.Interval, fathomv1alpha1.MinCheckInterval, fathomv1alpha1.DefaultNodeHealthCheckInterval))
		d.sub("Timeout", cadence(o.Spec.Timeout, fathomv1alpha1.MinCheckTimeout, fathomv1alpha1.DefaultNodeHealthCheckTimeout))
		d.sub("History limit", int32PtrString(o.Spec.HistoryLimit, "10 (default)"))
		d.sub("Control-plane nodes", fmt.Sprint(o.Spec.IncludeControlPlaneNodes != nil && *o.Spec.IncludeControlPlaneNodes))
		if len(o.Spec.NodeSelector) > 0 {
			d.sub("Node selector", formatMap(o.Spec.NodeSelector))
		}
		for _, c := range o.Spec.Checks {
			label := string(c.Type)
			if c.Path != "" {
				label += " " + c.Path
			}
			var parts []string
			switch c.Type {
			case fathomv1alpha1.NodeHealthCheckDiskHeadroom, fathomv1alpha1.NodeHealthCheckInodeHeadroom:
				parts = append(parts,
					"warn<="+int32PtrString(c.WarnPercentFree, fmt.Sprintf("%d (default)", fathomv1alpha1.DefaultNodeHealthWarnPercentFree))+"%",
					"critical<="+int32PtrString(c.CriticalPercentFree, fmt.Sprintf("%d (default)", fathomv1alpha1.DefaultNodeHealthCriticalPercentFree))+"%")
			case fathomv1alpha1.NodeHealthCheckNodeCondition:
				conds := c.Conditions
				if len(conds) == 0 {
					conds = fathomv1alpha1.DefaultNodeHealthConditions()
				}
				parts = append(parts, strings.Join(conds, ","))
			case fathomv1alpha1.NodeHealthCheckKubeletHealthz:
				parts = append(parts, "hostNetwork")
			case fathomv1alpha1.NodeHealthCheckContainerRuntime:
				sock := c.SocketPath
				if sock == "" {
					sock = fathomv1alpha1.DefaultNodeHealthContainerRuntimeSocket + " (default)"
				}
				parts = append(parts, sock, "runs as root")
			}
			d.sub("Check "+label, strings.Join(parts, ", "))
		}
	case *fathomv1alpha1.HealthCheck:
		ref := o.Spec.CheckRef
		target := ref.Kind + "/" + ref.Name
		if ref.Namespace != "" {
			target = ref.Kind + "/" + ref.Namespace + "/" + ref.Name
		}
		d.sub("Check ref", target)
		if o.Spec.Description != "" {
			d.sub("Description", o.Spec.Description)
		}
		d.sub("Paused", fmt.Sprint(o.Spec.Paused))
	case *fathomv1alpha1.ClusterHealth:
		if o.Spec.Selector != nil {
			d.sub("Selector", metav1.FormatLabelSelector(o.Spec.Selector))
		} else {
			d.sub("Selector", "(none: every HealthCheck in scope)")
		}
		if len(o.Spec.Namespaces) > 0 {
			d.sub("Namespaces", strings.Join(o.Spec.Namespaces, ", "))
		}
		if len(o.Spec.ExcludedNamespaces) > 0 {
			d.sub("Excluded namespaces", strings.Join(o.Spec.ExcludedNamespaces, ", "))
		}
		if o.Spec.Description != "" {
			d.sub("Description", o.Spec.Description)
		}
	}
}

func (d *describer) status(k *kindDescriptor, obj client.Object) {
	snap := k.Snapshot(obj)
	d.section("Status")
	d.sub("Verdict", orDash(string(snap.Verdict)))
	d.sub("Summary", orDash(snap.Summary))
	d.sub("Last run", timestamp(snap.LastRun, d.now))
	if snap.NextRun != nil {
		d.sub("Next run", "in "+until(snap.NextRun, d.now))
	}
	if snap.ConsumedTrigger != "" {
		d.sub("Consumed trigger", snap.ConsumedTrigger)
	}
	switch o := obj.(type) {
	case *fathomv1alpha1.AddonCheck:
		d.sub("Observed generation", fmt.Sprint(o.Status.ObservedGeneration))
		if o.Status.DetectedVersion != "" {
			d.sub("Detected version", o.Status.DetectedVersion)
		}
		d.sub("Absent checks", fmt.Sprint(o.Status.Absent))
	case *fathomv1alpha1.DNSCheck:
		d.sub("Observed generation", fmt.Sprint(o.Status.ObservedGeneration))
		d.sub("Observed targets", fmt.Sprint(o.Status.ObservedTargets))
	case *fathomv1alpha1.NodeCertificateCheck:
		d.sub("Observed generation", fmt.Sprint(o.Status.ObservedGeneration))
		d.sub("Desired nodes", fmt.Sprint(o.Status.DesiredNodes))
		d.sub("Reporting nodes", fmt.Sprint(o.Status.ReportingNodes))
	case *fathomv1alpha1.NodeHealthCheck:
		d.sub("Observed generation", fmt.Sprint(o.Status.ObservedGeneration))
		d.sub("Desired nodes", fmt.Sprint(o.Status.DesiredNodes))
		d.sub("Reporting nodes", fmt.Sprint(o.Status.ReportingNodes))
	case *fathomv1alpha1.HealthCheck:
		d.sub("Observed generation", fmt.Sprint(o.Status.ObservedGeneration))
		if o.Status.SourceInterval != nil {
			d.sub("Source interval", o.Status.SourceInterval.Duration.String())
		}
	case *fathomv1alpha1.ClusterHealth:
		d.sub("Observed generation", fmt.Sprint(o.Status.ObservedGeneration))
		d.sub("Matched", fmt.Sprint(o.Status.MatchedCount))
	}
}

func (d *describer) conditions(conds []metav1.Condition) {
	d.section("Conditions")
	if len(conds) == 0 {
		_, _ = fmt.Fprintln(d.w, "  (none)")
		return
	}
	tb := newTable(d.w)
	tb.row("  TYPE", "STATUS", "REASON", "MESSAGE", "LAST TRANSITION")
	for _, c := range conds {
		lt := c.LastTransitionTime
		tb.row("  "+c.Type, string(c.Status), c.Reason, truncate(c.Message, summaryColumnWidth), age(&lt, d.now))
	}
	_ = tb.flush()
}

func (d *describer) details(obj client.Object) {
	switch o := obj.(type) {
	case *fathomv1alpha1.DNSCheck:
		if len(o.Status.TargetResults) == 0 {
			return
		}
		d.section("Target results")
		tb := newTable(d.w)
		tb.row("  NAME", "TYPE", "RESOLVER", "RESULT", "LATENCY", "ANSWERS", "MESSAGE")
		for _, r := range o.Status.TargetResults {
			latency := "-"
			if r.LatencyMillis > 0 {
				latency = fmt.Sprintf("%dms", r.LatencyMillis)
			}
			tb.row("  "+r.Name, string(r.RecordType), r.Resolver, orDash(r.Result), latency,
				orDash(truncate(strings.Join(r.Answers, ","), 40)), truncate(r.Message, summaryColumnWidth))
		}
		_ = tb.flush()
	case *fathomv1alpha1.NodeHealthCheck:
		if len(o.Status.NodeResults) == 0 {
			return
		}
		d.section("Node results")
		tb := newTable(d.w)
		tb.row("  NODE", "RESULT", "OBSERVED", "MESSAGE")
		for _, r := range o.Status.NodeResults {
			tb.row("  "+r.Node, orDash(r.Result), age(r.ObservedAt, d.now), truncate(r.Message, summaryColumnWidth))
		}
		_ = tb.flush()
		if int(o.Status.ReportingNodes) > len(o.Status.NodeResults) {
			_, _ = fmt.Fprintf(d.w, "  (%d of %d nodes listed; the list is capped at %d)\n",
				len(o.Status.NodeResults), o.Status.ReportingNodes, fathomv1alpha1.MaxNodeHealthNodeResults)
		}
	case *fathomv1alpha1.ClusterHealth:
		if len(o.Status.Children) == 0 {
			return
		}
		d.section("Children")
		tb := newTable(d.w)
		tb.row("  NAMESPACE", "NAME", "RESULT", "SUMMARY", "OBSERVED")
		for _, c := range o.Status.Children {
			tb.row("  "+c.Namespace, c.Name, orDash(string(c.Result)), truncate(c.Summary, summaryColumnWidth), age(c.ObservedAt, d.now))
		}
		_ = tb.flush()
		if int(o.Status.MatchedCount) > len(o.Status.Children) {
			_, _ = fmt.Fprintf(d.w, "  (%d of %d contributors listed; the list is capped at %d)\n",
				len(o.Status.Children), o.Status.MatchedCount, fathomv1alpha1.MaxClusterHealthChildren)
		}
	}
}

func (d *describer) report(ref checkRef, snap snapshot) {
	if snap.ReportName == "" {
		return
	}
	_, _ = fmt.Fprintf(d.w, "\nLatest report: %s (see: fathomctl reports %s)\n", snap.ReportName, ref)
}

func conditionsOf(obj client.Object) []metav1.Condition {
	switch o := obj.(type) {
	case *fathomv1alpha1.AddonCheck:
		return o.Status.Conditions
	case *fathomv1alpha1.DNSCheck:
		return o.Status.Conditions
	case *fathomv1alpha1.NodeCertificateCheck:
		return o.Status.Conditions
	case *fathomv1alpha1.NodeHealthCheck:
		return o.Status.Conditions
	case *fathomv1alpha1.HealthCheck:
		return o.Status.Conditions
	case *fathomv1alpha1.ClusterHealth:
		return o.Status.Conditions
	}
	return nil
}

// cadence renders a spec duration with the controller's effective value when
// the field is unset or clamped, so the user sees what actually runs.
func cadence(d *metav1.Duration, floor, def time.Duration) string {
	eff := effectiveDuration(d, floor, def)
	switch {
	case d == nil || d.Duration <= 0:
		return eff.String() + " (default)"
	case d.Duration < floor:
		return fmt.Sprintf("%s (declared %s, clamped to the floor)", eff, d.Duration)
	}
	return eff.String()
}

func int32PtrString(v *int32, def string) string {
	if v == nil {
		return def
	}
	return fmt.Sprint(*v)
}

func timestamp(t *metav1.Time, now time.Time) string {
	if t == nil || t.IsZero() {
		return "never"
	}
	return fmt.Sprintf("%s (%s ago)", t.UTC().Format(time.RFC3339), age(t, now))
}

func formatMap(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
}
