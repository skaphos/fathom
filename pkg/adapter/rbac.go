/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package adapter

// PolicyRule is a least-privilege RBAC grant an adapter declares. It mirrors the
// shape of rbacv1.PolicyRule (without depending on k8s.io/api, so the contract
// stays dependency-light) so an adapter can declare exactly the reads — and the
// few audited writes — its Run performs. Fathom generates one ServiceAccount and
// a read-only ClusterRole per addon from these rules and, at reconcile time,
// hands the adapter a client impersonating that ServiceAccount via
// [Request.Client]. An adapter's blast radius is therefore exactly its declared
// rules (SKA-58).
type PolicyRule struct {
	// APIGroups is the set of API groups the rule applies to. The core group is
	// the empty string "".
	APIGroups []string
	// Resources is the set of resource plurals (e.g. "deployments", "pods").
	Resources []string
	// Verbs is the set of verbs granted (e.g. "get", "list", "watch").
	Verbs []string
	// Justification defends this grant for the least-privilege audit: why the
	// rule is needed AND why a narrower grant (fewer verbs, fewer resources,
	// namespaced instead of cluster-scoped, read-only instead of write) would not
	// be sufficient. It is REQUIRED on every rule — the read-only RBAC guard fails
	// any rule with an empty Justification — and is rendered into the generated
	// RBAC matrix (docs/reference/rbac.md), so every permission the operator
	// impersonates is documented and defensible next to the grant, with no
	// separate table to drift from.
	//
	// For a write verb (anything outside {get, list, watch}) the justification
	// must additionally explain why the write is necessary and why a read cannot
	// achieve it. The two shipped writes are the CoreDNS probe pod (pods
	// create;delete) and the cert-manager admission dry-run (certificates;issuers
	// create).
	Justification string
}

// RBACDeclarer is implemented by adapters that declare the least-privilege RBAC
// their Run performs. Fathom generates a per-addon ServiceAccount + read-only
// ClusterRole from these rules and impersonates that ServiceAccount when it
// invokes the adapter, making the [Request.Client] "least-privilege scoped
// client" contract real rather than aspirational.
//
// An adapter that does not implement RBACDeclarer gets no generated role; the
// generator surfaces it (with nil rules) so the omission is caught in review
// rather than silently shipping an adapter that can read nothing.
type RBACDeclarer interface {
	// RBACRules returns the grants the adapter's Run requires. The returned
	// slice is treated as read-only by callers.
	RBACRules() []PolicyRule
}

const (
	// AddonServiceAccountPrefix is prepended to an addon's name to form the base
	// name of its generated ServiceAccount, ClusterRole, and bindings — before
	// kustomize's namePrefix is applied. e.g. addon "cilium" yields the base
	// name "addon-cilium", which the deploy renames to "fathom-addon-cilium".
	AddonServiceAccountPrefix = "addon-"

	// AddonLabel keys the addon's name onto its generated ServiceAccount. The
	// reconciler resolves the ServiceAccount's real (possibly namePrefix-renamed)
	// name by listing on this label rather than reconstructing the prefix, so the
	// impersonation username is correct regardless of how the deploy renames
	// objects.
	AddonLabel = "fathom.skaphos.io/addon"
)

// AddonServiceAccountName returns the base ServiceAccount name for an addon,
// before any kustomize namePrefix. It is used by the RBAC generator and tests.
// The reconciler does NOT use it to build the impersonation username — the
// deployed name may be prefixed — it resolves the real name via [AddonLabel].
func AddonServiceAccountName(addon string) string {
	return AddonServiceAccountPrefix + addon
}

// IsReadVerb reports whether v is a read-only verb (get, list, watch). Every
// other verb is a write, which a rule's [PolicyRule.Justification] must defend.
func IsReadVerb(v string) bool {
	switch v {
	case "get", "list", "watch":
		return true
	}
	return false
}

// IsReadOnly reports whether every verb in the rule is a read verb.
func (r PolicyRule) IsReadOnly() bool {
	for _, v := range r.Verbs {
		if !IsReadVerb(v) {
			return false
		}
	}
	return true
}
