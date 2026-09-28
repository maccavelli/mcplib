package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ClaudeProvider implements Provider using the Anthropic Claude Messages API via standard http client.
//
// ClaudeProvider does NOT implement Continuer. Anthropic's Messages API is
// stateless (no response-ID chaining, no server-managed history). This is a
// permanent limitation of Anthropic's current API design, not a TODO.
// Callers must replay prior items as messages on every call.
type ClaudeProvider struct {
	apiKey          string
	model           string
	baseURL         string // For testing
	client          *http.Client
	maxTokens       int
	thinkingBudget  int    // extended-thinking token budget for the GenerateThinking path
	reasoningEffort string // effort for the GenerateThinking path (see addMessagesThinking)
	// identity names the client on every request (MADR 0012 §1.4).
	identity clientIdentity
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
		apiKey:          apiKey,
		model:           model,
		baseURL:         baseURL,
		client:          cfg.HTTPClient,
		identity:        identityOf(cfg),
		maxTokens:       cfg.MaxTokens,
		thinkingBudget:  cfg.ThinkingBudget,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

// Name returns the provider's canonical identifier "claude".
func (p *ClaudeProvider) Name() string {
	return ProviderClaude
}

// Generate sends a prompt to Anthropic's Messages API and returns the generated text.
func (p *ClaudeProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateThinking runs Generate with Anthropic extended thinking enabled, satisfying
// ThinkingProvider.
func (p *ClaudeProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateWithTool sends a prompt to Anthropic's Messages API, forcing it to use a specific tool, and returns the JSON arguments.
func (p *ClaudeProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("claude returned no tool_use content")
}

// GenerateWithToolThinking runs GenerateWithTool with extended thinking enabled,
// satisfying ThinkingToolProvider.
func (p *ClaudeProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithToolThinking(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("claude returned no tool_use content")
}

// GenerateItems sends items to Anthropic's Messages API and returns typed output items.
func (p *ClaudeProvider) GenerateItems(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false)
}

// GenerateItemsWithTool sends items to Anthropic's Messages API with a tool specified.
func (p *ClaudeProvider) GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, false)
}

// GenerateItemsThinking sends items to Anthropic's Messages API with thinking enabled.
func (p *ClaudeProvider) GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, true)
}

// GenerateItemsWithToolThinking sends items with both tool calling and thinking enabled.
func (p *ClaudeProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, true)
}

func claudeItemsToMessages(items []Item) []map[string]any {
	var messages []map[string]any
	// appendBlock adds a content block to the previous message when it has the
	// same role and already holds blocks (or, for the assistant, text), so a
	// turn's calls and a turn's results each stay in one message (MADR 0012
	// §2); otherwise it opens a message.
	appendBlock := func(role string, block map[string]any) {
		if n := len(messages); n > 0 && messages[n-1][jsonKeyRole] == role {
			switch content := messages[n-1][jsonKeyContent].(type) {
			case []map[string]any:
				messages[n-1][jsonKeyContent] = append(content, block)
				return
			case string:
				if role == jsonRoleAssistant {
					messages[n-1][jsonKeyContent] = []map[string]any{{jsonKeyType: jsonKeyText, jsonKeyText: content}, block}
					return
				}
			}
		}
		messages = append(messages, map[string]any{jsonKeyRole: role, jsonKeyContent: []map[string]any{block}})
	}
	for _, item := range items {
		switch v := item.(type) {
		case MessageItem:
			role := v.Role
			if role == jsonRoleSystem {
				continue // top-level system field; see systemPrompt
			}
			if role == "" || role == jsonRoleUser {
				role = jsonRoleUser
			} else {
				role = jsonRoleAssistant
			}
			messages = append(messages, map[string]any{
				jsonKeyRole:    role,
				jsonKeyContent: v.Text,
			})
		case FunctionCallItem:
			appendBlock(jsonRoleAssistant, map[string]any{
				jsonKeyType:  "tool_use",
				"id":         v.CallID,
				jsonKeyName:  v.Name,
				jsonKeyInput: toolArguments(v.Arguments),
			})
		case FunctionCallOutputItem:
			appendBlock(jsonRoleUser, map[string]any{
				jsonKeyType:    "tool_result",
				"tool_use_id":  v.CallID,
				jsonKeyContent: v.Output,
			})
		}
	}
	return messages
}

func (p *ClaudeProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
	maxTokens := p.maxTokens
	body := map[string]any{
		jsonKeyModel:    p.model,
		"max_tokens":    maxTokens,
		jsonKeyMessages: claudeItemsToMessages(input),
	}
	if system := systemPrompt(input); system != "" {
		body[jsonKeySystem] = system
	}

	if thinking {
		body["max_tokens"] = addMessagesThinking(body, p.model, p.reasoningEffort, p.thinkingBudget, maxTokens)
	}

	if tool != nil {
		body[jsonKeyTools] = []map[string]any{
			{
				jsonKeyName:        tool.Name,
				jsonKeyDescription: tool.Description,
				"input_schema":     tool.Schema,
			},
		}
		if thinking {
			// Extended thinking is incompatible with a forced tool_choice; steer via "auto".
			body["tool_choice"] = map[string]any{jsonKeyType: "auto"}
		} else {
			body["tool_choice"] = map[string]any{
				jsonKeyType: "tool",
				jsonKeyName: tool.Name,
			}
		}
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("claude: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	p.identity.setUserAgent(req)
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if err := classifyHTTPError(ProviderClaude, resp); err != nil {
		return nil, err
	}

	return decodeClaudeResponse(limitedBody)
}

func decodeClaudeResponse(body io.Reader) (*Response, error) {
	var result struct {
		Content []struct {
			Type     string         `json:"type"`
			Text     string         `json:"text"`
			Thinking string         `json:"thinking"`
			ID       string         `json:"id"`
			Name     string         `json:"name"`
			Input    map[string]any `json:"input"`
		} `json:"content"`
	}
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("claude returned empty content")
	}

	res := &Response{ID: ""} // Claude Messages API is stateless
	for _, b := range result.Content {
		switch b.Type {
		case "thinking":
			text := b.Thinking
			if text == "" {
				text = b.Text
			}
			res.Output = append(res.Output, ReasoningItem{Text: text})
		case jsonKeyText, "":
			if b.Text != "" {
				res.Output = append(res.Output, MessageItem{Role: jsonRoleAssistant, Text: b.Text})
			}
		case "tool_use":
			var argsStr string
			if b.Input != nil {
				argsBytes, err := json.Marshal(b.Input)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal claude tool input: %w", err)
				}
				argsStr = string(argsBytes)
			}
			res.Output = append(res.Output, FunctionCallItem{
				CallID:    b.ID,
				Name:      b.Name,
				Arguments: argsStr,
			})
		}
	}

	if len(res.Output) == 0 {
		return nil, fmt.Errorf("claude returned no usable content")
	}

	return res, nil
}

// DiscoverModels returns curated Claude text models (Models API + catalog),
// with an optional short health probe. Falls back to the static catalog.
func (p *ClaudeProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listClaudeModels(ctx, p.apiKey, p.identity.apply(ProviderConfig{
		HTTPClient: p.client,
		BaseURL:    p.baseURL,
	}))
	if err != nil || len(listed) == 0 {
		listed = StaticModels(ProviderClaude)
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := NewClaude(p.apiKey, modelID, append(p.identity.options(), WithHTTPClient(p.client), WithBaseURL(p.baseURL))...)
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
