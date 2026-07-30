package schema

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sample struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func TestDerive_Basic(t *testing.T) {
	tool := &mcp.Tool{}
	Derive[sample]()(tool)
	m, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema not a map: %T", tool.InputSchema)
	}
	if m["type"] != "object" {
		t.Errorf("expected object schema, got %v", m["type"])
	}
	props, ok := m["properties"].(map[string]any)
	if !ok || props["name"] == nil {
		t.Errorf("expected 'name' property, got %v", m["properties"])
	}
}

func TestDerive_Namespaces(t *testing.T) {
	tool := &mcp.Tool{}
	Derive[sample]("alpha", "beta")(tool)
	m := tool.InputSchema.(map[string]any)
	props := m["properties"].(map[string]any)
	ns, ok := props["namespace"].(map[string]any)
	if !ok {
		t.Fatalf("namespace property missing: %v", props)
	}
	enum, ok := ns["enum"].([]any)
	if !ok || len(enum) != 2 || enum[0] != "alpha" || enum[1] != "beta" {
		t.Errorf("namespace enum not injected: %v", ns["enum"])
	}
}
