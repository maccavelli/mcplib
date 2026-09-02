package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveTargetHomeRoot(t *testing.T) {
	home, exe := withTempHome(t)
	got, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != want || got.Base != "demo" || got.Dir != filepath.Dir(want) {
		t.Fatalf("%+v want %s under %s (home %s)", got, want, filepath.Dir(want), home)
	}
}

func TestResolveTargetRejectsSymlink(t *testing.T) {
	home, exe := withTempHome(t)
	link := filepath.Join(home, "alias")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTarget(TargetPolicy{ExecutablePath: link}); err == nil {
		t.Fatal("accepted symlink invocation")
	}
}

func TestResolveTargetRejectsOutsideHome(t *testing.T) {
	_, _ = withTempHome(t)
	outside := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(outside, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTarget(TargetPolicy{ExecutablePath: outside}); err == nil {
		t.Fatal("accepted target outside home")
	}
}

func TestResolveTargetExtraRoot(t *testing.T) {
	_, _ = withTempHome(t)
	extra := t.TempDir()
	exe := filepath.Join(extra, "demo")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveTarget(TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{extra}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != want {
		t.Fatalf("%s", got.Path)
	}
}

func TestCanonicalizeRootRejectsFilesystemRoot(t *testing.T) {
	root := "/"
	if runtime.GOOS == "windows" {
		root = os.Getenv("SystemDrive") + `\`
	}
	if _, err := canonicalizeRoot(root); err == nil {
		t.Fatal("accepted filesystem root")
	}
}

func TestCanonicalizeRootRejectsRelative(t *testing.T) {
	if _, err := canonicalizeRoot("rel"); err == nil {
		t.Fatal("accepted relative root")
	}
}

func withTempHome(t *testing.T) (home, exe string) {
	t.Helper()
	home = t.TempDir()
	prev := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = prev })
	exe = filepath.Join(home, "demo")
	if err := os.WriteFile(exe, []byte("old-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, exe
}
