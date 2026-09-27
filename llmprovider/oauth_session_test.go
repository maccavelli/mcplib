package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidateOAuthSession_RejectsFixture(t *testing.T) {
	t.Parallel()

	accessOnly := func(expiry time.Time) *OAuthSession {
		return &OAuthSession{Access: "eyJhbGciOiJub25lIn0.e30.x", Issuer: DefaultOpenAIIssuer,
			ClientID: DefaultOpenAIClientID, Expiry: expiry}
	}
	for _, test := range []struct {
		name    string
		session *OAuthSession
		valid   bool
	}{
		{name: "nil", session: nil},
		{name: "empty access", session: &OAuthSession{Refresh: "refresh", ClientID: "client"}},
		{name: "chatgpt-access fixture", session: &OAuthSession{Access: "chatgpt-access", Issuer: DefaultOpenAIIssuer}},
		{name: "chatgpt-access fixture with refresh", session: &OAuthSession{Access: "chatgpt-access", Refresh: "refresh",
			Issuer: DefaultOpenAIIssuer, ClientID: DefaultOpenAIClientID}},
		{name: "no refresh outside ChatGPT", session: &OAuthSession{Access: "access", Issuer: DefaultGrokOAuthIssuer,
			ClientID: DefaultOpenAIClientID}},
		{name: "refreshable with token URL", session: &OAuthSession{Access: "access", Refresh: "refresh",
			ClientID: "client", TokenURL: "https://issuer.test/token"}, valid: true},
		{name: "refreshable with derived token URL", session: &OAuthSession{Access: "access", Refresh: "refresh",
			ClientID: DefaultGrokOAuthClientID, Issuer: DefaultGrokOAuthIssuer}, valid: true},
		{name: "refreshable without client id", session: &OAuthSession{Access: "access", Refresh: "refresh",
			TokenURL: "https://issuer.test/token"}},
		{name: "ChatGPT access-only", session: accessOnly(time.Time{}), valid: true},
		{name: "ChatGPT access-only with expiry", session: accessOnly(time.Now().Add(time.Hour))},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateOAuthSession(test.session)
			if (err == nil) != test.valid {
				t.Fatalf("ValidateOAuthSession() = %v, want valid=%t", err, test.valid)
			}
		})
	}
}

// jwtShapedDescription gives the redactor a token to remove from an error body.
const jwtShapedDescription = "eyJhbGciOiJub25lIn0.aaa.bbb"

func TestExchangeOAuthCode_IncludesRedactedBody(t *testing.T) {
	t.Parallel()
	srv := tokenErrorServer(t, `{"error":"invalid_grant","error_description":"`+jwtShapedDescription+`"}`)
	_, err := exchangeOAuthCode(context.Background(),
		oauthFlowConfig{provider: ProviderGrok, clientID: "test-client", httpClient: srv.Client()},
		srv.URL, "code", "http://127.0.0.1/callback", "verifier")
	assertRedactedTokenError(t, err)
}

func TestRefreshOAuthSession_IncludesRedactedBody(t *testing.T) {
	t.Parallel()
	srv := tokenErrorServer(t, `{"error":"invalid_grant","error_description":"`+jwtShapedDescription+`"}`)
	_, _, err := refreshOAuthSession(context.Background(), oauthSessionState{
		provider: ProviderGrok, refresh: "refresh", clientID: "test-client", tokenURL: srv.URL, httpClient: srv.Client(),
	})
	assertRedactedTokenError(t, err)
}

func TestOAuthTokenError_CapsBody(t *testing.T) {
	t.Parallel()
	srv := tokenErrorServer(t, strings.Repeat("A", 4096))
	_, err := exchangeOAuthCode(context.Background(),
		oauthFlowConfig{provider: ProviderGrok, clientID: "test-client", httpClient: srv.Client()},
		srv.URL, "code", "http://127.0.0.1/callback", "verifier")
	if err == nil {
		t.Fatal("exchangeOAuthCode() error = nil, want the 400")
	}
	if msg := err.Error(); !strings.Contains(msg, strings.Repeat("A", 2048)) || strings.Contains(msg, strings.Repeat("A", 2049)) {
		t.Fatalf("error carries %d body bytes, want exactly the first 2048", strings.Count(msg, "A"))
	}
}

func tokenErrorServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func assertRedactedTokenError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want the token endpoint's 400")
	}
	msg := err.Error()
	if !strings.Contains(msg, "400") || !strings.Contains(msg, "invalid_grant") {
		t.Errorf("error = %q, want the status and the body's invalid_grant", msg)
	}
	if strings.Contains(msg, "eyJ") {
		t.Errorf("error = %q leaks the JWT-shaped value", msg)
	}
}

type failingTokenStore struct {
	err          error
	savedAccess  string
	savedRefresh string
}

func (s *failingTokenStore) Load(context.Context, string) (*OAuthSession, error) {
	return nil, nil
}

