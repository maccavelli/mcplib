package wizard

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/maccavelli/mcplib/llmprovider"
)

const grokVendorAuthFixture = `{
  "xai::api_key": {
    "key": "xai-MUST-SKIP",
    "auth_mode": "api_key"
  },
  "https://auth.x.ai::b1a00492-073a-47ea-816f-4c329264a828": {
    "key": "sess-grok",
    "auth_mode": "oidc",
    "refresh_token": "rt-grok",
    "expires_at": "2099-01-01T00:00:00Z",
    "oidc_issuer": "https://auth.x.ai",
    "oidc_client_id": "b1a00492-073a-47ea-816f-4c329264a828"
  }
}`

func TestImportGrok_SkipsAPIKeyScope(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(grokVendorAuthFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	session, err := importVendorSession(llmprovider.ProviderGrok, Options{
		LookupEnv: func(name string) string {
			if name == "GROK_HOME" {
				return dir
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("importVendorSession() error = %v", err)
	}
	if session.Access != "sess-grok" || session.Refresh != "rt-grok" || session.Access == "xai-MUST-SKIP" {
		t.Fatalf("imported access/refresh = %q/%q", session.Access, session.Refresh)
	}
	if session.Provider != llmprovider.ProviderGrok || session.Issuer != llmprovider.DefaultGrokOAuthIssuer ||
		session.ClientID != llmprovider.DefaultGrokOAuthClientID || session.Expiry.IsZero() {
		t.Fatalf("imported Grok metadata = %#v", session)
	}
}

func TestConfigureLLM_ImportConfirmsAndPersistsSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(grokVendorAuthFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	store := newMemoryTokenStore()
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderGrok), 4, 0},
		confirms: []bool{true},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{
		TokenStore: store,
		LookupEnv: func(name string) string {
			if name == "GROK_HOME" {
				return dir
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if res.Kind != CredOAuth || res.APIKey != "" || res.AccessToken != "sess-grok" {
		t.Errorf("result credential = %q/%q/%q", res.Kind, res.APIKey, res.AccessToken)
	}
	if store.saves != 1 || store.sessions[llmprovider.ProviderGrok] == nil {
		t.Errorf("TokenStore saves/session = %d/%v", store.saves, store.sessions[llmprovider.ProviderGrok])
	}
	assertTextMasksSecret(t, f.allText, "sess-grok")
}

func TestImportOpenAI_IgnoresPlatformKeyInAuthJSON(t *testing.T) {
	dir := t.TempDir()
	fixture := `{
  "OPENAI_API_KEY": "sk-MUST-IGNORE",
  "tokens": {
    "access_token": "at-chatgpt",
    "refresh_token": "rt-chatgpt",
    "account_id": "acct_test"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	session, err := importVendorSession(llmprovider.ProviderOpenAI, Options{
		LookupEnv: func(name string) string {
			if name == "CODEX_HOME" {
				return dir
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("importVendorSession() error = %v", err)
	}
	if session.Access != "at-chatgpt" || session.Refresh != "rt-chatgpt" || session.Access == "sk-MUST-IGNORE" {
		t.Fatalf("imported access/refresh = %q/%q", session.Access, session.Refresh)
	}
	if session.AccountID != "acct_test" || session.TokenURL != llmprovider.DefaultOpenAIIssuer+"/oauth/token" {
		t.Fatalf("imported OpenAI metadata = %#v", session)
	}
}
