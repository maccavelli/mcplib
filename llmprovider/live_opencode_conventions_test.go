//go:build live_gateways

package llmprovider

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// recordingClient records the last POST body sent to an opencode.ai host.
func recordingClient(sent *[]byte) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "opencode.ai") && r.Method == http.MethodPost && r.Body != nil {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			*sent = b
			r.Body = io.NopCloser(bytes.NewReader(b))
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
}

// TestLive_OpencodeResponsesStoreFalse: OpenCode Go's Responses route accepts
// store:false (measured 2026-09-27 on gpt-6-luna, with and without it).
func TestLive_OpencodeResponsesStoreFalse(t *testing.T) {
	var sent []byte
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "gpt-6-luna", WithHTTPClient(recordingClient(&sent)))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !bytes.Contains(sent, []byte(`"store":false`)) {
		t.Fatalf("request did not send store:false: %s", sent)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("empty output")
	}
}

// TestLive_OpencodeMinimaxM3Adaptive: minimax-m3 on OpenCode Go's Messages
// route thinks under thinking.type "adaptive" (measured 2026-09-27: budget,
// adaptive and none are all accepted, so this proves acceptance and that
// reasoning is returned, not necessity).
func TestLive_OpencodeMinimaxM3Adaptive(t *testing.T) {
	var sent []byte
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "minimax-m3", WithHTTPClient(recordingClient(&sent)))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	res, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: "What is 17 * 23? Reply with the number."})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateItemsThinking: %v", err)
	}
	if !bytes.Contains(sent, []byte(`"thinking":{"type":"adaptive"}`)) {
		t.Fatalf("request did not think adaptively: %s", sent)
	}
	var reasoned bool
	for _, it := range res.Output {
		if r, ok := it.(ReasoningItem); ok && strings.TrimSpace(r.Text) != "" {
			reasoned = true
		}
	}
	if !reasoned {
		t.Errorf("no ReasoningItem in %#v", res.Output)
	}
	if !strings.Contains(res.OutputText(), "391") {
		t.Errorf("reply %q, want 391", res.OutputText())
	}
}
