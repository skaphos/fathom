/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

const nodeAgentReportAccessSuffix = "-report-access"

// These checks use the real API-server authorizer. The node-check controller
// tests pin the desired RBAC objects; this suite proves the rendered operator
// RBAC can provision them and that their effective permissions stay narrow.
var _ = Describe("Node-agent RBAC posture", Label(utils.CoreLabel), func() {
	It("denies the operator runtime access to ClusterRoles", func() {
		operator := fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccountName)
		checks := [][]string{
			{"get", "clusterroles/fathom-node-agent-role"},
			{"list", "clusterroles"},
			{"watch", "clusterroles"},
			{"create", "clusterroles"},
			{"update", "clusterroles/fathom-node-agent-role"},
		}
		for _, check := range checks {
			Expect(rbacCanI(operator, check...)).To(Equal("no"),
				"operator unexpectedly may %s", strings.Join(check, " "))
		}
	})
})

func assertNodeAgentReportRBAC(g Gomega, namespace, serviceAccount, sourceKind, sourceName string) {
	reportName, err := nodeAgentReportName(namespace, sourceKind, sourceName)
	g.Expect(err).NotTo(HaveOccurred(), "discover the node-agent report ConfigMap")
	g.Expect(reportName).NotTo(BeEmpty(), "node-agent has not published a report ConfigMap")

	identity := fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccount)
	g.Expect(rbacCanI(identity, "create", "configmaps", "-n", namespace)).To(Equal("yes"))
	g.Expect(rbacCanI(identity, "get", "configmaps/"+reportName, "-n", namespace)).To(Equal("yes"))
	g.Expect(rbacCanI(identity, "update", "configmaps/"+reportName, "-n", namespace)).To(Equal("yes"))
	g.Expect(rbacCanI(identity, "list", "configmaps", "-n", namespace)).To(Equal("no"))

	const unrelatedReport = "fathom-e2e-unrelated-node-report"
	g.Expect(rbacCanI(identity, "get", "configmaps/"+unrelatedReport, "-n", namespace)).To(Equal("no"))
	g.Expect(rbacCanI(identity, "update", "configmaps/"+unrelatedReport, "-n", namespace)).To(Equal("no"))

	bindingName := serviceAccount + nodeAgentReportAccessSuffix
	binding, err := nodeAgentRoleBinding(namespace, bindingName)
	g.Expect(err).NotTo(HaveOccurred(), "read scoped node-agent RoleBinding")
	g.Expect(binding.RoleRef.APIGroup).To(Equal("rbac.authorization.k8s.io"))
	g.Expect(binding.RoleRef.Kind).To(Equal("Role"))
	g.Expect(binding.RoleRef.Name).To(Equal(bindingName))
	g.Expect(binding.Subjects).To(ConsistOf(rbacSubject{
		Kind:      "ServiceAccount",
		Name:      serviceAccount,
		Namespace: namespace,
	}))

	// Fresh installations never create the old ClusterRole binding. An upgraded
	// installation may retain the owner-referenced object, but its subjects must
	// be empty so it conveys no authority.
	legacySubjects, err := utils.Run(exec.Command("kubectl", "get", "rolebinding", serviceAccount,
		"-n", namespace, "--ignore-not-found=true", "-o", "jsonpath={.subjects[*].name}"))
	g.Expect(err).NotTo(HaveOccurred(), "read legacy node-agent RoleBinding")
	g.Expect(strings.TrimSpace(legacySubjects)).To(BeEmpty(), "legacy node-agent RoleBinding still grants access")
}

