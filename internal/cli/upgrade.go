/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	helmchart "github.com/skaphos/fathom/deploy/helm"
)

// publishedChart is the OCI chart releases are published to; only a Helm
// upgrade below the upgrade floor still points at it.
const publishedChart = "oci://ghcr.io/skaphos/charts/fathom-operator"

// readUpgradePathFile reads the declaration embedded with this release's
// chart. Tests swap it to exercise stops this release does not declare.
var readUpgradePathFile = func() ([]byte, error) {
	return helmchart.ReadFile(helmchart.UpgradePathFile)
}

// upgradePlan is the plan for one transition from the installed version to
// this CLI's version (data-model "Upgrade plan").
type upgradePlan struct {
	From      string         `json:"from,omitempty"`
	To        string         `json:"to"`
	Release   string         `json:"release,omitempty"`
	Namespace string         `json:"namespace,omitempty"`
	Path      string         `json:"path"`
	Stops     []string       `json:"stops,omitempty"`
	CRDs      []crdPlan      `json:"crds"`
	Refusals  []refusal      `json:"refusals,omitempty"`
	Operator  operatorChange `json:"operator"`
	Values    map[string]any `json:"values,omitempty"`
}

type operatorChange struct {
	FromImages map[string]string `json:"fromImages,omitempty"`
	ToImages   map[string]string `json:"toImages"`
}

// planRequest is what buildUpgradePlan needs to judge one transition.
type planRequest struct {
	// from is the installed version used for the path and the downgrade
	// check; empty means nothing is installed.
	from string
	to   string
	// perCRDFrom judges each CRD against the version recorded on it rather
	// than from: install over CRDs a plain uninstall left behind (FR-014). A
	// CRD with no recorded version is compared without a path, so any
	// incompatible finding refuses.
	perCRDFrom bool
	accept     bool
	// command is what the user ran, for "next:" lines.
	command string
}

// buildUpgradePlan computes the CRD plan, the upgrade path and every
// refusal, changing nothing (research R9 steps 1–2).
func buildUpgradePlan(ctx context.Context, c client.Client, req planRequest) (*upgradePlan, error) {
	raw, err := readUpgradePathFile()
	if err != nil {
		return nil, fmt.Errorf("read embedded upgrade path: %w", err)
	}
	decl, err := parseUpgradePath(raw)
	if err != nil {
		return nil, err
	}
	to := semver.MustParse(req.to)
	plan := &upgradePlan{From: req.from, To: req.to, Path: "direct"}

	if req.from != "" {
		from, err := semver.NewVersion(req.from)
		if err != nil {
			return nil, fmt.Errorf("installed version %q is not SemVer: %w", req.from, err)
		}
		switch {
		case from.GreaterThan(to):
			plan.Refusals = append(plan.Refusals, refusal{
				Reason: reasonDowngrade,
				Detail: fmt.Sprintf("the installed version v%s is newer than this fathomctl (v%s); downgrades are not supported", from, to),
				Next:   fmt.Sprintf("use fathomctl v%s or newer", from),
			})
		case from.LessThan(to):
			res := decl.upgradePath(from, to)
			plan.Path, plan.Stops = res.render(from.String(), to.String()), res.Stops
			plan.Refusals = append(plan.Refusals, pathRefusals(decl, res, from, to, req.command)...)
		}
	}

	targets, err := targetCRDs()
	if err != nil {
		return nil, err
	}
	crds, err := planCRDs(ctx, c, targets)
	if err != nil {
		return nil, err
	}
	if req.perCRDFrom {
		plan.Refusals = append(plan.Refusals, recordedVersionRefusals(crds, to)...)
	}
	var ack []string
	for i := range crds {
		p := &crds[i]
		if p.Action == crdReplace && req.perCRDFrom {
			// Leftover CRDs carry no recorded version to judge a sanction
			// against, and a replace deletes every object of the kind.
			p.Action = crdRefuse
			plan.Refusals = append(plan.Refusals, leftoverStoredVersionRefusal(p))
			continue
		}
		from, judged := sanctionBase(req, p)
		for j := range p.Findings {
			finding := &p.Findings[j]
			if finding.Check == scopeCheck {
				p.Action = crdRefuse
				plan.Refusals = append(plan.Refusals, scopeRefusal(p))
				continue
			}
			var s *sanctionedChange
			var release string
			if judged {
				s, release = decl.sanctionedBy(p.Name, finding.Path, finding.Check, from, to)
			}
			if s == nil {
				p.Action = crdRefuse
				plan.Refusals = append(plan.Refusals, unsanctionedRefusal(p, finding))
				continue
			}
			finding.SanctionedBy = fmt.Sprintf("v%s (%s)", release, s.Issue)
			if req.accept {
				continue
			}
			switch {
			case p.Action == crdReplace && !p.needsAcknowledgement():
				// Nothing would be removed: the sanction alone suffices.
			case p.Action == crdReplace:
				ack = append(ack, inventoryAckLine(p, s, release))
			default:
				ack = append(ack, fmt.Sprintf("%s %s: %s — %s (sanctioned by v%s)", p.Name, finding.Path, s.Reason, s.Issue, release))
			}
		}
	}
	if len(ack) > 0 {
		plan.Refusals = append(plan.Refusals, refusal{
			Reason: reasonAckRequired,
			Detail: "this release makes incompatible CRD changes it sanctions; existing resources may fail the new validation:\n  " + strings.Join(ack, "\n  "),
			Next:   req.command + " --accept-incompatible-crds",
		})
	}
	plan.CRDs = crds
	return plan, nil
}

