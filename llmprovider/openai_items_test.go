package llmprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAI_GenerateItems_MessageOutput verifies message output items are decoded.
func TestOpenAI_GenerateItems_MessageOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "resp_oai_1",
			"output": [
				{"type": "message", "content": [{"type": "output_text", "text": "hello from openai"}]}
			]
		}`))
	}))
	defer srv.Close()

	p, _ := NewOpenAI("k", "gpt-4o", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: "user", Text: "hi"})
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	if m, ok := resp.Output[0].(MessageItem); !ok || m.Text != "hello from openai" {
		t.Errorf("expected MessageItem with text 'hello from openai', got %v", resp.Output[0])
	}
	if resp.OutputText() != "hello from openai" {
		t.Errorf("OutputText() = %q, want %q", resp.OutputText(), "hello from openai")
	}
}

// TestOpenAI_GenerateItems_FunctionCallOutput verifies tool call output is decoded.
func TestOpenAI_GenerateItems_FunctionCallOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "resp_oai_2",
			"output": [
				{"type": "function_call", "call_id": "call_123", "name": "calc", "arguments": "{\"x\":42}"}
			]
		}`))
	}))
	defer srv.Close()

	p, _ := NewOpenAI("k", "gpt-4o", WithBaseURL(srv.URL))
	tool := Tool{Name: "calc", Description: "Calculator", Schema: map[string]any{"type": "object"}}
	resp, err := p.GenerateItemsWithTool(context.Background(), tool, MessageItem{Role: "user", Text: "compute"})
	if err != nil {
		t.Fatalf("GenerateItemsWithTool: %v", err)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	fc, ok := resp.Output[0].(FunctionCallItem)
	if !ok || fc.CallID != "call_123" || fc.Name != "calc" || fc.Arguments != `{"x":42}` {
		t.Errorf("FunctionCallItem = %+v", resp.Output[0])
	}

	// Also verify wrapper
	args, err := p.GenerateWithTool(context.Background(), "compute", tool)
	if err != nil {
		t.Fatalf("GenerateWithTool: %v", err)
	}
	if args != `{"x":42}` {
		t.Errorf("GenerateWithTool = %q, want %q", args, `{"x":42}`)
	}
}

// TestOpenAI_GenerateItems_InterleavedItems verifies reasoning + message items in sequence.
func TestOpenAI_GenerateItems_InterleavedItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "resp_oai_3",
			"output": [
				{"type": "reasoning", "summary": [{"type": "summary_text", "text": "let me think"}]},
				{"type": "message", "content": [{"type": "output_text", "text": "result is 42"}]}
			]
		}`))
	}))
	defer srv.Close()

	p, _ := NewOpenAI("k", "o4", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: "user", Text: "question"})
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if len(resp.Output) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Output))
	}
	if _, ok := resp.Output[0].(ReasoningItem); !ok {
		t.Errorf("output[0] should be ReasoningItem, got %T", resp.Output[0])
	}
	if _, ok := resp.Output[1].(MessageItem); !ok {
		t.Errorf("output[1] should be MessageItem, got %T", resp.Output[1])
	}
	if resp.OutputText() != "result is 42" {
		t.Errorf("OutputText() = %q, want %q", resp.OutputText(), "result is 42")
	}
}

// TestOpenAI_Continue verifies previous_response_id is sent in request body.
func TestOpenAI_Continue(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"id":"resp_oai_4","output":[{"type":"message","content":[{"type":"output_text","text":"continued"}]}]}`)

	p, _ := NewOpenAI("k", "gpt-4o", WithBaseURL(srv.URL))
	resp, err := p.Continue(context.Background(), "resp_prev_1", MessageItem{Role: "user", Text: "next step"})
	if err != nil {
		t.Fatal(err)
	}
	if body["previous_response_id"] != "resp_prev_1" {
		t.Errorf("previous_response_id = %v, want resp_prev_1", body["previous_response_id"])
	}
	if resp.ID != "resp_oai_4" {
		t.Errorf("Response.ID = %q, want resp_oai_4", resp.ID)
	}
}

// TestOpenAI_ResponseID confirms Response.ID is populated from the response envelope.
func TestOpenAI_ResponseID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"resp_id_999","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	p, _ := NewOpenAI("k", "gpt-4o", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: "user", Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "resp_id_999" {
		t.Errorf("Response.ID = %q, want resp_id_999", resp.ID)
	}
}

// TestOpenAIInterfaceSatisfaction verifies OpenAI satisfies all expected interfaces.
func TestOpenAIInterfaceSatisfaction(t *testing.T) {
	var _ Provider = (*OpenAIProvider)(nil)
	var _ ToolProvider = (*OpenAIProvider)(nil)
	var _ ThinkingProvider = (*OpenAIProvider)(nil)
	var _ ThinkingToolProvider = (*OpenAIProvider)(nil)
	var _ ItemProvider = (*OpenAIProvider)(nil)
	var _ ItemToolProvider = (*OpenAIProvider)(nil)
	var _ ItemThinkingProvider = (*OpenAIProvider)(nil)
	var _ ItemThinkingToolProvider = (*OpenAIProvider)(nil)
	var _ Continuer = (*OpenAIProvider)(nil)
	var _ ModelDiscoverer = (*OpenAIProvider)(nil)
}

func TestOpenAI_GenerateItems_FunctionCallOutputItem(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"output":[{"type":"message","content":[{"type":"output_text","text":"received output"}]}]}`)

	p, _ := NewOpenAI("k", "gpt-4o", WithBaseURL(srv.URL))
	items := []Item{
		MessageItem{Role: "user", Text: "compute"},
		FunctionCallOutputItem{CallID: "call_abc", Output: "{\"res\":1}"},
	}
	resp, err := p.GenerateItems(context.Background(), items...)
	if err != nil {
		t.Fatalf("GenerateItems error: %v", err)
	}
	if resp.OutputText() != "received output" {
		t.Errorf("OutputText = %q, want 'received output'", resp.OutputText())
	}
}

func TestOpenAI_ErrorClassification(t *testing.T) {
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
		p, _ := NewOpenAI("k", "gpt-4o", WithBaseURL(srv.URL))
		_, err := p.Generate(context.Background(), "hi")
		srv.Close()
		if err == nil || !errors.Is(err, tc.target) {
			t.Errorf("HTTP %d: got %v, want %v", tc.status, err, tc.target)
		}
	}
}
