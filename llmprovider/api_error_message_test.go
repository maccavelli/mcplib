package llmprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProviders_ErrorCarriesServiceMessage: every provider's error keeps the
// service's own explanation instead of discarding the body (MADR 0012 §1.1,
// 0013 B3), and still matches the sentinel its status always mapped to.
func TestProviders_ErrorCarriesServiceMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model is unavailable"}}`))
	}))
	t.Cleanup(srv.Close)
	base := WithBaseURL(srv.URL)

	build := map[string]func() (Provider, error){
		"openai": func() (Provider, error) { return NewOpenAI("k", "gpt-4.1-mini", base) },
		"claude": func() (Provider, error) { return NewClaude("k", "claude-haiku-4-5", base) },
		"gemini": func() (Provider, error) {
			return NewGemini(context.Background(), "k", "gemini-3.7-flash", base)
		},
		"grok":        func() (Provider, error) { return NewGrok("k", "grok-4.5", base) },
		"kilo":        func() (Provider, error) { return NewKilo("k", "some/model", base) },
		"huggingface": func() (Provider, error) { return NewHuggingFace("k", "org/model", base) },
		"ollama":      func() (Provider, error) { return NewOllama("", "llama3", base) },
	}
	for _, route := range []OpencodeRoute{OpencodeRouteResponses, OpencodeRouteMessages,
		OpencodeRouteChatCompletions, OpencodeRouteGoogle} {
		build["opencode/"+string(route)] = func() (Provider, error) {
			return NewOpencode(ProviderOpencodeGo, "k", "some-model", base, WithOpencodeRoute(route))
		}
	}
	for name, newProvider := range build {
		t.Run(name, func(t *testing.T) {
			p, err := newProvider()
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			_, err = p.Generate(context.Background(), "hello")
			if err == nil || !strings.Contains(err.Error(), "model is unavailable") {
				t.Fatalf("error = %v, want it to carry the service's message", err)
			}
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("error = %v, want it to still match ErrInvalidRequest", err)
			}
		})
	}
}
