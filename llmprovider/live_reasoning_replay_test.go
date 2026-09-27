//go:build live_gateways

package llmprovider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive_InterleavedReasoningReplay: an interleaved Go chat model accepts the
// replayed reasoning on the assistant tool-call turn and answers from the
// result. Measured 2026-09-27: kimi-k2.6 accepts the turn with or without the
// field, so this proves acceptance, not necessity.
func TestLive_InterleavedReasoningReplay(t *testing.T) {
	key := os.Getenv("OPENCODE_API_KEY")
	if key == "" {
		t.Skip("OPENCODE_API_KEY unset")
	}
	t.Setenv(envDisableModelMetadata, "0") // the real models.opencode.ai document decides
	var sent []byte
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/chat/completions") && r.Body != nil {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			sent = b
			r.Body = io.NopCloser(bytes.NewReader(b))
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	p, err := NewOpencode(ProviderOpencodeGo, key, liveModel(t, ProviderOpencodeGo, "kimi-k2.6"), WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := p.GenerateItems(ctx,
		MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris? Use the tool."},
		ReasoningItem{Text: "The user wants Paris weather; call get_weather."},
		FunctionCallItem{CallID: "call_rp_1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
		FunctionCallOutputItem{CallID: "call_rp_1", Output: `{"forecast":"sunny, 21C"}`})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if !bytes.Contains(sent, []byte(`"reasoning_content":"The user wants Paris weather; call get_weather."`)) {
		t.Fatalf("request did not replay the reasoning: %s", sent)
	}
	if text := strings.ToLower(res.OutputText()); !strings.Contains(text, "sunny") && !strings.Contains(text, "21") {
		t.Fatalf("reply %q does not use the tool result", res.OutputText())
	}
}
