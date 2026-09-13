package llmprovider

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestListenFirstAvailable_UsesSecondPortWhenFirstBusy(t *testing.T) {
	t.Parallel()

	first, firstPort := listenOnFreePort(t)
	defer closeListener(t, first)
	second, secondPort := listenOnFreePort(t)
	closeListener(t, second)

	listener, port, err := listenFirstAvailable("127.0.0.1", []int{firstPort, secondPort})
	if err != nil {
		t.Fatalf("listenFirstAvailable() error = %v", err)
	}
	defer closeListener(t, listener)
	if port != secondPort {
		t.Fatalf("port = %d, want second available port %d", port, secondPort)
	}
}

func TestListenFirstAvailable_ErrorsWhenAllBusy(t *testing.T) {
	t.Parallel()

	first, firstPort := listenOnFreePort(t)
	defer closeListener(t, first)
	second, secondPort := listenOnFreePort(t)
	defer closeListener(t, second)

	listener, _, err := listenFirstAvailable("127.0.0.1", []int{firstPort, secondPort})
	if listener != nil {
		closeListener(t, listener)
		t.Fatal("listenFirstAvailable() returned a listener when all ports were busy")
	}
	if err == nil || !strings.Contains(err.Error(), "device-code") {
		t.Fatalf("error = %v, want device-code guidance", err)
	}
}

func TestOpenAILoopbackPortsAre1455Then1457(t *testing.T) {
	t.Parallel()

	want := []int{1455, 1457}
	if !reflect.DeepEqual(openaiLoopbackPorts, want) {
		t.Fatalf("openaiLoopbackPorts = %v, want %v", openaiLoopbackPorts, want)
	}
}

func TestBuildAuthorizeURL_OpenAIContract(t *testing.T) {
	t.Parallel()

	config := oauthFlowConfig{provider: ProviderOpenAI, clientID: DefaultOpenAIClientID}
	rawURL, err := buildAuthorizeURL(
		config,
		DefaultOpenAIIssuer+"/oauth/authorize",
		"http://localhost:1455/auth/callback",
		"challenge",
		"state",
	)
	if err != nil {
		t.Fatalf("buildAuthorizeURL() error = %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	want := url.Values{
		"client_id":                  {DefaultOpenAIClientID},
		"code_challenge":             {"challenge"},
		"code_challenge_method":      {"S256"},
		"codex_cli_simplified_flow":  {"true"},
		"id_token_add_organizations": {"true"},
		"originator":                 {"mcplib"},
		"redirect_uri":               {"http://localhost:1455/auth/callback"},
		"response_type":              {"code"},
		"scope":                      {openAIOAuthScopes},
		"state":                      {"state"},
	}
	if parsed.Scheme != "https" || parsed.Host != "auth.openai.com" || parsed.Path != "/oauth/authorize" {
		t.Fatalf("authorization endpoint = %s", parsed.Redacted())
	}
	if !reflect.DeepEqual(parsed.Query(), want) {
		t.Fatalf("authorization query = %v, want %v", parsed.Query(), want)
	}
}

func TestOAuthCallback_RejectsStateMismatch(t *testing.T) {
	t.Parallel()

	result := make(chan oauthCallbackResult, 1)
	handler := oauthCallbackHandler("/callback", "expected-state", result)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet,
		"http://127.0.0.1/callback?code=secret&state=wrong-state",
		http.NoBody,
	))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want 400", recorder.Code)
	}
	select {
	case callback := <-result:
		t.Fatalf("mismatched state produced callback result %#v", callback)
	default:
	}
}

func TestResolveOAuthFlowConfig_GrokEnvironmentOverrides(t *testing.T) {
	t.Setenv("GROK_OAUTH2_ISSUER", "")
	t.Setenv("GROK_OAUTH2_CLIENT_ID", "")
	defaults, err := resolveOAuthFlowConfig(ProviderGrok, OAuthFlowOptions{})
	if err != nil {
		t.Fatalf("resolve default OAuth flow config: %v", err)
	}
	if defaults.issuer != DefaultGrokOAuthIssuer || defaults.clientID != DefaultGrokOAuthClientID {
		t.Fatalf("default issuer/client = (%q, %q)", defaults.issuer, defaults.clientID)
	}

	t.Setenv("GROK_OAUTH2_ISSUER", "https://issuer.example/")
	t.Setenv("GROK_OAUTH2_CLIENT_ID", "environment-client")

	config, err := resolveOAuthFlowConfig(ProviderGrok, OAuthFlowOptions{})
	if err != nil {
		t.Fatalf("resolveOAuthFlowConfig() error = %v", err)
	}
	if config.issuer != "https://issuer.example" || config.clientID != "environment-client" {
		t.Fatalf("resolved issuer/client = (%q, %q)", config.issuer, config.clientID)
	}
}

