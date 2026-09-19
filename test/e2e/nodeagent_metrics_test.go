/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	prommodel "github.com/prometheus/common/model"

	"github.com/skaphos/fathom/test/utils"
)

const (
	metricsTestNamespace        = "fathom-metrics-e2e"
	metricsReaderServiceAccount = "metrics-reader"
	metricsDeniedServiceAccount = "metrics-denied"
	metricsReaderBinding        = "fathom-metrics-e2e-reader"
	podNetworkHealthCheckName   = "nodehealth-metrics-e2e"
)

var metricsCurlSequence atomic.Uint64

var _ = AfterSuite(func() {
	By("cleaning up the metrics security test namespace and binding")
	_, _ = utils.Run(exec.Command("kubectl", "delete", "clusterrolebinding", metricsReaderBinding,
		"--ignore-not-found=true"))
	_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", metricsTestNamespace,
		"--ignore-not-found=true", "--wait=false"))
})

var _ = Describe("Node-agent metrics security", Ordered, Label(utils.CoreLabel), func() {
	BeforeAll(func() {
		ensureMetricsTestNamespace()
	})

	It("authenticates and authorizes the operator metrics endpoint", func() {
		endpoint := fmt.Sprintf("https://%s.%s.svc.cluster.local:8443/metrics", metricsServiceName, namespace)

		By("denying an anonymous request from an admitted metrics namespace")
		anonymousStatus := strings.TrimSpace(runCurlPod(metricsDeniedServiceAccount,
			anonymousStatusCommand(endpoint)))
		Expect(anonymousStatus).To(SatisfyAny(Equal("401"), Equal("403")))

		By("denying an authenticated service account without metrics-reader")
		Expect(strings.TrimSpace(runCurlPod(metricsDeniedServiceAccount, authenticatedStatusCommand(endpoint)))).To(Equal("403"))

		By("allowing the service account bound to the shipped metrics-reader role")
		body := runCurlPod(metricsReaderServiceAccount, authenticatedBodyCommand(endpoint))
		Expect(body).To(ContainSubstring("fathom_adapter_registered"))
	})
})

func ensureMetricsTestNamespace() {
	GinkgoHelper()

	_, _ = utils.Run(exec.Command("kubectl", "create", "namespace", metricsTestNamespace))
	cmd := exec.Command("kubectl", "label", "namespace", metricsTestNamespace,
		"metrics=enabled", "pod-security.kubernetes.io/enforce=restricted", "--overwrite")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed to label metrics test namespace")

	for _, name := range []string{metricsReaderServiceAccount, metricsDeniedServiceAccount} {
		_, _ = utils.Run(exec.Command("kubectl", "create", "serviceaccount", name, "-n", metricsTestNamespace))
	}
	_, _ = utils.Run(exec.Command("kubectl", "create", "clusterrolebinding", metricsReaderBinding,
		"--clusterrole=fathom-metrics-reader",
		fmt.Sprintf("--serviceaccount=%s:%s", metricsTestNamespace, metricsReaderServiceAccount)))
}

func authenticatedStatusCommand(endpoint string) string {
	return fmt.Sprintf(`token="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"; %s`,
		operatorStatusCommand(endpoint, `-H "Authorization: Bearer ${token}"`))
}

func authenticatedBodyCommand(endpoint string) string {
	return fmt.Sprintf(`token="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"; : > /tmp/curl-error; i=0; while [ "$i" -lt 12 ]; do if curl -sSkf --connect-timeout 3 --max-time 5 -H "Authorization: Bearer ${token}" -o /tmp/body %s 2>/tmp/curl-error; then cat /tmp/body; exit 0; fi; i=$((i + 1)); [ "$i" -lt 12 ] && sleep 2; done; printf 'operator metrics request failed after 12 attempts\n' >&2; cat /tmp/curl-error >&2; exit 1`, endpoint)
}

func anonymousStatusCommand(endpoint string) string {
	return operatorStatusCommand(endpoint, "")
}

// operatorStatusCommand retries only curl transport failures. Any HTTP response
// is returned immediately so an unexpected 200 in a denial test cannot be
// retried into a passing 401/403 response.
func operatorStatusCommand(endpoint, header string) string {
	return fmt.Sprintf(`scratch=$(mktemp -d) || exit 1; trap 'rm -rf "$scratch"' EXIT; : > "$scratch/curl-error"; i=0; while [ "$i" -lt 12 ]; do if curl -sSk --connect-timeout 3 --max-time 5 %s -o /dev/null -w '%%{http_code}' %s > "$scratch/status" 2>"$scratch/curl-error"; then cat "$scratch/status"; exit 0; fi; i=$((i + 1)); [ "$i" -lt 12 ] && sleep 2; done; printf 'operator metrics transport failed after 12 attempts\n' >&2; cat "$scratch/curl-error" >&2; exit 1`, header, endpoint)
}

