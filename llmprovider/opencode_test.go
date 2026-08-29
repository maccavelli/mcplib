package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Per-route response fixtures. The responses one is the shape measured on Zen:
// summary is empty and the trace lives in encrypted_content, so the shared
// decoder yields a present-but-blank ReasoningItem.
const (
	fxOpencodeResponses = `{"id":"resp_1","output":[
		{"type":"reasoning","summary":[],"encrypted_content":"Q-PaDgFF"},
		{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}`
	fxOpencodeMessages = `{"content":[{"type":"text","text":"hello"}]}`
	fxOpencodeGoogle   = `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`
	fxOpencodeChat     = `{"id":"router-1","choices":[{"message":{"role":"assistant","content":"hello"}}]}`
)

// pathCapture records the request path and replies with a canned body.
func pathCapture(t *testing.T, lastPath *string, resp string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*lastPath = r.URL.Path
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestOpencode_RoutePaths asserts the per-gateway divergence at the HTTP layer:
// the same model id takes different paths on Zen and Go.
func TestOpencode_RoutePaths(t *testing.T) {
	tests := []struct {
		gateway, model, wantPath, fixture string
	}{
		{ProviderOpencodeZen, "gpt-5.5", "/responses", fxOpencodeResponses},
		{ProviderOpencodeZen, "claude-sonnet-5", "/messages", fxOpencodeMessages},
		{ProviderOpencodeZen, "gemini-3.7-flash", "/models/gemini-3.7-flash:generateContent", fxOpencodeGoogle},
		{ProviderOpencodeZen, "deepseek-v4-pro", "/chat/completions", fxOpencodeChat},
		{ProviderOpencodeZen, "minimax-m3", "/chat/completions", fxOpencodeChat},
		{ProviderOpencodeGo, "minimax-m3", "/messages", fxOpencodeMessages},
	}
	for _, tc := range tests {
		t.Run(tc.gateway+"/"+tc.model, func(t *testing.T) {
			var path string
			srv := pathCapture(t, &path, tc.fixture)
			p, err := NewOpencode(tc.gateway, "k", tc.model, WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("NewOpencode: %v", err)
			}
			if _, err := p.Generate(context.Background(), "hi"); err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if path != tc.wantPath {
				t.Errorf("path = %q, want %q", path, tc.wantPath)
			}
		})
	}
}

func TestOpencode_Generate_PerRoute(t *testing.T) {
	tests := []struct{ name, model, fixture string }{
		{"responses", "gpt-5.5", fxOpencodeResponses},
		{"messages", "claude-sonnet-5", fxOpencodeMessages},
		{"google", "gemini-3.7-flash", fxOpencodeGoogle},
		{"chat", "deepseek-v4-pro", fxOpencodeChat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			srv := pathCapture(t, &path, tc.fixture)
			p, _ := NewOpencode(ProviderOpencodeZen, "k", tc.model, WithBaseURL(srv.URL))
			resp, err := p.GenerateItems(context.Background(), MessageItem{Role: jsonRoleUser, Text: "hi"})
			if err != nil {
				t.Fatalf("GenerateItems: %v", err)
			}
			if got := resp.OutputText(); got != "hello" {
				t.Errorf("OutputText() = %q, want hello", got)
			}
			switch tc.name {
			case "responses":
				// Zen sends summary:[] with encrypted_content, so the decoder
				// emits a present-but-blank ReasoningItem.
				r, ok := resp.Output[0].(ReasoningItem)
				if !ok {
					t.Fatalf("output[0] = %T, want ReasoningItem", resp.Output[0])
				}
				if r.Text != "" {
					t.Errorf("reasoning text = %q, want empty (summary is [])", r.Text)
				}
			case "messages":
				if resp.ID != "" {
					t.Errorf("messages route is stateless; ID = %q, want empty", resp.ID)
				}
			}
		})
	}
}

