package llmprovider

import (
	"regexp"
	"slices"
	"strings"
)

// MaxListedModels is the hard cap for configure-time model menus.
// Keeps wizards short and avoids dumping dozens of unusable API IDs.
const MaxListedModels = 6

// Gemini generateContent method name (Models API supportedGenerationMethods).
const methodGenerateContent = "generateContent"

// Static curated catalogs — preferred production text models for commit messages
// and general generate/chat use. Ordered: fast/cheap first, then quality.
// Updated from provider docs (Gemini 2026-07, OpenAI 2026, Anthropic 2026).
//
// These are the PRIMARY source of truth for menus. Live list APIs are used only
// to confirm availability and drop IDs the key cannot access — not to dump the
// full catalog (Gemini alone exposes embeddings, TTS, Live, image, robotics, …).
//
//nolint:goconst // catalog IDs are intentionally repeated in tests and filters
var (
	// StaticGemini: stable GA text models only. gemini-2.0-* is shut down (2026).
	// Prefer Flash / Flash-Lite for latency-sensitive hooks; Pro last.
	StaticGemini = []string{
		"gemini-2.5-flash-lite",
		"gemini-2.5-flash",
		"gemini-3.1-flash-lite",
		"gemini-3.5-flash",
		"gemini-2.5-pro",
	}

	// StaticOpenAI: chat-capable defaults; mini/nano first for cost.
	StaticOpenAI = []string{
		"gpt-4.1-mini",
		"gpt-4.1-nano",
		"gpt-4o-mini",
		"gpt-4.1",
		"gpt-4o",
		"o4-mini",
	}

	// StaticClaude: current aliases first, then widely available older IDs.
	StaticClaude = []string{
		"claude-haiku-4-5",
		"claude-sonnet-5",
		"claude-sonnet-4-6",
		"claude-opus-4-8",
		"claude-3-5-haiku-latest",
		"claude-sonnet-4-20250514",
	}
)

// geminiDenySubstrings reject non-text / non-production / specialized endpoints.
// Matched as lowercase substrings of the model id (after models/ prefix strip).
const denyComputerUse = "computer-use"

var geminiDenySubstrings = []string{
	"image", "vision", "embed", "tts", "audio", "live", "preview",
	"exp", "experimental", "gemma", "learnlm", "aqa", "robotics",
	denyComputerUse, "deep-research", "antigravity", "veo", "lyria",
	"omni", "imagen", "native-audio", "thinking", "banana",
}

// openaiDenyPrefixes exclude non-chat model families from /v1/models.
var openaiDenyPrefixes = []string{
	"dall-e-", "whisper-", "tts-", "davinci-", "babbage-", "chatgpt-image",
	"text-embedding", "embedding", "moderation", "omni-moderation",
	"sora-", "gpt-image", "computer-use",
}

// openaiAllowPrefixes: chat / reasoning completions.
var openaiAllowPrefixes = []string{
	"gpt-4", "gpt-5", "gpt-3.5-turbo", "o1", "o3", "o4",
	"chatgpt-4o",
}

// datedOrSnapshotGemini matches dated previews and numeric snapshots we should
// not surface when a stable short alias exists (e.g. gemini-2.5-flash-001).
var datedOrSnapshotGemini = regexp.MustCompile(`(?i)(-\d{2}-\d{4}|-\d{4}-\d{2}-\d{2}|-preview-|-\d{3}$|-exp)`)

// StaticModels returns a copy of the curated catalog for provider.
func StaticModels(provider string) []string {
	switch strings.ToLower(provider) {
	case ProviderGemini:
		return append([]string(nil), StaticGemini...)
	case ProviderOpenAI:
		return append([]string(nil), StaticOpenAI...)
	case ProviderClaude:
		return append([]string(nil), StaticClaude...)
	default:
		return nil
	}
}

// isUsableGeminiTextModel reports whether id is a production text generateContent model.
func isUsableGeminiTextModel(id string, methods []string) bool {
	id = strings.TrimPrefix(id, "models/")
	sm := strings.ToLower(strings.TrimSpace(id))
	if sm == "" || !strings.HasPrefix(sm, "gemini") {
		return false
	}
	canGenerate := slices.Contains(methods, methodGenerateContent)
	if !canGenerate {
		return false
	}
	for _, deny := range geminiDenySubstrings {
		if strings.Contains(sm, deny) {
			return false
		}
	}
	// Reject dated/snapshot/experimental naming for menu stability.
	if datedOrSnapshotGemini.MatchString(sm) {
		return false
	}
	// Require flash, pro, or lite family (skip odd one-offs).
	if !strings.Contains(sm, "flash") && !strings.Contains(sm, "pro") && !strings.Contains(sm, "lite") {
		return false
	}
	return true
}

// isUsableOpenAIChatModel filters OpenAI /v1/models IDs to chat/reasoning models.
func isUsableOpenAIChatModel(id string) bool {
	sm := strings.ToLower(strings.TrimSpace(id))
	if sm == "" {
		return false
	}
	for _, p := range openaiDenyPrefixes {
		if strings.HasPrefix(sm, p) || strings.Contains(sm, p) {
			return false
		}
	}
	// Prefer undated aliases for menu; still allow dated gpt-4.1-2025-* if needed later.
	// Drop instruction-only and realtime.
	if strings.Contains(sm, "instruct") || strings.Contains(sm, "realtime") || strings.Contains(sm, "audio") {
		return false
	}
	for _, p := range openaiAllowPrefixes {
		if strings.HasPrefix(sm, p) {
			return true
		}
	}
	return false
}

