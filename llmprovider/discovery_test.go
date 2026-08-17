package llmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListAvailableModels_Gemini(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "key123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-3.7-flash", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-2.5-flash", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-embedding-001", "supportedGenerationMethods": ["embedContent"]}
			]
		}`))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderGemini, "key123", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}
	for _, m := range models {
		if m == "gemini-embedding-001" {
			t.Errorf("unusable model returned: %s", m)
		}
	}

	// Test fallback to static catalog on API error
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errSrv.Close()

	fallbackModels, err := ListAvailableModels(context.Background(), ProviderGemini, "key", WithBaseURL(errSrv.URL))
	if err != nil {
		t.Fatalf("fallback error: %v", err)
	}
	if len(fallbackModels) == 0 {
		t.Fatal("expected static catalog fallback")
	}
}

func TestListAvailableModels_OpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "gpt-4.1-mini"},
				{"id": "gpt-4o"},
				{"id": "dall-e-3"}
			]
		}`))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderOpenAI, "key123", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}

	// Test fallback on API error
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer errSrv.Close()

	fallbackModels, err := ListAvailableModels(context.Background(), ProviderOpenAI, "key", WithBaseURL(errSrv.URL))
	if err != nil {
		t.Fatalf("fallback error: %v", err)
	}
	if len(fallbackModels) == 0 {
		t.Fatal("expected static catalog fallback")
	}
}

func TestListAvailableModels_Claude(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "key123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "claude-haiku-4-5"},
				{"id": "claude-sonnet-5"},
				{"id": "claude-instant-1"}
			]
		}`))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderClaude, "key123", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}

	// Test fallback on API error
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer errSrv.Close()

	fallbackModels, err := ListAvailableModels(context.Background(), ProviderClaude, "key", WithBaseURL(errSrv.URL))
	if err != nil {
		t.Fatalf("fallback error: %v", err)
	}
	if len(fallbackModels) == 0 {
		t.Fatal("expected static catalog fallback")
	}
}

func TestListAvailableModels_Grok(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "grok-3-mini-fast"},
				{"id": "grok-4"},
				{"id": "grok-vision-beta"}
			]
		}`))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderGrok, "key123", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}

	// Test fallback on API error
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errSrv.Close()

	fallbackModels, err := ListAvailableModels(context.Background(), ProviderGrok, "key", WithBaseURL(errSrv.URL))
	if err != nil {
		t.Fatalf("fallback error: %v", err)
	}
	if len(fallbackModels) == 0 {
		t.Fatal("expected static catalog fallback")
	}
}

func TestListAvailableModels_Ollama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"models": [
				{"name": "llama3:latest"},
				{"name": "mistral:7b"}
			]
		}`))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), "ollama", "", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels(ollama) error: %v", err)
	}
	if len(models) != 2 || models[0] != "llama3:latest" {
		t.Errorf("unexpected models: %v", models)
	}

	// Test error response
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer errSrv.Close()

	_, err = ListAvailableModels(context.Background(), "ollama", "", WithBaseURL(errSrv.URL))
	if err == nil {
		t.Error("expected error for non-200 ollama response")
	}
}

func TestListAvailableModels_Unsupported(t *testing.T) {
	_, err := ListAvailableModels(context.Background(), "unsupported", "key")
	if err == nil {
		t.Error("expected error for unsupported provider")
	}
}

func TestValidateOllamaURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version": "0.1.24"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := ValidateOllamaURL(context.Background(), srv.URL); err != nil {
		t.Errorf("ValidateOllamaURL valid error: %v", err)
	}

	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errSrv.Close()

	if err := ValidateOllamaURL(context.Background(), errSrv.URL); err == nil {
		t.Error("expected error for non-200 status")
	}
}
