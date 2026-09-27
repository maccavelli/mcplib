package wizard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maccavelli/mcplib/llmprovider"
)

// TestVendorAuthPath pins each CLI's own resolution (MADR 0012 §5.1).
func TestVendorAuthPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	for _, tc := range []struct {
		name, provider string
		env            map[string]string
		want           string
	}{
		{"codex home", llmprovider.ProviderOpenAI, map[string]string{"CODEX_HOME": "/x/codex"}, "/x/codex/auth.json"},
		{"codex default", llmprovider.ProviderOpenAI, nil, filepath.Join(home, ".codex", "auth.json")},
		{"grok auth path", llmprovider.ProviderGrok,
			map[string]string{"GROK_AUTH_PATH": "/y/login.json", "GROK_HOME": "/x/grok"}, "/y/login.json"},
		{"grok home", llmprovider.ProviderGrok, map[string]string{"GROK_HOME": "/x/grok"}, "/x/grok/auth.json"},
		{"grok default", llmprovider.ProviderGrok, nil, filepath.Join(home, ".grok", "auth.json")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := vendorAuthPath(tc.provider, Options{LookupEnv: func(name string) string { return tc.env[name] }})
			if err != nil || got != tc.want {
				t.Fatalf("vendorAuthPath = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
