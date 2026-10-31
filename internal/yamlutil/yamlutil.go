// Package yamlutil decodes YAML into plain JSON-shaped values, keeping
// timestamps as the text that was written: a calendar anchor such as
// 2026-10-14 stays a string rather than becoming a time.
package yamlutil

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Unmarshal decodes b into a plain value.
func Unmarshal(b []byte) (any, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	if n.Kind == 0 {
		return nil, nil
	}
	return Plain(&n)
}

// Plain converts a node to maps, slices, and scalars. Timestamp scalars are
// kept as strings.
func Plain(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return Plain(n.Content[0])
	case yaml.AliasNode:
		return Plain(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			v, err := Plain(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[k] = v
		}
		return m, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := Plain(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.ScalarNode:
		if n.Tag == "!!timestamp" {
			return n.Value, nil
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, fmt.Errorf("line %d: %w", n.Line, err)
		}
		return v, nil
	}
	return nil, fmt.Errorf("line %d: unexpected YAML node", n.Line)
}
