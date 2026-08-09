/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// Package probe contains shared in-cluster probe pod plumbing for adapters.
package probe

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ModeDNS        Mode = "dns"
	ModeTCPConnect Mode = "tcp-connect"
	ModeTCPListen  Mode = "tcp-listen"
	ModeHTTPGet    Mode = "http-get"

	OutcomePass  Outcome = "Pass"
	OutcomeFail  Outcome = "Fail"
	OutcomeError Outcome = "Error"

	defaultBinaryPath = "/probe"
	defaultTimeout    = 10 * time.Second
	labelManagedBy    = "fathom.skaphos.io/managed-by"
	labelProbeName    = "fathom.skaphos.io/probe"
	managedByValue    = "fathom"
	// probeContainerName is the sole container in a probe pod. Sweeper's
	// shape check depends on it, so the two must not drift apart.
	probeContainerName = "probe"
)

type Mode string

type Outcome string

// Request describes one probe pod. It intentionally avoids adapter-specific
// concepts so CoreDNS, CNI, and NetworkPolicy checks can share the same path.
type Request struct {
	Name      string
	Namespace string
	Image     string
	Mode      Mode
	Target    string
	Port      int
	Timeout   time.Duration

	// Expect lists the Prometheus metric family names an http-get probe
	// requires in the response body (the probe binary's -expect flag).
	// Ignored by the other modes.
	Expect []string

	Labels          map[string]string
	NodeSelector    map[string]string
	Tolerations     []corev1.Toleration
	AvoidPodLabels  map[string]string
	TopologyKey     string
	ServiceAccount  string
	ImagePullPolicy corev1.PullPolicy

	// DNSNameservers, when non-empty, pins the pod's resolver: the pod runs
	// with dnsPolicy None and exactly these nameservers (no cluster search
	// domains), so DNS-mode probes query the given resolvers directly instead
	// of the cluster's default resolution path. Callers must therefore pass
	// fully qualified targets. Used by node-local DNS checks to assert the
	// per-node cache rather than whatever kubelet configured (SKA-511).
	DNSNameservers []string

	// DNSFrom selects which resolver the probe pod queries. The zero value,
	// DNSFromCluster, inherits the cluster's resolution path exactly as before
	// this field existed, so callers that never set it are unaffected.
	//
	// The vantage point is a property of the pod, not of the probe binary: the
	// binary always asks whatever resolver it was given and never needs to know
	// which of the three it is.
	DNSFrom DNSSource

	// RecordType, ExpectedAnswers, and Absent shape a dns-mode assertion. Each
	// is emitted as a flag only when it differs from the probe's default, so
	// the argv of every existing caller is byte-for-byte unchanged.
	RecordType      string
	ExpectedAnswers []string
	Absent          bool
}

// DNSSource is the resolver vantage point a dns-mode probe queries from.
type DNSSource string

const (
	// DNSFromCluster resolves through the cluster's own DNS service by
	// inheriting the pod's default policy. It is the zero value.
	DNSFromCluster DNSSource = ""
	// DNSFromNode resolves through the node's own resolver by running the pod
	// with dnsPolicy Default, which despite the name is not the pod default.
	DNSFromNode DNSSource = "Node"
	// DNSFromExplicit resolves through the nameservers in DNSNameservers.
	DNSFromExplicit DNSSource = "Explicit"
)

type Result struct {
	Outcome Outcome           `json:"outcome"`
	Summary string            `json:"summary"`
	Details map[string]string `json:"details,omitempty"`
}

