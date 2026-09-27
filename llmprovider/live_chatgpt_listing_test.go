//go:build live_gateways

package llmprovider

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// TestLive_ChatGPTListingVersion is gate G-C item 7 through mcplib: a v1.5.0
// build lists with client_version 1.5.0, and that listing holds every model
// the 0.0.0 fallback lists (on 2026-09-27 it added gpt-6-sol and gpt-6-luna,
// whose minimal_client_version is 0.155.0), and each added model generates.
func TestLive_ChatGPTListingVersion(t *testing.T) {
	session := liveChatGPTSession(t)
	ctx, cancel := liveCtx(t)
	defer cancel()
	var sent []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent = append(sent, r.URL.Query().Get("client_version"))
		return http.DefaultTransport.RoundTrip(r)
	})}
	list := func(build string) []string {
		withMcplibVersion(t, build)
		models, err := ListAvailableModelsWithSource(ctx, ProviderOpenAI, session, WithHTTPClient(client))
		skipIfTransient(t, err)
		if err != nil {
			t.Fatalf("listing as %s: %v", build, err)
		}
		return models
	}
	fallback, release := list("(devel)"), list("v1.5.0")
	if !slices.Equal(sent, []string{"0.0.0", "1.5.0"}) {
		t.Fatalf("client_version sent = %v, want [0.0.0 1.5.0]", sent)
	}
	for _, id := range fallback {
		if !slices.Contains(release, id) {
			t.Errorf("%s listed for 0.0.0 but not for 1.5.0 (%v)", id, release)
		}
	}
	t.Logf("0.0.0: %v; 1.5.0: %v", fallback, release)
	// The models the release listing adds must be served.
	for _, id := range release {
		if slices.Contains(fallback, id) {
			continue
		}
		p, err := NewOpenAIWithSource(session, id)
		if err != nil {
			t.Fatal(err)
		}
		out, err := p.Generate(ctx, "Reply with only the word ALPHA")
		skipIfTransient(t, err)
		if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
			t.Errorf("%s: Generate = %q, %v", id, out, err)
		}
	}
}
