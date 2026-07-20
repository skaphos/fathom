/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/skaphos/fathom/pkg/adapter"
)

// versionLabel is the standard Kubernetes label carrying an addon's release
// version (the primary detection source, per SKA-527).
const versionLabel = "app.kubernetes.io/version"

// versionFamily is the synthetic family the once-per-run version-gate
// CheckResult is emitted under — only when gating is active (VersionSource set
// and SupportedVersions non-empty). It is not a declared, policy-selectable
// family: it is absent from Capabilities().Families. Detection itself is keyed
// off VersionSource alone; SupportedVersions only controls whether a gate check
// is produced.
const versionFamily = adapter.Family("addon_version")

// detectAndGateVersion resolves the installed addon version from the
// definition's VersionSource and, when SupportedVersions is set, gates it
// against that range. It returns the detected version ("" when undetectable or
// when no VersionSource is configured) and zero or one Warn gate CheckResult to
// prepend to the run's checks:
//
//	no VersionSource                 -> ("", nil)
//	SupportedVersions == ""          -> (detected, nil)          detect-only, never Warn
//	detected in range                -> (detected, nil)
//	detected out of range            -> (detected, [Warn UnsupportedAddonVersion])
//	undetectable / unparseable       -> (detected, [Warn VersionUnknown])
//
// It never returns an error: detection is best-effort and must not fail a Run.
func (e *Engine) detectAndGateVersion(ctx context.Context, c client.Client, policy map[adapter.Family]adapter.FamilyPolicy) (string, []adapter.CheckResult) {
	if e.def.VersionSource == nil {
		return "", nil
	}
	addr, ok := e.resolveVersionAddress(policy)
	if !ok {
		// NewEngine rejects an unresolvable reference, so a constructed Engine
		// never lands here. Treat it as "no version source" rather than panicking.
		return "", nil
	}
	started := time.Now()
	detected := detectAddonVersion(ctx, c, addr)

	if e.def.SupportedVersions == "" {
		return detected, nil
	}

	ref := adapter.TargetRef{APIVersion: "apps/v1", Kind: string(addr.Kind), Namespace: addr.Namespace, Name: addr.Name}
	gate := func(outcome adapter.Outcome, summary, reason string) []adapter.CheckResult {
		details := adapter.MarkVersionGate(map[string]string{"component": e.def.AddonType}, reason, detected, e.def.SupportedVersions)
		return []adapter.CheckResult{result(versionFamily, ref, outcome, summary, details, started)}
	}

	constraint, err := semver.NewConstraint(e.def.SupportedVersions)
	if err != nil {
		// NewEngine validates the range, so a valid engine never reaches here.
		// Defensively treat a bad range as "cannot gate" rather than failing.
		return detected, nil
	}
	v, err := semver.NewVersion(detected)
	if detected == "" {
		return detected, gate(adapter.OutcomeWarn, "addon version could not be detected", adapter.ReasonVersionUnknown)
	}
	if err != nil {
		// Detected, but not parseable as semver (e.g. "nightly", a git SHA): a
		// distinct message from the undetectable case, so a HealthReport carrying
		// detectedVersion is not contradicted by "could not be detected".
		return detected, gate(adapter.OutcomeWarn,
			fmt.Sprintf("detected addon version %q is not valid semver", detected), adapter.ReasonVersionUnknown)
	}
	// Gate on the base release (major.minor.patch), so a prerelease or build of a
	// supported release (e.g. "1.15.0-rc.1" within ">=1.14 <2.0") counts as in
	// range. Masterminds/semver otherwise rejects ANY prerelease unless the range
	// itself carries a prerelease comparator, which would falsely Warn on common
	// RC/beta/canary addon builds. The full detected string is still surfaced.
	base := semver.New(v.Major(), v.Minor(), v.Patch(), "", "")
	if !constraint.Check(base) {
		return detected, gate(adapter.OutcomeWarn,
			fmt.Sprintf("detected addon version %s is outside the supported range %q", detected, e.def.SupportedVersions),
			adapter.ReasonUnsupportedAddonVersion)
	}
	return detected, nil
}

// versionAddress is the effective, policy-resolved address of the workload that
// reports the addon version.
type versionAddress struct {
	Kind      WorkloadKind
	Namespace string
	Name      string
	Container string
}

