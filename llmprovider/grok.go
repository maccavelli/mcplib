package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GrokProvider implements Provider using the xAI Grok API (Responses API) via standard http client.
type GrokProvider struct {
	src             TokenSource
	model           string
	baseURL         string // For testing
	client          *http.Client
	maxTokens       int
	reasoningEffort string // reasoning effort for the GenerateThinking path
}

// NewGrok creates a new Grok provider instance.
// Accepts variadic ProviderOption for shared http.Client injection.
func NewGrok(apiKey, model string, opts ...ProviderOption) (*GrokProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("grok api key is required")
	}
	return newGrokWithSource(NewStaticToken(apiKey), model, opts...)
}

func newGrokWithSource(src TokenSource, model string, opts ...ProviderOption) (*GrokProvider, error) {
	if src == nil {
		return nil, errors.New("grok: TokenSource is required")
	}
	if static, ok := src.(*StaticToken); ok && static.Value == "" {
		return nil, errors.New("grok api key is required")
	}
	cfg := ApplyOptions(opts)
	baseURL := DefaultGrokBaseURL
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	return &GrokProvider{
		src:             src,
		model:           model,
		baseURL:         baseURL,
		client:          cfg.HTTPClient,
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

// Name returns the provider's canonical identifier "grok".
func (p *GrokProvider) Name() string { return ProviderGrok }

// Generate sends a prompt to the Grok Responses API and returns the generated text.
func (p *GrokProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateThinking runs Generate with reasoning enabled, satisfying ThinkingProvider.
func (p *GrokProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateWithTool sends a prompt to the Grok Responses API, forcing it to use a specific tool,
// and returns the tool arguments.
func (p *GrokProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("grok returned no function call")
}

// GenerateWithToolThinking runs GenerateWithTool with reasoning enabled,
// satisfying ThinkingToolProvider.
func (p *GrokProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithToolThinking(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("grok returned no function call")
}

// GenerateItems sends items to the Grok Responses API and returns typed output items.
func (p *GrokProvider) GenerateItems(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false, "")
}

// GenerateItemsWithTool sends items to the Grok Responses API with a forced tool call.
func (p *GrokProvider) GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, false, "")
}

// GenerateItemsThinking sends items to the Grok Responses API with reasoning enabled.
func (p *GrokProvider) GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, true, "")
}

// GenerateItemsWithToolThinking sends items with both tool calling and reasoning enabled.
func (p *GrokProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, true, "")
}

// Continue sends items to the Grok Responses API, chaining from a previous response.
func (p *GrokProvider) Continue(ctx context.Context, previousResponseID string, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false, previousResponseID)
}

// itemsToInput converts Item values to the xAI Responses API input format.
func itemsToInput(items []Item) []map[string]any {
	var input []map[string]any
	for _, item := range items {
		switch v := item.(type) {
		case MessageItem:
			input = append(input, map[string]any{
				jsonKeyRole:    v.Role,
				jsonKeyContent: v.Text,
			})
		case FunctionCallOutputItem:
			input = append(input, map[string]any{
				jsonKeyType:   itemTypeFunctionCallOutput,
				jsonKeyCallID: v.CallID,
				jsonKeyOutput: v.Output,
			})
		}
	}
	return input
}

func (p *GrokProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool, prevResponseID string) (*Response, error) {
	response, err := p.doGenerateItemsOnce(ctx, input, tool, thinking, prevResponseID)
	var authErr *grokAuthError
	if err == nil || !errors.As(err, &authErr) || authErr.status != http.StatusUnauthorized || !expireGrokSession(p.src) {
		return response, err
	}
	return p.doGenerateItemsOnce(ctx, input, tool, thinking, prevResponseID)
}

func (p *GrokProvider) doGenerateItemsOnce(ctx context.Context, input []Item, tool *Tool, thinking bool, prevResponseID string) (*Response, error) {
	body := map[string]any{
		jsonKeyModel:        p.model,
		jsonKeyInput:        itemsToInput(input),
		"max_output_tokens": p.maxTokens,
	}

	if tool != nil {
		body[jsonKeyTools] = []map[string]any{
			{
				jsonKeyType:       jsonKeyFunction,
				jsonKeyName:       tool.Name,
				jsonKeyParameters: tool.Schema,
			},
		}
		body["tool_choice"] = map[string]any{
			jsonKeyType: jsonKeyFunction,
			jsonKeyName: tool.Name,
		}
	}

	if thinking {
		effort := grokClampReasoningEffort(p.model, p.reasoningEffort)
		if effort != "" {
			body["reasoning"] = map[string]any{"effort": effort}
		}
	}

	if prevResponseID != "" {
		body["previous_response_id"] = prevResponseID
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("grok: marshal request: %w", err)
	}

	url := p.baseURL + "/responses"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	token, err := p.src.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("grok: acquire token: %w", err)
	}
	req.Header.Set(oauthAuthorizationHeader, "Bearer "+token.Value)

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
			return nil, &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")), Status: resp.StatusCode, Provider: ProviderGrok}
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, &grokAuthError{status: resp.StatusCode}
		case resp.StatusCode >= 500:
			return nil, fmt.Errorf("%w: grok HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return nil, fmt.Errorf("%w: grok HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	return decodeResponsesAPIOutput(limitedBody)
}

// DiscoverModels returns curated Grok text models available to this key, with an
// optional short health probe. Falls back to the static catalog.
func (p *GrokProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := ListAvailableModelsWithSource(
		ctx,
		ProviderGrok,
		p.src,
		WithHTTPClient(p.client),
		WithBaseURL(p.baseURL),
	)
	if err != nil || len(listed) == 0 {
		listed = StaticModels(ProviderGrok)
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := newGrokWithSource(p.src, modelID, WithHTTPClient(p.client), WithBaseURL(p.baseURL))
		if err != nil {
			return "", err
		}
		return tp.Generate(tCtx, "Respond with ONLY the word Hello")
	})
	if len(healthy) > 0 {
		return healthy, nil
	}
	return listed, nil
}

type grokAuthError struct {
	status int
}

func (err *grokAuthError) Error() string {
	return fmt.Sprintf("%v: grok HTTP %d", ErrAuthFailure, err.status)
}

func (err *grokAuthError) Unwrap() error {
	return ErrAuthFailure
}

func expireGrokSession(src TokenSource) bool {
	session, ok := src.(*OAuthSession)
	if !ok {
		return false
	}
	session.mu.Lock()
	session.Expiry = time.Now().Add(-time.Second)
	session.mu.Unlock()
	return true
}
