package llmprovider

import (
	"net/http"
	"time"
)

// defaultHTTPClient returns an http.Client with conservative timeouts so a hung
// or non-responsive LLM endpoint can never block a caller indefinitely. The
// stdlib http.DefaultClient has no timeout and must not be used here.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   4,
		},
	}
}

// ProviderConfig holds optional configuration for provider constructors.
type ProviderConfig struct {
	HTTPClient *http.Client
	MaxTokens  int
	BaseURL    string // For Ollama URL and test injection
	// ThinkingBudget is the token budget for extended thinking / reasoning, used
	// by the GenerateThinking paths of providers that reason via a token budget
	// (Claude "thinking", Gemini "thinkingConfig"). Zero leaves the per-provider
	// default in effect.
	ThinkingBudget int
	// ReasoningEffort selects OpenAI reasoning effort ("low"|"medium"|"high") for
	// the GenerateThinking path. Empty leaves the per-provider default in effect.
	ReasoningEffort string
}

// ProviderOption is a functional option for provider constructors.
type ProviderOption func(*ProviderConfig)

// WithHTTPClient sets a custom HTTP client for connection pooling.
func WithHTTPClient(c *http.Client) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.HTTPClient = c
	}
}

// WithMaxTokens sets the maximum response tokens for the provider.
func WithMaxTokens(n int) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.MaxTokens = n
	}
}

// WithBaseURL sets a custom base URL for the provider (e.g., Ollama endpoint or test URL).
func WithBaseURL(url string) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.BaseURL = url
	}
}

// WithThinkingBudget sets the extended-thinking/reasoning token budget used by the
// provider's GenerateThinking path (Claude, Gemini). A non-positive value leaves the
// per-provider default in effect.
func WithThinkingBudget(n int) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.ThinkingBudget = n
	}
}

// WithReasoningEffort sets the OpenAI reasoning effort ("low"|"medium"|"high") used by
// the provider's GenerateThinking path. An empty value leaves the default in effect.
func WithReasoningEffort(s string) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.ReasoningEffort = s
	}
}

// ApplyOptions processes variadic ProviderOptions into a ProviderConfig.
func ApplyOptions(opts []ProviderOption) ProviderConfig {
	cfg := ProviderConfig{
		HTTPClient: defaultHTTPClient(),
		MaxTokens:  8192,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}
