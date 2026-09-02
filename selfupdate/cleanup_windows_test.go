//go:build windows

package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsCleanupReceiptRoundTrip(t *testing.T) {
	_, exe := withTempHome(t)
	target, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(target.Dir, "."+target.Base+".selfupdate-bak-test")
	if err := os.WriteFile(backup, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeCleanupReceipt(target, applyResult{backup: backup, oldDigest: digest}); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(target.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := processCleanupReceipt(target, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup remained: %v", err)
	}
}

func TestWindowsCleanupReceiptDigestMismatch(t *testing.T) {
	_, exe := withTempHome(t)
	target, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(target.Dir, "bak")
	if err := os.WriteFile(backup, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := cleanupReceipt{Version: 1, Backup: "bak", Digest: "00"}
	data, _ := json.Marshal(rec)
	if err := os.WriteFile(cleanupReceiptPath(target), data, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(target.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := processCleanupReceipt(target, root); err == nil {
		t.Fatal("accepted digest mismatch")
	}
}
