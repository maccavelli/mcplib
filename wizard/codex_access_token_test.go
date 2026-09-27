package wizard

import (
	"context"
	"testing"

	"github.com/maccavelli/mcplib/llmprovider"
)

// TestConfigureLLM_IgnoresCodexAccessTokenEnv: with CODEX_ACCESS_TOKEN set
// and the environment allowed, token_stdin still asks for the credential;
// the variable is never offered (codex login/src/auth/access_token.rs:1-14).
func TestConfigureLLM_IgnoresCodexAccessTokenEnv(t *testing.T) {
	f := &fakePrompter{
		t:       t,
		selects: []int{providerIdx(t, llmprovider.ProviderOpenAI), 3, 0},
		secrets: []string{testKey},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{
		AllowEnv:   true,
		TokenStore: newMemoryTokenStore(),
		LookupEnv: func(name string) string {
			if name == "CODEX_ACCESS_TOKEN" {
				return "at-personal-access-token"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if len(f.seenConfirm) != 0 || len(f.seenSecret) != 1 || res.Kind != CredAPIKey || res.APIKey != testKey {
		t.Fatalf("confirms %q, secrets %d, credential %q/%q; want the pasted key and no CODEX_ACCESS_TOKEN offer",
			f.seenConfirm, len(f.seenSecret), res.Kind, res.APIKey)
	}
}

// TestConfigureLLM_TokenStdinStillAcceptsChatGPTToken: a pasted ChatGPT
// access token is still an access-only OAuth session.
func TestConfigureLLM_TokenStdinStillAcceptsChatGPTToken(t *testing.T) {
	store := newMemoryTokenStore()
	f := &fakePrompter{
		t:       t,
		selects: []int{providerIdx(t, llmprovider.ProviderOpenAI), 3},
		secrets: []string{"pasted-chatgpt-access"},
		inputs:  []string{"chatgpt-model"},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{TokenStore: store})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if res.Kind != CredOAuth || res.AccessToken != "pasted-chatgpt-access" || store.saves != 1 {
		t.Fatalf("credential %q/%q, saves %d; want an access-only session", res.Kind, res.AccessToken, store.saves)
	}
}
