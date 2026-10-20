/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The 0.6.x -> 0.7.0 shape: the check CRDs serve and store v1alpha1 while
// this release's CRDs serve v1 (contracts/cli-upgrade-export.md).

const addonCheckCRD = "addonchecks.fathom.skaphos.io"

func alphaCheckCRDs(c *apiextensionsv1.CustomResourceDefinition) {
	if c.Spec.Names.Kind == "AddonDefinition" || c.Spec.Names.Kind == "AddonDefinitionBinding" {
		return
	}
	c.Spec.Versions[0].Name = alphaVersion
	c.Status.StoredVersions = []string{alphaVersion}
}

func alphaAddonCheck(ns, name string) unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetAPIVersion("fathom.skaphos.io/" + alphaVersion)
	u.SetKind("AddonCheck")
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

// replaceEnv is a 0.6.0 install whose seven check CRDs still serve alpha,
// holding the given alpha AddonChecks, seen by a v0.7.0 CLI.
func replaceEnv(t *testing.T, checks ...unstructured.Unstructured) *lifecycleEnv {
	t.Helper()
	withVersion(t, "v0.7.0", true)
	env := upgradeEnv(t, "0.6.0", "0.6.0", alphaCheckCRDs, nil)
	env.alpha["AddonCheckList"] = checks
	return env
}

func TestPlanDetectsAStoredVersionReplacement(t *testing.T) {
	env := replaceEnv(t, alphaAddonCheck("team-b", "zeta"), alphaAddonCheck("team-a", "coredns"))
	targets, err := targetCRDs()
	if err != nil {
		t.Fatal(err)
	}
	plans, err := planCRDs(context.Background(), env.c, targets)
	if err != nil {
		t.Fatal(err)
	}
	var addon, definition *crdPlan
	for i := range plans {
		switch plans[i].Name {
		case addonCheckCRD:
			addon = &plans[i]
		case "addondefinitions.fathom.skaphos.io":
			definition = &plans[i]
		}
	}
	if addon == nil || addon.Action != crdReplace || addon.FromVersion != alphaVersion || addon.ToVersion != "v1" {
		t.Fatalf("addonchecks plan = %+v, want replace v1alpha1 -> v1", addon)
	}
	if got := len(addon.Inventory); got != 2 || addon.Inventory[0].String() != "team-a/coredns" || addon.Inventory[1].String() != "team-b/zeta" {
		t.Fatalf("inventory = %v, want the two checks sorted by namespace then name", addon.Inventory)
	}
	if definition == nil || definition.Action != crdUnchanged {
		t.Fatalf("addondefinitions plan = %+v, want unchanged (it stays alpha)", definition)
	}
	found := false
	for _, f := range addon.Findings {
		if f.Check == "storedVersionRemoval" && f.Path == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("findings = %+v, want a CRD-level storedVersionRemoval", addon.Findings)
	}
}

func TestUpgradeReplaceRefusesWithTheInventoryUntilAcknowledged(t *testing.T) {
	env := replaceEnv(t, alphaAddonCheck("team-a", "coredns"))
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "upgrade")
	if err == nil || !strings.Contains(err.Error(), reasonAckRequired) {
		t.Fatalf("err = %v, want %s", err, reasonAckRequired)
	}
	for _, want := range []string{
		"replace crd/" + addonCheckCRD + " (v1alpha1 -> v1): 1 objects will be removed",
		"  team-a/coredns",
		"  and their HealthReport history",
		"replace crd/healthreports.fathom.skaphos.io (v1alpha1 -> v1): 0 objects will be removed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(err.Error(), "1 objects will be removed") || !strings.Contains(err.Error(), "issues/149") {
		t.Errorf("acknowledgement detail should name the removal and the sanction: %v", err)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused upgrade changed the cluster: %v", w)
	}
}

func TestUpgradeReplaceWithNoObjectsNeedsNoAcknowledgement(t *testing.T) {
	env := replaceEnv(t)
	out, _, err := execVerb(env.f, "upgrade")
	if err != nil {
		t.Fatalf("upgrade with an empty inventory: %v\n%s", err, out)
	}
	if !strings.Contains(out, "replaced crd/"+addonCheckCRD+": removed 0 objects") {
		t.Errorf("output lacks the replaced line:\n%s", out)
	}
}