// sanctionBase is the "from" version a CRD's findings are judged against,
// and whether they can be sanctioned at all.
func sanctionBase(req planRequest, p *crdPlan) (*semver.Version, bool) {
	if !req.perCRDFrom {
		if req.from == "" {
			return nil, false
		}
		return semver.MustParse(req.from), true
	}
	if p.RecordedVersion == "" {
		return nil, false
	}
	v, err := semver.NewVersion(p.RecordedVersion)
	if err != nil {
		return nil, false
	}
	return v, true
}

// pathRefusals refuses a transition below the floor or across a stop
// (FR-027); --accept-incompatible-crds never bypasses them.
func pathRefusals(decl *upgradePathDecl, res pathResult, from, to *semver.Version, command string) []refusal {
	switch {
	case res.BelowFloor:
		return []refusal{{
			Reason: reasonBelowFloor,
			Detail: fmt.Sprintf("v%s predates the oldest version this fathomctl has upgrade data for (v%s); the oldest version v%s upgrades from directly is v%s", from, decl.Floor, to, decl.oldestDirect(to)),
			Next: fmt.Sprintf("upgrade with Helm to the latest v%d.%d.x release first (apply its CRDs with kubectl, then `helm upgrade <release> %s --version <%d.%d.x>`; see docs/guides/getting-started.md), then rerun %q",
				decl.floor.Major(), decl.floor.Minor(), publishedChart, decl.floor.Major(), decl.floor.Minor(), command),
		}}
	case len(res.Stops) > 0:
		return []refusal{{
			Reason: reasonStopSkipped,
			Detail: fmt.Sprintf("v%s cannot be upgraded to v%s directly; upgrade through each stop in order. Upgrade path: %s", from, to, res.render(from.String(), to.String())),
			Next:   fmt.Sprintf("download fathomctl v%s and run %q", res.Stops[0], command),
		}}
	}
	return nil
}

// recordedVersionRefusals applies the install-over-leftover-CRDs rules: a
// CRD a newer release applied is a CRD downgrade (FR-014).
func recordedVersionRefusals(crds []crdPlan, to *semver.Version) []refusal {
	var newer []string
	for _, p := range crds {
		if p.RecordedVersion == "" {
			continue
		}
		if v, err := semver.NewVersion(p.RecordedVersion); err == nil && v.GreaterThan(to) {
			newer = append(newer, fmt.Sprintf("%s (v%s)", p.Name, v))
		}
	}
	if len(newer) == 0 {
		return nil
	}
	return []refusal{{
		Reason: reasonCRDDowngrade,
		Detail: fmt.Sprintf("these CRDs were applied by a newer release than this fathomctl (v%s) and installing would downgrade them: %s", to, strings.Join(newer, ", ")),
		Next:   "use the fathomctl release that matches those CRDs",
	}}
}

