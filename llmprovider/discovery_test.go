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

// opencodeListingFixture is the real GET /models shape: an OpenAI list whose
// entries carry no routing or capability metadata at all.
const opencodeListingFixture = `{"object":"list","data":[
	{"id":"gpt-5.4-nano","object":"model","created":1787968159,"owned_by":"opencode"},
	{"id":"claude-haiku-4-5","object":"model","created":1787968159,"owned_by":"opencode"},
	{"id":"gemini-3.7-flash","object":"model","created":1787968159,"owned_by":"opencode"},
	{"id":"kimi-k2.6","object":"model","created":1787968159,"owned_by":"opencode"},
	{"id":"deepseek-v4-flash-vision-exp","object":"model","created":1787968159,"owned_by":"opencode"}]}`

func TestListAvailableModels_OpencodeZen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(opencodeListingFixture))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderOpencodeZen, "k", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels: %v", err)
	}
	if len(models) == 0 || len(models) > MaxListedModels {
		t.Fatalf("got %d models, want 1..%d", len(models), MaxListedModels)
	}
	for _, m := range models {
		if m == "deepseek-v4-flash-vision-exp" {
			t.Errorf("denied model returned: %s", m)
		}
	}
}

func TestListAvailableModels_OpencodeGo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[
			{"id":"glm-5.3-flash","object":"model","owned_by":"opencode"},
			{"id":"grok-4.6","object":"model","owned_by":"opencode"}]}`))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderOpencodeGo, "k", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty model list")
	}
}

// TestListOpencodeModels_NoAPIKey pins the measured fact that the gateway's
// /models endpoint is public: the handler FAILS the test if a credential is
// sent, and the call must still succeed with an empty key.
func TestListOpencodeModels_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization must not be sent when no key is configured: %q",
				r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(opencodeListingFixture))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderOpencodeZen, "", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("listing with no API key must succeed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty model list with no credential")
	}
}

func TestListAvailableModels_OpencodeFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderOpencodeZen, "k", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("fallback must not error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected static catalog fallback")
	}
}
