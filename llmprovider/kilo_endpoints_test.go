package llmprovider

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
)

// kiloCall is one request a Kilo provider made.
type kiloCall struct {
	method, url, auth, org string
}

// absentHeader marks a header the request did not carry.
const absentHeader = "<absent>"

// kiloCalls runs one Generate and one DiscoverModels through a recording
// transport that answers without a network, and returns the requests made.
func kiloCalls(t *testing.T, token string, opts ...ProviderOption) []kiloCall {
	t.Helper()
	var (
		mu    sync.Mutex
		calls []kiloCall
	)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		org := absentHeader
		if v, ok := r.Header[http.CanonicalHeaderKey("X-KILOCODE-ORGANIZATIONID")]; ok {
			org = strings.Join(v, ",")
		}
		mu.Lock()
		calls = append(calls, kiloCall{r.Method, r.URL.String(), r.Header.Get("Authorization"), org})
		mu.Unlock()
		body := `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`
		if r.Method == http.MethodGet {
			body = `{"data":[{"id":"deepseek/deepseek-v4.1-flash","architecture":{"input_modalities":["text"],` +
				`"output_modalities":["text"]},"supported_parameters":["tools"],"pricing":{"completion":"0.1"}}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	p, err := NewKilo(token, "deepseek/deepseek-v4.1-flash", append([]ProviderOption{WithHTTPClient(client)}, opts...)...)
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := p.DiscoverModels(context.Background()); err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	return slices.Clone(calls)
}

func wantKiloCalls(t *testing.T, got []kiloCall, want ...kiloCall) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("requests:\n got  %+v\n want %+v", got, want)
	}
}

// TestKilo_URLTokenSelectsBase: a token "{url}:{secret}" sends generation and
// the listing to {origin}{prefix}/api/gateway (url.ts route()), with the whole
// token as the bearer.
func TestKilo_URLTokenSelectsBase(t *testing.T) {
	for _, tc := range []struct{ token, base string }{
		{"https://kilo.example.test/tenant:secret", "https://kilo.example.test/tenant/api/gateway"},
		{"http://127.0.0.1:8080:secret", "http://127.0.0.1:8080/api/gateway"},
		{"https://kilo.example.test/tenant/api/openrouter/:secret", "https://kilo.example.test/tenant/api/gateway"},
	} {
		t.Run(tc.token, func(t *testing.T) {
			wantKiloCalls(t, kiloCalls(t, tc.token),
				kiloCall{http.MethodPost, tc.base + "/chat/completions", "Bearer " + tc.token, absentHeader},
				kiloCall{http.MethodGet, tc.base + "/models", "Bearer " + tc.token, absentHeader})
		})
	}
}

// TestKilo_URLTokenOrganization: an /api/organizations/{id} token path scopes
// both requests to that organization and lists the organization's models
// (models.ts:219-229).
func TestKilo_URLTokenOrganization(t *testing.T) {
	const token = "https://kilo.example.test/api/organizations/org-9:secret"
	wantKiloCalls(t, kiloCalls(t, token),
		kiloCall{http.MethodPost, "https://kilo.example.test/api/gateway/chat/completions", "Bearer " + token, "org-9"},
		kiloCall{http.MethodGet, "https://kilo.example.test/api/organizations/org-9/models", "Bearer " + token, "org-9"})
}

// TestKilo_PlainTokenUsesDefaults: an ordinary key keeps the default gateway
// and sends no organization, even when a URL appears after its start.
func TestKilo_PlainTokenUsesDefaults(t *testing.T) {
	const token = "sk-plain:https://elsewhere.test/p:q"
	wantKiloCalls(t, kiloCalls(t, token),
		kiloCall{http.MethodPost, kiloBaseURL + "/chat/completions", "Bearer " + token, absentHeader},
		kiloCall{http.MethodGet, kiloBaseURL + "/models", "Bearer " + token, absentHeader})
}
