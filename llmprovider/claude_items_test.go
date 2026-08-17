package llmprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClaude_GenerateItems_TextBlock verifies single text block decoding.
func TestClaude_GenerateItems_TextBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hello from claude"}]}`))
	}))
	defer srv.Close()

	p, _ := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: "user", Text: "hi"})
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	m, ok := resp.Output[0].(MessageItem)
	if !ok || m.Text != "hello from claude" {
		t.Errorf("expected MessageItem with 'hello from claude', got %v", resp.Output[0])
	}
	if resp.OutputText() != "hello from claude" {
		t.Errorf("OutputText() = %q, want %q", resp.OutputText(), "hello from claude")
	}
	if resp.ID != "" {
		t.Errorf("Response.ID must be empty for Claude, got %q", resp.ID)
	}
}

// TestClaude_GenerateItems_MultiBlock verifies thinking + text blocks are decoded
// into ReasoningItem and MessageItem, and OutputText() returns only text.
func TestClaude_GenerateItems_MultiBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":[
			{"type":"thinking","thinking":"deep thought process"},
			{"type":"text","text":"the result"}
		]}`))
	}))
	defer srv.Close()

	p, _ := NewClaude("k", "claude-sonnet-5", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: "user", Text: "solve"})
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if len(resp.Output) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Output))
	}
	r, ok := resp.Output[0].(ReasoningItem)
	if !ok || r.Text != "deep thought process" {
		t.Errorf("output[0] should be ReasoningItem with 'deep thought process', got %v", resp.Output[0])
	}
	m, ok := resp.Output[1].(MessageItem)
	if !ok || m.Text != "the result" {
		t.Errorf("output[1] should be MessageItem with 'the result', got %v", resp.Output[1])
	}
	if resp.OutputText() != "the result" {
		t.Errorf("OutputText() = %q, want %q", resp.OutputText(), "the result")
	}
}

// TestClaude_GenerateItems_ToolUse verifies tool_use block decoding to FunctionCallItem.
func TestClaude_GenerateItems_ToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":[
			{"type":"tool_use","id":"tool_call_99","name":"search","input":{"q":"golang"}}
		]}`))
	}))
	defer srv.Close()

	p, _ := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL))
	tool := Tool{Name: "search", Description: "Search", Schema: map[string]any{"type": "object"}}
	resp, err := p.GenerateItemsWithTool(context.Background(), tool, MessageItem{Role: "user", Text: "search"})
	if err != nil {
		t.Fatalf("GenerateItemsWithTool: %v", err)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Output))
	}
	fc, ok := resp.Output[0].(FunctionCallItem)
	if !ok || fc.CallID != "tool_call_99" || fc.Name != "search" || fc.Arguments != `{"q":"golang"}` {
		t.Errorf("FunctionCallItem = %+v", resp.Output[0])
	}

	// Also verify wrapper
	args, err := p.GenerateWithTool(context.Background(), "search", tool)
	if err != nil {
		t.Fatalf("GenerateWithTool: %v", err)
	}
	if args != `{"q":"golang"}` {
		t.Errorf("GenerateWithTool = %q, want %q", args, `{"q":"golang"}`)
	}
}

// TestClaude_ResponseID_AlwaysEmpty confirms Claude does not set response ID.
func TestClaude_ResponseID_AlwaysEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	p, _ := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: "user", Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "" {
		t.Errorf("Claude Response.ID should be empty, got %q", resp.ID)
	}
}

// TestClaudeInterfaceSatisfaction verifies Claude satisfies all expected interfaces.
func TestClaudeInterfaceSatisfaction(t *testing.T) {
	var _ Provider = (*ClaudeProvider)(nil)
	var _ ToolProvider = (*ClaudeProvider)(nil)
	var _ ThinkingProvider = (*ClaudeProvider)(nil)
	var _ ThinkingToolProvider = (*ClaudeProvider)(nil)
	var _ ItemProvider = (*ClaudeProvider)(nil)
	var _ ItemToolProvider = (*ClaudeProvider)(nil)
	var _ ItemThinkingProvider = (*ClaudeProvider)(nil)
	var _ ItemThinkingToolProvider = (*ClaudeProvider)(nil)
	var _ ModelDiscoverer = (*ClaudeProvider)(nil)
	// Claude deliberately does NOT satisfy Continuer:
	// var _ Continuer = (*ClaudeProvider)(nil)
}

func TestClaude_GenerateItems_FunctionCallOutput(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"content":[{"type":"text","text":"got result"}]}`)

	p, _ := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL))
	items := []Item{
		MessageItem{Role: "user", Text: "compute"},
		FunctionCallOutputItem{CallID: "call_abc", Output: "{\"answer\":42}"},
	}
	resp, err := p.GenerateItems(context.Background(), items...)
	if err != nil {
		t.Fatalf("GenerateItems error: %v", err)
	}
	if resp.OutputText() != "got result" {
		t.Errorf("OutputText = %q, want 'got result'", resp.OutputText())
	}
}

func TestClaude_ErrorClassification(t *testing.T) {
	tests := []struct {
		status int
		target error
	}{
		{429, ErrRateLimited},
		{401, ErrAuthFailure},
		{403, ErrAuthFailure},
		{500, ErrProviderUnavailable},
		{400, ErrInvalidRequest},
	}
	for _, tc := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		p, _ := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL))
		_, err := p.Generate(context.Background(), "hi")
		srv.Close()
		if err == nil || !errors.Is(err, tc.target) {
			t.Errorf("HTTP %d: got %v, want %v", tc.status, err, tc.target)
		}
	}
}
