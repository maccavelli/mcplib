package wizard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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
		selects:  []int{providerIdx(t, llmprovider.ProviderOpenAI), 1},
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
			Model:        "kept-chatgpt-model",
		},
		TokenStore: store,
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if res.APIKey != "" || res.Kind != CredOAuth {
		t.Fatalf("APIKey/Kind = %q/%q, want empty/%q", res.APIKey, res.Kind, CredOAuth)
	}
	if res.AccessToken != "existing-access-abcd" || res.Model != "kept-chatgpt-model" {
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
	for _, authIdx := range []int{1, 2} {
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
		wantErr    bool
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
			secret:     jwtShapedAccess,
			wantKind:   CredOAuth,
			wantAccess: jwtShapedAccess,
			wantSaves:  1,
		},
		{
			name:     "chatgpt-access fixture",
			provider: llmprovider.ProviderOpenAI,
			secret:   "chatgpt-access",
			wantErr:  true,
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
			if test.wantKind == CredOAuth {
				f.inputs = []string{"chatgpt-model"}
			}
			res, err := ConfigureLLM(context.Background(), f, Options{TokenStore: store})
			if test.wantErr {
				if err == nil || store.saves != 0 {
					t.Fatalf("ConfigureLLM() error = %v with %d saves, want an error and no save", err, store.saves)
				}
				return
			}
			if err != nil {
				t.Fatalf("ConfigureLLM() error = %v", err)
			}
			if res.Kind != test.wantKind || res.APIKey != test.wantAPIKey || res.AccessToken != test.wantAccess {
				t.Fatalf("result credential = %q/%q/%q", res.Kind, res.APIKey, res.AccessToken)
			}
			if test.wantKind == CredOAuth && res.Model != "chatgpt-model" {
				t.Fatalf("oauth model = %q, want chatgpt-model", res.Model)
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

// TestConfigureLLM_ChatGPTListingFailurePromptsForModel: after ChatGPT sign-in
// a failed Codex listing asks for a model id; it never offers the Platform
// catalog or a frozen ChatGPT list (MADR 0009 D11).
func TestConfigureLLM_ChatGPTListingFailurePromptsForModel(t *testing.T) {
	var requests []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.Host+r.URL.Path)
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})}
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderOpenAI), 1},
		confirms: []bool{true},
		inputs:   []string{"manual-chatgpt"},
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
		TokenStore: newMemoryTokenStore(),
		Discover:   true,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if res.Model != "manual-chatgpt" {
		t.Fatalf("Model = %q, want the entered id", res.Model)
	}
	if len(requests) != 1 || strings.Contains(requests[0], "api.openai.com") {
		t.Fatalf("listing requests = %v, want one Codex /models call", requests)
	}
	for _, items := range f.seenSelectItems {
		for _, c := range items {
			if strings.Contains(c.Label, "gpt-5.4") || strings.Contains(c.Label, "gpt-4.1") {
				t.Fatalf("model menu offered %q after a failed ChatGPT listing", c.Label)
			}
		}
	}
	for _, n := range f.seenNotify {
		if strings.Contains(n, "built-in catalog") {
			t.Fatalf("notice %q claims a built-in catalog ChatGPT does not have", n)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// jwtShapedAccess is an access-only ChatGPT token as token_stdin receives it.
const jwtShapedAccess = "eyJhbGciOiJub25lIn0.e30.x"

func TestSaveOAuthCredential_RejectsFixture(t *testing.T) {
	store := newMemoryTokenStore()
	_, err := saveOAuthCredential(context.Background(), store, llmprovider.ProviderOpenAI, &llmprovider.OAuthSession{
		Access: "chatgpt-access",
		Issuer: llmprovider.DefaultOpenAIIssuer,
	})
	if err == nil || store.saves != 0 {
		t.Fatalf("saveOAuthCredential() error = %v with %d saves, want an error and no save", err, store.saves)
	}
	if msg := err.Error(); !strings.Contains(msg, "chatgpt-access") && !strings.Contains(msg, "stub") &&
		!strings.Contains(msg, "refresh") {
		t.Fatalf("error = %q, want it to name the fixture or the missing refresh", msg)
	}
}

// TestConfigureLLM_KeepRefusesStubSession: a saved stub is not kept (D7); the
// user must sign in again.
func TestConfigureLLM_KeepRefusesStubSession(t *testing.T) {
	store := newMemoryTokenStore()
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOpenAI), 1}, confirms: []bool{true},
		inputs: []string{"chatgpt-model"},
	}
	_, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{
			Provider:    llmprovider.ProviderOpenAI,
			Kind:        CredOAuth,
			AccessToken: "chatgpt-access",
			Issuer:      llmprovider.DefaultOpenAIIssuer,
			ClientID:    llmprovider.DefaultOpenAIClientID,
		},
		TokenStore: store,
	})
	if err == nil || !strings.Contains(err.Error(), "sign in again") || store.saves != 0 {
		t.Fatalf("ConfigureLLM() error = %v with %d saves, want the stub refused", err, store.saves)
	}
}