func leftoverStoredVersionRefusal(p *crdPlan) refusal {
	return refusal{
		Reason: reasonLeftoverStoredVersion,
		Detail: fmt.Sprintf("%s was left behind by an earlier uninstall and stores %s, which this release no longer serves; install cannot replace it because leftover CRDs record no version to sanction the change against", p.Name, strings.Join(storedVersionsDropped(p.live, p.target), ", ")),
		Next:   "fathomctl export --output <file>; kubectl delete crd " + p.Name + "; fathomctl install",
	}
}

func scopeRefusal(p *crdPlan) refusal {
	return refusal{
		Reason: reasonScopeChange,
		Detail: fmt.Sprintf("%s changes scope; Kubernetes cannot change a CRD's scope in place, and deleting the CRD to recreate it deletes every resource of that kind", p.Name),
		Next:   "fathomctl uninstall --purge-crds --export <file>, then install this release and re-apply the exported resources",
	}
}

func unsanctionedRefusal(p *crdPlan, f *crdFinding) refusal {
	where := p.Name
	if f.Path != "" {
		where += " " + f.Path
	}
	return refusal{
		Reason: reasonUnsanctioned,
		Detail: fmt.Sprintf("%s: incompatible change (%s) that no release sanctions: %s", where, f.Check, f.Message),
		Next:   "export the affected resources, apply the new CRD manually once they are compatible (or uninstall --purge-crds --export), then rerun",
	}
}

// upgradeOptions are upgrade's flags.
type upgradeOptions struct {
	lifecycleOptions
	dryRun            bool
	rollbackOnFailure bool
}

