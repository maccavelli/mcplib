package llmprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const fxOllamaChat = `{"id":"chatcmpl-1","object":"chat.completion","model":"llama3.2:latest",
"choices":[{"index":0,"message":{"role":"assistant","content":"ALPHA"}}]}`

// TestOllama_NoAuthHeader pins the property that makes Ollama the only
// credential-free provider: it requires no key and ignores any sent, so this
// provider sends none at all — for an empty AND a non-empty configured key.
func TestOllama_NoAuthHeader(t *testing.T) {
	for _, key := range []string{"", "ignored-by-ollama"} {
		t.Run("key="+key, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "" {
					t.Errorf("Authorization must never be sent to Ollama, got %q", got)
				}
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
				}
				_, _ = w.Write([]byte(fxOllamaChat))
			}))
			defer srv.Close()

			p, err := NewOllama(key, "llama3.2:latest", WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("NewOllama: %v", err)
			}
			if _, err := p.Generate(context.Background(), "hi"); err != nil {
				t.Fatalf("Generate: %v", err)
			}
		})
	}
}

// TestOllama_NeverForcesToolChoice pins the documented limitation: Ollama
// supports tools but not tool_choice, so a forced call would 400.
func TestOllama_NeverForcesToolChoice(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, fxOllamaChat)
	p, _ := NewOllama("", "llama3.2:latest", WithBaseURL(srv.URL))
	tool := Tool{Name: "get_weather", Schema: map[string]any{"type": "object"}}

	if _, err := p.GenerateItemsWithTool(context.Background(), tool,
		MessageItem{Role: jsonRoleUser, Text: "hi"}); err != nil {
		t.Fatalf("GenerateItemsWithTool: %v", err)
	}
	if _, ok := body[jsonKeyTools]; !ok {
		t.Error("tools must be offered")
	}
	if _, ok := body[jsonKeyToolChoice]; ok {
		t.Errorf("tool_choice must NEVER be sent: Ollama does not support it (got %v)",
			body[jsonKeyToolChoice])
	}
}

// TestOllama_ClampsXHighEffort: Ollama's vocabulary tops out at "max", not the
// "xhigh" this package models.
func TestOllama_ClampsXHighEffort(t *testing.T) {
	tests := []struct{ configured, want string }{
		{effortXHigh, ollamaMaxEffort},
		{effortMedium, effortMedium},
		{effortLow, effortLow},
		{"", effortMedium}, // default
	}
	for _, tc := range tests {
		var body map[string]any
		srv := captureServer(t, &body, fxOllamaChat)
		opts := []ProviderOption{WithBaseURL(srv.URL)}
		if tc.configured != "" {
			opts = append(opts, WithReasoningEffort(tc.configured))
		}
		p, _ := NewOllama("", "llama3.2:latest", opts...)
		if _, err := p.GenerateThinking(context.Background(), "hi"); err != nil {
			t.Fatalf("GenerateThinking: %v", err)
		}
		if got := body[jsonKeyReasoningEffort]; got != tc.want {
			t.Errorf("configured %q -> reasoning_effort %v, want %q", tc.configured, got, tc.want)
		}
	}
}

func TestOllama_Generate(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, fxOllamaChat)
	p, _ := NewOllama("", "llama3.2:latest", WithBaseURL(srv.URL))
	out, err := p.Generate(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "ALPHA" {
		t.Errorf("output = %q", out)
	}
	if p.Name() != ProviderOllama {
		t.Errorf("Name() = %q", p.Name())
	}
	// Non-thinking path sends no reasoning parameter.
	if _, ok := body[jsonKeyReasoningEffort]; ok {
		t.Error("reasoning_effort must be absent on the plain path")
	}
}

// TestOllama_EmptyKeyAccepted: unlike every other constructor in this package,
// an empty key is valid here.
func TestOllama_EmptyKeyAccepted(t *testing.T) {
	if _, err := NewOllama("", "llama3.2:latest"); err != nil {
		t.Errorf("NewOllama with an empty key must succeed: %v", err)
	}
	if _, err := NewProvider(ProviderOllama, "", "llama3.2:latest"); err != nil {
		t.Errorf("NewProvider(ollama, \"\") must succeed: %v", err)
	}
}

func TestOllama_ErrorClassification(t *testing.T) {
	tests := []struct {
		status  int
		wantErr error
	}{
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusInternalServerError, ErrProviderUnavailable},
		{http.StatusBadRequest, ErrInvalidRequest},
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			p, _ := NewOllama("", "llama3.2:latest", WithBaseURL(srv.URL))
			_, err := p.Generate(context.Background(), "hi")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want wrapping %v", err, tc.wantErr)
			}
			if tc.status != http.StatusTooManyRequests && !strings.Contains(err.Error(), ProviderOllama) {
				t.Errorf("error must name the provider: %v", err)
			}
		})
	}
}

func TestOllama_NoContinuer(t *testing.T) {
	var i any = (*OllamaProvider)(nil)
	if _, ok := i.(Continuer); ok {
		t.Error("OllamaProvider must not implement Continuer: Chat Completions is stateless")
	}
}

// TestDescriptors_EveryDescriptorIsConstructible is the payoff for adding a real
// OllamaProvider: no descriptor exists that a wizard can offer but NewProvider
// cannot build. Before this phase, "ollama" was listing-only and would have
// failed here.
func TestDescriptors_EveryDescriptorIsConstructible(t *testing.T) {
	for _, d := range Descriptors() {
		t.Run(d.ID, func(t *testing.T) {
			key := "test-key"
			if !d.RequiresAPIKey {
				key = ""
			}
			model := "test-model"
			if len(d.StaticModels) > 0 {
				model = d.StaticModels[0]
			}
			prov, err := NewProvider(d.ID, key, model)
			if err != nil {
				t.Fatalf("descriptor %q is offered to users but NewProvider fails: %v", d.ID, err)
			}
			if prov.Name() != d.ID {
				t.Errorf("Name() = %q, want %q", prov.Name(), d.ID)
			}
		})
	}
}
