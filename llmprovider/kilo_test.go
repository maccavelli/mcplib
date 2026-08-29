package llmprovider

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fxKiloChat is the real body measured on kilo-auto/free: reasoning arrives as
// message.reasoning, with a structured reasoning_details[] restatement.
const fxKiloChat = `{"id":"gen_01","object":"chat.completion","model":"stepfun/step-3.7-flash",
"choices":[{"index":0,"message":{"role":"assistant","content":"ALPHA",
"reasoning":"Got it, the user said to say ALPHA only.",
"reasoning_details":[{"type":"reasoning.text","text":"Got it."}]}}]}`

func TestKilo_RequestShape(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		switch r.URL.Path {
		case "/responses", "/messages":
			t.Errorf("%s must never be requested: Kilo translates formats, so one "+
				"route reaches the whole catalog", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer kilo_test" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(fxKiloChat))
	}))
	defer srv.Close()

	p, err := NewKilo("kilo_test", "kilo-auto/free", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", path)
	}
	if p.Name() != ProviderKilo {
		t.Errorf("Name() = %q, want %q", p.Name(), ProviderKilo)
	}
}

// TestKilo_SupportedParameterGating covers the capability gating that makes
// Kilo's per-model supported_parameters actionable.
func TestKilo_SupportedParameterGating(t *testing.T) {
	tool := Tool{Name: "get_weather", Schema: map[string]any{"type": "object"}}
	tests := []struct {
		name                              string
		caps                              []string
		wantTools, wantChoice, wantEffort bool
	}{
		{"nil caps sends everything", nil, true, true, true},
		{"tools only", []string{jsonKeyTools}, true, false, false},
		{"tools and tool_choice", []string{jsonKeyTools, jsonKeyToolChoice}, true, true, false},
		{"all three", []string{jsonKeyTools, jsonKeyToolChoice, jsonKeyReasoningEffort}, true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			srv := captureServer(t, &body, fxKiloChat)
			opts := []ProviderOption{WithBaseURL(srv.URL)}
			if tc.caps != nil {
				opts = append(opts, WithKiloCapabilities(tc.caps...))
			}
			p, err := NewKilo("k", "kilo-auto/free", opts...)
			if err != nil {
				t.Fatalf("NewKilo: %v", err)
			}
			// Item-shaped call: this test inspects the request body, and the
			// fixture deliberately returns no tool_calls.
			if _, err := p.GenerateItemsWithToolThinking(context.Background(), tool,
				MessageItem{Role: jsonRoleUser, Text: "hi"}); err != nil {
				t.Fatalf("GenerateItemsWithToolThinking: %v", err)
			}
			if _, ok := body[jsonKeyTools]; ok != tc.wantTools {
				t.Errorf("tools present = %v, want %v", ok, tc.wantTools)
			}
			if _, ok := body[jsonKeyToolChoice]; ok != tc.wantChoice {
				t.Errorf("tool_choice present = %v, want %v", ok, tc.wantChoice)
			}
			if _, ok := body[jsonKeyReasoningEffort]; ok != tc.wantEffort {
				t.Errorf("reasoning_effort present = %v, want %v", ok, tc.wantEffort)
			}
		})
	}
}

// TestWithKiloCapabilities proves caps is reachable from the public API — the
// option, not an internal side effect, is what populates it.
func TestWithKiloCapabilities(t *testing.T) {
	p, err := NewKilo("k", "m", WithKiloCapabilities(jsonKeyTools))
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	if !p.supports(jsonKeyTools) {
		t.Error("declared capability must be supported")
	}
	if p.supports(jsonKeyToolChoice) {
		t.Error("undeclared capability must not be supported when caps is set")
	}

	unknown, _ := NewKilo("k", "m")
	if !unknown.supports(jsonKeyTools) || !unknown.supports(jsonKeyToolChoice) {
		t.Error("nil caps means unknown: every parameter must be sent")
	}
}

