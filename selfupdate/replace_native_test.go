package selfupdate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("SELFUPDATE_NATIVE_HELPER") == "1" {
		nativeHelper()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func nativeHelper() {
	ready := os.Getenv("SELFUPDATE_READY")
	done := os.Getenv("SELFUPDATE_DONE")
	if ready != "" {
		_ = os.WriteFile(ready, []byte("ready"), 0o600)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if done != "" {
			if _, err := os.Stat(done); err == nil {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNativeReplaceRunningCopy(t *testing.T) {
	home, _ := withTempHome(t)
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "helper")
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, in, 0o755); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(home, "ready")
	done := filepath.Join(home, "done")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"SELFUPDATE_NATIVE_HELPER=1",
		"SELFUPDATE_READY="+ready,
		"SELFUPDATE_DONE="+done,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.WriteFile(done, []byte("x"), 0o600)
		_ = cmd.Wait()
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

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
	replacement := append([]byte{}, in...)
	replacement = append(replacement, []byte("REPLACED")...)
	if _, err := f.Write(replacement); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := sess.Install(context.Background(), InstallRequest{
		Product:  "helper",
		Artifact: StagedArtifact{Path: path, Size: int64(len(replacement))},
	})
	if err != nil && res.PendingBackup == "" {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(replacement) && string(got) != string(in) {
		t.Fatalf("target is neither old nor new (%d bytes)", len(got))
	}
	_ = os.WriteFile(done, []byte("x"), 0o600)
	waitErr := cmd.Wait()
	if waitErr != nil {
		t.Logf("helper exit: %v (allowed while replacing a running image)", waitErr)
	}
	final, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(final) != string(replacement) && string(final) != string(in) {
		t.Fatal("lost both old and new executable bytes")
	}
}
