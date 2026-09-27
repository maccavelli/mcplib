package llmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// conventionsBody runs one call against a stub gateway and returns the
// request body it sent.
func conventionsBody(t *testing.T, model string, thinking bool, opts ...ProviderOption) map[string]any {
	t.Helper()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
		default:
			_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	p, err := NewOpencode(ProviderOpencodeGo, "k", model, append([]ProviderOption{WithBaseURL(srv.URL)}, opts...)...)
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	if thinking {
		_, err = p.GenerateThinking(context.Background(), "hi")
	} else {
		_, err = p.Generate(context.Background(), "hi")
	}
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return body
}

// TestOpencodeResponses_SendsStoreFalse: OpenCode's client sets store false
// for every @ai-sdk/openai model (transform.ts:1235-1243).
func TestOpencodeResponses_SendsStoreFalse(t *testing.T) {
	body := conventionsBody(t, "gpt-6-luna", false)
	if v, ok := body["store"]; !ok || v != false {
		t.Fatalf("store = %v (present %t), want false", v, ok)
	}
}

// TestOpencodeMessages_MinimaxM3ThinksAdaptive: MiniMax's Anthropic interface
// takes thinking.type "adaptive" (transform.ts:1293-1296), with no budget and
// no effort, whatever effort is configured.
func TestOpencodeMessages_MinimaxM3ThinksAdaptive(t *testing.T) {
	body := conventionsBody(t, "minimax-m3", true, WithReasoningEffort(effortHigh))
	thinking, _ := body[jsonKeyThinking].(map[string]any)
	if len(thinking) != 1 || thinking[jsonKeyType] != "adaptive" {
		t.Fatalf("thinking = %v, want exactly {type: adaptive}", body[jsonKeyThinking])
	}
	if _, ok := body["output_config"]; ok {
		t.Errorf("output_config = %v, want absent", body["output_config"])
	}
}

// TestOpencodeMessages_OtherModelsKeepBudget: a non-Claude, non-MiniMax-M3
// model on the Messages route keeps the enabled budget.
func TestOpencodeMessages_OtherModelsKeepBudget(t *testing.T) {
	body := conventionsBody(t, "qwen3.8-flash", true)
	thinking, _ := body[jsonKeyThinking].(map[string]any)
	if thinking[jsonKeyType] != jsonKeyEnabled || thinking["budget_tokens"] == nil {
		t.Fatalf("thinking = %v, want an enabled budget", body[jsonKeyThinking])
	}
}
