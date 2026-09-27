package llmprovider

import "testing"

// TestSearchModels_GlobQuestionMark covers model_matcher.go's '?' glob rune:
// it matches exactly one character.
func TestSearchModels_GlobQuestionMark(t *testing.T) {
	got := SearchModels(ProviderClaude, []string{"claude-sonnet-5", "claude-sonnet-45", "claude-opus-5"}, "claude-sonnet-?")
	if len(got) != 1 || got[0].ID != "claude-sonnet-5" {
		t.Errorf("claude-sonnet-? = %v, want only claude-sonnet-5", got)
	}
}

// TestSearchModels_LabelSubstringTier covers model_matcher.go's label tier: a
// query found only in a curated label scores scoreLabelSubstring.
func TestSearchModels_LabelSubstringTier(t *testing.T) {
	got := SearchModels(ProviderClaude, []string{"claude-sonnet-5", "claude-opus-4-8"}, "balanced speed")
	if len(got) != 1 || got[0].ID != "claude-sonnet-5" || got[0].Score != scoreLabelSubstring {
		t.Errorf("balanced speed = %+v, want claude-sonnet-5 at scoreLabelSubstring", got)
	}
}
