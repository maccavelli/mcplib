package selfupdate

import (
	"os"
	"path/filepath"
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
	// The root of the volume that holds a temporary directory: "/" on Unix,
	// the drive root on Windows. It is derived from the filesystem, not from
	// an environment variable, so the test does not depend on SystemDrive.
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	if _, err := canonicalizeRoot(root); err == nil {
		t.Fatal("accepted filesystem root")
	}
}

func TestCanonicalizeRootContract(t *testing.T) {
	base := t.TempDir()
	want, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirLink := filepath.Join(base, "dirlink")
	dirLinked := os.Symlink(want, dirLink) == nil
	fileLink := filepath.Join(base, "filelink")
	fileLinked := os.Symlink(file, fileLink) == nil

	t.Run("rejects a relative path that names an existing directory", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(base, "sub"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Chdir(base)
		if got, err := canonicalizeRoot("sub"); err == nil {
			t.Fatalf("accepted %q", got)
		}
	})
	t.Run("accepts a directory", func(t *testing.T) {
		got, err := canonicalizeRoot(base)
		if err != nil || got != want {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
	})
	t.Run("accepts a symlink to a directory and resolves it", func(t *testing.T) {
		if !dirLinked {
			t.Skip("symlinks unavailable")
		}
		got, err := canonicalizeRoot(dirLink)
		if err != nil || got != want {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
	})
	t.Run("rejects a symlink to a file", func(t *testing.T) {
		if !fileLinked {
			t.Skip("symlinks unavailable")
		}
		if got, err := canonicalizeRoot(fileLink); err == nil {
			t.Fatalf("accepted %q", got)
		}
	})
	for name, path := range map[string]string{
		"empty":          "",
		"relative":       "rel",
		"a file":         file,
		"a missing path": filepath.Join(base, "missing"),
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			if got, err := canonicalizeRoot(path); err == nil {
				t.Fatalf("accepted %q", got)
			}
		})
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
