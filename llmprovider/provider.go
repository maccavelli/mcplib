// Package llmprovider provides a shared, SDK-free LLM provider abstraction
// used by both mcp-server-magictools and mcp-server-magicdev. All providers
// use raw net/http for maximum control over connection pooling and timeouts.
package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
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
	// Provider names the source of the limit, so a 429 is attributable when a
	// caller holds several providers. Empty is valid and reproduces the
	// original message verbatim.
	Provider string
	// Message is the service's own explanation, redacted and bounded (MADR
	// 0012 §1.1). Empty reproduces the original message verbatim.
	Message string
}

func (e *RateLimitError) Error() string {
	msg := fmt.Sprintf("%v: HTTP %d (retry-after %s)", ErrRateLimited, e.Status, e.RetryAfter)
	if e.Provider != "" {
		msg = fmt.Sprintf("%v: %s HTTP %d (retry-after %s)", ErrRateLimited, e.Provider, e.Status, e.RetryAfter)
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// parseRetryAfter parses an HTTP Retry-After header (delta-seconds or HTTP-date),
// returning 0 when absent or unparseable.
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(h, 64); err == nil && secs >= 0 && !math.IsInf(secs, 0) {
		return time.Duration(secs * float64(time.Second))
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// retryAfterFrom reads a server-directed delay from retry-after-ms (fractional
// milliseconds), else Retry-After (MADR 0012 §1.2).
func retryAfterFrom(h http.Header) time.Duration {
	if ms, err := strconv.ParseFloat(strings.TrimSpace(h.Get("Retry-After-Ms")), 64); err == nil && ms >= 0 && !math.IsInf(ms, 0) {
		return time.Duration(ms * float64(time.Millisecond))
	}
	return parseRetryAfter(h.Get("Retry-After"))
}

// retryBackoffCap bounds every retry delay. A server asking for longer gets its
// error back instead, so the caller can reschedule (MADR 0012 §1.2).
const retryBackoffCap = 30 * time.Second

// retryStops reports whether a failed attempt must not be retried: a terminal
// APIError, a server delay beyond retryBackoffCap, or an error whose sentinel
// is never retryable. An APIError's Terminal flag decides even when it also
// matches ErrInvalidRequest for compatibility (a 408 is retried, 0013 B4).
func retryStops(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Terminal || apiErr.RetryAfter > retryBackoffCap
	}
	var rl *RateLimitError
	if errors.As(err, &rl) && rl.RetryAfter > retryBackoffCap {
		return true
	}
	return errors.Is(err, ErrAuthFailure) || errors.Is(err, ErrInvalidRequest)
}

// serverRetryAfter is the delay a failed attempt asked for, or 0.
func serverRetryAfter(err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.RetryAfter
	}
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return rl.RetryAfter
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
	return retryWithBackoff(ctx, retries, delay, "llm: retrying after failure", func() (string, error) {
		return p.Generate(ctx, prompt)
	})
}

// GenerateThinkingWithRetry is GenerateWithRetry for the extended-thinking
// path: the same backoff, jitter and error classification around
// GenerateThinking (MADR 0010 §6).
func GenerateThinkingWithRetry(ctx context.Context, p ThinkingProvider, prompt string, retries int, delay time.Duration) (string, error) {
	return retryWithBackoff(ctx, retries, delay, "llm: retrying thinking after failure", func() (string, error) {
		return p.GenerateThinking(ctx, prompt)
	})
}

// retryWithBackoff runs call up to retries+1 times (at least once) with
// exponential, jittered backoff capped at retryBackoffCap, honouring a
// server-directed delay. It stops at once when retryStops says so, and on
// context cancellation (MADR 0012 §1.2).
func retryWithBackoff[T any](ctx context.Context, retries int, delay time.Duration, logMsg string, call func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	retries = max(retries, 0)
	for i := 0; i <= retries; i++ {
		if i > 0 {
			base := delay << (i - 1) // exponential
			if base <= 0 || base > retryBackoffCap {
				base = retryBackoffCap
			}
			jitteredDelay := base
			if base/4 > 0 {
				//nolint:gosec // G404: non-crypto jitter for retry backoff spacing
				jitteredDelay += rand.N(base / 4)
			}
			if server := serverRetryAfter(lastErr); server > 0 {
				jitteredDelay = server // retryStops already refused one above the cap
			}
			slog.Warn(logMsg,
				"attempt", i,
				"max_attempts", retries+1,
				"delay", jitteredDelay,
			)
			retryTimer := time.NewTimer(jitteredDelay)
			select {
			case <-ctx.Done():
				retryTimer.Stop()
				return zero, ctx.Err()
			case <-retryTimer.C:
				// Ready for next attempt
			}
		}

		res, err := call()
		if err == nil {
			return res, nil
		}
		lastErr = err
		if retryStops(err) {
			return zero, err
		}
	}
	return zero, fmt.Errorf("failed after %d attempts: %w", retries+1, lastErr)
}

// GenerateItemsWithRetry executes a GenerateItems call with the specified number of retries
// and jittered delay, under the same policy as GenerateWithRetry. It will stop
// retrying if the context is cancelled.
func GenerateItemsWithRetry(ctx context.Context, p ItemProvider, input []Item, retries int, delay time.Duration) (*Response, error) {
	return retryWithBackoff(ctx, retries, delay, "llm: retrying items after failure", func() (*Response, error) {
		return p.GenerateItems(ctx, input...)
	})
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
	case ProviderOllama:
		return NewOllama(apiKey, model, opts...)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}

// NewProviderWithSource creates a provider that accepts a dynamic token source.
func NewProviderWithSource(name string, src TokenSource, model string, opts ...ProviderOption) (Provider, error) {
	switch name {
	case ProviderOpenAI:
		return NewOpenAIWithSource(src, model, opts...)
	case ProviderGrok:
		return newGrokWithSource(src, model, opts...)
	default:
		return nil, fmt.Errorf("provider %q does not accept TokenSource", name)
	}
}
