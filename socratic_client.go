package mcplib

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Decision represents a single architectural decision from the Socratic synthesis.
type Decision struct {
	Topic     string `json:"topic"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}

// SocraticMachineResponse is the exact JSON contract returned by the
// socratic_thinker tool when machine_mode=true. Peer MCP servers MUST
// use this struct for deserialization — never parse markdown.
type SocraticMachineResponse struct {
	Decisions            []Decision `json:"decisions"`
	OutstandingQuestions []string   `json:"outstanding_questions"`
	DiagnosticLogHFSCKey string     `json:"diagnostic_log_hfsc_key,omitempty"`
}

// Connection state constants for the tri-state circuit breaker (Fix 3.4).
const (
	stateConnected    int32 = 0 // session active, Analyze() proceeds normally
	stateReconnecting int32 = 1 // session dropped, waiting for recovery
	stateDisconnected int32 = 2 // permanently dead or not yet started
)

// SocraticClient wraps an MCP ClientSession to the socratic-thinker server,
// providing a circuit-breaker-protected Analyze() method for autonomous
// dialectic interactions. Mirrors the robust connectivity design of RecallClient.
type SocraticClient struct {
	url             string
	serverName      string
	mu              sync.Mutex
	session         *mcp.ClientSession
	state           atomic.Int32  // tri-state: CONNECTED/RECONNECTING/DISCONNECTED (Fix 3.4)
	recoveredCh     chan struct{} // closed on RECONNECTING→CONNECTED transition (Fix 3.4)
	logger          *slog.Logger
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	closeOnce       sync.Once
}

// markdownFenceRegex strips markdown code fences that LLMs hallucinate around JSON.
var markdownFenceRe = regexp.MustCompile("(?s)^\\s*```(?:json)?\\s*\n?(.*?)\\s*```\\s*$")

// MarkdownFenceRegex returns the compiled regex used to strip markdown fences.
// Exported for test access only.
func MarkdownFenceRegex() *regexp.Regexp {
	return markdownFenceRe
}

// NewSocraticClient creates a new SocraticClient pointing at the given
// socratic-thinker Streamable HTTP endpoint. serverName identifies the
// calling server in logs (e.g., "go-modernizer").
func NewSocraticClient(url, serverName string) *SocraticClient {
	ctx, cancel := context.WithCancel(context.Background())
	c := &SocraticClient{
		url:             url,
		serverName:      serverName,
		recoveredCh:     make(chan struct{}),
		logger:          slog.Default(),
		lifecycleCtx:    ctx,
		lifecycleCancel: cancel,
	}
	c.state.Store(stateDisconnected)
	return c
}

// Enabled reports whether the Socratic Thinker connection is established
// and the circuit breaker is closed. Callers MUST check this before
// invoking Analyze().
func (c *SocraticClient) Enabled() bool {
	return c.state.Load() == stateConnected
}

// State returns the current connection state as a human-readable string.
// Used for structured telemetry (Fix 4.2).
func (c *SocraticClient) State() string {
	switch c.state.Load() {
	case stateConnected:
		return "CONNECTED"
	case stateReconnecting:
		return "RECONNECTING"
	default:
		return "DISCONNECTED"
	}
}

// Start connects to the socratic-thinker Streamable HTTP endpoint with
// auto-reconnect on crash detection. Blocks until ctx is cancelled.
func (c *SocraticClient) Start(parent context.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("[SOCRATIC] Start goroutine panic recovered", "server", c.serverName, "panic", r)
		}
	}()
	// Tie the reconnect loop to the lifecycle context so Close() stops it.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	context.AfterFunc(c.lifecycleCtx, cancel)

	for {
		if ctx.Err() != nil {
			return
		}

		backoff := 100 * time.Millisecond
		maxBackoff := 5 * time.Second

		for {
			if ctx.Err() != nil {
				return
			}

			client := mcp.NewClient(
				&mcp.Implementation{
					Name:    c.serverName,
					Version: "1.0.0",
				},
				nil,
			)

			transport := &mcp.StreamableClientTransport{
				Endpoint: c.url,
			}

			session, err := client.Connect(ctx, transport, nil)
			if err != nil {
				c.logger.Debug("[SOCRATIC] connection attempt failed",
					"server", c.serverName,
					"url", c.url,
					"error", err,
					"retry_in", backoff,
				)
				c.state.Store(stateReconnecting)

				// Fix 3.5: Add jitter to prevent synchronized reconnection storms
				jitter := time.Duration(rand.IntN(int(backoff / 4)))
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff + jitter):
				}
				backoff = min(backoff*2, maxBackoff)
				continue
			}

			c.mu.Lock()
			c.session = session
			c.mu.Unlock()

			// Fix 3.4: Transition to CONNECTED and wake all waiting goroutines
			c.mu.Lock()
			old := c.recoveredCh
			c.recoveredCh = make(chan struct{}) // allocate new for next cycle
			c.state.Store(stateConnected)
			c.mu.Unlock()
			close(old) // wake ALL waiting Analyze() goroutines

			c.logger.Info("[SOCRATIC] Streamable HTTP connection established",
				"url", c.url, "session_id", session.ID())

			// Wait for session closure
			errCh := make(chan error, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						c.logger.Error("[SOCRATIC] session.Wait goroutine panic recovered", "panic", r)
					}
				}()
				errCh <- session.Wait()
			}()

			select {
			case <-ctx.Done():
				c.logger.Info("[SOCRATIC] context cancelled, closing session", "url", c.url)
				session.Close()
				c.state.Store(stateDisconnected)
				return
			case err := <-errCh:
				if err != nil {
					c.logger.Warn("[SOCRATIC] session closed unexpectedly (crash detected)",
						"server", c.serverName, "url", c.url, "error", err)
				}
				c.state.Store(stateReconnecting)
			}
			break
		}

		// Fix 3.5: Reconnect delay with jitter
		c.logger.Warn("[SOCRATIC] connection dropped, reconnecting...")
		jitter := time.Duration(rand.IntN(1250)) * time.Millisecond
		select {
		case <-ctx.Done():
			return
		case <-time.After(3*time.Second + jitter):
		}
	}
}

// Analyze invokes the socratic_thinker tool with machine_mode=true, driving
// the full Socratic dialectic loop server-side. Returns the structured
// MachineResponse or an error.
//
// maxRounds caps the dialectic depth. Use 0 for the server's default (10).
func (c *SocraticClient) Analyze(ctx context.Context, problem string, maxRounds int) (*SocraticMachineResponse, error) {
	// Fix 3.4: Tri-state handling
	switch c.state.Load() {
	case stateDisconnected:
		return nil, fmt.Errorf("socratic client: circuit breaker open — server unavailable")
	case stateReconnecting:
		// Wait up to 2s for recovery via channel close-and-swap
		c.mu.Lock()
		ch := c.recoveredCh
		c.mu.Unlock()
		select {
		case <-ch:
			// Session recovered, proceed
		case <-time.After(2 * time.Second):
			return nil, fmt.Errorf("socratic client: session recovering, timed out waiting")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case stateConnected:
		// Proceed normally
	}

	c.mu.Lock()
	session := c.session
	c.mu.Unlock()

	if session == nil {
		return nil, fmt.Errorf("socratic client: session not initialized")
	}

	args := map[string]any{
		"stage":        "INITIALIZE",
		"machine_mode": true,
		"prompt":       problem,
	}
	if maxRounds > 0 {
		args["max_rounds"] = maxRounds
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "socratic_thinker",
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("socratic client: RPC failed: %w", err)
	}

	if result == nil {
		return nil, fmt.Errorf("socratic client: nil result")
	}

	// Detect tool-level errors (IsError=true)
	if result.IsError {
		errText := "unknown error"
		for _, ct := range result.Content {
			if tc, ok := ct.(*mcp.TextContent); ok && tc.Text != "" {
				errText = tc.Text
				break
			}
		}
		return nil, fmt.Errorf("socratic client: tool error: %s", errText)
	}

	// Extract raw JSON from TextContent
	var rawJSON string
	for _, ct := range result.Content {
		if tc, ok := ct.(*mcp.TextContent); ok && tc.Text != "" {
			rawJSON = tc.Text
			break
		}
	}
	if rawJSON == "" {
		return nil, fmt.Errorf("socratic client: no text content in response")
	}

	// CRITICAL: Aggressive regex stripping of markdown fences that LLMs
	// with RLHF training routinely hallucinate around complex JSON schemas.
	cleaned := strings.TrimSpace(rawJSON)
	if matches := markdownFenceRe.FindStringSubmatch(cleaned); len(matches) > 1 {
		cleaned = strings.TrimSpace(matches[1])
	}

	var resp SocraticMachineResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("socratic client: failed to unmarshal response (len=%d): %w", len(cleaned), err)
	}

	return &resp, nil
}

// FetchLogs retrieves the full diagnostic lemma trail from the HFSC memory
// channel using the key returned in DiagnosticLogHFSCKey. This is a lazy
// out-of-band retrieval method for debugging rejected mutations.
//
// Note: This calls the socratic_thinker's fetch_hfsc_logs tool if available,
// or returns empty string if the key has expired (10-minute TTL).
func (c *SocraticClient) FetchLogs(ctx context.Context, hfscKey string) string {
	if !c.Enabled() || hfscKey == "" {
		return ""
	}

	c.mu.Lock()
	session := c.session
	c.mu.Unlock()

	if session == nil {
		return ""
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "fetch_hfsc_logs",
		Arguments: map[string]any{
			"key": hfscKey,
		},
	})
	if err != nil {
		c.logger.Warn("[SOCRATIC] FetchLogs failed", "key", hfscKey, "error", err)
		return ""
	}

	for _, ct := range result.Content {
		if tc, ok := ct.(*mcp.TextContent); ok && tc.Text != "" {
			return tc.Text
		}
	}
	return ""
}

// Close gracefully closes the SocraticClient session and stops the reconnect
// loop. Idempotent.
func (c *SocraticClient) Close() {
	c.closeOnce.Do(func() {
		c.lifecycleCancel() // stop the Start() reconnect loop
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.session != nil {
			c.session.Close()
		}
		c.state.Store(stateDisconnected)
	})
}
