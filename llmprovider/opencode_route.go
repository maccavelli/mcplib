package llmprovider

import (
	"fmt"
	"strings"
)

// Default gateway base URLs. Both gateways share one credential and one auth
// scheme (Authorization: Bearer); they differ only in base URL, catalog, and
// per-model routing.
const (
	opencodeZenBaseURL = "https://opencode.ai/zen/v1"
	opencodeGoBaseURL  = "https://opencode.ai/zen/go/v1"
)

// wireShapesProbedOnOpencode is the date every wire shape in this file was measured
// against the live gateway: the per-gateway route tables, which routes reject
// which models (a mismatch returns HTTP 500, not a typed error), and the
// response envelopes each route returns.
//
// The OpenCode gateways are remote and continuously deployed. They expose no
// version endpoint, so there is nothing to compare this against at runtime and
// nothing warns when it goes stale — unlike a local engine, which can report a
// version on boot (see magic-cli-remote/internal/provider/opencode/version.go).
// Re-validate with: go test -tags live_gateways ./llmprovider/ -run Live
const wireShapesProbedOnOpencode = "2026-08-28"

// OpencodeRoute identifies which wire format the OpenCode gateway expects for a
// given model. OpenCode Zen and Go are multi-protocol gateways: they do not
// normalize to a single request shape, and sending a model to the wrong route
// fails with an opaque HTTP 500 rather than a typed error.
type OpencodeRoute string

// The four wire formats the OpenCode gateways dispatch to.
const (
	OpencodeRouteResponses       OpencodeRoute = "responses"
	OpencodeRouteMessages        OpencodeRoute = "messages"
	OpencodeRouteChatCompletions OpencodeRoute = "chat_completions"
	OpencodeRouteGoogle          OpencodeRoute = "google"
)

// path returns the request path suffix for the route. The Google route is
// model-scoped, so it takes the model id.
func (r OpencodeRoute) path(model string) string {
	switch r {
	case OpencodeRouteResponses:
		return "/responses"
	case OpencodeRouteMessages:
		return "/messages"
	case OpencodeRouteGoogle:
		return fmt.Sprintf("/models/%s:generateContent", model)
	case OpencodeRouteChatCompletions:
		return "/chat/completions"
	default:
		return "/chat/completions"
	}
}

// valid reports whether r is one of the four known routes.
func (r OpencodeRoute) valid() bool {
	switch r {
	case OpencodeRouteResponses, OpencodeRouteMessages,
		OpencodeRouteChatCompletions, OpencodeRouteGoogle:
		return true
	default:
		return false
	}
}

// opencodeBaseURL returns the default base URL for a gateway.
func opencodeBaseURL(gateway string) (string, error) {
	switch gateway {
	case ProviderOpencodeZen:
		return opencodeZenBaseURL, nil
	case ProviderOpencodeGo:
		return opencodeGoBaseURL, nil
	default:
		return "", fmt.Errorf("unsupported opencode gateway: %s", gateway)
	}
}

