package llmprovider

import (
	"context"
	"errors"
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

// hfListingFixture mirrors the real /v1/models shape. It deliberately contains
// a vision-language model, a model whose only provider is not live, and three
// text models with differing throughput so ordering can be asserted.
const hfListingFixture = `{"object":"list","data":[
 {"id":"org/vlm","architecture":{"input_modalities":["image","text"],"output_modalities":["text"]},
  "providers":[{"provider":"a","status":"live","supports_tools":true,"throughput":999,"first_token_latency_ms":10}]},
 {"id":"org/not-live","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "providers":[{"provider":"a","status":"queued","supports_tools":true,"throughput":888,"first_token_latency_ms":10}]},
 {"id":"org/slow","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "providers":[{"provider":"a","status":"live","supports_tools":true,"throughput":50,"first_token_latency_ms":900}]},
 {"id":"org/fast","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "providers":[{"provider":"a","status":"live","supports_tools":true,"throughput":300,"first_token_latency_ms":100}]},
 {"id":"org/mid","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "providers":[{"provider":"a","status":"live","supports_tools":true,"throughput":150,"first_token_latency_ms":400}]}]}`

// TestListHuggingFaceModels_MetadataCuration proves the curation is driven by
// published metadata rather than name heuristics: vision-language and non-live
// models are dropped, and survivors come back fastest-first. The ordering
// assertion also proves the nil rankFn preserves the caller's order.
func TestListHuggingFaceModels_MetadataCuration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(hfListingFixture))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderHuggingFace, "k", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels: %v", err)
	}
	for _, m := range models {
		if m == "org/vlm" {
			t.Error("vision-language model must be dropped via architecture.input_modalities")
		}
		if m == "org/not-live" {
			t.Error("model with no live provider must be dropped")
		}
	}
	want := []string{"org/fast", "org/mid", "org/slow"}
	if len(models) != len(want) {
		t.Fatalf("got %v, want %v", models, want)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("order = %v, want %v (descending throughput; nil rankFn must preserve it)", models, want)
		}
	}
}

// TestListHuggingFaceModels_NoToken pins that the router's /v1/models endpoint
// is public: the handler fails the test if a credential is sent.
func TestListHuggingFaceModels_NoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization must not be sent with an empty token: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(hfListingFixture))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderHuggingFace, "", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("listing with no token must succeed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty list with no credential")
	}
}

func TestListAvailableModels_HuggingFaceFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderHuggingFace, "k", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("fallback must not error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected static catalog fallback")
	}
}

// kiloListingFixture mirrors the real Kilo catalog: OpenRouter shape plus Kilo's
// extensions. It contains a vision model, a training-on-prompts model, a
// non-tool model, and three priced text models including the "-1" variable price.
const kiloListingFixture = `{"data":[
 {"id":"org/vlm","architecture":{"input_modalities":["image","text"],"output_modalities":["text"]},
  "pricing":{"completion":"0.000001"},"supported_parameters":["tools"],"mayTrainOnYourPrompts":false},
 {"id":"org/trains","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "pricing":{"completion":"0.0000001"},"supported_parameters":["tools"],"mayTrainOnYourPrompts":true},
 {"id":"org/no-tools","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "pricing":{"completion":"0.0000001"},"supported_parameters":["max_tokens"],"mayTrainOnYourPrompts":false},
 {"id":"org/dear","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "pricing":{"completion":"0.000002"},"supported_parameters":["tools"],"mayTrainOnYourPrompts":false},
 {"id":"org/cheap","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "pricing":{"completion":"0.0000005"},"supported_parameters":["tools","tool_choice"],"mayTrainOnYourPrompts":false},
 {"id":"kilo-auto/variable","architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "pricing":{"completion":"-1"},"supported_parameters":["tools"],"mayTrainOnYourPrompts":false}]}`

// TestListKiloModels_MetadataCuration asserts both documented traps and the
// policy exclusion: vision, training-on-prompts and non-tool models are dropped,
// survivors are cheapest-first, and "-1" variable pricing sorts LAST.
func TestListKiloModels_MetadataCuration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(kiloListingFixture))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderKilo, "k", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListAvailableModels: %v", err)
	}
	for _, m := range models {
		switch m {
		case "org/vlm":
			t.Error("vision-language model must be dropped")
		case "org/trains":
			t.Error("mayTrainOnYourPrompts model must be dropped (policy)")
		case "org/no-tools":
			t.Error("model without tools in supported_parameters must be dropped")
		}
	}
	want := []string{"org/cheap", "org/dear", "kilo-auto/variable"}
	if len(models) != len(want) {
		t.Fatalf("got %v, want %v", models, want)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("order = %v, want %v (cheapest first, \"-1\" last)", models, want)
		}
	}
}

// TestListKiloModels_NoAPIKey pins that Kilo's /models endpoint is public.
func TestListKiloModels_NoAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization must not be sent with an empty key: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(kiloListingFixture))
	}))
	defer srv.Close()

	models, err := ListAvailableModels(context.Background(), ProviderKilo, "", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("listing with no API key must succeed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected non-empty list with no credential")
	}
}

// TestIsUsableKiloModel_TrainingPolicy documents and pins the one judgement this
// package makes on the user's behalf: mayTrainOnYourPrompts models are excluded
// by the lister, not by the id-only filter.
func TestIsUsableKiloModel_TrainingPolicy(t *testing.T) {
	// The id-only filter knows nothing about the flag and must accept the id.
	if !isUsableKiloModel("org/trains") {
		t.Error("isUsableKiloModel is id-only; the training flag lives in the listing")
	}
	// The lister applies the policy. Asserted end-to-end above; this guards the
	// division of responsibility so the check is not silently moved or dropped.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(kiloListingFixture))
	}))
	defer srv.Close()
	models, _ := ListAvailableModels(context.Background(), ProviderKilo, "", WithBaseURL(srv.URL))
	for _, m := range models {
		if m == "org/trains" {
			t.Error("listKiloModels must exclude mayTrainOnYourPrompts models")
		}
	}
}

func TestKiloModelCapabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization must not be sent with an empty key: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(kiloListingFixture))
	}))
	defer srv.Close()

	caps, err := KiloModelCapabilities(context.Background(), "", "org/cheap", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("KiloModelCapabilities: %v", err)
	}
	if len(caps) != 2 || caps[0] != jsonKeyTools || caps[1] != jsonKeyToolChoice {
		t.Errorf("caps = %v, want [tools tool_choice]", caps)
	}

	if _, err := KiloModelCapabilities(context.Background(), "", "org/absent", WithBaseURL(srv.URL)); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("absent model err = %v, want wrapping ErrInvalidRequest", err)
	}
}
