/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/skaphos/fathom/test/utils"
)

const (
	definitionE2EAPIGroup   = "runtimefixture.skaphos.io"
	definitionE2EAPIVersion = definitionE2EAPIGroup + "/v1"
	definitionE2EAPIService = "v1." + definitionE2EAPIGroup
	definitionE2EAPIName    = "e2e-runtime-fixture"
)

// This server is test infrastructure, reached through a real APIService by the
// shipped operator. It never replaces an operator transport or controller.
type definitionE2EAPIFixture struct {
	server      *httptest.Server
	managerUser string
	readerUser  string

	mu                       sync.Mutex
	denyDiscovery            bool
	holdRead                 bool
	deniedReaderDiscovery    int
	managerDiscoveryInDenial int
	managerDiscoveryTotal    int
	readerLists              int
	users                    map[string]int
	entered                  chan *definitionE2EHeldRead
	active                   map[*definitionE2EHeldRead]bool
}

type definitionE2EHeldRead struct {
	user    string
	release chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (h *definitionE2EHeldRead) Release() { h.once.Do(func() { close(h.release) }) }

func definitionE2ENewAPIFixture(managerUser, readerUser string) *definitionE2EAPIFixture {
	f := &definitionE2EAPIFixture{
		managerUser: managerUser,
		readerUser:  readerUser,
		users:       map[string]int{},
		entered:     make(chan *definitionE2EHeldRead, 32),
		active:      map[*definitionE2EHeldRead]bool{},
	}
	return f
}

func (f *definitionE2EAPIFixture) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		return
	}
	user := r.Header.Get("X-Remote-User")
	if user == "" {
		user = r.Header.Get("Impersonate-User")
	}
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	f.users[user]++
	f.mu.Unlock()
	switch r.URL.Path {
	case "/apis/" + definitionE2EAPIGroup:
		_, _ = fmt.Fprintf(w, `{"kind":"APIGroup","apiVersion":"v1","name":%q,"versions":[{"groupVersion":%q,"version":"v1"}],"preferredVersion":{"groupVersion":%q,"version":"v1"}}`,
			definitionE2EAPIGroup, definitionE2EAPIVersion, definitionE2EAPIVersion)
	case "/apis/" + definitionE2EAPIVersion:
		f.mu.Lock()
		deny := f.denyDiscovery && user == f.readerUser
		if user == f.managerUser {
			f.managerDiscoveryTotal++
		}
		if f.denyDiscovery && user == f.managerUser {
			f.managerDiscoveryInDenial++
		}
		if deny {
			f.deniedReaderDiscovery++
		}
		f.mu.Unlock()
		if deny {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403,"message":"fixture denies delegated discovery"}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"kind":"APIResourceList","apiVersion":"v1","groupVersion":%q,"resources":[{"name":"widgets","singularName":"widget","namespaced":true,"kind":"Widget","verbs":["get","list"]}]}`,
			definitionE2EAPIVersion)
	case "/apis/" + definitionE2EAPIVersion + "/namespaces/external-secrets/widgets":
		f.mu.Lock()
		hold := f.holdRead && user == f.readerUser
		var held *definitionE2EHeldRead
		var entered chan *definitionE2EHeldRead
		if hold {
			held = &definitionE2EHeldRead{user: user, release: make(chan struct{}), done: make(chan struct{})}
			f.active[held] = true
			entered = f.entered
		}
		if user == f.readerUser {
			f.readerLists++
		}
		f.mu.Unlock()
		if hold {
			defer func() {
				f.mu.Lock()
				delete(f.active, held)
				f.mu.Unlock()
				close(held.done)
			}()
			select {
			case entered <- held:
			default:
			}
			select {
			case <-held.release:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = fmt.Fprintf(w, `{"kind":"WidgetList","apiVersion":%q,"metadata":{"resourceVersion":"1"},"items":[{"kind":"Widget","apiVersion":%q,"metadata":{"name":"ready","namespace":"external-secrets","resourceVersion":"1"},"status":{"phase":"Ready"}}]}`,
			definitionE2EAPIVersion, definitionE2EAPIVersion)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","reason":"NotFound","code":404}`))
	}
}

func (f *definitionE2EAPIFixture) setDenial(deny bool) {
	f.mu.Lock()
	f.denyDiscovery = deny
	f.mu.Unlock()
}

func (f *definitionE2EAPIFixture) holdNextRead() {
	Eventually(f.activeReads, 10*time.Second, 100*time.Millisecond).Should(BeZero(),
		"the prior held API request did not leave the fixture")
	f.mu.Lock()
	f.entered = make(chan *definitionE2EHeldRead, 32)
	f.holdRead = true
	f.mu.Unlock()
}

func (f *definitionE2EAPIFixture) releaseRead() {
	f.mu.Lock()
	f.holdRead = false
	held := make([]*definitionE2EHeldRead, 0, len(f.active))
	for token := range f.active {
		held = append(held, token)
	}
	f.mu.Unlock()
	for _, token := range held {
		token.Release()
	}
}

func (f *definitionE2EAPIFixture) activeReads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.active)
}

