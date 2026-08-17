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

// TestOpenAIThinking_RequestBody verifies the reasoning path sets reasoning.effort
// and max_output_tokens.
func TestOpenAIThinking_RequestBody(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)

	p, _ := NewOpenAI("k", "o4", WithBaseURL(srv.URL), WithMaxTokens(5000), WithReasoningEffort("high"))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if mt, ok := body["max_output_tokens"].(float64); !ok || int(mt) != 5000 {
		t.Errorf("max_output_tokens = %v, want 5000", body["max_output_tokens"])
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Errorf("reasoning.effort = %v, want high", body["reasoning"])
	}
}

// TestOpenAIThinking_DefaultEffort verifies the default effort is applied when unset.
func TestOpenAIThinking_DefaultEffort(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)

	p, _ := NewOpenAI("k", "o4", WithBaseURL(srv.URL))
	if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != defaultOpenAIReasoningEffort {
		t.Errorf("reasoning.effort = %v, want default %q", body["reasoning"], defaultOpenAIReasoningEffort)
	}
}

// TestOpenAINonThinking_UsesMaxTokens verifies the default path uses max_output_tokens and omits reasoning.
func TestOpenAINonThinking_UsesMaxTokens(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)

	p, _ := NewOpenAI("k", "gpt-x", WithBaseURL(srv.URL), WithMaxTokens(321))
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if mt, ok := body["max_output_tokens"].(float64); !ok || int(mt) != 321 {
		t.Errorf("max_output_tokens = %v, want 321", body["max_output_tokens"])
	}
	if _, present := body["reasoning"]; present {
		t.Errorf("non-thinking request must not send reasoning: %v", body)
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

func TestOpenAIThinkingTool(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"output":[{"type":"function_call","call_id":"c1","name":"calc","arguments":"{\"x\":1}"}]}`)

	p, _ := NewOpenAI("k", "o4", WithBaseURL(srv.URL), WithReasoningEffort("high"))
	tool := Tool{Name: "calc", Description: "calc", Schema: map[string]any{"type": "object"}}
	args, err := p.GenerateWithToolThinking(context.Background(), "hi", tool)
	if err != nil {
		t.Fatalf("GenerateWithToolThinking error: %v", err)
	}
	if args != `{"x":1}` {
		t.Errorf("expected arguments '{\"x\":1}', got %q", args)
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Errorf("expected reasoning effort high, got %v", body["reasoning"])
	}
}

func TestGeminiThinkingTool(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"q":"test"}}}]}}]}`)

	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL), WithThinkingBudget(1000))
	tool := Tool{Name: "lookup", Description: "lookup", Schema: map[string]any{"type": "object"}}
	args, err := p.GenerateWithToolThinking(context.Background(), "hi", tool)
	if err != nil {
		t.Fatalf("GenerateWithToolThinking error: %v", err)
	}
	if args != `{"q":"test"}` {
		t.Errorf("expected '{\"q\":\"test\"}', got %q", args)
	}
	gc := body["generationConfig"].(map[string]any)
	tc := gc["thinkingConfig"].(map[string]any)
	if b := tc["thinkingBudget"].(float64); int(b) != 1000 {
		t.Errorf("thinkingBudget = %v, want 1000", b)
	}
}

func TestGrokThinkingTool(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"output":[{"type":"function_call","call_id":"c1","name":"query","arguments":"{\"a\":2}"}]}`)

	p, _ := NewGrok("k", "grok-4.5", WithBaseURL(srv.URL), WithReasoningEffort("high"))
	tool := Tool{Name: "query", Description: "query", Schema: map[string]any{"type": "object"}}
	args, err := p.GenerateWithToolThinking(context.Background(), "hi", tool)
	if err != nil {
		t.Fatalf("GenerateWithToolThinking error: %v", err)
	}
	if args != `{"a":2}` {
		t.Errorf("expected '{\"a\":2}', got %q", args)
	}
}

func TestProviderNamesAndConstructors(t *testing.T) {
	cp, err := NewClaude("key", "claude-x")
	if err != nil || cp.Name() != ProviderClaude {
		t.Errorf("Claude name = %v, err = %v", cp.Name(), err)
	}
	if _, err := NewClaude("", "claude-x"); err == nil {
		t.Error("expected error for empty Claude API key")
	}

	op, err := NewOpenAI("key", "gpt-x")
	if err != nil || op.Name() != ProviderOpenAI {
		t.Errorf("OpenAI name = %v, err = %v", op.Name(), err)
	}

	gp, err := NewGemini(context.Background(), "key", "gemini-x")
	if err != nil || gp.Name() != ProviderGemini {
		t.Errorf("Gemini name = %v, err = %v", gp.Name(), err)
	}

	xaiP, err := NewGrok("key", "grok-x")
	if err != nil || xaiP.Name() != ProviderGrok {
		t.Errorf("Grok name = %v, err = %v", xaiP.Name(), err)
	}
	if _, err := NewGrok("", "grok-x"); err == nil {
		t.Error("expected error for empty Grok API key")
	}
}

// TestProvidersImplementThinkingInterfaces is a compile-time guarantee that all
// providers satisfy the optional thinking interfaces.
func TestProvidersImplementThinkingInterfaces(t *testing.T) {
	var _ ThinkingProvider = (*ClaudeProvider)(nil)
	var _ ThinkingProvider = (*OpenAIProvider)(nil)
	var _ ThinkingProvider = (*GeminiProvider)(nil)
	var _ ThinkingProvider = (*GrokProvider)(nil)
	var _ ThinkingToolProvider = (*ClaudeProvider)(nil)
	var _ ThinkingToolProvider = (*OpenAIProvider)(nil)
	var _ ThinkingToolProvider = (*GeminiProvider)(nil)
	var _ ThinkingToolProvider = (*GrokProvider)(nil)
	var _ ItemProvider = (*GrokProvider)(nil)
	var _ Continuer = (*GrokProvider)(nil)
}