// Pod builds the hardened pod manifest for a single probe execution.
func Pod(req Request) (*corev1.Pod, error) {
	if req.Name == "" {
		return nil, errors.New("probe name is required")
	}
	if req.Namespace == "" {
		return nil, errors.New("probe namespace is required")
	}
	if req.Image == "" {
		return nil, errors.New("probe image is required")
	}
	// dnsPolicy None with no nameservers produces a pod that cannot resolve
	// anything, and the resulting failures look like a DNS outage rather than
	// the misconfiguration they are. Reject it at build time.
	if req.DNSFrom == DNSFromExplicit && len(req.DNSNameservers) == 0 {
		return nil, errors.New("explicit dns vantage point requires at least one nameserver")
	}
	// Nameservers only mean anything under the explicit vantage point. Applying
	// dnsPolicy Default and quietly discarding them would produce a probe that
	// queries the node's resolver while its caller believes it is querying the
	// nameservers it supplied — a wrong answer that looks like a right one.
	if req.DNSFrom == DNSFromNode && len(req.DNSNameservers) > 0 {
		return nil, errors.New("node dns vantage point cannot take nameservers; use the explicit vantage point")
	}
	args, err := args(req)
	if err != nil {
		return nil, err
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	// Caller labels are applied first so the reserved probe-identifying labels
	// always win. Both are load-bearing for Sweeper: managed-by+probe is how
	// it recognises an orphan to reap, and how it recognises everything else
	// as off-limits. A caller that overrode managed-by would leak its pods
	// silently — they would never match a sweep.
	labels := make(map[string]string, len(req.Labels)+2)
	for key, value := range req.Labels {
		labels[key] = value
	}
	labels[labelManagedBy] = managedByValue
	labels[labelProbeName] = req.Name
	runAsNonRoot := true
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true
	runAsUser := int64(65532)
	seccompProfile := corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
	terminationGracePeriod := int64(1)
	activeDeadlineSeconds := int64(timeout.Seconds()) + 5
	if activeDeadlineSeconds < 6 {
		activeDeadlineSeconds = 6
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: labels},
		Spec: corev1.PodSpec{
			AutomountServiceAccountToken:  boolPtr(false),
			RestartPolicy:                 corev1.RestartPolicyNever,
			ActiveDeadlineSeconds:         &activeDeadlineSeconds,
			TerminationGracePeriodSeconds: &terminationGracePeriod,
			NodeSelector:                  copyStringMap(req.NodeSelector),
			Tolerations:                   append([]corev1.Toleration(nil), req.Tolerations...),
			SecurityContext:               &corev1.PodSecurityContext{RunAsNonRoot: &runAsNonRoot, RunAsUser: &runAsUser, SeccompProfile: &seccompProfile},
			Containers: []corev1.Container{{
				Name:                     probeContainerName,
				Image:                    req.Image,
				ImagePullPolicy:          req.ImagePullPolicy,
				Command:                  []string{defaultBinaryPath},
				Args:                     append(args, "-timeout", timeout.String()),
				TerminationMessagePath:   "/dev/termination-log",
				TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &allowPrivilegeEscalation,
					ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
					RunAsNonRoot:             &runAsNonRoot,
					RunAsUser:                &runAsUser,
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10m"),
					corev1.ResourceMemory: resource.MustParse("16Mi"),
				}, Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				}},
			}},
		},
	}
	if req.ServiceAccount != "" {
		pod.Spec.ServiceAccountName = req.ServiceAccount
	}
	// A non-empty DNSNameservers has always meant "pin these resolvers", so it
	// keeps implying Explicit even when DNSFrom is unset. That is what lets the
	// existing caller keep working untouched.
	switch {
	case req.DNSFrom == DNSFromNode:
		pod.Spec.DNSPolicy = corev1.DNSDefault
	case req.DNSFrom == DNSFromExplicit || len(req.DNSNameservers) > 0:
		pod.Spec.DNSPolicy = corev1.DNSNone
		pod.Spec.DNSConfig = &corev1.PodDNSConfig{Nameservers: append([]string(nil), req.DNSNameservers...)}
	}
	if len(req.AvoidPodLabels) > 0 {
		pod.Spec.Affinity = antiAffinity(req.AvoidPodLabels, req.TopologyKey)
	}
	return pod, nil
}

func args(req Request) ([]string, error) {
	switch req.Mode {
	case ModeDNS:
		if req.Target == "" {
			return nil, errors.New("dns probe target is required")
		}
		// Each optional flag is emitted only when it differs from the probe's
		// own default, so a caller that shapes no assertion produces exactly
		// the argv it always did.
		out := []string{"-mode", string(req.Mode), "-target", req.Target}
		if req.RecordType != "" {
			out = append(out, "-record-type", req.RecordType)
		}
		if len(req.ExpectedAnswers) > 0 {
			out = append(out, "-expect-answers", strings.Join(req.ExpectedAnswers, ","))
		}
		if req.Absent {
			out = append(out, "-absent")
		}
		return out, nil
	case ModeTCPConnect:
		if req.Target == "" {
			return nil, errors.New("tcp-connect probe target is required")
		}
		if req.Port <= 0 {
			return nil, errors.New("tcp-connect probe port is required")
		}
		return []string{"-mode", string(req.Mode), "-target", req.Target, "-port", strconv.Itoa(req.Port)}, nil
	case ModeTCPListen:
		if req.Port <= 0 {
			return nil, errors.New("tcp-listen probe port is required")
		}
		return []string{"-mode", string(req.Mode), "-port", strconv.Itoa(req.Port)}, nil
	case ModeHTTPGet:
		if req.Target == "" {
			return nil, errors.New("http-get probe target is required")
		}
		out := []string{"-mode", string(req.Mode), "-target", req.Target}
		if len(req.Expect) > 0 {
			out = append(out, "-expect", strings.Join(req.Expect, ","))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported probe mode %q", req.Mode)
	}
}

func antiAffinity(labels map[string]string, topologyKey string) *corev1.Affinity {
	if topologyKey == "" {
		topologyKey = corev1.LabelHostname
	}
	return &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
		LabelSelector: &metav1.LabelSelector{MatchLabels: copyStringMap(labels)},
		TopologyKey:   topologyKey,
	}}}}
}

func ParseResult(message string) (Result, error) {
	var result Result
	if err := json.Unmarshal([]byte(message), &result); err != nil {
		return Result{}, fmt.Errorf("parse probe result: %w", err)
	}
	if result.Outcome == "" {
		return Result{}, errors.New("probe result outcome is empty")
	}
	return result, nil
}

func boolPtr(value bool) *bool { return &value }

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
