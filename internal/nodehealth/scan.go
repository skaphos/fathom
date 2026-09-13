/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package nodehealth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// DefaultKubeletHealthzURL is the kubelet's localhost health endpoint. It
	// binds to 127.0.0.1 by default (--healthz-bind-address), which is why a
	// KubeletHealthz check needs the agent on the host network.
	DefaultKubeletHealthzURL = "http://127.0.0.1:10248/healthz"

	// defaultProbeTimeout bounds one kubelet or runtime-socket probe when
	// ScanOptions.Timeout is unset.
	defaultProbeTimeout = 5 * time.Second

	// maxHealthzBody bounds how much of the kubelet's response the agent reads;
	// the body is discarded, this only lets the connection be reused.
	maxHealthzBody = 1024
)

// permissionDeniedSummary describes a path the node-agent cannot read. Such
// paths are reported Skipped (not Error) so a root-only location never makes a
// healthy node's aggregate report Error, matching nodecert.
const permissionDeniedSummary = "permission denied: the node-agent cannot read this path"

type statfsFunc func(path string, st *unix.Statfs_t) error
type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// ScanOptions configures a single Scan pass.
type ScanOptions struct {
	// Items are the resolved checks to evaluate. NodeCondition items are
	// ignored: the operator evaluates them from the Node object.
	Items []Item
	// Timeout bounds each kubelet or runtime-socket probe. Zero means
	// defaultProbeTimeout.
	Timeout time.Duration
	// KubeletHealthzURL overrides DefaultKubeletHealthzURL.
	KubeletHealthzURL string

	// Seams for tests; nil selects the real implementation.
	statfs     statfsFunc
	dial       dialFunc
	httpClient *http.Client
}