func TestKilo_Generate(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, fxKiloChat)
	p, _ := NewKilo("k", "kilo-auto/free", WithBaseURL(srv.URL))
	resp, err := p.GenerateItems(context.Background(), MessageItem{Role: jsonRoleUser, Text: "hi"})
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if len(resp.Output) != 2 {
		t.Fatalf("expected reasoning + message, got %d items", len(resp.Output))
	}
	if _, ok := resp.Output[0].(ReasoningItem); !ok {
		t.Errorf("output[0] = %T, want ReasoningItem (from message.reasoning)", resp.Output[0])
	}
	if got := resp.OutputText(); got != "ALPHA" {
		t.Errorf("OutputText() = %q", got)
	}
}

func TestKilo_ErrorClassification(t *testing.T) {
	// The measured 401 envelope, which is Kilo-specific rather than OpenAI-shaped.
	const kiloAuthErr = `{"error":{"code":"PAID_MODEL_AUTH_REQUIRED","message":"You need to sign in to use this model."},"error_type":"paid_model_auth_required"}`
	tests := []struct {
		status  int
		wantErr error
	}{
		{http.StatusUnauthorized, ErrAuthFailure},
		{http.StatusPaymentRequired, ErrInvalidRequest}, // documented "insufficient balance"
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusInternalServerError, ErrProviderUnavailable},
		{http.StatusBadRequest, ErrInvalidRequest},
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(kiloAuthErr))
			}))
			defer srv.Close()
			p, _ := NewKilo("k", "kilo-auto/free", WithBaseURL(srv.URL))
			if _, err := p.Generate(context.Background(), "hi"); !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want wrapping %v", err, tc.wantErr)
			}
		})
	}
}

func TestKilo_NoContinuer(t *testing.T) {
	var i any = (*KiloProvider)(nil)
	if _, ok := i.(Continuer); ok {
		t.Error("KiloProvider must not implement Continuer: Chat Completions is stateless")
	}
}

// TestKiloPriceRank pins trap 1: "-1" is variable pricing on the kilo-auto
// tiers and must sort LAST, not before free.
func TestKiloPriceRank(t *testing.T) {
	if got := kiloPriceRank("0"); got != 0 {
		t.Errorf(`kiloPriceRank("0") = %v, want 0`, got)
	}
	if got := kiloPriceRank("0.0000004"); got != 0.0000004 {
		t.Errorf(`kiloPriceRank("0.0000004") = %v`, got)
	}
	for _, s := range []string{"-1", "", "abc"} {
		if got := kiloPriceRank(s); got != math.MaxFloat64 {
			t.Errorf("kiloPriceRank(%q) = %v, want MaxFloat64 (sorts last)", s, got)
		}
	}
	if kiloPriceRank("-1") <= kiloPriceRank("0") {
		t.Error(`"-1" (variable) must sort after "0" (free), not before`)
	}
}

// TestIsUsableKiloModel_ColonFree pins trap 2: ":free" is part of the id, not a
// policy suffix. Kilo must never use splitHuggingFaceModelPolicy.
func TestIsUsableKiloModel_ColonFree(t *testing.T) {
	for _, id := range []string{"tencent/hy3:free", "nvidia/nemotron-3.5-lightning:free", "minimax/minimax-m3:free"} {
		if !isUsableKiloModel(id) {
			t.Errorf("%q must be usable: :free is part of the id", id)
		}
	}
	if !isUsableKiloModel("kilo-auto/free") {
		t.Error("kilo-auto/free must be usable")
	}
	for _, id := range []string{"deepseek/deepseek-v4-flash-vision-exp", "mimo/mimo-v2-omni", "no-slash", ""} {
		if isUsableKiloModel(id) {
			t.Errorf("%q must be rejected", id)
		}
	}
}

func TestRankKiloModel(t *testing.T) {
	if RankKiloModel("kilo-auto/small") <= RankKiloModel("meta-llama/llama-3.1-8b-instruct") {
		t.Error("kilo-auto managed tiers should rank above concrete ids")
	}
	if RankKiloModel("kilo-auto/small") <= RankKiloModel("kilo-auto/frontier") {
		t.Error("small should rank above frontier for hook latency")
	}
}

func TestStaticKilo_Count(t *testing.T) {
	if len(StaticKilo) == 0 || len(StaticKilo) > MaxListedModels {
		t.Errorf("StaticKilo has %d entries, want 1..%d", len(StaticKilo), MaxListedModels)
	}
	for _, m := range StaticKilo {
		if !isUsableKiloModel(m) {
			t.Errorf("%q fails its own usability filter", m)
		}
	}
}
