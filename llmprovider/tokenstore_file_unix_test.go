//go:build unix

package llmprovider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileTokenStore_SaveMode0600(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileTokenStore(dir)
	if err != nil {
		t.Fatalf("NewFileTokenStore: %v", err)
	}
	sess := &OAuthSession{
		Provider: "openai",
		Access:   "at",
		Refresh:  "rt",
		Expiry:   time.Now().Add(time.Hour),
		Issuer:   "https://auth.openai.com",
		ClientID: "app_test",
	}
	if err := fs.Save(context.Background(), "openai", sess); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "openai.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 0600", perm)
	}
}
