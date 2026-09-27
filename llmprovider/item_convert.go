package llmprovider

import (
	"encoding/json"
	"strings"
)

// toolArguments decodes a call's JSON arguments into the object Anthropic's
// tool_use.input and Gemini's functionCall.args require. Empty arguments are
// an empty object; arguments that are not a JSON object are kept under
// "arguments" rather than dropped.
func toolArguments(arguments string) map[string]any {
	if strings.TrimSpace(arguments) == "" {
		return map[string]any{}
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err == nil && args != nil {
		return args
	}
	return map[string]any{jsonKeyArguments: arguments}
}

// claudeSystemPrompt joins the system items, in order, for the Messages API's
// top-level system field; claudeItemsToMessages leaves them out (MADR 0012 §2).
func claudeSystemPrompt(items []Item) string {
	var parts []string
	for _, item := range items {
		if m, ok := item.(MessageItem); ok && m.Role == jsonRoleSystem && m.Text != "" {
			parts = append(parts, m.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}
