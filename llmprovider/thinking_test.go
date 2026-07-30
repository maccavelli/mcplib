package llmprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureServer returns an httptest server that records the decoded JSON request body of
// the last call and replies with the given canned response body.
func captureServer(t *testing.T, lastBody *map[string]any, respBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		m := map[string]any{}
		_ = json.Unmarshal(raw, &m)
		*lastBody = m
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestClaudeThinking_RequestBody verifies the thinking path emits a thinking block and
// raises max_tokens above the budget (Anthropic requires max_tokens > budget_tokens).
func TestClaudeThinking_RequestBody(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"content":[{"type":"text","text":"ok"}]}`)

	// maxTokens (4096) <= budget (8000) must force the ceiling above the budget.
	p, err := NewClaude("k", "claude-x", WithBaseURL(srv.URL), WithMaxTokens(4096), WithThinkingBudget(8000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatalf("GenerateThinking: %v", err)
	}

	think, ok := body["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("expected thinking block, body=%v", body)
	}
	if think["type"] != "enabled" {
		t.Errorf("thinking.type = %v, want enabled", think["type"])
	}
	if budget := think["budget_tokens"].(float64); int(budget) != 8000 {
		t.Errorf("budget_tokens = %v, want 8000", budget)
	}
	if mt := body["max_tokens"].(float64); !(int(mt) > 8000) {
		t.Errorf("max_tokens = %v, must exceed budget 8000", mt)
	}
}

// TestClaudeNonThinking_NoThinkingBlock verifies the default path is unchanged.
func TestClaudeNonThinking_NoThinkingBlock(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"content":[{"type":"text","text":"ok"}]}`)

	p, _ := NewClaude("k", "claude-x", WithBaseURL(srv.URL))
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if _, present := body["thinking"]; present {
		t.Errorf("non-thinking request must not contain a thinking block: %v", body)
	}
}

// TestClaudeThinkingTool_AutoChoice verifies extended thinking drops the forced
// tool_choice (Anthropic forbids forcing a tool while thinking is enabled).
func TestClaudeThinkingTool_AutoChoice(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"content":[{"type":"tool_use","input":{"x":1}}]}`)

	p, _ := NewClaude("k", "claude-x", WithBaseURL(srv.URL), WithThinkingBudget(2048))
	tool := Tool{Name: "emit", Description: "d", Schema: map[string]any{"type": "object"}}
	if _, err := p.GenerateWithToolThinking(context.Background(), "hi", tool); err != nil {
		t.Fatal(err)
	}
	tc, ok := body["tool_choice"].(map[string]any)
	if !ok || tc["type"] != "auto" {
		t.Errorf("thinking tool_choice = %v, want type=auto", body["tool_choice"])
	}
}

// TestOpenAIThinking_RequestBody verifies the reasoning path swaps max_tokens for
// max_completion_tokens and sets reasoning_effort.
func TestOpenAIThinking_RequestBody(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"choices":[{"message":{"content":"ok"}}]}`)

	p, _ := NewOpenAI("k", "o4", WithBaseURL(srv.URL), WithMaxTokens(5000), WithReasoningEffort("high"))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if _, present := body["max_tokens"]; present {
		t.Errorf("reasoning request must not send max_tokens: %v", body)
	}
	if mct, ok := body["max_completion_tokens"].(float64); !ok || int(mct) != 5000 {
		t.Errorf("max_completion_tokens = %v, want 5000", body["max_completion_tokens"])
	}
	if body["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v, want high", body["reasoning_effort"])
	}
}

// TestOpenAIThinking_DefaultEffort verifies the default effort is applied when unset.
func TestOpenAIThinking_DefaultEffort(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"choices":[{"message":{"content":"ok"}}]}`)

	p, _ := NewOpenAI("k", "o4", WithBaseURL(srv.URL))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if body["reasoning_effort"] != defaultOpenAIReasoningEffort {
		t.Errorf("reasoning_effort = %v, want default %q", body["reasoning_effort"], defaultOpenAIReasoningEffort)
	}
}

// TestOpenAINonThinking_UsesMaxTokens verifies the default path still uses max_tokens.
func TestOpenAINonThinking_UsesMaxTokens(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"choices":[{"message":{"content":"ok"}}]}`)

	p, _ := NewOpenAI("k", "gpt-x", WithBaseURL(srv.URL), WithMaxTokens(321))
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if mt, ok := body["max_tokens"].(float64); !ok || int(mt) != 321 {
		t.Errorf("max_tokens = %v, want 321", body["max_tokens"])
	}
	if _, present := body["reasoning_effort"]; present {
		t.Errorf("non-thinking request must not send reasoning_effort: %v", body)
	}
}

// TestGeminiThinking_RequestBody verifies a thinkingConfig is nested in generationConfig
// with the configured budget, and that the default path omits it.
func TestGeminiThinking_RequestBody(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)

	p, _ := NewGemini(context.Background(), "k", "gemini-x", WithBaseURL(srv.URL), WithThinkingBudget(1234))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	gc, ok := body["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("missing generationConfig: %v", body)
	}
	tc, ok := gc["thinkingConfig"].(map[string]any)
	if !ok {
		t.Fatalf("missing thinkingConfig: %v", gc)
	}
	if b := tc["thinkingBudget"].(float64); int(b) != 1234 {
		t.Errorf("thinkingBudget = %v, want 1234", b)
	}

	// Default path: no thinkingConfig.
	body = nil
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	gc2 := body["generationConfig"].(map[string]any)
	if _, present := gc2["thinkingConfig"]; present {
		t.Errorf("non-thinking request must not contain thinkingConfig: %v", gc2)
	}
}

// TestGeminiThinking_DynamicBudgetDefault verifies an unset budget maps to -1 (dynamic).
func TestGeminiThinking_DynamicBudgetDefault(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)

	p, _ := NewGemini(context.Background(), "k", "gemini-x", WithBaseURL(srv.URL))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	gc := body["generationConfig"].(map[string]any)
	tc := gc["thinkingConfig"].(map[string]any)
	if b := tc["thinkingBudget"].(float64); int(b) != dynamicGeminiThinkingBudget {
		t.Errorf("thinkingBudget = %v, want %d (dynamic)", b, dynamicGeminiThinkingBudget)
	}
}

// TestProvidersImplementThinkingInterfaces is a compile-time guarantee that all three
// providers satisfy the optional thinking interfaces.
func TestProvidersImplementThinkingInterfaces(t *testing.T) {
	var _ ThinkingProvider = (*ClaudeProvider)(nil)
	var _ ThinkingProvider = (*OpenAIProvider)(nil)
	var _ ThinkingProvider = (*GeminiProvider)(nil)
	var _ ThinkingToolProvider = (*ClaudeProvider)(nil)
	var _ ThinkingToolProvider = (*OpenAIProvider)(nil)
	var _ ThinkingToolProvider = (*GeminiProvider)(nil)
}
