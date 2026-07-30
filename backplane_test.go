package mcplib

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ── Constructor Tests ───────────────────────────────────────────────────────

func TestNewBackplaneClient_Disabled(t *testing.T) {
	os.Unsetenv("MCP_LLM_ENABLED")
	c := NewBackplaneClient(context.Background(), "test", "fast")
	if c != nil {
		t.Errorf("expected nil when MCP_LLM_ENABLED not set, got client with addr=%s", c.addr)
	}
}

func TestNewBackplaneClient_MissingAddr(t *testing.T) {
	t.Setenv("MCP_LLM_ENABLED", "true")
	os.Unsetenv("MCP_LLM_ADDR")
	os.Unsetenv("MCP_LLM_TOKEN")

	c := NewBackplaneClient(context.Background(), "test", "fast")
	if c != nil {
		t.Errorf("expected nil when MCP_LLM_ADDR missing, got non-nil client")
	}
}

func TestNewBackplaneClient_Success(t *testing.T) {
	srv := mockBackplane(t)
	defer srv.Close()

	t.Setenv("MCP_LLM_ENABLED", "true")
	t.Setenv("MCP_LLM_ADDR", srv.Listener.Addr().String())
	t.Setenv("MCP_LLM_TOKEN", "test-token")

	ctx := t.Context()
	c := NewBackplaneClient(ctx, "test-server", "fast")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if !c.Available() {
		t.Error("expected Available()=true immediately after init (optimistic start)")
	}
	if c.serverName != "test-server" {
		t.Errorf("expected serverName=test-server, got %q", c.serverName)
	}
}

// ── Generate Tests ──────────────────────────────────────────────────────────

func TestGenerate_OK(t *testing.T) {
	srv := mockBackplane(t)
	defer srv.Close()

	c := clientFor(t, srv)
	c.available.Store(true)

	text, err := c.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if text != "mock response" {
		t.Errorf("expected 'mock response', got %q", text)
	}
}

func TestGenerateThinking_OK(t *testing.T) {
	srv := mockBackplane(t)
	defer srv.Close()

	c := clientFor(t, srv)
	c.available.Store(true)

	text, err := c.GenerateThinking(context.Background(), "deep analysis")
	if err != nil {
		t.Fatalf("GenerateThinking failed: %v", err)
	}
	if text != "mock response" {
		t.Errorf("expected 'mock response', got %q", text)
	}
}

func TestGenerate_Unavailable(t *testing.T) {
	c := &BackplaneClient{addr: "127.0.0.1:1", token: "x", serverName: "test", http: &http.Client{}}
	c.available.Store(false)

	_, err := c.Generate(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error when unavailable")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected unavailable error, got %v", err)
	}
}

// ── JSONResponse Tests ──────────────────────────────────────────────────────

func TestJSONResponse_OK(t *testing.T) {
	srv := mockBackplaneJSON(t, map[string]string{"result": "ok"})
	defer srv.Close()

	c := clientFor(t, srv)
	c.available.Store(true)

	var out map[string]string
	if err := c.JSONResponse(context.Background(), "give me json", &out); err != nil {
		t.Fatalf("JSONResponse failed: %v", err)
	}
	if out["result"] != "ok" {
		t.Errorf("expected result=ok, got %v", out)
	}
}

func TestJSONResponse_MarkdownFences(t *testing.T) {
	// Simulate LLM wrapping JSON in markdown fences.
	fenced := "```json\n{\"key\": \"value\"}\n```"
	srv := mockBackplaneText(t, fenced)
	defer srv.Close()

	c := clientFor(t, srv)
	c.available.Store(true)

	var out map[string]string
	if err := c.JSONResponse(context.Background(), "give me json", &out); err != nil {
		t.Fatalf("JSONResponse with fences failed: %v", err)
	}
	if out["key"] != "value" {
		t.Errorf("expected key=value, got %v", out)
	}
}

// ── Rate Limiting Tests ─────────────────────────────────────────────────────

func TestGenerate_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(backplaneError{Error: "quota exceeded"})
	}))
	defer srv.Close()

	c := clientFor(t, srv)
	c.available.Store(true)

	_, err := c.Generate(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected ErrBackplaneRateLimited")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected rate limited error, got %v", err)
	}
}

func TestGenerate_429Retry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			// First call: 429 with retry_after=0 (immediate retry for test speed).
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(backplaneError{Error: "quota exceeded", RetryAfter: 0})
			return
		}
		// Second call: success.
		json.NewEncoder(w).Encode(backplaneResponse{Text: "retried ok", Model: "test", LatencyMs: 1})
	}))
	defer srv.Close()

	c := clientFor(t, srv)
	c.available.Store(true)

	// RetryAfter=0 means the retry loop won't wait, but it also means
	// the retry condition (retryAfter > 0) won't be met. The request
	// will fail with the 429 error on the first attempt.
	_, err := c.Generate(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error on 429 with RetryAfter=0")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry on RetryAfter=0), got %d", calls.Load())
	}
}

