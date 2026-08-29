// Package llmprovider provides a shared, SDK-free LLM provider abstraction
// used by both mcp-server-magictools and mcp-server-magicdev. All providers
// use raw net/http for maximum control over connection pooling and timeouts.
package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Provider defines the interface for LLM backends.
type Provider interface {
	// Name returns the canonical identifier (e.g., "openai", "gemini", "claude").
	Name() string
	// Generate sends a prompt to the LLM and returns the generated text.
	Generate(ctx context.Context, prompt string) (string, error)
}

// Tool defines an LLM function/tool schema
type Tool struct {
	Name        string
	Description string
	Schema      any
}

// ToolProvider is an optional interface for providers that support tool/function execution
type ToolProvider interface {
	Provider
	GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error)
}

// ModelDiscoverer is an optional interface providers can implement to support
// dynamic querying of available models.
type ModelDiscoverer interface {
	// DiscoverModels returns a list of recommended models sorted by speed/preference.
	DiscoverModels(ctx context.Context) ([]string, error)
}

// ThinkingProvider is an optional interface implemented by providers that can run a
// generation with extended thinking / reasoning enabled (e.g. Claude "thinking",
// OpenAI reasoning_effort, Gemini thinkingConfig). Callers that hold a Provider can
// type-assert to this interface to request the higher-reasoning path for heavy tasks.
type ThinkingProvider interface {
	Provider
	GenerateThinking(ctx context.Context, prompt string) (string, error)
}

// ThinkingToolProvider is the tool-calling counterpart of ThinkingProvider: a forced
// (or, where the API forbids forcing under thinking, strongly-steered) tool/function
// call executed with extended thinking enabled.
type ThinkingToolProvider interface {
	ToolProvider
	GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error)
}

// Typed errors for programmatic classification (go.dev/doc/effective-go).
var (
	ErrRateLimited         = errors.New("llm: rate limited")
	ErrProviderUnavailable = errors.New("llm: provider unavailable")
	ErrAuthFailure         = errors.New("llm: authentication failed")
	// ErrInvalidRequest marks a non-retryable client error (4xx other than 429
	// and 401/403). Retrying it can never succeed.
	ErrInvalidRequest = errors.New("llm: invalid request")
)

