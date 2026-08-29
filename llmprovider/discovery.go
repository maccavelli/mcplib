package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
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
	case ProviderHuggingFace:
		return listHuggingFaceModels(ctx, apiKey, cfg)
	case ProviderKilo:
		return listKiloModels(ctx, apiKey, cfg)
	case ProviderOllama:
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

// onlyText reports whether a modality list is exactly ["text"].
func onlyText(mods []string) bool { return len(mods) == 1 && mods[0] == jsonKeyText }

// listHuggingFaceModels fetches the router catalog and curates it using the
// metadata Hugging Face publishes. The endpoint is PUBLIC (200 with no
// credential, verified 2026-08-29), so the Authorization header is optional.
//
// Unlike every other provider in this package, ranking here uses measured
// figures rather than name heuristics: the listing reports throughput
// (tokens/sec) and first_token_latency_ms per provider offering. The sorted
// order is handed to curateFromCatalog with a nil rankFn, which preserves it.
func listHuggingFaceModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := huggingFaceBaseURL
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
	if err != nil {
		return StaticModels(ProviderHuggingFace), nil
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(ProviderHuggingFace), nil
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return StaticModels(ProviderHuggingFace), nil
	}

	var result struct {
		Data []struct {
			ID           string `json:"id"`
			Architecture struct {
				InputModalities  []string `json:"input_modalities"`
				OutputModalities []string `json:"output_modalities"`
			} `json:"architecture"`
			Providers []struct {
				Status              string  `json:"status"`
				SupportsTools       bool    `json:"supports_tools"`
				Throughput          float64 `json:"throughput"`
				FirstTokenLatencyMs float64 `json:"first_token_latency_ms"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(ProviderHuggingFace), nil
	}

	type scored struct {
		id   string
		tps  float64
		ttft float64
	}
	var ranked []scored
	for _, m := range result.Data {
		if !onlyText(m.Architecture.InputModalities) || !onlyText(m.Architecture.OutputModalities) {
			continue
		}
		if !isUsableHuggingFaceModel(m.ID) {
			continue
		}
		best := scored{id: m.ID, ttft: math.MaxFloat64}
		live := false
		for _, pr := range m.Providers {
			if pr.Status != "live" {
				continue
			}
			live = true
			if pr.Throughput > best.tps {
				best.tps = pr.Throughput
			}
			if pr.FirstTokenLatencyMs > 0 && pr.FirstTokenLatencyMs < best.ttft {
				best.ttft = pr.FirstTokenLatencyMs
			}
		}
		if live {
			ranked = append(ranked, best)
		}
	}
	// Fastest first; ties broken by lowest time-to-first-token.
	slices.SortStableFunc(ranked, func(a, b scored) int {
		switch {
		case a.tps > b.tps:
			return -1
		case a.tps < b.tps:
			return 1
		case a.ttft < b.ttft:
			return -1
		case a.ttft > b.ttft:
			return 1
		}
		return 0
	})
	available := make([]string, 0, len(ranked))
	for _, r := range ranked {
		available = append(available, r.id)
	}

	// nil rankFn preserves the metadata-derived order above.
	curated := curateFromCatalog(StaticHuggingFace, available, isUsableHuggingFaceModel, nil)
	if len(curated) == 0 {
		return StaticModels(ProviderHuggingFace), nil
	}
	return curated, nil
}

// kiloCatalogEntry is the subset of Kilo's OpenRouter-shaped catalog entry this
// package reads. Shared by listKiloModels and KiloModelCapabilities.
type kiloCatalogEntry struct {
	ID           string `json:"id"`
	Architecture struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Completion string `json:"completion"`
	} `json:"pricing"`
	SupportedParameters   []string `json:"supported_parameters"`
	MayTrainOnYourPrompts bool     `json:"mayTrainOnYourPrompts"`
}

// kiloPriceRank parses Kilo's string pricing into a sortable value. A negative
// or unparseable price means "variable" (the kilo-auto tiers report "-1") and
// sorts last rather than first.
func kiloPriceRank(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v < 0 {
		return math.MaxFloat64
	}
	return v
}

// fetchKiloCatalog performs the shared GET {base}/models. The endpoint is PUBLIC
// (200 with no credential, verified 2026-08-29), so apiKey may be empty.
func fetchKiloCatalog(ctx context.Context, apiKey string, cfg ProviderConfig) ([]kiloCatalogEntry, error) {
	baseURL := kiloBaseURL
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kilo: models endpoint returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Data []kiloCatalogEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// listKiloModels fetches the Kilo catalog and curates it.
//
// Two documented traps are handled here:
//
//  1. pricing.completion is a STRING and is "-1" for the variable-priced
//     kilo-auto/{frontier,balanced,efficient} tiers. A naive ascending sort would
//     rank the most expensive tiers as cheaper than free, so kiloPriceRank sorts
//     negative and unparseable prices LAST.
//  2. Kilo model ids may legitimately END in ":free" (tencent/hy3:free), so
//     nothing is stripped at the colon. splitHuggingFaceModelPolicy must not be
//     used here.
//
// Models flagged mayTrainOnYourPrompts are excluded. That is a POLICY decision,
// not a capability filter — see isUsableKiloModel's comment.
func listKiloModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	entries, err := fetchKiloCatalog(ctx, apiKey, cfg)
	if err != nil {
		// Deliberate: every lister in this file degrades to the static catalog
		// rather than failing, so a configure wizard still offers models when
		// the network is down. Asserted by TestListAvailableModels_*Fallback.
		// The other listers inline the fetch, which hides this from nilerr;
		// extracting fetchKiloCatalog for KiloModelCapabilities made it visible.
		//nolint:nilerr // fallback-to-static is the established contract here
		return StaticModels(ProviderKilo), nil
	}

	type priced struct {
		id    string
		price float64
	}
	var ranked []priced
	for _, m := range entries {
		if !onlyText(m.Architecture.InputModalities) || !onlyText(m.Architecture.OutputModalities) {
			continue
		}
		if m.MayTrainOnYourPrompts { // POLICY — see isUsableKiloModel
			continue
		}
		if !slices.Contains(m.SupportedParameters, jsonKeyTools) {
			continue
		}
		if !isUsableKiloModel(m.ID) {
			continue
		}
		ranked = append(ranked, priced{id: m.ID, price: kiloPriceRank(m.Pricing.Completion)})
	}
	// Cheapest first; "-1" and unparseable prices sort last via kiloPriceRank.
	slices.SortStableFunc(ranked, func(a, b priced) int {
		switch {
		case a.price < b.price:
			return -1
		case a.price > b.price:
			return 1
		}
		return 0
	})
	available := make([]string, 0, len(ranked))
	for _, r := range ranked {
		available = append(available, r.id)
	}

	// nil rankFn preserves the price ordering above.
	curated := curateFromCatalog(StaticKilo, available, isUsableKiloModel, nil)
	if len(curated) == 0 {
		return StaticModels(ProviderKilo), nil
	}
	return curated, nil
}

// KiloModelCapabilities returns the supported_parameters published for one Kilo
// model, for use with WithKiloCapabilities. The catalog endpoint is public, so
// apiKey may be empty. An empty list is a valid answer; an error means the
// catalog was unreachable or the model is absent from it.
func KiloModelCapabilities(ctx context.Context, apiKey, model string, opts ...ProviderOption) ([]string, error) {
	cfg := ApplyOptions(opts)
	entries, err := fetchKiloCatalog(ctx, apiKey, cfg)
	if err != nil {
		return nil, err
	}
	for _, m := range entries {
		if m.ID == model {
			return m.SupportedParameters, nil
		}
	}
	return nil, fmt.Errorf("%w: kilo model %q not in catalog", ErrInvalidRequest, model)
}
