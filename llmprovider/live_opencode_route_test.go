//go:build live_gateways

package llmprovider

import (
	"net/http"
	"strings"
	"sync"
	"testing"
)

// TestLive_OpencodeRoutesFromMetadata: on OpenCode Go, qwen3.8-max follows its
// metadata (no provider.npm, so chat_completions) where the 2026-08-28 table
// said messages. Gate G-O (2026-09-27) measured both routes answering for every
// Go qwen model, so the metadata route is safe to follow.
func TestLive_OpencodeRoutesFromMetadata(t *testing.T) {
	t.Setenv(envDisableModelMetadata, "0") // the real models.opencode.ai document decides
	var (
		mu    sync.Mutex
		paths []string
	)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "opencode.ai") && r.Method == http.MethodPost {
			mu.Lock()
			paths = append(paths, r.URL.Path)
			mu.Unlock()
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), liveModel(t, ProviderOpencodeGo, "qwen3.8-max"),
		WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || !strings.HasSuffix(paths[0], "/chat/completions") {
		t.Fatalf("request paths = %v, want one /chat/completions", paths)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("empty output")
	}
}
