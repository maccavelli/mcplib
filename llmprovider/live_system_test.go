//go:build live_gateways

package llmprovider

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive_SystemMessage: a system instruction reaches the model on Anthropic
// and on OpenCode's messages route, and the model follows it.
func TestLive_SystemMessage(t *testing.T) {
	items := []Item{
		MessageItem{Role: "system", Text: "You only ever reply in French."},
		MessageItem{Role: jsonRoleUser, Text: "Say hello in one word, nothing else."},
	}
	for _, c := range []struct {
		name, env string
		build     func(key string) (ItemProvider, error)
	}{
		{"claude", "ANTHROPIC_API_KEY", func(k string) (ItemProvider, error) { return NewClaude(k, "claude-haiku-4-5") }},
		{"go-messages", "OPENCODE_API_KEY", func(k string) (ItemProvider, error) { return NewOpencode(ProviderOpencodeGo, k, "qwen3.8-flash") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			key := os.Getenv(c.env)
			if key == "" {
				t.Skipf("%s unset", c.env)
			}
			p, err := c.build(key)
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			res, err := p.GenerateItems(ctx, items...)
			skipIfTransient(t, err)
			if err != nil {
				t.Fatalf("GenerateItems: %v", err)
			}
			if text := strings.ToLower(res.OutputText()); !strings.Contains(text, "bonjour") && !strings.Contains(text, "salut") &&
				!strings.Contains(text, "coucou") {
				t.Fatalf("reply %q does not follow the system instruction", res.OutputText())
			}
		})
	}
}
