package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestClaude_MultiBlockContent is the regression for the Content[0] bug: a
// leading non-text (thinking) block must not cause an empty result.
func TestClaude_MultiBlockContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"thinking","text":""},{"type":"text","text":"the answer"}]}`))
	}))
	defer srv.Close()

	p, _ := NewClaude("k", "claude-x", WithBaseURL(srv.URL))
	out, err := p.Generate(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "the answer" {
		t.Errorf("expected concatenated text block, got %q", out)
	}
}

// bodyField spins up a server that records one top-level JSON field from the
// request body, then returns the supplied response.
func bodyCapture(t *testing.T, resp string) (*httptest.Server, *map[string]any) {
	t.Helper()
	captured := &map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		*captured = m
		_, _ = w.Write([]byte(resp))
	}))
	return srv, captured
}

// TestOpenAI_WithMaxTokens is the regression: WithMaxTokens must reach the body.
func TestOpenAI_WithMaxTokens(t *testing.T) {
	srv, body := bodyCapture(t, `{"choices":[{"message":{"content":"ok"}}]}`)
	defer srv.Close()
	p, _ := NewOpenAI("k", "gpt-x", WithBaseURL(srv.URL), WithMaxTokens(123))
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if mt, ok := (*body)["max_tokens"].(float64); !ok || int(mt) != 123 {
		t.Errorf("max_tokens not sent: %v", (*body)["max_tokens"])
	}
}

// TestGemini_WithMaxTokens is the regression for Gemini's generationConfig.
func TestGemini_WithMaxTokens(t *testing.T) {
	srv, body := bodyCapture(t, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
	defer srv.Close()
	p, _ := NewGemini(context.Background(), "k", "gemini-x", WithBaseURL(srv.URL), WithMaxTokens(123))
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	gc, ok := (*body)["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("generationConfig missing: %v", *body)
	}
	if mt, ok := gc["maxOutputTokens"].(float64); !ok || int(mt) != 123 {
		t.Errorf("maxOutputTokens not sent: %v", gc["maxOutputTokens"])
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("120"); got != 120*time.Second {
		t.Errorf("seconds: got %v", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("empty: got %v", got)
	}
	if got := parseRetryAfter("not-a-number"); got != 0 {
		t.Errorf("garbage: got %v", got)
	}
}

// TestRateLimitError_Classification: RateLimitError unwraps to ErrRateLimited
// (retryable) and is honored by the retry layer.
func TestRateLimitError_Classification(t *testing.T) {
	rl := &RateLimitError{RetryAfter: time.Millisecond, Status: 429}
	if !errors.Is(rl, ErrRateLimited) {
		t.Error("RateLimitError must unwrap to ErrRateLimited")
	}

	f := &fakeProvider{errs: []error{rl}} // fail once with RateLimitError, then succeed
	out, err := GenerateWithRetry(context.Background(), f, "p", 3, time.Nanosecond)
	if err != nil || out != "ok" {
		t.Fatalf("expected retry+success, got %q err=%v", out, err)
	}
	if f.calls != 2 {
		t.Errorf("expected 2 calls (1 rate-limited + 1 ok), got %d", f.calls)
	}
}
