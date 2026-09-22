/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
	"github.com/skaphos/fathom/test/utils"
)

const (
	definitionE2ENS       = "addondefinition-e2e"
	definitionE2EName     = "e2e-runtime-workload"
	definitionE2ERival    = "e2e-runtime-rival"
	definitionE2ESA       = "e2e-runtime-reader"
	definitionE2EManager  = "fathom-controller-manager"
	definitionE2EPeer     = "coredns-sample"
	definitionE2ELease    = "2d3dbc4f.skaphos.io"
	definitionE2EInterval = 10 * time.Second
)

// This suite exercises the shipped CRDs, Deployment, leader Lease, RBAC and
// delegated API reads together. Exact numeric boundaries and injected panics
// have deterministic component tests; this suite uses ordinary Kubernetes
// mutations and never installs a fault hook in the manager.
var _ = Describe("runtime AddonDefinition on a real cluster", Ordered, Serial, Label(utils.CoreLabel), func() {
	var originalArgs []string
	var definitionUID, readerUID, bindingUID string
	var passReports int
	var createdCollision bool
	var reviewedFile string

	BeforeAll(func() {
		By("remembering the exact manager arguments for cleanup")
		var deployment appsv1.Deployment
		definitionE2EGetJSON(&deployment, "deployment", definitionE2EManager, "-n", namespace)
		for _, c := range deployment.Spec.Template.Spec.Containers {
			if c.Name == "manager" {
				originalArgs = append([]string(nil), c.Args...)
			}
		}
		Expect(originalArgs).NotTo(BeEmpty())
		Expect(strings.Join(originalArgs, " ")).NotTo(ContainSubstring("--runtime-loading-enabled=true"),
			"the default-off baseline must start before this suite opts in")

		_, err := utils.Run(exec.Command("kubectl", "create", "namespace", definitionE2ENS))
		Expect(err).NotTo(HaveOccurred())
		root, err := utils.GetProjectDir()
		Expect(err).NotTo(HaveOccurred())
		build := exec.Command("go", "-C", "tools", "tool", "task", "fathomctl-build")
		build.Dir = root
		_, err = utils.Run(build)
		Expect(err).NotTo(HaveOccurred(), "build the target-tree fathomctl once")
		file, err := os.CreateTemp("", "fathom-definition-e2e-*.json")
		Expect(err).NotTo(HaveOccurred())
		reviewedFile = file.Name()
		Expect(file.Close()).To(Succeed())
	})

	AfterAll(func() {
		if reviewedFile != "" {
			_ = os.Remove(reviewedFile)
		}
		// Restore the precise original Pod args even after an assertion fails.
		if len(originalArgs) != 0 {
			definitionE2EPatchArgs(originalArgs)
			_, _ = utils.Run(exec.Command("kubectl", "rollout", "status", "deployment/"+definitionE2EManager,
				"-n", namespace, "--timeout=180s"))
		}
		_, _ = utils.Run(exec.Command("kubectl", "delete", "addondefinition", definitionE2EName,
			"--ignore-not-found=true", "--wait=false"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", definitionE2EName,
			"-n", namespace, "--ignore-not-found=true", "--wait=false"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", definitionE2ERival,
			"-n", namespace, "--ignore-not-found=true", "--wait=false"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "addondefinition", definitionE2ERival,
			"--ignore-not-found=true", "--wait=false"))
		if createdCollision {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "addondefinition", "coredns",
				"--ignore-not-found=true", "--wait=false"))
		}
		_, _ = utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", "coredns",
			"-n", namespace, "--ignore-not-found=true", "--wait=false"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "serviceaccount", "e2e-collision-reader",
			"-n", namespace, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "role,rolebinding", "e2e-collision-impersonate",
			"-n", namespace, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "rolebinding", "e2e-collision-read",
			"-n", "kube-system", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "role", definitionE2EName+"-read",
			"-n", "kube-system", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "rolebinding", definitionE2EName+"-read",
			"-n", "kube-system", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "role", definitionE2EName+"-read",
			"-n", "external-secrets", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "rolebinding", definitionE2EName+"-read",
			"-n", "external-secrets", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "role", definitionE2EName+"-hostile-read",
			"-n", "external-secrets", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "rolebinding", definitionE2EName+"-hostile-read",
			"-n", "external-secrets", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "configmap", definitionE2EName+"-hostile",
			"-n", "external-secrets", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "role", definitionE2EName+"-impersonate",
			"-n", namespace, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "rolebinding", definitionE2EName+"-impersonate",
			"-n", namespace, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "serviceaccount", definitionE2ESA,
			"-n", namespace, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", definitionE2ENS,
			"--ignore-not-found=true", "--wait=false"))
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		for _, args := range [][]string{
			{"get", "addondefinition", definitionE2EName, "-o", "yaml"},
			{"get", "addondefinitionbinding", definitionE2EName, "-n", namespace, "-o", "yaml"},
			{"get", "addoncheck,healthreport", "-n", definitionE2ENS, "-o", "yaml"},
			{"get", "lease", definitionE2ELease, "-n", namespace, "-o", "yaml"},
			{"logs", "deployment/" + definitionE2EManager, "-n", namespace, "--tail=150"},
		} {
			out, _ := utils.Run(exec.Command("kubectl", args...))
			_, _ = fmt.Fprintf(GinkgoWriter, "kubectl %s:\n%s\n", strings.Join(args, " "), out)
		}
	})

	It("keeps definitions inert by default and rejects unknown payload kinds", func() {
		invalid := strings.Replace(definitionE2EDefinition("kube-system", "coredns"),
			"kind: Workload", "kind: Imaginary", 1)
		cmd := exec.Command("kubectl", "apply", "--dry-run=server", "-f", "-")
		cmd.Stdin = strings.NewReader(invalid)
		_, err := utils.Run(cmd)
		Expect(err).To(HaveOccurred(), "the shipped CRD accepted an unknown check kind")

		definitionE2EApply(definitionE2EDefinition("kube-system", "coredns"))
		var def fathomv1alpha1.AddonDefinition
		definitionE2EGetJSON(&def, "addondefinition", definitionE2EName)
		definitionUID = string(def.UID)
		// Bind compares the complete admitted/defaulted live spec, so the
		// reviewed file deliberately contains those defaults verbatim.
		reviewed := fathomv1alpha1.AddonDefinition{
			TypeMeta:   metav1.TypeMeta{APIVersion: fathomv1alpha1.GroupVersion.String(), Kind: "AddonDefinition"},
			ObjectMeta: metav1.ObjectMeta{Name: definitionE2EName},
			Spec:       def.Spec,
		}
		data, err := json.Marshal(reviewed)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(reviewedFile, data, 0o600)).To(Succeed())
		definitionE2EApply(definitionE2ECheck(false))
		definitionE2EApply(fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: %s
  namespace: %s
spec:
  addonType: coredns
  interval: 10s
  timeout: 5s
  policy:
    system_health:
      enabled: true
`, definitionE2EPeer, definitionE2ENS))
		Eventually(func(g Gomega) {
			var peer fathomv1alpha1.AddonCheck
			definitionE2EGetJSON(&peer, "addoncheck", definitionE2EPeer, "-n", definitionE2ENS)
			g.Expect(peer.Status.LastResult).To(Equal("Pass"))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		Consistently(func(g Gomega) {
			var check fathomv1alpha1.AddonCheck
			definitionE2EGetJSON(&check, "addoncheck", definitionE2EName, "-n", definitionE2ENS)
			g.Expect(check.Status.LastSuccessfulEvaluation).To(BeNil())
		}, 12*time.Second, 3*time.Second).Should(Succeed())
	})

	It("uses only a dedicated, UID-bound identity to produce attributed evidence", func() {
		By("rendering reviewed resources twice without writing any cluster resource")
		renderArgs := []string{"definition", "render", "--file", reviewedFile,
			"--service-account", definitionE2ESA, "--operator-namespace", namespace,
			"--operator-service-account", serviceAccountName}
		first, err := fathomctl(renderArgs...)
		Expect(err).NotTo(HaveOccurred(), first)
		second, err := fathomctl(renderArgs...)
		Expect(err).NotTo(HaveOccurred(), second)
		Expect(second).To(Equal(first), "render output changed without changing input")
		Expect(first).To(ContainSubstring("kind: AddonDefinitionBinding"))
		Expect(first).To(ContainSubstring("enabled: false"))
		impersonationName := "fathom-definition-" + definitionE2EName + "-impersonate"
		Expect(first).To(ContainSubstring("name: " + impersonationName))
		for _, args := range [][]string{
			{"serviceaccount", definitionE2ESA, "-n", namespace},
			{"addondefinitionbinding", definitionE2EName, "-n", namespace},
			{"role", impersonationName, "-n", namespace},
			{"rolebinding", impersonationName, "-n", namespace},
		} {
			definitionE2EExpectAbsent(args...)
		}

		By("opting in through the real Deployment and waiting for election and grace")
		definitionE2EPatchArgs(append(append([]string(nil), originalArgs...), "--runtime-loading-enabled=true"))
		definitionE2EWaitRollout()
		Eventually(func() string {
			out, _ := utils.Run(exec.Command("kubectl", "logs", "deployment/"+definitionE2EManager,
				"-n", namespace, "--since=2m"))
			return out
		}, 100*time.Second, 5*time.Second).Should(ContainSubstring("runtime dispatch admitted"))
		// The definition is still unbound: election alone cannot authorize it.
		var unbound fathomv1alpha1.AddonCheck
		definitionE2EGetJSON(&unbound, "addoncheck", definitionE2EName, "-n", definitionE2ENS)
		Expect(unbound.Status.LastSuccessfulEvaluation).To(BeNil())

		By("installing the exact dedicated-SA impersonation grant and target reads")
		definitionE2EApply(fmt.Sprintf("apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: %s\n  namespace: %s\n", definitionE2ESA, namespace))
		var sa struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
		}
		definitionE2EGetJSON(&sa, "serviceaccount", definitionE2ESA, "-n", namespace)
		readerUID = string(sa.Metadata.UID)
		definitionE2EApply(definitionE2EGrants("kube-system", "coredns"))
		definitionE2EApply(definitionE2EGrants("external-secrets", "external-secrets"))
		definitionE2EApply(definitionE2EImpersonationGrant())
		bindOutput, err := fathomctl("definition", "bind", "--file", reviewedFile,
			"--name", definitionE2EName, "--service-account", definitionE2ESA,
			"--operator-namespace", namespace)
		Expect(err).NotTo(HaveOccurred(), bindOutput)
		var staged fathomv1alpha1.AddonDefinitionBinding
		Expect(yaml.UnmarshalStrict([]byte(bindOutput), &staged)).To(Succeed())
		Expect(staged.Spec.Enabled).To(BeFalse())
		Expect(staged.Spec.DefinitionRef.UID).To(Equal(definitionUID))
		Expect(staged.Spec.ServiceAccountRef.UID).To(Equal(readerUID))
		Expect(staged.Spec.TargetScope.Namespaces).To(ContainElement(fathomv1alpha1.DefinitionDNSLabel("kube-system")))
		definitionE2EExpectAbsent("addondefinitionbinding", definitionE2EName, "-n", namespace)
		// The administrator explicitly grants the second target namespace for
		// the same-UID retarget scenario below, then enables the reviewed bind.
		staged.Spec.TargetScope.Namespaces = append(staged.Spec.TargetScope.Namespaces, "external-secrets")
		staged.Spec.Enabled = true
		approved, err := yaml.Marshal(staged)
		Expect(err).NotTo(HaveOccurred())
		definitionE2EApply(string(approved))
		var binding fathomv1alpha1.AddonDefinitionBinding
		definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
		bindingUID = string(binding.UID)

		Eventually(func(g Gomega) {
			var check fathomv1alpha1.AddonCheck
			definitionE2EGetJSON(&check, "addoncheck", definitionE2EName, "-n", definitionE2ENS)
			e := check.Status.LastSuccessfulEvaluation
			g.Expect(e).NotTo(BeNil())
			g.Expect(e.Verdict).To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictPass))
			g.Expect(e.Revision.DefinitionUID).To(Equal(definitionUID))
			g.Expect(e.Authority.ServiceAccountUID).To(Equal(readerUID))
			g.Expect(e.Authority.BindingUID).To(Equal(bindingUID))
			g.Expect(e.Authority.LeaderEpoch).NotTo(BeNil())
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		Eventually(definitionE2EReportCount, time.Minute, 3*time.Second).Should(Equal(1),
			"first completed Pass should create one transition report")
		passReports = 1

		By("rejecting a stored invalid ratio policy before runtime evaluation and recovering on correction")
		initial := definitionE2ECheckStatus()
		Expect(initial.LastRunTime).NotTo(BeNil())
		Expect(initial.LastSuccessfulEvaluation).NotTo(BeNil())
		// Move the periodic run out of the test window, then wait for the
		// generation-triggered valid run to finish. This makes the baseline
		// independent of the prior 10-second cadence and leaves no valid run in
		// flight when the invalid generation is stored.
		out, err := utils.Run(exec.Command("kubectl", "patch", "addoncheck", definitionE2EName,
			"-n", definitionE2ENS, "--type=merge", "-p", `{"spec":{"interval":"5m"}}`))
		Expect(err).NotTo(HaveOccurred(), out)
		var quiesced fathomv1alpha1.AddonCheck
		definitionE2EGetJSON(&quiesced, "addoncheck", definitionE2EName, "-n", definitionE2ENS)
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.ObservedGeneration).To(Equal(quiesced.Generation))
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptCompleted))
			g.Expect(status.LastRunTime).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.Authority.CheckUID).To(Equal(string(quiesced.UID)))
			g.Expect(status.LastSuccessfulEvaluation.Authority.CheckGeneration).To(Equal(quiesced.Generation))
		}, 2*time.Minute, time.Second).Should(Succeed())

		beforeInvalid := definitionE2ECheckStatus()
		beforeInvalidRun := beforeInvalid.LastRunTime.DeepCopy()
		beforeInvalidEvidence := beforeInvalid.LastSuccessfulEvaluation.DeepCopy()
		beforeInvalidReports := definitionE2EReportCount()
		out, err = utils.Run(exec.Command("kubectl", "patch", "addoncheck", definitionE2EName,
			"-n", definitionE2ENS, "--type=merge", "-p",
			`{"spec":{"policy":{"health":{"thresholds":{"failRatio":"150"}}}}}`))
		Expect(err).NotTo(HaveOccurred(), out)
		var invalid fathomv1alpha1.AddonCheck
		definitionE2EGetJSON(&invalid, "addoncheck", definitionE2EName, "-n", definitionE2ENS)
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.ObservedGeneration).To(Equal(invalid.Generation))
			g.Expect(status.Conditions).To(ContainElement(And(
				HaveField("Type", "Accepted"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "InvalidPolicy"))))
			g.Expect(status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "InvalidPolicy"))))
			g.Expect(status.LastRunTime).To(Equal(beforeInvalidRun))
			g.Expect(status.LastSuccessfulEvaluation).To(Equal(beforeInvalidEvidence))
			g.Expect(definitionE2EReportCount()).To(Equal(beforeInvalidReports))
		}, 2*time.Minute, time.Second).Should(Succeed())
		Consistently(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LastRunTime).To(Equal(beforeInvalidRun))
			g.Expect(status.LastSuccessfulEvaluation).To(Equal(beforeInvalidEvidence))
			g.Expect(definitionE2EReportCount()).To(Equal(beforeInvalidReports))
		}, 8*time.Second, time.Second).Should(Succeed(),
			"the invalid stored ratio policy produced runtime evidence or history")

		// failRatio=0 is valid engine policy and preserves worst-of behavior for
		// the later lifecycle scenarios while restoring the normal cadence.
		out, err = utils.Run(exec.Command("kubectl", "patch", "addoncheck", definitionE2EName,
			"-n", definitionE2ENS, "--type=merge", "-p",
			`{"spec":{"interval":"10s","policy":{"health":{"thresholds":{"failRatio":"0"}}}}}`))
		Expect(err).NotTo(HaveOccurred(), out)
		var corrected fathomv1alpha1.AddonCheck
		definitionE2EGetJSON(&corrected, "addoncheck", definitionE2EName, "-n", definitionE2ENS)
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.ObservedGeneration).To(Equal(corrected.Generation))
			g.Expect(status.Conditions).To(ContainElement(And(
				HaveField("Type", "Accepted"), HaveField("Status", metav1.ConditionTrue))))
			g.Expect(status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionTrue))))
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptCompleted))
			g.Expect(status.LastRunTime).NotTo(BeNil())
			g.Expect(status.LastRunTime.Time).To(BeTemporally(">", beforeInvalidRun.Time))
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.Verdict).
				To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictPass))
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).
				To(BeTemporally(">", beforeInvalidEvidence.ObservedAt.Time))
		}, 2*time.Minute, time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(beforeInvalidReports),
			"same-verdict recovery must not add transition history")
	})

	It("fences delegated discovery, timeout, revocation and valid edits without manager fallback", func() {
		reader := "system:serviceaccount:" + namespace + ":" + definitionE2ESA
		manager := "system:serviceaccount:" + namespace + ":" + serviceAccountName
		fixture := definitionE2ENewAPIFixture(manager, reader)
		fixture.install()
		definitionE2EApply(definitionE2EAPIReaderGrant(definitionE2ESA))
		managerDiscovery, err := utils.Run(exec.Command("kubectl", "--as="+manager,
			"--as-group=system:authenticated", "get", "--raw", "/apis/"+definitionE2EAPIVersion))
		Expect(err).NotTo(HaveOccurred(), managerDiscovery)
		Expect(managerDiscovery).To(ContainSubstring(`"kind":"Widget"`))
		Eventually(fixture.managerRequests, 10*time.Second, time.Second).Should(BeNumerically(">", 0),
			"the APIService did not observe the manager identity during the control probe: users=%v", fixture.observedUsers())

		before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		Expect(before).NotTo(BeNil())
		fixture.setDenial(true)
		definitionE2EApply(definitionE2EAPIDefinition())
		Eventually(func(g Gomega) {
			denied, fallback, _ := fixture.counts()
			g.Expect(denied).To(BeNumerically(">", 0),
				"the delegated identity did not reach group discovery")
			g.Expect(fallback).To(BeZero(), "manager discovery was used after delegated denial")
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptError))
			g.Expect(status.LatestAttemptReason).To(Equal("AccessDenied"))
			g.Expect(status.LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, time.Second).Should(Succeed(),
			"delegated discovery denial did not complete: users=%v", fixture.observedUsers())
		Expect(definitionE2EReportCount()).To(Equal(passReports))

		fixture.setDenial(false)
		_, err = utils.Run(exec.Command("kubectl", "annotate", "addoncheck", definitionE2EName,
			"-n", definitionE2ENS, "fathom.skaphos.io/run-now=fixture-ready", "--overwrite"))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptCompleted))
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.Verdict).To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictPass))
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
		}, 2*time.Minute, time.Second).Should(Succeed())

		By("letting real held API reads time out while a built-in peer advances and old Pass evidence ages")
		definitionE2EApply(fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: HealthCheck
metadata:
  name: %s-fixture-wrapper
  namespace: %s
  labels:
    fathom-e2e-runtime: held-input
spec:
  checkRef:
    kind: AddonCheck
    name: %s
---
apiVersion: fathom.skaphos.io/v1alpha1
kind: ClusterHealth
metadata:
  name: %s-fixture-aggregate
spec:
  selector:
    matchLabels:
      fathom-e2e-runtime: held-input
  namespaces: [%s]
`, definitionE2EName, definitionE2ENS, definitionE2EName,
			definitionE2EName, definitionE2ENS))
		DeferCleanup(func() {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "healthcheck", definitionE2EName+"-fixture-wrapper",
				"-n", definitionE2ENS, "--ignore-not-found=true"))
			_, _ = utils.Run(exec.Command("kubectl", "delete", "clusterhealth", definitionE2EName+"-fixture-aggregate",
				"--ignore-not-found=true"))
		})
		beforeTimeout := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		peerBefore := definitionE2EPeerStatus().LastRunTime.DeepCopy()
		fixture.holdNextRead()
		_, err = utils.Run(exec.Command("kubectl", "annotate", "addoncheck", definitionE2EName,
			"-n", definitionE2ENS, "fathom.skaphos.io/run-now=fixture-timeout", "--overwrite"))
		Expect(err).NotTo(HaveOccurred())
		select {
		case held := <-fixture.entered:
			Expect(held.user).To(Equal(reader))
		case <-time.After(40 * time.Second):
			Fail("the delegated LIST never reached the fixture for the timeout case")
		}
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptError))
			g.Expect(status.LatestAttemptReason).To(Equal("Timeout"))
			g.Expect(status.LastSuccessfulEvaluation).To(Equal(beforeTimeout))
		}, 30*time.Second, time.Second).Should(Succeed())
		remaining := time.Until(beforeTimeout.ObservedAt.Add(25*time.Second + time.Second))
		if remaining > 0 {
			time.Sleep(remaining)
		}
		_, err = utils.Run(exec.Command("kubectl", "annotate", "healthcheck", definitionE2EName+"-fixture-wrapper",
			"-n", definitionE2ENS, "fathom.skaphos.io/observe-age=true", "--overwrite"))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			g.Expect(definitionE2ECheckStatus().EvidenceFreshness).
				To(Equal(fathomv1alpha1.AddonCheckEvidenceStale))
			var wrapper fathomv1alpha1.HealthCheck
			definitionE2EGetJSON(&wrapper, "healthcheck", definitionE2EName+"-fixture-wrapper", "-n", definitionE2ENS)
			g.Expect(wrapper.Status.Result).To(Equal(fathomv1alpha1.HealthReportResultPass))
			g.Expect(wrapper.Status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceStale))
			g.Expect(wrapper.Status.SourceObservedAt).To(Equal(&beforeTimeout.ObservedAt))
			var aggregate fathomv1alpha1.ClusterHealth
			definitionE2EGetJSON(&aggregate, "clusterhealth", definitionE2EName+"-fixture-aggregate")
			g.Expect(aggregate.Status.MatchedCount).To(Equal(int32(1)))
			g.Expect(aggregate.Status.ObservedAt).To(Equal(&beforeTimeout.ObservedAt))
			g.Expect(aggregate.Status.Children).To(ContainElement(And(
				HaveField("Name", definitionE2EName+"-fixture-wrapper"),
				HaveField("ObservedAt", Equal(&beforeTimeout.ObservedAt)))))
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(beforeTimeout))
			peer := definitionE2EPeerStatus()
			g.Expect(peer.LastResult).To(Equal("Pass"))
			g.Expect(peer.LastRunTime.Time).To(BeTemporally(">", peerBefore.Time))
		}, 40*time.Second, time.Second).Should(Succeed())
		fixture.releaseRead()
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).
				To(BeTemporally(">", beforeTimeout.ObservedAt.Time))
			g.Expect(status.LastSuccessfulEvaluation.Verdict).To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictPass))
		}, 2*time.Minute, time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports))

		By("revoking the binding after observing a delegated API read in flight")
		beforeRevocation := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		fixture.holdNextRead()
		_, err = utils.Run(exec.Command("kubectl", "annotate", "addoncheck", definitionE2EName,
			"-n", definitionE2ENS, "fathom.skaphos.io/run-now=fixture-held", "--overwrite"))
		Expect(err).NotTo(HaveOccurred())
		var held *definitionE2EHeldRead
		select {
		case held = <-fixture.entered:
		case <-time.After(40 * time.Second):
			Fail("the delegated LIST never reached the fixture before revocation")
		}
		Expect(held.user).To(Equal(reader), "the held read used an unexpected identity")
		// The request itself has a five-second cap. Make the live binding edit
		// immediately after observing the read, then let the server answer.
		select {
		case <-held.done:
			Fail("the delegated read ended before binding revocation began")
		default:
		}
		definitionE2EApply(definitionE2EBinding(definitionUID, readerUID, false))
		var disabled fathomv1alpha1.AddonDefinitionBinding
		definitionE2EGetJSON(&disabled, "addondefinitionbinding", definitionE2EName, "-n", namespace)
		held.Release()
		fixture.releaseRead()
		Eventually(func(g Gomega) {
			var binding fathomv1alpha1.AddonDefinitionBinding
			definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
			g.Expect(binding.Status.ObservedGeneration).To(Equal(disabled.Generation))
			g.Expect(binding.Status.ActiveRuns).To(BeZero())
			g.Expect(binding.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "AuthorizationRevoked"))))
			g.Expect(binding.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Drained"), HaveField("Status", metav1.ConditionTrue))))
			status := definitionE2ECheckStatus()
			g.Expect(status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse))))
			g.Expect(status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceUnavailable))
			g.Expect(status.LastSuccessfulEvaluation).To(Equal(beforeRevocation))
		}, 2*time.Minute, time.Second).Should(Succeed(),
			"the disabled binding did not drain and preserve the previous evidence")
		Expect(definitionE2EReportCount()).To(Equal(passReports))
		definitionE2EApply(definitionE2EBinding(definitionUID, readerUID, true))
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptCompleted))
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).
				To(BeTemporally(">", beforeRevocation.ObservedAt.Time))
			g.Expect(status.LastSuccessfulEvaluation.Authority.BindingGeneration).
				To(BeNumerically(">", beforeRevocation.Authority.BindingGeneration))
		}, 2*time.Minute, time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports))

		By("editing a valid definition while its delegated read is held")
		beforeEdit := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		fixture.holdNextRead()
		_, err = utils.Run(exec.Command("kubectl", "annotate", "addoncheck", definitionE2EName,
			"-n", definitionE2ENS, "fathom.skaphos.io/run-now=fixture-edit", "--overwrite"))
		Expect(err).NotTo(HaveOccurred())
		select {
		case held = <-fixture.entered:
			Expect(held.user).To(Equal(reader))
		case <-time.After(40 * time.Second):
			Fail("the delegated LIST never reached the fixture before the valid edit")
		}
		select {
		case <-held.done:
			Fail("the delegated read ended before the valid definition edit began")
		default:
		}
		definitionE2EApply(strings.Replace(definitionE2EAPIDefinition(),
			"adapterVersion: 1.0.0", "adapterVersion: 1.0.1", 1))
		var edited fathomv1alpha1.AddonDefinition
		definitionE2EGetJSON(&edited, "addondefinition", definitionE2EName)
		held.Release()
		Eventually(func(g Gomega) {
			g.Expect(held.done).To(BeClosed())
			var def fathomv1alpha1.AddonDefinition
			definitionE2EGetJSON(&def, "addondefinition", definitionE2EName)
			g.Expect(def.Status.ObservedGeneration).To(Equal(edited.Generation))
			g.Expect(def.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionTrue))))
		}, 40*time.Second, time.Second).Should(Succeed())
		var next *definitionE2EHeldRead
		for deadline := time.After(40 * time.Second); next == nil; {
			select {
			case candidate := <-fixture.entered:
				if candidate == held {
					continue
				}
				select {
				case <-candidate.done:
					continue
				default:
					next = candidate
				}
			case <-deadline:
				Fail("the edited valid revision never reached an active held API read")
			}
		}
		Expect(next.user).To(Equal(reader))
		Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(beforeEdit),
			"the old revision published while the edited revision was held")
		Expect(definitionE2EReportCount()).To(Equal(passReports))
		fixture.releaseRead()
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptCompleted))
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.Revision.DefinitionGeneration).
				To(Equal(edited.Generation))
			g.Expect(status.LastSuccessfulEvaluation.Revision.AdapterVersion).To(Equal("1.0.1"))
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).
				To(BeTemporally(">", beforeEdit.ObservedAt.Time))
		}, 2*time.Minute, time.Second).Should(Succeed())
		definitionE2EApply(definitionE2EDefinition("kube-system", "coredns"))
		var restored fathomv1alpha1.AddonDefinition
		definitionE2EGetJSON(&restored, "addondefinition", definitionE2EName)
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LastSuccessfulEvaluation.Revision.DefinitionGeneration).
				To(Equal(restored.Generation))
			g.Expect(status.LastSuccessfulEvaluation.Verdict).To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictPass))
		}, 2*time.Minute, time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports))
	})

	It("allows a granted same-UID retarget and rejects an out-of-scope retarget", func() {
		definitionE2EApply(definitionE2EDefinition("external-secrets", "external-secrets"))
		var retargeted fathomv1alpha1.AddonDefinition
		definitionE2EGetJSON(&retargeted, "addondefinition", definitionE2EName)
		Eventually(func(g Gomega) {
			var check fathomv1alpha1.AddonCheck
			definitionE2EGetJSON(&check, "addoncheck", definitionE2EName, "-n", definitionE2ENS)
			g.Expect(check.Status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(check.Status.LastSuccessfulEvaluation.Revision.DefinitionGeneration).To(Equal(retargeted.Generation))
			g.Expect(check.Status.LastSuccessfulEvaluation.Verdict).To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictPass))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports), "same verdict must not add history")

		before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		definitionE2EApply(definitionE2EDefinition("kube-public", "missing-workload"))
		Eventually(func(g Gomega) {
			var def fathomv1alpha1.AddonDefinition
			definitionE2EGetJSON(&def, "addondefinition", definitionE2EName)
			g.Expect(def.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse))))
		}, time.Minute, 3*time.Second).Should(Succeed())
		Consistently(func(g Gomega) {
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		}, 12*time.Second, 3*time.Second).Should(Succeed())
		definitionE2EApply(definitionE2EDefinition("external-secrets", "external-secrets"))
		var recovered fathomv1alpha1.AddonDefinition
		definitionE2EGetJSON(&recovered, "addondefinition", definitionE2EName)
		Eventually(func(g Gomega) {
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation.Revision.DefinitionGeneration).
				To(Equal(recovered.Generation))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("withdraws eligibility for a stored semantic-invalid revision and recovers without rewriting evidence", func() {
		before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		Expect(before).NotTo(BeNil())
		out, err := utils.Run(exec.Command("kubectl", "patch", "addondefinition", definitionE2EName,
			"--type=merge", "-p", `{"spec":{"versionSource":{"fromFamily":"missing","fromComponent":"missing"}}}`))
		Expect(err).NotTo(HaveOccurred(), out)
		Eventually(func(g Gomega) {
			var def fathomv1alpha1.AddonDefinition
			definitionE2EGetJSON(&def, "addondefinition", definitionE2EName)
			g.Expect(def.Spec.VersionSource).NotTo(BeNil(), "the API server pruned the semantic-invalid revision")
			g.Expect(def.Status.ObservedGeneration).To(Equal(def.Generation))
			g.Expect(def.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Accepted"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "InvalidDefinition"))))
			g.Expect(def.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "InvalidDefinition"))))
			status := definitionE2ECheckStatus()
			g.Expect(status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceUnavailable))
			g.Expect(status.LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		out, err = utils.Run(exec.Command("kubectl", "patch", "addondefinition", definitionE2EName,
			"--type=merge", "-p", `{"spec":{"versionSource":null}}`))
		Expect(err).NotTo(HaveOccurred(), out)
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
			g.Expect(status.LastSuccessfulEvaluation.Revision.DefinitionGeneration).
				To(BeNumerically(">", before.Revision.DefinitionGeneration))
			g.Expect(status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceCurrent))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports),
			"invalid attempt and same-verdict recovery must not add transition history")
	})

	It("blocks both definitions while a peer binding borrows the dedicated reader", func() {
		before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		Expect(before).NotTo(BeNil())
		rivalDefinition := strings.ReplaceAll(definitionE2EDefinition("external-secrets", "external-secrets"),
			definitionE2EName, definitionE2ERival)
		definitionE2EApply(rivalDefinition)
		var rival fathomv1alpha1.AddonDefinition
		definitionE2EGetJSON(&rival, "addondefinition", definitionE2ERival)
		definitionE2EApply(fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonDefinitionBinding
metadata:
  name: %s
  namespace: %s
spec:
  definitionRef:
    name: %s
    uid: %s
  serviceAccountRef:
    name: %s
    uid: %s
  enabled: true
  targetScope:
    namespaces: [external-secrets]
`, definitionE2ERival, namespace, definitionE2ERival, rival.UID, definitionE2ESA, readerUID))
		Eventually(func(g Gomega) {
			for _, name := range []string{definitionE2EName, definitionE2ERival} {
				var def fathomv1alpha1.AddonDefinition
				definitionE2EGetJSON(&def, "addondefinition", name)
				g.Expect(def.Status.Conditions).To(ContainElement(And(
					HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
					HaveField("Reason", "BindingMismatch"))), name)
				var binding fathomv1alpha1.AddonDefinitionBinding
				definitionE2EGetJSON(&binding, "addondefinitionbinding", name, "-n", namespace)
				g.Expect(binding.Status.Conditions).To(ContainElement(And(
					HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
					HaveField("Reason", "BindingMismatch"))), name)
			}
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		_, err := utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", definitionE2ERival,
			"-n", namespace))
		Expect(err).NotTo(HaveOccurred())
		_, err = utils.Run(exec.Command("kubectl", "delete", "addondefinition", definitionE2ERival))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			var def fathomv1alpha1.AddonDefinition
			definitionE2EGetJSON(&def, "addondefinition", definitionE2EName)
			g.Expect(def.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionTrue))))
			status := definitionE2ECheckStatus()
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
			g.Expect(status.LastSuccessfulEvaluation.Authority.ServiceAccountUID).To(Equal(readerUID))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports))
	})

	It("does not inherit authority when the dedicated ServiceAccount is recreated", func() {
		before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		_, err := utils.Run(exec.Command("kubectl", "delete", "serviceaccount", definitionE2ESA, "-n", namespace))
		Expect(err).NotTo(HaveOccurred())
		definitionE2EApply(fmt.Sprintf("apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: %s\n  namespace: %s\n", definitionE2ESA, namespace))
		var sa struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
		}
		definitionE2EGetJSON(&sa, "serviceaccount", definitionE2ESA, "-n", namespace)
		newUID := string(sa.Metadata.UID)
		Expect(newUID).NotTo(Equal(readerUID))
		Eventually(func(g Gomega) {
			var binding fathomv1alpha1.AddonDefinitionBinding
			definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
			g.Expect(binding.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse))))
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		// Binding references are immutable: reauthorization requires a new binding
		// object with the new SA UID, rather than a status write to the old one.
		_, err = utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", definitionE2EName,
			"-n", namespace))
		Expect(err).NotTo(HaveOccurred())
		var managerSA struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
		}
		definitionE2EGetJSON(&managerSA, "serviceaccount", serviceAccountName, "-n", namespace)
		definitionE2EApply(definitionE2EBindingFor(definitionUID, serviceAccountName,
			string(managerSA.Metadata.UID), true))
		Eventually(func(g Gomega) {
			var borrowed fathomv1alpha1.AddonDefinitionBinding
			definitionE2EGetJSON(&borrowed, "addondefinitionbinding", definitionE2EName, "-n", namespace)
			g.Expect(borrowed.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "BindingMismatch"))))
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"the operator accepted its own ServiceAccount as a runtime identity")
		_, err = utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", definitionE2EName,
			"-n", namespace))
		Expect(err).NotTo(HaveOccurred())
		var builtinSA struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
		}
		definitionE2EGetJSON(&builtinSA, "serviceaccount", "fathom-addon-coredns", "-n", namespace)
		definitionE2EApply(definitionE2EBindingFor(definitionUID, builtinSA.Metadata.Name,
			string(builtinSA.Metadata.UID), true))
		Eventually(func(g Gomega) {
			var borrowed fathomv1alpha1.AddonDefinitionBinding
			definitionE2EGetJSON(&borrowed, "addondefinitionbinding", definitionE2EName, "-n", namespace)
			g.Expect(borrowed.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "BindingMismatch"))))
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"the operator accepted a built-in ServiceAccount as a runtime identity")
		_, err = utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", definitionE2EName,
			"-n", namespace))
		Expect(err).NotTo(HaveOccurred())
		definitionE2EApply(definitionE2EBinding(definitionUID, newUID, true))
		readerUID = newUID
		var binding fathomv1alpha1.AddonDefinitionBinding
		definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
		bindingUID = string(binding.UID)
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.Authority.ServiceAccountUID).To(Equal(newUID))
			g.Expect(status.LastSuccessfulEvaluation.Authority.BindingUID).To(Equal(bindingUID))
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports), "same-verdict reauthorization must not add a report")
	})

	It("retains old evidence across definition deletion and requires a new UID binding", func() {
		before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		_, err := utils.Run(exec.Command("kubectl", "delete", "addondefinition", definitionE2EName))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			var binding fathomv1alpha1.AddonDefinitionBinding
			definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
			g.Expect(binding.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse))))
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		definitionE2EApply(definitionE2EDefinition("external-secrets", "external-secrets"))
		var replacement fathomv1alpha1.AddonDefinition
		definitionE2EGetJSON(&replacement, "addondefinition", definitionE2EName)
		newUID := string(replacement.UID)
		Expect(newUID).NotTo(Equal(definitionUID))
		Eventually(func(g Gomega) {
			var binding fathomv1alpha1.AddonDefinitionBinding
			definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
			g.Expect(binding.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "BindingMismatch"))))
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"a recreated definition inherited authority from the previous UID")

		_, err = utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", definitionE2EName,
			"-n", namespace))
		Expect(err).NotTo(HaveOccurred())
		definitionE2EApply(definitionE2EBinding(newUID, readerUID, true))
		definitionUID = newUID
		var binding fathomv1alpha1.AddonDefinitionBinding
		definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
		bindingUID = string(binding.UID)
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.Revision.DefinitionUID).To(Equal(newUID))
			g.Expect(status.LastSuccessfulEvaluation.Authority.BindingUID).To(Equal(bindingUID))
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports),
			"same-verdict definition recreation must preserve transition-only history")
	})

	It("retains evidence on denied reads, then records Pass to Skipped only once", func() {
		before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		peerBefore := definitionE2EPeerStatus().LastRunTime.DeepCopy()
		_, err := utils.Run(exec.Command("kubectl", "delete", "rolebinding", definitionE2EName+"-read",
			"-n", "external-secrets"))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptError))
			g.Expect(status.LatestAttemptReason).To(Equal("AccessDenied"))
			g.Expect(status.EvidenceFreshness).To(Equal(fathomv1alpha1.AddonCheckEvidenceUnavailable))
			g.Expect(status.LastSuccessfulEvaluation).To(Equal(before))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		Eventually(func(g Gomega) {
			peer := definitionE2EPeerStatus()
			g.Expect(peer.LastResult).To(Equal("Pass"))
			g.Expect(peer.LastRunTime).NotTo(BeNil())
			g.Expect(peer.LastRunTime.Time).To(BeTemporally(">", peerBefore.Time))
		}, 90*time.Second, 5*time.Second).Should(Succeed(),
			"compiled CoreDNS stopped making progress while runtime reads were denied")
		definitionE2EApply(definitionE2EGrants("external-secrets", "external-secrets"))
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptCompleted))
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		definitionE2EHostileInput()

		definitionE2EApply(definitionE2ECheck(true))
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			e := status.LastSuccessfulEvaluation
			g.Expect(e).NotTo(BeNil())
			g.Expect(e.Verdict).To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictSkipped))
			g.Expect(e.Coverage).To(Equal(fathomv1alpha1.AddonCheckCoverageNoChecksEvaluated))
			g.Expect(e.Message).To(Equal("no checks evaluated"))
			g.Expect(e.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		skippedAt := definitionE2ECheckStatus().LastSuccessfulEvaluation.ObservedAt
		Eventually(definitionE2EReportCount, time.Minute, 3*time.Second).Should(Equal(passReports + 1))
		skippedReports := passReports + 1
		_, err = utils.Run(exec.Command("kubectl", "annotate", "addoncheck", definitionE2EName,
			"-n", definitionE2ENS, "fathom.skaphos.io/run-now=repeat-skipped", "--overwrite"))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			g.Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation.ObservedAt.Time).
				To(BeTemporally(">", skippedAt.Time))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(skippedReports))
	})

	It("verifies current-leader drain and preserves history through rollback", func() {
		By("using the previously built versioned CLI for independent Lease and binding reads")
		var err error
		out, err := fathomctl("definition", "collisions")
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("version="))
		Expect(out).To(ContainSubstring("build="))
		Expect(out).To(ContainSubstring("No built-in collisions"))
		By("finding a live definition that collides with this binary's bundled inventory")
		collision := strings.ReplaceAll(definitionE2EDefinition("kube-system", "coredns"),
			definitionE2EName, "coredns")
		definitionE2EApply(collision)
		createdCollision = true
		out, err = fathomctl("definition", "collisions")
		Expect(fathomctlExitCode(err)).To(Equal(1), out)
		Expect(out).To(ContainSubstring("collision: coredns"))
		var collisionDef fathomv1alpha1.AddonDefinition
		definitionE2EGetJSON(&collisionDef, "addondefinition", "coredns")
		definitionE2EApply(fmt.Sprintf("apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: e2e-collision-reader\n  namespace: %s\n", namespace))
		var collisionSA struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
		}
		definitionE2EGetJSON(&collisionSA, "serviceaccount", "e2e-collision-reader", "-n", namespace)
		definitionE2EApply(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: e2e-collision-impersonate
  namespace: %s
rules:
- apiGroups: [""]
  resources: [serviceaccounts]
  resourceNames: [e2e-collision-reader]
  verbs: [impersonate]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: e2e-collision-impersonate
  namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: e2e-collision-impersonate
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: e2e-collision-read
  namespace: kube-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s-read
subjects:
- kind: ServiceAccount
  name: e2e-collision-reader
  namespace: %s
`, namespace, namespace, serviceAccountName, namespace, definitionE2EName, namespace))
		definitionE2EApply(fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonDefinitionBinding
metadata:
  name: coredns
  namespace: %s
spec:
  definitionRef:
    name: coredns
    uid: %s
  serviceAccountRef:
    name: e2e-collision-reader
    uid: %s
  enabled: true
  targetScope:
    namespaces: [kube-system]
`, namespace, collisionDef.UID, collisionSA.Metadata.UID))
		Eventually(func(g Gomega) {
			var peer fathomv1alpha1.AddonCheck
			definitionE2EGetJSON(&peer, "addoncheck", definitionE2EPeer, "-n", definitionE2ENS)
			g.Expect(peer.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "BuiltinCollision"))))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"the runtime definition and compiled CoreDNS ran through a collision")
		blockedAt := definitionE2EPeerStatus().LastRunTime.DeepCopy()
		Consistently(func(g Gomega) {
			g.Expect(definitionE2EPeerStatus().LastRunTime).To(Equal(blockedAt))
		}, 12*time.Second, 3*time.Second).Should(Succeed(),
			"compiled CoreDNS executed through the collision barrier")
		By("starting the same target binary against the already-stored collision")
		out, err = utils.Run(exec.Command("kubectl", "rollout", "restart", "deployment/"+definitionE2EManager,
			"-n", namespace))
		Expect(err).NotTo(HaveOccurred(), out)
		definitionE2EWaitRollout()
		Eventually(func() string {
			logs, _ := utils.Run(exec.Command("kubectl", "logs", "deployment/"+definitionE2EManager,
				"-n", namespace, "--since=2m"))
			return logs
		}, 100*time.Second, 3*time.Second).Should(ContainSubstring("runtime dispatch admitted"))
		Eventually(func(g Gomega) {
			peer := definitionE2EPeerStatus()
			g.Expect(peer.Conditions).To(ContainElement(And(
				HaveField("Type", "Ready"), HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "BuiltinCollision"))))
			g.Expect(peer.LastRunTime).To(Equal(blockedAt))
		}, 2*time.Minute, 3*time.Second).Should(Succeed(),
			"startup inventory admitted a stored collision before explicit resolution")
		_, err = utils.Run(exec.Command("kubectl", "delete", "addondefinitionbinding", "coredns", "-n", namespace))
		Expect(err).NotTo(HaveOccurred())
		_, err = utils.Run(exec.Command("kubectl", "delete", "addondefinition", "coredns"))
		Expect(err).NotTo(HaveOccurred())
		createdCollision = false
		Eventually(func(g Gomega) {
			peer := definitionE2EPeerStatus()
			g.Expect(peer.LastResult).To(Equal("Pass"))
			g.Expect(peer.LastRunTime).NotTo(BeNil())
			g.Expect(peer.LastRunTime.Time).To(BeTemporally(">", blockedAt.Time))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"compiled CoreDNS did not recover after the collision was resolved")

		before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		definitionE2EApply(definitionE2EBinding(definitionUID, readerUID, false))
		Eventually(func(g Gomega) {
			var binding fathomv1alpha1.AddonDefinitionBinding
			definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
			g.Expect(binding.Status.ActiveRuns).To(BeZero())
			g.Expect(binding.Status.LeaderEpoch).NotTo(BeNil())
			g.Expect(binding.Status.ObservedGeneration).To(Equal(binding.Generation))
			g.Expect(binding.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Drained"), HaveField("Status", metav1.ConditionTrue))))
		}, 2*time.Minute, 3*time.Second).Should(Succeed())
		out, err = fathomctl("definition", "drain", "--name", definitionE2EName,
			"--operator-namespace", namespace, "--leader-election-id", definitionE2ELease)
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("Verified drained"))
		var drainedBinding fathomv1alpha1.AddonDefinitionBinding
		definitionE2EGetJSON(&drainedBinding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
		oldEpoch := drainedBinding.Status.LeaderEpoch.DeepCopy()
		Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		Expect(definitionE2EReportCount()).To(Equal(passReports + 1))

		By("revoking the drained reader's impersonation and target bindings before disabling runtime loading")
		for _, target := range [][]string{
			{"role,rolebinding", definitionE2EName + "-impersonate", "-n", namespace},
			{"rolebinding", definitionE2EName + "-read", "-n", "kube-system"},
			{"rolebinding", definitionE2EName + "-read", "-n", "external-secrets"},
		} {
			args := append([]string{"delete"}, target...)
			out, err = utils.Run(exec.Command("kubectl", args...))
			Expect(err).NotTo(HaveOccurred(), out)
		}
		definitionE2EExpectAbsent("role", definitionE2EName+"-impersonate", "-n", namespace)
		definitionE2EExpectAbsent("rolebinding", definitionE2EName+"-impersonate", "-n", namespace)
		definitionE2EExpectAbsent("rolebinding", definitionE2EName+"-read", "-n", "kube-system")
		definitionE2EExpectAbsent("rolebinding", definitionE2EName+"-read", "-n", "external-secrets")
		definitionE2EPatchArgs(originalArgs)
		definitionE2EWaitRollout()
		Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		Expect(definitionE2EReportCount()).To(Equal(passReports + 1))
		By("rejecting the prior leader's drain acknowledgement after the manager restarts")
		Eventually(func(g Gomega) {
			var lease coordinationv1.Lease
			definitionE2EGetJSON(&lease, "lease", definitionE2ELease, "-n", namespace)
			g.Expect(lease.Spec.HolderIdentity).NotTo(BeNil())
			g.Expect(*lease.Spec.HolderIdentity).NotTo(Equal(oldEpoch.HolderIdentity))
		}, time.Minute, 2*time.Second).Should(Succeed())
		out, err = fathomctl("definition", "drain", "--name", definitionE2EName,
			"--operator-namespace", namespace, "--leader-election-id", definitionE2ELease)
		Expect(fathomctlExitCode(err)).To(Equal(2), out)
		Expect(out).To(ContainSubstring("no acknowledgement for the observed leader epoch"))

		By("restoring reviewed grants before testing an enabled binding without election")
		definitionE2EApply(definitionE2EGrants("kube-system", "coredns"))
		definitionE2EApply(definitionE2EGrants("external-secrets", "external-secrets"))
		definitionE2EApply(definitionE2EImpersonationGrant())
		definitionE2EApply(definitionE2EBinding(definitionUID, readerUID, true))
		By("refusing runtime activation without leader election while compiled checks continue")
		noElection := definitionE2EArgsWithoutElection(originalArgs)
		definitionE2EPatchArgs(append(noElection, "--leader-elect=false", "--runtime-loading-enabled=true"))
		definitionE2EWaitRollout()
		Eventually(func() string {
			out, _ := utils.Run(exec.Command("kubectl", "logs", "deployment/"+definitionE2EManager,
				"-n", namespace, "--since=2m"))
			return out
		}, time.Minute, 5*time.Second).Should(ContainSubstring("LeaderElectionRequired"))
		Consistently(func() *fathomv1alpha1.AddonCheckEvidence {
			return definitionE2ECheckStatus().LastSuccessfulEvaluation
		}, 12*time.Second, 3*time.Second).Should(Equal(before),
			"an enabled binding ran without leader election")
		peerBefore := definitionE2EPeerStatus().LastRunTime.DeepCopy()
		Eventually(func(g Gomega) {
			peer := definitionE2EPeerStatus()
			g.Expect(peer.LastResult).To(Equal("Pass"))
			g.Expect(peer.LastRunTime.Time).To(BeTemporally(">", peerBefore.Time))
		}, 90*time.Second, 5*time.Second).Should(Succeed())

		definitionE2EApply(definitionE2EBinding(definitionUID, readerUID, false))
		definitionE2EPatchArgs(originalArgs)
		definitionE2EWaitRollout()
		Expect(definitionE2ECheckStatus().LastSuccessfulEvaluation).To(Equal(before))
		By("re-enabling runtime with metrics disabled")
		definitionE2EPatchArgs(append(append([]string(nil), originalArgs...),
			"--runtime-loading-enabled=true", "--metrics-bind-address=0"))
		definitionE2EWaitRollout()
		Eventually(func(g Gomega) {
			var binding fathomv1alpha1.AddonDefinitionBinding
			definitionE2EGetJSON(&binding, "addondefinitionbinding", definitionE2EName, "-n", namespace)
			g.Expect(binding.Status.LeaderEpoch).NotTo(BeNil())
			g.Expect(binding.Status.LeaderEpoch).NotTo(Equal(oldEpoch))
			g.Expect(binding.Status.ObservedGeneration).To(Equal(binding.Generation))
			g.Expect(binding.Status.Conditions).To(ContainElement(And(
				HaveField("Type", "Drained"), HaveField("Status", metav1.ConditionTrue))))
		}, 2*time.Minute, 3*time.Second).Should(Succeed(),
			"the new runtime leader did not acknowledge the disabled binding after grace")
		out, err = fathomctl("definition", "drain", "--name", definitionE2EName,
			"--operator-namespace", namespace, "--leader-election-id", definitionE2ELease)
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("Verified drained"))
		definitionE2EApply(definitionE2ECheck(false))
		definitionE2EApply(definitionE2EBinding(definitionUID, readerUID, true))
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LastSuccessfulEvaluation).NotTo(BeNil())
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
			g.Expect(status.LastSuccessfulEvaluation.Verdict).To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictPass))
			g.Expect(status.LastSuccessfulEvaluation.Authority.ServiceAccountUID).To(Equal(readerUID))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		Eventually(definitionE2EReportCount, time.Minute, 3*time.Second).Should(Equal(passReports+2),
			"Skipped to Pass must create one transition report")
		metricsPass := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
		By("reporting a real delegated read denial while metrics are disabled")
		_, err = utils.Run(exec.Command("kubectl", "delete", "rolebinding", definitionE2EName+"-read",
			"-n", "external-secrets"))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptError))
			g.Expect(status.LatestAttemptReason).To(Equal("AccessDenied"))
			g.Expect(status.LastSuccessfulEvaluation).To(Equal(metricsPass))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		definitionE2EApply(definitionE2EGrants("external-secrets", "external-secrets"))
		Eventually(func(g Gomega) {
			status := definitionE2ECheckStatus()
			g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptCompleted))
			g.Expect(status.LastSuccessfulEvaluation.Verdict).To(Equal(fathomv1alpha1.AddonCheckEvidenceVerdictPass))
			g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).
				To(BeTemporally(">", metricsPass.ObservedAt.Time))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		Expect(definitionE2EReportCount()).To(Equal(passReports + 2))
	})
})

func definitionE2EGetJSON(dst any, args ...string) {
	out, err := utils.Run(exec.Command("kubectl", append([]string{"get"}, append(args, "-o", "json")...)...))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), out)
	ExpectWithOffset(1, json.Unmarshal([]byte(out), dst)).To(Succeed())
}

func definitionE2EExpectAbsent(args ...string) {
	query := append([]string{"get"}, args...)
	query = append(query, "--ignore-not-found=true", "-o", "name")
	out, err := utils.Run(exec.Command("kubectl", query...))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "could not verify absence of %v: %s", args, out)
	ExpectWithOffset(1, strings.TrimSpace(out)).To(BeEmpty(), "CLI wrote resource %v", args)
}

func definitionE2EApply(manifest string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), out)
}

func definitionE2EPatchArgs(args []string) {
	patch, err := json.Marshal(map[string]any{"spec": map[string]any{"template": map[string]any{
		"spec": map[string]any{"containers": []any{map[string]any{"name": "manager", "args": args}}},
	}}})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	out, err := utils.Run(exec.Command("kubectl", "patch", "deployment", definitionE2EManager,
		"-n", namespace, "--type=strategic", "-p", string(patch)))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), out)
}

func definitionE2EArgsWithoutElection(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--leader-elect" || strings.HasPrefix(arg, "--leader-elect=") {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

func definitionE2EWaitRollout() {
	out, err := utils.Run(exec.Command("kubectl", "rollout", "status", "deployment/"+definitionE2EManager,
		"-n", namespace, "--timeout=180s"))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), out)
}

func definitionE2ECheckStatus() fathomv1alpha1.AddonCheckStatus {
	var check fathomv1alpha1.AddonCheck
	definitionE2EGetJSON(&check, "addoncheck", definitionE2EName, "-n", definitionE2ENS)
	return check.Status
}

func definitionE2EPeerStatus() fathomv1alpha1.AddonCheckStatus {
	var check fathomv1alpha1.AddonCheck
	definitionE2EGetJSON(&check, "addoncheck", definitionE2EPeer, "-n", definitionE2ENS)
	return check.Status
}

func definitionE2EReportCount() int {
	var list fathomv1alpha1.HealthReportList
	definitionE2EGetJSON(&list, "healthreport", "-n", definitionE2ENS)
	n := 0
	for _, report := range list.Items {
		if report.Spec.SourceRef.Name == definitionE2EName && report.Spec.SourceRef.Kind == "AddonCheck" {
			n++
		}
	}
	return n
}

// The payload is legal Kubernetes data but beyond the runtime YAML parser's
// 64 KiB bound. It verifies that the shipped operator preserves old evidence
// and leaves a compiled peer running when hostile stored data is encountered.
func definitionE2EHostileInput() {
	By("exercising a real oversized ConfigMap value without a manager fault hook")
	before := definitionE2ECheckStatus().LastSuccessfulEvaluation.DeepCopy()
	peerBefore := definitionE2EPeerStatus().LastRunTime.DeepCopy()
	cmName := definitionE2EName + "-hostile"
	cm, err := json.Marshal(map[string]any{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]string{"name": cmName, "namespace": "external-secrets"},
		"data":     map[string]string{"policy.yaml": "value: " + strings.Repeat("x", 65536)},
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	definitionE2EApply(string(cm))
	definitionE2EApply(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s-hostile-read
  namespace: external-secrets
rules:
- apiGroups: [""]
  resources: [configmaps]
  resourceNames: [%s]
  verbs: [get]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s-hostile-read
  namespace: external-secrets
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s-hostile-read
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, definitionE2EName, cmName, definitionE2EName, definitionE2EName,
		definitionE2ESA, namespace))
	workload := definitionE2EDefinition("external-secrets", "external-secrets")
	hostile := strings.Replace(workload, "        defaultName: external-secrets\n",
		fmt.Sprintf(`        defaultName: external-secrets
    - name: hostile-config
      kind: ConfigMap
      configMap:
        target:
          scope: Namespaced
          namespaces: [external-secrets]
        defaultName: %s
        key: policy.yaml
`, cmName), 1)
	ExpectWithOffset(1, hostile).NotTo(Equal(workload))
	definitionE2EApply(hostile)
	Eventually(func(g Gomega) {
		status := definitionE2ECheckStatus()
		g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptError))
		g.Expect(status.LatestAttemptReason).To(Equal("InputLimitExceeded"))
		g.Expect(status.LastSuccessfulEvaluation).To(Equal(before))
	}, 2*time.Minute, 5*time.Second).Should(Succeed())
	Eventually(func(g Gomega) {
		peer := definitionE2EPeerStatus()
		g.Expect(peer.LastResult).To(Equal("Pass"))
		g.Expect(peer.LastRunTime).NotTo(BeNil())
		g.Expect(peer.LastRunTime.Time).To(BeTemporally(">", peerBefore.Time))
	}, 90*time.Second, 5*time.Second).Should(Succeed())
	definitionE2EApply(workload)
	Eventually(func(g Gomega) {
		status := definitionE2ECheckStatus()
		g.Expect(status.LatestAttemptOutcome).To(Equal(fathomv1alpha1.AddonCheckAttemptCompleted))
		g.Expect(status.LastSuccessfulEvaluation.ObservedAt.Time).To(BeTemporally(">", before.ObservedAt.Time))
	}, 2*time.Minute, 5*time.Second).Should(Succeed())
}

func definitionE2EDefinition(targetNS, targetName string) string {
	return fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonDefinition
metadata:
  name: %s
spec:
  addonType: %s
  adapterVersion: 1.0.0
  semanticsVersion: 1
  families:
  - name: health
    defaultEnabled: true
    checks:
    - name: workload
      kind: Workload
      workload:
        target:
          scope: Namespaced
          namespaces: [%s]
        kind: Deployment
        defaultName: %s
`, definitionE2EName, definitionE2EName, targetNS, targetName)
}

