package llmprovider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

const grokTestResponse = `{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`

func TestGrok_SessionUsesAPIXAIHost(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	client := &http.Client{Transport: grokTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		captured = request
		return grokTestHTTPResponse(request, http.StatusOK, grokTestResponse), nil
	})}
	session := &OAuthSession{
		Provider: ProviderGrok,
		Issuer:   DefaultGrokOAuthIssuer,
		Access:   "session-access",
		Refresh:  "session-refresh",
		Expiry:   time.Now().Add(time.Hour),
	}
	provider, err := NewProviderWithSource(ProviderGrok, session, "grok-4.6", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewProviderWithSource() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if captured == nil {
		t.Fatal("Generate() made no request")
	}
	if captured.URL.Host != "api.x.ai" || captured.URL.Path != "/v1/responses" ||
		strings.Contains(captured.URL.String(), "cli-chat-proxy") {
		t.Fatalf("request URL = %s, want api.x.ai/v1/responses", captured.URL.Redacted())
	}
}

func TestGrok_SessionOmitsCLITokenAuthHeader(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: grokTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		for header := range request.Header {
			if strings.EqualFold(header, "X-XAI-Token-Auth") {
				t.Errorf("request unexpectedly contains %s", header)
			}
		}
		return grokTestHTTPResponse(request, http.StatusOK, grokTestResponse), nil
	})}
	session := &OAuthSession{
		Provider: ProviderGrok,
		Issuer:   DefaultGrokOAuthIssuer,
		Access:   "session-access",
		Refresh:  "session-refresh",
		Expiry:   time.Now().Add(time.Hour),
	}
	provider, err := NewProviderWithSource(ProviderGrok, session, "grok-4.6", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewProviderWithSource() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestGrok_EmptyStaticKeyStillRejected(t *testing.T) {
	t.Parallel()

	if _, err := NewGrok("", "grok-4.6"); err == nil {
		t.Fatal("NewGrok() error = nil, want empty-key error")
	}
	if _, err := NewProviderWithSource(ProviderGrok, NewStaticToken(""), "grok-4.6"); err == nil {
		t.Fatal("NewProviderWithSource() error = nil, want empty-static-key error")
	}
}

func TestGrok_TokenSourceCalledPerRequest(t *testing.T) {
	t.Parallel()

	source := &grokSequenceTokenSource{values: []string{"first-token", "second-token"}}
	var authorizations []string
	client := &http.Client{Transport: grokTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		authorizations = append(authorizations, request.Header.Get("Authorization"))
		return grokTestHTTPResponse(request, http.StatusOK, grokTestResponse), nil
	})}
	provider, err := NewProviderWithSource(ProviderGrok, source, "grok-4.6", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewProviderWithSource() error = %v", err)
	}
	for range 2 {
		if _, err := provider.Generate(context.Background(), "hello"); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
	}
	want := []string{"Bearer first-token", "Bearer second-token"}
	if !reflect.DeepEqual(authorizations, want) || source.calls != 2 {
		t.Fatalf("authorizations/calls = %v/%d, want %v/2", authorizations, source.calls, want)
	}
}

func TestGrok_OAuth401RetriesOnceAfterRefresh(t *testing.T) {
	t.Parallel()

	var generateCalls, refreshCalls int
	client := &http.Client{}
	client.Transport = grokTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "api.x.ai":
			generateCalls++
			if generateCalls == 1 {
				if got := request.Header.Get("Authorization"); got != "Bearer old-access" {
					t.Errorf("first Authorization = %q", got)
				}
				return grokTestHTTPResponse(request, http.StatusUnauthorized, ""), nil
			}
			if got := request.Header.Get("Authorization"); got != "Bearer new-access" {
				t.Errorf("retry Authorization = %q", got)
			}
			return grokTestHTTPResponse(request, http.StatusOK, grokTestResponse), nil
		case "auth.test":
			refreshCalls++
			if err := request.ParseForm(); err != nil {
				t.Errorf("parse refresh form: %v", err)
			}
			want := url.Values{
				"client_id":     {DefaultGrokOAuthClientID},
				"grant_type":    {"refresh_token"},
				"refresh_token": {"old-refresh"},
			}
			if !reflect.DeepEqual(request.PostForm, want) {
				t.Errorf("refresh form = %v, want %v", request.PostForm, want)
			}
			return grokTestHTTPResponse(request, http.StatusOK, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`), nil
		default:
			return nil, errors.New("unexpected request host")
		}
	})
	session := &OAuthSession{
		Provider:   ProviderGrok,
		Issuer:     DefaultGrokOAuthIssuer,
		ClientID:   DefaultGrokOAuthClientID,
		Access:     "old-access",
		Refresh:    "old-refresh",
		Expiry:     time.Now().Add(time.Hour),
		TokenURL:   "https://auth.test/oauth/token",
		HTTPClient: client,
	}
	provider, err := NewProviderWithSource(ProviderGrok, session, "grok-4.6", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewProviderWithSource() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if generateCalls != 2 || refreshCalls != 1 {
		t.Fatalf("generate/refresh calls = %d/%d, want 2/1", generateCalls, refreshCalls)
	}
}

func TestGrok_Static401DoesNotRetry(t *testing.T) {
	t.Parallel()

	var calls int
	client := &http.Client{Transport: grokTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return grokTestHTTPResponse(request, http.StatusUnauthorized, ""), nil
	})}
	provider, err := NewGrok("static-key", "grok-4.6", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewGrok() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); !errors.Is(err, ErrAuthFailure) {
		t.Fatalf("Generate() error = %v, want ErrAuthFailure", err)
	}
	if calls != 1 {
		t.Fatalf("generate calls = %d, want 1", calls)
	}
}

type grokSequenceTokenSource struct {
	values []string
	calls  int
}

func (source *grokSequenceTokenSource) Token(context.Context) (Token, error) {
	value := source.values[source.calls]
	source.calls++
	return Token{Value: value, Type: TokenBearer, Header: oauthAuthorizationHeader}, nil
}

type grokTestRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn grokTestRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func grokTestHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
