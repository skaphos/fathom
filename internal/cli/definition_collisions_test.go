/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"fmt"
	"strings"
	"testing"

	api "github.com/skaphos/fathom/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDefinitionCollisions(t *testing.T) {
	for _, tc := range []struct {
		name, version, build string
		inventory            []string
		readFailure          bool
		code                 int
	}{
		{name: "clear", version: "v1.0.0", build: "abc", inventory: []string{"other"}},
		{name: "collision", version: "v1.0.0", build: "abc", inventory: []string{"custom"}, code: 1},
		{name: "unknown build", version: "v1.0.0", inventory: []string{"custom"}, code: 2},
		{name: "development", version: "devel+abc", build: "abc", inventory: []string{"custom"}, code: 2},
		{name: "missing inventory", version: "v1.0.0", build: "abc", code: 2},
		{name: "read failure", version: "v1.0.0", build: "abc", inventory: []string{"custom"}, readFailure: true, code: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := fakeFactory(t, &api.AddonDefinition{ObjectMeta: metav1.ObjectMeta{Name: "custom"}, Spec: api.AddonDefinitionSpec{AddonType: "custom"}})
			f.definitionInventory = func() []string { return tc.inventory }
			f.definitionReleaseInfo = func() (string, string) { return tc.version, tc.build }
			if tc.readFailure {
				f.newClient = func(*rest.Config, client.Options) (client.Client, error) { return nil, fmt.Errorf("denied") }
			}
			out, _, err := execVerb(f, "definition", "collisions")
			if ExitCode(err) != tc.code {
				t.Fatalf("exit=%d want=%d err=%v", ExitCode(err), tc.code, err)
			}
			if !strings.Contains(out, "version="+tc.version) || !strings.Contains(out, "build="+tc.build) {
				t.Fatal("missing binary provenance")
			}
		})
	}
	f, _ := fakeFactory(t)
	if _, _, err := execVerb(f, "definition", "collisions", "--inventory-file", "anything"); err == nil {
		t.Fatal("external inventory accepted")
	}
}
