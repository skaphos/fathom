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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ModeDNS        Mode = "dns"
	ModeTCPConnect Mode = "tcp-connect"
	ModeTCPListen  Mode = "tcp-listen"

	OutcomePass  Outcome = "Pass"
	OutcomeFail  Outcome = "Fail"
	OutcomeError Outcome = "Error"

	defaultBinaryPath = "/probe"
	defaultTimeout    = 10 * time.Second
	labelManagedBy    = "fathom.skaphos.io/managed-by"
	labelProbeName    = "fathom.skaphos.io/probe"
	managedByValue    = "fathom"
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

	Labels          map[string]string
	NodeSelector    map[string]string
	Tolerations     []corev1.Toleration
	AvoidPodLabels  map[string]string
	TopologyKey     string
	ServiceAccount  string
	ImagePullPolicy corev1.PullPolicy
}

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
	args, err := args(req)
	if err != nil {
		return nil, err
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	labels := map[string]string{labelManagedBy: managedByValue, labelProbeName: req.Name}
	for key, value := range req.Labels {
		labels[key] = value
	}
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
				Name:                     "probe",
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
		return []string{"-mode", string(req.Mode), "-target", req.Target}, nil
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
