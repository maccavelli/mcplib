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

const huggingFaceBaseURL = "https://router.huggingface.co/v1"

// wireShapesProbedOnHuggingFace is the date this file's wire shapes were measured
// against the live router (see opencode_route.go Step 1.0 pattern): the
// /v1/models metadata field names that drive curation (throughput,
// first_token_latency_ms, supports_tools, architecture.output_modalities), and
// the /v1/responses HTTP-200-with-status:"failed" behaviour that is the reason
// this provider does not use it.
// Re-validate with: go test -tags live_gateways ./llmprovider/ -run Live
const wireShapesProbedOnHuggingFace = "2026-08-29"

// HuggingFaceProvider implements Provider against Hugging Face Inference
// Providers, a routing proxy in front of 18 partner inference providers.
//
// Only POST {base}/chat/completions is used. A POST {base}/responses endpoint
// also exists and returns a Responses envelope, but it is deliberately NOT used:
// it is undocumented, the official docs scope the OpenAI-compatible surface to
// "chat completion tasks only", and on auth failure it returns HTTP 200 with
// status:"failed" and the error in the body (measured 2026-08-29). Under this
// package's status-only classification that would decode as an empty, error-free
// success. Do not "fix" this omission without re-measuring.
//
// HuggingFaceProvider does NOT implement Continuer: Chat Completions is
// stateless, the same reason ClaudeProvider omits it.
type HuggingFaceProvider struct {
	apiKey          string
	model           string
	baseURL         string
	client          *http.Client
	maxTokens       int
	reasoningEffort string
}

// NewHuggingFace creates a Hugging Face Inference Providers router client.
func NewHuggingFace(apiKey, model string, opts ...ProviderOption) (*HuggingFaceProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("huggingface api key is required")
	}
	cfg := ApplyOptions(opts)
	baseURL := huggingFaceBaseURL
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	return &HuggingFaceProvider{
		apiKey:          apiKey,
		model:           model,
		baseURL:         baseURL,
		client:          cfg.HTTPClient,
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

// Name returns the provider's canonical identifier "huggingface".
func (p *HuggingFaceProvider) Name() string { return ProviderHuggingFace }

// Generate sends a prompt to the router and returns the generated text.
func (p *HuggingFaceProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateThinking runs Generate with reasoning_effort set.
//
// Hugging Face documents reasoning_effort support as "provider and
// model-dependent", so an ignored parameter is a normal outcome: this path
// degrades to a plain generation rather than failing.
func (p *HuggingFaceProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateWithTool forces a tool call and returns its JSON arguments.
func (p *HuggingFaceProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return firstFunctionCallArgs(resp, ProviderHuggingFace)
}

// GenerateWithToolThinking runs GenerateWithTool with reasoning_effort set.
func (p *HuggingFaceProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithToolThinking(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return firstFunctionCallArgs(resp, ProviderHuggingFace)
}

// GenerateItems sends items to the router and returns typed output items.
func (p *HuggingFaceProvider) GenerateItems(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false)
}

// GenerateItemsWithTool sends items with a forced tool call.
func (p *HuggingFaceProvider) GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, false)
}

// GenerateItemsThinking sends items with reasoning_effort set.
func (p *HuggingFaceProvider) GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, true)
}

// GenerateItemsWithToolThinking sends items with both tool calling and reasoning.
func (p *HuggingFaceProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, true)
}

func (p *HuggingFaceProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
	effort := ""
	if thinking {
		effort = p.reasoningEffort
		if effort == "" {
			effort = effortMedium
		}
	}
	body := chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
		Tool:            tool,
		ForceTool:       tool != nil,
		ReasoningEffort: effort,
	})

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("huggingface: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// Response size limit: 1MB, applied BEFORE the status check so error
	// response bodies are also bounded.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if err := classifyHTTPStatus(ProviderHuggingFace, resp); err != nil {
		return nil, err
	}
	return decodeChatCompletionsResponse(limitedBody)
}

// DiscoverModels returns curated router models, with a short health probe.
// Falls back to the static catalog.
func (p *HuggingFaceProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listHuggingFaceModels(ctx, p.apiKey, ProviderConfig{
		HTTPClient: p.client,
		BaseURL:    p.baseURL,
	})
	if err != nil || len(listed) == 0 {
		listed = StaticModels(ProviderHuggingFace)
	}

	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := NewHuggingFace(p.apiKey, modelID, WithHTTPClient(p.client), WithBaseURL(p.baseURL))
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
