package llmprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGemini_GenerateItems_TextParts verifies text candidate parts are decoded to MessageItem.
func TestGemini_GenerateItems_TextParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "gemini_resp_1",
			"candidates": [{
				"content": {
					"parts": [{"text": "hello from gemini"}]
				}
			}]
		}`))
	}))
	defer srv.Close()

	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: "user", Text: "hi"})
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	m, ok := resp.Output[0].(MessageItem)
	if !ok || m.Text != "hello from gemini" {
		t.Errorf("expected MessageItem with 'hello from gemini', got %v", resp.Output[0])
	}
	if resp.OutputText() != "hello from gemini" {
		t.Errorf("OutputText() = %q, want %q", resp.OutputText(), "hello from gemini")
	}
	if resp.ID != "gemini_resp_1" {
		t.Errorf("Response.ID = %q, want gemini_resp_1", resp.ID)
	}
}

// TestGemini_GenerateItems_FunctionCallPart verifies functionCall candidate parts are decoded.
func TestGemini_GenerateItems_FunctionCallPart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [{
						"functionCall": {
							"name": "lookup",
							"args": {"query": "weather"}
						}
					}]
				}
			}]
		}`))
	}))
	defer srv.Close()

	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	tool := Tool{Name: "lookup", Description: "Lookup info", Schema: map[string]any{"type": "object"}}
	resp, err := p.GenerateItemsWithTool(context.Background(), tool, MessageItem{Role: "user", Text: "find"})
	if err != nil {
		t.Fatalf("GenerateItemsWithTool: %v", err)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	fc, ok := resp.Output[0].(FunctionCallItem)
	if !ok || fc.Name != "lookup" || fc.Arguments != `{"query":"weather"}` {
		t.Errorf("FunctionCallItem = %+v", resp.Output[0])
	}

	// Also verify wrapper
	args, err := p.GenerateWithTool(context.Background(), "find", tool)
	if err != nil {
		t.Fatalf("GenerateWithTool: %v", err)
	}
	if args != `{"query":"weather"}` {
		t.Errorf("GenerateWithTool = %q, want %q", args, `{"query":"weather"}`)
	}
}

// TestGemini_GenerateItems_InterleavedParts verifies thought and text parts are decoded.
func TestGemini_GenerateItems_InterleavedParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [
						{"thought": "pondering the question"},
						{"text": "the solution is 42"}
					]
				}
			}]
		}`))
	}))
	defer srv.Close()

	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
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
	if resp.OutputText() != "the solution is 42" {
		t.Errorf("OutputText() = %q, want %q", resp.OutputText(), "the solution is 42")
	}
}

// TestGemini_Continue verifies previous_interaction_id is sent in request body.
func TestGemini_Continue(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"id":"interaction_2","candidates":[{"content":{"parts":[{"text":"more text"}]}}]}`)

	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	resp, err := p.Continue(context.Background(), "interaction_1", MessageItem{Role: "user", Text: "go on"})
	if err != nil {
		t.Fatal(err)
	}
	if body["previous_interaction_id"] != "interaction_1" {
		t.Errorf("previous_interaction_id = %v, want interaction_1", body["previous_interaction_id"])
	}
	if resp.ID != "interaction_2" {
		t.Errorf("Response.ID = %q, want interaction_2", resp.ID)
	}
}

// TestGeminiInterfaceSatisfaction verifies Gemini satisfies all expected interfaces.
func TestGeminiInterfaceSatisfaction(t *testing.T) {
	var _ Provider = (*GeminiProvider)(nil)
	var _ ToolProvider = (*GeminiProvider)(nil)
	var _ ThinkingProvider = (*GeminiProvider)(nil)
	var _ ThinkingToolProvider = (*GeminiProvider)(nil)
	var _ ItemProvider = (*GeminiProvider)(nil)
	var _ ItemToolProvider = (*GeminiProvider)(nil)
	var _ ItemThinkingProvider = (*GeminiProvider)(nil)
	var _ ItemThinkingToolProvider = (*GeminiProvider)(nil)
	var _ Continuer = (*GeminiProvider)(nil)
	var _ ModelDiscoverer = (*GeminiProvider)(nil)
}

func TestGemini_GenerateItems_FunctionCallOutput(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"received output"}]}}]}`)

	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	items := []Item{
		MessageItem{Role: "user", Text: "compute"},
		FunctionCallOutputItem{CallID: "fn1", Output: "{\"res\":1}"},
	}
	resp, err := p.GenerateItems(context.Background(), items...)
	if err != nil {
		t.Fatalf("GenerateItems error: %v", err)
	}
	if resp.OutputText() != "received output" {
		t.Errorf("OutputText = %q, want 'received output'", resp.OutputText())
	}
}

func TestGemini_ErrorClassification(t *testing.T) {
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
		p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
		_, err := p.Generate(context.Background(), "hi")
		srv.Close()
		if err == nil || !errors.Is(err, tc.target) {
			t.Errorf("HTTP %d: got %v, want %v", tc.status, err, tc.target)
		}
	}
}
