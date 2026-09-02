package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStandaloneReplaceAndRollbackMaterial(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	f, path, err := sess.CreateStaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("new-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path, Size: 9, ReleaseDigest: "x", InstalledDigest: "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied {
		t.Fatal("not applied")
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-bytes" {
		t.Fatalf("got %q", got)
	}
	if res.Backup != "" {
		if _, err := os.Stat(res.Backup); !os.IsNotExist(err) {
			t.Fatalf("backup remained after commit: %v", err)
		}
	}
}

func TestChmodStagingUsesPermOnly(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old")
	if err := os.WriteFile(old, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(old)
	if err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(dir, "stage")
	if err := os.WriteFile(staging, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := chmodStaging(staging, info); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(staging)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != info.Mode().Perm() {
		t.Fatalf("perm = %s, want %s", st.Mode().Perm(), info.Mode().Perm())
	}
}

func TestSyncDirectoryOnTempRoot(t *testing.T) {
	dir := t.TempDir()
	if err := syncDirectory(dir); err != nil {
		t.Fatal(err)
	}
}

func TestIsUnsupportedSync(t *testing.T) {
	if !isUnsupportedSync(syscall.EINVAL) {
		t.Fatal("EINVAL must be treated as unsupported directory sync")
	}
	if isUnsupportedSync(errors.New("boom")) {
		t.Fatal("generic errors must remain fatal")
	}
}
