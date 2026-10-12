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
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CRD states (FR-040).
const (
	crdCurrent  = "current"
	crdOutdated = "outdated"
	crdNewer    = "newer"
	crdMissing  = "missing"
	crdUnknown  = "unknown"
)

// crdStatus is one CRD's currency against the installed operator.
type crdStatus struct {
	Name            string `json:"name"`
	Scope           string `json:"scope"`
	RecordedVersion string `json:"recordedVersion,omitempty"`
	State           string `json:"state"`
	Reason          string `json:"reason,omitempty"`
}

// statusReport is `status -o json|yaml`: the Install entity plus crds[] and
// upgradePath.
type statusReport struct {
	install
	Installed   bool        `json:"installed"`
	CRDs        []crdStatus `json:"crds,omitempty"`
	UpgradePath string      `json:"upgradePath,omitempty"`
	CLIVersion  string      `json:"cliVersion"`
	Problems    []string    `json:"problems,omitempty"`
}

func newStatusCommand(f *factory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report whether Fathom is installed, at which version, and whether it is healthy",
		Long: `status reports whether Fathom is installed and, when it is: the managing tool,
the Helm release and namespace, the operator version and images (and whether
they are pinned by digest), the operator's readiness, whether each CRD matches
the operator's version, and the upgrade path from the installed version to
this fathomctl's.

It is read-only and needs only: list Deployments across namespaces (to find
the operator), get/list Secrets in the operator's namespace and the -n
namespace (Helm release records) and get/list CRDs — no cluster-wide Secret
access.

Exit codes: 0 when Fathom is installed and current, or not installed at all;
1 when the operator is not Available, a CRD is outdated, newer or missing, the
release is pending or failed, or more than one Fathom release exists. A CRD
whose version cannot be determined is reported as unknown and does not fail
status by itself.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(commandContext(cmd), cmd, f)
		},
	}
}

func runStatus(ctx context.Context, cmd *cobra.Command, f *factory) error {
	c, err := f.client()
	if err != nil {
		return err
	}
	rc, err := f.newReleaseClient(f)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	in, err := detectInstall(ctx, c, rc, f.opts.namespace)
	var r refusal
	if errors.As(err, &r) && r.Reason == reasonMultipleInstalls {
		// Reported, not refused: status describes the cluster as it is.
		_, _ = fmt.Fprintf(out, "Fathom:     %s\n", r.Detail)
		return errors.New("more than one Fathom install")
	}
	if err != nil {
		return err
	}
	report, err := buildStatus(ctx, c, in)
	if err != nil {
		return err
	}
	if f.opts.output.structured() {
		if err := encode(out, f.opts.output, report); err != nil {
			return err
		}
	} else {
		printStatus(out, report)
	}
	if len(report.Problems) > 0 {
		return errors.New("fathom is installed but not healthy: " + strings.Join(report.Problems, "; "))
	}
	return nil
}

func buildStatus(ctx context.Context, c client.Client, in *install) (*statusReport, error) {
	report := &statusReport{install: *in, CLIVersion: clientVersion()}
	if in.ManagedBy == managedByNone {
		return report, nil
	}
	report.Installed = true
	if in.pending() {
		report.Problems = append(report.Problems, fmt.Sprintf("release %s is %s", in.Release, in.ReleaseStatus))
	}
	if !in.Ready {
		report.Problems = append(report.Problems, "operator not Available: "+in.ReadyReason)
	}
	crds, err := crdStatuses(ctx, c, in.Version)
	if err != nil {
		return nil, err
	}
	report.CRDs = crds
	var bad []string
	for _, s := range crds {
		switch s.State {
		case crdOutdated, crdNewer, crdMissing:
			bad = append(bad, s.Name+" ("+s.State+")")
		}
	}
	if len(bad) > 0 {
		report.Problems = append(report.Problems, "CRDs not current: "+strings.Join(bad, ", "))
	}
	report.UpgradePath = statusUpgradePath(in.Version)
	return report, nil
}

// crdStatuses judges each shipped CRD against the operator's version by the
// version recorded on it (FR-015), never by comparing against this CLI's
// embedded CRDs unless the CLI is the same release (FR-040).
func crdStatuses(ctx context.Context, c client.Client, operator string) ([]crdStatus, error) {
	targets, err := targetCRDs()
	if err != nil {
		return nil, err
	}
	plans, err := planCRDs(ctx, c, targets)
	if err != nil {
		return nil, err
	}
	opV, opErr := semver.NewVersion(operator)
	cliV, _ := normalizeChartVersion(clientVersion())
	var out []crdStatus
	for _, p := range plans {
		s := crdStatus{Name: p.Name, Scope: p.Scope, RecordedVersion: p.RecordedVersion}
		switch {
		case p.Action == crdAdd:
			s.State = crdMissing
		case p.RecordedVersion != "" && opErr == nil:
			recV, err := semver.NewVersion(p.RecordedVersion)
			switch {
			case err != nil:
				s.State, s.Reason = crdUnknown, "recorded version is not SemVer"
			case recV.Equal(opV):
				s.State = crdCurrent
			case recV.LessThan(opV):
				s.State = crdOutdated
			default:
				s.State = crdNewer
			}
		case opErr == nil && cliV != "" && semver.MustParse(cliV).Equal(opV):
			// No recorded version (applied by helm or kubectl), but this CLI is
			// the operator's release, so its embedded CRDs are the reference.
			if p.Action == crdUnchanged {
				s.State = crdCurrent
			} else {
				s.State = crdOutdated
				s.Reason = "differs from v" + cliV + "'s schema"
			}
		default:
			s.State = crdUnknown
			s.Reason = "no recorded version (applied by helm or kubectl); run the matching fathomctl's status to compare"
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// statusUpgradePath describes how to get from the installed version to this
// CLI's (FR-028).
func statusUpgradePath(installed string) string {
	cli, err := normalizeChartVersion(clientVersion())
	if err != nil {
		return "unknown (this fathomctl is a development build without a SemVer version)"
	}
	from, err := semver.NewVersion(installed)
	if err != nil {
		return "unknown (installed version " + installed + " is not SemVer)"
	}
	to := semver.MustParse(cli)
	switch {
	case from.Equal(to):
		return "none needed (already at this fathomctl's version)"
	case from.GreaterThan(to):
		return fmt.Sprintf("none (this fathomctl, v%s, is older than the install; use fathomctl v%s)", to, from)
	}
	raw, err := readUpgradePathFile()
	if err != nil {
		return "unknown (" + err.Error() + ")"
	}
	decl, err := parseUpgradePath(raw)
	if err != nil {
		return "unknown (" + err.Error() + ")"
	}
	res := decl.upgradePath(from, to)
	if res.BelowFloor {
		return fmt.Sprintf("blocked: v%s predates the upgrade floor v%s; upgrade with Helm to v%d.%d.x first", from, decl.Floor, decl.floor.Major(), decl.floor.Minor())
	}
	return res.render(from.String(), to.String())
}

func printStatus(w io.Writer, r *statusReport) {
	if !r.Installed {
		_, _ = fmt.Fprintln(w, "Fathom:     not installed")
		return
	}
	switch r.ManagedBy {
	case managedByHelm:
		_, _ = fmt.Fprintf(w, "Fathom:     installed (helm release %s in %s, revision %d, by %s", r.Release, r.Namespace, r.Revision, r.InstalledBy)
		if r.pending() {
			_, _ = fmt.Fprintf(w, ", %s", r.ReleaseStatus)
		}
		_, _ = fmt.Fprintln(w, ")")
	default:
		_, _ = fmt.Fprintf(w, "Fathom:     installed in %s by %s; not managed by fathomctl\n", r.Namespace, r.ManagedBy)
	}
	_, _ = fmt.Fprintf(w, "Version:    v%s (images %s)\n", strings.TrimPrefix(r.Version, "v"), describePinning(r.Pinning))
	for _, name := range []string{"operator", "probe", "node-agent"} {
		if ref := r.Images[name]; ref != "" {
			_, _ = fmt.Fprintf(w, "  %-11s%s\n", name, ref)
		}
	}
	if r.Ready {
		_, _ = fmt.Fprintln(w, "Operator:   Available")
	} else {
		_, _ = fmt.Fprintf(w, "Operator:   not Available (%s)\n", r.ReadyReason)
	}
	current := 0
	for _, c := range r.CRDs {
		if c.State == crdCurrent {
			current++
		}
	}
	_, _ = fmt.Fprintf(w, "CRDs:       %d/%d current\n", current, len(r.CRDs))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range r.CRDs {
		if c.State == crdCurrent {
			continue
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", c.Name, c.State, c.Reason)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "Upgrade to this fathomctl (%s): %s\n", r.CLIVersion, r.UpgradePath)
}
