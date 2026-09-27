//go:build live_gateways

package llmprovider

import (
	"errors"
	"strings"
	"testing"
)

// TestLive_ChatGPTErrorDetail: the backend refuses a model outside the Codex
// catalog with 400 {"detail": ...}; the error carries that text.
func TestLive_ChatGPTErrorDetail(t *testing.T) {
	session := liveChatGPTSession(t)
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewOpenAIWithSource(session, "gpt-4.1-mini")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Generate(ctx, "hi")
	var apiErr *APIError
	if !errors.Is(err, ErrInvalidRequest) || !errors.As(err, &apiErr) || !strings.Contains(apiErr.Message, "not supported") {
		t.Fatalf("err = %v, want ErrInvalidRequest carrying the backend's detail", err)
	}
}
