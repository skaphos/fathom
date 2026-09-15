/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth

import (
	"path"
	"sort"
	"strings"
)

// allowedPathPrefixes is the set of host directories a headroom check may
// measure — and therefore the set the operator will mount read-only into the
// node-agent. It is the operator-approved allowlist that stops a namespaced
// tenant from turning the node-agent into a confused deputy that mounts
// arbitrary host directories. The same list is mirrored in the NodeHealthCheck
// CRD's x-kubernetes-validations so a disallowed path is rejected at admission;
// PathAllowed enforces it again in the operator as defense-in-depth for
// clusters running an older CRD. The host root is never allowed: statfs of
// /var/lib/kubelet already reports the root filesystem on any node where it is
// not a separate mount.
var allowedPathPrefixes = []string{
	"/var/lib/kubelet",
	"/var/lib/containerd",
	"/var/lib/docker",
	"/var/lib/etcd",
	"/var/log",
	"/var/lib/rancher",
	"/etc/kubernetes",
	"/run/containerd",
}

// allowedSocketDirs is the set of directories a ContainerRuntime check's CRI
// socket may live under. Mirrored in the CRD's socketPath rule.
var allowedSocketDirs = []string{
	"/run/containerd",
	"/var/run/containerd",
	"/run/crio",
	"/var/run/crio",
	"/run/cri-dockerd",
	"/var/run/cri-dockerd",
	// k3s and RKE2 run their embedded containerd here.
	"/run/k3s/containerd",
	"/var/run/k3s/containerd",
}

// allowedSocketFiles are exact socket paths admitted on their own: runtimes
// whose socket sits directly in /run rather than in a directory of its own.
// A directory allowance for /run would admit every socket on the node (the
// Docker daemon's, for one), so these are exact matches. Mirrored in the CRD's
// socketPath rule.
var allowedSocketFiles = []string{
	// cri-dockerd's default (its documented endpoint is unix:///var/run/cri-dockerd.sock).
	"/run/cri-dockerd.sock",
	"/var/run/cri-dockerd.sock",
}

// AllowedSocketFiles returns a copy of the exact socket paths admitted on
// their own.
func AllowedSocketFiles() []string {
	return append([]string(nil), allowedSocketFiles...)
}

// AllowedPathPrefixes returns a copy of the operator-approved headroom-path
// prefixes.
func AllowedPathPrefixes() []string {
	return append([]string(nil), allowedPathPrefixes...)
}

// AllowedSocketDirs returns a copy of the operator-approved CRI socket
// directories.
func AllowedSocketDirs() []string {
	return append([]string(nil), allowedSocketDirs...)
}

// PathAllowed reports whether p is a measurable headroom path: absolute, clean
// of "..", never the host root, and rooted at one of allowedPathPrefixes. It is
// the Go twin of the CRD's path validation and must stay in lockstep with it.
// It does NOT trim whitespace, for the same reason nodecert.PathAllowed does
// not: the CEL rule validates the raw value.
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

// SocketPathAllowed reports whether p is a dialable CRI socket path: a
// traversal-free .sock file anywhere under one of allowedSocketDirs (nested
// paths included), or one of the exact allowedSocketFiles. It is the Go twin
// of the CRD's socketPath validation.
func SocketPathAllowed(p string) bool {
	if p == "" || !path.IsAbs(p) || strings.Contains(p, "..") || !strings.HasSuffix(p, ".sock") {
		return false
	}
	for _, dir := range allowedSocketDirs {
		if strings.HasPrefix(p, dir+"/") {
			return true
		}
	}
	for _, file := range allowedSocketFiles {
		if p == file {
			return true
		}
	}
	return false
}

// MountDirs computes the least-privilege set of host directories the agent
// must have mounted read-only to measure every headroom item, collapsed so no
// returned directory is a descendant of another. Every headroom path is a
// directory by contract — nodecert.MinimalMountDirs is not reused because its
// file heuristic turns a directory with a dot in its last segment
// (/var/log/app.v1) into its parent, mounting the wrong thing. The host root
// is never mounted. Socket paths are not included: the socket is mounted
// individually with hostPath type Socket.
func MountDirs(items []Item) []string {
	seen := map[string]struct{}{}
	for _, it := range items {
		if it.Type != TypeDiskHeadroom && it.Type != TypeInodeHeadroom {
			continue
		}
		// The raw value, untrimmed, exactly as PathAllowed judged it: a mount
		// must never be computed from a different string than admission saw.
		p := it.Path
		if p == "" || !path.IsAbs(p) {
			continue
		}
		clean := path.Clean(p)
		if clean == "/" {
			continue
		}
		seen[clean] = struct{}{}
	}
	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
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