func definitionE2ECheck(skip bool) string {
	enabled := "true"
	if skip {
		enabled = "false"
	}
	return fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonCheck
metadata:
  name: %s
  namespace: %s
spec:
  addonType: %s
  interval: %s
  timeout: 5s
  policy:
    health:
      enabled: %s
`, definitionE2EName, definitionE2ENS, definitionE2EName, definitionE2EInterval, enabled)
}

func definitionE2EBinding(defUID, saUID string, enabled bool) string {
	return definitionE2EBindingFor(defUID, definitionE2ESA, saUID, enabled)
}

func definitionE2EBindingFor(defUID, saName, saUID string, enabled bool) string {
	return fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: AddonDefinitionBinding
metadata:
  name: %s
  namespace: %s
spec:
  definitionRef:
    name: %s
    uid: %s
  serviceAccountRef:
    name: %s
    uid: %s
  enabled: %t
  targetScope:
    namespaces: [kube-system, external-secrets]
`, definitionE2EName, namespace, definitionE2EName, defUID, saName, saUID, enabled)
}

func definitionE2EGrants(targetNS, deployment string) string {
	return fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s-read
  namespace: %s
rules:
- apiGroups: [apps]
  resources: [deployments]
  resourceNames: [%s]
  verbs: [get]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s-read
  namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s-read
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, definitionE2EName, targetNS, deployment, definitionE2EName, targetNS,
		definitionE2EName, definitionE2ESA, namespace)
}

func definitionE2EImpersonationGrant() string {
	return fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s-impersonate
  namespace: %s
rules:
- apiGroups: [""]
  resources: [serviceaccounts]
  resourceNames: [%s]
  verbs: [impersonate]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s-impersonate
  namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s-impersonate
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, definitionE2EName, namespace, definitionE2ESA, definitionE2EName,
		namespace, definitionE2EName, serviceAccountName, namespace)
}
