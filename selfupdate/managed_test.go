package selfupdate

import (
	"context"
	"errors"
	"os"
	"testing"
)

type fakeLife struct {
	installed bool
	running   bool
	stopErr   error
	startErr  error
	healthErr error
	stops     int
	starts    int
	healths   int
}

func (f *fakeLife) Installed(context.Context, string) (bool, error) { return f.installed, nil }
func (f *fakeLife) Running(context.Context, string) (bool, error)   { return f.running, nil }
func (f *fakeLife) Stop(context.Context, string) error {
	f.stops++
	return f.stopErr
}
func (f *fakeLife) Start(context.Context, string) error {
	f.starts++
	return f.startErr
}
func (f *fakeLife) WaitHealthy(context.Context, string) error {
	f.healths++
	return f.healthErr
}

type fakeRec struct {
	changed  bool
	err      error
	restores int
	restoreE error
}

func (f *fakeRec) Reconcile(context.Context, string, string) (ReconcileResult, error) {
	return ReconcileResult{Changed: f.changed, State: "unit"}, f.err
}
func (f *fakeRec) Restore(context.Context, string, ReconcileResult) error {
	f.restores++
	return f.restoreE
}

func managedEnv(t *testing.T, life *fakeLife, rec *fakeRec) (*ManagedInstaller, Target, InstallSession, string) {
	t.Helper()
	_, exe := withTempHome(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstaller(inner, life, rec)
	if err != nil {
		t.Fatal(err)
	}
	target, err := m.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := m.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return m, target, sess, exe
}

func stageNew(t *testing.T, sess InstallSession) string {
	t.Helper()
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
	return path
}

func TestManagedAbsentIsBinaryOnly(t *testing.T) {
	life := &fakeLife{}
	rec := &fakeRec{}
	_, _, sess, exe := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	res, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if life.stops != 0 || life.starts != 0 || rec.restores != 0 {
		t.Fatalf("service called on absent definition: %+v %+v", life, rec)
	}
	if !res.Applied || res.ServiceInstalled {
		t.Fatalf("%+v", res)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new-bytes" {
		t.Fatalf("%q", got)
	}
}

func TestManagedRunningStopReconcileStart(t *testing.T) {
	life := &fakeLife{installed: true, running: true}
	rec := &fakeRec{changed: true}
	_, _, sess, exe := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	res, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if life.stops != 1 || life.starts != 1 || life.healths != 1 {
		t.Fatalf("lifecycle counts %+v", life)
	}
	if !res.ServiceInstalled || !res.ServiceWasRunning {
		t.Fatalf("%+v", res)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new-bytes" {
		t.Fatalf("%q", got)
	}
}

func TestManagedDownHeals(t *testing.T) {
	life := &fakeLife{installed: true, running: false}
	rec := &fakeRec{changed: true}
	_, _, sess, _ := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	if _, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	}); err != nil {
		t.Fatal(err)
	}
	if life.stops != 0 || life.starts != 1 || life.healths != 1 {
		t.Fatalf("lifecycle counts %+v", life)
	}
}

func TestManagedHealthFailureRollsBack(t *testing.T) {
	life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
	rec := &fakeRec{changed: true}
	_, _, sess, exe := managedEnv(t, life, rec)
	orig, _ := os.ReadFile(exe)
	path := stageNew(t, sess)
	_, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	if rec.restores != 1 {
		t.Fatalf("restores = %d", rec.restores)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != string(orig) {
		t.Fatalf("binary not restored: %q", got)
	}
	if life.starts < 2 {
		t.Fatalf("expected restart of prior service, starts=%d", life.starts)
	}
}

func TestManagedReconcileFailureRollsBack(t *testing.T) {
	life := &fakeLife{installed: true, running: true}
	rec := &fakeRec{changed: true, err: errors.New("partial write")}
	_, _, sess, exe := managedEnv(t, life, rec)
	orig, _ := os.ReadFile(exe)
	path := stageNew(t, sess)
	_, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	if rec.restores != 1 {
		t.Fatalf("must restore even when Reconcile errors, restores=%d", rec.restores)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != string(orig) {
		t.Fatalf("binary not restored: %q", got)
	}
}

func TestManagedRollbackErrorJoined(t *testing.T) {
	life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
	rec := &fakeRec{changed: true, restoreE: errors.New("restore failed")}
	_, _, sess, _ := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	err := error(nil)
	_, err = sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err == nil || !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, rec.restoreE) && rec.restoreE != nil {
		if rec.restores != 1 {
			t.Fatalf("restores = %d", rec.restores)
		}
	}
}
