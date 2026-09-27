package llmprovider

import (
	"net/http"
	"time"
)

// defaultHTTPClient returns an http.Client with bounded timeouts so a hung or
// non-responsive LLM endpoint can never block a caller indefinitely. The stdlib
// http.DefaultClient has no timeout and must not be used here. A generation may
// take 300 s to its first byte, as the reference clients allow (MADR 0012
// §1.3); callers wanting less set a context deadline. Listings keep their own
// 10 s bound.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 330 * time.Second,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 300 * time.Second,
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
	// ReasoningEffort selects the reasoning effort ("low"|"medium"|"high") for
	// the GenerateThinking path of every provider. Effort APIs send it as is;
	// Claude 4.7 and later send output_config.effort; older Claude and Gemini
	// map "low" to a small budget or thinkingLevel (MADR 0013 Q1). Empty leaves
	// each provider's documented default: medium on the effort APIs, high on
	// Grok 4.5, the model's own on Kilo and Claude 4.7+, a 4096 budget on older
	// Claude, and dynamic thinking on Gemini (MADR 0013 Q2).
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
	// KiloDataCollection allows Kilo upstreams that may train on prompts.
	// false (the default) sends provider.data_collection "deny"; see
	// WithKiloDataCollection. Ignored by all other providers.
	KiloDataCollection bool
	// KiloOrganization scopes Kilo requests to an organization; see
	// WithKiloOrganization. Ignored by all other providers.
	KiloOrganization string
	// ModelProfile selects how the recommended models of the open catalogs
	// (Kilo, OpenCode Zen and Go, Hugging Face) are ranked. The zero value is
	// ProfileUtility. Those providers' DiscoverModels ranks with it too.
	ModelProfile ModelProfile
	// ModelMetadataURL overrides the models.dev-format document the open
	// catalogs are ranked with, and OpenCode's chat route reads
	// reasoning_options from. Empty uses MCPLIB_MODELS_METADATA_URL, then
	// https://models.opencode.ai/api.json.
	ModelMetadataURL string
	// ClientName and ClientVersion name the consuming application in
	// User-Agent (MADR 0012 §1.4); see WithClientInfo.
	ClientName    string
	ClientVersion string
	// SessionID is the conversation id OpenCode and Kilo receive; see
	// WithSessionID.
	SessionID string
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

// WithReasoningEffort sets the reasoning effort ("low"|"medium"|"high") used by the
// provider's GenerateThinking path; see ProviderConfig.ReasoningEffort. An empty value
// leaves each provider's default in effect.
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
// calls and "reasoning" and "reasoning_effort" for the thinking path.
//
// Omit it and every parameter is sent — the gateway is the authority, and
// withholding a parameter we merely cannot confirm would silently degrade
// requests. Ignored by all other providers.
func WithKiloCapabilities(params ...string) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.KiloCapabilities = params
	}
}

// WithKiloDataCollection lets Kilo route to upstreams that may train on
// prompts when allow is true. By default every Kilo request sends
// provider.data_collection "deny", matching the listing's exclusion of such
// models (MADR 0012 §3.3). A model that requires collection is then refused
// with ErrNotPermitted: kilo-auto/free was, and on 2026-09-27 every free text
// model in Kilo's listing was flagged mayTrainOnYourPrompts.
func WithKiloDataCollection(allow bool) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.KiloDataCollection = allow
	}
}

// WithKiloOrganization scopes Kilo generation and listing to an organization:
// requests carry X-KILOCODE-ORGANIZATIONID and the listing is the
// organization's /api/organizations/{id}/models, as Kilo's client does
// (MADR 0012 §3.3). A URL-prefixed token whose path is
// .../api/organizations/{id} names the organization without this option.
func WithKiloOrganization(id string) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.KiloOrganization = id
	}
}

// WithModelProfile selects how ListAvailableModels, ListModelCatalog and the
// open catalogs' DiscoverModels rank the recommended models (MADR 0010 §1,
// MADR 0013 A4).
func WithModelProfile(p ModelProfile) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.ModelProfile = p
	}
}

// WithModelMetadataURL overrides the model metadata document (MADR 0010 §2).
// MCPLIB_DISABLE_MODELS_METADATA=1 turns the fetch off whatever the URL.
func WithModelMetadataURL(url string) ProviderOption {
	return func(cfg *ProviderConfig) {
		cfg.ModelMetadataURL = url
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
