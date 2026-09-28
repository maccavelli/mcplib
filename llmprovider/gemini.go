package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GeminiProvider implements Provider over the Gemini Interactions API
// (gemini_interactions.go, MADR 0014). It stores no interaction unless
// WithStore(true), which Continue requires.
type GeminiProvider struct {
	apiKey          string
	model           string
	baseURL         string // For testing
	client          *http.Client
	maxTokens       int
	reasoningEffort string // effort for the GenerateThinking path (see geminiThinkingLevel)
	store           bool   // WithStore(true): interactions are stored and Continue works
	// identity names the client on every request (MADR 0012 §1.4).
	identity clientIdentity
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
		apiKey:          apiKey,
		model:           model,
		baseURL:         baseURL,
		client:          cfg.HTTPClient,
		identity:        identityOf(cfg),
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
		store:           cfg.Store != nil && *cfg.Store,
	}, nil
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

// Continue sends the new items, chaining from a stored interaction. Gemini
// requires store true to chain, so a provider made without WithStore(true)
// returns ErrInvalidRequest without a request (MADR 0014 §2).
func (p *GeminiProvider) Continue(ctx context.Context, previousInteractionID string, input ...Item) (*Response, error) {
	if !p.store {
		return nil, fmt.Errorf("%w: gemini: Continue needs stored interactions; create the provider "+
			"WithStore(true), or replay the items", ErrInvalidRequest)
	}
	return p.doGenerateItems(ctx, input, nil, false, previousInteractionID)
}

// geminiSystemInstruction is generateContent's systemInstruction for the
// system items, or nil when there are none (MADR 0014 §3).
func geminiSystemInstruction(items []Item) map[string]any {
	system := systemPrompt(items)
	if system == "" {
		return nil
	}
	return map[string]any{"parts": []map[string]any{{jsonKeyText: system}}}
}

// geminiItemsToContents is generateContent's contents, for OpenCode's google
// route; GeminiProvider uses interactionsInput.
func geminiItemsToContents(items []Item) []map[string]any {
	// Gemini pairs a functionResponse with its functionCall by name, so a
	// result takes the name of the call it answers (MADR 0012 §2).
	names := map[string]string{}
	for _, item := range items {
		if call, ok := item.(FunctionCallItem); ok {
			names[call.CallID] = call.Name
		}
	}
	var contents []map[string]any
	lastIsResponses := false
	appendPart := func(role string, part map[string]any, merge bool) {
		if n := len(contents); merge && n > 0 && contents[n-1][jsonKeyRole] == role {
			if parts, ok := contents[n-1]["parts"].([]map[string]any); ok {
				contents[n-1]["parts"] = append(parts, part)
				return
			}
		}
		contents = append(contents, map[string]any{jsonKeyRole: role, "parts": []map[string]any{part}})
	}
	for _, item := range items {
		switch v := item.(type) {
		case MessageItem:
			if v.Role == jsonRoleSystem {
				continue // systemInstruction; see geminiSystemInstruction
			}
			role := v.Role
			if role == "" || role == jsonRoleUser {
				role = jsonRoleUser
			} else {
				role = geminiRoleModel
			}
			appendPart(role, map[string]any{jsonKeyText: v.Text}, false)
			lastIsResponses = false
		case FunctionCallItem:
			// Gemini refuses a replayed call without its thoughtSignature. A call
			// Gemini did not issue (synthetic, or from another provider) carries
			// the documented skip value instead.
			signature := v.Signature
			if signature == "" {
				signature = geminiSkipThoughtSignature
			}
			appendPart(geminiRoleModel, map[string]any{
				"functionCall":     map[string]any{jsonKeyName: v.Name, "args": toolArguments(v.Arguments)},
				"thoughtSignature": signature,
			}, true)
			lastIsResponses = false
		case FunctionCallOutputItem:
			name := names[v.CallID]
			if name == "" {
				name = v.CallID
			}
			// A turn's results share one user turn, as its calls share one model turn.
			appendPart(jsonRoleUser, map[string]any{
				"functionResponse": map[string]any{
					jsonKeyName: name,
					"response":  map[string]any{jsonKeyOutput: v.Output},
				},
			}, lastIsResponses)
			lastIsResponses = true
		}
	}
	return contents
}

func (p *GeminiProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool, prevInteractionID string) (*Response, error) {
	reqBody, err := json.Marshal(p.interactionsBody(input, tool, thinking, prevInteractionID))
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+interactionsPath, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	p.identity.setUserAgent(req)
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

	if err := classifyHTTPError(ProviderGemini, resp); err != nil {
		return nil, err
	}

	return decodeInteraction(limitedBody)
}

func decodeGeminiResponse(body io.Reader) (*Response, error) {
	var raw struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string `json:"text"`
					Thought      bool   `json:"thought"`
					FunctionCall *struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args"`
					} `json:"functionCall"`
					ThoughtSignature string `json:"thoughtSignature"`
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

	result := &Response{}
	for _, part := range raw.Candidates[0].Content.Parts {
		// A thought summary is flagged thought: true, with its text in text.
		if part.Thought {
			if part.Text != "" {
				result.Output = append(result.Output, ReasoningItem{Text: part.Text})
			}
			continue
		}
		if part.Text != "" {
			result.Output = append(result.Output, MessageItem{Role: jsonRoleAssistant, Text: part.Text})
		}
		if part.FunctionCall != nil {
			args := part.FunctionCall.Args
			if args == nil {
				args = map[string]any{} // a tool without parameters
			}
			argsBytes, err := json.Marshal(args)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal gemini function args: %w", err)
			}
			result.Output = append(result.Output, FunctionCallItem{
				CallID:    part.FunctionCall.Name,
				Name:      part.FunctionCall.Name,
				Arguments: string(argsBytes),
				Signature: part.ThoughtSignature,
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
	listed, err := listGeminiModels(ctx, p.apiKey, p.identity.apply(ProviderConfig{
		HTTPClient: p.client,
		BaseURL:    p.baseURL,
	}))
	if err != nil || len(listed) == 0 {
		listed = StaticModels(ProviderGemini)
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp := &GeminiProvider{
			apiKey: p.apiKey, model: modelID, baseURL: p.baseURL,
			client: p.client, maxTokens: p.maxTokens, identity: p.identity,
		}
		return tp.Generate(tCtx, "Respond with ONLY the word Hello")
	})
	if len(healthy) > 0 {
		return healthy, nil
	}
	// Probes failed (network/quota): still return curated IDs for the wizard.
	return listed, nil
}
