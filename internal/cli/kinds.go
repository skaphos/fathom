/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	fathomv1alpha1 "github.com/skaphos/fathom/api/v1alpha1"
)

// kindDescriptor is the only place kind-specific knowledge lives. Every verb
// dispatches through it, so adding a kind is one table row plus its typed
// accessors rather than a new branch in each verb.
type kindDescriptor struct {
	// Kind is the CRD Kind (e.g. "DNSCheck"); Resource is the plural resource
	// name (e.g. "dnschecks"). Aliases are CLI-only short forms: the CRDs
	// declare no shortNames, so kubectl does not recognise them.
	Kind     string
	Resource string
	Aliases  []string

	// Namespaced is false only for ClusterHealth.
	Namespaced bool
	// Executable kinds perform their own evaluation and therefore honour the
	// on-demand run trigger. Derived kinds (HealthCheck, ClusterHealth) only
	// mirror or aggregate.
	Executable bool
	// WritesReports mirrors Executable today: only kinds that run write
	// HealthReport history.
	WritesReports bool

	New     func() client.Object
	NewList func() client.ObjectList
	// Items unpacks a typed list into client.Objects for kind-agnostic code.
	Items func(client.ObjectList) []client.Object
	// Paused reads spec.paused. Kinds without the field report false.
	Paused func(client.Object) bool
	// Snapshot is the kind's verdict normalisation (see snapshot.go).
	Snapshot func(client.Object) snapshot
	// DefaultTimeout is the effective spec.timeout the controller would apply,
	// used to size `run --wait`. Zero for derived kinds, which never run.
	DefaultTimeout func(client.Object) time.Duration
	// Sources resolves the executable checks behind a derived kind (see
	// run.go). Nil for executable kinds, which are their own source.
	Sources func(context.Context, client.Client, client.Object) ([]sourceResolution, error)
}

// sourceResolution is one executable check reached through a derived kind,
// or the reason it could not be reached (Skip non-empty). Via names the
// HealthCheck the source was found through, for the user's benefit.
type sourceResolution struct {
	Ref  checkRef
	Via  string
	Skip string
}

var kinds = []*kindDescriptor{
	{
		Kind: "AddonCheck", Resource: "addonchecks", Aliases: []string{"ac"},
		Namespaced: true, Executable: true, WritesReports: true,
		New:     func() client.Object { return &fathomv1alpha1.AddonCheck{} },
		NewList: func() client.ObjectList { return &fathomv1alpha1.AddonCheckList{} },
		Items: func(l client.ObjectList) []client.Object {
			list := l.(*fathomv1alpha1.AddonCheckList)
			out := make([]client.Object, 0, len(list.Items))
			for i := range list.Items {
				out = append(out, &list.Items[i])
			}
			return out
		},
		Paused:         func(o client.Object) bool { return o.(*fathomv1alpha1.AddonCheck).Spec.Paused },
		Snapshot:       addonCheckSnapshot,
		DefaultTimeout: addonCheckTimeout,
	},
	{
		Kind: "DNSCheck", Resource: "dnschecks", Aliases: []string{"dns"},
		Namespaced: true, Executable: true, WritesReports: true,
		New:     func() client.Object { return &fathomv1alpha1.DNSCheck{} },
		NewList: func() client.ObjectList { return &fathomv1alpha1.DNSCheckList{} },
		Items: func(l client.ObjectList) []client.Object {
			list := l.(*fathomv1alpha1.DNSCheckList)
			out := make([]client.Object, 0, len(list.Items))
			for i := range list.Items {
				out = append(out, &list.Items[i])
			}
			return out
		},
		Paused:         func(client.Object) bool { return false },
		Snapshot:       dnsCheckSnapshot,
		DefaultTimeout: dnsCheckTimeout,
	},
	{
		Kind: "NodeCertificateCheck", Resource: "nodecertificatechecks", Aliases: []string{"ncc"},
		Namespaced: true, Executable: true, WritesReports: true,
		New:     func() client.Object { return &fathomv1alpha1.NodeCertificateCheck{} },
		NewList: func() client.ObjectList { return &fathomv1alpha1.NodeCertificateCheckList{} },
		Items: func(l client.ObjectList) []client.Object {
			list := l.(*fathomv1alpha1.NodeCertificateCheckList)
			out := make([]client.Object, 0, len(list.Items))
			for i := range list.Items {
				out = append(out, &list.Items[i])
			}
			return out
		},
		Paused:         func(o client.Object) bool { return o.(*fathomv1alpha1.NodeCertificateCheck).Spec.Paused },
		Snapshot:       nodeCertificateCheckSnapshot,
		DefaultTimeout: nodeCertificateCheckTimeout,
	},
	{
		Kind: "NodeHealthCheck", Resource: "nodehealthchecks", Aliases: []string{"nhc"},
		Namespaced: true, Executable: true, WritesReports: true,
		New:     func() client.Object { return &fathomv1alpha1.NodeHealthCheck{} },
		NewList: func() client.ObjectList { return &fathomv1alpha1.NodeHealthCheckList{} },
		Items: func(l client.ObjectList) []client.Object {
			list := l.(*fathomv1alpha1.NodeHealthCheckList)
			out := make([]client.Object, 0, len(list.Items))
			for i := range list.Items {
				out = append(out, &list.Items[i])
			}
			return out
		},
		Paused:         func(client.Object) bool { return false },
		Snapshot:       nodeHealthCheckSnapshot,
		DefaultTimeout: nodeHealthCheckTimeout,
	},
	{
		Kind: "HealthCheck", Resource: "healthchecks", Aliases: []string{"hc"},
		Namespaced: true,
		New:        func() client.Object { return &fathomv1alpha1.HealthCheck{} },
		NewList:    func() client.ObjectList { return &fathomv1alpha1.HealthCheckList{} },
		Items: func(l client.ObjectList) []client.Object {
			list := l.(*fathomv1alpha1.HealthCheckList)
			out := make([]client.Object, 0, len(list.Items))
			for i := range list.Items {
				out = append(out, &list.Items[i])
			}
			return out
		},
		Paused:         func(o client.Object) bool { return o.(*fathomv1alpha1.HealthCheck).Spec.Paused },
		Snapshot:       healthCheckSnapshot,
		DefaultTimeout: noTimeout,
		// Sources is wired in run.go's init: the resolvers look kinds up by
		// name, which would be an initialization cycle here.
	},
	{
		Kind: "ClusterHealth", Resource: "clusterhealths", Aliases: []string{"ch"},
		New:     func() client.Object { return &fathomv1alpha1.ClusterHealth{} },
		NewList: func() client.ObjectList { return &fathomv1alpha1.ClusterHealthList{} },
		Items: func(l client.ObjectList) []client.Object {
			list := l.(*fathomv1alpha1.ClusterHealthList)
			out := make([]client.Object, 0, len(list.Items))
			for i := range list.Items {
				out = append(out, &list.Items[i])
			}
			return out
		},
		Paused:         func(client.Object) bool { return false },
		Snapshot:       clusterHealthSnapshot,
		DefaultTimeout: noTimeout,
	},
}

