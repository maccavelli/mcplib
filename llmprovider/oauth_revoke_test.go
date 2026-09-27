package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// revokeCapture records the revoke request a stub issuer received.
type revokeCapture struct {
	path, contentType string
	json              map[string]string
	form              url.Values
}

func revokeIssuer(t *testing.T, discovery bool) (*httptest.Server, *revokeCapture) {
	t.Helper()
	c := &revokeCapture{}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		doc := fmt.Sprintf(`{"token_endpoint":%q}`, srv.URL+"/oauth2/token")
		if discovery {
			doc = fmt.Sprintf(`{"token_endpoint":%q,"revocation_endpoint":%q}`, srv.URL+"/oauth2/token", srv.URL+"/oauth2/revoke")
		}
		_, _ = w.Write([]byte(doc))
	})
	record := func(w http.ResponseWriter, r *http.Request) {
		c.path, c.contentType = r.URL.Path, r.Header.Get("Content-Type")
		if c.contentType == "application/json" {
			_ = json.NewDecoder(r.Body).Decode(&c.json)
		} else if err := r.ParseForm(); err == nil {
			c.form = r.PostForm
		}
		w.WriteHeader(http.StatusOK)
	}
	mux.HandleFunc("/oauth/revoke", record)
	mux.HandleFunc("/oauth2/revoke", record)
	return srv, c
}

// TestRevokeOAuthSession_OpenAIRevokesRefreshToken: Codex's JSON body, at
// the token URL's origin with path /oauth/revoke.
func TestRevokeOAuthSession_OpenAIRevokesRefreshToken(t *testing.T) {
	srv, c := revokeIssuer(t, false)
	err := RevokeOAuthSession(context.Background(), &OAuthSession{Access: "a", Refresh: "rt", Issuer: DefaultOpenAIIssuer,
		ClientID: DefaultOpenAIClientID, TokenURL: srv.URL + "/oauth/token", HTTPClient: srv.Client()})
	want := map[string]string{"token": "rt", "token_type_hint": "refresh_token", "client_id": DefaultOpenAIClientID}
	if err != nil || c.path != "/oauth/revoke" || c.contentType != "application/json" || fmt.Sprint(c.json) != fmt.Sprint(want) {
		t.Fatalf("err = %v, request %s %s %v; want JSON %v at /oauth/revoke", err, c.path, c.contentType, c.json, want)
	}
}

// TestRevokeOAuthSession_OpenAIAccessOnly: with no refresh token the access
// token is revoked, and no client_id is sent (revoke.rs:31-53).
func TestRevokeOAuthSession_OpenAIAccessOnly(t *testing.T) {
	srv, c := revokeIssuer(t, false)
	err := RevokeOAuthSession(context.Background(), &OAuthSession{Access: "a", Issuer: DefaultOpenAIIssuer,
		ClientID: DefaultOpenAIClientID, TokenURL: srv.URL + "/oauth/token", HTTPClient: srv.Client()})
	want := map[string]string{"token": "a", "token_type_hint": "access_token"}
	if err != nil || fmt.Sprint(c.json) != fmt.Sprint(want) {
		t.Fatalf("err = %v, body %v; want %v", err, c.json, want)
	}
}

// TestRevokeOAuthSession_GrokUsesDiscovery: an RFC 7009 form to the
// discovered revocation_endpoint.
func TestRevokeOAuthSession_GrokUsesDiscovery(t *testing.T) {
	srv, c := revokeIssuer(t, true)
	err := RevokeOAuthSession(context.Background(), &OAuthSession{Access: "a", Refresh: "rt", Issuer: srv.URL,
		ClientID: "grok-client", HTTPClient: srv.Client()})
	if err != nil || c.path != "/oauth2/revoke" || c.form.Get("token") != "rt" ||
		c.form.Get("token_type_hint") != "refresh_token" || c.form.Get("client_id") != "grok-client" {
		t.Fatalf("err = %v, request %s %v; want the form at /oauth2/revoke", err, c.path, c.form)
	}
}

// TestRevokeOAuthSession_GrokWithoutEndpoint: an issuer with no
// revocation_endpoint is unsupported, and nothing is sent.
func TestRevokeOAuthSession_GrokWithoutEndpoint(t *testing.T) {
	srv, c := revokeIssuer(t, false)
	err := RevokeOAuthSession(context.Background(), &OAuthSession{Access: "a", Refresh: "rt", Issuer: srv.URL,
		ClientID: "grok-client", HTTPClient: srv.Client()})
	if !errors.Is(err, errors.ErrUnsupported) || c.path != "" {
		t.Fatalf("err = %v after request %q; want ErrUnsupported and none", err, c.path)
	}
}