// opencodeRouteTable maps gateway -> model id -> wire format, transcribed from the
// published endpoint tables at https://opencode.ai/docs/zen/ and
// https://opencode.ai/docs/go/ (retrieved 2026-08-28).
//
// Routing is per-gateway, not per-model: the minimax-* family takes
// chat_completions on Zen and messages on Go. Reconcile this table against both
// docs pages when either gateway announces new models.
//
//nolint:goconst // model IDs are intentionally repeated across gateways and tests
var opencodeRouteTable = map[string]map[string]OpencodeRoute{
	ProviderOpencodeZen: {
		// responses (@ai-sdk/openai)
		"gpt-5.6-sol": OpencodeRouteResponses, "gpt-5.6-terra": OpencodeRouteResponses,
		"gpt-5.6-luna": OpencodeRouteResponses, "gpt-5.5": OpencodeRouteResponses,
		"gpt-5.5-pro": OpencodeRouteResponses, "gpt-5.4": OpencodeRouteResponses,
		"gpt-5.4-pro": OpencodeRouteResponses, "gpt-5.4-mini": OpencodeRouteResponses,
		"gpt-5.4-nano": OpencodeRouteResponses, "gpt-5.3-codex": OpencodeRouteResponses,
		"gpt-5.3-codex-spark": OpencodeRouteResponses, "gpt-5.2": OpencodeRouteResponses,
		"gpt-5.2-codex": OpencodeRouteResponses, "gpt-5.1": OpencodeRouteResponses,
		"gpt-5.1-codex": OpencodeRouteResponses, "gpt-5.1-codex-max": OpencodeRouteResponses,
		"gpt-5.1-codex-mini": OpencodeRouteResponses, "gpt-5": OpencodeRouteResponses,
		"gpt-5-codex": OpencodeRouteResponses, "gpt-5-nano": OpencodeRouteResponses,
		"grok-4.6": OpencodeRouteResponses, "grok-4.5": OpencodeRouteResponses,
		"grok-build-0.1": OpencodeRouteResponses, "muse-spark-1.2": OpencodeRouteResponses,
		"muse-spark-1.2-contributor-free": OpencodeRouteResponses,

		// messages (@ai-sdk/anthropic)
		"claude-fable-5": OpencodeRouteMessages, "claude-opus-5": OpencodeRouteMessages,
		"claude-opus-4-8": OpencodeRouteMessages, "claude-opus-4-7": OpencodeRouteMessages,
		"claude-opus-4-6": OpencodeRouteMessages, "claude-opus-4-5": OpencodeRouteMessages,
		"claude-sonnet-5": OpencodeRouteMessages, "claude-sonnet-4-6": OpencodeRouteMessages,
		"claude-sonnet-4-5": OpencodeRouteMessages, "claude-haiku-4-5": OpencodeRouteMessages,
		"qwen3.7-max": OpencodeRouteMessages, "qwen3.7-plus": OpencodeRouteMessages,
		"qwen3.6-plus": OpencodeRouteMessages, "qwen3.5-plus": OpencodeRouteMessages,

		// google (@ai-sdk/google)
		"gemini-3.7-flash": OpencodeRouteGoogle, "gemini-3.6-flash": OpencodeRouteGoogle,
		"gemini-3.5-flash": OpencodeRouteGoogle, "gemini-3.5-flash-lite": OpencodeRouteGoogle,
		"gemini-3.1-pro": OpencodeRouteGoogle, "gemini-3-flash": OpencodeRouteGoogle,

		// chat_completions (@ai-sdk/openai-compatible)
		"deepseek-v4-pro":   OpencodeRouteChatCompletions,
		"deepseek-v4-flash": OpencodeRouteChatCompletions,
		"minimax-m3":        OpencodeRouteChatCompletions,
		"minimax-m2.7":      OpencodeRouteChatCompletions,
		"minimax-m2.5":      OpencodeRouteChatCompletions,
		"glm-5.2":           OpencodeRouteChatCompletions,
		"glm-5.1":           OpencodeRouteChatCompletions,
		"glm-5":             OpencodeRouteChatCompletions,
		"kimi-k2.5":         OpencodeRouteChatCompletions,
		"kimi-k2.6":         OpencodeRouteChatCompletions,
		"kimi-k2.7-code":    OpencodeRouteChatCompletions,
		"kimi-k3":           OpencodeRouteChatCompletions,
		"big-pickle":        OpencodeRouteChatCompletions,
		"mimo-v2.5-free":    OpencodeRouteChatCompletions,
		"hy3-free":          OpencodeRouteChatCompletions,

		"ling-3.0-flash-fin-free":     OpencodeRouteChatCompletions,
		"nemotron-3-ultra-free":       OpencodeRouteChatCompletions,
		"nemotron-3.5-lightning-free": OpencodeRouteChatCompletions,
	},
	ProviderOpencodeGo: {
		// responses (@ai-sdk/openai)
		"grok-4.6": OpencodeRouteResponses, "gpt-5.6-luna": OpencodeRouteResponses,
		"muse-spark-1.2-contributor": OpencodeRouteResponses,

		// messages (@ai-sdk/anthropic) — note minimax-* differs from Zen
		"minimax-m3": OpencodeRouteMessages, "minimax-m2.7": OpencodeRouteMessages,
		"minimax-m2.5": OpencodeRouteMessages, "qwen3.8-max": OpencodeRouteMessages,
		"qwen3.8-flash": OpencodeRouteMessages, "qwen3.7-max": OpencodeRouteMessages,
		"qwen3.7-plus": OpencodeRouteMessages, "qwen3.6-plus": OpencodeRouteMessages,

		// chat_completions (@ai-sdk/openai-compatible)
		"glm-5.3-flash":                OpencodeRouteChatCompletions,
		"glm-5.3":                      OpencodeRouteChatCompletions,
		"glm-5.2":                      OpencodeRouteChatCompletions,
		"glm-5.1":                      OpencodeRouteChatCompletions,
		"kimi-k3":                      OpencodeRouteChatCompletions,
		"kimi-k2.7-code":               OpencodeRouteChatCompletions,
		"kimi-k2.6":                    OpencodeRouteChatCompletions,
		"longcat-2.0":                  OpencodeRouteChatCompletions,
		"deepseek-v4-pro":              OpencodeRouteChatCompletions,
		"deepseek-v4-flash":            OpencodeRouteChatCompletions,
		"deepseek-v4-flash-vision-exp": OpencodeRouteChatCompletions,
		"mimo-v2.5":                    OpencodeRouteChatCompletions,
		"mimo-v2.5-pro":                OpencodeRouteChatCompletions,
		"hy4-preview":                  OpencodeRouteChatCompletions,
		"hy3":                          OpencodeRouteChatCompletions,
	},
}