func installLegacyNodeAgentBinding(namespace, serviceAccount, ownerKind, ownerName string) (createdClusterRole, createdBinding bool, err error) {
	const legacyClusterRole = "fathom-node-agent-role"

	out, err := utils.Run(exec.Command("kubectl", "get", "rolebinding", serviceAccount,
		"-n", namespace, "--ignore-not-found=true", "-o", "name"))
	if err != nil {
		return false, false, fmt.Errorf("look up legacy RoleBinding: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return false, false, nil
	}

	out, err = utils.Run(exec.Command("kubectl", "get", "clusterrole", legacyClusterRole,
		"--ignore-not-found=true", "-o", "name"))
	if err != nil {
		return false, false, fmt.Errorf("look up legacy ClusterRole: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		manifest := `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: fathom-node-agent-role
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["create"]
`
		apply := exec.Command("kubectl", "apply", "-f", "-")
		apply.Stdin = strings.NewReader(manifest)
		if _, err := utils.Run(apply); err != nil {
			return false, false, fmt.Errorf("create legacy ClusterRole: %w", err)
		}
		createdClusterRole = true
	}

	uid, err := utils.Run(exec.Command("kubectl", "get", strings.ToLower(ownerKind), ownerName,
		"-n", namespace, "-o", "jsonpath={.metadata.uid}"))
	if err != nil {
		return createdClusterRole, false, fmt.Errorf("read check UID: %w", err)
	}
	manifest := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s
  namespace: %s
  labels:
    fathom.skaphos.io/managed-by: fathom
  ownerReferences:
  - apiVersion: fathom.skaphos.io/v1alpha1
    kind: %s
    name: %s
    uid: %s
    controller: true
    blockOwnerDeletion: true
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: fathom-node-agent-role
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, serviceAccount, namespace, ownerKind, ownerName, strings.TrimSpace(uid), serviceAccount, namespace)
	create := exec.Command("kubectl", "create", "-f", "-")
	create.Stdin = strings.NewReader(manifest)
	if _, err := utils.Run(create); err != nil {
		return createdClusterRole, false, fmt.Errorf("create legacy RoleBinding: %w", err)
	}
	return createdClusterRole, true, nil
}

func legacyNodeAgentBindingSubjects(namespace, serviceAccount string) (string, error) {
	out, err := utils.Run(exec.Command("kubectl", "get", "rolebinding", serviceAccount,
		"-n", namespace, "-o", "jsonpath={.subjects[*].name}"))
	if err != nil {
		return "", fmt.Errorf("read legacy RoleBinding subjects: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func nodeAgentReportName(namespace, sourceKind, sourceName string) (string, error) {
	selector := fmt.Sprintf("fathom.skaphos.io/source-kind=%s,fathom.skaphos.io/source-name=%s", sourceKind, sourceName)
	out, err := utils.Run(exec.Command("kubectl", "get", "configmaps", "-n", namespace,
		"-l", selector, "-o", "jsonpath={.items[0].metadata.name}"))
	if err != nil {
		return "", fmt.Errorf("kubectl get report ConfigMap: %w", err)
	}
	return strings.TrimSpace(out), nil
}

type rbacSubject struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type rbacRoleBinding struct {
	RoleRef struct {
		APIGroup string `json:"apiGroup"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
	} `json:"roleRef"`
	Subjects []rbacSubject `json:"subjects"`
}

func nodeAgentRoleBinding(namespace, name string) (rbacRoleBinding, error) {
	out, err := utils.Run(exec.Command("kubectl", "get", "rolebinding", name, "-n", namespace, "-o", "json"))
	if err != nil {
		return rbacRoleBinding{}, fmt.Errorf("kubectl get RoleBinding: %w", err)
	}
	var binding rbacRoleBinding
	if err := json.Unmarshal([]byte(out), &binding); err != nil {
		return rbacRoleBinding{}, fmt.Errorf("unmarshal RoleBinding: %w", err)
	}
	return binding, nil
}

// rbacCanI returns the final non-empty output line because kubectl may print
// warnings before its yes/no verdict. A denied check exits non-zero, so the
// command error is deliberately ignored.
func rbacCanI(as string, args ...string) string {
	full := append([]string{"auth", "can-i"}, args...)
	full = append(full, "--as="+as)
	out, _ := utils.Run(exec.Command("kubectl", full...))
	var verdict string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			verdict = line
		}
	}
	return verdict
}
