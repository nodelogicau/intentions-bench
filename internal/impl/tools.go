package impl

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nodelogicau/intentions-bench/internal/agent"
)

// ReferenceTools is the format's reference tool set (harness-tools), plus
// migrate, which the reference implementation also serves. A tool outside it
// is an extension.
var ReferenceTools = []string{
	"desire_add", "desire_edit", "desire_adopt", "desire_retire", "desire_show", "desire_list",
	"intention_add", "intention_edit", "intention_firm", "intention_retire", "intention_show", "intention_list",
	"availability_add", "availability_renew", "availability_supersede", "availability_retire", "availability_list",
	"commitment_accept", "commitment_decline", "commitment_cancel", "commitment_show", "commitment_list",
	"generate", "resolve", "select", "unresolved", "check", "acknowledge", "bounds", "validate", "workspace_status",
	"migrate",
}

// IsReferenceTool reports whether name is in the reference tool set.
func IsReferenceTool(name string) bool { return slices.Contains(ReferenceTools, name) }

// ToolDef returns an MCP tool's name, description, and input schema
// unchanged, for a provider to render.
func ToolDef(t *mcp.Tool) (agent.ToolDef, error) {
	schema, err := SchemaMap(t)
	if err != nil {
		return agent.ToolDef{}, err
	}
	return agent.ToolDef{Name: t.Name, Description: t.Description, Schema: schema}, nil
}

// SchemaMap returns a tool's input schema as decoded JSON.
func SchemaMap(t *mcp.Tool) (map[string]any, error) {
	b, err := json.Marshal(t.InputSchema)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("tool %s: input schema: %w", t.Name, err)
	}
	if m == nil {
		m = map[string]any{"type": "object"}
	}
	return m, nil
}

// ToolDefs returns every tool's definition.
func ToolDefs(tools []*mcp.Tool) ([]agent.ToolDef, error) {
	out := make([]agent.ToolDef, 0, len(tools))
	for _, t := range tools {
		d, err := ToolDef(t)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ReadOnly reports whether a tool is annotated readOnlyHint: true.
func ReadOnly(t *mcp.Tool) bool { return t.Annotations != nil && t.Annotations.ReadOnlyHint }

// Extensions names the tools outside the reference tool set, sorted.
func Extensions(tools []*mcp.Tool) []string {
	var out []string
	for _, t := range tools {
		if !IsReferenceTool(t.Name) {
			out = append(out, t.Name)
		}
	}
	sort.Strings(out)
	return out
}