func (s *failingTokenStore) Save(_ context.Context, _ string, session *OAuthSession) error {
	s.savedAccess = session.Access
	s.savedRefresh = session.Refresh
	return s.err
}

func (s *failingTokenStore) Delete(context.Context, string) error {
	return nil
}

func TestOAuthSession_RefreshPersistsBeforeReturn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := r.Form.Get("refresh_token"); got != "old-refresh" {
			t.Errorf("refresh_token = %q, want old-refresh", got)
		}
		if got := r.Form.Get("client_id"); got != "client-test" {
			t.Errorf("client_id = %q, want client-test", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    1800,
		})
	}))
	defer srv.Close()

	saveErr := errors.New("save failed")
	store := &failingTokenStore{err: saveErr}
	session := &OAuthSession{
		Provider:   "openai",
		Access:     "old-access",
		Refresh:    "old-refresh",
		Expiry:     time.Now().Add(-time.Minute),
		ClientID:   "client-test",
		TokenURL:   srv.URL,
		Store:      store,
		HTTPClient: srv.Client(),
	}

	_, err := session.Token(context.Background())
	if !errors.Is(err, saveErr) {
		t.Fatalf("Token error = %v, want %v", err, saveErr)
	}
	if session.Access != "old-access" {
		t.Errorf("session.Access = %q, want old-access", session.Access)
	}
	if session.Refresh != "old-refresh" {
		t.Errorf("session.Refresh = %q, want old-refresh", session.Refresh)
	}
	if store.savedAccess != "new-access" || store.savedRefresh != "new-refresh" {
		t.Errorf("saved token = (%q, %q), want (new-access, new-refresh)", store.savedAccess, store.savedRefresh)
	}
}

func TestOAuthSession_SingleFlight(t *testing.T) {
	var calls atomic.Int32
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		arrived <- struct{}{}
		<-release
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fresh-access",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	session := &OAuthSession{
		Provider:   "openai",
		Refresh:    "refresh",
		ClientID:   "client-test",
		TokenURL:   srv.URL,
		HTTPClient: srv.Client(),
	}
	type result struct {
		token Token
		err   error
	}
	results := make(chan result, 2)
	requestToken := func() {
		tok, err := session.Token(context.Background())
		results <- result{token: tok, err: err}
	}

	go requestToken()
	select {
	case <-arrived:
	case <-time.After(time.Second):
		t.Fatal("first refresh request did not arrive")
	}

	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		requestToken()
	}()
	<-secondStarted

	select {
	case <-arrived:
		// A second request is the failure asserted below after both calls return.
	case <-time.After(250 * time.Millisecond):
	}
	close(release)

	for range 2 {
		res := <-results
		if res.err != nil {
			t.Errorf("Token: %v", res.err)
		}
		if res.token.Value != "fresh-access" {
			t.Errorf("token value = %q, want fresh-access", res.token.Value)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("refresh request count = %d, want 1", got)
	}
}

func TestOAuthSession_SkewsFiveMinutes(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "refreshed-access",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	session := &OAuthSession{
		Provider:   "openai",
		Access:     "current-access",
		Refresh:    "refresh",
		ClientID:   "client-test",
		TokenURL:   srv.URL,
		HTTPClient: srv.Client(),
	}

	session.Expiry = time.Time{}
	tok, err := session.Token(context.Background())
	if err != nil {
		t.Fatalf("Token with zero expiry: %v", err)
	}
	if tok.Value != "current-access" || calls.Load() != 0 {
		t.Errorf("zero expiry token = %q, calls = %d; want current-access, 0", tok.Value, calls.Load())
	}

	session.Expiry = time.Now().Add(6 * time.Minute)
	tok, err = session.Token(context.Background())
	if err != nil {
		t.Fatalf("Token outside skew: %v", err)
	}
	if tok.Value != "current-access" || calls.Load() != 0 {
		t.Errorf("outside-skew token = %q, calls = %d; want current-access, 0", tok.Value, calls.Load())
	}

	session.Expiry = time.Now().Add(90 * time.Second)
	tok, err = session.Token(context.Background())
	if err != nil {
		t.Fatalf("Token inside skew: %v", err)
	}
	if tok.Value != "refreshed-access" || calls.Load() != 1 {
		t.Errorf("inside-skew token = %q, calls = %d; want refreshed-access, 1", tok.Value, calls.Load())
	}
}

func TestOAuthSession_ChatGPTDetectsIssuer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		issuer string
		want   bool
	}{
		{name: "exact", issuer: DefaultOpenAIIssuer, want: true},
		{name: "trailing slash", issuer: DefaultOpenAIIssuer + "/", want: true},
		{name: "different issuer", issuer: "https://auth.x.ai", want: false},
		{name: "empty", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &OAuthSession{Issuer: tc.issuer}
			if got := session.ChatGPT(); got != tc.want {
				t.Errorf("ChatGPT() = %t, want %t", got, tc.want)
			}
		})
	}
}