func TestGenerate_429RetryWithRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			// First call: 429 with retry_after=1.
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(backplaneError{Error: "quota exceeded", RetryAfter: 1})
			return
		}
		// Second call: success.
		json.NewEncoder(w).Encode(backplaneResponse{Text: "retried ok", Model: "test", LatencyMs: 1})
	}))
	defer srv.Close()

	c := clientFor(t, srv)
	c.available.Store(true)

	text, err := c.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if text != "retried ok" {
		t.Errorf("expected 'retried ok', got %q", text)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (1 retry), got %d", calls.Load())
	}
}

// ── Circuit Breaker Tests ───────────────────────────────────────────────────

func TestCircuitBreaker_TripsAfter3(t *testing.T) {
	// Server that always returns connection refused (by closing immediately).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 500 to simulate a non-OK response (not a connection error).
		w.WriteHeader(http.StatusInternalServerError)
	}))
	srv.Close() // Close immediately to cause connection errors.

	c := &BackplaneClient{
		addr:       srv.Listener.Addr().String(),
		token:      "x",
		serverName: "test",
		probeNow:   make(chan struct{}, 1),
		http:       &http.Client{Timeout: 100 * time.Millisecond},
	}
	c.available.Store(true)

	// Fire 3 requests — circuit breaker should trip.
	for i := range 3 {
		_, _ = c.Generate(context.Background(), "hello")
		if i < 2 && !c.available.Load() {
			t.Errorf("circuit breaker tripped too early at attempt %d", i+1)
		}
	}

	if c.available.Load() {
		t.Error("expected circuit breaker to trip after 3 consecutive failures")
	}
}

func TestCircuitBreaker_IgnoresContextCancellation(t *testing.T) {
	// Server that always hangs (never responds).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {} // block forever
	}))
	defer srv.Close()

	c := &BackplaneClient{
		addr:       srv.Listener.Addr().String(),
		token:      "x",
		serverName: "test",
		probeNow:   make(chan struct{}, 1),
		http:       &http.Client{},
	}
	c.available.Store(true)

	// Cancel context immediately — error is context cancellation, not network.
	for range 5 {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		time.Sleep(2 * time.Millisecond) // ensure context expires
		_, _ = c.Generate(ctx, "hello")
		cancel()
	}

	// Circuit breaker should NOT have tripped — context cancellation
	// errors are not counted toward consecutive failures.
	if !c.available.Load() {
		t.Error("circuit breaker tripped on context cancellation — should be ignored")
	}
	if c.consecutiveFailures.Load() != 0 {
		t.Errorf("expected 0 consecutive failures, got %d", c.consecutiveFailures.Load())
	}
}

// ── ServerName Test ─────────────────────────────────────────────────────────

func TestServerName_SentInRequest(t *testing.T) {
	var captured backplaneRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		json.NewEncoder(w).Encode(backplaneResponse{Text: "ok", Model: "test"})
	}))
	defer srv.Close()

	c := clientFor(t, srv)
	c.serverName = "my-custom-server"
	c.available.Store(true)

	c.Generate(context.Background(), "test")
	if captured.ServerName != "my-custom-server" {
		t.Errorf("expected server_name=my-custom-server, got %q", captured.ServerName)
	}
}

// ── extractJSON Tests ───────────────────────────────────────────────────────

func TestExtractJSON_CleanJSON(t *testing.T) {
	input := `{"key": "value"}`
	got := extractJSON(input)
	if got != input {
		t.Errorf("expected %q, got %q", input, got)
	}
}

func TestExtractJSON_WithFences(t *testing.T) {
	input := "```json\n{\"key\": \"value\"}\n```"
	want := `{"key": "value"}`
	got := extractJSON(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestExtractJSON_Array(t *testing.T) {
	input := "Here is the result:\n[1, 2, 3]\nDone."
	want := `[1, 2, 3]`
	got := extractJSON(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ── Test Helpers ────────────────────────────────────────────────────────────

func mockBackplane(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/llm/status":
			w.WriteHeader(http.StatusOK)
		case "/llm/generate", "/llm/generate-thinking":
			json.NewEncoder(w).Encode(backplaneResponse{Text: "mock response", Model: "test", LatencyMs: 1})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func mockBackplaneJSON(t *testing.T, payload any) *httptest.Server {
	t.Helper()
	b, _ := json.Marshal(payload)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(backplaneResponse{Text: string(b), Model: "test", LatencyMs: 1})
	}))
}

func mockBackplaneText(t *testing.T, text string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(backplaneResponse{Text: text, Model: "test", LatencyMs: 1})
	}))
}

func clientFor(t *testing.T, srv *httptest.Server) *BackplaneClient {
	t.Helper()
	return &BackplaneClient{
		addr:       srv.Listener.Addr().String(),
		token:      "test-token",
		serverName: "test",
		model:      "fast",
		probeNow:   make(chan struct{}, 1),
		http:       &http.Client{},
	}
}
