/*
SPDX-FileCopyrightText: 2026 Skaphos
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
