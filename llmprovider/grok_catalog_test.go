package llmprovider

import (
	"slices"
	"testing"
)

// TestIsUsableGrokModel_RejectsMedia: xAI lists grok-imagine-video and
// grok-imagine-video-1.5 (2026-09-27), which "image" does not match.
func TestIsUsableGrokModel_RejectsMedia(t *testing.T) {
	for _, id := range []string{"grok-imagine-video", "grok-imagine-video-1.5", "grok-imagine-image",
		"grok-voice-1", "grok-stt-1", "grok-tts-1"} {
		if isUsableGrokModel(id) {
			t.Errorf("isUsableGrokModel(%q) = true, want false", id)
		}
	}
}

// TestIsUsableGrokModel_KeepsTextModels: every text model xAI listed on
// 2026-09-27 stays usable.
func TestIsUsableGrokModel_KeepsTextModels(t *testing.T) {
	for _, id := range []string{"grok-4.20-0309-non-reasoning", "grok-4.20-0309-reasoning", "grok-4.20-multi-agent-0309",
		"grok-4.3", "grok-4.5", "grok-4.6", "grok-4.7", "grok-build-0.1", "grok-3-mini"} {
		if !isUsableGrokModel(id) {
			t.Errorf("isUsableGrokModel(%q) = false, want true", id)
		}
	}
}

// TestStaticGrok_LeadsWithCLIDefaults: the Grok CLI's default model is
// grok-4.6, then grok-4.5 (grok-build f0e3be11:
// xai-grok-models/default_models.json).
func TestStaticGrok_LeadsWithCLIDefaults(t *testing.T) {
	if len(StaticGrok) < 2 || !slices.Equal(StaticGrok[:2], []string{"grok-4.6", "grok-4.5"}) {
		t.Fatalf("StaticGrok = %v, want it to lead with grok-4.6, grok-4.5", StaticGrok)
	}
}
