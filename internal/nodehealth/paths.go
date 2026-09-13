/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth

import (
	"path"
	"strings"

	"github.com/skaphos/fathom/internal/nodecert"
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
// traversal-free .sock file directly under one of allowedSocketDirs. It is the
// Go twin of the CRD's socketPath validation.
func SocketPathAllowed(p string) bool {
	if p == "" || !path.IsAbs(p) || strings.Contains(p, "..") || !strings.HasSuffix(p, ".sock") {
		return false
	}
	for _, dir := range allowedSocketDirs {
		if strings.HasPrefix(p, dir+"/") {
			return true
		}
	}
	return false
}

// FilterAllowedItems returns the items whose paths satisfy the allowlists,
// preserving order. Headroom items with a disallowed Path and ContainerRuntime
// items with a disallowed SocketPath are dropped; every other item passes
// through. The operator applies it before building the agent's mount set so a
// value that slipped past admission still cannot widen the hostPath surface.
func FilterAllowedItems(items []Item) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		switch it.Type {
		case TypeDiskHeadroom, TypeInodeHeadroom:
			if !PathAllowed(it.Path) {
				continue
			}
		case TypeContainerRuntime:
			if it.SocketPath != "" && !SocketPathAllowed(it.SocketPath) {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

// MountDirs computes the least-privilege set of host directories the agent
// must have mounted read-only to measure every headroom item, collapsed so no
// returned directory is a descendant of another. The computation is shared
// with nodecert: a headroom path is a directory, which MinimalMountDirs uses
// as-is. Socket paths are not included — the socket is mounted individually,
// as a hostPath of type Socket, by the operator.
func MountDirs(items []Item) []string {
	var paths []string
	for _, it := range items {
		if it.Type == TypeDiskHeadroom || it.Type == TypeInodeHeadroom {
			paths = append(paths, it.Path)
		}
	}
	return nodecert.MinimalMountDirs(paths)
}
