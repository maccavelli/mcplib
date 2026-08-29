package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ListAvailableModels fetches models from a provider listing API when available,
// then curates them against the static catalog so configure UIs never show
// huge unusable lists (embeddings, TTS, Live, image, dated previews, …).
//
// Unlike DiscoverModels(), this performs NO generate health checks and consumes
// no generation tokens. All calls are wrapped with a 10-second hard timeout.
func ListAvailableModels(ctx context.Context, providerName, apiKey string, opts ...ProviderOption) ([]string, error) {
	cfg := ApplyOptions(opts)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	switch strings.ToLower(providerName) {
	case ProviderGemini:
		return listGeminiModels(ctx, apiKey, cfg)
	case ProviderOpenAI:
		return listOpenAIModels(ctx, apiKey, cfg)
	case ProviderClaude:
		return listClaudeModels(ctx, apiKey, cfg)
	case ProviderGrok:
		return listGrokModels(ctx, apiKey, cfg)
	case ProviderOpencodeZen, ProviderOpencodeGo:
		return listOpencodeModels(ctx, strings.ToLower(providerName), apiKey, cfg)
	case "ollama":
		return listOllamaModels(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported provider for model listing: %s", providerName)
	}
}

// listGeminiModels lists Gemini models and returns a short curated production set.
func listGeminiModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}

	url := fmt.Sprintf("%s/models", baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return StaticModels(ProviderGemini), nil
	}
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(ProviderGemini), nil
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return StaticModels(ProviderGemini), nil
	}

	var result struct {
		Models []struct {
			Name                       string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(ProviderGemini), nil
	}

	var available []string
	for _, m := range result.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if isUsableGeminiTextModel(id, m.SupportedGenerationMethods) {
			available = append(available, id)
		}
	}

	curated := curateFromCatalog(StaticGemini, available, func(s string) bool {
		return isUsableGeminiTextModel(s, []string{methodGenerateContent})
	}, RankGeminiModel)
	if len(curated) == 0 {
		return StaticModels(ProviderGemini), nil
	}
	return curated, nil
}

// listOpenAIModels fetches OpenAI models and curates to chat-capable catalog hits.
func listOpenAIModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := "https://api.openai.com/v1"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
	if err != nil {
		return StaticModels(ProviderOpenAI), nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(ProviderOpenAI), nil
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return StaticModels(ProviderOpenAI), nil
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(ProviderOpenAI), nil
	}

	var available []string
	for _, m := range result.Data {
		if isUsableOpenAIChatModel(m.ID) {
			available = append(available, m.ID)
		}
	}

	curated := curateFromCatalog(StaticOpenAI, available, isUsableOpenAIChatModel, RankOpenAIModel)
	if len(curated) == 0 {
		return StaticModels(ProviderOpenAI), nil
	}
	return curated, nil
}

// listClaudeModels uses Anthropic's Models API when available; otherwise returns
// the curated static catalog (Anthropic historically lacked a public list endpoint).
func listClaudeModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := "https://api.anthropic.com"
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/v1/models", http.NoBody)
	if err != nil {
		return StaticModels(ProviderClaude), nil
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(ProviderClaude), nil
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		// Older keys / regional proxies may not support Models API.
		return StaticModels(ProviderClaude), nil
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(ProviderClaude), nil
	}

	var available []string
	for _, m := range result.Data {
		if isUsableClaudeTextModel(m.ID) {
			available = append(available, m.ID)
		}
	}
	if len(available) == 0 {
		return StaticModels(ProviderClaude), nil
	}

	curated := curateFromCatalog(StaticClaude, available, isUsableClaudeTextModel, RankClaudeModel)
	if len(curated) == 0 {
		return StaticModels(ProviderClaude), nil
	}
	return curated, nil
}

// listOllamaModels fetches installed models from a local Ollama instance.
func listOllamaModels(ctx context.Context, cfg ProviderConfig) ([]string, error) {
	baseURL := "http://localhost:11434"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Ollama at %s: %w", baseURL, err)
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	var models []string
	for _, m := range result.Models {
		models = append(models, m.Name)
	}
	if len(models) > MaxListedModels {
		models = models[:MaxListedModels]
	}
	return models, nil
}

// ValidateOllamaURL checks if an Ollama instance is reachable at the given URL
// by calling GET /api/version. Returns nil on success, error on failure.
func ValidateOllamaURL(ctx context.Context, baseURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/version", http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Ollama at %s: %w", baseURL, err)
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// listGrokModels fetches xAI models and curates to usable Grok chat models.
func listGrokModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := "https://api.x.ai/v1"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
	if err != nil {
		return StaticModels(ProviderGrok), nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(ProviderGrok), nil
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return StaticModels(ProviderGrok), nil
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(ProviderGrok), nil
	}

	var available []string
	for _, m := range result.Data {
		if isUsableGrokModel(m.ID) {
			available = append(available, m.ID)
		}
	}

	curated := curateFromCatalog(StaticGrok, available, isUsableGrokModel, RankGrokModel)
	if len(curated) == 0 {
		return StaticModels(ProviderGrok), nil
	}
	return curated, nil
}

// listOpencodeModels fetches the gateway catalog and curates it. The OpenCode
// /models endpoint is PUBLIC — it answers 200 with no credentials (verified
// 2026-08-28) — so the Authorization header is sent only when a key is
// available, and an empty key is not an error.
//
// The listing carries no routing or capability metadata (every entry reports
// owned_by "opencode"), so route selection cannot be derived from it; see
// opencode_route.go.
func listOpencodeModels(ctx context.Context, gateway, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL, err := opencodeBaseURL(gateway)
	if err != nil {
		return nil, err
	}
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
	if err != nil {
		return StaticModels(gateway), nil
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(gateway), nil
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return StaticModels(gateway), nil
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(gateway), nil
	}

	var available []string
	for _, m := range result.Data {
		if isUsableOpencodeModel(m.ID) {
			available = append(available, m.ID)
		}
	}

	curated := curateFromCatalog(staticOpencodeCatalog(gateway), available,
		isUsableOpencodeModel, RankOpencodeModel)
	if len(curated) == 0 {
		return StaticModels(gateway), nil
	}
	return curated, nil
}
