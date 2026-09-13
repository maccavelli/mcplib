package wizard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/mcplib/llmprovider"
)

func TestConfigureLLM_OrchestratedReturnsErr(t *testing.T) {
	f := &fakePrompter{t: t}
	orchestrated := true
	_, err := ConfigureLLM(context.Background(), f, Options{Orchestrated: &orchestrated})
	if !errors.Is(err, ErrOrchestrated) {
		t.Fatalf("ConfigureLLM() error = %v, want ErrOrchestrated", err)
	}
	if len(f.seenSelect) != 0 {
		t.Fatalf("Select calls = %d, want 0", len(f.seenSelect))
	}
}

func TestConfigureLLM_ChatGPTResultDoesNotPopulateAPIKey(t *testing.T) {
	store := newMemoryTokenStore()
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderOpenAI), 1, 0},
		confirms: []bool{true},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{
			Provider:     llmprovider.ProviderOpenAI,
			Kind:         CredOAuth,
			AccessToken:  "existing-access-abcd",
			RefreshToken: "existing-refresh",
			TokenExpiry:  time.Now().Add(time.Hour),
			Issuer:       llmprovider.DefaultOpenAIIssuer,
			ClientID:     llmprovider.DefaultOpenAIClientID,
			AccountID:    "acct_test",
		},
		TokenStore: store,
		Discover:   true,
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if res.APIKey != "" || res.Kind != CredOAuth {
		t.Fatalf("APIKey/Kind = %q/%q, want empty/%q", res.APIKey, res.Kind, CredOAuth)
	}
	if res.AccessToken != "existing-access-abcd" || res.Model != llmprovider.StaticOpenAIChatGPT[0] {
		t.Fatalf("access/model = %q/%q", res.AccessToken, res.Model)
	}
	assertTextMasksSecret(t, f.allText, "existing-access-abcd")
}

func TestConfigureLLM_APIKeyKindUnchanged(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		selections []int
	}{
		{name: "gemini", provider: llmprovider.ProviderGemini},
		{name: "openai", provider: llmprovider.ProviderOpenAI, selections: []int{0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selects := []int{providerIdx(t, test.provider)}
			selects = append(selects, test.selections...)
			selects = append(selects, 0)
			f := &fakePrompter{t: t, selects: selects, secrets: []string{testKey}}
			res, err := ConfigureLLM(context.Background(), f, Options{})
			if err != nil {
				t.Fatalf("ConfigureLLM() error = %v", err)
			}
			if res.Kind != CredAPIKey || res.APIKey != testKey {
				t.Fatalf("Kind/APIKey = %q/%q, want %q/key", res.Kind, res.APIKey, CredAPIKey)
			}
		})
	}
}

func TestConfigureLLM_DoesNotOfferClaudeOAuth(t *testing.T) {
	f := &fakePrompter{
		t:       t,
		selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0},
		secrets: []string{testKey},
	}
	if _, err := ConfigureLLM(context.Background(), f, Options{}); err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if len(f.seenSelect) != 2 {
		t.Fatalf("Select calls = %d, want provider and model only", len(f.seenSelect))
	}
	for _, title := range f.seenSelect {
		if strings.Contains(strings.ToLower(title), "authentication") {
			t.Fatalf("Claude unexpectedly received auth menu %q", title)
		}
	}
}

func TestConfigureLLM_OAuthMethodsRequireTokenStore(t *testing.T) {
	for _, authIdx := range []int{1, 2, 4} {
		f := &fakePrompter{
			t:       t,
			selects: []int{providerIdx(t, llmprovider.ProviderOpenAI), authIdx},
		}
		_, err := ConfigureLLM(context.Background(), f, Options{})
		if err == nil || err.Error() != "wizard: TokenStore is required for OAuth" {
			t.Fatalf("auth index %d error = %v", authIdx, err)
		}
	}
}

func TestConfigureLLM_TokenStdinClassification(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		secret     string
		wantKind   CredentialKind
		wantAPIKey string
		wantAccess string
		wantSaves  int
	}{
		{
			name:       "openai platform key",
			provider:   llmprovider.ProviderOpenAI,
			secret:     "sk-platform",
			wantKind:   CredAPIKey,
			wantAPIKey: "sk-platform",
		},
		{
			name:       "openai access token",
			provider:   llmprovider.ProviderOpenAI,
			secret:     "chatgpt-access",
			wantKind:   CredOAuth,
			wantAccess: "chatgpt-access",
			wantSaves:  1,
		},
		{
			name:       "grok always api key",
			provider:   llmprovider.ProviderGrok,
			secret:     "xai-key",
			wantKind:   CredAPIKey,
			wantAPIKey: "xai-key",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryTokenStore()
			f := &fakePrompter{
				t:       t,
				selects: []int{providerIdx(t, test.provider), 3, 0},
				secrets: []string{test.secret},
			}
			res, err := ConfigureLLM(context.Background(), f, Options{TokenStore: store})
			if err != nil {
				t.Fatalf("ConfigureLLM() error = %v", err)
			}
			if res.Kind != test.wantKind || res.APIKey != test.wantAPIKey || res.AccessToken != test.wantAccess {
				t.Fatalf("result credential = %q/%q/%q", res.Kind, res.APIKey, res.AccessToken)
			}
			if store.saves != test.wantSaves {
				t.Fatalf("TokenStore saves = %d, want %d", store.saves, test.wantSaves)
			}
		})
	}
}

