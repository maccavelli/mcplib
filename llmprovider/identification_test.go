package llmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// headerRecorder is a test server that records every request's headers and
// answers a listing and a metadata document, and a 500 to everything else.
type headerRecorder struct {
	mu   sync.Mutex
	seen []recordedRequest
	srv  *httptest.Server
}

type recordedRequest struct {
	path   string
	header http.Header
}

func newHeaderRecorder(t *testing.T) *headerRecorder {
	t.Helper()
	r := &headerRecorder{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.seen = append(r.seen, recordedRequest{path: req.URL.Path, header: req.Header.Clone()})
		r.mu.Unlock()
		switch {
		case strings.HasSuffix(req.URL.Path, "/api.json"):
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(req.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"glm-5.3-flash"},{"id":"qwen3.8-flash"}]}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *headerRecorder) requests() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.seen...)
}

var userAgentPattern = regexp.MustCompile(`^mcplib/\S+ \(\w+; \w+\) mcplib/\S+$`)

// TestIdentification_UserAgent: every generation and listing request names
// mcplib honestly (MADR 0012 §1.4), never Go's default agent.
func TestIdentification_UserAgent(t *testing.T) {
	rec := newHeaderRecorder(t)
	base := WithBaseURL(rec.srv.URL)
	build := map[string]func() (Provider, error){
		"openai": func() (Provider, error) { return NewOpenAI("k", "gpt-4.1-mini", base) },
		"claude": func() (Provider, error) { return NewClaude("k", "claude-haiku-4-5", base) },
		"gemini": func() (Provider, error) {
			return NewGemini(context.Background(), "k", "gemini-3.7-flash", base)
		},
		"grok":        func() (Provider, error) { return NewGrok("k", "grok-4.5", base) },
		"kilo":        func() (Provider, error) { return NewKilo("k", "some/model", base) },
		"huggingface": func() (Provider, error) { return NewHuggingFace("k", "org/model", base) },
		"ollama":      func() (Provider, error) { return NewOllama("", "llama3", base) },
		"opencode":    func() (Provider, error) { return NewOpencode(ProviderOpencodeGo, "k", "glm-5.3-flash", base) },
	}
	for name, newProvider := range build {
		p, err := newProvider()
		if err != nil {
			t.Fatalf("%s: construct: %v", name, err)
		}
		_, _ = p.Generate(context.Background(), "hello")
	}
	if _, err := ListModelCatalog(context.Background(), ProviderKilo, "k", base); err != nil {
		t.Fatalf("listing: %v", err)
	}
	for _, r := range rec.requests() {
		if ua := r.header.Get("User-Agent"); !userAgentPattern.MatchString(ua) {
			t.Errorf("%s: User-Agent = %q, want mcplib/<version> (<os>; <arch>) mcplib/<version>", r.path, ua)
		}
	}
}

// TestIdentification_OpencodeListingCarriesSession: a gateway listing sends
// x-opencode-session, and a provider's DiscoverModels uses one session id for
// every request it makes (0013 B7).
func TestIdentification_OpencodeListingCarriesSession(t *testing.T) {
	rec := newHeaderRecorder(t)
	p, err := NewOpencode(ProviderOpencodeGo, "k", "glm-5.3-flash",
		WithBaseURL(rec.srv.URL), WithModelMetadataURL(rec.srv.URL+"/api.json"))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	if _, err := p.DiscoverModels(context.Background()); err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	sessions := map[string]bool{}
	for _, r := range rec.requests() {
		if strings.HasSuffix(r.path, "/api.json") {
			continue
		}
		s := r.header.Get(opencodeSessionHeader)
		if s == "" {
			t.Errorf("%s: no %s", r.path, opencodeSessionHeader)
		}
		sessions[s] = true
	}
	if len(sessions) != 1 {
		t.Errorf("DiscoverModels used %d session ids, want one: %v", len(sessions), sessions)
	}
}

// TestIdentification_KiloHeaders: Kilo generation names the client and the task.
func TestIdentification_KiloHeaders(t *testing.T) {
	rec := newHeaderRecorder(t)
	p, err := NewKilo("k", "some/model", WithBaseURL(rec.srv.URL))
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	_, _ = p.Generate(context.Background(), "hello")
	reqs := rec.requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	if h := reqs[0].header; h.Get("X-KILOCODE-EDITORNAME") != "mcplib" || h.Get("X-KiloCode-TaskId") == "" {
		t.Fatalf("editor/task = %q/%q, want mcplib and a task id", h.Get("X-KILOCODE-EDITORNAME"), h.Get("X-KiloCode-TaskId"))
	}
}

// TestIdentification_NoForbiddenHeaders: no request vouches for another
// client (MADR 0012 §1.4).
func TestIdentification_NoForbiddenHeaders(t *testing.T) {
	rec := newHeaderRecorder(t)
	base := WithBaseURL(rec.srv.URL)
	for _, route := range []OpencodeRoute{OpencodeRouteResponses, OpencodeRouteMessages,
		OpencodeRouteChatCompletions, OpencodeRouteGoogle} {
		p, err := NewOpencode(ProviderOpencodeZen, "k", "some-model", base, WithOpencodeRoute(route))
		if err != nil {
			t.Fatalf("NewOpencode: %v", err)
		}
		_, _ = p.Generate(context.Background(), "hello")
	}
	for _, build := range []func() (Provider, error){
		func() (Provider, error) { return NewGrok("k", "grok-4.5", base) },
		func() (Provider, error) { return NewKilo("k", "some/model", base) },
	} {
		p, err := build()
		if err != nil {
			t.Fatalf("construct: %v", err)
		}
		_, _ = p.Generate(context.Background(), "hello")
	}
	for _, r := range rec.requests() {
		for name := range r.header {
			lower := strings.ToLower(name)
			if lower == "x-opencode-client" || lower == "x-xai-token-auth" || strings.HasPrefix(lower, "x-grok-") {
				t.Errorf("%s: forbidden header %s", r.path, name)
			}
		}
		if ua := strings.ToLower(r.header.Get("User-Agent")); strings.Contains(ua, "codex") ||
			strings.Contains(ua, "kilo-code") || strings.Contains(ua, "opencode/") {
			t.Errorf("%s: User-Agent %q impersonates a reference client", r.path, ua)
		}
	}
}
