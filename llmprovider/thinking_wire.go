package llmprovider

import (
	"maps"
	"regexp"
	"strconv"
	"strings"
)

// Thinking request shapes for the budget-based wires, the Anthropic Messages
// API and Gemini's generateContent, shared by ClaudeProvider, GeminiProvider
// and OpenCode's messages and google routes (MADR 0013 Q1, B9). The model id
// selects the shape, as in OpenCode's own client
// (packages/opencode/src/provider/transform.ts).

// lowEffortThinkingBudget is the budget "low" maps to where a model takes only
// a budget. It is Anthropic's minimum: 1023 is refused with HTTP 400
// (measured 2026-09-27).
const lowEffortThinkingBudget = 1024

// Request keys of the two budget-based wires.
const (
	jsonKeyThinking       = "thinking"
	jsonKeyThinkingBudget = "thinkingBudget"
)

// claudeVersionRE reads a Claude version, family-first (claude-opus-4-8) or
// version-first (claude-4.7-opus). A minor has at most two digits, so the date
// in claude-sonnet-4-20250514 is not read as one. OpenCode's
// anthropicUsesModernAdaptiveThinking uses the same pattern.
var claudeVersionRE = regexp.MustCompile(`(?i)claude-(?:[a-z]+-)?(\d+)(?:[.-](\d{1,2}))?(?:[.@-]|$)`)

// geminiLegacyRE matches Gemini 1.x and 2.x ids, which take thinkingBudget but
// not thinkingLevel (HTTP 400 on gemini-2.5-flash, measured 2026-09-27). It is
// OpenCode's GEMINI_LEGACY_RE.
var geminiLegacyRE = regexp.MustCompile(`(?i)gemini-(?:(?:flash|pro)-)?[12](?:[.-]|$)`)

// claudeAdaptiveOnly reports whether model refuses thinking.type "enabled":
// Claude 4.7 and later, and a claude- id with no readable version (HTTP 400 on
// claude-sonnet-5 and claude-opus-4-8, measured 2026-09-27). Other ids,
// including the non-Claude models OpenCode serves on its messages route, take
// a budget.
func claudeAdaptiveOnly(model string) bool {
	lower := strings.ToLower(model)
	if !strings.Contains(lower, "claude-") {
		return false
	}
	m := claudeVersionRE.FindStringSubmatch(lower)
	if m == nil {
		return true
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return true
	}
	minor := 0
	if m[2] != "" {
		if minor, err = strconv.Atoi(m[2]); err != nil {
			return true
		}
	}
	return major > 4 || (major == 4 && minor >= 7)
}

// minimaxAdaptiveThinking reports a MiniMax-M3 id. MiniMax's Anthropic
// interface takes thinking.type "adaptive" with no budget or effort, as
// OpenCode's client sends it (transform.ts:1293-1296, MADR 0012 §3.2).
func minimaxAdaptiveThinking(model string) bool {
	return strings.Contains(strings.ToLower(model), "minimax-m3")
}

// addMessagesThinking adds the thinking fields of an Anthropic Messages
// request to body and returns its max_tokens. Adaptive-only models get
// thinking.type "adaptive", plus output_config.effort when an effort is set; a
// budget does not apply to them. Other models get an enabled budget: the
// configured one, else lowEffortThinkingBudget for "low", else
// defaultClaudeThinkingBudget, with max_tokens raised above it when needed
// (Anthropic requires max_tokens > budget_tokens).
func addMessagesThinking(body map[string]any, model, effort string, budget, maxTokens int) int {
	if minimaxAdaptiveThinking(model) {
		body[jsonKeyThinking] = map[string]any{jsonKeyType: "adaptive"}
		return maxTokens
	}
	if claudeAdaptiveOnly(model) {
		fields := map[string]any{jsonKeyThinking: map[string]any{jsonKeyType: "adaptive"}}
		if effort != "" {
			fields["output_config"] = map[string]any{jsonKeyEffort: effort}
		}
		maps.Copy(body, fields)
		return maxTokens
	}
	if budget <= 0 {
		budget = defaultClaudeThinkingBudget
		if effort == effortLow {
			budget = lowEffortThinkingBudget
		}
	}
	if maxTokens <= budget {
		maxTokens = budget + defaultClaudeThinkingBudget
	}
	body[jsonKeyThinking] = map[string]any{jsonKeyType: jsonKeyEnabled, "budget_tokens": budget}
	return maxTokens
}

// geminiThinkingConfig returns a Gemini thinkingConfig. A configured budget
// wins. Otherwise "low" is thinkingLevel "low" on Gemini 3 and later and a
// lowEffortThinkingBudget budget on 1.x and 2.x, and any other effort keeps
// the dynamic budget.
func geminiThinkingConfig(model, effort string, budget int) map[string]any {
	switch {
	case budget > 0:
		return map[string]any{jsonKeyThinkingBudget: budget}
	case effort != effortLow:
		return map[string]any{jsonKeyThinkingBudget: dynamicGeminiThinkingBudget}
	case geminiLegacyRE.MatchString(model):
		return map[string]any{jsonKeyThinkingBudget: lowEffortThinkingBudget}
	}
	return map[string]any{"thinkingLevel": effortLow}
}
