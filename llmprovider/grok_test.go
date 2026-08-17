package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGrok_Generate verifies Generate returns text from a Responses API fixture.
func TestGrok_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"resp_1","output":[{"type":"message","content":[{"type":"output_text","text":"hello world"}]}]}`))
	}))
	defer srv.Close()

	p, _ := NewGrok("k", "grok-4", WithBaseURL(srv.URL))
	out, err := p.Generate(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "hello world" {
		t.Errorf("output = %q, want %q", out, "hello world")
	}
}

// TestGrok_GenerateItems_MessageAndReasoning verifies interleaved output items
// are parsed correctly and OutputText() returns only message text. This is the
// Grok equivalent of TestClaude_MultiBlockContent.
func TestGrok_GenerateItems_MessageAndReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "resp_2",
			"output": [
				{"type": "reasoning", "summary": [{"type": "summary_text", "text": "thinking hard"}]},
				{"type": "message", "content": [{"type": "output_text", "text": "the answer"}]}
			]
		}`))
	}))
	defer srv.Close()

	p, _ := NewGrok("k", "grok-4", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: "user", Text: "hi"})
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if len(resp.Output) != 2 {
		t.Fatalf("expected 2 output items, got %d", len(resp.Output))
	}
	if _, ok := resp.Output[0].(ReasoningItem); !ok {
		t.Errorf("output[0] should be ReasoningItem, got %T", resp.Output[0])
	}
	if _, ok := resp.Output[1].(MessageItem); !ok {
		t.Errorf("output[1] should be MessageItem, got %T", resp.Output[1])
	}
	if got := resp.OutputText(); got != "the answer" {
		t.Errorf("OutputText() = %q, want %q", got, "the answer")
	}
	if resp.ID != "resp_2" {
		t.Errorf("ID = %q, want %q", resp.ID, "resp_2")
	}
}

// TestGrok_GenerateItemsWithTool verifies function_call output is parsed correctly.
func TestGrok_GenerateItemsWithTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "resp_3",
			"output": [
				{"type": "function_call", "call_id": "call_1", "name": "get_weather", "arguments": "{\"city\":\"SF\"}"}
			]
		}`))
	}))
	defer srv.Close()

	p, _ := NewGrok("k", "grok-4", WithBaseURL(srv.URL))
	tool := Tool{Name: "get_weather", Description: "Get weather", Schema: map[string]any{"type": "object"}}
	resp, err := p.GenerateItemsWithTool(context.Background(), tool, MessageItem{Role: "user", Text: "weather?"})
	if err != nil {
		t.Fatalf("GenerateItemsWithTool: %v", err)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	fc, ok := resp.Output[0].(FunctionCallItem)
	if !ok {
		t.Fatalf("output[0] should be FunctionCallItem, got %T", resp.Output[0])
	}
	if fc.CallID != "call_1" || fc.Name != "get_weather" || fc.Arguments != `{"city":"SF"}` {
		t.Errorf("FunctionCallItem = %+v", fc)
	}

	// Also verify the string wrapper returns arguments
	args, err := p.GenerateWithTool(context.Background(), "weather?", tool)
	if err != nil {
		t.Fatalf("GenerateWithTool: %v", err)
	}
	if args != `{"city":"SF"}` {
		t.Errorf("GenerateWithTool = %q, want %q", args, `{"city":"SF"}`)
	}
}

// TestGrok_GenerateThinking_ReasoningEffortSent verifies reasoning.effort is
// present in the request body for a model that supports it (grok-4.5).
func TestGrok_GenerateThinking_ReasoningEffortSent(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)

	p, _ := NewGrok("k", "grok-4.5", WithBaseURL(srv.URL), WithReasoningEffort("high"))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning block, body=%v", body)
	}
	if reasoning["effort"] != "high" {
		t.Errorf("reasoning.effort = %v, want high", reasoning["effort"])
	}
}

// TestGrok_GenerateThinking_ReasoningEffortOmitted verifies reasoning key is
// absent for a model that rejects it (grok-4).
func TestGrok_GenerateThinking_ReasoningEffortOmitted(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)

	p, _ := NewGrok("k", "grok-4", WithBaseURL(srv.URL), WithReasoningEffort("high"))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if _, present := body["reasoning"]; present {
		t.Errorf("reasoning must be omitted for grok-4: %v", body)
	}
}

// TestGrok_GenerateThinking_ReasoningEffortClamped verifies grok-3-mini clamps
// "medium" to "high" (only low/high supported).
func TestGrok_GenerateThinking_ReasoningEffortClamped(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)

	p, _ := NewGrok("k", "grok-3-mini", WithBaseURL(srv.URL), WithReasoningEffort("medium"))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	reasoning := body["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" {
		t.Errorf("reasoning.effort = %v, want high (clamped from medium)", reasoning["effort"])
	}
}

// TestGrok_Continue verifies previous_response_id is sent in request body
// and response ID is captured.
func TestGrok_Continue(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"id":"resp_next","output":[{"type":"message","content":[{"type":"output_text","text":"continued"}]}]}`)

	p, _ := NewGrok("k", "grok-4", WithBaseURL(srv.URL))
	resp, err := p.Continue(context.Background(), "resp_prev", MessageItem{Role: "user", Text: "go on"})
	if err != nil {
		t.Fatal(err)
	}
	if body["previous_response_id"] != "resp_prev" {
		t.Errorf("previous_response_id = %v, want resp_prev", body["previous_response_id"])
	}
	if resp.ID != "resp_next" {
		t.Errorf("Response.ID = %q, want resp_next", resp.ID)
	}
}

