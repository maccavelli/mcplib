package selfupdate

import (
	"path/filepath"
	"testing"
)

func TestCleanupReceiptName(t *testing.T) {
	got := cleanupReceiptName("demo")
	if got != ".demo.selfupdate.cleanup" {
		t.Fatalf("%q", got)
	}
	path := cleanupReceiptPath(Target{Dir: filepath.FromSlash("/tmp/x"), Base: "demo"})
	if filepath.Base(path) != got {
		t.Fatalf("%q", path)
	}
}
