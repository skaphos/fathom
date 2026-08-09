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
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

const (
	dnsCheckNamespace = "dnscheck-e2e"

	// dnsCheckBlackholeResolver is an address in 10.255.255.0/24 — routable
	// nowhere in a kind cluster, so a query to it hangs until the probe's own
	// timeout. That is what keeps a probe Pod alive long enough to observe:
	// a pod answering through cluster DNS is gone in under a second, far too
	// short to catch a live ownerReference or a deletion cascade.
	dnsCheckBlackholeResolver = "10.255.255.1"
)

// dnsCheckPod is the slice of a probe Pod these specs assert on.
type dnsCheckPod struct {
	Metadata struct {
		Name            string            `json:"name"`
		Namespace       string            `json:"namespace"`
		Labels          map[string]string `json:"labels"`
		OwnerReferences []struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Name       string `json:"name"`
			UID        string `json:"uid"`
			Controller *bool  `json:"controller"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

type dnsCheckPodList struct {
	Items []dnsCheckPod `json:"items"`
}

// listProbePods returns the probe Pods currently present in a namespace,
// selected the same way the orphan sweeper selects them.
func listProbePods(namespace string) ([]dnsCheckPod, error) {
	cmd := exec.Command("kubectl", "get", "pods",
		"-n", namespace,
		"-l", "fathom.skaphos.io/managed-by=fathom",
		"-o", "json",
	)
	out, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	var list dnsCheckPodList
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// writeDNSCheckManifest renders a DNSCheck that resolves through a blackholed
// upstream, so its probe Pod stays alive for the declared timeout.
func writeDNSCheckManifest(name string, timeout, interval string) string {
	manifest := fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: DNSCheck
metadata:
  name: %s
  namespace: %s
spec:
  interval: %s
  timeout: %s
  resolvers:
    - name: blackhole
      from: Explicit
      address: %s
  targets:
    - name: slow.example.com.
      recordType: A
`, name, dnsCheckNamespace, interval, timeout, dnsCheckBlackholeResolver)

	path := filepath.Join(os.TempDir(), "fathom-e2e-"+name+".yaml")
	Expect(os.WriteFile(path, []byte(manifest), 0o600)).To(Succeed())
	return path
}

