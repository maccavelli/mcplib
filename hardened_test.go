package mcplib

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestInternalWrapHandler_PanicRecovery(t *testing.T) {
	tool := &mcp.Tool{Name: "panic_tool"}
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		panic("deliberate panic")
	}
	wrapped := InternalWrapHandler(tool, handler)
	res, _, err := wrapped(context.Background(), &mcp.CallToolRequest{}, nil)
	if err != nil {
		t.Fatalf("expected nil error from panic recovery; got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected IsError result from panic recovery")
	}
}

func TestInternalWrapHandler_NilSafety(t *testing.T) {
	tool := &mcp.Tool{Name: "nil_content_tool"}
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	}
	wrapped := InternalWrapHandler(tool, handler)
	res, _, _ := wrapped(context.Background(), &mcp.CallToolRequest{}, nil)
	if res.Content == nil {
		t.Fatal("expected Content to be initialized to empty slice")
	}
}

func TestInternalWrapHandler_TelemetryInjection(t *testing.T) {
	os.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	defer os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	tool := &mcp.Tool{Name: "telemetry_tool"}
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil, nil
	}
	wrapped := InternalWrapHandler(tool, handler)
	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{Name: "telemetry_tool"}
	res, _, _ := wrapped(context.Background(), req, nil)
	if len(res.Content) < 2 {
		t.Fatalf("expected telemetry signal appended; got %d content entries", len(res.Content))
	}
}

func TestInternalWrapHandler_NoTelemetryWhenNotOrchestrated(t *testing.T) {
	os.Unsetenv("MCP_ORCHESTRATOR_OWNED")

	tool := &mcp.Tool{Name: "standalone_tool"}
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, nil, nil
	}
	wrapped := InternalWrapHandler(tool, handler)
	res, _, _ := wrapped(context.Background(), &mcp.CallToolRequest{}, nil)
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content entry (no telemetry); got %d", len(res.Content))
	}
}

func TestInternalWrapHandler_WithSerializedCalls(t *testing.T) {
	tool := &mcp.Tool{Name: "serial_tool"}
	called := false
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		called = true
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "done"}}}, nil, nil
	}
	wrapped := InternalWrapHandler(tool, handler, WithSerializedCalls())
	_, _, _ = wrapped(context.Background(), &mcp.CallToolRequest{}, nil)
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestInternalWrapHandler_WithSchemaDerival(t *testing.T) {
	tool := &mcp.Tool{Name: "schema_tool"}
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{}}, nil, nil
	}
	deriveCalled := false
	_ = InternalWrapHandler(tool, handler, WithSchemaDerival(func(t *mcp.Tool) {
		deriveCalled = true
		t.InputSchema = map[string]any{"type": "object"}
	}))
	if !deriveCalled {
		t.Fatal("schema derive function was not called")
	}
	if tool.InputSchema == nil {
		t.Fatal("expected InputSchema to be set")
	}
}

func TestInternalWrapHandler_WithStackTrace(t *testing.T) {
	tool := &mcp.Tool{Name: "stack_tool"}
	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		panic("stack trace test")
	}
	wrapped := InternalWrapHandler(tool, handler, WithStackTrace())
	res, _, err := wrapped(context.Background(), &mcp.CallToolRequest{}, nil)
	if err != nil {
		t.Fatalf("expected nil error from panic recovery; got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected IsError result")
	}
}

// TestInternalWrapHandler_FallbackProbe verifies the divergence probe is
// read-only: when the SDK-decoded input differs from a stdlib re-decode of the
// raw arguments, the handler still receives the SDK value and a warning fires.
func TestInternalWrapHandler_FallbackProbe(t *testing.T) {
	type In struct {
		Value string `json:"value"`
	}
	t.Setenv("MCP_HARDENED_DECODE_PROBE", "true") // opt into the diagnostic probe
	tool := &mcp.Tool{Name: "probe_tool"}
	var received In
	handler := func(_ context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		received = in
		return &mcp.CallToolResult{Content: []mcp.Content{}}, nil, nil
	}
	wrapped := InternalWrapHandler(tool, handler)

	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{
		Name:      "probe_tool",
		Arguments: json.RawMessage(`{"value":"from_json"}`),
	}

	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	// Simulate the SDK having decoded a value different from the raw arguments.
	sdkInput := In{Value: "from_sdk"}
	if _, _, err := wrapped(context.Background(), req, sdkInput); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Value != "from_sdk" {
		t.Errorf("probe must not overwrite SDK-decoded input: got %q, want %q", received.Value, "from_sdk")
	}
	if !strings.Contains(logBuf.String(), "diverged") {
		t.Errorf("expected divergence warning to fire; log was: %s", logBuf.String())
	}
}

// TestInternalWrapHandler_ProbeDisabledByDefault verifies the probe is off when
// the env flag is unset: no divergence warning fires, and the handler still
// receives the SDK-decoded value (no overwrite).
func TestInternalWrapHandler_ProbeDisabledByDefault(t *testing.T) {
	t.Setenv("MCP_HARDENED_DECODE_PROBE", "") // explicitly disabled
	type In struct {
		Value string `json:"value"`
	}
	tool := &mcp.Tool{Name: "probe_off_tool"}
	var received In
	handler := func(_ context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		received = in
		return &mcp.CallToolResult{Content: []mcp.Content{}}, nil, nil
	}
	wrapped := InternalWrapHandler(tool, handler)

	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{Name: "probe_off_tool", Arguments: json.RawMessage(`{"value":"from_json"}`)}

	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	if _, _, err := wrapped(context.Background(), req, In{Value: "from_sdk"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received.Value != "from_sdk" {
		t.Errorf("handler must receive SDK value: got %q", received.Value)
	}
	if strings.Contains(logBuf.String(), "diverged") {
		t.Errorf("probe must not fire when disabled; log: %s", logBuf.String())
	}
}

func TestHardenedAddSimpleTool_Success(t *testing.T) {
	type TestIn struct {
		Name string `json:"name"`
	}
	type TestOut struct {
		Greeting string `json:"greeting"`
	}

	s := mcp.NewServer(&mcp.Implementation{Name: "test-server"}, &mcp.ServerOptions{})
	tool := &mcp.Tool{
		Name:        "greet",
		Description: "Test tool",
		InputSchema: map[string]any{"type": "object"},
	}
	HardenedAddSimpleTool(s, tool, func(_ context.Context, in TestIn) (TestOut, error) {
		return TestOut{Greeting: "Hello, " + in.Name}, nil
	})
	// Tool was registered without panic — success.
}

func TestHardenedAddSimpleTool_Error(t *testing.T) {
	type TestIn struct{}
	type TestOut struct{}

	s := mcp.NewServer(&mcp.Implementation{Name: "test-server"}, &mcp.ServerOptions{})
	tool := &mcp.Tool{
		Name:        "fail_tool",
		Description: "Test tool that fails",
		InputSchema: map[string]any{"type": "object"},
	}
	HardenedAddSimpleTool(s, tool, func(_ context.Context, _ TestIn) (TestOut, error) {
		return TestOut{}, fmt.Errorf("handler failed")
	})
	// Tool was registered without panic — success.
}