func TestOAuthEndpointsFor_GrokFallsBackAfterDiscoveryFailure(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Status:     "503 Service Unavailable",
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	config := oauthFlowConfig{provider: ProviderGrok, issuer: "https://issuer.example", httpClient: client}
	endpoints, err := oauthEndpointsFor(context.Background(), config)
	if err != nil {
		t.Fatalf("oauthEndpointsFor() error = %v", err)
	}
	if endpoints.Authorization != "https://issuer.example/oauth2/authorize" ||
		endpoints.Token != defaultGrokOAuthRefreshURL || endpoints.Device != defaultGrokOAuthDeviceURL {
		t.Fatalf("fallback endpoints = %#v", endpoints)
	}
}

func TestLoginBrowserOAuth_GrokCompletesCallbackAndExchange(t *testing.T) {
	t.Parallel()

	var tokenForm url.Values
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, `{"authorization_endpoint":`+strconv.Quote(srv.URL+"/authorize")+`,"token_endpoint":`+strconv.Quote(srv.URL+"/token")+`}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse token form: %v", err)
		}
		tokenForm = make(url.Values, len(r.PostForm))
		for key, values := range r.PostForm {
			tokenForm[key] = append([]string(nil), values...)
		}
		writeTestJSON(t, w, `{"access_token":"access","refresh_token":"refresh","expires_in":3600}`)
	})

	openURL := func(rawURL string) error {
		authURL, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		if authURL.Path != "/authorize" {
			return errors.New("unexpected authorization endpoint")
		}
		query := authURL.Query()
		if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
			return errors.New("authorization URL missing S256 PKCE")
		}
		callbackURL := query.Get("redirect_uri") + "?code=browser-secret&state=" + url.QueryEscape(query.Get("state"))
		resp, err := http.Get(callbackURL) //nolint:gosec // Local callback URL created by the code under test.
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "You can close this window.") {
			return errors.New("callback did not return the success page")
		}
		if strings.Contains(string(body), "browser-secret") || strings.Contains(string(body), "access") {
			return errors.New("callback page exposed a credential")
		}
		return errors.New("browser opener failure after callback")
	}

	session, err := LoginBrowserOAuth(context.Background(), ProviderGrok, OAuthFlowOptions{
		HTTPClient: srv.Client(),
		OpenURL:    openURL,
		ClientID:   "test-client",
		Issuer:     srv.URL,
	})
	if err != nil {
		t.Fatalf("LoginBrowserOAuth() error = %v", err)
	}
	if session.Access != "access" || session.Refresh != "refresh" || session.Provider != ProviderGrok {
		t.Fatalf("session = %#v", session)
	}
	if tokenForm.Get("code") != "browser-secret" || tokenForm.Get("code_verifier") == "" {
		t.Fatalf("token form = %v", tokenForm)
	}
	if got := tokenForm.Get("redirect_uri"); !strings.HasPrefix(got, "http://127.0.0.1:") || !strings.HasSuffix(got, "/callback") {
		t.Fatalf("redirect_uri = %q, want ephemeral 127.0.0.1 callback", got)
	}
}

func TestOAuthLogin_RejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := LoginBrowserOAuth(ctx, ProviderClaude, OAuthFlowOptions{}); err == nil {
		t.Fatal("LoginBrowserOAuth() error = nil, want unsupported-provider error")
	}
	if _, err := LoginDeviceOAuth(ctx, ProviderClaude, OAuthFlowOptions{}); err == nil {
		t.Fatal("LoginDeviceOAuth() error = nil, want unsupported-provider error")
	}
}

func listenOnFreePort(t *testing.T) (net.Listener, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free port: %v", err)
	}
	return listener, listener.Addr().(*net.TCPAddr).Port
}

func closeListener(t *testing.T, listener net.Listener) {
	t.Helper()
	if err := listener.Close(); err != nil {
		t.Errorf("close listener: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