// TestConfigureLLM_BrowserOAuthSetsInputCode: browser sign-in races a paste
// prompt, and the authorize URL and paste instruction are shown even when the
// consumer opens the browser itself (MADR 0009 D3).
func TestConfigureLLM_BrowserOAuthSetsInputCode(t *testing.T) {
	stubBrowserLogin(t, func(_ context.Context, provider string, opts llmprovider.OAuthFlowOptions) (*llmprovider.OAuthSession, error) {
		if opts.InputCode == nil {
			t.Fatal("browser OAuth has no paste-code InputCode")
		}
		if err := opts.OpenURL("https://authorize.test"); err != nil {
			t.Fatalf("OpenURL() error = %v", err)
		}
		return testOAuthSession(provider), nil
	})
	opened := 0
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), 1, 0}}
	_, err := ConfigureLLM(context.Background(), f, Options{
		TokenStore: newMemoryTokenStore(),
		OpenURL:    func(string) error { opened++; return nil },
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if opened != 1 {
		t.Errorf("consumer OpenURL called %d times, want 1", opened)
	}
	if countContaining(f.seenNotify, "https://authorize.test") != 1 {
		t.Errorf("notices = %v, want the authorize URL shown once", f.seenNotify)
	}
	pasteHint := false
	for _, n := range f.seenNotify {
		pasteHint = pasteHint || strings.Contains(strings.ToLower(n), "paste")
	}
	if !pasteHint {
		t.Errorf("notices = %v, want a paste instruction", f.seenNotify)
	}
}

// TestConfigureLLM_BrowserLoopbackWinDrainsPastePrompt: when the loopback wins
// while the paste prompt is still reading, the wizard asks for Enter and waits
// for that read before its next prompt, so two reads never share the input.
func TestConfigureLLM_BrowserLoopbackWinDrainsPastePrompt(t *testing.T) {
	p := &drainPrompter{
		fakePrompter: &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), 1, 0}},
		inputStarted: make(chan struct{}),
		release:      make(chan struct{}),
	}
	stubBrowserLogin(t, func(ctx context.Context, provider string, opts llmprovider.OAuthFlowOptions) (*llmprovider.OAuthSession, error) {
		if opts.InputCode == nil {
			t.Fatal("browser OAuth has no paste-code InputCode")
		}
		if err := opts.OpenURL("https://authorize.test"); err != nil {
			t.Fatalf("OpenURL() error = %v", err)
		}
		go func() { _, _ = opts.InputCode(ctx) }()
		<-p.inputStarted
		return testOAuthSession(provider), nil // the loopback won
	})
	res, err := ConfigureLLM(context.Background(), p, Options{TokenStore: newMemoryTokenStore()})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if res.Kind != CredOAuth || countContaining(p.seenNotify, "press Enter to continue") != 1 {
		t.Fatalf("Kind = %q, notices = %v; want OAuth and one press-Enter notice", res.Kind, p.seenNotify)
	}
}

// drainPrompter's Input blocks like a terminal read until the wizard asks for
// the Enter that finishes it, and its Select fails a prompt that starts while
// that read is pending.
type drainPrompter struct {
	*fakePrompter
	inputStarted chan struct{}
	release      chan struct{}
	releaseOnce  sync.Once
	reading      atomic.Bool
}

func (d *drainPrompter) Input(prompt, def string) (string, error) {
	v, err := d.fakePrompter.Input(prompt, def)
	if prompt != pasteCodePrompt {
		return v, err
	}
	d.reading.Store(true)
	close(d.inputStarted)
	<-d.release
	d.reading.Store(false)
	return v, err
}

func (d *drainPrompter) Notify(level Level, format string, args ...any) {
	d.fakePrompter.Notify(level, format, args...)
	if strings.Contains(fmt.Sprintf(format, args...), "press Enter") {
		d.releaseOnce.Do(func() { close(d.release) })
	}
}

func (d *drainPrompter) Select(title string, choices []Choice, defaultIdx int) (int, error) {
	if d.reading.Load() {
		d.t.Errorf("Select(%q) started while the paste prompt was still reading", title)
	}
	return d.fakePrompter.Select(title, choices, defaultIdx)
}

func stubBrowserLogin(t *testing.T, login func(context.Context, string, llmprovider.OAuthFlowOptions) (*llmprovider.OAuthSession, error)) {
	t.Helper()
	original := loginBrowserOAuth
	t.Cleanup(func() { loginBrowserOAuth = original })
	loginBrowserOAuth = login
}