// executableKinds returns the descriptors that honour the run trigger, in
// table order.
func executableKinds() []*kindDescriptor {
	var out []*kindDescriptor
	for _, k := range kinds {
		if k.Executable {
			out = append(out, k)
		}
	}
	return out
}

// kindByName returns the descriptor for an exact Kind ("DNSCheck"), or nil.
func kindByName(kind string) *kindDescriptor {
	for _, k := range kinds {
		if k.Kind == kind {
			return k
		}
	}
	return nil
}

// parseKind resolves user input to a descriptor. Matching is case-insensitive
// over the Kind, the plural resource name, and the CLI aliases.
func parseKind(s string) (*kindDescriptor, error) {
	needle := strings.ToLower(strings.TrimSpace(s))
	if needle == "" {
		return nil, errors.New("kind must not be empty")
	}
	for _, k := range kinds {
		if needle == strings.ToLower(k.Kind) || needle == k.Resource {
			return k, nil
		}
		for _, a := range k.Aliases {
			if needle == a {
				return k, nil
			}
		}
	}
	return nil, fmt.Errorf("unknown kind %q (want one of %s)", s, kindNamesForHelp())
}

// kindNamesForHelp lists the accepted spellings for error messages.
func kindNamesForHelp() string {
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%s/%s/%s", k.Resource, strings.ToLower(k.Kind), strings.Join(k.Aliases, "/")))
	}
	return strings.Join(parts, ", ")
}

// checkRef is a parsed target: which kind, which namespace (empty for
// cluster-scoped kinds), which name.
type checkRef struct {
	Kind      *kindDescriptor
	Namespace string
	Name      string
}

func (r checkRef) String() string {
	if r.Namespace == "" {
		return strings.ToLower(r.Kind.Kind) + "/" + r.Name
	}
	return strings.ToLower(r.Kind.Kind) + "/" + r.Namespace + "/" + r.Name
}

// parseTarget accepts `<kind>/<name>`, `<kind>/<namespace>/<name>` (the form
// every verb prints, so output can be pasted back), or `<kind> <name>` from
// the positional arguments and returns the reference plus any arguments left
// over. Without an inline namespace the caller's resolved namespace is used;
// cluster-scoped kinds take none. A namespaced kind with no namespace at all
// (the caller passed -A) is an error rather than a silent lookup in "".
func parseTarget(args []string, namespace string) (checkRef, []string, error) {
	if len(args) == 0 {
		return checkRef{}, nil, errors.New("a check is required: <kind>/<name>, <kind>/<namespace>/<name>, or <kind> <name>")
	}
	var kindArg, name, inlineNS string
	rest := args[1:]
	if strings.Contains(args[0], "/") {
		parts := strings.Split(args[0], "/")
		switch len(parts) {
		case 2:
			kindArg, name = parts[0], parts[1]
		case 3:
			kindArg, inlineNS, name = parts[0], parts[1], parts[2]
		default:
			return checkRef{}, nil, fmt.Errorf("cannot parse %q: use <kind>/<name> or <kind>/<namespace>/<name>", args[0])
		}
	} else {
		if len(args) < 2 {
			return checkRef{}, nil, fmt.Errorf("a name is required after %q: <kind>/<name> or <kind> <name>", args[0])
		}
		kindArg, name = args[0], args[1]
		rest = args[2:]
	}
	k, err := parseKind(kindArg)
	if err != nil {
		return checkRef{}, nil, err
	}
	if name == "" {
		return checkRef{}, nil, fmt.Errorf("a name is required after %q", kindArg)
	}
	ref := checkRef{Kind: k, Name: name}
	switch {
	case !k.Namespaced && inlineNS != "":
		return checkRef{}, nil, fmt.Errorf("%s is cluster-scoped; use %s/%s", k.Kind, strings.ToLower(k.Kind), name)
	case k.Namespaced && inlineNS != "":
		ref.Namespace = inlineNS
	case k.Namespaced:
		if namespace == "" {
			return checkRef{}, nil, fmt.Errorf("%s/%s needs a namespace: pass -n <namespace> or use %s/<namespace>/%s (--all-namespaces only applies to listing)", strings.ToLower(k.Kind), name, strings.ToLower(k.Kind), name)
		}
		ref.Namespace = namespace
	}
	return ref, rest, nil
}
