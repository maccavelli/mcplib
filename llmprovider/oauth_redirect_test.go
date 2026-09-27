package llmprovider

import (
	"strings"
	"testing"
)

// TestBrowserListener_OpenAIRedirectsToLoopbackIP: Codex, with the same
// client id, redirects to 127.0.0.1 on 1455 or 1457 (codex
// login/src/server.rs:193, :77-79).
func TestBrowserListener_OpenAIRedirectsToLoopbackIP(t *testing.T) {
	listeners, redirect, path, err := browserListener(ProviderOpenAI)
	if err != nil {
		t.Skipf("OpenAI loopback ports busy: %v", err)
	}
	defer closeCallbackListeners(listeners...)
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/auth/callback") || path != "/auth/callback" {
		t.Fatalf("redirect = %q, path = %q; want http://127.0.0.1:{port}/auth/callback", redirect, path)
	}
}

// TestBrowserListener_GrokRedirectUnchanged: Grok keeps its ephemeral
// 127.0.0.1 /callback.
func TestBrowserListener_GrokRedirectUnchanged(t *testing.T) {
	listeners, redirect, path, err := browserListener(ProviderGrok)
	if err != nil {
		t.Fatal(err)
	}
	defer closeCallbackListeners(listeners...)
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/callback") || path != "/callback" {
		t.Fatalf("redirect = %q, path = %q", redirect, path)
	}
}
