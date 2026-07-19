/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodecert

import (
	"path"
	"sort"
	"strings"
)

// defaultCertPaths is the distribution-agnostic default scan set. It spans the
// common control-plane and node certificate locations across kubeadm, k3s,
// RKE2, and standalone etcd. Paths that do not exist on a node — or that the
// hardened, non-root agent cannot read (e.g. root-only kubeconfigs and the
// kubelet PKI dir) — are reported Skipped, never Error, so listing a
// cross-distribution superset is safe and a healthy node still aggregates to
// Pass/Warn/Fail from its readable certificates.
var defaultCertPaths = []string{
	// kubeadm / generic control-plane PKI (covers apiserver, kubelet-client,
	// front-proxy, ca, and the etcd subdirectory beneath it).
	"/etc/kubernetes/pki",
	// kubeadm kubeconfigs with embedded client certificates.
	"/etc/kubernetes/admin.conf",
	"/etc/kubernetes/controller-manager.conf",
	"/etc/kubernetes/scheduler.conf",
	"/etc/kubernetes/super-admin.conf",
	"/etc/kubernetes/kubelet.conf",
	// kubelet client/serving certificates.
	"/var/lib/kubelet/pki",
	// standalone etcd.
	"/etc/etcd/pki",
	// k3s server TLS material.
	"/var/lib/rancher/k3s/server/tls",
	// RKE2 server TLS material.
	"/var/lib/rancher/rke2/server/tls",
}

// DefaultCertPaths returns a copy of the default scan path set.
func DefaultCertPaths() []string {
	return append([]string(nil), defaultCertPaths...)
}

// allowedPathPrefixes is the set of host-directory roots a NodeCertificateCheck
// may scan. It is the operator-approved allowlist that stops a namespaced tenant
// from turning the privileged node-agent into a confused deputy that mounts
// arbitrary host directories. Every entry in defaultCertPaths lives under one of
// these prefixes (asserted by a unit test), and the same allowlist is mirrored
// in the NodeCertificateCheck CRD's x-kubernetes-validations so a disallowed path
// is rejected at admission; PathAllowed enforces it again in the operator as
// defense-in-depth for clusters running an older CRD.
var allowedPathPrefixes = []string{
	"/etc/kubernetes",
	"/var/lib/kubelet",
	"/etc/etcd",
	"/var/lib/etcd",
	"/var/lib/rancher",
}

// AllowedPathPrefixes returns a copy of the operator-approved scan-path prefixes.
func AllowedPathPrefixes() []string {
	return append([]string(nil), allowedPathPrefixes...)
}

// PathAllowed reports whether p is a scannable path: absolute, clean of "..",
// never the host root, and rooted at one of allowedPathPrefixes. It is the Go
// twin of the CRD's path validation and must stay in lockstep with it. Note it
// does NOT trim surrounding whitespace: the CRD CEL rule validates the raw value
// (a leading space fails startsWith('/')), so normalizing here would make the
// operator accept inputs the API server rejects and break that lockstep.
func PathAllowed(p string) bool {
	if p == "" || !path.IsAbs(p) || strings.Contains(p, "..") {
		return false
	}
	clean := path.Clean(p)
	if clean == "/" {
		return false
	}
	for _, pre := range allowedPathPrefixes {
		if clean == pre || strings.HasPrefix(clean, pre+"/") {
			return true
		}
	}
	return false
}

// FilterAllowedPaths returns the entries of paths that satisfy PathAllowed,
// preserving order. It is applied by the operator before building the agent's
// mount set so a path that slipped past admission (e.g. an older CRD without the
// allowlist rule) still cannot widen the agent's hostPath surface.
func FilterAllowedPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if PathAllowed(p) {
			out = append(out, p)
		}
	}
	return out
}

// certFileExtensions are the file extensions treated as raw PEM certificate
// files when scanning a directory or classifying a single path.
var certFileExtensions = map[string]struct{}{
	".crt":  {},
	".pem":  {},
	".cert": {},
}

// kubeconfigExtensions are the file extensions parsed as kubeconfigs (their
// embedded client/CA certificate data is extracted).
var kubeconfigExtensions = map[string]struct{}{
	".conf":       {},
	".kubeconfig": {},
}

func isCertFile(name string) bool {
	_, ok := certFileExtensions[strings.ToLower(path.Ext(name))]
	return ok
}

func isKubeconfigFile(name string) bool {
	_, ok := kubeconfigExtensions[strings.ToLower(path.Ext(name))]
	return ok
}

// MinimalMountDirs computes the least-privilege set of host directories that
// must be mounted read-only into the node-agent so it can read every configured
// path. For a path with a file extension the parent directory is used; a path
// without an extension is treated as a directory and used as-is. The result is
// collapsed so that no returned directory is a descendant of another (mounting
// an ancestor already exposes its children), keeping the DaemonSet's hostPath
// surface as small as possible. Returned directories are clean, absolute, and
// sorted. Non-absolute paths are ignored.
func MinimalMountDirs(paths []string) []string {
	seen := map[string]struct{}{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || !path.IsAbs(p) {
			continue
		}
		clean := path.Clean(p)
		dir := clean
		if path.Ext(clean) != "" {
			dir = path.Dir(clean)
		}
		if dir == "" || dir == "/" {
			// Refuse to mount the host root: that is the opposite of least
			// privilege. A path whose only ancestor is "/" is dropped; users
			// must point at a real subdirectory.
			continue
		}
		seen[dir] = struct{}{}
	}

	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	// Collapse descendants: walk shortest-first and drop any dir already
	// covered by a kept ancestor.
	var kept []string
	for _, d := range dirs {
		covered := false
		for _, k := range kept {
			if d == k || strings.HasPrefix(d, k+"/") {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, d)
		}
	}
	return kept
}
