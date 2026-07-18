/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodecert

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	// maxDirDepth bounds recursive directory scans so a misconfigured path
	// cannot make the agent walk the whole filesystem.
	maxDirDepth = 6
	// maxCertsPerPath bounds the certificates emitted for a single configured
	// path so a large CA bundle directory cannot blow up the report.
	maxCertsPerPath = 256
)

// ScanOptions configures a single Scan pass.
type ScanOptions struct {
	// Paths are the absolute files/directories to scan. Empty uses DefaultCertPaths.
	Paths []string
	// Thresholds are the warn/critical day boundaries for classification.
	Thresholds Thresholds
	// Now is the reference time for expiry math. Zero means time.Now().
	Now time.Time
}

// Scan reads and classifies every certificate found under opts.Paths. It never
// returns an error. Unparsable certificates and genuine (non-permission) read
// failures surface as CertResults with OutcomeError; paths the non-root agent
// lacks permission to read surface as OutcomeSkipped (so a root-only cert never
// dominates a healthy node's aggregate); and paths that simply do not exist are
// omitted entirely. Results are returned in a deterministic order (by path,
// then source).
func Scan(opts ScanOptions) []CertResult {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	paths := opts.Paths
	if len(paths) == 0 {
		paths = DefaultCertPaths()
	}

	var out []CertResult
	for _, p := range paths {
		out = append(out, scanPath(p, opts.Thresholds, now)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func scanPath(p string, th Thresholds, now time.Time) []CertResult {
	info, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Absent paths are expected across distributions; stay silent so
			// the report is not flooded with Skipped entries.
			return nil
		}
		if errors.Is(err, fs.ErrPermission) {
			// The parent directory is not searchable by the non-root agent, so
			// we cannot even stat the path. That is a permission verdict, not a
			// certificate error: Skipped, consistent with scanDir/scanCertFile.
			return []CertResult{skippedResult(p, "file", permissionDeniedSummary)}
		}
		return []CertResult{errorResult(p, "file", fmt.Sprintf("cannot stat path: %v", err))}
	}
	if info.IsDir() {
		return scanDir(p, th, now)
	}
	if isKubeconfigFile(p) {
		return scanKubeconfig(p, th, now)
	}
	return scanCertFile(p, "file", th, now)
}

func scanDir(root string, th Thresholds, now time.Time) []CertResult {
	var out []CertResult
	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))

	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if len(out) >= maxCertsPerPath {
			return fs.SkipAll
		}
		if err != nil {
			// A permission error is not a certificate verdict: the non-root
			// node-agent simply cannot read this path (e.g. kubeadm's root-only
			// /etc/kubernetes/pki/etcd, mode 0700). Report it as Skipped — never
			// Error — so it does not dominate the node aggregate (Error outranks
			// every other outcome). A *discovered* unreadable subdirectory is
			// skipped silently; only the configured root surfaces a Skipped
			// result. Genuine, non-permission read errors are surfaced as Error.
			if errors.Is(err, fs.ErrPermission) {
				if p == root {
					out = append(out, skippedResult(p, "dir", permissionDeniedSummary))
				}
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			out = append(out, errorResult(p, "dir", fmt.Sprintf("cannot read: %v", err)))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			if strings.Count(filepath.Clean(p), string(os.PathSeparator))-rootDepth >= maxDirDepth {
				return fs.SkipDir
			}
			return nil
		}
		if !isCertFile(d.Name()) {
			return nil
		}
		out = append(out, scanCertFile(p, "dir:"+d.Name(), th, now)...)
		return nil
	})
	return out
}

func scanCertFile(p, source string, th Thresholds, now time.Time) []CertResult {
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return []CertResult{skippedResult(p, source, permissionDeniedSummary)}
		}
		return []CertResult{errorResult(p, source, fmt.Sprintf("cannot read certificate: %v", err))}
	}
	certs, perr := parsePEMCertificates(data)
	if perr != nil {
		return []CertResult{errorResult(p, source, perr.Error())}
	}
	if len(certs) == 0 {
		return []CertResult{errorResult(p, source, "no PEM certificate blocks found")}
	}
	return classifyAll(certs, p, source, th, now)
}

// minimalKubeconfig is the subset of a kubeconfig needed to extract embedded
// certificate material. Files that reference certificates by path instead of
// embedding them yield no results here (the referenced files can be scanned
// directly).
type minimalKubeconfig struct {
	Clusters []struct {
		Name    string `json:"name"`
		Cluster struct {
			CertificateAuthorityData string `json:"certificate-authority-data"`
		} `json:"cluster"`
	} `json:"clusters"`
	Users []struct {
		Name string `json:"name"`
		User struct {
			ClientCertificateData string `json:"client-certificate-data"`
		} `json:"user"`
	} `json:"users"`
}

