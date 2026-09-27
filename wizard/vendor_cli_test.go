package wizard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/mcplib/llmprovider"
)

const grokCLILogin = `{
  "https://auth.x.ai::b1a00492-073a-47ea-816f-4c329264a828": {
    "key": "sess-grok-cli",
    "auth_mode": "oidc",
    "refresh_token": "rt-grok-cli",
    "expires_at": "2099-01-01T00:00:00Z",
    "oidc_issuer": "https://auth.x.ai",
    "oidc_client_id": "b1a00492-073a-47ea-816f-4c329264a828"
  }
}`

func writeGrokCLILogin(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, []byte(grokCLILogin), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestConfigureLLM_VendorLoginReadsThrough: using the Grok CLI's login copies
// no token and saves nothing; the CLI keeps its refresh token to itself
// (MADR 0012 §5.1).
func TestConfigureLLM_VendorLoginReadsThrough(t *testing.T) {
	dir := t.TempDir()
	writeGrokCLILogin(t, dir)
	store := newMemoryTokenStore()
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), 4, 0}, confirms: []bool{true}}
	res, err := ConfigureLLM(context.Background(), f, Options{
		TokenStore: store,
		LookupEnv: func(name string) string {
			if name == "GROK_HOME" {
				return dir
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if string(res.Kind) != "vendor_cli" || res.AccessToken != "" || res.RefreshToken != "" || store.saves != 0 {
		t.Fatalf("Kind/access/refresh/saves = %q/%q/%q/%d, want vendor_cli, no tokens, no save",
			res.Kind, res.AccessToken, res.RefreshToken, store.saves)
	}
	assertTextMasksSecret(t, f.allText, "sess-grok-cli")
}

// TestConfigureLLM_VendorLoginHonoursGrokAuthPath: a non-empty
// GROK_AUTH_PATH wins over GROK_HOME, as the Grok CLI resolves it
// (xai-grok-login/src/storage.rs:45-55).
func TestConfigureLLM_VendorLoginHonoursGrokAuthPath(t *testing.T) {
	path := writeGrokCLILogin(t, t.TempDir())
	emptyHome := t.TempDir()
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), 4, 0}, confirms: []bool{true}}
	_, err := ConfigureLLM(context.Background(), f, Options{
		TokenStore: newMemoryTokenStore(),
		LookupEnv: func(name string) string {
			switch name {
			case "GROK_AUTH_PATH":
				return path
			case "GROK_HOME":
				return emptyHome
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if len(f.seenConfirm) == 0 || !strings.Contains(f.seenConfirm[0], path) {
		t.Fatalf("confirm prompts = %q, want one naming %s", f.seenConfirm, path)
	}
}
