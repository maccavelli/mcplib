// Package mcplib — backplane.go provides the canonical LLM backplane client.
//
// When the magictools orchestrator is running with a shared LLM pool, it
// injects three environment variables into every sub-server process:
//
//	MCP_LLM_ENABLED=true
//	MCP_LLM_ADDR=<host:port>      (e.g. 127.0.0.1:48081)
//	MCP_LLM_TOKEN=<bearer-token>
//
// Call [NewBackplaneClient] at server startup to obtain a [BackplaneClient].
// If the env vars are absent (standalone mode), it returns nil — all call
// sites must treat nil as "LLM not available" and degrade gracefully.
//
// Individual MCP servers MUST NOT create bespoke copies of this client.
// See STD-MCP-CLIENT-WRAPPER-001 and STD-MCP-BACKPLANE-CLIENT-001.

package mcplib

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// ── Sentinel Errors ─────────────────────────────────────────────────────────

// Sentinel errors — use [errors.Is] for classification in callers.
var (
	// ErrBackplaneUnavailable indicates a connection failure to the backplane.
	// Callers should degrade gracefully (skip LLM enhancement, not hard-fail).
	ErrBackplaneUnavailable = errors.New("backplane: unavailable")

	// ErrBackplaneRateLimited indicates the orchestrator returned HTTP 429.
	// Callers MUST NOT retry immediately — respect the backplane quota.
	ErrBackplaneRateLimited = errors.New("backplane: rate limited")

	// ErrBackplaneTokenThreshold indicates the per-server token quota was exceeded.
	ErrBackplaneTokenThreshold = errors.New("backplane: token threshold exceeded")
)

// ── Constants ───────────────────────────────────────────────────────────────

const (
	// maxConsecutiveFailures is the number of consecutive network errors
	// before the circuit breaker trips and the client returns
	// [ErrBackplaneUnavailable] without attempting a request.
	maxConsecutiveFailures int32 = 3

	// probeTimeout bounds individual probe HTTP requests, preventing
	// TCP SYN timeouts (20-30s on Linux) from blocking recovery detection.
	probeTimeout = 5 * time.Second

	// clientTimeout is the outermost deadline for generation requests.
	// Layered hierarchy: provider 2m < pool 2.5m < client 3m.
	clientTimeout = 3 * time.Minute
)

// ── Wire Types ──────────────────────────────────────────────────────────────

// backplaneRequest is the JSON body sent to the backplane.
type backplaneRequest struct {
	Prompt     string `json:"prompt"`
	MaxTokens  int    `json:"max_tokens,omitempty"`
	ServerName string `json:"server_name"`
}

// backplaneResponse is the JSON body received from the backplane on success.
type backplaneResponse struct {
	Text       string `json:"text"`
	TokensUsed int    `json:"tokens_used"`
	Model      string `json:"model"`
	LatencyMs  int64  `json:"latency_ms"`
}

// backplaneError is the JSON body received from the backplane on error.
type backplaneError struct {
	Error      string `json:"error"`
	Type       string `json:"type,omitempty"`
	RetryAfter int    `json:"retry_after,omitempty"`
}

// ── Client ──────────────────────────────────────────────────────────────────

// BackplaneClient is a thin HTTP proxy to the magictools shared LLM backplane.
// Obtain one via [NewBackplaneClient]; never construct directly in production code.
type BackplaneClient struct {
	addr                string        // host:port of the backplane HTTP listener
	token               string        // bearer token for Authorization header
	serverName          string        // identifies this client in request payloads and logs
	model               string        // specific model tier requested ("fast" or "thinking")
	http                *http.Client  // reusable transport with keep-alive
	available           atomic.Bool   // circuit-breaker state (~1 ns read)
	consecutiveFailures atomic.Int32  // trips breaker at maxConsecutiveFailures
	probeNow            chan struct{} // triggers immediate probe with jitter
}

