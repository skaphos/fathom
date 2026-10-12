/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// reportOwnedBy is a HealthReport with a controller reference to its check,
// as the operator writes it.
func reportOwnedBy(check *fathomv1alpha1.AddonCheck) *fathomv1alpha1.HealthReport {
	return &fathomv1alpha1.HealthReport{ObjectMeta: metav1.ObjectMeta{
		Name: "coredns-report-1", Namespace: check.Namespace,
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: "AddonCheck",
			Name: check.Name, UID: "11111111-2222-3333-4444-555555555555", Controller: ptr.To(true),
		}},
	}}
}

func TestPurgePromptListsEveryKindAndMentionsExport(t *testing.T) {
	env := installedEnv(t)
	env.f.isTerminal = func() bool { return true }
	env.f.stdin = strings.NewReader("n\n")
	before := len(env.log.all())
	_, errOut, err := execVerb(env.f, "uninstall", "--purge-crds")
	if err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("err = %v, want a declined confirmation", err)
	}
	for _, kind := range purgeKinds {
		if !strings.Contains(errOut, kind) {
			t.Errorf("prompt does not count %s:\n%s", kind, errOut)
		}
	}
	if !regexp.MustCompile(`(?m)^\s+AddonCheck\s+1$`).MatchString(errOut) {
		t.Errorf("prompt count for AddonCheck is wrong:\n%s", errOut)
	}
	if !strings.Contains(errOut, "--export <file>") {
		t.Errorf("prompt does not mention --export:\n%s", errOut)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("declined purge changed the cluster: %v", w)
	}
}

func TestPurgeWithoutTerminalOrYesRefuses(t *testing.T) {
	env := installedEnv(t)
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "uninstall", "--purge-crds")
	if err == nil || !strings.Contains(err.Error(), "confirmation-required") || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("err = %v, want confirmation-required naming --yes", err)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused purge changed the cluster: %v", w)
	}
}

func TestPurgeWithYesDeletesResourcesThenCRDs(t *testing.T) {
	check := addonCheck("team-a", "coredns")
	env := installedEnv(t, reportOwnedBy(check))
	before := len(env.log.all())
	if _, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--yes"); err != nil {
		t.Fatalf("uninstall --purge-crds --yes: %v", err)
	}
	events := env.log.all()[before:]
	lastResource, firstCRD := -1, -1
	for i, e := range events {
		if strings.HasPrefix(e, "delete Unstructured") {
			lastResource = i
		}
		if strings.HasPrefix(e, "delete CustomResourceDefinition") && firstCRD < 0 {
			firstCRD = i
		}
	}
	if lastResource < 0 || firstCRD < 0 || lastResource > firstCRD {
		t.Errorf("events = %v, want every resource deleted before any CRD", events)
	}
	var crds apiextensionsv1.CustomResourceDefinitionList
	_ = env.c.List(context.Background(), &crds)
	if len(crds.Items) != 0 {
		t.Errorf("%d CRDs remain after purge", len(crds.Items))
	}
	if exists(t, env.c, check) {
		t.Error("a check survived the purge")
	}
}

func TestPurgeExportIsWrittenFirstAndStripped(t *testing.T) {
	check := addonCheck("team-a", "coredns")
	report := reportOwnedBy(check)
	env := installedEnv(t, report)
	path := filepath.Join(t.TempDir(), "backup.yaml")
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--export", path, "--yes")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "Exported 2 Fathom resource(s)") || strings.Contains(out, "Re-exported") {
		t.Errorf("output = %q", out)
	}
	// The export precedes every deletion.
	if strings.Index(out, "Exported") > strings.Index(out, "Removed release") {
		t.Errorf("export reported after deletions began:\n%s", out)
	}
	_ = before
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	docs := strings.Split(string(raw), "---\n")[1:]
	if len(docs) != 2 {
		t.Fatalf("export has %d documents, want 2:\n%s", len(docs), raw)
	}
	// Dependency order: the check before the report it owns.
	if !strings.Contains(docs[0], "kind: AddonCheck") || !strings.Contains(docs[1], "kind: HealthReport") {
		t.Errorf("export order wrong:\n%s", raw)
	}
	for _, doc := range docs {
		var obj map[string]any
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			t.Fatal(err)
		}
		md, _ := obj["metadata"].(map[string]any)
		for _, field := range []string{"uid", "resourceVersion", "managedFields", "creationTimestamp", "generation", "ownerReferences"} {
			if _, ok := md[field]; ok {
				t.Errorf("exported %s keeps metadata.%s", md["name"], field)
			}
		}
		if _, ok := obj["status"]; ok {
			t.Errorf("exported %s keeps status", md["name"])
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".fathom-export-*")); len(matches) != 0 {
		t.Errorf("temporary export files left behind: %v", matches)
	}
}

// The operator can write until it stops; the export must still hold every
// resource the purge deletes.
func TestPurgeExportCoversResourcesWrittenDuringUninstall(t *testing.T) {
	check := addonCheck("team-a", "coredns")
	env := installedEnv(t, reportOwnedBy(check))
	env.rc.onUninstall = func(ctx context.Context) {
		late := reportOwnedBy(check)
		late.Name = "coredns-report-2"
		if err := env.c.Create(ctx, late); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "backup.yaml")
	out, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--export", path, "--yes")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "Re-exported 3 Fathom resource(s)") || !strings.Contains(out, "Deleted 3 Fathom resource(s)") {
		t.Errorf("output = %q", out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "coredns-report-2") {
		t.Errorf("export misses the report written during the uninstall:\n%s", raw)
	}
}

func TestPurgeExportFailureDeletesNothing(t *testing.T) {
	env := installedEnv(t)
	before := len(env.log.all())
	path := filepath.Join(t.TempDir(), "missing-dir", "backup.yaml")
	_, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--export", path, "--yes")
	if err == nil || !strings.Contains(err.Error(), "nothing was deleted") {
		t.Fatalf("err = %v, want an export failure that deleted nothing", err)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("failed export still changed the cluster: %v", w)
	}
}

// After a plain uninstall only CRDs remain; --purge-crds still removes them.
func TestPurgeWithOnlyCRDsLeft(t *testing.T) {
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", nil), addonCheck("team-a", "coredns"))...)
	out, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--yes")
	if err != nil {
		t.Fatalf("uninstall --purge-crds: %v", err)
	}
	if !strings.Contains(out, "every fathom.skaphos.io CRD") {
		t.Errorf("output = %q", out)
	}
	if exists(t, env.c, &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: dnsCRD}}) {
		t.Error("CRDs survived")
	}
}

// With no release namespace to find, the purge still runs the permission
// preflight, so a missing permission refuses before anything is deleted.
func TestPurgeWithOnlyCRDsLeftChecksPermissionsFirst(t *testing.T) {
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", nil), addonCheck("team-a", "coredns"))...)
	env.denied["delete customresourcedefinitions"] = true
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "uninstall", "--purge-crds", "--yes")
	var r refusal
	if !errors.As(err, &r) || r.Reason != reasonMissingPermissions || !strings.Contains(r.Error(), "customresourcedefinitions") {
		t.Fatalf("err = %v, want a %s refusal naming the CRD permission", err, reasonMissingPermissions)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused purge changed the cluster: %v", w)
	}
}

var _ client.Object = (*fathomv1alpha1.HealthReport)(nil)
