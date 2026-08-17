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
	// StaticGemini: stable fast text models only. gemini-2.0-* and 1.5-* are shut down.
	// Prioritizes low-latency Flash and Flash-Lite models for fast Git hook execution.
	StaticGemini = []string{
		"gemini-3.7-flash",
		"gemini-3.6-flash",
		"gemini-3.5-flash",
		"gemini-3.5-flash-lite",
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
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

	// StaticGrok: fast/flagship models first.
	StaticGrok = []string{
		"grok-3-mini-fast",
		"grok-3-mini",
		"grok-4",
		"grok-4.5",
		"grok-4.6",
		"grok-4-fast-reasoning",
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
	case ProviderGrok:
		return append([]string(nil), StaticGrok...)
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
// match first, then case-insensitive). Preserves catalog order. If catalog hits do not
// fill MaxListedModels, dynamically backfills with top-ranked usable models from available.
// If nothing from the catalog is available, falls back to filtering `available` with rankFn.
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

	// Backfill with remaining usable models from available if catalog hits < MaxListedModels.
	if len(available) > 0 {
		var remaining []string
		for _, a := range available {
			key := strings.ToLower(strings.TrimSpace(a))
			if usable != nil && !usable(a) {
				continue
			}
			if _, dup := seen[key]; !dup {
				remaining = append(remaining, a)
			}
		}
		if rankFn != nil {
			sortByRankDesc(remaining, rankFn)
		}
		for _, r := range remaining {
			if len(out) >= MaxListedModels {
				break
			}
			key := strings.ToLower(r)
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				out = append(out, r)
			}
		}
	}

	return out
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
// Higher is better. Prioritizes low-latency Flash and Flash-Lite models for fast Git hook execution,
// while penalizing heavy reasoning (Pro) models and deprecated generations.
func RankGeminiModel(m string) int {
	score := 0
	sm := strings.ToLower(m)

	if strings.Contains(sm, "flash-lite") || strings.HasSuffix(sm, "lite") {
		score += 200
	} else if strings.Contains(sm, "flash") {
		score += 180
	} else if strings.Contains(sm, "pro") {
		score -= 500 // Penalize heavy reasoning models for Git hook latency
	}

	if strings.Contains(sm, "preview") || strings.Contains(sm, "exp") || strings.Contains(sm, "deep-research") {
		score -= 1000
	}
	if datedOrSnapshotGemini.MatchString(sm) {
		score -= 200
	}

	// Version weights (newer generations first).
	switch {
	case strings.Contains(sm, "3.7"):
		score += 100
	case strings.Contains(sm, "3.6"):
		score += 90
	case strings.Contains(sm, "3.5"):
		score += 80
	case strings.Contains(sm, "3.1") || strings.Contains(sm, "3.0") || strings.Contains(sm, "gemini-3-"):
		score += 40
	case strings.Contains(sm, "2.5"):
		score += 30
	case strings.Contains(sm, "2.0") || strings.Contains(sm, "1.5"):
		score -= 2000 // Deprecated / shut down
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

// isUsableGrokModel filters xAI model IDs for Responses API text use.
func isUsableGrokModel(id string) bool {
	sm := strings.ToLower(strings.TrimSpace(id))
	if sm == "" || !strings.HasPrefix(sm, "grok") {
		return false
	}
	// Skip non-text specialties if they appear.
	for _, deny := range []string{"vision", "image", "embed"} {
		if strings.Contains(sm, deny) {
			return false
		}
	}
	return true
}

// RankGrokModel prefers mini-fast for cost/speed, then by generation.
func RankGrokModel(m string) int {
	sm := strings.ToLower(m)
	score := 0
	switch {
	case strings.Contains(sm, "mini-fast"):
		score += 200
	case strings.Contains(sm, "mini"):
		score += 180
	case strings.Contains(sm, "fast-reasoning"):
		score += 100
	}
	// Generation weights.
	switch {
	case strings.Contains(sm, "4.6"):
		score += 60
	case strings.Contains(sm, "4.5"):
		score += 50
	case strings.HasPrefix(sm, "grok-4"):
		score += 40
	case strings.HasPrefix(sm, "grok-3"):
		score += 30
	}
	return score
}
