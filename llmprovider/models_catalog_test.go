package llmprovider

import (
	"strings"
	"testing"
)

func TestIsUsableGeminiTextModel(t *testing.T) {
	methods := []string{methodGenerateContent}
	cases := []struct {
		id   string
		want bool
	}{
		{"gemini-3.7-flash", true},
		{"gemini-3.6-flash", true},
		{"gemini-3.5-flash", true},
		{"gemini-3.5-flash-lite", true},
		{"gemini-2.5-flash", true},
		{"gemini-2.5-flash-lite", true},
		{"gemini-2.5-pro", true},
		{"models/gemini-3.7-flash", true},
		// Specialty / non-production
		{"gemini-2.5-flash-image", false},
		{"gemini-2.5-flash-preview-tts", false},
		{"gemini-2.5-flash-native-audio-preview-12-2025", false},
		{"gemini-embedding-001", false},
		{"gemini-2.5-computer-use-preview-10-2025", false},
		{"gemini-2.5-flash-preview-09-2025", false},
		{"gemini-2.5-flash-001", false},
		{"gemma-3-27b", false},
		{"imagen-4", false},
		{"", false},
	}
	for _, tc := range cases {
		got := isUsableGeminiTextModel(tc.id, methods)
		if got != tc.want {
			t.Errorf("isUsableGeminiTextModel(%q)=%v want %v", tc.id, got, tc.want)
		}
	}
	if isUsableGeminiTextModel("gemini-3.7-flash", []string{"embedContent"}) {
		t.Error("must require generateContent")
	}
}

func TestIsUsableOpenAIChatModel(t *testing.T) {
	if !isUsableOpenAIChatModel("gpt-4.1-mini") {
		t.Error("gpt-4.1-mini should be allowed")
	}
	if !isUsableOpenAIChatModel("gpt-4o") {
		t.Error("gpt-4o should be allowed")
	}
	if isUsableOpenAIChatModel("dall-e-3") {
		t.Error("dall-e should be denied")
	}
	if isUsableOpenAIChatModel("text-embedding-3-small") {
		t.Error("embeddings denied")
	}
	if isUsableOpenAIChatModel("whisper-1") {
		t.Error("whisper denied")
	}
}

func TestCurateFromCatalog_Gemini(t *testing.T) {
	// API returns noise + good models
	available := []string{
		"gemini-2.5-flash-image",
		"gemini-3.7-flash",
		"gemini-3.6-flash",
		"gemini-2.5-flash-preview-09-2025",
		"gemini-3.5-flash",
		"gemini-embedding-001",
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-2.0-flash", // shut down-ish; still text if not denied by filter
	}
	// Filter first as listGemini does
	var filtered []string
	for _, a := range available {
		if isUsableGeminiTextModel(a, []string{methodGenerateContent}) {
			filtered = append(filtered, a)
		}
	}
	out := curateFromCatalog(StaticGemini, filtered, func(s string) bool {
		return isUsableGeminiTextModel(s, []string{methodGenerateContent})
	}, RankGeminiModel)

	if len(out) == 0 {
		t.Fatal("expected curated models")
	}
	if len(out) > MaxListedModels {
		t.Fatalf("cap exceeded: %d", len(out))
	}
	for _, m := range out {
		if strings.Contains(m, "image") || strings.Contains(m, "preview") || strings.Contains(m, "embed") {
			t.Errorf("junk leaked into menu: %s", m)
		}
	}
	// Catalog order preference: 3.7-flash before 2.5-flash when both present
	if idx37, idx25 := indexOf(out, "gemini-3.7-flash"), indexOf(out, "gemini-2.5-flash"); idx37 >= 0 && idx25 >= 0 && idx37 > idx25 {
		t.Errorf("expected 3.7-flash before 2.5-flash in catalog order: %v", out)
	}
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func TestCurateFromCatalog_EmptyAvailableUsesNothingThenFallback(t *testing.T) {
	out := curateFromCatalog(StaticGemini, nil, nil, RankGeminiModel)
	if len(out) != 0 {
		// no available, no fallback path without available items
		t.Fatalf("expected empty, got %v", out)
	}
	// With only unusable available, usable filter empties map → empty
	out = curateFromCatalog(StaticOpenAI, []string{"dall-e-3"}, isUsableOpenAIChatModel, RankOpenAIModel)
	if len(out) != 0 {
		t.Fatalf("expected empty when only junk available: %v", out)
	}
}

func TestStaticModels_NoShutDownGemini20(t *testing.T) {
	if len(StaticGemini) > MaxListedModels {
		t.Errorf("StaticGemini should not exceed MaxListedModels (%d), got %d", MaxListedModels, len(StaticGemini))
	}
	for _, m := range StaticGemini {
		if strings.Contains(m, "2.0") {
			t.Errorf("static catalog should not recommend shut-down 2.0 models: %s", m)
		}
		if strings.Contains(m, "1.5") {
			t.Errorf("static catalog should not recommend shut-down 1.5 models: %s", m)
		}
		if strings.Contains(m, "preview") {
			t.Errorf("static catalog must not include preview: %s", m)
		}
		if strings.Contains(m, "pro") {
			t.Errorf("static catalog must not include slow reasoning pro models: %s", m)
		}
	}
}

func TestRankGeminiModel_PrefersFlashLite(t *testing.T) {
	if RankGeminiModel("gemini-3.7-flash") <= RankGeminiModel("gemini-2.5-pro") {
		t.Error("3.7 flash should rank above pro for hook latency")
	}
	if RankGeminiModel("gemini-3.7-flash") <= RankGeminiModel("gemini-2.5-flash") {
		t.Error("newer 3.7 generation should rank above 2.5")
	}
	if RankGeminiModel("gemini-2.5-flash-preview-09-2025") >= RankGeminiModel("gemini-2.5-flash") {
		t.Error("preview should rank far below stable flash")
	}
}
