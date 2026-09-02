//go:build windows

package selfupdate

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestIsUnsupportedDirSyncAccessDenied(t *testing.T) {
	err := &os.PathError{Op: "sync", Path: `.`, Err: windows.ERROR_ACCESS_DENIED}
	if !isUnsupportedSync(err) {
		t.Fatal("Windows directory ACCESS_DENIED must be treated as unsupported sync")
	}
	if isUnsupportedDirSync(windows.ERROR_INVALID_FUNCTION) {
		t.Fatal("unrelated Windows errors must remain fatal")
	}
}