// TestOpencode_KeyInHeader is the regression guard for MADR correction #5: the
// gateway takes Bearer on every route and ignores the vendor-native key headers.
func TestOpencode_KeyInHeader(t *testing.T) {
	tests := []struct{ model, fixture string }{
		{"gpt-5.5", fxOpencodeResponses},
		{"claude-sonnet-5", fxOpencodeMessages},
		{"gemini-3.7-flash", fxOpencodeGoogle},
		{"deepseek-v4-pro", fxOpencodeChat},
	}
	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
					t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
				}
				if r.Header.Get("x-api-key") != "" {
					t.Error("x-api-key must not be sent: the gateway ignores it")
				}
				if r.Header.Get("x-goog-api-key") != "" {
					t.Error("x-goog-api-key must not be sent: the gateway ignores it")
				}
				if r.URL.RawQuery != "" {
					t.Errorf("key must never reach the URL; RawQuery = %q", r.URL.RawQuery)
				}
				_, _ = w.Write([]byte(tc.fixture))
			}))
			defer srv.Close()
			p, _ := NewOpencode(ProviderOpencodeZen, "test-key", tc.model, WithBaseURL(srv.URL))
			if _, err := p.Generate(context.Background(), "hi"); err != nil {
				t.Fatalf("Generate: %v", err)
			}
		})
	}
}

func TestOpencode_ToolCall_PerRoute(t *testing.T) {
	tool := Tool{Name: "get_weather", Description: "Get weather", Schema: map[string]any{"type": "object"}}
	tests := []struct{ name, model, fixture string }{
		{"responses", "gpt-5.5",
			`{"output":[{"type":"function_call","call_id":"c1","name":"get_weather","arguments":"{\"city\":\"SF\"}"}]}`},
		{"messages", "claude-sonnet-5",
			`{"content":[{"type":"tool_use","id":"c1","name":"get_weather","input":{"city":"SF"}}]}`},
		{"google", "gemini-3.7-flash",
			`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"SF"}}}]}}]}`},
		{"chat", "deepseek-v4-pro",
			`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}]}}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			srv := pathCapture(t, &path, tc.fixture)
			p, _ := NewOpencode(ProviderOpencodeZen, "k", tc.model, WithBaseURL(srv.URL))
			args, err := p.GenerateWithTool(context.Background(), "weather?", tool)
			if err != nil {
				t.Fatalf("GenerateWithTool: %v", err)
			}
			var parsed map[string]any
			if err := json.Unmarshal([]byte(args), &parsed); err != nil {
				t.Fatalf("arguments %q are not valid JSON: %v", args, err)
			}
			if parsed["city"] != "SF" {
				t.Errorf("arguments = %q", args)
			}
		})
	}
}

// TestOpencode_Thinking_PerRoute pins where each route carries reasoning, and
// that the chat route carries none at all.
func TestOpencode_Thinking_PerRoute(t *testing.T) {
	t.Run("responses uses reasoning.effort", func(t *testing.T) {
		var body map[string]any
		srv := captureServer(t, &body, fxOpencodeResponses)
		p, _ := NewOpencode(ProviderOpencodeZen, "k", "gpt-5.5", WithBaseURL(srv.URL))
		if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
			t.Fatalf("GenerateThinking: %v", err)
		}
		r, ok := body[jsonKeyReasoning].(map[string]any)
		if !ok || r["effort"] != effortMedium {
			t.Errorf("reasoning = %v, want effort=%q", body[jsonKeyReasoning], effortMedium)
		}
	})

	t.Run("messages uses thinking.budget_tokens and raises max_tokens", func(t *testing.T) {
		var body map[string]any
		srv := captureServer(t, &body, fxOpencodeMessages)
		p, _ := NewOpencode(ProviderOpencodeZen, "k", "claude-sonnet-5",
			WithBaseURL(srv.URL), WithMaxTokens(4096), WithThinkingBudget(8000))
		if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
			t.Fatalf("GenerateThinking: %v", err)
		}
		th, ok := body["thinking"].(map[string]any)
		if !ok || th["budget_tokens"].(float64) != 8000 {
			t.Fatalf("thinking = %v", body["thinking"])
		}
		mt, ok := body[jsonKeyMaxTokens].(float64)
		if !ok || mt <= 8000 {
			t.Errorf("max_tokens = %v, must exceed budget_tokens", body[jsonKeyMaxTokens])
		}
	})

	t.Run("google uses generationConfig.thinkingConfig", func(t *testing.T) {
		var body map[string]any
		srv := captureServer(t, &body, fxOpencodeGoogle)
		p, _ := NewOpencode(ProviderOpencodeZen, "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
		if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
			t.Fatalf("GenerateThinking: %v", err)
		}
		gc, ok := body["generationConfig"].(map[string]any)
		if !ok {
			t.Fatalf("generationConfig missing: %v", body)
		}
		if _, ok := gc["thinkingConfig"]; !ok {
			t.Errorf("thinkingConfig missing: %v", gc)
		}
	})

	t.Run("chat carries no reasoning key at all", func(t *testing.T) {
		var body map[string]any
		srv := captureServer(t, &body, fxOpencodeChat)
		p, _ := NewOpencode(ProviderOpencodeZen, "k", "deepseek-v4-pro", WithBaseURL(srv.URL))
		if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
			t.Fatalf("GenerateThinking: %v", err)
		}
		for _, k := range []string{jsonKeyReasoning, jsonKeyReasoningEffort, "thinking"} {
			if _, present := body[k]; present {
				t.Errorf("chat route must not send %q: %v", k, body[k])
			}
		}
	})
}

func TestOpencode_ErrorClassification(t *testing.T) {
	tests := []struct {
		status  int
		wantErr error
	}{
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusUnauthorized, ErrAuthFailure},
		{http.StatusInternalServerError, ErrProviderUnavailable},
		{http.StatusBadRequest, ErrInvalidRequest},
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"AuthError","message":"x"}}`))
			}))
			defer srv.Close()
			p, _ := NewOpencode(ProviderOpencodeZen, "k", "claude-sonnet-5", WithBaseURL(srv.URL))
			_, err := p.Generate(context.Background(), "hi")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want wrapping %v", err, tc.wantErr)
			}
			if tc.status == http.StatusTooManyRequests {
				var rl *RateLimitError
				if !errors.As(err, &rl) || rl.RetryAfter != 0 {
					// The gateway sends no Retry-After (measured 2026-08-28).
					t.Errorf("RetryAfter = %v, want 0", rl.RetryAfter)
				}
			}
			// Every classification names gateway/route, 429 included.
			if !strings.Contains(err.Error(), "opencode-zen/messages") {
				t.Errorf("error must name gateway/route for diagnosability: %v", err)
			}
		})
	}
}