func TestUpgradeReplacesTheCRDsWithTheFlag(t *testing.T) {
	env := replaceEnv(t, alphaAddonCheck("team-a", "coredns"))
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "upgrade", "--accept-incompatible-crds")
	if err != nil {
		t.Fatalf("upgrade --accept-incompatible-crds: %v\n%s", err, out)
	}
	events := env.log.all()[before:]
	del, create, rel := -1, -1, -1
	for i, e := range events {
		switch {
		case e == "delete CustomResourceDefinition /"+addonCheckCRD && del < 0:
			del = i
		case e == "create CustomResourceDefinition /"+addonCheckCRD && create < 0:
			create = i
		case strings.HasPrefix(e, "release:upgrade"):
			rel = i
		}
	}
	if del < 0 || create < 0 || rel < 0 || del >= create || create >= rel {
		t.Fatalf("events = %v, want delete, then create, then the release upgrade", events)
	}
	for _, e := range events {
		if strings.HasPrefix(e, "update CustomResourceDefinition /"+addonCheckCRD) {
			t.Fatalf("a replaced CRD must not be updated in place: %v", events)
		}
	}
	if !strings.Contains(out, "replaced crd/"+addonCheckCRD+": removed 1 objects") {
		t.Errorf("output lacks the replaced line:\n%s", out)
	}
	got := &apiextensionsv1.CustomResourceDefinition{}
	if err := env.c.Get(context.Background(), client.ObjectKey{Name: addonCheckCRD}, got); err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Versions) != 1 || got.Spec.Versions[0].Name != "v1" || got.Annotations[crdVersionAnnotation] != "0.7.0" {
		t.Errorf("live CRD after replace: versions=%v annotation=%q", got.Spec.Versions, got.Annotations[crdVersionAnnotation])
	}
}

func TestUpgradeReplaceDryRunPrintsTheInventoryAndChangesNothing(t *testing.T) {
	env := replaceEnv(t, alphaAddonCheck("team-a", "coredns"))
	before := len(env.log.all())
	out, _, err := execVerb(env.f, "upgrade", "--dry-run", "--accept-incompatible-crds")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, "replace crd/"+addonCheckCRD+" (v1alpha1 -> v1): 1 objects will be removed") {
		t.Errorf("dry-run plan lacks the inventory:\n%s", out)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("dry run changed the cluster: %v", w)
	}
}

func TestUpgradeReplaceRefusesRollbackOnFailure(t *testing.T) {
	env := replaceEnv(t)
	_, _, err := execVerb(env.f, "upgrade", "--rollback-on-failure", "--accept-incompatible-crds")
	if err == nil || !strings.Contains(err.Error(), reasonRollbackRefused) {
		t.Fatalf("err = %v, want %s", err, reasonRollbackRefused)
	}
}

func TestUpgradeReplaceRefusesWhenTheInventoryCannotBeListed(t *testing.T) {
	env := replaceEnv(t)
	env.listForbidden["AddonCheckList"] = true
	out, _, err := execVerb(env.f, "upgrade")
	if err == nil || !strings.Contains(err.Error(), reasonAckRequired) {
		t.Fatalf("err = %v, want %s (an unlistable inventory still needs consent)", err, reasonAckRequired)
	}
	if !strings.Contains(out, "objects could not be listed") {
		t.Errorf("plan should say the inventory failed:\n%s", out)
	}
}

func TestUpgradeReplaceTreatsAVanishedCRDAsReplaced(t *testing.T) {
	env := replaceEnv(t)
	deleted := false
	env.beforeDelete = func(obj client.Object) error {
		if crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition); ok && crd.Name == addonCheckCRD && !deleted {
			deleted = true
			// Someone else removed it between the plan and the delete.
			return env.c.Delete(context.Background(), crd)
		}
		return nil
	}
	if _, _, err := execVerb(env.f, "upgrade"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
}

func TestInstallOverLeftoverAlphaCRDsRefuses(t *testing.T) {
	withVersion(t, "v0.7.0", true)
	env := newLifecycleEnv(t, append(liveCRDs(t, "0.6.0", alphaCheckCRDs), namespaceObj("fathom-system"))...)
	env.alpha["AddonCheckList"] = []unstructured.Unstructured{alphaAddonCheck("team-a", "coredns")}
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "install", "--accept-incompatible-crds")
	if err == nil || !strings.Contains(err.Error(), reasonLeftoverStoredVersion) {
		t.Fatalf("err = %v, want %s", err, reasonLeftoverStoredVersion)
	}
	if !strings.Contains(err.Error(), "fathomctl export") || !strings.Contains(err.Error(), "kubectl delete crd "+addonCheckCRD) {
		t.Errorf("refusal should name the export-delete-install sequence: %v", err)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused install changed the cluster: %v", w)
	}
}

