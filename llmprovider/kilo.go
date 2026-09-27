package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const kiloBaseURL = "https://api.kilo.ai/api/gateway"

// wireShapesProbedOnKilo is the date this file's wire shapes were measured
// against the live gateway (see opencode_route.go Step 1.0 pattern): the
// message.reasoning spelling (not reasoning_content), supported_parameters as a
// per-model capability list, pricing.completion as a string with "-1" for
// variable-priced tiers, and /chat/completions answering free models with no
// credential.
// Re-validate with: go test -tags live_gateways ./llmprovider/ -run Live
const wireShapesProbedOnKilo = "2026-08-29"

// KiloProvider implements Provider against Kilo Gateway, the inference API
// behind the Kilo Code agent.
//
// Only POST {base}/chat/completions is used. Kilo also answers POST /responses
// and POST /messages with the SAME model — it is a format-translating gateway,
// unlike OpenCode where routes are per-model and a mismatch returns HTTP 500
// (verified 2026-08-29). Those two routes are undocumented, and Kilo's
// /responses places reasoning text in output[].content[].type=="reasoning_text"
// with summary:[], which decodeResponsesAPIOutput does not read, so it would
// silently drop every trace. Because the gateway translates, one route reaches
// the whole catalog; a second buys no model coverage.
//
// https://kilo.ai/api/openrouter serves a byte-identical catalog but is an
// undocumented alias retained for the editor extension; it is not used here.
//
// KiloProvider does NOT implement Continuer: Chat Completions is stateless.
type KiloProvider struct {
	apiKey          string
	model           string
	baseURL         string
	client          *http.Client
	maxTokens       int
	reasoningEffort string
	modelProfile    ModelProfile
	// caps is the model's supported_parameters set, from WithKiloCapabilities.
	// nil means "unknown" — send the standard request rather than guessing a
	// model lacks a capability.
	caps map[string]struct{}
	// allowDataCollection omits the data_collection "deny" preference.
	allowDataCollection bool
	// org is the organization every request is scoped to, or "".
	org string
	// identity names the client on every request (MADR 0012 §1.4).
	identity clientIdentity
}

// kiloTokenURLRE matches Kilo's URL-prefixed token, "{backend URL}:{secret}"
// (kilocode auth/token.ts:9). The whole token is still the bearer.
var kiloTokenURLRE = regexp.MustCompile(`^(https?://[^:]+(?::\d+)?(?:/[^:]*)?):`)

// kiloOrganizationHeader scopes a request to a Kilo organization.
const kiloOrganizationHeader = "X-KILOCODE-ORGANIZATIONID"

// kiloEndpoints are the URLs and organization one Kilo credential uses
// (MADR 0012 §3.3).
type kiloEndpoints struct {
	gateway string // generation base, {origin}{prefix}/api/gateway by default
	models  string // listing URL
	org     string // organization id, or ""
}

// resolveKiloEndpoints derives the endpoints from the configured base (else
// kiloBaseURL), the token and an explicit organization. A URL-prefixed token
// replaces the base with its origin and path prefix, as Kilo's client does
// (api/url.ts:9-30), and an /api/organizations/{id} path in it names the
// organization when none is given. An organization lists
// {origin}{prefix}/api/organizations/{id}/models (api/models.ts:219).
func resolveKiloEndpoints(baseURL, token, org string) kiloEndpoints {
	base := kiloBaseURL
	if baseURL != "" {
		base = strings.TrimRight(baseURL, "/")
	}
	if m := kiloTokenURLRE.FindStringSubmatch(token); m != nil {
		if u, err := url.Parse(m[1]); err == nil && u.Host != "" {
			base = kiloRoute(u, "gateway")
			if org == "" {
				org = kiloPathOrganization(u)
			}
		}
	}
	e := kiloEndpoints{gateway: base, models: base + "/models", org: org}
	if u, err := url.Parse(base); err == nil && org != "" {
		e.models = kiloRoute(u, "organizations/"+url.PathEscape(org)) + "/models"
	}
	return e
}

// kiloPathSegments splits a URL path into its non-empty segments and returns
// them with the index of the last "api" segment, or -1.
func kiloPathSegments(u *url.URL) (parts []string, api int) {
	parts = strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	for api = len(parts) - 1; api >= 0; api-- {
		if parts[api] == "api" {
			break
		}
	}
	return parts, api
}

// kiloRoute is Kilo's route() (api/url.ts:9-18): the path before the last
// "api" segment, then /api/{name}, without query, fragment or trailing slash.
func kiloRoute(u *url.URL, name string) string {
	parts, api := kiloPathSegments(u)
	if api >= 0 {
		parts = parts[:api]
	}
	return u.Scheme + "://" + u.Host + "/" + strings.Join(append(parts, "api", name), "/")
}

// kiloPathOrganization returns {id} from a path ending .../api/organizations/{id},
// or "".
func kiloPathOrganization(u *url.URL) string {
	parts, api := kiloPathSegments(u)
	if api >= 0 && len(parts) == api+3 && parts[api+1] == "organizations" {
		return parts[api+2]
	}
	return ""
}

// kiloAnonymousToken is the bearer Kilo's own client sends without a login;
// free models answer it and paid ones fail with a typed error (MADR 0012 §1.7).
const kiloAnonymousToken = "anonymous"

