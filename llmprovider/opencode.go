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

// OpencodeProvider implements Provider against the OpenCode Zen and OpenCode Go
// AI gateways. Both are multi-protocol: the gateway dispatches each model to one
// of four upstream wire formats and does NOT normalize them, so this provider
// selects the request shape, path and decoder per model via resolveOpencodeRoute.
// Sending a model to the wrong route fails with an opaque HTTP 500, which
// classifies as the retryable ErrProviderUnavailable — use WithOpencodeRoute to
// override the table when the gateway adds a model.
//
// OpencodeProvider does NOT implement Continuer. The gateway rejects
// previous_response_id with HTTP 400 "referenced response not found or expired"
// (measured 2026-08-28 against a response id seconds old). This is a property of
// the gateway, not a TODO. Callers must replay prior items on every call.
type OpencodeProvider struct {
	gateway         string
	apiKey          string
	model           string
	baseURL         string
	client          *http.Client
	maxTokens       int
	thinkingBudget  int
	reasoningEffort string
	route           OpencodeRoute
}

// NewOpencode creates an OpenCode gateway provider. gateway must be
// ProviderOpencodeZen or ProviderOpencodeGo. The wire format is resolved once,
// here, so a misroute is a construction-time fact rather than a per-call surprise.
func NewOpencode(gateway, apiKey, model string, opts ...ProviderOption) (*OpencodeProvider, error) {
	defaultBase, err := opencodeBaseURL(gateway)
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, fmt.Errorf("opencode api key is required")
	}
	cfg := ApplyOptions(opts)
	baseURL := defaultBase
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	route, err := resolveOpencodeRoute(gateway, model, cfg.OpencodeRoute)
	if err != nil {
		return nil, err
	}
	return &OpencodeProvider{
		gateway:         gateway,
		apiKey:          apiKey,
		model:           model,
		baseURL:         baseURL,
		client:          cfg.HTTPClient,
		maxTokens:       cfg.MaxTokens,
		thinkingBudget:  cfg.ThinkingBudget,
		reasoningEffort: cfg.ReasoningEffort,
		route:           route,
	}, nil
}

// Name returns the gateway identifier this provider was constructed for.
func (p *OpencodeProvider) Name() string { return p.gateway }

// Route reports the wire format resolved for this provider's model.
func (p *OpencodeProvider) Route() OpencodeRoute { return p.route }

// Generate sends a prompt to the gateway and returns the generated text.
func (p *OpencodeProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateThinking runs Generate with reasoning enabled, satisfying ThinkingProvider.
func (p *OpencodeProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateWithTool forces a tool call and returns its JSON arguments.
func (p *OpencodeProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return firstFunctionCallArgs(resp, "opencode")
}

// GenerateWithToolThinking runs GenerateWithTool with reasoning enabled.
func (p *OpencodeProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithToolThinking(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return firstFunctionCallArgs(resp, "opencode")
}

// GenerateItems sends items to the gateway and returns typed output items.
func (p *OpencodeProvider) GenerateItems(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false)
}

// GenerateItemsWithTool sends items with a forced tool call.
func (p *OpencodeProvider) GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, false)
}

// GenerateItemsThinking sends items with reasoning enabled.
func (p *OpencodeProvider) GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, true)
}

// GenerateItemsWithToolThinking sends items with both tool calling and reasoning.
func (p *OpencodeProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, true)
}

// responsesBody builds the OpenAI Responses API shape, reusing itemsToInput.
func (p *OpencodeProvider) responsesBody(input []Item, tool *Tool, thinking bool) map[string]any {
	body := map[string]any{
		jsonKeyModel:           p.model,
		jsonKeyInput:           itemsToInput(input),
		jsonKeyMaxOutputTokens: p.maxTokens,
	}
	if tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			jsonKeyType:        jsonKeyFunction,
			jsonKeyName:        tool.Name,
			jsonKeyDescription: tool.Description,
			jsonKeyParameters:  tool.Schema,
		}}
		body[jsonKeyToolChoice] = map[string]any{jsonKeyType: jsonKeyFunction, jsonKeyName: tool.Name}
	}
	if thinking {
		effort := p.reasoningEffort
		if effort == "" {
			effort = effortMedium
		}
		body[jsonKeyReasoning] = map[string]any{jsonKeyEffort: effort}
	}
	return body
}

