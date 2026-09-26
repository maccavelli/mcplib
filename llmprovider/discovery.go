package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ModelCatalog is the result of one model listing, viewed two ways.
type ModelCatalog struct {
	// Recommended is what ListAvailableModels returns: at most
	// MaxListedModels ids, curated against the static catalog.
	Recommended []string
	// Usable is every id the provider's usability filters admit, in listing
	// order, uncapped. It equals Recommended when Live is false.
	Usable []string
	// Live reports whether Usable came from the provider's listing rather
	// than the static catalog.
	Live bool
}

// ListAvailableModels fetches models from a provider listing API when available,
// then curates them against the static catalog so configure UIs never show
// huge unusable lists (embeddings, TTS, Live, image, dated previews, …).
//
// Unlike DiscoverModels(), this performs NO generate health checks and consumes
// no generation tokens. All calls are wrapped with a 10-second hard timeout.
func ListAvailableModels(ctx context.Context, providerName, apiKey string, opts ...ProviderOption) ([]string, error) {
	return ListAvailableModelsWithSource(ctx, providerName, NewStaticToken(apiKey), opts...)
}

// ListAvailableModelsWithSource lists models using a static or refreshable token source.
func ListAvailableModelsWithSource(ctx context.Context, providerName string, src TokenSource, opts ...ProviderOption) ([]string, error) {
	return recommendedOf(ListModelCatalogWithSource(ctx, providerName, src, opts...))
}

// ListModelCatalog performs one model listing and returns both the curated
// recommendation and every usable id (MADR 0009 §1).
func ListModelCatalog(ctx context.Context, providerName, apiKey string, opts ...ProviderOption) (ModelCatalog, error) {
	return ListModelCatalogWithSource(ctx, providerName, NewStaticToken(apiKey), opts...)
}

// ListModelCatalogWithSource is ListModelCatalog with a static or refreshable
// token source. A failed listing degrades to the static catalog with a nil
// error and Live false; only Ollama, an unknown provider, a missing source or
// a token failure return an error.
func ListModelCatalogWithSource(ctx context.Context, providerName string, src TokenSource, opts ...ProviderOption) (ModelCatalog, error) {
	cfg := ApplyOptions(opts)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if strings.EqualFold(providerName, ProviderOpenAI) && isChatGPTTokenSource(src) {
		return staticCatalog(slices.Clone(StaticOpenAIChatGPT)), nil
	}
	if src == nil {
		return ModelCatalog{}, errors.New("model listing: TokenSource is required")
	}
	token, err := src.Token(ctx)
	if err != nil {
		return ModelCatalog{}, fmt.Errorf("model listing: acquire token: %w", err)
	}
	return modelCatalogFor(ctx, providerName, token.Value, cfg)
}

// recommendedOf adapts a catalog result to the ListAvailableModels contract.
func recommendedOf(cat ModelCatalog, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	return cat.Recommended, nil
}

// catalogFrom applies the degrade-to-static contract every lister except
// Ollama has always had: a failed fetch, or one that yields no usable id,
// substitutes the static catalog.
func catalogFrom(usable []string, fetchErr error, static []string, curate func([]string) []string) ModelCatalog {
	if fetchErr != nil || len(usable) == 0 {
		return staticCatalog(static)
	}
	recommended := curate(usable)
	if len(recommended) == 0 {
		return staticCatalog(static)
	}
	return ModelCatalog{Recommended: recommended, Usable: usable, Live: true}
}

// staticCatalog wraps a caller-owned copy of a static catalog.
func staticCatalog(static []string) ModelCatalog {
	return ModelCatalog{Recommended: static, Usable: slices.Clone(static), Live: false}
}

// modelCatalogFor dispatches one provider's fetch and curation. The caller
// owns the timeout.
func modelCatalogFor(ctx context.Context, providerName, apiKey string, cfg ProviderConfig) (ModelCatalog, error) {
	switch p := strings.ToLower(providerName); p {
	case ProviderGemini:
		usable, err := fetchGeminiUsable(ctx, apiKey, cfg)
		return catalogFrom(usable, err, StaticModels(ProviderGemini), curateGemini), nil
	case ProviderOpenAI:
		usable, err := fetchOpenAIUsable(ctx, apiKey, cfg)
		return catalogFrom(usable, err, StaticModels(ProviderOpenAI), curateOpenAI), nil
	case ProviderClaude:
		usable, err := fetchClaudeUsable(ctx, apiKey, cfg)
		return catalogFrom(usable, err, StaticModels(ProviderClaude), curateClaude), nil
	case ProviderGrok:
		usable, err := fetchGrokUsable(ctx, apiKey, cfg)
		return catalogFrom(usable, err, StaticModels(ProviderGrok), curateGrok), nil
	case ProviderOpencodeZen, ProviderOpencodeGo:
		return opencodeCatalog(ctx, p, apiKey, cfg)
	case ProviderHuggingFace:
		usable, err := fetchHuggingFaceUsable(ctx, apiKey, cfg)
		return catalogFrom(usable, err, StaticModels(ProviderHuggingFace), curateHuggingFace), nil
	case ProviderKilo:
		entries, err := fetchKiloCatalog(ctx, apiKey, cfg)
		// Deliberate: a failed Kilo fetch degrades to the static catalog rather
		// than failing, like every other lister here. See catalogFrom.
		return catalogFrom(kiloUsable(entries), err, StaticModels(ProviderKilo), curateKilo), nil
	case ProviderOllama:
		return ollamaCatalog(ctx, cfg)
	default:
		return ModelCatalog{}, fmt.Errorf("unsupported provider for model listing: %s", providerName)
	}
}

