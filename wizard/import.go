package wizard

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/maccavelli/mcplib/llmprovider"
)

// vendorAuthPath resolves a vendor CLI's auth file as the CLI does (MADR 0012
// §5.1). Codex: $CODEX_HOME/auth.json, else ~/.codex/auth.json. Grok: a
// non-empty $GROK_AUTH_PATH verbatim, else $GROK_HOME/auth.json, else
// ~/.grok/auth.json (grok-build xai-grok-login/src/storage.rs:45-55,
// xai-dirs/src/lib.rs:43-58).
func vendorAuthPath(provider string, o Options) (string, error) {
	env := o.lookupEnv()
	var homeEnv, defaultDir string
	switch provider {
	case llmprovider.ProviderOpenAI:
		homeEnv, defaultDir = "CODEX_HOME", ".codex"
	case llmprovider.ProviderGrok:
		if path := env("GROK_AUTH_PATH"); path != "" {
			return path, nil
		}
		homeEnv, defaultDir = "GROK_HOME", ".grok"
	default:
		return "", fmt.Errorf("wizard: provider %q has no vendor CLI login", provider)
	}
	if dir := env(homeEnv); dir != "" {
		return filepath.Join(dir, "auth.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("wizard: resolve home directory: %w", err)
	}
	return filepath.Join(home, defaultDir, "auth.json"), nil
}
