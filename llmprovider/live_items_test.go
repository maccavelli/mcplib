//go:build live_gateways

package llmprovider

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive_ToolRoundTrip sends a completed tool call and its result on every
// wire, and requires the model to answer from the result (MADR 0012 §2).
// Before the fix the call was dropped: Anthropic, Gemini and the Responses
// route answered 400, and the chat routes answered without the tool.
func TestLive_ToolRoundTrip(t *testing.T) {
	convo := []Item{
		MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris? Use the tool."},
		FunctionCallItem{CallID: "call_rt_1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
		FunctionCallOutputItem{CallID: "call_rt_1", Output: `{"forecast":"sunny, 21C"}`},
	}
	for _, c := range []struct {
		name, env string
		build     func(t *testing.T, key string) (ItemProvider, error)
	}{
		{"claude", "ANTHROPIC_API_KEY", func(_ *testing.T, k string) (ItemProvider, error) { return NewClaude(k, "claude-haiku-4-5") }},
		{"gemini", "GEMINI_API_KEY", func(_ *testing.T, k string) (ItemProvider, error) {
			return NewGemini(context.Background(), k, "gemini-3.7-flash")
		}},
		{"grok", "XAI_API_KEY", func(_ *testing.T, k string) (ItemProvider, error) { return NewGrok(k, "grok-4.5") }},
		{"kilo", "KILO_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
			return NewKilo(k, liveModel(t, ProviderKilo, kiloNonTraining...))
		}},
		{"go-chat", "OPENCODE_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
			return NewOpencode(ProviderOpencodeGo, k, liveModel(t, ProviderOpencodeGo, "glm-5.3-flash", "glm-5.3", "kimi-k2.6"))
		}},
		{"go-messages", "OPENCODE_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
			return NewOpencode(ProviderOpencodeGo, k, liveModel(t, ProviderOpencodeGo, "qwen3.8-flash", "minimax-m3"))
		}},
		{"go-responses", "OPENCODE_API_KEY", func(t *testing.T, k string) (ItemProvider, error) {
			return NewOpencode(ProviderOpencodeGo, k, liveModel(t, ProviderOpencodeGo, "gpt-6-luna", "grok-4.6"))
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			key := os.Getenv(c.env)
			if key == "" {
				t.Skipf("%s unset", c.env)
			}
			p, err := c.build(t, key)
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			res, err := p.GenerateItems(ctx, convo...)
			skipIfTransient(t, err)
			if err != nil {
				t.Fatalf("GenerateItems: %v", err)
			}
			if text := strings.ToLower(res.OutputText()); !strings.Contains(text, "sunny") && !strings.Contains(text, "21") {
				t.Fatalf("reply %q does not use the tool result", res.OutputText())
			}
		})
	}
}

// TestLive_GeminiReplaysRealCall: a call Gemini issued goes back with the
// thoughtSignature it came with, and Gemini answers from the result.
func TestLive_GeminiReplaysRealCall(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p, err := NewGemini(ctx, key, "gemini-3.7-flash")
	if err != nil {
		t.Fatalf("NewGemini: %v", err)
	}
	ask := MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris? Use the tool."}
	tool := Tool{Name: "get_weather", Description: "Current weather for a city",
		Schema: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}},
			"required": []string{"city"}}}
	first, err := p.GenerateItemsWithTool(ctx, tool, ask)
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateItemsWithTool: %v", err)
	}
	var call FunctionCallItem
	for _, item := range first.Output {
		if c, ok := item.(FunctionCallItem); ok {
			call = c
		}
	}
	if call.Name != "get_weather" {
		t.Fatalf("no get_weather call in %+v", first.Output)
	}
	res, err := p.GenerateItems(ctx, ask, call, FunctionCallOutputItem{CallID: call.CallID, Output: `{"forecast":"sunny, 21C"}`})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if text := strings.ToLower(res.OutputText()); !strings.Contains(text, "sunny") && !strings.Contains(text, "21") {
		t.Fatalf("reply %q does not use the tool result", res.OutputText())
	}
}