// Scan evaluates every agent-side item. It never returns an error: a path the
// agent cannot stat surfaces as a Skipped result, an unreachable kubelet or
// runtime socket as Fail (that unreachability is the signal the check exists
// to detect), and genuine measurement failures as Error. Results are returned
// in a deterministic order (by type, then path).
func Scan(ctx context.Context, opts ScanOptions) []CheckResult {
	statfs := opts.statfs
	if statfs == nil {
		statfs = unix.Statfs
	}
	dial := opts.dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	client := opts.httpClient
	if client == nil {
		client = &http.Client{}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	healthzURL := opts.KubeletHealthzURL
	if healthzURL == "" {
		healthzURL = DefaultKubeletHealthzURL
	}

	var out []CheckResult
	for _, it := range opts.Items {
		switch it.Type {
		case TypeDiskHeadroom:
			out = append(out, headroom(it, statfs, false))
		case TypeInodeHeadroom:
			out = append(out, headroom(it, statfs, true))
		case TypeKubeletHealthz:
			out = append(out, kubeletHealthz(ctx, client, healthzURL, timeout))
		case TypeContainerRuntime:
			out = append(out, containerRuntime(ctx, dial, it.SocketPath, timeout))
		case TypeNodeCondition:
			// Operator-evaluated from the Node object; nothing for the agent.
		default:
			out = append(out, CheckResult{Type: it.Type, Path: it.Path, Outcome: OutcomeError,
				Summary: fmt.Sprintf("unknown check type %q (operator newer than this agent?)", it.Type)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// headroom measures free bytes (or inodes) on the filesystem holding it.Path
// and classifies the free percentage against the item's thresholds. Bavail —
// blocks available to an unprivileged caller — is used rather than Bfree so the
// reservation ext4 keeps for root is not counted as headroom the kubelet's
// workloads can use.
func headroom(it Item, statfs statfsFunc, inodes bool) CheckResult {
	res := CheckResult{Type: it.Type, Path: it.Path}
	var st unix.Statfs_t
	if err := statfs(it.Path, &st); err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOENT):
			res.Outcome = OutcomeSkipped
			res.Summary = "path does not exist on this node"
		case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EACCES):
			res.Outcome = OutcomeSkipped
			res.Summary = permissionDeniedSummary
		default:
			res.Outcome = OutcomeError
			res.Summary = fmt.Sprintf("cannot stat filesystem: %v", err)
		}
		return res
	}

	// Blocks, Bavail, Files, and Ffree are uint64 on every target the agent
	// ships for (linux/amd64, linux/arm64) and on darwin; only Bsize differs
	// (int64 on linux, uint32 on darwin), so it alone needs the conversion.
	var total, free uint64
	unit := "bytes"
	if inodes {
		total, free = st.Files, st.Ffree
		unit = "inodes"
	} else {
		bsize := uint64(st.Bsize) //nolint:unconvert // width differs by GOOS; see above.
		total, free = st.Blocks*bsize, st.Bavail*bsize
	}
	if total == 0 {
		res.Outcome = OutcomeError
		res.Summary = fmt.Sprintf("filesystem reports zero %s", unit)
		return res
	}
	pct := float64(free) / float64(total) * 100
	res.PercentFree = &pct
	res.Total = total
	res.Free = free

	switch {
	case pct <= float64(it.CriticalPercentFree):
		res.Outcome = OutcomeFail
		res.Summary = fmt.Sprintf("%.1f%% of %s free (at or below criticalPercentFree %d)", pct, unit, it.CriticalPercentFree)
	case pct <= float64(it.WarnPercentFree):
		res.Outcome = OutcomeWarn
		res.Summary = fmt.Sprintf("%.1f%% of %s free (at or below warnPercentFree %d)", pct, unit, it.WarnPercentFree)
	default:
		res.Outcome = OutcomePass
		res.Summary = fmt.Sprintf("%.1f%% of %s free", pct, unit)
	}
	return res
}

// kubeletHealthz GETs the kubelet's health endpoint. An unreachable endpoint is
// Fail, not Error: a kubelet that is down is exactly what the check detects,
// and the agent only reaches this code when the operator put it on the host
// network for that purpose.
func kubeletHealthz(ctx context.Context, client *http.Client, url string, timeout time.Duration) CheckResult {
	res := CheckResult{Type: TypeKubeletHealthz}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		res.Outcome = OutcomeError
		res.Summary = fmt.Sprintf("cannot build kubelet health request: %v", err)
		return res
	}
	resp, err := client.Do(req)
	if err != nil {
		res.Outcome = OutcomeFail
		res.Summary = fmt.Sprintf("kubelet health endpoint %s unreachable: %v", url, err)
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxHealthzBody))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		res.Outcome = OutcomePass
		res.Summary = fmt.Sprintf("kubelet health endpoint %s returned %d", url, resp.StatusCode)
		return res
	}
	res.Outcome = OutcomeFail
	res.Summary = fmt.Sprintf("kubelet health endpoint %s returned %d", url, resp.StatusCode)
	return res
}

// containerRuntime dials the CRI socket and closes the connection. Accepting a
// connection is the liveness signal; the agent deliberately speaks no CRI so it
// carries no gRPC/protobuf and can never issue a runtime command even if
// compromised. A missing socket is Fail (the runtime is not where the check
// says it is), a socket the agent cannot open is Skipped (a permission verdict,
// not a runtime one), and a refused connection is Fail.
func containerRuntime(ctx context.Context, dial dialFunc, socket string, timeout time.Duration) CheckResult {
	res := CheckResult{Type: TypeContainerRuntime, Path: socket}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := dial(ctx, "unix", socket)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOENT):
			res.Outcome = OutcomeFail
			res.Summary = fmt.Sprintf("container runtime socket %s does not exist", socket)
		case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EACCES):
			res.Outcome = OutcomeSkipped
			res.Summary = fmt.Sprintf("permission denied: the node-agent cannot open %s", socket)
		default:
			res.Outcome = OutcomeFail
			res.Summary = fmt.Sprintf("container runtime socket %s refused connection: %v", socket, err)
		}
		return res
	}
	_ = conn.Close()
	res.Outcome = OutcomePass
	res.Summary = fmt.Sprintf("container runtime accepting connections on %s", socket)
	return res
}
