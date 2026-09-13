package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	oauthRefreshSkew         = 2 * time.Minute
	oauthAuthorizationHeader = "Authorization"
)

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
		statusErr := fmt.Errorf("oauth: refresh failed: %s", resp.Status)
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, Token{}, errors.Join(statusErr, fmt.Errorf("oauth: close refresh response: %w", closeErr))
		}
		return nil, Token{}, statusErr
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