func scanKubeconfig(p string, th Thresholds, now time.Time) []CertResult {
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return []CertResult{skippedResult(p, "kubeconfig", permissionDeniedSummary)}
		}
		return []CertResult{errorResult(p, "kubeconfig", fmt.Sprintf("cannot read kubeconfig: %v", err))}
	}
	var kc minimalKubeconfig
	if err := yaml.Unmarshal(data, &kc); err != nil {
		return []CertResult{errorResult(p, "kubeconfig", fmt.Sprintf("cannot parse kubeconfig: %v", err))}
	}

	var out []CertResult
	add := func(b64, source string) {
		if strings.TrimSpace(b64) == "" {
			return
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
		if err != nil {
			out = append(out, errorResult(p, source, fmt.Sprintf("invalid base64 certificate data: %v", err)))
			return
		}
		certs, perr := parsePEMCertificates(raw)
		if perr != nil {
			out = append(out, errorResult(p, source, perr.Error()))
			return
		}
		out = append(out, classifyAll(certs, p, source, th, now)...)
	}
	for _, u := range kc.Users {
		add(u.User.ClientCertificateData, "kubeconfig:client:"+u.Name)
	}
	for _, c := range kc.Clusters {
		add(c.Cluster.CertificateAuthorityData, "kubeconfig:ca:"+c.Name)
	}
	return out
}

func parsePEMCertificates(data []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		certs = append(certs, cert)
		if len(certs) >= maxCertsPerPath {
			break
		}
	}
	return certs, nil
}

func classifyAll(certs []*x509.Certificate, p, source string, th Thresholds, now time.Time) []CertResult {
	out := make([]CertResult, 0, len(certs))
	for i, cert := range certs {
		src := source
		if len(certs) > 1 {
			src = fmt.Sprintf("%s#%d", source, i)
		}
		out = append(out, classify(cert, now, th, p, src))
	}
	return out
}

func classify(cert *x509.Certificate, now time.Time, th Thresholds, p, source string) CertResult {
	remaining := cert.NotAfter.Sub(now)
	days := daysFromDuration(remaining)
	res := CertResult{
		Path:          p,
		Source:        source,
		Subject:       cert.Subject.String(),
		Issuer:        cert.Issuer.String(),
		SANs:          subjectAltNames(cert),
		Serial:        fmt.Sprintf("%x", cert.SerialNumber),
		NotAfter:      cert.NotAfter.UTC(),
		DaysRemaining: days,
	}
	switch {
	case remaining <= 0:
		res.Outcome = OutcomeFail
		res.Summary = fmt.Sprintf("certificate expired %d day(s) ago", -days)
	case days <= th.CriticalDays:
		res.Outcome = OutcomeFail
		res.Summary = fmt.Sprintf("certificate expires in %d day(s) (at or below criticalDays %d)", days, th.CriticalDays)
	case days <= th.WarnDays:
		res.Outcome = OutcomeWarn
		res.Summary = fmt.Sprintf("certificate expires in %d day(s) (at or below warnDays %d)", days, th.WarnDays)
	default:
		res.Outcome = OutcomePass
		res.Summary = fmt.Sprintf("certificate valid for %d day(s)", days)
	}
	return res
}

func subjectAltNames(cert *x509.Certificate) []string {
	if len(cert.DNSNames) == 0 && len(cert.IPAddresses) == 0 {
		return nil
	}
	sans := make([]string, 0, len(cert.DNSNames)+len(cert.IPAddresses))
	sans = append(sans, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		sans = append(sans, ip.String())
	}
	return sans
}

// daysFromDuration converts a remaining duration to whole days: ceiling while
// positive (28.2 days -> 29 "days remaining"), floor once negative (-1.2 days
// expired -> -2) so the value never rounds an expired certificate back to 0.
func daysFromDuration(d time.Duration) int {
	days := d.Hours() / 24
	if days < 0 {
		return int(math.Floor(days))
	}
	return int(math.Ceil(days))
}

// permissionDeniedSummary describes a path the hardened, non-root node-agent
// cannot read. Such paths are reported Skipped (not Error) so a root-only
// certificate set — common on kubeadm nodes — never makes a healthy node's
// aggregate report Error.
const permissionDeniedSummary = "permission denied: the non-root node-agent cannot read this path (run the agent with elevated read access to scan root-only certificates)"

func errorResult(p, source, summary string) CertResult {
	return CertResult{Path: p, Source: source, Outcome: OutcomeError, Summary: summary}
}

func skippedResult(p, source, summary string) CertResult {
	return CertResult{Path: p, Source: source, Outcome: OutcomeSkipped, Summary: summary}
}