func TestOpencode_WithOpencodeRoute(t *testing.T) {
	var path string
	srv := pathCapture(t, &path, fxOpencodeChat)
	// gpt-5.5 is tabled as responses; the override must win.
	p, err := NewOpencode(ProviderOpencodeZen, "k", "gpt-5.5",
		WithBaseURL(srv.URL), WithOpencodeRoute(OpencodeRouteChatCompletions))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	if p.Route() != OpencodeRouteChatCompletions {
		t.Errorf("Route() = %q, want override", p.Route())
	}
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", path)
	}
}

func TestOpencode_ConstructorErrors(t *testing.T) {
	if _, err := NewOpencode("nope", "k", "m"); err == nil {
		t.Error("expected error for unknown gateway")
	}
	if _, err := NewOpencode(ProviderOpencodeZen, "", "m"); err == nil {
		t.Error("expected error for empty api key")
	}
	_, err := NewOpencode(ProviderOpencodeZen, "k", "m", WithOpencodeRoute(OpencodeRoute("bogus")))
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("invalid route override err = %v, want wrapping ErrInvalidRequest", err)
	}
}

// TestOpencode_NoContinuer pins the measured gateway behaviour: chaining via
// previous_response_id returns HTTP 400, so Continuer is deliberately absent.
func TestOpencode_NoContinuer(t *testing.T) {
	var i any = (*OpencodeProvider)(nil)
	if _, ok := i.(Continuer); ok {
		t.Error("OpencodeProvider must not implement Continuer: the gateway rejects " +
			"previous_response_id with HTTP 400 (measured 2026-08-28)")
	}
}

func TestOpencode_RetryOnRateLimit(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(fxOpencodeMessages))
	}))
	defer srv.Close()
	p, _ := NewOpencode(ProviderOpencodeZen, "k", "claude-sonnet-5", WithBaseURL(srv.URL))
	out, err := GenerateWithRetry(context.Background(), p, "hi", 2, time.Millisecond)
	if err != nil {
		t.Fatalf("GenerateWithRetry: %v", err)
	}
	if out != "hello" || calls != 2 {
		t.Errorf("out = %q after %d calls", out, calls)
	}
}
