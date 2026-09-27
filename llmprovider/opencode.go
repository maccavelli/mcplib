package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
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
	metadataURL     string
	modelProfile    ModelProfile
	// identity names the client on every request (MADR 0012 §1.4).
	identity clientIdentity
}

// opencodeSessionHeader carries a stable conversation id. OpenCode Go rejects
// requests without it (400 MissingSessionID, measured 2026-09-26), and both
// gateways use it for routing and prompt caching.
const opencodeSessionHeader = "x-opencode-session"

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
		metadataURL:     cfg.ModelMetadataURL,
		modelProfile:    cfg.ModelProfile,
		identity:        identityOf(cfg),
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
		maxTokens = addMessagesThinking(body, p.model, p.reasoningEffort, p.thinkingBudget, maxTokens)
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
		genCfg["thinkingConfig"] = geminiThinkingConfig(p.model, p.reasoningEffort, p.thinkingBudget)
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

// chatReasoningEffort returns the reasoning_effort for the chat route: the
// configured effort, when this is a thinking call and the model's published
// reasoning_options list it (MADR 0010 §6); otherwise "". Metadata that is
// unavailable, disabled or silent on the model sends nothing. The lookup waits
// at most metadataLookupTimeout, and a failed fetch is not retried for
// modelMetadataRetryAfter (MADR 0013 A6).
func (p *OpencodeProvider) chatReasoningEffort(ctx context.Context, thinking bool) string {
	if !thinking || p.reasoningEffort == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, metadataLookupTimeout)
	defer cancel()
	doc, err := loadModelMetadata(ctx, ProviderConfig{HTTPClient: p.client, ModelMetadataURL: p.metadataURL})
	if err != nil || !slices.Contains(doc.reasoningEfforts(p.gateway, p.model), p.reasoningEffort) {
		return ""
	}
	return p.reasoningEffort
}

// chatBody delegates to the shared primitive. The chat route carries
// reasoning_effort only when chatReasoningEffort resolves one (MADR 0010 §6);
// the DeepSeek/GLM/Kimi/MiniMax families routed there share no other
// portable reasoning parameter. Asserted by TestOpencode_Thinking_PerRoute
// and TestOpencode_ChatReasoningEffort.
func (p *OpencodeProvider) chatBody(input []Item, tool *Tool, effort string) map[string]any {
	return chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
		Tool:            tool,
		ForceTool:       tool != nil,
		ReasoningEffort: effort,
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
		body = p.chatBody(input, tool, p.chatReasoningEffort(ctx, thinking))
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
	p.identity.setUserAgent(req)
	// Each route reads the key from its vendor's header (MADR 0009 §1c); the
	// key stays in a header, never the URL.
	name, value := opencodeKeyHeader(p.route, p.apiKey)
	req.Header.Set(name, value)
	// x-opencode-session is fixed for the provider's lifetime (MADR 0012 §1.4).
	req.Header.Set(opencodeSessionHeader, p.identity.session)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// Response size limit: 1MB. Applied BEFORE the status check so error
	// response bodies are also bounded.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if err := classifyHTTPError(p.gateway+"/"+string(p.route), resp); err != nil {
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

// opencodeKeyHeader returns the header the Zen/Go server reads the key from
// on route r. Each route parses only its vendor's header (MADR 0009 §1c).
func opencodeKeyHeader(r OpencodeRoute, key string) (name, value string) {
	switch r {
	case OpencodeRouteMessages:
		return "x-api-key", key
	case OpencodeRouteGoogle:
		return "x-goog-api-key", key
	default:
		return oauthAuthorizationHeader, "Bearer " + key
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

// DiscoverModels returns curated gateway models, with a short health probe.
// Falls back to the static catalog.
//
// Each probe reconstructs the provider so the per-model route is resolved
// correctly. Cloning p would send every candidate down the first model's wire
// format and 500 on most of them.
func (p *OpencodeProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listOpencodeModels(ctx, p.gateway, p.apiKey, p.identity.apply(ProviderConfig{
		HTTPClient:       p.client,
		BaseURL:          p.baseURL,
		ModelProfile:     p.modelProfile,
		ModelMetadataURL: p.metadataURL,
	}))
	if err != nil || len(listed) == 0 {
		listed = StaticModels(p.gateway)
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := NewOpencode(p.gateway, p.apiKey, modelID,
			append(p.identity.options(), WithHTTPClient(p.client), WithBaseURL(p.baseURL))...)
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
