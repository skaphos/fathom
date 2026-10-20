/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/crdify/pkg/config"
	"sigs.k8s.io/crdify/pkg/runner"
	"sigs.k8s.io/yaml"

	helmchart "github.com/skaphos/fathom/deploy/helm"
)

const (
	// crdVersionAnnotation records which release last applied a CRD
	// (FR-015); status and install read it back.
	crdVersionAnnotation = "fathom.skaphos.io/version"
	// lastAppliedAnnotation is kubectl's client-side-apply record. A full
	// replace drops it so a later `kubectl apply` cannot resurrect old fields.
	lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"
)

// CRD plan actions (data-model "Upgrade plan").
const (
	crdAdd       = "add"
	crdUpdate    = "update"
	crdUnchanged = "unchanged"
	crdRefuse    = "refuse"
	// crdReplace: the target drops a version the live CRD stores, so the CRD
	// is deleted (with its objects) and created again (lifecycle_replace.go).
	crdReplace = "replace"
)

// crdFinding is one crdify incompatibility. Path is empty for CRD-level
// checks (scope, stored version removal).
type crdFinding struct {
	Check        string `json:"check"`
	Path         string `json:"path,omitempty"`
	Message      string `json:"message"`
	SanctionedBy string `json:"sanctionedBy,omitempty"`
}

// crdPlan is the outcome for one CRD.
type crdPlan struct {
	Name            string       `json:"name"`
	Scope           string       `json:"scope"`
	Action          string       `json:"action"`
	RecordedVersion string       `json:"recordedVersion,omitempty"`
	Findings        []crdFinding `json:"findings,omitempty"`
	// Replace only: the served versions before and after, the objects the
	// replace removes (or why they could not be listed), and how many it did.
	FromVersion    string      `json:"fromVersion,omitempty"`
	ToVersion      string      `json:"toVersion,omitempty"`
	Inventory      []objectRef `json:"inventory,omitempty"`
	InventoryErr   string      `json:"inventoryError,omitempty"`
	RemovedObjects int         `json:"removedObjects,omitempty"`

	live   *apiextensionsv1.CustomResourceDefinition
	target *apiextensionsv1.CustomResourceDefinition
}

// normalizeChartVersion turns the CLI version into a chart version.
func normalizeChartVersion(version string) (string, error) {
	return helmchart.NormalizeVersion(version)
}

// targetCRDs loads the release's CRDs from the embedded chart, by name.
func targetCRDs() ([]*apiextensionsv1.CustomResourceDefinition, error) {
	files, err := helmchart.Files()
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(files, helmchart.CRDDir)
	if err != nil {
		return nil, fmt.Errorf("read embedded CRDs: %w", err)
	}
	var crds []*apiextensionsv1.CustomResourceDefinition
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		raw, err := fs.ReadFile(files, path.Join(helmchart.CRDDir, e.Name()))
		if err != nil {
			return nil, err
		}
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := yaml.Unmarshal(raw, crd); err != nil {
			return nil, fmt.Errorf("decode embedded CRD %s: %w", e.Name(), err)
		}
		crds = append(crds, crd)
	}
	sort.Slice(crds, func(i, j int) bool { return crds[i].Name < crds[j].Name })
	return crds, nil
}

