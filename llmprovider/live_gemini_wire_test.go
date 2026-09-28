//go:build live_gateways

package llmprovider

import (
	"os"
	"strings"
	"testing"
)

// TestLive_GeminiThoughtSummary: a Gemini thinking call returns its thought
// summary as a ReasoningItem (MADR 0014).
func TestLive_GeminiThoughtSummary(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY unset")
	}
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewGemini(ctx, key, "gemini-3.7-flash")
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: "What is 17 * 23? Reply with only the number."})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateItemsThinking: %v", err)
	}
	var reasoning string
	for _, it := range res.Output {
		if r, ok := it.(ReasoningItem); ok {
			reasoning += r.Text
		}
	}
	if strings.TrimSpace(reasoning) == "" || !strings.Contains(res.OutputText(), "391") {
		t.Fatalf("reasoning %q, answer %q; want a thought summary and 391", reasoning, res.OutputText())
	}
}
