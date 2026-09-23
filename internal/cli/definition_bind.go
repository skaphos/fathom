/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

func newDefinitionBindCommand(f *factory) *cobra.Command {
	var file, name, account, namespace string
	cmd := &cobra.Command{Use: "bind", Short: "Read live UIDs and render a disabled binding; never apply it", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		reviewed, err := readDefinition(file)
		if err != nil {
			return err
		}
		if reviewed.Name != name {
			return fmt.Errorf("--name must match the reviewed definition")
		}
		inventory := f.definitionInventory()
		if len(inventory) == 0 {
			return fmt.Errorf("built-in inventory unavailable")
		}
		for _, builtin := range inventory {
			if name == builtin {
				return fmt.Errorf("definition collides with built-in %s", name)
			}
		}
		scope, err := definitions.TargetScope(reviewed)
		if err != nil {
			return err
		}
		c, err := f.client()
		if err != nil {
			return err
		}
		var live api.AddonDefinition
		if err := c.Get(commandContext(cmd), types.NamespacedName{Name: name}, &live); err != nil {
			return err
		}
		// Exact comparison is deliberately conservative: admission defaults must be
		// represented in the reviewed file, rather than silently approving a change.
		wanted, _ := json.Marshal(reviewed.Spec)
		actual, _ := json.Marshal(live.Spec)
		if !bytes.Equal(wanted, actual) {
			return fmt.Errorf("live definition differs from reviewed spec (including admission defaults); review the live spec before binding")
		}
		var sa corev1.ServiceAccount
		if err := c.Get(commandContext(cmd), types.NamespacedName{Namespace: namespace, Name: account}, &sa); err != nil {
			return err
		}
		if !live.DeletionTimestamp.IsZero() || !sa.DeletionTimestamp.IsZero() {
			return fmt.Errorf("definition or service account is being deleted")
		}
		binding := &api.AddonDefinitionBinding{TypeMeta: metav1.TypeMeta{APIVersion: api.GroupVersion.String(), Kind: "AddonDefinitionBinding"}, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Spec: api.AddonDefinitionBindingSpec{
			DefinitionRef: api.DefinitionReference{Name: api.DefinitionDNSLabel(name), UID: string(live.UID)}, ServiceAccountRef: api.DefinitionObjectReference{Name: api.DefinitionResourceName(account), UID: string(sa.UID)}, TargetScope: scope,
		}}
		if err := definitions.ValidateBinding(binding); err != nil {
			return err
		}
		data, err := yaml.Marshal(binding)
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}}
	cmd.Flags().StringVar(&file, "file", "", "Reviewed definition, including any admission defaults")
	cmd.Flags().StringVar(&name, "name", "", "Definition name")
	cmd.Flags().StringVar(&account, "service-account", "", "Existing dedicated service account")
	cmd.Flags().StringVar(&namespace, "operator-namespace", "", "Operator and service account namespace")
	for _, flag := range []string{"file", "name", "service-account", "operator-namespace"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}