// resolveVersionAddress computes the version-source workload's effective address
// by reusing the referenced WorkloadCheck and applying that family's policy
// overrides — the very surface the workload check itself resolves through
// (firstNamespace for policy.namespaces, its NameThresholdKey for the name). A
// renamed or relocated addon therefore keeps reporting its version instead of
// silently detecting nothing (#172).
//
// The referenced family's policy is used even when that family is disabled:
// version detection is orthogonal to whether the family's checks run, and an
// operator who renamed a workload expects detection to follow the rename either
// way. A family absent from a non-nil policy map yields a zero FamilyPolicy, so
// both helpers fall back to the check's declared defaults.
func (e *Engine) resolveVersionAddress(policy map[adapter.Family]adapter.FamilyPolicy) (versionAddress, bool) {
	fam, wc, ok := e.def.versionWorkload()
	if !ok {
		return versionAddress{}, false
	}
	fp, _ := resolveFamily(policy, fam.Name, fam.DefaultEnabled)
	name := wc.DefaultName
	if wc.NameThresholdKey != "" {
		name = stringThreshold(fp, wc.NameThresholdKey, wc.DefaultName)
	}
	return versionAddress{
		Kind:      wc.Kind,
		Namespace: firstNamespace(fp, wc.DefaultNamespace),
		Name:      name,
		Container: e.def.VersionSource.Container,
	}, true
}

// detectAddonVersion reads the version-source workload and returns the addon
// version from its app.kubernetes.io/version label — the pod template's labels
// first (they propagate to the live pods), then the workload's own metadata —
// falling back to the selected container's image tag. It returns "" when the
// workload is absent or carries no usable version. Any read error yields ""
// (best-effort).
func detectAddonVersion(ctx context.Context, c client.Client, addr versionAddress) string {
	key := types.NamespacedName{Namespace: addr.Namespace, Name: addr.Name}
	var meta, podMeta map[string]string
	var containers []corev1.Container
	switch addr.Kind {
	case KindDeployment:
		var w appsv1.Deployment
		if err := c.Get(ctx, key, &w); err != nil {
			return ""
		}
		meta, podMeta, containers = w.Labels, w.Spec.Template.Labels, w.Spec.Template.Spec.Containers
	case KindDaemonSet:
		var w appsv1.DaemonSet
		if err := c.Get(ctx, key, &w); err != nil {
			return ""
		}
		meta, podMeta, containers = w.Labels, w.Spec.Template.Labels, w.Spec.Template.Spec.Containers
	case KindStatefulSet:
		var w appsv1.StatefulSet
		if err := c.Get(ctx, key, &w); err != nil {
			return ""
		}
		meta, podMeta, containers = w.Labels, w.Spec.Template.Labels, w.Spec.Template.Spec.Containers
	default:
		return ""
	}
	if v := podMeta[versionLabel]; v != "" {
		return v
	}
	if v := meta[versionLabel]; v != "" {
		return v
	}
	return imageTag(pickImage(containers, addr.Container))
}

// pickImage returns the image reference of the named container, or the first
// container's image when name is "" or not found.
func pickImage(containers []corev1.Container, name string) string {
	if len(containers) == 0 {
		return ""
	}
	if name != "" {
		for i := range containers {
			if containers[i].Name == name {
				return containers[i].Image
			}
		}
	}
	return containers[0].Image
}

// imageTag extracts the tag from a container image reference, e.g.
// "quay.io/cilium/cilium:v1.15.6@sha256:abc" -> "v1.15.6". It returns "" when
// there is no tag or the tag is "latest" (treated as no usable version), and it
// does not mistake a registry-port colon (host:port/repo) for a tag.
func imageTag(ref string) string {
	if ref == "" {
		return ""
	}
	if at := strings.LastIndex(ref, "@"); at >= 0 {
		ref = ref[:at] // strip any digest
	}
	// A tag colon must come after the last "/", else the only colon is a
	// registry port (host:port/repo) and there is no tag.
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		tag := ref[colon+1:]
		if tag != "" && tag != "latest" {
			return tag
		}
	}
	return ""
}

// validSupportedVersions returns an error when s is a non-empty but malformed
// semver range, so NewEngine can reject it at construction.
func validSupportedVersions(s string) error {
	if s == "" {
		return nil
	}
	if _, err := semver.NewConstraint(s); err != nil {
		return fmt.Errorf("invalid supportedVersions range %q: %w", s, err)
	}
	return nil
}
