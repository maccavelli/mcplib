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

const ollamaBaseURL = "http://localhost:11434"

// wireShapesProbedOnOllama is the date this file's wire shapes were measured
// against a running instance (v0.31.1): GET /v1/models returns the OpenAI list
// shape, and POST /v1/chat/completions answers 200 with NO Authorization header.
// Re-validate with: go test -tags live_gateways ./llmprovider/ -run Live
const wireShapesProbedOnOllama = "2026-08-29"

// ollamaMaxEffort is Ollama's highest reasoning level. This package models
// "xhigh" (constants.go), which Ollama does not accept, so it is clamped here.
const ollamaMaxEffort = "max"

// OllamaProvider implements Provider against a local Ollama instance using its
// OpenAI-compatible endpoint, so it reuses this package's shared Chat
// Completions primitive rather than adding a fifth wire format.
//
// Three properties differ from every other provider here, all taken from
// Ollama's published compatibility notes and confirmed against a live v0.31.1:
//
//   - An API key is "required but ignored", so NewOllama accepts an empty key
//     and sends no Authorization header at all. This is the only provider in
//     the package that needs no credential, and the reason
//     ProviderDescriptor has a RequiresAPIKey field.
//   - tool_choice is NOT supported, so ForceTool is always false: tools are
//     offered, never forced. This is the same seam Kilo uses for the models
//     that accept "tools" but not "tool_choice".
//   - reasoning_effort accepts none|low|medium|high|max — "max", not this
//     package's "xhigh", which is clamped by clampOllamaEffort.
//
// Generation uses {baseURL}/v1/chat/completions. Model listing deliberately
// stays on the native /api/tags (listOllamaModels), which is stable and tested;
// /v1/models is a compatibility shim and switching to it would change working
// code for cosmetic symmetry.
//
// OllamaProvider does NOT implement Continuer: Chat Completions is stateless.
type OllamaProvider struct {
	apiKey          string
	model           string
	baseURL         string
	client          *http.Client
	maxTokens       int
	reasoningEffort string
}

// NewOllama creates a provider for a local Ollama instance. Unlike every other
// constructor in this package, apiKey may be empty: Ollama requires no
// credential and ignores any that is sent.
func NewOllama(apiKey, model string, opts ...ProviderOption) (*OllamaProvider, error) {
	cfg := ApplyOptions(opts)
	baseURL := ollamaBaseURL
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	return &OllamaProvider{
		apiKey:          apiKey,
		model:           model,
		baseURL:         baseURL,
		client:          cfg.HTTPClient,
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

// Name returns the provider's canonical identifier "ollama".
func (p *OllamaProvider) Name() string { return ProviderOllama }

// clampOllamaEffort maps this package's effort vocabulary onto Ollama's.
// Ollama accepts none|low|medium|high|max; "xhigh" becomes "max".
func clampOllamaEffort(effort string) string {
	if effort == effortXHigh {
		return ollamaMaxEffort
	}
	return effort
}

// Generate sends a prompt to the local instance and returns the generated text.
func (p *OllamaProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateThinking runs Generate with reasoning_effort set.
func (p *OllamaProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateWithTool offers a tool and returns the first call's JSON arguments.
// Ollama does not support tool_choice, so the call is offered, not forced.
func (p *OllamaProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return firstFunctionCallArgs(resp, ProviderOllama)
}

// GenerateWithToolThinking runs GenerateWithTool with reasoning_effort set.
func (p *OllamaProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithToolThinking(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return firstFunctionCallArgs(resp, ProviderOllama)
}

// GenerateItems sends items and returns typed output items.
func (p *OllamaProvider) GenerateItems(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false)
}

// GenerateItemsWithTool sends items with a tool offered.
func (p *OllamaProvider) GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, false)
}

// GenerateItemsThinking sends items with reasoning_effort set.
func (p *OllamaProvider) GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, true)
}

// GenerateItemsWithToolThinking sends items with both tool calling and reasoning.
func (p *OllamaProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, true)
}

func (p *OllamaProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
	effort := ""
	if thinking {
		effort = p.reasoningEffort
		if effort == "" {
			effort = effortMedium
		}
		effort = clampOllamaEffort(effort)
	}
	body := chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
		Tool: tool,
		// Ollama does not support tool_choice; offering the tool unforced is
		// the only option it accepts.
		ForceTool:       false,
		ReasoningEffort: effort,
	})

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header: Ollama requires no credential and ignores any
	// that is sent, so sending one would be noise on a local socket.

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// Response size limit: 1MB, applied BEFORE the status check so error
	// response bodies are also bounded.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if err := classifyHTTPStatus(ProviderOllama, resp); err != nil {
		return nil, err
	}
	return decodeChatCompletionsResponse(limitedBody)
}

// DiscoverModels returns the models installed on the local instance, with a
// short health probe. There is no static catalog to fall back on: installed
// models are machine-specific.
func (p *OllamaProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listOllamaModels(ctx, ProviderConfig{
		HTTPClient: p.client,
		BaseURL:    p.baseURL,
	})
	if err != nil || len(listed) == 0 {
		return nil, err
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := NewOllama(p.apiKey, modelID, WithHTTPClient(p.client), WithBaseURL(p.baseURL))
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
