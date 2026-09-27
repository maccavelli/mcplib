package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// kiloBody runs one Generate against a stub gateway and returns the request
// body it sent.
func kiloBody(t *testing.T, opts ...ProviderOption) map[string]any {
	t.Helper()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)
	p, err := NewKilo("k", "deepseek/deepseek-v4.1-flash", append([]ProviderOption{WithBaseURL(srv.URL)}, opts...)...)
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return body
}

// TestKilo_DeniesDataCollectionByDefault: Kilo's opt-out from upstreams that
// train on prompts is sent by default, matching the listing's policy.
func TestKilo_DeniesDataCollectionByDefault(t *testing.T) {
	provider, _ := kiloBody(t)["provider"].(map[string]any)
	if provider["data_collection"] != "deny" {
		t.Fatalf("provider = %v, want data_collection deny", provider)
	}
}

// kiloDataCollectionRequired is gate G-K's response to deny on a model that
// requires collection (kilo-auto/free, 2026-09-27).
const kiloDataCollectionRequired = `{"error":"Data collection is required for this model. Please enable data collection to use this model or choose another model.","error_type":"data_collection_required","message":"Data collection is required for this model. Please enable data collection to use this model or choose another model."}`

// TestClassify_KiloDataCollectionRequired: the refusal is a terminal
// ErrNotPermitted carrying Kilo's error_type and message.
func TestClassify_KiloDataCollectionRequired(t *testing.T) {
	err := classifyHTTPError(ProviderKilo, &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(kiloDataCollectionRequired)),
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T %v, want *APIError", err, err)
	}
	if !errors.Is(err, ErrNotPermitted) || !apiErr.Terminal {
		t.Errorf("err = %v (terminal %t), want terminal ErrNotPermitted", err, apiErr.Terminal)
	}
	if apiErr.Type != "data_collection_required" || !strings.HasPrefix(apiErr.Message, "Data collection is required") {
		t.Errorf("Type = %q, Message = %q", apiErr.Type, apiErr.Message)
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("err = %v, want it to still match the 400 status sentinel ErrInvalidRequest", err)
	}
}
