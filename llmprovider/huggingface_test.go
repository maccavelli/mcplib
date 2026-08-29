package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const fxHFChat = `{"id":"cmpl-hf","choices":[{"message":{"role":"assistant","content":"hello"}}]}`

// TestHuggingFace_RequestShape pins the single endpoint this provider uses and
// fails loudly if /responses is ever requested — that endpoint returns HTTP 200
// with status:"failed" on auth errors, which would decode as a silent success.
func TestHuggingFace_RequestShape(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if r.URL.Path == "/responses" {
			t.Error("/responses must never be requested: it returns 200 on auth failure")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer hf_test" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer hf_test")
		}
		_, _ = w.Write([]byte(fxHFChat))
	}))
	defer srv.Close()

	p, err := NewHuggingFace("hf_test", "openai/gpt-oss-20b", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewHuggingFace: %v", err)
	}
	out, err := p.Generate(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "hello" {
		t.Errorf("output = %q", out)
	}
	if path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", path)
	}
	if p.Name() != ProviderHuggingFace {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestHuggingFace_ReasoningEffort(t *testing.T) {
	t.Run("thinking defaults to medium", func(t *testing.T) {
		var body map[string]any
		srv := captureServer(t, &body, fxHFChat)
		p, _ := NewHuggingFace("k", "openai/gpt-oss-20b", WithBaseURL(srv.URL))
		if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
			t.Fatalf("GenerateThinking: %v", err)
		}
		if body[jsonKeyReasoningEffort] != effortMedium {
			t.Errorf("reasoning_effort = %v, want %q", body[jsonKeyReasoningEffort], effortMedium)
		}
	})

	t.Run("WithReasoningEffort honoured", func(t *testing.T) {
		var body map[string]any
		srv := captureServer(t, &body, fxHFChat)
		p, _ := NewHuggingFace("k", "openai/gpt-oss-20b", WithBaseURL(srv.URL), WithReasoningEffort(effortXHigh))
		if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
			t.Fatalf("GenerateThinking: %v", err)
		}
		if body[jsonKeyReasoningEffort] != effortXHigh {
			t.Errorf("reasoning_effort = %v, want %q", body[jsonKeyReasoningEffort], effortXHigh)
		}
	})

	t.Run("non-thinking path sends no reasoning_effort", func(t *testing.T) {
		var body map[string]any
		srv := captureServer(t, &body, fxHFChat)
		p, _ := NewHuggingFace("k", "openai/gpt-oss-20b", WithBaseURL(srv.URL))
		if _, err := p.Generate(context.Background(), "hi"); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if _, present := body[jsonKeyReasoningEffort]; present {
			t.Errorf("reasoning_effort must be absent: %v", body[jsonKeyReasoningEffort])
		}
	})
}

func TestHuggingFace_ToolCall(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"choices":[{"message":{"role":"assistant","content":"",
		"tool_calls":[{"id":"c1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}]}}]}`)
	p, _ := NewHuggingFace("k", "openai/gpt-oss-20b", WithBaseURL(srv.URL))
	tool := Tool{Name: "get_weather", Description: "Get weather", Schema: map[string]any{"type": "object"}}

	args, err := p.GenerateWithTool(context.Background(), "weather?", tool)
	if err != nil {
		t.Fatalf("GenerateWithTool: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		t.Fatalf("arguments %q not valid JSON: %v", args, err)
	}
	if parsed["city"] != "SF" {
		t.Errorf("arguments = %q", args)
	}
	// Forced tool_choice must be present on this provider.
	tc, ok := body[jsonKeyToolChoice].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice missing: %v", body)
	}
	fn, _ := tc[jsonKeyFunction].(map[string]any)
	if fn[jsonKeyName] != "get_weather" {
		t.Errorf("tool_choice.function = %v", tc[jsonKeyFunction])
	}
}

// TestHuggingFace_ErrorClassification uses the measured 401 envelope, whose
// `error` field is a plain string rather than the OpenAI object.
func TestHuggingFace_ErrorClassification(t *testing.T) {
	tests := []struct {
		status  int
		wantErr error
	}{
		{http.StatusUnauthorized, ErrAuthFailure},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusInternalServerError, ErrProviderUnavailable},
		{http.StatusBadRequest, ErrInvalidRequest},
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":"Invalid username or password."}`))
			}))
			defer srv.Close()
			p, _ := NewHuggingFace("k", "openai/gpt-oss-20b", WithBaseURL(srv.URL))
			if _, err := p.Generate(context.Background(), "hi"); !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want wrapping %v", err, tc.wantErr)
			}
		})
	}
}

func TestHuggingFace_NoContinuer(t *testing.T) {
	var i any = (*HuggingFaceProvider)(nil)
	if _, ok := i.(Continuer); ok {
		t.Error("HuggingFaceProvider must not implement Continuer: Chat Completions is stateless")
	}
}

func TestSplitHuggingFaceModelPolicy(t *testing.T) {
	tests := []struct{ id, wantBase, wantPolicy string }{
		{"openai/gpt-oss-120b", "openai/gpt-oss-120b", ""},
		{"openai/gpt-oss-120b:groq", "openai/gpt-oss-120b", "groq"},
		{"openai/gpt-oss-120b:cheapest", "openai/gpt-oss-120b", "cheapest"},
		{"openai/gpt-oss-120b:fastest", "openai/gpt-oss-120b", "fastest"},
		{"no-slash", "no-slash", ""},
	}
	for _, tc := range tests {
		base, policy := splitHuggingFaceModelPolicy(tc.id)
		if base != tc.wantBase || policy != tc.wantPolicy {
			t.Errorf("split(%q) = (%q,%q), want (%q,%q)", tc.id, base, policy, tc.wantBase, tc.wantPolicy)
		}
	}
}

func TestStaticHuggingFace_Count(t *testing.T) {
	if len(StaticHuggingFace) == 0 || len(StaticHuggingFace) > MaxListedModels {
		t.Errorf("StaticHuggingFace has %d entries, want 1..%d", len(StaticHuggingFace), MaxListedModels)
	}
	for _, m := range StaticHuggingFace {
		if !isUsableHuggingFaceModel(m) {
			t.Errorf("%q fails its own usability filter", m)
		}
	}
}
