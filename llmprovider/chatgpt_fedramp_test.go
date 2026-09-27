package llmprovider

import (
	"context"
	"testing"
	"time"
)

// chatGPTLoginSession is the session a ChatGPT login yields for an id token
// with these auth claims.
func chatGPTLoginSession(t *testing.T, auth map[string]any) *OAuthSession {
	t.Helper()
	session, err := oauthSessionFromResponse(
		oauthFlowConfig{provider: ProviderOpenAI, issuer: DefaultOpenAIIssuer, clientID: DefaultOpenAIClientID, now: time.Now},
		DefaultOpenAIIssuer+"/oauth/token",
		oauthTokenResponse{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 3600,
			IDToken: openAITestJWT(t, map[string]any{"https://api.openai.com/auth": auth})})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// fedrampHeader returns X-OpenAI-Fedramp as one ChatGPT call sent it.
func fedrampHeader(t *testing.T, session *OAuthSession) (string, bool) {
	t.Helper()
	client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
	p, err := NewOpenAIWithSource(session, "gpt-6-astra", WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.header["X-Openai-Fedramp"]
	if !ok {
		return "", false
	}
	return v[0], true
}

// TestOpenAIChatGPT_FedRAMPHeader: an id token with chatgpt_account_is_fedramp
// sends X-OpenAI-Fedramp: true, as Codex's bearer auth does
// (model-provider/src/bearer_auth_provider.rs:43-45; login/src/token_data.rs).
func TestOpenAIChatGPT_FedRAMPHeader(t *testing.T) {
	session := chatGPTLoginSession(t, map[string]any{"chatgpt_account_id": "acct", "chatgpt_account_is_fedramp": true})
	if v, ok := fedrampHeader(t, session); !ok || v != "true" {
		t.Fatalf("X-OpenAI-Fedramp = %q (present %t), want true", v, ok)
	}
}

// TestOpenAIChatGPT_NoFedRAMPHeader: without the claim, no header.
func TestOpenAIChatGPT_NoFedRAMPHeader(t *testing.T) {
	session := chatGPTLoginSession(t, map[string]any{"chatgpt_account_id": "acct"})
	if v, ok := fedrampHeader(t, session); ok {
		t.Fatalf("X-OpenAI-Fedramp = %q, want absent", v)
	}
}