// isUsableClaudeTextModel filters Anthropic model IDs for Messages API text use.
func isUsableClaudeTextModel(id string) bool {
	sm := strings.ToLower(strings.TrimSpace(id))
	if sm == "" || !strings.HasPrefix(sm, "claude") {
		return false
	}
	// Skip non-Messages specialties if they appear.
	for _, deny := range []string{denyComputerUse, "bedrock", "instant-1"} {
		if strings.Contains(sm, deny) {
			return false
		}
	}
	return true
}

// curateFromCatalog returns catalog models that appear in available (case-sensitive
// match first, then case-insensitive). Preserves catalog order. Caps at MaxListedModels.
// If nothing from the catalog is available, falls back to filtering `available` with
// rankFn (optional) and returns up to MaxListedModels.
func curateFromCatalog(catalog, available []string, usable func(string) bool, rankFn func(string) int) []string {
	avail := make(map[string]string, len(available)) // lower -> canonical
	for _, a := range available {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if usable != nil && !usable(a) {
			continue
		}
		avail[strings.ToLower(a)] = a
	}

	out := make([]string, 0, MaxListedModels)
	seen := make(map[string]struct{}, MaxListedModels)

	for _, c := range catalog {
		key := strings.ToLower(c)
		if canon, ok := avail[key]; ok {
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, canon)
			if len(out) >= MaxListedModels {
				return out
			}
		}
	}

	if len(out) > 0 {
		return out
	}

	// Catalog miss (old key / regional): return best filtered available.
	var fallback []string
	for _, a := range available {
		if usable != nil && !usable(a) {
			continue
		}
		fallback = append(fallback, a)
	}
	if rankFn != nil {
		sortByRankDesc(fallback, rankFn)
	}
	if len(fallback) > MaxListedModels {
		fallback = fallback[:MaxListedModels]
	}
	return fallback
}

// sortByRankDesc sorts ids by rankFn descending (stable enough via simple insertion).
func sortByRankDesc(ids []string, rankFn func(string) int) {
	// Small N (≤ dozens after filter); use simple sort.
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			if rankFn(ids[j]) > rankFn(ids[i]) {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
}

// RankGeminiModel scores a Gemini model name for sorting by preference.
// Higher is better. Prefers stable Flash/Lite for hook latency.
func RankGeminiModel(m string) int {
	score := 0
	sm := strings.ToLower(m)

	if strings.Contains(sm, "flash-lite") || strings.HasSuffix(sm, "lite") {
		score += 200
	} else if strings.Contains(sm, "flash") {
		score += 150
	} else if strings.Contains(sm, "pro") {
		score += 80
	}

	if strings.Contains(sm, "preview") || strings.Contains(sm, "exp") {
		score -= 1000
	}
	if datedOrSnapshotGemini.MatchString(sm) {
		score -= 200
	}

	// Version weights (newer generations first).
	switch {
	case strings.Contains(sm, "3.5"):
		score += 50
	case strings.Contains(sm, "3.1"):
		score += 45
	case strings.Contains(sm, "3.0") || strings.Contains(sm, "gemini-3-"):
		score += 40
	case strings.Contains(sm, "2.5"):
		score += 30
	case strings.Contains(sm, "2.0"):
		score += 5 // largely shut down
	case strings.Contains(sm, "1.5"):
		score += 0
	}

	return score
}

// RankOpenAIModel prefers mini/nano for cost, then flagship chat.
func RankOpenAIModel(m string) int {
	sm := strings.ToLower(m)
	score := 0
	switch {
	case strings.Contains(sm, "nano"):
		score += 200
	case strings.Contains(sm, "mini"):
		score += 180
	case strings.HasPrefix(sm, "o4"):
		score += 120
	case strings.HasPrefix(sm, "o3"):
		score += 100
	case strings.Contains(sm, "4.1"):
		score += 160
	case strings.Contains(sm, "4o"):
		score += 140
	case strings.Contains(sm, "gpt-5"):
		score += 150
	}
	if strings.Contains(sm, "realtime") || strings.Contains(sm, "audio") {
		score -= 500
	}
	return score
}

// RankClaudeModel prefers Haiku/Sonnet for speed, then newer gens.
func RankClaudeModel(m string) int {
	sm := strings.ToLower(m)
	score := 0
	if strings.Contains(sm, "haiku") {
		score += 200
	} else if strings.Contains(sm, "sonnet") {
		score += 150
	} else if strings.Contains(sm, "opus") {
		score += 100
	} else if strings.Contains(sm, "fable") {
		score += 90
	}
	// Generation hints.
	if strings.Contains(sm, "4-5") || strings.Contains(sm, "4.5") || strings.Contains(sm, "sonnet-5") || strings.Contains(sm, "haiku-4") {
		score += 40
	}
	if strings.Contains(sm, "opus-4-8") || strings.Contains(sm, "sonnet-4-6") {
		score += 35
	}
	if strings.Contains(sm, "3-5") || strings.Contains(sm, "3.5") {
		score += 10
	}
	return score
}
