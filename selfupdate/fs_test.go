package selfupdate

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestInjectedRenameFailure(t *testing.T) {
	_, exe := withTempHome(t)
	prev := osRename
	osRename = func(oldpath, newpath string) error {
		return errors.New("injected rename failure")
	}
	t.Cleanup(func() { osRename = prev })

	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(exe)
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
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err == nil {
		t.Fatal("expected injected rename failure")
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(orig) {
		t.Fatalf("target mutated after failed rename: %q", got)
	}
}

func TestLockSymlinkRejected(t *testing.T) {
	home, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lock := home + string(os.PathSeparator) + lockBasename(target.Base)
	if err := os.Symlink(exe, lock); err != nil {
		t.Fatal(err)
	}
	_, err = inst.Begin(context.Background(), target)
	if err == nil {
		t.Fatal("accepted lock symlink")
	}
}