// TestGrok_ErrorClassification verifies status codes map to correct sentinel errors.
func TestGrok_ErrorClassification(t *testing.T) {
	tests := []struct {
		status int
		target error
	}{
		{429, ErrRateLimited},
		{401, ErrAuthFailure},
		{403, ErrAuthFailure},
		{500, ErrProviderUnavailable},
		{503, ErrProviderUnavailable},
		{400, ErrInvalidRequest},
		{422, ErrInvalidRequest},
	}
	for _, tc := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		p, _ := NewGrok("k", "grok-4", WithBaseURL(srv.URL))
		_, err := p.Generate(context.Background(), "hi")
		srv.Close()
		if !errors.Is(err, tc.target) {
			t.Errorf("HTTP %d: got %v, want %v", tc.status, err, tc.target)
		}
	}
}

// TestGrok_KeyInHeader verifies API key travels in Authorization header, not URL.
func TestGrok_KeyInHeader(t *testing.T) {
	const key = "test-api-key-grok-123"
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	p, _ := NewGrok(key, "grok-4", WithBaseURL(srv.URL))
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+key {
		t.Errorf("Authorization = %q, want Bearer %s", gotAuth, key)
	}
}

// TestGrok_GenerateItems_RequestBody verifies the Responses API request shape.
func TestGrok_GenerateItems_RequestBody(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer srv.Close()

	p, _ := NewGrok("k", "grok-4", WithBaseURL(srv.URL), WithMaxTokens(4096))
	if _, err := p.Generate(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}

	if body["model"] != "grok-4" {
		t.Errorf("model = %v", body["model"])
	}
	if mt, ok := body["max_output_tokens"].(float64); !ok || int(mt) != 4096 {
		t.Errorf("max_output_tokens = %v", body["max_output_tokens"])
	}
	input, ok := body["input"].([]any)
	if !ok || len(input) == 0 {
		t.Fatalf("input missing or empty: %v", body["input"])
	}
}

func TestGrok_GenerateItems_FunctionCallOutput(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"output":[{"type":"message","content":[{"type":"output_text","text":"got output"}]}]}`)

	p, _ := NewGrok("k", "grok-4", WithBaseURL(srv.URL))
	items := []Item{
		MessageItem{Role: "user", Text: "compute"},
		FunctionCallOutputItem{CallID: "call_abc", Output: "{\"res\":1}"},
	}
	resp, err := p.GenerateItems(context.Background(), items...)
	if err != nil {
		t.Fatalf("GenerateItems error: %v", err)
	}
	if resp.OutputText() != "got output" {
		t.Errorf("OutputText = %q, want 'got output'", resp.OutputText())
	}
}
