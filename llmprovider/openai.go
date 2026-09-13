package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// OpenAIProvider implements Provider using the OpenAI Responses API via standard http client.
type OpenAIProvider struct {
	src             TokenSource
	chatGPT         bool
	model           string
	baseURL         string // For testing
	client          *http.Client
	maxTokens       int
	reasoningEffort string // reasoning effort for the GenerateThinking path
}

// defaultOpenAIReasoningEffort is used by GenerateThinking when none is configured.
const defaultOpenAIReasoningEffort = effortMedium

// NewOpenAI creates a new OpenAI provider instance.
// Accepts variadic ProviderOption for shared http.Client injection.
func NewOpenAI(apiKey, model string, opts ...ProviderOption) (*OpenAIProvider, error) {
	return NewOpenAIWithSource(NewStaticToken(apiKey), model, opts...)
}

// Name returns the provider's unique identifier "openai".
func (p *OpenAIProvider) Name() string { return ProviderOpenAI }

// Generate sends a prompt to the OpenAI Responses API and returns the generated text.
func (p *OpenAIProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateThinking runs Generate with OpenAI reasoning enabled, satisfying
// ThinkingProvider.
func (p *OpenAIProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateWithTool sends a prompt to the OpenAI Responses API, forcing it to use a specific tool, and returns the tool arguments.
func (p *OpenAIProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("openai returned no function call")
}

// GenerateWithToolThinking runs GenerateWithTool with reasoning enabled, satisfying
// ThinkingToolProvider.
func (p *OpenAIProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithToolThinking(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("openai returned no function call")
}

// GenerateItems sends items to the OpenAI Responses API and returns typed output items.
func (p *OpenAIProvider) GenerateItems(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false, "")
}

// GenerateItemsWithTool sends items to the OpenAI Responses API with a forced tool call.
func (p *OpenAIProvider) GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, false, "")
}

// GenerateItemsThinking sends items to the OpenAI Responses API with reasoning enabled.
func (p *OpenAIProvider) GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, true, "")
}

// GenerateItemsWithToolThinking sends items with both tool calling and reasoning enabled.
func (p *OpenAIProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, true, "")
}

// Continue sends items to the OpenAI Responses API, chaining from a previous response.
func (p *OpenAIProvider) Continue(ctx context.Context, previousResponseID string, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false, previousResponseID)
}

func (p *OpenAIProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool, prevResponseID string) (*Response, error) {
	response, err := p.doGenerateItemsOnce(ctx, input, tool, thinking, prevResponseID)
	var authErr *openAIAuthError
	if err == nil || !errors.As(err, &authErr) || authErr.status != http.StatusUnauthorized || !expireOpenAISession(p.src) {
		return response, err
	}
	return p.doGenerateItemsOnce(ctx, input, tool, thinking, prevResponseID)
}

func (p *OpenAIProvider) doGenerateItemsOnce(ctx context.Context, input []Item, tool *Tool, thinking bool, prevResponseID string) (*Response, error) {
	body := map[string]any{
		jsonKeyModel:        p.model,
		jsonKeyInput:        itemsToInput(input),
		"max_output_tokens": p.maxTokens,
	}

	if tool != nil {
		body[jsonKeyTools] = []map[string]any{
			{
				jsonKeyType:        jsonKeyFunction,
				jsonKeyName:        tool.Name,
				jsonKeyDescription: tool.Description,
				jsonKeyParameters:  tool.Schema,
			},
		}
		body["tool_choice"] = map[string]any{
			jsonKeyType: jsonKeyFunction,
			jsonKeyName: tool.Name,
		}
	}

	if thinking {
		effort := p.reasoningEffort
		if effort == "" {
			effort = defaultOpenAIReasoningEffort
		}
		body["reasoning"] = map[string]any{"effort": effort}
	}

	if prevResponseID != "" {
		body["previous_response_id"] = prevResponseID
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	url := p.baseURL + "/responses"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	token, err := p.src.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("openai: acquire token: %w", err)
	}
	req.Header.Set(oauthAuthorizationHeader, "Bearer "+token.Value)
	if p.chatGPT {
		if accountID := openAIAccountID(p.src); accountID != "" {
			req.Header.Set(openAIAccountHeader, accountID)
		}
		if residency := openAIResidency(token.Value); residency != "" {
			req.Header.Set(openAIResidencyHeader, residency)
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if resp.StatusCode != http.StatusOK {
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")), Status: resp.StatusCode, Provider: ProviderOpenAI}
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, &openAIAuthError{status: resp.StatusCode}
		case resp.StatusCode >= 500:
			return nil, fmt.Errorf("%w: openai HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return nil, fmt.Errorf("%w: openai HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	return decodeResponsesAPIOutput(limitedBody)
}

// DiscoverModels returns curated chat models available to this key, with an
// optional short health probe. Never returns the raw /v1/models dump.
func (p *OpenAIProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := ListAvailableModelsWithSource(
		ctx,
		ProviderOpenAI,
		p.src,
		WithHTTPClient(p.client),
		WithBaseURL(p.baseURL),
	)
	if err != nil || len(listed) == 0 {
		if p.chatGPT {
			listed = append([]string(nil), StaticOpenAIChatGPT...)
		} else {
			listed = StaticModels(ProviderOpenAI)
		}
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := NewOpenAIWithSource(p.src, modelID, WithHTTPClient(p.client), WithBaseURL(p.baseURL))
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
