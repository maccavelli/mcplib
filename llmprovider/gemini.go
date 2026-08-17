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
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateThinking runs Generate with Gemini thinking enabled, satisfying
// ThinkingProvider.
func (p *GeminiProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateWithTool sends a prompt to the Gemini API, forcing it to use a specific tool, and returns the JSON arguments.
func (p *GeminiProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("gemini returned no function call args")
}

// GenerateWithToolThinking runs GenerateWithTool with Gemini thinking enabled,
// satisfying ThinkingToolProvider.
func (p *GeminiProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithToolThinking(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("gemini returned no function call args")
}

// GenerateItems sends items to the Gemini API and returns typed output items.
func (p *GeminiProvider) GenerateItems(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false, "")
}

// GenerateItemsWithTool sends items to the Gemini API with a forced tool call.
func (p *GeminiProvider) GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, false, "")
}

// GenerateItemsThinking sends items to the Gemini API with thinking enabled.
func (p *GeminiProvider) GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, true, "")
}

// GenerateItemsWithToolThinking sends items with both tool calling and thinking enabled.
func (p *GeminiProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, true, "")
}

// Continue sends items to the Gemini API, chaining from a previous interaction.
func (p *GeminiProvider) Continue(ctx context.Context, previousInteractionID string, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false, previousInteractionID)
}

func geminiItemsToContents(items []Item) []map[string]any {
	var contents []map[string]any
	for _, item := range items {
		switch v := item.(type) {
		case MessageItem:
			role := v.Role
			if role == "" || role == jsonRoleUser {
				role = jsonRoleUser
			} else {
				role = "model"
			}
			contents = append(contents, map[string]any{
				jsonKeyRole: role,
				"parts": []map[string]string{
					{jsonKeyText: v.Text},
				},
			})
		case FunctionCallOutputItem:
			contents = append(contents, map[string]any{
				jsonKeyRole: "function",
				"parts": []map[string]any{
					{
						"functionResponse": map[string]any{
							jsonKeyName: v.CallID,
							"response": map[string]any{
								jsonKeyOutput: v.Output,
							},
						},
					},
				},
			})
		}
	}
	return contents
}

func (p *GeminiProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool, prevInteractionID string) (*Response, error) {
	body := map[string]any{
		"contents":         geminiItemsToContents(input),
		"generationConfig": p.genConfig(thinking),
	}

	if tool != nil {
		body[jsonKeyTools] = []map[string]any{
			{
				"functionDeclarations": []map[string]any{
					{
						jsonKeyName:        tool.Name,
						jsonKeyDescription: tool.Description,
						jsonKeyParameters:  tool.Schema,
					},
				},
			},
		}
		body["toolConfig"] = map[string]any{
			"functionCallingConfig": map[string]any{
				"mode":                 "ANY",
				"allowedFunctionNames": []string{tool.Name},
			},
		}
	}

	if prevInteractionID != "" {
		body["previous_interaction_id"] = prevInteractionID
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, p.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Key in a header (not the URL) so it can't leak via *url.Error in logs.
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// Response size limit: 1MB to prevent OOM on runaway model output.
	// Applied BEFORE status check so error response bodies are also bounded.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if resp.StatusCode != http.StatusOK {
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")), Status: resp.StatusCode}
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, fmt.Errorf("%w: gemini HTTP %d", ErrAuthFailure, resp.StatusCode)
		case resp.StatusCode >= 500:
			return nil, fmt.Errorf("%w: gemini HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return nil, fmt.Errorf("%w: gemini HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	return decodeGeminiResponse(limitedBody)
}

func decodeGeminiResponse(body io.Reader) (*Response, error) {
	var raw struct {
		ID            string `json:"id"`
		InteractionID string `json:"interaction_id"`
		Candidates    []struct {
			Content struct {
				Parts []struct {
					Text         string `json:"text"`
					Thought      string `json:"thought"`
					FunctionCall *struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}

	if len(raw.Candidates) == 0 || len(raw.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned no content")
	}

	id := raw.ID
	if id == "" {
		id = raw.InteractionID
	}

	result := &Response{ID: id}
	for _, part := range raw.Candidates[0].Content.Parts {
		if part.Thought != "" {
			result.Output = append(result.Output, ReasoningItem{Text: part.Thought})
		}
		if part.Text != "" {
			result.Output = append(result.Output, MessageItem{Role: jsonRoleAssistant, Text: part.Text})
		}
		if part.FunctionCall != nil && part.FunctionCall.Args != nil {
			argsBytes, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal gemini function args: %w", err)
			}
			result.Output = append(result.Output, FunctionCallItem{
				CallID:    part.FunctionCall.Name,
				Name:      part.FunctionCall.Name,
				Arguments: string(argsBytes),
			})
		}
	}

	return result, nil
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