func (f *definitionE2EAPIFixture) counts() (denied, manager, lists int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deniedReaderDiscovery, f.managerDiscoveryInDenial, f.readerLists
}

func (f *definitionE2EAPIFixture) managerRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.managerDiscoveryTotal
}

func (f *definitionE2EAPIFixture) observedUsers() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := make(map[string]int, len(f.users))
	for user, count := range f.users {
		copy[user] = count
	}
	return copy
}

func (f *definitionE2EAPIFixture) install() {
	By("registering a host HTTPS fixture through a real Kubernetes APIService")
	cert, ca := definitionE2EAPICertificate()
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	port := listener.Addr().(*net.TCPAddr).Port
	f.server = httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	f.server.Listener = listener
	f.server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	f.server.StartTLS()
	DeferCleanup(func() {
		f.releaseRead()
		_, _ = utils.Run(exec.Command("kubectl", "delete", "apiservice", definitionE2EAPIService, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "endpointslice", definitionE2EAPIName,
			"-n", definitionE2ENS, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "service", definitionE2EAPIName,
			"-n", definitionE2ENS, "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "role,rolebinding", definitionE2EAPIName+"-read",
			"-n", "external-secrets", "--ignore-not-found=true"))
		f.server.Close()
	})
	address := definitionE2EHostGateway()
	definitionE2EApply(fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  ports:
  - name: https
    port: 443
    protocol: TCP
---
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: %s
  namespace: %s
  labels:
    kubernetes.io/service-name: %s
    endpointslice.kubernetes.io/managed-by: fathom-e2e
addressType: IPv4
ports:
- name: https
  protocol: TCP
  port: %d
endpoints:
- addresses: [%s]
  conditions:
    ready: true
---
apiVersion: apiregistration.k8s.io/v1
kind: APIService
metadata:
  name: %s
spec:
  group: %s
  version: v1
  groupPriorityMinimum: 1000
  versionPriority: 15
  service:
    name: %s
    namespace: %s
    port: 443
  caBundle: %s
`, definitionE2EAPIName, definitionE2ENS,
		definitionE2EAPIName, definitionE2ENS, definitionE2EAPIName, port, address,
		definitionE2EAPIService, definitionE2EAPIGroup,
		definitionE2EAPIName, definitionE2ENS, base64.StdEncoding.EncodeToString(ca)))
	Eventually(func() error {
		_, err := utils.Run(exec.Command("kubectl", "get", "--raw", "/apis/"+definitionE2EAPIVersion))
		return err
	}, 40*time.Second, time.Second).Should(Succeed(),
		"aggregated API did not reach host fixture at %s:%d", address, port)
}

func definitionE2EAPICertificate() (tls.Certificate, []byte) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	dns := definitionE2EAPIName + "." + definitionE2ENS + ".svc"
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: dns},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		DNSNames: []string{dns}, BasicConstraintsValid: true, IsCA: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	private, err := x509.MarshalPKCS8PrivateKey(key)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	parsed, err := tls.X509KeyPair(ca, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return parsed, ca
}

func definitionE2EHostGateway() string {
	cluster := os.Getenv("E2E_KIND_CLUSTER")
	if cluster == "" {
		cluster = "fathom-e2e"
	}
	nodes, err := utils.Run(exec.Command("kind", "get", "nodes", "--name", cluster))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), nodes)
	parts := strings.Fields(nodes)
	ExpectWithOffset(1, parts).NotTo(BeEmpty())
	if out, err := utils.Run(exec.Command("docker", "exec", parts[0], "getent", "ahostsv4", "host.docker.internal")); err == nil {
		for _, field := range strings.Fields(out) {
			if ip := net.ParseIP(field); ip != nil && ip.To4() != nil {
				return field
			}
		}
	}
	out, err := utils.Run(exec.Command("docker", "network", "inspect", "kind", "--format", "{{(index .IPAM.Config 0).Gateway}}"))
	ExpectWithOffset(1, err).NotTo(HaveOccurred(),
		"the test host has no Docker host gateway reachable from Kind")
	address := strings.TrimSpace(out)
	ExpectWithOffset(1, net.ParseIP(address)).NotTo(BeNil(), "invalid Docker Kind gateway %q", address)
	ExpectWithOffset(1, net.ParseIP(address).To4()).NotTo(BeNil(), "fixture requires an IPv4 host gateway")
	return address
}

func definitionE2EAPIReaderGrant(reader string) string {
	return fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s-read
  namespace: external-secrets
rules:
- apiGroups: [%s]
  resources: [widgets]
  verbs: [get, list]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s-read
  namespace: external-secrets
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s-read
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, definitionE2EAPIName, definitionE2EAPIGroup, definitionE2EAPIName,
		definitionE2EAPIName, reader, namespace)
}

func definitionE2EAPIDefinition() string {
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
    - name: fixture
      kind: Field
      field:
        target:
          scope: Namespaced
          namespaces: [external-secrets]
        apiVersion: %s
        kind: Widget
        listKind: WidgetList
        fieldPath: [status, phase]
        expectedValue: Ready
`, definitionE2EName, definitionE2EName, definitionE2EAPIVersion)
}
