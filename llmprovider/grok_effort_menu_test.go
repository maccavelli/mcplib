package llmprovider

import "testing"

type grokEffortCase struct{ model, effort, want string }

func checkGrokEfforts(t *testing.T, cases []grokEffortCase) {
	t.Helper()
	for _, tc := range cases {
		if got := grokClampReasoningEffort(tc.model, tc.effort); got != tc.want {
			t.Errorf("grokClampReasoningEffort(%q, %q) = %q, want %q", tc.model, tc.effort, got, tc.want)
		}
	}
}

// TestGrokEffortMenus_ClampToCLIMenu: the Grok CLI offers grok-4.5 only
// high/medium/low and grok-4.6-build and grok-3-mini only low/high
// (grok-build f0e3be11: xai-grok-models/default_models.json;
// xai-grok-shell/src/agent/config_tests.rs:7958).
func TestGrokEffortMenus_ClampToCLIMenu(t *testing.T) {
	checkGrokEfforts(t, []grokEffortCase{
		{"grok-4.5", "xhigh", "high"},
		{"grok-4.6-build", "xhigh", "high"},
		{"grok-4.6-build", "medium", "low"},
		{"grok-3-mini", "medium", "low"},
		{"grok-3-mini-fast", "medium", "low"},
	})
}

// TestGrokEffortMenus_KeepMenuValues: an effort on the menu is sent as is,
// no effort sends high, and a model without a menu sends none.
func TestGrokEffortMenus_KeepMenuValues(t *testing.T) {
	checkGrokEfforts(t, []grokEffortCase{
		{"grok-4.6", "xhigh", "xhigh"},
		{"grok-4.6", "medium", "medium"},
		{"grok-4.5", "low", "low"},
		{"grok-4.5", "", "high"},
		{"grok-4.6", "", "high"},
		{"grok-3-mini", "high", "high"},
		{"grok-4", "high", ""},
		{"grok-4.7", "high", ""},
	})
}