// opencodeHeuristicRoute infers a route from the model id prefix when the table
// has no entry. Both gateways send gpt/grok/muse to responses and qwen to
// messages; they differ on minimax (Go only) and gemini/claude (Zen only).
// Chat Completions is the default because it is the largest bucket on both.
func opencodeHeuristicRoute(gateway, model string) OpencodeRoute {
	m := strings.ToLower(strings.TrimSpace(model))

	switch {
	case strings.HasPrefix(m, "gpt-"), strings.HasPrefix(m, "grok-"),
		strings.HasPrefix(m, "muse-"):
		return OpencodeRouteResponses
	case strings.HasPrefix(m, "qwen"):
		return OpencodeRouteMessages
	}

	if gateway == ProviderOpencodeZen {
		switch {
		case strings.HasPrefix(m, "claude-"):
			return OpencodeRouteMessages
		case strings.HasPrefix(m, "gemini-"):
			return OpencodeRouteGoogle
		}
	}
	if gateway == ProviderOpencodeGo && strings.HasPrefix(m, "minimax-") {
		return OpencodeRouteMessages
	}

	return OpencodeRouteChatCompletions
}

// resolveOpencodeRoute picks the wire format for (gateway, model), honouring an
// explicit override first, then the published table, then the prefix heuristic.
func resolveOpencodeRoute(gateway, model string, override OpencodeRoute) (OpencodeRoute, error) {
	if override != "" {
		if !override.valid() {
			return "", fmt.Errorf("%w: unknown opencode route %q", ErrInvalidRequest, override)
		}
		return override, nil
	}
	if byModel, ok := opencodeRouteTable[gateway]; ok {
		if r, ok := byModel[strings.ToLower(strings.TrimSpace(model))]; ok {
			return r, nil
		}
	}
	return opencodeHeuristicRoute(gateway, model), nil
}
