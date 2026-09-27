//go:build live_gateways

// Live checks of the thinking shapes on the budget-based wires (MADR 0013 Q1,
// B9). They REQUIRE ANTHROPIC_API_KEY and GEMINI_API_KEY (and OPENCODE_API_KEY
// for the Go route) and skip without them. Only rate limiting skips: a 400
// here is the regression these tests exist to catch.
package llmprovider

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// liveEnvKey returns a credential from the environment or skips.
func liveEnvKey(t *testing.T, name string) string {
	t.Helper()
	key := os.Getenv(name)
	if key == "" {
		t.Skipf("%s unset", name)
	}
	return key
}

// assertAlpha fails unless a thinking call returned text containing ALPHA.
func assertAlpha(t *testing.T, out string, err error) {
	t.Helper()
	if errors.Is(err, ErrRateLimited) {
		t.Skipf("rate limited: %v", err)
	}
	if err != nil {
		t.Fatalf("GenerateThinking: %v", err)
	}
	if !strings.Contains(strings.ToUpper(out), "ALPHA") {
		t.Errorf("output %q does not contain ALPHA", out)
	}
}

// TestLive_ClaudeThinkingShapes: a budget-only model (Haiku 4.5) and two
// adaptive-only models accept the utility and capable efforts.
func TestLive_ClaudeThinkingShapes(t *testing.T) {
	key := liveEnvKey(t, "ANTHROPIC_API_KEY")
	for _, model := range []string{"claude-haiku-4-5", "claude-sonnet-5", "claude-opus-4-8"} {
		for _, effort := range []string{effortLow, ""} {
			t.Run(model+"/"+effort, func(t *testing.T) {
				ctx, cancel := liveCtx(t)
				defer cancel()
				p, err := NewClaude(key, model, WithReasoningEffort(effort), WithMaxTokens(2048))
				if err != nil {
					t.Fatal(err)
				}
				out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
				assertAlpha(t, out, err)
			})
		}
	}
}

// TestLive_GeminiThinkingShapes: Gemini 2.5 (budget) and 3.x (thinkingLevel)
// accept the utility and capable efforts.
func TestLive_GeminiThinkingShapes(t *testing.T) {
	key := liveEnvKey(t, "GEMINI_API_KEY")
	for _, model := range []string{"gemini-2.5-flash", "gemini-3.7-flash"} {
		for _, effort := range []string{effortLow, ""} {
			t.Run(model+"/"+effort, func(t *testing.T) {
				ctx, cancel := liveCtx(t)
				defer cancel()
				p, err := NewGemini(ctx, key, model, WithReasoningEffort(effort), WithMaxTokens(2048))
				if err != nil {
					t.Fatal(err)
				}
				out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
				if errors.Is(err, ErrProviderUnavailable) {
					t.Skipf("Gemini overloaded: %v", err)
				}
				assertAlpha(t, out, err)
			})
		}
	}
}

// TestLive_OpencodeMessagesThinking: OpenCode Go's messages route accepts the
// low-effort budget on qwen3.8-flash (one of Go's utility six).
func TestLive_OpencodeMessagesThinking(t *testing.T) {
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t), "qwen3.8-flash", WithReasoningEffort(effortLow))
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
	assertAlpha(t, out, err)
}