func TestConfigureLLM_TokenStdinOAuthRequiresTokenStore(t *testing.T) {
	f := &fakePrompter{
		t:       t,
		selects: []int{providerIdx(t, llmprovider.ProviderOpenAI), 3},
		secrets: []string{"chatgpt-access"},
	}
	_, err := ConfigureLLM(context.Background(), f, Options{})
	if err == nil || err.Error() != "wizard: TokenStore is required for OAuth" {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
}

func TestConfigureLLM_TokenStdinUsesCodexEnvironment(t *testing.T) {
	store := newMemoryTokenStore()
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderOpenAI), 3, 0},
		confirms: []bool{true},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{
		AllowEnv:   true,
		TokenStore: store,
		LookupEnv: func(name string) string {
			if name == "CODEX_ACCESS_TOKEN" {
				return "codex-access-abcd"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if res.Kind != CredOAuth || res.AccessToken != "codex-access-abcd" || len(f.seenSecret) != 0 || store.saves != 1 {
		t.Fatalf(
			"result credential = %q/%q; Secret calls = %d; TokenStore saves = %d",
			res.Kind, res.AccessToken, len(f.seenSecret), store.saves,
		)
	}
	assertTextMasksSecret(t, f.allText, "codex-access-abcd")
}

func TestConfigureLLM_BrowserAndDevicePersistSessions(t *testing.T) {
	originalBrowser := loginBrowserOAuth
	originalDevice := loginDeviceOAuth
	t.Cleanup(func() {
		loginBrowserOAuth = originalBrowser
		loginDeviceOAuth = originalDevice
	})

	loginBrowserOAuth = func(_ context.Context, provider string, opts llmprovider.OAuthFlowOptions) (*llmprovider.OAuthSession, error) {
		if opts.OpenURL == nil {
			t.Fatal("browser OpenURL hook is nil")
		}
		if err := opts.OpenURL("https://authorize.test"); err != nil {
			t.Fatalf("OpenURL() error = %v", err)
		}
		return testOAuthSession(provider), nil
	}
	loginDeviceOAuth = func(_ context.Context, provider string, opts llmprovider.OAuthFlowOptions) (*llmprovider.OAuthSession, error) {
		if opts.NotifyDevice == nil {
			t.Fatal("device NotifyDevice hook is nil")
		}
		opts.NotifyDevice("https://verify.test", "ABCD-EFGH")
		return testOAuthSession(provider), nil
	}

	for _, test := range []struct {
		name    string
		authIdx int
	}{
		{name: "browser", authIdx: 1},
		{name: "device", authIdx: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryTokenStore()
			f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), test.authIdx, 0}}
			res, err := ConfigureLLM(context.Background(), f, Options{TokenStore: store})
			if err != nil {
				t.Fatalf("ConfigureLLM() error = %v", err)
			}
			if res.Kind != CredOAuth || store.saves != 1 || store.sessions[llmprovider.ProviderGrok] == nil {
				t.Fatalf("Kind/saves/session = %q/%d/%v", res.Kind, store.saves, store.sessions[llmprovider.ProviderGrok])
			}
			if len(f.seenNotify) == 0 {
				t.Fatal("OAuth flow produced no user notification")
			}
		})
	}
}

func testOAuthSession(provider string) *llmprovider.OAuthSession {
	issuer := llmprovider.DefaultGrokOAuthIssuer
	clientID := llmprovider.DefaultGrokOAuthClientID
	if provider == llmprovider.ProviderOpenAI {
		issuer = llmprovider.DefaultOpenAIIssuer
		clientID = llmprovider.DefaultOpenAIClientID
	}
	return &llmprovider.OAuthSession{
		Provider: provider,
		Access:   "oauth-access",
		Refresh:  "oauth-refresh",
		Expiry:   time.Now().Add(time.Hour),
		Issuer:   issuer,
		ClientID: clientID,
	}
}

type memoryTokenStore struct {
	sessions map[string]*llmprovider.OAuthSession
	saves    int
}

func newMemoryTokenStore() *memoryTokenStore {
	return &memoryTokenStore{sessions: make(map[string]*llmprovider.OAuthSession)}
}

func (store *memoryTokenStore) Load(_ context.Context, provider string) (*llmprovider.OAuthSession, error) {
	return store.sessions[provider], nil
}

func (store *memoryTokenStore) Save(_ context.Context, provider string, session *llmprovider.OAuthSession) error {
	store.sessions[provider] = session
	store.saves++
	return nil
}

func (store *memoryTokenStore) Delete(_ context.Context, provider string) error {
	delete(store.sessions, provider)
	return nil
}

func assertTextMasksSecret(t *testing.T, displayed []string, secret string) {
	t.Helper()
	joined := strings.Join(displayed, "\n")
	if strings.Contains(joined, secret) {
		t.Fatalf("displayed text contains raw credential %q", secret)
	}
	if !strings.Contains(joined, "••••") {
		t.Fatalf("displayed text has no masked credential: %q", joined)
	}
}