func runCurlPod(serviceAccount, shellCommand string) string {
	GinkgoHelper()
	ensureMetricsTestNamespace()

	podName := fmt.Sprintf("metrics-curl-%d", metricsCurlSequence.Add(1))
	overrides := fmt.Sprintf(`{
		"spec": {
			"activeDeadlineSeconds": 120,
			"serviceAccountName": %q,
			"restartPolicy": "Never",
			"containers": [{
				"name": "curl",
				"image": "curlimages/curl:latest",
				"command": ["/bin/sh", "-c"],
				"args": [%q],
				"securityContext": {
					"allowPrivilegeEscalation": false,
					"capabilities": {"drop": ["ALL"]},
					"runAsNonRoot": true,
					"runAsUser": 1000,
					"seccompProfile": {"type": "RuntimeDefault"}
				}
			}]
		}
	}`, serviceAccount, shellCommand)

	cmd := exec.Command("kubectl", "run", podName, "--restart=Never", "-n", metricsTestNamespace,
		"--image=curlimages/curl:latest", "--overrides", overrides)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed to create curl pod %s", podName)
	DeferCleanup(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "pod", podName, "-n", metricsTestNamespace,
			"--ignore-not-found=true", "--wait=false"))
	})

	Eventually(func(g Gomega) {
		out, err := utils.Run(exec.Command("kubectl", "get", "pod", podName, "-n", metricsTestNamespace,
			"-o", "jsonpath={.status.phase}"))
		g.Expect(err).NotTo(HaveOccurred())
		if out == "Failed" {
			logs, logErr := utils.Run(exec.Command("kubectl", "logs", podName, "-n", metricsTestNamespace))
			if logErr != nil {
				StopTrying(fmt.Sprintf("curl pod %s failed; logs unavailable: %v", podName, logErr)).Now()
			}
			StopTrying(fmt.Sprintf("curl pod %s failed: %s", podName, strings.TrimSpace(logs))).Now()
		}
		g.Expect(out).To(Equal("Succeeded"), "curl pod %s phase", podName)
	}, 3*time.Minute, 2*time.Second).Should(Succeed())

	out, err := utils.Run(exec.Command("kubectl", "logs", podName, "-n", metricsTestNamespace))
	Expect(err).NotTo(HaveOccurred(), "failed to read curl pod %s output", podName)
	return out
}

func assertAgentDoesNotServeMetrics(daemonSet, ns string) {
	GinkgoHelper()
	ensureMetricsTestNamespace()

	var endpoint agentEndpoint
	Eventually(func(g Gomega) {
		var err error
		endpoint, err = daemonSetAgentEndpoint(daemonSet, ns)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(endpoint.IP).NotTo(BeEmpty())
		g.Expect(endpoint.Port).To(BeNumerically(">", 0))
	}, time.Minute, 2*time.Second).Should(Succeed())

	command := fmt.Sprintf(
		`metrics=$(curl -sS --connect-timeout 5 --max-time 10 -o /dev/null -w '%%{http_code}' http://%s:%d/metrics) || exit $?; healthz=$(curl -sS --connect-timeout 5 --max-time 10 -o /dev/null -w '%%{http_code}' http://%s:%d/healthz) || exit $?; printf 'metrics=%%s healthz=%%s' "$metrics" "$healthz"`,
		endpoint.IP, endpoint.Port, endpoint.IP, endpoint.Port,
	)
	Expect(strings.TrimSpace(runCurlPod(metricsDeniedServiceAccount, command))).To(Equal("metrics=404 healthz=200"))
}

type agentEndpoint struct {
	IP   string
	Port int32
}

