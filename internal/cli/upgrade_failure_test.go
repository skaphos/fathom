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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// A rollout that is not Available in time leaves the new release in place
// by default and says how to recover (FR-026).
func TestUpgradeTimeoutLeavesTheRelease(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.5.1", "0.5.1", nil, nil)
	env.rc.operatorReady = false
	_, _, err := execVerb(env.f, "upgrade", "--timeout", "30ms")
	if err == nil {
		t.Fatal("upgrade succeeded with an unavailable operator")
	}
	for _, want := range []string{
		"not Available within 30ms", "Available=False",
		"kubectl -n fathom-system describe deploy/fathom-fathom-operator",
		"--timeout 60ms",
		"helm rollback fathom -n fathom-system",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
	if env.log.index("release:rollback") >= 0 {
		t.Error("rolled back without --rollback-on-failure")
	}
	if env.rc.releases["fathom-system/fathom"].AppVersion != "0.6.0" {
		t.Error("the new release was not left in place")
	}
}

func TestUpgradeRollbackOnFailure(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.5.1", "0.5.1", nil, nil)
	env.rc.operatorReady = false
	_, _, err := execVerb(env.f, "upgrade", "--timeout", "30ms", "--rollback-on-failure")
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("err = %v, want a rolled-back failure", err)
	}
	rb, up := env.log.index("release:rollback"), env.log.index("release:upgrade")
	if rb < 0 || rb < up {
		t.Errorf("events = %v, want a rollback after the upgrade", env.log.all())
	}
	// CRDs are never reverted: still stamped with the new release.
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := env.c.Get(context.Background(), client.ObjectKey{Name: dnsCRD}, crd); err != nil {
		t.Fatal(err)
	}
	if crd.Annotations[crdVersionAnnotation] != "0.6.0" {
		t.Errorf("CRD reverted to %q", crd.Annotations[crdVersionAnnotation])
	}
}

// A new pod that never becomes ready fails the upgrade even though the old
// pod keeps the Deployment Available, and --rollback-on-failure fires
// (adversarial review F2).
func TestUpgradeDetectsAStuckSurge(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.5.1", "0.5.1", nil, nil)
	env.rc.surgeStuck = true
	_, _, err := execVerb(env.f, "upgrade", "--timeout", "30ms", "--rollback-on-failure")
	if err == nil || !strings.Contains(err.Error(), "old replica") || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("err = %v, want a stuck-surge failure that rolled back", err)
	}
	if env.log.index("release:rollback") < 0 {
		t.Error("no rollback for a stuck surge")
	}
}

// A first install left failed has no revision to roll back to; the refusal
// points at uninstall instead (adversarial review F6).
func TestPendingRefusalForAFailedFirstInstall(t *testing.T) {
	r := pendingRefusal(&install{Release: "fathom", Namespace: "fathom-system", Revision: 1, ReleaseStatus: "failed"})
	if strings.Contains(r.Next, "rollback") || !strings.Contains(r.Next, "fathomctl uninstall -n fathom-system") {
		t.Errorf("next = %q, want uninstall then reinstall", r.Next)
	}
	r = pendingRefusal(&install{Release: "fathom", Namespace: "fathom-system", Revision: 4, ReleaseStatus: "pending-upgrade"})
	if !strings.Contains(r.Next, "helm rollback fathom -n fathom-system") {
		t.Errorf("next = %q, want a rollback for a later revision", r.Next)
	}
}

// --rollback-on-failure is refused before any change when the plan applies an
// incompatible CRD change, sanctioned or not (FR-029).
func TestUpgradeRollbackOnFailureRefusedWithIncompatibleChanges(t *testing.T) {
	withVersion(t, "v0.6.0", true)
	env := upgradeEnv(t, "0.5.1", "0.5.1", unboundClusterHealthChildren, nil)
	before := len(env.log.all())
	_, _, err := execVerb(env.f, "upgrade", "--accept-incompatible-crds", "--rollback-on-failure")
	if err == nil || !strings.Contains(err.Error(), reasonRollbackRefused) {
		t.Fatalf("err = %v, want %s", err, reasonRollbackRefused)
	}
	if w := env.log.all()[before:]; len(w) != 0 {
		t.Errorf("refused upgrade wrote %v", w)
	}
}
