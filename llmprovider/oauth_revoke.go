package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// RevokeOAuthSession revokes an mcplib-owned session at its issuer, so a
// logout ends the refresh-token family rather than only forgetting it (MADR
// 0012 §5.3). It revokes the refresh token when there is one, else the access
// token. It does not touch any TokenStore: the caller deletes its copy
// whether or not revocation succeeds, as Codex does
// (codex login/src/auth/revoke.rs:1-5). Never pass a vendor CLI's session:
// revoking it would log the CLI out.
//
// OpenAI: a JSON request to {issuer}/oauth/revoke (revoke.rs:31-53, 104-114).
// Grok: an RFC 7009 form request to the discovery document's
// revocation_endpoint; without one the result wraps errors.ErrUnsupported.
func RevokeOAuthSession(ctx context.Context, session *OAuthSession) error {
	if session == nil {
		return errors.New("oauth: revoke: nil session")
	}
	session.mu.Lock()
	state := session.refreshState()
	access := session.Access
	session.mu.Unlock()

	token, hint := state.refresh, oauthRefreshToken
	if token == "" {
		token, hint = access, "access_token"
	}
	if token == "" {
		return errors.New("oauth: revoke: session holds no token")
	}
	client := state.httpClient
	if client == nil {
		client = defaultHTTPClient()
	}

	var req *http.Request
	var err error
	if strings.TrimRight(state.issuer, "/") == DefaultOpenAIIssuer {
		req, err = openAIRevokeRequest(ctx, state, token, hint)
	} else {
		req, err = grokRevokeRequest(ctx, state, client, token, hint)
	}
	if err != nil {
		return err
	}
	identityOf(ProviderConfig{}).setUserAgent(req)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("oauth: revoke request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return oauthHTTPStatusError("revoke", resp)
	}
	return resp.Body.Close()
}

// openAIRevokeRequest posts Codex's JSON body. The endpoint is the token
// URL's origin with path /oauth/revoke, as Codex derives it from its refresh
// URL (revoke.rs:134-150), else the issuer's.
func openAIRevokeRequest(ctx context.Context, state oauthSessionState, token, hint string) (*http.Request, error) {
	endpoint := DefaultOpenAIIssuer + "/oauth/revoke"
	if state.tokenURL != "" {
		u, err := url.Parse(state.tokenURL)
		if err != nil {
			return nil, fmt.Errorf("oauth: revoke: parse token URL: %w", err)
		}
		u.Path, u.RawQuery, u.Fragment = "/oauth/revoke", "", ""
		endpoint = u.String()
	}
	body := map[string]string{"token": token, "token_type_hint": hint}
	if hint == oauthRefreshToken {
		body[oauthParamClientID] = state.clientID
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("oauth: revoke: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("oauth: revoke: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// grokRevokeRequest posts an RFC 7009 form to the issuer's discovered
// revocation_endpoint (https://auth.x.ai/oauth2/revoke on 2026-09-27).
func grokRevokeRequest(ctx context.Context, state oauthSessionState, client *http.Client, token, hint string) (*http.Request, error) {
	endpoints, err := oauthEndpointsFor(ctx, oauthFlowConfig{provider: ProviderGrok, issuer: state.issuer, httpClient: client})
	if err != nil {
		return nil, err
	}
	if endpoints.Revocation == "" {
		return nil, fmt.Errorf("oauth: revoke: %s publishes no revocation_endpoint: %w", state.issuer, errors.ErrUnsupported)
	}
	form := url.Values{"token": {token}, "token_type_hint": {hint}, oauthParamClientID: {state.clientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Revocation, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("oauth: revoke: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}
