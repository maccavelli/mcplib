package llmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGemini_KeyInHeaderNotURL is the #4 regression: the API key must travel in
// the x-goog-api-key header, never the URL query.
func TestGemini_KeyInHeaderNotURL(t *testing.T) {
	const key = "test-api-key-gemini-123"
	var gotKeyHeader, gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKeyHeader = r.Header.Get("x-goog-api-key")
		gotRawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
	}))
	defer srv.Close()

	p, _ := NewGemini(context.Background(), key, "gemini-x", WithBaseURL(srv.URL))
	out, err := p.Generate(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "ok" {
		t.Errorf("output: got %q", out)
	}
	if gotKeyHeader != key {
		t.Errorf("x-goog-api-key header: got %q want %q", gotKeyHeader, key)
	}
	if strings.Contains(gotRawQuery, key) || strings.Contains(gotRawQuery, "key=") {
		t.Errorf("key leaked into URL query: %q", gotRawQuery)
	}
}

// TestGemini_KeyNotInError confirms a transport error (*url.Error) does not
// embed the API key now that it is out of the URL.
func TestGemini_KeyNotInError(t *testing.T) {
	const key = "test-api-key-gemini-err"
	p, _ := NewGemini(context.Background(), key, "gemini-x", WithBaseURL("http://127.0.0.1:0"))
	_, err := p.Generate(context.Background(), "hi")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), key) {
		t.Errorf("API key leaked into error: %v", err)
	}
}