// messagesBody builds the Anthropic Messages shape, reusing claudeItemsToMessages.
func (p *OpencodeProvider) messagesBody(input []Item, tool *Tool, thinking bool) map[string]any {
	maxTokens := p.maxTokens
	body := map[string]any{
		jsonKeyModel:    p.model,
		jsonKeyMessages: claudeItemsToMessages(input),
	}
	if thinking {
		budget := p.thinkingBudget
		if budget <= 0 {
			budget = defaultClaudeThinkingBudget
		}
		if maxTokens <= budget {
			maxTokens = budget + defaultClaudeThinkingBudget
		}
		body["thinking"] = map[string]any{jsonKeyType: jsonKeyEnabled, "budget_tokens": budget}
	}
	body[jsonKeyMaxTokens] = maxTokens
	if tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			jsonKeyName:        tool.Name,
			jsonKeyDescription: tool.Description,
			"input_schema":     tool.Schema,
		}}
		if thinking {
			// Extended thinking is incompatible with a forced tool_choice.
			body[jsonKeyToolChoice] = map[string]any{jsonKeyType: "auto"}
		} else {
			body[jsonKeyToolChoice] = map[string]any{jsonKeyType: "tool", jsonKeyName: tool.Name}
		}
	}
	return body
}

// googleBody builds the Gemini generateContent shape, reusing geminiItemsToContents.
func (p *OpencodeProvider) googleBody(input []Item, tool *Tool, thinking bool) map[string]any {
	genCfg := map[string]any{"maxOutputTokens": p.maxTokens}
	if thinking {
		budget := p.thinkingBudget
		if budget <= 0 {
			budget = dynamicGeminiThinkingBudget
		}
		genCfg["thinkingConfig"] = map[string]any{"thinkingBudget": budget}
	}
	body := map[string]any{
		"contents":         geminiItemsToContents(input),
		"generationConfig": genCfg,
	}
	if tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			"functionDeclarations": []map[string]any{{
				jsonKeyName:        tool.Name,
				jsonKeyDescription: tool.Description,
				jsonKeyParameters:  tool.Schema,
			}},
		}}
		body["toolConfig"] = map[string]any{"functionCallingConfig": map[string]any{
			"mode":                 "ANY",
			"allowedFunctionNames": []string{tool.Name},
		}}
	}
	return body
}

// chatBody delegates to the shared primitive. OpenCode's chat route has no
// portable reasoning parameter across the DeepSeek/GLM/Kimi/MiniMax families
// routed there, so ReasoningEffort is deliberately left empty and the thinking
// path returns a plain generation. Asserted by TestOpencode_Thinking_PerRoute.
func (p *OpencodeProvider) chatBody(input []Item, tool *Tool) map[string]any {
	return chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
		Tool:      tool,
		ForceTool: tool != nil,
	})
}

func (p *OpencodeProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
	var body map[string]any
	switch p.route {
	case OpencodeRouteResponses:
		body = p.responsesBody(input, tool, thinking)
	case OpencodeRouteMessages:
		body = p.messagesBody(input, tool, thinking)
	case OpencodeRouteGoogle:
		body = p.googleBody(input, tool, thinking)
	case OpencodeRouteChatCompletions:
		body = p.chatBody(input, tool)
	default:
		return nil, fmt.Errorf("%w: unresolved opencode route for model %q", ErrInvalidRequest, p.model)
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("opencode: marshal request: %w", err)
	}

	url := p.baseURL + p.route.path(p.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// The gateway accepts only Authorization: Bearer on every route — it ignores
	// x-api-key and x-goog-api-key even for the Anthropic- and Google-shaped
	// routes (verified 2026-08-28). Key stays in a header, never the URL.
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// Response size limit: 1MB. Applied BEFORE the status check so error
	// response bodies are also bounded.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if err := classifyHTTPStatus(p.gateway+"/"+string(p.route), resp); err != nil {
		return nil, err
	}

	switch p.route {
	case OpencodeRouteResponses:
		return decodeResponsesAPIOutput(limitedBody)
	case OpencodeRouteMessages:
		return decodeClaudeResponse(limitedBody)
	case OpencodeRouteGoogle:
		// decodeGeminiResponse's error text names the Google wire format, which
		// is accurate here even though the gateway is OpenCode.
		return decodeGeminiResponse(limitedBody)
	default:
		return decodeChatCompletionsResponse(limitedBody)
	}
}

// firstFunctionCallArgs returns the arguments of the first FunctionCallItem in
// resp, or an error naming the provider when the model returned none.
func firstFunctionCallArgs(resp *Response, provider string) (string, error) {
	for _, item := range resp.Output {
		if fc, ok := item.(FunctionCallItem); ok {
			return fc.Arguments, nil
		}
	}
	return "", fmt.Errorf("%s: no function call in response", provider)
}
