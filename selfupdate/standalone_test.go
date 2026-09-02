package selfupdate

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStandaloneInstallerTimeout(t *testing.T) {
	if _, err := NewStandaloneInstaller(InstallOptions{LockTimeout: -time.Second}); err == nil {
		t.Fatal("accepted negative lock timeout")
	}
	got, err := NewStandaloneInstaller(InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.lockTimeout != DefaultLockTimeout {
		t.Fatalf("timeout = %s", got.lockTimeout)
	}
}

func TestStandaloneResolveUsesPolicy(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if target.Path != want {
		t.Fatalf("path = %s", target.Path)
	}
}
