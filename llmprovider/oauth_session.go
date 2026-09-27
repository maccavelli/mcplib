package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maccavelli/mcplib/logging"
)

const (
	oauthRefreshSkew         = 2 * time.Minute
	oauthAuthorizationHeader = "Authorization"
	// oauthErrorBodyLimit caps how much of a failed token response an error
	// carries (MADR 0009 D8).
	oauthErrorBodyLimit = 2048
)

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

	next, token, err := refreshOAuthSession(ctx, state)
	if err == nil && state.store != nil {
		err = state.store.Save(ctx, state.provider, next)
	}

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
	s.TokenURL = next.TokenURL
	s.Store = next.Store
	s.HTTPClient = next.HTTPClient
}

func refreshOAuthSession(ctx context.Context, state oauthSessionState) (*OAuthSession, Token, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {state.refresh},
		"client_id":     {state.clientID},
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		refreshTokenURL(state),
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, Token{}, fmt.Errorf("oauth: create refresh request: %w", err)
	}
	identityOf(ProviderConfig{}).setUserAgent(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := state.httpClient
	if client == nil {
		client = defaultHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, Token{}, fmt.Errorf("oauth: refresh request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, Token{}, oauthHTTPStatusError("refresh", resp)
	}

	var payload oauthRefreshResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
	closeErr := resp.Body.Close()
	if decodeErr != nil {
		err := fmt.Errorf("oauth: decode refresh response: %w", decodeErr)
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("oauth: close refresh response: %w", closeErr))
		}
		return nil, Token{}, err
	}
	if closeErr != nil {
		return nil, Token{}, fmt.Errorf("oauth: close refresh response: %w", closeErr)
	}
	if payload.AccessToken == "" {
		return nil, Token{}, errors.New("oauth: refresh response missing access token")
	}

	refresh := payload.RefreshToken
	if refresh == "" {
		refresh = state.refresh
	}
	expiresIn := payload.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	expiry := time.Now().Add(time.Duration(expiresIn) * time.Second)
	next := &OAuthSession{
		Provider:   state.provider,
		Access:     payload.AccessToken,
		Refresh:    refresh,
		Expiry:     expiry,
		Issuer:     state.issuer,
		ClientID:   state.clientID,
		AccountID:  state.accountID,
		TokenURL:   state.tokenURL,
		Store:      state.store,
		HTTPClient: state.httpClient,
	}
	return next, Token{
		Value:  next.Access,
		Type:   TokenBearer,
		Expiry: next.Expiry,
		Header: oauthAuthorizationHeader,
	}, nil
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
