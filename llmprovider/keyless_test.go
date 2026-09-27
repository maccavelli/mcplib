package llmprovider

import (
	"context"
	"testing"
)

// TestKeyless_KiloAndOpencodeSendAnonymousToken: without a key, Kilo sends
// its "anonymous" token and OpenCode Zen/Go their "public" one, each in the
// header its route reads (MADR 0012 §1.7).
func TestKeyless_KiloAndOpencodeSendAnonymousToken(t *testing.T) {
	rec := newHeaderRecorder(t)
	base := WithBaseURL(rec.srv.URL)
	kilo, err := NewKilo("", "some/model", base)
	if err != nil {
		t.Fatalf("NewKilo without a key: %v", err)
	}
	_, _ = kilo.Generate(context.Background(), "hello")
	routes := []OpencodeRoute{OpencodeRouteResponses, OpencodeRouteMessages, OpencodeRouteChatCompletions, OpencodeRouteGoogle}
	for _, route := range routes {
		p, err := NewOpencode(ProviderOpencodeZen, "", "some-model", base, WithOpencodeRoute(route))
		if err != nil {
			t.Fatalf("NewOpencode(%s) without a key: %v", route, err)
		}
		_, _ = p.Generate(context.Background(), "hello")
	}
	reqs := rec.requests()
	if len(reqs) != 1+len(routes) {
		t.Fatalf("requests = %d, want %d", len(reqs), 1+len(routes))
	}
	want := []struct{ name, value string }{
		{"Authorization", "Bearer anonymous"},
		{"Authorization", "Bearer public"},
		{"x-api-key", "public"},
		{"Authorization", "Bearer public"},
		{"x-goog-api-key", "public"},
	}
	for i, w := range want {
		if got := reqs[i].header.Get(w.name); got != w.value {
			t.Errorf("%s: %s = %q, want %q", reqs[i].path, w.name, got, w.value)
		}
	}
}

// TestKeyless_KeyStillWins: a supplied key is sent as is.
func TestKeyless_KeyStillWins(t *testing.T) {
	rec := newHeaderRecorder(t)
	p, err := NewKilo("real-key", "some/model", WithBaseURL(rec.srv.URL))
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	_, _ = p.Generate(context.Background(), "hello")
	if got := rec.requests()[0].header.Get("Authorization"); got != "Bearer real-key" {
		t.Fatalf("Authorization = %q, want Bearer real-key", got)
	}
}
