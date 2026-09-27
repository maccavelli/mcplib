package llmprovider

import (
	"context"
	"reflect"
	"testing"
)

// thinkingCase is one GenerateThinking request and the thinking fields its
// body must carry. A nil want entry means the key must be absent.
type thinkingCase struct {
	model, effort string
	budget        int
	want          map[string]any
}

func assertThinkingFields(t *testing.T, body map[string]any, want map[string]any) {
	t.Helper()
	for k, v := range want {
		got, present := body[k]
		switch {
		case v == nil && present:
			t.Errorf("%s = %v, want it absent", k, got)
		case v != nil && !reflect.DeepEqual(got, v):
			t.Errorf("%s = %v, want %v", k, got, v)
		}
	}
}

// TestThinkingWire_Claude pins MADR 0013 Q1 and B9 on the Anthropic wire:
// Claude 4.7 and later take adaptive thinking with output_config.effort
// (thinking.type "enabled" is HTTP 400 there); older models take a budget,
// 1024 for "low". An explicit budget wins where a budget is accepted.
func TestThinkingWire_Claude(t *testing.T) {
	adaptive := map[string]any{"type": "adaptive"}
	for _, tc := range []thinkingCase{
		{"claude-sonnet-5", effortLow, 0, map[string]any{"thinking": adaptive,
			"output_config": map[string]any{"effort": "low"}}},
		{"claude-opus-4-8", "", 0, map[string]any{"thinking": adaptive, "output_config": nil}},
		{"claude-sonnet-5", "", 9000, map[string]any{"thinking": adaptive, "max_tokens": float64(8192)}},
		{"claude-haiku-4-5", effortLow, 0, map[string]any{"output_config": nil,
			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(1024)}}},
		{"claude-haiku-4-5", "", 0, map[string]any{
			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(4096)}}},
		{"claude-haiku-4-5", effortLow, 2000, map[string]any{
			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(2000)}}},
		{"claude-sonnet-4-20250514", effortLow, 0, map[string]any{
			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(1024)}}},
	} {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			var body map[string]any
			srv := captureServer(t, &body, `{"content":[{"type":"text","text":"ok"}]}`)
			p, err := NewClaude("k", tc.model, WithBaseURL(srv.URL),
				WithReasoningEffort(tc.effort), WithThinkingBudget(tc.budget))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
				t.Fatalf("GenerateThinking: %v", err)
			}
			assertThinkingFields(t, body, tc.want)
		})
	}
}

// TestThinkingWire_Gemini pins MADR 0013 Q1 on the Gemini wire: "low" is
// thinkingLevel on Gemini 3 and later and a 1024 budget on 2.x (thinkingLevel
// is HTTP 400 there); other efforts keep the dynamic budget; a budget wins.
func TestThinkingWire_Gemini(t *testing.T) {
	for _, tc := range []thinkingCase{
		{"gemini-3.7-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingLevel": "low"}}},
		{"gemini-2.5-flash", effortLow, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(1024)}}},
		{"gemini-3.7-flash", "", 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(-1)}}},
		{"gemini-3.7-flash", effortHigh, 0, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(-1)}}},
		{"gemini-2.5-flash", effortLow, 512, map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": float64(512)}}},
	} {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			var body map[string]any
			srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
			p, err := NewGemini(context.Background(), "k", tc.model, WithBaseURL(srv.URL),
				WithReasoningEffort(tc.effort), WithThinkingBudget(tc.budget))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
				t.Fatalf("GenerateThinking: %v", err)
			}
			gc, _ := body["generationConfig"].(map[string]any)
			assertThinkingFields(t, gc, tc.want)
		})
	}
}

// TestThinkingWire_OpencodeRoutes pins the same shapes on OpenCode's messages
// and google routes, which forward to the same upstream APIs.
func TestThinkingWire_OpencodeRoutes(t *testing.T) {
	for _, tc := range []struct {
		gateway, model, fixture string
		inGenCfg                bool
		want                    map[string]any
	}{
		{ProviderOpencodeZen, "claude-sonnet-5", fxOpencodeMessages, false, map[string]any{
			"thinking": map[string]any{"type": "adaptive"}, "output_config": map[string]any{"effort": "low"}}},
		{ProviderOpencodeGo, "qwen3.8-flash", fxOpencodeMessages, false, map[string]any{
			"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(1024)}}},
		{ProviderOpencodeZen, "gemini-3.8-flash", fxOpencodeGoogle, true, map[string]any{
			"thinkingConfig": map[string]any{"thinkingLevel": "low"}}},
	} {
		t.Run(tc.gateway+"/"+tc.model, func(t *testing.T) {
			var body map[string]any
			srv := captureServer(t, &body, tc.fixture)
			p, err := NewOpencode(tc.gateway, "k", tc.model, WithBaseURL(srv.URL), WithReasoningEffort(effortLow))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
				t.Fatalf("GenerateThinking: %v", err)
			}
			if tc.inGenCfg {
				body, _ = body["generationConfig"].(map[string]any)
			}
			assertThinkingFields(t, body, tc.want)
		})
	}
}
