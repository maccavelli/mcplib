//go:build live_gateways

package llmprovider

import (
	"errors"
	"testing"
)

// TestLive_StaticClaudeServed pins MADR 0013 B10: every StaticClaude id
// answers on the Messages API. The static catalog is what the wizard offers
// when the listing fails, so a retired id there is a dead end. It REQUIRES
// ANTHROPIC_API_KEY and skips without it.
func TestLive_StaticClaudeServed(t *testing.T) {
	key := liveEnvKey(t, "ANTHROPIC_API_KEY")
	for _, model := range StaticClaude {
		t.Run(model, func(t *testing.T) {
			ctx, cancel := liveCtx(t)
			defer cancel()
			p, err := NewClaude(key, model, WithMaxTokens(16))
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Generate(ctx, "Reply with only the word ALPHA")
			if errors.Is(err, ErrRateLimited) {
				t.Skipf("rate limited: %v", err)
			}
			if err != nil {
				t.Errorf("%s: %v", model, err)
			}
		})
	}
}