// NewBackplaneClient creates a [BackplaneClient] from the orchestrator-injected
// environment variables. serverName identifies this client in request payloads
// and structured logs.
//
// Returns nil if MCP_LLM_ENABLED != "true" (standalone / non-orchestrated mode).
// The background availability probe exits when ctx is cancelled (server shutdown).
func NewBackplaneClient(ctx context.Context, serverName, model string) *BackplaneClient {
	if os.Getenv("MCP_LLM_ENABLED") != EnvValueTrue {
		return nil
	}

	addr := os.Getenv("MCP_LLM_ADDR")
	token := os.Getenv("MCP_LLM_TOKEN")
	if addr == "" || token == "" {
		slog.Warn("backplane: MCP_LLM_ENABLED=true but MCP_LLM_ADDR or MCP_LLM_TOKEN missing — disabled",
			"server", serverName)
		return nil
	}

	c := &BackplaneClient{
		addr:       addr,
		token:      token,
		serverName: serverName,
		model:      model,
		probeNow:   make(chan struct{}, 1),
		http: &http.Client{
			Timeout: clientTimeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
	// Optimistic start: assume reachable until the first probe fires.
	c.available.Store(true)

	// Background availability probe with jittered interval.
	// Uses a dedicated done channel closed via context.AfterFunc for clean
	// goroutine teardown (STD-MCP-CONTEXT-DETACHMENT-001).
	done := make(chan struct{})
	//nolint:gosec // G404: non-crypto jitter for probe spacing, not security-sensitive
	ticker := time.NewTicker(2*time.Second + rand.N(500*time.Millisecond))
	context.AfterFunc(ctx, func() {
		ticker.Stop()
		close(done)
	})

	go c.probeLoop(ctx, done, ticker, "http://"+addr+"/llm/status")

	slog.Info("backplane: client initialized", "addr", addr, "server", serverName)
	return c
}

// ── Probe Loop ──────────────────────────────────────────────────────────────

// probeLoop runs the background availability probe. It selects on three
// channels: done (shutdown), ticker.C (periodic), and probeNow (immediate).
func (c *BackplaneClient) probeLoop(ctx context.Context, done <-chan struct{}, ticker *time.Ticker, url string) {
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Standard periodic probe.
		case <-c.probeNow:
			// Immediate probe with jitter to prevent probe storms.
			//nolint:gosec // G404: non-crypto jitter for probe de-storming
			time.Sleep(time.Duration(50+rand.IntN(150)) * time.Millisecond)
		}

		// 5-second deadline prevents TCP SYN timeout (20-30s on Linux)
		// from blocking recovery detection.
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		//nolint:gosec // G704: url is orchestrator-injected MCP_LLM_ADDR, not user input
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, http.NoBody)
		if err != nil {
			cancel()
			continue
		}

		//nolint:gosec // G704: probe target is fixed orchestrator backplane status endpoint
		resp, err := c.http.Do(req)
		cancel()

		wasAvailable := c.available.Load()
		isAvailable := err == nil && resp != nil && resp.StatusCode == http.StatusOK
		c.available.Store(isAvailable)
		if isAvailable {
			c.consecutiveFailures.Store(0)
		}
		if resp != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				slog.Debug("backplane: close probe response body", "error", closeErr)
			}
		}
		if !wasAvailable && isAvailable {
			slog.Info("backplane: recovered", "addr", c.addr, "server", c.serverName)
		}
	}
}

// ── Public API ──────────────────────────────────────────────────────────────

// Available reports whether the backplane is currently reachable.
// This is a ~1 ns atomic read — safe to call in hot paths.
func (c *BackplaneClient) Available() bool {
	return c.available.Load()
}

// Generate sends prompt to the fast inference tier and returns the response text.
// Returns [ErrBackplaneUnavailable] if the backplane is unreachable.
// Returns [ErrBackplaneRateLimited] if the orchestrator quota is exhausted.
func (c *BackplaneClient) Generate(ctx context.Context, prompt string) (string, error) {
	return c.generateWithRetry(ctx, "/llm/generate", prompt)
}

// GenerateThinking sends prompt to the extended reasoning tier.
// Preferred for synthesis steps where deeper inference improves quality.
func (c *BackplaneClient) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	return c.generateWithRetry(ctx, "/llm/generate-thinking", prompt)
}

