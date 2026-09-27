//go:build live_gateways

package llmprovider

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// liveVendorSession returns a read-through session on a real CLI login, or
// skips. It never writes the file and never refreshes (MADR 0012 §5.1).
func liveVendorSession(t *testing.T, provider, optIn, homeEnv, dir string) *VendorCLISession {
	t.Helper()
	if os.Getenv(optIn) != "1" {
		t.Skipf("%s unset: this spends the CLI's subscription", optIn)
	}
	home := os.Getenv(homeEnv)
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			t.Skip(err)
		}
		home = filepath.Join(userHome, dir)
	}
	path := filepath.Join(home, "auth.json")
	if provider == ProviderGrok && os.Getenv("GROK_AUTH_PATH") != "" {
		path = os.Getenv("GROK_AUTH_PATH")
	}
	s := &VendorCLISession{Provider: provider, Path: path}
	if _, err := s.Token(t.Context()); errors.Is(err, ErrAuthFailure) {
		t.Skipf("no live %s CLI login: %v", provider, err)
	}
	return s
}

// TestLive_VendorCLISession generates through each CLI's own login.
func TestLive_VendorCLISession(t *testing.T) {
	for _, tc := range []struct{ provider, optIn, homeEnv, dir, model string }{
		{ProviderOpenAI, "MCPLIB_LIVE_CHATGPT", "CODEX_HOME", ".codex", "gpt-6-astra"},
		{ProviderGrok, "MCPLIB_LIVE_GROK_CLI", "GROK_HOME", ".grok", "grok-4.6"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			s := liveVendorSession(t, tc.provider, tc.optIn, tc.homeEnv, tc.dir)
			ctx, cancel := liveCtx(t)
			defer cancel()
			p, err := NewProviderWithSource(tc.provider, s, tc.model)
			if err != nil {
				t.Fatal(err)
			}
			out, err := p.Generate(ctx, "Reply with only the word ALPHA")
			skipIfTransient(t, err)
			if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
				t.Fatalf("Generate = %q, %v", out, err)
			}
		})
	}
}