func daemonSetAgentEndpoint(daemonSet, ns string) (agentEndpoint, error) {
	out, err := utils.Run(exec.Command("kubectl", "get", "pods", "-n", ns,
		"-l", "fathom.skaphos.io/managed-by=fathom,fathom.skaphos.io/source-name="+strings.TrimSuffix(strings.TrimSuffix(daemonSet, "-node-agent"), "-node-health-agent"),
		"-o", "json"))
	if err != nil {
		return agentEndpoint{}, fmt.Errorf("get agent pods: %w", err)
	}
	var list struct {
		Items []struct {
			Status struct {
				Phase string `json:"phase"`
				PodIP string `json:"podIP"`
			} `json:"status"`
			Spec struct {
				Containers []struct {
					Name  string `json:"name"`
					Ports []struct {
						Name          string `json:"name"`
						ContainerPort int32  `json:"containerPort"`
					} `json:"ports"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return agentEndpoint{}, fmt.Errorf("decode agent pods: %w", err)
	}
	for _, pod := range list.Items {
		if pod.Status.Phase != "Running" || pod.Status.PodIP == "" {
			continue
		}
		for _, container := range pod.Spec.Containers {
			if container.Name != "node-agent" {
				continue
			}
			for _, port := range container.Ports {
				if port.Name == "metrics" {
					return agentEndpoint{IP: pod.Status.PodIP, Port: port.ContainerPort}, nil
				}
			}
		}
	}
	return agentEndpoint{}, fmt.Errorf("no running node-agent pod with a metrics port for daemonset %s/%s", ns, daemonSet)
}

func assertCertificateMetrics(report healthReport) {
	GinkgoHelper()
	body := scrapeAuthorizedOperatorMetrics("fathom_node_certificate_expiry_days")
	families := parseMetrics(body)
	family := families["fathom_node_certificate_expiry_days"]
	Expect(family).NotTo(BeNil(), "certificate expiry metric family absent")

	wantByNode := map[string]float64{}
	for _, check := range report.Spec.Checks {
		if check.Family != "node_certificate" {
			continue
		}
		days, err := strconv.ParseFloat(check.Details["daysRemaining"], 64)
		Expect(err).NotTo(HaveOccurred(), "invalid daysRemaining in HealthReport")
		current, exists := wantByNode[check.TargetRef.Name]
		if !exists || days < current {
			wantByNode[check.TargetRef.Name] = days
		}
	}
	Expect(wantByNode).NotTo(BeEmpty())

	gotByNode := map[string]float64{}
	for _, metric := range family.Metric {
		labels := metricLabels(metric)
		Expect(labels).NotTo(HaveKey("path"))
		Expect(labels).NotTo(HaveKey("subject"))
		Expect(labels).NotTo(HaveKey("issuer"))
		if labels["namespace"] == nodeCertSampleNS && labels["check"] == nodeCertSampleName {
			gotByNode[labels["node"]] = metric.GetGauge().GetValue()
		}
	}
	Expect(gotByNode).To(Equal(wantByNode), "operator expiry metric must equal the report minimum per node")
}

func assertNodeHealthMetrics() {
	GinkgoHelper()
	body := scrapeAuthorizedOperatorMetrics(
		"fathom_node_health_check_result",
		"fathom_node_health_filesystem_free_percent",
	)
	families := parseMetrics(body)

	result := families["fathom_node_health_check_result"]
	Expect(result).NotTo(BeNil(), "node-health result metric family absent")
	Expect(result.Metric).To(ContainElement(Satisfy(func(metric *dto.Metric) bool {
		labels := metricLabels(metric)
		return labels["namespace"] == nodeHealthSampleNS && labels["check"] == nodeHealthSampleName &&
			labels["node"] != "" && labels["type"] != "" && labels["path"] != "" && labels["result"] != ""
	})), "node-health result metric does not retain its item dimensions")

	headroom := families["fathom_node_health_filesystem_free_percent"]
	Expect(headroom).NotTo(BeNil(), "node-health filesystem metric family absent")
	Expect(headroom.Metric).To(ContainElement(Satisfy(func(metric *dto.Metric) bool {
		labels := metricLabels(metric)
		return labels["namespace"] == nodeHealthSampleNS && labels["check"] == nodeHealthSampleName &&
			labels["node"] != "" && labels["path"] != "" && labels["resource"] != ""
	})), "node-health filesystem metric does not retain its measurement dimensions")
}

func scrapeAuthorizedOperatorMetrics(requiredFamilies ...string) string {
	GinkgoHelper()
	endpoint := fmt.Sprintf("https://%s.%s.svc.cluster.local:8443/metrics", metricsServiceName, namespace)
	checks := make([]string, 0, len(requiredFamilies))
	for _, family := range requiredFamilies {
		checks = append(checks, fmt.Sprintf("grep -q '^%s{' /tmp/metrics", family))
	}
	command := fmt.Sprintf(
		`token="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"; : > /tmp/curl-error; i=0; while [ "$i" -lt 12 ]; do if curl -sSkf --connect-timeout 3 --max-time 5 -H "Authorization: Bearer ${token}" -o /tmp/metrics %s 2>/tmp/curl-error && %s; then cat /tmp/metrics; exit 0; fi; i=$((i + 1)); [ "$i" -lt 12 ] && sleep 2; done; printf 'required operator metric families unavailable after 12 attempts\n' >&2; cat /tmp/curl-error >&2; exit 1`,
		endpoint, strings.Join(checks, " && "),
	)
	return runCurlPod(metricsReaderServiceAccount, command)
}

func parseMetrics(body string) map[string]*dto.MetricFamily {
	GinkgoHelper()
	families, err := parseMetricsText(body)
	Expect(err).NotTo(HaveOccurred(), "operator returned invalid Prometheus text")
	return families
}

func parseMetricsText(body string) (map[string]*dto.MetricFamily, error) {
	// Prometheus text requires the final sample line to end in a newline.
	// Preserve the raw curl body in production, and tolerate callers or test
	// harnesses that have already trimmed it.
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	parser := expfmt.NewTextParser(prommodel.UTF8Validation)
	return parser.TextToMetricFamilies(strings.NewReader(body))
}

func TestParseMetricsText(t *testing.T) {
	families, err := parseMetricsText("# TYPE fathom_test gauge\nfathom_test{namespace=\"default\"} 1\n")
	if err != nil {
		t.Fatalf("parse metrics text: %v", err)
	}
	if got := families["fathom_test"]; got == nil || len(got.Metric) != 1 || got.Metric[0].GetGauge().GetValue() != 1 {
		t.Fatalf("parsed metric family = %#v, want one gauge with value 1", got)
	}
}

func TestParseMetricsTextAcceptsTrimmedCurlOutput(t *testing.T) {
	body := strings.TrimSpace("# TYPE fathom_test gauge\nfathom_test{namespace=\"default\"} 1\n")
	families, err := parseMetricsText(body)
	if err != nil {
		t.Fatalf("parse trimmed metrics text: %v", err)
	}
	if got := families["fathom_test"]; got == nil || len(got.Metric) != 1 {
		t.Fatalf("parsed metric family = %#v, want one metric", got)
	}
}

func TestOperatorStatusCommandRetriesTransportOnly(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			panic(http.ErrAbortHandler)
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	out, err := exec.Command("/bin/sh", "-c", anonymousStatusCommand(server.URL)).CombinedOutput()
	if err != nil {
		t.Fatalf("run retrying status command: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "403" {
		t.Fatalf("status = %q, want 403", got)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2 after one transport failure", got)
	}
}

func TestOperatorStatusCommandDoesNotRetryHTTPResponse(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	out, err := exec.Command("/bin/sh", "-c", anonymousStatusCommand(server.URL)).CombinedOutput()
	if err != nil {
		t.Fatalf("run status command: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "200" {
		t.Fatalf("status = %q, want 200", got)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want one HTTP attempt", got)
	}
}

func metricLabels(metric *dto.Metric) map[string]string {
	labels := make(map[string]string, len(metric.Label))
	for _, pair := range metric.Label {
		labels[pair.GetName()] = pair.GetValue()
	}
	return labels
}

func assertPodNetworkHealthAgentSecurity() {
	GinkgoHelper()
	manifest := fmt.Sprintf(`apiVersion: fathom.skaphos.io/v1alpha1
kind: NodeHealthCheck
metadata:
  name: %s
  namespace: %s
spec:
  includeControlPlaneNodes: true
  interval: 30s
  timeout: 20s
  checks:
    - type: DiskHeadroom
      path: /var/lib/kubelet
      warnPercentFree: 0
      criticalPercentFree: 0
`, podNetworkHealthCheckName, nodeHealthSampleNS)
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed to create pod-network NodeHealthCheck")
	DeferCleanup(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "nodehealthcheck", podNetworkHealthCheckName,
			"-n", nodeHealthSampleNS, "--ignore-not-found=true", "--wait=false"))
	})

	daemonSet := podNetworkHealthCheckName + "-node-health-agent"
	Eventually(func(g Gomega) {
		rollout, err := daemonSetRollout(daemonSet, nodeHealthSampleNS)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(rollout.desired).To(BeNumerically(">", 0))
		g.Expect(rollout.ready).To(Equal(rollout.desired))
	}, 3*time.Minute, 5*time.Second).Should(Succeed())

	out, err := utils.Run(exec.Command("kubectl", "get", "daemonset", daemonSet, "-n", nodeHealthSampleNS,
		"-o", "jsonpath={.spec.template.spec.hostNetwork}"))
	Expect(err).NotTo(HaveOccurred())
	Expect(out).To(SatisfyAny(BeEmpty(), Equal("false")), "headroom-only health agent must use the pod network")
	assertAgentDoesNotServeMetrics(daemonSet, nodeHealthSampleNS)

	Eventually(func(g Gomega) {
		status, err := nodeHealthStatus(podNetworkHealthCheckName, nodeHealthSampleNS)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(status.LastResult).To(Equal("Pass"), "pod-network agent did not keep publishing reports")
	}, 3*time.Minute, 5*time.Second).Should(Succeed())
}
