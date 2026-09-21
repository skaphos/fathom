/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package declarative

import (
	"fmt"
	"io"
	"strings"

	limits "github.com/skaphos/fathom/pkg/addondefinition"
	yaml "go.yaml.in/yaml/v3"
)

func (ec EvalContext) inputFailure(detail string) error {
	if b := ec.budget(); b != nil {
		return b.Fail("InputLimitExceeded", detail)
	}
	return fmt.Errorf("InputLimitExceeded: %s", detail)
}

// runtimeYAML checks the syntax tree before conversion can expand aliases.
// The byte cap also bounds parser allocation before the node/depth walk. Syntax
// errors remain the check's configured invalid verdict; budget failures abort.
func (ec EvalContext) runtimeYAML(value string) (syntaxErr, errorLimit error) {
	if len(value) > limits.MaxYAMLBytes {
		return nil, ec.inputFailure("ConfigMap value exceeds byte limit")
	}
	if err := ec.Ctx.Err(); err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(value))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil && err != io.EOF {
		return err, nil
	}
	nodes := 0
	var walk func(*yaml.Node, int) error
	walk = func(node *yaml.Node, depth int) error {
		if err := ec.Ctx.Err(); err != nil {
			return err
		}
		nodes++
		if nodes > limits.MaxYAMLNodes || depth > limits.MaxYAMLDepth || node.Kind == yaml.AliasNode {
			return ec.inputFailure("ConfigMap YAML exceeds node/depth limit or contains aliases")
		}
		if b := ec.budget(); b != nil {
			if err := b.Visit(1); err != nil {
				return err
			}
		}
		for _, child := range node.Content {
			if err := walk(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(&root, 0); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err, nil
		}
		return fmt.Errorf("multiple YAML documents are not supported"), nil
	}
	return nil, nil
}
