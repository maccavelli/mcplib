package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionStagingAndCloseCleanup(t *testing.T) {
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
	f, path, err := sess.CreateStaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("staged")); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != target.Dir {
		t.Fatalf("staging dir = %s", filepath.Dir(path))
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("staging remained: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close must be idempotent: %v", err)
	}
}

func TestSessionRejectsForeignArtifact(t *testing.T) {
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
	foreign := filepath.Join(target.Dir, "foreign")
	if err := os.WriteFile(foreign, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: foreign, Size: 4},
	})
	if err == nil {
		t.Fatal("accepted foreign artifact")
	}
}