// NewKilo creates a Kilo Gateway provider. An empty apiKey uses Kilo's
// anonymous token. A URL-prefixed token ("https://host/prefix:secret") selects
// that backend (MADR 0012 §3.3).
func NewKilo(apiKey, model string, opts ...ProviderOption) (*KiloProvider, error) {
	if apiKey == "" {
		apiKey = kiloAnonymousToken
	}
	cfg := ApplyOptions(opts)
	endpoints := resolveKiloEndpoints(cfg.BaseURL, apiKey, cfg.KiloOrganization)
	var caps map[string]struct{}
	if len(cfg.KiloCapabilities) > 0 {
		caps = make(map[string]struct{}, len(cfg.KiloCapabilities))
		for _, c := range cfg.KiloCapabilities {
			caps[c] = struct{}{}
		}
	}
	return &KiloProvider{
		apiKey:          apiKey,
		model:           model,
		baseURL:         endpoints.gateway,
		client:          cfg.HTTPClient,
		identity:        identityOf(cfg),
		maxTokens:       cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
		modelProfile:    cfg.ModelProfile,
		caps:            caps,

		allowDataCollection: cfg.KiloDataCollection,
		org:                 endpoints.org,
	}, nil
}

// Name returns the provider's canonical identifier "kilo".
func (p *KiloProvider) Name() string { return ProviderKilo }

// supports reports whether the model accepts a request parameter. When caps is
// unknown (nil), it returns true: the gateway is the authority, and refusing to
// send a parameter we merely cannot confirm would silently degrade requests.
func (p *KiloProvider) supports(param string) bool {
	if p.caps == nil {
		return true
	}
	_, ok := p.caps[param]
	return ok
}

// Generate sends a prompt to the gateway and returns the generated text.
func (p *KiloProvider) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateThinking runs Generate with reasoning enabled, when the model accepts it.
func (p *KiloProvider) GenerateThinking(ctx context.Context, prompt string) (string, error) {
	resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

// GenerateWithTool requests a tool call and returns its JSON arguments.
func (p *KiloProvider) GenerateWithTool(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return firstFunctionCallArgs(resp, ProviderKilo)
}

// GenerateWithToolThinking runs GenerateWithTool with reasoning enabled.
func (p *KiloProvider) GenerateWithToolThinking(ctx context.Context, prompt string, tool Tool) (string, error) {
	resp, err := p.GenerateItemsWithToolThinking(ctx, tool, MessageItem{Role: jsonRoleUser, Text: prompt})
	if err != nil {
		return "", err
	}
	return firstFunctionCallArgs(resp, ProviderKilo)
}

// GenerateItems sends items to the gateway and returns typed output items.
func (p *KiloProvider) GenerateItems(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, false)
}

// GenerateItemsWithTool sends items with a tool offered.
func (p *KiloProvider) GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, false)
}

// GenerateItemsThinking sends items with reasoning enabled, when accepted.
func (p *KiloProvider) GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, nil, true)
}

// GenerateItemsWithToolThinking sends items with both tool calling and reasoning.
func (p *KiloProvider) GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error) {
	return p.doGenerateItems(ctx, input, &tool, true)
}

// thinkingFields returns the reasoning fields for one call (MADR 0010 §6).
// When the model accepts "reasoning" (or its capabilities are unknown), Kilo's
// reasoning object is sent: {effort} when an effort is configured, else
// {enabled: true} for the model's default effort. reasoning_effort is sent
// only when the model lists it and not "reasoning".
func (p *KiloProvider) thinkingFields(thinking bool) (effort string, reasoning map[string]any) {
	switch {
	case !thinking:
		return "", nil
	case p.supports(jsonKeyReasoning):
		if p.reasoningEffort != "" {
			return "", map[string]any{jsonKeyEffort: p.reasoningEffort}
		}
		return "", map[string]any{jsonKeyEnabled: true}
	case p.supports(jsonKeyReasoningEffort):
		if p.reasoningEffort != "" {
			return p.reasoningEffort, nil
		}
		return effortMedium, nil
	}
	return "", nil
}

func (p *KiloProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
	effort, reasoning := p.thinkingFields(thinking)
	body := chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
		Tool: tool,
		// 301 of 366 models accept "tools" but only 279 accept "tool_choice";
		// offering the tool unforced is strictly better than a 400.
		ForceTool:       tool != nil && p.supports(jsonKeyToolChoice),
		ReasoningEffort: effort,
		Reasoning:       reasoning,
	})
	if !p.allowDataCollection {
		// Kilo's opt-out from upstreams that train on prompts, which its client
		// sends with hide_prompt_training_models (MADR 0012 §3.3).
		body["provider"] = map[string]any{"data_collection": "deny"}
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("kilo: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	p.identity.setUserAgent(req)
	req.Header.Set(kiloEditorHeader, p.identity.name)
	req.Header.Set(kiloTaskHeader, p.identity.session)
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	if p.org != "" {
		req.Header.Set(kiloOrganizationHeader, p.org)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// Response size limit: 1MB, applied BEFORE the status check so error
	// response bodies are also bounded.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if err := classifyHTTPError(ProviderKilo, resp); err != nil {
		return nil, err
	}
	return decodeChatCompletionsResponse(limitedBody)
}

// DiscoverModels returns the curated gateway listing, falling back to the
// static catalog. It spends no generation on probes (MADR 0012 §1.6).
func (p *KiloProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listKiloModels(ctx, p.apiKey, p.identity.apply(ProviderConfig{
		HTTPClient:       p.client,
		BaseURL:          p.baseURL,
		ModelProfile:     p.modelProfile,
		KiloOrganization: p.org,
	}))
	if err != nil || len(listed) == 0 {
		listed = StaticModels(ProviderKilo)
	}

	// No generation probe: this service meters every call (MADR 0012 §1.6).
	return listed, nil
}
