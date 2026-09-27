package llmprovider

import (
	"context"
	"strings"
	"testing"
)

// TestIdentification_ClientInfoAndSessionOptions: WithClientInfo leads
// User-Agent and names Kilo's editor; WithSessionID reaches OpenCode and Kilo.
func TestIdentification_ClientInfoAndSessionOptions(t *testing.T) {
	rec := newHeaderRecorder(t)
	opts := []ProviderOption{WithBaseURL(rec.srv.URL), WithClientInfo("pcm", "1.2.3"), WithSessionID("s-1")}
	kilo, err := NewKilo("k", "some/model", opts...)
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	_, _ = kilo.Generate(context.Background(), "hello")
	opencode, err := NewOpencode(ProviderOpencodeGo, "k", "glm-5.3-flash", opts...)
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	_, _ = opencode.Generate(context.Background(), "hello")

	reqs := rec.requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	for _, r := range reqs {
		if ua := r.header.Get("User-Agent"); !strings.HasPrefix(ua, "pcm/1.2.3 (") || !strings.Contains(ua, ") mcplib/") {
			t.Errorf("%s: User-Agent = %q, want pcm/1.2.3 (…) mcplib/…", r.path, ua)
		}
	}
	if h := reqs[0].header; h.Get(kiloEditorHeader) != "pcm" || h.Get(kiloTaskHeader) != "s-1" {
		t.Errorf("kilo editor/task = %q/%q, want pcm/s-1", h.Get(kiloEditorHeader), h.Get(kiloTaskHeader))
	}
	if s := reqs[1].header.Get(opencodeSessionHeader); s != "s-1" {
		t.Errorf("opencode session = %q, want s-1", s)
	}
}
