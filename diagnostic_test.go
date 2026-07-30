package mcplib

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDiagnosticHandlerFunc(t *testing.T) {
	buffer := NewLogBuffer(WithBufferSize(2*1024*1024, 1024*1024))

	handleFunc := DiagnosticHandlerFunc(buffer)

	callReq := func(input DiagnosticInput) *mcp.CallToolResult {
		res, _, err := handleFunc(context.Background(), &mcp.CallToolRequest{}, input)
		if err != nil {
			t.Fatalf("Handle failed: %v", err)
		}
		return res
	}

	t.Run("empty_buffer", func(t *testing.T) {
		res := callReq(DiagnosticInput{MaxLines: 10})
		if !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "[NO LOGS AVAILABLE]") {
			t.Errorf("Expected NO LOGS AVAILABLE, got %v", res.Content[0])
		}
	})

	t.Run("max_lines_clamping", func(t *testing.T) {
		buffer.Write([]byte("line 1\nline 2\n"))

		// 0 defaults to 25
		res := callReq(DiagnosticInput{MaxLines: 0})
		text := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "line 1") || !strings.Contains(text, "line 2") {
			t.Errorf("Expected lines 1 and 2, got %v", text)
		}

		// Negative defaults to 25
		res = callReq(DiagnosticInput{MaxLines: -10})
		if !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "line 1") {
			t.Errorf("Expected line 1 for negative max lines")
		}

		// Excessive capped to 1000
		res = callReq(DiagnosticInput{MaxLines: 5000})
		if !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "line 1") {
			t.Errorf("Expected line 1 for excessive max lines")
		}
	})

	t.Run("session_id_isolation", func(t *testing.T) {
		buffer.Write([]byte("session:123 - error 1\n"))
		buffer.Write([]byte("session:456 - normal\n"))
		buffer.Write([]byte("session:123 - error 2\n"))

		res := callReq(DiagnosticInput{MaxLines: 10, SessionID: "session:123"})
		text := res.Content[0].(*mcp.TextContent).Text
		if strings.Contains(text, "session:456") {
			t.Errorf("Expected logs to be filtered, but saw session:456. Output: %v", text)
		}
		if !strings.Contains(text, "error 1") || !strings.Contains(text, "error 2") {
			t.Errorf("Expected both error 1 and error 2, got %v", text)
		}
	})

	t.Run("session_id_no_match", func(t *testing.T) {
		res := callReq(DiagnosticInput{MaxLines: 10, SessionID: "session:999"})
		text := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "[NO LOGS AVAILABLE FOR SESSION]") {
			t.Errorf("Expected NO LOGS AVAILABLE FOR SESSION, got %v", text)
		}
	})
}

func TestResolveDiagnosticConfig(t *testing.T) {
	def := resolveDiagnosticConfig()
	if def.defaultMaxLines != 25 || def.maxCap != 1000 {
		t.Errorf("defaults: got (%d,%d), want (25,1000)", def.defaultMaxLines, def.maxCap)
	}
	override := resolveDiagnosticConfig(WithDefaultMaxLines(50), WithMaxLineCap(200))
	if override.defaultMaxLines != 50 || override.maxCap != 200 {
		t.Errorf("override: got (%d,%d), want (50,200)", override.defaultMaxLines, override.maxCap)
	}
	// Non-positive values are ignored and fall back to the defaults.
	bad := resolveDiagnosticConfig(WithDefaultMaxLines(-5), WithMaxLineCap(0))
	if bad.defaultMaxLines != 25 || bad.maxCap != 1000 {
		t.Errorf("invalid opts should fall back: got (%d,%d), want (25,1000)", bad.defaultMaxLines, bad.maxCap)
	}
}

func TestDiagnosticHandler_ConfigurableLimits(t *testing.T) {
	buffer := NewLogBuffer()
	buffer.Write([]byte("l1\nl2\nl3\nl4\nl5\n"))

	call := func(h func(context.Context, *mcp.CallToolRequest, DiagnosticInput) (*mcp.CallToolResult, any, error), in DiagnosticInput) string {
		res, _, err := h(context.Background(), &mcp.CallToolRequest{}, in)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		return res.Content[0].(*mcp.TextContent).Text
	}

	// Custom default of 2: omitting max_lines returns the last 2 lines only.
	h2 := diagnosticHandler(buffer, resolveDiagnosticConfig(WithDefaultMaxLines(2)))
	text := call(h2, DiagnosticInput{MaxLines: 0})
	if !strings.Contains(text, "l5") || !strings.Contains(text, "l4") {
		t.Errorf("expected last 2 lines, got %q", text)
	}
	if strings.Contains(text, "l1") {
		t.Errorf("default of 2 should exclude l1, got %q", text)
	}

	// Custom cap of 1: a large max_lines is clamped to the last line only.
	h1 := diagnosticHandler(buffer, resolveDiagnosticConfig(WithMaxLineCap(1)))
	text = call(h1, DiagnosticInput{MaxLines: 100})
	if !strings.Contains(text, "l5") || strings.Contains(text, "l4") {
		t.Errorf("cap of 1 should return only l5, got %q", text)
	}
}
