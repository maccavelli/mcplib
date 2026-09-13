package llmprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	oauthBrowserTimeout         = 10 * time.Minute
	oauthResponseLimit          = 1 << 20
	oauthParamClientID          = "client_id"
	oauthParamGrantType         = "grant_type"
	oauthGrantAuthorizationCode = "authorization_code"
	openAIOAuthScopes           = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	grokOAuthScopes             = "openid profile email offline_access grok-cli:access api:access"
)

var openaiLoopbackPorts = []int{1455, 1457}

// OAuthFlowOptions supplies transport and user-interaction hooks for OAuth login.
type OAuthFlowOptions struct {
	HTTPClient   *http.Client
	OpenURL      func(string) error
	InputCode    func(context.Context) (string, error)
	NotifyDevice func(verificationURL, userCode string)
	ClientID     string
	Issuer       string

	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

type oauthFlowConfig struct {
	provider   string
	clientID   string
	issuer     string
	httpClient *http.Client
	openURL    func(string) error
	inputCode  func(context.Context) (string, error)
	notify     func(string, string)
	now        func() time.Time
	sleep      func(context.Context, time.Duration) error
}

type oauthEndpoints struct {
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	Device        string `json:"device_authorization_endpoint"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type oauthCallbackResult struct {
	code string
	err  error
}

// LoginBrowserOAuth completes a browser authorization-code flow with PKCE.
func LoginBrowserOAuth(ctx context.Context, provider string, opts OAuthFlowOptions) (*OAuthSession, error) {
	config, err := resolveOAuthFlowConfig(provider, opts)
	if err != nil {
		return nil, err
	}

	endpoints, err := oauthEndpointsFor(ctx, config)
	if err != nil {
		return nil, err
	}
	pkce, err := newPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomBase64URL(32)
	if err != nil {
		return nil, fmt.Errorf("oauth: generate state: %w", err)
	}

	listener, redirectURI, callbackPath, err := browserListener(config.provider)
	if err != nil {
		return nil, err
	}
	flowCtx, cancel := context.WithTimeout(ctx, oauthBrowserTimeout)
	defer cancel()
	result := make(chan oauthCallbackResult, 1)
	handler := oauthCallbackHandler(callbackPath, state, result)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serveErrors <- fmt.Errorf("oauth: callback server: %w", serveErr)
		}
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		ignoreOAuthError(server.Shutdown(shutdownCtx))
	}()

	authorizeURL, err := buildAuthorizeURL(config, endpoints.Authorization, redirectURI, pkce.challenge, state)
	if err != nil {
		return nil, err
	}
	if config.openURL != nil {
		ignoreOAuthError(config.openURL(authorizeURL))
	}

	inputCancel := func() {}
	if config.inputCode != nil {
		var inputCtx context.Context
		inputCtx, inputCancel = context.WithCancel(flowCtx)
		go func() {
			input, inputErr := config.inputCode(inputCtx)
			if inputErr == nil {
				input, inputErr = parseOAuthInput(input, state)
			}
			select {
			case result <- oauthCallbackResult{code: input, err: inputErr}:
			case <-inputCtx.Done():
			}
		}()
	}
	defer inputCancel()

	var callback oauthCallbackResult
	select {
	case callback = <-result:
	case serveErr := <-serveErrors:
		return nil, serveErr
	case <-flowCtx.Done():
		return nil, fmt.Errorf("oauth: browser login timed out: %w", flowCtx.Err())
	}
	if callback.err != nil {
		return nil, callback.err
	}
	return exchangeOAuthCode(flowCtx, config, endpoints.Token, callback.code, redirectURI, pkce.verifier)
}

func resolveOAuthFlowConfig(provider string, opts OAuthFlowOptions) (oauthFlowConfig, error) {
	issuer := strings.TrimRight(opts.Issuer, "/")
	clientID := opts.ClientID
	switch provider {
	case ProviderOpenAI:
		if issuer == "" {
			issuer = DefaultOpenAIIssuer
		}
		if clientID == "" {
			clientID = DefaultOpenAIClientID
		}
	case ProviderGrok:
		if issuer == "" {
			issuer = strings.TrimRight(os.Getenv("GROK_OAUTH2_ISSUER"), "/")
			if issuer == "" {
				issuer = DefaultGrokOAuthIssuer
			}
		}
		if clientID == "" {
			clientID = os.Getenv("GROK_OAUTH2_CLIENT_ID")
			if clientID == "" {
				clientID = DefaultGrokOAuthClientID
			}
		}
	default:
		return oauthFlowConfig{}, fmt.Errorf("oauth: provider %q is not supported", provider)
	}

	client := opts.HTTPClient
	if client == nil {
		client = defaultHTTPClient()
	}
	now := opts.now
	if now == nil {
		now = time.Now
	}
	sleep := opts.sleep
	if sleep == nil {
		sleep = sleepWithContext
	}
	return oauthFlowConfig{
		provider:   provider,
		clientID:   clientID,
		issuer:     issuer,
		httpClient: client,
		openURL:    opts.OpenURL,
		inputCode:  opts.InputCode,
		notify:     opts.NotifyDevice,
		now:        now,
		sleep:      sleep,
	}, nil
}

func oauthEndpointsFor(ctx context.Context, config oauthFlowConfig) (oauthEndpoints, error) {
	if config.provider == ProviderOpenAI {
		return oauthEndpoints{
			Authorization: config.issuer + "/oauth/authorize",
			Token:         config.issuer + "/oauth/token",
		}, nil
	}

	fallback := oauthEndpoints{
		Authorization: config.issuer + "/oauth2/authorize",
		Token:         defaultGrokOAuthRefreshURL,
		Device:        defaultGrokOAuthDeviceURL,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.issuer+"/.well-known/openid-configuration", http.NoBody)
	if err != nil {
		return oauthEndpoints{}, fmt.Errorf("oauth: create discovery request: %w", err)
	}
	resp, err := config.httpClient.Do(req)
	if err != nil {
		return oauthDiscoveryFallback(fallback)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		ignoreOAuthError(resp.Body.Close())
		return oauthDiscoveryFallback(fallback)
	}
	var discovered oauthEndpoints
	decodeErr := decodeOAuthResponse(resp, &discovered)
	if decodeErr != nil {
		return oauthDiscoveryFallback(fallback)
	}
	if discovered.Authorization == "" {
		discovered.Authorization = fallback.Authorization
	}
	if discovered.Token == "" {
		discovered.Token = fallback.Token
	}
	if discovered.Device == "" {
		discovered.Device = fallback.Device
	}
	return discovered, nil
}

func browserListener(provider string) (net.Listener, string, string, error) {
	if provider == ProviderOpenAI {
		listener, port, err := listenFirstAvailable("127.0.0.1", openaiLoopbackPorts)
		if err != nil {
			return nil, "", "", err
		}
		return listener, fmt.Sprintf("http://localhost:%d/auth/callback", port), "/auth/callback", nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", "", fmt.Errorf("oauth: listen for Grok callback: %w", err)
	}
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		ignoreOAuthError(listener.Close())
		return nil, "", "", errors.New("oauth: Grok callback listener has an unexpected address type")
	}
	port := address.Port
	return listener, fmt.Sprintf("http://127.0.0.1:%d/callback", port), "/callback", nil
}

func listenFirstAvailable(host string, ports []int) (net.Listener, int, error) {
	var failures []error
	for _, port := range ports {
		listener, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
		if err == nil {
			return listener, port, nil
		}
		failures = append(failures, err)
	}
	return nil, 0, fmt.Errorf("oauth: registered callback ports unavailable; use device-code login: %w", errors.Join(failures...))
}

func buildAuthorizeURL(config oauthFlowConfig, endpoint, redirectURI, challenge, state string) (string, error) {
	authorizeURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("oauth: parse authorization endpoint: %w", err)
	}
	query := authorizeURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", config.clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	if config.provider == ProviderOpenAI {
		query.Set("scope", openAIOAuthScopes)
		query.Set("id_token_add_organizations", "true")
		query.Set("codex_cli_simplified_flow", "true")
		query.Set("originator", "mcplib")
	} else {
		nonce, nonceErr := randomBase64URL(32)
		if nonceErr != nil {
			return "", fmt.Errorf("oauth: generate nonce: %w", nonceErr)
		}
		query.Set("scope", grokOAuthScopes)
		query.Set("nonce", nonce)
		query.Set("referrer", "mcplib")
	}
	authorizeURL.RawQuery = query.Encode()
	return authorizeURL.String(), nil
}

func oauthCallbackHandler(path, state string, result chan<- oauthCallbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("state") != state {
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		if oauthError := query.Get("error"); oauthError != "" {
			http.Error(w, "Authorization failed", http.StatusBadRequest)
			select {
			case result <- oauthCallbackResult{err: fmt.Errorf("oauth: authorization failed: %s", oauthError)}:
			default:
			}
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "<!doctype html><html><body>You can close this window.</body></html>"); err != nil {
			select {
			case result <- oauthCallbackResult{err: fmt.Errorf("oauth: write callback response: %w", err)}:
			default:
			}
			return
		}
		select {
		case result <- oauthCallbackResult{code: code}:
		default:
		}
	})
	return mux
}

func parseOAuthInput(input, expectedState string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("oauth: empty authorization code")
	}
	parsed, err := url.Parse(input)
	if err == nil && parsed.Scheme != "" {
		query := parsed.Query()
		if oauthError := query.Get("error"); oauthError != "" {
			return "", fmt.Errorf("oauth: authorization failed: %s", oauthError)
		}
		if receivedState := query.Get("state"); receivedState != "" && receivedState != expectedState {
			return "", errors.New("oauth: callback state mismatch")
		}
		if code := query.Get("code"); code != "" {
			return code, nil
		}
		return "", errors.New("oauth: callback URL missing authorization code")
	}
	if input != "" {
		return input, nil
	}
	return "", errors.New("oauth: empty authorization code")
}

func exchangeOAuthCode(
	ctx context.Context,
	config oauthFlowConfig,
	tokenURL, code, redirectURI, verifier string,
) (*OAuthSession, error) {
	form := url.Values{
		oauthParamGrantType: {oauthGrantAuthorizationCode},
		"code":              {code},
		"redirect_uri":      {redirectURI},
		oauthParamClientID:  {config.clientID},
		"code_verifier":     {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("oauth: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := config.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth: token request: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		statusErr := fmt.Errorf("oauth: token exchange failed: %s", resp.Status)
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, errors.Join(statusErr, fmt.Errorf("oauth: close token response: %w", closeErr))
		}
		return nil, statusErr
	}
	var payload oauthTokenResponse
	if err := decodeOAuthResponse(resp, &payload); err != nil {
		return nil, fmt.Errorf("oauth: decode token response: %w", err)
	}
	return oauthSessionFromResponse(config, tokenURL, payload)
}

func oauthSessionFromResponse(config oauthFlowConfig, tokenURL string, payload oauthTokenResponse) (*OAuthSession, error) {
	if payload.AccessToken == "" {
		return nil, errors.New("oauth: token response missing access token")
	}
	expiresIn := payload.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	return &OAuthSession{
		Provider:   config.provider,
		Access:     payload.AccessToken,
		Refresh:    payload.RefreshToken,
		Expiry:     config.now().Add(time.Duration(expiresIn) * time.Second),
		Issuer:     config.issuer,
		ClientID:   config.clientID,
		AccountID:  chatGPTAccountID(payload.IDToken),
		TokenURL:   tokenURL,
		HTTPClient: config.httpClient,
	}, nil
}

func chatGPTAccountID(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		AccountID string `json:"chatgpt_account_id"`
		Auth      struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if claims.AccountID != "" {
		return claims.AccountID
	}
	return claims.Auth.AccountID
}

func decodeOAuthResponse(resp *http.Response, target any) error {
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, oauthResponseLimit)).Decode(target)
	closeErr := resp.Body.Close()
	if decodeErr != nil {
		if closeErr != nil {
			return errors.Join(decodeErr, closeErr)
		}
		return decodeErr
	}
	return closeErr
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func oauthDiscoveryFallback(fallback oauthEndpoints) (oauthEndpoints, error) {
	return fallback, nil
}

func ignoreOAuthError(_ error) {}
