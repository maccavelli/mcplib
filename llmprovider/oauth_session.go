package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/maccavelli/mcplib/logging"
)

const (
	// oauthRefreshSkew refreshes an access token this long before it
	// expires: five minutes, as Codex (login/src/auth/manager.rs:203-216) and
	// the Grok CLI (xai-grok-login/src/model.rs:9) do.
	oauthRefreshSkew         = 5 * time.Minute
	oauthAuthorizationHeader = "Authorization"
	// oauthErrorBodyLimit caps how much of a failed token response an error
	// carries (MADR 0009 D8).
	oauthErrorBodyLimit = 2048
	// oauthRefreshAttempts and oauthRefreshBackoff retry a refresh that
	// failed in transport, with 429 or with 5xx, as the Grok CLI does
	// (xai-grok-login/src/oidc/protocol.rs:430-438).
	oauthRefreshAttempts = 3
	oauthRefreshBackoff  = 200 * time.Millisecond
)

// oauthTerminalRefreshCodes mean the refresh token is dead: Codex's permanent
// failures (login/src/auth/manager.rs:1657-1690) and the Grok CLI's
// (xai-grok-login/src/oidc/refresh.rs:21-27).
var oauthTerminalRefreshCodes = []string{
	"invalid_grant", "invalid_client",
	"refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated",
}

// chatGPTAccessFixture is the stub access token a consumer test once wrote into
// a live token store (MADR 0009 F3, F8).
const chatGPTAccessFixture = "chatgpt-access"

// ValidateOAuthSession reports whether a session can be used for generation
// (MADR 0009 D7). It must be refreshable, or the explicit access-only ChatGPT
// token that token_stdin and CODEX_ACCESS_TOKEN produce, and never a stub. A
// refreshable session needs a client id. Its token URL may be empty, because
// the refresh derives it from the issuer.
func ValidateOAuthSession(session *OAuthSession) error {
	switch {
	case session == nil || strings.TrimSpace(session.Access) == "":
		return errors.New("oauth: session has no access token")
	case session.Access == chatGPTAccessFixture:
		return errors.New("oauth: session holds the chatgpt-access test fixture, not a real token")
	case session.Refresh == "":
		if strings.TrimRight(session.Issuer, "/") != DefaultOpenAIIssuer ||
			session.ClientID != DefaultOpenAIClientID || !session.Expiry.IsZero() {
			return errors.New("oauth: no refresh token")
		}
	case session.ClientID == "":
		return errors.New("oauth: refreshable session has no client id")
	}
	return nil
}

// oauthHTTPStatusError reports a failed token-endpoint response by its status
// and the first oauthErrorBodyLimit bytes of its body, redacted. It closes the
// body; the raw body is never logged.
func oauthHTTPStatusError(op string, resp *http.Response) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, oauthErrorBodyLimit))
	closeErr := resp.Body.Close()
	err := fmt.Errorf("oauth: %s failed: %s: %s", op, resp.Status,
		logging.RedactString(strings.TrimSpace(string(body))))
	if readErr != nil {
		err = errors.Join(err, fmt.Errorf("oauth: read %s response: %w", op, readErr))
	}
	if closeErr != nil {
		err = errors.Join(err, fmt.Errorf("oauth: close %s response: %w", op, closeErr))
	}
	return err
}

type oauthRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// Token returns the current bearer token, refreshing and persisting it when
// it is expired or within the refresh skew.
func (s *OAuthSession) Token(ctx context.Context) (Token, error) {
	s.mu.Lock()
	if token, ok := s.currentToken(); ok {
		s.mu.Unlock()
		return token, nil
	}
	if future := s.inflight; future != nil {
		s.mu.Unlock()
		select {
		case <-future.done:
			return future.tok, future.err
		case <-ctx.Done():
			return Token{}, ctx.Err()
		}
	}
	if s.Refresh == "" {
		s.mu.Unlock()
		return Token{}, errors.New("oauth: no refresh token")
	}

	future := &tokenFuture{done: make(chan struct{})}
	s.inflight = future
	state := s.refreshState()
	s.mu.Unlock()

	next, token, err := reloadOrRefresh(ctx, state)

	s.mu.Lock()
	if err == nil {
		s.adopt(next)
	}
	future.tok = token
	future.err = err
	s.inflight = nil
	close(future.done)
	s.mu.Unlock()

	return token, err
}