var _ = Describe("DNSCheck lifecycle", Ordered, Label(utils.CoreLabel, "dnscheck"), func() {
	BeforeAll(func() {
		By("creating the DNSCheck e2e namespace")
		cmd := exec.Command("kubectl", "create", "namespace", dnsCheckNamespace)
		_, _ = utils.Run(cmd) // already-exists is fine on a reused cluster
	})

	AfterAll(func() {
		By("removing the DNSCheck e2e namespace")
		cmd := exec.Command("kubectl", "delete", "namespace", dnsCheckNamespace, "--ignore-not-found=true", "--wait=false")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		By("dumping DNSCheck diagnostics")
		for _, args := range [][]string{
			{"get", "dnschecks", "-n", dnsCheckNamespace, "-o", "yaml"},
			{"get", "pods", "-n", dnsCheckNamespace, "-o", "wide"},
			{"get", "events", "-n", dnsCheckNamespace, "--sort-by=.lastTimestamp"},
		} {
			out, _ := utils.Run(exec.Command("kubectl", args...))
			_, _ = fmt.Fprintf(GinkgoWriter, "kubectl %s:\n%s\n", strings.Join(args, " "), out)
		}
	})

	// T042 / US4 — FR-113 and FR-114. DNSCheck is the first kind whose probe
	// Pods can carry an ownerReference at all: a namespaced owner must share its
	// dependent's namespace, and FR-031 is what puts them together. Adapter
	// probes land in the addon's namespace and cannot do this.
	It("owns its probe Pods and garbage-collects them when the check is deleted", func() {
		manifest := writeDNSCheckManifest("owned-probe", "40s", "2m")
		DeferCleanup(func() {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", manifest, "--ignore-not-found=true"))
			_ = os.Remove(manifest)
		})

		By("applying a DNSCheck whose resolver is blackholed, so its probe Pod lingers")
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", manifest))
		Expect(err).NotTo(HaveOccurred())

		By("waiting for a live probe Pod in the check's own namespace")
		var observed dnsCheckPod
		Eventually(func(g Gomega) {
			pods, listErr := listProbePods(dnsCheckNamespace)
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(pods).NotTo(BeEmpty(), "no probe Pod appeared in %s", dnsCheckNamespace)
			observed = pods[0]
		}, 3*time.Minute, 2*time.Second).Should(Succeed())

		By("asserting the Pod is owned by the DNSCheck that caused it")
		Expect(observed.Metadata.Namespace).To(Equal(dnsCheckNamespace),
			"FR-031: resolution must run in the check's own namespace, never the operator's")
		Expect(observed.Metadata.OwnerReferences).To(HaveLen(1),
			"probe Pod carries no ownerReference; deletion would not cascade")
		owner := observed.Metadata.OwnerReferences[0]
		Expect(owner.Kind).To(Equal("DNSCheck"))
		Expect(owner.Name).To(Equal("owned-probe"))
		Expect(owner.Controller).NotTo(BeNil())
		Expect(*owner.Controller).To(BeTrue())

		By("deleting the DNSCheck and expecting its Pods to be garbage-collected")
		_, err = utils.Run(exec.Command("kubectl", "delete", "-f", manifest, "--wait=true"))
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			pods, listErr := listProbePods(dnsCheckNamespace)
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(pods).To(BeEmpty(), "probe Pods outlived the deleted DNSCheck")
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
	})

	// T043 / US4 — FR-113, the crash case.
	//
	// This asserts that an orphan is *reclaimable*, not that it has been
	// reclaimed. The sweeper only deletes pods that are terminal AND older than
	// its 5-minute minimum age, and it runs at startup then hourly
	// (internal/probe/sweeper.go) — so waiting out a real sweep would take over
	// an hour of wall clock. The sweep itself has its own unit coverage in
	// internal/probe/sweeper_test.go; what is specific to DNSCheck, and what is
	// checked here, is that the pod it leaves behind carries everything both
	// reclamation paths need: the two labels the sweeper selects on, and an
	// ownerReference so deleting the check collects it immediately without
	// waiting for any sweep at all.
	It("leaves a crash orphan that both reclamation paths can collect", func() {
		manifest := writeDNSCheckManifest("orphan-probe", "40s", "2m")
		DeferCleanup(func() {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", manifest, "--ignore-not-found=true"))
			_ = os.Remove(manifest)
		})

		By("applying a DNSCheck whose probe Pod lingers")
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", manifest))
		Expect(err).NotTo(HaveOccurred())

		var observed dnsCheckPod
		Eventually(func(g Gomega) {
			pods, listErr := listProbePods(dnsCheckNamespace)
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(pods).NotTo(BeEmpty())
			observed = pods[0]
		}, 3*time.Minute, 2*time.Second).Should(Succeed())

		By("restarting the operator mid-run, orphaning the in-flight probe Pod")
		_, err = utils.Run(exec.Command("kubectl", "delete", "pods",
			"-n", "fathom-system", "-l", "control-plane=controller-manager", "--wait=true"))
		Expect(err).NotTo(HaveOccurred())

		By("asserting the orphan carries what the sweeper selects on")
		Expect(observed.Metadata.Labels).To(HaveKeyWithValue("fathom.skaphos.io/managed-by", "fathom"))
		Expect(observed.Metadata.Labels).To(HaveKey("fathom.skaphos.io/probe"),
			"without the probe label the orphan sweep would never see this Pod")

		By("asserting the orphan is still owned, so deleting the check collects it now")
		Expect(observed.Metadata.OwnerReferences).To(HaveLen(1))
		Expect(observed.Metadata.OwnerReferences[0].Kind).To(Equal("DNSCheck"))

		By("deleting the check and expecting garbage collection, not a sweep")
		_, err = utils.Run(exec.Command("kubectl", "delete", "-f", manifest, "--wait=true"))
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			pods, listErr := listProbePods(dnsCheckNamespace)
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(pods).To(BeEmpty(),
				"the orphan survived its owner's deletion; garbage collection did not engage")
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("waiting for the operator to become ready again after the restart")
		Eventually(func(g Gomega) {
			out, readyErr := utils.Run(exec.Command("kubectl", "get", "pods",
				"-n", "fathom-system", "-l", "control-plane=controller-manager",
				"-o", "jsonpath={.items[*].status.phase}"))
			g.Expect(readyErr).NotTo(HaveOccurred())
			g.Expect(out).To(ContainSubstring("Running"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	})
})
