package mcplib

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClassifySuccess_DefaultSuccess(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "everything is fine"},
		},
	}
	ok, conf := ClassifySuccess(res)
	if !ok || conf != 1.0 {
		t.Fatalf("expected success=true, confidence=1.0; got %v, %v", ok, conf)
	}
}

func TestClassifySuccess_NoMatchesFound(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "No matches found in the workspace"},
		},
	}
	ok, conf := ClassifySuccess(res)
	if ok || conf != 0.5 {
		t.Fatalf("expected success=false, confidence=0.5; got %v, %v", ok, conf)
	}
}

func TestClassifySuccess_TargetNotIdentified(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Target not identified in AST"},
		},
	}
	ok, conf := ClassifySuccess(res)
	if ok || conf != 0.5 {
		t.Fatalf("expected success=false, confidence=0.5; got %v, %v", ok, conf)
	}
}

func TestClassifySuccess_CouldNotFind(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Could not find the specified file"},
		},
	}
	ok, conf := ClassifySuccess(res)
	if ok || conf != 0.5 {
		t.Fatalf("expected success=false, confidence=0.5; got %v, %v", ok, conf)
	}
}

func TestClassifySuccess_EmptyContent(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{},
	}
	ok, conf := ClassifySuccess(res)
	if !ok || conf != 1.0 {
		t.Fatalf("expected success=true, confidence=1.0 for empty content; got %v, %v", ok, conf)
	}
}

func TestInjectSignal(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "result data"},
		},
	}
	InjectSignal(res, OrchestratorSignal{
		Success:       true,
		Confidence:    1.0,
		IntentContext: "test_tool",
		Category:      "test_category",
	})
	if len(res.Content) != 2 {
		t.Fatalf("expected 2 content entries; got %d", len(res.Content))
	}
	tc, ok := res.Content[1].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected second content to be TextContent")
	}
	if tc.Text == "" {
		t.Fatal("expected non-empty signal text")
	}
}
