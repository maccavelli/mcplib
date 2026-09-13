package llmprovider

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	openAIAccountHeader   = "ChatGPT-Account-Id"
	openAIResidencyHeader = "x-openai-internal-codex-residency"
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
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

type openAIAuthError struct {
	status int
}

func (err *openAIAuthError) Error() string {
	return fmt.Sprintf("%v: openai HTTP %d", ErrAuthFailure, err.status)
}

func (err *openAIAuthError) Unwrap() error {
	return ErrAuthFailure
}

func isChatGPTTokenSource(src TokenSource) bool {
	session, ok := src.(*OAuthSession)
	return ok && session.ChatGPT()
}

func openAIAccountID(src TokenSource) string {
	session, ok := src.(*OAuthSession)
	if !ok {
		return ""
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.AccountID
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
