// Package schema provides shared JSON-schema derivation for MCP tool inputs.
//
// It is a separate subpackage so the invopop/jsonschema dependency stays opt-in
// and out of the mcplib root package: servers that derive schemas import
// mcplib/schema; everyone else (and mcplib root) never pulls jsonschema.
package schema

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	schemaTypeObject         = "object"
	schemaKeyType            = "type"
	schemaKeyAdditionalProps = "additionalProperties"
)

// Derive returns a callback (for mcplib.WithSchemaDerival) that reflects In into
// a JSON schema and assigns it to the tool's InputSchema. If reflection panics
// it falls back to a permissive open-object schema.
//
// When namespaces are provided, an enum constraint is injected onto a top-level
// "namespace" property — used by recall to scope a tool to known namespaces.
func Derive[In any](namespaces ...string) func(*mcp.Tool) {
	return func(t *mcp.Tool) {
		defer func() {
			if r := recover(); r != nil {
				t.InputSchema = map[string]any{
					schemaKeyType:            schemaTypeObject,
					schemaKeyAdditionalProps: true,
				}
			}
		}()
		r := new(jsonschema.Reflector)
		r.ExpandedStruct = true
		sch := r.Reflect(new(In))
		b, err := json.Marshal(sch)
		if err != nil {
			t.InputSchema = map[string]any{
				schemaKeyType:            schemaTypeObject,
				schemaKeyAdditionalProps: true,
			}
			return
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.InputSchema = map[string]any{
				schemaKeyType:            schemaTypeObject,
				schemaKeyAdditionalProps: true,
			}
			return
		}

		if len(namespaces) > 0 {
			if props, ok := m["properties"].(map[string]any); ok {
				if ns, exists := props["namespace"].(map[string]any); exists {
					enumList := make([]any, 0, len(namespaces))
					for _, n := range namespaces {
						enumList = append(enumList, n)
					}
					ns["enum"] = enumList
				}
			}
		}

		t.InputSchema = m
	}
}