// ChatGPT reports whether the session was issued by the default OpenAI issuer.
func (s *OAuthSession) ChatGPT() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimRight(s.Issuer, "/") == DefaultOpenAIIssuer
}

func (s *OAuthSession) currentToken() (Token, bool) {
	if s.Access == "" {
		return Token{}, false
	}
	if !s.Expiry.IsZero() && time.Until(s.Expiry) <= oauthRefreshSkew {
		return Token{}, false
	}
	return Token{
		Value:  s.Access,
		Type:   TokenBearer,
		Expiry: s.Expiry,
		Header: oauthAuthorizationHeader,
	}, true
}

type oauthSessionState struct {
	provider   string
	refresh    string
	issuer     string
	clientID   string
	accountID  string
	fedramp    bool
	tokenURL   string
	store      TokenStore
	httpClient *http.Client
}

func (s *OAuthSession) refreshState() oauthSessionState {
	return oauthSessionState{
		provider:   s.Provider,
		refresh:    s.Refresh,
		issuer:     s.Issuer,
		clientID:   s.ClientID,
		accountID:  s.AccountID,
		fedramp:    s.FedRAMP,
		tokenURL:   s.TokenURL,
		store:      s.Store,
		httpClient: s.HTTPClient,
	}
}

func (s *OAuthSession) adopt(next *OAuthSession) {
	s.Provider = next.Provider
	s.Access = next.Access
	s.Refresh = next.Refresh
	s.Expiry = next.Expiry
	s.Issuer = next.Issuer
	s.ClientID = next.ClientID
	s.AccountID = next.AccountID
	s.FedRAMP = next.FedRAMP
	s.TokenURL = next.TokenURL
	s.Store = next.Store
	s.HTTPClient = next.HTTPClient
}

func refreshOAuthSession(ctx context.Context, state oauthSessionState) (*OAuthSession, Token, error) {
	var lastErr error
	for attempt := range oauthRefreshAttempts {
		if attempt > 0 {
			if err := sleepWithContext(ctx, oauthRefreshBackoff<<(attempt-1)); err != nil {
				return nil, Token{}, err
			}
		}
		next, token, retry, err := refreshOAuthSessionOnce(ctx, state)
		if !retry {
			return next, token, err
		}
		lastErr = err
	}
	return nil, Token{}, lastErr
}

// refreshOAuthSessionOnce makes one refresh request. retry reports a
// transport error, 429 or 5xx.
func refreshOAuthSessionOnce(ctx context.Context, state oauthSessionState) (next *OAuthSession, token Token, retry bool, err error) {
	req, err := newRefreshRequest(ctx, state)
	if err != nil {
		return nil, Token{}, false, err
	}
	client := state.httpClient
	if client == nil {
		client = defaultHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, Token{}, true, fmt.Errorf("oauth: refresh request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		retry = resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
		return nil, Token{}, retry, refreshFailure(resp)
	}

	var payload oauthRefreshResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
	closeErr := resp.Body.Close()
	if decodeErr != nil {
		err := fmt.Errorf("oauth: decode refresh response: %w", decodeErr)
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("oauth: close refresh response: %w", closeErr))
		}
		return nil, Token{}, false, err
	}
	if closeErr != nil {
		return nil, Token{}, false, fmt.Errorf("oauth: close refresh response: %w", closeErr)
	}
	if payload.AccessToken == "" {
		return nil, Token{}, false, errors.New("oauth: refresh response missing access token")
	}

	refresh := payload.RefreshToken
	if refresh == "" {
		refresh = state.refresh
	}
	next = &OAuthSession{
		Provider:   state.provider,
		Access:     payload.AccessToken,
		Refresh:    refresh,
		Expiry:     tokenExpiry(payload.AccessToken, payload.ExpiresIn, time.Now()),
		Issuer:     state.issuer,
		ClientID:   state.clientID,
		AccountID:  state.accountID,
		FedRAMP:    state.fedramp,
		TokenURL:   state.tokenURL,
		Store:      state.store,
		HTTPClient: state.httpClient,
	}
	return next, Token{
		Value:  next.Access,
		Type:   TokenBearer,
		Expiry: next.Expiry,
		Header: oauthAuthorizationHeader,
	}, false, nil
}

