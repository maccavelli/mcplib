package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GeminiProvider implements Provider using the Google Gemini API via standard http client.
type GeminiProvider struct {
	apiKey         string
	model          string
	baseURL        string // For testing
	client         *http.Client
	maxTokens      int
	thinkingBudget int // thinkingConfig budget for the GenerateThinking path
}

// dynamicGeminiThinkingBudget (-1) lets the model size its own thinking budget.
const dynamicGeminiThinkingBudget = -1

// NewGemini creates a Gemini provider with the given API key and model.
// Accepts variadic ProviderOption for shared http.Client injection.
func NewGemini(ctx context.Context, apiKey, model string, opts ...ProviderOption) (*GeminiProvider, error) {
	cfg := ApplyOptions(opts)
	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	return &GeminiProvider{
		apiKey:         apiKey,
		model:          model,
		baseURL:        baseURL,
		client:         cfg.HTTPClient,
		maxTokens:      cfg.MaxTokens,
		thinkingBudget: cfg.ThinkingBudget,
	}, nil
}

// genConfig builds the generationConfig map, adding a thinkingConfig when the thinking
// path is requested. A non-positive configured budget maps to -1 (dynamic thinking).
func (p *GeminiProvider) genConfig(thinking bool) map[string]any {
	cfg := map[string]any{"maxOutputTokens": p.maxTokens}
	if thinking {
		budget := p.thinkingBudget
		if budget <= 0 {
			budget = dynamicGeminiThinkingBudget
		}
		cfg["thinkingConfig"] = map[string]any{"thinkingBudget": budget}
	}
	return cfg
}

// Name returns the provider's unique identifier "gemini".
func (p *GeminiProvider) Name() string { return ProviderGemini }

// Generate sends a prompt to the Gemini API and returns the generated text.
func (p *GeminiProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return p.doGenerate(ctx, prompt, false)
}

// GenerateThinking runs Generate with Gemini thinking enabled, satisfying
// ThinkingProvider. includeThoughts is left off, so the returned parts contain only the
// final answer and the existing response parser needs no change.
func (p *GeminiProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	return p.doGenerate(ctx, prompt, true)
}

func (p *GeminiProvider) doGenerate(ctx context.Context, prompt string, thinking bool) (string, error) {
	reqBody, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{jsonKeyText: prompt},
				},
			},
		},
		"generationConfig": p.genConfig(thinking),
	})
	if err != nil {
		return "", fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, p.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// Key in a header (not the URL) so it can't leak via *url.Error in logs.
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer closeResponseBody(resp)

	// Response size limit: 1MB to prevent OOM on runaway model output.
	// Applied BEFORE status check so error response bodies are also bounded.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if resp.StatusCode != http.StatusOK {
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			return "", &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")), Status: resp.StatusCode}
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return "", fmt.Errorf("%w: gemini HTTP %d", ErrAuthFailure, resp.StatusCode)
		case resp.StatusCode >= 500:
			return "", fmt.Errorf("%w: gemini HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return "", fmt.Errorf("%w: gemini HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no content")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

// GenerateWithTool sends a prompt to the Gemini API, forcing it to use a specific tool, and returns the JSON arguments.
func (p *GeminiProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	return p.doGenerateWithTool(ctx, prompt, tool, false)
}

// GenerateWithToolThinking runs GenerateWithTool with Gemini thinking enabled,
// satisfying ThinkingToolProvider. Forced function-calling (mode ANY) is compatible
// with thinkingConfig, so only generationConfig changes.
func (p *GeminiProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	return p.doGenerateWithTool(ctx, prompt, tool, true)
}

func (p *GeminiProvider) doGenerateWithTool(ctx context.Context, prompt string, tool Tool, thinking bool) (string, error) {
	reqBody, err := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{jsonKeyText: prompt},
				},
			},
		},
		jsonKeyTools: []map[string]any{
			{
				"functionDeclarations": []map[string]any{
					{
						jsonKeyName:        tool.Name,
						jsonKeyDescription: tool.Description,
						"parameters":       tool.Schema,
					},
				},
			},
		},
		"toolConfig": map[string]any{
			"functionCallingConfig": map[string]any{
				"mode":                 "ANY",
				"allowedFunctionNames": []string{tool.Name},
			},
		},
		"generationConfig": p.genConfig(thinking),
	})
	if err != nil {
		return "", fmt.Errorf("gemini: marshal tool request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, p.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// Key in a header (not the URL) so it can't leak via *url.Error in logs.
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer closeResponseBody(resp)

	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if resp.StatusCode != http.StatusOK {
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			return "", &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")), Status: resp.StatusCode}
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return "", fmt.Errorf("%w: gemini HTTP %d", ErrAuthFailure, resp.StatusCode)
		case resp.StatusCode >= 500:
			return "", fmt.Errorf("%w: gemini HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return "", fmt.Errorf("%w: gemini HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					FunctionCall struct {
						Args map[string]any `json:"args"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 || result.Candidates[0].Content.Parts[0].FunctionCall.Args == nil {
		return "", fmt.Errorf("gemini returned no function call args")
	}

	argsBytes, err := json.Marshal(result.Candidates[0].Content.Parts[0].FunctionCall.Args)
	if err != nil {
		return "", fmt.Errorf("failed to marshal gemini function args: %w", err)
	}

	return string(argsBytes), nil
}

// DiscoverModels returns a short, curated list of production text models.
// It lists via the free Models API, intersects with the static catalog (never
// dumps dozens of TTS/image/Live/preview IDs), then optionally health-probes
// only that short list. On total probe failure the curated list is still returned.
func (p *GeminiProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listGeminiModels(ctx, p.apiKey, ProviderConfig{
		HTTPClient: p.client,
		BaseURL:    p.baseURL,
	})
	if err != nil || len(listed) == 0 {
		listed = StaticModels(ProviderGemini)
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp := &GeminiProvider{
			apiKey: p.apiKey, model: modelID, baseURL: p.baseURL,
			client: p.client, maxTokens: p.maxTokens,
		}
		return tp.Generate(tCtx, "Respond with ONLY the word Hello")
	})
	if len(healthy) > 0 {
		return healthy, nil
	}
	// Probes failed (network/quota): still return curated IDs for the wizard.
	return listed, nil
}
