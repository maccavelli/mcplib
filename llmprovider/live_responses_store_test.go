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

// TestLive_ResponsesStoreFalse: OpenAI (API key) and xAI accept store:false
// from WithStore(false).
func TestLive_ResponsesStoreFalse(t *testing.T) {
	for _, tc := range []struct{ name, env, model string }{
		{ProviderOpenAI, "OPENAI_API_KEY", "gpt-6-luna"},
		{ProviderGrok, "XAI_API_KEY", "grok-4.6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := os.Getenv(tc.env)
			if key == "" {
				t.Skipf("%s unset", tc.env)
			}
			var sent []byte
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPost && r.Body != nil {
					b, err := io.ReadAll(r.Body)
					if err != nil {
						return nil, err
					}
					sent = b
					r.Body = io.NopCloser(bytes.NewReader(b))
				}
				return http.DefaultTransport.RoundTrip(r)
			})}
			ctx, cancel := liveCtx(t)
			defer cancel()
			p, err := NewProvider(tc.name, key, tc.model, WithStore(false), WithHTTPClient(client))
			if err != nil {
				t.Fatal(err)
			}
			out, err := p.Generate(ctx, "Reply with only the word ALPHA")
			skipIfTransient(t, err)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if !bytes.Contains(sent, []byte(`"store":false`)) || !strings.Contains(strings.ToUpper(out), "ALPHA") {
				t.Fatalf("out = %q, sent %s", out, sent)
			}
		})
	}
}