// listGeminiModels lists Gemini models and returns a short curated production set.
func listGeminiModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	return recommendedOf(modelCatalogFor(ctx, ProviderGemini, apiKey, cfg))
}

// fetchGeminiUsable returns the usable Gemini text models in listing order.
func fetchGeminiUsable(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}

	url := fmt.Sprintf("%s/models", baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: models endpoint returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name                       string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var available []string
	for _, m := range result.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if isUsableGeminiTextModel(id, m.SupportedGenerationMethods) {
			available = append(available, id)
		}
	}
	return available, nil
}

func curateGemini(usable []string) []string {
	return curateFromCatalog(StaticGemini, usable, func(s string) bool {
		return isUsableGeminiTextModel(s, []string{methodGenerateContent})
	}, RankGeminiModel)
}

// fetchOpenAIUsable returns the usable OpenAI chat models in listing order.
func fetchOpenAIUsable(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := "https://api.openai.com/v1"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	ids, err := fetchDataIDs(ctx, baseURL+"/models", "Bearer "+apiKey, cfg, ProviderOpenAI)
	if err != nil {
		return nil, err
	}
	return filterIDs(ids, isUsableOpenAIChatModel), nil
}

func curateOpenAI(usable []string) []string {
	return curateFromCatalog(StaticOpenAI, usable, isUsableOpenAIChatModel, RankOpenAIModel)
}

// listClaudeModels uses Anthropic's Models API when available; otherwise returns
// the curated static catalog (Anthropic historically lacked a public list endpoint).
func listClaudeModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	return recommendedOf(modelCatalogFor(ctx, ProviderClaude, apiKey, cfg))
}

