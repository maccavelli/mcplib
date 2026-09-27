package llmprovider

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	openAIAccountHeader    = "ChatGPT-Account-Id"
	openAIResidencyHeader  = "x-openai-internal-codex-residency"
	openAIOriginatorHeader = "originator"
	openAIOriginatorValue  = "mcplib"
	// openAISessionHeader carries the conversation id, as Codex sends it
	// (codex-api/src/requests/headers.rs:8).
	openAISessionHeader = "session-id"
	// openAIFedRAMPHeader marks a FedRAMP account's requests, as Codex's
	// bearer auth does (model-provider/src/bearer_auth_provider.rs:43-45).
	openAIFedRAMPHeader = "X-OpenAI-Fedramp"
)

// NewOpenAIWithSource creates an OpenAI provider from a dynamic token source.
func NewOpenAIWithSource(src TokenSource, model string, opts ...ProviderOption) (*OpenAIProvider, error) {
	if src == nil {
		return nil, errors.New("openai: TokenSource is required")
	}
	cfg := ApplyOptions(opts)
	chatGPT := isChatGPTTokenSource(src)
	baseURL := DefaultOpenAIPlatformBaseURL
	if chatGPT {
		baseURL = DefaultOpenAIChatGPTBaseURL
	}
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	return &OpenAIProvider{
		src:             src,
		chatGPT:         chatGPT,
		model:           model,
		baseURL:         baseURL,
		client:          cfg.HTTPClient,
		identity:        identityOf(cfg),
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

func isChatGPTTokenSource(src TokenSource) bool {
	if vendor, ok := src.(*VendorCLISession); ok {
		return vendor.Provider == ProviderOpenAI
	}
	session, ok := src.(*OAuthSession)
	return ok && session.ChatGPT()
}

func openAIAccountID(src TokenSource) string {
	if vendor, ok := src.(*VendorCLISession); ok {
		accountID, _ := vendor.vendorAccount()
		return accountID
	}
	session, ok := src.(*OAuthSession)
	if !ok {
		return ""
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.AccountID
}

// openAIFedRAMP reports whether src is a FedRAMP ChatGPT session.
func openAIFedRAMP(src TokenSource) bool {
	if vendor, ok := src.(*VendorCLISession); ok {
		_, fedramp := vendor.vendorAccount()
		return fedramp
	}
	session, ok := src.(*OAuthSession)
	if !ok {
		return false
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.FedRAMP
}

func expireOpenAISession(src TokenSource) bool {
	session, ok := src.(*OAuthSession)
	if !ok {
		return false
	}
	session.mu.Lock()
	session.Expiry = time.Now().Add(-time.Second)
	session.mu.Unlock()
	return true
}

// openAIResidency reads chatgpt_compute_residency from the access token for
// the residency header. Its source is OpenCode's Codex plugin
// (plugin/openai/codex.ts:83, :426), not Codex (MADR 0012 §4.4).
func openAIResidency(accessToken string) string {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Residency *string `json:"chatgpt_compute_residency"`
		Auth      struct {
			Residency *string `json:"chatgpt_compute_residency"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	residency := claims.Auth.Residency
	if residency == nil {
		residency = claims.Residency
	}
	if residency == nil || *residency == "" || *residency == "no_constraint" {
		return ""
	}
	return *residency
}