// newRefreshRequest builds the refresh request: JSON for the OpenAI issuer,
// as Codex sends it (login/src/oauth/client.rs:80-110), form-encoded
// otherwise, as the Grok CLI sends it (xai-grok-login/src/oidc/protocol.rs:492-507).
func newRefreshRequest(ctx context.Context, state oauthSessionState) (*http.Request, error) {
	params := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": state.refresh,
		"client_id":     state.clientID,
	}
	contentType := "application/x-www-form-urlencoded"
	var body io.Reader
	if strings.TrimRight(state.issuer, "/") == DefaultOpenAIIssuer {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("oauth: encode refresh request: %w", err)
		}
		contentType, body = "application/json", bytes.NewReader(raw)
	} else {
		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshTokenURL(state), body)
	if err != nil {
		return nil, fmt.Errorf("oauth: create refresh request: %w", err)
	}
	identityOf(ProviderConfig{}).setUserAgent(req)
	req.Header.Set("Content-Type", contentType)
	return req, nil
}

// refreshFailure reports a failed refresh response, closing its body. A 401
// or a terminal error code wraps ErrAuthFailure: the refresh token is dead
// and the user must sign in again.
func refreshFailure(resp *http.Response) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, oauthErrorBodyLimit))
	ignoreOAuthError(resp.Body.Close())
	resp.Body = io.NopCloser(bytes.NewReader(body))
	err := oauthHTTPStatusError("refresh", resp)
	if readErr != nil {
		err = errors.Join(err, fmt.Errorf("oauth: read refresh response: %w", readErr))
	}
	if resp.StatusCode == http.StatusUnauthorized || slices.Contains(oauthTerminalRefreshCodes, refreshErrorCode(body)) {
		return fmt.Errorf("%w: %w", ErrAuthFailure, err)
	}
	return err
}

// refreshErrorCode reads an OAuth error code: {"error": "code"},
// {"error": {"code": "code"}} or {"code": "code"}, lower-cased.
func refreshErrorCode(body []byte) string {
	var top struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
	}
	if json.Unmarshal(body, &top) != nil {
		return ""
	}
	if code := jsonString(top.Error); code != "" {
		return strings.ToLower(code)
	}
	var inner struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(top.Error, &inner) == nil && inner.Code != "" {
		return strings.ToLower(inner.Code)
	}
	return strings.ToLower(top.Code)
}

// tokenExpiry is the access token's JWT exp when it has one, else now plus
// expires_in (3600 s when absent), as Codex reads it (MADR 0012 §5.2).
func tokenExpiry(access string, expiresIn int64, now time.Time) time.Time {
	if exp := jwtExpiry(access); !exp.IsZero() {
		return exp
	}
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	return now.Add(time.Duration(expiresIn) * time.Second)
}

// reloadOrRefresh re-reads the session from its store before refreshing:
// another process sharing the store may already have rotated the refresh
// token, and sending the old one again trips the issuer's reuse detection.
// A stored session with a different refresh token is adopted, and refreshed
// only if its own access token is due (MADR 0012 §5.2).
func reloadOrRefresh(ctx context.Context, state oauthSessionState) (*OAuthSession, Token, error) {
	if state.store != nil {
		stored, err := state.store.Load(ctx, state.provider)
		if err == nil && stored != nil && stored.Refresh != "" && stored.Refresh != state.refresh {
			stored.Store, stored.HTTPClient = state.store, state.httpClient
			if token, ok := stored.currentToken(); ok {
				return stored, token, nil
			}
			state = stored.refreshState()
		}
	}
	next, token, err := refreshOAuthSession(ctx, state)
	if err == nil && state.store != nil {
		err = state.store.Save(ctx, state.provider, next)
	}
	return next, token, err
}

func refreshTokenURL(state oauthSessionState) string {
	if state.tokenURL != "" {
		return state.tokenURL
	}
	issuer := strings.TrimRight(state.issuer, "/")
	if issuer == DefaultOpenAIIssuer {
		return issuer + "/oauth/token"
	}
	return defaultGrokOAuthRefreshURL
}