// JSONResponse sends prompt, then [json.Unmarshal]s the response text into target.
// A JSON-output directive is appended to the prompt automatically.
// If the LLM wraps the JSON in markdown fences, they are stripped before parsing.
func (c *BackplaneClient) JSONResponse(ctx context.Context, prompt string, target any) error {
	fullPrompt := prompt + "\n\nReturn strictly valid JSON without markdown wrapping or code blocks."

	path := "/llm/generate"
	if c.model == "thinking" {
		path = "/llm/generate-thinking"
	}

	text, err := c.generateWithRetry(ctx, path, fullPrompt)
	if err != nil {
		return err
	}

	cleaned := strings.TrimSpace(text)

	// Two-step approach: try raw unmarshal first, extract JSON on failure.
	if err := json.Unmarshal([]byte(cleaned), target); err == nil {
		return nil
	}

	// Fallback: extract the substring between the first {/[ and last }/].
	extracted := extractJSON(cleaned)
	return json.Unmarshal([]byte(extracted), target)
}

// ── Internal ────────────────────────────────────────────────────────────────

// generateWithRetry executes a generation request with bounded 429 retry.
// At most 1 retry is attempted on HTTP 429 with a server-reported Retry-After.
func (c *BackplaneClient) generateWithRetry(ctx context.Context, path, prompt string) (string, error) {
	if !c.available.Load() {
		return "", ErrBackplaneUnavailable
	}

	body, err := json.Marshal(backplaneRequest{
		Prompt:     prompt,
		ServerName: c.serverName,
	})
	if err != nil {
		return "", fmt.Errorf("backplane: marshal failed: %w", err)
	}

	// Bounded iterative retry: initial attempt + at most 1 retry on 429.
	var lastErr error
	for attempt := range 2 {
		result, retryAfter, reqErr := c.doRequest(ctx, path, body)
		if reqErr == nil {
			return result, nil
		}
		lastErr = reqErr

		// Only retry on 429 with Retry-After, and only on first attempt.
		if attempt == 0 && retryAfter > 0 && errors.Is(reqErr, ErrBackplaneRateLimited) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(retryAfter) * time.Second):
			}
			continue
		}
		return "", reqErr
	}
	return "", lastErr
}

// doRequest executes a single HTTP POST to the backplane.
// Returns the response text, any retry-after value (seconds), and an error.
func (c *BackplaneClient) doRequest(ctx context.Context, path string, body []byte) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+c.addr+path, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("backplane: request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		// Context-aware failure counting: don't trip breaker on caller-side
		// context cancellation. Only count genuine network errors.
		if ctx.Err() == nil {
			if c.consecutiveFailures.Add(1) >= maxConsecutiveFailures {
				c.available.Store(false)
			}
			// Trigger immediate probe with jitter.
			select {
			case c.probeNow <- struct{}{}:
			default: // non-blocking — probe already pending
			}
		}
		return "", 0, fmt.Errorf("%w: %w", ErrBackplaneUnavailable, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Debug("backplane: close response body", "error", closeErr)
		}
	}()

	// Reset consecutive failure counter on any successful HTTP response.
	c.consecutiveFailures.Store(0)

	switch resp.StatusCode {
	case http.StatusOK:
		var result backplaneResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", 0, fmt.Errorf("backplane: decode failed: %w", err)
		}
		slog.Debug("backplane: response",
			"model", result.Model,
			"latency_ms", result.LatencyMs,
			"server", c.serverName,
		)
		return result.Text, 0, nil

	case http.StatusTooManyRequests:
		var errResp backplaneError
		_ = json.NewDecoder(resp.Body).Decode(&errResp) //nolint:errcheck // best-effort parse of error body
		return "", errResp.RetryAfter, fmt.Errorf("%w: %s", ErrBackplaneRateLimited, errResp.Error)

	default:
		var errResp backplaneError
		_ = json.NewDecoder(resp.Body).Decode(&errResp) //nolint:errcheck // best-effort parse of error body
		return "", 0, fmt.Errorf("backplane: HTTP %d: %s", resp.StatusCode, errResp.Error)
	}
}

// extractJSON finds the outermost JSON object or array in text by locating
// the first '{' or '[' and the last '}' or ']'. This handles markdown fences,
// nested fences, and surrounding prose without fragile prefix/suffix stripping.
func extractJSON(text string) string {
	start := strings.IndexAny(text, "{[")
	if start < 0 {
		return text
	}

	var end int
	if text[start] == '{' {
		end = strings.LastIndex(text, "}")
	} else {
		end = strings.LastIndex(text, "]")
	}

	if end > start {
		return text[start : end+1]
	}
	return text
}
