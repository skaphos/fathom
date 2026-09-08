/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
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

// parseTarget accepts `<kind>/<name>` or `<kind> <name>` from the positional
// arguments and returns the reference plus any arguments left over. The
// namespace is the caller's resolved namespace, dropped for cluster-scoped
// kinds.
func parseTarget(args []string, namespace string) (checkRef, []string, error) {
	if len(args) == 0 {
		return checkRef{}, nil, errors.New("a check is required: <kind>/<name> or <kind> <name>")
	}
	var kindArg, name string
	rest := args[1:]
	if i := strings.IndexByte(args[0], '/'); i >= 0 {
		kindArg, name = args[0][:i], args[0][i+1:]
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
	if k.Namespaced {
		ref.Namespace = namespace
	}
	return ref, rest, nil
}