// fetchClaudeUsable returns the usable Claude text models in listing order.
func fetchClaudeUsable(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := "https://api.anthropic.com"
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/v1/models", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		// Older keys / regional proxies may not support Models API.
		return nil, fmt.Errorf("claude: models endpoint returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var available []string
	for _, m := range result.Data {
		if isUsableClaudeTextModel(m.ID) {
			available = append(available, m.ID)
		}
	}
	return available, nil
}

func curateClaude(usable []string) []string {
	return curateFromCatalog(StaticClaude, usable, isUsableClaudeTextModel, RankClaudeModel)
}

// listOllamaModels fetches installed models from a local Ollama instance.
func listOllamaModels(ctx context.Context, cfg ProviderConfig) ([]string, error) {
	return recommendedOf(ollamaCatalog(ctx, cfg))
}

// ollamaCatalog lists every installed model. Ollama has no static catalog, so
// its errors are returned rather than degraded, and an empty install is a
// successful (Live) listing.
func ollamaCatalog(ctx context.Context, cfg ProviderConfig) (ModelCatalog, error) {
	names, err := fetchOllamaNames(ctx, cfg)
	if err != nil {
		return ModelCatalog{}, err
	}
	recommended := names
	if len(names) > MaxListedModels {
		recommended = slices.Clone(names[:MaxListedModels])
	}
	return ModelCatalog{Recommended: recommended, Usable: names, Live: true}, nil
}

// fetchOllamaNames returns every installed model name in /api/tags order.
func fetchOllamaNames(ctx context.Context, cfg ProviderConfig) ([]string, error) {
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

// fetchGrokUsable returns the usable Grok models in listing order.
func fetchGrokUsable(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := "https://api.x.ai/v1"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	ids, err := fetchDataIDs(ctx, baseURL+"/models", "Bearer "+apiKey, cfg, ProviderGrok)
	if err != nil {
		return nil, err
	}
	return filterIDs(ids, isUsableGrokModel), nil
}

func curateGrok(usable []string) []string {
	return curateFromCatalog(StaticGrok, usable, isUsableGrokModel, RankGrokModel)
}

// fetchDataIDs performs GET endpoint and decodes a {"data":[{"id":…}]} body.
// authorization is sent as the Authorization header when non-empty.
func fetchDataIDs(ctx context.Context, endpoint, authorization string, cfg ProviderConfig, provider string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, http.NoBody)
	if err != nil {
		return nil, err
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: models endpoint returned HTTP %d", provider, resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// filterIDs keeps the ids usable admits, preserving order.
func filterIDs(ids []string, usable func(string) bool) []string {
	var out []string
	for _, id := range ids {
		if usable(id) {
			out = append(out, id)
		}
	}
	return out
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
	return recommendedOf(opencodeCatalog(ctx, gateway, apiKey, cfg))
}

// opencodeCatalog lists one OpenCode gateway. An unknown gateway is an error,
// not a degradation.
func opencodeCatalog(ctx context.Context, gateway, apiKey string, cfg ProviderConfig) (ModelCatalog, error) {
	if _, err := opencodeBaseURL(gateway); err != nil {
		return ModelCatalog{}, err
	}
	usable, fetchErr := fetchOpencodeUsable(ctx, gateway, apiKey, cfg)
	curate := func(usable []string) []string {
		return curateFromCatalog(staticOpencodeCatalog(gateway), usable, isUsableOpencodeModel, RankOpencodeModel)
	}
	return catalogFrom(usable, fetchErr, StaticModels(gateway), curate), nil
}

// fetchOpencodeUsable returns the usable gateway models in listing order.
func fetchOpencodeUsable(ctx context.Context, gateway, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL, err := opencodeBaseURL(gateway)
	if err != nil {
		return nil, err
	}
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	authorization := ""
	if apiKey != "" {
		authorization = "Bearer " + apiKey
	}
	ids, err := fetchDataIDs(ctx, baseURL+"/models", authorization, cfg, "opencode")
	if err != nil {
		return nil, err
	}
	return filterIDs(ids, isUsableOpencodeModel), nil
}

// onlyText reports whether a modality list is exactly ["text"].
func onlyText(mods []string) bool { return len(mods) == 1 && mods[0] == jsonKeyText }

// hasText reports whether a modality list includes "text". A model that also
// accepts images or files still serves a text prompt (MADR 0009 §1b).
func hasText(mods []string) bool { return slices.Contains(mods, jsonKeyText) }

// listHuggingFaceModels fetches the router catalog and curates it using the
// metadata Hugging Face publishes. The endpoint is PUBLIC (200 with no
// credential, verified 2026-08-29), so the Authorization header is optional.
//
// Unlike every other provider in this package, ranking here uses measured
// figures rather than name heuristics: the listing reports throughput
// (tokens/sec) and first_token_latency_ms per provider offering. The sorted
// order is handed to curateFromCatalog with a nil rankFn, which preserves it.
func listHuggingFaceModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	return recommendedOf(modelCatalogFor(ctx, ProviderHuggingFace, apiKey, cfg))
}

// fetchHuggingFaceUsable returns the usable router models, fastest first: input
// must include text and output must be exactly text (MADR 0009 §1b), and at
// least one provider offering must be live.
func fetchHuggingFaceUsable(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := huggingFaceBaseURL
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
		return nil, fmt.Errorf("huggingface: models endpoint returned HTTP %d", resp.StatusCode)
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
		return nil, err
	}

	type scored struct {
		id   string
		tps  float64
		ttft float64
	}
	var ranked []scored
	for _, m := range result.Data {
		if !hasText(m.Architecture.InputModalities) || !onlyText(m.Architecture.OutputModalities) {
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
	return available, nil
}

// curateHuggingFace passes a nil rankFn, which preserves the metadata order.
func curateHuggingFace(usable []string) []string {
	return curateFromCatalog(StaticHuggingFace, usable, isUsableHuggingFaceModel, nil)
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
// Two documented traps are handled in kiloUsable:
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
	return recommendedOf(modelCatalogFor(ctx, ProviderKilo, apiKey, cfg))
}

// kiloUsable returns the usable Kilo models, cheapest first: input must include
// text and output must be exactly text (MADR 0009 §1b), tools must be supported,
// and the training policy applies.
func kiloUsable(entries []kiloCatalogEntry) []string {
	type priced struct {
		id    string
		price float64
	}
	var ranked []priced
	for _, m := range entries {
		if !hasText(m.Architecture.InputModalities) || !onlyText(m.Architecture.OutputModalities) {
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
	return available
}

// curateKilo passes a nil rankFn, which preserves the price ordering.
func curateKilo(usable []string) []string {
	return curateFromCatalog(StaticKilo, usable, isUsableKiloModel, nil)
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
