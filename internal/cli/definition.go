/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	api "github.com/skaphos/fathom/api/v1alpha1"
	definitions "github.com/skaphos/fathom/pkg/addondefinition"
	"github.com/spf13/cobra"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

func newDefinitionCommand(f *factory) *cobra.Command {
	cmd := &cobra.Command{Use: "definition", Short: "Prepare and inspect runtime addon definitions"}
	cmd.AddCommand(newDefinitionRenderCommand(f), newDefinitionCollisionsCommand(f), newDefinitionBindCommand(f), newDefinitionDrainCommand(f))
	return cmd
}

func newDefinitionRenderCommand(f *factory) *cobra.Command {
	var file string
	var opts definitions.RenderOptions
	cmd := &cobra.Command{Use: "render", Short: "Render staged manifests offline; never apply resources", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		d, err := readDefinition(file)
		if err != nil {
			return err
		}
		opts.BuiltinNames = f.definitionInventory()
		data, _, err := definitions.Render(d, opts)
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}}
	cmd.Flags().StringVar(&file, "file", "", "Single AddonDefinition YAML or JSON file")
	cmd.Flags().StringVar(&opts.ServiceAccount, "service-account", "", "Dedicated reader service account")
	cmd.Flags().StringVar(&opts.OperatorNamespace, "operator-namespace", "", "Operator and reader service account namespace")
	cmd.Flags().StringVar(&opts.OperatorServiceAccount, "operator-service-account", "", "Actual operator service account receiving impersonation permission")
	for _, flag := range []string{"file", "service-account", "operator-namespace", "operator-service-account"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}

func readDefinition(path string) (*api.AddonDefinition, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	// Bound file bytes before parsing. The tighter canonical spec bound is
	// enforced independently by semantic validation after strict decoding.
	const maxFile = 1 << 20
	data, err := io.ReadAll(io.LimitReader(file, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFile {
		return nil, fmt.Errorf("definition file exceeds 1 MiB")
	}
	documents := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	first, err := documents.Read()
	if err != nil {
		return nil, err
	}
	if _, err := documents.Read(); err != io.EOF {
		return nil, fmt.Errorf("file must contain exactly one definition document")
	}
	raw, err := yaml.YAMLToJSONStrict(first)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var definition api.AddonDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, err
	}
	if definition.APIVersion != api.GroupVersion.String() || definition.Kind != "AddonDefinition" || definition.Namespace != "" {
		return nil, fmt.Errorf("file must contain a cluster-scoped %s AddonDefinition", api.GroupVersion.String())
	}
	if err := definitions.Validate(&definition); err != nil {
		return nil, err
	}
	return &definition, nil
}