func newUpgradeCommand(f *factory) *cobra.Command {
	opts := &upgradeOptions{}
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade Fathom to this fathomctl's version, CRDs first",
		Long: `upgrade moves the installed Fathom release to exactly this fathomctl's
version, from the chart embedded in it. It first prints the plan: each CRD
added, updated or unchanged, the upgrade path, and the operator images. It
refuses, before changing anything, a downgrade, a skipped stop, a CRD scope
change and any incompatible CRD change the release does not sanction;
sanctioned incompatible changes need --accept-incompatible-crds (--yes never
implies it). Then it applies the CRDs, waits for them to be Established, and
only then rolls out the operator, keeping the release's values.

If the new operator is not Available within --timeout, the new release stays
in place and upgrade exits 1 with the unmet condition and recovery commands;
--rollback-on-failure rolls the operator (never the CRDs) back instead, and is
refused when the plan applies an incompatible CRD change.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpgrade(commandContext(cmd), cmd, f, opts)
		},
	}
	opts.register(cmd, false)
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Print the plan and change nothing.")
	cmd.Flags().BoolVar(&opts.rollbackOnFailure, "rollback-on-failure", false,
		"Roll the operator back to the previous revision if it is not Available within --timeout. CRDs are never rolled back.")
	return cmd
}

func runUpgrade(ctx context.Context, cmd *cobra.Command, f *factory, opts *upgradeOptions) error {
	version, err := cliChartVersion()
	if err != nil {
		return err
	}
	env, err := opts.lifecycleEnv(f)
	if err != nil {
		return err
	}
	in, err := detectInstall(ctx, env.c, env.rc, f.opts.namespace)
	if err != nil {
		return err
	}
	command := "fathomctl upgrade"
	switch in.ManagedBy {
	case managedByNone:
		return refusal{Reason: reasonNotInstalled, Detail: "Fathom is not installed", Next: "fathomctl install"}
	case managedByHelm:
	default:
		return notHelmRefusal(in)
	}
	command += " -n " + in.Namespace
	if in.pending() {
		return pendingRefusal(in)
	}
	// A dry run changes nothing, so a read-only identity may still see the plan.
	if !opts.dryRun {
		if err := preflightPermissions(ctx, f, env.c, lifecyclePermissions("upgrade", in.Namespace, permissionScope{})); err != nil {
			return err
		}
	}
	plan, err := buildUpgradePlan(ctx, env.c, planRequest{from: in.Version, to: version, accept: opts.acceptIncompatible, command: command})
	if err != nil {
		return err
	}
	plan.Release, plan.Namespace = in.Release, in.Namespace
	plan.Operator = operatorChange{FromImages: in.Images, ToImages: imageReferences(version)}
	if opts.rollbackOnFailure && planHasIncompatibleChange(plan) {
		plan.Refusals = append(plan.Refusals, refusal{
			Reason: reasonRollbackRefused,
			Detail: "--rollback-on-failure cannot be used: the plan applies an incompatible CRD change, and the previous operator may not run against the new schema",
			Next:   strings.TrimSpace(command + " --accept-incompatible-crds"),
		})
	}

	// Values: the release's own, overridden by the user's, with the CLI's
	// images always on top (FR-007, FR-025).
	vals := mergeValues(mergeValues(in.values, env.userValues), imageValues(version))
	plan.Values = redactValues(vals)
	if _, err := env.rc.Render(ctx, releaseRequest{Name: in.Release, Namespace: in.Namespace, Version: version, Values: vals, Upgrade: true}); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if opts.dryRun || len(plan.Refusals) > 0 {
		if err := printPlan(out, f.opts.output, plan); err != nil {
			return err
		}
		if len(plan.Refusals) > 0 {
			return refusalList(plan.Refusals)
		}
		return nil
	}
	if !f.opts.output.structured() {
		if err := printPlan(out, outputTable, plan); err != nil {
			return err
		}
	}
	// Same version and current CRDs is a no-op only when the user asked for no
	// value changes; -f/--set at the same version still upgrades (FR-025).
	if plan.From == plan.To && allUnchanged(plan) && len(env.userValues) == 0 {
		// The documented retry after a timed-out rollout lands here, so the
		// operator must be Available before this reports success.
		if err := waitManagerAvailable(ctx, f, env.c, in.Namespace, in.Release, opts.timeout); err != nil {
			return fmt.Errorf("%w\n%s", err, upgradeRecovery(ctx, env.c, in, opts))
		}
		if f.opts.output.structured() {
			return encode(out, f.opts.output, plan)
		}
		_, _ = fmt.Fprintf(out, "Fathom is already at v%s; nothing to do.\n", version)
		return nil
	}

	if err := applyCRDs(ctx, f, env.c, plan.CRDs, version, opts.timeout); err != nil {
		return err
	}
	if !f.opts.output.structured() {
		renderReplaced(out, plan.CRDs)
	}
	if _, err := env.rc.Upgrade(ctx, releaseRequest{
		Name: in.Release, Namespace: in.Namespace, Version: version,
		Values: mergeValues(env.userValues, imageValues(version)), ReuseValues: true, Timeout: opts.timeout,
	}); err != nil {
		return fmt.Errorf("%w\nThe CRDs are already at v%s's schema; the operator was not upgraded. To recover:\n"+
			"  inspect: helm history %s -n %s\n"+
			"  if the latest revision is failed: helm rollback %s -n %s  # restores the previous operator; CRDs stay\n"+
			"  retry:   fathomctl upgrade -n %s  # once the cause above is fixed", err, version, in.Release, in.Namespace, in.Release, in.Namespace, in.Namespace)
	}
	if err := waitManagerAvailable(ctx, f, env.c, in.Namespace, in.Release, opts.timeout); err != nil {
		if opts.rollbackOnFailure {
			if rbErr := env.rc.Rollback(ctx, in.Release, in.Namespace, opts.timeout); rbErr != nil {
				return errors.Join(err, fmt.Errorf("rollback failed: %w", rbErr))
			}
			return fmt.Errorf("%w\nthe operator was rolled back to the previous revision; the CRDs stay on v%s's schema", err, version)
		}
		return fmt.Errorf("%w\n%s", err, upgradeRecovery(ctx, env.c, in, opts))
	}
	if f.opts.output.structured() {
		return encode(out, f.opts.output, plan)
	}
	_, _ = fmt.Fprintf(out, "Upgraded Fathom to v%s (release %s in %s); the operator is Available.\n", version, in.Release, in.Namespace)
	return nil
}

func upgradeRecovery(ctx context.Context, c client.Client, in *install, opts *upgradeOptions) string {
	deploy := managerDeploymentName(ctx, c, in.Namespace, in.Release)
	return strings.Join([]string{
		"The new release was left in place. To recover:",
		fmt.Sprintf("  inspect:   kubectl -n %s describe deploy/%s", in.Namespace, deploy),
		fmt.Sprintf("  retry:     fathomctl upgrade -n %s --timeout %s  # a longer timeout", in.Namespace, opts.timeout*2),
		fmt.Sprintf("  roll back: helm rollback %s -n %s  # the operator only; CRDs stay", in.Release, in.Namespace),
	}, "\n")
}

func planHasIncompatibleChange(plan *upgradePlan) bool {
	for _, p := range plan.CRDs {
		if len(p.Findings) > 0 {
			return true
		}
	}
	return false
}

func allUnchanged(plan *upgradePlan) bool {
	for _, p := range plan.CRDs {
		if p.Action != crdUnchanged || p.RecordedVersion != plan.To {
			return false
		}
	}
	return true
}

// secretKey matches value keys whose values must not be printed.
var secretKey = regexp.MustCompile(`(?i)(password|passwd|token|secret|apikey|api_key|credential)`)

// redactValues copies values with the value of every secret-looking key
// replaced, scalar or structured, so a plan never prints credentials
// (data-model "values without secrets").
func redactValues(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if secretKey.MatchString(k) {
			out[k] = "<redacted>"
			continue
		}
		if val, ok := v.(map[string]any); ok {
			out[k] = redactValues(val)
			continue
		}
		out[k] = v
	}
	return out
}

// printPlan renders the plan per the contract: a table, or the Upgrade plan
// entity as JSON/YAML.
func printPlan(w io.Writer, format outputFormat, plan *upgradePlan) error {
	if format.structured() {
		return encode(w, format, plan)
	}
	from := "not installed"
	if plan.From != "" {
		from = "v" + plan.From
	}
	_, _ = fmt.Fprintf(w, "Fathom upgrade plan: %s -> v%s (release %s in %s)\n", from, plan.To, plan.Release, plan.Namespace)
	_, _ = fmt.Fprintf(w, "Path:      %s\n", plan.Path)
	_, _ = fmt.Fprintln(w, "CRDs:")
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, p := range plan.CRDs {
		detail := ""
		if len(p.Findings) > 0 {
			var parts []string
			for _, fd := range p.Findings {
				label := "incompatible"
				if fd.SanctionedBy != "" {
					label += " (sanctioned by " + fd.SanctionedBy + ")"
				}
				parts = append(parts, fmt.Sprintf("%s: %s %s: %s", label, fd.Path, fd.Check, fd.Message))
			}
			detail = strings.Join(parts, "; ")
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", p.Name, p.Action, detail)
	}
	_ = tw.Flush()
	renderInventory(w, plan.CRDs)
	_, _ = fmt.Fprintf(w, "Operator:  %s -> %s\n", plan.Operator.FromImages["operator"], plan.Operator.ToImages["operator"])
	if len(plan.Values) > 0 {
		raw, err := yaml.Marshal(plan.Values)
		if err != nil {
			return fmt.Errorf("encode values: %w", err)
		}
		_, _ = fmt.Fprintln(w, "Values:    (in effect; the chart's defaults apply to everything else)")
		for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
			_, _ = fmt.Fprintln(w, "  "+line)
		}
	}
	return nil
}
