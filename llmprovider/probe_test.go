package llmprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeGenerateHealth(t *testing.T) {
	// Empty candidates
	if res := probeGenerateHealth(context.Background(), nil, nil); res != nil {
		t.Errorf("expected nil for empty candidates, got %v", res)
	}

	// 2 candidates: m1 succeeds with "Hello", m2 fails with error, m3 succeeds without "hello"
	candidates := []string{"m1", "m2", "m3"}
	gen := func(_ context.Context, modelID string) (string, error) {
		switch modelID {
		case "m1":
			return "Hello world", nil
		case "m2":
			return "", fmt.Errorf("timeout")
		case "m3":
			return "bad response", nil
		}
		return "", nil
	}

	healthy := probeGenerateHealth(context.Background(), candidates, gen)
	if len(healthy) != 1 || healthy[0] != "m1" {
		t.Errorf("expected only [m1], got %v", healthy)
	}
}

func TestClaudeProvider_DiscoverModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-haiku-4-5"}]}`))
		case "/v1/messages", "/messages":
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Hello"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, _ := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL))
	models, err := p.DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected discovered models")
	}
}

func TestOpenAIProvider_DiscoverModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1-mini"}]}`))
		case "/responses":
			_, _ = w.Write([]byte(`{"id":"r1","output":[{"type":"message","content":[{"type":"output_text","text":"Hello"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, _ := NewOpenAI("k", "gpt-4.1-mini", WithBaseURL(srv.URL))
	models, err := p.DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected discovered models")
	}
}

func TestGeminiProvider_DiscoverModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-3.7-flash","supportedGenerationMethods":["generateContent"]}]}`))
		default:
			// generateContent
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}`))
		}
	}))
	defer srv.Close()

	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	models, err := p.DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected discovered models")
	}
}

func TestGrokProvider_DiscoverModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"grok-3-mini-fast"}]}`))
		case "/responses":
			_, _ = w.Write([]byte(`{"id":"r1","output":[{"type":"message","content":[{"type":"output_text","text":"Hello"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, _ := NewGrok("k", "grok-3-mini-fast", WithBaseURL(srv.URL))
	models, err := p.DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected discovered models")
	}
}
