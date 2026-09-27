//go:build live_gateways

package llmprovider

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// xaiRecordingClient records the last POST body sent to api.x.ai.
func xaiRecordingClient(sent *[]byte) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && r.Body != nil {
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

// TestLive_GrokEffortMenu: grok-4.5 asked for xhigh sends the CLI menu's high
// and answers. xAI accepted every effort on grok-4.5 on 2026-09-27, so this
// proves conformance to the CLI, not a rejection avoided.
func TestLive_GrokEffortMenu(t *testing.T) {
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY unset")
	}
	var sent []byte
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewGrok(key, "grok-4.5", WithReasoningEffort(effortXHigh), WithHTTPClient(xaiRecordingClient(&sent)))
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateThinking: %v", err)
	}
	if !bytes.Contains(sent, []byte(`"reasoning":{"effort":"high"}`)) {
		t.Fatalf("request did not clamp xhigh to high: %s", sent)
	}
	if !strings.Contains(strings.ToUpper(out), "ALPHA") {
		t.Errorf("reply %q", out)
	}
}
