/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

const (
	// bulkConfirmThreshold is the target count above which `run` asks before
	// writing anything (or refuses without --yes off a terminal).
	bulkConfirmThreshold = 10

	// fieldManager identifies fathomctl in an object's managedFields, so the
	// trigger write is attributable to the tool even though the token itself
	// carries no caller identity.
	fieldManager = "fathomctl"

	// waitMargin is added to a check's effective timeout to form the default
	// --wait bound: room for the operator to notice the annotation and write
	// status, on top of the run itself.
	waitMargin = 30 * time.Second
)

// The derived kinds' source resolvers look kinds up by name, so they are
// attached here rather than in the kinds table literal, which would be an
// initialization cycle.
func init() {
	kindByName("HealthCheck").Sources = healthCheckSources
	kindByName("ClusterHealth").Sources = clusterHealthSources
}

type runOptions struct {
	wait     bool
	timeout  time.Duration
	yes      bool
	dryRun   bool
	all      bool
	selector string
}

// runTarget is a resolved executable check: the object as read, its
// reference, and the derived check it was reached through (empty when
// addressed directly).
type runTarget struct {
	ref checkRef
	obj client.Object
	via string
}

// runOutcome is what `run` reports per target. It is also the structured
// output shape, so the field names are part of the CLI contract.
type runOutcome struct {
	Target     string `json:"target"`
	Via        string `json:"via,omitempty"`
	Token      string `json:"token,omitempty"`
	Triggered  bool   `json:"triggered"`
	Skipped    string `json:"skipped,omitempty"`
	Error      string `json:"error,omitempty"`
	Verdict    string `json:"verdict,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Superseded bool   `json:"superseded,omitempty"`
	TimedOut   bool   `json:"timedOut,omitempty"`
}

func newRunCommand(f *factory) *cobra.Command {
	opts := &runOptions{}
	cmd := &cobra.Command{
		Use:   "run [<kind>/<name> | <kind> <name>]",
		Short: "Ask Fathom to validate a check right now",
		Long: `run asks the operator to re-evaluate a check immediately, out of band from
its interval, by writing a fresh token to the fathom.skaphos.io/run-now
annotation. The operator records the token in status.lastRunTrigger when the
run completes, which is what --wait watches for.

Executable checks (AddonCheck, DNSCheck, NodeCertificateCheck) are triggered
directly. A HealthCheck triggers the check it references; a ClusterHealth
triggers the source behind every HealthCheck it selects. --all and -l select
executable checks only, within the namespace scope.

A NodeCertificateCheck run restarts one node-agent pod per node; on a large
cluster give --wait a longer --timeout. Exit codes follow kubectl: 0 when every
trigger was accepted and, with --wait, every verdict is Pass, Warn, or
Skipped; 1 otherwise.`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd, f, opts, args)
		},
	}
	cmd.Flags().BoolVar(&opts.wait, "wait", false, "Wait for the run to complete and print its verdict.")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 0, "How long --wait may take. Defaults to the check's timeout plus 30s (the largest, for several checks).")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Skip the confirmation prompt when more than 10 checks would be triggered.")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Print the checks that would be triggered and exit without writing anything.")
	cmd.Flags().BoolVar(&opts.all, "all", false, "Trigger every executable check in the namespace scope.")
	cmd.Flags().StringVarP(&opts.selector, "selector", "l", "", "Trigger the executable checks matching this label selector, within the namespace scope.")
	return cmd
}

func runRun(cmd *cobra.Command, f *factory, opts *runOptions, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	out := cmd.OutOrStdout()

	selections := 0
	if len(args) > 0 {
		selections++
	}
	if opts.all {
		selections++
	}
	if opts.selector != "" {
		selections++
	}
	if selections != 1 {
		return errors.New("specify exactly one of a check (<kind>/<name>), --all, or --selector")
	}

	c, err := f.client()
	if err != nil {
		return err
	}
	ns, err := f.namespace()
	if err != nil {
		return err
	}

	targets, skipped, err := resolveRunTargets(ctx, c, f, opts, args, ns)
	if err != nil {
		return err
	}
	for _, s := range skipped {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skipping %s: %s\n", s.Target, s.Skipped)
	}
	if len(targets) == 0 {
		return errors.New("no checks to run")
	}

	if opts.dryRun {
		_, _ = fmt.Fprintf(out, "Would trigger %d check(s):\n", len(targets))
		for _, t := range targets {
			_, _ = fmt.Fprintf(out, "  %s%s\n", t.ref, viaSuffix(t.via))
		}
		return nil
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Triggering %d check(s).\n", len(targets))
	if len(targets) > bulkConfirmThreshold && !opts.yes {
		if !f.isTerminal() {
			return fmt.Errorf("%d checks exceed the confirmation threshold of %d and no terminal is available: pass --yes to proceed", len(targets), bulkConfirmThreshold)
		}
		if !confirm(f.stdin, cmd.ErrOrStderr(), len(targets)) {
			return errors.New("aborted: not confirmed")
		}
	}

	token, err := newToken(time.Now())
	if err != nil {
		return err
	}
	outcomes := make([]runOutcome, len(targets))
	for i, t := range targets {
		outcomes[i] = runOutcome{Target: t.ref.String(), Via: t.via, Token: token}
		if err := writeTrigger(ctx, c, t.obj, token); err != nil {
			outcomes[i].Error = err.Error()
			continue
		}
		outcomes[i].Triggered = true
	}

	if opts.wait {
		waitForOutcomes(ctx, c, f, targets, outcomes, token, opts.timeout)
	}

	outcomes = append(outcomes, skipped...)
	if err := printRunOutcomes(out, f.opts.output, outcomes, opts.wait); err != nil {
		return err
	}
	return runExitError(outcomes, opts.wait)
}

// resolveRunTargets turns the selection into de-duplicated executable
// targets, separating out the ones that cannot run (paused, missing) so the
// rest still proceed.
func resolveRunTargets(ctx context.Context, c client.Client, f *factory, opts *runOptions, args []string, ns string) ([]runTarget, []runOutcome, error) {
	var refs []sourceResolution
	switch {
	case opts.all || opts.selector != "":
		listed, err := listExecutable(ctx, c, ns, opts.selector)
		if err != nil {
			return nil, nil, err
		}
		refs = listed
	default:
		ref, rest, err := parseTarget(args, ns)
		if err != nil {
			return nil, nil, err
		}
		if len(rest) > 0 {
			return nil, nil, fmt.Errorf("unexpected argument %q", rest[0])
		}
		obj := ref.Kind.New()
		if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, obj); err != nil {
			return nil, nil, describeGetError(ref, err)
		}
		if ref.Kind.Executable {
			refs = []sourceResolution{{Ref: ref}}
			break
		}
		if ref.Kind.Paused(obj) {
			return nil, nil, fmt.Errorf("%s is paused; it would not mirror a fresh result, unpause it or run its source directly", ref)
		}
		sources, err := ref.Kind.Sources(ctx, c, obj)
		if err != nil {
			return nil, nil, err
		}
		refs = sources
	}

	seen := map[string]bool{}
	var targets []runTarget
	var skipped []runOutcome
	for _, r := range refs {
		if r.Skip != "" {
			skipped = append(skipped, runOutcome{Target: r.Ref.String(), Via: r.Via, Skipped: r.Skip})
			continue
		}
		key := r.Ref.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		obj := r.Ref.Kind.New()
		if err := c.Get(ctx, types.NamespacedName{Namespace: r.Ref.Namespace, Name: r.Ref.Name}, obj); err != nil {
			if apierrors.IsNotFound(err) {
				skipped = append(skipped, runOutcome{Target: key, Via: r.Via, Skipped: "not found"})
				continue
			}
			return nil, nil, fmt.Errorf("get %s: %w", key, err)
		}
		if r.Ref.Kind.Paused(obj) {
			skipped = append(skipped, runOutcome{Target: key, Via: r.Via, Skipped: "paused; the operator would never consume the trigger"})
			continue
		}
		targets = append(targets, runTarget{ref: r.Ref, obj: obj, via: r.Via})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ref.String() < targets[j].ref.String() })
	return targets, skipped, nil
}

// listExecutable lists every executable kind in the namespace scope,
// optionally filtered by a label selector. Derived kinds are never selected
// this way.
func listExecutable(ctx context.Context, c client.Client, ns, selector string) ([]sourceResolution, error) {
	listOpts := []client.ListOption{client.InNamespace(ns)}
	if selector != "" {
		sel, err := labels.Parse(selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector %q: %w", selector, err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: sel})
	}
	var refs []sourceResolution
	for _, k := range executableKinds() {
		list := k.NewList()
		if err := c.List(ctx, list, listOpts...); err != nil {
			return nil, fmt.Errorf("list %s: %w", k.Resource, err)
		}
		for _, obj := range k.Items(list) {
			refs = append(refs, sourceResolution{Ref: checkRef{Kind: k, Namespace: obj.GetNamespace(), Name: obj.GetName()}})
		}
	}
	return refs, nil
}

// healthCheckSources resolves the executable check a HealthCheck mirrors.
func healthCheckSources(_ context.Context, _ client.Client, obj client.Object) ([]sourceResolution, error) {
	hc := obj.(*fathomv1alpha1.HealthCheck)
	via := checkRef{Kind: kindByName("HealthCheck"), Namespace: hc.Namespace, Name: hc.Name}.String()
	return []sourceResolution{healthCheckSource(hc, via)}, nil
}

func healthCheckSource(hc *fathomv1alpha1.HealthCheck, via string) sourceResolution {
	ref := hc.Spec.CheckRef
	k := kindByName(ref.Kind)
	if k == nil || !k.Executable {
		return sourceResolution{Via: via, Ref: checkRef{Kind: kindByName("HealthCheck"), Namespace: hc.Namespace, Name: hc.Name},
			Skip: fmt.Sprintf("references unsupported kind %q", ref.Kind)}
	}
	ns := ref.Namespace
	if ns == "" {
		ns = hc.Namespace
	}
	res := sourceResolution{Ref: checkRef{Kind: k, Namespace: ns, Name: ref.Name}, Via: via}
	if hc.Spec.Paused {
		res.Skip = "reached through a paused HealthCheck, which would not mirror the result"
	}
	return res
}

// clusterHealthSources resolves the sources behind every HealthCheck a
// ClusterHealth currently selects, reading the operator's own record of
// selection (status.children, capped by the controller) rather than
// re-implementing the selector.
func clusterHealthSources(ctx context.Context, c client.Client, obj client.Object) ([]sourceResolution, error) {
	ch := obj.(*fathomv1alpha1.ClusterHealth)
	if len(ch.Status.Children) == 0 {
		return nil, fmt.Errorf("clusterhealth/%s selects no HealthChecks (status.children is empty); nothing to trigger", ch.Name)
	}
	hcKind := kindByName("HealthCheck")
	var out []sourceResolution
	for _, child := range ch.Status.Children {
		via := checkRef{Kind: hcKind, Namespace: child.Namespace, Name: child.Name}.String()
		hc := &fathomv1alpha1.HealthCheck{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: child.Namespace, Name: child.Name}, hc); err != nil {
			if apierrors.IsNotFound(err) {
				out = append(out, sourceResolution{Ref: checkRef{Kind: hcKind, Namespace: child.Namespace, Name: child.Name}, Via: via, Skip: "HealthCheck not found"})
				continue
			}
			return nil, fmt.Errorf("get %s: %w", via, err)
		}
		out = append(out, healthCheckSource(hc, via))
	}
	return out, nil
}

// newToken builds the trigger value: the request time in RFC 3339 UTC plus
// six hex characters of randomness. Unique across concurrent invocations,
// sorts chronologically, reads as "when" in status, carries no identity.
func newToken(now time.Time) (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate trigger token: %w", err)
	}
	return now.UTC().Format(time.RFC3339) + "-" + hex.EncodeToString(b[:]), nil
}

// writeTrigger sets the run-now annotation with a merge patch so nothing else
// on the object is touched, attributed to fathomctl in managedFields.
func writeTrigger(ctx context.Context, c client.Client, obj client.Object, token string) error {
	before := obj.DeepCopyObject().(client.Object)
	ann := obj.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[fathomv1alpha1.AnnotationRunNow] = token
	obj.SetAnnotations(ann)
	if err := c.Patch(ctx, obj, client.MergeFrom(before), client.FieldOwner(fieldManager)); err != nil {
		return fmt.Errorf("write trigger: %w", err)
	}
	return nil
}

// waitForOutcomes waits on every triggered target concurrently and fills the
// verdict fields of its outcome.
func waitForOutcomes(ctx context.Context, c client.Client, f *factory, targets []runTarget, outcomes []runOutcome, token string, timeout time.Duration) {
	if timeout <= 0 {
		for _, t := range targets {
			timeout = max(timeout, t.ref.Kind.DefaultTimeout(t.obj)+waitMargin)
		}
	}
	var wg sync.WaitGroup
	for i := range targets {
		if !outcomes[i].Triggered {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res := f.waitForRun(ctx, c, targets[i], token, timeout)
			switch {
			case res.err != nil:
				outcomes[i].Error = res.err.Error()
			case res.superseded:
				outcomes[i].Superseded = true
			case res.timedOut:
				outcomes[i].TimedOut = true
			default:
				outcomes[i].Verdict = string(res.snap.Verdict)
				outcomes[i].Summary = res.snap.Summary
			}
		}(i)
	}
	wg.Wait()
}

func confirm(in io.Reader, prompt io.Writer, n int) bool {
	_, _ = fmt.Fprintf(prompt, "About to trigger %d checks (more than %d). Continue? [y/N] ", n, bulkConfirmThreshold)
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func describeGetError(ref checkRef, err error) error {
	if apierrors.IsNotFound(err) {
		if ref.Namespace == "" {
			return fmt.Errorf("%s %q not found", ref.Kind.Kind, ref.Name)
		}
		return fmt.Errorf("%s %q not found in namespace %s", ref.Kind.Kind, ref.Name, ref.Namespace)
	}
	return fmt.Errorf("get %s: %w", ref, err)
}

func viaSuffix(via string) string {
	if via == "" {
		return ""
	}
	return " (via " + via + ")"
}

// verdictPasses is the exit-code rule: Pass, Warn, and Skipped are not
// failures; Fail, Error, Unknown, and "no verdict" are.
func verdictPasses(v string) bool {
	switch fathomv1alpha1.HealthReportResult(v) {
	case fathomv1alpha1.HealthReportResultPass, fathomv1alpha1.HealthReportResultWarn, fathomv1alpha1.HealthReportResultSkipped:
		return true
	}
	return false
}

func printRunOutcomes(w io.Writer, format outputFormat, outcomes []runOutcome, waited bool) error {
	if format.structured() {
		return encode(w, format, outcomes)
	}
	tb := newTable(w)
	if waited {
		tb.row("TARGET", "VERDICT", "SUMMARY")
	} else {
		tb.row("TARGET", "STATUS")
	}
	for _, o := range outcomes {
		target := o.Target + viaSuffix(o.Via)
		switch {
		case o.Skipped != "":
			tb.row(target, "skipped: "+o.Skipped, "")
		case o.Error != "":
			tb.row(target, "error: "+o.Error, "")
		case !waited:
			tb.row(target, "triggered (token "+o.Token+")")
		case o.Superseded:
			tb.row(target, "superseded", "another trigger replaced token "+o.Token+" before it was consumed")
		case o.TimedOut:
			tb.row(target, "timed out", "token "+o.Token+" was not consumed; check `fathomctl version` (operator older than the CLI?), whether the check is paused, or (NodeCertificateCheck) the node-agent rollout")
		default:
			tb.row(target, orDash(o.Verdict), truncate(o.Summary, 80))
		}
	}
	return tb.flush()
}

// runExitError turns the outcomes into the command's exit status: nil only
// when every target was triggered and, if waited on, passed.
func runExitError(outcomes []runOutcome, waited bool) error {
	var failed []string
	for _, o := range outcomes {
		switch {
		case o.Skipped != "":
			failed = append(failed, o.Target+" ("+o.Skipped+")")
		case o.Error != "":
			failed = append(failed, o.Target+" ("+o.Error+")")
		case waited && o.Superseded:
			failed = append(failed, o.Target+" (superseded)")
		case waited && o.TimedOut:
			failed = append(failed, o.Target+" (timed out)")
		case waited && !verdictPasses(o.Verdict):
			failed = append(failed, o.Target+" ("+orDash(o.Verdict)+")")
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("%d of %d check(s) did not succeed: %s", len(failed), len(outcomes), strings.Join(failed, ", "))
}
