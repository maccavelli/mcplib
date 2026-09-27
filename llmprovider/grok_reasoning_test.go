package llmprovider

import "testing"

// TestGrokClampReasoningEffort pins models without a CLI menu: they send no
// reasoning_effort (MADR 0012 §6). The menus are pinned by
// grok_effort_menu_test.go.
func TestGrokClampReasoningEffort(t *testing.T) {
	checkGrokEfforts(t, []grokEffortCase{
		{"grok-3", "high", ""},
		{"grok-4", "medium", ""},
		{"grok-4-fast-reasoning", "low", ""},
		{"grok-code-fast-1", "high", ""},
		{"unknown-model", "high", ""},
	})
}