// planCRDs compares every target CRD with its live counterpart. Findings are
// raw here; matching them against the upgrade-path declaration is the
// caller's job, because only it knows the installed version.
func planCRDs(ctx context.Context, c client.Client, targets []*apiextensionsv1.CustomResourceDefinition) ([]crdPlan, error) {
	plans := make([]crdPlan, 0, len(targets))
	for _, target := range targets {
		p := crdPlan{Name: target.Name, Scope: string(target.Spec.Scope), target: target}
		live := &apiextensionsv1.CustomResourceDefinition{}
		err := c.Get(ctx, client.ObjectKey{Name: target.Name}, live)
		switch {
		case apierrors.IsNotFound(err):
			p.Action = crdAdd
			plans = append(plans, p)
			continue
		case err != nil:
			return nil, fmt.Errorf("get CRD %s: %w", target.Name, err)
		}
		p.live = live
		p.RecordedVersion = live.Annotations[crdVersionAnnotation]
		findings, err := compareCRD(live, target)
		if err != nil {
			return nil, err
		}
		p.Findings = findings
		switch {
		case len(storedVersionsDropped(live, target)) > 0:
			p.Action = crdReplace
			p.FromVersion, _ = servedVersion(live)
			p.ToVersion, _ = servedVersion(target)
			if p.Inventory, err = inventoryOf(ctx, c, live); err != nil {
				p.InventoryErr = err.Error()
			}
		case crdSpecEqual(live, target):
			p.Action = crdUnchanged
		default:
			p.Action = crdUpdate
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// crdifyConfig mirrors the repository's .crdify.yaml, so the CLI and the
// crd-compat CI gate apply the same rules (research R4); a test pins them.
func crdifyConfig() *config.Config {
	return &config.Config{
		Validations:          []config.ValidationConfig{{Name: "description", Enforcement: config.EnforcementPolicyNone}},
		UnhandledEnforcement: config.EnforcementPolicyError,
	}
}

// compareCRD runs crdify from live (old) to target (new) and returns every
// error-level finding.
func compareCRD(live, target *apiextensionsv1.CustomResourceDefinition) ([]crdFinding, error) {
	r, err := runner.New(crdifyConfig(), runner.DefaultRegistry())
	if err != nil {
		return nil, fmt.Errorf("configure crdify: %w", err)
	}
	results := r.Run(live, target)
	var findings []crdFinding
	for _, cr := range results.CRDValidation {
		if len(cr.Errors) > 0 {
			findings = append(findings, crdFinding{Check: cr.Name, Message: strings.Join(cr.Errors, "; ")})
		}
	}
	for _, vr := range append(results.SameVersionValidation, results.ServedVersionValidation...) {
		for _, pc := range vr.PropertyComparisons {
			for _, cr := range pc.ComparisonResults {
				if len(cr.Errors) > 0 {
					findings = append(findings, crdFinding{
						Check:   cr.Name,
						Path:    pc.Property,
						Message: vr.Version + ": " + strings.Join(cr.Errors, "; "),
					})
				}
			}
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Check < findings[j].Check
	})
	return dedupeFindings(findings), nil
}

func dedupeFindings(in []crdFinding) []crdFinding {
	var out []crdFinding
	seen := map[crdFinding]bool{}
	for _, f := range in {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// crdSpecEqual compares specs after API-server defaulting, so a live CRD the
// server defaulted still equals the embedded one it came from.
func crdSpecEqual(live, target *apiextensionsv1.CustomResourceDefinition) bool {
	a, b := live.DeepCopy(), target.DeepCopy()
	apiextensionsv1.SetObjectDefaults_CustomResourceDefinition(a)
	apiextensionsv1.SetObjectDefaults_CustomResourceDefinition(b)
	return equality.Semantic.DeepEqual(a.Spec, b.Spec)
}

// applyCRDs makes every live CRD exactly its target, stamped with version,
// then waits for each to be Established within timeout (FR-015, FR-020).
// Existing CRDs are replaced whole (Update with their resourceVersion), not
// merged: server-side apply only removes fields its own manager owned, so a
// CRD first created by helm or kubectl would keep properties and versions the
// target removed. A post-apply comparison proves the result.
func applyCRDs(ctx context.Context, f *factory, c client.Client, plans []crdPlan, version string, timeout time.Duration) error {
	for i := range plans {
		p := &plans[i]
		if p.Action == crdUnchanged && p.RecordedVersion == version {
			continue
		}
		if p.Action == crdReplace {
			if err := replaceCRD(ctx, f, c, p, version, timeout); err != nil {
				return err
			}
			continue
		}
		if err := applyCRD(ctx, c, p.target, version); err != nil {
			return err
		}
	}
	for _, p := range plans {
		if err := waitEstablished(ctx, f, c, p.Name, timeout); err != nil {
			return err
		}
	}
	return nil
}

func applyCRD(ctx context.Context, c client.Client, target *apiextensionsv1.CustomResourceDefinition, version string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		desired := target.DeepCopy()
		live := &apiextensionsv1.CustomResourceDefinition{}
		err := c.Get(ctx, client.ObjectKey{Name: target.Name}, live)
		switch {
		case apierrors.IsNotFound(err):
			desired.Annotations = withAnnotation(desired.Annotations, crdVersionAnnotation, version)
			return c.Create(ctx, desired, client.FieldOwner(fieldManager))
		case err != nil:
			return err
		}
		// Keep metadata other tools put on the CRD; replace everything we own.
		desired.ResourceVersion = live.ResourceVersion
		desired.Labels = mergeStringMaps(live.Labels, desired.Labels)
		desired.Annotations = mergeStringMaps(live.Annotations, desired.Annotations)
		delete(desired.Annotations, lastAppliedAnnotation)
		desired.Annotations[crdVersionAnnotation] = version
		return c.Update(ctx, desired, client.FieldOwner(fieldManager))
	})
	if err != nil {
		return fmt.Errorf("apply CRD %s: %w", target.Name, err)
	}
	applied := &apiextensionsv1.CustomResourceDefinition{}
	if err := c.Get(ctx, client.ObjectKey{Name: target.Name}, applied); err != nil {
		return fmt.Errorf("read back CRD %s: %w", target.Name, err)
	}
	if !crdSpecEqual(applied, target) {
		return fmt.Errorf("CRD %s does not match this release's schema after applying it; the operator rollout was not started", target.Name)
	}
	return nil
}

// waitEstablished waits for the CRD's Established condition.
func waitEstablished(ctx context.Context, f *factory, c client.Client, name string, timeout time.Duration) error {
	err := waitBounded(ctx, f, timeout, func(ctx context.Context) (bool, error) {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := c.Get(ctx, client.ObjectKey{Name: name}, crd); err != nil {
			return false, err
		}
		for _, cond := range crd.Status.Conditions {
			if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if errors.Is(err, errWaitTimeout) {
		return fmt.Errorf("CRD %s not Established within %s", name, timeout)
	}
	return err
}

func withAnnotation(m map[string]string, k, v string) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	m[k] = v
	return m
}

// mergeStringMaps returns base overlaid with over; neither input is changed.
func mergeStringMaps(base, over map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}
