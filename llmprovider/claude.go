package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ClaudeProvider implements Provider using the Anthropic Claude API via standard http client.
type ClaudeProvider struct {
	apiKey         string
	model          string
	baseURL        string // For testing
	client         *http.Client
	maxTokens      int
	thinkingBudget int // extended-thinking token budget for the GenerateThinking path
}

// defaultClaudeThinkingBudget is used by GenerateThinking when no budget is configured.
const defaultClaudeThinkingBudget = 4096

// NewClaude creates a new Claude provider instance.
// Accepts variadic ProviderOption for shared http.Client injection and max_tokens configuration.
func NewClaude(apiKey, model string, opts ...ProviderOption) (*ClaudeProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("claude api key is required")
	}
	cfg := ApplyOptions(opts)
	baseURL := "https://api.anthropic.com/v1"
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	return &ClaudeProvider{
		apiKey:         apiKey,
		model:          model,
		baseURL:        baseURL,
		client:         cfg.HTTPClient,
		maxTokens:      cfg.MaxTokens,
		thinkingBudget: cfg.ThinkingBudget,
	}, nil
}

// Name returns the provider's canonical identifier "claude".
func (p *ClaudeProvider) Name() string {
	return ProviderClaude
}

// Generate sends a prompt to Anthropic's Messages API and returns the generated text.
func (p *ClaudeProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return p.doGenerate(ctx, prompt, false)
}

// GenerateThinking runs Generate with Anthropic extended thinking enabled, satisfying
// ThinkingProvider. Used for heavy-reasoning tasks routed to the thinking tier.
func (p *ClaudeProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	return p.doGenerate(ctx, prompt, true)
}

// thinkingParams returns the budget and the effective max_tokens for a thinking request.
// Anthropic requires max_tokens > budget_tokens, so the ceiling is raised when needed.
func (p *ClaudeProvider) thinkingParams() (budget, maxTokens int) {
	budget = p.thinkingBudget
	if budget <= 0 {
		budget = defaultClaudeThinkingBudget
	}
	maxTokens = p.maxTokens
	if maxTokens <= budget {
		maxTokens = budget + defaultClaudeThinkingBudget
	}
	return budget, maxTokens
}

func (p *ClaudeProvider) doGenerate(ctx context.Context, prompt string, thinking bool) (string, error) {
	body := map[string]any{
		jsonKeyModel: p.model,
		"max_tokens": p.maxTokens,
		jsonKeyMessages: []map[string]string{
			{jsonKeyRole: jsonRoleUser, jsonKeyContent: prompt},
		},
	}
	if thinking {
		budget, maxTokens := p.thinkingParams()
		body["max_tokens"] = maxTokens
		body["thinking"] = map[string]any{jsonKeyType: jsonKeyEnabled, "budget_tokens": budget}
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("claude: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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
			return "", fmt.Errorf("%w: claude HTTP %d", ErrAuthFailure, resp.StatusCode)
		case resp.StatusCode >= 500:
			return "", fmt.Errorf("%w: claude HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return "", fmt.Errorf("%w: claude HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("claude returned empty content")
	}

	// Concatenate all text blocks. A leading thinking/tool_use block has empty
	// Text, so returning Content[0].Text alone would drop the real answer.
	var sb strings.Builder
	for _, b := range result.Content {
		if b.Type == jsonKeyText || (b.Type == "" && b.Text != "") {
			sb.WriteString(b.Text)
		}
	}
	out := sb.String()
	if out == "" {
		return "", fmt.Errorf("claude returned no text content")
	}
	return out, nil
}

// GenerateWithTool sends a prompt to Anthropic's Messages API, forcing it to use a specific tool, and returns the JSON arguments.
func (p *ClaudeProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	return p.doGenerateWithTool(ctx, prompt, tool, false)
}

// GenerateWithToolThinking runs GenerateWithTool with extended thinking enabled,
// satisfying ThinkingToolProvider. Anthropic forbids a forced tool_choice while
// thinking is enabled, so the request steers toward the tool via "auto" rather than
// forcing it; the response parser still extracts the tool_use block when present.
func (p *ClaudeProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	return p.doGenerateWithTool(ctx, prompt, tool, true)
}

func (p *ClaudeProvider) doGenerateWithTool(ctx context.Context, prompt string, tool Tool, thinking bool) (string, error) {
	body := map[string]any{
		jsonKeyModel: p.model,
		"max_tokens": p.maxTokens,
		jsonKeyMessages: []map[string]string{
			{jsonKeyRole: jsonRoleUser, jsonKeyContent: prompt},
		},
		jsonKeyTools: []map[string]any{
			{
				jsonKeyName:        tool.Name,
				jsonKeyDescription: tool.Description,
				"input_schema":     tool.Schema,
			},
		},
		"tool_choice": map[string]any{
			jsonKeyType: "tool",
			jsonKeyName: tool.Name,
		},
	}
	if thinking {
		budget, maxTokens := p.thinkingParams()
		body["max_tokens"] = maxTokens
		body["thinking"] = map[string]any{jsonKeyType: jsonKeyEnabled, "budget_tokens": budget}
		// Extended thinking is incompatible with a forced tool_choice; steer via "auto".
		body["tool_choice"] = map[string]any{jsonKeyType: "auto"}
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("claude: marshal tool request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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
			return "", fmt.Errorf("%w: claude HTTP %d", ErrAuthFailure, resp.StatusCode)
		case resp.StatusCode >= 500:
			return "", fmt.Errorf("%w: claude HTTP %d", ErrProviderUnavailable, resp.StatusCode)
		default:
			return "", fmt.Errorf("%w: claude HTTP %d", ErrInvalidRequest, resp.StatusCode)
		}
	}

	var result struct {
		Content []struct {
			Type  string         `json:"type"`
			Input map[string]any `json:"input"`
		} `json:"content"`
	}
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return "", err
	}

	for _, c := range result.Content {
		if c.Type == "tool_use" && c.Input != nil {
			argsBytes, err := json.Marshal(c.Input)
			if err != nil {
				return "", fmt.Errorf("failed to marshal claude tool input: %w", err)
			}
			return string(argsBytes), nil
		}
	}

	return "", fmt.Errorf("claude returned no tool_use content")
}

// DiscoverModels returns curated Claude text models (Models API + catalog),
// with an optional short health probe. Falls back to the static catalog.
func (p *ClaudeProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listClaudeModels(ctx, p.apiKey, ProviderConfig{
		HTTPClient: p.client,
		BaseURL:    p.baseURL,
	})
	if err != nil || len(listed) == 0 {
		listed = StaticModels(ProviderClaude)
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := NewClaude(p.apiKey, modelID, WithHTTPClient(p.client), WithBaseURL(p.baseURL))
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
