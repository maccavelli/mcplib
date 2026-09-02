package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ManagedInstaller composes standalone replacement with consumer lifecycle
// and definition reconciliation.
type ManagedInstaller struct {
	inner *StandaloneInstaller
	life  Lifecycle
	rec   Reconciler
}

// NewManagedInstaller wraps a standalone installer. life and rec are required.
func NewManagedInstaller(inner *StandaloneInstaller, life Lifecycle, rec Reconciler) (*ManagedInstaller, error) {
	if inner == nil {
		return nil, fmt.Errorf("selfupdate: managed installer requires a standalone installer")
	}
	if life == nil {
		return nil, fmt.Errorf("selfupdate: managed installer requires a lifecycle")
	}
	if rec == nil {
		return nil, fmt.Errorf("selfupdate: managed installer requires a reconciler")
	}
	return &ManagedInstaller{inner: inner, life: life, rec: rec}, nil
}

// ResolveTarget implements Installer.
func (m *ManagedInstaller) ResolveTarget(ctx context.Context) (Target, error) {
	return m.inner.ResolveTarget(ctx)
}

// Begin implements Installer.
func (m *ManagedInstaller) Begin(ctx context.Context, target Target) (InstallSession, error) {
	inner, err := m.inner.Begin(ctx, target)
	if err != nil {
		return nil, err
	}
	sess, ok := inner.(*installSession)
	if !ok {
		return nil, joinClose(fmt.Errorf("selfupdate: unexpected standalone session type"), inner)
	}
	return &managedSession{inner: sess, life: m.life, rec: m.rec}, nil
}

type managedSession struct {
	inner *installSession
	life  Lifecycle
	rec   Reconciler
}

func (s *managedSession) Target() Target { return s.inner.Target() }

func (s *managedSession) CreateStaging(ctx context.Context) (*os.File, string, error) {
	return s.inner.CreateStaging(ctx)
}

func (s *managedSession) Close() error { return s.inner.Close() }

func (s *managedSession) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	product := req.Product
	installed, err := s.life.Installed(ctx, product)
	if err != nil {
		return InstallResult{}, fmt.Errorf("selfupdate: probe installed: %w", errors.Join(ErrManagedInstall, err))
	}
	if !installed {
		return s.inner.Install(ctx, req)
	}
	running, err := s.life.Running(ctx, product)
	if err != nil {
		return InstallResult{}, fmt.Errorf("selfupdate: probe running: %w", errors.Join(ErrManagedInstall, err))
	}
	stopped := false
	if running {
		if err := s.life.Stop(ctx, product); err != nil {
			return InstallResult{}, fmt.Errorf("selfupdate: stop service: %w", errors.Join(ErrManagedInstall, err))
		}
		stopped = true
	}
	applied, err := s.inner.apply(ctx, req)
	if err != nil {
		return InstallResult{}, s.recover(ctx, product, applyResult{}, ReconcileResult{}, stopped, err)
	}
	receipt, recErr := s.rec.Reconcile(ctx, product, s.inner.target.Path)
	if recErr != nil {
		return InstallResult{}, s.recover(ctx, product, applied, receipt, true, recErr)
	}
	if err := s.life.Start(ctx, product); err != nil {
		return InstallResult{}, s.recover(ctx, product, applied, receipt, true, err)
	}
	if err := s.life.WaitHealthy(ctx, product); err != nil {
		return InstallResult{}, s.recover(ctx, product, applied, receipt, true, err)
	}
	pending, err := s.inner.commit(applied)
	result := InstallResult{
		Target:            s.inner.target.Path,
		Backup:            applied.backup,
		Applied:           true,
		ServiceInstalled:  true,
		ServiceWasRunning: running,
		PendingBackup:     pending,
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *managedSession) recover(ctx context.Context, product string, applied applyResult, receipt ReconcileResult, restart bool, origin error) error {
	var recov error
	if receipt.Changed || receipt.State != nil {
		if err := s.rec.Restore(ctx, product, receipt); err != nil {
			recov = errors.Join(recov, err)
		}
	}
	if applied.backup != "" {
		if err := s.inner.rollback(applied); err != nil {
			recov = errors.Join(recov, err)
		}
	}
	if restart {
		if err := s.life.Start(ctx, product); err != nil {
			recov = errors.Join(recov, err)
		} else if err := s.life.WaitHealthy(ctx, product); err != nil {
			recov = errors.Join(recov, err)
		}
	}
	return fmt.Errorf("selfupdate: managed install failed: %w", errors.Join(ErrManagedInstall, origin, recov))
}
