package llmprovider

import "testing"

func TestGrokReasoningSupport(t *testing.T) {
	tests := []struct {
		model string
		want  reasoningSupport
	}{
		{"grok-3-mini", reasoningLowHigh},
		{"grok-3-mini-fast", reasoningLowHigh},
		{"grok-4.5", reasoningFull},
		{"grok-4.6", reasoningFull},
		{"grok-3", reasoningUnsupported},
		{"grok-4", reasoningUnsupported},
		{"grok-4-fast-reasoning", reasoningUnsupported},
		{"grok-code-fast-1", reasoningUnsupported},
		{"unknown-model", reasoningUnsupported},
	}
	for _, tc := range tests {
		if got := grokReasoningSupport(tc.model); got != tc.want {
			t.Errorf("grokReasoningSupport(%q) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

func TestGrokClampReasoningEffort(t *testing.T) {
	tests := []struct {
		model, effort, want string
	}{
		// unsupported: must not send
		{"grok-3", "high", ""},
		{"grok-4", "medium", ""},
		{"grok-4-fast-reasoning", "low", ""},
		// low/high only (grok-3-mini)
		{"grok-3-mini", "medium", "high"},
		{"grok-3-mini", "low", "low"},
		{"grok-3-mini", "high", "high"},
		{"grok-3-mini-fast", "xhigh", "high"},
		// full range (grok-4.5, grok-4.6)
		{"grok-4.5", "xhigh", "xhigh"},
		{"grok-4.5", "", "high"},
		{"grok-4.6", "medium", "medium"},
		{"grok-4.6", "low", "low"},
	}
	for _, tc := range tests {
		if got := grokClampReasoningEffort(tc.model, tc.effort); got != tc.want {
			t.Errorf("grokClampReasoningEffort(%q, %q) = %q, want %q", tc.model, tc.effort, got, tc.want)
		}
	}
}
