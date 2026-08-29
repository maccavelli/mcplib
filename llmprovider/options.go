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
	// OpencodeRoute overrides the wire format the OpenCode gateway providers use
	// for the configured model. Empty means "resolve from the built-in route
	// table, then the per-gateway prefix heuristic". Set this when a model is
	// newer than the table. Ignored by all other providers.
	OpencodeRoute OpencodeRoute
	// KiloCapabilities lists the request parameters the configured Kilo model
	// accepts (its supported_parameters). Empty means "unknown — send
	// everything". Ignored by all other providers.
	KiloCapabilities []string
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

// WithOpencodeRoute pins the wire format used by the OpenCode Zen/Go providers,
// overriding the built-in route table. Use it when the gateway adds a model
// before this package's table is updated; sending a model to the wrong route
// fails with an opaque HTTP 500. Ignored by all other providers.
func WithOpencodeRoute(route OpencodeRoute) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.OpencodeRoute = route
	}
}

// WithKiloCapabilities declares the request parameters the configured Kilo model
// accepts, as published in that model's supported_parameters (GET {base}/models).
// It gates optional fields the model may reject: "tool_choice" for forced tool
// calls and "reasoning_effort" for the thinking path.
//
// Omit it and every parameter is sent — the gateway is the authority, and
// withholding a parameter we merely cannot confirm would silently degrade
// requests. Ignored by all other providers.
func WithKiloCapabilities(params ...string) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.KiloCapabilities = params
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
