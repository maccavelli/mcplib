package llmprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

const openAITestResponse = `{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`

func TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	client := &http.Client{Transport: openAITestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		captured = request
		return openAITestHTTPResponse(request, http.StatusOK, openAITestResponse), nil
	})}
	session := &OAuthSession{
		Issuer:  DefaultOpenAIIssuer,
		Access:  "sess",
		Refresh: "refresh",
		Expiry:  time.Now().Add(time.Hour),
	}
	provider, err := NewOpenAIWithSource(session, "gpt-5.4-mini", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewOpenAIWithSource() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if captured == nil {
		t.Fatal("Generate() made no request")
	}
	if captured.URL.Host != "chatgpt.com" || captured.URL.Path != "/backend-api/codex/responses" {
		t.Fatalf("request URL = %s, want chatgpt.com/backend-api/codex/responses", captured.URL.Redacted())
	}
}

func TestOpenAI_StaticKeyDoesNotHitChatGPTHost(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	client := &http.Client{Transport: openAITestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		captured = request
		return openAITestHTTPResponse(request, http.StatusOK, openAITestResponse), nil
	})}
	provider, err := NewOpenAIWithSource(NewStaticToken("sk-test"), "gpt-4.1-mini", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewOpenAIWithSource() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if captured == nil || captured.URL.Host != "api.openai.com" || captured.URL.Path != "/v1/responses" {
		t.Fatalf("request URL = %v, want api.openai.com/v1/responses", captured)
	}
}

func TestOpenAI_ChatGPTSetsAccountHeader(t *testing.T) {
	t.Parallel()

	var accountHeader string
	client := &http.Client{Transport: openAITestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		accountHeader = request.Header.Get("ChatGPT-Account-Id")
		return openAITestHTTPResponse(request, http.StatusOK, openAITestResponse), nil
	})}
	session := &OAuthSession{
		Issuer:    DefaultOpenAIIssuer,
		Access:    "sess",
		Refresh:   "refresh",
		Expiry:    time.Now().Add(time.Hour),
		AccountID: "acct_1",
	}
	provider, err := NewOpenAIWithSource(session, "gpt-5.4-mini", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewOpenAIWithSource() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if accountHeader != "acct_1" {
		t.Fatalf("ChatGPT-Account-Id = %q, want acct_1", accountHeader)
	}
}

func TestOpenAI_ChatGPTSetsResidencyHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		claims map[string]any
		want   string
	}{
		{
			name: "namespaced",
			claims: map[string]any{
				"https://api.openai.com/auth": map[string]any{"chatgpt_compute_residency": "eu"},
			},
			want: "eu",
		},
		{
			name:   "root_fallback",
			claims: map[string]any{"chatgpt_compute_residency": "us"},
			want:   "us",
		},
		{
			name: "namespaced_no_constraint_wins",
			claims: map[string]any{
				"chatgpt_compute_residency":   "eu",
				"https://api.openai.com/auth": map[string]any{"chatgpt_compute_residency": "no_constraint"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var residencyHeader string
			client := &http.Client{Transport: openAITestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				residencyHeader = request.Header.Get("x-openai-internal-codex-residency")
				return openAITestHTTPResponse(request, http.StatusOK, openAITestResponse), nil
			})}
			session := &OAuthSession{
				Issuer:  DefaultOpenAIIssuer,
				Access:  openAITestJWT(t, test.claims),
				Refresh: "refresh",
				Expiry:  time.Now().Add(time.Hour),
			}
			provider, err := NewOpenAIWithSource(session, "gpt-5.4-mini", WithHTTPClient(client))
			if err != nil {
				t.Fatalf("NewOpenAIWithSource() error = %v", err)
			}
			if _, err := provider.Generate(context.Background(), "hello"); err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if residencyHeader != test.want {
				t.Fatalf("residency header = %q, want %q", residencyHeader, test.want)
			}
		})
	}
}

func TestOpenAI_OAuth401RetriesOnceAfterRefresh(t *testing.T) {
	t.Parallel()

	var generateCalls, refreshCalls int
	client := &http.Client{}
	client.Transport = openAITestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "chatgpt.com":
			generateCalls++
			if generateCalls == 1 {
				if got := request.Header.Get("Authorization"); got != "Bearer old-access" {
					t.Errorf("first Authorization = %q", got)
				}
				return openAITestHTTPResponse(request, http.StatusUnauthorized, ""), nil
			}
			if got := request.Header.Get("Authorization"); got != "Bearer new-access" {
				t.Errorf("retry Authorization = %q", got)
			}
			return openAITestHTTPResponse(request, http.StatusOK, openAITestResponse), nil
		case "auth.test":
			refreshCalls++
			if err := request.ParseForm(); err != nil {
				t.Errorf("parse refresh form: %v", err)
			}
			want := url.Values{
				"client_id":     {DefaultOpenAIClientID},
				"grant_type":    {"refresh_token"},
				"refresh_token": {"old-refresh"},
			}
			if !reflect.DeepEqual(request.PostForm, want) {
				t.Errorf("refresh form = %v, want %v", request.PostForm, want)
			}
			return openAITestHTTPResponse(request, http.StatusOK, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`), nil
		default:
			return nil, errors.New("unexpected request host")
		}
	})
	session := &OAuthSession{
		Provider:   ProviderOpenAI,
		Issuer:     DefaultOpenAIIssuer,
		ClientID:   DefaultOpenAIClientID,
		Access:     "old-access",
		Refresh:    "old-refresh",
		Expiry:     time.Now().Add(time.Hour),
		TokenURL:   "https://auth.test/oauth/token",
		HTTPClient: client,
	}
	provider, err := NewOpenAIWithSource(session, "gpt-5.4-mini", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewOpenAIWithSource() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if generateCalls != 2 || refreshCalls != 1 {
		t.Fatalf("generate/refresh calls = %d/%d, want 2/1", generateCalls, refreshCalls)
	}
}

func TestNewProviderWithSource_RejectsClaude(t *testing.T) {
	t.Parallel()

	if _, err := NewProviderWithSource(ProviderClaude, NewStaticToken("x"), "x"); err == nil {
		t.Fatal("NewProviderWithSource() error = nil, want unsupported-provider error")
	}
}

func TestNewProvider_APIKeyStillPlatform(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	client := &http.Client{Transport: openAITestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		captured = request
		return openAITestHTTPResponse(request, http.StatusOK, openAITestResponse), nil
	})}
	provider, err := NewProvider(ProviderOpenAI, "sk-test", "gpt-4.1-mini", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if captured == nil || captured.URL.Host != "api.openai.com" || captured.URL.Path != "/v1/responses" {
		t.Fatalf("request URL = %v, want api.openai.com/v1/responses", captured)
	}
}

type openAITestRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn openAITestRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func openAITestHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func openAITestJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal JWT claims: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