// RateLimitError carries a server-directed Retry-After hint for a 429 response.
// It unwraps to ErrRateLimited so existing errors.Is(err, ErrRateLimited) checks
// continue to classify it as retryable.
type RateLimitError struct {
	RetryAfter time.Duration
	Status     int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%v: HTTP %d (retry-after %s)", ErrRateLimited, e.Status, e.RetryAfter)
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// parseRetryAfter parses an HTTP Retry-After header (delta-seconds or HTTP-date),
// returning 0 when absent or unparseable.
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// ProviderEnvVars maps canonical provider names to their standard environment variable.
var ProviderEnvVars = map[string]string{
	ProviderGemini: "GEMINI_API_KEY",
	ProviderClaude: "CLAUDE_API_KEY",
	ProviderOpenAI: "OPENAI_API_KEY",
	ProviderGrok:   "XAI_API_KEY",
	// One credential serves both OpenCode gateways; models.dev declares
	// OPENCODE_API_KEY for each, and neither docs page names any other variable.
	ProviderOpencodeZen: "OPENCODE_API_KEY",
	ProviderOpencodeGo:  "OPENCODE_API_KEY",
	ProviderHuggingFace: "HF_TOKEN",
	ProviderKilo:        "KILO_API_KEY",
}

// GenerateWithRetry executes a Generate call with the specified number of retries
// and jittered delay. It will stop retrying if the context is cancelled.
func GenerateWithRetry(ctx context.Context, p Provider, prompt string, retries int, delay time.Duration) (string, error) {
	const maxBackoff = 30 * time.Second
	var lastErr error
	for i := 0; i <= retries; i++ {
		if i > 0 {
			base := delay << (i - 1) // exponential
			if base <= 0 || base > maxBackoff {
				base = maxBackoff
			}
			jitteredDelay := base
			if base/4 > 0 {
				//nolint:gosec // G404: non-crypto jitter for retry backoff spacing
				jitteredDelay += rand.N(base / 4)
			}
			// Honor a server-directed Retry-After when present (capped at maxBackoff).
			var rl *RateLimitError
			if errors.As(lastErr, &rl) && rl.RetryAfter > 0 {
				jitteredDelay = min(rl.RetryAfter, maxBackoff)
			}
			slog.Warn("llm: retrying after failure",
				"attempt", i,
				"max_attempts", retries+1,
				"delay", jitteredDelay,
			)
			retryTimer := time.NewTimer(jitteredDelay)
			select {
			case <-ctx.Done():
				retryTimer.Stop()
				return "", ctx.Err()
			case <-retryTimer.C:
				// Ready for next attempt
			}
		}

		res, err := p.Generate(ctx, prompt)
		if err == nil {
			return res, nil
		}
		lastErr = err
		// Non-retryable errors will never succeed — stop immediately.
		if errors.Is(err, ErrAuthFailure) || errors.Is(err, ErrInvalidRequest) {
			return "", err
		}
	}
	return "", fmt.Errorf("failed after %d attempts: %w", retries+1, lastErr)
}

// GenerateItemsWithRetry executes a GenerateItems call with the specified number of retries
// and jittered delay. It will stop retrying if the context is cancelled.
func GenerateItemsWithRetry(ctx context.Context, p ItemProvider, input []Item, retries int, delay time.Duration) (*Response, error) {
	const maxBackoff = 30 * time.Second
	var lastErr error
	for i := 0; i <= retries; i++ {
		if i > 0 {
			base := delay << (i - 1) // exponential
			if base <= 0 || base > maxBackoff {
				base = maxBackoff
			}
			jitteredDelay := base
			if base/4 > 0 {
				//nolint:gosec // G404: non-crypto jitter for retry backoff spacing
				jitteredDelay += rand.N(base / 4)
			}
			// Honor a server-directed Retry-After when present (capped at maxBackoff).
			var rl *RateLimitError
			if errors.As(lastErr, &rl) && rl.RetryAfter > 0 {
				jitteredDelay = min(rl.RetryAfter, maxBackoff)
			}
			slog.Warn("llm: retrying items after failure",
				"attempt", i,
				"max_attempts", retries+1,
				"delay", jitteredDelay,
			)
			retryTimer := time.NewTimer(jitteredDelay)
			select {
			case <-ctx.Done():
				retryTimer.Stop()
				return nil, ctx.Err()
			case <-retryTimer.C:
				// Ready for next attempt
			}
		}

		res, err := p.GenerateItems(ctx, input...)
		if err == nil {
			return res, nil
		}
		lastErr = err
		// Non-retryable errors will never succeed — stop immediately.
		if errors.Is(err, ErrAuthFailure) || errors.Is(err, ErrInvalidRequest) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("failed after %d attempts: %w", retries+1, lastErr)
}

// NewProvider creates a Provider by canonical name. Accepts variadic ProviderOption
// for shared http.Client injection and configuration.
func NewProvider(name, apiKey, model string, opts ...ProviderOption) (Provider, error) {
	switch name {
	case ProviderGemini:
		return NewGemini(context.Background(), apiKey, model, opts...)
	case ProviderClaude:
		return NewClaude(apiKey, model, opts...)
	case ProviderOpenAI:
		return NewOpenAI(apiKey, model, opts...)
	case ProviderGrok:
		return NewGrok(apiKey, model, opts...)
	case ProviderOpencodeZen, ProviderOpencodeGo:
		return NewOpencode(name, apiKey, model, opts...)
	case ProviderHuggingFace:
		return NewHuggingFace(apiKey, model, opts...)
	case ProviderKilo:
		return NewKilo(apiKey, model, opts...)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}