func TestStatusNamesTheUpgradeRequiredForAlphaCRDs(t *testing.T) {
	env := replaceEnv(t)
	out, _, err := execVerb(env.f, "status")
	if err == nil {
		t.Fatal("status must exit 1 while a CRD is outdated")
	}
	if !strings.Contains(out, "serves v1alpha1; run fathomctl export, then fathomctl upgrade") {
		t.Errorf("status output lacks the upgrade-required reason:\n%s", out)
	}
}

// Helm never touches crds/ on upgrade, so a Helm-managed install can sit at
// 0.7.0 with CRDs that still store v1alpha1. The same-version fathomctl
// upgrade that `status` recommends must judge the replace by 0.7.0's own
// sanction instead of dead-ending on unsanctioned-incompatible.
func TestUpgradeAtTheSameVersionStillSanctionsTheReplace(t *testing.T) {
	withVersion(t, "v0.7.0", true)
	env := upgradeEnv(t, "0.7.0", "0.6.0", alphaCheckCRDs, nil)
	env.alpha["AddonCheckList"] = []unstructured.Unstructured{alphaAddonCheck("team-a", "coredns")}
	_, _, err := execVerb(env.f, "upgrade")
	if err == nil || !strings.Contains(err.Error(), reasonAckRequired) {
		t.Fatalf("err = %v, want %s", err, reasonAckRequired)
	}
	out, _, err := execVerb(env.f, "upgrade", "--accept-incompatible-crds")
	if err != nil {
		t.Fatalf("same-version upgrade --accept-incompatible-crds: %v\n%s", err, out)
	}
	if !strings.Contains(out, "replaced crd/"+addonCheckCRD+": removed 1 objects") {
		t.Errorf("output lacks the replaced line:\n%s", out)
	}
}

// A replace deletes CRDs; the generic preflight never asked for that verb, so
// an identity with create/update/get would have failed part-way through.
func TestUpgradeReplaceRefusesWithoutDeleteOnCRDs(t *testing.T) {
	env := replaceEnv(t)
	env.denied["delete customresourcedefinitions"] = true
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "upgrade", "--accept-incompatible-crds")
	if err == nil || !strings.Contains(err.Error(), reasonMissingPermissions) || !strings.Contains(err.Error(), "delete customresourcedefinitions") {
		t.Fatalf("err = %v, want missing-permissions naming delete customresourcedefinitions", err)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused upgrade changed the cluster: %v", w)
	}
}

// Without a sanction on the path, the refusal must name the documented manual
// sequence rather than the generic "apply the new CRD manually" advice, which
// the API server rejects for a stored version.
func TestUpgradeReplaceWithoutASanctionPointsAtTheManualSequence(t *testing.T) {
	withVersion(t, "v0.6.1", true)
	env := upgradeEnv(t, "0.6.0", "0.6.0", alphaCheckCRDs, nil)
	_, _, err := execVerb(env.f, "upgrade")
	if err == nil || !strings.Contains(err.Error(), reasonUnsanctioned) {
		t.Fatalf("err = %v, want %s", err, reasonUnsanctioned)
	}
	if !strings.Contains(err.Error(), "RELEASE.md") {
		t.Errorf("refusal does not point at the documented sequence:\n%v", err)
	}
}

// The same-version window must not let 0.7.0's "*" swap sanction rewrite
// CRDs a later 0.7.x applied: upgrade refuses CRDs recorded by a newer
// release before any sanction is judged, as install already did.
func TestUpgradeRefusesCRDsRecordedByANewerRelease(t *testing.T) {
	withVersion(t, "v0.7.0", true)
	env := upgradeEnv(t, "0.7.0", "0.7.1", unboundClusterHealthChildren, nil)
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "upgrade", "--accept-incompatible-crds")
	if err == nil || !strings.Contains(err.Error(), reasonCRDDowngrade) {
		t.Fatalf("err = %v, want %s", err, reasonCRDDowngrade)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused upgrade changed the cluster: %v", w)
	}
}
