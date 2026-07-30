package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OpenAIProvider implements Provider using the OpenAI API via standard http client.
type OpenAIProvider struct {
	apiKey          string
	model           string
	baseURL         string // For testing
	client          *http.Client
	maxTokens       int
	reasoningEffort string // reasoning effort for the GenerateThinking path
}

// defaultOpenAIReasoningEffort is used by GenerateThinking when none is configured.
const defaultOpenAIReasoningEffort = "medium"

// NewOpenAI creates a new OpenAI provider instance.
// Accepts variadic ProviderOption for shared http.Client injection.
func NewOpenAI(apiKey, model string, opts ...ProviderOption) (*OpenAIProvider, error) {
	cfg := ApplyOptions(opts)
	baseURL := "https://api.openai.com/v1"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	return &OpenAIProvider{
		apiKey:          apiKey,
		model:           model,
		baseURL:         baseURL,
		client:          cfg.HTTPClient,
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

// Name returns the provider's unique identifier "openai".
func (p *OpenAIProvider) Name() string { return ProviderOpenAI }

// Generate sends a prompt to the OpenAI chat completion API and returns the generated text.
func (p *OpenAIProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return p.doGenerate(ctx, prompt, false)
}

// GenerateThinking runs Generate with OpenAI reasoning enabled, satisfying
// ThinkingProvider. Reasoning models reject "max_tokens" and require
// "max_completion_tokens", so the thinking path swaps the token key and adds
// "reasoning_effort".
func (p *OpenAIProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	return p.doGenerate(ctx, prompt, true)
}

// applyTokenAndReasoning sets the correct token-limit key (and reasoning_effort) on an
// OpenAI request body depending on whether the reasoning path is requested.
func (p *OpenAIProvider) applyTokenAndReasoning(body map[string]any, thinking bool) {
	if thinking {
		body["max_completion_tokens"] = p.maxTokens
		effort := p.reasoningEffort
		if effort == "" {
			effort = defaultOpenAIReasoningEffort
		}
		body["reasoning_effort"] = effort
		return
	}
	body["max_tokens"] = p.maxTokens
}

func (p *OpenAIProvider) doGenerate(ctx context.Context, prompt string, thinking bool) (string, error) {
	body := map[string]any{
		jsonKeyModel: p.model,
		jsonKeyMessages: []map[string]string{
			{jsonKeyRole: jsonRoleUser, jsonKeyContent: prompt},
		},
	}
	p.applyTokenAndReasoning(body, thinking)
	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("openai: marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

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
			return "", fmt.Errorf("%w: openai HTTP %d", ErrAuthFailure, resp.StatusCode)
		case resp.StatusCode >= 500:
			return "", fmt.Errorf("%w: openai HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return "", fmt.Errorf("%w: openai HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}

// GenerateWithTool sends a prompt to the OpenAI chat completion API, forcing it to use a specific tool, and returns the tool arguments.
func (p *OpenAIProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	return p.doGenerateWithTool(ctx, prompt, tool, false)
}

// GenerateWithToolThinking runs GenerateWithTool with reasoning enabled, satisfying
// ThinkingToolProvider. OpenAI reasoning models support forced tool_choice; only the
// token-limit key and reasoning_effort change.
func (p *OpenAIProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	return p.doGenerateWithTool(ctx, prompt, tool, true)
}

func (p *OpenAIProvider) doGenerateWithTool(ctx context.Context, prompt string, tool Tool, thinking bool) (string, error) {
	body := map[string]any{
		jsonKeyModel: p.model,
		jsonKeyMessages: []map[string]string{
			{jsonKeyRole: jsonRoleUser, jsonKeyContent: prompt},
		},
		jsonKeyTools: []map[string]any{
			{
				jsonKeyType: jsonKeyFunction,
				jsonKeyFunction: map[string]any{
					jsonKeyName:        tool.Name,
					jsonKeyDescription: tool.Description,
					"parameters":       tool.Schema,
				},
			},
		},
		"tool_choice": map[string]any{
			jsonKeyType: jsonKeyFunction,
			jsonKeyFunction: map[string]string{
				jsonKeyName: tool.Name,
			},
		},
	}
	p.applyTokenAndReasoning(body, thinking)
	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("openai: marshal tool request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

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
			return "", fmt.Errorf("%w: openai HTTP %d", ErrAuthFailure, resp.StatusCode)
		case resp.StatusCode >= 500:
			return "", fmt.Errorf("%w: openai HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return "", fmt.Errorf("%w: openai HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	var result struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 || len(result.Choices[0].Message.ToolCalls) == 0 {
		return "", fmt.Errorf("openai returned no tool calls")
	}

	return result.Choices[0].Message.ToolCalls[0].Function.Arguments, nil
}

// DiscoverModels returns curated chat models available to this key, with an
// optional short health probe. Never returns the raw /v1/models dump.
func (p *OpenAIProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listOpenAIModels(ctx, p.apiKey, ProviderConfig{
		HTTPClient: p.client,
		BaseURL:    p.baseURL,
	})
	if err != nil || len(listed) == 0 {
		listed = StaticModels(ProviderOpenAI)
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := NewOpenAI(p.apiKey, modelID, WithHTTPClient(p.client), WithBaseURL(p.baseURL))
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
